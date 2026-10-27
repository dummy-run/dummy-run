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
	"github.com/dummy-run/dummy-run/internal/lighthouse"
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
	var private, offers atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/robots.txt":
			_, _ = fmt.Fprint(w, "User-agent: *\nDisallow: /private\n")
		case strings.HasPrefix(r.URL.Path, "/private"):
			private.Add(1)
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/offer":
			offers.Add(1)
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
			offers.Store(0)
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
			if offers.Load() == 0 {
				t.Error("the frame never asked for /offer, so the redirect wasn't tried")
			}
			if rep.Results[0].RequestsStopped < 2 {
				t.Errorf("requests_stopped is %d, want both frames' loads", rep.Results[0].RequestsStopped)
			}
		})
	}
}

// TestSweepGuardsTabsThePageOpens sweeps a page whose button calls window.open on a page that sends a form as it
// loads. No write may reach the server, and the keyboard check must keep the page's focus, which Firefox would give
// to a new tab.
func TestSweepGuardsTabsThePageOpens(t *testing.T) {
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
<button id="offer" aria-expanded="false" onclick="this.setAttribute('aria-expanded','true');window.open('/offer')">See the offer</button>
</main></body></html>`)
		case "/offer":
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Offer</title></head><body><main><h1>Offer</h1>
<script>fetch("/claim", {method: "POST", body: "offer=1"}).catch(function () {});</script></main></body></html>`)
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
			// The comparison clicked the button, so window.open ran.
			if ts := rep.Results[0].Tasks; ts == nil || len(ts.Tasks) != 1 || !ts.Tasks[0].Mouse {
				t.Fatalf("the button was never pressed, so the test proves nothing: %+v", ts)
			}
			mu.Lock()
			defer mu.Unlock()
			for k := range seen {
				if !strings.HasPrefix(k, "GET ") && !strings.HasPrefix(k, "HEAD ") {
					t.Errorf("%s reached the server: %v", k, seen)
				}
			}
		})
	}
}

// TestLighthouseSendsNoWrites sweeps a page that sends a form as it loads, with Lighthouse on. Lighthouse loads the
// page in a tab of its own over its own connection, and its write must fail too.
func TestLighthouseSendsNoWrites(t *testing.T) {
	if !strings.Contains(os.Getenv("DUMMYRUN_BROWSERS"), "chrome") {
		t.Skip("set DUMMYRUN_BROWSERS=chrome to run the Lighthouse browser test")
	}
	if _, err := lighthouse.Find(); err != nil {
		t.Skip("lighthouse is not installed: ", err)
	}
	var posts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			posts.Add(1)
		}
		if r.URL.Path != "/" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Visit</title></head><body><main>
<h1>Visit</h1><script>fetch("/visit", {method: "POST", body: "seen=1"}).catch(function () {});</script></main></body></html>`)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/"}, Browser: "chrome", Viewports: []Viewport{Desktop},
		Schemes: []string{Light}, Headless: true, Lighthouse: true})
	if err != nil {
		t.Fatal(err)
	}
	r := rep.Results[0]
	if r.Error != "" || !r.LighthouseChecked {
		t.Fatalf("error %q, Lighthouse checked %v (%s)", r.Error, r.LighthouseChecked, r.LighthouseError)
	}
	if n := posts.Load(); n != 0 {
		t.Errorf("%d writes reached the server", n)
	}
	if r.RequestsStopped < 2 {
		t.Errorf("requests_stopped is %d, want the sweep's POST and Lighthouse's", r.RequestsStopped)
	}
}

// TestComparisonTabKeepsToRobots sweeps a page whose menu button also sends the page to an address robots.txt
// disallows, with --respect-robots and the keyboard checks on. The comparison presses the button, and the navigation
// it starts must not reach the server.
func TestComparisonTabKeepsToRobots(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	var private, clicked atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/robots.txt":
			_, _ = fmt.Fprint(w, "User-agent: *\nDisallow: /private\n")
		case r.URL.Path == "/clicked":
			clicked.Add(1)
			w.WriteHeader(http.StatusNoContent)
		case strings.HasPrefix(r.URL.Path, "/private"):
			private.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Account</title></head><body><main>
<h1>Account</h1><button id="menu" aria-expanded="false" onclick="this.setAttribute('aria-expanded','true');fetch('/clicked').finally(function () { location.assign('/private/account'); })">Your account</button>
</main></body></html>`)
		}
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			private.Store(0)
			clicked.Store(0)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/"}, Browser: browser, Viewports: []Viewport{Desktop},
				Schemes: []string{Light}, Headless: true, Keyboard: true, RespectRobots: true})
			if err != nil {
				t.Fatal(err)
			}
			r := rep.Results[0]
			if r.Error != "" {
				t.Fatalf("error: %s", r.Error)
			}
			// The button tells the server it was pressed before it navigates, so the navigation was tried.
			if clicked.Load() == 0 {
				t.Fatalf("the button was never pressed, so the test proves nothing: %+v", r.Tasks)
			}
			if n := private.Load(); n != 0 {
				t.Errorf("a disallowed address was loaded %d times", n)
			}
			if r.RequestsStopped < 1 {
				t.Errorf("requests_stopped is %d, want the comparison's navigation", r.RequestsStopped)
			}
		})
	}
}

