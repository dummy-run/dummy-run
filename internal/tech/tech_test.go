package tech

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

func TestJourneysFromCategories(t *testing.T) {
	got := Journeys([]Found{{Name: "Calendly", Category: Booking}, {Name: "Shopify", Category: Shop}, {Name: "Stripe", Category: Payments}, {Name: "Site search", Category: Search}, {Name: "WordPress", Category: CMS}})
	if want := []string{"search", "basket", "checkout", "booking"}; !slices.Equal(got, want) {
		t.Errorf("journeys %v, want %v", got, want)
	}
	if Journeys(nil) != nil {
		t.Error("no technologies should suggest no journeys")
	}
}

// A made-up technology in webappanalyzer's file format, not taken from its data.
const waFile = `{
 "Example Shop": {"cats": [6], "js": {"exampleShop": "\\;confidence:50"}, "scriptSrc": "cdn\\.example-shop\\.test\\;version:\\1",
  "cookies": {"_exshop": ""}, "dom": {"div.ex-cart": {"exists": ""}}, "meta": {"ex-shop": ["\\d+"]}, "headers": {"x-ex": ""}},
 "Header Only": {"cats": [22], "headers": {"server": "only"}},
 "Example Forms": {"cats": [110, 5], "html": ["<form[^>]+ex-form"]},
 "Example Widget": {"cats": [5], "dom": {"div.ex-widget": {"exists": "", "attributes": {"data-ex": "^w\\d"}}}}
}`

func TestLoadWebappanalyzer(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src", "technologies"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "technologies", "e.json"), []byte(waFile), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadWebappanalyzer(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Name != "Example Forms" || got[1].Name != "Example Shop" || got[2].Name != "Example Widget" {
		t.Fatalf("want the three with page markers, got %+v", got)
	}
	// An entry with exists and attributes keeps its attribute test, rather than matching on the selector alone.
	if w := got[2]; len(w.Selectors) != 0 || len(w.DOM) != 1 || w.DOM[0].Selector != "div.ex-widget" || w.DOM[0].Attributes["data-ex"] != `^w\d` {
		t.Errorf("widget fingerprint %+v", w)
	}
	shop := got[1]
	if shop.Category != Shop || shop.Source != "webappanalyzer" || !slices.Equal(shop.Globals, []string{"exampleShop"}) ||
		!slices.Equal(shop.Scripts, []string{`cdn\.example-shop\.test`}) || !slices.Equal(shop.Cookies, []string{"^_exshop$"}) ||
		!slices.Equal(shop.Selectors, []string{"div.ex-cart"}) || shop.Meta["ex-shop"] != `\d+` {
		t.Errorf("shop fingerprint %+v", shop)
	}
	if got[0].Category != Forms || !slices.Equal(got[0].HTML, []string{"<form[^>]+ex-form"}) {
		t.Errorf("forms fingerprint %+v", got[0])
	}
	if _, err := LoadWebappanalyzer(t.TempDir()); err == nil {
		t.Error("an empty directory loaded without an error")
	}
}

const techPage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Tech</title>
<meta name="generator" content="WordPress 6.9">
<script src="/s/files/stub.js" async></script>
<script>window.Shopify = { shop: "example.myshopify.com" }; document.cookie = "_exshop=1";</script>
</head><body><main><h1>Tech</h1>
<form role="search" action="/search"><label>Search <input name="q"></label></form>
<a href="https://calendly.com/example/30min">Book a call</a>
<div class="ex-cart"></div>
<div class="ex-widget" data-ex="w2">Widget</div>
</main></body></html>`

func TestDetectOnAPage(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	srv := httptest.NewServer(techHandler())
	defer srv.Close()
	// The page has div.ex-widget with data-ex="w2", so the first DOM check matches and the second doesn't.
	prints := append(slices.Clone(Own), Fingerprint{Name: "Example Shop", Category: Shop, Source: "webappanalyzer", Cookies: []string{"^_exshop$"}},
		Fingerprint{Name: "Broken", Category: Other, Scripts: []string{"("}},
		Fingerprint{Name: "Widget", Category: Other, Source: "webappanalyzer", DOM: []DOMCheck{{Selector: "div.ex-widget", Attributes: map[string]string{"data-ex": `^w\d`}}}},
		Fingerprint{Name: "Not this widget", Category: Other, Source: "webappanalyzer", DOM: []DOMCheck{{Selector: "div.ex-widget", Attributes: map[string]string{"data-ex": "^z"}}}})
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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
			if _, err := b.Conn.Navigate(ctx, cx, srv.URL); err != nil {
				t.Fatal(err)
			}
			found, err := Detect(ctx, b.Conn, cx, prints)
			if err != nil {
				t.Fatal(err)
			}
			names := map[string]Found{}
			for _, f := range found {
				names[f.Name] = f
			}
			for _, want := range []string{"Shopify", "WordPress", "Site search", "Calendly", "Example Shop", "Widget"} {
				if _, ok := names[want]; !ok {
					t.Errorf("%s not found in %+v", want, found)
				}
			}
			if f := names["Shopify"]; f.Evidence != "window.Shopify.shop" || f.Source != "dummyrun" {
				t.Errorf("Shopify %+v", f)
			}
			if _, ok := names["Broken"]; ok {
				t.Error("a bad pattern matched")
			}
			if f := names["Widget"]; f.Evidence != "element div.ex-widget with data-ex=w2" {
				t.Errorf("Widget %+v", f)
			}
			if _, ok := names["Not this widget"]; ok {
				t.Error("a DOM check matched an attribute value it doesn't allow")
			}
			if j := Journeys(found); !slices.Equal(j, []string{"search", "basket", "checkout", "booking"}) {
				t.Errorf("journeys %v", j)
			}
		})
	}
}

// With a webappanalyzer checkout in DUMMYRUN_WEBAPPANALYZER, its whole set loads and runs on a page.
func TestWebappanalyzerFromEnv(t *testing.T) {
	if os.Getenv("DUMMYRUN_WEBAPPANALYZER") == "" || os.Getenv("DUMMYRUN_BROWSERS") == "" {
		t.Skip("set DUMMYRUN_WEBAPPANALYZER and DUMMYRUN_BROWSERS to run against your copy of webappanalyzer")
	}
	prints, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if len(prints) < 1000 {
		t.Fatalf("only %d fingerprints", len(prints))
	}
	srv := httptest.NewServer(techHandler())
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	b, err := bidi.Launch(ctx, bidi.LaunchOptions{Browser: strings.Split(os.Getenv("DUMMYRUN_BROWSERS"), ",")[0], Headless: true})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	cx, _ := b.Conn.CreateContext(ctx)
	if _, err := b.Conn.Navigate(ctx, cx, srv.URL); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	found, err := Detect(ctx, b.Conn, cx, append(slices.Clone(Own), prints...))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d fingerprints, %d found in %v: %+v", len(prints), len(found), time.Since(start), found)
}

// techHandler serves techPage, and its script from the same server, so the test never goes to the network.
func techHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".js") {
			w.Header().Set("Content-Type", "text/javascript")
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, techPage)
	})
}
