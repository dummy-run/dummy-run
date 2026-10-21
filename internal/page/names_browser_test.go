package page_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/page"
)

const namesPage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Names</title></head>
<body><main><h1>Names</h1>
<details><summary>Docs menu</summary><a href="/glossary">Glossary</a> <a href="/cascade">Data Cascade</a></details>
<a href="/home"><img src="/logo.png" alt="Home"></a>
<a href="/home2"><svg width="20" height="20"><title>Start page</title><rect width="20" height="20"/></svg></a>
<form action="/search" method="get"><label for="q">Search the site</label><input id="q" name="q">
<input type="image" src="/go.png" alt="Search" width="20" height="20"></form>
<button aria-hidden="true">Ghost</button>
<button id="empty"></button>
<label for="c">Country</label><select id="c"><option>United Kingdom</option><option>France</option></select>
<label for="topic">What are you contacting us about?</label><button id="topic" role="combobox" aria-expanded="false">Choose one</button>
<a href="/long">A very long link name that keeps going well past the sixty character limit of the outline</a>
<h3>Third level</h3><h6>Sixth level</h6>
<fieldset disabled><legend>Locked</legend><button>Locked button</button></fieldset>
<div hidden style="display:block"><button>Shown anyway</button></div>
<a href="/feed.xml">Feed</a>
</main></body></html>`

// specPage plants a fake axe that would name every control "planted". The engine must not use it, so the names
// match the plain page and the page's own window.axe is left as it was.
const specPage = `<script>window.axe = { version: "4.13.0", setup() {}, teardown() {}, commons: { text: { accessibleText: () => "planted" }, dom: { isVisibleToScreenReaders: () => true } }, run: () => Promise.resolve({ violations: [] }) };</script>`

const feed = `<?xml version="1.0"?><rss version="2.0"><channel><title>Feed</title><item><title>Post</title></item></channel></rss>`

func TestControlNamesAreAccessibleNames(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed.xml":
			// Served as XML, which browsers show as a document. As application/rss+xml Firefox downloads it instead.
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, feed)
		case "/logo.png", "/go.png":
			http.NotFound(w, r)
		case "/spec":
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, strings.Replace(namesPage, "<head>", "<head>"+specPage, 1))
		case "/busy":
			w.Header().Set("Content-Type", "text/html")
			var nav strings.Builder
			for i := range 150 {
				fmt.Fprintf(&nav, `<a href="/n%d">Menu entry number %d</a> `, i, i)
			}
			_, _ = io.WriteString(w, `<!doctype html><html lang="en"><title>Busy</title><header><nav>`+nav.String()+
				`</nav></header><main><h1>Busy</h1><button>Main action</button></main></html>`)
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, namesPage)
		}
	}))
	t.Cleanup(srv.Close)
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			var mu sync.Mutex
			kinds := map[string]bool{}
			hook := func(r page.Request) page.Decision {
				mu.Lock()
				defer mu.Unlock()
				u, err := url.Parse(r.URL)
				if err != nil {
					return page.Decision{}
				}
				path := u.Path
				if _, seen := kinds[path]; !seen {
					kinds[path] = r.Navigation
				}
				return page.Decision{}
			}
			ctx := context.Background()
			octx, cancel := context.WithTimeout(ctx, 90*time.Second)
			p, err := page.Open(octx, page.Options{Browser: browser, Headless: true, Width: 1200, Height: 800,
				Hosts: []string{"127.0.0.1"}, RequestHook: hook})
			cancel()
			if err != nil {
				t.Fatalf("open %s: %v", browser, err)
			}
			t.Cleanup(p.Close)
			o := p.Visit(ctx, srv.URL+"/")
			if o.Error != "" || o.Snapshot == nil {
				t.Fatalf("visit: %s", o.Error)
			}
			labels := map[string]bool{}
			for _, c := range o.Snapshot.Controls {
				labels[c.Label] = true
			}
			for _, want := range []string{"Docs menu", "Home", "Start page", "Search the site", "Search", "(no name)", "Country", "Feed", "What are you contacting us about?"} {
				if !labels[want] {
					t.Errorf("no control named %q in %v", want, labels)
				}
			}
			for l := range labels {
				if strings.Contains(l, "Glossary") || strings.Contains(l, "Data Cascade") || strings.Contains(l, "United Kingdom") || strings.Contains(l, "Ghost") {
					t.Errorf("control %q should not be listed or named so", l)
				}
			}
			if strings.Contains(o.Snapshot.Tree, "Glossary") || strings.Contains(o.Snapshot.Tree, "Ghost") {
				t.Errorf("the outline lists a control a screen reader cannot reach:\n%s", o.Snapshot.Tree)
			}
			// One name per control: the outline names every numbered control as the control list does.
			for _, c := range o.Snapshot.Controls {
				name := c.Label
				if strings.HasPrefix(name, "(no name)") {
					name = "(no name)"
				} else {
					name = `"` + strings.TrimSuffix(name, "\u2026")
				}
				if !strings.Contains(o.Snapshot.Tree, c.Role) || !strings.Contains(o.Snapshot.Tree, "@"+c.Ref) {
					t.Errorf("control @%s (%s %q) is not in the outline:\n%s", c.Ref, c.Role, c.Label, o.Snapshot.Tree)
				}
				for _, line := range strings.Split(o.Snapshot.Tree, "\n") {
					if strings.Contains(line, " @"+c.Ref+" ") || strings.HasSuffix(line, " @"+c.Ref) {
						if !strings.Contains(line, strings.TrimSuffix(name, "\u2026")) {
							t.Errorf("control @%s is %q in the list and %q in the outline", c.Ref, c.Label, line)
						}
					}
				}
			}
			for _, want := range []string{"heading 3 \"Third level\"", "heading 6 \"Sixth level\"", "combobox \"What are you contacting us about?\"",
				"button \"Locked button\"", "[disabled", "button \"Shown anyway\""} {
				if !strings.Contains(o.Snapshot.Tree, want) {
					t.Errorf("outline lacks %q:\n%s", want, o.Snapshot.Tree)
				}
			}
			const long = "A very long link name that keeps going well past the sixty character limit of the outline"
			for _, line := range strings.Split(o.Snapshot.Tree, "\n") {
				start := strings.Index(line, `"A very long`)
				end := strings.Index(line, "\u2026")
				if start < 0 || end < 0 {
					continue
				}
				kept := line[start+1 : end]
				if !strings.HasPrefix(long, kept) || (len(kept) < len(long) && long[len(kept)] != ' ') {
					t.Errorf("a name was cut inside a word: %q", kept)
				}
			}

			// The same names when axe is not trusted, so the spec computation and axe agree.
			sp := p.Visit(ctx, srv.URL+"/spec")
			if sp.Snapshot == nil {
				t.Fatalf("spec visit: %s", sp.Error)
			}
			specLabels := map[string]bool{}
			for _, c := range sp.Snapshot.Controls {
				specLabels[c.Label] = true
			}
			for l := range labels {
				if !specLabels[l] {
					t.Errorf("%q with axe has no match without it: %v", l, specLabels)
				}
			}
			if specLabels["planted"] {
				t.Errorf("the page's own window.axe named the controls: %v", specLabels)
			}
			if a := p.A11y(ctx); a.Error != "" || len(a.A11y) == 0 {
				t.Errorf("the accessibility check used the page's fake axe: %+v %q", a.A11y, a.Error)
			}

			// On a busy page the main content's controls stay and the menu goes first.
			busy := p.Visit(ctx, srv.URL+"/busy")
			if busy.Snapshot == nil {
				t.Fatalf("busy visit: %s", busy.Error)
			}
			if !strings.Contains(busy.Snapshot.Tree, `button "Main action"`) || !strings.Contains(busy.Snapshot.Tree, "more not shown") {
				t.Errorf("busy outline:\n%s", busy.Snapshot.Tree)
			}
			mu.Lock()
			logoNav, logoSeen := kinds["/logo.png"]
			if !kinds["/"] || !logoSeen || logoNav {
				t.Errorf("navigation flags: %v", kinds)
			}
			mu.Unlock()
			if o.Snapshot.ContentType != "text/html" {
				t.Errorf("content type %q", o.Snapshot.ContentType)
			}

			f := p.Visit(ctx, srv.URL+"/feed.xml")
			if f.Snapshot == nil || !f.Snapshot.NotAWebPage() || len(f.Snapshot.Controls) != 0 || f.Snapshot.Text != "" {
				t.Fatalf("feed snapshot: %+v (error %s)", f.Snapshot, f.Error)
			}
			a := p.A11y(ctx)
			if a.A11y != nil || !strings.Contains(a.Error, "not a web page") {
				t.Errorf("axe ran on a feed: %+v %q", a.A11y, a.Error)
			}
		})
	}
}
