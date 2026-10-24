package journey

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
	"github.com/dummy-run/dummy-run/internal/tech"
)

// The home page offers six ways in, each with a known answer:
//   - search works by mouse and keyboard;
//   - the menu opens on mousedown only, so the keyboard fails;
//   - sign-up and basket are plain links that work both ways;
//   - the contact link swallows Enter, so the keyboard fails, and its form takes text by mouse;
//   - a Calendly script suggests a booking journey the page shows no way into.
//
// The page is long, so a driver that scrolls before tabbing would start Tab past the menu.
const home = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Shop</title>
<script>window.Shopify = { shop: "x" };</script><script src="https://assets.calendly.com/assets/external/widget.js" async></script></head>
<body><header><button id="menu" aria-expanded="false" aria-controls="navlist" onmousedown="this.setAttribute('aria-expanded','true');document.getElementById('navlist').hidden=false">Menu</button>
<ul id="navlist" hidden><li><a href="/a">A</a></li></ul>
<form role="search" action="/search"><label for="q">Search</label><input id="q" name="q" type="search"></form></header>
<main><h1>Shop</h1>
<a href="/signup">Sign up</a> <a href="/basket">Basket</a>
<a href="/contact" onkeydown="if (event.key === 'Enter') event.preventDefault()">Contact us</a>
<form action="/order" method="post"><button type="submit">Buy</button></form>
<div style="height:3000px"></div><a href="/a">Far down</a>
</main></body></html>`

var pages = map[string]string{
	"/":        home,
	"/signup":  `<!doctype html><html lang="en"><title>Sign up</title><main><h1>Create an account</h1><form method="post"><label>Email <input type="email" name="e"></label><label>Password <input type="password" name="p"></label><button>Create</button></form></main></html>`,
	"/basket":  `<!doctype html><html lang="en"><title>Your basket</title><main><h1>Basket</h1><p>It's empty.</p></main></html>`,
	"/contact": `<!doctype html><html lang="en"><title>Contact</title><main><h1>Contact</h1><form method="post" action="/send"><label>Name <input name="n"></label><label>Email <input type="email" name="e"></label><label>Message <textarea name="m"></textarea></label><button type="submit">Send</button></form></main></html>`,
}

func TestJourneysByMouseAndKeyboard(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	var posts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			posts++
		}
		body, ok := pages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			posts = 0
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
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
			_ = b.Conn.SetViewport(ctx, cx, 1200, 800)
			d := &Driver{Conn: b.Conn, CX: cx}
			if _, err := b.Conn.Navigate(ctx, cx, srv.URL+"/"); err != nil {
				t.Fatal(err)
			}
			found, err := tech.Detect(ctx, b.Conn, cx, tech.Own)
			if err != nil {
				t.Fatal(err)
			}
			js, err := d.Find(ctx, srv.URL+"/", found)
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]*Journey{}
			for i := range js {
				d.Try(ctx, srv.URL+"/", &js[i])
				got[js[i].Kind] = &js[i]
			}
			want := map[string][2]string{
				"search":  {Pass, Pass},
				"menu":    {Pass, Fail},
				"signup":  {Pass, Pass},
				"basket":  {Pass, Pass},
				"contact": {Pass, Fail},
				"booking": {NotFound, NotFound},
			}
			for kind, w := range want {
				j := got[kind]
				if j == nil {
					t.Errorf("%s: not found in %+v", kind, js)
					continue
				}
				if j.Mouse.Outcome != w[0] || j.Keyboard.Outcome != w[1] {
					t.Errorf("%s: mouse %+v keyboard %+v, want %s and %s", kind, *j.Mouse, *j.Keyboard, w[0], w[1])
				}
			}
			if j := got["booking"]; j != nil && j.Source != "tech:Calendly" {
				t.Errorf("booking source %q", j.Source)
			}
			if j := got["contact"]; j != nil && !strings.Contains(j.Mouse.Detail, "filled 3 of 3") {
				t.Errorf("contact by mouse: %q", j.Mouse.Detail)
			}
			if j := got["menu"]; j == nil || !j.KeyboardShortfall() || got["search"].KeyboardShortfall() {
				t.Error("the shortfall test is wrong")
			}
			if posts != 0 {
				t.Errorf("%d requests that were not GET or HEAD: a journey submitted something", posts)
			}
		})
	}
}
