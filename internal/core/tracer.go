package core

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// Tracer wraps an OTel trace.Tracer so that Start returns, alongside the
// usual (ctx, span), a *Logger pre-bound to the new span's context. Logs
// written through that *Logger correlate to the span automatically.
type Tracer struct {
	tr  trace.Tracer
	log *Logger
}

func newTracer(tr trace.Tracer, log *Logger) *Tracer {
	return &Tracer{tr: tr, log: log}
}

// Start begins a new span and returns:
//   - the child context carrying the span,
//   - the span itself,
//   - a *Logger whose Info/Warn/... methods automatically log against the
//     new span (via the otelslog bridge — trace_id and span_id are
//     attached to the emitted log record).
func (t *Tracer) Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span, *Logger) {
	ctx, span := t.tr.Start(ctx, name, opts...)
	return ctx, span, t.log.withContext(ctx)
}

// OTel returns the underlying trace.Tracer. Escape hatch for callers
// that need to hand the raw tracer to a library expecting it.
func (t *Tracer) OTel() trace.Tracer { return t.tr }
