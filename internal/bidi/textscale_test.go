package bidi_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

func TestTextScalePrefs(t *testing.T) {
	chrome, firefox, err := bidi.TextScalePrefs("chrome", 200)
	if err != nil || firefox != nil {
		t.Fatalf("chrome: %v %v %v", chrome, firefox, err)
	}
	web := chrome["webkit"].(map[string]any)["webprefs"].(map[string]any)
	if web["default_font_size"] != 32 || web["default_fixed_font_size"] != 26 {
		t.Errorf("chrome sizes %v, want 32 and 26", web)
	}
	if _, set := web["minimum_font_size"]; set {
		t.Error("a minimum font size would enlarge text that the page sizes in px, which a larger default does not")
	}
	chrome, firefox, err = bidi.TextScalePrefs("firefox", 150)
	if err != nil || chrome != nil {
		t.Fatalf("firefox: %v %v %v", chrome, firefox, err)
	}
	if firefox["font.size.variable.x-western"] != 24 || firefox["font.size.monospace.x-western"] != 20 {
		t.Errorf("firefox sizes %v, want 24 and 20", firefox)
	}
	for k, want := range map[string]int{"font.size.variable.x-cyrillic": 24, "font.size.variable.ja": 24, "font.size.variable.ar": 24,
		"font.size.variable.el": 24, "font.size.monospace.zh-CN": 20} {
		if firefox[k] != want {
			t.Errorf("%s = %v, want %d: every language group needs the size, or pages in that language stay at normal size", k, firefox[k], want)
		}
	}
	for k := range firefox {
		if k == "font.minimum-size.x-western" {
			t.Error("firefox must not set a minimum font size")
		}
	}
	for _, b := range []string{"chrome", "firefox"} {
		if c, f, err := bidi.TextScalePrefs(b, 100); err != nil || c != nil || f != nil {
			t.Errorf("%s at 100 percent must change nothing: %v %v %v", b, c, f, err)
		}
		for _, n := range []int{0, 99, 301} {
			if _, _, err := bidi.TextScalePrefs(b, n); err == nil {
				t.Errorf("%s at %d percent must be refused", b, n)
			}
		}
	}
	if _, _, err := bidi.TextScalePrefs("safari", 200); err == nil {
		t.Error("an unknown browser must be refused")
	}
}

func TestWithTextScaleKeepsOtherPrefs(t *testing.T) {
	in := bidi.LaunchOptions{Browser: "firefox", FirefoxPrefs: map[string]any{"layout.css.prefers-color-scheme.content-override": 0}}
	out, err := bidi.WithTextScale(in, 200)
	if err != nil {
		t.Fatal(err)
	}
	if out.FirefoxPrefs["layout.css.prefers-color-scheme.content-override"] != 0 || out.FirefoxPrefs["font.size.variable.x-western"] != 32 {
		t.Errorf("prefs %v", out.FirefoxPrefs)
	}
	if _, set := in.FirefoxPrefs["font.size.variable.x-western"]; set {
		t.Error("the caller's map was changed")
	}
}

const scalePage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Scale</title></head><body>
<p id="plain">Unstyled text</p><p id="rem" style="font-size:1.25rem">Rem text</p><p id="em" style="font-size:1.5em">Em text</p>
<p id="px" style="font-size:20px">Px text</p></body></html>`

// TestTextScaleInBrowser launches each browser with a text scale of 200 percent and reads the sizes
// the page ends up with: unstyled, rem and em text double, px text and the window do not.
func TestTextScaleInBrowser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, scalePage)
	}))
	t.Cleanup(srv.Close)
	for _, name := range browsersUnderTest(t) {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			size := func(scale int) map[string]float64 {
				opts, err := bidi.WithTextScale(bidi.LaunchOptions{Browser: name, Headless: true}, scale)
				if err != nil {
					t.Fatal(err)
				}
				b, err := bidi.Launch(ctx, opts)
				if err != nil {
					t.Fatal(err)
				}
				defer b.Close()
				cx, err := b.Conn.CreateContext(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if err := b.Conn.SetViewport(ctx, cx, 375, 740); err != nil {
					t.Fatal(err)
				}
				if _, err := b.Conn.Navigate(ctx, cx, srv.URL); err != nil {
					t.Fatal(err)
				}
				var got map[string]float64
				err = b.Conn.EvaluateJSON(ctx, cx, `Object.fromEntries(["plain","rem","em","px"].map((id) => [id, parseFloat(getComputedStyle(document.getElementById(id)).fontSize)]).concat([["root", parseFloat(getComputedStyle(document.documentElement).fontSize)], ["width", innerWidth], ["dpr", devicePixelRatio]]))`, &got)
				if err != nil {
					t.Fatal(err)
				}
				return got
			}
			normal, large := size(100), size(200)
			if normal["plain"] != 16 || normal["rem"] != 20 || normal["em"] != 24 || normal["px"] != 20 {
				t.Fatalf("at 100 percent: %v", normal)
			}
			if large["plain"] != 32 || large["rem"] != 40 || large["em"] != 48 {
				t.Errorf("at 200 percent unstyled, rem and em text should double: %v", large)
			}
			if large["px"] != 20 {
				t.Errorf("px text must keep its size: %v", large)
			}
			if large["width"] != 375 || large["dpr"] != normal["dpr"] {
				t.Errorf("the window and pixel ratio must not change: %v against %v", large, normal)
			}
		})
	}
}
