package telemetry

import "testing"

func TestTransportDefaultIsGRPC(t *testing.T) {
	// Zero value of Options.Transport must be TransportGRPC so the
	// caller can omit the field for the common case.
	var opts Options
	if opts.Transport != TransportGRPC {
		t.Errorf("zero-value Options.Transport = %v, want TransportGRPC", opts.Transport)
	}
}

func TestResolveOTLPEndpointAutofillsPort(t *testing.T) {
	cases := []struct {
		name      string
		endpoint  string
		transport Transport
		want      string
	}{
		{"empty stays empty", "", TransportGRPC, ""},
		{"empty stays empty (http)", "", TransportHTTP, ""},
		{"grpc default port", "collector", TransportGRPC, "collector:4317"},
		{"http default port", "collector", TransportHTTP, "collector:4318"},
		{"explicit port preserved (grpc)", "collector:9000", TransportGRPC, "collector:9000"},
		{"explicit port preserved (http)", "collector:8080", TransportHTTP, "collector:8080"},
		{"hostname with dots", "otel.svc.cluster.local", TransportGRPC, "otel.svc.cluster.local:4317"},
		{"ipv4 no port", "10.0.0.1", TransportHTTP, "10.0.0.1:4318"},
		{"ipv4 with port", "10.0.0.1:4318", TransportHTTP, "10.0.0.1:4318"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := resolveOTLPEndpoint(c.endpoint, c.transport)
			if got != c.want {
				t.Errorf("resolveOTLPEndpoint(%q, %v) = %q, want %q", c.endpoint, c.transport, got, c.want)
			}
		})
	}
}
