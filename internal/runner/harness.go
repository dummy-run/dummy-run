package runner

import (
	"fmt"
	"strings"

	"github.com/dummy-run/dummy-run/internal/record"
)

// blockedSymptoms are the symptoms a request the test itself refused can produce on its own: the site did not
// fail, the test stopped it.
var blockedSymptoms = map[string]bool{
	"wrong-outcome": true, "not-operable": true, "error-shown": true, "not-reachable": true, "dead-link": true, "slow": true,
}

// browserErrorPage is true for the page a browser shows itself when a load fails, such as Chrome's
// chrome-error://chromewebdata/ and Firefox's about:neterror.
func browserErrorPage(u string) bool {
	return strings.HasPrefix(u, "chrome-error://") || strings.HasPrefix(u, "about:neterror") || strings.HasPrefix(u, "about:certerror")
}

// interactions are the actions that send requests of their own.
var interactions = map[string]bool{"click": true, "press": true, "fill": true, "type": true, "select": true, "visit": true}

// harnessErrors are the errors that come from the test, not the site: a lost or slow browser connection, a page
// the engine could not read, a tool that failed or timed out, a file that is not a web page.
var harnessErrors = []string{
	"bidi:", "could not read the page", "context deadline exceeded", "this is not a web page",
	"inject axe-core", "axe-core did not load", "axe.run",
}

// harnessStep says why a step shows the test's limits rather than the site, or "" when it shows the site.
// symptom is the finding's base symptom, which decides whether a refused request is the whole story.
func harnessStep(s *Step, prevURL, symptom string) string {
	o := s.Obs
	switch {
	case s.Refusal != "":
		return "the persona's tool was refused"
	case o.Snapshot == nil:
		return "the browser could not read the page"
	case o.Snapshot.NotAWebPage():
		return "the step shows a file that is not a web page (" + o.Snapshot.ContentType + ")"
	}
	low := strings.ToLower(o.Error)
	for _, e := range harnessErrors {
		if strings.Contains(low, e) {
			return "the step's tool failed: " + shorten(o.Error, 120)
		}
	}
	// A blocked request explains the step only when the persona's own action sent it (a click, a key, a form or a
	// visit) and the address stayed where it was, as when a form's POST is refused. A request a page sends by
	// itself, such as an analytics beacon while the persona reads or on the next page, does not excuse a failure.
	if len(o.Blocked) > 0 && interactions[o.Action] && browserErrorPage(o.URL) {
		return "the browser showed its own error page for a request the test blocked (" + shorten(o.Blocked[0], 120) + ")"
	}
	if len(o.Blocked) > 0 && blockedSymptoms[symptom] && interactions[o.Action] && (prevURL == "" || o.URL == prevURL) {
		return "the test blocked a request the action sent (" + shorten(o.Blocked[0], 120) + ")"
	}
	return ""
}

// HarnessOnly reports whether every step a finding cites shows only the test's own limits: a step the browser
// could not read, a refused or failed tool, a file that is not a web page, or a request the test blocked when
// the finding's symptom is the kind a blocked request causes. Such a finding is about the test, not the site.
// A finding with at least one cited step that shows the real page is kept. The reason names the first step.
func HarnessOnly(ff FinishFinding, steps []Step) (string, bool) {
	base, _, _ := strings.Cut(ff.Symptom, ":")
	var first string
	cited := 0
	for _, n := range ff.Steps {
		if n < 1 || n > len(steps) {
			continue
		}
		cited++
		prev := ""
		if n >= 2 {
			prev = steps[n-2].Obs.URL
		}
		why := harnessStep(&steps[n-1], prev, base)
		if why == "" {
			return "", false
		}
		if first == "" {
			first = fmt.Sprintf("step %d: %s", n, why)
		}
	}
	return first, cited > 0
}

// dropHarnessOnly removes the findings that HarnessOnly says are about the test, and records each as an event.
func (r *run) dropHarnessOnly(f *Finish) {
	kept := f.Findings[:0:0]
	for i, ff := range f.Findings {
		if why, ok := HarnessOnly(ff, r.steps); ok {
			// Each removal gets its own key, so two findings with one title are both accounted for.
			r.addEvent(fmt.Sprintf("harness:%d:%v:%s", i, ff.Steps, ff.Title), record.Event{Kind: record.EventHarnessLimitFinding, Step: len(r.steps),
				Detail: "finding dropped because it shows the test's limits, not the site (" + why + "): " + ff.Title})
			continue
		}
		kept = append(kept, ff)
	}
	if n := len(f.Findings) - len(kept); n > 0 {
		fmt.Fprintf(r.log, "finish: %d finding(s) dropped, they show the test's limits and not the site\n", n)
	}
	f.Findings = kept
}
