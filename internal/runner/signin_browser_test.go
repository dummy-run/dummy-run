package runner_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/cast"
	"github.com/dummy-run/dummy-run/internal/page"
	"github.com/dummy-run/dummy-run/internal/policy"
	"github.com/dummy-run/dummy-run/internal/provider"
	"github.com/dummy-run/dummy-run/internal/record"
	"github.com/dummy-run/dummy-run/internal/runner"
	"github.com/dummy-run/dummy-run/internal/signin"
	"github.com/dummy-run/dummy-run/internal/signin/signintest"
)

func writeRoleCast(t *testing.T, roles ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, role := range roles {
		dir := filepath.Join(root, role, "scenarios")
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		files := map[string]string{
			filepath.Join(role, "profile.md"):                     "---\nid: " + role + "\nrole: " + role + "\ndevice: desktop\n---\n# " + role + "\n",
			filepath.Join(role, "prompt.md"):                      "You are the " + role + ".\n",
			filepath.Join(role, "scenarios", "01-look-around.md"): "# Look around\n\nVisit the pages your role may reach.\n",
		}
		for name, body := range files {
			if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root
}

func call(name, args string) provider.ToolCall {
	return provider.ToolCall{Name: name, Args: json.RawMessage(args)}
}

// readAll returns every byte the run wrote under dir, record, steps, report and screenshots included.
func readAll(t *testing.T, dir string) []byte {
	t.Helper()
	var all bytes.Buffer
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p) //nolint:gosec // a path under the test's own temp dir
		all.Write(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return all.Bytes()
}

func noSecrets(t *testing.T, where string, data []byte, secrets []string) {
	t.Helper()
	for _, s := range secrets {
		if bytes.Contains(data, []byte(s)) {
			t.Errorf("%s holds a credential (%d characters)", where, len(s))
		}
	}
}

func stepStatus(rec *record.Record, pathSuffix string) int {
	for _, s := range rec.Steps {
		if s.Action == "visit" && strings.HasSuffix(strings.SplitN(s.URL, "?", 2)[0], pathSuffix) && s.Status != nil {
			return *s.Status
		}
	}
	return 0
}

// TestSigninEndToEnd signs in as a viewer and as an admin through the form on each browser in
// DUMMYRUN_BROWSERS. It checks that the sign-in works, that each role sees its own pages, that the
// read-only policy still blocks every other POST, and that no credential reaches any output.
func TestSigninEndToEnd(t *testing.T) {
	browsers := os.Getenv("DUMMYRUN_BROWSERS")
	if browsers == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the sign-in test")
	}
	for _, browser := range strings.Split(browsers, ",") {
		for _, app := range signintest.Apps(t) {
			t.Run(browser+"/"+app.Name, func(t *testing.T) {
				castDir := writeRoleCast(t, "viewer", "admin")
				want := map[string]int{"viewer": 403, "admin": 200}
				for _, role := range []string{"viewer", "admin"} {
					persona, err := cast.Load(castDir, role)
					if err != nil {
						t.Fatal(err)
					}
					prov := &scripted{calls: []provider.ToolCall{
						call("visit", `{"url":"/drafts"}`),
						call("visit", `{"url":"/admin/users"}`),
						call("click", `{"target":"Sign out"}`),
						call("visit", `{"url":"/drafts"}`),
					}}
					out := t.TempDir()
					var log bytes.Buffer
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
					before := len(app.Requests())
					res, err := runner.Run(ctx, runner.Config{Persona: persona, Scenario: persona.Scenarios[0], Provider: prov,
						Browser: browser, Headless: true, OutDir: out, Target: app.Target(t), Getenv: app.Getenv, Log: &log})
					cancel()
					if err != nil {
						t.Fatalf("%s: run: %v", role, err)
					}
					rec := res.Record
					si := rec.Signin
					if si == nil || !si.OK || si.Method != "form" || si.Role != role || si.Error != "" {
						t.Fatalf("%s: signin block %+v", role, si)
					}
					if si.AllowedRequest == nil || si.AllowedRequest.Method != http.MethodPost || !strings.HasSuffix(si.AllowedRequest.URL, "/signin") {
						t.Errorf("%s: allowed request %+v", role, si.AllowedRequest)
					}
					// The role sees its own pages.
					if got := stepStatus(rec, "/admin/users"); got != want[role] {
						t.Errorf("%s: /admin/users status %d, want %d", role, got, want[role])
					}
					if got := stepStatus(rec, "/drafts"); got != 200 {
						t.Errorf("%s: /drafts status %d; the sign-in did not hold", role, got)
					}
					// The only non-GET request the app saw is the sign-in. The sign-out POST was blocked.
					reqs := app.Requests()[before:]
					posts := 0
					for _, r := range reqs {
						if r.Method != http.MethodGet && r.Method != http.MethodHead {
							posts++
							if r.Method != http.MethodPost || r.Path != "/signin" {
								t.Errorf("%s: unexpected %s %s reached the app", role, r.Method, r.Path)
							}
						}
					}
					if posts != 1 {
						t.Errorf("%s: %d non-GET requests reached the app, want 1", role, posts)
					}
					blocked := false
					for _, s := range rec.Steps {
						if s.Action == "click" && strings.Contains(s.Note, "blocked: POST") && strings.Contains(s.Note, "read-only target") {
							blocked = true
						}
					}
					if !blocked {
						t.Errorf("%s: no step records the blocked sign-out POST", role)
					}
					// The model was told it is signed in, and saw no credential.
					reqJSON, _ := json.Marshal(prov.requests)
					if !strings.Contains(string(reqJSON), "signed you in as the "+role) {
						t.Errorf("%s: the prompt does not say the harness signed the persona in", role)
					}
					// Pages may show a user name, so what holds page text is checked for passwords. The
					// engine's own output, the record and the log, is checked for both.
					noSecrets(t, role+" model requests", reqJSON, app.Passwords())
					noSecrets(t, role+" output files", readAll(t, out), app.Passwords())
					noSecrets(t, role+" log", log.Bytes(), app.Secrets())
					raw, _ := json.Marshal(rec)
					noSecrets(t, role+" record", raw, app.Secrets())
				}
			})
		}
	}
}

// TestSigninGetLogoutBlocked checks that a declared sign-out path cannot end the session.
func TestSigninGetLogoutBlocked(t *testing.T) {
	browsers := os.Getenv("DUMMYRUN_BROWSERS")
	if browsers == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the sign-in test")
	}
	for _, browser := range strings.Split(browsers, ",") {
		t.Run(browser, func(t *testing.T) {
			app := signintest.Fake(t)
			persona, err := cast.Load(writeRoleCast(t, "viewer"), "viewer")
			if err != nil {
				t.Fatal(err)
			}
			prov := &scripted{calls: []provider.ToolCall{call("visit", `{"url":"/logout"}`), call("visit", `{"url":"/drafts"}`)}}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			res, err := runner.Run(ctx, runner.Config{Persona: persona, Scenario: persona.Scenarios[0], Provider: prov, Browser: browser,
				Headless: true, OutDir: t.TempDir(), Target: app.Target(t), Getenv: app.Getenv})
			if err != nil {
				t.Fatal(err)
			}
			if app.Count("GET", "/logout") != 0 {
				t.Error("the sign-out request reached the app")
			}
			if got := stepStatus(res.Record, "/drafts"); got != 200 {
				t.Errorf("/drafts status %d after the blocked sign-out", got)
			}
		})
	}
}

