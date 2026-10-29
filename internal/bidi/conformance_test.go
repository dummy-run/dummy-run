package bidi_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

// browsersUnderTest reads DUMMYRUN_BROWSERS, such as "chrome,firefox". Without
// it the conformance suite is skipped, so unit tests run anywhere.
func browsersUnderTest(t *testing.T) []string {
	t.Helper()
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the conformance suite")
	}
	return strings.Split(v, ",")
}

const testPage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Conformance</title></head>
<body>
<h1 id="h">Hello</h1>
<nav><a id="link" href="/second">Second page</a></nav>
<input id="i" aria-label="Name">
<button id="b" onclick="console.error('boom'); fetch('/x.json').catch(()=>{}); document.getElementById('h').textContent='Clicked'">Go</button>
<form id="f" method="post" action="/submit"><input name="q" value="1"><button id="submit" type="submit">Submit</button></form>
<select id="s" aria-label="Colour"><option>Red</option><option>Green</option></select>
<script>
document.getElementById('i').addEventListener('keydown', (e) => { if (e.key === 'Enter') document.getElementById('h').textContent = 'Entered'; });
document.addEventListener('keydown', (e) => { if (e.key.length === 1) document.body.dataset.chord = [e.ctrlKey && 'Control', e.altKey && 'Alt', e.shiftKey && 'Shift', e.metaKey && 'Meta', e.code].filter(Boolean).join('+'); });
</script>
</body></html>`

type recorder struct {
	mu   sync.Mutex
	seen []*http.Request
}

func (r *recorder) record(req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := req.Clone(context.Background())
	r.seen = append(r.seen, c)
}

func (r *recorder) find(method, path string) *http.Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, req := range r.seen {
		if req.Method == method && req.URL.Path == path {
			return req
		}
	}
	return nil
}

func testServer(t *testing.T) (*httptest.Server, *recorder) {
	t.Helper()
	rec := &recorder{}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		switch r.URL.Path {
		case "/x.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"ok":true}`)
		case "/second":
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, `<!doctype html><title>Second</title><h1>Second</h1>`)
		case "/submit":
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, `<!doctype html><title>Submitted</title><h1>Submitted</h1>`)
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, testPage)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, rec
}

func launch(t *testing.T, browser string) *bidi.Browser {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	b, err := bidi.Launch(ctx, bidi.LaunchOptions{Browser: browser, Headless: true})
	if err != nil {
		t.Fatalf("launch %s: %v", browser, err)
	}
	// Cleanups run last in, first out, so this check runs after Close.
	t.Cleanup(func() {
		for _, d := range b.ProfileDirs() {
			if _, err := os.Stat(d); err == nil {
				t.Errorf("profile %s outlived its test", d)
			}
		}
		if b.Alive() {
			t.Errorf("%s still has a running process after Close", browser)
		}
	})
	t.Cleanup(b.Close)
	return b
}

func TestLaunchAndCloseLeaveNothingBehind(t *testing.T) {
	for _, browser := range browsersUnderTest(t) {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			b, err := bidi.Launch(ctx, bidi.LaunchOptions{Browser: browser, Headless: true})
			if err != nil {
				t.Fatalf("launch %s: %v", browser, err)
			}
			dirs := b.ProfileDirs()
			if len(dirs) == 0 || !b.Alive() {
				t.Fatalf("expected a running browser with a profile, got alive=%v dirs=%v", b.Alive(), dirs)
			}
			before, err := bidi.FindLeftovers()
			if err != nil {
				t.Fatal(err)
			}
			b.Close()
			if b.Alive() {
				t.Error("a process is still running after Close")
			}
			for _, d := range dirs {
				if _, err := os.Stat(d); !os.IsNotExist(err) {
					t.Errorf("profile %s remains", d)
				}
			}
			after, err := bidi.FindLeftovers()
			if err != nil {
				t.Fatal(err)
			}
			// Other browsers may be running for other work, so look only for this one's profile.
			for _, p := range after.Processes {
				for _, d := range dirs {
					if strings.Contains(p.Args, d) {
						t.Errorf("process %d still names %s", p.PID, d)
					}
				}
			}
			if len(before.Processes) == 0 {
				t.Error("FindLeftovers did not see the running browser")
			}
		})
	}
}

