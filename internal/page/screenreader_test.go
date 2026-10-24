package page_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/page"
)

// TestScreenReaderReadsThePage checks, on each browser, that a page opened for a screen reader persona carries what
// the virtual screen reader says, in reading order, and that other pages do not.
func TestScreenReaderReadsThePage(t *testing.T) {
	for _, browser := range browsers(t) {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			srv := outlineServer(t, outlinePage)
			p, err := page.Open(ctx, page.Options{Browser: browser, Headless: true, Width: 1200, Height: 800, ScreenReader: true})
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			o := p.Visit(ctx, srv.URL+"/")
			if o.Snapshot == nil || o.Snapshot.SpokenError != "" || o.Snapshot.Spoken == nil {
				t.Fatalf("no reading: %s %+v", o.Error, o.Snapshot)
			}
			sp := o.Snapshot.Spoken
			if !sp.Complete {
				t.Errorf("the reading did not reach the end: %d phrases", len(sp.Phrases))
			}
			order := []string{"banner", "navigation, Main", "heading, Welcome, level 1", "button, Menu, not expanded", "checkbox, I agree, checked"}
			at := -1
			for _, want := range order {
				i := slices.IndexFunc(sp.Phrases, func(s string) bool { return strings.HasPrefix(s, want) })
				if i < 0 || i < at {
					t.Errorf("%q missing or out of order in %q", want, sp.Phrases)
				}
				at = i
			}
			for _, ph := range sp.Phrases {
				if strings.Contains(ph, "Hidden link") {
					t.Errorf("a hidden link was read: %q", ph)
				}
			}

			plain := openPage(t, browser, 1200, 800, 0)
			if o := plain.Visit(ctx, srv.URL+"/"); o.Snapshot == nil || o.Snapshot.Spoken != nil {
				t.Errorf("a page opened without ScreenReader was read aloud")
			}
		})
	}
}

// TestScreenReaderIgnoresAPageReader checks that a page which puts its own reader in the global the bundle once used
// is still read by the bundled one, on a second reading too.
func TestScreenReaderIgnoresAPageReader(t *testing.T) {
	const fake = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Fake</title>
<script>globalThis.dummyrunVSR = { start() {}, next() {}, stop() {}, lastSpokenPhrase() { return "end of document"; },
  spokenPhraseLog() { return ["planted phrase"]; } };</script></head>
<body><main><h1>Real heading</h1></main></body></html>`
	for _, browser := range browsers(t) {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			srv := outlineServer(t, fake)
			p, err := page.Open(ctx, page.Options{Browser: browser, Headless: true, Width: 1200, Height: 800, ScreenReader: true})
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			for range 2 {
				o := p.Visit(ctx, srv.URL+"/")
				if o.Snapshot == nil || o.Snapshot.Spoken == nil {
					t.Fatalf("no reading: %s %+v", o.Error, o.Snapshot)
				}
				got := strings.Join(o.Snapshot.Spoken.Phrases, " | ")
				if strings.Contains(got, "planted phrase") || !strings.Contains(got, "Real heading") {
					t.Errorf("read %q", got)
				}
			}
		})
	}
}
