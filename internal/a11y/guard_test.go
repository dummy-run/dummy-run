package a11y

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/access"
	"github.com/dummy-run/dummy-run/internal/bidi"
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
		"/null": {`<form id="order" onsubmit="this.setAttribute('target', null)" ` + fields + `<button id="send">Send</button></form>`,
			"document.getElementById('send').click()", "order.getAttribute('target')", "_self"},
		"/newline": {`<script>window.name = 'a\n<b';</script><form id="order" target="a&#10;&lt;b" ` + fields + `</form>`,
			"order.submit()", "order.getAttribute('target')", "_self"},
		"/attrnode": {`<div id="src" target="_blank" hidden></div><form id="order" onsubmit="this.setAttributeNode(document.getElementById('src').getAttributeNode('target').cloneNode())" ` + fields + `<button id="send">Send</button></form>`,
			"document.getElementById('send').click()", "order.getAttribute('target')", "_self"},
		"/nameditem": {`<div id="src" target="_blank" hidden></div><form id="order" onsubmit="this.attributes.setNamedItem(document.getElementById('src').getAttributeNode('target').cloneNode())" ` + fields + `<button id="send">Send</button></form>`,
			"document.getElementById('send').click()", "order.getAttribute('target')", "_self"},
		"/attrvalue": {`<form id="order" target="_self" onsubmit="this.getAttributeNode('target').value = '_blank'" ` + fields + `<button id="send">Send</button></form>`,
			"document.getElementById('send').click()", "order.getAttribute('target')", "_self"},
		"/shadow": {`<div id="host"></div>
<script>document.getElementById('host').attachShadow({mode: 'open'}).innerHTML = '<form id="order" target="_blank" method="post" action="/order"><input type="hidden" name="item" value="1"><button id="send">Send</button></form>';</script>`,
			"order.querySelector('button').dispatchEvent(new MouseEvent('click', {bubbles: true, composed: true}))", "order.getAttribute('target')", "_self"},
		"/declarative": {`<shop-order><template shadowrootmode="closed"><form id="order" target="_blank" method="post" action="/order"><input type="hidden" name="item" value="1"><button>Send</button></form></template></shop-order>
<script>customElements.define('shop-order', class extends HTMLElement { constructor() { super(); this.internals = this.attachInternals(); } });</script>`,
			"order.querySelector('button').dispatchEvent(new MouseEvent('click', {bubbles: true, composed: true}))", "order.getAttribute('target')", "_self"},
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
		switch r.URL.Path {
		case "/sibling":
			order = "document.getElementById('inner').contentDocument.getElementById('order')"
		case "/shadow":
			order = "document.getElementById('host').shadowRoot.getElementById('order')"
		case "/declarative":
			order = "document.querySelector('shop-order').internals.shadowRoot.getElementById('order')"
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
		"/declarative": `<shop-account><template shadowrootmode="closed"><a href="/private/account" target="_blank">Account page</a></template></shop-account>
<script>customElements.define('shop-account', class extends HTMLElement { constructor() { super(); this.internals = this.attachInternals(); } });</script>
<button id="account" aria-expanded="false" onclick="this.setAttribute('aria-expanded','true');fetch('/clicked').finally(function () { document.querySelector('shop-account').internals.shadowRoot.querySelector('a').dispatchEvent(new MouseEvent('click', {bubbles: true, composed: true})); })">Your account</button>`,
		"/closed": `<div id="host"></div>
<script>(function () { var root = document.getElementById('host').attachShadow({mode: 'closed'}); root.innerHTML = '<a href="/private/account" target="_blank">Account page</a>'; window.closedLink = root.querySelector('a'); })();</script>
<button id="account" aria-expanded="false" onclick="this.setAttribute('aria-expanded','true');fetch('/clicked').finally(function () { window.closedLink.dispatchEvent(new MouseEvent('click', {bubbles: true, composed: true})); })">Your account</button>`,
		"/shadow": `<div id="host"></div>
<script>document.getElementById('host').attachShadow({mode: 'open'}).innerHTML = '<a href="/private/account" target="_blank">Account page</a>';</script>
<button id="account" aria-expanded="false" onclick="this.setAttribute('aria-expanded','true');fetch('/clicked').finally(function () { document.getElementById('host').shadowRoot.querySelector('a').dispatchEvent(new MouseEvent('click', {bubbles: true, composed: true})); })">Your account</button>`,
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

// TestOpenedTabsAreCounted opens a tab from a page that has no one-tab script, so the tab really opens. The guard
// must count it, or the zero the closed-root test expects would prove nothing.
func TestOpenedTabsAreCounted(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!doctype html><html lang="en"><title>Shop</title><main><h1>Shop</h1></main></html>`)
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			b, err := bidi.Launch(ctx, bidi.LaunchOptions{Browser: browser, Headless: true})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(b.Close)
			opened, err := guardOpenedTabs(ctx, b.Conn, Config{}, func(string) (bool, string) { return true, "" })
			if err != nil {
				t.Fatal(err)
			}
			defer opened.close()
			cx, err := b.Conn.CreateContext(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := b.Conn.Navigate(ctx, cx, srv.URL+"/"); err != nil {
				t.Fatal(err)
			}
			if err := b.Conn.Call(ctx, "script.callFunction", map[string]any{
				"functionDeclaration": "() => { window.open('/next'); }",
				"target":              map[string]any{"context": cx},
				"awaitPromise":        true,
				"userActivation":      true,
			}, nil); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(10 * time.Second)
			for opened.Opened() == 0 && time.Now().Before(deadline) {
				time.Sleep(50 * time.Millisecond)
			}
			opened.settle(10 * time.Second)
			if n := opened.Opened(); n != 1 {
				t.Errorf("counted %d opened tabs, want 1", n)
			}
		})
	}
}

// TestSameTabKeepsTrustedTypes sweeps a page that enforces Trusted Types and sets a script's src from a trusted value.
// The one-tab script wraps setAttribute, so it must pass values for other attributes through untouched, or the page's
// own script throws.
func TestSameTabKeepsTrustedTypes(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	var mu sync.Mutex
	seen := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.URL.Path]++
		mu.Unlock()
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("Content-Security-Policy", "require-trusted-types-for 'script'")
			_, _ = fmt.Fprint(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Shop</title></head><body><main><h1>Shop</h1>
<script>
if (!window.trustedTypes) {
	fetch('/tt/none');
} else {
	var policy = trustedTypes.createPolicy('shop', { createScriptURL: function (s) { return s; } });
	var s = document.createElement('script');
	try { s.setAttribute('src', policy.createScriptURL('/tt.js')); fetch('/tt/ok'); } catch (e) { fetch('/tt/error'); }
}
</script>
</main></body></html>`)
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
				Schemes: []string{Light}, Headless: true})
			if err != nil {
				t.Fatal(err)
			}
			if r := rep.Results[0]; r.Error != "" {
				t.Fatalf("error: %s", r.Error)
			}
			mu.Lock()
			defer mu.Unlock()
			if seen["/tt/none"] > 0 && seen["/tt/ok"] == 0 && seen["/tt/error"] == 0 {
				t.Skipf("%s doesn't support Trusted Types", browser)
			}
			if seen["/tt/error"] > 0 || seen["/tt/ok"] == 0 {
				t.Errorf("the page's trusted setAttribute failed: %v", seen)
			}
		})
	}
}

