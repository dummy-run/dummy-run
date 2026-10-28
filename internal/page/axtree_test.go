package page_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dummy-run/dummy-run/internal/page"
)

// axPage has controls whose names only the browser's own tree gets right: a button named by CSS generated content,
// which names.js does not read, and a button added after the first look, which a second look must mark too.
const axPage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Tree</title>
<style>.icon::before { content: "Search"; }</style></head>
<body><main><h1>Tree</h1>
<button class="icon"></button>
<label for="when">Arrival</label><input id="when" type="date">
<button aria-expanded="false">Menu</button>
<button onclick="const b = document.createElement('button'); b.className = 'icon'; b.id = 'late'; document.querySelector('main').append(b)">Add</button>
</main></body></html>`

func TestChromeNamesComeFromItsAccessibilityTree(t *testing.T) {
	srv := outlineServer(t, axPage)
	for _, browser := range browsers(t) {
		t.Run(browser, func(t *testing.T) {
			ctx := context.Background()
			p := openPage(t, browser, 1000, 700, 0)
			o := p.Visit(ctx, srv.URL)
			if o.Error != "" || o.Snapshot == nil {
				t.Fatalf("visit: %s", o.Error)
			}
			s := o.Snapshot
			want := ""
			if browser == "chrome" {
				want = page.NamesFromChrome
			}
			if s.NamesFrom != want {
				t.Fatalf("names_from %q, want %q", s.NamesFrom, want)
			}
			for _, line := range []string{`textbox "Arrival"`, `button "Menu" @3 [collapsed]`} {
				if !strings.Contains(s.Tree, line) {
					t.Errorf("outline lacks %q:\n%s", line, s.Tree)
				}
			}
			if browser != "chrome" {
				return
			}
			if !strings.Contains(s.Tree, `button "Search"`) {
				t.Errorf("Chrome's name from CSS content is missing:\n%s", s.Tree)
			}
			add := ""
			for _, c := range s.Controls {
				if c.Label == "Add" {
					add = c.Ref
				}
			}
			if add == "" {
				t.Fatalf("no Add control in %+v", s.Controls)
			}
			o = p.Click(ctx, "@"+add)
			if o.Error != "" || o.Snapshot == nil {
				t.Fatalf("click: %s", o.Error)
			}
			if n := strings.Count(o.Snapshot.Tree, `button "Search"`); n != 2 {
				t.Errorf("want both Search buttons named after the second look, got %d:\n%s", n, o.Snapshot.Tree)
			}
		})
	}
}
