package a11y

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
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
	var gone atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/gone" {
			gone.Add(1)
		}
		mux.ServeHTTP(w, r)
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			gone.Store(0)
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
			if n := gone.Load(); n != 1 {
				t.Errorf("/gone was requested %d times, want once", n)
			}
		})
	}
}

// With --respect-robots, a link robots.txt disallows is never fetched.
func TestLinkCheckRespectsRobots(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	if _, err := links.Find(); err != nil {
		t.Skip("lychee is not installed: ", err)
	}
	var private atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			_, _ = fmt.Fprint(w, "User-agent: *\nDisallow: /private\n")
		case "/private":
			private.Add(1)
		case "/fine":
			_, _ = fmt.Fprint(w, "ok")
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<!doctype html><html lang="en"><title>Links</title><main><h1>Links</h1><a href="/fine">Fine</a> <a href="/private">Private</a></main></html>`)
		}
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			private.Store(0)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/"}, Browser: browser, Viewports: []Viewport{Desktop},
				Schemes: []string{Light}, Headless: true, Links: true, RespectRobots: true})
			if err != nil {
				t.Fatal(err)
			}
			r := rep.Results[0]
			// Like a link lychee leaves out, the skipped link counts as checked and isn't broken.
			if r.LinksError != "" || r.LinksChecked != 2 || len(r.Advice) != 0 {
				t.Errorf("checked %d, error %q, advice %+v", r.LinksChecked, r.LinksError, r.Advice)
			}
			if n := private.Load(); n != 0 {
				t.Errorf("/private was requested %d times", n)
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
			if r.LinksError != "" || r.LinksChecked != 4 {
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
				// /redirect points at the other host. The browser counts the redirect as the answer and doesn't
				// follow it, so the only requests there are lychee's for the direct link.
				if strings.Contains(req.Header.Get("User-Agent"), "Mozilla") {
					t.Errorf("the browser followed a redirect to the other host: %q", req.Header.Get("User-Agent"))
				}
			}
		})
	}
}

// planLinks sends only same-origin links through the browser, and keeps a page's links to private addresses away
// from lychee unless every swept URL is private too.
func TestPlanLinks(t *testing.T) {
	private := func(host string) bool { return strings.HasPrefix(host, "10.") || strings.HasPrefix(host, "127.") }
	links := []string{"https://shop.test/a", "http://shop.test/b", "https://other.test/c", "http://10.0.0.5/admin", "http://127.0.0.1:8080/"}

	p := planLinks([]string{"https://shop.test/"}, true, links, private)
	if strings.Join(p.browser, " ") != "https://shop.test/a" {
		t.Errorf("browser %v: only the swept origin goes through the browser, not http on the same host", p.browser)
	}
	if strings.Join(p.lychee, " ") != "http://shop.test/b https://other.test/c" || p.allowPrivate {
		t.Errorf("lychee %v, allow private %v", p.lychee, p.allowPrivate)
	}
	if strings.Join(p.skipped, " ") != "http://10.0.0.5/admin http://127.0.0.1:8080/" {
		t.Errorf("skipped %v: a public sweep must not check private addresses", p.skipped)
	}

	// A sweep of a local site checks its private links too, and without preview access nothing uses the browser.
	p = planLinks([]string{"http://127.0.0.1:8080/"}, false, links, private)
	if len(p.browser) != 0 || len(p.skipped) != 0 || len(p.lychee) != 5 || !p.allowPrivate {
		t.Errorf("local sweep: %+v", p)
	}

	// One public URL among local ones keeps the filter on.
	p = planLinks([]string{"http://127.0.0.1:8080/", "https://shop.test/"}, false, links, private)
	if p.allowPrivate || len(p.skipped) != 2 {
		t.Errorf("mixed sweep: %+v", p)
	}
}

// A page whose links couldn't be checked says so in the terminal summary.
func TestSummaryListsLinkErrors(t *testing.T) {
	rep := &Report{Browser: "chrome", Results: []PageResult{
		{URL: "https://example.com/a", Browser: "chrome", Viewport: "desktop", Scheme: "light", LinksError: "links: lychee not found"},
	}}
	rep.summarise(Config{Allow: &AllowList{}})
	var b strings.Builder
	rep.WriteSummary(&b)
	for _, want := range []string{"LINKS https://example.com/a (chrome/desktop/light): links: lychee not found", "links not checked on 1 pages"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("summary lacks %q:\n%s", want, b.String())
		}
	}
}

// A link's href goes into the finding's selector as a CSS string: & and < stay as they are, and quotes, backslashes
// and control characters are escaped.
func TestCSSString(t *testing.T) {
	for in, want := range map[string]string{
		"/search?q=a&b=<c>": `"/search?q=a&b=<c>"`,
		`/a"b\c`:            `"/a\"b\\c"`,
		"/a\nb":             `"/a\a b"`,
	} {
		if got := cssString(in); got != want {
			t.Errorf("cssString(%q) = %s, want %s", in, got, want)
		}
	}
}

// A public origin's robots.txt can't send the sweep to a private address by a redirect. A private origin, one you
// chose to sweep, keeps its redirects.
func TestRobotsRedirectStaysPublic(t *testing.T) {
	var internal atomic.Int32
	inside := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		internal.Add(1)
		_, _ = fmt.Fprint(w, "User-agent: *\nAllow: /\n")
	}))
	defer inside.Close()
	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, inside.URL+"/robots.txt", http.StatusFound)
	}))
	defer front.Close()
	// 203.0.113.7 is a public address set aside for documentation, so no DNS is involved. Every request is dialled
	// to the front server.
	dial := func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, strings.TrimPrefix(front.URL, "http://"))
	}
	base := &http.Client{Transport: &http.Transport{DialContext: dial}}
	ctx := context.Background()
	c := robotsClient(ctx, base, "http://203.0.113.7")
	resp, err := c.Get("http://203.0.113.7/robots.txt")
	if err == nil {
		_ = resp.Body.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "private address") {
		t.Errorf("redirect to a private address: %v", err)
	}
	if n := internal.Load(); n != 0 {
		t.Errorf("the private address was requested %d times", n)
	}
	if base.CheckRedirect != nil {
		t.Error("the sweep's own client was changed")
	}
	// An origin whose name can't be looked up, here because the lookup's context has ended, isn't trusted as
	// private, so it gets the guard too. The request itself still runs.
	ended, stop := context.WithCancel(ctx)
	stop()
	resp, err = robotsClient(ended, base, "http://unknown.invalid").Get("http://unknown.invalid/robots.txt")
	if err == nil {
		_ = resp.Body.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "private address") {
		t.Errorf("redirect from an origin that couldn't be looked up: %v", err)
	}
	if n := internal.Load(); n != 0 {
		t.Errorf("the private address was requested %d times", n)
	}
	resp, err = robotsClient(ctx, nil, front.URL).Get(front.URL + "/robots.txt")
	if err != nil {
		t.Fatalf("a private origin's redirect: %v", err)
	}
	_ = resp.Body.Close()
	if internal.Load() != 1 {
		t.Error("a private origin lost its redirect")
	}
}