// TestFormToNewWindowSendsNoWrite sweeps pages whose button sends a POST form somewhere other than its own frame,
// with the keyboard checks on. Chrome never reports a new tab's first request to the interception, so a form that
// names a new window must stay in the guarded tab, where it fails like any other write. Each page reports the target
// the form ended up with, so the test sees where the script sent it: a new window becomes _self, and a frame on the
// page, a sibling frame included, keeps its name. The formdata page names the new window only from a listener that
// runs after the script has checked the target.
func TestFormToNewWindowSendsNoWrite(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	const fields = `method="post" action="/order"><input type="hidden" name="item" value="1">`
	pages := map[string]struct{ body, press, target, kept string }{
		"/target": {`<form id="order" target="_blank" ` + fields + `</form>`, "order.submit()", "order.getAttribute('target')", "_self"},
		"/formdata": {`<form id="order" ` + fields + `</form>
<script>document.getElementById('order').addEventListener('formdata', function () { this.target = '_blank'; });</script>`,
			"order.submit()", "order.getAttribute('target')", "_self"},
		"/formtarget": {`<form id="order" ` + fields + `<button id="send" formtarget="_blank">Send</button></form>`,
			"document.getElementById('send').click()", "document.getElementById('send').getAttribute('formtarget')", "_self"},
		"/base": {`<base target="_blank"><form id="order" ` + fields + `</form>`,
			"order.submit()", "document.querySelector('base').getAttribute('target')", "_self"},
		"/frame": {`<form id="order" target="result" ` + fields + `</form><iframe name="result" title="Result"></iframe>`,
			"order.submit()", "order.getAttribute('target')", "result"},
		"/onsubmit": {`<form id="order" onsubmit="this.target = '_blank'" ` + fields + `<button id="send">Send</button></form>`,
			"document.getElementById('send').click()", "order.getAttribute('target')", "_self"},
		"/setattribute": {`<form id="order" onsubmit="this.setAttribute('target', '_blank')" ` + fields + `<button id="send">Send</button></form>`,
			"document.getElementById('send').click()", "order.getAttribute('target')", "_self"},
		"/named": {`<script>window.name = '_blank';</script><form id="order" target="_blank" ` + fields + `</form>`,
			"order.submit()", "order.getAttribute('target')", "_self"},
		"/sibling": {`<iframe id="inner" title="Order" srcdoc="<form id='order' target='result' method='post' action='/order'><input type='hidden' name='item' value='1'></form>"></iframe><iframe name="result" title="Result"></iframe>`,
			"order.submit()", "order.getAttribute('target')", "result"},
	}
	for path, p := range pages {
		if p.target == "" {
			continue
		}
		p.target = "fetch('/kept/' + encodeURIComponent(" + p.target + "), {keepalive: true})"
		pages[path] = p
	}
	var mu sync.Mutex
	seen := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.Method+" "+r.URL.Path]++
		mu.Unlock()
		p, ok := pages[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		order := "document.getElementById('order')"
		if r.URL.Path == "/sibling" {
			order = "document.getElementById('inner').contentDocument.getElementById('order')"
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Shop</title></head><body><main><h1>Shop</h1>
`+p.body+`
<button id="buy" aria-expanded="false" onclick="this.setAttribute('aria-expanded','true');fetch('/clicked').finally(function () { var order = `+order+`; `+p.press+`; `+p.target+`; })">Buy now</button>
</main></body></html>`)
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		for path, p := range pages {
			t.Run(browser+path, func(t *testing.T) {
				mu.Lock()
				clear(seen)
				mu.Unlock()
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				defer cancel()
				rep, err := Run(ctx, Config{URLs: []string{srv.URL + path}, Browser: browser, Viewports: []Viewport{Desktop},
					Schemes: []string{Light}, Headless: true, Keyboard: true})
				if err != nil {
					t.Fatal(err)
				}
				r := rep.Results[0]
				if r.Error != "" {
					t.Fatalf("error: %s", r.Error)
				}
				mu.Lock()
				defer mu.Unlock()
				if seen["GET /clicked"] == 0 {
					t.Fatalf("the button was never pressed, so the test proves nothing: %v", seen)
				}
				for k := range seen {
					if !strings.HasPrefix(k, "GET ") && !strings.HasPrefix(k, "HEAD ") {
						t.Errorf("%s reached the server: %v", k, seen)
					}
				}
				if p.kept != "" && seen["GET /kept/"+p.kept] == 0 {
					t.Errorf("the form's target did not end up %q: %v", p.kept, seen)
				}
				if r.RequestsStopped < 1 {
					t.Errorf("requests_stopped is %d, want the form", r.RequestsStopped)
				}
			})
		}
	}
}

// TestOpenedWindowKeepsToRobots sweeps pages whose button opens an address robots.txt disallows in a new window,
// with --respect-robots and the keyboard checks on. The address must never be loaded, in Chrome as in Firefox. On the
// open page, window.open does nothing. On the link page, the link is sent to the page's own tab, where the robots
// guard stops it.
func TestOpenedWindowKeepsToRobots(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	pages := map[string]string{
		"/open": `<button id="account" aria-expanded="false" onclick="this.setAttribute('aria-expanded','true');fetch('/clicked');window.open('/private/account')">Your account</button>`,
		"/onclick": `<a id="link" href="/private/account" onclick="this.target = '_blank'">Account page</a>
<button id="account" aria-expanded="false" onclick="this.setAttribute('aria-expanded','true');fetch('/clicked').finally(function () { document.getElementById('link').click(); })">Your account</button>`,
		"/link": `<a id="link" href="/private/account" target="_blank">Account page</a>
<button id="account" aria-expanded="false" onclick="this.setAttribute('aria-expanded','true');fetch('/clicked').finally(function () { document.getElementById('link').click(); })">Your account</button>`,
	}
	var private, clicked atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/robots.txt":
			_, _ = fmt.Fprint(w, "User-agent: *\nDisallow: /private\n")
		case r.URL.Path == "/clicked":
			clicked.Add(1)
			w.WriteHeader(http.StatusNoContent)
		case strings.HasPrefix(r.URL.Path, "/private"):
			private.Add(1)
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<!doctype html><html lang="en"><title>Account</title><main><h1>Account</h1></main></html>`)
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Shop</title></head><body><main>
<h1>Shop</h1>`+pages[r.URL.Path]+`
</main></body></html>`)
		}
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		for path := range pages {
			t.Run(browser+path, func(t *testing.T) {
				private.Store(0)
				clicked.Store(0)
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				defer cancel()
				rep, err := Run(ctx, Config{URLs: []string{srv.URL + path}, Browser: browser, Viewports: []Viewport{Desktop},
					Schemes: []string{Light}, Headless: true, Keyboard: true, RespectRobots: true})
				if err != nil {
					t.Fatal(err)
				}
				r := rep.Results[0]
				if r.Error != "" {
					t.Fatalf("error: %s", r.Error)
				}
				if clicked.Load() == 0 {
					t.Fatal("the button was never pressed, so the test proves nothing")
				}
				if n := private.Load(); n != 0 {
					t.Errorf("a disallowed address was loaded %d times", n)
				}
				if path != "/open" && r.RequestsStopped < 1 {
					t.Errorf("requests_stopped is %d, want the link", r.RequestsStopped)
				}
			})
		}
	}
}
