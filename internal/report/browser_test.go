package report

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/a11y"
)

// TestHTMLReportHasNoAxeViolations sweeps the rendered report, with findings, an axe section and a
// baseline, in each browser named in DUMMYRUN_BROWSERS. The report must have no violation at all.
func TestHTMLReportHasNoAxeViolations(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser test")
	}
	opt := fixtureOptions(t)
	base, err := Build(loadFixture(t), Options{Date: fixedDate})
	if err != nil {
		t.Fatal(err)
	}
	opt.Baseline = base
	rep, err := Build(loadFixture(t), opt)
	if err != nil {
		t.Fatal(err)
	}
	page := render(t, rep, "html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	}))
	t.Cleanup(srv.Close)
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			sweep, err := a11y.Run(ctx, a11y.Config{URLs: []string{srv.URL}, Browser: browser,
				Viewports: []a11y.Viewport{a11y.Desktop, a11y.Phone}, Schemes: []string{a11y.Light, a11y.Dark}, Headless: true})
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range sweep.Results {
				if r.Error != "" {
					t.Fatalf("%s/%s: %s", r.Viewport, r.Scheme, r.Error)
				}
				for _, f := range r.Violations {
					t.Errorf("%s/%s: %s (%s) at %v", r.Viewport, r.Scheme, f.Rule, f.Impact, f.Nodes)
				}
			}
		})
	}
}
