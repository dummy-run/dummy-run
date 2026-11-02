package policy

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
)

// Block lists requests and socket.io events a run must never send, whatever read_only says. It is for a target
// the persona may change, where a few actions would still reach a real person: a feedback form someone reviews,
// a support ticket, a referral, an email. Rules only add blocks. They never let through a request the rest of
// the policy refuses.
type Block struct {
	// Requests are matched against every request to the allowed hosts.
	Requests []RequestRule `json:"requests,omitempty"`
	// SocketEvents are socket.io event names that the page may not emit. "*" matches any run of characters but
	// "/", and "?" one character. The engine
	// drops them in the page before they are sent, over a WebSocket or over HTTP long-polling.
	SocketEvents []string `json:"socket_events,omitempty"`
}

// RequestRule matches requests by method and path.
type RequestRule struct {
	// Method is an HTTP method, matched case-insensitively. Empty matches every method.
	Method string `json:"method,omitempty"`
	// Path starts with "/". A segment may hold path.Match wildcards, so "*" is exactly one non-empty segment. A
	// last segment of "**" matches one or more segments. The match ignores case, the query and a trailing slash.
	Path string `json:"path"`
}

var methodToken = regexp.MustCompile(`^[A-Za-z]+$`)

func (b *Block) validate() error {
	if b == nil {
		return nil
	}
	for i, r := range b.Requests {
		if r.Method != "" && !methodToken.MatchString(r.Method) {
			return fmt.Errorf("block.requests[%d]: method %q is not an HTTP method", i, r.Method)
		}
		if !strings.HasPrefix(r.Path, "/") {
			return fmt.Errorf("block.requests[%d]: path %q must start with /", i, r.Path)
		}
		if strings.ContainsAny(r.Path, "?#") {
			return fmt.Errorf("block.requests[%d]: path %q must not hold a query or fragment", i, r.Path)
		}
		segs := splitPath(r.Path)
		for j, s := range segs {
			if s == "**" {
				if j != len(segs)-1 {
					return fmt.Errorf("block.requests[%d]: ** may only be the last segment of %q", i, r.Path)
				}
				continue
			}
			if _, err := path.Match(s, ""); err != nil {
				return fmt.Errorf("block.requests[%d]: bad pattern %q: %w", i, r.Path, err)
			}
		}
	}
	for i, e := range b.SocketEvents {
		if e == "" {
			return fmt.Errorf("block.socket_events[%d] is empty", i)
		}
		// The page matches the same patterns in JavaScript, which knows only "*" and "?".
		if strings.ContainsAny(e, `[]\`) {
			return fmt.Errorf("block.socket_events[%d]: %q may use only * and ? as wildcards", i, e)
		}
	}
	return nil
}

// RequestBlocked reports whether a request matches one of the rules.
func (b *Block) RequestBlocked(method string, u *url.URL) bool {
	if b == nil {
		return false
	}
	segs := splitPath(strings.ToLower(u.EscapedPath()))
	for _, r := range b.Requests {
		if r.Method != "" && !strings.EqualFold(r.Method, method) {
			continue
		}
		if matchSegments(splitPath(strings.ToLower(r.Path)), segs) {
			return true
		}
	}
	return false
}

// SocketEventBlocked reports whether a socket.io event name matches one of the patterns. Event names are
// case-sensitive, as socket.io treats them.
func (b *Block) SocketEventBlocked(name string) bool {
	if b == nil {
		return false
	}
	for _, p := range b.SocketEvents {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}

// SocketEventList returns the socket.io patterns, or nil for a nil block.
func (b *Block) SocketEventList() []string {
	if b == nil {
		return nil
	}
	return b.SocketEvents
}

// splitPath splits a path into its segments, without the leading slash or a trailing one. Empty segments
// between two slashes are kept, so "*" can refuse them.
func splitPath(p string) []string {
	p = strings.TrimSuffix(strings.TrimPrefix(p, "/"), "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

func matchSegments(pattern, segs []string) bool {
	for i, p := range pattern {
		if p == "**" {
			if len(segs) <= i {
				return false
			}
			for _, s := range segs[i:] {
				if s == "" {
					return false
				}
			}
			return true
		}
		if i >= len(segs) || segs[i] == "" {
			return false
		}
		if ok, _ := path.Match(p, segs[i]); !ok {
			return false
		}
	}
	return len(segs) == len(pattern)
}
