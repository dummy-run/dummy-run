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
	"github.com/dummy-run/dummy-run/internal/record"
	"github.com/dummy-run/dummy-run/internal/signin"
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
	// Cards that look clickable and hold a link to another page or a submit button: a click on the card can land on
	// what it holds, so neither is tried.
	"/cards": `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Cards</title></head><body><main>
<h1>Cards</h1>
<div id="linkcard" style="cursor:pointer"><a href="/elsewhere" style="display:block;padding:20px">Read the story</a></div>
<form action="/sent"><div id="sendcard" style="cursor:pointer"><button type="submit" style="display:block;padding:20px">Send it</button></div></form>
<a id="jslink" href="javascript:location.href='/elsewhere'" aria-expanded="false">More</a>
</main></body></html>`,
	// The first control can't be reached by Tab, so the walk for it runs off the end of the page, and in Firefox
	// focus is left in the browser's own controls. The next control must still be reached on the next load.
	"/recover": `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Recover</title></head><body><main>
<h1>Recover</h1>
<div id="mouse" aria-expanded="false">Mouse only</div>
<button id="both" aria-expanded="false">Both ways</button>
<script>
document.getElementById("mouse").addEventListener("click", (e) => e.currentTarget.setAttribute("aria-expanded", "true"));
document.getElementById("both").addEventListener("click", (e) => e.currentTarget.setAttribute("aria-expanded", "true"));
</script></main></body></html>`,
	"/skip-good": `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Skip</title></head><body>
<a id="skip" href="#main">Skip to content</a><nav><a href="/a">Home</a> <a href="/b">About</a></nav>
<main id="main"><h1>Main</h1><a id="first" href="/c">First in main</a></main></body></html>`,
	"/skip-bad": `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Skip</title></head><body>
<a id="skip" href="#main">Skip to content</a><nav><a href="/a">Home</a> <a href="/b">About</a></nav>
<main id="main"><h1>Main</h1><a id="first" href="/c">First in main</a></main>
<script>document.getElementById("skip").addEventListener("click", (e) => e.preventDefault());</script></body></html>`,
}

// longPage puts more Tab stops than a walk takes before a button that works both ways, so the button has no verdict.
func longPage() string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Long</title></head><body><main><h1>Long</h1>`)
	for i := 0; i < page.MaxTabStops+5; i++ {
		fmt.Fprintf(&b, `<a href="#s%d">Stop %d</a> `, i, i)
	}
	b.WriteString(`<button id="late" aria-expanded="false">Late</button>
<script>document.getElementById("late").addEventListener("click", (e) => e.currentTarget.setAttribute("aria-expanded", "true"));</script>
</main></body></html>`)
	return b.String()
}

func TestKeyboardAgainstMouseInBrowser(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	mux := http.NewServeMux()
	pages := map[string]string{"/long": longPage()}
	for path, body := range tasksPages {
		pages[path] = body
	}
	for path, body := range pages {
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
			rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/tasks", srv.URL + "/skip-good", srv.URL + "/skip-bad", srv.URL + "/cards", srv.URL + "/recover", srv.URL + "/long"}, Browser: browser,
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
			if n := len(by["/cards"].Tasks.Tasks); n != 0 {
				t.Errorf("a javascript: link, or a card holding a link or a submit button, was tried: %+v", by["/cards"].Tasks.Tasks)
			}
			rec := map[string]page.Task{}
			for _, task := range by["/recover"].Tasks.Tasks {
				rec[task.Selector] = task
			}
			if m := rec["#mouse"]; !m.Mouse || m.Reached || m.Skipped != "" {
				t.Errorf("#mouse should be a mouse-only verdict: %+v", m)
			}
			if b := rec["#both"]; !b.Mouse || !b.Keyboard {
				t.Errorf("#both should work both ways after the walk ran off the page: %+v", b)
			}
			long := by["/long"]
			if len(long.Tasks.Tasks) != 1 || !strings.Contains(long.Tasks.Tasks[0].Skipped, "within") || layoutFinding(long, RuleMouseOnly) != nil {
				t.Errorf("a button past the last Tab stop the walk takes should have no verdict: %+v", long.Tasks.Tasks)
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

// Signed in, no control is tried: any click handler could send a request that changes the account.
func TestSignedInSweepTriesNoControls(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, tasksPages["/tasks"])
	}))
	defer srv.Close()
	session := &signin.Session{Cookies: []signin.SessionCookie{{Name: "sid", Value: "test", Domain: "127.0.0.1", Path: "/", Expires: -1, SameSite: "Lax"}}}
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/tasks"}, Browser: browser, Session: session,
				Viewports: []Viewport{Desktop}, Schemes: []string{Light}, Headless: true, Keyboard: true})
			if err != nil {
				t.Fatal(err)
			}
			r := rep.Results[0]
			if r.Error != "" || r.Tasks == nil {
				t.Fatalf("error %q tasks %v", r.Error, r.Tasks)
			}
			if len(r.Tasks.Tasks) != 0 {
				t.Errorf("signed in, controls were tried: %+v", r.Tasks.Tasks)
			}
		})
	}
}

// TestMouseOnlyIsATaskVerdict checks that a control the sweep could not work by keyboard is a task-verdict
// finding, while the skip link check stays a measurement.
func TestMouseOnlyIsATaskVerdict(t *testing.T) {
	tasks := &page.Tasks{
		Tasks: []page.Task{{Selector: "#menu", Kind: "expanded", Mouse: true, Reached: true}},
		Skip:  &page.SkipTry{Link: "a.skip", Target: "#main", Label: "Skip to content"},
	}
	got := map[string]string{}
	for _, f := range taskFindings(tasks, "/", nil) {
		got[f.Rule] = f.Evidence
	}
	want := map[string]string{RuleMouseOnly: record.EvidenceTaskVerdict, RuleSkipLinkNoFocus: record.EvidenceMeasurement}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("evidence by rule %v, want %v", got, want)
	}
}
