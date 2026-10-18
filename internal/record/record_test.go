package record

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sample() *Record {
	start := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	return &Record{
		SchemaVersion: 1,
		ID:            "2026-10-18-qa-lead-first-look-ab12",
		StartedAt:     start,
		EndedAt:       start.Add(90 * time.Second),
		Persona:       Persona{ID: "qa-lead", Role: "visitor", Device: "desktop"},
		Scenario:      Scenario{ID: "first-look", Title: "First look at the home page"},
		Target:        Target{Adapter: "url", Environment: "production", BaseURL: "https://example.test/"},
		Provider:      Provider{Kind: "azure-foundry", Model: "gpt-4-1-mini-uk", Region: "uksouth"},
		Browser:       Browser{Name: "firefox", Version: "158.0"},
		Outcome:       OutcomeCompleted,
		Steps: []Step{
			{N: 1, At: start, Action: "look", DurationMs: 400, URL: "https://example.test/", Status: Int(200), ConsoleErrors: Int(0), Screenshot: "2026-10-18-qa-lead-first-look-ab12/screens/001.png"},
			{N: 2, At: start.Add(time.Second), Action: "a11y", DurationMs: 900, A11yViolations: Int(2)},
			{N: 3, At: start.Add(2 * time.Second), Action: "finish", DurationMs: 10, Note: "done"},
		},
		Findings: []Finding{{ID: FindingID("accessibility", "The search field has no label"), Kind: "accessibility", Severity: "minor",
			Title: "The search field has no label", Steps: []int{2}, Evidence: "measured", Verdict: VerdictPending}},
		Usage: Usage{ModelCalls: 3, InputTokens: 1200, OutputTokens: 300},
	}
}

func TestMarshalValidRecord(t *testing.T) {
	data, err := Marshal(sample())
	if err != nil {
		t.Fatalf("valid record rejected: %v", err)
	}
	if !bytes.HasSuffix(data, []byte("}\n")) {
		t.Error("output does not end with a newline")
	}
	var back map[string]any
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if _, ok := back["batch"]; ok {
		t.Error("empty batch was written")
	}
}

func TestMarshalRejectsInvalidRecords(t *testing.T) {
	cases := map[string]func(*Record){
		"bad id":           func(r *Record) { r.ID = "no-date" },
		"bad outcome":      func(r *Record) { r.Outcome = "done" },
		"bad action":       func(r *Record) { r.Steps[0].Action = "hover" },
		"bad device":       func(r *Record) { r.Persona.Device = "watch" },
		"bad browser":      func(r *Record) { r.Browser.Name = "safari" },
		"finding no steps": func(r *Record) { r.Findings[0].Steps = nil },
		"bad verdict":      func(r *Record) { r.Findings[0].Verdict = "maybe" },
		"bad commit":       func(r *Record) { r.Target.Commit = "xyz" },
		"negative tokens":  func(r *Record) { r.Usage.InputTokens = -1 },
		"bad base url":     func(r *Record) { r.Target.BaseURL = "not a url" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := sample()
			mutate(r)
			if _, err := Marshal(r); !errors.Is(err, ErrInvalid) {
				t.Errorf("expected ErrInvalid, got %v", err)
			}
		})
	}
}

func TestWriteAndRead(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runs")
	r := sample()
	path, err := Write(dir, r)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != r.ID+".json" {
		t.Errorf("wrote %s", path)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("temporary file left behind")
	}
	back, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if back.ID != r.ID || len(back.Steps) != 3 || *back.Steps[0].Status != 200 {
		t.Errorf("round trip changed the record: %+v", back)
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{"schema_version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(bad); !errors.Is(err, ErrInvalid) {
		t.Errorf("Read accepted an invalid record: %v", err)
	}
}

func TestFindingID(t *testing.T) {
	for in, want := range map[string]string{
		"The search field has no label": "accessibility-the-search-field-has-no-label",
		"  Weird --- punctuation!!  ":   "accessibility-weird-punctuation",
		"":                              "accessibility-untitled",
		strings.Repeat("word ", 30):     "accessibility-word-word-word-word-word-word-word-word-word-word-word-word",
	} {
		if got := FindingID("accessibility", in); got != want {
			t.Errorf("FindingID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEmbeddedSchemaMatchesRepository(t *testing.T) {
	repo, err := os.ReadFile(filepath.Join("..", "..", "schema", "run-record.schema.json"))
	if err != nil {
		t.Skip("repository schema not available")
	}
	if !bytes.Equal(repo, SchemaJSON()) {
		t.Error("internal/record/run-record.schema.json differs from schema/run-record.schema.json")
	}
}

// FuzzWrite feeds the writer records built from arbitrary field values. It
// must either write a record that reads back equal, or refuse it with
// ErrInvalid; it must never panic or write an invalid file.
func FuzzWrite(f *testing.F) {
	f.Add("2026-10-18-x-1", "completed", "look", "bug", "minor", "measured", "pending", 200, 3, 0)
	f.Add("bad id", "completed", "look", "bug", "minor", "measured", "pending", 200, 3, 0)
	f.Add("2026-01-01-a", "blocked", "finish", "friction", "info", "inferred", "pending", 0, 0, -1)
	f.Add("2026-01-01-a", "errored", "visit", "permission", "blocker", "code", "confirmed", 500, 1, 7)
	f.Fuzz(func(t *testing.T, id, outcome, action, kind, severity, evidence, verdict string, status, steps, tokens int) {
		r := sample()
		r.ID = id
		r.Outcome = outcome
		r.Steps[0].Action = action
		r.Steps[0].Status = Int(status)
		r.Findings[0].Kind = kind
		r.Findings[0].Severity = severity
		r.Findings[0].Evidence = evidence
		r.Findings[0].Verdict = verdict
		r.Findings[0].ID = FindingID(kind, id)
		r.Findings[0].Steps = make([]int, 0, max(steps, 0))
		for i := range max(steps, 0) {
			r.Findings[0].Steps = append(r.Findings[0].Steps, i+1)
		}
		r.Usage.InputTokens = tokens
		dir := t.TempDir()
		path, err := Write(dir, r)
		if err != nil {
			if !errors.Is(err, ErrInvalid) && !strings.Contains(err.Error(), "no such file") && !strings.Contains(err.Error(), "invalid argument") && !strings.Contains(err.Error(), "file name too long") {
				t.Fatalf("unexpected error: %v", err)
			}
			return
		}
		back, err := Read(path)
		if err != nil {
			t.Fatalf("written record does not read back: %v", err)
		}
		if back.ID != r.ID || back.Outcome != r.Outcome || back.Usage.InputTokens != r.Usage.InputTokens {
			t.Fatalf("round trip changed the record")
		}
	})
}
