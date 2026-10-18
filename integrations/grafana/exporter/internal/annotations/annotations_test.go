package annotations

import (
	"context"
	"encoding/json"
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
}

func (f *fakeGrafana) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/annotations", func(w http.ResponseWriter, r *http.Request) {
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
	c, err := New(srv.URL+"/", "glsa_test", srv.Client())
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
	c, err := New(srv.URL, "glsa_test", srv.Client())
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
	c, err := New(srv.URL, "wrong", srv.Client())
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
}
