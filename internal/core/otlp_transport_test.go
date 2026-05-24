package core

import (
	"context"
	"testing"

	logsnoop "go.opentelemetry.io/otel/log/noop"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

// Both transports must build real (non-noop) providers when OTLPEndpoint
// is set. Exporters dial lazily, so a fake endpoint is fine.

func TestInitGRPCTransportBuildsRealProviders(t *testing.T) {
	opts := baseOpts()
	opts.OTLPEndpoint = "127.0.0.1:14317"
	opts.Transport = TransportGRPC
	tel, err := Init(context.Background(), opts)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer tel.Shutdown(context.Background())

	if _, ok := tel.TracerProvider.(tracenoop.TracerProvider); ok {
		t.Error("TracerProvider is noop, expected real")
	}
	if _, ok := tel.MeterProvider.(metricnoop.MeterProvider); ok {
		t.Error("MeterProvider is noop, expected real")
	}
	if _, ok := tel.LoggerProvider.(logsnoop.LoggerProvider); ok {
		t.Error("LoggerProvider is noop, expected real")
	}
}

func TestInitHTTPTransportBuildsRealProviders(t *testing.T) {
	opts := baseOpts()
	opts.OTLPEndpoint = "127.0.0.1:14318"
	opts.Transport = TransportHTTP
	tel, err := Init(context.Background(), opts)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer tel.Shutdown(context.Background())

	if _, ok := tel.TracerProvider.(tracenoop.TracerProvider); ok {
		t.Error("TracerProvider is noop, expected real")
	}
	if _, ok := tel.MeterProvider.(metricnoop.MeterProvider); ok {
		t.Error("MeterProvider is noop, expected real")
	}
	if _, ok := tel.LoggerProvider.(logsnoop.LoggerProvider); ok {
		t.Error("LoggerProvider is noop, expected real")
	}
}
