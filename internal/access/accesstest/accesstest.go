// Package accesstest holds the protected preview that the access tests run against: one server that
// wants HTTP Basic auth on one path and a bypass header on another, and a second server that a
// redirect points to. Every request each server gets is kept with its headers, so a test can prove
// which requests carried the credentials and that the second server got none.
//
// Every secret here is made up and obviously fake.
package accesstest

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/dummy-run/dummy-run/internal/policy"
)

// Fake credentials. They are the only secrets in any test output, so a test greps for them.
//
//nolint:gosec // made-up credentials for tests
const (
	User   = "fake-preview-user"
	Pass   = "fake-Pr3view-pass-not-real"
	Bypass = "fake-bypass-token-not-real-9Zq"
	// BypassHeader is the Vercel Protection Bypass for Automation header.
	BypassHeader = "x-vercel-protection-bypass"
)

// Secrets lists every secret value, for greps over outputs. The base64 form of the Basic header is
// included, since that is how the password travels.
func Secrets() []string {
	return []string{User, Pass, Bypass, base64.StdEncoding.EncodeToString([]byte(User + ":" + Pass))}
}

// Env maps the environment variable names the target file uses to the fake secrets.
var Env = map[string]string{"PREVIEW_USER": User, "PREVIEW_PASS": Pass, "PREVIEW_BYPASS": Bypass}

// Getenv reads Env.
func Getenv(name string) string { return Env[name] }

// Request is one request a server received.
type Request struct {
	Method, Path string
	Header       http.Header
}

// Server records what it receives.
type Server struct {
	*httptest.Server
	mu   sync.Mutex
	reqs []Request
}

func (s *Server) record(r *http.Request) {
	s.mu.Lock()
	s.reqs = append(s.reqs, Request{Method: r.Method, Path: r.URL.Path, Header: r.Header.Clone()})
	s.mu.Unlock()
}

// Requests returns a copy of the requests received so far.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.reqs...)
}

// For returns the requests to one path.
func (s *Server) For(path string) []Request {
	var out []Request
	for _, r := range s.Requests() {
		if r.Path == path {
			out = append(out, r)
		}
	}
	return out
}

// Fixture is the protected preview and the other host.
type Fixture struct {
	// Preview is the protected site. /basic wants Basic auth, /bypass wants the bypass header,
	// /redirect sends the browser to Elsewhere's /landing.
	Preview *Server
	// Elsewhere is a second host that is not part of the target.
	Elsewhere *Server
}

func page(title string) string {
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>` + title + `</title></head><body><main><h1>` + title + `</h1></main></body></html>`
}

// New starts the fixture. Its servers close with the test.
func New(t *testing.T) *Fixture {
	t.Helper()
	other := &Server{}
	other.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		other.record(r)
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, page("Elsewhere")) // nosemgrep: go.lang.security.audit.xss.no-io-writestring-to-responsewriter.no-io-writestring-to-responsewriter // constant fixture pages
	}))
	t.Cleanup(other.Close)

	prev := &Server{}
	mux := http.NewServeMux()
	html := func(w http.ResponseWriter, title string) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, page(title)) // nosemgrep: go.lang.security.audit.xss.no-io-writestring-to-responsewriter.no-io-writestring-to-responsewriter // constant fixture pages
	}
	basicOK := func(r *http.Request) bool {
		u, p, ok := r.BasicAuth()
		return ok && u == User && p == Pass
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { html(w, "Home") })
	mux.HandleFunc("/basic", func(w http.ResponseWriter, r *http.Request) {
		if !basicOK(r) {
			w.Header().Set("WWW-Authenticate", `Basic realm="preview"`)
			http.Error(w, "unauthorised", http.StatusUnauthorized)
			return
		}
		html(w, "Basic ok")
	})
	mux.HandleFunc("/bypass", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(BypassHeader) != Bypass {
			http.Error(w, "protected", http.StatusForbidden)
			return
		}
		html(w, "Bypass ok")
	})
	mux.HandleFunc("/both", func(w http.ResponseWriter, r *http.Request) {
		if !basicOK(r) || r.Header.Get(BypassHeader) != Bypass {
			w.Header().Set("WWW-Authenticate", `Basic realm="preview"`)
			http.Error(w, "unauthorised", http.StatusUnauthorized)
			return
		}
		html(w, "Both ok")
	})
	mux.HandleFunc("/form", func(w http.ResponseWriter, r *http.Request) {
		if !basicOK(r) {
			w.Header().Set("WWW-Authenticate", `Basic realm="preview"`)
			http.Error(w, "unauthorised", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Form</title></head><body><main><h1>Form</h1><form method="post" action="/submit"><button type="submit">Send it</button></form></main></body></html>`) // nosemgrep: go.lang.security.audit.xss.no-io-writestring-to-responsewriter.no-io-writestring-to-responsewriter // constant fixture pages
	})
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/landing", http.StatusFound)
	})
	prev.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prev.record(r)
		if strings.HasSuffix(r.URL.Path, "/favicon.ico") {
			http.NotFound(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(prev.Close)
	return &Fixture{Preview: prev, Elsewhere: other}
}

// Target returns a target for the preview with the given auth block. Only the preview is allowed.
func (f *Fixture) Target(auth *policy.Auth) policy.Target {
	host := strings.TrimPrefix(f.Preview.URL, "http://")
	return policy.Target{Adapter: "url", Environment: "preview-test", BaseURL: f.Preview.URL + "/",
		AllowedHosts: []string{host}, ReadOnly: true, Auth: auth}
}

// Auth is the auth block the tests use: Basic auth and the Vercel bypass header.
func Auth() *policy.Auth {
	return &policy.Auth{
		Basic:   &policy.BasicAuth{UsernameEnv: "PREVIEW_USER", PasswordEnv: "PREVIEW_PASS"},
		Headers: map[string]policy.HeaderSource{BypassHeader: {ValueEnv: "PREVIEW_BYPASS"}},
	}
}
