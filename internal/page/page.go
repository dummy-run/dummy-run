// Package page drives one browser tab with the persona's thirteen tools and
// records what each action produced: the page as text and numbered controls,
// a screenshot, console errors, failed requests and blocked requests.
package page

import (
	"context"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

//go:embed look.js
var lookJS string

//go:embed axe/axe.min.js
var axeJS string

const (
	maxControls = 120
	textLimit   = 2500
	logLimit    = 20000
	settleWait  = 5 * time.Second
	// MaxWait caps the wait tool.
	MaxWait = 15
)

// Request is an intercepted request offered to the hook.
type Request struct {
	Method  string
	URL     string
	Headers []bidi.Header
}

// Decision is what the hook wants done with a request.
type Decision struct {
	// Block fails the request as a network error.
	Block bool
	// Reason is recorded with a blocked request.
	Reason string
	// Set lists headers to add or replace when the request goes through.
	Set []bidi.Header
}

// Options configure a page.
type Options struct {
	Browser   string
	Headless  bool
	UserAgent string
	Width     int
	Height    int
	// Hosts limits network interception to these hostnames. Empty intercepts everything.
	Hosts []string
	// RequestHook decides each intercepted request. nil lets everything through.
	RequestHook func(Request) Decision
	Launch      bidi.LaunchOptions
}

// Control is one numbered interactive element from the last look.
type Control struct {
	Ref       string `json:"ref"`
	Tag       string `json:"tag"`
	Type      string `json:"type"`
	Label     string `json:"label"`
	Href      string `json:"href,omitempty"`
	Disabled  bool   `json:"disabled,omitempty"`
	Offscreen bool   `json:"offscreen,omitempty"`
}

// Snapshot is the page as text.
type Snapshot struct {
	URL      string    `json:"url"`
	Title    string    `json:"title"`
	Viewport []int     `json:"viewport"`
	Scroll   []int     `json:"scroll"`
	Headings []string  `json:"headings"`
	Alerts   []string  `json:"alerts"`
	Text     string    `json:"text"`
	LogText  string    `json:"log_text"`
	Controls []Control `json:"controls"`
}

// Violation is one axe-core rule that failed.
type Violation struct {
	ID     string `json:"id"`
	Impact string `json:"impact"`
	Help   string `json:"help"`
	Nodes  int    `json:"nodes"`
}

// Observation is what one action produced.
type Observation struct {
	Action         string
	Target         string
	Error          string
	URL            string
	Status         int
	Duration       time.Duration
	Snapshot       *Snapshot
	Screenshot     []byte
	ConsoleErrors  []string
	FailedRequests []string
	Blocked        []string
	// A11y is nil unless the a11y tool ran.
	A11y []Violation
}

// Page is one tab in a launched browser.
type Page struct {
	browser *bidi.Browser
	conn    *bidi.Conn
	cx      string
	hook    func(Request) Decision

	mu            sync.Mutex
	inflight      map[string]bool
	consoleErrors []string
	failed        []string
	blocked       []string
	blockedIDs    map[string]bool
	status        int
	controls      []Control
}

// Browser returns the launched browser.
func (p *Page) Browser() *bidi.Browser { return p.browser }

// Controls returns the numbered controls from the last look.
func (p *Page) Controls() []Control {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Control(nil), p.controls...)
}

// Open launches the browser and prepares one tab.
func Open(ctx context.Context, opts Options) (*Page, error) {
	lo := opts.Launch
	lo.Browser = opts.Browser
	lo.Headless = opts.Headless
	b, err := bidi.Launch(ctx, lo)
	if err != nil {
		return nil, err
	}
	p := &Page{browser: b, conn: b.Conn, hook: opts.RequestHook, inflight: map[string]bool{}, blockedIDs: map[string]bool{}}
	if err := p.setup(ctx, opts); err != nil {
		b.Close()
		return nil, err
	}
	return p, nil
}

