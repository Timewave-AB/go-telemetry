package telemetry

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// fixedTime is used so output is deterministic.
var fixedTime = time.Date(2026, 5, 24, 14, 2, 11, 0, time.UTC)

func newTextHandlerForTest(t *testing.T, level slog.Level) (*textHandler, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	return &textHandler{level: level, writer: &buf}, &buf
}

func emit(t *testing.T, h slog.Handler, lvl slog.Level, msg string, attrs ...slog.Attr) {
	t.Helper()
	r := slog.NewRecord(fixedTime, lvl, msg, 0)
	r.AddAttrs(attrs...)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle: %v", err)
	}
}

func TestTextHandlerSimpleRecord(t *testing.T) {
	h, buf := newTextHandlerForTest(t, LevelInfo)
	emit(t, h, LevelInfo, "hello")
	got := buf.String()
	want := `2026-05-24 14:02:11 [INFO] msg="hello"` + "\n"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestTextHandlerAllLevels(t *testing.T) {
	cases := []struct {
		lvl  slog.Level
		want string
	}{
		{LevelError, "ERROR"},
		{LevelWarning, "WARN"},
		{LevelInfo, "INFO"},
		{LevelVerbose, "VERBOSE"},
		{LevelDebug, "DEBUG"},
	}
	for _, c := range cases {
		h, buf := newTextHandlerForTest(t, LevelDebug)
		emit(t, h, c.lvl, "x")
		if !strings.Contains(buf.String(), "["+c.want+"]") {
			t.Errorf("level %s: missing tag in %q", c.want, buf.String())
		}
	}
}

func TestTextHandlerAttrsSortedAlphabetically(t *testing.T) {
	h, buf := newTextHandlerForTest(t, LevelInfo)
	emit(t, h, LevelInfo, "msg",
		slog.String("zeta", "z"),
		slog.Int("alpha", 1),
		slog.String("mike", "m"),
	)
	got := buf.String()
	want := `2026-05-24 14:02:11 [INFO] msg="msg" alpha=1 mike="m" zeta="z"` + "\n"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestTextHandlerLevelFiltering(t *testing.T) {
	h, buf := newTextHandlerForTest(t, LevelInfo)
	if h.Enabled(context.Background(), LevelDebug) {
		t.Error("debug should be filtered when level=info")
	}
	if !h.Enabled(context.Background(), LevelInfo) {
		t.Error("info should be enabled when level=info")
	}
	if !h.Enabled(context.Background(), LevelError) {
		t.Error("error should always be enabled when level=info")
	}
	// slog.Logger consults Enabled before calling Handle, so the gate
	// is real end-to-end.
	slog.New(h).Debug("should not appear")
	if buf.Len() != 0 {
		t.Errorf("expected no output for filtered level, got %q", buf.String())
	}
}

func TestTextHandlerWithAttrsPersist(t *testing.T) {
	h, buf := newTextHandlerForTest(t, LevelInfo)
	child := h.WithAttrs([]slog.Attr{slog.String("service", "queue-worker")})
	emit(t, child, LevelInfo, "first")
	emit(t, child, LevelInfo, "second", slog.Int("n", 42))
	out := buf.String()
	// Each line carries the persisted attr and the per-record attrs, sorted.
	want := strings.Join([]string{
		`2026-05-24 14:02:11 [INFO] msg="first" service="queue-worker"`,
		`2026-05-24 14:02:11 [INFO] msg="second" n=42 service="queue-worker"`,
		"",
	}, "\n")
	if out != want {
		t.Errorf("got  %q\nwant %q", out, want)
	}
}

func TestTextHandlerWithGroupNestsKeys(t *testing.T) {
	h, buf := newTextHandlerForTest(t, LevelInfo)
	child := h.WithGroup("req").WithAttrs([]slog.Attr{slog.String("id", "abc")})
	emit(t, child, LevelInfo, "hit", slog.Int("status", 200))
	got := buf.String()
	// Group prefixes both persisted and per-record attrs; alphabetical sort.
	want := `2026-05-24 14:02:11 [INFO] msg="hit" req.id="abc" req.status=200` + "\n"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestTextHandlerTimestampUTC(t *testing.T) {
	h, buf := newTextHandlerForTest(t, LevelInfo)
	// Record carries a non-UTC time; handler must render in UTC.
	loc, _ := time.LoadLocation("Europe/Stockholm")
	r := slog.NewRecord(time.Date(2026, 5, 24, 16, 0, 0, 0, loc), LevelInfo, "x", 0)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(buf.String(), "2026-05-24 14:00:00 ") {
		t.Errorf("expected UTC-rendered timestamp, got %q", buf.String())
	}
}
