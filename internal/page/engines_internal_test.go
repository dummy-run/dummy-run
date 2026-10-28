package page

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestQualWebRunnersFailApart swaps in broken QualWeb bundles. When the WCAG techniques throw or are missing, the ACT
// rules still run and QualWeb counts as run, with the error kept. When the page model is missing, QualWeb does not
// count as run at all.
func TestQualWebRunnersFailApart(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<!doctype html><html lang="en"><title>QualWeb</title><main><h1>QualWeb</h1><img src="data:image/gif;base64,R0lGODlhAQABAAAAACw="></main></html>`)
	}))
	t.Cleanup(srv.Close)
	page, wcag := qwPageJS, qwWCAGJS
	t.Cleanup(func() { qwPageJS, qwWCAGJS = page, wcag })
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			p, err := Open(ctx, Options{Browser: browser, Headless: true, Width: 1200, Height: 800, Hosts: []string{"127.0.0.1"}})
			if err != nil {
				t.Fatalf("open %s: %v", browser, err)
			}
			defer p.Close()
			run := func() *EngineResults {
				// Each run gets a fresh page, so no bundle is left from the last one.
				if o := p.Visit(ctx, srv.URL+"/"); o.Error != "" {
					t.Fatalf("visit: %s", o.Error)
				}
				r, err := RunEngines(ctx, p.Conn(), p.Context(), nil, []string{"1.1.1"})
				if err != nil {
					t.Fatal(err)
				}
				return r
			}
			qwPageJS, qwWCAGJS = page, `globalThis.WCAGTechniquesRunner = function () { throw new Error("techniques broke"); };`
			r := run()
			if !slices.Contains(r.Ran, "qualweb") || !strings.Contains(r.Errors["qualweb"], "techniques broke") {
				t.Errorf("with the techniques broken, the ACT rules should still run: ran %v errors %v", r.Ran, r.Errors)
			}
			qwPageJS, qwWCAGJS = page, ""
			r = run()
			if !slices.Contains(r.Ran, "qualweb") || r.Errors["qualweb"] == "" {
				t.Errorf("with the techniques missing, the ACT rules should still run: ran %v errors %v", r.Ran, r.Errors)
			}
			qwPageJS, qwWCAGJS = "", wcag
			r = run()
			if slices.Contains(r.Ran, "qualweb") || r.Errors["qualweb"] == "" {
				t.Errorf("with no page model, QualWeb should not count as run: ran %v errors %v", r.Ran, r.Errors)
			}
		})
	}
}

// TestAHungEngineTimesOut swaps in an Alfa that never finishes. It is an error for Alfa alone, after its time, and
// the other engines still report.
func TestAHungEngineTimesOut(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<!doctype html><html lang="en"><title>Hung</title><main><h1>Hung</h1><img src="data:image/gif;base64,R0lGODlhAQABAAAAACw="></main></html>`)
	}))
	t.Cleanup(srv.Close)
	alfa, limit := alfaJS, engineTimeout
	t.Cleanup(func() { alfaJS, engineTimeout = alfa, limit })
	alfaJS, engineTimeout = `globalThis.dummyrunAlfa = () => new Promise(() => {});`, 2*time.Second
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			p, err := Open(ctx, Options{Browser: browser, Headless: true, Width: 1200, Height: 800, Hosts: []string{"127.0.0.1"}})
			if err != nil {
				t.Fatalf("open %s: %v", browser, err)
			}
			defer p.Close()
			if o := p.Visit(ctx, srv.URL+"/"); o.Error != "" {
				t.Fatalf("visit: %s", o.Error)
			}
			r, err := RunEngines(ctx, p.Conn(), p.Context(), nil, []string{"1.1.1"})
			if err != nil {
				t.Fatal(err)
			}
			if slices.Contains(r.Ran, "alfa") || !strings.Contains(r.Errors["alfa"], "took more than 2 seconds") {
				t.Errorf("ran %v errors %v", r.Ran, r.Errors)
			}
			if !slices.Contains(r.Ran, "ibm") || !slices.Contains(r.Ran, "htmlcs") || !slices.Contains(r.Ran, "qualweb") {
				t.Errorf("the other engines didn't report: ran %v", r.Ran)
			}
		})
	}
}
