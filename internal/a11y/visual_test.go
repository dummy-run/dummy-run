package a11y

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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
