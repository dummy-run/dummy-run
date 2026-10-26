package page_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
	"github.com/dummy-run/dummy-run/internal/page"
)

// TestEngineFindingOnAnAttribute checks, on each browser, that a finding an engine points at an attribute, as Alfa
// does for a lang value that isn't a language, lands on the element the attribute belongs to.
func TestEngineFindingOnAnAttribute(t *testing.T) {
	srv := outlineServer(t, `<!doctype html><html lang="nonsense"><head><meta charset="utf-8"><title>Lang</title></head><body><main><h1>Lang</h1>
<p id="q" lang="zz-notalang">Bonjour tout le monde, ceci est une phrase.</p></main></body></html>`)
	for _, browser := range browsers(t) {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			b, err := bidi.Launch(ctx, bidi.LaunchOptions{Browser: browser, Headless: true})
			if err != nil {
				t.Fatal(err)
			}
			defer b.Close()
			cx, err := b.Conn.CreateContext(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := b.Conn.Navigate(ctx, cx, srv.URL+"/"); err != nil {
				t.Fatal(err)
			}
			r, err := page.RunEngines(ctx, b.Conn, cx, nil, []string{"3.1.1", "3.1.2"})
			if err != nil {
				t.Fatal(err)
			}
			alfa := map[string]bool{}
			for _, g := range r.Groups {
				if slices.Contains(g.Engines, "alfa") {
					alfa[g.Selector] = true
				}
			}
			if !alfa["html"] || !alfa["#q"] {
				t.Errorf("Alfa's lang findings: %v in %+v", alfa, r.Groups)
			}
		})
	}
}

// TestEngineSelectorsAreUnique checks that each finding's selector matches only the element it is about, even when
// the four nearest levels look the same elsewhere on the page.
func TestEngineSelectorsAreUnique(t *testing.T) {
	deep := `<div><div><div><div><img src="data:image/gif;base64,R0lGODlhAQABAAAAACw="></div></div></div></div>`
	srv := outlineServer(t, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Deep</title></head><body><main><h1>Deep</h1>
<section>`+deep+`</section><section>`+deep+`</section></main></body></html>`)
	for _, browser := range browsers(t) {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			b, err := bidi.Launch(ctx, bidi.LaunchOptions{Browser: browser, Headless: true})
			if err != nil {
				t.Fatal(err)
			}
			defer b.Close()
			cx, err := b.Conn.CreateContext(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := b.Conn.Navigate(ctx, cx, srv.URL+"/"); err != nil {
				t.Fatal(err)
			}
			r, err := page.RunEngines(ctx, b.Conn, cx, nil, []string{"1.1.1"})
			if err != nil {
				t.Fatal(err)
			}
			if len(r.Groups) != 2 {
				t.Fatalf("groups %+v, want one for each image", r.Groups)
			}
			for _, g := range r.Groups {
				var n int
				if err := b.Conn.CallFunctionJSON(ctx, cx, "(s) => document.querySelectorAll(s).length", &n, g.Selector); err != nil || n != 1 {
					t.Errorf("%q matches %d elements (%v)", g.Selector, n, err)
				}
			}
		})
	}
}
