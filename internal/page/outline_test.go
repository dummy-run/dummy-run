package page_test

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/page"
)

const outlinePage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Outline</title>
<style>body{margin:0}</style></head>
<body>
<header><a href="/">Home</a></header>
<nav aria-label="Main"><a href="/docs">Docs</a><a href="/blog"><img src="data:image/gif;base64,R0lGODlhAQABAAAAACwAAAAAAQABAAA=" alt="Blog"></a></nav>
<main>
<h1>Welcome</h1>
<h2 id="two">Second heading</h2>
<a id="icon" href="/icon"><svg width="10" height="10"></svg></a>
<label for="email">Email address</label><input id="email" type="email" required>
<button id="menu" aria-expanded="false">Menu</button>
<button id="labelled" aria-labelledby="lbl"></button><span id="lbl">Labelled by span</span>
<input id="agree" type="checkbox" aria-label="I agree" checked>
<button id="off" disabled>Not now</button>
<div id="dead" onclick="1">A clickable div</div>
<div style="display:none"><a href="/hidden">Hidden link</a></div>
</main>
<footer>Contact</footer>
</body></html>`

func outlineServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func browsers(t *testing.T) []string {
	t.Helper()
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	return strings.Split(v, ",")
}

func openPage(t *testing.T, browser string, width, height, zoom int) *page.Page {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	p, err := page.Open(ctx, page.Options{Browser: browser, Headless: true, Width: width, Height: height, Zoom: zoom})
	if err != nil {
		t.Fatalf("open %s: %v", browser, err)
	}
	t.Cleanup(p.Close)
	return p
}

func mustFocus(t *testing.T, o page.Observation) *page.Focus {
	t.Helper()
	if o.Error != "" {
		t.Fatalf("%s %s failed: %s", o.Action, o.Target, o.Error)
	}
	if o.Snapshot == nil || o.Snapshot.Focus == nil {
		t.Fatalf("after %s %s no element had focus", o.Action, o.Target)
	}
	return o.Snapshot.Focus
}

// TestFocusAndOutline checks, on each browser, that every observation reports
// the focused element with its role, accessible name and @ref, follows Tab, and
// carries an outline of landmarks, headings and interactive elements.
func TestFocusAndOutline(t *testing.T) {
	for _, browser := range browsers(t) {
		t.Run(browser, func(t *testing.T) {
			ctx := context.Background()
			srv := outlineServer(t, outlinePage)
			p := openPage(t, browser, 1200, 800, 0)

			o := p.Visit(ctx, srv.URL+"/")
			if o.Snapshot == nil {
				t.Fatalf("no snapshot: %s", o.Error)
			}
			if o.Snapshot.Focus != nil {
				t.Errorf("nothing was focused yet, got %+v", o.Snapshot.Focus)
			}

			t.Run("Tab walks the page in order", func(t *testing.T) {
				want := []struct{ role, name string }{
					{"link", "Home"}, {"link", "Docs"}, {"link", "Blog"}, {"link", ""}, {"textbox", "Email address"},
					{"button", "Menu"}, {"button", "Labelled by span"}, {"checkbox", "I agree"},
				}
				for i, w := range want {
					f := mustFocus(t, p.Press(ctx, "Tab"))
					if f.Role != w.role || f.Name != w.name {
						t.Errorf("Tab %d focused %s %q, want %s %q", i+1, f.Role, f.Name, w.role, w.name)
					}
					if f.Ref == "" {
						t.Errorf("Tab %d: focused %s %q has no @ref", i+1, f.Role, f.Name)
					}
				}
			})
			t.Run("the ref names the control that has focus", func(t *testing.T) {
				o := p.Click(ctx, "Menu")
				f := mustFocus(t, o)
				if f.Role != "button" || f.Name != "Menu" {
					t.Fatalf("click focused %+v", f)
				}
				for _, c := range o.Snapshot.Controls {
					if c.Ref == f.Ref && c.Label != "Menu" {
						t.Errorf("focus ref @%s is %q in the controls, want Menu", f.Ref, c.Label)
					}
				}
			})
			t.Run("the outline lists landmarks, headings and controls with roles and names", func(t *testing.T) {
				o := p.Look(ctx)
				tree := o.Snapshot.Tree
				for _, want := range []string{
					"banner\n", `navigation "Main"`, "main\n", "contentinfo",
					"heading 1 \"Welcome\"", "heading 2 \"Second heading\"",
					`link "Home" @`, `link "Blog" @`, "link (no name) @",
					`textbox "Email address" @`, "[required]", `button "Menu" @`, "[collapsed]", "<- focus",
					`button "Labelled by span" @`, `checkbox "I agree" @`, "[checked]", `button "Not now" @`, "[disabled]",
				} {
					if !strings.Contains(tree, want) {
						t.Errorf("outline lacks %q:\n%s", want, tree)
					}
				}
				if strings.Contains(tree, "Hidden link") {
					t.Errorf("outline lists a hidden link:\n%s", tree)
				}
				if strings.Contains(tree, "A clickable div") {
					t.Errorf("outline lists a div that is not focusable or a control:\n%s", tree)
				}
				if !strings.Contains(tree, "  link \"Docs\"") {
					t.Errorf("links in a landmark should be indented under it:\n%s", tree)
				}
				if o.Snapshot.TreeOmitted != 0 {
					t.Errorf("a small page lost %d outline entries", o.Snapshot.TreeOmitted)
				}
			})
			t.Run("focus is cleared when the page loses it", func(t *testing.T) {
				o := p.Visit(ctx, srv.URL+"/again")
				if o.Snapshot.Focus != nil {
					t.Errorf("a new page started with focus on %+v", o.Snapshot.Focus)
				}
			})
		})
	}
}

// TestOutlineBudget gives the outline a page with far more controls than fit
// and checks it stays within the token budget, keeps the landmarks and the
// focused element, and says what it left out.
func TestOutlineBudget(t *testing.T) {
	for _, browser := range browsers(t) {
		t.Run(browser, func(t *testing.T) {
			var b strings.Builder
			b.WriteString(`<!doctype html><title>Big</title><header><nav aria-label="Site">`)
			for i := 0; i < 400; i++ {
				fmt.Fprintf(&b, `<a href="/p%d">Link number %d in a long list of links</a>`, i, i)
			}
			b.WriteString(`</nav></header><main><h1>Big page</h1><h2>Below</h2><button id="last">Last button</button></main><footer>f</footer>`)
			srv := outlineServer(t, b.String())
			p := openPage(t, browser, 1200, 800, 0)
			ctx := context.Background()
			if o := p.Visit(ctx, srv.URL+"/"); o.Snapshot == nil {
				t.Fatalf("no snapshot: %s", o.Error)
			}
			o := p.Click(ctx, "css:#last")
			s := o.Snapshot
			if s == nil {
				t.Fatalf("no snapshot: %s", o.Error)
			}
			if n := len(s.Tree); n > page.TreeChars {
				t.Errorf("outline is %d characters, the budget is %d (%d tokens at %d per token)", n, page.TreeChars, page.TreeTokens, page.CharsPerToken)
			}
			if s.TreeOmitted < 100 {
				t.Errorf("only %d entries omitted from a page of 400 links", s.TreeOmitted)
			}
			for _, want := range []string{`navigation "Site"`, "banner", "main", "contentinfo", "heading 1 \"Big page\"", `button "Last button"`, "<- focus", "more not shown"} {
				if !strings.Contains(s.Tree, want) {
					t.Errorf("outline lost %q:\n%s", want, s.Tree)
				}
			}
			if s.Focus == nil || s.Focus.Name != "Last button" {
				t.Errorf("focus %+v", s.Focus)
			}
		})
	}
}

const zoomPage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Zoom</title><style>
body{margin:0;font-size:16px} #wide{display:block} #narrow{display:none}
@media (max-width: 600px){#wide{display:none} #narrow{display:block}}</style></head>
<body><main><h1>Zoom</h1><p id="wide">Wide layout</p><p id="narrow">Narrow layout</p></main></body></html>`

