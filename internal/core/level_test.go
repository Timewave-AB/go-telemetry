package core

import (
	"log/slog"
	"testing"
)

func TestLevelOrdering(t *testing.T) {
	// slog convention: higher = more important.
	// Verbose must sit between Debug and Info.
	if !(LevelDebug < LevelVerbose && LevelVerbose < LevelInfo && LevelInfo < LevelWarning && LevelWarning < LevelError) {
		t.Fatalf("levels not strictly ordered: debug=%d verbose=%d info=%d warning=%d error=%d",
			LevelDebug, LevelVerbose, LevelInfo, LevelWarning, LevelError)
	}
}

func TestLevelExactValues(t *testing.T) {
	cases := map[string]struct {
		got, want slog.Level
	}{
		"Error":   {LevelError, slog.LevelError},   // 8
		"Warning": {LevelWarning, slog.LevelWarn},  // 4
		"Info":    {LevelInfo, slog.LevelInfo},     // 0
		"Verbose": {LevelVerbose, slog.Level(-2)},  // -2
		"Debug":   {LevelDebug, slog.LevelDebug},   // -4
	}
	for name, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", name, c.got, c.want)
		}
	}
}

func TestParseLogLevel(t *testing.T) {
	cases := []struct {
		in        string
		want      slog.Level
		warnEmpty bool // true means parseLogLevel should signal "unknown" via the second return
	}{
		{"error", LevelError, false},
		{"ERROR", LevelError, false},
		{"Error", LevelError, false},
		{"warn", LevelWarning, false},
		{"warning", LevelWarning, false},
		{"WARNING", LevelWarning, false},
		{"info", LevelInfo, false},
		{"verbose", LevelVerbose, false},
		{"VERBOSE", LevelVerbose, false},
		{"debug", LevelDebug, false},
		{"", LevelInfo, false},        // empty: silent default
		{"bogus", LevelInfo, true},    // unknown: stderr warning
		{"   ", LevelInfo, true},      // whitespace-only is "set but garbage", warn
	}
	for _, c := range cases {
		got, unknown := parseLogLevel(c.in)
		if got != c.want {
			t.Errorf("parseLogLevel(%q) level = %d, want %d", c.in, got, c.want)
		}
		if unknown != c.warnEmpty {
			t.Errorf("parseLogLevel(%q) unknown = %v, want %v", c.in, unknown, c.warnEmpty)
		}
	}
}

func TestLevelName(t *testing.T) {
	cases := map[slog.Level]string{
		LevelError:   "ERROR",
		LevelWarning: "WARN",
		LevelInfo:    "INFO",
		LevelVerbose: "VERBOSE",
		LevelDebug:   "DEBUG",
	}
	for lvl, want := range cases {
		if got := levelName(lvl); got != want {
			t.Errorf("levelName(%d) = %q, want %q", lvl, got, want)
		}
	}
}
