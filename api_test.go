package telemetry_test

import (
	"context"
	"testing"

	"github.com/Timewave-AB/go-telemetry"
	"go.opentelemetry.io/otel/propagation"
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
	t.Cleanup(func() {
		if err := tel.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})

	if tel.Logger == nil || tel.Tracer == nil {
		t.Fatal("Logger and Tracer must be non-nil")
	}
	if tel.Meter == nil {
		t.Fatal("Meter must be non-nil")
	}

	// OTel escape hatches grouped behind OTel().
	h := tel.OTel()
	if h.LoggerProvider == nil || h.TracerProvider == nil || h.MeterProvider == nil || h.Propagator == nil {
		t.Fatal("OTel handles must be non-nil")
	}

	// Flush is a no-op on noop providers but must be callable.
	if err := tel.Flush(context.Background()); err != nil {
		t.Errorf("Flush: %v", err)
	}

	// Tracer.Extract joins an incoming W3C trace-context; the returned
	// ctx feeds Start. Empty carrier → fresh trace, no error.
	extracted := tel.Tracer.Extract(context.Background(), propagation.MapCarrier{})

	// Tracer.Start: (ctx, *SpanLogger).
	_, spanLog := tel.Tracer.Start(extracted, "test-span")
	spanLog.Info("inside-span")
	spanLog.With("k", "v").Warn("attr-attached")
	spanLog.Span().End()
	_ = tel.Tracer.OTel()

	// Top-level Logger surface (ctx-free).
	tel.Logger.Debug("d")
	tel.Logger.Verbose("v")
	tel.Logger.Info("i")
	tel.Logger.Warn("w")
	tel.Logger.Error("e")
	tel.Logger.With("k", "v").Info("with attr")
	if tel.Logger.Slog() == nil {
		t.Fatal("Slog() must return non-nil")
	}

	// Level constants compile in spanLog-compatible expressions.
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

	// SpanLogger type alias is reachable.
	var _ *telemetry.SpanLogger = spanLog
	var _ telemetry.OTelHandles = h
}
