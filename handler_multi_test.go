package telemetry

import (
	"context"
	"errors"
	"log/slog"
	"testing"
)

type recordingHandler struct {
	level    slog.Level
	enabled  bool
	handled  []slog.Record
	attrs    []slog.Attr
	groups   []string
	handleErr error
}

func (r *recordingHandler) Enabled(_ context.Context, lvl slog.Level) bool {
	return r.enabled && lvl >= r.level
}
func (r *recordingHandler) Handle(_ context.Context, rec slog.Record) error {
	r.handled = append(r.handled, rec)
	return r.handleErr
}
func (r *recordingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := *r
	out.attrs = append(append([]slog.Attr{}, r.attrs...), attrs...)
	return &out
}
func (r *recordingHandler) WithGroup(name string) slog.Handler {
	out := *r
	out.groups = append(append([]string{}, r.groups...), name)
	return &out
}

func TestMultiHandlerFansOutToAll(t *testing.T) {
	a := &recordingHandler{enabled: true}
	b := &recordingHandler{enabled: true}
	mh := &multiHandler{handlers: []slog.Handler{a, b}}

	rec := slog.NewRecord(fixedTime, LevelInfo, "x", 0)
	if err := mh.Handle(context.Background(), rec); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(a.handled) != 1 || len(b.handled) != 1 {
		t.Errorf("expected both to receive: a=%d b=%d", len(a.handled), len(b.handled))
	}
}

func TestMultiHandlerCollectsErrors(t *testing.T) {
	errA := errors.New("a failed")
	errB := errors.New("b failed")
	a := &recordingHandler{enabled: true, handleErr: errA}
	b := &recordingHandler{enabled: true, handleErr: errB}
	mh := &multiHandler{handlers: []slog.Handler{a, b}}

	err := mh.Handle(context.Background(), slog.NewRecord(fixedTime, LevelInfo, "x", 0))
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errA) || !errors.Is(err, errB) {
		t.Errorf("expected joined error containing both, got %v", err)
	}
	// Even though A failed, B must still have been called.
	if len(b.handled) != 1 {
		t.Error("B was skipped after A errored — must keep going")
	}
}

func TestMultiHandlerEnabledIfAny(t *testing.T) {
	a := &recordingHandler{enabled: false, level: LevelError}
	b := &recordingHandler{enabled: true, level: LevelDebug}
	mh := &multiHandler{handlers: []slog.Handler{a, b}}
	if !mh.Enabled(context.Background(), LevelDebug) {
		t.Error("expected enabled because B accepts debug")
	}
}

func TestMultiHandlerWithAttrsPropagates(t *testing.T) {
	a := &recordingHandler{enabled: true}
	b := &recordingHandler{enabled: true}
	mh := &multiHandler{handlers: []slog.Handler{a, b}}

	child := mh.WithAttrs([]slog.Attr{slog.String("k", "v")}).(*multiHandler)
	childA := child.handlers[0].(*recordingHandler)
	childB := child.handlers[1].(*recordingHandler)
	if len(childA.attrs) != 1 || len(childB.attrs) != 1 {
		t.Errorf("expected attrs propagated to both: a=%v b=%v", childA.attrs, childB.attrs)
	}
}

func TestMultiHandlerWithGroupPropagates(t *testing.T) {
	a := &recordingHandler{enabled: true}
	mh := &multiHandler{handlers: []slog.Handler{a}}
	child := mh.WithGroup("req").(*multiHandler)
	childA := child.handlers[0].(*recordingHandler)
	if len(childA.groups) != 1 || childA.groups[0] != "req" {
		t.Errorf("expected group propagated, got %v", childA.groups)
	}
}
