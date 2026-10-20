package robots

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Result is what was learned from an origin's robots.txt.
type Result struct {
	Rules *Rules
	// Origin is the scheme and host the file was fetched from.
	Origin string
	// Status says how the rules were obtained, in words a person can read.
	Status string
}

// Origin returns the scheme and host (with any port) of a URL.
func Origin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("robots: %q is not an absolute URL", raw)
	}
	return u.Scheme + "://" + u.Host, nil
}

// Fetch gets /robots.txt from the origin of rawURL once and returns the rules for token. Following RFC 9309,
// a 4xx answer means there are no rules, and a 5xx answer or a network failure means the whole site is
// off limits. The request carries userAgent when it is not empty. client may be nil.
func Fetch(ctx context.Context, client *http.Client, rawURL, token, userAgent string) (*Result, error) {
	origin, err := Origin(rawURL)
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/robots.txt", http.NoBody)
	if err != nil {
		return nil, err
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil && ctx.Err() != context.DeadlineExceeded {
			return nil, ctx.Err()
		}
		return &Result{Rules: DenyAll(), Origin: origin, Status: "robots.txt could not be fetched (" + short(err) + "), so the site is treated as off limits"}, nil
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBytes))
		if err != nil {
			return &Result{Rules: DenyAll(), Origin: origin, Status: "robots.txt could not be read, so the site is treated as off limits"}, nil
		}
		rules := Parse(body, token)
		status := "robots.txt has no group for " + token + " or *"
		if rules.Group != "" {
			status = "robots.txt group " + rules.Group
		}
		return &Result{Rules: rules, Origin: origin, Status: status}, nil
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		return &Result{Rules: AllowAll(), Origin: origin, Status: fmt.Sprintf("robots.txt answered %d, so there are no rules", resp.StatusCode)}, nil
	default:
		return &Result{Rules: DenyAll(), Origin: origin, Status: fmt.Sprintf("robots.txt answered %d, so the site is treated as off limits", resp.StatusCode)}, nil
	}
}

func short(err error) string {
	s := err.Error()
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}
