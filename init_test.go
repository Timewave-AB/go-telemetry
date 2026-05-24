package telemetry

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
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

	if _, ok := tel.TracerProvider.(tracenoop.TracerProvider); !ok {
		t.Errorf("TracerProvider = %T, want noop.TracerProvider", tel.TracerProvider)
	}
	if _, ok := tel.MeterProvider.(metricnoop.MeterProvider); !ok {
		t.Errorf("MeterProvider = %T, want noop.MeterProvider", tel.MeterProvider)
	}
	if _, ok := tel.LoggerProvider.(logsnoop.LoggerProvider); !ok {
		t.Errorf("LoggerProvider = %T, want noop.LoggerProvider", tel.LoggerProvider)
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
	if tel.Propagator == (propagation.TextMapPropagator)(nil) {
		t.Error("Propagator is nil")
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
