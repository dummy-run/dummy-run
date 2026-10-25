package bidi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCDPPassthrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html lang="en"><title>CDP</title><button aria-label="Close">x</button></html>`))
	}))
	t.Cleanup(srv.Close)
	for _, browser := range browsersUnderTest(t) {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			b := launch(t, browser)
			c := b.Conn
			cx, err := c.CreateContext(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.Navigate(ctx, cx, srv.URL); err != nil {
				t.Fatal(err)
			}
			session, err := c.CDPSession(ctx, cx)
			if browser != "chrome" {
				if err == nil {
					t.Fatalf("%s gave a CDP session %q; only Chrome has one", browser, session)
				}
				return
			}
			if err != nil {
				t.Fatalf("CDPSession: %v", err)
			}
			var tree struct {
				Nodes []struct {
					Role struct {
						Value string `json:"value"`
					} `json:"role"`
					Name struct {
						Value string `json:"value"`
					} `json:"name"`
					BackendDOMNodeID int `json:"backendDOMNodeId"`
				} `json:"nodes"`
			}
			if err := c.CDP(ctx, session, "Accessibility.getFullAXTree", nil, &tree); err != nil {
				t.Fatalf("getFullAXTree: %v", err)
			}
			found := false
			for _, n := range tree.Nodes {
				if n.Role.Value == "button" && n.Name.Value == "Close" && n.BackendDOMNodeID > 0 {
					found = true
				}
			}
			if !found {
				t.Errorf("no button named Close in %d nodes", len(tree.Nodes))
			}
		})
	}
}