// TestZoom checks that a zoom of 200 percent gives the page the viewport of a
// zoomed browser on each browser: half the CSS pixels, a device pixel ratio of
// 2, the narrow layout, and a screenshot as large as the window.
func TestZoom(t *testing.T) {
	for _, browser := range browsers(t) {
		t.Run(browser, func(t *testing.T) {
			srv := outlineServer(t, zoomPage)
			ctx := context.Background()
			for _, tc := range []struct {
				zoom, cssW, cssH, shotW, shotH int
				ratio                          float64
				narrow                         bool
			}{
				{0, 1000, 800, 0, 0, 0, false},
				{200, 500, 400, 1000, 800, 2, true},
				{400, 250, 200, 1000, 800, 4, true},
			} {
				t.Run(fmt.Sprintf("%d percent", tc.zoom), func(t *testing.T) {
					p := openPage(t, browser, tc.cssW, tc.cssH, tc.zoom)
					o := p.Visit(ctx, srv.URL+"/")
					s := o.Snapshot
					if s == nil {
						t.Fatalf("no snapshot: %s", o.Error)
					}
					if len(s.Viewport) != 2 || s.Viewport[0] != tc.cssW || s.Viewport[1] != tc.cssH {
						t.Errorf("viewport %v, want %d by %d CSS pixels", s.Viewport, tc.cssW, tc.cssH)
					}
					if tc.zoom != 0 && s.DevicePixelRatio != tc.ratio {
						t.Errorf("devicePixelRatio %v, want %v", s.DevicePixelRatio, tc.ratio)
					}
					wantZoom := max(tc.zoom, 100)
					if s.Zoom != wantZoom {
						t.Errorf("snapshot zoom %d, want %d", s.Zoom, wantZoom)
					}
					if narrow := strings.Contains(s.Text, "Narrow layout"); narrow != tc.narrow && tc.zoom != 0 {
						t.Errorf("text %q: narrow layout shown %v, want %v", s.Text, narrow, tc.narrow)
					}
					if tc.shotW > 0 {
						cfg, err := png.DecodeConfig(bytes.NewReader(o.Screenshot))
						if err != nil {
							t.Fatal(err)
						}
						if cfg.Width != tc.shotW || cfg.Height != tc.shotH {
							t.Errorf("screenshot %d by %d pixels, want %d by %d", cfg.Width, cfg.Height, tc.shotW, tc.shotH)
						}
					}
					// Zoom has to survive a navigation, since every step may load a page.
					o = p.Visit(ctx, srv.URL+"/second")
					if o.Snapshot == nil || o.Snapshot.Viewport[0] != tc.cssW {
						t.Errorf("after navigating: %+v", o.Snapshot)
					}
				})
			}
		})
	}
}
