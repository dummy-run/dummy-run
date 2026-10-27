// Package page drives one browser tab with the persona's thirteen tools and
// records what each action produced: the page as text and numbered controls,
// a screenshot, console errors, failed requests and blocked requests.
package page

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/dummy-run/dummy-run/internal/access"
	"github.com/dummy-run/dummy-run/internal/bidi"
)

//go:embed look.js
var lookSrc string

//go:embed tree.js
var treeSrc string

// namesJS is the role, name and exposure code that look.js, tree.js and the a11y tool share. It is spliced into
// each where it says NAMES, so they can never name an element two ways.
//
//go:embed names.js
var namesJS string

var (
	lookJS   = withNames(lookSrc)
	treeJS   = withNames(treeSrc)
	axeRunJS = withNames(axeRunSrc)
)

func withNames(src string) string { return strings.Replace(src, "/*NAMES*/", namesJS, 1) }

//go:embed axe/axe.min.js
var axeJS string

const (
	maxControls = 120
	textLimit   = 4000
	logLimit    = 20000
	settleWait  = 5 * time.Second
	// MaxWait caps the wait tool.
	MaxWait = 15
	// TreeTokens is the budget for the page outline in one observation: landmarks, headings and controls. It is the
	// only list of controls the persona reads, so it has the room the second control list used to take.
	TreeTokens = 1500
	// CharsPerToken converts that budget to characters. Three is deliberately
	// low: outline lines are short names and punctuation, which tokenise worse than prose.
	CharsPerToken = 3
	// TreeChars is the outline's size limit in characters.
	TreeChars = TreeTokens * CharsPerToken
)

// Request is an intercepted request offered to the hook.
type Request struct {
	Method  string
	URL     string
	Headers []bidi.Header
	// Navigation is true for a document load, in the persona's tab, a frame or a pop-up: what policy.Navigation
	// means. Everything else, images, styles, scripts, media and fetches, is a subresource.
	Navigation bool
}

// isTopNavigation decides whether a request loads a document: into the persona's tab, a frame or a pop-up. It
// errs towards navigation: a request with a navigation id, or whose destination is a document, frame, object or
// embed, is one, and so is a request with no hints at all that asks for HTML first.
func isTopNavigation(e bidi.BeforeRequestSent, _ string) bool {
	switch e.Request.Destination {
	case "document", "iframe", "frame", "object", "embed":
		return true
	case "":
	default:
		return false
	}
	if e.Navigation != "" {
		return true
	}
	for _, h := range e.Request.Headers {
		if strings.EqualFold(h.Name, "accept") {
			return strings.HasPrefix(strings.TrimSpace(h.Value.Value), "text/html")
		}
	}
	return false
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
	// Width and Height are the viewport in CSS pixels. With a Zoom above 100
	// the browser window is Zoom percent larger in each direction.
	Width  int
	Height int
	// Zoom is the page zoom in percent. 0 and 100 mean none. Above 100 the
	// viewport command also sets devicePixelRatio to Zoom/100, which is what a
	// browser does when the user zooms.
	Zoom int
	// TextScale is the system text size in percent, 0 and 100 meaning none. Above 100 the browser is
	// launched with a larger default font size. The window and the device pixel ratio stay as they are.
	TextScale int
	// Hosts limits network interception to these hostnames. Empty intercepts everything.
	Hosts []string
	// RequestHook decides each intercepted request. nil lets everything through.
	RequestHook func(Request) Decision
	// Access adds the preview-protection headers to requests to the target's hosts and answers
	// Basic auth challenges from them. Nil does neither.
	Access *access.Resolved
	Launch bidi.LaunchOptions
	// ScreenReader adds what guidepup's virtual screen reader says to each look, for a persona that works
	// without seeing the screen.
	ScreenReader bool
}