func (p *Page) setup(ctx context.Context, opts Options) error {
	c := p.conn
	c.OnLogEntry(p.onLog)
	c.OnBeforeRequestSent(p.onBeforeRequest)
	c.OnResponseCompleted(p.onResponse)
	c.OnFetchError(p.onFetchError)
	if err := c.Subscribe(ctx, []string{"log.entryAdded", "network.beforeRequestSent",
		"network.responseCompleted", "network.fetchError"}); err != nil {
		return err
	}
	cx, err := c.CreateContext(ctx)
	if err != nil {
		return err
	}
	p.cx = cx
	// The browser's own first tab stays on about:blank; the run uses this one.
	if opts.Width > 0 && opts.Height > 0 {
		if err := c.SetViewport(ctx, cx, opts.Width, opts.Height); err != nil {
			return err
		}
	}
	if opts.UserAgent != "" {
		if err := c.SetUserAgentOverride(ctx, opts.UserAgent, []string{cx}); err != nil {
			return err
		}
		p.browser.UserAgent = opts.UserAgent
	} else {
		v, err := c.Evaluate(ctx, cx, "navigator.userAgent")
		if err == nil {
			p.browser.UserAgent, _ = v.String()
		}
	}
	if _, err := c.AddIntercept(ctx, opts.Hosts); err != nil {
		return err
	}
	return nil
}

// Close ends the browser.
func (p *Page) Close() { p.browser.Close() }

func (p *Page) onLog(e bidi.LogEntry) {
	if e.Level != "error" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.consoleErrors) < 50 {
		p.consoleErrors = append(p.consoleErrors, firstLine(e.Text))
	}
}

func (p *Page) onBeforeRequest(e bidi.BeforeRequestSent) {
	p.mu.Lock()
	p.inflight[e.Request.Request] = true
	p.mu.Unlock()
	if !e.IsBlocked {
		return
	}
	go p.decide(e)
}

func (p *Page) decide(e bidi.BeforeRequestSent) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var d Decision
	if p.hook != nil {
		d = p.hook(Request{Method: e.Request.Method, URL: e.Request.URL, Headers: e.Request.Headers})
	}
	if d.Block {
		p.mu.Lock()
		p.blockedIDs[e.Request.Request] = true
		p.blocked = append(p.blocked, fmt.Sprintf("%s %s (%s)", e.Request.Method, redact(e.Request.URL), d.Reason))
		p.mu.Unlock()
		_ = p.conn.FailRequest(ctx, e.Request.Request)
		return
	}
	if len(d.Set) == 0 {
		_ = p.conn.ContinueRequestUnchanged(ctx, e.Request.Request)
		return
	}
	_ = p.conn.ContinueRequest(ctx, e.Request.Request, e.Request.Headers, d.Set)
}

func (p *Page) onResponse(e bidi.ResponseCompleted) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.inflight, e.Request.Request)
	if e.Navigation != "" && e.Context == p.cx {
		p.status = e.Response.Status
	}
	if e.Response.Status >= http.StatusBadRequest && !strings.Contains(e.Request.URL, "/favicon") {
		p.failed = append(p.failed, fmt.Sprintf("%d %s", e.Response.Status, redact(e.Request.URL)))
	}
}

func (p *Page) onFetchError(e bidi.FetchError) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.inflight, e.Request.Request)
	if p.blockedIDs[e.Request.Request] {
		return
	}
	if strings.Contains(strings.ToLower(e.ErrorText), "abort") || strings.Contains(strings.ToLower(e.ErrorText), "cancel") {
		return
	}
	p.failed = append(p.failed, fmt.Sprintf("%s %s", e.ErrorText, redact(e.Request.URL)))
}

var secretParam = regexp.MustCompile(`([?&][^=&]*(?:pass|token|secret|key|code|auth)[^=&]*=)[^&#]*`)

// redact blanks query values that look like credentials before they are logged.
func redact(u string) string {
	return secretParam.ReplaceAllString(u, "${1}[hidden]")
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

// --- tools -------------------------------------------------------------------

// Look observes the page without acting.
func (p *Page) Look(ctx context.Context) Observation {
	return p.act(ctx, "look", "", func(context.Context) error { return nil })
}

// Screenshot is Look under the name the persona asked for.
func (p *Page) Screenshot(ctx context.Context) Observation {
	return p.act(ctx, "screenshot", "", func(context.Context) error { return nil })
}

// Visit navigates to a URL.
func (p *Page) Visit(ctx context.Context, url string) Observation {
	return p.act(ctx, "visit", url, func(ctx context.Context) error {
		_, err := p.conn.Navigate(ctx, p.cx, url)
		return err
	})
}

// Click clicks a control: "@12", "css:selector" or its visible label.
func (p *Page) Click(ctx context.Context, target string) Observation {
	return p.act(ctx, "click", target, func(ctx context.Context) error {
		el, err := p.resolve(ctx, target, "clickable")
		if err != nil {
			return err
		}
		return p.conn.Click(ctx, p.cx, el.X, el.Y)
	})
}

// Fill sets a field's value the way a paste would: the value is set and the
// input and change events fire.
func (p *Page) Fill(ctx context.Context, target, value string) Observation {
	return p.act(ctx, "fill", target, func(ctx context.Context) error {
		if _, err := p.resolve(ctx, target, "fillable"); err != nil {
			return err
		}
		var ok bool
		err := p.conn.CallFunctionJSON(ctx, p.cx, fillJS, &ok, value)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("the field refused the value")
		}
		return nil
	})
}

