package page

import (
	"context"
	_ "embed"
	"math"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

// WebVitalsVersion is the web-vitals release bundled in webvitals/.
const WebVitalsVersion = "6.2.3"

//go:embed webvitals/web-vitals.attribution.iife.js
var webVitalsJS string

const inpSandbox = "dummyrun-inp"

// inpJS starts web-vitals' INP measure on the page with op "start", and with op "read" returns the slowest
// response to an interaction since then, with its attribution, or null when there has been none. web-vitals runs
// in a sandbox of its own, so the page can't see it or change it.
var inpJS = `(op) => {
  if (op === "start") {
    if (globalThis.__drVitals) return true;
    ` + webVitalsJS + `
;
    globalThis.__drVitals = webVitals;
    globalThis.__drINP = null;
    webVitals.onINP((m) => { globalThis.__drINP = m; }, { reportAllChanges: true });
    return true;
  }
  // The browser hands web-vitals an interaction's timing after the frame that shows its result, so the read waits
  // for a frame and a moment after it.
  return new Promise((r) => requestAnimationFrame(() => setTimeout(r, 100))).then(() => {
  const m = globalThis.__drINP;
  if (!m) return null;
  const a = m.attribution || {};
  return { duration: m.value, rating: m.rating, target: a.interactionTarget || "", type: a.interactionType || "",
    input_delay: a.inputDelay || 0, processing: a.processingDuration || 0, presentation: a.presentationDelay || 0,
    load_state: a.loadState || "" };
  });
}`

// Response is how long a page took to show the next frame after an interaction, as web-vitals measures INP, with
// where the time went. All times are in milliseconds.
type Response struct {
	// Action is what the sweep did: "click", "enter" or "space". Selector is the control it did it to.
	Action   string `json:"action"`
	Selector string `json:"selector"`
	// Duration is the INP value: the longest of the page's interactions so far, which can be an earlier Tab press.
	Duration float64 `json:"duration"`
	// Rating is web-vitals' own: good up to 200 ms, needs-improvement up to 500 ms, poor beyond.
	Rating string `json:"rating"`
	// Target is web-vitals' selector for the element the slowest interaction was on, and Type is pointer or keyboard.
	Target string `json:"target,omitempty"`
	Type   string `json:"type,omitempty"`
	// InputDelay, Processing and Presentation split the duration: waiting to start, running the page's handlers,
	// and drawing the result.
	InputDelay   float64 `json:"input_delay"`
	Processing   float64 `json:"processing"`
	Presentation float64 `json:"presentation"`
	// LoadState is how far the page had loaded, such as complete.
	LoadState string `json:"load_state,omitempty"`
}

// startINP begins measuring responses on the loaded page. A page web-vitals can't run on, such as one that isn't
// HTML, just gets no measurements.
func startINP(ctx context.Context, conn *bidi.Conn, cx string) {
	var ok bool
	_ = conn.CallFunctionJSONIn(ctx, cx, inpSandbox, inpJS, &ok, "start")
}

// readINP returns the slowest response since startINP, or nil when there is none yet.
func readINP(ctx context.Context, conn *bidi.Conn, cx, action, selector string) *Response {
	var r *Response
	if err := conn.CallFunctionJSONIn(ctx, cx, inpSandbox, inpJS, &r, "read"); err != nil || r == nil {
		return nil
	}
	r.Action, r.Selector = action, selector
	for _, v := range []*float64{&r.Duration, &r.InputDelay, &r.Processing, &r.Presentation} {
		*v = math.Round(*v)
	}
	return r
}