// TestSigninFailureLeaksNothing signs in with a wrong password. The run errors, the record says why,
// and neither the right nor the wrong password appears anywhere.
func TestSigninFailureLeaksNothing(t *testing.T) {
	browsers := os.Getenv("DUMMYRUN_BROWSERS")
	if browsers == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the sign-in test")
	}
	for _, browser := range strings.Split(browsers, ",") {
		t.Run(browser, func(t *testing.T) {
			app := signintest.Fake(t)
			wrong := "wrong-Passw0rd-9K4mQ-not-real"
			getenv := func(n string) string {
				if strings.HasSuffix(n, "_PASS") {
					return wrong
				}
				return app.Getenv(n)
			}
			persona, err := cast.Load(writeRoleCast(t, "viewer"), "viewer")
			if err != nil {
				t.Fatal(err)
			}
			out := t.TempDir()
			var log bytes.Buffer
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			res, err := runner.Run(ctx, runner.Config{Persona: persona, Scenario: persona.Scenarios[0], Provider: &scripted{}, Browser: browser,
				Headless: true, OutDir: out, Target: app.Target(t), Getenv: getenv, Log: &log})
			if err == nil {
				t.Fatal("a wrong password should stop the run")
			}
			if res == nil || res.Record.Outcome != record.OutcomeErrored || res.Record.Signin == nil || res.Record.Signin.OK || res.Record.Signin.Error == "" {
				t.Fatalf("record %+v", res)
			}
			secrets := append(app.Secrets(), wrong)
			noSecrets(t, "error", []byte(err.Error()), secrets)
			noSecrets(t, "output files", readAll(t, out), secrets)
			noSecrets(t, "log", log.Bytes(), secrets)
			if len(res.Record.Steps) != 0 {
				t.Errorf("%d steps ran after a failed sign-in", len(res.Record.Steps))
			}
			shots, _ := filepath.Glob(filepath.Join(out, "*", "screens", "*.png"))
			if len(shots) != 0 {
				t.Errorf("%d screenshots were taken during a failed sign-in", len(shots))
			}
		})
	}
}

