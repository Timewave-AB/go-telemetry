package core

import (
	"context"
	"log/slog"
)

// Logger is a thin facade over *slog.Logger. It is the top-level logger
// returned as Telemetry.Logger and emits records with context.Background().
//
// To get a logger that auto-correlates to a span, use Tracer.Start —
// it returns a [SpanLogger] bound to the new span's context.
//
// Use Slog() to reach the underlying *slog.Logger when interfacing with
// libraries that expect stdlib slog.
type Logger struct {
	log *slog.Logger
}

func newLogger(log *slog.Logger) *Logger {
	return &Logger{log: log}
}

func (l *Logger) Debug(msg string, args ...any) {
	l.log.DebugContext(context.Background(), msg, args...)
}
func (l *Logger) Verbose(msg string, args ...any) {
	l.log.Log(context.Background(), LevelVerbose, msg, args...)
}
func (l *Logger) Info(msg string, args ...any) {
	l.log.InfoContext(context.Background(), msg, args...)
}
func (l *Logger) Warn(msg string, args ...any) {
	l.log.WarnContext(context.Background(), msg, args...)
}
func (l *Logger) Error(msg string, args ...any) {
	l.log.ErrorContext(context.Background(), msg, args...)
}

// With returns a new *Logger whose every record carries the given attrs.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{log: l.log.With(args...)}
}

// Slog returns the underlying *slog.Logger. Escape hatch for libraries
// that take a stdlib logger directly.
func (l *Logger) Slog() *slog.Logger {
	return l.log
}
