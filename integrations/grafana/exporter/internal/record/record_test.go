package record

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot is the repository root relative to this package.
const repoRoot = "../../../../.."

func TestEmbeddedSchemaMatchesRepository(t *testing.T) {
	want, err := os.ReadFile(filepath.Join(repoRoot, "schema", "run-record.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, SchemaJSON) {
		t.Fatal("internal/record/run-record.schema.json differs from schema/run-record.schema.json; run go generate ./internal/record")
	}
}

func TestExamplesLoad(t *testing.T) {
	v, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	files, err := v.Load(filepath.Join(repoRoot, "examples", "runs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 8 {
		t.Fatalf("got %d files, want 8", len(files))
	}
	for _, f := range files {
		if f.Run.ID+".json" != filepath.Base(f.Path) {
			t.Errorf("%s: id %q does not match the file name", f.Path, f.Run.ID)
		}
		if f.Run.EndedAt.Before(f.Run.StartedAt) {
			t.Errorf("%s: ends before it starts", f.Path)
		}
	}
}

// A record that dummyrun review has edited still loads, and the exporter reads the verdict the reviewer set.
func TestReviewedFindingsLoad(t *testing.T) {
	v, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	run, err := v.LoadFile(filepath.Join(repoRoot, "examples", "runs", "2026-10-16-ada-editor-003.json"))
	if err != nil {
		t.Fatal(err)
	}
	f := run.Findings[0]
	if f.Verdict != "confirmed" || f.Reviewer == "" || f.ReviewedAt == nil || run.ConfirmedFindings() != 1 {
		t.Errorf("reviewed finding: %+v", f)
	}
}

func TestLoadNamesTheBadFile(t *testing.T) {
	v, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	good, err := os.ReadFile(filepath.Join(repoRoot, "examples", "runs", "2026-10-17-ravi-reviewer-005.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a-good.json"), good, 0o600); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "b-bad.json")
	if err := os.WriteFile(bad, []byte(`{"schema_version": 1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = v.Load(dir)
	if err == nil {
		t.Fatal("want an error for the invalid record")
	}
	if !strings.Contains(err.Error(), bad) {
		t.Errorf("error %q does not name %s", err, bad)
	}
	if !strings.Contains(err.Error(), "invalid run record") {
		t.Errorf("error %q does not say the record is invalid", err)
	}
}

func TestParseRejects(t *testing.T) {
	v, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	good, err := os.ReadFile(filepath.Join(repoRoot, "examples", "runs", "2026-10-16-ada-editor-003.json"))
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		mutate func(string) string
		want   string
	}{
		"not json": {
			mutate: func(string) string { return "{" },
			want:   "not JSON",
		},
		"bad outcome": {
			mutate: func(s string) string { return strings.Replace(s, `"outcome": "blocked"`, `"outcome": "exploded"`, 1) },
			want:   "does not match the run record schema",
		},
		"bad id": {
			mutate: func(s string) string {
				return strings.Replace(s, `"id": "2026-10-16-ada-editor-003"`, `"id": "Ada Editor"`, 1)
			},
			want: "does not match the run record schema",
		},
		"unknown field": {
			mutate: func(s string) string { return strings.Replace(s, `"outcome":`, `"mood": "fine", "outcome":`, 1) },
			want:   "does not match the run record schema",
		},
		"bad timestamp": {
			mutate: func(s string) string {
				return strings.Replace(s, `"started_at": "2026-10-16T19:33:44.000Z"`, `"started_at": "yesterday"`, 1)
			},
			want: "does not match the run record schema",
		},
		"ends before it starts": {
			mutate: func(s string) string {
				return strings.Replace(s, `"ended_at": "2026-10-16T19:34:26.535Z"`, `"ended_at": "2026-10-16T19:00:00.000Z"`, 1)
			},
			want: "is before started_at",
		},
		"step longer than a year": {
			mutate: func(s string) string {
				return strings.Replace(s, `"duration_ms": 1134,`, `"duration_ms": 99999999999999,`, 1)
			},
			want: "longer than",
		},
		"finding refers to a missing step": {
			mutate: func(s string) string {
				return strings.Replace(s, "\"steps\": [\n        7\n      ]", "\"steps\": [\n        70\n      ]", 1)
			},
			want: "refers to step 70",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			in := tc.mutate(string(good))
			if in == string(good) {
				t.Fatal("the mutation did not change the record")
			}
			_, err := v.Parse([]byte(in))
			if err == nil {
				t.Fatal("want an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}

func TestEmptyDirectoryIsAnError(t *testing.T) {
	v, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if _, err := v.Load(dir); err == nil || !strings.Contains(err.Error(), "no .json files") {
		t.Fatalf("got %v, want a no .json files error", err)
	}
	if _, err := v.Load(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("want an error for a missing file")
	}
}

func TestFirstStepAndConfirmedFindings(t *testing.T) {
	v, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	run, err := v.LoadFile(filepath.Join(repoRoot, "examples", "runs", "2026-10-17-lin-contributor-008.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := run.ConfirmedFindings(); got != 1 {
		t.Errorf("confirmed findings = %d, want 1", got)
	}
	f := record(run.Findings, "author-edits-others-draft")
	if s := run.FirstStep(f); s == nil || s.N != 7 {
		t.Errorf("first step = %v, want step 7", s)
	}
	if run.Step(99) != nil {
		t.Error("Step(99) should be nil")
	}
}

func record(fs []Finding, id string) Finding {
	for _, f := range fs {
		if f.ID == id {
			return f
		}
	}
	return Finding{}
}

func TestVersionOneRecordsStillLoad(t *testing.T) {
	v, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, "examples", "runs", "2026-10-16-ada-editor-003.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	// Turn the version 2 example into a version 1 record by removing what version 2 added.
	m["schema_version"] = 1
	delete(m, "repeat_index")
	delete(m, "seed")
	delete(m, "observations")
	delete(m["persona"].(map[string]any), "dimensions")
	delete(m["scenario"].(map[string]any), "goal_met")
	delete(m["usage"].(map[string]any), "cost_usd")
	for _, s := range m["steps"].([]any) {
		delete(s.(map[string]any), "route")
	}
	for _, f := range m["findings"].([]any) {
		for _, k := range []string{"class", "route", "locus", "symptom", "taxonomy_version", "fingerprint", "reviewer", "reviewed_at"} {
			delete(f.(map[string]any), k)
		}
	}
	v1, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	run, err := v.Parse(v1)
	if err != nil {
		t.Fatalf("version 1 record rejected: %v", err)
	}
	if run.SchemaVersion != 1 || run.Findings[0].Fingerprint != "" {
		t.Errorf("%+v", run)
	}
	// A version 1 record must not carry version 2 fields, and an unknown version is refused.
	m["repeat_index"] = 0
	bad, _ := json.Marshal(m)
	if _, err := v.Parse(bad); err == nil {
		t.Error("version 1 record with repeat_index accepted")
	}
	m["schema_version"] = 3
	bad, _ = json.Marshal(m)
	if _, err := v.Parse(bad); err == nil {
		t.Error("schema_version 3 accepted")
	}
}

func TestRecordWithSigninBlockLoads(t *testing.T) {
	v, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, "examples", "runs", "2026-10-16-ada-editor-003.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	m["signin"] = map[string]any{"method": "form", "role": "editor", "ok": true, "duration_ms": 700,
		"allowed_request": map[string]any{"method": "POST", "url": "https://example.test/signin"}}
	b, _ := json.Marshal(m)
	run, err := v.Parse(b)
	if err != nil {
		t.Fatalf("a record with a sign-in block was rejected: %v", err)
	}
	if run.Signin == nil || run.Signin.Role != "editor" || run.Signin.AllowedRequest == nil {
		t.Errorf("signin %+v", run.Signin)
	}
}

func TestRecordsWithEventsLoad(t *testing.T) {
	v, err := NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, "examples", "runs", "2026-10-16-ada-editor-003.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	m["events"] = []any{map[string]any{"kind": "suspected_injection", "detail": "ignore previous instructions", "step": 1, "source": "hidden", "pattern": "ignore-previous"}}
	withEvents, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	run, err := v.Parse(withEvents)
	if err != nil {
		t.Fatalf("a record with events must load: %v", err)
	}
	if len(run.Events) != 1 {
		t.Errorf("%d events kept, want 1", len(run.Events))
	}
}
