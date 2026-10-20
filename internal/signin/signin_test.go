package signin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/policy"
)

const fakePass = "unit-Passw0rd-3Hh8-not-real"

func target(t *testing.T) policy.Target {
	t.Helper()
	tg := policy.Target{Adapter: "url", Environment: "t", BaseURL: "http://127.0.0.1:9/", AllowedHosts: []string{"127.0.0.1"}, ReadOnly: true,
		Signin: &policy.Signin{LoginURL: "/signin", Username: policy.Field{Selector: "#u"}, Password: policy.Field{Selector: "#p"},
			Submit: policy.Field{Label: "Sign in"}, AllowRequest: policy.AllowedRequest{URL: "/signin"},
			Success: policy.Success{Selector: "#out"}, Dir: "/cfg",
			Credentials: map[string]policy.Credential{
				"admin":  {UsernameEnv: "A_USER", PasswordEnv: "A_PASS"},
				"editor": {SessionFile: "editor.json"},
			}}}
	if err := tg.Validate(); err != nil {
		t.Fatal(err)
	}
	return tg
}

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestResolve(t *testing.T) {
	tg := target(t)
	p, err := Resolve(tg, "ADMIN", "", env(map[string]string{"A_USER": "ann", "A_PASS": fakePass}))
	if err != nil || p.Method != MethodForm || p.Role != "admin" || p.Creds.Password != fakePass {
		t.Fatalf("form plan %+v %v", p, err)
	}
	if p, err = Resolve(tg, "editor", "", env(nil)); err != nil || p.Method != MethodSession || p.SessionPath != "/cfg/editor.json" {
		t.Fatalf("session plan %+v %v", p, err)
	}
	if p, err = Resolve(tg, "admin", "/tmp/s.json", env(nil)); err != nil || p.Method != MethodSession || p.SessionPath != "/tmp/s.json" {
		t.Fatalf("flag plan %+v %v", p, err)
	}
	if _, err = ResolveForm(tg, "editor", env(nil)); err == nil {
		t.Error("ResolveForm used a role that has only a session file")
	}
	_, err = Resolve(tg, "admin", "", env(map[string]string{"A_USER": "ann"}))
	if err == nil || !strings.Contains(err.Error(), "A_PASS") || strings.Contains(err.Error(), "ann") {
		t.Errorf("missing variable error %v", err)
	}
	_, err = Resolve(tg, "root", "", env(nil))
	if err == nil || !strings.Contains(err.Error(), "admin, editor") {
		t.Errorf("unknown role error %v", err)
	}
	if _, err = Resolve(policy.Target{}, "admin", "", env(nil)); err == nil {
		t.Error("a target with no signin block resolved")
	}
}

func TestScrub(t *testing.T) {
	got := Scrub("bad login for ann with "+fakePass+" twice "+fakePass, "ann", fakePass, "x")
	if strings.Contains(got, fakePass) || strings.Contains(got, "ann") || !strings.Contains(got, "bad login") {
		t.Errorf("scrubbed: %q", got)
	}
}

func TestSessionRoundTripAndMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deep", "s.json")
	s := &Session{Cookies: []SessionCookie{{Name: "sid", Value: "cookie-value-1", Domain: "127.0.0.1", Path: "/", Expires: -1, HTTPOnly: true, SameSite: "Lax"}},
		Origins: []SessionOrigin{{Origin: "http://127.0.0.1:9", LocalStorage: []StorageItem{{Name: "k", Value: "v"}}}}}
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(path); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v %v", st, err)
	}
	back, err := LoadSession(path)
	if err != nil || back.Cookies[0].Value != "cookie-value-1" || back.Origins[0].LocalStorage[0].Name != "k" {
		t.Fatalf("round trip %+v %v", back, err)
	}
	// The file is Playwright's storageState: the same keys.
	raw, _ := os.ReadFile(path) //nolint:gosec // the test's own temp file
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil || generic["cookies"] == nil || generic["origins"] == nil {
		t.Errorf("not storageState: %s", raw)
	}
}

func TestLoadSessionErrorsDoNotQuoteTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte(`{"cookies": [{"name":"sid","value":"secret-cookie-value" BROKEN`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadSession(path)
	if err == nil || strings.Contains(err.Error(), "secret-cookie-value") {
		t.Errorf("error %v", err)
	}
	empty := filepath.Join(t.TempDir(), "empty.json")
	_ = os.WriteFile(empty, []byte(`{"cookies":[],"origins":[]}`), 0o600)
	if _, err := LoadSession(empty); err == nil {
		t.Error("an empty session loaded")
	}
	if _, err := LoadSession(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("a missing file loaded")
	}
}

func TestCookieConversion(t *testing.T) {
	exp := time.Now().Add(time.Hour).Unix()
	c := SessionCookie{Name: "a", Value: "b", Domain: "x.test", Expires: float64(exp), SameSite: "Strict", Secure: true}
	b := c.toBiDi()
	if b.SameSite != "strict" || b.Path != "/" || b.Expiry == nil || *b.Expiry != exp || b.Value.Type != "string" {
		t.Errorf("to bidi %+v", b)
	}
	back := fromBiDi(b)
	if back.SameSite != "Strict" || back.Expires != float64(exp) || !back.Secure {
		t.Errorf("from bidi %+v", back)
	}
	if got := (SessionCookie{Expires: -1}).toBiDi(); got.Expiry != nil || got.SameSite != "lax" {
		t.Errorf("session cookie %+v", got)
	}
}
