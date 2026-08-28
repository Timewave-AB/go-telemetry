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

// Pins the bridge's level-to-severity mapping, which severityFor recomputes
// rather than imports: a drift there would otherwise widen exports silently.
func TestOTLPLegExportsExactSeveritiesPerLevel(t *testing.T) {
	for _, tc := range []struct {
		level string
		want  []logsapi.Severity
	}{
		{"debug", []logsapi.Severity{logsapi.SeverityDebug, logsapi.SeverityDebug3, logsapi.SeverityInfo, logsapi.SeverityWarn, logsapi.SeverityError}},
		{"verbose", []logsapi.Severity{logsapi.SeverityDebug3, logsapi.SeverityInfo, logsapi.SeverityWarn, logsapi.SeverityError}},
		{"info", []logsapi.Severity{logsapi.SeverityInfo, logsapi.SeverityWarn, logsapi.SeverityError}},
		{"warning", []logsapi.Severity{logsapi.SeverityWarn, logsapi.SeverityError}},
		{"error", []logsapi.Severity{logsapi.SeverityError}},
	} {
		t.Run(tc.level, func(t *testing.T) {
			rec := &severityRecorder{}

			_, restore := captureStdout(t)
			defer restore()

			opts := baseOpts()
			opts.Level = tc.level
			opts.LogExporter = rec

			tel, err := Init(context.Background(), opts)
			if err != nil {
				t.Fatalf("Init: %v", err)
			}
			tel.Logger.Debug("d")
			tel.Logger.Verbose("v")
			tel.Logger.Info("i")
			tel.Logger.Warn("w")
			tel.Logger.Error("e")
			if err := tel.Shutdown(context.Background()); err != nil {
				t.Fatalf("Shutdown: %v", err)
			}

			got := rec.severities()
			if len(got) != len(tc.want) {
				t.Fatalf("exported %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("record %d severity %v, want %v (all: %v)", i, got[i], tc.want[i], got)
				}
			}
		})
	}
}

// Emit does not consult Enabled, so the OnEmit gate is the only thing standing
// between a direct provider caller and the exporter.
func TestOTLPLegFiltersDirectProviderEmit(t *testing.T) {
	rec := &severityRecorder{}

	_, restore := captureStdout(t)
	defer restore()

	opts := baseOpts()
	opts.LogExporter = rec

	tel, err := Init(context.Background(), opts)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	lg := tel.OTel().LoggerProvider.Logger("probe")

	var below logsapi.Record
	below.SetSeverity(logsapi.SeverityDebug)
	below.SetBody(logsapi.StringValue("direct-debug"))
	lg.Emit(context.Background(), below)

	var above logsapi.Record
	above.SetSeverity(logsapi.SeverityInfo)
	above.SetBody(logsapi.StringValue("direct-info"))
	lg.Emit(context.Background(), above)

	if err := tel.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	if got := rec.severities(); len(got) != 1 || got[0] != logsapi.SeverityInfo {
		t.Errorf("exported %v, want exactly [INFO]: OnEmit did not gate a direct emit", got)
	}
}

// The SDK documents an unset severity as indeterminate, which a processor must
// answer with true rather than dropping the record.
func TestLevelProcessorTreatsUndefinedSeverityAsIndeterminate(t *testing.T) {
	p := levelProcessor{Processor: stubProcessor{}, min: logsapi.SeverityError}

	if !p.Enabled(context.Background(), sdklog.EnabledParameters{}) {
		t.Error("Enabled with an unset severity returned false; unset is indeterminate, not below")
	}
}

// Enabled treats an unset severity as indeterminate, so OnEmit must not then
// drop it: "cannot tell" has to mean the same thing on both gates.
func TestOTLPLegKeepsUnsetSeverityOnDirectEmit(t *testing.T) {
	rec := &severityRecorder{}

	_, restore := captureStdout(t)
	defer restore()

	opts := baseOpts()
	opts.LogExporter = rec

	tel, err := Init(context.Background(), opts)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	var unset logsapi.Record
	unset.SetBody(logsapi.StringValue("no-severity"))
	tel.OTel().LoggerProvider.Logger("probe").Emit(context.Background(), unset)

	if err := tel.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	if got := rec.severities(); len(got) != 1 {
		t.Errorf("exported %v, want the unset-severity record kept", got)
	}
}

// stubProcessor stands in where a test only needs levelProcessor's own gates
// and would otherwise leave a batch processor's goroutine running.
type stubProcessor struct{}

func (stubProcessor) Enabled(context.Context, sdklog.EnabledParameters) bool { return true }
func (stubProcessor) OnEmit(context.Context, *sdklog.Record) error           { return nil }
func (stubProcessor) Shutdown(context.Context) error                         { return nil }
func (stubProcessor) ForceFlush(context.Context) error                       { return nil }
