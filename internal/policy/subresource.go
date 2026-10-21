package policy

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Kind says what a request is for, which decides how far it may go.
type Kind int

const (
	// Navigation is a document load: the page in the persona's tab, a frame or a pop-up. It stays on the allowed
	// hosts. Frames count, because a frame from another host runs with that host's origin, and a host that
	// resolves to an internal address after the check (DNS rebinding) would let its script read internal pages.
	Navigation Kind = iota
	// Subresource is everything else a page loads to show itself: images, stylesheets, fonts, scripts, media
	// and fetch or XHR calls. A GET or HEAD of one may go to any public host, because a site's
	// pictures and styles often live on a CDN, and a page without them is not the page a visitor sees.
	Subresource
)

// LookupFunc resolves a host name to its addresses. Tests replace it.
type LookupFunc func(ctx context.Context, host string) ([]net.IP, error)

// DecideRequest applies the policy to one request of the given kind. Writes are never let out to another
// host, and a read-only target still fails every write to its own hosts. A subresource GET or HEAD outside
// the allowed hosts goes through only to a public address: a host that is, or resolves to, a loopback,
// private, link-local or other internal address is refused, even when the target itself is local. An internal
// host the page needs, such as a dev server's asset port, goes in allowed_hosts.
func (t Target) DecideRequest(method, rawURL string, kind Kind) Verdict {
	v := t.Decide(method, rawURL)
	if v != BlockHost || kind != Subresource {
		return v
	}
	m := strings.ToUpper(method)
	if m != "GET" && m != "HEAD" {
		return BlockHost
	}
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" {
		return BlockHost
	}
	if !t.publicHost(u.Hostname()) {
		return BlockHost
	}
	return Allow
}

// publicHost is true when host is a public name or address. A name is resolved, and every address it has must
// be public, so a DNS name that points inside the network is refused too. A name that does not resolve is refused.
func (t Target) publicHost(host string) bool {
	host = normHost(host)
	if internalName(host) {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return !internalIP(ip)
	}
	lookup := t.Lookup
	if lookup == nil {
		lookup = defaultLookup
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ips, err := lookup(ctx, host)
	if err != nil || len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		if internalIP(ip) {
			return false
		}
	}
	return true
}

func internalName(host string) bool {
	host = normHost(host)
	if host == "localhost" || !strings.Contains(host, ".") && net.ParseIP(host) == nil {
		return true
	}
	for _, suffix := range []string{".localhost", ".local", ".internal", ".lan", ".home.arpa"} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

// specialUse are ranges outside the standard library's checks that are not public destinations: "this network",
// carrier-grade NAT (RFC 6598), IETF protocol assignments, benchmarking and the reserved block.
var specialUse = func() []*net.IPNet {
	var out []*net.IPNet
	for _, c := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "198.18.0.0/15", "240.0.0.0/4"} {
		_, n, _ := net.ParseCIDR(c)
		out = append(out, n)
	}
	return out
}()

// nat64 and sixToFour are IPv6 prefixes that carry an IPv4 address inside, which is checked as an address itself.
var (
	_, nat64, _     = net.ParseCIDR("64:ff9b::/96")
	_, sixToFour, _ = net.ParseCIDR("2002::/16")
)

func internalIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	} else if nat64.Contains(ip) {
		return internalIP(ip[12:16])
	} else if sixToFour.Contains(ip) {
		return internalIP(ip[2:6])
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	for _, n := range specialUse {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// resolve looks a name up. Tests replace it to check the cache.
var resolve = func(ctx context.Context, host string) ([]net.IP, error) {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		ips = append(ips, a.IP)
	}
	return ips, err
}

// maxCached is how many names the cache holds before it starts again.
const maxCached = 4096

// lookupCache keeps answers for a minute, so a page with fifty images from one CDN costs one lookup. Requests for
// a name already being looked up wait for that lookup instead of starting another. A lookup that timed out or was
// cancelled is not kept, so one slow answer does not refuse a CDN for a minute.
var lookupCache = struct {
	sync.Mutex
	m        map[string]cached
	inflight map[string]*pending
}{m: map[string]cached{}, inflight: map[string]*pending{}}

type cached struct {
	ips []net.IP
	err error
	at  time.Time
}

type pending struct {
	done chan struct{}
	ips  []net.IP
	err  error
}

func defaultLookup(ctx context.Context, host string) ([]net.IP, error) {
	lookupCache.Lock()
	if c, ok := lookupCache.m[host]; ok && time.Since(c.at) < time.Minute {
		lookupCache.Unlock()
		return c.ips, c.err
	}
	if p, ok := lookupCache.inflight[host]; ok {
		lookupCache.Unlock()
		select {
		case <-p.done:
			return p.ips, p.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	p := &pending{done: make(chan struct{})}
	lookupCache.inflight[host] = p
	lookupCache.Unlock()

	p.ips, p.err = resolve(ctx, host)
	lookupCache.Lock()
	delete(lookupCache.inflight, host)
	if timedOut := errors.Is(p.err, context.DeadlineExceeded) || errors.Is(p.err, context.Canceled) || ctx.Err() != nil; p.err == nil || !timedOut {
		if len(lookupCache.m) >= maxCached {
			lookupCache.m = map[string]cached{}
		}
		lookupCache.m[host] = cached{ips: p.ips, err: p.err, at: time.Now()}
	}
	lookupCache.Unlock()
	close(p.done)
	return p.ips, p.err
}

// KindOf maps whether a request loads a document onto the policy's kinds.
func KindOf(navigation bool) Kind {
	if navigation {
		return Navigation
	}
	return Subresource
}

// TargetOrigin reports whether a request goes to one of the target's own origins, the only places the engine
// sends its run headers.
func (t Target) TargetOrigin(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return t.OriginAllowed(u.Scheme, u.Hostname(), u.Port())
}
