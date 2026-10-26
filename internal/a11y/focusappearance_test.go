package a11y

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Each button has its own focus style. The ones starting "bad" have an indicator WCAG 1.4.11 or 2.4.13 would fail,
// and the ones starting "ok" have one that meets both. Two fade in, so the pictures must wait for them. The last
// draws its indicator with ::after, which only the pictures can see, and it comes after the walk has tabbed out
// of the page, which in Firefox takes focus from the document.
const focusPage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Focus</title>
<style>
body { background: #fff; color: #111; font: 16px sans-serif; }
button { display: block; margin: 24px; padding: 8px 16px; border: 1px solid #111; background: #fff; color: #111; outline: none; }
#ok-ring:focus { outline: 3px solid #005fcc; outline-offset: 2px; }
#ok-fade { transition: outline-color 0.3s; outline: 3px solid transparent; }
#ok-fade:focus { outline-color: #000; }
#bad-faint:focus { outline: 3px solid #e6e6e6; outline-offset: 2px; }
#bad-shade:focus { background: #f0f0f0; }
#bad-fade { transition: background-color 0.3s; }
#bad-fade:focus { background: #eeeeee; }
#bad-thin:focus { outline: 1px solid #000; }
#ok-wide { width: 700px; }
#ok-wide:focus { outline: 1px solid #000; }
#ok-after { position: relative; }
#ok-after:focus::after { content: ""; position: absolute; inset: -5px; border: 3px solid #000; }
</style></head><body><main><h1>Focus</h1>
<button id="ok-ring">Ring</button>
<button id="ok-fade">Fade in</button>
<button id="bad-faint">Faint</button>
<button id="bad-shade">Shade</button>
<button id="bad-fade">Fading shade</button>
<button id="bad-thin">Thin</button>
<button id="ok-wide">Too wide to measure</button>
<button id="ok-after">Drawn after</button>
</main></body></html>`

func TestFocusIndicatorContrastAndSize(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, focusPage)
	}))
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL}, Browser: browser, Viewports: []Viewport{Desktop},
				Schemes: []string{Light}, Headless: true, Keyboard: true})
			if err != nil {
				t.Fatal(err)
			}
			r := rep.Results[0]
			if r.Error != "" {
				t.Fatal(r.Error)
			}
			got := map[string][]string{}
			for _, f := range append(append([]Finding{}, r.Violations...), r.Advice...) {
				if f.Rule != RuleFocusLowContrast && f.Rule != RuleFocusAppearance && f.Rule != RuleFocusNotVisible {
					continue
				}
				for _, n := range f.Nodes {
					got[f.Rule] = append(got[f.Rule], n.Target)
				}
			}
			for _, f := range r.Advice {
				if f.Rule == RuleFocusLowContrast {
					t.Error("low contrast focus is a WCAG AA failure, not advice")
				}
			}
			for _, f := range r.Violations {
				if f.Rule == RuleFocusAppearance {
					t.Error("focus appearance is a AAA criterion, so advice, not a failure")
				}
			}
			want := map[string][]string{
				RuleFocusLowContrast: {"#bad-faint", "#bad-shade", "#bad-fade"},
				RuleFocusAppearance:  {"#bad-thin"},
			}
			if !reflect.DeepEqual(got, want) {
				for _, f := range append(append([]Finding{}, r.Violations...), r.Advice...) {
					for _, n := range f.Nodes {
						t.Logf("%s %s: %s", f.Rule, n.Target, n.Detail)
					}
				}
				t.Errorf("got  %v\nwant %v", got, want)
			}
		})
	}
}
