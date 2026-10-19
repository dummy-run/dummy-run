package policy

import (
	"testing"

	"github.com/dummy-run/dummy-run/internal/robots"
)

func target(readOnly bool) Target {
	return Target{Adapter: "url", Environment: "production", BaseURL: "https://www.example.test/",
		AllowedHosts: []string{"example.test", "www.example.test"}, ReadOnly: readOnly}
}

func TestValidate(t *testing.T) {
	if err := target(true).Validate(); err != nil {
		t.Fatalf("valid target rejected: %v", err)
	}
	bad := []Target{
		{Adapter: "vercel", Environment: "x", BaseURL: "https://a.test", AllowedHosts: []string{"a.test"}},
		{Adapter: "url", BaseURL: "https://a.test", AllowedHosts: []string{"a.test"}},
		{Adapter: "url", Environment: "x", BaseURL: "a.test", AllowedHosts: []string{"a.test"}},
		{Adapter: "url", Environment: "x", BaseURL: "http://a.test", AllowedHosts: []string{"a.test"}},
		{Adapter: "url", Environment: "x", BaseURL: "https://a.test"},
		{Adapter: "url", Environment: "x", BaseURL: "https://a.test", AllowedHosts: []string{"b.test"}},
	}
	for i, b := range bad {
		if err := b.Validate(); err == nil {
			t.Errorf("case %d accepted: %+v", i, b)
		}
	}
	local := Target{Adapter: "url", Environment: "local", BaseURL: "http://127.0.0.1:8080/", AllowedHosts: []string{"127.0.0.1"}}
	if err := local.Validate(); err != nil {
		t.Errorf("localhost over http rejected: %v", err)
	}
}

func TestDecide(t *testing.T) {
	ro := target(true)
	rw := target(false)
	cases := []struct {
		name   string
		t      Target
		method string
		url    string
		want   Verdict
	}{
		{"get allowed host", ro, "GET", "https://www.example.test/page", Allow},
		{"head allowed host", ro, "HEAD", "https://example.test/", Allow},
		{"options allowed host", ro, "OPTIONS", "https://example.test/api", Allow},
		{"post read only", ro, "POST", "https://www.example.test/subscribe", BlockWrite},
		{"put read only", ro, "PUT", "https://www.example.test/x", BlockWrite},
		{"delete read only", ro, "DELETE", "https://www.example.test/x", BlockWrite},
		{"lower case post read only", ro, "post", "https://www.example.test/x", BlockWrite},
		{"post read write", rw, "POST", "https://www.example.test/subscribe", Allow},
		{"other host", ro, "GET", "https://cdn.other.test/a.js", BlockHost},
		{"other host write", rw, "POST", "https://tracker.test/collect", BlockHost},
		{"subdomain is not the host", ro, "GET", "https://api.example.test/", BlockHost},
		{"case insensitive host", ro, "GET", "https://WWW.Example.TEST/", Allow},
		{"data url", ro, "GET", "data:text/plain,hi", Allow},
		{"garbage url", ro, "GET", "::not a url", BlockHost},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.t.Decide(tc.method, tc.url); got != tc.want {
				t.Errorf("Decide(%s %s) = %s, want %s", tc.method, tc.url, got, tc.want)
			}
		})
	}
}

func TestVerdictString(t *testing.T) {
	for v, s := range map[Verdict]string{BlockRobots: "robots.txt disallows DummyRun", Allow: "allow", BlockHost: "host not allowed", BlockWrite: "read-only target", Verdict(9): "unknown"} {
		if v.String() != s {
			t.Errorf("%d.String() = %q", v, v.String())
		}
	}
}

func TestDecideRobots(t *testing.T) {
	tg := target(true)
	tg.Robots = robots.Parse([]byte("User-agent: DummyRun\nDisallow: /admin\nDisallow: /*.pdf$\n"), "DummyRun")
	cases := []struct {
		name, method, url string
		want              Verdict
	}{
		{"disallowed path", "GET", "https://www.example.test/admin/users", BlockRobots},
		{"disallowed with query", "GET", "https://www.example.test/admin?x=1", BlockRobots},
		{"pattern", "GET", "https://www.example.test/files/a.pdf", BlockRobots},
		{"allowed path", "GET", "https://www.example.test/blog", Allow},
		{"robots.txt itself", "GET", "https://www.example.test/robots.txt", Allow},
		{"other allowed host is not covered", "GET", "https://example.test/admin", Allow},
		{"other scheme is not covered", "GET", "http://www.example.test/admin", Allow},
		{"host block comes first", "GET", "https://cdn.other.test/admin", BlockHost},
		{"write to a disallowed path reports robots", "POST", "https://www.example.test/admin", BlockRobots},
		{"write to an allowed path", "POST", "https://www.example.test/blog", BlockWrite},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tg.Decide(tc.method, tc.url); got != tc.want {
				t.Errorf("Decide(%s %s) = %s, want %s", tc.method, tc.url, got, tc.want)
			}
		})
	}
	tg.Robots = nil
	if tg.Decide("GET", "https://www.example.test/admin") != Allow {
		t.Error("no rules should allow")
	}
}
