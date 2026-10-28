package robots

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestMatchEdges checks wildcards and the end anchor where a part is empty or only just fits.
func TestMatchEdges(t *testing.T) {
	for _, c := range []struct {
		pattern, s string
		want       bool
	}{
		{"/a*b$", "/ab", true},
		{"/a*ab$", "/ab", false},
		{"/a*b$", "/abb", true},
		{"/a*b$", "/abc", false},
		{"/a$", "/a", true},
		{"/a$", "/ab", false},
		{"/a*b*c", "/abc", true},
		{"/a*b*c", "/acb", false},
		{"/a*c*b", "/abcab", true},
		{"/*$", "/", true},
		{"*", "", true},
	} {
		if got := matches(c.pattern, c.s); got != c.want {
			t.Errorf("matches(%q, %q) = %v, want %v", c.pattern, c.s, got, c.want)
		}
	}
}

// TestNormaliseEdges checks escapes cut short at the end of a path, each edge of the hex digits, and the first byte
// past ASCII.
func TestNormaliseEdges(t *testing.T) {
	for in, want := range map[string]string{
		"/a%4":    "/a%4",
		"/a%":     "/a%",
		"/a%41":   "/aA",
		"/%2f":    "/%2F",
		"/%2F":    "/%2F",
		"/%3a":    "/%3A",
		"/%3A":    "/%3A",
		"/%7e":    "/~",
		"/%4F":    "/O",
		"/%4f":    "/O",
		"/%30%39": "/09",
		"/%61%7A": "/az",
		"/%41%5A": "/AZ",
		"/%2/":    "/%2/",
		"/%2:":    "/%2:",
		"/%2`":    "/%2`",
		"/%2g":    "/%2g",
		"/%2@":    "/%2@",
		"/%2G":    "/%2G",
		"/\x7f":   "/\x7f",
		"/\x80":   "/%80",
	} {
		if got := normalise(in); got != want {
			t.Errorf("normalise(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestFetchStopsWhenCancelled checks a fetch the caller cancelled returns the error, not rules that close the site.
func TestFetchStopsWhenCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := Fetch(ctx, nil, srv.URL+"/", "DummyRun", "")
	if !errors.Is(err, context.Canceled) || res != nil {
		t.Errorf("Fetch with a cancelled context = %+v, %v; want context.Canceled", res, err)
	}
}
