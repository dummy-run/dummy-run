// Package signintest holds the apps that the sign-in tests run against: a small fake app in this file,
// and Inkwell (examples/inkwell) when the repository has it. Both have a sign-in form, a viewer and an
// admin, a page only the admin may open, and a sign-out button that posts. Every request they get is
// counted, so a test can prove which requests the read-only policy let through.
package signintest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/policy"
	"github.com/dummy-run/dummy-run/internal/signin"
)

// FakePassword is the fake app's password. It is made up for the tests and obviously not real.
const FakePassword = "fake-Passw0rd-7Q2xZ-not-real"

// Request is one request an app received.
type Request struct{ Method, Path string }

// App is an app under test and the requests it has received.
type App struct {
	Name string
	URL  string
	// Users maps a role to the user name and password that sign in as it.
	Users map[string]signin.Creds
	// TargetJSON is the target file for the app, with base_url filled in by Target.
	targetJSON string
	// GetSignout is true when GET /logout signs the user out.
	GetSignout bool

	mu   sync.Mutex
	reqs []Request
}

// Requests returns a copy of the requests received so far.
func (a *App) Requests() []Request {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]Request(nil), a.reqs...)
}

// Count counts the requests with a method and path.
func (a *App) Count(method, path string) int {
	n := 0
	for _, r := range a.Requests() {
		if r.Method == method && r.Path == path {
			n++
		}
	}
	return n
}

// NonGET lists the requests whose method is not GET.
func (a *App) NonGET() []Request {
	var out []Request
	for _, r := range a.Requests() {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			out = append(out, r)
		}
	}
	return out
}

func (a *App) record(r *http.Request) {
	a.mu.Lock()
	a.reqs = append(a.reqs, Request{r.Method, r.URL.Path})
	a.mu.Unlock()
}

// Env names the environment variables that hold each role's credentials.
func envNames(role string) (user, pass string) {
	return "SIGNINTEST_" + role + "_USER", "SIGNINTEST_" + role + "_PASS"
}

// Getenv reads the app's test credentials, as if they were in the environment.
func (a *App) Getenv(name string) string {
	for role, c := range a.Users {
		u, p := envNames(role)
		switch name {
		case u:
			return c.Username
		case p:
			return c.Password
		}
	}
	return ""
}

// Target builds the target file for the app. The target is read-only.
func (a *App) Target(t testing.TB) policy.Target {
	t.Helper()
	var tg policy.Target
	if err := json.Unmarshal([]byte(a.targetJSON), &tg); err != nil {
		t.Fatal(err)
	}
	tg.BaseURL = a.URL + "/"
	if err := tg.Validate(); err != nil {
		t.Fatalf("test target: %v", err)
	}
	return tg
}

// Secrets lists every user name and password of the app.
func (a *App) Secrets() []string {
	var out []string
	for _, c := range a.Users {
		out = append(out, c.Username, c.Password)
	}
	return out
}

const targetTemplate = `{
  "adapter": "url", "environment": "signin-test", "base_url": "http://127.0.0.1/", "allowed_hosts": ["127.0.0.1"], "read_only": true,
  "signin": {
    "login_url": "/signin",
    "username": {"selector": "#email"},
    "password": {"label": "Password"},
    "submit": {"selector": "form[action='/signin'] button[type=submit]"},
    "allow_request": {"method": "POST", "url": "/signin"},
    "success": {"selector": "form[action='/signout']"},
    "block_paths": %s,
    "credentials": {
      "viewer": {"username_env": "SIGNINTEST_viewer_USER", "password_env": "SIGNINTEST_viewer_PASS"},
      "admin": {"username_env": "SIGNINTEST_admin_USER", "password_env": "SIGNINTEST_admin_PASS"}
    }
  }
}`