// Type focuses a field, clears it and types the text one key at a time.
func (p *Page) Type(ctx context.Context, target, text string) Observation {
	return p.act(ctx, "type", target, func(ctx context.Context) error {
		if _, err := p.resolve(ctx, target, "fillable"); err != nil {
			return err
		}
		var existing int
		if err := p.conn.CallFunctionJSON(ctx, p.cx, focusJS, &existing); err != nil {
			return err
		}
		if existing > 0 {
			if err := p.conn.PressKey(ctx, p.cx, bidi.KeyEnd); err != nil {
				return err
			}
			if err := p.conn.PressKeyTimes(ctx, p.cx, bidi.KeyBackspace, existing); err != nil {
				return err
			}
		}
		return p.conn.TypeText(ctx, p.cx, text)
	})
}

// Select chooses an option in a select element by its text or value. On any
// other field it behaves like Fill.
func (p *Page) Select(ctx context.Context, target, option string) Observation {
	return p.act(ctx, "select", target, func(ctx context.Context) error {
		if _, err := p.resolve(ctx, target, "fillable"); err != nil {
			return err
		}
		var result string
		if err := p.conn.CallFunctionJSON(ctx, p.cx, selectJS, &result, option); err != nil {
			return err
		}
		if result != "" {
			return errors.New(result)
		}
		return nil
	})
}

// Press sends one key to whatever has focus.
func (p *Page) Press(ctx context.Context, key string) Observation {
	return p.act(ctx, "press", key, func(ctx context.Context) error {
		code, err := bidi.Key(key)
		if err != nil {
			return err
		}
		return p.conn.PressKey(ctx, p.cx, code)
	})
}

// Back goes back one page.
func (p *Page) Back(ctx context.Context) Observation {
	return p.act(ctx, "back", "", func(ctx context.Context) error {
		return p.conn.TraverseHistory(ctx, p.cx, -1)
	})
}

// Scroll moves the largest scrollable element, or the window, by most of a screen.
func (p *Page) Scroll(ctx context.Context, direction string) Observation {
	return p.act(ctx, "scroll", direction, func(ctx context.Context) error {
		by := 700
		if direction == "up" {
			by = -700
		}
		return p.conn.CallFunctionJSON(ctx, p.cx, scrollJS, nil, by)
	})
}

