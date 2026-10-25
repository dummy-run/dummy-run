package benchmark

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const fixtures = "../../testdata/benchmark"

func readPages(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(fixtures, "pages", "*.html"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixture pages: %v", err)
	}
	pages := map[string]string{}
	for _, f := range files {
		b, err := os.ReadFile(f) //nolint:gosec // fixture files
		if err != nil {
			t.Fatal(err)
		}
		pages[strings.TrimSuffix(filepath.Base(f), ".html")] = string(b)
	}
	return pages
}

func TestEveryDefectHasAPageToLandOn(t *testing.T) {
	pages := readPages(t)
	for _, d := range Defects {
		if d.ID == "" || d.Describe == "" || len(d.Rules) == 0 || d.apply == nil {
			t.Errorf("defect %+v is missing a field", d.ID)
			continue
		}
		landed := 0
		for name, src := range pages {
			out, ok := d.Plant(src)
			if !ok {
				continue
			}
			landed++
			if out == src {
				t.Errorf("%s on %s planted nothing", d.ID, name)
			}
			if _, again := d.Plant(out); again && (d.ID == "remove-label" || d.ID == "remove-title") {
				t.Errorf("%s on %s found something to change in its own output", d.ID, name)
			}
		}
		if landed == 0 {
			t.Errorf("%s fits none of the fixture pages", d.ID)
		}
	}
}

func TestPlantChangesOnlyWhatItSays(t *testing.T) {
	src := readPages(t)["signup"]
	cases := map[string]struct{ gone, there string }{
		"remove-label":        {"<label", `id="first"`},
		"remove-lang":         {`lang="en"`, "<title>"},
		"remove-title":        {"<title>", "<h1>"},
		"remove-autocomplete": {`autocomplete="email"`, `autocomplete="given-name"`},
		"break-skip-link":     {`href="#content"`, `href="#nowhere"`},
	}
	for id, c := range cases {
		d, _ := Lookup(id)
		out, ok := d.Plant(src)
		if !ok {
			t.Fatalf("%s did not land on the sign-up page", id)
		}
		if strings.Contains(out, c.gone) && id != "break-skip-link" {
			t.Errorf("%s left %q behind", id, c.gone)
		}
		if !strings.Contains(out, c.there) {
			t.Errorf("%s lost %q", id, c.there)
		}
	}
	if _, ok := Lookup("nothing"); ok {
		t.Error("Lookup found a defect that is not there")
	}
}

func TestPlantNeedsSomethingToChange(t *testing.T) {
	d, _ := Lookup("remove-alt")
	if _, ok := d.Plant(`<!doctype html><html lang="en"><head><title>x</title></head><body><p>No pictures.</p></body></html>`); ok {
		t.Error("remove-alt planted a defect in a page with no image")
	}
}

func TestScoreCountsRecallPerCheck(t *testing.T) {
	outs := []Outcome{
		{Page: "a", Defect: "d1", Rules: []string{"label"}, After: []string{"label"}},
		{Page: "b", Defect: "d1", Rules: []string{"label"}, After: nil},
		// A rule the clean page already showed is not a find.
		{Page: "c", Defect: "d1", Rules: []string{"label"}, Before: []string{"label"}, After: []string{"label"}},
		{Page: "a", Defect: "d2", Rules: []string{"image-alt", "other"}, After: []string{"other"}},
		{Page: "a", Defect: "d3", Rules: []string{"x"}, Error: "boom"},
	}
	s := ScoreOutcomes(outs)
	if s.Planted != 4 || s.Found != 2 || s.Errors != 1 {
		t.Fatalf("totals %+v", s)
	}
	if len(s.Checks) != 2 || s.Checks[0].Rule != "image-alt" || s.Checks[1].Rule != "label" {
		t.Fatalf("checks %+v", s.Checks)
	}
	if got := s.Checks[1]; got.Planted != 3 || got.Found != 1 || len(got.Missed) != 2 {
		t.Errorf("label %+v", got)
	}
	var buf bytes.Buffer
	s.Write(&buf)
	for _, want := range []string{"label", "33%", "1 cases could not be run"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("table lacks %q:\n%s", want, buf.String())
		}
	}
	if (ScoreOutcomes(nil)).Recall != 0 {
		t.Error("empty score has recall")
	}
}

