// Command textscale renders the built site in Chrome with a larger text size and fails when the layout breaks. It checks these faults at each page:
//
//   - the page scrolls sideways (WCAG 1.4.10 reflow),
//   - text is clipped inside a button or label, or a link or field runs past the page edge (WCAG 1.4.4 resize text),
//   - two controls overlap.
//
// Large text is simulated as a larger default font size in the Chrome profile, so the site's rem units are tested.
// Phones map their text size setting in different ways, and this is one model of it. -spacing adds the WCAG
// 1.4.12 text spacing overrides. -shots saves a full-page screenshot of every page.
//
//	go run ./internal/tools/textscale -sitemap http://127.0.0.1:1313/sitemap.xml -rebase http://127.0.0.1:1313
//
// It starts Chrome through the same BiDi client as dummyrun a11y, so it uses the pinned Chrome for Testing
// named by DUMMYRUN_CHROME and DUMMYRUN_CHROMEDRIVER.
package main

import (
	"context"
	_ "embed"
	"encoding/base64"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/dummy-run/dummy-run/internal/a11y"
	"github.com/dummy-run/dummy-run/internal/bidi"
)

//go:embed checks.js
var checksJS string

// Spacing is the WCAG 1.4.12 text spacing override, applied with !important as a user style sheet would.
const spacingCSS = `*{line-height:1.5!important;letter-spacing:.12em!important;word-spacing:.16em!important}p{margin-bottom:2em!important}`

type result struct {
	Overflow        []string `json:"overflow"`
	Clipped         []string `json:"clipped"`
	Overlaps        []string `json:"overlaps"`
	PageScrollWidth int      `json:"pageScrollWidth"`
	ViewportWidth   int      `json:"viewportWidth"`
}

func main() {
	sitemap := flag.String("sitemap", "", "sitemap URL listing the pages")
	rebase := flag.String("rebase", "", "origin to move sitemap URLs onto")
	width := flag.Int("width", 320, "viewport width in CSS pixels")
	height := flag.Int("height", 640, "viewport height in CSS pixels")
	font := flag.Int("font", 32, "the browser's default font size in pixels (16 is normal)")
	zoom := flag.Int("zoom", 0, "also set the root font size to this percent, such as 200")
	spacing := flag.Bool("spacing", false, "apply the WCAG 1.4.12 text spacing overrides")
	scheme := flag.String("scheme", "light", "light, dark or both")
	shots := flag.String("shots", "", "directory for a full-page PNG of every page (created if missing)")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := run(ctx, *sitemap, *rebase, flag.Args(), *width, *height, *font, *zoom, *spacing, *scheme, *shots)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "textscale:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, sitemap, rebase string, urls []string, width, height, font, zoom int, spacing bool, scheme, shots string) error {
	if sitemap != "" {
		pages, err := a11y.FetchSitemap(ctx, http.DefaultClient, sitemap, nil)
		if err != nil {
			return err
		}
		if rebase != "" {
			if pages, err = a11y.Rebase(pages, rebase); err != nil {
				return err
			}
		}
		urls = append(pages, urls...)
	}
	if len(urls) == 0 {
		return fmt.Errorf("no pages: give -sitemap or URLs")
	}
	schemes := []string{scheme}
	if scheme == "both" {
		schemes = []string{a11y.Light, a11y.Dark}
	}
	if shots != "" {
		if err := os.MkdirAll(shots, 0o750); err != nil {
			return err
		}
	}
	failed := 0
	for _, sc := range schemes {
		n, err := sweep(ctx, urls, sc, width, height, font, zoom, spacing, shots)
		if err != nil {
			return err
		}
		failed += n
	}
	if failed > 0 {
		return fmt.Errorf("%d fault(s) at %d px wide with a %d px default font", failed, width, font)
	}
	fmt.Printf("ok: %d pages at %d px wide with a %d px default font\n", len(urls)*len(schemes), width, font)
	return nil
}

func sweep(ctx context.Context, urls []string, scheme string, width, height, font, zoom int, spacing bool, shots string) (int, error) {
	args, _, err := a11y.ColourSchemeLaunch("chrome", scheme)
	if err != nil {
		return 0, err
	}
	prefs := map[string]any{"webkit": map[string]any{"webprefs": map[string]any{"default_font_size": font, "default_fixed_font_size": font * 13 / 16}}}
	b, err := bidi.Launch(ctx, bidi.LaunchOptions{Browser: "chrome", Headless: true, Args: append(args, "--hide-scrollbars"), ChromePrefs: prefs})
	if err != nil {
		return 0, err
	}
	defer b.Close()
	cx, err := b.Conn.CreateContext(ctx)
	if err != nil {
		return 0, err
	}
	if err := b.Conn.SetViewport(ctx, cx, width, height); err != nil {
		return 0, err
	}
	failed := 0
	for _, u := range urls {
		pctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		res, err := check(pctx, b.Conn, cx, u, font, zoom, spacing, shots, scheme)
		cancel()
		if err != nil {
			return failed, fmt.Errorf("%s: %w", u, err)
		}
		n := len(res.Overflow) + len(res.Clipped) + len(res.Overlaps)
		if res.PageScrollWidth > res.ViewportWidth {
			n++
			fmt.Printf("FAIL %s (%s): the page scrolls sideways: %d px of content in a %d px window\n", u, scheme, res.PageScrollWidth, res.ViewportWidth)
		}
		for _, s := range res.Overflow {
			fmt.Printf("FAIL %s (%s): past the edge: %s\n", u, scheme, s)
		}
		for _, s := range res.Clipped {
			fmt.Printf("FAIL %s (%s): clipped text: %s\n", u, scheme, s)
		}
		for _, s := range res.Overlaps {
			fmt.Printf("FAIL %s (%s): overlap: %s\n", u, scheme, s)
		}
		if n == 0 {
			fmt.Printf("ok   %s (%s)\n", u, scheme)
		}
		failed += n
	}
	return failed, nil
}

