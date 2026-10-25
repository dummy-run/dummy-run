package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLabels(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCalibrateBuildsATableAndAReport(t *testing.T) {
	dir := t.TempDir()
	var rows []string
	for i := 0; i < 12; i++ {
		rows = append(rows, `{"inputs":{"evidence_kind":"engine-rule","engines":["axe"],"check":"label","on_journey":false},"true":true,"source":"test"}`)
	}
	rows = append(rows, `{"inputs":{"evidence_kind":"model-only","engines":[],"check":"unclear-label","on_journey":true},"true":false}`)
	labels := writeLabels(t, dir, "labels.json", "["+strings.Join(rows, ",")+"]")
	table := filepath.Join(dir, "table.json")
	report := filepath.Join(dir, "calibration.md")
	if got := run([]string{"calibrate", "--labels", labels, "--out-table", table, "--report", report}); got != 0 {
		t.Fatalf("calibrate exited %d", got)
	}
	tb, err := os.ReadFile(table)
	if err != nil || !strings.Contains(string(tb), `"check": "label"`) || !strings.Contains(string(tb), `"total": 12`) {
		t.Errorf("table: %v %s", err, tb)
	}
	md, err := os.ReadFile(report)
	if err != nil || !strings.Contains(string(md), "13 labelled findings") || !strings.Contains(string(md), "| medium | 12 | 12 |") {
		t.Errorf("report: %v %s", err, md)
	}
	// A table can be checked against other labels, and the report then goes to standard output.
	held := writeLabels(t, dir, "held.json", `[{"inputs":{"evidence_kind":"engine-rule","engines":["axe"],"check":"label","on_journey":false},"true":false}]`)
	var out bytes.Buffer
	if got := calibrateTo([]string{"--table", table, "--eval", held}, &out); got != 0 {
		t.Errorf("calibrate --table exited %d", got)
	}
	if !strings.Contains(out.String(), "1 labelled findings") || !strings.Contains(out.String(), "| medium | 1 | 0 |") {
		t.Errorf("eval report:\n%s", out.String())
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("closed pipe") }

func TestCalibrateFailsWhenTheReportCannotBeWritten(t *testing.T) {
	labels := writeLabels(t, t.TempDir(), "l.json", `[{"inputs":{"evidence_kind":"model-only","engines":[],"check":"x","on_journey":true},"true":false}]`)
	if got := calibrateTo([]string{"--labels", labels}, failingWriter{}); got != 1 {
		t.Errorf("exited %d, want 1", got)
	}
}

func TestCalibrateRefusals(t *testing.T) {
	dir := t.TempDir()
	bad := writeLabels(t, dir, "bad.json", `[{"inputs":{"evidence_kind":"vibes","engines":[],"check":"x","on_journey":false},"true":true}]`)
	for name, args := range map[string][]string{
		"no inputs":        {"calibrate"},
		"unreadable":       {"calibrate", "--labels", filepath.Join(dir, "missing.json")},
		"bad evidence":     {"calibrate", "--labels", bad},
		"missing table":    {"calibrate", "--table", filepath.Join(dir, "none.json")},
		"nothing to check": {"calibrate", "--table", writeLabels(t, dir, "t.json", `{"min_samples":10,"buckets":[]}`)},
	} {
		if got := run(args); got != exitRefused {
			t.Errorf("%s: exited %d, want %d", name, got, exitRefused)
		}
	}
	if got := run([]string{"calibrate", "-h"}); got != 0 {
		t.Errorf("-h exited %d", got)
	}
}
