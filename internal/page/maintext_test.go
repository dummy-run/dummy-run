package page_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/page"
)

// TestMainHTMLDropsHiddenTextAndSaysWhenItIsCut checks, on each browser, that text the page's CSS hides is left out
// and that content longer than MainHTMLMax is marked as cut.
func TestMainHTMLDropsHiddenTextAndSaysWhenItIsCut(t *testing.T) {
	short := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>T</title><style>.gone { display: none }</style></head>
<body><main><p>Shown words.</p><p style="display:none">Leverage synergies.</p><div class="gone"><p>Utilise this.</p></div></main></body></html>`
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
				if !c.truncated && (!strings.Contains(m.HTML, "Shown words.") || strings.Contains(m.HTML, "Leverage") || strings.Contains(m.HTML, "Utilise")) {
					t.Errorf("hidden text kept or shown text lost: %s", m.HTML)
				}
			}
		})
	}
}
