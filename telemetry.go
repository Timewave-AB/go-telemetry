// Package telemetry is an opinionated OpenTelemetry bootstrap for Go
// services. One [Init] call wires logs, traces, and metrics against the
// same OTLP collector and returns a bundle of handles.
//
// The implementation lives in internal/core; this file re-exports the
// public surface via type aliases so callers depend only on this package.
package telemetry

import (
	"context"

	"github.com/Timewave-AB/go-telemetry/internal/core"
)

// Type aliases preserve methods, field access, and identity — callers
// see exactly the types they had before the impl moved under internal/.
type (
	Logger    = core.Logger
	Options   = core.Options
	Telemetry = core.Telemetry
	Tracer    = core.Tracer
	Transport = core.Transport
)

// Transport selects the OTLP wire protocol.
const (
	TransportGRPC = core.TransportGRPC
	TransportHTTP = core.TransportHTTP
)

// Log levels. Five levels in slog's "higher value = more important" ordering.
const (
	LevelDebug   = core.LevelDebug
	LevelVerbose = core.LevelVerbose
	LevelInfo    = core.LevelInfo
	LevelWarning = core.LevelWarning
	LevelError   = core.LevelError
)

// Init bootstraps logs, traces, and metrics. See [Options] for configuration.
func Init(ctx context.Context, opts Options) (*Telemetry, error) {
	return core.Init(ctx, opts)
}
