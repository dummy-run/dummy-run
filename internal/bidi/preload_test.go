package bidi_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

// A page that asks the browser to fetch the next page ahead of a click gets nothing fetched: such a request would skip
// the interception, with no run headers and no request policy.
func TestNoPagesAreFetchedAhead(t *testing.T) {
	var ahead atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Sec-Purpose") != "" {
			ahead.Add(1)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Ahead</title>
<script type="speculationrules">{"prefetch": [{"source": "list", "urls": ["/next"]}], "prerender": [{"source": "list", "urls": ["/later"]}]}</script>
</head><body><a href="/next">Next</a> <a href="/later">Later</a></body></html>`)
	}))
	defer srv.Close()
	for _, browser := range browsersUnderTest(t) {
		t.Run(browser, func(t *testing.T) {
			ahead.Store(0)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
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
			time.Sleep(3 * time.Second)
			if n := ahead.Load(); n != 0 {
				t.Errorf("%d pages were fetched ahead of a click", n)
			}
		})
	}
}
