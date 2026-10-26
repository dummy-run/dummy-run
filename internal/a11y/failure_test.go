package a11y

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

// failurePage loads items with fetch, and more with XMLHttpRequest, and shows an error in msg when the fetch fails.
// Every load also tries three
// writes: a POST with fetch, a beacon and a PUT with XMLHttpRequest. live sets whether msg is an alert.
func failurePage(live bool) string {
	role := ""
	if live {
		role = ` role="alert"`
	}
	return fmt.Sprintf(`<!doctype html><html lang="en"><head><title>Items</title></head><body><main><h1>Items</h1>
<ul id="items"></ul><div id="msg"%s></div>
<script>
fetch("/api/items").then(r => { if (!r.ok) throw new Error(); return r.json(); })
  .then(items => { for (const i of items) { const li = document.createElement("li"); li.textContent = i; document.getElementById("items").append(li); } })
  .catch(() => { document.getElementById("msg").textContent = "We couldn't load the items. Try again later."; });
const g = new XMLHttpRequest(); g.open("GET", "/api/more"); g.onerror = () => {}; g.send();
fetch("/api/save", {method: "POST", body: "x"}).catch(() => {});
try { navigator.sendBeacon("/api/beacon", "x"); } catch (e) {}
const x = new XMLHttpRequest(); x.open("PUT", "/api/put"); x.onerror = () => {}; x.send("x");
</script></main></body></html>`, role)
}

