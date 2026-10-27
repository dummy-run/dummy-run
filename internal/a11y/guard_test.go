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
)

// TestSweepTabSendsNoWrites sweeps a page whose script sends a form as it loads and again when its button is
// pressed, the way the keyboard check presses it. Nothing but GET and HEAD may reach the server, the page's result
// counts what was stopped, and the page's ordinary GETs still load.
func TestSweepTabSendsNoWrites(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	var mu sync.Mutex
	seen := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.Method+" "+r.URL.Path]++
		mu.Unlock()
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Shop</title></head><body><main><h1>Shop</h1>
<p id="out">Ready.</p><div role="button" tabindex="0" id="buy" onclick="buy()">Buy now</div>
<img src="/pixel.gif" alt="A pixel"></main>
<script>
function buy() { fetch("/order", {method: "POST", body: "item=1"}).catch(function () {}); document.getElementById("out").textContent = "Ordered."; }
fetch("/visit", {method: "POST", body: "seen=1"}).catch(function () {});
</script></body></html>`)
		case "/pixel.gif":
			w.Header().Set("Content-Type", "image/gif")
			_, _ = w.Write([]byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff!\xf9\x04\x01\x00\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;"))
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			mu.Lock()
			clear(seen)
			mu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/"}, Browser: browser, Viewports: []Viewport{Desktop},
				Schemes: []string{Light}, Headless: true, Keyboard: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(rep.Results) != 1 || rep.Results[0].Error != "" {
				t.Fatalf("results: %+v", rep.Results)
			}
			r := rep.Results[0]
			mu.Lock()
			defer mu.Unlock()
			for k := range seen {
				if !strings.HasPrefix(k, "GET ") && !strings.HasPrefix(k, "HEAD ") {
					t.Errorf("%s reached the server: %v", k, seen)
				}
			}
			if seen["GET /pixel.gif"] == 0 {
				t.Errorf("the page's image never loaded, so the guard stopped too much: %v", seen)
			}
			if r.RequestsStopped < 2 {
				t.Errorf("requests_stopped is %d, want at least the load's POST and one press of the button", r.RequestsStopped)
			}
		})
	}
}
