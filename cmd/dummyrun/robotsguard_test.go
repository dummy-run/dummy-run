package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRobotsGuardBlocksADisallowedSitemapBeforeItIsRequested(t *testing.T) {
	var sitemapHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			_, _ = w.Write([]byte("User-agent: DummyRun\nDisallow: /private-sitemap.xml\n"))
		default:
			sitemapHits.Add(1)
		}
	}))
	defer srv.Close()
	guard := robotsGuard(srv.Client(), false, true)
	err := guard(context.Background(), srv.URL+"/private-sitemap.xml")
	if err == nil || !strings.Contains(err.Error(), "disallows DummyRun") {
		t.Fatalf("got %v, want a robots refusal", err)
	}
	if err := guard(context.Background(), srv.URL+"/sitemap.xml"); err != nil {
		t.Errorf("an allowed sitemap was refused: %v", err)
	}
	if sitemapHits.Load() != 0 {
		t.Errorf("the guard requested %d sitemap files", sitemapHits.Load())
	}
	if robotsGuard(srv.Client(), false, false) != nil {
		t.Error("with respect off the guard must be nil")
	}
}
