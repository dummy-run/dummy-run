// Package links checks the links on a page with lychee, which runs as a separate program. A link that answers
// with an error status or can't be reached at all is broken. No model is involved.
package links

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
)

// LycheeVersion is the lychee release the container image ships and the docs name.
const LycheeVersion = "0.24.2"

// ErrNoLychee means no lychee program was found.
var ErrNoLychee = errors.New("links: lychee not found; install it or set DUMMYRUN_LYCHEE (see docs/ENGINE.md)")

// Find returns the lychee program to run: DUMMYRUN_LYCHEE when set, otherwise lychee on the PATH.
func Find() (string, error) {
	if p := os.Getenv("DUMMYRUN_LYCHEE"); p != "" {
		found, err := exec.LookPath(p)
		if err != nil {
			return "", fmt.Errorf("links: DUMMYRUN_LYCHEE: %w", err)
		}
		return found, nil
	}
	p, err := exec.LookPath("lychee")
	if err != nil {
		return "", ErrNoLychee
	}
	return p, nil
}

// Outcome kinds.
const (
	OK       = "ok"
	Broken   = "broken"
	Refused  = "refused"   // the site turned the checker away, such as 403 or 429, so the link may still work
	TimedOut = "timed out" // no answer in time
	Skipped  = "skipped"   // lychee left it out, such as a .invalid or private address
)

// Result is what one link did.
type Result struct {
	URL     string `json:"url"`
	Outcome string `json:"outcome"`
	// Code is the HTTP status, 0 when there was none, such as a host that doesn't exist.
	Code int    `json:"code,omitempty"`
	Text string `json:"text,omitempty"`
}

// refused are status codes that mean the site turned an automated checker away rather than that the page is gone.
var refused = map[int]bool{401: true, 403: true, 429: true, 999: true}

// Classify sorts a status into an outcome. A 2xx or 3xx code is fine, 401, 403, 429 and 999 are refused, any other
// error code is broken, and no code at all is broken unless the error was a timeout.
func Classify(code int, timedOut bool) string {
	switch {
	case timedOut:
		return TimedOut
	case code >= 200 && code < 400:
		return OK
	case refused[code]:
		return Refused
	default:
		return Broken
	}
}

// Check runs lychee once on the URLs and returns a result for each. The user agent is the one the sweep sends.
// lychee gets no credentials: links that need them are checked from the browser instead. Unless allowPrivate is
// set, lychee skips loopback, private and link-local addresses, so a page can't use the checker to reach the
// machine it runs on or the network around it.
func Check(ctx context.Context, bin string, urls []string, userAgent string, allowPrivate bool) ([]Result, error) {
	if len(urls) == 0 {
		return nil, nil
	}
	args := []string{"--format", "json", "--no-progress", "--max-concurrency", "8",
		"--timeout", "20", "--max-retries", "1", "--user-agent", userAgent}
	if !allowPrivate {
		args = append(args, "--exclude-all-private")
	}
	args = append(args, "-")
	var out, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // lychee is the program the user chose // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- the path comes from DUMMYRUN_LYCHEE or PATH, never from a page
	cmd.Stdin = strings.NewReader(strings.Join(urls, "\n") + "\n")
	cmd.Stdout, cmd.Stderr = &out, &stderr
	// lychee exits 2 when it found broken links, which is a result, not a failure.
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 2 {
			return nil, fmt.Errorf("links: lychee: %w: %s", err, firstLine(stderr.String()))
		}
	}
	return parse(out.Bytes(), urls)
}

type entry struct {
	URL    string `json:"url"`
	Status struct {
		Text string `json:"text"`
		Code int    `json:"code"`
	} `json:"status"`
}

// parse reads lychee's JSON report. A URL in none of its maps succeeded.
func parse(b []byte, urls []string) ([]Result, error) {
	var r struct {
		ErrorMap    map[string][]entry `json:"error_map"`
		TimeoutMap  map[string][]entry `json:"timeout_map"`
		ExcludedMap map[string][]entry `json:"excluded_map"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("links: read lychee's report: %w", err)
	}
	got := map[string]Result{}
	for _, es := range r.ErrorMap {
		for _, e := range es {
			got[e.URL] = Result{URL: e.URL, Outcome: Classify(e.Status.Code, false), Code: e.Status.Code, Text: firstLine(e.Status.Text)}
		}
	}
	for _, es := range r.TimeoutMap {
		for _, e := range es {
			got[e.URL] = Result{URL: e.URL, Outcome: TimedOut, Text: firstLine(e.Status.Text)}
		}
	}
	for _, es := range r.ExcludedMap {
		for _, e := range es {
			got[e.URL] = Result{URL: e.URL, Outcome: Skipped}
		}
	}
	out := make([]Result, 0, len(urls))
	for _, u := range urls {
		res, ok := got[u]
		if !ok {
			// lychee may have written the URL normalised, such as with a trailing slash.
			res, ok = got[strings.TrimSuffix(u, "/")]
			if !ok {
				res, ok = got[u+"/"]
			}
		}
		if !ok {
			res = Result{Outcome: OK}
		}
		res.URL = u
		out = append(out, res)
	}
	return out, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// PrivateHost reports whether host is localhost, or an address on a loopback, private or link-local network, or a
// name that resolves to one. A name that doesn't resolve isn't private: lychee reports it as broken.
func PrivateHost(ctx context.Context, host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return true
	}
	ips := []net.IP{net.ParseIP(host)}
	if ips[0] == nil {
		addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return false
		}
		ips = ips[:0]
		for _, a := range addrs {
			ips = append(ips, a.IP)
		}
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return true
		}
	}
	return false
}
