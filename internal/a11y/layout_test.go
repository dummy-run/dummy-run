package a11y

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/page"
)

const layoutHead = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Layout fixture</title>
<style>body { margin: 0; font: 1rem/1.4 sans-serif; } .btn { display: inline-block; min-width: 48px; min-height: 48px; margin: 8px; padding: 8px 12px; border: 1px solid #333; background: #eee; color: #000; }</style>
</head><body><main>`

const layoutTail = `</main></body></html>`

// layoutPages are fixture pages, each with one known layout problem, and a clean page.
var layoutPages = map[string]string{
	"/clean": layoutHead + `<h1>Clean</h1><p>Text in a <a href="/x">sentence link</a> is exempt.</p>
<button class="btn" id="a">First</button> <button class="btn" id="b">Second</button>
<button id="lone" style="width:12px;height:12px;margin:60px"></button>
<button class="btn" id="icon"><span style="position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0 0 0 0);white-space:nowrap">Screen reader label</span>Menu</button></main>` + layoutTail,
	// Two links in one narrow paragraph wrap onto several lines. Their bounding boxes cross but the lines do not,
	// so they do not overlap. The details are closed, so the long block in them is not on the page.
	"/wrapped": layoutHead + `<h1>Wrapped</h1><p style="width:120px;font-size:20px;line-height:1.6">See <a href="/a">the first long link text</a> and <a href="/b">the second long link text</a> here.</p>
<details><summary class="btn">More</summary><pre style="height:400px">hidden</pre><button id="hiddentiny" style="width:8px;height:8px;padding:0"></button><button id="hiddentiny2" style="width:8px;height:8px;padding:0"></button></details>
<button class="btn" id="after">After</button>` + layoutTail,
	"/overflow": layoutHead + `<h1>Overflow</h1><div id="wide" style="width:640px;height:40px;background:#cde">A fixed-width block</div>
<button class="btn" id="a">Ok</button>` + layoutTail,
	"/clipped": layoutHead + `<h1>Clipped</h1>
<button id="clip" class="btn" style="width:110px;overflow:hidden;white-space:nowrap;padding:8px 0">Subscribe today for updates</button>
<a href="/y" id="ell" class="btn" style="display:block;width:90px;overflow:hidden;white-space:nowrap;text-overflow:ellipsis">A very long link label</a>` + layoutTail,
	"/overlap": layoutHead + `<h1>Overlap</h1><div style="position:relative;height:120px">
<button id="one" class="btn" style="position:absolute;left:10px;top:10px;width:140px;height:48px">Buy now</button>
<button id="two" class="btn" style="position:absolute;left:60px;top:30px;width:140px;height:48px">Save for later</button>
<button id="touch" class="btn" style="position:absolute;left:250px;top:10px;width:48px;height:48px">Ok</button>
<button id="touch2" class="btn" style="position:absolute;left:298px;top:10px;width:48px;height:48px">No</button></div>` + layoutTail,
	"/small": layoutHead + `<h1>Small</h1>
<button id="s1" style="width:14px;height:14px;margin:0 2px 20px 8px;padding:0"></button><button id="s2" style="width:14px;height:14px;margin:0;padding:0"></button>
<button id="ok" class="btn">Fine</button>` + layoutTail,
	// Two 10 px buttons 8 px apart: neither has another target in its 24 px circle, but the circles of the
	// two meet, so both fail.
	"/spaced": layoutHead + `<h1>Spaced</h1><div style="margin:40px">
<button id="pa" style="width:10px;height:10px;padding:0;margin:0 8px 0 0"></button><button id="pb" style="width:10px;height:10px;padding:0;margin:0"></button></div>` + layoutTail,
	// Decoys with a known answer of nothing wrong: the other slides of a carousel lie wholly outside the box that
	// clips them, two links' transparent padding boxes meet but neither covers the other's text, a long paragraph
	// is shortened on purpose with an ellipsis and a line clamp, a data table is wider than the window (WCAG 1.4.10
	// lets it scroll), and two radio buttons are left at the browser's own size.
	"/decoys": layoutHead + `<h1>Decoys</h1>
<div style="width:200px;overflow:hidden;white-space:nowrap"><div style="width:600px"><span style="display:inline-block;width:200px">Slide one</span><span style="display:inline-block;width:200px">Slide two</span><span style="display:inline-block;width:200px">Slide three</span></div></div>
<p><a href="/p" id="pad1" style="display:inline-block;padding:4px 30px 4px 0">First</a><a href="/q" id="pad2" style="display:inline-block;padding:4px 0 4px 30px;margin-left:-25px">Second</a></p>
<p id="ell" style="width:150px;overflow:hidden;white-space:nowrap;text-overflow:ellipsis">A teaser that runs on far past its box</p>
<p id="clamp" style="width:150px;overflow:hidden;display:-webkit-box;-webkit-box-orient:vertical;-webkit-line-clamp:2">A longer teaser that runs on over many lines until the clamp stops it after two of them</p>
<table id="data"><tr><td style="min-width:300px">Wide column one</td><td style="min-width:300px">Wide column two</td></tr></table>
<p><input type="radio" name="r" id="r1"><input type="radio" name="r" id="r2"></p>
<figure id="term" style="width:300px;overflow:hidden;margin:0"><div tabindex="0" style="overflow-x:auto"><pre style="white-space:pre;margin:0">$ a command whose output line is far wider than the terminal box around it, so it scrolls</pre></div></figure>` + layoutTail,
	// Text in a box of fixed height that hides overflow: part of the paragraph is cut off.
	"/cut": layoutHead + `<h1>Cut</h1><div id="note" style="height:44px;overflow:hidden;width:200px"><p style="margin:0">This notice runs on for several lines, so the box of fixed height cuts off its last lines.</p></div>` + layoutTail,
	// A fixed header 100 px tall, opaque, over a link at the top of the page: with focus the link is entirely
	// hidden. A second link is only partly under the header, which WCAG 2.4.11 allows.
	"/sticky": `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Sticky</title>
<style>body { margin: 0; font: 1rem/1.4 sans-serif; } header { position: fixed; top: 0; left: 0; right: 0; height: 100px; background: #fff; }</style></head>
<body><header><span>Site name</span></header><main style="padding-top:10px">
<a href="/hidden" id="under" style="display:inline-block;padding:12px">Hidden link</a>
<p style="margin-top:44px"><a href="/partly" id="partly" style="display:inline-block;padding:12px 12px 40px">Partly hidden link</a></p>
<h1 style="margin-top:200px">Content</h1><p style="height:1200px">Long page.</p></main></body></html>`,
	// Sized in rem and in px: text that follows the default font size grows, the px text does not.
	"/scale": layoutHead + `<h1>Scale</h1>
<p id="rem" style="font-size:1rem">Rem text</p><p id="px" style="font-size:16px">Px text</p>
<button id="tight" class="btn" style="width:150px;overflow:hidden;white-space:nowrap;padding:8px 0">Subscribe today</button>` + layoutTail,
}

func layoutServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for path, body := range layoutPages {
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, body)
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// layoutRun sweeps one page in one browser at the phone size and returns its result.
func layoutRun(t *testing.T, browser, url string, scales []int, shots string) []PageResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	rep, err := Run(ctx, Config{URLs: []string{url}, Browser: browser, Viewports: []Viewport{Phone}, Schemes: []string{Light},
		Headless: true, Layout: true, TextScales: scales, ScreenshotDir: shots})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rep.Results {
		if r.Error != "" {
			t.Fatalf("%s: %s", r.URL, r.Error)
		}
	}
	return rep.Results
}

func layoutFinding(r PageResult, rule string) *Finding {
	for i := range r.Violations {
		if r.Violations[i].Rule == rule {
			return &r.Violations[i]
		}
	}
	return nil
}

func targets(f *Finding) string {
	var ts []string
	for _, n := range f.Nodes {
		ts = append(ts, n.Target)
	}
	return strings.Join(ts, " | ")
}

// TestLayoutChecksInBrowser loads each fixture page in each browser named in DUMMYRUN_BROWSERS and
// checks that the geometry checks flag exactly what each page plants.
func TestLayoutChecksInBrowser(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome or firefox to run the browser layout test")
	}
	srv := layoutServer(t)
	cases := []struct {
		path, rule, target string
		wcag               string
	}{
		{"/overflow", RuleReflow, "wide", "1.4.10"},
		{"/clipped", RuleClippedText, "clip", "1.4.4"},
		{"/clipped", RuleClippedText, "ell", "1.4.4"},
		{"/overlap", RuleOverlap, "one", "1.4.10 1.4.4"},
		{"/small", RuleTargetSize, "s1", "2.5.8"},
		{"/cut", RuleClippedText, "p", "1.4.4"},
		{"/sticky", RuleFocusObscured, "under", "2.4.11"},
	}
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			shots := t.TempDir()
			for _, c := range cases {
				res := layoutRun(t, browser, srv.URL+c.path, nil, shots)[0]
				if !res.LayoutChecked {
					t.Errorf("%s: the layout checks did not run", c.path)
				}
				f := layoutFinding(res, c.rule)
				if f == nil {
					t.Errorf("%s: no %s finding; got %v", c.path, c.rule, res.Violations)
					continue
				}
				if !strings.Contains(targets(f), c.target) {
					t.Errorf("%s: %s targets %q, want one that names %q", c.path, c.rule, targets(f), c.target)
				}
				if got := strings.Join(f.WCAG, " "); got != c.wcag {
					t.Errorf("%s: %s WCAG %q, want %q", c.path, c.rule, got, c.wcag)
				}
				if f.Impact == "" || f.Help == "" || f.HelpURL == "" || f.Nodes[0].Detail == "" {
					t.Errorf("%s: %s is missing impact, help, URL or detail: %+v", c.path, c.rule, f)
				}
				if res.Failing() == 0 {
					t.Errorf("%s: a layout finding must fail the sweep", c.path)
				}
				// Each fixture plants one kind of problem, so no other layout rule may fire.
				for _, other := range LayoutRules {
					// Fixed-size fixtures may also trip the target size check.
					if other != c.rule && other != RuleTargetSize && layoutFinding(res, other) != nil {
						t.Errorf("%s: unexpected %s: %s", c.path, other, targets(layoutFinding(res, other)))
					}
				}
			}
			// The overlap page also has two touching buttons that must not be called an overlap.
			res := layoutRun(t, browser, srv.URL+"/overlap", nil, "")[0]
			if f := layoutFinding(res, RuleOverlap); f == nil || len(f.Nodes) != 1 || strings.Contains(targets(f), "touch") {
				t.Errorf("overlap nodes %v, want only the pair #one and #two", f)
			}
			// Spacing: two undersized targets whose circles meet both fail, even with no box inside the other's circle.
			res = layoutRun(t, browser, srv.URL+"/spaced", nil, "")[0]
			if f := layoutFinding(res, RuleTargetSize); f == nil || !strings.Contains(targets(f), "pa") || !strings.Contains(targets(f), "pb") {
				t.Errorf("spaced targets %v, want both #pa and #pb", f)
			}
			// Spacing: the isolated 12 px button is exempt, the crowded pair is not.
			res = layoutRun(t, browser, srv.URL+"/small", nil, "")[0]
			f := layoutFinding(res, RuleTargetSize)
			if f == nil || !strings.Contains(targets(f), "s2") || strings.Contains(targets(f), "ok") {
				t.Errorf("small targets %v, want both crowded 14 px buttons and not #ok", f)
			}
			// Links that wrap compare line by line, and the content of a closed <details> is not on the page.
			res = layoutRun(t, browser, srv.URL+"/wrapped", nil, "")[0]
			if f := layoutFinding(res, RuleOverlap); f != nil {
				t.Errorf("wrapped links reported as overlapping: %s", targets(f))
			}
			// Two tiny buttons side by side would fail the target size rule if they were on the page.
			if f := layoutFinding(res, RuleTargetSize); f != nil && strings.Contains(targets(f), "hiddentiny") {
				t.Errorf("content of a closed details is reported: %s", targets(f))
			}
			// Known answer: nothing on the decoy page is a layout failure.
			res = layoutRun(t, browser, srv.URL+"/decoys", nil, "")[0]
			for _, rule := range LayoutRules {
				if f := layoutFinding(res, rule); f != nil {
					t.Errorf("decoys page has %s: %s (%s)", rule, targets(f), f.Nodes[0].Detail)
				}
			}
			// The partly hidden link is allowed at level AA; only the wholly hidden one fails.
			res = layoutRun(t, browser, srv.URL+"/sticky", nil, "")[0]
			if f := layoutFinding(res, RuleFocusObscured); f == nil || len(f.Nodes) != 1 || !strings.Contains(f.Nodes[0].Detail, "(fixed)") {
				t.Errorf("obscured %v, want only #under, hidden by the fixed header", f)
			}
			res = layoutRun(t, browser, srv.URL+"/clean", nil, "")[0]
			for _, rule := range LayoutRules {
				if f := layoutFinding(res, rule); f != nil {
					t.Errorf("clean page has %s: %s", rule, targets(f))
				}
			}
			// Screenshots: a crop file exists for the clipped button.
			_ = layoutRun(t, browser, srv.URL+"/clipped", nil, shots)
			files, _ := filepath.Glob(filepath.Join(shots, "*clipped-text*.png"))
			if len(files) == 0 {
				t.Errorf("no screenshot crop in %s", shots)
			}
			for _, f := range files {
				if fi, err := os.Stat(f); err != nil || fi.Size() < 100 {
					t.Errorf("screenshot %s is empty or missing", f)
				}
			}
		})
	}
}

// TestTextScaleInBrowser checks that text scale works the way a person's setting does: the default
// font size grows, rem text follows it, px text does not, the window stays the same, and a control
// that fits at 100 percent clips at 200.
func TestTextScaleInBrowser(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome or firefox to run the browser text scale test")
	}
	srv := layoutServer(t)
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			results := layoutRun(t, browser, srv.URL+"/scale", []int{100, 200}, "")
			if len(results) != 2 {
				t.Fatalf("want a run at 100 and one at 200, got %d", len(results))
			}
			at100, at200 := results[0], results[1]
			if at100.TextScale != 0 || at200.TextScale != 200 {
				t.Errorf("text scales recorded as %d and %d, want 0 and 200", at100.TextScale, at200.TextScale)
			}
			if f := layoutFinding(at100, RuleClippedText); f != nil {
				t.Errorf("at 100 percent the button fits, got %s", targets(f))
			}
			f := layoutFinding(at200, RuleClippedText)
			if f == nil || !strings.Contains(targets(f), "tight") {
				t.Errorf("at 200 percent the button's text should be cut off, got %v", f)
			}
			if got := at200.Setting(); !strings.HasSuffix(got, "/text-200") {
				t.Errorf("setting %q lacks the text scale", got)
			}
			if at200.Width != Phone.Width {
				t.Errorf("window width %d, want %d: text scale must not change the window", at200.Width, Phone.Width)
			}
		})
	}
}

func TestLayoutFindingsFromMeasurements(t *testing.T) {
	l := &page.Layout{
		Overflow: &page.LayoutOverflow{ScrollWidth: 640, ClientWidth: 375, Excess: 265},
		Clipped:  []page.LayoutClipped{{LayoutElement: page.LayoutElement{Selector: "#clip"}, Detail: "text is cut off", Box: page.LayoutBox{X: 1, Y: 2, Width: 3, Height: 4}}},
		Overlaps: []page.LayoutOverlap{{A: page.LayoutElement{Selector: "#a"}, B: page.LayoutElement{Selector: "#b"}, Covered: 6, Sampled: 9, Text: true}},
		Small:    []page.LayoutSmall{{LayoutElement: page.LayoutElement{Selector: "#s"}, Width: 12, Height: 12}},
		Obscured: []page.LayoutObscured{{LayoutElement: page.LayoutElement{Selector: "#skip"}, By: page.LayoutElement{Selector: "header"}, ByPosition: "sticky"}},
	}
	allow, err := ParseAllowList(strings.NewReader("dummyrun-target-size | /p | #s | Icon buttons are spaced by the design\n"))
	if err != nil {
		t.Fatal(err)
	}
	list, boxes := layoutFindings(l, "/p", allow)
	var rules []string
	for _, f := range list {
		rules = append(rules, f.Rule)
	}
	if got := strings.Join(rules, " "); got != "dummyrun-reflow dummyrun-clipped-text dummyrun-overlap dummyrun-target-size dummyrun-focus-obscured" {
		t.Fatalf("rules %q", got)
	}
	if list[0].Nodes[0].Target != "html" || !strings.Contains(list[0].Nodes[0].Detail, "265 px wider than the 375 px window") {
		t.Errorf("reflow node %+v", list[0].Nodes[0])
	}
	if n := list[2].Nodes[0]; n.Target != "#a & #b" || n.Detail != "its text is under #b at 6 of 9 points the browser checked" {
		t.Errorf("overlap node %+v", n)
	}
	if n := list[4].Nodes[0]; n.Target != "#skip" || n.Detail != "with focus, it is entirely hidden by header (sticky)" || list[4].WCAG[0] != "2.4.11" {
		t.Errorf("obscured node %+v", n)
	}
	if n := list[3].Nodes[0]; !n.Allowed || n.Reason == "" {
		t.Errorf("the allow-list should accept the small target: %+v", n)
	}
	if len(boxes[RuleClippedText]) != 1 || boxes[RuleClippedText][0] == nil || boxes[RuleReflow][0] != nil {
		t.Errorf("boxes %+v", boxes)
	}
	if !IsLayoutRule(RuleOverlap) || IsLayoutRule("color-contrast") {
		t.Error("IsLayoutRule")
	}
	if list2, _ := layoutFindings(&page.Layout{}, "/", nil); len(list2) != 0 {
		t.Errorf("an empty measurement made findings: %+v", list2)
	}
}

func TestScreenshotNamesDoNotCollide(t *testing.T) {
	// Two addresses that share their first 60 characters, or differ only in punctuation, must not share a file.
	l := &page.Layout{Clipped: []page.LayoutClipped{{LayoutElement: page.LayoutElement{Selector: "#c"}, Box: page.LayoutBox{Width: 10, Height: 10}}}}
	names := map[string]bool{}
	for _, u := range []string{"http://x.test/a/b", "http://x.test/a-b", "http://x.test/" + strings.Repeat("p", 70) + "1", "http://x.test/" + strings.Repeat("p", 70) + "2"} {
		list, _ := layoutFindings(l, "/", nil)
		res := PageResult{URL: u, Browser: "chrome", Viewport: "phone", Scheme: "light", Violations: list}
		name := screenshotName("", &res, RuleClippedText, 0)
		if names[name] {
			t.Errorf("%s collides with an earlier address: %s", u, name)
		}
		names[name] = true
	}
}

func TestSettingNamesTheTextScale(t *testing.T) {
	p := PageResult{Browser: "chrome", Viewport: "phone", Scheme: "light"}
	if got := p.Setting(); got != "chrome/phone/light" {
		t.Errorf("got %q", got)
	}
	p.TextScale = 200
	if got := p.Setting(); got != "chrome/phone/light/text-200" {
		t.Errorf("got %q", got)
	}
}

func TestRunRefusesATextScaleOutsideTheRange(t *testing.T) {
	for _, n := range []int{50, 99, 301, 500} {
		if _, err := Run(context.Background(), Config{URLs: []string{"http://127.0.0.1:1/"}, Browser: "chrome", TextScales: []int{n}}); err == nil || !strings.Contains(err.Error(), "text scale") {
			t.Errorf("%d: err = %v", n, err)
		}
	}
}

func TestSummaryShowsDetailAndScreenshot(t *testing.T) {
	r := &Report{Browser: "chrome", Results: []PageResult{{URL: "http://x/", Browser: "chrome", Viewport: "phone", Scheme: "light", TextScale: 200,
		Violations: []Finding{{Rule: RuleOverlap, Impact: "serious", WCAG: []string{"1.4.10", "1.4.4"}, Help: "A control is covered by another control", HelpURL: "https://example.test/h",
			Nodes: []Node{{Target: "#a & #b", Detail: "its text is under #b at 6 of 9 points the browser checked", Screenshot: "/shots/a.png"}}}}}}}
	var b strings.Builder
	r.WriteSummary(&b)
	for _, want := range []string{"dummyrun-overlap (serious, WCAG 1.4.10 1.4.4) at #a & #b", "its text is under #b at 6 of 9 points the browser checked", "screenshot /shots/a.png", "chrome/phone/light/text-200"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("summary lacks %q:\n%s", want, b.String())
		}
	}
}
