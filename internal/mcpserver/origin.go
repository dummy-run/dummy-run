package mcpserver

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// privateSuffixes are host name endings that stay on a private network or a developer's machine.
var privateSuffixes = []string{".localhost", ".local", ".internal", ".test"}

// metadataHosts are the names of cloud metadata services. They are refused even when they end in a
// private suffix or an --allow-origin entry names them.
var metadataHosts = map[string]bool{
	"metadata": true, "metadata.goog": true, "metadata.google.internal": true,
	"instance-data": true, "instance-data.ec2.internal": true, "metadata.azure.internal": true,
}

// Origins decides which sites a tool call may point the browser at. A site is allowed when it is on this
// machine or a private network, or when an --allow-origin entry names it.
type Origins struct {
	entries []originEntry
}

type originEntry struct {
	scheme   string // empty for any
	host     string // lower case, may start with "*."
	port     string // empty for any
	wildcard bool
}

// ParseOrigins reads allow-list entries. An entry is a host such as preview.example.com, a wildcard such as
// *.example.com, or an origin such as https://preview.example.com:8443.
func ParseOrigins(list []string) (*Origins, error) {
	o := &Origins{}
	for _, raw := range list {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		e := originEntry{}
		rest := raw
		if i := strings.Index(rest, "://"); i >= 0 {
			e.scheme = strings.ToLower(rest[:i])
			if e.scheme != "http" && e.scheme != "https" {
				return nil, fmt.Errorf("allowed origin %q: the scheme must be http or https", raw)
			}
			rest = rest[i+3:]
		}
		if strings.ContainsAny(rest, "/?#@") {
			return nil, fmt.Errorf("allowed origin %q: give a host or an origin, with no path or credentials", raw)
		}
		host, port := rest, ""
		if h, p, err := net.SplitHostPort(rest); err == nil {
			host, port = h, p
		}
		host = strings.ToLower(strings.Trim(host, "[]"))
		if strings.HasPrefix(host, "*.") {
			e.wildcard = true
			host = host[2:]
			if !strings.Contains(host, ".") {
				return nil, fmt.Errorf("allowed origin %q: a wildcard needs a domain with at least two parts", raw)
			}
		}
		if host == "" || strings.Contains(host, "*") {
			return nil, fmt.Errorf("allowed origin %q: not a host", raw)
		}
		e.host, e.port = host, port
		o.entries = append(o.entries, e)
	}
	return o, nil
}

// Check returns an error that says why a URL may not be used. It accepts http and https URLs only.
func (o *Origins) Check(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("%q is not an absolute URL", raw)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("the URL scheme must be http or https, not %q", u.Scheme)
	}
	if u.User != nil {
		return nil, fmt.Errorf("the URL must not contain a user name or password")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" {
		return nil, fmt.Errorf("%q has no host", raw)
	}
	if metadataHosts[host] {
		return nil, fmt.Errorf("%s is a cloud metadata service, which this server never tests", host)
	}
	if privateHost(host) || o.listed(u.Scheme, host, portOf(u)) {
		if err := resolvesToAllowedAddress(host); err != nil {
			return nil, err
		}
		return u, nil
	}
	return nil, fmt.Errorf("%s is not allowed. This server only tests localhost and private-network hosts. "+
		"Start it with --allow-origin %s or set DUMMYRUN_MCP_ALLOW_ORIGINS to add a site", u.Host, host)
}

func portOf(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

func (o *Origins) listed(scheme, host, port string) bool {
	for _, e := range o.entries {
		if e.scheme != "" && e.scheme != scheme {
			continue
		}
		if e.port != "" && e.port != port {
			continue
		}
		if e.wildcard {
			if strings.HasSuffix(host, "."+e.host) {
				return true
			}
		} else if host == e.host {
			return true
		}
	}
	return false
}

// resolvesToAllowedAddress refuses a host name that points at a link-local address, where cloud metadata
// services live. A literal address is already judged by privateHost. A name that does not resolve is left to
// the browser to fail. The answer is read once, so this narrows the risk and is not a defence against DNS
// rebinding.
func resolvesToAllowedAddress(host string) error {
	if net.ParseIP(host) != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil
	}
	for _, a := range addrs {
		if a.IP.IsLinkLocalUnicast() || a.IP.IsLinkLocalMulticast() || a.IP.IsUnspecified() {
			return fmt.Errorf("%s resolves to %s, a link-local address where cloud metadata services live", host, a.IP)
		}
	}
	return nil
}

// privateHost reports whether a host name or address is on this machine or a private network.
func privateHost(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		ip = ip.To16()
		if v4 := ip.To4(); v4 != nil {
			ip = v4
		}
		return ip.IsLoopback() || ip.IsPrivate()
	}
	if host == "localhost" {
		return true
	}
	for _, s := range privateSuffixes {
		if strings.HasSuffix(host, s) {
			return true
		}
	}
	return false
}
