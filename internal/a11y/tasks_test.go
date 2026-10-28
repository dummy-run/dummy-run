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

	"github.com/dummy-run/dummy-run/internal/access"
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
	// A sign-up form with a header, a footer and a select. At desktop width in Firefox, focus can take more than ten
	// Tab presses to come back into the page from the browser's own controls after a walk runs off the end.
	"/form": `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Join</title></head><body>
<a href="#content">Skip to the form</a>
<header><nav aria-label="Main"><a href="/">Home</a> <a href="/books">Books</a> <a href="/help">Help</a></nav></header>
<main id="content"><h1>Join</h1>
<form action="/join" method="get">
<label for="first">First name</label><input id="first" name="first" autocomplete="given-name">
<label for="last">Last name</label><input id="last" name="last" autocomplete="family-name">
<label for="email">Email address</label><input id="email" name="email" type="email" autocomplete="email">
<label for="branch">Nearest branch</label><select id="branch" name="branch"><option>Central</option><option>Riverside</option></select>
<button type="submit">Join now</button>
</form></main>
<footer><p><a href="/privacy">Privacy</a> <a href="/contact">Contact us</a></p></footer>
</body></html>`,
	// A combobox opens with the arrow keys and a roving tab is reached by them, so Enter and Tab would look as if
	// they failed. Neither is tried.
	"/entry": `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Entry</title></head><body><main>
<h1>Entry</h1>
<label for="town">Town</label><input id="town" role="combobox" aria-expanded="false" aria-controls="towns"><ul id="towns" hidden><li>Leeds</li></ul>
<div contenteditable="true"><p id="note" aria-expanded="false">Notes</p></div>
<div role="tablist"><button id="t1" role="tab" aria-selected="true" tabindex="0">Day</button><button id="t2" role="tab" aria-selected="false" tabindex="-1">Week</button></div>
<script>
document.getElementById("town").addEventListener("click", (e) => { e.currentTarget.setAttribute("aria-expanded", "true"); document.getElementById("towns").hidden = false; });
document.getElementById("t2").addEventListener("click", (e) => e.currentTarget.setAttribute("aria-selected", "true"));
</script></main></body></html>`,
	// Two skip links in the same deep nesting give a selector that names both, so the next load could test the
	// other one. There is no verdict.
	"/skip-twice": `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Skip</title></head><body>
` + strings.Repeat(`<div><div><div><div><div><div><a href="#main">Skip to content</a></div></div></div></div></div></div>`, 2) + `
<nav><a href="/a">Home</a></nav><main id="main"><h1>Main</h1><a id="first" href="/c">First in main</a></main>
<script>document.querySelectorAll("a[href='#main']").forEach((a) => a.addEventListener("click", (e) => e.preventDefault()));</script></body></html>`,
	"/skip-good": `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Skip</title></head><body>
<a id="skip" href="#main">Skip to content</a><nav><a href="/a">Home</a> <a href="/b">About</a></nav>
<main id="main"><h1>Main</h1><a id="first" href="/c">First in main</a></main></body></html>`,
	"/skip-bad": `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Skip</title></head><body>
<a id="skip" href="#main">Skip to content</a><nav><a href="/a">Home</a> <a href="/b">About</a></nav>
<main id="main"><h1>Main</h1><a id="first" href="/c">First in main</a></main>
<script>document.getElementById("skip").addEventListener("click", (e) => e.preventDefault());</script></body></html>`,
	// A broken skip link just before its target: Tab alone lands in the target, so there is no verdict.
	"/skip-next": `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Skip</title></head><body>
<a id="skip" href="#main">Skip to content</a>
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

// TestKeyboardAgainstMouseInBrowser sweeps pages of controls, skip links and forms with the keyboard and mouse
// comparison on, and checks its verdict on each page in every browser.
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
			rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/tasks", srv.URL + "/skip-good", srv.URL + "/skip-bad", srv.URL + "/skip-next", srv.URL + "/cards", srv.URL + "/recover", srv.URL + "/entry", srv.URL + "/skip-twice", srv.URL + "/long", srv.URL + "/form"}, Browser: browser,
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
			if n := len(by["/entry"].Tasks.Tasks); n != 0 || layoutFinding(by["/entry"], RuleMouseOnly) != nil {
				t.Errorf("a combobox or a roving tab was tried: %+v", by["/entry"].Tasks.Tasks)
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
			if sk := by["/skip-twice"].Tasks.Skip; sk != nil || layoutFinding(by["/skip-twice"], RuleSkipLinkNoFocus) != nil {
				t.Errorf("a skip link whose selector names two links should have no verdict: %+v", sk)
			}
			if sk := by["/skip-next"].Tasks.Skip; sk != nil {
				t.Errorf("a skip link just before its target should have no verdict: %+v", sk)
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

// Signed in, nothing is tried, not even the skip link: any handler could send a request that changes the account.
func TestSignedInSweepTriesNothing(t *testing.T) {
	v := os.Getenv("DUMMYRUN_BROWSERS")
	if v == "" {
		t.Skip("set DUMMYRUN_BROWSERS=chrome,firefox to run the browser tests")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, tasksPages[r.URL.Path])
	}))
	defer srv.Close()
	session := &signin.Session{Cookies: []signin.SessionCookie{{Name: "sid", Value: "test", Domain: "127.0.0.1", Path: "/", Expires: -1, SameSite: "Lax"}}}
	for _, browser := range strings.Split(v, ",") {
		t.Run(browser, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			rep, err := Run(ctx, Config{URLs: []string{srv.URL + "/tasks", srv.URL + "/skip-bad"}, Browser: browser, Session: session,
				Viewports: []Viewport{Desktop}, Schemes: []string{Light}, Headless: true, Keyboard: true})
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range rep.Results {
				if r.Error != "" || !r.KeyboardChecked {
					t.Fatalf("%s: error %q, keyboard checked %v", r.URL, r.Error, r.KeyboardChecked)
				}
				if r.Tasks != nil {
					t.Errorf("signed in, %s was tried: %+v", r.URL, r.Tasks)
				}
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

func TestTasksAreTriedOnlyWhenNotSignedIn(t *testing.T) {
	for _, c := range []struct {
		name string
		cfg  Config
		want bool
	}{
		{"keyboard", Config{Keyboard: true}, true},
		{"no keyboard", Config{}, false},
		{"session", Config{Keyboard: true, Session: &signin.Session{}}, false},
		{"preview access", Config{Keyboard: true, Access: &access.Resolved{}}, false},
	} {
		if got := triesTasks(c.cfg); got != c.want {
			t.Errorf("%s: triesTasks %v, want %v", c.name, got, c.want)
		}
		j := c.cfg
		j.Keyboard, j.Journeys = false, c.cfg.Keyboard
		if got := triesJourneys(j); got != c.want {
			t.Errorf("%s: triesJourneys %v, want %v", c.name, got, c.want)
		}
	}
}
