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

// Client talks to one Grafana instance with a service account token.
type Client struct {
	base  string
	token string
	http  *http.Client
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
	return &Client{base: strings.TrimRight(baseURL, "/"), token: token, http: hc}, nil
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

func (c *Client) do(ctx context.Context, method, path string, body []byte, out any) error {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%s %s: HTTP %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s %s: bad JSON in response: %w", method, path, err)
	}
	return nil
}
