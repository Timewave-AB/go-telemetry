package telemetry

import (
	"os"
	"strconv"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func TestBuildResourceCarriesRequiredAttrs(t *testing.T) {
	res, err := buildResource("queue-worker", "1.2.3")
	if err != nil {
		t.Fatalf("buildResource: %v", err)
	}
	attrs := attrMap(res.Attributes())
	if got := attrs[string(semconv.ServiceNameKey)]; got != "queue-worker" {
		t.Errorf("service.name = %q, want %q", got, "queue-worker")
	}
	if got := attrs[string(semconv.ServiceVersionKey)]; got != "1.2.3" {
		t.Errorf("service.version = %q, want %q", got, "1.2.3")
	}
}

func TestBuildResourceAutoDetectsHostAndPid(t *testing.T) {
	res, err := buildResource("api", "0.0.1")
	if err != nil {
		t.Fatalf("buildResource: %v", err)
	}
	attrs := attrMap(res.Attributes())

	wantHost, _ := os.Hostname()
	if got := attrs[string(semconv.HostNameKey)]; got != wantHost {
		t.Errorf("host.name = %q, want %q", got, wantHost)
	}
	if got := attrs[string(semconv.ProcessPIDKey)]; got != strconv.Itoa(os.Getpid()) {
		t.Errorf("process.pid = %q, want %q", got, strconv.Itoa(os.Getpid()))
	}
}

func attrMap(attrs []attribute.KeyValue) map[string]string {
	m := make(map[string]string, len(attrs))
	for _, kv := range attrs {
		m[string(kv.Key)] = kv.Value.Emit()
	}
	return m
}
