package core

import (
	"context"
	"sync"
	"testing"

	logsapi "go.opentelemetry.io/otel/log"
	logsnoop "go.opentelemetry.io/otel/log/noop"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
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

	h := tel.OTel()
	if _, ok := h.TracerProvider.(tracenoop.TracerProvider); ok {
		t.Error("TracerProvider is noop, expected real")
	}
	if _, ok := h.MeterProvider.(metricnoop.MeterProvider); ok {
		t.Error("MeterProvider is noop, expected real")
	}
	if _, ok := h.LoggerProvider.(logsnoop.LoggerProvider); ok {
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

	h := tel.OTel()
	if _, ok := h.TracerProvider.(tracenoop.TracerProvider); ok {
		t.Error("TracerProvider is noop, expected real")
	}
	if _, ok := h.MeterProvider.(metricnoop.MeterProvider); ok {
		t.Error("MeterProvider is noop, expected real")
	}
	if _, ok := h.LoggerProvider.(logsnoop.LoggerProvider); ok {
		t.Error("LoggerProvider is noop, expected real")
	}
}

// fakeSpanExporter / fakeLogExporter / fakeMetricExporter prove an
// override exporter (a) replaces the OTLP exporter and (b) enables that
// signal even when OTLPEndpoint is unset.

type fakeSpanExporter struct{ mu sync.Mutex; spans []sdktrace.ReadOnlySpan }

func (f *fakeSpanExporter) ExportSpans(_ context.Context, s []sdktrace.ReadOnlySpan) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.spans = append(f.spans, s...)
	return nil
}
func (f *fakeSpanExporter) Shutdown(context.Context) error { return nil }

type fakeLogExporter struct{ mu sync.Mutex; n int }

func (f *fakeLogExporter) Export(_ context.Context, records []sdklog.Record) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n += len(records)
	return nil
}
func (f *fakeLogExporter) ForceFlush(context.Context) error { return nil }
func (f *fakeLogExporter) Shutdown(context.Context) error   { return nil }

type fakeMetricExporter struct{}

func (f *fakeMetricExporter) Temporality(sdkmetric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}
func (f *fakeMetricExporter) Aggregation(sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return sdkmetric.DefaultAggregationSelector(sdkmetric.InstrumentKindCounter)
}
func (f *fakeMetricExporter) Export(context.Context, *metricdata.ResourceMetrics) error { return nil }
func (f *fakeMetricExporter) ForceFlush(context.Context) error                          { return nil }
func (f *fakeMetricExporter) Shutdown(context.Context) error                            { return nil }

func TestExporterOverrideEnablesSignalWithoutOTLPEndpoint(t *testing.T) {
	opts := baseOpts()
	opts.OTLPEndpoint = "" // no OTLP — overrides alone must enable signals
	opts.TraceExporter = &fakeSpanExporter{}
	opts.LogExporter = &fakeLogExporter{}
	opts.MetricExporter = &fakeMetricExporter{}

	tel, err := Init(context.Background(), opts)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer tel.Shutdown(context.Background())

	h := tel.OTel()
	if _, ok := h.TracerProvider.(tracenoop.TracerProvider); ok {
		t.Error("TracerProvider stayed noop despite TraceExporter override")
	}
	if _, ok := h.MeterProvider.(metricnoop.MeterProvider); ok {
		t.Error("MeterProvider stayed noop despite MetricExporter override")
	}
	if _, ok := h.LoggerProvider.(logsnoop.LoggerProvider); ok {
		t.Error("LoggerProvider stayed noop despite LogExporter override")
	}
}

func TestTraceExporterOverrideReceivesSpans(t *testing.T) {
	fake := &fakeSpanExporter{}
	opts := baseOpts()
	opts.TraceExporter = fake
	tel, err := Init(context.Background(), opts)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	_, log := tel.Tracer.Start(context.Background(), "x")
	log.Span().End()
	if err := tel.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if err := tel.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.spans) == 0 {
		t.Error("override TraceExporter received no spans")
	}
}

// keep otel/log import used in older test code paths
var _ logsapi.LoggerProvider = (logsapi.LoggerProvider)(nil)
