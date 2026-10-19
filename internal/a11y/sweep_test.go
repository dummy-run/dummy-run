package a11y

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

//nolint:misspell // the axe rule ID
const ruleContrast = "color-contrast"

//nolint:misspell // CSS property and media feature names
const fixtureBad = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Fixture</title>
<style>
body { background: #fff; color: #000; }
@media (prefers-color-scheme: dark) { body { background: #000; color: #fff; } }
.faint { color: #ccc; background: #fff; }
@media (prefers-color-scheme: dark) { .faint { color: #222; background: #000; } }
</style></head><body><main>
<h1>Known violations</h1>
<img id="logo" src="data:image/gif;base64,R0lGODlhAQABAAAAACH5BAEKAAEALAAAAAABAAEAAAICTAEAOw==">
<p class="faint" id="faint">This text has too little contrast.</p>
<button id="nameless"></button>
</main></body></html>`

const fixtureGood = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Clean</title></head>
<body><main><h1>Clean page</h1><p>Nothing to see.</p></main></body></html>`

func fixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/bad", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, fixtureBad)
	})
	mux.HandleFunc("/good", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, fixtureGood)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestCriteria(t *testing.T) {
	got := Criteria([]string{"cat.color", "wcag2aa", "wcag143", "wcag21aa", "wcag412", "wcag1410"})
	if want := []string{"1.4.10", "1.4.3", "4.1.2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestColourSchemeLaunch(t *testing.T) {
	for _, b := range []string{"chrome", "firefox"} {
		for _, s := range []string{Light, Dark} {
			args, prefs, err := ColourSchemeLaunch(b, s)
			if err != nil || len(args)+len(prefs) == 0 {
				t.Errorf("%s %s: args %v prefs %v err %v", b, s, args, prefs, err)
			}
		}
	}
	if _, _, err := ColourSchemeLaunch("chrome", "sepia"); err == nil {
		t.Error("an unknown scheme must be refused")
	}
	if _, _, err := ColourSchemeLaunch("safari", Dark); err == nil {
		t.Error("an unknown browser must be refused")
	}
}

func TestRunNeedsURLs(t *testing.T) {
	if _, err := Run(context.Background(), Config{Browser: "chrome"}); err == nil {
		t.Fatal("no URLs must be an error")
	}
}

// TestConformanceSweepFixture runs the sweep against a local page with known violations in each browser named
// in DUMMYRUN_BROWSERS, such as "chrome,firefox". Without it the test is skipped.
func TestConformanceSweepFixture(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser conformance test")
	}
	srv := fixtureServer(t)
	allow, err := ParseAllowList(strings.NewReader("image-alt | /bad | #logo | The fixture's missing alt text is accepted here\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{
				URLs:      []string{srv.URL + "/bad", srv.URL + "/good"},
				Browser:   browser,
				Viewports: []Viewport{Desktop, Phone},
				Schemes:   []string{Light, Dark},
				Allow:     allow,
				Headless:  true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(rep.Results) != 8 {
				t.Fatalf("want 8 runs (2 pages, 2 viewports, 2 schemes), got %d", len(rep.Results))
			}
			if rep.AxeVersion == "" || rep.BrowserVersion == "" {
				t.Errorf("missing versions: axe %q browser %q", rep.AxeVersion, rep.BrowserVersion)
			}
			for _, r := range rep.Results {
				if r.Error != "" {
					t.Fatalf("%s %s/%s: %s", r.URL, r.Viewport, r.Scheme, r.Error)
				}
				if r.Scheme != r.SchemeActual {
					t.Errorf("asked for %s, browser rendered %s", r.Scheme, r.SchemeActual)
				}
				if strings.HasSuffix(r.URL, "/good") {
					if len(r.Violations) != 0 {
						t.Errorf("clean page %s/%s has violations: %+v", r.Viewport, r.Scheme, r.Violations)
					}
					continue
				}
				got := map[string]Finding{}
				for _, f := range r.Violations {
					got[f.Rule] = f
				}
				for _, rule := range []string{"image-alt", "button-name", ruleContrast} {
					if _, ok := got[rule]; !ok {
						t.Errorf("%s/%s: expected a %s violation, got %v", r.Viewport, r.Scheme, rule, keys(got))
					}
				}
				if f, ok := got["image-alt"]; ok {
					if len(f.Nodes) != 1 || !f.Nodes[0].Allowed || f.Nodes[0].Reason == "" {
						t.Errorf("image-alt should be allowed with its reason: %+v", f.Nodes)
					}
				}
				if f, ok := got["button-name"]; ok {
					if f.Impact == "" || f.HelpURL == "" || !reflect.DeepEqual(f.WCAG, []string{"4.1.2"}) || f.Nodes[0].Allowed {
						t.Errorf("button-name finding incomplete or wrongly allowed: %+v", f)
					}
					if !strings.Contains(f.Nodes[0].Target, "nameless") {
						t.Errorf("button-name target %q does not name the button", f.Nodes[0].Target)
					}
				}
				if r.Failing() != 2 {
					t.Errorf("%s/%s: want 2 failing nodes (button-name, contrast), got %d", r.Viewport, r.Scheme, r.Failing())
				}
			}
			if !rep.Failed() {
				t.Error("a report with failing nodes must say it failed")
			}
			if rep.Summary.Allowed != 4 || len(allow.Unused()) != 0 {
				t.Errorf("want 4 allowed nodes and no unused entries, got %d and %d", rep.Summary.Allowed, len(allow.Unused()))
			}
			var sum strings.Builder
			rep.WriteSummary(&sum)
			for _, want := range []string{"FAIL ", "button-name", ruleContrast, "WCAG 4.1.2", "dequeuniversity.com"} {
				if !strings.Contains(sum.String(), want) {
					t.Errorf("summary lacks %q:\n%s", want, sum.String())
				}
			}
		})
	}
}

func keys(m map[string]Finding) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