// Control is one numbered interactive element from the last look.
type Control struct {
	Ref  string `json:"ref"`
	Tag  string `json:"tag"`
	Type string `json:"type"`
	// Role is the control's role, such as link, button or combobox.
	Role string `json:"role,omitempty"`
	// Label is the control's accessible name, the same one the outline gives it, cut at a word boundary. A control
	// with no name is "(no name)", sometimes followed by its link path as a hint.
	Label     string `json:"label"`
	Href      string `json:"href,omitempty"`
	Disabled  bool   `json:"disabled,omitempty"`
	Offscreen bool   `json:"offscreen,omitempty"`
}

// Snapshot is the page as text.
type Snapshot struct {
	URL string `json:"url"`
	// ContentType is the document's MIME type as the browser reports it (document.contentType). Empty means
	// unknown, which is read as HTML. Anything else, such as an RSS feed, a PDF or JSON, is not a web page:
	// the observation says so and has no controls, and the accessibility check does not run on it.
	ContentType string    `json:"content_type,omitempty"`
	Title       string    `json:"title"`
	Viewport    []int     `json:"viewport"`
	Scroll      []int     `json:"scroll"`
	Headings    []string  `json:"headings"`
	Alerts      []string  `json:"alerts"`
	Text        string    `json:"text"`
	LogText     string    `json:"log_text"`
	Controls    []Control `json:"controls"`
	// Hidden is text and attribute values a person does not see: hidden or off-screen text, comments,
	// aria-label, title and alt values. The model is not sent it. The engine scans it for injection.
	Hidden []string `json:"hidden,omitempty"`
	// ScanText is the page text, headings and alerts with each visible code or quote element cut out where it
	// stands. The injection scan reads it in place of Text, because a page that shows an example of an attack
	// is not making one. It is empty when the page has no text.
	ScanText string `json:"scan_text,omitempty"`
	// DevicePixelRatio is window.devicePixelRatio as the page sees it.
	DevicePixelRatio float64 `json:"device_pixel_ratio,omitempty"`
	// Zoom is the page zoom in percent the run set, 100 when none.
	Zoom int `json:"zoom,omitempty"`
	// TextScale is the system text size in percent the run set, 0 when none.
	TextScale int `json:"text_scale,omitempty"`
	// Layout is set when the geometry checks found something wrong. Nil means nothing was found.
	Layout *LayoutNote `json:"layout,omitempty"`
	// Focus is the element that has keyboard focus, nil when nothing does.
	Focus *Focus `json:"focus"`
	// Spoken is what a screen reader says reading the page from the top, when the page was opened with ScreenReader.
	Spoken *Spoken `json:"spoken,omitempty"`
	// SpokenError says why the page could not be read aloud.
	SpokenError string `json:"spoken_error,omitempty"`
	// Tree is the outline of landmarks, headings and interactive elements,
	// one line each, within TreeTokens.
	Tree string `json:"tree,omitempty"`
	// TreeOmitted counts the outline entries cut to fit the budget.
	TreeOmitted int `json:"tree_omitted,omitempty"`
	// NamesFrom is "chrome" when the roles, names, states and exposure of the controls and the outline came from
	// Chrome's own accessibility tree. Empty means the engine worked them out in the page (names.js), as in Firefox.
	NamesFrom string `json:"names_from,omitempty"`
	// TreeError is set when focus and the outline could not be read, so that
	// "no focus" is never reported for a page that was not checked.
	TreeError string `json:"tree_error,omitempty"`
}

// Focus is the focused element as an assistive technology would announce it.
type Focus struct {
	Role string `json:"role"`
	Name string `json:"name"`
	// Ref is the control's @number from the same look, empty if it has none.
	Ref string `json:"ref,omitempty"`
	Tag string `json:"tag"`
}

// Violation is one axe-core rule that failed.
type Violation struct {
	ID     string `json:"id"`
	Impact string `json:"impact"`
	Help   string `json:"help"`
	Nodes  int    `json:"nodes"`
	// Elements names a few of the failing elements: axe's selector and the element's accessible name.
	Elements []ViolationElement `json:"elements,omitempty"`
}

