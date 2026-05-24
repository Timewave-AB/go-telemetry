package telemetry

import (
	"context"
	"log/slog"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func newTestTracer() (*Tracer, *ctxRecordingHandler) {
	h := newCtxRecordingHandler()
	log := newLogger(slog.New(h))
	tp := sdktrace.NewTracerProvider()
	return newTracer(tp.Tracer("test"), log), h
}

func TestTracerStartReturnsCtxSpanLog(t *testing.T) {
	tr, _ := newTestTracer()
	ctx, span, log := tr.Start(context.Background(), "boot")
	if ctx == nil {
		t.Error("ctx is nil")
	}
	if span == nil {
		t.Error("span is nil")
	}
	if log == nil {
		t.Error("log is nil")
	}
	defer span.End()
}

func TestTracerStartCtxCarriesNewSpan(t *testing.T) {
	tr, _ := newTestTracer()
	ctx, span, _ := tr.Start(context.Background(), "boot")
	defer span.End()
	got := trace.SpanFromContext(ctx)
	if !sameSpan(got.SpanContext(), span.SpanContext()) {
		t.Errorf("returned ctx does not carry the new span")
	}
}

func TestTracerStartLoggerCarriesSpanCtx(t *testing.T) {
	tr, h := newTestTracer()
	_, span, log := tr.Start(context.Background(), "boot")
	defer span.End()

	log.Info("inside-span")
	recs := h.records()
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	gotSpan := trace.SpanFromContext(recs[0].ctx)
	if !sameSpan(gotSpan.SpanContext(), span.SpanContext()) {
		t.Errorf("log emitted with wrong span context: got %v, want %v",
			gotSpan.SpanContext(), span.SpanContext())
	}
}

func sameSpan(a, b trace.SpanContext) bool {
	return a.TraceID() == b.TraceID() && a.SpanID() == b.SpanID()
}

func TestTracerStartLoggerSeparateFromTopLevel(t *testing.T) {
	// The Logger returned from Start must not mutate the Tracer's base
	// logger — sibling spans need independent bindings.
	tr, h := newTestTracer()
	_, spanA, logA := tr.Start(context.Background(), "a")
	defer spanA.End()
	_, spanB, logB := tr.Start(context.Background(), "b")
	defer spanB.End()

	logA.Info("from-a")
	logB.Info("from-b")
	recs := h.records()
	if len(recs) != 2 {
		t.Fatalf("expected 2 records, got %d", len(recs))
	}
	if !sameSpan(trace.SpanFromContext(recs[0].ctx).SpanContext(), spanA.SpanContext()) {
		t.Error("first record carried wrong span")
	}
	if !sameSpan(trace.SpanFromContext(recs[1].ctx).SpanContext(), spanB.SpanContext()) {
		t.Error("second record carried wrong span")
	}
}

func TestTracerStartPassesOptionsThrough(t *testing.T) {
	tr, _ := newTestTracer()
	_, span, _ := tr.Start(context.Background(), "boot",
		trace.WithSpanKind(trace.SpanKindServer))
	defer span.End()
	rw, ok := span.(sdktrace.ReadWriteSpan)
	if !ok {
		t.Fatalf("expected ReadWriteSpan, got %T", span)
	}
	if rw.SpanKind() != trace.SpanKindServer {
		t.Errorf("span kind = %v, want Server", rw.SpanKind())
	}
}
