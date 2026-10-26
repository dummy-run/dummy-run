package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// ctrfSpecVersion is the version of the Common Test Report Format (https://ctrf.io) the report follows.
const ctrfSpecVersion = "0.2.0"

type ctrfReport struct {
	ReportFormat string      `json:"reportFormat"`
	SpecVersion  string      `json:"specVersion"`
	Timestamp    string      `json:"timestamp,omitempty"`
	GeneratedBy  string      `json:"generatedBy"`
	Results      ctrfResults `json:"results"`
}

type ctrfResults struct {
	Tool    ctrfTool    `json:"tool"`
	Summary ctrfSummary `json:"summary"`
	Tests   []ctrfTest  `json:"tests"`
}

type ctrfTool struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type ctrfSummary struct {
	Tests   int   `json:"tests"`
	Passed  int   `json:"passed"`
	Failed  int   `json:"failed"`
	Skipped int   `json:"skipped"`
	Pending int   `json:"pending"`
	Other   int   `json:"other"`
	Start   int64 `json:"start"`
	Stop    int64 `json:"stop"`
}

type ctrfTest struct {
	Name     string         `json:"name"`
	Status   string         `json:"status"`
	Duration int            `json:"duration"`
	Suite    []string       `json:"suite"`
	Message  string         `json:"message,omitempty"`
	Trace    string         `json:"trace,omitempty"`
	Tags     []string       `json:"tags,omitempty"`
	Extra    map[string]any `json:"extra,omitempty"`
}

// renderCTRF writes the same tests as JUnit, as the Common Test Report Format: one for each distinct problem and
// each failing axe rule. Blocker, major and minor problems and failing rules fail, and info problems are skipped.
// The report has no durations, so each is 0. Each test carries the raw counts in extra, such as how many runs found
// a problem out of how many, so a tool that rates stability can work from them. The report rates none itself.
func renderCTRF(w io.Writer, rep *Report) error {
	out := ctrfReport{ReportFormat: "CTRF", SpecVersion: ctrfSpecVersion, GeneratedBy: "dummyrun",
		Results: ctrfResults{Tool: ctrfTool{Name: "dummyrun", Version: rep.ToolVersion}, Tests: []ctrfTest{}}}
	if t, err := time.Parse("2006-01-02", rep.Date); err == nil {
		out.Timestamp = t.UTC().Format(time.RFC3339)
	}
	s := &out.Results.Summary
	s.Start, s.Stop = millis(rep.Scope.StartedAt), millis(rep.Scope.EndedAt)
	for _, f := range rep.Findings {
		t := ctrfTest{Name: f.Title, Status: "failed", Suite: []string{"dummyrun personas", orStr(f.Symptom, f.Kind)},
			Message: f.Severity + ": " + f.Title, Tags: []string{f.Severity, f.Status},
			Trace: fmt.Sprintf("Route %s. Found by %s in %d of %d runs. Fingerprint %s.", f.Route, joinList(f.Personas), f.Runs, f.OfRuns, f.Fingerprint),
			Extra: map[string]any{"fingerprint": f.Fingerprint, "route": f.Route, "runs_found": f.Runs, "runs_total": f.OfRuns, "confidence": f.Confidence}}
		if f.Status != StatusConfirmed {
			t.Message += " (not reviewed)"
		}
		if len(f.WCAG) > 0 {
			t.Extra["wcag"] = f.WCAG
		}
		if f.Severity == "info" {
			t.Status = "skipped"
			s.Skipped++
		} else {
			s.Failed++
		}
		out.Results.Tests = append(out.Results.Tests, t)
	}
	if rep.Axe != nil {
		for _, r := range rep.Axe.Rules {
			var trace strings.Builder
			for _, pg := range r.Pages {
				fmt.Fprintf(&trace, "%s: %s\n", pg.URL, joinList(pg.Targets))
			}
			t := ctrfTest{Name: r.Help, Status: "failed", Suite: []string{"dummyrun axe sweep", r.Rule},
				Message: r.Impact + ": " + r.Help, Trace: trace.String(), Tags: []string{r.Severity, r.Impact},
				Extra: map[string]any{"rule": r.Rule, "elements": r.Elements, "pages": len(r.Pages), "confidence": r.Confidence}}
			if len(r.WCAG) > 0 {
				t.Extra["wcag"] = r.WCAG
			}
			out.Results.Tests = append(out.Results.Tests, t)
			s.Failed++
		}
	}
	s.Tests = len(out.Results.Tests)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// millis is an RFC 3339 time in milliseconds since 1970, or 0 when there is none.
func millis(s string) int64 {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0
	}
	return t.UnixMilli()
}
