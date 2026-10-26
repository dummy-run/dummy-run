package a11y

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDiffLines(t *testing.T) {
	a := []string{"- banner:", "  - link \"Home\"", "- main:", "  - heading \"Shop\" [level=1]", "  - button \"Buy\""}
	b := []string{"- banner:", "  - link \"Home\"", "  - link \"Basket\"", "- main:", "  - heading \"Shop\" [level=1]", "  - button \"Buy\" [disabled]"}
	c := diffLines(a, b)
	if !reflect.DeepEqual(c.Added, []string{"  - link \"Basket\"", "  - button \"Buy\" [disabled]"}) || !reflect.DeepEqual(c.Removed, []string{"  - button \"Buy\""}) {
		t.Errorf("added %q removed %q", c.Added, c.Removed)
	}
	if same := diffLines(a, a); same.AddedCount+same.RemovedCount != 0 {
		t.Errorf("no change: %+v", same)
	}
	var long []string
	for i := range 80 {
		long = append(long, fmt.Sprintf("- link \"%d\"", i))
	}
	if c := diffLines(nil, long); c.AddedCount != 80 || len(c.Added) != maxChangeLines {
		t.Errorf("long: %d counted, %d listed", c.AddedCount, len(c.Added))
	}
}

func TestSnapshotNames(t *testing.T) {
	a := snapshotName("https://example.com/shop?a=1", "chrome/desktop/light")
	if a != snapshotName("https://example.com/shop?a=1", "chrome/desktop/light") || a == snapshotName("https://example.com/shop?a=2", "chrome/desktop/light") ||
		a == snapshotName("https://example.com/shop?a=1", "chrome/phone/light") || strings.ContainsAny(a, "/?:") || !strings.HasSuffix(a, ".yaml") {
		t.Errorf("snapshot name %q", a)
	}
}

func TestARIASnapshotsShowWhatChanged(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	var second atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		extra, buy := "", `<button>Buy</button>`
		if second.Load() {
			extra, buy = `<a href="/basket">Basket</a>`, `<button disabled>Buy</button>`
		}
		_, _ = fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Shop</title></head><body>
<header><a href="/">Home</a> %s</header><main><h1>Shop</h1><p>Words that change: %d</p>%s
<input type="checkbox" aria-label="Gift wrap" checked><div hidden><button>Hidden</button></div></main></body></html>`, extra, time.Now().UnixNano(), buy)
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			second.Store(false)
			dir := t.TempDir()
			run := func() PageResult {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				rep, err := Run(ctx, Config{URLs: []string{srv.URL}, Browser: browser, Viewports: []Viewport{Desktop},
					Schemes: []string{Light}, Headless: true, ARIASnapshots: dir})
				if err != nil {
					t.Fatal(err)
				}
				r := rep.Results[0]
				if r.Error != "" || r.ARIASnapshotError != "" || r.ARIASnapshot == "" {
					t.Fatalf("error %q, snapshot error %q, file %q", r.Error, r.ARIASnapshotError, r.ARIASnapshot)
				}
				return r
			}
			first := run()
			if first.ARIAChanges != nil {
				t.Errorf("a first snapshot has changes: %+v", first.ARIAChanges)
			}
			text, _ := os.ReadFile(first.ARIASnapshot)
			for _, want := range []string{"- banner:\n  - link \"Home\"\n", "- main:\n  - heading \"Shop\" [level=1]\n  - button \"Buy\"\n", "  - checkbox \"Gift wrap\" [checked]\n"} {
				if !strings.Contains(string(text), want) {
					t.Errorf("snapshot lacks %q:\n%s", want, text)
				}
			}
			if strings.Contains(string(text), "Hidden") || strings.Contains(string(text), "Words that change") {
				t.Errorf("the snapshot has hidden controls or text:\n%s", text)
			}
			// The same page again, with different words, has no changes.
			if again := run(); again.ARIAChanges == nil || again.ARIAChanges.AddedCount+again.ARIAChanges.RemovedCount != 0 {
				t.Errorf("unchanged page: %+v", again.ARIAChanges)
			}
			second.Store(true)
			c := run().ARIAChanges
			if c == nil || !reflect.DeepEqual(c.Added, []string{`  - link "Basket"`, `  - button "Buy" [disabled]`}) || !reflect.DeepEqual(c.Removed, []string{`  - button "Buy"`}) {
				t.Errorf("changes %+v", c)
			}
		})
	}
}

func TestSummaryListsSnapshotChanges(t *testing.T) {
	r := &Report{Results: []PageResult{
		{URL: "https://example.com/", Viewport: "desktop", Scheme: "light", ARIAChanges: &ARIAChanges{
			Added: []string{`  - link "Basket"`}, Removed: []string{`  - button "Buy"`}, AddedCount: 1, RemovedCount: 1}},
		{URL: "https://example.com/same", Viewport: "desktop", Scheme: "light", ARIAChanges: &ARIAChanges{}},
		{URL: "https://example.com/bad", Viewport: "desktop", Scheme: "light", ARIASnapshotError: "disk full"},
	}}
	var b strings.Builder
	r.WriteSummary(&b)
	out := b.String()
	for _, want := range []string{"CHANGED https://example.com/ (", "1 lines added and 1 removed", `     - button "Buy"`, `     + link "Basket"`, "SNAPSHOT https://example.com/bad (", "disk full"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "CHANGED https://example.com/same") {
		t.Errorf("an unchanged page is listed:\n%s", out)
	}
}
