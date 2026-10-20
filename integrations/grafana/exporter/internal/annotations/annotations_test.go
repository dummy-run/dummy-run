package annotations

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/integrations/grafana/exporter/internal/telemetry"
)

// fakeGrafana implements enough of /api/annotations to test creation and lookup.
type fakeGrafana struct {
	mu    sync.Mutex
	token string
	store []annotation
	posts int
	gets  int
	// requests counts every call, including those the token check refuses.
	requests int
}

func (f *fakeGrafana) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/annotations", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests++
		f.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+f.token {
			http.Error(w, `{"message":"invalid API key"}`, http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			f.gets++
			q := r.URL.Query()
			from, _ := strconv.ParseInt(q.Get("from"), 10, 64)
			to, _ := strconv.ParseInt(q.Get("to"), 10, 64)
			var out []annotation
			for _, a := range f.store {
				// Grafana returns annotations that overlap the window and carry every tag.
				if a.TimeEnd < from || a.Time > to || !hasAllTags(a.Tags, q["tags"]) {
					continue
				}
				out = append(out, a)
			}
			if out == nil {
				out = []annotation{}
			}
			_ = json.NewEncoder(w).Encode(out)
		case http.MethodPost:
			f.posts++
			var a annotation
			if err := json.NewDecoder(r.Body).Decode(&a); err != nil || a.Text == "" {
				http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
				return
			}
			a.ID = int64(len(f.store) + 1)
			f.store = append(f.store, a)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "Annotation added", "id": a.ID})
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	})
	return mux
}

func sample() telemetry.Annotation {
	start := time.Date(2026, 10, 16, 19, 33, 44, 0, time.UTC)
	return telemetry.Annotation{
		Time:    start,
		TimeEnd: start.Add(42535 * time.Millisecond),
		Tags:    []string{"dummyrun", "persona:ada-editor", "scenario:reassign-draft", "env:pr-41", "outcome:blocked", "findings"},
		Text:    "ada-editor / reassign-draft: blocked, 1 confirmed finding",
	}
}

