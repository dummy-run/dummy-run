package a11y

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

// RuleFailureSilent is a page whose data requests failed and that announced nothing to assistive technology.
const RuleFailureSilent = "dummyrun-failure-silent"

// failureTimeout bounds the failure probe for one page, which loads the page twice.
const failureTimeout = 90 * time.Second

// failureSettle is how long the probe waits after each load for the page's scripts to finish and show the result.
var failureSettle = 2 * time.Second

// FailureProbe is what happened when the page's own data requests failed.
type FailureProbe struct {
	// Failed counts the page's data requests, such as fetch and XMLHttpRequest, that the probe failed on purpose.
	// Every one was a GET or HEAD.
	Failed int `json:"failed"`
	// Stopped counts the requests other than GET and HEAD the page tried in either load. None of them left the
	// browser.
	Stopped int `json:"stopped,omitempty"`
	// Announced says a live region or alert held text after the failure that it didn't hold on a normal load.
	// Announcement is the text, cut to 300 characters.
	Announced    bool   `json:"announced"`
	Announcement string `json:"announcement,omitempty"`
	// Changed says the page's visible text differs from a normal load.
	Changed bool `json:"changed"`
}

// liveTextJS reads the text of the page's live regions and alerts, and the whole page's visible text.
const liveTextJS = `() => {
  const live = [];
  for (const el of document.querySelectorAll('[role=alert],[role=alertdialog],[role=status],[role=log],[aria-live]:not([aria-live=off]),output')) {
    const t = (el.innerText || el.textContent || "").trim();
    if (t) live.push(t);
  }
  return {live: live.join("\n"), text: document.body ? document.body.innerText : ""};
}`

type pageText struct {
	Live string `json:"live"`
	Text string `json:"text"`
}

// failureGuard decides each request in the probe's tab. A request other than GET or HEAD always fails before it
// leaves the browser, so the probe can't change anything on the site. While failing is set, the page's data
// requests, the ones that aren't a page load, a frame, a script, a style, an image or a font, fail too.
type failureGuard struct {
	conn    *bidi.Conn
	id      atomic.Value
	failing atomic.Bool
	failed  atomic.Int32
	stopped atomic.Int32
	allowed func(string) bool
}

func (g *failureGuard) handle(e bidi.BeforeRequestSent) {
	mine, _ := g.id.Load().(string)
	if !e.IsBlocked || mine == "" || !slices.Contains(e.Intercepts, mine) {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), bidi.DefaultTimeout)
		defer cancel()
		m := strings.ToUpper(e.Request.Method)
		switch {
		case m != "GET" && m != "HEAD":
			g.stopped.Add(1)
			_ = g.conn.FailRequest(ctx, e.Request.Request)
		case e.Navigation != "":
			if g.allowed != nil && !g.allowed(e.Request.URL) {
				_ = g.conn.FailRequest(ctx, e.Request.Request)
				return
			}
			_ = g.conn.ContinueRequestUnchanged(ctx, e.Request.Request)
		case g.failing.Load() && isDataRequest(e.Request):
			g.failed.Add(1)
			_ = g.conn.FailRequest(ctx, e.Request.Request)
		default:
			_ = g.conn.ContinueRequestUnchanged(ctx, e.Request.Request)
		}
	}()
}

// isDataRequest says a request fetches data for the page's scripts. Fetch and XMLHttpRequest have no destination,
// where an image, a script or a style has one.
func isDataRequest(r bidi.RequestData) bool {
	return r.Destination == ""
}

// failureCheck loads the page twice in a tab of its own: once as normal, and once with its data requests failing,
// as when its server or API is down. It records whether the page told assistive technology anything. A page that
// had data requests fail and announced nothing gets advice under WCAG 4.1.3, since a visitor who can't see the
// page may never learn that something went wrong.
func failureCheck(ctx context.Context, conn *bidi.Conn, u string, vp Viewport, res *PageResult, allowed func(string) (bool, string)) {
	fctx, cancel := context.WithTimeout(ctx, failureTimeout)
	defer cancel()
	p, err := probeFailures(fctx, conn, u, vp, allowed)
	if err != nil {
		res.FailureProbeError = firstLine(err.Error())
		return
	}
	res.FailureProbe = p
}

func probeFailures(ctx context.Context, conn *bidi.Conn, u string, vp Viewport, allowed func(string) (bool, string)) (*FailureProbe, error) {
	cx, err := conn.CreateContext(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = conn.CloseContext(cctx, cx)
	}()
	if err := conn.SetViewport(ctx, cx, vp.Width, vp.Height); err != nil {
		return nil, err
	}
	g := &failureGuard{conn: conn, allowed: func(raw string) bool { ok, _ := allowed(raw); return ok }}
	g.id.Store("")
	conn.OnBeforeRequestSent(g.handle)
	if err := conn.Subscribe(ctx, []string{"network.beforeRequestSent"}, cx); err != nil {
		return nil, err
	}
	in, err := conn.AddInterceptInContexts(ctx, []string{cx}, "beforeRequestSent")
	if err != nil {
		return nil, err
	}
	g.id.Store(in)
	defer func() {
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = conn.RemoveIntercept(cctx, in)
	}()
	load := func() (pageText, error) {
		var t pageText
		if _, err := conn.Navigate(ctx, cx, u); err != nil {
			return t, err
		}
		select {
		case <-ctx.Done():
			return t, ctx.Err()
		case <-time.After(failureSettle):
		}
		err := conn.CallFunctionJSON(ctx, cx, liveTextJS, &t)
		return t, err
	}
	normal, err := load()
	if err != nil {
		return nil, fmt.Errorf("normal load: %w", err)
	}
	g.failing.Store(true)
	failed, err := load()
	if err != nil {
		return nil, fmt.Errorf("load with failing requests: %w", err)
	}
	p := &FailureProbe{Failed: int(g.failed.Load()), Stopped: int(g.stopped.Load()), Changed: failed.Text != normal.Text}
	if failed.Live != "" && failed.Live != normal.Live {
		p.Announced = true
		p.Announcement = cut(failed.Live, 300)
	}
	return p, nil
}

// failureFindings is the advice for a page that had data requests fail and announced nothing.
func failureFindings(p *FailureProbe, path string, allow *AllowList) []Finding {
	if p == nil || p.Failed == 0 || p.Announced {
		return nil
	}
	detail := fmt.Sprintf("%d data requests failed and the page's text didn't change", p.Failed)
	if p.Changed {
		detail = fmt.Sprintf("%d data requests failed; the page's text changed but no live region or alert said so", p.Failed)
	}
	n := Node{Target: "html", Detail: detail}
	if e := allow.Match(RuleFailureSilent, path, n.Target); e != nil {
		n.Allowed, n.Reason = true, e.Reason
	}
	return []Finding{{Rule: RuleFailureSilent, Impact: "moderate", WCAG: []string{"4.1.3"}, Evidence: "measurement",
		Help:    "The page says nothing when its data fails to load",
		HelpURL: "https://www.w3.org/WAI/WCAG22/Understanding/status-messages.html", Nodes: []Node{n}}}
}

// cut shortens s to at most n runes, ending in three dots when it was longer.
func cut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-3]) + "..."
}
