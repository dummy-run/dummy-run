package page_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dummy-run/dummy-run/internal/page"
)

// hitTestPage has the two traps seen on a real site (#180). A modal dialog sits over a list, and both have a
// button named Delete; the list's comes first in the document. A Send button sits under a fixed notice bar
// that covers all of it. Each button records its click in the URL hash.
const hitTestPage = `<!doctype html><html lang="en"><title>Hit test</title>
<style>
  body { margin: 0; font: 16px sans-serif; }
  .overlay { position: fixed; top: 0; left: 0; right: 0; height: 600px; background: rgba(0,0,0,.5); }
  .dialog { position: fixed; top: 300px; left: 50px; width: 400px; padding: 20px; background: #fff; }
  .send { position: absolute; top: 700px; left: 20px; width: 120px; height: 40px; }
  .bar { position: fixed; top: 680px; left: 0; right: 0; height: 120px; background: #eee; }
</style>
<main>
  <h1>Children</h1>
  <ul><li>Mia <button style="position:absolute;top:320px;left:120px" onclick="location.hash='list-delete'">Delete</button></li></ul>
  <button class="send" onclick="location.hash='sent'">Send message</button>
</main>
<div class="bar">See our privacy policy. <button onclick="this.parentNode.remove()">Accept</button></div>
<div class="overlay"></div>
<div class="dialog" role="dialog" aria-modal="true" aria-labelledby="t">
  <h2 id="t">Delete child profile</h2>
  <button onclick="location.hash='dialog-delete'">Delete</button>
  <button onclick="location.hash='cancel'">Cancel</button>
</div>
</html>`

// TestClickHitsWhatIsOnTop checks that a click by name goes to the control a person could press: the one in an
// open modal dialog rather than one behind its overlay, and never a control something else covers.
func TestClickHitsWhatIsOnTop(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, hitTestPage)
	}))
	t.Cleanup(srv.Close)
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx := context.Background()
			octx, cancel := context.WithTimeout(ctx, 90*time.Second)
			p, err := page.Open(octx, page.Options{Browser: browser, Headless: true, Width: 1000, Height: 800,
				Hosts: []string{"127.0.0.1"}})
			cancel()
			if err != nil {
				t.Fatalf("open %s: %v", browser, err)
			}
			t.Cleanup(p.Close)
			if o := p.Visit(ctx, srv.URL+"/"); o.Error != "" {
				t.Fatalf("visit: %s", o.Error)
			}

			// The dialog's Delete wins over the list's, which the overlay covers.
			o := p.Click(ctx, "Delete")
			if o.Error != "" {
				t.Fatalf("click Delete: %s", o.Error)
			}
			if !strings.HasSuffix(o.URL, "#dialog-delete") {
				t.Errorf("click Delete went to %q, want the dialog's button (#dialog-delete)", o.URL)
			}

			// The Send button is wholly under the notice bar. The click is refused, the error names what covers
			// it, and the button's handler never runs.
			o = p.Click(ctx, "Send message")
			if o.Error == "" || !strings.Contains(o.Error, "covered") || !strings.Contains(o.Error, "privacy policy") {
				t.Errorf("click on a covered Send: error %q, want one saying it is covered by the privacy notice", o.Error)
			}
			if strings.HasSuffix(o.URL, "#sent") {
				t.Errorf("the covered Send button was pressed: %q", o.URL)
			}
		})
	}
}
