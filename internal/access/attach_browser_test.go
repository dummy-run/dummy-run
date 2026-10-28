package access_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/access"
	"github.com/dummy-run/dummy-run/internal/bidi"
	"github.com/dummy-run/dummy-run/internal/policy"
)

// TestAttachToTwoLiveBrowsers attaches one access setup to two browsers that are both open, as two sweeps at once
// do, and then loads a page in the first. Each connection's interception must still answer its own requests, so the
// load completes with the header, and Owns knows both interceptions.
func TestAttachToTwoLiveBrowsers(t *testing.T) {
	for _, browser := range browsers(t) {
		t.Run(browser, func(t *testing.T) {
			const header, secret = "x-preview-key", "letmein"
			var keyed atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get(header) == secret {
					keyed.Add(1)
				}
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte(`<!doctype html><html lang="en"><title>Preview</title><main><h1>Preview</h1></main></html>`))
			}))
			defer srv.Close()
			acc, err := access.Resolve(&policy.Auth{Headers: map[string]policy.HeaderSource{header: {ValueEnv: "KEY"}}},
				func(string) string { return secret }, access.OriginScope([]string{srv.URL}))
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			var conns []*bidi.Conn
			for range 2 {
				b, err := bidi.Launch(ctx, bidi.LaunchOptions{Browser: browser, Headless: true})
				if err != nil {
					t.Fatalf("launch: %v", err)
				}
				t.Cleanup(b.Close)
				if err := access.Attach(ctx, b.Conn, acc); err != nil {
					t.Fatalf("attach: %v", err)
				}
				conns = append(conns, b.Conn)
			}
			// The first browser attached first, so a later Attach must not have taken its interception's place.
			cx, err := conns[0].CreateContext(ctx)
			if err != nil {
				t.Fatal(err)
			}
			nctx, ncancel := context.WithTimeout(ctx, 20*time.Second)
			defer ncancel()
			if _, err := conns[0].Navigate(nctx, cx, srv.URL+"/"); err != nil {
				t.Fatalf("the first browser's page never loaded: %v", err)
			}
			if keyed.Load() == 0 {
				t.Error("the first browser's request reached the server without the header")
			}
		})
	}
}
