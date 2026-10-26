package a11y

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/pdfua"
)

func TestPDFsAreCheckedOnceAndOnlyOnTheSweptSite(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	if _, err := pdfua.Find(); err != nil {
		t.Skip("set DUMMYRUN_VERAPDF or put verapdf on the PATH to run the PDF check")
	}
	untagged, err := os.ReadFile("../pdfua/testdata/untagged.pdf")
	if err != nil {
		t.Fatal(err)
	}
	var elsewhereHit atomic.Bool
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		elsewhereHit.Store(true)
		_, _ = w.Write(untagged)
	}))
	defer elsewhere.Close()
	var gets, posts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".pdf") {
			if r.Method != http.MethodGet {
				posts.Add(1)
			}
			gets.Add(1)
		}
		switch r.URL.Path {
		case "/files/report.pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write(untagged)
		case "/files/notes.pdf":
			_, _ = fmt.Fprint(w, "<html>not a pdf</html>")
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Docs</title></head><body><main><h1>Docs</h1>
<a id="r" href="/files/report.pdf">Annual report (PDF)</a> <a href="/files/report.pdf#page=2">Page 2</a>
<a href="/files/notes.pdf">Notes (PDF)</a> <a href="%s/other.pdf">Partner report (PDF)</a>
<div hidden><a href="/files/hidden.pdf">Hidden (PDF)</a></div></main></body></html>`, elsewhere.URL)
		}
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			gets.Store(0)
			posts.Store(0)
			elsewhereHit.Store(false)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/", srv.URL + "/again"}, Browser: browser, Viewports: []Viewport{Desktop},
				Schemes: []string{Light}, Headless: true, PDF: true})
			if err != nil {
				t.Fatal(err)
			}
			if rep.PDFError != "" || len(rep.PDFs) != 2 {
				t.Fatalf("pdf error %q, pdfs %+v", rep.PDFError, rep.PDFs)
			}
			if gets.Load() != 2 || posts.Load() != 0 || elsewhereHit.Load() {
				t.Errorf("%d GETs (want one for each PDF), %d other requests, another host fetched %v", gets.Load(), posts.Load(), elsewhereHit.Load())
			}
			byURL := map[string]PDFCheck{}
			for _, p := range rep.PDFs {
				byURL[strings.TrimPrefix(p.URL, srv.URL)] = p
			}
			report, notes := byURL["/files/report.pdf"], byURL["/files/notes.pdf"]
			if report.Compliant || len(report.Rules) == 0 || report.Page != srv.URL+"/" || report.Link != "#r" {
				t.Errorf("report.pdf: %+v", report)
			}
			if notes.Error == "" {
				t.Errorf("notes.pdf isn't a PDF but has no error: %+v", notes)
			}
			var advice []Node
			for _, r := range rep.Results {
				for _, f := range r.Advice {
					if f.Rule == RulePDFUA {
						advice = append(advice, f.Nodes...)
					}
				}
				for _, f := range r.Violations {
					if f.Rule == RulePDFUA {
						t.Error("a PDF/UA failure is advice, not a failure")
					}
				}
			}
			if len(advice) != 1 || advice[0].Target != "#r" || !strings.Contains(advice[0].Detail, "report.pdf fails") {
				t.Errorf("advice %+v", advice)
			}
		})
	}
}
