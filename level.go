package telemetry

import (
	"log/slog"
	"strings"
)

// slog convention: higher value = more important.
const (
	LevelDebug   = slog.LevelDebug // -4
	LevelVerbose = slog.Level(-2)
	LevelInfo    = slog.LevelInfo // 0
	LevelWarning = slog.LevelWarn // 4
	LevelError   = slog.LevelError // 8
)

// parseLogLevel maps a case-insensitive string to a slog.Level.
// Empty input returns LevelInfo with unknown=false (silent default).
// Unrecognised non-empty input returns LevelInfo with unknown=true so
// the caller can emit a one-time stderr warning.
func parseLogLevel(s string) (lvl slog.Level, unknown bool) {
	if s == "" {
		return LevelInfo, false
	}
	switch strings.ToLower(s) {
	case "error":
		return LevelError, false
	case "warn", "warning":
		return LevelWarning, false
	case "info":
		return LevelInfo, false
	case "verbose":
		return LevelVerbose, false
	case "debug":
		return LevelDebug, false
	default:
		return LevelInfo, true
	}
}

func levelName(lvl slog.Level) string {
	switch lvl {
	case LevelError:
		return "ERROR"
	case LevelWarning:
		return "WARN"
	case LevelInfo:
		return "INFO"
	case LevelVerbose:
		return "VERBOSE"
	case LevelDebug:
		return "DEBUG"
	default:
		return lvl.String()
	}
}
