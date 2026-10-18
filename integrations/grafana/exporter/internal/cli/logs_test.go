package cli

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/dummy-run/dummy-run/integrations/grafana/exporter/internal/otelsetup"
	"github.com/dummy-run/dummy-run/integrations/grafana/exporter/internal/record"
	"github.com/dummy-run/dummy-run/integrations/grafana/exporter/internal/telemetry"
	"github.com/dummy-run/dummy-run/integrations/grafana/exporter/internal/testexporters"
)

// The example files sort by name, which is not time order: 2026-10-17-lin-contributor-008 ran at
// 13:53 and comes before 2026-10-17-ravi-reviewer-005, which ran at 05:41. Loki would reject the
// later records as too far behind, so export must send logs oldest first.
func TestExportSendsLogsOldestFirst(t *testing.T) {
	res, err := otelsetup.Resource(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	logs := &testexporters.Logs{}
	providers := otelsetup.Assemble(res, tracetest.NewNoopExporter(), &testexporters.Metrics{}, logs, nil)
	emitter, err := telemetry.NewEmitter(providers.Tracer, providers.Meter, providers.Logger)
	if err != nil {
		t.Fatal(err)
	}
	v, err := record.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	files, err := v.Load(examples)
	if err != nil {
		t.Fatal(err)
	}
	s := &session{env: Env{Now: time.Now}, emitter: emitter, providers: providers}
	var plans []telemetry.Plan
	for _, f := range files {
		plans = append(plans, s.prepare(f.Run))
	}
	entries := sortedLogs(plans)
	if err := s.guard(entries[0].rec.Time); err != nil {
		t.Fatalf("guard refused a fresh export: %v", err)
	}
	for i, f := range files {
		if err := s.sendPlan(context.Background(), f.Path, plans[i], false); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(logs.Records()); got != 0 {
		t.Fatalf("%d log records sent before sorting, want 0", got)
	}
	s.emitLogs(context.Background(), entries)
	// After sending, a batch that ends more than an hour before the newest line is refused.
	if err := s.guard(entries[0].rec.Time); err == nil {
		t.Error("guard accepted lines more than an hour behind the newest sent")
	}
	if err := s.guard(s.newestSent.Add(-59 * time.Minute)); err != nil {
		t.Errorf("guard refused a line within the hour: %v", err)
	}
	if err := providers.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	recs := logs.Records()
	if len(recs) != 16 {
		t.Fatalf("%d log records, want 16", len(recs))
	}
	for i := 1; i < len(recs); i++ {
		if recs[i].Timestamp().Before(recs[i-1].Timestamp()) {
			t.Errorf("record %d at %s comes after record %d at %s", i, recs[i].Timestamp(), i-1, recs[i-1].Timestamp())
		}
	}
	// Each record still carries its own run's trace context.
	byTrace := map[string]int{}
	for _, r := range recs {
		byTrace[r.TraceID().String()]++
	}
	if len(byTrace) != 8 {
		t.Errorf("records span %d traces, want 8", len(byTrace))
	}
	if byTrace[telemetry.TraceIDFor("2026-10-17-lin-contributor-008").String()] != 3 {
		t.Errorf("lin-contributor-008 should have 3 records (run end plus 2 findings): %v", byTrace)
	}
}