// ViolationElement is one element that failed an axe rule.
type ViolationElement struct {
	Selector string `json:"selector"`
	Name     string `json:"name,omitempty"`
}

// ErrNotAWebPage is the error of a tool that needs an HTML document on a feed, a PDF or another file.
var ErrNotAWebPage = errors.New("this is not a web page")

// IsHTML reports whether a document content type is a web page. An empty type counts as one.
func IsHTML(contentType string) bool {
	t := strings.ToLower(strings.TrimSpace(contentType))
	return t == "" || strings.HasPrefix(t, "text/html") || strings.HasPrefix(t, "application/xhtml+xml")
}

// NotAWebPage reports whether a snapshot is of a document that is not a web page.
func (s *Snapshot) NotAWebPage() bool { return s != nil && !IsHTML(s.ContentType) }

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
	access  *access.Resolved
	// axeName is the window property the engine's own axe-core is loaded under, random for each page, so a
	// page cannot plant a fake axe where the engine looks.
	axeName       string
	mu            sync.Mutex
	inflight      map[string]bool
	consoleErrors []string
	failed        []string
	blocked       []string
	blockedIDs    map[string]bool
	status        int
	controls      []Control
	zoom          int
	textScale     int
	screenReader  bool
	// navBlocked is set when the run refuses a navigation of the page itself during an action.
	navBlocked bool
	// ax is Chrome's accessibility tree for this tab, read before each look. It stays off in Firefox.
	ax axState
}

// Browser returns the launched browser.
func (p *Page) Browser() *bidi.Browser { return p.browser }

// Conn returns the BiDi connection, for the engine's own set-up steps.
func (p *Page) Conn() *bidi.Conn { return p.conn }

// Context returns the ID of the tab the run uses.
func (p *Page) Context() string { return p.cx }

