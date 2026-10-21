package access_test

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/access"
	"github.com/dummy-run/dummy-run/internal/access/accesstest"
	"github.com/dummy-run/dummy-run/internal/page"
)

func browsers(t *testing.T) []string {
	t.Helper()
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser test")
	}
	return strings.Split(v, ",")
}

// readOnly is the engine's read-only policy in miniature: GET, HEAD and OPTIONS pass, the rest fail.
func readOnly(r page.Request) page.Decision {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return page.Decision{}
	}
	return page.Decision{Block: true, Reason: "read-only target"}
}

func open(t *testing.T, browser string, acc *access.Resolved) *page.Page {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	p, err := page.Open(ctx, page.Options{Browser: browser, Headless: true, Width: 1000, Height: 700,
		RequestHook: readOnly, Access: acc})
	if err != nil {
		t.Fatalf("open %s: %v", browser, err)
	}
	t.Cleanup(p.Close)
	return p
}

func visit(t *testing.T, p *page.Page, url string) page.Observation {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	o := p.Visit(ctx, url)
	if o.Error != "" {
		t.Fatalf("visit %s: %s", url, o.Error)
	}
	return o
}

// One browser at a time: the subtests run in sequence and each closes its browser before the next opens.
func TestPreviewAccessInBrowser(t *testing.T) {
	for _, browser := range browsers(t) {
		t.Run(browser, func(t *testing.T) {
			fx := accesstest.New(t)
			scope := access.OriginScope([]string{fx.Preview.URL})
			resolved, err := access.Resolve(accesstest.Auth(), accesstest.Getenv, scope)
			if err != nil {
				t.Fatal(err)
			}

			t.Run("without access the preview refuses", func(t *testing.T) {
				p := open(t, browser, nil)
				if o := visit(t, p, fx.Preview.URL+"/bypass"); o.Status != http.StatusForbidden {
					t.Errorf("/bypass status %d, want 403", o.Status)
				}
			})

			t.Run("without access a Basic challenge goes unanswered", func(t *testing.T) {
				// A browser of its own, since Firefox can be left waiting for an answer that never comes.
				p := open(t, browser, nil)
				// Chrome fails the navigation outright on an unanswered challenge. Firefox waits, so the visit is cut short.
				ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
				defer cancel()
				if o := p.Visit(ctx, fx.Preview.URL+"/basic"); o.Error == "" && o.Status != http.StatusUnauthorized {
					t.Errorf("/basic status %d, want a refusal", o.Status)
				}
			})

			t.Run("basic auth and the bypass header get through", func(t *testing.T) {
				p := open(t, browser, resolved)
				for path, title := range map[string]string{"/basic": "Basic ok", "/bypass": "Bypass ok", "/both": "Both ok"} {
					o := visit(t, p, fx.Preview.URL+path)
					if o.Status != http.StatusOK || o.Snapshot == nil || o.Snapshot.Title != title {
						t.Errorf("%s: status %d, snapshot %+v", path, o.Status, o.Snapshot)
					}
				}
			})

			t.Run("a challenge is answered when no header was sent ahead", func(t *testing.T) {
				p := open(t, browser, resolved.ChallengeOnly())
				before := len(fx.Preview.For("/basic"))
				o := visit(t, p, fx.Preview.URL+"/basic")
				if o.Status != http.StatusOK {
					t.Fatalf("/basic status %d; the challenge was not answered", o.Status)
				}
				reqs := fx.Preview.For("/basic")[before:]
				if len(reqs) < 2 || reqs[0].Header.Get("Authorization") != "" || reqs[len(reqs)-1].Header.Get("Authorization") == "" {
					t.Errorf("want a first request with no credentials and a retry with them, got %d requests", len(reqs))
				}
			})

			t.Run("wrong credentials fail without looping", func(t *testing.T) {
				bad, err := access.Resolve(accesstest.Auth(), func(n string) string {
					if n == "PREVIEW_PASS" {
						return "fake-wrong-pass"
					}
					return accesstest.Getenv(n)
				}, scope)
				if err != nil {
					t.Fatal(err)
				}
				for _, acc := range []*access.Resolved{bad, bad.ChallengeOnly()} {
					p := open(t, browser, acc)
					before := len(fx.Preview.For("/basic"))
					if o := visit(t, p, fx.Preview.URL+"/basic"); o.Status != http.StatusUnauthorized {
						t.Errorf("status %d, want 401", o.Status)
					}
					if n := len(fx.Preview.For("/basic")) - before; n > 4 {
						t.Errorf("%d requests for one refused page; the retry loops", n)
					}
				}
			})

			t.Run("a redirect to another host gets no credentials", func(t *testing.T) {
				p := open(t, browser, resolved)
				before := len(fx.Elsewhere.For("/landing"))
				o := visit(t, p, fx.Preview.URL+"/redirect")
				if o.Status != http.StatusOK || o.Snapshot == nil || o.Snapshot.Title != "Elsewhere" {
					t.Fatalf("the redirect did not land: status %d, %+v", o.Status, o.Snapshot)
				}
				got := fx.Elsewhere.For("/landing")[before:]
				if len(got) == 0 {
					t.Fatal("the other host saw no request, so the test proved nothing")
				}
				for _, r := range got {
					for _, name := range []string{"Authorization", accesstest.BypassHeader} {
						if v := r.Header.Get(name); v != "" {
							t.Errorf("the other host received %s", name)
						}
					}
				}
			})

			t.Run("the read-only policy still holds", func(t *testing.T) {
				p := open(t, browser, resolved)
				visit(t, p, fx.Preview.URL+"/form")
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				o := p.Click(ctx, "Send it")
				if len(o.Blocked) == 0 {
					t.Errorf("the POST was not reported as blocked: %+v", o)
				}
				if n := len(fx.Preview.For("/submit")); n != 0 {
					t.Errorf("%d requests reached /submit; the read-only policy was bypassed", n)
				}
			})
		})
	}
}
