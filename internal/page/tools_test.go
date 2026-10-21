package page_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
	"github.com/dummy-run/dummy-run/internal/page"
)

const toolsPage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Tools</title>
<style>body{margin:0} #tall{height:3000px}</style></head>
<body>
<main>
<h1 id="h">Welcome</h1>
<p>Some text for the persona to read.</p>
<p>Run <code>QUOTED-EXAMPLE</code> to start.</p>
<nav><a href="/about">About us</a> <a href="/pricing">Pricing</a></nav>
<label for="name">Your name</label><input id="name" value="old">
<input id="unlabelled" placeholder="">
<label for="colour">Colour</label><select id="colour"><option>Red</option><option>Green</option></select>
<button id="go" onclick="document.getElementById('h').textContent='Clicked'">Go now</button>
<form method="post" action="/subscribe"><label for="email">Email</label><input id="email" name="email"><button type="submit">Subscribe</button></form>
<nav id="drawer" style="position:fixed;left:-300px;top:0;width:250px;height:100%"><a href="/hidden">Hidden drawer link</a> <a href="/about">About us</a></nav>
<div id="tall"></div>
<p id="bottom">The end.</p>
</main>
<script>
document.getElementById('name').addEventListener('keydown', (e) => { if (e.key === 'Enter') document.getElementById('h').textContent = 'Entered'; });
</script>
</body></html>`

type recorder struct {
	mu   sync.Mutex
	reqs []*http.Request
}

func (r *recorder) has(method, path string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, q := range r.reqs {
		if q.Method == method && q.URL.Path == path {
			return true
		}
	}
	return false
}

func (r *recorder) header(path, name string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, q := range r.reqs {
		if q.URL.Path == path {
			return q.Header.Get(name)
		}
	}
	return ""
}

func server(t *testing.T) (*httptest.Server, *recorder) {
	t.Helper()
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.reqs = append(rec.reqs, r.Clone(context.Background()))
		rec.mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/about":
			_, _ = io.WriteString(w, `<!doctype html><html lang="en"><title>About</title><main><h1>About</h1></main></html>`)
		case "/subscribe":
			_, _ = io.WriteString(w, `<!doctype html><title>Subscribed</title><h1>Subscribed</h1>`)
		default:
			_, _ = io.WriteString(w, toolsPage)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

// TestTools runs the thirteen persona tools on each browser named in
// DUMMYRUN_BROWSERS, with the read-only policy on. Each subtest is one row
// of the tool conformance matrix.
func TestTools(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the tool suite")
	}
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			srv, rec := server(t)
			ctx := context.Background()
			hook := func(r page.Request) page.Decision {
				switch r.Method {
				case http.MethodGet, http.MethodHead, http.MethodOptions:
					return page.Decision{Set: []bidi.Header{bidi.StringHeader("baggage", "dummyrun.run.id=t1,dummyrun.persona.id=p1")}}
				default:
					return page.Decision{Block: true, Reason: "read-only target"}
				}
			}
			octx, cancel := context.WithTimeout(ctx, 90*time.Second)
			p, err := page.Open(octx, page.Options{Browser: browser, Headless: true, Width: 1200, Height: 800,
				UserAgent: "ToolsTest/1 DummyRun/test", Hosts: []string{"127.0.0.1"}, RequestHook: hook})
			cancel()
			if err != nil {
				t.Fatalf("open %s: %v", browser, err)
			}
			t.Cleanup(p.Close)

			check := func(t *testing.T, o page.Observation) page.Observation {
				t.Helper()
				if o.Error != "" {
					t.Fatalf("%s %s failed: %s", o.Action, o.Target, o.Error)
				}
				if o.Snapshot == nil || len(o.Screenshot) == 0 {
					t.Fatalf("%s produced no snapshot or screenshot", o.Action)
				}
				return o
			}

			t.Run("visit", func(t *testing.T) {
				o := check(t, p.Visit(ctx, srv.URL+"/"))
				if o.Snapshot.Title != "Tools" || o.Status != 200 {
					t.Errorf("title %q status %d", o.Snapshot.Title, o.Status)
				}
				if got := rec.header("/", "baggage"); got != "dummyrun.run.id=t1,dummyrun.persona.id=p1" {
					t.Errorf("baggage header %q", got)
				}
				if got := rec.header("/", "User-Agent"); !strings.HasSuffix(got, "DummyRun/test") {
					t.Errorf("user agent %q", got)
				}
			})
			t.Run("look", func(t *testing.T) {
				o := check(t, p.Look(ctx))
				if len(o.Snapshot.Controls) < 7 {
					t.Errorf("only %d controls: %+v", len(o.Snapshot.Controls), o.Snapshot.Controls)
				}
				if !strings.Contains(o.Snapshot.Text, "Some text for the persona") {
					t.Errorf("text missing: %q", o.Snapshot.Text)
				}
				// The model sees the code. The injection scan reads a copy with the code cut out. Neither holds a marker.
				if !strings.Contains(o.Snapshot.Text, "QUOTED-EXAMPLE") || strings.Contains(o.Snapshot.ScanText, "QUOTED-EXAMPLE") ||
					!strings.Contains(o.Snapshot.ScanText, "Some text for the persona") || !strings.Contains(o.Snapshot.ScanText, "to start.") {
					t.Errorf("text %q, scan text %q", o.Snapshot.Text, o.Snapshot.ScanText)
				}
				for _, got := range []string{o.Snapshot.Text, o.Snapshot.ScanText} {
					if strings.ContainsAny(got, "\uE000\uE001\uE002\uE003") {
						t.Errorf("a marker is left in %q", got)
					}
				}
				var labels []string
				for _, c := range o.Snapshot.Controls {
					labels = append(labels, c.Label)
				}
				joined := strings.Join(labels, "|")
				for _, want := range []string{"About us", "Your name", "Colour", "Go now", "Subscribe", "(no label)"} {
					if !strings.Contains(joined, want) {
						t.Errorf("control %q not in %s", want, joined)
					}
				}
			})
			t.Run("click by label", func(t *testing.T) {
				o := check(t, p.Click(ctx, "Go now"))
				if len(o.Snapshot.Headings) == 0 || o.Snapshot.Headings[0] != "Clicked" {
					t.Errorf("headings %v", o.Snapshot.Headings)
				}
			})
			t.Run("fill", func(t *testing.T) {
				check(t, p.Fill(ctx, "Your name", "Ada"))
				var v string
				if err := p.Browser().Conn.EvaluateJSON(ctx, contextOf(t, p), "document.getElementById('name').value", &v); err != nil || v != "Ada" {
					t.Errorf("value %q %v", v, err)
				}
			})
			t.Run("type", func(t *testing.T) {
				check(t, p.Type(ctx, "css:#name", "Bob"))
				var v string
				if err := p.Browser().Conn.EvaluateJSON(ctx, contextOf(t, p), "document.getElementById('name').value", &v); err != nil || v != "Bob" {
					t.Errorf("value %q %v (type must replace, not append)", v, err)
				}
			})
			t.Run("press", func(t *testing.T) {
				o := check(t, p.Press(ctx, "Enter"))
				if o.Snapshot.Headings[0] != "Entered" {
					t.Errorf("headings %v", o.Snapshot.Headings)
				}
			})
			t.Run("select", func(t *testing.T) {
				check(t, p.Select(ctx, "Colour", "Green"))
				var v string
				if err := p.Browser().Conn.EvaluateJSON(ctx, contextOf(t, p), "document.getElementById('colour').value", &v); err != nil || v != "Green" {
					t.Errorf("value %q %v", v, err)
				}
				if o := p.Select(ctx, "Colour", "Blue"); o.Error == "" || !strings.Contains(o.Error, "Red, Green") {
					t.Errorf("a missing option should list the options: %q", o.Error)
				}
			})
			t.Run("scroll", func(t *testing.T) {
				o := check(t, p.Scroll(ctx, "down"))
				if o.Snapshot.Scroll[0] < 500 {
					t.Errorf("scrollY %v", o.Snapshot.Scroll)
				}
				check(t, p.Scroll(ctx, "up"))
			})
			t.Run("wait", func(t *testing.T) {
				start := time.Now()
				check(t, p.Wait(ctx, 1))
				if time.Since(start) < time.Second {
					t.Error("wait returned early")
				}
			})
			t.Run("a11y", func(t *testing.T) {
				o := check(t, p.A11y(ctx))
				if o.A11y == nil {
					t.Fatal("a11y did not run")
				}
				var ids []string
				for _, v := range o.A11y {
					ids = append(ids, v.ID)
				}
				if !strings.Contains(strings.Join(ids, ","), "label") {
					t.Errorf("expected the unlabelled input to fail a label rule, got %v", ids)
				}
			})
			t.Run("screenshot", func(t *testing.T) {
				o := check(t, p.Screenshot(ctx))
				if string(o.Screenshot[1:4]) != "PNG" {
					t.Error("not a PNG")
				}
			})
			t.Run("read-only policy blocks the form", func(t *testing.T) {
				check(t, p.Fill(ctx, "Email", "a@example.test"))
				o := p.Click(ctx, "Subscribe")
				if len(o.Blocked) != 1 || !strings.HasPrefix(o.Blocked[0], "POST ") {
					t.Errorf("blocked %v", o.Blocked)
				}
				time.Sleep(300 * time.Millisecond)
				if rec.has(http.MethodPost, "/subscribe") {
					t.Fatal("the server received the POST")
				}
			})
			t.Run("click with @ref and back", func(t *testing.T) {
				check(t, p.Visit(ctx, srv.URL+"/"))
				o := check(t, p.Look(ctx))
				var ref string
				for _, c := range o.Snapshot.Controls {
					if c.Label == "About us" {
						ref = "@" + c.Ref
					}
				}
				if ref == "" {
					t.Fatal("no About us control")
				}
				o = check(t, p.Click(ctx, ref))
				if !strings.HasSuffix(o.URL, "/about") {
					t.Errorf("after click at %s", o.URL)
				}
				o = check(t, p.Back(ctx))
				if strings.HasSuffix(o.URL, "/about") {
					t.Errorf("after back at %s", o.URL)
				}
			})
			t.Run("off-canvas controls are hidden", func(t *testing.T) {
				o := check(t, p.Look(ctx))
				for _, c := range o.Snapshot.Controls {
					if c.Label == "Hidden drawer link" {
						t.Errorf("a control in a closed drawer was listed: %+v", c)
					}
				}
				if o = p.Click(ctx, "css:#drawer a"); !strings.Contains(o.Error, "off screen") {
					t.Errorf("clicking into the drawer gave %q", o.Error)
				}
				if o = check(t, p.Click(ctx, "About us")); !strings.HasSuffix(o.URL, "/about") {
					t.Errorf("the visible About us link was not preferred: %s %s", o.URL, o.Error)
				}
				check(t, p.Back(ctx))
			})
			t.Run("errors are reported, not fatal", func(t *testing.T) {
				o := p.Click(ctx, "@999")
				if !strings.Contains(o.Error, "no control @999") || o.Snapshot == nil {
					t.Errorf("error %q snapshot %v", o.Error, o.Snapshot != nil)
				}
				o = p.Press(ctx, "F13")
				if !strings.Contains(o.Error, "unknown key") {
					t.Errorf("error %q", o.Error)
				}
			})
			t.Run("finish", func(_ *testing.T) {
				// finish has no browser action; the runner ends the run. Listed so the
				// matrix shows all thirteen tools.
			})
		})
	}
}

func contextOf(t *testing.T, p *page.Page) string {
	t.Helper()
	tree, err := p.Browser().Conn.GetTree(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range tree {
		if c.URL != "about:blank" {
			return c.Context
		}
	}
	t.Fatal("no page context")
	return ""
}
