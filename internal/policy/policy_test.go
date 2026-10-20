package policy

import "testing"

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
		{"default https port", ro, "GET", "https://www.example.test:443/", Allow},
		{"other port on an allowed host", ro, "GET", "https://www.example.test:8443/", BlockHost},
		{"plain http default port", ro, "GET", "http://www.example.test/", Allow},
		{"websocket on another port", ro, "GET", "wss://www.example.test:9000/socket", BlockHost},
		{"file url", ro, "GET", "file:///etc/passwd", BlockHost},
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
	for v, s := range map[Verdict]string{Allow: "allow", BlockHost: "host not allowed", BlockWrite: "read-only target", Verdict(9): "unknown"} {
		if v.String() != s {
			t.Errorf("%d.String() = %q", v, v.String())
		}
	}
}

func TestOriginAllowedPorts(t *testing.T) {
	tg := Target{Adapter: "url", Environment: "local", BaseURL: "http://127.0.0.1:8080/",
		AllowedHosts: []string{"127.0.0.1", "api.local:4000", "other.local"}}
	if err := tg.Validate(); err != nil {
		t.Fatal(err)
	}
	for url, want := range map[string]Verdict{
		"http://127.0.0.1:8080/x":    Allow,
		"http://127.0.0.1:9090/x":    BlockHost,
		"http://127.0.0.1/x":         Allow,
		"http://api.local:4000/x":    Allow,
		"http://api.local:4001/x":    BlockHost,
		"http://api.local/x":         BlockHost,
		"http://other.local:8080/x":  BlockHost,
		"https://other.local/x":      Allow,
		"https://evil.example/x":     BlockHost,
		"http://127.0.0.1.evil.test": BlockHost,
	} {
		if got := tg.Decide("GET", url); got != want {
			t.Errorf("Decide(GET %s) = %s, want %s", url, got, want)
		}
	}
}