// DrainProblems forgets the console errors, failed requests and blocked requests collected so far
// and returns the blocked ones. The engine calls it after its set-up steps, so that the first
// observation shows only what the persona caused.
func (p *Page) DrainProblems() (blocked []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	blocked = uniq(p.blocked)
	p.consoleErrors, p.failed, p.blocked = nil, nil, nil
	p.status = 0
	return blocked
}

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
	if opts.TextScale > 100 {
		var err error
		if lo, err = bidi.WithTextScale(lo, opts.TextScale); err != nil {
			return nil, err
		}
	}
	b, err := bidi.Launch(ctx, lo)
	if err != nil {
		return nil, err
	}
	p := &Page{browser: b, conn: b.Conn, hook: opts.RequestHook, access: opts.Access, inflight: map[string]bool{}, blockedIDs: map[string]bool{}, textScale: max(opts.TextScale, 100), screenReader: opts.ScreenReader}
	if mark, err := lookMark(); err == nil {
		p.axeName = "__dr_axe_" + mark
	} else {
		p.axeName = "__dr_axe"
	}
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
	p.zoom = max(opts.Zoom, 100)
	if opts.Width > 0 && opts.Height > 0 {
		var err error
		if p.zoom > 100 {
			err = c.SetViewportScaled(ctx, cx, opts.Width, opts.Height, float64(p.zoom)/100)
		} else {
			err = c.SetViewport(ctx, cx, opts.Width, opts.Height)
		}
		if err != nil {
			return fmt.Errorf("set viewport at %d percent: %w", p.zoom, err)
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
	phases := []string{"beforeRequestSent"}
	if p.access.HasBasic() {
		c.OnAuthRequired(p.onAuthRequired)
		if err := c.Subscribe(ctx, []string{"network.authRequired"}); err != nil {
			return err
		}
		phases = append(phases, "authRequired")
	}
	if _, err := c.AddInterceptPhases(ctx, opts.Hosts, phases...); err != nil {
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
	nav := isTopNavigation(e, p.cx)
	if p.hook != nil {
		d = p.hook(Request{Method: e.Request.Method, URL: e.Request.URL, Headers: e.Request.Headers, Navigation: nav})
	}
	if d.Block {
		p.mu.Lock()
		p.blockedIDs[e.Request.Request] = true
		p.blocked = append(p.blocked, fmt.Sprintf("%s %s (%s)", e.Request.Method, redact(e.Request.URL), d.Reason))
		if e.Context == p.cx && nav {
			p.navBlocked = true
		}
		p.mu.Unlock()
		_ = p.conn.FailRequest(ctx, e.Request.Request)
		return
	}
	d.Set = append(d.Set, p.access.Headers(e.Request.URL, e.Request.Headers)...)
	if len(d.Set) == 0 {
		_ = p.conn.ContinueRequestUnchanged(ctx, e.Request.Request)
		return
	}
	_ = p.conn.ContinueRequest(ctx, e.Request.Request, e.Request.Headers, d.Set)
}

func (p *Page) onAuthRequired(e bidi.AuthRequired) {
	if !e.IsBlocked {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		p.access.HandleAuth(ctx, p.conn, e)
	}()
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
		var ctype string
		if err := p.conn.CallFunctionJSON(ctx, p.cx, "function(){ return document.contentType || ''; }", &ctype); err == nil && !IsHTML(ctype) {
			return fmt.Errorf("%w, so the accessibility check did not run", ErrNotAWebPage)
		}
		var loaded bool
		if err := p.conn.CallFunctionJSONIn(ctx, p.cx, sandbox, p.loadAxeJS(), &loaded); err != nil {
			return fmt.Errorf("inject axe-core: %w", err)
		}
		if !loaded {
			return errors.New("axe-core did not load")
		}
		p.ax.read(ctx, p.conn, p.cx)
		actx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		if err := p.conn.CallFunctionJSONIn(actx, p.cx, sandbox, p.withAxe(axeRunJS), &violations); err != nil {
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
	p.mu.Lock()
	p.navBlocked = false
	p.mu.Unlock()
	if err := fn(actx); err != nil {
		obs.Error = firstLine(err.Error())
	}
	p.settle(actx)
	p.leaveErrorPage(actx)
	p.observe(actx, &obs)
	obs.Duration = time.Since(started)
	return obs
}

// leaveErrorPage goes back to the page the action started on when the run refused a navigation and the
// browser put its own error page in its place, as Chrome does. Firefox keeps the page, so there is nothing to do.
// Without this, a refused link strands every later step of the run on the error page.
func (p *Page) leaveErrorPage(ctx context.Context) {
	p.mu.Lock()
	blocked := p.navBlocked
	p.mu.Unlock()
	if !blocked {
		return
	}
	v, err := p.conn.Evaluate(ctx, p.cx, "location.href")
	if err != nil {
		return
	}
	if u, _ := v.String(); !strings.HasPrefix(u, "chrome-error:") {
		return
	}
	if p.conn.TraverseHistory(ctx, p.cx, -1) == nil {
		p.settle(ctx)
	}
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
	mark, merr := lookMark()
	if merr != nil {
		obs.Error = "could not read the page: " + merr.Error()
	} else if err := p.lookWithAxe(ctx, &snap, mark); err == nil {
		snap.Zoom = p.zoom
		if p.textScale > 100 {
			snap.TextScale = p.textScale
		}
		obs.Snapshot = &snap
		obs.URL = snap.URL
		p.addOutline(ctx, &snap)
		p.addLayout(ctx, &snap)
		if p.screenReader {
			if sp, err := ReadAloud(ctx, p.conn, p.cx); err != nil {
				snap.SpokenError = firstLine(err.Error())
			} else {
				snap.Spoken = sp
			}
		}
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

// lookWithAxe loads axe-core into the page when it is a web page and axe is not there yet, so look.js and tree.js
// can name controls with axe's own functions, then runs look.js. If axe cannot load, the spec computation names them.
func (p *Page) lookWithAxe(ctx context.Context, snap *Snapshot, mark string) error {
	var loaded bool
	_ = p.conn.CallFunctionJSONIn(ctx, p.cx, sandbox, p.loadAxeJS(), &loaded)
	chrome := p.ax.read(ctx, p.conn, p.cx)
	if err := p.conn.CallFunctionJSONIn(ctx, p.cx, sandbox, p.withAxe(lookJS), snap, maxControls, textLimit, logLimit, mark); err != nil {
		return err
	}
	if chrome {
		snap.NamesFrom = NamesFromChrome
	}
	return nil
}

// loadAxeJS loads the engine's axe-core into a web page under p.axeName, once per document. Whatever the page had
// at window.axe is put back afterwards, so the page's own axe, or a fake one, is never what the engine calls.
func (p *Page) loadAxeJS() string {
	return "function(){ if (!/^(text\\/html|application\\/xhtml\\+xml)/i.test(document.contentType || '')) return false; " +
		"if (window['" + p.axeName + "']) return true; " +
		"const had = Object.prototype.hasOwnProperty.call(window, 'axe'); const prev = window.axe; " +
		"try { delete window.axe; } catch (e) {} " +
		axeJS + "\n ; const mine = window.axe; " +
		"if (had) { try { window.axe = prev; } catch (e) {} } else { try { delete window.axe; } catch (e) {} } " +
		"if (!mine || mine === prev) return false; " +
		"Object.defineProperty(window, '" + p.axeName + "', { value: mine, enumerable: false, configurable: false, writable: false }); return true; }"
}

// sandbox is the realm that look.js, tree.js and axe run in. It shares the page's DOM, so the @numbers look.js
// writes are there for the click tools, but page scripts cannot reach its globals, axe included, or change the
// functions the engine's scripts call.
const sandbox = "dummyrun"

// withAxe points a script at the engine's own axe for this page.
func (p *Page) withAxe(script string) string {
	return strings.ReplaceAll(script, "__DR_AXE__", p.axeName)
}

// addOutline adds focus and the accessibility outline to a snapshot. A page
// that cannot be read this way keeps the rest of its snapshot.
func (p *Page) addOutline(ctx context.Context, snap *Snapshot) {
	var out struct {
		Focus   *Focus `json:"focus"`
		Tree    string `json:"tree"`
		Omitted int    `json:"omitted"`
	}
	if err := p.conn.CallFunctionJSONIn(ctx, p.cx, sandbox, p.withAxe(treeJS), &out, TreeChars); err != nil {
		snap.TreeError = firstLine(err.Error())
		return
	}
	snap.Focus, snap.Tree, snap.TreeOmitted = out.Focus, out.Tree, out.Omitted
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

// axeRunSrc runs axe and names up to three failing elements of each rule by selector and accessible name.
const axeRunSrc = `() => window.__DR_AXE__.run(document, { elementRef: true, runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"] } })
  .then((r) => {
/*NAMES*/
    // axe gives the element itself for nodes in this document and its shadow roots. Nodes in frames have none
    // and stay unnamed.
    const named = (el) => {
      try {
        return el && el.nodeType === 1 ? drClip(drName(el, drRole(el)), 60) : "";
      } catch (e) {
        return "";
      }
    };
    return r.violations.map((v) => ({ id: v.id, impact: v.impact || "", help: v.help, nodes: v.nodes.length,
      elements: v.nodes.slice(0, 3).map((n) => {
        const sel = (n.target || []).map(String).join(" ");
        return { selector: drClip(sel, 120), name: named(n.element) };
      }) }));
  })`

// SetUserAgent replaces navigator.userAgent for the run's tab.
func (p *Page) SetUserAgent(ctx context.Context, ua string) error {
	if err := p.conn.SetUserAgentOverride(ctx, ua, []string{p.cx}); err != nil {
		return err
	}
	p.browser.UserAgent = ua
	return nil
}

// lookMark returns the random value that look.js puts in the markers around quoted text. It is made here,
// outside the page, so a page script cannot predict it. It fails when no randomness is available.
func lookMark() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("no random value for the page markers: %w", err)
	}
	return hex.EncodeToString(b), nil
}
