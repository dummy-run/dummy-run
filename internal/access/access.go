// Package access gets the browser past the protection in front of a staging or preview site: HTTP
// Basic auth and header-based bypasses such as Vercel's protection bypass or Cloudflare Access
// service tokens.
//
// The secrets come only from environment variables and live only in memory. They go only to requests
// whose origin the caller's scope accepts (the target's allowed hosts), so a redirect to another host
// never carries them. Nothing here formats a secret into an error, a log line or a record: Summary
// is the only description of the access that leaves the package, and it holds names only.
package access

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/dummy-run/dummy-run/internal/bidi"
	"github.com/dummy-run/dummy-run/internal/policy"
)

// Summary says which access methods are set up, without any value. It is what a run record keeps.
type Summary struct {
	// Methods is basic and/or headers.
	Methods []string `json:"methods"`
	// HeaderNames are the names of the headers that carry a secret. Names only.
	HeaderNames []string `json:"header_names,omitempty"`
}

// Resolved holds the secrets read from the environment and the scope they may be sent to.
type Resolved struct {
	basicHeader      bidi.Header // preemptive Authorization: Basic ...
	user, pass       string
	hasBasic         bool
	challengeOnly    bool
	headers          []bidi.Header
	summary          Summary
	inScope          func(rawURL string) bool
	mu               sync.Mutex
	challengeCounter map[string]int
}

// maxChallenges is how many times one request may be answered. A second challenge means the
// credentials were wrong, so the third is refused instead of looping.
const maxChallenges = 1

// Resolve reads the variables a configuration names. getenv is usually os.Getenv. inScope says whether
// a URL belongs to the target; credentials are only ever offered for URLs it accepts. A nil auth
// gives a nil Resolved, which does nothing.
func Resolve(a *policy.Auth, getenv func(string) string, inScope func(rawURL string) bool) (*Resolved, error) {
	if a == nil {
		return nil, nil
	}
	if err := a.Validate(); err != nil {
		return nil, err
	}
	if inScope == nil {
		return nil, fmt.Errorf("access: no scope was given for the credentials")
	}
	r := &Resolved{inScope: inScope, challengeCounter: map[string]int{}}
	if b := a.Basic; b != nil {
		user, err := lookup(getenv, b.UsernameEnv, "Basic auth user name (username_env)")
		if err != nil {
			return nil, err
		}
		pass, err := lookup(getenv, b.PasswordEnv, "Basic auth password (password_env)")
		if err != nil {
			return nil, err
		}
		if strings.ContainsRune(user, ':') {
			return nil, fmt.Errorf("access: the Basic auth user name (username_env) contains a colon, which Basic auth cannot carry")
		}
		r.hasBasic, r.user, r.pass = true, user, pass
		r.basicHeader = bidi.StringHeader("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(user+":"+pass)))
		r.summary.Methods = append(r.summary.Methods, "basic")
	}
	for _, name := range a.HeaderNames() {
		v, err := lookup(getenv, a.Headers[name].ValueEnv, "value of header "+name+" (value_env)")
		if err != nil {
			return nil, err
		}
		if strings.ContainsAny(v, "\r\n\x00") {
			return nil, fmt.Errorf("access: the value of header %s contains a line break or NUL, which a header cannot carry", name)
		}
		r.headers = append(r.headers, bidi.StringHeader(name, v))
		r.summary.HeaderNames = append(r.summary.HeaderNames, name)
	}
	if len(r.headers) > 0 {
		r.summary.Methods = append(r.summary.Methods, "headers")
	}
	return r, nil
}

// lookup reads one variable. The error names the variable only when its name looks like a variable
// name by convention (upper case); anything else may be a secret typed where a name belongs.
func lookup(getenv func(string) string, name, what string) (string, error) {
	v := getenv(name)
	if v != "" {
		return v, nil
	}
	if name == strings.ToUpper(name) {
		return "", fmt.Errorf("access: the %s is not set: environment variable %s is empty or missing", what, name)
	}
	return "", fmt.Errorf("access: the %s is not set: the environment variable it names is empty or missing", what)
}

// Summary returns the names-only description of the access.
func (r *Resolved) Summary() Summary {
	if r == nil {
		return Summary{}
	}
	s := r.summary
	s.Methods = append([]string(nil), s.Methods...)
	s.HeaderNames = append([]string(nil), s.HeaderNames...)
	sort.Strings(s.HeaderNames)
	return s
}

// Active reports whether there is anything to send.
func (r *Resolved) Active() bool { return r != nil && (r.hasBasic || len(r.headers) > 0) }

// HasBasic reports whether Basic auth is set up, so the browser must also intercept auth challenges.
func (r *Resolved) HasBasic() bool { return r != nil && r.hasBasic }

// Headers returns the headers to add to a request to rawURL: none unless the URL is in scope. The
// bypass headers replace any header of the same name. The Basic Authorization header is added only
// when the request has no Authorization of its own, so a page's calls to its own API keep their
// token; an in-scope server that wants Basic gets it through the challenge answer instead.
func (r *Resolved) Headers(rawURL string, existing []bidi.Header) []bidi.Header {
	if !r.Active() || !r.inScope(rawURL) {
		return nil
	}
	out := append([]bidi.Header(nil), r.headers...)
	if r.hasBasic && !r.challengeOnly {
		if _, has := bidi.HeaderValue(existing, "Authorization"); !has {
			out = append(out, r.basicHeader)
		}
	}
	return out
}

// ChallengeOnly returns a copy that never sends the Basic Authorization header ahead of a challenge,
// so only the answer to a 401 carries the credentials. The tests use it to exercise that path.
func (r *Resolved) ChallengeOnly() *Resolved {
	if r == nil {
		return nil
	}
	c := &Resolved{basicHeader: r.basicHeader, user: r.user, pass: r.pass, hasBasic: r.hasBasic, challengeOnly: true,
		headers: r.headers, summary: r.summary, inScope: r.inScope, challengeCounter: map[string]int{}}
	return c
}

// Challenge answers a Basic auth challenge for request id on rawURL. It says no for a URL out of
// scope, and for a request that was already answered once, which means the credentials are wrong.
func (r *Resolved) Challenge(id, rawURL string) (user, pass string, ok bool) {
	if !r.HasBasic() || !r.inScope(rawURL) {
		return "", "", false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.challengeCounter[id] >= maxChallenges {
		return "", "", false
	}
	if len(r.challengeCounter) > 4096 {
		r.challengeCounter = map[string]int{}
	}
	r.challengeCounter[id]++
	return r.user, r.pass, true
}

// HandleAuth answers one network.authRequired event: credentials for an in-scope challenge, a refusal
// for anything else.
func (r *Resolved) HandleAuth(ctx context.Context, c *bidi.Conn, e bidi.AuthRequired) {
	if !e.IsBlocked {
		return
	}
	if user, pass, ok := r.Challenge(e.Request.Request, e.Request.URL); ok {
		_ = c.ProvideCredentials(ctx, e.Request.Request, user, pass)
		return
	}
	_ = c.CancelAuth(ctx, e.Request.Request)
}

// OriginScope returns a scope that accepts the scheme, host and port of each given URL, and nothing else.
// A URL that is not https, and not on a loopback host, adds nothing.
func OriginScope(rawURLs []string) func(string) bool {
	origins := map[string]bool{}
	for _, raw := range rawURLs {
		if k, ok := originKey(raw); ok && secure(raw) {
			origins[k] = true
		}
	}
	return func(raw string) bool {
		k, ok := originKey(raw)
		return ok && secure(raw) && origins[k]
	}
}

// secure reports whether credentials may travel to rawURL: https, or http on a loopback host, which
// is the same rule that base_url follows. A cleartext request to any other host never carries them,
// even when the host is in scope, since anyone on the network could read them.
func secure(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return true
	case "http":
		h := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
		if h == "localhost" {
			return true
		}
		ip := net.ParseIP(h)
		return ip != nil && ip.IsLoopback()
	}
	return false
}

// TargetScope returns a scope that accepts what the target's allowed_hosts accept, over https or on a loopback host.
func TargetScope(t policy.Target) func(string) bool {
	return func(raw string) bool {
		u, err := url.Parse(raw)
		if err != nil || !secure(raw) {
			return false
		}
		return t.OriginAllowed(u.Scheme, u.Hostname(), u.Port())
	}
}

func originKey(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	port := u.Port()
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		default:
			return "", false
		}
	}
	return scheme + "://" + strings.ToLower(strings.TrimSuffix(u.Hostname(), ".")) + ":" + port, true
}

