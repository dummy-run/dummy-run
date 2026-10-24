package page_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/page"
)

// TestMainHTMLDropsHiddenTextAndSaysWhenItIsCut checks, on each browser, that text the page's CSS hides is left out,
// by display, visibility or opacity,
// and that content longer than MainHTMLMax is marked as cut.
func TestMainHTMLDropsHiddenTextAndSaysWhenItIsCut(t *testing.T) {
	short := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>T</title><style>.gone { display: none }</style></head>
<body><main><p>Shown words.</p><p style="display:none">Leverage synergies.</p><div class="gone"><p>Utilise this.</p></div><p style="visibility:hidden">Facilitate outcomes.</p><div style="opacity:0"><p>Going forward.</p></div></main></body></html>`
	long := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>T</title></head><body><main>` +
		strings.Repeat("<p>A plain sentence about opening hours.</p>", page.MainHTMLMax/40+100) + `</main></body></html>`
	for _, browser := range browsers(t) {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			p := openPage(t, browser, 1200, 800, 0)
			for _, c := range []struct {
				body      string
				truncated bool
			}{{short, false}, {long, true}} {
				srv := outlineServer(t, c.body)
				if o := p.Visit(ctx, srv.URL+"/"); o.Error != "" {
					t.Fatal(o.Error)
				}
				m, err := page.MainHTML(ctx, p.Conn(), p.Context())
				if err != nil {
					t.Fatal(err)
				}
				if m.Truncated != c.truncated || m.Selector != "main" {
					t.Errorf("truncated %v selector %q, want %v main", m.Truncated, m.Selector, c.truncated)
				}
				if !c.truncated && (!strings.Contains(m.HTML, "Shown words.") || strings.Contains(m.HTML, "Leverage") || strings.Contains(m.HTML, "Utilise") ||
					strings.Contains(m.HTML, "Facilitate") || strings.Contains(m.HTML, "Going forward")) {
					t.Errorf("hidden text kept or shown text lost: %s", m.HTML)
				}
			}
		})
	}
}

// TestMainHTMLFindsTheMainContent checks the three places main content can come from: a role=main element, and
// the body without its header, navigation, footer and asides when there is no main landmark.
func TestMainHTMLFindsTheMainContent(t *testing.T) {
	head := `<!doctype html><html lang="cy"><head><meta charset="utf-8"><title>T</title></head><body>`
	cases := []struct{ body, selector, kept string }{
		{head + `<nav>Menu words.</nav><div role="main"><p>Role words.</p></div></body></html>`, "[role=main]", "Role words."},
		{head + `<header>Top words.</header><nav>Menu words.</nav><p>Body words.</p><aside>Side words.</aside><footer>Foot words.</footer></body></html>`,
			"body", "Body words."},
	}
	for _, browser := range browsers(t) {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			p := openPage(t, browser, 1200, 800, 0)
			for _, c := range cases {
				srv := outlineServer(t, c.body)
				if o := p.Visit(ctx, srv.URL+"/"); o.Error != "" {
					t.Fatal(o.Error)
				}
				m, err := page.MainHTML(ctx, p.Conn(), p.Context())
				if err != nil {
					t.Fatal(err)
				}
				if m.Selector != c.selector || !strings.Contains(m.HTML, c.kept) || !strings.Contains(m.HTML, `lang="cy"`) {
					t.Errorf("selector %q, want %q, with %q and the page's language: %s", m.Selector, c.selector, c.kept, m.HTML)
				}
				for _, gone := range []string{"Menu words.", "Top words.", "Side words.", "Foot words."} {
					if strings.Contains(m.HTML, gone) {
						t.Errorf("%s: %q kept: %s", c.selector, gone, m.HTML)
					}
				}
			}
		})
	}
}