// Fake starts the fake app.
func Fake(t testing.TB) *App {
	t.Helper()
	a := &App{Name: "fake", GetSignout: true, Users: map[string]signin.Creds{
		"viewer": {Username: "viewer@fake.test", Password: FakePassword},
		"admin":  {Username: "admin@fake.test", Password: FakePassword + "-admin"},
	}, targetJSON: fmt.Sprintf(targetTemplate, `["/logout"]`)}
	f := &fakeApp{app: a, sessions: map[string]string{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	a.URL = srv.URL
	return a
}

type fakeApp struct {
	app      *App
	mu       sync.Mutex
	sessions map[string]string
	next     int
}

const layout = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>%s</title></head><body>
<header>%s</header><main>%s</main></body></html>`

func (f *fakeApp) role(r *http.Request) string {
	c, err := r.Cookie("fake_session")
	if err != nil {
		return ""
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sessions[c.Value]
}

func (f *fakeApp) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.app.record(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	role := f.role(r)
	head := `<a href="/signin">Sign in</a>`
	if role != "" {
		head = fmt.Sprintf(`<p>Signed in as the %s role.</p><form method="post" action="/signout"><button type="submit">Sign out</button></form>`, role)
	}
	page := func(status int, title, body string) {
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(w, layout, title, head, body)
	}
	switch {
	case r.URL.Path == "/" && r.Method == http.MethodGet:
		page(200, "Home", `<h1>Home</h1><p>A fake app for tests.</p><a href="/drafts">Drafts</a>`)
	case r.URL.Path == "/signin" && r.Method == http.MethodGet:
		page(200, "Sign in", `<h1>Sign in</h1><form method="post" action="/signin"><label for="email">Email</label><input id="email" name="email" type="email">
<label for="password">Password</label><input id="password" name="password" type="password"><button type="submit">Sign in</button></form>`)
	case r.URL.Path == "/signin" && r.Method == http.MethodPost:
		_ = r.ParseForm()
		for role, c := range f.app.Users {
			if r.PostForm.Get("email") == c.Username && r.PostForm.Get("password") == c.Password {
				f.mu.Lock()
				f.next++
				tok := fmt.Sprintf("tok-%d-%s", f.next, role)
				f.sessions[tok] = role
				f.mu.Unlock()
				http.SetCookie(w, &http.Cookie{Name: "fake_session", Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode}) //nolint:gosec // plain HTTP on localhost in a test
				http.Redirect(w, r, "/drafts", http.StatusSeeOther)
				return
			}
		}
		page(401, "Sign in", `<h1>Sign in</h1><p role="alert">Wrong email or password.</p>`)
	case r.URL.Path == "/drafts" && r.Method == http.MethodGet:
		if role == "" {
			http.Redirect(w, r, "/signin", http.StatusSeeOther)
			return
		}
		page(200, "Drafts", `<h1>Drafts</h1><p>Three drafts.</p>`)
	case r.URL.Path == "/admin/users" && r.Method == http.MethodGet:
		switch role {
		case "":
			http.Redirect(w, r, "/signin", http.StatusSeeOther)
		case "admin":
			page(200, "Users", `<h1>Users</h1><p>Two users.</p>`)
		default:
			page(403, "Not allowed", `<h1>Not allowed</h1>`)
		}
	case (r.URL.Path == "/signout" && r.Method == http.MethodPost) || r.URL.Path == "/logout":
		if c, err := r.Cookie("fake_session"); err == nil {
			f.mu.Lock()
			delete(f.sessions, c.Value)
			f.mu.Unlock()
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	default:
		page(404, "Not found", `<h1>Not found</h1>`)
	}
}

// Inkwell builds and starts examples/inkwell. It returns nil when the repository does not have it.
func Inkwell(t testing.TB) *App {
	t.Helper()
	root := repoRoot()
	if _, err := os.Stat(filepath.Join(root, "examples", "inkwell", "main.go")); err != nil {
		return nil
	}
	bin := filepath.Join(t.TempDir(), "inkwell")
	build := exec.CommandContext(context.Background(), "go", "build", "-o", bin, "./examples/inkwell") //nolint:gosec // fixed arguments
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build inkwell: %v\n%s", err, out)
	}
	l, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	cmd := exec.CommandContext(context.Background(), bin, "-addr", addr) //nolint:gosec // the binary was built above
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	waitHealthy(t, "http://"+addr+"/healthz")

	a := &App{Name: "inkwell", Users: map[string]signin.Creds{
		"viewer": {Username: "vik@inkwell.test", Password: "inkwell-test"},
		"admin":  {Username: "root@inkwell.test", Password: "inkwell-test"},
	}, targetJSON: fmt.Sprintf(targetTemplate, `[]`)}
	// A counting proxy in front, so the test sees every request the browser makes.
	u, _ := url.Parse("http://" + addr)
	proxy := httputil.NewSingleHostReverseProxy(u)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.record(r)
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	a.URL = srv.URL
	return a
}

func waitHealthy(t testing.TB, u string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, u, nil)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("%s did not become healthy", u)
}

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

// Apps returns the fake app and, when the repository has it, Inkwell.
func Apps(t testing.TB) []*App {
	t.Helper()
	apps := []*App{Fake(t)}
	if ink := Inkwell(t); ink != nil {
		apps = append(apps, ink)
	}
	return apps
}
