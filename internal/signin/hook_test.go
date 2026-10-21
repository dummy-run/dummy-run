package signin

import (
	"context"
	"net"
	"testing"

	"github.com/dummy-run/dummy-run/internal/page"
	"github.com/dummy-run/dummy-run/internal/policy"
)

// The sign-in hook applies the same request rule as a run: pages stay on the target, a page's own images may come
// from a public host, and writes are refused on a read-only target.
func TestHookAppliesTheRequestRule(t *testing.T) {
	tgt := policy.Target{Adapter: "url", Environment: "test", BaseURL: "https://app.example.test/", AllowedHosts: []string{"app.example.test"}, ReadOnly: true,
		Lookup: func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("93.184.216.34")}, nil }}
	hook := Hook(tgt, policy.NewGate(tgt))
	for _, c := range []struct {
		name  string
		req   page.Request
		block bool
	}{
		{"own page", page.Request{Method: "GET", URL: "https://app.example.test/login", Navigation: true}, false},
		{"own image", page.Request{Method: "GET", URL: "https://app.example.test/logo.png"}, false},
		{"cdn image", page.Request{Method: "GET", URL: "https://cdn.other.test/logo.png"}, false},
		{"page on another site", page.Request{Method: "GET", URL: "https://cdn.other.test/", Navigation: true}, true},
		{"write to the target", page.Request{Method: "POST", URL: "https://app.example.test/x"}, true},
		{"write to another site", page.Request{Method: "POST", URL: "https://cdn.other.test/x"}, true},
	} {
		if got := hook(c.req).Block; got != c.block {
			t.Errorf("%s: block = %v, want %v", c.name, got, c.block)
		}
	}
}
