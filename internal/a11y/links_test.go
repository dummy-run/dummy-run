package a11y

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/access"
	"github.com/dummy-run/dummy-run/internal/access/accesstest"
	"github.com/dummy-run/dummy-run/internal/links"
)

func TestLinkCheckReportsBrokenLinks(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	if _, err := links.Find(); err != nil {
		t.Skip("lychee is not installed: ", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!doctype html><html lang="en"><title>Links</title><main><h1>Links</h1>
<a href="/fine">Fine</a> <a href="/fine#part">Fine again</a> <a href="/gone">Gone</a> <a href="/broken">Broken</a>
<a href="/wall">Wall</a> <a href="mailto:a@example.test">Mail</a> <a href="http://nohost.invalid/">Nowhere</a>
<a href="/second">Second</a></main></html>`)
	})
	mux.HandleFunc("/second", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!doctype html><html lang="en"><title>Second</title><main><h1>Second</h1><a href="/gone">Gone too</a></main></html>`)
	})
	mux.HandleFunc("/fine", func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, "ok") })
	mux.HandleFunc("/gone", http.NotFound)
	mux.HandleFunc("/broken", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
	mux.HandleFunc("/wall", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) })
	var gone int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/gone" {
			gone++
		}
		mux.ServeHTTP(w, r)
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			gone = 0
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/", srv.URL + "/second"}, Browser: browser,
				Viewports: []Viewport{Desktop, Phone}, Schemes: []string{Light}, Headless: true, Links: true})
			if err != nil {
				t.Fatal(err)
			}
			if rep.Lychee != links.LycheeVersion {
				t.Errorf("lychee version %q", rep.Lychee)
			}
			broken := map[string]string{}
			first := map[string]PageResult{}
			for _, r := range rep.Results {
				if r.LinksError != "" {
					t.Fatalf("%s: %s", r.URL, r.LinksError)
				}
				if r.LinksChecked > 0 {
					if _, dup := first[r.URL]; dup {
						t.Errorf("%s checked twice", r.URL)
					}
					first[r.URL] = r
				}
				for _, f := range r.Advice {
					if f.Rule == RuleBrokenLink {
						for _, n := range f.Nodes {
							broken[r.URL+" "+n.Target] = n.Detail
						}
					}
				}
			}
			home := first[srv.URL+"/"]
			if home.LinksChecked != 6 || home.LinksRefused != 1 {
				t.Errorf("home checked %d refused %d, want 6 and 1 (the fragment link is the same page)", home.LinksChecked, home.LinksRefused)
			}
			for _, want := range []struct{ key, detail string }{
				{srv.URL + `/ a[href="/gone"]`, "/gone: HTTP 404"},
				{srv.URL + `/ a[href="/broken"]`, "/broken: HTTP 500"},
				{srv.URL + `/second a[href="/gone"]`, "/gone: HTTP 404"},
			} {
				if !strings.Contains(broken[want.key], want.detail) {
					t.Errorf("%s: got %q, want %q in %v", want.key, broken[want.key], want.detail, broken)
				}
			}
			if len(broken) != 3 {
				t.Errorf("broken %v", broken)
			}
			// The second page's link to /gone was already checked from the first page.
			if gone != 1 {
				t.Errorf("/gone was requested %d times, want once", gone)
			}
		})
	}
}

// Behind preview access, links on the preview are checked from the browser, which holds the credentials, and
// lychee only gets the other host's link, with no credentials.
func TestLinkCheckBehindPreviewAccess(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	if _, err := links.Find(); err != nil {
		t.Skip("lychee is not installed: ", err)
	}
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			fx := accesstest.New(t)
			acc, err := access.Resolve(accesstest.Auth(), accesstest.Getenv, access.OriginScope([]string{fx.Preview.URL}))
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{fx.Preview.URL + "/links"}, Browser: browser, Viewports: []Viewport{Desktop},
				Schemes: []string{Light}, Headless: true, Access: acc, Links: true})
			if err != nil {
				t.Fatal(err)
			}
			r := rep.Results[0]
			if r.LinksError != "" || r.LinksChecked != 3 {
				t.Fatalf("checked %d, error %q", r.LinksChecked, r.LinksError)
			}
			var broken []string
			for _, f := range r.Advice {
				if f.Rule == RuleBrokenLink {
					for _, n := range f.Nodes {
						broken = append(broken, n.Detail)
					}
				}
			}
			// Without the credentials /missing would answer 401, which counts as refused, not broken.
			if len(broken) != 1 || !strings.Contains(broken[0], "/missing: HTTP 404") {
				t.Errorf("broken %v, want only /missing with 404", broken)
			}
			if len(fx.Elsewhere.For("/landing")) == 0 {
				t.Fatal("the other host's link was not checked")
			}
			for _, req := range fx.Elsewhere.For("/landing") {
				if req.Header.Get("Authorization") != "" || req.Header.Get(accesstest.BypassHeader) != "" {
					t.Error("the other host received the credentials")
				}
			}
		})
	}
}
