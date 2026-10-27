package a11y

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/access"
	"github.com/dummy-run/dummy-run/internal/policy"
)

// TestSweepTabSendsNoWrites sweeps a page whose script sends a form as it loads and again when its button is
// pressed, the way the keyboard check presses it. Nothing but GET and HEAD may reach the server, the page's result
// counts what was stopped, and the page's ordinary GETs still load.
func TestSweepTabSendsNoWrites(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	var mu sync.Mutex
	seen := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.Method+" "+r.URL.Path]++
		mu.Unlock()
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Shop</title></head><body><main><h1>Shop</h1>
<p id="out">Ready.</p><div role="button" tabindex="0" id="buy" onclick="buy()">Buy now</div>
<img src="/pixel.gif" alt="A pixel"></main>
<script>
function buy() { fetch("/order", {method: "POST", body: "item=1"}).catch(function () {}); document.getElementById("out").textContent = "Ordered."; }
fetch("/visit", {method: "POST", body: "seen=1"}).catch(function () {});
</script></body></html>`)
		case "/pixel.gif":
			w.Header().Set("Content-Type", "image/gif")
			_, _ = w.Write([]byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff!\xf9\x04\x01\x00\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;"))
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			mu.Lock()
			clear(seen)
			mu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/"}, Browser: browser, Viewports: []Viewport{Desktop},
				Schemes: []string{Light}, Headless: true, Keyboard: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(rep.Results) != 1 || rep.Results[0].Error != "" {
				t.Fatalf("results: %+v", rep.Results)
			}
			r := rep.Results[0]
			mu.Lock()
			defer mu.Unlock()
			for k := range seen {
				if !strings.HasPrefix(k, "GET ") && !strings.HasPrefix(k, "HEAD ") {
					t.Errorf("%s reached the server: %v", k, seen)
				}
			}
			if seen["GET /pixel.gif"] == 0 {
				t.Errorf("the page's image never loaded, so the guard stopped too much: %v", seen)
			}
			if r.RequestsStopped < 2 {
				t.Errorf("requests_stopped is %d, want at least the load's POST and one press of the button", r.RequestsStopped)
			}
		})
	}
}

// TestSweepTabSendsNoWritesWithPreviewAccess sweeps the same kind of page with a preview access header. A request in
// the sweep's tab is then held both by the access interception and by the tab's guard, and only the guard may answer:
// the page's GETs must carry the header, and its POST must still fail.
func TestSweepTabSendsNoWritesWithPreviewAccess(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	const header, secret = "x-preview-key", "letmein"
	var mu sync.Mutex
	var posts, gets, keyed int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		switch r.Method {
		case http.MethodGet, http.MethodHead:
			gets++
			if r.Header.Get(header) == secret {
				keyed++
			}
		default:
			posts++
		}
		mu.Unlock()
		if r.URL.Path != "/" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Preview</title></head><body><main>
<h1>Preview</h1><script>fetch("/visit", {method: "POST", body: "seen=1"}).catch(function () {});</script></main></body></html>`)
	}))
	defer srv.Close()
	acc, err := access.Resolve(&policy.Auth{Headers: map[string]policy.HeaderSource{header: {ValueEnv: "KEY"}}},
		func(string) string { return secret }, access.OriginScope([]string{srv.URL}))
	if err != nil {
		t.Fatal(err)
	}
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			mu.Lock()
			posts, gets, keyed = 0, 0, 0
			mu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/"}, Browser: browser, Viewports: []Viewport{Desktop},
				Schemes: []string{Light}, Headless: true, Access: acc})
			if err != nil {
				t.Fatal(err)
			}
			if len(rep.Results) != 1 || rep.Results[0].Error != "" {
				t.Fatalf("results: %+v", rep.Results)
			}
			mu.Lock()
			defer mu.Unlock()
			if posts != 0 {
				t.Errorf("%d POSTs reached the server", posts)
			}
			if gets == 0 || keyed != gets {
				t.Errorf("%d of %d GETs carried the access header", keyed, gets)
			}
			if rep.Results[0].RequestsStopped < 1 {
				t.Errorf("requests_stopped is %d, want the load's POST", rep.Results[0].RequestsStopped)
			}
		})
	}
}

// TestSweepTabKeepsToRobots sweeps a page whose frame loads an address robots.txt disallows, once straight and once
// through a redirect. With --respect-robots neither may reach the server, and the page's result counts both.
func TestSweepTabKeepsToRobots(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	var private atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/robots.txt":
			_, _ = fmt.Fprint(w, "User-agent: *\nDisallow: /private\n")
		case strings.HasPrefix(r.URL.Path, "/private"):
			private.Add(1)
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/offer":
			http.Redirect(w, r, "/private/offer", http.StatusFound)
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Robots</title></head><body><main>
<h1>Robots</h1><iframe src="/private/frame" title="Account"></iframe><iframe src="/offer" title="Offer"></iframe></main></body></html>`)
		}
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			private.Store(0)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/"}, Browser: browser, Viewports: []Viewport{Desktop},
				Schemes: []string{Light}, Headless: true, RespectRobots: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(rep.Results) != 1 || rep.Results[0].Error != "" {
				t.Fatalf("results: %+v", rep.Results)
			}
			if n := private.Load(); n != 0 {
				t.Errorf("a disallowed address was loaded %d times", n)
			}
			if rep.Results[0].RequestsStopped < 2 {
				t.Errorf("requests_stopped is %d, want both frames' loads", rep.Results[0].RequestsStopped)
			}
		})
	}
}
