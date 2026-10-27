package page_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/page"
)

// TestRefusedNavigationKeepsThePage clicks a link the run refuses. The refusal is reported, and the page the click
// started on is still there for the next step, in Chrome as in Firefox. Chrome puts its own error page in place of
// a navigation that fails, so without the step back every later action would act on that error page.
func TestRefusedNavigationKeepsThePage(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<!doctype html><html lang="en"><title>Start</title><main><h1>Start</h1>`+
			`<a href="/away">Leave the site</a> <a href="/next">Next page</a></main></html>`)
	}))
	t.Cleanup(srv.Close)
	hook := func(r page.Request) page.Decision {
		if r.Navigation && strings.HasSuffix(r.URL, "/away") {
			return page.Decision{Block: true, Reason: "host not allowed"}
		}
		return page.Decision{}
	}
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx := context.Background()
			octx, cancel := context.WithTimeout(ctx, 90*time.Second)
			p, err := page.Open(octx, page.Options{Browser: browser, Headless: true, Width: 1200, Height: 800,
				Hosts: []string{"127.0.0.1"}, RequestHook: hook})
			cancel()
			if err != nil {
				t.Fatalf("open %s: %v", browser, err)
			}
			t.Cleanup(p.Close)
			if o := p.Visit(ctx, srv.URL+"/"); o.Error != "" {
				t.Fatalf("visit: %s", o.Error)
			}
			o := p.Click(ctx, "Leave the site")
			if len(o.Blocked) == 0 || !strings.Contains(strings.Join(o.Blocked, " "), "/away") {
				t.Errorf("the refused navigation is not reported: %v", o.Blocked)
			}
			if o.URL != srv.URL+"/" {
				t.Fatalf("after the refused link the page is %q, want %q", o.URL, srv.URL+"/")
			}
			// The page is usable: the next click works.
			if n := p.Click(ctx, "Next page"); n.Error != "" || n.URL != srv.URL+"/next" {
				t.Errorf("the next click: %q at %q", n.Error, n.URL)
			}
		})
	}
}