// TestFailureProbeSendsNoWrites probes three pages and checks what reached the server: no request other than
// GET or HEAD, and the data request only on the normal load.
func TestFailureProbeSendsNoWrites(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	defer func(d time.Duration) { failureSettle = d }(failureSettle)
	failureSettle = 1500 * time.Millisecond
	var mu sync.Mutex
	var seen, agents []string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.Path)
		agents = append(agents, r.UserAgent())
		mu.Unlock()
		switch r.URL.Path {
		case "/announced", "/silent":
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, failurePage(r.URL.Path == "/announced"))
		case "/static":
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<!doctype html><html lang="en"><head><title>Static</title></head><body><link rel="prefetch" href="/next"><p>Hello</p></body></html>`)
		case "/api/items":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `["Tea","Cake"]`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	allowAll := func(string) (bool, string) { return true, "" }
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			// The browser is launched as the sweep launches it, with no intercept of its own beside the probe's.
			b, err := bidi.Launch(ctx, bidi.LaunchOptions{Browser: browser, Headless: true})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(b.Close)
			probe := func(path string) *FailureProbe {
				t.Helper()
				mu.Lock()
				seen = nil
				mu.Unlock()
				fp, err := probeFailures(ctx, b.Conn, srv.URL+path, Desktop, false, allowAll)
				if err != nil {
					t.Fatalf("%s: %v", path, err)
				}
				mu.Lock()
				defer mu.Unlock()
				items := 0
				for _, s := range seen {
					method, _, _ := strings.Cut(s, " ")
					if method != "GET" && method != "HEAD" {
						t.Errorf("%s: %s reached the server", path, s)
					}
					if s == "GET /api/items" {
						items++
					}
				}
				if want := map[bool]int{true: 1, false: 0}[path != "/static"]; items != want {
					t.Errorf("%s: /api/items reached the server %d times, want %d, on the normal load only (%v)", path, items, want, seen)
				}
				return fp
			}

			a := probe("/announced")
			if a.Failed != 2 || !a.Announced || !strings.Contains(a.Announcement, "couldn't load") || a.Stopped < 4 {
				t.Errorf("announced: %+v", a)
			}
			if f := failureFindings(a, "/announced", nil); len(f) != 0 {
				t.Errorf("announced page got advice: %+v", f)
			}
			s := probe("/silent")
			if s.Failed != 2 || s.Announced || !s.Changed {
				t.Errorf("silent: %+v", s)
			}
			if f := failureFindings(s, "/silent", nil); len(f) != 1 || f[0].Rule != RuleFailureSilent || !strings.Contains(f[0].Nodes[0].Detail, "no live region") {
				t.Errorf("silent page advice: %+v", f)
			}
			if st := probe("/static"); st.Failed != 0 || st.Changed || len(failureFindings(st, "/static", nil)) != 0 {
				t.Errorf("static: %+v", st)
			}

			// With --identify, every request from the probe's tab names the scanner.
			mu.Lock()
			agents = nil
			mu.Unlock()
			if _, err := probeFailures(ctx, b.Conn, srv.URL+"/static", Desktop, true, allowAll); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			for _, ua := range agents {
				if !strings.Contains(ua, "DummyRun/") {
					t.Errorf("a request from the probe's tab didn't identify itself: %q", ua)
				}
			}
			if len(agents) == 0 {
				t.Error("the identified probe made no requests")
			}
			seen = nil
			mu.Unlock()

			// A page robots.txt disallows never loads.
			refuse := func(string) (bool, string) { return false, "robots.txt disallows it" }
			if _, err := probeFailures(ctx, b.Conn, srv.URL+"/silent", Desktop, false, refuse); err == nil {
				t.Error("a disallowed page loaded")
			}
			mu.Lock()
			for _, s := range seen {
				// The tab before can still ask for its icon after it closes.
				if s != "GET /favicon.ico" {
					t.Errorf("a disallowed page reached the server: %v", seen)
					break
				}
			}
			mu.Unlock()
		})
	}
}

func TestFailureFindingsWording(t *testing.T) {
	f := failureFindings(&FailureProbe{Failed: 2}, "/", nil)
	if len(f) != 1 || f[0].Nodes[0].Detail != "2 data requests failed and the page's visible text didn't change" || f[0].WCAG[0] != "4.1.3" {
		t.Errorf("%+v", f)
	}
	allow, err := ParseAllowList(strings.NewReader("dummyrun-failure-silent | / | html | the page has no data to lose\n"))
	if err != nil {
		t.Fatal(err)
	}
	if f := failureFindings(&FailureProbe{Failed: 1}, "/", allow); !f[0].Nodes[0].Allowed {
		t.Errorf("the allow-list didn't accept it: %+v", f)
	}
	if f := failureFindings(&FailureProbe{Failed: 1, Changed: true}, "/", nil); !strings.HasPrefix(f[0].Nodes[0].Detail, "1 data request failed;") {
		t.Errorf("one request: %q", f[0].Nodes[0].Detail)
	}
	if cut("abcdef", 5) != "ab..." || cut("abc", 5) != "abc" {
		t.Error("cut")
	}
}

// TestFailureGuardIgnoresCancelledRequests checks a decision on a request the page already cancelled doesn't stop
// the probe, and any other refused decision does.
func TestFailureGuardIgnoresCancelledRequests(t *testing.T) {
	var g failureGuard
	g.record(&bidi.CommandError{Method: "network.failRequest", Code: "no such request", Message: "gone"})
	if g.err != nil {
		t.Errorf("a cancelled request was recorded: %v", g.err)
	}
	g.record(&bidi.CommandError{Method: "network.failRequest", Code: "invalid argument", Message: "bad"})
	if g.err == nil {
		t.Error("a refused decision wasn't recorded")
	}
}

// TestIsDataRequest checks which requests the failing load fails: fetch and XMLHttpRequest, and a request with no
// destination when the browser doesn't say what started it, but never a prefetch or a page's resources.
func TestIsDataRequest(t *testing.T) {
	prefetch := []bidi.Header{{Name: "Sec-Purpose", Value: bidi.BytesValue{Type: "string", Value: "prefetch"}}}
	for _, c := range []struct {
		r    bidi.RequestData
		want bool
	}{
		{bidi.RequestData{InitiatorType: "fetch"}, true},
		{bidi.RequestData{InitiatorType: "xmlhttprequest"}, true},
		{bidi.RequestData{}, true},
		{bidi.RequestData{InitiatorType: "link"}, false},
		{bidi.RequestData{InitiatorType: "img", Destination: "image"}, false},
		{bidi.RequestData{Destination: "image"}, false},
		{bidi.RequestData{Headers: prefetch}, false},
	} {
		if got := isDataRequest(c.r); got != c.want {
			t.Errorf("%+v: %v, want %v", c.r, got, c.want)
		}
	}
}
