package core

import (
	"strings"
	"time"

	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Transport selects the wire protocol used to ship OTLP signals to the
// collector. The zero value is TransportGRPC.
type Transport int

const (
	// TransportGRPC sends OTLP over gRPC. Collector default port: 4317.
	TransportGRPC Transport = iota
	// TransportHTTP sends OTLP over HTTP/protobuf. Collector default port: 4318.
	TransportHTTP
)

func (t Transport) String() string {
	switch t {
	case TransportGRPC:
		return "grpc"
	case TransportHTTP:
		return "http"
	default:
		return "unknown"
	}
}

// Options configures Init. ServiceName is required. ServiceVersion is
// required unless debug.ReadBuildInfo can supply one. OTLPEndpoint being
// empty disables OTLP entirely — logs still print to stdout, traces and
// metrics fall through to noop providers. A per-signal exporter override
// (LogExporter/TraceExporter/MetricExporter) enables that signal even
// when OTLPEndpoint is empty.
type Options struct {
	// Level is the minimum log level. "" defaults to Info silently;
	// unrecognised values fall back to Info and emit one stderr warning.
	// Accepted (case-insensitive): error, warn/warning, info, verbose, debug.
	Level string

	// LogExporter overrides the OTLP log exporter. Set this to plug in a
	// test exporter or a non-OTLP backend. When set, OTLPEndpoint /
	// Transport / OTLPSecure are ignored for logs.
	LogExporter sdklog.Exporter

	// MetricExporter overrides the OTLP metric exporter. Same semantics
	// as LogExporter.
	MetricExporter sdkmetric.Exporter

	// MetricExportInterval controls how often metrics are pushed to the
	// collector. Zero uses the SDK default (60s).
	MetricExportInterval time.Duration

	// OnError, if set, receives async errors from the OTel SDK (exporter
	// failures, dropped batches) and from the multi-handler (per-handler
	// write errors). When nil, errors are silently discarded.
	//
	// Setting OnError causes Init to install otel.SetErrorHandler — the
	// only OTel global Init ever touches, and only on opt-in.
	OnError func(error)

	// OTLPEndpoint is the host[:port] of the collector. When the port is
	// omitted, it defaults to 4317 for TransportGRPC and 4318 for
	// TransportHTTP. Empty disables OTLP export.
	OTLPEndpoint string

	// OTLPSecure controls whether the OTLP exporter uses TLS.
	OTLPSecure bool

	// ServiceName is required.
	ServiceName string

	// ServiceVersion is the version reported as service.version on every
	// signal. Empty triggers a debug.ReadBuildInfo() lookup; if that also
	// yields nothing, the final value is "unknown".
	ServiceVersion string

	// TraceExporter overrides the OTLP trace exporter. Same semantics as
	// LogExporter.
	TraceExporter sdktrace.SpanExporter

	// TraceSampleRatio: 0 means ParentBased(AlwaysOn). A value in (0,1]
	// switches to ParentBased(TraceIDRatioBased(ratio)).
	TraceSampleRatio float64

	// Transport selects the OTLP wire protocol. Defaults to gRPC.
	Transport Transport
}

// resolveOTLPEndpoint appends the default port for the transport when the
// endpoint has no explicit port. Empty input passes through unchanged.
func resolveOTLPEndpoint(endpoint string, transport Transport) string {
	if endpoint == "" {
		return ""
	}
	if strings.LastIndex(endpoint, ":") > strings.LastIndex(endpoint, "]") {
		return endpoint // already has a port
	}
	switch transport {
	case TransportHTTP:
		return endpoint + ":4318"
	default:
		return endpoint + ":4317"
	}
}
