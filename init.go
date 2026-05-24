package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
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
// returns a per-span *Logger so logs in that scope correlate without the
// caller having to thread a context through every log call:
//
//	ctx, span, log := tel.Tracer.Start(ctx, "boot")
//	defer span.End()
//	log.Info("connected to the boot span")
//
// The Provider fields and Propagator are an escape hatch for wiring
// third-party instrumentation libraries (otelhttp, otelgrpc, otelsql,
// …). Ignore them otherwise — Init never registers any OTel globals.
type Telemetry struct {
	Logger *Logger
	Tracer *Tracer
	Meter  metricapi.Meter

	LoggerProvider logsapi.LoggerProvider
	MeterProvider  metricapi.MeterProvider
	Propagator     propagation.TextMapPropagator
	TracerProvider traceapi.TracerProvider

	shutdown func(context.Context) error
}

const shutdownTimeout = 5 * time.Second

// Init builds a Telemetry bundle. Init never reads or writes OTel globals.
// Call (*Telemetry).Shutdown to flush exporters; it is idempotent.
func Init(ctx context.Context, opts Options) (*Telemetry, error) {
	if opts.ServiceName == "" {
		return nil, errors.New("telemetry: ServiceName is required")
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

	propagator := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)

	tel := &Telemetry{
		LoggerProvider: logsnoop.NewLoggerProvider(),
		MeterProvider:  metricnoop.NewMeterProvider(),
		Propagator:     propagator,
		TracerProvider: tracenoop.NewTracerProvider(),
	}

	var shutdowns []func(context.Context) error

	if opts.OTLPEndpoint != "" {
		lp, lShutdown, err := newLoggerProvider(ctx, opts, res)
		if err != nil {
			return nil, fmt.Errorf("telemetry: logs: %w", err)
		}
		tp, tShutdown, err := newTracerProvider(ctx, opts, res)
		if err != nil {
			_ = lShutdown(ctx)
			return nil, fmt.Errorf("telemetry: traces: %w", err)
		}
		mp, mShutdown, err := newMeterProvider(ctx, opts, res)
		if err != nil {
			_ = lShutdown(ctx)
			_ = tShutdown(ctx)
			return nil, fmt.Errorf("telemetry: metrics: %w", err)
		}
		tel.LoggerProvider = lp
		tel.TracerProvider = tp
		tel.MeterProvider = mp
		// Reverse-of-init order on shutdown: metrics, traces, logs.
		shutdowns = append(shutdowns, mShutdown, tShutdown, lShutdown)
	}

	tel.Meter = tel.MeterProvider.Meter(opts.ServiceName)
	baseLog := newLogger(buildSlog(opts.ServiceName, level, tel.LoggerProvider))
	tel.Logger = baseLog
	tel.Tracer = newTracer(tel.TracerProvider.Tracer(opts.ServiceName), baseLog)

	tel.shutdown = makeShutdown(shutdowns)
	return tel, nil
}

// Shutdown flushes exporters under a bounded timeout. Safe to call
// multiple times — the second call is a no-op.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	if t == nil || t.shutdown == nil {
		return nil
	}
	return t.shutdown(ctx)
}

// buildSlog always has the text handler; if a real LoggerProvider is
// in play, also fan out to it via the otelslog bridge.
func buildSlog(serviceName string, level slog.Level, lp logsapi.LoggerProvider) *slog.Logger {
	text := &textHandler{level: level, writer: os.Stdout}
	if _, isNoop := lp.(logsnoop.LoggerProvider); isNoop {
		return slog.New(text)
	}
	bridge := otelslog.NewHandler(serviceName, otelslog.WithLoggerProvider(lp))
	return slog.New(&multiHandler{handlers: []slog.Handler{text, bridge}})
}

// makeShutdown returns a callback that runs all per-provider shutdowns
// under a bounded timeout, collecting errors. Safe to invoke twice;
// the second call is a no-op.
func makeShutdown(fns []func(context.Context) error) func(context.Context) error {
	var once sync.Once
	var result error
	return func(ctx context.Context) error {
		once.Do(func() {
			cctx, cancel := context.WithTimeout(ctx, shutdownTimeout)
			defer cancel()
			var errs []error
			for _, fn := range fns {
				if err := fn(cctx); err != nil {
					errs = append(errs, err)
				}
			}
			result = errors.Join(errs...)
		})
		return result
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