// Attach sets up a connection that has no request hook of its own, as the a11y sweep does: every
// in-scope request gets the headers, and Basic challenges are answered. It returns once the
// interception is live. Call it before the first navigation.
func Attach(ctx context.Context, c *bidi.Conn, r *Resolved) error {
	if !r.Active() {
		return nil
	}
	c.OnBeforeRequestSent(func(e bidi.BeforeRequestSent) {
		if !e.IsBlocked {
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), bidi.DefaultTimeout)
			defer cancel()
			set := r.Headers(e.Request.URL, e.Request.Headers)
			if len(set) == 0 {
				_ = c.ContinueRequestUnchanged(ctx, e.Request.Request)
				return
			}
			_ = c.ContinueRequest(ctx, e.Request.Request, e.Request.Headers, set)
		}()
	})
	events := []string{"network.beforeRequestSent"}
	phases := []string{"beforeRequestSent"}
	if r.HasBasic() {
		c.OnAuthRequired(func(e bidi.AuthRequired) {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), bidi.DefaultTimeout)
				defer cancel()
				r.HandleAuth(ctx, c, e)
			}()
		})
		events = append(events, "network.authRequired")
		phases = append(phases, "authRequired")
	}
	if err := c.Subscribe(ctx, events); err != nil {
		return err
	}
	_, err := c.AddInterceptPhases(ctx, nil, phases...)
	return err
}

// transport adds the access headers to the engine's own HTTP requests (robots.txt, sitemaps, the home
// page hash) that go to an in-scope origin. It sets them on a copy of each request, one hop at a time,
// so a redirect to another host never inherits them.
type transport struct {
	r    *Resolved
	next http.RoundTripper
}

func (t transport) RoundTrip(req *http.Request) (*http.Response, error) {
	set := t.r.Headers(req.URL.String(), nil)
	if len(set) == 0 {
		return t.next.RoundTrip(req)
	}
	clone := req.Clone(req.Context())
	for _, h := range set {
		clone.Header.Set(h.Name, h.Value.Value)
	}
	return t.next.RoundTrip(clone)
}

// WrapClient returns a copy of c whose requests carry the access headers when they are in scope. A nil c
// means http.DefaultClient. With nothing to send it returns c unchanged.
func WrapClient(c *http.Client, r *Resolved) *http.Client {
	if !r.Active() {
		return c
	}
	if c == nil {
		c = http.DefaultClient
	}
	next := c.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	cp := *c
	cp.Transport = transport{r: r, next: next}
	return &cp
}
