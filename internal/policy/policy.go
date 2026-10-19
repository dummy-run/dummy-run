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
	// AllowedHosts are the only hosts the browser may send requests to.
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
	if !t.HostAllowed(u.Hostname()) {
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
