package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"sync"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	logsapi "go.opentelemetry.io/otel/log"
	logsnoop "go.opentelemetry.io/otel/log/noop"
	metricapi "go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	traceapi "go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

// Telemetry is the bundle returned by Init.
//
// The day-to-day handles are Logger, Tracer, and Meter. Tracer.Start
// returns a *SpanLogger so logs in that scope correlate to the span
// without the caller having to thread a context through every log call:
//
//	ctx, log := tel.Tracer.Start(ctx, "boot")
//	defer log.Span().End()
//	log.Info("connected to the boot span")
//
// The OTel providers and propagator are reachable via OTel() for wiring
// third-party instrumentation libraries (otelhttp, otelgrpc, …).
type Telemetry struct {
	Logger *Logger
	Tracer *Tracer
	Meter  metricapi.Meter

	loggerProvider logsapi.LoggerProvider
	meterProvider  metricapi.MeterProvider
	propagator     propagation.TextMapPropagator
	tracerProvider traceapi.TracerProvider

	flush    func(context.Context) error
	shutdown func(context.Context) error
}

// OTelHandles bundles the OpenTelemetry providers and propagator. Returned
// by (*Telemetry).OTel(). Use these when wiring third-party
// instrumentation libraries that take a provider/propagator directly.
type OTelHandles struct {
	LoggerProvider logsapi.LoggerProvider
	MeterProvider  metricapi.MeterProvider
	Propagator     propagation.TextMapPropagator
	TracerProvider traceapi.TracerProvider
}

// Init builds a Telemetry bundle. Init never reads or writes OTel globals
// unless Options.OnError is set (in which case otel.SetErrorHandler is
// installed so SDK errors reach the caller).
//
// Call (*Telemetry).Shutdown to flush exporters; it is idempotent and
// respects the caller's ctx for its timeout budget.
func Init(ctx context.Context, opts Options) (*Telemetry, error) {
	if opts.ServiceName == "" {
		return nil, errors.New("telemetry: ServiceName is required")
	}
	if opts.Transport != TransportGRPC && opts.Transport != TransportHTTP {
		return nil, fmt.Errorf("telemetry: unknown Transport value %d", opts.Transport)
	}
	version := resolveServiceVersion(opts.ServiceVersion)
	if version == "" {
		return nil, errors.New("telemetry: ServiceVersion is required (and debug.ReadBuildInfo returned nothing)")
	}

	level, unknown := parseLogLevel(opts.Level)
	if unknown {
		fmt.Fprintf(os.Stderr, "telemetry: unknown log level %q, defaulting to info\n", opts.Level)
	}

	res, err := buildResource(opts.ServiceName, version)
	if err != nil {
		return nil, fmt.Errorf("telemetry: build resource: %w", err)
	}

	tel := &Telemetry{
		loggerProvider: logsnoop.NewLoggerProvider(),
		meterProvider:  metricnoop.NewMeterProvider(),
		propagator: propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
		tracerProvider: tracenoop.NewTracerProvider(),
	}

	// Per-signal enablement: a signal turns on if OTLPEndpoint is set OR
	// that signal's exporter override is set. This lets tests/non-OTLP
	// users enable a single signal without dragging the others along.
	logsOn := opts.OTLPEndpoint != "" || opts.LogExporter != nil
	tracesOn := opts.OTLPEndpoint != "" || opts.TraceExporter != nil
	metricsOn := opts.OTLPEndpoint != "" || opts.MetricExporter != nil

	var initOrder []func(context.Context) error // shutdowns in init order
	var flushes []func(context.Context) error

	rollback := func() {
		for i := len(initOrder) - 1; i >= 0; i-- {
			_ = initOrder[i](ctx)
		}
	}

	if logsOn {
		lp, lShutdown, lFlush, err := newLoggerProvider(ctx, opts, res)
		if err != nil {
			return nil, fmt.Errorf("telemetry: logs: %w", err)
		}
		tel.loggerProvider = lp
		initOrder = append(initOrder, lShutdown)
		flushes = append(flushes, lFlush)
	}
	if tracesOn {
		tp, tShutdown, tFlush, err := newTracerProvider(ctx, opts, res)
		if err != nil {
			rollback()
			return nil, fmt.Errorf("telemetry: traces: %w", err)
		}
		tel.tracerProvider = tp
		initOrder = append(initOrder, tShutdown)
		flushes = append(flushes, tFlush)
	}
	if metricsOn {
		mp, mShutdown, mFlush, err := newMeterProvider(ctx, opts, res)
		if err != nil {
			rollback()
			return nil, fmt.Errorf("telemetry: metrics: %w", err)
		}
		tel.meterProvider = mp
		initOrder = append(initOrder, mShutdown)
		flushes = append(flushes, mFlush)
	}

	// Shutdowns run in reverse-of-init order.
	shutdowns := make([]func(context.Context) error, len(initOrder))
	for i, fn := range initOrder {
		shutdowns[len(initOrder)-1-i] = fn
	}

	tel.Meter = tel.meterProvider.Meter(opts.ServiceName)
	baseLog := newLogger(buildSlog(opts.ServiceName, level, tel.loggerProvider, opts.OnError))
	tel.Logger = baseLog
	tel.Tracer = newTracer(tel.tracerProvider.Tracer(opts.ServiceName), baseLog)

	tel.shutdown = makeShutdown(shutdowns)
	tel.flush = makeFlush(flushes)

	if opts.OnError != nil {
		otel.SetErrorHandler(otel.ErrorHandlerFunc(opts.OnError))
	}
	return tel, nil
}

