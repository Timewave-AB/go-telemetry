package core

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	logsapi "go.opentelemetry.io/otel/log"
	logsnoop "go.opentelemetry.io/otel/log/noop"
	metricapi "go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

func baseOpts() Options {
	return Options{
		ServiceName:    "test-svc",
		ServiceVersion: "0.0.1",
		Level:          "info",
	}
}

func TestInitRejectsEmptyServiceName(t *testing.T) {
	opts := baseOpts()
	opts.ServiceName = ""
	_, err := Init(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for empty ServiceName")
	}
}

func TestInitRejectsUnknownTransport(t *testing.T) {
	opts := baseOpts()
	opts.Transport = Transport(99)
	_, err := Init(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for unknown Transport value")
	}
	if !strings.Contains(err.Error(), "Transport") {
		t.Errorf("error should mention Transport, got %q", err.Error())
	}
}

func TestInitAutoDetectsServiceVersionWhenEmpty(t *testing.T) {
	opts := baseOpts()
	opts.ServiceVersion = "" // should fall back to ReadBuildInfo or "unknown"
	tel, err := Init(context.Background(), opts)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer tel.Shutdown(context.Background())
	if tel == nil {
		t.Fatal("expected bundle")
	}
}

func TestInitNoOTLPUsesNoopProviders(t *testing.T) {
	opts := baseOpts()
	opts.OTLPEndpoint = ""
	tel, err := Init(context.Background(), opts)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer tel.Shutdown(context.Background())

	h := tel.OTel()
	if _, ok := h.TracerProvider.(tracenoop.TracerProvider); !ok {
		t.Errorf("TracerProvider = %T, want noop.TracerProvider", h.TracerProvider)
	}
	if _, ok := h.MeterProvider.(metricnoop.MeterProvider); !ok {
		t.Errorf("MeterProvider = %T, want noop.MeterProvider", h.MeterProvider)
	}
	if _, ok := h.LoggerProvider.(logsnoop.LoggerProvider); !ok {
		t.Errorf("LoggerProvider = %T, want noop.LoggerProvider", h.LoggerProvider)
	}
	if tel.Tracer == nil {
		t.Error("Tracer is nil")
	}
	if tel.Meter == (metricapi.Meter)(nil) {
		t.Error("Meter is nil")
	}
	if tel.Logger == nil {
		t.Error("Logger is nil")
	}
	if h.Propagator == (propagation.TextMapPropagator)(nil) {
		t.Error("Propagator is nil")
	}
}

func TestOTelHandlesEmptyOnNilTelemetry(t *testing.T) {
	var tel *Telemetry
	if h := tel.OTel(); h.LoggerProvider != nil || h.TracerProvider != nil || h.MeterProvider != nil || h.Propagator != nil {
		t.Errorf("nil Telemetry.OTel() should return zero OTelHandles, got %+v", h)
	}
}

func TestInitDoesNotTouchErrorHandlerWhenOnErrorUnset(t *testing.T) {
	// Install a marker handler before Init. If Init leaves the global
	// error handler alone (the documented behaviour for OnError == nil),
	// our marker remains active and otel.Handle reaches it.
	var hit int
	installMarkerErrorHandler(func(error) { hit++ })
	defer restoreDefaultOtelErrorHandler()

	opts := baseOpts()
	tel, err := Init(context.Background(), opts)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer tel.Shutdown(context.Background())

	otelHandle(errors.New("probe"))
	if hit != 1 {
		t.Errorf("Init replaced our marker handler; hit=%d, want 1", hit)
	}
}

func TestInitOnErrorReceivesOTelSDKErrors(t *testing.T) {
	var captured []error
	var mu sync.Mutex
	opts := baseOpts()
	opts.OnError = func(err error) {
		mu.Lock()
		defer mu.Unlock()
		captured = append(captured, err)
	}
	tel, err := Init(context.Background(), opts)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer tel.Shutdown(context.Background())
	defer restoreDefaultOtelErrorHandler()

	otelHandle(errors.New("sdk-failure"))

	mu.Lock()
	defer mu.Unlock()
	if len(captured) != 1 || captured[0].Error() != "sdk-failure" {
		t.Errorf("OnError did not receive SDK error; captured = %v", captured)
	}
}