// TestSigninSessionFile saves a session with one form sign-in, then runs a persona from the file.
// The second browser sends no sign-in POST at all, and the file is private.
func TestSigninSessionFile(t *testing.T) {
	browsers := os.Getenv("DUMMYRUN_BROWSERS")
	if browsers == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the sign-in test")
	}
	for _, browser := range strings.Split(browsers, ",") {
		for _, app := range signintest.Apps(t) {
			t.Run(browser+"/"+app.Name, func(t *testing.T) {
				target := app.Target(t)
				plan, err := signin.ResolveForm(target, "admin", app.Getenv)
				if err != nil {
					t.Fatal(err)
				}
				gate := policy.NewGate(target)
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				defer cancel()
				p, err := page.Open(ctx, page.Options{Browser: browser, Headless: true, Width: 1200, Height: 800, RequestHook: signin.Hook(target, gate)})
				if err != nil {
					t.Fatal(err)
				}
				file := filepath.Join(t.TempDir(), "sub", "admin.json")
				n, err := signin.SaveForm(ctx, p, target, plan, gate, file)
				p.Close()
				if err != nil || n == 0 {
					t.Fatalf("save: %d cookies, %v", n, err)
				}
				if st, err := os.Stat(file); err != nil || st.Mode().Perm() != 0o600 {
					t.Fatalf("session file %v, %v", st, err)
				}
				before := app.Count("POST", "/signin")
				persona, err := cast.Load(writeRoleCast(t, "admin"), "admin")
				if err != nil {
					t.Fatal(err)
				}
				out := t.TempDir()
				prov := &scripted{calls: []provider.ToolCall{call("visit", `{"url":"/admin/users"}`)}}
				// No Getenv: a session needs no credentials.
				res, err := runner.Run(ctx, runner.Config{Persona: persona, Scenario: persona.Scenarios[0], Provider: prov, Browser: browser,
					Headless: true, OutDir: out, Target: target, SessionFile: file, Getenv: func(string) string { return "" }})
				if err != nil {
					t.Fatal(err)
				}
				si := res.Record.Signin
				if si == nil || !si.OK || si.Method != "session" || si.AllowedRequest != nil {
					t.Errorf("signin block %+v", si)
				}
				if app.Count("POST", "/signin") != before {
					t.Error("the session run posted to the sign-in form")
				}
				if got := stepStatus(res.Record, "/admin/users"); got != 200 {
					t.Errorf("/admin/users status %d, the session did not sign in; steps %+v", got, res.Record.Steps)
				}
				// The session file's cookie value is a secret too.
				raw, _ := os.ReadFile(file) //nolint:gosec // the test's own temp file
				var s signin.Session
				if err := json.Unmarshal(raw, &s); err != nil || len(s.Cookies) == 0 {
					t.Fatalf("session file does not parse as storageState: %v", err)
				}
				noSecrets(t, "output files", readAll(t, out), append(app.Passwords(), s.Cookies[0].Value))
				noSecrets(t, "session file", raw, app.Secrets())
			})
		}
	}
}