func TestPostIsIdempotent(t *testing.T) {
	fake := &fakeGrafana{token: "glsa_test"}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()
	c, err := newFast(srv.URL+"/", "glsa_test", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	a := sample()
	first, err := c.Post(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Created || first.ID != 1 {
		t.Errorf("first post = %+v, want created with id 1", first)
	}
	second, err := c.Post(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	if second.Created || second.ID != 1 {
		t.Errorf("second post = %+v, want found with id 1", second)
	}
	if fake.posts != 1 || len(fake.store) != 1 {
		t.Errorf("%d posts and %d stored, want 1 and 1", fake.posts, len(fake.store))
	}
	got := fake.store[0]
	if got.Time != a.Time.UnixMilli() || got.TimeEnd != a.TimeEnd.UnixMilli() || got.Text != a.Text || len(got.Tags) != 6 {
		t.Errorf("stored %+v", got)
	}
}

func TestPostDistinguishesRuns(t *testing.T) {
	fake := &fakeGrafana{token: "glsa_test"}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()
	c, err := newFast(srv.URL, "glsa_test", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	a := sample()
	if _, err := c.Post(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	// Same tags, one second later: a different run of the same persona and scenario.
	later := a
	later.Time = a.Time.Add(time.Second)
	later.TimeEnd = a.TimeEnd.Add(time.Second)
	res, err := c.Post(context.Background(), later)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Created {
		t.Error("an annotation at a different time must be created")
	}
	// Same time, fewer tags: a run without the findings tag.
	fewer := a
	fewer.Tags = a.Tags[:5]
	res, err = c.Post(context.Background(), fewer)
	if err != nil {
		t.Fatal(err)
	}
	if res.Created {
		t.Error("an annotation whose tags are a subset of an existing one at the same time is the same run; it must not be created twice")
	}
	if len(fake.store) != 2 {
		t.Errorf("%d stored, want 2", len(fake.store))
	}
}

func TestBadTokenIsReported(t *testing.T) {
	fake := &fakeGrafana{token: "glsa_test"}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()
	c, err := newFast(srv.URL, "wrong", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Post(context.Background(), sample())
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("got %v, want an HTTP 401 error", err)
	}
}

func TestNewRejectsBadInput(t *testing.T) {
	if _, err := New("localhost:3000", "t", nil); err == nil {
		t.Error("a URL without a scheme must be rejected")
	}
	if _, err := New("http://localhost:3000", "", nil); err == nil || !strings.Contains(err.Error(), "GRAFANA_TOKEN") {
		t.Errorf("an empty token must be rejected with a message naming GRAFANA_TOKEN, got %v", err)
	}
	if _, err := New("http://grafana.example.net", "glsa_x", nil); err == nil || !strings.Contains(err.Error(), "clear text") {
		t.Errorf("a token over plain http to a remote host must be rejected, got %v", err)
	}
	if _, err := New("https://grafana.example.net", "glsa_x", nil); err != nil {
		t.Errorf("https must be accepted: %v", err)
	}
}

// newFast returns a client that records its waits instead of sleeping.
func newFast(baseURL, token string, hc *http.Client) (*Client, error) {
	c, err := New(baseURL, token, hc)
	if err != nil {
		return nil, err
	}
	c.Sleep = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
	return c, nil
}

// flakyGrafana answers the first n POSTs with status and the headers, then behaves like fakeGrafana.
type flakyGrafana struct {
	fakeGrafana
	// method is the HTTP method that gets refused. It defaults to POST.
	method  string
	status  int
	header  map[string]string
	refuse  int
	refused int
}

func (f *flakyGrafana) handler() http.Handler {
	inner := f.fakeGrafana.handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := f.method
		if m == "" {
			m = http.MethodPost
		}
		if r.Method == m && (f.refuse < 0 || f.refused < f.refuse) {
			f.refused++
			for k, v := range f.header {
				w.Header().Set(k, v)
			}
			http.Error(w, `{"message":"slow down"}`, f.status)
			return
		}
		inner.ServeHTTP(w, r)
	})
}

func TestPostRetriesAfter429AndHonoursRetryAfter(t *testing.T) {
	fake := &flakyGrafana{fakeGrafana: fakeGrafana{token: "glsa_test"}, status: http.StatusTooManyRequests, header: map[string]string{"Retry-After": "7"}, refuse: 2}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()
	c, err := New(srv.URL, "glsa_test", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	var waits []time.Duration
	c.Sleep = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	res, err := c.Post(context.Background(), sample())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Created || len(fake.store) != 1 {
		t.Errorf("result %+v with %d stored, want one created", res, len(fake.store))
	}
	if fake.refused != 2 {
		t.Errorf("%d refusals, want 2", fake.refused)
	}
	sevens := 0
	for _, w := range waits {
		if w == 7*time.Second {
			sevens++
		}
	}
	if sevens != 2 {
		t.Errorf("waits %v, want two of 7s from Retry-After", waits)
	}
}

func TestPostBacksOffOn5xxWithoutRetryAfter(t *testing.T) {
	fake := &flakyGrafana{fakeGrafana: fakeGrafana{token: "glsa_test"}, method: http.MethodGet, status: http.StatusBadGateway, refuse: 3}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()
	c, _ := New(srv.URL, "glsa_test", srv.Client())
	c.MinInterval = 0
	var waits []time.Duration
	c.Sleep = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	if _, err := c.Post(context.Background(), sample()); err != nil {
		t.Fatal(err)
	}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
	if len(waits) != 3 || waits[0] != want[0] || waits[1] != want[1] || waits[2] != want[2] {
		t.Errorf("waits %v, want %v", waits, want)
	}
}

func TestPostGivesUpWhenGrafanaAlwaysSays429(t *testing.T) {
	fake := &flakyGrafana{fakeGrafana: fakeGrafana{token: "glsa_test"}, status: http.StatusTooManyRequests, refuse: -1}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()
	c, _ := newFast(srv.URL, "glsa_test", srv.Client())
	c.Retries = 3
	_, err := c.Post(context.Background(), sample())
	if err == nil || !strings.Contains(err.Error(), "HTTP 429") || !strings.Contains(err.Error(), "after 3 retries") {
		t.Fatalf("got %v, want an HTTP 429 error after 3 retries", err)
	}
	if fake.refused != 4 {
		t.Errorf("%d attempts, want 4", fake.refused)
	}
}

func TestPostDoesNotRetryOtherClientErrors(t *testing.T) {
	fake := &fakeGrafana{token: "glsa_test"}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()
	c, _ := newFast(srv.URL, "wrong", srv.Client())
	if _, err := c.Post(context.Background(), sample()); err == nil {
		t.Fatal("want an error")
	}
	// One listing call, refused by the token check and not repeated.
	if fake.requests != 1 {
		t.Errorf("the server saw %d requests, want 1", fake.requests)
	}
}

func TestPostDoesNotRepeatACreateAfter5xx(t *testing.T) {
	fake := &flakyGrafana{fakeGrafana: fakeGrafana{token: "glsa_test"}, status: http.StatusBadGateway, refuse: -1}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()
	c, _ := newFast(srv.URL, "glsa_test", srv.Client())
	if _, err := c.Post(context.Background(), sample()); err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("got %v, want an HTTP 502 error", err)
	}
	if fake.refused != 1 {
		t.Errorf("%d create calls, want 1 because a repeat could duplicate the annotation", fake.refused)
	}
}

func TestPostSpacesCalls(t *testing.T) {
	fake := &fakeGrafana{token: "glsa_test"}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()
	c, _ := New(srv.URL, "glsa_test", srv.Client())
	c.MinInterval = time.Hour
	var waits []time.Duration
	c.Sleep = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	if _, err := c.Post(context.Background(), sample()); err != nil {
		t.Fatal(err)
	}
	// The listing call goes first. The create call waits for the interval.
	if len(waits) != 1 || waits[0] <= 59*time.Minute {
		t.Errorf("waits %v, want one wait close to the interval", waits)
	}
}

func TestRetryAfterReadsSecondsAndDates(t *testing.T) {
	c, _ := New("https://grafana.example.net", "t", nil)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	c.Now = func() time.Time { return now }
	if d, ok := c.retryAfter("12"); !ok || d != 12*time.Second {
		t.Errorf("seconds: %v %v", d, ok)
	}
	if d, ok := c.retryAfter(now.Add(30 * time.Second).Format(http.TimeFormat)); !ok || d != 30*time.Second {
		t.Errorf("date: %v %v", d, ok)
	}
	if _, ok := c.retryAfter("soon"); ok {
		t.Error("an unreadable value must be ignored")
	}
}

func TestRetryStopsWhenTheContextEnds(t *testing.T) {
	fake := &flakyGrafana{fakeGrafana: fakeGrafana{token: "glsa_test"}, status: http.StatusTooManyRequests, refuse: -1}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()
	c, _ := New(srv.URL, "glsa_test", srv.Client())
	// Skip the spacing pause so the first wait is the retry backoff.
	c.MinInterval = 0
	ctx, cancel := context.WithCancel(context.Background())
	var waits int
	c.Sleep = func(ctx context.Context, _ time.Duration) error { waits++; cancel(); return ctx.Err() }
	if _, err := c.Post(ctx, sample()); err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want the context error", err)
	}
	if waits != 1 || fake.refused != 1 {
		t.Errorf("%d waits and %d refusals, want the backoff wait to end the retries", waits, fake.refused)
	}
}
