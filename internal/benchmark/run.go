package benchmark

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"

	"github.com/dummy-run/dummy-run/internal/a11y"
)

// Runner serves pages from memory and runs a sweep on them in a browser.
type Runner struct {
	Browser string
	Log     io.Writer

	mu    sync.Mutex
	pages map[string]string
	srv   *httptest.Server
}

// Start opens the local server. Close it when the runs are done.
func (r *Runner) Start() {
	r.pages = map[string]string{}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		body, ok := r.pages[req.URL.Path]
		r.mu.Unlock()
		if !ok {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, body)
	}))
}

// Close stops the local server.
func (r *Runner) Close() { r.srv.Close() }

// Rules serves the page, runs a sweep with the checks the options turn on, and returns the rules it reported on the
// desktop, light run. Reports are rule IDs from violations, needs-review, advice and candidates.
func (r *Runner) Rules(ctx context.Context, path, body string, keyboard, markup bool) ([]string, error) {
	r.mu.Lock()
	r.pages[path] = body
	r.mu.Unlock()
	rep, err := a11y.Run(ctx, a11y.Config{URLs: []string{r.srv.URL + path}, Browser: r.Browser,
		Viewports: []a11y.Viewport{a11y.Desktop}, Schemes: []string{a11y.Light}, Headless: true, Log: r.Log,
		Keyboard: keyboard, Markup: markup})
	if err != nil {
		return nil, err
	}
	if len(rep.Results) == 0 {
		return nil, fmt.Errorf("benchmark: no result for %s", path)
	}
	res := rep.Results[0]
	if res.Error != "" {
		return nil, fmt.Errorf("benchmark: %s: %s", path, res.Error)
	}
	seen := map[string]bool{}
	for _, group := range [][]a11y.Finding{res.Violations, res.NeedsReview, res.Advice, res.Candidates} {
		for _, f := range group {
			seen[f.Rule] = true
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}