// TestPressedHostKeepsClosedRootsInTab sweeps pages whose control is a custom element with a closed shadow root in
// its markup, with the keyboard checks on and --respect-robots. Inside the root is a link to a disallowed address
// or a POST form's submit button, each aimed at a new window. No script on the page ever reaches the root, so only
// the sweep's own click or key press can land on what is inside. On the click pages it covers the whole control,
// so the mouse try lands on it. On the keyboard page it sits in a corner, so the click opens the control and the
// comparison goes on to its keyboard try, where Tab reaches the link and Enter follows it. On the nested page the
// link is in a closed root inside the control's closed root. The hostile page also
// breaks addEventListener on shadow roots and WeakSet's has before the guard reaches its root, and the decoy page
// makes document.documentElement return an empty element, to hide its roots from the guard. The late page builds
// its closed root only as the mouse comes over the control, after the load. The address must
// never be loaded and the form must never arrive, in Chrome as in Firefox.
func TestPressedHostKeepsClosedRootsInTab(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	const fill = `style="display:block;position:absolute;inset:0;margin:0;padding:0;border:0;width:100%;height:100%;opacity:0"`
	type fixture struct{ inside, script, pressed string }
	pages := map[string]fixture{
		"/link": {inside: `<a href="/private/account" target="_blank" ` + fill + `>Account page</a>`, pressed: "GET /clicked"},
		"/form": {inside: `<form method="post" action="/order" target="_blank"><input type="hidden" name="item" value="1"><button ` + fill + `>Buy</button></form>`,
			pressed: "GET /clicked"},
		"/hostile": {inside: `<a href="/private/account" target="_blank" ` + fill + `>Account page</a>`, pressed: "GET /clicked",
			script: `ShadowRoot.prototype.addEventListener = function () { throw new Error('no'); };
var has = WeakSet.prototype.has;
WeakSet.prototype.has = function (x) { return x instanceof ShadowRoot || has.call(this, x); };`},
		"/decoy": {inside: `<a href="/private/account" target="_blank" ` + fill + `>Account page</a>`, pressed: "GET /clicked",
			script: `var decoy = document.createElement('html');
Object.defineProperty(document, 'documentElement', {get: function () { return decoy; }});`},
		"/late": {pressed: "GET /clicked",
			script: `var late = document.createElement('span');
late.setAttribute('style', 'position:absolute;inset:0');
menu.appendChild(late);
document.addEventListener('mousemove', function built() {
	document.removeEventListener('mousemove', built);
	late.setHTMLUnsafe('<shop-panel style="position:absolute;inset:0"><template shadowrootmode="closed"><a href="/private/account" target="_blank" ` + fill + `>Account page</a></template></shop-panel>');
});`},
		"/nested": {inside: `<shop-inner style="position:absolute;inset:0"><template shadowrootmode="closed"><a href="/private/account" target="_blank" ` + fill + `>Account page</a></template></shop-inner>`,
			pressed: "GET /clicked"},
		"/keyboard": {inside: `<a href="/private/account" target="_blank" style="position:absolute;right:0;top:0;width:20px;height:20px">Account page</a>`,
			pressed: "GET /keyed"},
	}
	var mu sync.Mutex
	seen := map[string]int{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.Method+" "+r.URL.Path]++
		mu.Unlock()
		f, ok := pages[r.URL.Path]
		switch {
		case r.URL.Path == "/robots.txt":
			_, _ = fmt.Fprint(w, "User-agent: *\nDisallow: /private\n")
		case ok:
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Shop</title></head><body><main><h1>Shop</h1>
<shop-menu role="button" aria-expanded="false" style="display:inline-block;position:relative;width:200px;height:40px">Your account<template shadowrootmode="closed"><slot></slot>`+f.inside+`</template></shop-menu>
<script>
var menu = document.querySelector('shop-menu');
menu.addEventListener('click', function () { fetch('/clicked', {keepalive: true}); menu.setAttribute('aria-expanded', 'true'); });
menu.addEventListener('keydown', function (e) { if (e.key === 'Enter') { fetch('/keyed', {keepalive: true}); } });
`+f.script+`</script>
</main></body></html>`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})
	for _, browser := range strings.Split(v, ",") {
		for path, f := range pages {
			t.Run(browser+path, func(t *testing.T) {
				mu.Lock()
				clear(seen)
				mu.Unlock()
				// Each test has its own server, so closing it waits out any request still in flight.
				srv := httptest.NewServer(handler)
				defer srv.Close()
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				defer cancel()
				rep, err := Run(ctx, Config{URLs: []string{srv.URL + path}, Browser: browser, Viewports: []Viewport{Desktop},
					Schemes: []string{Light}, Headless: true, Keyboard: true, RespectRobots: true})
				// Run closes the browser before it returns, so no request starts after it, and Close returns once
				// every request that did start has been answered.
				srv.Close()
				if err != nil {
					t.Fatal(err)
				}
				r := rep.Results[0]
				if r.Error != "" {
					t.Fatalf("error: %s", r.Error)
				}
				mu.Lock()
				defer mu.Unlock()
				if seen[f.pressed] == 0 {
					t.Fatalf("no %s, so the press the test is about never happened: %v", f.pressed, seen)
				}
				for k, n := range seen {
					if strings.HasPrefix(k, "GET /private") || (!strings.HasPrefix(k, "GET ") && !strings.HasPrefix(k, "HEAD ")) {
						t.Errorf("%s reached the server %d times: %v", k, n, seen)
					}
				}
				// The root was guarded before the press, so the press stays in the guarded tab, which stops it.
				if n := rep.opened.Opened(); n != 0 {
					t.Errorf("the press opened %d tabs, want none", n)
				}
				if r.RequestsStopped < 1 {
					t.Errorf("requests_stopped is %d, want the press: %v", r.RequestsStopped, seen)
				}
			})
		}
	}
}

// TestRootBuiltAfterTheScanStaysInTab builds a closed shadow root in the instant between the guard's last scan and
// the press, the gap the scan alone can't close. The page builds it with setHTMLUnsafe or with
// Document.parseHTMLUnsafe, the two ways a script can declare a closed root once the page has loaded. Inside it is a
// link to a disallowed address or a POST form's submit button, each aimed at a new window and filling the spot the
// mouse then presses. The test drives the scan and the press itself, so the build lands in the gap every time. The
// address must never be loaded, the form must never arrive and no tab may open, in Chrome as in Firefox.
func TestRootBuiltAfterTheScanStaysInTab(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	const fill = `style="display:block;position:absolute;inset:0;margin:0;padding:0;border:0;width:100%;height:100%;opacity:0"`
	inside := map[string]string{
		"link": `<a href="/private/account" target="_blank" ` + fill + `>Account page</a>`,
		"form": `<form method="post" action="/order" target="_blank"><input type="hidden" name="item" value="1"><button ` + fill + `>Buy</button></form>`,
	}
	build := map[string]string{
		"setHTMLUnsafe":   `(html) => { document.getElementById('spot').setHTMLUnsafe(html); }`,
		"parseHTMLUnsafe": `(html) => { const d = Document.parseHTMLUnsafe('<body>' + html); document.getElementById('spot').append(...d.body.childNodes); }`,
	}
	var mu sync.Mutex
	seen := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.Method+" "+r.URL.Path]++
		mu.Unlock()
		switch r.URL.Path {
		case "/robots.txt":
			_, _ = fmt.Fprint(w, "User-agent: *\nDisallow: /private\n")
		case "/", "/tt":
			w.Header().Set("Content-Type", "text/html")
			if r.URL.Path == "/tt" {
				w.Header().Set("Content-Security-Policy", "require-trusted-types-for 'script'")
			}
			_, _ = fmt.Fprint(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Shop</title></head><body style="margin:0"><main><h1>Shop</h1>
<div id="spot" style="position:absolute;left:0;top:100px;width:200px;height:40px"></div>
</main></body></html>`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	allowed := func(raw string) (bool, string) { return !strings.Contains(raw, "/private"), "" }
	for _, browser := range strings.Split(v, ",") {
		for how, fn := range build {
			for what, html := range inside {
				t.Run(browser+"/"+how+"/"+what, func(t *testing.T) {
					mu.Lock()
					clear(seen)
					mu.Unlock()
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
					defer cancel()
					b, err := bidi.Launch(ctx, bidi.LaunchOptions{Browser: browser, Headless: true})
					if err != nil {
						t.Fatal(err)
					}
					defer b.Close()
					if err := keepToOneTab(ctx, b.Conn); err != nil {
						t.Fatal(err)
					}
					opened, err := guardOpenedTabs(ctx, b.Conn, Config{}, allowed)
					if err != nil {
						t.Fatal(err)
					}
					defer opened.close()
					cx, err := b.Conn.CreateContext(ctx)
					if err != nil {
						t.Fatal(err)
					}
					d, stop, err := guardTab(ctx, b.Conn, cx, Config{}, allowed)
					if err != nil {
						t.Fatal(err)
					}
					defer stop()
					if _, err := b.Conn.Navigate(ctx, cx, srv.URL+"/"); err != nil {
						t.Fatal(err)
					}
					if err := b.Conn.MoveMouse(ctx, cx, 100, 120); err != nil {
						t.Fatal(err)
					}
					if err := guardClosedRoots(ctx, b.Conn, cx); err != nil {
						t.Fatal(err)
					}
					root := `<shop-panel style="display:block;position:absolute;inset:0"><template shadowrootmode="closed">` + html + `</template></shop-panel>`
					if err := b.Conn.Call(ctx, "script.callFunction", map[string]any{
						"functionDeclaration": fn,
						"arguments":           []any{map[string]any{"type": "string", "value": root}},
						"target":              map[string]any{"context": cx},
						"awaitPromise":        true,
					}, nil); err != nil {
						t.Fatal(err)
					}
					var closed bool
					if err := b.Conn.EvaluateJSON(ctx, cx, "document.querySelector('shop-panel').shadowRoot === null", &closed); err != nil {
						t.Fatal(err)
					}
					if !closed {
						t.Fatal("the root isn't closed any more")
					}
					if err := b.Conn.PressMouse(ctx, cx); err != nil {
						t.Fatal(err)
					}
					// Give the press time to reach a new tab or the server, as it would before the next step.
					time.Sleep(2 * time.Second)
					opened.settle(10 * time.Second)
					srv.CloseClientConnections()
					mu.Lock()
					defer mu.Unlock()
					for k, n := range seen {
						if strings.HasPrefix(k, "GET /private") || (!strings.HasPrefix(k, "GET ") && !strings.HasPrefix(k, "HEAD ")) {
							t.Errorf("%s reached the server %d times: %v", k, n, seen)
						}
					}
					if n := opened.Opened(); n != 0 {
						t.Errorf("the press opened %d tabs, want none", n)
					}
					if n := d.Stopped() + opened.Stopped(); n < 1 {
						t.Errorf("stopped %d requests, want the press: %v", n, seen)
					}
				})
			}
		}
		// An open root shows what the guard did to the markup: a new window becomes the page's own tab, a frame
		// the page has keeps its name, and markup with nothing to change is left as the page wrote it. An object
		// whose toString changes between reads gets built from the markup the guard read. A shadow root's own
		// setHTMLUnsafe and a root nested in another are covered, and so are area and input targets. A Trusted Types
		// value goes through as the page wrote it, a gap ENGINE.md names. A call with no markup throws as it would
		// without the guard and leaves the element alone. An SVG element named template doesn't stop
		// the guard reaching the root after it. A whole document keeps its doctype, language, title and body class.
		// A page that replaces what the guard calls still has its new window put right.
		t.Run(browser+"/markup", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			b, err := bidi.Launch(ctx, bidi.LaunchOptions{Browser: browser, Headless: true})
			if err != nil {
				t.Fatal(err)
			}
			defer b.Close()
			if err := keepToOneTab(ctx, b.Conn); err != nil {
				t.Fatal(err)
			}
			cx, err := b.Conn.CreateContext(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := b.Conn.Navigate(ctx, cx, srv.URL+"/"); err != nil {
				t.Fatal(err)
			}
			var got []string
			if err := b.Conn.EvaluateJSON(ctx, cx, `(() => {
	const spot = document.getElementById('spot');
	spot.insertAdjacentHTML('beforeend', '<iframe name="side"></iframe>');
	const host = document.createElement('div');
	spot.append(host);
	host.setHTMLUnsafe('<shop-panel><template shadowrootmode="open"><a id="a" target="_blank" href="/a">A</a><a id="b" target="side" href="/b">B</a><form id="f" target="_blank"><button id="s" formtarget="_blank">Go</button></form></template></shop-panel>');
	const r = host.firstElementChild.shadowRoot;
	const plain = document.createElement('div');
	plain.setHTMLUnsafe('<p class="x">Hello</p>');
	let reads = 0;
	const turncoat = { toString() { return reads++ ? '<shop-panel><template shadowrootmode="open"><a target="_blank" href="/c">C</a></template></shop-panel>' : '<p>Plain</p>'; } };
	const changing = document.createElement('div');
	changing.setHTMLUnsafe(turncoat);
	const out = [String(!!r), r.getElementById('a').target, r.getElementById('b').target, r.getElementById('f').target, r.getElementById('s').formTarget, plain.innerHTML, changing.innerHTML];
	r.setHTMLUnsafe('<shop-outer><template shadowrootmode="open"><shop-inner><template shadowrootmode="open"><map name="m"><area id="ar" target="_blank" href="/ar"></map><form><input id="in" type="submit" formtarget="_blank"></form></template></shop-inner></template></shop-outer>');
	const deep = r.querySelector('shop-outer').shadowRoot.querySelector('shop-inner').shadowRoot;
	out.push(deep.getElementById('ar').target, deep.getElementById('in').formTarget);
	if (window.trustedTypes) {
		const policy = trustedTypes.createPolicy('shop-test', { createHTML: (s) => s });
		const kept = '<shop-panel><template shadowrootmode="open"><a id="k" target="_blank" href="/k">K</a></template></shop-panel>';
		const viaSet = document.createElement('div');
		viaSet.setHTMLUnsafe(policy.createHTML(kept));
		const viaParse = Document.parseHTMLUnsafe(policy.createHTML('<body>' + kept));
		out.push(viaSet.firstElementChild.shadowRoot.getElementById('k').target, viaParse.querySelector('shop-panel').shadowRoot.getElementById('k').target);
	} else {
		out.push('_blank', '_blank');
	}
	const empty = document.createElement('div');
	empty.textContent = 'kept';
	let none = 'nothing';
	try { empty.setHTMLUnsafe(); } catch (e) { none = e.name; }
	out.push(none, empty.textContent);
	const svg = document.createElement('div');
	svg.setHTMLUnsafe('<svg><template></template></svg><shop-panel><template shadowrootmode="open"><a id="v" target="_blank" href="/v">V</a></template></shop-panel>');
	out.push(svg.querySelector('shop-panel').shadowRoot.getElementById('v').target);
	const whole = Document.parseHTMLUnsafe('<!DOCTYPE html><html lang="cy"><head><title>Siop</title></head><body class="b"><shop-panel><template shadowrootmode="open"><a id="d" target="_blank" href="/d">D</a></template></shop-panel></body></html>');
	out.push(whole.documentElement.lang, whole.title, whole.body.className, whole.compatMode, whole.querySelector('shop-panel').shadowRoot.getElementById('d').target);
	RegExp.prototype.test = function () { return false; };
	String.prototype.toLowerCase = function () { return '_self'; };
	Element.prototype.getAttribute = function () { return null; };
	Object.defineProperty(NodeList.prototype, 'length', { get() { return 0; } });
	RegExp.prototype.exec = function () { return null; };
	Array.prototype.forEach = function () {};
	Array.prototype[Symbol.iterator] = function* () {};
	const tampered = document.createElement('div');
	spot.append(tampered);
	tampered.setHTMLUnsafe('<shop-panel><template shadowrootmode="open"><a id="t" target="_blank" href="/t">T</a></template></shop-panel>');
	out.push(tampered.firstElementChild.shadowRoot.getElementById('t').target);
	return out;
})()`, &got); err != nil {
				t.Fatal(err)
			}
			want := []string{"true", "_self", "side", "_self", "_self", `<p class="x">Hello</p>`, "<p>Plain</p>", "_self", "_self", "_blank", "_blank",
				"TypeError", "kept", "_self", "cy", "Siop", "b", "CSS1Compat", "_self", "_self"}
			if !slices.Equal(got, want) {
				t.Errorf("the markup came out as %q, want %q", got, want)
			}
		})
		// On a page that enforces Trusted Types, markup the guard can't read as a plain string fails, as it would
		// without the guard, and builds no root.
		t.Run(browser+"/trusted-types", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			b, err := bidi.Launch(ctx, bidi.LaunchOptions{Browser: browser, Headless: true})
			if err != nil {
				t.Fatal(err)
			}
			defer b.Close()
			if err := keepToOneTab(ctx, b.Conn); err != nil {
				t.Fatal(err)
			}
			cx, err := b.Conn.CreateContext(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := b.Conn.Navigate(ctx, cx, srv.URL+"/tt"); err != nil {
				t.Fatal(err)
			}
			var got []string
			if err := b.Conn.EvaluateJSON(ctx, cx, `(() => {
	if (!window.trustedTypes) return ['none'];
	const spot = document.getElementById('spot');
	let thrown = 'nothing';
	try { spot.setHTMLUnsafe('<shop-panel><template shadowrootmode="closed"><a target="_blank" href="/x">X</a></template></shop-panel>'); } catch (e) { thrown = e.name; }
	return [thrown, String(spot.childElementCount)];
})()`, &got); err != nil {
				t.Fatal(err)
			}
			if slices.Equal(got, []string{"none"}) {
				t.Skipf("%s doesn't support Trusted Types", browser)
			}
			if want := []string{"TypeError", "0"}; !slices.Equal(got, want) {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}