func TestAccessGuruCasesLoad(t *testing.T) {
	cs, err := LoadAccessGuru(filepath.Join(fixtures, "accessguru", "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 150 {
		t.Fatalf("%d cases, want 150", len(cs))
	}
	seen := map[string]bool{}
	for _, c := range cs {
		if seen[c.ID] {
			t.Errorf("case %s is listed twice", c.ID)
		}
		seen[c.ID] = true
		if !strings.Contains(c.Page(), c.HTML) {
			t.Errorf("case %s: the page lost the element", c.ID)
		}
	}
	if !c0found(cs) {
		t.Error("no link-name case")
	}
	if _, err := LoadAccessGuru(filepath.Join(fixtures, "nope.json")); err == nil {
		t.Error("a missing file loaded")
	}
}

func c0found(cs []AccessGuruCase) bool {
	for _, c := range cs {
		if c.Rule == "link-name" {
			return true
		}
	}
	return false
}

func TestAccessGuruLoaderRejectsAHalfFilledCase(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.json")
	if err := os.WriteFile(p, []byte(`[{"id":"a","rule":"","html":"<p>"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAccessGuru(p); err == nil {
		t.Error("a case with no rule loaded")
	}
}

// TestBenchmark plants every defect in every page it fits, and the AccessGuru elements, and prints recall per check.
// It needs a browser: set DUMMYRUN_BROWSERS to chrome or firefox. DUMMYRUN_BENCHMARK_OUT names a file for the score
// as JSON. The browsers run one after the other.
func TestBenchmark(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome to run the benchmark")
	}
	pages := readPages(t)
	cases, err := LoadAccessGuru(filepath.Join(fixtures, "accessguru", "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Minute)
			defer cancel()
			r := &Runner{Browser: browser}
			r.Start()
			defer r.Close()
			var outs []Outcome
			for name, src := range pages {
				// Opening the clean page with the keyboard and markup checks on gives each defect its baseline.
				before, err := r.Rules(ctx, "/"+name, src, true, true)
				if err != nil {
					t.Fatal(err)
				}
				for _, d := range Defects {
					planted, ok := d.Plant(src)
					if !ok {
						continue
					}
					o := Outcome{Page: name, Defect: d.ID, Rules: d.Rules, Before: before}
					o.After, err = r.Rules(ctx, "/"+name+"-"+d.ID, planted, d.Keyboard, d.Markup)
					if err != nil {
						o.Error = err.Error()
					}
					outs = append(outs, o)
				}
			}
			s := ScoreOutcomes(outs)
			t.Logf("planted defects")
			var buf bytes.Buffer
			s.Write(&buf)
			t.Log("\n" + buf.String())

			var ag []Outcome
			for _, c := range cases {
				o := c.Outcome(nil)
				o.After, err = r.Rules(ctx, "/ag-"+c.ID, c.Page(), false, false)
				if err != nil {
					o.Error = err.Error()
				}
				ag = append(ag, o)
			}
			as := ScoreOutcomes(ag)
			buf.Reset()
			as.Write(&buf)
			t.Log("\nAccessGuru elements\n" + buf.String())

			if out := os.Getenv("DUMMYRUN_BENCHMARK_OUT"); out != "" {
				if len(strings.Split(v, ",")) > 1 {
					// One file for each browser, so a later browser doesn't replace an earlier one's scores.
					out = strings.TrimSuffix(out, filepath.Ext(out)) + "." + browser + filepath.Ext(out)
				}
				b, _ := json.MarshalIndent(map[string]Score{"planted": s, "accessguru": as}, "", " ")
				if err := os.WriteFile(out, b, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if s.Errors > 0 || as.Errors > 0 {
				t.Errorf("%d planted and %d AccessGuru cases could not be run", s.Errors, as.Errors)
			}
		})
	}
}
