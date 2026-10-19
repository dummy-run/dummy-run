// Package policy decides which requests a run may send. The decision is
// enforced by network interception in the browser, not by the prompt.
package policy

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/dummy-run/dummy-run/internal/robots"
)

// Target is the plain-URL target adapter's configuration.
type Target struct {
	// Adapter names the adapter; only "url" exists.
	Adapter string `json:"adapter"`
	// Environment is the preview name recorded in the run, such as "production" or "pr-42".
	Environment string `json:"environment"`
	// BaseURL is where the persona starts.
	BaseURL string `json:"base_url"`
	// AllowedHosts are the only hosts the browser may send requests to.
	AllowedHosts []string `json:"allowed_hosts"`
	// ReadOnly fails every request to the allowed hosts whose method is not GET, HEAD or OPTIONS.
	ReadOnly bool `json:"read_only"`
	// Commit is the target's commit, when known.
	Commit string `json:"commit,omitempty"`
	// Identify adds a contact URL to the user agent, as "dummyrun run --identify" does.
	Identify bool `json:"identify,omitempty"`
	// RespectRobots honours the robots.txt of the base URL's origin for the token DummyRun and for *.
	RespectRobots bool `json:"respect_robots,omitempty"`
	// Robots holds the rules once they are fetched. Requests to the base URL's origin that they disallow are blocked.
	Robots *robots.Rules `json:"-"`
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
	return nil
}

// HostAllowed reports whether a hostname is in the allow list. The match is
// exact and case-insensitive.
func (t Target) HostAllowed(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, h := range t.AllowedHosts {
		if strings.ToLower(h) == host {
			return true
		}
	}
	return false
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
	// BlockRobots fails a request to a path the target's robots.txt disallows for Dummy Run.
	BlockRobots
)

func (v Verdict) String() string {
	switch v {
	case Allow:
		return "allow"
	case BlockHost:
		return "host not allowed"
	case BlockWrite:
		return "read-only target"
	case BlockRobots:
		return "robots.txt disallows DummyRun"
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
	if !t.HostAllowed(u.Hostname()) {
		return BlockHost
	}
	if t.Robots != nil && t.sameOrigin(u) && !t.Robots.Allowed(u.RequestURI()) {
		return BlockRobots
	}
	if t.ReadOnly && !safeMethods[strings.ToUpper(method)] {
		return BlockWrite
	}
	return Allow
}

// sameOrigin reports whether u has the scheme, host and port of the base URL, the origin robots.txt speaks for.
// A trailing dot on the host and an explicit default port do not change the origin.
func (t Target) sameOrigin(u *url.URL) bool {
	base, err := url.Parse(t.BaseURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(base.Scheme, u.Scheme) && originHost(base) == originHost(u)
}

// originHost is the lower-case host without a trailing dot, with the port unless it is the scheme's default.
func originHost(u *url.URL) string {
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	port := u.Port()
	if port == "" || (port == "443" && u.Scheme == "https") || (port == "80" && u.Scheme == "http") {
		return host
	}
	return host + ":" + port
}