// OTel returns the bundle's OpenTelemetry providers and propagator.
// Use these when wiring third-party instrumentation libraries (otelhttp,
// otelgrpc, otelsql, …). Most callers never need this.
func (t *Telemetry) OTel() OTelHandles {
	if t == nil {
		return OTelHandles{}
	}
	return OTelHandles{
		LoggerProvider: t.loggerProvider,
		MeterProvider:  t.meterProvider,
		Propagator:     t.propagator,
		TracerProvider: t.tracerProvider,
	}
}

// Flush forces all enabled exporters to flush their pending data. Useful
// for short-lived jobs, k8s preStop hooks, or post-crash diagnostics that
// need data on the wire before continuing.
func (t *Telemetry) Flush(ctx context.Context) error {
	if t == nil || t.flush == nil {
		return nil
	}
	return t.flush(ctx)
}

// Shutdown flushes exporters and tears down providers. Honours the
// caller's ctx — pick the deadline that matches your environment (a few
// seconds for a sidecar, longer for k8s preStop). Safe to call multiple
// times; the second call is a no-op.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	if t == nil || t.shutdown == nil {
		return nil
	}
	return t.shutdown(ctx)
}

// buildSlog always has the text handler; if a real LoggerProvider is
// in play, also fan out to it via the otelslog bridge. onError, if
// non-nil, receives per-handler write failures observed by the multi
// handler — slog itself discards them otherwise.
func buildSlog(serviceName string, level slog.Level, lp logsapi.LoggerProvider, onError func(error)) *slog.Logger {
	text := &textHandler{level: level, writer: os.Stdout}
	if _, isNoop := lp.(logsnoop.LoggerProvider); isNoop {
		return slog.New(text)
	}
	bridge := otelslog.NewHandler(serviceName, otelslog.WithLoggerProvider(lp))
	return slog.New(&multiHandler{handlers: []slog.Handler{text, bridge}, onError: onError})
}

// makeShutdown returns a callback that runs all per-provider shutdowns
// under the caller's ctx, collecting errors. Safe to invoke twice; the
// second call is a no-op.
func makeShutdown(fns []func(context.Context) error) func(context.Context) error {
	var once sync.Once
	var result error
	return func(ctx context.Context) error {
		once.Do(func() {
			var errs []error
			for _, fn := range fns {
				if err := fn(ctx); err != nil {
					errs = append(errs, err)
				}
			}
			result = errors.Join(errs...)
		})
		return result
	}
}

// makeFlush returns a callback that runs all per-provider ForceFlush
// under the caller's ctx, collecting errors. Unlike Shutdown, Flush may
// be invoked many times.
func makeFlush(fns []func(context.Context) error) func(context.Context) error {
	return func(ctx context.Context) error {
		var errs []error
		for _, fn := range fns {
			if err := fn(ctx); err != nil {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	}
}

// resolveServiceVersion prefers the explicit value; falls back to
// debug.ReadBuildInfo; final fallback "unknown" so the SDK never sees empty.
func resolveServiceVersion(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "unknown"
}
