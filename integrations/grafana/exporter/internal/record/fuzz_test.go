package record

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// FuzzParseRunRecord feeds mutated run records to the validator and parser. Parse must never
// panic, and whatever it accepts must be a record the rest of the exporter can use.
func FuzzParseRunRecord(f *testing.F) {
	seeds, err := filepath.Glob(filepath.Join(repoRoot, "examples", "runs", "*.json"))
	if err != nil || len(seeds) == 0 {
		f.Fatalf("no example records: %v", err)
	}
	for _, name := range seeds {
		data, err := os.ReadFile(name)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	f.Add([]byte(`{}`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`null`))
	f.Add([]byte(``))

	v, err := NewValidator()
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		run, err := v.Parse(data)
		if err != nil {
			if run != nil {
				t.Fatal("Parse returned both a run and an error")
			}
			return
		}
		if run == nil {
			t.Fatal("Parse returned neither a run nor an error")
		}
		// The schema guarantees these, and the exporter relies on them.
		if (run.SchemaVersion != 1 && run.SchemaVersion != 2) || run.ID == "" || run.Persona.ID == "" || run.Scenario.ID == "" {
			t.Fatalf("accepted a record missing required fields: %+v", run)
		}
		if run.EndedAt.Before(run.StartedAt) {
			t.Fatalf("accepted a run that ends before it starts: %s < %s", run.EndedAt, run.StartedAt)
		}
		if run.Usage.InputTokens < 0 || run.Usage.OutputTokens < 0 || (run.Usage.CostUSD != nil && *run.Usage.CostUSD < 0) {
			t.Fatalf("accepted negative usage: %+v", run.Usage)
		}
		for _, s := range run.Steps {
			if s.N < 1 || s.DurationMS < 0 || s.At.IsZero() {
				t.Fatalf("accepted a bad step: %+v", s)
			}
			if s.At.Add(time.Duration(s.DurationMS) * time.Millisecond).Before(s.At) {
				t.Fatalf("step end overflows: %+v", s)
			}
		}
		for _, fd := range run.Findings {
			if len(fd.Steps) == 0 || run.FirstStep(fd) == nil {
				t.Fatalf("accepted a finding without a resolvable step: %+v", fd)
			}
		}
	})
}
