package a11y

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
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
