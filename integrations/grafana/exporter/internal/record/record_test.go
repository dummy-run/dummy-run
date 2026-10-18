package record

import (
	"bytes"
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
	f := record(run.Findings, "contributor-sees-rates-table")
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