func check(ctx context.Context, conn *bidi.Conn, cx, u string, font, zoom int, spacing bool, shots, scheme string) (result, error) {
	var res result
	if _, err := conn.Navigate(ctx, cx, u); err != nil {
		return res, err
	}
	css := ""
	if zoom > 0 {
		css += fmt.Sprintf("html{font-size:%d%%!important}", zoom)
	}
	if spacing {
		css += spacingCSS
	}
	if css != "" {
		js := fmt.Sprintf("(()=>{const s=document.createElement('style');s.textContent=%q;document.head.append(s);})()", css)
		if _, err := conn.Evaluate(ctx, cx, js); err != nil {
			return res, err
		}
	}
	_, _ = conn.Evaluate(ctx, cx, "document.fonts ? document.fonts.ready.then(() => true) : true")
	select {
	case <-time.After(500 * time.Millisecond):
	case <-ctx.Done():
	}
	// A larger default font is only a test of the site if the root really grew.
	var root float64
	if err := conn.EvaluateJSON(ctx, cx, "parseFloat(getComputedStyle(document.documentElement).fontSize)", &root); err != nil {
		return res, err
	}
	want := wantRoot(font, zoom)
	if root < want-0.5 {
		return res, fmt.Errorf("the root font size is %.1f px, not %.1f px: the browser did not apply the setting, or the page fixes its font size in px", root, want)
	}
	if err := conn.EvaluateJSON(ctx, cx, checksJS, &res); err != nil {
		return res, err
	}
	// The menu is shut on a narrow screen, so open it and look again: its links are controls too.
	var opened bool
	if err := conn.EvaluateJSON(ctx, cx, `(()=>{const b=document.querySelector(".nav-toggle");if(!b||b.hidden||b.getBoundingClientRect().width<1)return false;b.click();return b.getAttribute("aria-expanded")==="true"})()`, &opened); err != nil {
		return res, err
	}
	if opened {
		var open result
		if err := conn.EvaluateJSON(ctx, cx, checksJS, &open); err != nil {
			return res, err
		}
		mergeOpen(&res, open)
		// Shut it again, so a screenshot shows the page as a visitor first sees it.
		if _, err := conn.Evaluate(ctx, cx, `document.querySelector(".nav-toggle").click()`); err != nil {
			return res, err
		}
	}
	if shots != "" {
		var out struct {
			Data string `json:"data"`
		}
		if err := conn.Call(ctx, "browsingContext.captureScreenshot", map[string]any{"context": cx, "origin": "document", "format": map[string]any{"type": "image/png"}}, &out); err != nil {
			return res, err
		}
		png, err := base64.StdEncoding.DecodeString(out.Data)
		if err != nil {
			return res, err
		}
		if err := os.WriteFile(filepath.Join(shots, shotName(u, scheme)), png, 0o600); err != nil {
			return res, err
		}
	}
	return res, nil
}

func shotName(raw, scheme string) string {
	s := raw
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[i:]
	}
	s = strings.Trim(strings.NewReplacer("/", "-", ".html", "", "?", "-", "#", "-").Replace(s), "-")
	if s == "" {
		s = "home"
	}
	return s + "-" + scheme + ".png"
}

func prefix(p string, in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, p+s)
	}
	return out
}

// wantRoot is the root font size the page should compute to: the browser's default font size, times the root
// percent when -zoom is set. A page that fixes its font size in px computes to less, and the test is void.
func wantRoot(font, zoom int) float64 {
	want := float64(font)
	if zoom > 0 {
		want = want * float64(zoom) / 100
	}
	return want
}

// mergeOpen adds the faults found with the menu open to the result, marked as such.
func mergeOpen(res *result, open result) {
	res.Overflow = append(res.Overflow, prefix("menu open: ", open.Overflow)...)
	res.Clipped = append(res.Clipped, prefix("menu open: ", open.Clipped)...)
	res.Overlaps = append(res.Overlaps, prefix("menu open: ", open.Overlaps)...)
	if open.PageScrollWidth > open.ViewportWidth && res.PageScrollWidth <= res.ViewportWidth {
		res.Overflow = append(res.Overflow, fmt.Sprintf("menu open: the page scrolls sideways: %d px in a %d px window", open.PageScrollWidth, open.ViewportWidth))
	}
}
