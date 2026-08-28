package core

import (
	"context"
	"strings"
	"sync"
	"testing"

	logsapi "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// severityRecorder captures the severity of every record that reaches the
// exporter, so a test can tell what the OTLP leg actually shipped.
type severityRecorder struct {
	mu   sync.Mutex
	seen []logsapi.Severity
}

func (r *severityRecorder) Export(_ context.Context, records []sdklog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range records {
		r.seen = append(r.seen, rec.Severity())
	}
	return nil
}

func (r *severityRecorder) ForceFlush(context.Context) error { return nil }
func (r *severityRecorder) Shutdown(context.Context) error   { return nil }

func (r *severityRecorder) severities() []logsapi.Severity {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]logsapi.Severity(nil), r.seen...)
}

func TestOTLPLegFiltersBelowConfiguredLevel(t *testing.T) {
	rec := &severityRecorder{}

	stdout, restore := captureStdout(t)
	opts := baseOpts()
	opts.LogExporter = rec

	tel, err := Init(context.Background(), opts)
	if err != nil {
		restore()
		t.Fatalf("Init: %v", err)
	}
	tel.Logger.Debug("debug-record")
	tel.Logger.Info("info-record")
	tel.Shutdown(context.Background())
	restore()

	for _, sev := range rec.severities() {
		if sev < logsapi.SeverityInfo {
			t.Errorf("severity %v was exported at Level=info; OTLP leg is not filtered", sev)
		}
	}
	if len(rec.severities()) != 1 {
		t.Errorf("exported %d records, want 1 (info only): %v", len(rec.severities()), rec.severities())
	}
	if strings.Contains(stdout.String(), "debug-record") {
		t.Errorf("debug reached stdout at Level=info, got %q", stdout.String())
	}
}

func TestOTLPLegExportsEveryLevelAtDebug(t *testing.T) {
	rec := &severityRecorder{}

	stdout, restore := captureStdout(t)
	opts := baseOpts()
	opts.Level = "debug"
	opts.LogExporter = rec

	tel, err := Init(context.Background(), opts)
	if err != nil {
		restore()
		t.Fatalf("Init: %v", err)
	}
	tel.Logger.Debug("d")
	tel.Logger.Verbose("v")
	tel.Logger.Info("i")
	tel.Logger.Warn("w")
	tel.Logger.Error("e")
	tel.Shutdown(context.Background())
	restore()

	if got := len(rec.severities()); got != 5 {
		t.Errorf("exported %d records, want 5: %v", got, rec.severities())
	}
	if !strings.Contains(stdout.String(), "[DEBUG]") {
		t.Errorf("debug missing from stdout at Level=debug, got %q", stdout.String())
	}
}
