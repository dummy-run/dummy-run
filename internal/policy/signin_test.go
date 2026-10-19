package policy

import (
	"encoding/json"
	"strings"
	"testing"
)

const signinTarget = `{
  "adapter": "url", "environment": "test", "base_url": "https://app.example.test/", "allowed_hosts": ["app.example.test"], "read_only": true,
  "signin": {
    "login_url": "/signin",
    "username": {"selector": "#email"}, "password": {"label": "Password"}, "submit": {"label": "Sign in"},
    "allow_request": {"url": "/signin"},
    "success": {"selector": "#signout"},
    "block_paths": ["/logout"],
    "credentials": {"admin": {"username_env": "APP_ADMIN_USER", "password_env": "APP_ADMIN_PASS"}}
  }
}`

func signinTargetWith(t *testing.T, edit func(map[string]any)) Target {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(signinTarget), &m); err != nil {
		t.Fatal(err)
	}
	edit(m["signin"].(map[string]any))
	b, _ := json.Marshal(m)
	var tg Target
	if err := json.Unmarshal(b, &tg); err != nil {
		t.Fatal(err)
	}
	return tg
}

func TestSigninValidate(t *testing.T) {
	if err := signinTargetWith(t, func(map[string]any) {}).Validate(); err != nil {
		t.Fatalf("valid sign-in rejected: %v", err)
	}
	bad := map[string]func(map[string]any){
		"no login url":        func(s map[string]any) { delete(s, "login_url") },
		"login host outside":  func(s map[string]any) { s["login_url"] = "https://other.test/signin" },
		"post host outside":   func(s map[string]any) { s["allow_request"] = map[string]any{"url": "https://other.test/signin"} },
		"field with both":     func(s map[string]any) { s["username"] = map[string]any{"selector": "#a", "label": "A"} },
		"field with neither":  func(s map[string]any) { s["password"] = map[string]any{} },
		"safe method":         func(s map[string]any) { s["allow_request"] = map[string]any{"method": "GET", "url": "/signin"} },
		"no success check":    func(s map[string]any) { delete(s, "success") },
		"bad success pattern": func(s map[string]any) { s["success"] = map[string]any{"url_matches": "("} },
		"no credentials":      func(s map[string]any) { s["credentials"] = map[string]any{} },
		"value as env name": func(s map[string]any) {
			s["credentials"] = map[string]any{"a": map[string]any{"username_env": "me@x.test", "password_env": "hunter2!"}}
		},
		"relative block path": func(s map[string]any) { s["block_paths"] = []string{"logout"} },
	}
	for name, edit := range bad {
		if err := signinTargetWith(t, edit).Validate(); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestSigninBlocksSignoutPaths(t *testing.T) {
	tg := signinTargetWith(t, func(map[string]any) {})
	for _, m := range []string{"GET", "POST"} {
		if v := tg.Decide(m, "https://app.example.test/logout/"); v != BlockSignout {
			t.Errorf("%s /logout: %v", m, v)
		}
	}
	if v := tg.Decide("GET", "https://app.example.test/logouts"); v != Allow {
		t.Errorf("a different path was blocked: %v", v)
	}
}

func TestGateAllowsExactlyTheLoginPost(t *testing.T) {
	tg := signinTargetWith(t, func(map[string]any) {})
	g := NewGate(tg)
	const login = "https://app.example.test/signin"
	if g.Take("POST", login) {
		t.Fatal("a closed gate let a request through")
	}
	g.Open()
	for _, c := range []struct{ method, url string }{
		{"GET", login}, {"PUT", login}, {"POST", "https://app.example.test/signin/other"},
		{"POST", "https://app.example.test/drafts/1/delete"}, {"POST", "https://app.example.test:8443/signin"},
		{"POST", "http://app.example.test/signin"},
	} {
		if g.Take(c.method, c.url) {
			t.Errorf("%s %s taken", c.method, c.url)
		}
	}
	if _, _, ok := g.Used(); ok {
		t.Error("gate used by non-matching requests")
	}
	if !g.Take("post", login+"?next=%2Fdrafts") {
		t.Fatal("the sign-in POST was refused")
	}
	if g.Take("POST", login) {
		t.Error("a second sign-in POST was let through")
	}
	m, u, ok := g.Used()
	if !ok || m != "POST" || u != login {
		t.Errorf("used %s %s %v", m, u, ok)
	}
	g.Close()
	// After sign-in, the strict policy decides again.
	if v := tg.Decide("POST", login); v != BlockWrite {
		t.Errorf("POST after sign-in: %v", v)
	}
}

func TestGateWithoutSignin(t *testing.T) {
	g := NewGate(target(true))
	g.Open()
	if g.Take("POST", "https://www.example.test/signin") {
		t.Error("a target with no sign-in opened an exception")
	}
}

func TestSessionPath(t *testing.T) {
	s := &Signin{Dir: "/etc/targets"}
	if got := s.SessionPath(Credential{SessionFile: "s/admin.json"}); got != "/etc/targets/s/admin.json" {
		t.Errorf("relative: %s", got)
	}
	if got := s.SessionPath(Credential{SessionFile: "/var/admin.json"}); got != "/var/admin.json" {
		t.Errorf("absolute: %s", got)
	}
	if !strings.EqualFold("ADMIN", "admin") {
		t.Fatal()
	}
	if _, name, ok := signinTargetWith(t, func(map[string]any) {}).Signin.Role("Admin"); !ok || name != "admin" {
		t.Error("role lookup is case sensitive")
	}
}
