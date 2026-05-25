package core

import (
	"context"
	"log/slog"
	"testing"
)

// ctxRecordingHandler captures the ctx each record was emitted with,
// so Logger/SpanLogger tests can assert ctx propagation. Cloned handlers
// (via WithAttrs/WithGroup) share the same records sink.
type ctxRecordingHandler struct {
	sink   *[]ctxRecord
	attrs  []slog.Attr
	groups []string
}

type ctxRecord struct {
	ctx context.Context
	rec slog.Record
}

func newCtxRecordingHandler() *ctxRecordingHandler {
	return &ctxRecordingHandler{sink: &[]ctxRecord{}}
}

func (h *ctxRecordingHandler) records() []ctxRecord { return *h.sink }

func (h *ctxRecordingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *ctxRecordingHandler) Handle(ctx context.Context, r slog.Record) error {
	r2 := r.Clone()
	for _, a := range h.attrs {
		r2.AddAttrs(a)
	}
	*h.sink = append(*h.sink, ctxRecord{ctx: ctx, rec: r2})
	return nil
}
func (h *ctxRecordingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := *h
	out.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &out
}
func (h *ctxRecordingHandler) WithGroup(name string) slog.Handler {
	out := *h
	out.groups = append(append([]string{}, h.groups...), name)
	return &out
}

type ctxKey struct{}

func newTestLogger() (*Logger, *ctxRecordingHandler) {
	h := newCtxRecordingHandler()
	return newLogger(slog.New(h)), h
}

func TestLoggerInfoEmitsRecord(t *testing.T) {
	log, h := newTestLogger()
	log.Info("hello", "k", "v")
	recs := h.records()
	if len(recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recs))
	}
	if recs[0].rec.Level != LevelInfo {
		t.Errorf("level = %v, want Info", recs[0].rec.Level)
	}
	if recs[0].rec.Message != "hello" {
		t.Errorf("message = %q, want hello", recs[0].rec.Message)
	}
}

func TestLoggerAllLevels(t *testing.T) {
	log, h := newTestLogger()
	log.Debug("d")
	log.Verbose("v")
	log.Info("i")
	log.Warn("w")
	log.Error("e")
	wants := []slog.Level{LevelDebug, LevelVerbose, LevelInfo, LevelWarning, LevelError}
	recs := h.records()
	if len(recs) != len(wants) {
		t.Fatalf("expected %d records, got %d", len(wants), len(recs))
	}
	for i, w := range wants {
		if recs[i].rec.Level != w {
			t.Errorf("record %d level = %v, want %v", i, recs[i].rec.Level, w)
		}
	}
}

func TestLoggerEmitsWithBackgroundContext(t *testing.T) {
	// Logger is ctx-free: it always emits with context.Background().
	log, h := newTestLogger()
	log.Info("x")
	if got := h.records()[0].ctx; got != context.Background() {
		t.Errorf("ctx = %v, want context.Background()", got)
	}
}

func TestLoggerWithAttrsReturnsNewLogger(t *testing.T) {
	log, h := newTestLogger()
	child := log.With("service", "queue-worker")
	if child == log {
		t.Fatal("With returned same instance")
	}
	child.Info("hi")
	r := h.records()[0].rec
	found := false
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "service" && a.Value.String() == "queue-worker" {
			found = true
		}
		return true
	})
	if !found {
		t.Error("attrs from With() not present on emitted record")
	}
}

func TestLoggerSlogReturnsUnderlying(t *testing.T) {
	log, _ := newTestLogger()
	if log.Slog() == nil {
		t.Error("Slog() returned nil")
	}
	if log.Slog() != log.log {
		t.Error("Slog() did not return the underlying logger")
	}
}
