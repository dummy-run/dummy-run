package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func exampleReport(t *testing.T, pending bool) *Report {
	t.Helper()
	runs, err := LoadRuns("../../examples/runs")
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Build(runs, Options{IncludePending: pending, Date: fixedDate, ToolVersion: "0.0.0-test"})
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func TestBadgeGolden(t *testing.T) {
	for _, tc := range []struct {
		name, format, file string
		pending            bool
	}{
		{"confirmed svg", "badge", "badge.svg", false},
		{"confirmed json", "shields", "badge.shields.json", false},
		{"pending svg", "badge", "badge-pending.svg", true},
		{"pending json", "shields", "badge-pending.shields.json", true},
	} {
		got := render(t, exampleReport(t, tc.pending), tc.format)
		path := filepath.Join("testdata", "golden", tc.file)
		if *update {
			if err := os.WriteFile(path, []byte(got), 0o644); err != nil { //nolint:gosec // test data
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path) //nolint:gosec // test data
		if err != nil {
			t.Fatalf("%v (run go test ./internal/report -update)", err)
		}
		if got != string(want) {
			t.Errorf("%s differs from %s. Run go test ./internal/report -update and read the diff", tc.name, path)
		}
	}
}

func syntheticReport(basis, ended string, sevs ...string) *Report {
	rep := &Report{Basis: basis, Date: "2026-10-20", Scope: Scope{EndedAt: ended}}
	for _, s := range sevs {
		rep.Findings = append(rep.Findings, Finding{Severity: s})
	}
	return rep
}

func TestBadgeColourAndMessage(t *testing.T) {
	const confirmed = "confirmed findings"
	for _, tc := range []struct {
		rep    *Report
		colour string
		msg    string
	}{
		{syntheticReport(confirmed, "2026-10-20T23:59:00-05:00", "minor", "blocker"), badgeBlocker, "2 open · 21 Oct 2026"},
		{syntheticReport(confirmed, "2026-10-20T10:00:00Z", "info", "major", "minor"), badgeMajor, "3 open · 20 Oct 2026"},
		{syntheticReport(confirmed, "2026-10-20T10:00:00Z", "minor"), badgeMinor, "1 open · 20 Oct 2026"},
		{syntheticReport(confirmed, "2026-10-20T10:00:00Z", "info"), badgeNone, "1 open · 20 Oct 2026"},
		{syntheticReport(confirmed, ""), badgeNone, "0 open · 20 Oct 2026"},
		{syntheticReport("confirmed findings and findings not reviewed", "2026-10-05T10:00:00Z", "major"), badgeMajor, "1 open unreviewed · 5 Oct 2026"},
	} {
		if got := badgeColour(tc.rep); got != tc.colour {
			t.Errorf("colour = %s, want %s", got, tc.colour)
		}
		if got := badgeMessage(tc.rep); got != tc.msg {
			t.Errorf("message = %q, want %q", got, tc.msg)
		}
	}
}

func TestBadgeShape(t *testing.T) {
	for _, pending := range []bool{false, true} {
		svg := render(t, exampleReport(t, pending), "badge")
		if len(svg) >= 2048 {
			t.Errorf("badge is %d bytes, want under 2048", len(svg))
		}
		if !strings.Contains(svg, "<title>Dummy Run: ") {
			t.Error("badge has no title")
		}
		for _, bad := range []string{"<script", "@font-face", "<image", "href="} {
			if strings.Contains(svg, bad) {
				t.Errorf("badge contains %s", bad)
			}
		}
		var v struct {
			SchemaVersion             int
			Label, Message, NamedLogo string
		}
		if err := json.Unmarshal([]byte(render(t, exampleReport(t, pending), "shields")), &v); err != nil {
			t.Fatal(err)
		}
		if v.SchemaVersion != 1 || v.Label != "Dummy Run" || v.NamedLogo != "none" || !strings.Contains(svg, v.Message) {
			t.Errorf("endpoint JSON does not match the badge: %+v", v)
		}
	}
}

// A badge is a sample. It must not read as a verdict or a conformance claim.
func TestBadgeAvoidsVerdictWords(t *testing.T) {
	banned := regexp.MustCompile(`(?i)\b(pass|passed|passes|passing|compliant|accessible|certified)\b`)
	reports := []*Report{exampleReport(t, false), exampleReport(t, true), syntheticReport("confirmed findings", ""), syntheticReport("confirmed findings", "2026-10-20T10:00:00Z", "blocker", "info")}
	for _, rep := range reports {
		for _, format := range []string{"badge", "shields"} {
			if m := banned.FindString(render(t, rep, format)); m != "" {
				t.Errorf("%s output contains %q", format, m)
			}
		}
	}
}
