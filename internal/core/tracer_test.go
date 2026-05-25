package core

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

func TestTracerStartReturnsCtxAndSpanLogger(t *testing.T) {
	tr, _ := newTestTracer()
	ctx, log := tr.Start(context.Background(), "boot")
	if ctx == nil {
		t.Error("ctx is nil")
	}
	if log == nil {
		t.Error("log is nil")
	}
	if log.Span() == nil {
		t.Error("log.Span() is nil")
	}
	defer log.Span().End()
}

func TestTracerStartCtxCarriesNewSpan(t *testing.T) {
	tr, _ := newTestTracer()
	ctx, log := tr.Start(context.Background(), "boot")
	defer log.Span().End()
	got := trace.SpanFromContext(ctx)
	if !sameSpan(got.SpanContext(), log.Span().SpanContext()) {
		t.Errorf("returned ctx does not carry the new span")
	}
}

func TestSpanLoggerEmitsWithSpanContext(t *testing.T) {
	tr, h := newTestTracer()
	_, log := tr.Start(context.Background(), "boot")
	defer log.Span().End()

	log.Info("inside-span")
	recs := h.records()
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	gotSpan := trace.SpanFromContext(recs[0].ctx)
	if !sameSpan(gotSpan.SpanContext(), log.Span().SpanContext()) {
		t.Errorf("log emitted with wrong span context: got %v, want %v",
			gotSpan.SpanContext(), log.Span().SpanContext())
	}
}

func sameSpan(a, b trace.SpanContext) bool {
	return a.TraceID() == b.TraceID() && a.SpanID() == b.SpanID()
}

func TestSpanLoggersAreIndependent(t *testing.T) {
	// The SpanLogger returned from Start must not mutate the Tracer's
	// base logger — sibling spans need independent bindings.
	tr, h := newTestTracer()
	_, logA := tr.Start(context.Background(), "a")
	defer logA.Span().End()
	_, logB := tr.Start(context.Background(), "b")
	defer logB.Span().End()

	logA.Info("from-a")
	logB.Info("from-b")
	recs := h.records()
	if len(recs) != 2 {
		t.Fatalf("expected 2 records, got %d", len(recs))
	}
	if !sameSpan(trace.SpanFromContext(recs[0].ctx).SpanContext(), logA.Span().SpanContext()) {
		t.Error("first record carried wrong span")
	}
	if !sameSpan(trace.SpanFromContext(recs[1].ctx).SpanContext(), logB.Span().SpanContext()) {
		t.Error("second record carried wrong span")
	}
}

func TestSpanLoggerWithPreservesSpan(t *testing.T) {
	tr, h := newTestTracer()
	_, log := tr.Start(context.Background(), "boot")
	defer log.Span().End()

	child := log.With("k", "v")
	child.Info("inside")
	recs := h.records()
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	gotSpan := trace.SpanFromContext(recs[0].ctx)
	if !sameSpan(gotSpan.SpanContext(), log.Span().SpanContext()) {
		t.Error("With() dropped span binding")
	}
}

func TestTracerStartPassesOptionsThrough(t *testing.T) {
	tr, _ := newTestTracer()
	_, log := tr.Start(context.Background(), "boot",
		trace.WithSpanKind(trace.SpanKindServer))
	defer log.Span().End()
	rw, ok := log.Span().(sdktrace.ReadWriteSpan)
	if !ok {
		t.Fatalf("expected ReadWriteSpan, got %T", log.Span())
	}
	if rw.SpanKind() != trace.SpanKindServer {
		t.Errorf("span kind = %v, want Server", rw.SpanKind())
	}
}
