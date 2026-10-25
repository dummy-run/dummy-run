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

	"github.com/dummy-run/dummy-run/internal/page"
)

// tasksPages have controls whose answer is known: some work by keyboard and by mouse, some only by mouse.
var tasksPages = map[string]string{
	"/tasks": `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Tasks</title></head><body><main>
<h1>Tasks</h1>
<button id="disc" aria-expanded="false" aria-controls="p1">Opening hours</button><div id="p1" hidden>Nine to five.</div>
<button id="down" aria-expanded="false">Delivery</button>
<div id="card" style="cursor:pointer">Show prices</div><div id="prices" hidden>From ten pounds.</div>
<div id="ok" role="button" tabindex="0" aria-expanded="false">Returns</div>
<form action="/sent"><button id="send" type="submit" aria-expanded="false">Send</button></form>
<a id="away" href="/elsewhere" aria-expanded="false">Elsewhere</a>
<script>
document.getElementById("disc").addEventListener("click", (e) => { e.currentTarget.setAttribute("aria-expanded", "true"); document.getElementById("p1").hidden = false; });
// Only the mouse button going down opens it, so Enter and Space, which send a click, do not.
document.getElementById("down").addEventListener("mousedown", (e) => e.currentTarget.setAttribute("aria-expanded", "true"));
document.getElementById("card").addEventListener("click", () => { document.getElementById("prices").hidden = false; });
const ok = document.getElementById("ok");
const open = () => ok.setAttribute("aria-expanded", "true");
ok.addEventListener("click", open);
ok.addEventListener("keydown", (e) => { if (e.key === "Enter" || e.key === " ") open(); });
</script></main></body></html>`,
	"/skip-good": `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Skip</title></head><body>
<a id="skip" href="#main">Skip to content</a><nav><a href="/a">Home</a> <a href="/b">About</a></nav>
<main id="main"><h1>Main</h1><a id="first" href="/c">First in main</a></main></body></html>`,
	"/skip-bad": `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Skip</title></head><body>
<a id="skip" href="#main">Skip to content</a><nav><a href="/a">Home</a> <a href="/b">About</a></nav>
<main id="main"><h1>Main</h1><a id="first" href="/c">First in main</a></main>
<script>document.getElementById("skip").addEventListener("click", (e) => e.preventDefault());</script></body></html>`,
}

func TestKeyboardAgainstMouseInBrowser(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	mux := http.NewServeMux()
	for path, body := range tasksPages {
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, body)
		})
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/tasks", srv.URL + "/skip-good", srv.URL + "/skip-bad"}, Browser: browser,
				Viewports: []Viewport{Desktop}, Schemes: []string{Light}, Headless: true, Keyboard: true})
			if err != nil {
				t.Fatal(err)
			}
			by := map[string]PageResult{}
			for _, r := range rep.Results {
				if r.Error != "" || r.Tasks == nil {
					t.Fatalf("%s: error %q tasks %v", r.URL, r.Error, r.Tasks)
				}
				by[strings.TrimPrefix(r.URL, srv.URL)] = r
			}
			tried := map[string]page.Task{}
			for _, task := range by["/tasks"].Tasks.Tasks {
				tried[task.Selector] = task
			}
			if _, ok := tried["#send"]; ok {
				t.Error("a submit button was tried")
			}
			if _, ok := tried["#away"]; ok {
				t.Error("a link to another page was tried")
			}
			f := layoutFinding(by["/tasks"], RuleMouseOnly)
			if f == nil || targets(f) != "#down | #card" {
				t.Fatalf("mouse-only %v, want #down then #card; tasks %+v", f, by["/tasks"].Tasks.Tasks)
			}
			if !strings.Contains(f.Nodes[0].Detail, "Enter and Space did nothing") || !strings.Contains(f.Nodes[1].Detail, "Tab never reached it") {
				t.Errorf("details %q, %q", f.Nodes[0].Detail, f.Nodes[1].Detail)
			}
			for _, sel := range []string{"#disc", "#ok"} {
				if task := tried[sel]; !task.Mouse || !task.Keyboard || task.Skipped != "" {
					t.Errorf("%s should work both ways: %+v", sel, task)
				}
			}
			if sk := by["/skip-good"].Tasks.Skip; sk == nil || !sk.Moved || layoutFinding(by["/skip-good"], RuleSkipLinkNoFocus) != nil {
				t.Errorf("working skip link: %+v", sk)
			}
			if f := layoutFinding(by["/skip-bad"], RuleSkipLinkNoFocus); f == nil || f.Nodes[0].Target != "#skip" {
				t.Errorf("broken skip link not reported: %+v", by["/skip-bad"].Tasks.Skip)
			}
		})
	}
}

func TestTaskFindingsNeedAWorkingClick(t *testing.T) {
	ts := &page.Tasks{Tasks: []page.Task{
		{Selector: "#a", Kind: "expanded", Mouse: true, Reached: true},
		{Selector: "#b", Kind: "change", Mouse: true, Keyboard: true, Reached: true},
		{Selector: "#c", Kind: "change", Mouse: false, Skipped: "a click did not change it either"},
		{Selector: "#d", Kind: "controls", Mouse: true, Skipped: "the page differed on the second load"},
	}, Skip: &page.SkipTry{Link: "#skip", Target: "#main", Label: "Skip to content"}}
	got := taskFindings(ts, "/", nil)
	if len(got) != 2 || got[0].Rule != RuleMouseOnly || len(got[0].Nodes) != 1 || got[0].Nodes[0].Target != "#a" ||
		got[0].Nodes[0].Detail != "a click worked (its aria-expanded became true); Tab reached it, but Enter and Space did nothing" {
		t.Fatalf("findings %+v", got)
	}
	if got[1].Rule != RuleSkipLinkNoFocus || got[1].WCAG[0] != "2.4.1" {
		t.Errorf("skip finding %+v", got[1])
	}
	if taskFindings(&page.Tasks{}, "/", nil) != nil {
		t.Error("no tasks should give no findings")
	}
}
