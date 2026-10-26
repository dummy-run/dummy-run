package a11y

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/page"
)

const imageContrastPage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Banners</title>
<style>.hero{padding:30px;font-size:18px;color:#fff;width:600px}</style></head><body><main><h1>Banners</h1>
<p id="faint" class="hero" style="background:linear-gradient(90deg,#ddd,#fff)">Book your appointment online today</p>
<p id="strong" class="hero" style="background:linear-gradient(90deg,#123,#000)">Book your appointment online today</p>
<p id="short" class="hero" style="background:linear-gradient(90deg,#000 0 30%,#fff 30%)">Sale</p>
<p id="pinned" class="hero" style="background:linear-gradient(90deg,#ddd,#fff)">Book <b style="color:#fff !important;-webkit-text-fill-color:#fff !important">your</b> appointment</p>
<p id="tinted" class="hero" style="background:linear-gradient(90deg,#ddd,#fff)">Book <b style="-webkit-text-fill-color:#000">your</b> appointment</p>
<p id="filled" class="hero" style="background:linear-gradient(90deg,#ddd,#fff);-webkit-text-fill-color:#000">Book your appointment online today</p>
<p id="faded" class="hero" style="background:linear-gradient(90deg,#123,#000)">Book <b style="opacity:.3">your</b> appointment</p>
<p id="sized" class="hero" style="background:linear-gradient(90deg,#123,#000);font-size:24px">Book <small style="font-size:14px">your</small> appointment</p>
<p id="eased" class="hero" style="background:linear-gradient(90deg,#123,#000);transition:color 5s">Book your appointment online today</p>
<p id="halo" class="hero" style="background:linear-gradient(90deg,#ddd,#fff);text-shadow:0 0 3px #000">Book your appointment online today</p>
</main></body></html>`

func TestImageContrastMeasuresTextOnGradients(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, imageContrastPage)
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL}, Browser: browser, Viewports: []Viewport{Desktop},
				Schemes: []string{Light}, Headless: true, ImageContrast: true})
			if err != nil {
				t.Fatal(err)
			}
			r := rep.Results[0]
			if r.Error != "" || !r.ImageContrastChecked {
				t.Fatalf("error %q checked %v", r.Error, r.ImageContrastChecked)
			}
			measured := map[string]bool{}
			for _, m := range r.ImageContrast {
				measured[m.Selector] = true
			}
			// #eased fades its colour over five seconds. The hiding rule is !important, which no transition eases, so the
			// text is clear at once and #eased is measured.
			if !measured["#faint"] || !measured["#strong"] || !measured["#short"] || !measured["#eased"] {
				t.Fatalf("the gradients should be measured: %+v (needs review %+v)", r.ImageContrast, r.NeedsReview)
			}
			// A shadow can give text its contrast, and the picture can't show it, so that text is left for a person.
			if !slices.Contains(imageContrastTargets(r.NeedsReview), "#halo") {
				t.Fatalf("axe should leave #halo for review, or the shadow case isn't tested: %+v", r.NeedsReview)
			}
			// #pinned's child is the same white, but its inline !important fill colour beats the hiding style, so it
			// would still be in the picture.
			if !slices.Contains(imageContrastTargets(r.NeedsReview), "#pinned") {
				t.Fatalf("axe should leave #pinned for review, or the visible-child case isn't tested: %+v", r.NeedsReview)
			}
			// #tinted has a word drawn in black inside white text: one colour can't stand for it, so it's left too.
			if !slices.Contains(imageContrastTargets(r.NeedsReview), "#tinted") {
				t.Fatalf("axe should leave #tinted for review: %+v", r.NeedsReview)
			}
			if measured["#pinned"] || measured["#tinted"] {
				t.Errorf("#pinned and #tinted should be left unmeasured: %+v", r.ImageContrast)
			}
			// #faded has a word at 30% opacity, drawn paler than its colour. #sized mixes large text with a smaller word
			// that needs 4.5:1, not 3:1. One colour and one threshold can't stand for either, so both are left.
			for _, id := range []string{"#faded", "#sized"} {
				if !slices.Contains(imageContrastTargets(r.NeedsReview), id) {
					t.Fatalf("axe should leave %s for review: %+v", id, r.NeedsReview)
				}
				if measured[id] {
					t.Errorf("%s should be left unmeasured: %+v", id, r.ImageContrast)
				}
			}
			if measured["#halo"] {
				t.Errorf("text with a shadow should be left unmeasured: %+v", r.ImageContrast)
			}
			if want := len(imageContrastTargets(r.NeedsReview)) - len(r.ImageContrast); r.ImageContrastUnmeasured != want {
				t.Errorf("unmeasured %d, want %d", r.ImageContrastUnmeasured, want)
			}
			// #filled is white by colour but drawn black by its fill colour, so it's measured as black and passes.
			for _, m := range r.ImageContrast {
				if m.Selector == "#filled" && m.Text != "#000000" {
					t.Errorf("#filled should be measured with its drawn colour #000000, got %s", m.Text)
				}
			}
			if !measured["#filled"] {
				t.Errorf("#filled should be measured: %+v", r.ImageContrast)
			}
			var flagged []string
			for _, f := range r.Violations {
				if f.Rule == RuleImageContrast {
					for _, n := range f.Nodes {
						flagged = append(flagged, n.Target)
					}
				}
			}
			// #short sits on black, with white only in the empty part of its box, which isn't behind any words.
			if strings.Join(flagged, ",") != "#faint" {
				t.Errorf("flagged %v, want #faint only: %+v", flagged, r.ImageContrast)
			}
		})
	}
}

func TestImageContrastTargetsAndFindings(t *testing.T) {
	review := []Finding{
		{Rule: "color-contrast", Nodes: []Node{{Target: "#hero"}, {Target: "#card >> p"}}},
		{Rule: "image-alt", Nodes: []Node{{Target: "img"}}},
	}
	if got := strings.Join(imageContrastTargets(review), ","); got != "#hero,#card >> p" {
		t.Errorf("targets %q: only color-contrast nodes, in order", got)
	}
	allow, err := ParseAllowList(strings.NewReader("dummyrun-contrast-over-image | /home | #b | the banner is decorative\n"))
	if err != nil {
		t.Fatal(err)
	}
	ms := []page.ImageContrast{
		{Selector: "#a", Text: "#ffffff", Need: 4.5, Median: 2.1, Worst: 1.3},
		{Selector: "#b", Text: "#ffffff", Need: 3, Median: 2.9, Worst: 2},
		{Selector: "#ok", Text: "#ffffff", Need: 4.5, Median: 7, Worst: 3},
	}
	got := imageContrastFindings(ms, "/home", allow)
	if len(got) != 1 || got[0].Rule != RuleImageContrast || got[0].WCAG[0] != "1.4.3" || got[0].Impact != "serious" {
		t.Fatalf("one serious 1.4.3 finding: %+v", got)
	}
	n := got[0].Nodes
	// #ok passes at the median and fails only in its worst tenth, so it isn't reported.
	if len(n) != 2 || n[0].Target != "#a" || n[1].Target != "#b" {
		t.Fatalf("nodes %+v, want #a and #b", n)
	}
	if want := "text #ffffff measures 2.10:1 at the median of its background pixels and 1.30:1 at the worst tenth, and needs 4.5:1"; n[0].Detail != want {
		t.Errorf("detail %q, want %q", n[0].Detail, want)
	}
	if n[0].Allowed || !n[1].Allowed || n[1].Reason != "the banner is decorative" {
		t.Errorf("allow-list marking: %+v", n)
	}
	if imageContrastFindings(ms[2:], "/home", allow) != nil {
		t.Error("no failures should give no finding")
	}
}

// A page whose content security policy blocks inline styles keeps its text visible, so nothing is measured.
func TestImageContrastLeavesTextItCannotHide(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/s.css" {
			w.Header().Set("Content-Type", "text/css")
			_, _ = fmt.Fprint(w, "p{padding:30px;font-size:18px;color:#fff;width:600px;background:linear-gradient(90deg,#ddd,#fff)}")
			return
		}
		w.Header().Set("Content-Security-Policy", "style-src 'self'")
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Banner</title>
<link rel="stylesheet" href="/s.css"></head><body><main><h1>Banner</h1><p id="faint">Book your appointment online today</p></main></body></html>`)
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL}, Browser: browser, Viewports: []Viewport{Desktop},
				Schemes: []string{Light}, Headless: true, ImageContrast: true})
			if err != nil {
				t.Fatal(err)
			}
			r := rep.Results[0]
			if r.Error != "" {
				t.Fatal(r.Error)
			}
			if !slices.Contains(imageContrastTargets(r.NeedsReview), "#faint") {
				t.Fatalf("axe should leave #faint for review: %+v", r.NeedsReview)
			}
			if len(r.ImageContrast) != 0 || r.ImageContrastUnmeasured != 1 {
				t.Errorf("measured %+v, unmeasured %d: text the style can't hide should be left unmeasured", r.ImageContrast, r.ImageContrastUnmeasured)
			}
		})
	}
}
