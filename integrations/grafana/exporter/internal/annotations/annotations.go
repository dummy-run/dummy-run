// Package annotations posts region annotations to Grafana's HTTP API and skips ones that exist.
package annotations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dummy-run/dummy-run/integrations/grafana/exporter/internal/telemetry"
	"github.com/dummy-run/dummy-run/integrations/grafana/exporter/internal/verify"
)

// Defaults for the pause between calls and for retries. Grafana Cloud answers 429 when annotation
// calls come too fast, so the client spaces its calls and retries a call that is refused.
const (
	// DefaultMinInterval is the least time between two calls to the Grafana API.
	DefaultMinInterval = 500 * time.Millisecond
	// DefaultRetries is how many times a call is repeated after a 429, or after a 5xx for a read.
	DefaultRetries = 5
	// DefaultBackoff is the first wait between retries when Grafana sends no Retry-After. It doubles.
	DefaultBackoff = time.Second
	// maxWait caps any single wait, including a longer Retry-After.
	maxWait = 2 * time.Minute
)

// Client talks to one Grafana instance with a service account token.
type Client struct {
	base  string
	token string
	http  *http.Client

	// MinInterval is the least time between two calls. Zero means no pause.
	MinInterval time.Duration
	// Retries is how many times a call is repeated after a 429, or after a 5xx for a read. Zero means none.
	Retries int
	// Backoff is the first wait between retries. Each retry doubles it.
	Backoff time.Duration
	// Sleep waits for d or until ctx ends. Tests replace it.
	Sleep func(ctx context.Context, d time.Duration) error
	// Now exists so tests can fix the clock that a Retry-After date is read against.
	Now func() time.Time

	last time.Time
}

// New returns a client for the Grafana at baseURL, such as http://localhost:3000. The token is a
// service account token with the Editor role or the annotations:create permission.
func New(baseURL, token string, hc *http.Client) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("grafana URL %q must be absolute, such as http://localhost:3000", baseURL)
	}
	if token == "" {
		return nil, fmt.Errorf("GRAFANA_TOKEN is not set; annotations need a Grafana service account token")
	}
	if err := verify.RequireSecureTransport(u, "GRAFANA_TOKEN"); err != nil {
		return nil, err
	}
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second, CheckRedirect: verify.RefuseInsecureRedirect}
	}
	return &Client{
		base: strings.TrimRight(baseURL, "/"), token: token, http: hc,
		MinInterval: DefaultMinInterval, Retries: DefaultRetries, Backoff: DefaultBackoff,
		Sleep: sleep, Now: time.Now,
	}, nil
}

// Result says what happened to one annotation.
type Result struct {
	// ID is the Grafana annotation ID, whether new or found.
	ID int64
	// Created is false when an identical annotation already existed.
	Created bool
}

type annotation struct {
	ID      int64    `json:"id,omitempty"`
	Time    int64    `json:"time"`
	TimeEnd int64    `json:"timeEnd"`
	Tags    []string `json:"tags"`
	Text    string   `json:"text"`
}

// Post creates the annotation unless one with the same tags, time and timeEnd already exists.
func (c *Client) Post(ctx context.Context, a telemetry.Annotation) (Result, error) {
	want := annotation{
		Time:    a.Time.UnixMilli(),
		TimeEnd: a.TimeEnd.UnixMilli(),
		Tags:    a.Tags,
		Text:    a.Text,
	}
	existing, err := c.find(ctx, want)
	if err != nil {
		return Result{}, err
	}
	if existing != nil {
		return Result{ID: existing.ID, Created: false}, nil
	}
	body, err := json.Marshal(want)
	if err != nil {
		return Result{}, err
	}
	var resp struct {
		ID      int64  `json:"id"`
		Message string `json:"message"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/annotations", body, &resp); err != nil {
		return Result{}, fmt.Errorf("create annotation: %w", err)
	}
	return Result{ID: resp.ID, Created: true}, nil
}

// find lists organisation annotations that carry every tag and overlap the run's time, then
// keeps the one whose time and timeEnd match exactly.
func (c *Client) find(ctx context.Context, want annotation) (*annotation, error) {
	q := url.Values{}
	q.Set("type", "annotation")
	q.Set("from", strconv.FormatInt(want.Time, 10))
	q.Set("to", strconv.FormatInt(want.TimeEnd, 10))
	q.Set("limit", "100")
	for _, t := range want.Tags {
		q.Add("tags", t)
	}
	var found []annotation
	if err := c.do(ctx, http.MethodGet, "/api/annotations?"+q.Encode(), nil, &found); err != nil {
		return nil, fmt.Errorf("list annotations: %w", err)
	}
	for i := range found {
		f := &found[i]
		if f.Time == want.Time && f.TimeEnd == want.TimeEnd && hasAllTags(f.Tags, want.Tags) {
			return f, nil
		}
	}
	return nil, nil
}

func hasAllTags(have, want []string) bool {
	set := make(map[string]bool, len(have))
	for _, t := range have {
		set[t] = true
	}
	for _, t := range want {
		if !set[t] {
			return false
		}
	}
	return true
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// retryable says whether a call is worth another try. A 429 means Grafana did not act on the call, so
// every method may repeat it. A 5xx may arrive after Grafana stored an annotation, so only a read
// repeats on it. A repeated create could leave a duplicate.
func retryable(method string, status int) bool {
	if status == http.StatusTooManyRequests {
		return true
	}
	return status >= 500 && method == http.MethodGet
}

// retryAfter reads a Retry-After header, which holds seconds or an HTTP date. It returns false when
// the header is missing or unreadable.
func (c *Client) retryAfter(h string) (time.Duration, bool) {
	h = strings.TrimSpace(h)
	if h == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(h); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(h); err == nil {
		return max(t.Sub(c.Now()), 0), true
	}
	return 0, false
}

// do sends a request. It spaces calls by MinInterval and repeats a call that gets a 429 (or a 5xx for a read),
// waiting for Retry-After when Grafana sends it and for a doubling backoff when it does not.
func (c *Client) do(ctx context.Context, method, path string, body []byte, out any) error {
	wait := c.Backoff
	for attempt := 0; ; attempt++ {
		if gap := c.MinInterval - c.Now().Sub(c.last); !c.last.IsZero() && gap > 0 {
			if err := c.Sleep(ctx, gap); err != nil {
				return err
			}
		}
		status, header, err := c.once(ctx, method, path, body, out)
		c.last = c.Now()
		if err == nil || status == 0 || !retryable(method, status) || attempt >= c.Retries {
			if err != nil && attempt > 0 {
				err = fmt.Errorf("%w (after %d retries)", err, attempt)
			}
			return err
		}
		d := wait
		if ra, ok := c.retryAfter(header.Get("Retry-After")); ok {
			d = ra
		}
		wait *= 2
		if err := c.Sleep(ctx, min(d, maxWait)); err != nil {
			return err
		}
	}
}

// once sends one request. It returns the HTTP status, 0 when none arrived, and the response headers.
func (c *Client) once(ctx context.Context, method, path string, body []byte, out any) (int, http.Header, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp.StatusCode, resp.Header, fmt.Errorf("%s %s: HTTP %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return resp.StatusCode, resp.Header, nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return 0, nil, fmt.Errorf("%s %s: bad JSON in response: %w", method, path, err)
	}
	return resp.StatusCode, resp.Header, nil
}
