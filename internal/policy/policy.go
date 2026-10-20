// Package policy decides which requests a run may send. The decision is
// enforced by network interception in the browser, not by the prompt.
package policy

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Target is the plain-URL target adapter's configuration.
type Target struct {
	// Adapter names the adapter; only "url" exists.
	Adapter string `json:"adapter"`
	// Environment is the preview name recorded in the run, such as "production" or "pr-42".
	Environment string `json:"environment"`
	// BaseURL is where the persona starts.
	BaseURL string `json:"base_url"`
	// AllowedHosts are the only hosts the browser may send requests to. An entry is a host name or
	// host:port. A bare host matches the default port of the request's scheme, and also the port of
	// base_url when it is the base URL's own host. Another port needs its own host:port entry.
	AllowedHosts []string `json:"allowed_hosts"`
	// ReadOnly fails every request to the allowed hosts whose method is not GET, HEAD or OPTIONS.
	ReadOnly bool `json:"read_only"`
	// Commit is the target's commit, when known.
	Commit string `json:"commit,omitempty"`
	// Signin declares how the engine signs in before the persona starts. Nil means no sign-in.
	Signin *Signin `json:"signin,omitempty"`
}

// Validate checks the target configuration.
func (t Target) Validate() error {
	if t.Adapter == "" {
		t.Adapter = "url"
	}
	if t.Adapter != "url" {
		return fmt.Errorf("policy: unknown adapter %q", t.Adapter)
	}
	if t.Environment == "" {
		return fmt.Errorf("policy: environment is required")
	}
	u, err := url.Parse(t.BaseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("policy: base_url must be an absolute URL")
	}
	if u.Scheme != "https" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" {
		return fmt.Errorf("policy: base_url must use https (or localhost)")
	}
	if len(t.AllowedHosts) == 0 {
		return fmt.Errorf("policy: allowed_hosts must name at least one host")
	}
	if !t.HostAllowed(u.Hostname()) {
		return fmt.Errorf("policy: base_url host %s is not in allowed_hosts", u.Hostname())
	}
	if t.Signin != nil {
		if err := t.validateSignin(); err != nil {
			return fmt.Errorf("policy: %w", err)
		}
	}
	return nil
}

// HostAllowed reports whether a hostname is in the allow list, whatever its port. The match is
// exact and case-insensitive.
func (t Target) HostAllowed(host string) bool {
	host = normHost(host)
	for _, h := range t.AllowedHosts {
		if name, _ := splitEntry(h); name == host {
			return true
		}
	}
	return false
}

// OriginAllowed reports whether a request to scheme://host:port may leave the browser. port is empty
// for the scheme's default. A different port on an allowed host is a different origin, so a page on
// the target cannot reach another service on the same machine.
func (t Target) OriginAllowed(scheme, host, port string) bool {
	host = normHost(host)
	if port == "" {
		port = defaultPort(scheme)
	}
	var basePort string
	if u, err := url.Parse(t.BaseURL); err == nil && normHost(u.Hostname()) == host {
		basePort = u.Port()
		if basePort == "" {
			basePort = defaultPort(u.Scheme)
		}
	}
	for _, h := range t.AllowedHosts {
		name, p := splitEntry(h)
		if name != host {
			continue
		}
		if p == port || (p == "" && (port == defaultPort(scheme) || port == basePort)) {
			return true
		}
	}
	return false
}

func normHost(h string) string { return strings.ToLower(strings.TrimSuffix(h, ".")) }

// splitEntry splits an allow-list entry into a lower-case host and an optional port.
func splitEntry(e string) (host, port string) {
	e = strings.ToLower(strings.TrimSpace(e))
	if strings.Count(e, ":") == 1 {
		host, port, _ = strings.Cut(e, ":")
		return normHost(host), port
	}
	return normHost(e), ""
}

func defaultPort(scheme string) string {
	switch strings.ToLower(scheme) {
	case "http", "ws":
		return "80"
	case "https", "wss":
		return "443"
	}
	return ""
}

// Verdict is what the interceptor does with a request.
type Verdict int

const (
	// Allow lets the request through.
	Allow Verdict = iota
	// BlockHost fails a request to a host outside the allow list.
	BlockHost
	// BlockWrite fails a request that could change the read-only target.
	BlockWrite
	// BlockSignout fails a request to a path the target declares as a sign-out.
	BlockSignout
)

func (v Verdict) String() string {
	switch v {
	case Allow:
		return "allow"
	case BlockHost:
		return "host not allowed"
	case BlockWrite:
		return "read-only target"
	case BlockSignout:
		return "sign-out is blocked"
	}
	return "unknown"
}

// safeMethods cannot change state on a well behaved server.
var safeMethods = map[string]bool{http.MethodGet: true, http.MethodHead: true, http.MethodOptions: true}

// Decide applies the policy to one request.
func (t Target) Decide(method, rawURL string) Verdict {
	u, err := url.Parse(rawURL)
	if err != nil {
		return BlockHost
	}
	if u.Scheme == "data" || u.Scheme == "blob" || u.Scheme == "about" {
		return Allow
	}
	if !t.OriginAllowed(u.Scheme, u.Hostname(), u.Port()) {
		return BlockHost
	}
	if t.signoutBlocked(u) {
		return BlockSignout
	}
	if t.ReadOnly && !safeMethods[strings.ToUpper(method)] {
		return BlockWrite
	}
	return Allow
}
