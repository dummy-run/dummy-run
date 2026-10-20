package main

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

type env struct {
	t   *testing.T
	srv *httptest.Server
	s   *Server
}

func newEnv(t *testing.T, m *Manifest) *env {
	t.Helper()
	s := NewServer(m, true)
	s.sleep = func(time.Duration) {}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return &env{t: t, srv: ts, s: s}
}

func manifestOf(sw ...string) *Manifest {
	m := &Manifest{Version: 1}
	for _, k := range sw {
		m.Defects = append(m.Defects, Defect{ID: "T-" + k, Switch: k})
	}
	return m
}

// client returns an HTTP client signed in as the first user with the role, or
// anonymous for "". It does not follow redirects.
func (e *env) client(role string) *http.Client {
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if role != "" {
		resp, err := c.Get(e.srv.URL + "/test-login?as=" + role)
		if err != nil {
			e.t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			e.t.Fatalf("test-login as %s: %d", role, resp.StatusCode)
		}
	}
	return c
}

func (e *env) do(c *http.Client, method, path string, form url.Values) (int, string) {
	e.t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func allowed(code int) bool { return code < 400 }

type step struct {
	role, method, path string
	form               url.Values
	want               bool // true: the action is allowed
}

func f(kv ...string) url.Values {
	v := url.Values{}
	for i := 0; i < len(kv); i += 2 {
		v.Set(kv[i], kv[i+1])
	}
	return v
}

// The documented policy, one row per protected function and role. The client for
// "author" is Sam, who owns drafts 1, 3 and 6. Lin owns 2, 4 and 7. Ada owns 5.
var policy = []step{
	// draft.create
	{"viewer", "GET", "/drafts/new", nil, false},
	{"author", "GET", "/drafts/new", nil, true},
	{"reviewer", "GET", "/drafts/new", nil, false},
	// draft.edit
	{"viewer", "GET", "/drafts/1/edit", nil, false},
	{"viewer", "POST", "/drafts/1/edit", f("title", "Changed title", "body", "x"), false},
	{"author", "GET", "/drafts/1/edit", nil, true},
	{"author", "GET", "/drafts/2/edit", nil, false},
	{"admin", "GET", "/drafts/2/edit", nil, true},
	{"admin", "GET", "/drafts/4/edit", nil, false},
	// draft.review: draft 2 is in review
	{"author", "POST", "/drafts/2/review", f("verdict", "approve"), false},
	{"reviewer", "POST", "/drafts/2/review", f("verdict", "approve"), true},
	// draft.publish: draft 3 is approved (Sam's), draft 5 is approved (Ada's)
	{"viewer", "POST", "/drafts/3/publish", nil, false},
	{"reviewer", "POST", "/drafts/3/publish", nil, false},
	{"editor", "POST", "/drafts/5/publish", nil, false},
	{"editor", "POST", "/drafts/3/publish", nil, true},
	// draft.delete
	{"editor", "POST", "/drafts/1/delete", nil, false},
	{"author", "POST", "/drafts/1/delete", nil, false},
	{"admin", "POST", "/drafts/1/delete", nil, true},
	// reviews.list
	{"viewer", "GET", "/reviews", nil, false},
	{"author", "GET", "/reviews", nil, false},
	{"reviewer", "GET", "/reviews", nil, true},
	{"editor", "GET", "/reviews", nil, true},
	// issue.create
	{"author", "POST", "/issues", f("title", "Spring issue", "date", "2099-01-01"), false},
	{"editor", "POST", "/issues", f("title", "Spring issue", "date", "2099-01-01"), true},
	// payments.view
	{"viewer", "GET", "/payments", nil, false},
	{"reviewer", "GET", "/payments", nil, false},
	{"author", "GET", "/payments", nil, true},
	{"editor", "GET", "/payments", nil, true},
	// payment.approve
	{"editor", "POST", "/payments/3/approve", nil, false},
	{"admin", "POST", "/payments/3/approve", nil, true},
	// users.manage
	{"editor", "GET", "/admin/users", nil, false},
	{"admin", "GET", "/admin/users", nil, true},
	{"editor", "POST", "/admin/users/sam/role", f("role", "admin"), false},
	{"admin", "POST", "/admin/users/sam/role", f("role", "editor"), true},
	// settings.change
	{"editor", "GET", "/settings", nil, false},
	{"editor", "POST", "/settings", f("site_name", "Hacked"), false},
	{"admin", "POST", "/settings", f("site_name", "Inkwell Press"), true},
}

func runPolicy(t *testing.T, m *Manifest, rows []step) {
	t.Helper()
	for _, r := range rows {
		e := newEnv(t, m) // fresh state for every row
		code, _ := e.do(e.client(r.role), r.method, r.path, r.form)
		if allowed(code) != r.want {
			t.Errorf("%s %s %s: status %d, allowed should be %v", r.role, r.method, r.path, code, r.want)
		}
	}
}

func TestPolicyWithNoDefects(t *testing.T) {
	runPolicy(t, nil, policy)
}

func TestAnonymousIsSentToSignIn(t *testing.T) {
	e := newEnv(t, nil)
	c := e.client("")
	for _, p := range []string{"/drafts", "/payments", "/settings"} {
		code, _ := e.do(c, "GET", p, nil)
		if code != http.StatusSeeOther {
			t.Errorf("%s: want redirect, got %d", p, code)
		}
	}
	for _, p := range []string{"/", "/help", "/signin"} {
		if code, _ := e.do(c, "GET", p, nil); code != 200 {
			t.Errorf("%s: want 200, got %d", p, code)
		}
	}
}

// Each permission switch must open exactly the hole it names.
func TestPermissionSwitches(t *testing.T) {
	cases := []struct {
		sw  string
		row step
	}{
		{"perm-viewer-edit", step{"viewer", "POST", "/drafts/1/edit", f("title", "Changed title", "body", "x"), true}},
		{"perm-editor-self-publish", step{"editor", "POST", "/drafts/5/publish", nil, true}},
		{"perm-reviewer-payments", step{"reviewer", "GET", "/payments", nil, true}},
		{"perm-author-edit-others", step{"author", "GET", "/drafts/2/edit", nil, true}},
		{"perm-editor-users", step{"editor", "GET", "/admin/users", nil, true}},
		{"perm-editor-settings", step{"editor", "GET", "/settings", nil, true}},
		{"perm-editor-settings", step{"editor", "POST", "/settings", f("site_name", "Hacked"), true}},
		{"perm-editor-delete", step{"editor", "POST", "/drafts/1/delete", nil, true}},
		{"perm-reviewer-denied", step{"reviewer", "GET", "/reviews", nil, false}},
	}
	for _, c := range cases {
		t.Run(c.sw, func(t *testing.T) {
			runPolicy(t, manifestOf(c.sw), []step{c.row})
			opposite := c.row
			opposite.want = !c.row.want
			runPolicy(t, nil, []step{opposite})
		})
	}
	t.Run("perm-owner-self-review", func(t *testing.T) {
		for _, on := range []bool{false, true} {
			var m *Manifest
			if on {
				m = manifestOf("perm-owner-self-review")
			}
			e := newEnv(t, m)
			c := e.client("author")
			e.do(c, "POST", "/drafts/1/submit", nil)
			code, _ := e.do(c, "POST", "/drafts/1/review", f("verdict", "approve"))
			if allowed(code) != on {
				t.Errorf("switch %v: review status %d", on, code)
			}
		}
	})
}

func withSwitch(on bool, sw string) *Manifest {
	if on {
		return manifestOf(sw)
	}
	return nil
}

func TestBugSwitches(t *testing.T) {
	t.Run("signin silent", func(t *testing.T) {
		for _, on := range []bool{false, true} {
			e := newEnv(t, withSwitch(on, "bug-signin-silent"))
			code, body := e.do(e.client(""), "POST", "/signin", f("email", "sam@inkwell.test", "password", "wrong"))
			if on && code != http.StatusSeeOther {
				t.Errorf("on: want redirect, got %d", code)
			}
			if !on && (code != 401 || !strings.Contains(body, "not right")) {
				t.Errorf("off: want 401 with a message, got %d", code)
			}
		}
	})
	t.Run("edit title lost", func(t *testing.T) {
		for _, on := range []bool{false, true} {
			e := newEnv(t, withSwitch(on, "bug-edit-title-lost"))
			c := e.client("author")
			e.do(c, "POST", "/drafts/1/edit", f("title", "A new title", "body", "new body"))
			_, body := e.do(c, "GET", "/drafts/1", nil)
			if got := strings.Contains(body, "A new title"); got == on {
				t.Errorf("switch %v: title saved = %v", on, got)
			}
			if !strings.Contains(body, "new body") {
				t.Errorf("body should always be saved")
			}
		}
	})
	t.Run("issue in the past", func(t *testing.T) {
		for _, on := range []bool{false, true} {
			e := newEnv(t, withSwitch(on, "bug-issue-past-500"))
			code, _ := e.do(e.client("editor"), "POST", "/issues", f("title", "Old issue", "date", "2001-01-01"))
			want := 422
			if on {
				want = 500
			}
			if code != want {
				t.Errorf("switch %v: status %d, want %d", on, code, want)
			}
		}
	})
	t.Run("submit does nothing", func(t *testing.T) {
		for _, on := range []bool{false, true} {
			e := newEnv(t, withSwitch(on, "bug-submit-noop"))
			c := e.client("author")
			e.do(c, "POST", "/drafts/1/submit", nil)
			_, body := e.do(c, "GET", "/drafts/1", nil)
			if got := strings.Contains(body, "in_review"); got == on {
				t.Errorf("switch %v: in review = %v", on, got)
			}
		}
	})
	t.Run("search ignored", func(t *testing.T) {
		for _, on := range []bool{false, true} {
			e := newEnv(t, withSwitch(on, "bug-search-ignored"))
			_, body := e.do(e.client("author"), "GET", "/drafts?q=harbour", nil)
			if got := strings.Contains(body, "Spring reading list"); got != on {
				t.Errorf("switch %v: unrelated row shown = %v", on, got)
			}
		}
	})
	t.Run("next loops", func(t *testing.T) {
		for _, on := range []bool{false, true} {
			e := newEnv(t, withSwitch(on, "bug-page-next-loop"))
			_, body := e.do(e.client("author"), "GET", "/drafts", nil)
			want := `href="/drafts?page=2"`
			if on {
				want = `href="/drafts?page=1"`
			}
			if !strings.Contains(body, want) {
				t.Errorf("switch %v: Next link %s not found", on, want)
			}
		}
	})
	t.Run("payments total", func(t *testing.T) {
		for _, on := range []bool{false, true} {
			e := newEnv(t, withSwitch(on, "bug-payments-total"))
			_, body := e.do(e.client("editor"), "GET", "/payments", nil)
			want := "£335.00"
			if on {
				want = "£290.00"
			}
			if !strings.Contains(body, want) {
				t.Errorf("switch %v: total %s not found", on, want)
			}
		}
	})
	t.Run("stale status", func(t *testing.T) {
		e := newEnv(t, manifestOf("bug-list-stale-status"))
		_, body := e.do(e.client("author"), "GET", "/drafts?page=2", nil)
		if strings.Contains(body, ">published<") {
			t.Error("published draft should show as approved")
		}
	})
}

func TestPageFaultsAppearOnlyWhenSwitchedOn(t *testing.T) {
	cases := []struct {
		sw, role, path, marker string
	}{
		{"a11y-edit-title-nolabel", "author", "/drafts/1/edit", `placeholder="Title"`},
		{"a11y-signin-pw-nolabel", "", "/signin", `placeholder="Password"`},
		{"a11y-status-contrast", "author", "/drafts", "color:#b5b5b5"},
		{"a11y-delete-noname", "admin", "/drafts", `<svg width="16"`},
		{"a11y-help-dialog-trap", "author", "/drafts", "e.key==='Tab'"},
		{"a11y-payments-nozoom", "author", "/payments", "user-scalable=no"},
		{"a11y-focus-hidden", "author", "/drafts", "outline:none"},
		{"bug-dead-link", "", "/help", `href="/style-guide"`},
		{"fric-feedback-covers-save", "author", "/drafts/1/edit", `class="feedback"`},
		{"fric-banner-covers-publish", "author", "/drafts/1", `class="notice"`},
		{"fric-signout-hover", "author", "/drafts", `class="menu"`},
		{"fric-jargon-slots", "author", "/issues", "Embargo slots"},
		{"fric-payments-wide", "author", "/payments", "min-width:900px"},
	}
	for _, c := range cases {
		t.Run(c.sw, func(t *testing.T) {
			for _, on := range []bool{false, true} {
				e := newEnv(t, withSwitch(on, c.sw))
				_, body := e.do(e.client(c.role), "GET", c.path, nil)
				// The stylesheet carries the markers only when the switch is on.
				if got := strings.Contains(body, c.marker); got != on {
					t.Errorf("switch %v: marker %q present = %v", on, c.marker, got)
				}
			}
		})
	}
	t.Run("issue image alt", func(t *testing.T) {
		e := newEnv(t, nil)
		_, body := e.do(e.client("author"), "GET", "/issues", nil)
		if !strings.Contains(body, `alt="Cover of Autumn issue"`) {
			t.Error("baseline cover image needs alt text")
		}
		e = newEnv(t, manifestOf("a11y-issue-img-noalt"))
		_, body = e.do(e.client("author"), "GET", "/issues", nil)
		if strings.Contains(body, "alt=") {
			t.Error("planted fault should drop alt")
		}
	})
	t.Run("reviews title", func(t *testing.T) {
		e := newEnv(t, manifestOf("a11y-reviews-notitle"))
		_, body := e.do(e.client("editor"), "GET", "/reviews", nil)
		if !strings.Contains(body, "<title></title>") {
			t.Error("title should be empty")
		}
	})
}

func TestVagueErrorAndDateFormat(t *testing.T) {
	e := newEnv(t, manifestOf("fric-error-vague", "fric-date-format"))
	c := e.client("author")
	_, body := e.do(c, "POST", "/drafts/new", f("title", "", "body", "x"))
	if !strings.Contains(body, "Invalid.") {
		t.Error("vague error missing")
	}
	c = e.client("editor")
	code, body := e.do(c, "POST", "/issues", f("title", "Spring issue", "date", "2099-01-01"))
	if code != 422 || !strings.Contains(body, "Invalid date.") {
		t.Errorf("date in ISO form should fail with the vague message, got %d", code)
	}
	if code, _ := e.do(c, "POST", "/issues", f("title", "Spring issue", "date", "01-01-2099")); code != 303 {
		t.Errorf("DD-MM-YYYY should work, got %d", code)
	}
}

func TestSlowReviewsAndShortSession(t *testing.T) {
	e := newEnv(t, manifestOf("fric-reviews-slow", "fric-session-short"))
	var slept time.Duration
	e.s.sleep = func(d time.Duration) { slept += d }
	now := time.Now()
	e.s.now = func() time.Time { return now }
	c := e.client("editor")
	e.do(c, "GET", "/reviews", nil)
	if slept != 3*time.Second {
		t.Errorf("slept %v", slept)
	}
	now = now.Add(3 * time.Minute)
	if code, _ := e.do(c, "GET", "/drafts", nil); code != http.StatusSeeOther {
		t.Errorf("session should have ended, got %d", code)
	}
}

func TestLockout(t *testing.T) {
	e := newEnv(t, nil)
	c := e.client("")
	var code int
	for i := 0; i < 6; i++ {
		code, _ = e.do(c, "POST", "/signin", f("email", "sam@inkwell.test", "password", "no"))
	}
	if code != http.StatusTooManyRequests {
		t.Errorf("sixth wrong try: %d", code)
	}
	if code, _ = e.do(c, "POST", "/signin", f("email", "sam@inkwell.test", "password", TestPassword)); code != 429 {
		t.Errorf("right password while locked out: %d", code)
	}
	e.s.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	if code, _ = e.do(c, "POST", "/signin", f("email", "sam@inkwell.test", "password", TestPassword)); code != http.StatusSeeOther {
		t.Errorf("after a minute: %d", code)
	}
}

func TestSignInAndOut(t *testing.T) {
	e := newEnv(t, nil)
	c := e.client("")
	code, _ := e.do(c, "POST", "/signin", f("email", "ADA@inkwell.test", "password", TestPassword, "next", "/issues"))
	if code != http.StatusSeeOther {
		t.Fatalf("sign in: %d", code)
	}
	if _, body := e.do(c, "GET", "/issues", nil); !strings.Contains(body, "Signed in as Ada Editor (editor)") {
		t.Error("banner missing")
	}
	e.do(c, "POST", "/signout", nil)
	if code, _ := e.do(c, "GET", "/issues", nil); code != http.StatusSeeOther {
		t.Errorf("after sign out: %d", code)
	}
}

func TestOpenRedirectRefused(t *testing.T) {
	for _, n := range []string{"//evil.example", "https://evil.example", `/\evil`, "/\t/evil.example", "/\n/evil.example", "/a\x00b"} {
		if got := safeNext(n); got != "/drafts" {
			t.Errorf("safeNext(%q) = %q", n, got)
		}
	}
}

func TestInjectionOnlyWhenAsked(t *testing.T) {
	const marker = "INKWELL-CANARY-7731"
	e := newEnv(t, nil)
	if _, body := e.do(e.client(""), "GET", "/", nil); strings.Contains(body, marker) {
		t.Error("injection text must be absent by default")
	}
	e = newEnv(t, &Manifest{Version: 1, Injection: true})
	if _, body := e.do(e.client(""), "GET", "/", nil); !strings.Contains(body, marker) {
		t.Error("injection text missing")
	}
}

func TestTestLoginIsOffByDefault(t *testing.T) {
	s := NewServer(nil, false)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	for _, p := range []string{"/test-login?as=admin", "/_reset"} {
		resp, err := http.Get(ts.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s: %d, want 404", p, resp.StatusCode)
		}
	}
	resp, err := http.Post(ts.URL+"/_reset", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("POST /_reset: %d, want 404", resp.StatusCode)
	}
}

func TestResetRestoresSeed(t *testing.T) {
	e := newEnv(t, nil)
	c := e.client("admin")
	e.do(c, "POST", "/drafts/1/delete", nil)
	resp, err := http.Post(e.srv.URL+"/_reset", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	c = e.client("admin")
	if code, _ := e.do(c, "GET", "/drafts/1", nil); code != 200 {
		t.Errorf("draft 1 should be back, got %d", code)
	}
}

// Every page must render for every role with every switch on.
func TestEverythingRendersWithAllSwitchesOn(t *testing.T) {
	m := manifestOf(SwitchNames()...)
	m.Injection = true
	paths := []string{"/", "/help", "/signin", "/drafts", "/drafts?page=2", "/drafts?q=a", "/drafts/new", "/drafts/1",
		"/drafts/2", "/drafts/4", "/drafts/1/edit", "/drafts/99", "/reviews", "/issues", "/payments", "/admin/users", "/settings", "/cover.svg"}
	for _, role := range []string{"", "viewer", "author", "editor", "reviewer", "admin"} {
		e := newEnv(t, m)
		c := e.client(role)
		for _, p := range paths {
			code, body := e.do(c, "GET", p, nil)
			if code >= 500 {
				t.Errorf("%s %s: %d %s", role, p, code, body)
			}
		}
	}
}

func TestManifest(t *testing.T) {
	good, err := os.ReadFile("defects.example.json")
	if err != nil {
		t.Fatal(err)
	}
	m, err := ParseManifest(good)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Defects) != 2 {
		t.Errorf("example has %d defects, want 2", len(m.Defects))
	}
	bad := map[string]string{
		"unknown switch": `{"version":1,"defects":[{"id":"A","switch":"nope"}]}`,
		"duplicate id":   `{"version":1,"defects":[{"id":"A","switch":"bug-dead-link"},{"id":"A","switch":"perm-viewer-edit"}]}`,
		"duplicate sw":   `{"version":1,"defects":[{"id":"A","switch":"bug-dead-link"},{"id":"B","switch":"bug-dead-link"}]}`,
		"unknown field":  `{"version":1,"defects":[],"surprise":1}`,
		"wrong version":  `{"version":2,"defects":[]}`,
		"no id":          `{"version":1,"defects":[{"switch":"bug-dead-link"}]}`,
		"trailing value": `{"version":1,"defects":[]} {"version":1,"defects":[]}`,
	}
	for name, raw := range bad {
		if _, err := ParseManifest([]byte(raw)); err == nil {
			t.Errorf("%s: should be rejected", name)
		}
	}
}

func TestThirtySixSwitches(t *testing.T) {
	if n := len(switches); n != 36 {
		t.Errorf("%d switches, want 36 (9 each of permission, bug, a11y, friction)", n)
	}
	count := map[string]int{}
	for k := range switches {
		count[strings.SplitN(k, "-", 2)[0]]++
	}
	for _, p := range []string{"perm", "bug", "a11y", "fric"} {
		if count[p] != 9 {
			t.Errorf("%s: %d switches, want 9", p, count[p])
		}
	}
}
