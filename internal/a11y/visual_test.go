package a11y

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/page"
	"github.com/dummy-run/dummy-run/internal/visual"
)

// TestVisualComparesWithTheLastSweep sweeps a page three times into one directory: the first keeps a picture, the
// second finds nothing changed though the masked clock did, and the third finds the changed box.
func TestVisualComparesWithTheLastSweep(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	var loads, colour atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		bg := []string{"#1d70b8", "#d4351c"}[colour.Load()]
		_, _ = fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Visual</title>
<style>body{margin:0;font:16px sans-serif} #box{width:200px;height:120px;background:%s} .clock{font-size:40px}</style>
</head><body><main><h1>Visual</h1><p class="clock">%d</p><div id="box"></div></main></body></html>`, bg, loads.Add(1)*7919)
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			dir := t.TempDir()
			colour.Store(0)
			sweep := func(masks ...string) PageResult {
				t.Helper()
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				defer cancel()
				rep, err := Run(ctx, Config{URLs: []string{srv.URL}, Browser: browser, Viewports: []Viewport{Desktop},
					Schemes: []string{Light}, Headless: true, VisualDir: dir, Masks: masks})
				if err != nil {
					t.Fatal(err)
				}
				return rep.Results[0]
			}
			first := sweep(".clock")
			if first.VisualError != "" || first.Visual == nil || !first.Visual.First || first.Visual.Masked != 1 {
				t.Fatalf("first sweep: %+v, error %q", first.Visual, first.VisualError)
			}
			if _, err := os.Stat(first.Visual.Screen); err != nil {
				t.Fatal(err)
			}
			same := sweep(".clock")
			if c := same.Visual; same.VisualError != "" || c == nil || c.Change == nil || c.Changed != 0 || c.Total == 0 || c.Diff != "" {
				t.Fatalf("masked clock: %+v, error %q", c, same.VisualError)
			}
			colour.Store(1)
			changed := sweep(".clock")
			c := changed.Visual
			if c == nil || c.Change == nil || c.Changed < 200*120/2 || c.Diff == "" {
				t.Fatalf("changed box: %+v, error %q", c, changed.VisualError)
			}
			if _, err := os.Stat(c.Diff); err != nil {
				t.Fatal(err)
			}
			if bad := sweep("!!"); !strings.Contains(bad.VisualError, `--mask "!!" is not a CSS selector`) || bad.Error != "" {
				t.Errorf("a bad mask: visual error %q, page error %q", bad.VisualError, bad.Error)
			}
		})
	}
}

func TestSummaryListsVisualChanges(t *testing.T) {
	base := PageResult{URL: "https://example.com/", Browser: "chrome", Viewport: "desktop", Scheme: Light,
		Violations: []Finding{}, NeedsReview: []Finding{}}
	with := func(v *Visual, verr string) PageResult {
		p := base
		p.Visual, p.VisualError = v, verr
		return p
	}
	for name, c := range map[string]struct {
		p    PageResult
		want string
	}{
		"error":     {with(nil, `--mask "!!" is not a CSS selector`), `VISUAL https://example.com/ (chrome/desktop/light): --mask "!!" is not a CSS selector`},
		"first":     {with(&Visual{Screen: "s.png", First: true}, ""), ""},
		"unchanged": {with(&Visual{Screen: "s.png", Change: &visual.Change{Total: 100}}, ""), ""},
		"resized":   {with(&Visual{Screen: "s.png", Change: &visual.Change{SizeChanged: true, HashDistance: 9}}, ""), "SCREEN https://example.com/ (chrome/desktop/light): the window changed size, hash distance 9 of 64"},
		"changed":   {with(&Visual{Screen: "s.png", Change: &visual.Change{Changed: 5, Total: 100, HashDistance: 2, Diff: "s.diff.png"}}, ""), "SCREEN https://example.com/ (chrome/desktop/light): 5 of 100 pixels changed, hash distance 2 of 64, s.diff.png"},
	} {
		var b strings.Builder
		(&Report{Results: []PageResult{c.p}}).WriteSummary(&b)
		got := b.String()
		if c.want == "" {
			if strings.Contains(got, "SCREEN") || strings.Contains(got, "VISUAL") {
				t.Errorf("%s: a line for no change:\n%s", name, got)
			}
		} else if !strings.Contains(got, c.want) {
			t.Errorf("%s: summary lacks %q:\n%s", name, c.want, got)
		}
	}
}

