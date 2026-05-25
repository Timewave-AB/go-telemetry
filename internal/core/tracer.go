package core

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// Tracer wraps an OTel trace.Tracer so that Start returns, alongside the
// child context, a *SpanLogger pre-bound to the new span. Logs written
// through that SpanLogger correlate to the span automatically.
type Tracer struct {
	tr  trace.Tracer
	log *slog.Logger
}

func newTracer(tr trace.Tracer, log *Logger) *Tracer {
	return &Tracer{tr: tr, log: log.log}
}

// Start begins a new span and returns the child context and a
// *SpanLogger bound to it. End the span via spanLogger.Span().End().
func (t *Tracer) Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, *SpanLogger) {
	ctx, span := t.tr.Start(ctx, name, opts...)
	return ctx, &SpanLogger{log: t.log, ctx: ctx, span: span}
}

// OTel returns the underlying trace.Tracer. Escape hatch for callers
// that need to hand the raw tracer to a library expecting it.
func (t *Tracer) OTel() trace.Tracer { return t.tr }

// SpanLogger is a Logger bound to a span. Returned by Tracer.Start. Its
// log methods correlate to the span's trace_id and span_id via the
// otelslog bridge. SpanLogger is independent of [Logger] — they share an
// underlying *slog.Logger but the SpanLogger always emits with the bound
// span context.
type SpanLogger struct {
	log  *slog.Logger
	ctx  context.Context
	span trace.Span
}

func (s *SpanLogger) Debug(msg string, args ...any) {
	s.log.DebugContext(s.ctx, msg, args...)
}
func (s *SpanLogger) Verbose(msg string, args ...any) {
	s.log.Log(s.ctx, LevelVerbose, msg, args...)
}
func (s *SpanLogger) Info(msg string, args ...any) {
	s.log.InfoContext(s.ctx, msg, args...)
}
func (s *SpanLogger) Warn(msg string, args ...any) {
	s.log.WarnContext(s.ctx, msg, args...)
}
func (s *SpanLogger) Error(msg string, args ...any) {
	s.log.ErrorContext(s.ctx, msg, args...)
}

// With returns a new SpanLogger bound to the same span whose every
// record carries the given attrs.
func (s *SpanLogger) With(args ...any) *SpanLogger {
	return &SpanLogger{log: s.log.With(args...), ctx: s.ctx, span: s.span}
}

// Span returns the underlying trace.Span. Use it to End the span,
// RecordError, SetAttributes, etc.
func (s *SpanLogger) Span() trace.Span { return s.span }

// Slog returns the underlying *slog.Logger. Note: callers passing this
// to libraries lose the span binding unless they also pass the ctx.
func (s *SpanLogger) Slog() *slog.Logger { return s.log }