func evalString(t *testing.T, c *bidi.Conn, cx, expr string) string {
	t.Helper()
	v, err := c.Evaluate(context.Background(), cx, expr)
	if err != nil {
		t.Fatalf("evaluate %s: %v", expr, err)
	}
	s, err := v.String()
	if err != nil {
		t.Fatalf("result of %s: %v", expr, err)
	}
	return s
}

func centre(t *testing.T, c *bidi.Conn, cx, id string) (float64, float64) {
	t.Helper()
	var box []float64
	err := c.EvaluateJSON(context.Background(), cx,
		fmt.Sprintf("(r=>[r.x+r.width/2, r.y+r.height/2])(document.getElementById(%q).getBoundingClientRect())", id), &box)
	if err != nil || len(box) != 2 {
		t.Fatalf("centre of %s: %v %v", id, box, err)
	}
	return box[0], box[1]
}

// waitFor polls cond for up to 5 seconds.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// TestConformance runs every BiDi command the engine uses on each browser.
// Each subtest is one row of the conformance matrix.
func TestConformance(t *testing.T) {
	for _, browser := range browsersUnderTest(t) {
		t.Run(browser, func(t *testing.T) {
			ctx := context.Background()
			b := launch(t, browser)
			c := b.Conn
			if (browser == "chrome") != (b.DebuggerAddress != "") {
				t.Errorf("debugger address %q for %s; only Chrome has one", b.DebuggerAddress, browser)
			}
			if b.Version == "" {
				t.Errorf("session reported no browserVersion")
			}
			srv, rec := testServer(t)
			host := "127.0.0.1"
			u, _ := url.Parse(srv.URL)

			var logs []bidi.LogEntry
			var responses []bidi.ResponseCompleted
			var fetchErrors []bidi.FetchError
			var mu sync.Mutex
			c.OnLogEntry(func(e bidi.LogEntry) { mu.Lock(); logs = append(logs, e); mu.Unlock() })
			c.OnResponseCompleted(func(e bidi.ResponseCompleted) { mu.Lock(); responses = append(responses, e); mu.Unlock() })
			c.OnFetchError(func(e bidi.FetchError) { mu.Lock(); fetchErrors = append(fetchErrors, e); mu.Unlock() })

			// The interceptor: GET, HEAD and OPTIONS carry extra headers, anything else fails.
			c.OnBeforeRequestSent(func(e bidi.BeforeRequestSent) {
				if !e.IsBlocked {
					return
				}
				go func() {
					ictx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					switch e.Request.Method {
					case http.MethodGet, http.MethodHead, http.MethodOptions:
						_ = c.ContinueRequest(ictx, e.Request.Request, e.Request.Headers, []bidi.Header{
							bidi.StringHeader("baggage", "dummyrun.run.id=r1,dummyrun.persona.id=p1"),
							bidi.StringHeader("traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"),
						})
					default:
						_ = c.FailRequest(ictx, e.Request.Request)
					}
				}()
			})

			var cx string
			t.Run("session.subscribe", func(t *testing.T) {
				if err := c.Subscribe(ctx, []string{"log.entryAdded", "network.beforeRequestSent",
					"network.responseCompleted", "network.fetchError"}); err != nil {
					t.Fatal(err)
				}
			})
			t.Run("browsingContext.create", func(t *testing.T) {
				var err error
				cx, err = c.CreateContext(ctx)
				if err != nil || cx == "" {
					t.Fatalf("create: %q %v", cx, err)
				}
			})
			t.Run("browsingContext.getTree", func(t *testing.T) {
				tree, err := c.GetTree(ctx)
				if err != nil || len(tree) == 0 {
					t.Fatalf("getTree: %v %v", tree, err)
				}
			})
			t.Run("browsingContext.setViewport", func(t *testing.T) {
				if err := c.SetViewport(ctx, cx, 1024, 700); err != nil {
					t.Fatal(err)
				}
			})
			t.Run("emulation.setUserAgentOverride", func(t *testing.T) {
				if err := c.SetUserAgentOverride(ctx, "ConformanceUA/1 DummyRun/test", []string{cx}); err != nil {
					t.Fatal(err)
				}
			})
			t.Run("network.addIntercept", func(t *testing.T) {
				if _, err := c.AddIntercept(ctx, []string{host}); err != nil {
					t.Fatal(err)
				}
			})
			t.Run("browsingContext.navigate", func(t *testing.T) {
				res, err := c.Navigate(ctx, cx, srv.URL+"/index.html")
				if err != nil {
					t.Fatal(err)
				}
				if !strings.HasPrefix(res.URL, srv.URL) {
					t.Errorf("navigated to %s", res.URL)
				}
			})
			t.Run("script.evaluate", func(t *testing.T) {
				var size []int
				if err := c.EvaluateJSON(ctx, cx, "[innerWidth, innerHeight]", &size); err != nil {
					t.Fatal(err)
				}
				if len(size) != 2 || size[0] != 1024 {
					t.Errorf("viewport %v, want width 1024", size)
				}
				ua := evalString(t, c, cx, "navigator.userAgent")
				if !strings.HasSuffix(ua, "DummyRun/test") {
					t.Errorf("user agent %q does not end with the override", ua)
				}
				if got := rec.find(http.MethodGet, "/index.html"); got == nil || got.Header.Get("User-Agent") != "ConformanceUA/1 DummyRun/test" {
					t.Errorf("server saw user agent %v", got)
				}
			})
			t.Run("network.continueRequest merges headers", func(t *testing.T) {
				req := rec.find(http.MethodGet, "/index.html")
				if req == nil {
					t.Fatal("server did not see the page request")
				}
				if req.Header.Get("baggage") != "dummyrun.run.id=r1,dummyrun.persona.id=p1" {
					t.Errorf("baggage %q", req.Header.Get("baggage"))
				}
				if !strings.HasPrefix(req.Header.Get("traceparent"), "00-0af7651916cd43dd8448eb211c80319c-") {
					t.Errorf("traceparent %q", req.Header.Get("traceparent"))
				}
				if req.Host != u.Host {
					t.Errorf("Host header %q, want %q (headers were replaced, not merged)", req.Host, u.Host)
				}
			})
			t.Run("network.responseCompleted", func(t *testing.T) {
				waitFor(t, "responseCompleted", func() bool {
					mu.Lock()
					defer mu.Unlock()
					for _, r := range responses {
						if strings.HasSuffix(r.Request.URL, "/index.html") && r.Response.Status == 200 {
							return true
						}
					}
					return false
				})
			})
			t.Run("browsingContext.locateNodes", func(t *testing.T) {
				nodes, err := c.LocateNodes(ctx, cx, "button#b", 5)
				if err != nil || len(nodes) != 1 {
					t.Fatalf("locateNodes: %v %v", nodes, err)
				}
				t.Run("script.callFunction with a node argument", func(t *testing.T) {
					var text string
					if err := c.CallFunctionJSON(ctx, cx, "(el, suffix) => el.textContent + suffix", &text, nodes[0], "!"); err != nil {
						t.Fatal(err)
					}
					if text != "Go!" {
						t.Errorf("callFunction returned %q", text)
					}
				})
			})
			t.Run("input.performActions click", func(t *testing.T) {
				x, y := centre(t, c, cx, "b")
				if err := c.Click(ctx, cx, x, y); err != nil {
					t.Fatal(err)
				}
				waitFor(t, "click to change the heading", func() bool {
					return evalString(t, c, cx, "document.getElementById('h').textContent") == "Clicked"
				})
			})
			t.Run("log.entryAdded", func(t *testing.T) {
				waitFor(t, "console.error", func() bool {
					mu.Lock()
					defer mu.Unlock()
					for _, l := range logs {
						if l.Level == "error" && strings.Contains(l.Text, "boom") {
							return true
						}
					}
					return false
				})
			})
			t.Run("input.performActions type and press", func(t *testing.T) {
				x, y := centre(t, c, cx, "i")
				if err := c.Click(ctx, cx, x, y); err != nil {
					t.Fatal(err)
				}
				if err := c.TypeText(ctx, cx, "abc"); err != nil {
					t.Fatal(err)
				}
				if got := evalString(t, c, cx, "document.getElementById('i').value"); got != "abc" {
					t.Errorf("typed %q", got)
				}
				if err := c.PressKey(ctx, cx, bidi.KeyEnter); err != nil {
					t.Fatal(err)
				}
				waitFor(t, "Enter to be seen", func() bool {
					return evalString(t, c, cx, "document.getElementById('h').textContent") == "Entered"
				})
			})
			t.Run("input.performActions holds modifiers", func(t *testing.T) {
				x, y := centre(t, c, cx, "b")
				if err := c.MoveMouse(ctx, cx, x, y); err != nil {
					t.Fatal(err)
				}
				if _, err := c.Evaluate(ctx, cx, "document.getElementById('b').focus()"); err != nil {
					t.Fatal(err)
				}
				mods, key, err := bidi.Chord("Shift+Tab")
				if err != nil {
					t.Fatal(err)
				}
				if err := c.PressChord(ctx, cx, mods, key); err != nil {
					t.Fatal(err)
				}
				waitFor(t, "Shift+Tab to move focus back to the field", func() bool {
					return evalString(t, c, cx, "document.activeElement.id") == "i"
				})
				if mods, key, err = bidi.Chord("Control+Alt+k"); err != nil {
					t.Fatal(err)
				}
				if err := c.PressChord(ctx, cx, mods, key); err != nil {
					t.Fatal(err)
				}
				waitFor(t, "Control and Alt to be held with k", func() bool {
					return evalString(t, c, cx, "document.body.dataset.chord || ''") == "Control+Alt+KeyK"
				})
				if err := c.PressKey(ctx, cx, "x"); err != nil {
					t.Fatal(err)
				}
				waitFor(t, "both modifiers to be let go before the next key", func() bool {
					return evalString(t, c, cx, "document.body.dataset.chord || ''") == "KeyX"
				})
			})
			t.Run("browsingContext.captureScreenshot", func(t *testing.T) {
				data, err := c.CaptureScreenshot(ctx, cx)
				if err != nil {
					t.Fatal(err)
				}
				png, err := base64.StdEncoding.DecodeString(data)
				if err != nil || len(png) < 8 || string(png[1:4]) != "PNG" {
					t.Fatalf("not a PNG: %v (%d bytes)", err, len(png))
				}
			})
			t.Run("network.failRequest blocks a POST", func(t *testing.T) {
				x, y := centre(t, c, cx, "submit")
				if err := c.Click(ctx, cx, x, y); err != nil {
					t.Fatal(err)
				}
				waitFor(t, "the POST to fail", func() bool {
					mu.Lock()
					defer mu.Unlock()
					for _, e := range fetchErrors {
						if e.Request.Method == http.MethodPost {
							return true
						}
					}
					return false
				})
				time.Sleep(300 * time.Millisecond)
				if rec.find(http.MethodPost, "/submit") != nil {
					t.Fatal("the server received the POST; the read-only policy did not hold")
				}
			})
			t.Run("browsingContext.traverseHistory", func(t *testing.T) {
				if _, err := c.Navigate(ctx, cx, srv.URL+"/index.html"); err != nil {
					t.Fatal(err)
				}
				if _, err := c.Navigate(ctx, cx, srv.URL+"/second"); err != nil {
					t.Fatal(err)
				}
				if err := c.TraverseHistory(ctx, cx, -1); err != nil {
					t.Fatal(err)
				}
				// Chrome replaces the realm during the traversal, so an evaluate can
				// briefly fail with "cannot find context"; the poll rides that out.
				waitFor(t, "back to land on index", func() bool {
					v, err := c.Evaluate(ctx, cx, "location.pathname")
					if err != nil {
						return false
					}
					s, _ := v.String()
					return strings.HasSuffix(s, "/index.html")
				})
			})
			t.Run("browsingContext.close", func(t *testing.T) {
				if err := c.CloseContext(ctx, cx); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}

// TestSetViewportDevicePixelRatio checks the part of browsingContext.setViewport
// the zoom setting depends on: a smaller viewport with a larger
// devicePixelRatio lays the page out in fewer CSS pixels, keeps doing so after
// a navigation, and paints each CSS pixel as that many screenshot pixels.
func TestSetViewportDevicePixelRatio(t *testing.T) {
	for _, browser := range browsersUnderTest(t) {
		t.Run(browser, func(t *testing.T) {
			ctx := context.Background()
			c := launch(t, browser).Conn
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				_, _ = io.WriteString(w, `<!doctype html><title>Zoom</title><style>
body{margin:0} #w{width:100px;height:50px;background:#c00}
#narrow{display:none} @media (max-width: 500px){#narrow{display:block}}</style>
<div id="w"></div><p id="narrow">narrow layout</p>`)
			}))
			t.Cleanup(srv.Close)
			cx, err := c.CreateContext(ctx)
			if err != nil {
				t.Fatal(err)
			}
			read := func() (w, h int, dpr float64, narrow bool, shotW, shotH int) {
				t.Helper()
				var got struct {
					W, H   int
					Dpr    float64
					Narrow bool
				}
				if err := c.EvaluateJSON(ctx, cx, `({W: innerWidth, H: innerHeight, Dpr: devicePixelRatio,
					Narrow: getComputedStyle(document.getElementById('narrow')).display === 'block'})`, &got); err != nil {
					t.Fatal(err)
				}
				data, err := c.CaptureScreenshot(ctx, cx)
				if err != nil {
					t.Fatal(err)
				}
				png, _ := base64.StdEncoding.DecodeString(data)
				cfg, err := pngConfig(png)
				if err != nil {
					t.Fatal(err)
				}
				return got.W, got.H, got.Dpr, got.Narrow, cfg.Width, cfg.Height
			}
			if _, err := c.Navigate(ctx, cx, srv.URL); err != nil {
				t.Fatal(err)
			}
			// 200 percent on a 1000 by 800 window: 500 by 400 CSS pixels at a ratio of 2.
			if err := c.SetViewportScaled(ctx, cx, 500, 400, 2); err != nil {
				t.Fatalf("setViewport with devicePixelRatio: %v", err)
			}
			w, h, dpr, narrow, sw, sh := read()
			if w != 500 || h != 400 || dpr != 2 || !narrow {
				t.Errorf("after setViewport: innerWidth %d innerHeight %d devicePixelRatio %v narrow media query %v", w, h, dpr, narrow)
			}
			if sw != 1000 || sh != 800 {
				t.Errorf("screenshot is %d by %d pixels, want 1000 by 800", sw, sh)
			}
			if _, err := c.Navigate(ctx, cx, srv.URL+"/again"); err != nil {
				t.Fatal(err)
			}
			if w, _, dpr, _, _, _ := read(); w != 500 || dpr != 2 {
				t.Errorf("after a navigation: innerWidth %d devicePixelRatio %v", w, dpr)
			}
			// Back to 100 percent.
			if err := c.SetViewportScaled(ctx, cx, 1000, 800, 0); err != nil {
				t.Fatalf("reset devicePixelRatio: %v", err)
			}
			if w, _, dpr, narrow, _, _ := read(); w != 1000 || dpr != 1 || narrow {
				t.Errorf("after reset: innerWidth %d devicePixelRatio %v narrow %v", w, dpr, narrow)
			}
		})
	}
}

func pngConfig(data []byte) (image.Config, error) {
	return png.DecodeConfig(bytes.NewReader(data))
}
