package report

import (
	"encoding/json"
	"testing"
)

// The CTRF output has the fields the format requires, the same tests as JUnit, and the raw run counts.
func TestCTRF(t *testing.T) {
	rep, err := Build(loadFixture(t), fixtureOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		ReportFormat string `json:"reportFormat"`
		SpecVersion  string `json:"specVersion"`
		Results      struct {
			Tool    struct{ Name string } `json:"tool"`
			Summary struct {
				Tests, Passed, Failed, Skipped, Pending, Other int
				Start, Stop                                    int64
			} `json:"summary"`
			Tests []struct {
				Name     string         `json:"name"`
				Status   string         `json:"status"`
				Duration *int           `json:"duration"`
				Suite    []string       `json:"suite"`
				Extra    map[string]any `json:"extra"`
			} `json:"tests"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(render(t, rep, "ctrf")), &doc); err != nil {
		t.Fatal(err)
	}
	r := doc.Results
	if doc.ReportFormat != "CTRF" || doc.SpecVersion != ctrfSpecVersion || r.Tool.Name != "dummyrun" {
		t.Errorf("header: %+v", doc)
	}
	want := len(rep.Findings) + len(rep.Axe.Rules)
	if r.Summary.Tests != want || len(r.Tests) != want || r.Summary.Failed+r.Summary.Skipped != want || r.Summary.Start == 0 || r.Summary.Stop < r.Summary.Start {
		t.Errorf("summary %+v for %d tests", r.Summary, len(r.Tests))
	}
	sawRuns := false
	for _, c := range r.Tests {
		if c.Name == "" || c.Duration == nil || len(c.Suite) == 0 || (c.Status != "failed" && c.Status != "skipped") {
			t.Errorf("test %+v", c)
		}
		if _, ok := c.Extra["runs_found"]; ok {
			if _, ok := c.Extra["runs_total"]; !ok {
				t.Errorf("runs_found without runs_total: %+v", c.Extra)
			}
			sawRuns = true
		}
	}
	if !sawRuns {
		t.Error("no persona test carries its run counts")
	}
}
