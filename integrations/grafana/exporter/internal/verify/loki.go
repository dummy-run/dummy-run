// Package verify reads back from Loki what an export sent, because a collector between the exporter
// and Loki answers 200 even when Loki rejected lines as too far behind the stream.
package verify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Loki queries one Loki instance.
type Loki struct {
	base      string
	basicAuth string
	http      *http.Client
}

// NewLoki returns a client for the Loki at baseURL, such as http://localhost:3100. basicAuth is
// "user:password" for Grafana Cloud, or empty. Credentials travel only over HTTPS or to the loopback
// interface.
func NewLoki(baseURL, basicAuth string, hc *http.Client) (*Loki, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("loki URL %q must be absolute, such as http://localhost:3100", baseURL)
	}
	if basicAuth != "" && !strings.Contains(basicAuth, ":") {
		return nil, errors.New("LOKI_BASIC_AUTH must be user:password")
	}
	if basicAuth != "" {
		if err := RequireSecureTransport(u, "LOKI_BASIC_AUTH"); err != nil {
			return nil, err
		}
	}
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second, CheckRedirect: RefuseInsecureRedirect}
	}
	return &Loki{base: strings.TrimRight(baseURL, "/"), basicAuth: basicAuth, http: hc}, nil
}

// RequireSecureTransport rejects a plain HTTP URL on anything but the loopback interface when a
// credential named by secret would be sent to it.
func RequireSecureTransport(u *url.URL, secret string) error {
	if u.Scheme == "https" {
		return nil
	}
	host := u.Hostname()
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("%s would be sent in clear text to %s; use https, or http only on localhost", secret, u.Redacted())
}

// RefuseInsecureRedirect stops a redirect from HTTPS to plain HTTP, which would otherwise carry the
// Authorization header along.
func RefuseInsecureRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if req.URL.Scheme == "http" && via[0].URL.Scheme == "https" {
		return fmt.Errorf("refusing redirect from %s to plain http", via[0].URL.Redacted())
	}
	return nil
}

// Want is how many log lines one run should have in Loki.
type Want struct {
	RunID string
	Lines int
}

// Shortfall is a run with fewer lines in Loki than were sent.
type Shortfall struct {
	RunID string
	Got   int
	Want  int
}

func (s Shortfall) String() string {
	return fmt.Sprintf("run %s: %d of %d log lines in Loki", s.RunID, s.Got, s.Want)
}

// get performs an authenticated GET and returns the body of a 200 response.
func (l *Loki) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.base+path, nil)
	if err != nil {
		return nil, err
	}
	if l.basicAuth != "" {
		user, pass, _ := strings.Cut(l.basicAuth, ":")
		req.SetBasicAuth(user, pass)
	}
	resp, err := l.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("loki %s: HTTP %d: %s", strings.SplitN(path, "?", 2)[0], resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}

// countChunk bounds the run IDs per query, which also bounds the series Loki must return. Loki caps a
// metric query at max_query_series, 500 by default.
const countChunk = 200

// countWindow bounds the range of one count query, well inside Loki's max_query_length of 30 days.
const countWindow = 7 * 24 * time.Hour

// Count returns the number of log lines per run ID in the service's stream between oldest and
// newest, for the runs named. Runs without lines are absent from the result. The span is cut into
// windows of countWindow, and each window queries at most countChunk runs.
func (l *Loki) Count(ctx context.Context, serviceName string, runIDs []string, oldest, newest time.Time) (map[string]int, error) {
	counts := map[string]int{}
	// Loki's range vector covers (end-range, end], so adjoining windows do not overlap.
	for start := oldest.Add(-time.Second); start.Before(newest.Add(time.Second)); start = start.Add(countWindow) {
		end := start.Add(countWindow)
		if end.After(newest.Add(time.Second)) {
			end = newest.Add(time.Second)
		}
		for i := 0; i < len(runIDs); i += countChunk {
			if err := l.countInto(ctx, counts, serviceName, runIDs[i:min(i+countChunk, len(runIDs))], start, end); err != nil {
				return nil, err
			}
		}
	}
	return counts, nil
}

// countInto adds the lines in (start, end] per run to counts.
func (l *Loki) countInto(ctx context.Context, counts map[string]int, serviceName string, runIDs []string, start, end time.Time) error {
	quoted := make([]string, len(runIDs))
	for i, id := range runIDs {
		quoted[i] = regexp.QuoteMeta(id)
	}
	q := url.Values{}
	q.Set("query", fmt.Sprintf(`sum by (dummyrun_run_id) (count_over_time({service_name=%q} | dummyrun_run_id=~%q [%ds]))`,
		serviceName, strings.Join(quoted, "|"), int64(end.Sub(start).Seconds())))
	q.Set("time", strconv.FormatInt(end.UnixNano(), 10))
	data, err := l.get(ctx, "/loki/api/v1/query?"+q.Encode())
	if err != nil {
		return err
	}
	var out struct {
		Data struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
				Value  []any             `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return fmt.Errorf("loki query: bad JSON: %w", err)
	}
	for _, r := range out.Data.Result {
		if len(r.Value) != 2 {
			continue
		}
		str, _ := r.Value[1].(string)
		n, err := strconv.ParseFloat(str, 64)
		if err != nil {
			continue
		}
		counts[r.Metric["dummyrun_run_id"]] += int(n)
	}
	return nil
}

// Check compares the lines Loki holds with what was sent. It retries for up to wait, because the
// collector batches, and returns the runs that are still short, sorted by run ID.
func (l *Loki) Check(ctx context.Context, serviceName string, wants []Want, oldest, newest time.Time, wait time.Duration) ([]Shortfall, error) {
	deadline := time.Now().Add(wait)
	ids := make([]string, len(wants))
	for i, w := range wants {
		ids[i] = w.RunID
	}
	for {
		counts, err := l.Count(ctx, serviceName, ids, oldest, newest)
		if err != nil {
			return nil, err
		}
		var short []Shortfall
		for _, w := range wants {
			if counts[w.RunID] < w.Lines {
				short = append(short, Shortfall{RunID: w.RunID, Got: counts[w.RunID], Want: w.Lines})
			}
		}
		if len(short) == 0 || time.Now().After(deadline) {
			sort.Slice(short, func(i, j int) bool { return short[i].RunID < short[j].RunID })
			return short, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}
