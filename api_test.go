package telemetry_test

import (
	"context"
	"testing"

	"github.com/Timewave-AB/go-telemetry"
)

// Locks down the public surface documented in README.md. If any
// re-export goes missing or changes shape, this stops compiling.
func TestPublicAPISurface(t *testing.T) {
	// Transport constants are distinct.
	if telemetry.TransportGRPC == telemetry.TransportHTTP {
		t.Fatal("TransportGRPC and TransportHTTP must differ")
	}
	if got := telemetry.TransportGRPC.String(); got != "grpc" {
		t.Fatalf("TransportGRPC.String() = %q, want grpc", got)
	}

	// Init returns a usable bundle when OTLP is disabled.
	tel, err := telemetry.Init(context.Background(), telemetry.Options{
		ServiceName:      "api-surface-test",
		ServiceVersion:   "0.0.0",
		Level:            "info",
		Transport:        telemetry.TransportGRPC,
		TraceSampleRatio: 0,
	})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = tel.Shutdown(context.Background()) })

	if tel.Logger == nil || tel.Tracer == nil {
		t.Fatal("Logger and Tracer must be non-nil")
	}
	if tel.Meter == nil || tel.LoggerProvider == nil || tel.TracerProvider == nil || tel.MeterProvider == nil || tel.Propagator == nil {
		t.Fatal("escape-hatch providers must be non-nil")
	}

	// Tracer.Start triple return.
	_, span, log := tel.Tracer.Start(context.Background(), "test-span")
	span.End()
	_ = tel.Tracer.OTel()

	// Logger surface.
	log.Debug("d")
	log.Verbose("v")
	log.Info("i")
	log.Warn("w")
	log.Error("e")
	log.With("k", "v").Info("with attr")
	if log.Slog() == nil {
		t.Fatal("Slog() must return non-nil")
	}

	// Level constants compile in slog-compatible expressions.
	levels := []any{
		telemetry.LevelDebug,
		telemetry.LevelVerbose,
		telemetry.LevelInfo,
		telemetry.LevelWarning,
		telemetry.LevelError,
	}
	if len(levels) != 5 {
		t.Fatal("five levels expected")
	}
}