func TestInitDoesNotTouchGlobals(t *testing.T) {
	beforeT := getDefaultTracerProvider()
	beforeM := getDefaultMeterProvider()
	beforeP := getDefaultPropagator()

	opts := baseOpts()
	opts.OTLPEndpoint = ""
	tel, err := Init(context.Background(), opts)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer tel.Shutdown(context.Background())

	if getDefaultTracerProvider() != beforeT {
		t.Error("global TracerProvider was modified")
	}
	if getDefaultMeterProvider() != beforeM {
		t.Error("global MeterProvider was modified")
	}
	if getDefaultPropagator() != beforeP {
		t.Error("global TextMapPropagator was modified")
	}
	_ = logsapi.Severity(0) // touch the logs api import
}

func TestInitWritesNothingToStderrOnEmptyLevel(t *testing.T) {
	stderr, restore := captureStderr(t)

	opts := baseOpts()
	opts.Level = "" // silent default
	tel, err := Init(context.Background(), opts)
	if err != nil {
		restore()
		t.Fatalf("Init: %v", err)
	}
	tel.Shutdown(context.Background())
	restore()

	if got := stderr.String(); got != "" {
		t.Errorf("expected silent default for empty level, got stderr=%q", got)
	}
}

func TestInitWarnsOnUnknownLevel(t *testing.T) {
	stderr, restore := captureStderr(t)

	opts := baseOpts()
	opts.Level = "louder-than-shouting"
	tel, err := Init(context.Background(), opts)
	if err != nil {
		restore()
		t.Fatalf("Init: %v", err)
	}
	tel.Shutdown(context.Background())
	restore()

	if !strings.Contains(stderr.String(), "louder-than-shouting") {
		t.Errorf("expected stderr warning mentioning the input, got %q", stderr.String())
	}
}

func TestInitLoggerEmitsToStdout(t *testing.T) {
	stdout, restore := captureStdout(t)

	opts := baseOpts()
	tel, err := Init(context.Background(), opts)
	if err != nil {
		restore()
		t.Fatalf("Init: %v", err)
	}
	tel.Logger.Info("hello", "k", "v")
	tel.Shutdown(context.Background())
	restore()

	if !strings.Contains(stdout.String(), `[INFO] msg="hello"`) {
		t.Errorf("stdout missing expected line, got %q", stdout.String())
	}
}

func TestShutdownBoundedByTimeout(t *testing.T) {
	opts := baseOpts()
	tel, err := Init(context.Background(), opts)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	start := time.Now()
	if err := tel.Shutdown(context.Background()); err != nil {
		t.Errorf("shutdown returned err on noop providers: %v", err)
	}
	if took := time.Since(start); took > time.Second {
		t.Errorf("shutdown took %v on noop providers — expected near-instant", took)
	}
}

func TestShutdownUsesCallerContextNotInternalTimeout(t *testing.T) {
	// makeShutdown must respect the caller's ctx instead of imposing its
	// own bound, so callers (k8s preStop, CLIs) can choose the budget.
	slow := func(ctx context.Context) error {
		select {
		case <-time.After(10 * time.Second):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	sh := makeShutdown([]func(context.Context) error{slow})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := sh(ctx)
	if took := time.Since(start); took > time.Second {
		t.Errorf("shutdown did not honour caller ctx, took %v", took)
	}
	if err == nil {
		t.Error("expected ctx deadline error")
	}
}

func TestFlushOnNoopProvidersReturnsNil(t *testing.T) {
	opts := baseOpts()
	tel, err := Init(context.Background(), opts)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer tel.Shutdown(context.Background())
	if err := tel.Flush(context.Background()); err != nil {
		t.Errorf("Flush on noop providers returned err: %v", err)
	}
}

func TestFlushSafeOnNilTelemetry(t *testing.T) {
	var tel *Telemetry
	if err := tel.Flush(context.Background()); err != nil {
		t.Errorf("Flush on nil returned err: %v", err)
	}
}

func TestShutdownSafeWhenCalledTwice(t *testing.T) {
	opts := baseOpts()
	tel, err := Init(context.Background(), opts)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := tel.Shutdown(context.Background()); err != nil {
		t.Errorf("first shutdown: %v", err)
	}
	if err := tel.Shutdown(context.Background()); err != nil {
		t.Errorf("second shutdown: %v", err)
	}
}

// captureStdout/Stderr swap the OS-level FDs for the duration of a test
// because parseLogLevel writes its warning via fmt.Fprintf(os.Stderr, ...)
// (the call site is fixed; we can't inject a writer there).
func captureStdout(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()
	return captureFD(t, &os.Stdout)
}
func captureStderr(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()
	return captureFD(t, &os.Stderr)
}
func captureFD(t *testing.T, target **os.File) (*bytes.Buffer, func()) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := *target
	*target = w
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()
	return &buf, func() {
		_ = w.Close()
		<-done
		*target = orig
		_ = r.Close()
	}
}

var _ = slog.Default
