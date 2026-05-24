package telemetry

import (
	"context"
	"log/slog"
)

// Logger is a thin facade over *slog.Logger. It carries an optional bound
// context (the active span's ctx, when returned from Tracer.Start), so the
// plain Info/Warn/... methods correlate logs to that span without the
// caller having to pass the ctx on every call site.
//
// Use Slog() to reach the underlying *slog.Logger when interfacing with
// libraries that expect stdlib slog.
type Logger struct {
	log *slog.Logger
	ctx context.Context // nil → use context.Background()
}

func newLogger(log *slog.Logger) *Logger {
	return &Logger{log: log}
}

func (l *Logger) context() context.Context {
	if l.ctx != nil {
		return l.ctx
	}
	return context.Background()
}

// withContext returns a copy of l bound to ctx. Used by Tracer.Start.
func (l *Logger) withContext(ctx context.Context) *Logger {
	return &Logger{log: l.log, ctx: ctx}
}

func (l *Logger) Debug(msg string, args ...any) {
	l.log.DebugContext(l.context(), msg, args...)
}
func (l *Logger) Verbose(msg string, args ...any) {
	l.log.Log(l.context(), LevelVerbose, msg, args...)
}
func (l *Logger) Info(msg string, args ...any) {
	l.log.InfoContext(l.context(), msg, args...)
}
func (l *Logger) Warn(msg string, args ...any) {
	l.log.WarnContext(l.context(), msg, args...)
}
func (l *Logger) Error(msg string, args ...any) {
	l.log.ErrorContext(l.context(), msg, args...)
}

// With returns a new *Logger whose every record carries the given attrs.
// The bound context (if any) is preserved.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{log: l.log.With(args...), ctx: l.ctx}
}

// Slog returns the underlying *slog.Logger. Escape hatch for libraries
// that take a stdlib logger directly.
func (l *Logger) Slog() *slog.Logger {
	return l.log
}
