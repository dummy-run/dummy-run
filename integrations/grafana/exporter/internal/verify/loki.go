// Package verify reads back from Loki what an export sent, because a collector between the exporter
// and Loki answers 200 even when Loki rejected lines as too far behind the stream.
package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
// "user:password" for Grafana Cloud, or empty.
func NewLoki(baseURL, basicAuth string, hc *http.Client) (*Loki, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("loki URL %q must be absolute, such as http://localhost:3100", baseURL)
	}
	if basicAuth != "" && !strings.Contains(basicAuth, ":") {
		return nil, fmt.Errorf("LOKI_BASIC_AUTH must be user:password")
	}
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Loki{base: strings.TrimRight(baseURL, "/"), basicAuth: basicAuth, http: hc}, nil
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

// Newest returns the time of the newest line in the service's stream over the last 29 days, and
// false when the stream is empty.
func (l *Loki) Newest(ctx context.Context, serviceName string) (time.Time, bool, error) {
	now := time.Now()
	q := url.Values{}
	q.Set("query", fmt.Sprintf(`{service_name=%q}`, serviceName))
	q.Set("limit", "1")
	q.Set("direction", "backward")
	q.Set("start", strconv.FormatInt(now.Add(-29*24*time.Hour).UnixNano(), 10))
	q.Set("end", strconv.FormatInt(now.Add(time.Hour).UnixNano(), 10))
	data, err := l.get(ctx, "/loki/api/v1/query_range?"+q.Encode())
	if err != nil {
		return time.Time{}, false, err
	}
	var out struct {
		Data struct {
			Result []struct {
				Values [][]string `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return time.Time{}, false, fmt.Errorf("loki query_range: bad JSON: %w", err)
	}
	var newest time.Time
	for _, r := range out.Data.Result {
		for _, v := range r.Values {
			if len(v) < 1 {
				continue
			}
			ns, err := strconv.ParseInt(v[0], 10, 64)
			if err != nil {
				continue
			}
			if t := time.Unix(0, ns); t.After(newest) {
				newest = t
			}
		}
	}
	return newest, !newest.IsZero(), nil
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

// Count returns the number of log lines per run ID in the dummyrun stream between oldest and newest.
func (l *Loki) Count(ctx context.Context, serviceName string, oldest, newest time.Time) (map[string]int, error) {
	span := newest.Sub(oldest) + 2*time.Second
	q := url.Values{}
	q.Set("query", fmt.Sprintf(`sum by (dummyrun_run_id) (count_over_time({service_name=%q} [%ds]))`, serviceName, int64(span.Seconds())+1))
	q.Set("time", strconv.FormatInt(newest.Add(time.Second).UnixNano(), 10))
	data, err := l.get(ctx, "/loki/api/v1/query?"+q.Encode())
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("loki query: bad JSON: %w", err)
	}
	counts := map[string]int{}
	for _, r := range out.Data.Result {
		if len(r.Value) != 2 {
			continue
		}
		str, _ := r.Value[1].(string)
		n, err := strconv.ParseFloat(str, 64)
		if err != nil {
			continue
		}
		counts[r.Metric["dummyrun_run_id"]] = int(n)
	}
	return counts, nil
}

// Check compares the lines Loki holds with what was sent. It retries for up to wait, because the
// collector batches, and returns the runs that are still short, sorted by run ID.
func (l *Loki) Check(ctx context.Context, serviceName string, wants []Want, oldest, newest time.Time, wait time.Duration) ([]Shortfall, error) {
	deadline := time.Now().Add(wait)
	for {
		counts, err := l.Count(ctx, serviceName, oldest, newest)
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
