package page_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/dummy-run/dummy-run/internal/page"
)

// The page sends socket.io packets as a real client would: as WebSocket frames, and as HTTP long-polling bodies by
// XHR and by fetch. The packets cover the forms socket.io writes: a plain event, one in a namespace, one with an
// acknowledgement id and a binary event, next to packets that are not events at all.
const socketPage = `<!doctype html><html lang="en"><title>Chat</title><main><h1>Chat</h1>
<button id="go" onclick="go()">Send</button><p id="state">idle</p></main>
<script>
async function go() {
  const ws = new WebSocket(location.origin.replace('http', 'ws') + '/socket.io/?EIO=4&transport=websocket');
  await new Promise(r => ws.onopen = r);
  for (const f of [
    '42["feedback:submit",{"rating":1}]',
    '42["chat:message","hello"]',
    '42/admin,["message:feedback",1]',
    '4217["feedback:pilot","text"]',
    '451-["feedback:survey",{"_placeholder":true,"num":0}]',
    '42["feedbackx"]',
    '2',
    'not socket.io',
  ]) ws.send(f);
  ws.send('42["done"]');
  const x = new XMLHttpRequest();
  x.open('POST', '/socket.io/?EIO=4&transport=polling&sid=1');
  x.send('42["chat:message","a"]\x1e42["feedback:submit",1]\x1e3');
  await new Promise(r => x.onloadend = r);
  await fetch('/socket.io/?EIO=4&transport=polling&sid=2', {method: 'POST', body: '42["message:feedback",1]'});
  await fetch('/socket.io/?EIO=4&transport=polling&sid=3', {method: 'POST', body: '42["chat:message","b"]'});
  document.getElementById('state').textContent = 'sent';
}
</script></html>`

type socketRecorder struct {
	mu      sync.Mutex
	frames  []string
	polling map[string]string
}

func (s *socketRecorder) snapshot() ([]string, map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := map[string]string{}
	for k, v := range s.polling {
		m[k] = v
	}
	return slices.Clone(s.frames), m
}

func TestSocketEventsAreDropped(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			rec := &socketRecorder{polling: map[string]string{}}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/":
					w.Header().Set("Content-Type", "text/html")
					_, _ = io.WriteString(w, socketPage)
				case r.URL.Query().Get("transport") == "websocket":
					c, err := websocket.Accept(w, r, nil)
					if err != nil {
						return
					}
					defer c.CloseNow()
					for {
						_, data, err := c.Read(r.Context())
						if err != nil {
							return
						}
						rec.mu.Lock()
						rec.frames = append(rec.frames, string(data))
						rec.mu.Unlock()
					}
				case r.URL.Query().Get("transport") == "polling":
					body, _ := io.ReadAll(r.Body)
					rec.mu.Lock()
					rec.polling[r.URL.Query().Get("sid")] = string(body)
					rec.mu.Unlock()
					_, _ = io.WriteString(w, "ok")
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(srv.Close)

			ctx := context.Background()
			octx, cancel := context.WithTimeout(ctx, 90*time.Second)
			p, err := page.Open(octx, page.Options{Browser: browser, Headless: true, Width: 1200, Height: 800,
				Hosts: []string{"127.0.0.1"}, BlockSocketEvents: []string{"message:feedback", "feedback:*"}})
			cancel()
			if err != nil {
				t.Fatalf("open %s: %v", browser, err)
			}
			t.Cleanup(p.Close)
			if o := p.Visit(ctx, srv.URL+"/"); o.Error != "" {
				t.Fatalf("visit: %s", o.Error)
			}
			o := p.Click(ctx, "Send")
			deadline := time.Now().Add(10 * time.Second)
			for {
				frames, polls := rec.snapshot()
				if (slices.Contains(frames, `42["done"]`) && len(polls) == 3) || time.Now().After(deadline) {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			blocked := slices.Concat(o.Blocked, p.Look(ctx).Blocked)

			frames, polls := rec.snapshot()
			want := []string{`42["chat:message","hello"]`, `42["feedbackx"]`, `2`, `not socket.io`, `42["done"]`}
			if !slices.Equal(frames, want) {
				t.Errorf("the server got these frames:\n%q\nwant\n%q", frames, want)
			}
			if got := polls["1"]; got != "42[\"chat:message\",\"a\"]\x1e3" {
				t.Errorf("XHR polling body: %q", got)
			}
			if got, ok := polls["2"]; !ok || strings.Contains(got, "feedback") {
				t.Errorf("fetch polling body with only a blocked event: %q (sent %v)", got, ok)
			}
			if got := polls["3"]; got != `42["chat:message","b"]` {
				t.Errorf("fetch polling body with no blocked event: %q", got)
			}
			all := strings.Join(blocked, "\n")
			for _, name := range []string{"feedback:submit", "message:feedback", "feedback:pilot", "feedback:survey"} {
				if !strings.Contains(all, `socket.io event "`+name+`"`) {
					t.Errorf("the dropped event %s is not reported; blocked: %q", name, blocked)
				}
			}
		})
	}
}
