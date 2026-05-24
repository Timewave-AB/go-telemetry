package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// buildResource assembles the OTel resource describing this process.
// service.name / service.version come from the caller; host.name and
// process.pid are auto-detected from the OS.
func buildResource(serviceName, serviceVersion string) (*resource.Resource, error) {
	return resource.New(
		context.Background(),
		resource.WithHost(),     // host.name
		resource.WithProcess(),  // process.pid (and friends)
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		),
	)
}
