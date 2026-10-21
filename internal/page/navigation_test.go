package page

import (
	"testing"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

func TestIsTopNavigation(t *testing.T) {
	html := []bidi.Header{bidi.StringHeader("Accept", "text/html,application/xhtml+xml")}
	img := []bidi.Header{bidi.StringHeader("Accept", "image/avif,image/webp,*/*")}
	ev := func(ctx, nav, dest string, h []bidi.Header) bidi.BeforeRequestSent {
		return bidi.BeforeRequestSent{Context: ctx, Navigation: nav, Request: bidi.RequestData{Destination: dest, Headers: h}}
	}
	cases := []struct {
		name string
		e    bidi.BeforeRequestSent
		want bool
	}{
		{"document destination", ev("tab", "", "document", nil), true},
		{"document destination in another context", ev("frame", "", "document", nil), true},
		{"image destination", ev("tab", "", "image", img), false},
		{"iframe destination", ev("tab", "n1", "iframe", html), true},
		{"navigation id in the tab", ev("tab", "n1", "", nil), true},
		{"navigation id in a frame", ev("frame", "n2", "", html), true},
		{"script destination", ev("tab", "", "script", nil), false},
		{"no hints, html in the tab", ev("tab", "", "", html), true},
		{"no hints, image in the tab", ev("tab", "", "", img), false},
		{"no hints, no accept", ev("tab", "", "", nil), false},
	}
	for _, c := range cases {
		if got := isTopNavigation(c.e, "tab"); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
