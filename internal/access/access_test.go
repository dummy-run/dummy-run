package access_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/dummy-run/dummy-run/internal/access"
	"github.com/dummy-run/dummy-run/internal/access/accesstest"
	"github.com/dummy-run/dummy-run/internal/bidi"
	"github.com/dummy-run/dummy-run/internal/policy"
)

func target(t *testing.T) policy.Target {
	t.Helper()
	return policy.Target{Environment: "p", BaseURL: "https://preview.example.com/",
		AllowedHosts: []string{"preview.example.com", "assets.example.com:8443"}}
}

func resolve(t *testing.T, a *policy.Auth) *access.Resolved {
	t.Helper()
	r, err := access.Resolve(a, accesstest.Getenv, access.TargetScope(target(t)))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func names(hs []bidi.Header) string {
	var n []string
	for _, h := range hs {
		n = append(n, h.Name)
	}
	return strings.Join(n, ",")
}

func TestHeadersAreScopedToTheTarget(t *testing.T) {
	r := resolve(t, accesstest.Auth())
	for url, want := range map[string]string{
		"https://preview.example.com/page":               "x-vercel-protection-bypass,Authorization",
		"https://PREVIEW.example.com./page":              "x-vercel-protection-bypass,Authorization",
		"https://assets.example.com:8443/a.js":           "x-vercel-protection-bypass,Authorization",
		"https://assets.example.com/a.js":                "", // another port is another origin
		"https://other.example.com/":                     "",
		"https://preview.example.com.evil.test/":         "",
		"https://evil.test/https://preview.example.com/": "",
		"data:text/html,hi":                              "",
		"about:blank":                                    "",
		"not a url":                                      "",
	} {
		if got := names(r.Headers(url, nil)); got != want {
			t.Errorf("%s: headers %q, want %q", url, got, want)
		}
	}
}

// A cleartext request would show the password and the bypass token to anyone on the network.
func TestNoCredentialsOverCleartext(t *testing.T) {
	r := resolve(t, accesstest.Auth())
	for _, url := range []string{"http://preview.example.com/page", "http://assets.example.com:8443/a.js", "ws://preview.example.com/socket"} {
		if got := r.Headers(url, nil); got != nil {
			t.Errorf("%s: sent %s over cleartext", url, names(got))
		}
		if _, _, ok := r.Challenge("c", url); ok {
			t.Errorf("%s: answered a challenge over cleartext", url)
		}
	}
	local := policy.Target{Environment: "p", BaseURL: "http://127.0.0.1:8080/", AllowedHosts: []string{"127.0.0.1:8080", "localhost:3000"}}
	lr, err := access.Resolve(accesstest.Auth(), accesstest.Getenv, access.TargetScope(local))
	if err != nil {
		t.Fatal(err)
	}
	for _, url := range []string{"http://127.0.0.1:8080/", "http://localhost:3000/x"} {
		if got := lr.Headers(url, nil); got == nil {
			t.Errorf("%s: loopback over http should get the headers", url)
		}
	}
	scope := access.OriginScope([]string{"http://staging.example.com/", "https://staging.example.com/", "http://127.0.0.1:9000/"})
	if scope("http://staging.example.com/a") || !scope("https://staging.example.com/a") || !scope("http://127.0.0.1:9000/a") {
		t.Error("OriginScope must refuse cleartext off loopback and keep https and loopback")
	}
}

func TestPagesOwnAuthorizationIsKept(t *testing.T) {
	r := resolve(t, accesstest.Auth())
	own := []bidi.Header{bidi.StringHeader("authorization", "Bearer app-token")}
	if got := names(r.Headers("https://preview.example.com/api", own)); got != "x-vercel-protection-bypass" {
		t.Errorf("headers %q: the page's own Authorization must not be replaced", got)
	}
}

func TestChallengeScopeAndRetry(t *testing.T) {
	r := resolve(t, accesstest.Auth())
	if _, _, ok := r.Challenge("1", "https://other.example.com/"); ok {
		t.Error("answered a challenge from another host")
	}
	u, p, ok := r.Challenge("2", "https://preview.example.com/basic")
	if !ok || u != accesstest.User || p != accesstest.Pass {
		t.Errorf("first challenge not answered")
	}
	if _, _, ok := r.Challenge("2", "https://preview.example.com/basic"); ok {
		t.Error("answered the same request twice; wrong credentials would loop")
	}
	hdrOnly := resolve(t, &policy.Auth{Headers: accesstest.Auth().Headers})
	if _, _, ok := hdrOnly.Challenge("3", "https://preview.example.com/"); ok {
		t.Error("answered a challenge without Basic credentials")
	}
}

func TestChallengeOnlySendsNoPreemptiveHeader(t *testing.T) {
	r := resolve(t, accesstest.Auth()).ChallengeOnly()
	if got := names(r.Headers("https://preview.example.com/", nil)); got != "x-vercel-protection-bypass" {
		t.Errorf("headers %q", got)
	}
}

func TestResolveErrorsNameNoSecret(t *testing.T) {
	getenv := func(n string) string {
		if n == "PREVIEW_USER" {
			return "u"
		}
		return ""
	}
	_, err := access.Resolve(accesstest.Auth(), getenv, access.TargetScope(target(t)))
	if err == nil || !strings.Contains(err.Error(), "PREVIEW_PASS") {
		t.Errorf("a missing variable should be named: %v", err)
	}
	// A lower-case "name" may be a secret typed where a name belongs, so it is not echoed.
	a := &policy.Auth{Headers: map[string]policy.HeaderSource{"X-Token": {ValueEnv: "hunter2"}}}
	_, err = access.Resolve(a, func(string) string { return "" }, access.TargetScope(target(t)))
	if err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Errorf("error %v", err)
	}
	for _, bad := range []string{"line\nbreak", "nul\x00"} {
		_, err = access.Resolve(&policy.Auth{Headers: accesstest.Auth().Headers}, func(string) string { return bad }, access.TargetScope(target(t)))
		if err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	_, err = access.Resolve(accesstest.Auth(), func(n string) string {
		if n == "PREVIEW_USER" {
			return "a:b"
		}
		return "x"
	}, access.TargetScope(target(t)))
	if err == nil {
		t.Error("accepted a user name with a colon")
	}
}

func TestSummaryHoldsNamesOnly(t *testing.T) {
	r := resolve(t, accesstest.Auth())
	b, err := json.Marshal(r.Summary())
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"methods":["basic","headers"],"header_names":["x-vercel-protection-bypass"]}` {
		t.Errorf("summary %s", b)
	}
	for _, s := range accesstest.Secrets() {
		if strings.Contains(string(b), s) {
			t.Error("the summary holds a secret")
		}
	}
	var none *access.Resolved
	if none.Active() || none.HasBasic() || none.Headers("https://preview.example.com/", nil) != nil || len(none.Summary().Methods) != 0 {
		t.Error("a nil Resolved must do nothing")
	}
}

// The engine's own HTTP requests carry the access on the target and drop it on a redirect to another host.
func TestWrapClientDoesNotFollowCredentialsToAnotherHost(t *testing.T) {
	fx := accesstest.New(t)
	scope := access.OriginScope([]string{fx.Preview.URL})
	r, err := access.Resolve(accesstest.Auth(), accesstest.Getenv, scope)
	if err != nil {
		t.Fatal(err)
	}
	c := access.WrapClient(nil, r)
	for _, path := range []string{"/both", "/redirect"} {
		resp, err := c.Get(fx.Preview.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if path == "/both" && resp.StatusCode != http.StatusOK {
			t.Errorf("/both status %d", resp.StatusCode)
		}
	}
	got := fx.Elsewhere.For("/landing")
	if len(got) != 1 {
		t.Fatalf("the redirect reached the other host %d times", len(got))
	}
	for _, name := range []string{"Authorization", accesstest.BypassHeader} {
		if v := got[0].Header.Get(name); v != "" {
			t.Errorf("the other host received %s", name)
		}
	}
	if access.WrapClient(nil, nil) != nil {
		t.Error("with nothing to send the client is returned as it is")
	}
}
