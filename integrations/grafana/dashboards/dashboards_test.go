package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const logoPath = "../../../brand/logo/lockup-horizontal-dark.svg"

func testConfig(t *testing.T) Config {
	t.Helper()
	svg, err := os.ReadFile(logoPath)
	if err != nil {
		t.Fatalf("read logo: %v", err)
	}
	return Config{PrometheusUID: "prometheus", LokiUID: "loki", TempoUID: "tempo", LogoSVG: string(svg)}
}

// panel is the subset of a Grafana panel the tests look at.
type panel struct {
	ID          uint32           `json:"id"`
	Type        string           `json:"type"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Datasource  *json.RawMessage `json:"datasource"`
	Targets     []map[string]any `json:"targets"`
	GridPos     struct{ H, W, X, Y uint32 }
	Repeat      string  `json:"repeat"`
	Panels      []panel `json:"panels"`
}

type dash struct {
	UID         string   `json:"uid"`
	Title       string   `json:"title"`
	Tags        []string `json:"tags"`
	Panels      []panel  `json:"panels"`
	Templating  struct{ List []map[string]any }
	Annotations struct{ List []map[string]any }
}

func rendered(t *testing.T) map[string]dash {
	t.Helper()
	files, err := testConfig(t).Render()
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	out := map[string]dash{}
	for name, raw := range files {
		var d dash
		if err := json.Unmarshal(raw, &d); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out[name] = d
	}
	return out
}

// TestCommittedJSONIsCurrent is the generate and diff check. CI also runs `go run . -check`.
func TestCommittedJSONIsCurrent(t *testing.T) {
	files, err := testConfig(t).Render()
	if err != nil {
		t.Fatal(err)
	}
	for name, raw := range files {
		current, err := os.ReadFile(filepath.Join("json", name))
		if err != nil {
			t.Fatalf("%s: %v (run `go run .`)", name, err)
		}
		if !bytes.Equal(current, raw) {
			t.Errorf("json/%s is out of date, run `go run .` and commit the result", name)
		}
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	c := testConfig(t)
	a, _ := c.Render()
	b, _ := c.Render()
	for name := range a {
		if !bytes.Equal(a[name], b[name]) {
			t.Errorf("%s differs between two renders", name)
		}
	}
}

func TestEveryDashboardHasFiltersAndAnnotations(t *testing.T) {
	for name, d := range rendered(t) {
		if name == "home.json" {
			continue
		}
		if !strings.HasPrefix(d.Title, "Dummy Run: ") {
			t.Errorf("%s: title %q does not start with \"Dummy Run: \"", name, d.Title)
		}
		if !strings.HasPrefix(d.UID, "dummyrun-") {
			t.Errorf("%s: uid %q", name, d.UID)
		}
		vars := map[string]bool{}
		for _, v := range d.Templating.List {
			vars[v["name"].(string)] = true
		}
		for _, want := range []string{"environment", "persona", "scenario"} {
			if !vars[want] {
				t.Errorf("%s: missing variable %s", name, want)
			}
		}
		found := false
		for _, a := range d.Annotations.List {
			if a["name"] == "Dummy Run runs" {
				found = true
				target := a["target"].(map[string]any)
				if tags := target["tags"].([]any); len(tags) != 1 || tags[0] != tagDummyRun {
					t.Errorf("%s: annotation tags %v", name, tags)
				}
			}
		}
		if !found {
			t.Errorf("%s: missing the Dummy Run runs annotation", name)
		}
		if len(d.Tags) != 1 || d.Tags[0] != tagDummyRun {
			t.Errorf("%s: tags %v", name, d.Tags)
		}
	}
}

func flatten(ps []panel) []panel {
	var out []panel
	for _, p := range ps {
		out = append(out, p)
		out = append(out, flatten(p.Panels)...)
	}
	return out
}

var knownMetrics = []string{metricRuns, metricRunDuration, metricSteps, metricStepBucket, metricFindings, metricTokens, metricCost}

func TestPanelsAreWellFormed(t *testing.T) {
	metricRe := regexp.MustCompile(`dummyrun_[a-zA-Z_]+`)
	for name, d := range rendered(t) {
		seen := map[uint32]bool{}
		for _, p := range flatten(d.Panels) {
			where := name + ": " + p.Title
			if p.ID == 0 || seen[p.ID] {
				t.Errorf("%s: panel id %d missing or duplicated", where, p.ID)
			}
			seen[p.ID] = true
			if p.GridPos.W == 0 || p.GridPos.W > 24 || p.GridPos.X+p.GridPos.W > 24 {
				t.Errorf("%s: grid position %+v", where, p.GridPos)
			}
			if p.Type == "row" || p.Type == "text" || p.Type == "dashlist" {
				continue
			}
			if p.Datasource == nil || len(p.Targets) == 0 {
				t.Errorf("%s: no data source or no query", where)
			}
			if p.Description == "" {
				t.Errorf("%s: no description", where)
			}
			for _, target := range p.Targets {
				expr, _ := target["expr"].(string)
				if ds, _ := target["datasource"].(map[string]any); ds != nil && ds["type"] == "prometheus" {
					for _, m := range metricRe.FindAllString(expr, -1) {
						if !strings.HasPrefix(m, "dummyrun_") {
							continue
						}
						if !isMetricOrLabel(m) {
							t.Errorf("%s: unknown metric or label %q", where, m)
						}
					}
				}
				if strings.Contains(expr, `{service_name="dummyrun"}`) && name != "run.json" && !strings.Contains(expr, "$persona") && !strings.Contains(expr, "$run") {
					t.Errorf("%s: Loki query ignores the persona filter: %s", where, expr)
				}
			}
		}
	}
}

var knownLabels = []string{
	"dummyrun_target_environment", "dummyrun_persona_id", "dummyrun_scenario_id", "dummyrun_outcome",
	"dummyrun_action", "dummyrun_kind", "dummyrun_severity", "dummyrun_verdict", "dummyrun_direction",
}

func isMetricOrLabel(s string) bool {
	for _, m := range knownMetrics {
		if s == m || s == strings.TrimSuffix(m, "_bucket") {
			return true
		}
	}
	for _, l := range knownLabels {
		if s == l {
			return true
		}
	}
	return false
}

// TestProseRules applies docs/STYLE.md to titles and descriptions: at most two commas per
// sentence, no em dashes, no signposts and no staged contrasts.
func TestProseRules(t *testing.T) {
	signpost := regexp.MustCompile(`(?i)\b(note that|in short|worth noting|importantly|simply put)\b`)
	staged := regexp.MustCompile(`(?i)\bnot\b[^.;:]{1,80}?,\s*but\b|,\s+not\s+\w+`)
	for name, d := range rendered(t) {
		texts := []string{d.Title}
		for _, p := range flatten(d.Panels) {
			texts = append(texts, p.Title, p.Description)
		}
		for _, text := range texts {
			if strings.ContainsRune(text, '—') {
				t.Errorf("%s: em dash in %q", name, text)
			}
			if signpost.MatchString(text) {
				t.Errorf("%s: signpost in %q", name, text)
			}
			if m := staged.FindString(text); m != "" && !strings.Contains(strings.ToLower(m), "not only") {
				t.Errorf("%s: staged contrast in %q", name, text)
			}
			for _, sentence := range regexp.MustCompile(`[.!?]\s+`).Split(text, -1) {
				if strings.Count(sentence, ",") > 2 {
					t.Errorf("%s: more than two commas in %q", name, sentence)
				}
			}
		}
	}
}

func TestBrandColours(t *testing.T) {
	brand, err := os.ReadFile("../../../brand/README.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, hex := range []string{colourPass, colourBlocker, colourAmber, colourInk} {
		if !bytes.Contains(brand, []byte(hex)) {
			t.Errorf("colour %s is not in brand/README.md", hex)
		}
	}
}

func TestLinksCarryTheCellValue(t *testing.T) {
	for _, u := range []string{exploreTempoURL("tempo"), exploreLokiURL("loki", `{a="__DR_VALUE__"}`), runDashboardURL()} {
		if !strings.Contains(u, "${__value.raw}") {
			t.Errorf("link %q does not use the cell value", u)
		}
		if strings.Contains(u, "__DR_") {
			t.Errorf("link %q still has a placeholder", u)
		}
	}
	// A data source UID or query must survive the placeholder substitution untouched.
	u := exploreLokiURL("loki-TOKYO", `{a="__DR_VALUE__"} |= "FROM TO"`)
	if !strings.Contains(u, "loki-TOKYO") || !strings.Contains(u, "FROM+TO") {
		t.Errorf("link %q changed the UID or the query", u)
	}
}

func TestDataSourceUIDsAreConfigurable(t *testing.T) {
	c := Config{PrometheusUID: "P1", LokiUID: "L1", TempoUID: "T1"}
	files, err := c.Render()
	if err != nil {
		t.Fatal(err)
	}
	for name, raw := range files {
		if name == "home.json" {
			continue
		}
		for _, uid := range []string{`"uid": "prometheus"`, `"uid": "loki"`, `"uid": "tempo"`} {
			if bytes.Contains(raw, []byte(uid)) {
				t.Errorf("%s still refers to %s", name, uid)
			}
		}
	}
}

func TestHomeHasLogo(t *testing.T) {
	files, err := testConfig(t).Render()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(files["home.json"], []byte("data:image/svg+xml;base64,")) {
		t.Error("home dashboard has no embedded logo")
	}
}
