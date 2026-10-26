package a11y

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

// statePage is a page whose ids, link addresses and form token change on every load, as many sites' do. The query
// sets what changes the page itself: its text, a button's state or the text in a shadow root.
func statePage(r *http.Request) string {
	q := r.URL.Query()
	text, expanded, shadow := "Opening times", "false", "Inside"
	if q.Has("text") {
		text = "Closing times"
	}
	if q.Has("expanded") {
		expanded = "true"
	}
	if q.Has("shadow") {
		shadow = "Changed inside"
	}
	n := time.Now().UnixNano()
	return fmt.Sprintf(`<!doctype html><html lang="en"><head><title>State</title><style>p{color:#111}</style></head><body>
<main id="m%d"><h1>%s</h1>
<button aria-expanded="%s" aria-controls="menu%d">Menu</button><ul id="menu%d"><li><a href="/x?session=%d">Home</a></li></ul>
<form><input type="hidden" name="csrf" value="%d"><label for="q%d">Search</label><input id="q%d" type="search"></form>
<div id="host"></div>
<script>document.getElementById("host").attachShadow({mode: "open"}).innerHTML = "<p>%s</p>";</script>
</main></body></html>`, n, text, expanded, n, n, n, n, n, n, shadow)
}

// TestStateHashSeesThePageNotItsTokens sweeps the same page twice, with fresh ids and tokens each time, and three
// changed versions, and checks only the changed ones hash differently.
func TestStateHashSeesThePageNotItsTokens(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, statePage(r))
	}))
	defer srv.Close()
	urls := []string{srv.URL + "/?a", srv.URL + "/?b", srv.URL + "/?text", srv.URL + "/?expanded", srv.URL + "/?shadow"}
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: urls, Browser: browser, Viewports: []Viewport{Desktop}, Schemes: []string{Light}, Headless: true})
			if err != nil {
				t.Fatal(err)
			}
			hash := map[string]string{}
			for _, r := range rep.Results {
				if r.State == nil {
					t.Fatalf("%s: no state, error %q", r.URL, r.StateError)
				}
				if r.State.Elements < 10 || r.State.Partial || len(r.State.Hash) != 64 {
					t.Errorf("%s: state %+v", r.URL, r.State)
				}
				hash[strings.TrimPrefix(r.URL, srv.URL+"/?")] = r.State.Hash
			}
			if hash["a"] != hash["b"] {
				t.Error("the same page with new ids and tokens hashed differently")
			}
			for _, k := range []string{"text", "expanded", "shadow"} {
				if hash[k] == hash["a"] {
					t.Errorf("a page with a changed %s hashed the same", k)
				}
			}
		})
	}
}

// TestStateIsPartialPastTheLimit checks a page whose description passes the limit is hashed from its start.
func TestStateIsPartialPastTheLimit(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	defer func(n int) { maxStateChars = n }(maxStateChars)
	maxStateChars = 200
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		end := "same"
		if r.URL.Query().Has("end") {
			end = "different"
		}
		_, _ = fmt.Fprintf(w, `<!doctype html><html lang="en"><head><title>Long</title></head><body><main><h1>Long</h1><p>%s</p><p>%s</p></main></body></html>`,
			strings.Repeat("word ", 100), end)
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/", srv.URL + "/?end"}, Browser: browser, Viewports: []Viewport{Desktop}, Schemes: []string{Light}, Headless: true})
			if err != nil {
				t.Fatal(err)
			}
			a, b := rep.Results[0].State, rep.Results[1].State
			if a == nil || b == nil || !a.Partial || !b.Partial || a.Hash != b.Hash {
				t.Errorf("past the limit: %+v and %+v, want both partial with one hash", a, b)
			}
		})
	}
}

// TestStateErrorLeavesTheRulesRunning checks a state that can't be read is recorded and axe still runs.
func TestStateErrorLeavesTheRulesRunning(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	defer func(f func(context.Context, *bidi.Conn, string) (*PageState, error)) { readPageState = f }(readPageState)
	readPageState = func(context.Context, *bidi.Conn, string) (*PageState, error) {
		return nil, errors.New("the sandbox went away\nsecond line")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<!doctype html><html lang="en"><head><title>Image</title></head><body><main><h1>Image</h1><img src="data:image/gif;base64,R0lGODlhAQABAAAAACw="></main></body></html>`)
	}))
	defer srv.Close()
	browser := strings.Split(v, ",")[0]
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	rep, err := Run(ctx, Config{URLs: []string{srv.URL}, Browser: browser, Viewports: []Viewport{Desktop}, Schemes: []string{Light}, Headless: true})
	if err != nil {
		t.Fatal(err)
	}
	r := rep.Results[0]
	if r.State != nil || r.StateError != "the sandbox went away" || r.Error != "" || len(r.Violations) == 0 {
		t.Errorf("state %+v, state error %q, error %q, %d violations", r.State, r.StateError, r.Error, len(r.Violations))
	}
}