// Wait pauses for up to MaxWait seconds.
func (p *Page) Wait(ctx context.Context, seconds int) Observation {
	if seconds > MaxWait {
		seconds = MaxWait
	}
	if seconds < 0 {
		seconds = 0
	}
	return p.act(ctx, "wait", fmt.Sprintf("%ds", seconds), func(ctx context.Context) error {
		select {
		case <-time.After(time.Duration(seconds) * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	})
}

// A11y runs axe-core on the page with the WCAG 2.2 A and AA rules. It records
// rule IDs and counts, never element HTML.
func (p *Page) A11y(ctx context.Context) Observation {
	var violations []Violation
	obs := p.act(ctx, "a11y", "", func(ctx context.Context) error {
		var loaded bool
		if err := p.conn.CallFunctionJSON(ctx, p.cx, "function(){ if (!window.axe) { "+axeJS+"\n } return !!window.axe; }", &loaded); err != nil {
			return fmt.Errorf("inject axe-core: %w", err)
		}
		if !loaded {
			return errors.New("axe-core did not load")
		}
		actx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		if err := p.conn.CallFunctionJSON(actx, p.cx, axeRunJS, &violations); err != nil {
			return fmt.Errorf("axe.run: %w", err)
		}
		if violations == nil {
			violations = []Violation{}
		}
		return nil
	})
	obs.A11y = violations
	return obs
}

// --- act and observe -----------------------------------------------------------

func (p *Page) act(ctx context.Context, action, target string, fn func(context.Context) error) Observation {
	started := time.Now()
	obs := Observation{Action: action, Target: target}
	actx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := fn(actx); err != nil {
		obs.Error = firstLine(err.Error())
	}
	p.settle(actx)
	p.observe(actx, &obs)
	obs.Duration = time.Since(started)
	return obs
}

// settle waits for the page to load and the network to go quiet, for up to settleWait.
func (p *Page) settle(ctx context.Context) {
	deadline := time.Now().Add(settleWait)
	quietSince := time.Time{}
	for time.Now().Before(deadline) {
		p.mu.Lock()
		n := len(p.inflight)
		p.mu.Unlock()
		ready := false
		if v, err := p.conn.Evaluate(ctx, p.cx, "document.readyState"); err == nil {
			s, _ := v.String()
			ready = s == "complete" || s == "interactive"
		}
		if n == 0 && ready {
			if quietSince.IsZero() {
				quietSince = time.Now()
			} else if time.Since(quietSince) > 250*time.Millisecond {
				return
			}
		} else {
			quietSince = time.Time{}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func (p *Page) observe(ctx context.Context, obs *Observation) {
	var snap Snapshot
	if err := p.conn.CallFunctionJSON(ctx, p.cx, lookJS, &snap, maxControls, textLimit, logLimit); err == nil {
		obs.Snapshot = &snap
		obs.URL = snap.URL
	} else if obs.Error == "" {
		obs.Error = "could not read the page: " + firstLine(err.Error())
	}
	if obs.URL == "" {
		if v, err := p.conn.Evaluate(ctx, p.cx, "location.href"); err == nil {
			obs.URL, _ = v.String()
		}
	}
	obs.URL = redact(obs.URL)
	if obs.Snapshot != nil {
		obs.Snapshot.URL = redact(obs.Snapshot.URL)
	}
	if data, err := p.conn.CaptureScreenshot(ctx, p.cx); err == nil {
		if png, err := base64.StdEncoding.DecodeString(data); err == nil {
			obs.Screenshot = png
		}
	}
	p.mu.Lock()
	obs.ConsoleErrors = uniq(p.consoleErrors)
	obs.FailedRequests = uniq(p.failed)
	obs.Blocked = uniq(p.blocked)
	obs.Status = p.status
	p.consoleErrors, p.failed, p.blocked = nil, nil, nil
	if obs.Snapshot != nil {
		p.controls = obs.Snapshot.Controls
	}
	p.mu.Unlock()
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// --- element resolution ----------------------------------------------------------

type element struct {
	X, Y  float64
	Tag   string
	Label string
}

// resolve finds the target, marks it data-dummyrun-target for the scripts that
// follow, scrolls it into view and returns its centre.
func (p *Page) resolve(ctx context.Context, target, kind string) (element, error) {
	var res struct {
		Error string  `json:"error"`
		X     float64 `json:"x"`
		Y     float64 `json:"y"`
		Tag   string  `json:"tag"`
		Label string  `json:"label"`
	}
	if err := p.conn.CallFunctionJSON(ctx, p.cx, resolveJS, &res, target, kind); err != nil {
		return element{}, err
	}
	if res.Error != "" {
		return element{}, errors.New(res.Error)
	}
	return element{X: res.X, Y: res.Y, Tag: res.Tag, Label: res.Label}, nil
}

const resolveJS = `(target, kind) => {
  const visible = (el) => { const r = el.getBoundingClientRect(); const s = getComputedStyle(el);
    return r.width > 0 && r.height > 0 && r.right > 0 && r.left < innerWidth && s.visibility !== "hidden" && s.display !== "none"; };
  const inView = (el) => { const r = el.getBoundingClientRect(); return r.bottom > 0 && r.top < innerHeight; };
  const squash = (s) => (s || "").replace(/\s+/g, " ").trim().toLowerCase();
  const name = (el) => squash(el.getAttribute("aria-label") || el.labels?.[0]?.innerText || el.placeholder || el.innerText || el.value || el.title || "");
  document.querySelectorAll("[data-dummyrun-target]").forEach((el) => el.removeAttribute("data-dummyrun-target"));
  let el = null;
  let m;
  if ((m = /^@(\d+)$/.exec(target.trim()))) {
    el = document.querySelector('[data-dummyrun-ref="' + m[1] + '"]');
    if (!el) return { error: "no control " + target + " on this page; look again, the numbers change" };
  } else if (target.startsWith("css:")) {
    try { el = document.querySelector(target.slice(4)); } catch (e) { return { error: "bad selector: " + e.message }; }
    if (!el) return { error: "nothing matches " + target };
  } else {
    const want = squash(target);
    const sel = kind === "fillable"
      ? "input:not([type=hidden]), textarea, select, [contenteditable=true], [role=combobox], [role=textbox]"
      : "a[href], button, input[type=submit], input[type=button], [role=button], [role=link], [role=tab], [role=menuitem], [role=option], summary, label";
    // Candidates already on screen come first, so a visible link beats one far down the page.
    const all = [...document.querySelectorAll(sel)].filter(visible).sort((a, b) => Number(inView(b)) - Number(inView(a)));
    el = all.find((e) => name(e) === want) || all.find((e) => name(e).includes(want));
    if (!el && kind !== "fillable") {
      const texts = [...document.querySelectorAll("body *")].filter((e) => visible(e) && e.children.length === 0 && squash(e.innerText) === want);
      el = texts[0] || null;
    }
    if (!el) return { error: "no " + (kind === "fillable" ? "field" : "control") + " named " + JSON.stringify(target) + "; use @ref from the last look" };
    if (el.tagName === "LABEL" && el.control) el = el.control;
  }
  el.setAttribute("data-dummyrun-target", "1");
  el.scrollIntoView({ block: "center", inline: "nearest" });
  const r = el.getBoundingClientRect();
  const x = r.left + r.width / 2;
  const y = r.top + r.height / 2;
  if (x < 0 || x >= innerWidth || y < 0 || y >= innerHeight) {
    return { error: "the control " + JSON.stringify(name(el) || target) + " is off screen, probably inside a closed menu or drawer; open that first" };
  }
  return { x, y, tag: el.tagName.toLowerCase(), label: name(el) };
}`

// fillJS sets the value through the native setter so framework-bound inputs
// notice, then fires input and change.
const fillJS = `(value) => {
  const el = document.querySelector("[data-dummyrun-target]");
  if (!el) return false;
  el.focus();
  if (el.isContentEditable) { el.textContent = value; el.dispatchEvent(new InputEvent("input", { bubbles: true })); return true; }
  const proto = el.tagName === "TEXTAREA" ? HTMLTextAreaElement.prototype : el.tagName === "SELECT" ? HTMLSelectElement.prototype : HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, "value")?.set;
  if (setter) setter.call(el, value); else el.value = value;
  el.dispatchEvent(new Event("input", { bubbles: true }));
  el.dispatchEvent(new Event("change", { bubbles: true }));
  return true;
}`

const focusJS = `() => {
  const el = document.querySelector("[data-dummyrun-target]");
  if (!el) return 0;
  el.focus();
  return (el.value || el.textContent || "").length;
}`

const selectJS = `(option) => {
  const el = document.querySelector("[data-dummyrun-target]");
  if (!el) return "the field is gone";
  const want = option.trim().toLowerCase();
  if (el.tagName === "SELECT") {
    const opts = [...el.options];
    const hit = opts.find((o) => o.text.trim().toLowerCase() === want || o.value.toLowerCase() === want)
      || opts.find((o) => o.text.trim().toLowerCase().includes(want));
    if (!hit) return "no option " + JSON.stringify(option) + "; the options are " + opts.map((o) => o.text.trim()).join(", ");
    el.value = hit.value;
    el.dispatchEvent(new Event("input", { bubbles: true }));
    el.dispatchEvent(new Event("change", { bubbles: true }));
    return "";
  }
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
  if (setter && el instanceof HTMLInputElement) setter.call(el, option); else el.value = option;
  el.dispatchEvent(new Event("input", { bubbles: true }));
  el.dispatchEvent(new Event("change", { bubbles: true }));
  return "";
}`

const scrollJS = `(by) => {
  const candidates = [...document.querySelectorAll("*")].filter((el) => {
    const s = getComputedStyle(el);
    return /(auto|scroll)/.test(s.overflowY) && el.scrollHeight > el.clientHeight + 10 && el.clientHeight > 100;
  }).sort((a, b) => b.clientHeight * b.clientWidth - a.clientHeight * a.clientWidth);
  (candidates[0] || document.scrollingElement || document.documentElement).scrollBy(0, by);
  return null;
}`

const axeRunJS = `() => window.axe.run(document, { runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"] } })
  .then((r) => r.violations.map((v) => ({ id: v.id, impact: v.impact || "", help: v.help, nodes: v.nodes.length })))`

// SetUserAgent replaces navigator.userAgent for the run's tab.
func (p *Page) SetUserAgent(ctx context.Context, ua string) error {
	if err := p.conn.SetUserAgentOverride(ctx, ua, []string{p.cx}); err != nil {
		return err
	}
	p.browser.UserAgent = ua
	return nil
}