func TestScreenName(t *testing.T) {
	p := PageResult{URL: "https://example.com/a?b=1", Browser: "firefox", Viewport: "phone", Scheme: Dark, TextScale: 200}
	got := filepath.Base(screenName("dir", &p))
	if !strings.HasPrefix(got, "example-com-a-b-1-") || !strings.HasSuffix(got, "-firefox-phone-dark-text-200.png") {
		t.Errorf("name %q", got)
	}
	q := p
	q.URL = "https://example.com/a?b=2"
	if screenName("dir", &q) == screenName("dir", &p) {
		t.Error("two addresses that differ only in punctuation share a file")
	}
}

// TestMaskLeavesNothingBehind checks a bad selector after a good one draws no box, that the boxes come off after
// the picture, and that an unreadable earlier picture is replaced and a stale diff removed.
func TestMaskLeavesNothingBehind(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!doctype html><html lang="en"><head><title>Mask</title></head><body><p class="clock">12:00</p></body></html>`)
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			p, err := page.Open(ctx, page.Options{Browser: browser, Headless: true, Width: 800, Height: 600})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(p.Close)
			if o := p.Visit(ctx, srv.URL); o.Error != "" {
				t.Fatal(o.Error)
			}
			conn, cx := p.Conn(), p.Context()
			boxes := func() int {
				t.Helper()
				var n int
				if err := conn.CallFunctionJSON(ctx, cx, `() => document.querySelectorAll("[data-dummyrun-mask]").length`, &n); err != nil {
					t.Fatal(err)
				}
				return n
			}
			dir := t.TempDir()
			res := PageResult{URL: srv.URL, Browser: browser, Viewport: "desktop", Scheme: Light}
			if err := compareScreen(ctx, conn, cx, dir, []string{".clock", "!!"}, &res); err == nil || boxes() != 0 {
				t.Fatalf("a bad selector: error %v, %d boxes left", err, boxes())
			}
			if err := compareScreen(ctx, conn, cx, dir, []string{".clock"}, &res); err != nil || res.Visual.Masked != 1 || boxes() != 0 {
				t.Fatalf("a good selector: error %v, %+v, %d boxes left", err, res.Visual, boxes())
			}
			shot := res.Visual.Screen
			diff := strings.TrimSuffix(shot, ".png") + ".diff.png"
			if err := os.WriteFile(diff, []byte("old"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := compareScreen(ctx, conn, cx, dir, []string{".clock"}, &res); err != nil || res.Visual.Change == nil || res.Visual.Changed != 0 {
				t.Fatalf("no change: error %v, %+v", err, res.Visual)
			}
			if _, err := os.Stat(diff); !os.IsNotExist(err) {
				t.Errorf("the stale diff is still there: %v", err)
			}
			if err := os.WriteFile(shot, []byte("not a png"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(diff, []byte("old"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := compareScreen(ctx, conn, cx, dir, nil, &res); err != nil || !res.Visual.First {
				t.Fatalf("unreadable earlier picture: error %v, %+v", err, res.Visual)
			}
			if _, err := os.Stat(diff); !os.IsNotExist(err) {
				t.Errorf("the diff from the unreadable picture is still there: %v", err)
			}
			if err := compareScreen(ctx, conn, cx, dir, nil, &res); err != nil || res.Visual.Change == nil {
				t.Fatalf("after replacing it: error %v, %+v", err, res.Visual)
			}
		})
	}
}
