package a11y

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/dummy-run/dummy-run/internal/access"
	"github.com/dummy-run/dummy-run/internal/bidi"
	"github.com/dummy-run/dummy-run/internal/page"
	"github.com/dummy-run/dummy-run/internal/record"
	"github.com/dummy-run/dummy-run/internal/robots"
	"github.com/dummy-run/dummy-run/internal/signin"
	"github.com/dummy-run/dummy-run/internal/telemetry"
)

// ErrRobotsDisallow is returned when robots.txt disallows Dummy Run from every page given. No browser is started.
var ErrRobotsDisallow = errors.New("robots.txt disallows DummyRun from every page")

// Tags are the axe-core rule tags the sweep runs: WCAG 2.0, 2.1 and 2.2 at levels A and AA.
var Tags = []string{"wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"}

// AdviceTags are the axe-core best-practice rules. A rule that only has these tags is not a WCAG failure: its
// results go to a result's Advice list, never fail the sweep and are shown in a report as advice.
var AdviceTags = []string{"best-practice"}

// Viewport is a named window size in CSS pixels.
type Viewport struct {
	Name          string
	Width, Height int
}

// Viewport presets. They match the sizes the site's earlier pa11y check used.
var (
	Desktop = Viewport{Name: "desktop", Width: 1280, Height: 900}
	Phone   = Viewport{Name: "phone", Width: 375, Height: 740}
	// Small is 320 CSS px wide, the width WCAG 1.4.10 sets for reflow, and the narrowest phone in use.
	Small = Viewport{Name: "small", Width: 320, Height: 640}
)

// Colour schemes the sweep can ask the browser for.
const (
	Light = "light"
	Dark  = "dark"
)

// Config says what to sweep.
type Config struct {
	URLs      []string
	Browser   string // chrome or firefox
	Viewports []Viewport
	Schemes   []string // light and/or dark
	Allow     *AllowList
	// TextScales are the system text sizes to test, in percent. Empty means 100 only. Each size is a
	// separate browser launch, because the browser reads the setting from its profile at start.
	TextScales []int
	// Layout adds the layout checks to each run: sideways overflow, clipped text, overlapping controls
	// and small targets. A non-100 text scale turns it on.
	Layout bool
	// Keyboard adds the Tab walk to each run: it presses Tab through the page and reports focus that cannot be seen,
	// focus that cannot leave, and a skip link that goes nowhere.
	Keyboard bool
	// Markup adds the markup checks to each run: a control whose visible words are missing from its accessible
	// name, and a field for the person's own details with no autocomplete value.
	Markup bool
	// Consent answers a cookie banner with DuckDuckGo autoconsent, choosing to opt out, after the rule engines have
	// seen it and before the layout checks, so the banner does not cover the page they measure.
	Consent bool
	// ImageContrast measures the text axe could not judge for contrast against the pixels behind it, after any
	// cookie banner has been answered.
	ImageContrast bool
	// PlainLanguage checks the main content of each page with Vale: readability scores, long sentences, words the
	// GOV.UK style guide asks you to avoid and Latin abbreviations. The results are advice.
	PlainLanguage bool
	// Engines runs IBM Equal Access, HTML_CodeSniffer and Siteimprove Alfa beside axe-core, and counts how many
	// engines agree on each failing element.
	Engines bool
	// ScreenshotDir, when set, receives a PNG crop of each element the layout checks flag.
	ScreenshotDir string
	Headless      bool
	// Identify adds a contact URL to the browser's user agent and to the requests for robots.txt.
	Identify bool
	// RespectRobots skips pages that the robots.txt of their origin disallows for DummyRun or *.
	RespectRobots bool
	// HTTPClient fetches robots.txt. Nil uses a default client.
	HTTPClient *http.Client
	// Settle is how long to wait after the page loads, for late layout. Default 300 ms.
	Settle time.Duration
	// Log receives one progress line per page. Nil is silent.
	Log io.Writer
	// Session, when set, is loaded into each browser before the first page, so the sweep sees the
	// pages as a signed-in user does. Cookies for hosts that none of the URLs use are skipped.
	Session *signin.Session
	// Access gets the browser past Basic auth or header protection on a preview site. Its secrets go only to
	// the origins its scope names. Nil means none.
	Access *access.Resolved
}

// Node is one element that failed a rule.
type Node struct {
	Target  string `json:"target"`
	Allowed bool   `json:"allowed,omitempty"`
	Reason  string `json:"allowed_reason,omitempty"`
	// Detail is a measurement in words, such as the size of a small target. Layout checks fill it.
	Detail string `json:"detail,omitempty"`
	// Engines are the rule engines that report this element for the same criterion, when the sweep ran --engines.
	Engines []string `json:"engines,omitempty"`
	// Confidence is high, medium, low or unknown: how often findings with this element's inputs (the kind of
	// evidence, the rule and the engines that agree) have turned out to be right, looked up in the confidence table.
	Confidence string `json:"confidence,omitempty"`
	// Screenshot is the path of a PNG crop of the element. Layout checks fill it when screenshots are on.
	Screenshot string `json:"screenshot,omitempty"`
}

// Finding is one axe rule that failed, or needs review, on one page.
type Finding struct {
	Rule    string   `json:"rule"`
	Impact  string   `json:"impact,omitempty"`
	WCAG    []string `json:"wcag,omitempty"`
	Help    string   `json:"help"`
	HelpURL string   `json:"help_url"`
	// Evidence is the kind of evidence behind the rule: engine-rule for axe and the other engines, measurement for
	// the layout and keyboard checks, ax-fact for the markup checks.
	Evidence string `json:"evidence,omitempty"`
	Nodes    []Node `json:"nodes"`
}

// PageResult is one page in one browser, viewport, colour scheme and text size.
type PageResult struct {
	URL      string `json:"url"`
	Browser  string `json:"browser"`
	Viewport string `json:"viewport"`
	Width    int    `json:"width"`
	Scheme   string `json:"scheme"`
	// TextScale is the system text size in percent the run used. Zero means 100 and is left out.
	TextScale int `json:"text_scale,omitempty"`
	// MarkupChecked says the markup checks ran, so a result with none of their rules means they found nothing.
	MarkupChecked bool `json:"markup_checked,omitempty"`
	// LayoutChecked says the layout checks ran, so a result with none of their rules means they found nothing.
	LayoutChecked bool `json:"layout_checked,omitempty"`
	// KeyboardChecked says the Tab walk ran, so a result with none of its rules means it found nothing.
	KeyboardChecked bool `json:"keyboard_checked,omitempty"`
	// KeyboardPartial says the Tab walk stopped before it had been through every stop, so later stops went unchecked.
	KeyboardPartial bool `json:"keyboard_partial,omitempty"`
	// ImageContrast holds each measurement of text against the pixels behind it, when the sweep ran
	// --image-contrast. ImageContrastChecked says the check ran, even when there was nothing to measure.
	// ImageContrastUnmeasured counts the elements it was asked about and could not measure: ones inside a shadow
	// root or frame, ones it couldn't find or picture, and any past the limit for one page.
	ImageContrast           []page.ImageContrast `json:"image_contrast,omitempty"`
	ImageContrastChecked    bool                 `json:"image_contrast_checked,omitempty"`
	ImageContrastUnmeasured int                  `json:"image_contrast_unmeasured,omitempty"`
	// PlainLanguageChecked says Vale read the page's main content. PlainLanguagePartial says the content was too
	// long, so Vale read only its start. PlainLanguageError says why it could not read it.
	PlainLanguageChecked bool   `json:"plain_language_checked,omitempty"`
	PlainLanguagePartial bool   `json:"plain_language_partial,omitempty"`
	PlainLanguageError   string `json:"plain_language_error,omitempty"`
	// Consent says what happened to the page's cookie banner, when the sweep ran --consent.
	Consent *page.Consent `json:"consent,omitempty"`
	// EnginesRan names the other rule engines that finished on the page. EngineErrors says why any others did not.
	EnginesRan   []string          `json:"engines_ran,omitempty"`
	EngineErrors map[string]string `json:"engine_errors,omitempty"`
	// EngineGroupsOmitted counts the other engines' failures left out to keep the result small. The ones more
	// engines agree on are kept first.
	EngineGroupsOmitted int    `json:"engine_groups_omitted,omitempty"`
	Error               string `json:"error,omitempty"`
	// Skipped says why a page was not tested, such as robots.txt disallowing it. A skipped page is not an error.
	Skipped     string    `json:"skipped,omitempty"`
	Violations  []Finding `json:"violations"`
	NeedsReview []Finding `json:"needs_review"`
	// Advice holds the best-practice rules that failed and have no WCAG criterion. They never fail the sweep.
	Advice []Finding `json:"advice,omitempty"`
	// Candidates are failures one other engine reports and axe does not. They are kept for review and never fail
	// the sweep.
	Candidates   []Finding `json:"candidates,omitempty"`
	DurationMS   int64     `json:"duration_ms"`
	SchemeActual string    `json:"scheme_applied,omitempty"`
}

// Setting names the browser, viewport and colour scheme of the run, and the text size when it is not 100.
func (p PageResult) Setting() string {
	s := fmt.Sprintf("%s/%s/%s", p.Browser, p.Viewport, p.Scheme)
	if p.TextScale > 100 {
		s += fmt.Sprintf("/text-%d", p.TextScale)
	}
	return s
}

// Failing counts the violating nodes that are not on the allow-list.
func (p PageResult) Failing() int {
	n := 0
	for _, f := range p.Violations {
		for _, node := range f.Nodes {
			if !node.Allowed {
				n++
			}
		}
	}
	return n
}

// Report is the result of a sweep.
type Report struct {
	Tool           string `json:"tool"`
	Browser        string `json:"browser"`
	BrowserVersion string `json:"browser_version,omitempty"`
	AxeVersion     string `json:"axe_version,omitempty"`
	// Engines are the versions of the other rule engines, when the sweep ran them.
	Engines map[string]string `json:"engines,omitempty"`
	// Autoconsent is the version of DuckDuckGo autoconsent, when the sweep answered cookie banners.
	Autoconsent string   `json:"autoconsent,omitempty"`
	Tags        []string `json:"tags"`
	// AdviceTags are the best-practice tags the sweep also ran. Empty in a sweep written before advice.
	AdviceTags    []string `json:"advice_tags,omitempty"`
	Identify      bool     `json:"identify,omitempty"`
	RespectRobots bool     `json:"respect_robots,omitempty"`
	// PreviewAccess names the protection the sweep answered. Names only, never a value.
	PreviewAccess *access.Summary `json:"preview_access,omitempty"`
	Results       []PageResult    `json:"results"`
	Summary       Summary         `json:"summary"`
}

// Summary totals a report.
type Summary struct {
	Pages         int `json:"pages"`
	Runs          int `json:"runs"`
	Failing       int `json:"failing_nodes"`
	Allowed       int `json:"allowed_nodes"`
	NeedsReview   int `json:"needs_review_rules"`
	PageErrors    int `json:"page_errors"`
	Skipped       int `json:"skipped_pages,omitempty"`
	UnusedAllowed int `json:"unused_allow_entries"`
	// Advice counts the elements that failed a best-practice rule. They never fail the sweep.
	Advice int `json:"advice_nodes,omitempty"`
	// Candidates counts the elements one other engine reports alone. They never fail the sweep.
	Candidates int `json:"candidate_nodes,omitempty"`
}

// Failed reports whether the sweep should exit non-zero: any violation not on the allow-list, or
// any page that could not be tested.
func (r *Report) Failed() bool { return r.Summary.Failing > 0 || r.Summary.PageErrors > 0 }

var wcagTag = regexp.MustCompile(`^wcag(\d)(\d)(\d+)$`)

// Criteria turns axe tags such as "wcag143" into "1.4.3". Level tags such as "wcag21aa" are skipped.
func Criteria(tags []string) []string {
	var out []string
	for _, t := range tags {
		if m := wcagTag.FindStringSubmatch(t); m != nil {
			out = append(out, m[1]+"."+m[2]+"."+m[3])
		}
	}
	sort.Strings(out)
	return out
}

// ColourSchemeLaunch returns the browser launch settings that force a colour scheme. BiDi has no
// command for the colour scheme media feature, so the scheme is fixed when the browser starts. Both schemes
// are set explicitly, so a light run is light on a machine set to dark.
func ColourSchemeLaunch(browser, scheme string) (args []string, prefs map[string]any, err error) {
	if scheme != Light && scheme != Dark {
		return nil, nil, fmt.Errorf("colour scheme %q: want light or dark", scheme)
	}
	switch browser {
	case "chrome":
		// Blink's preferredColorScheme is 0 for dark and 1 for light.
		if scheme == Dark {
			return []string{"--blink-settings=preferredColorScheme=0", "--force-dark-mode"}, nil, nil
		}
		return []string{"--blink-settings=preferredColorScheme=1"}, nil, nil
	case "firefox":
		// 0 is dark, 1 is light, 2 follows the system.
		v := 1
		if scheme == Dark {
			v = 0
		}
		return nil, map[string]any{"layout.css.prefers-color-scheme.content-override": v}, nil
	}
	return nil, nil, fmt.Errorf("browser %q: want chrome or firefox", browser)
}

// Run sweeps every URL at every viewport and colour scheme in one browser. A page that fails to
// load is recorded as an error in its result and the sweep carries on.
func Run(ctx context.Context, cfg Config) (*Report, error) {
	if len(cfg.URLs) == 0 {
		return nil, errors.New("a11y: no URLs to test")
	}
	if len(cfg.Viewports) == 0 {
		cfg.Viewports = []Viewport{Desktop, Phone}
	}
	if len(cfg.Schemes) == 0 {
		cfg.Schemes = []string{Light, Dark}
	}
	if len(cfg.TextScales) == 0 {
		cfg.TextScales = []int{100}
	}
	for _, n := range cfg.TextScales {
		if n < bidi.MinTextScale || n > bidi.MaxTextScale {
			return nil, fmt.Errorf("a11y: text scale %d percent is outside %d to %d", n, bidi.MinTextScale, bidi.MaxTextScale)
		}
		if n > 100 {
			cfg.Layout = true
		}
	}
	if cfg.Settle == 0 {
		cfg.Settle = 300 * time.Millisecond
	}
	rep := &Report{Tool: "dummyrun a11y", Browser: cfg.Browser, Tags: Tags, AdviceTags: AdviceTags, Identify: cfg.Identify, RespectRobots: cfg.RespectRobots}
	if cfg.Engines {
		rep.Engines = page.EngineVersions
	}
	if cfg.Consent {
		rep.Autoconsent = page.AutoconsentVersion
	}
	if cfg.Access.Active() {
		sum := cfg.Access.Summary()
		rep.PreviewAccess = &sum
		cfg.HTTPClient = access.WrapClient(cfg.HTTPClient, cfg.Access)
	}
	var skipped []PageResult
	if cfg.RespectRobots {
		var err error
		if cfg.URLs, skipped, err = filterRobots(ctx, cfg); err != nil {
			return rep, err
		}
		if len(cfg.URLs) == 0 {
			rep.Results = skipped
			rep.summarise(cfg)
			return rep, ErrRobotsDisallow
		}
	}
	for _, scale := range cfg.TextScales {
		for _, scheme := range cfg.Schemes {
			if err := sweepScheme(ctx, cfg, scheme, scale, rep); err != nil {
				return rep, err
			}
		}
	}
	rep.Results = append(rep.Results, skipped...)
	rep.summarise(cfg)
	return rep, nil
}

// filterRobots fetches robots.txt once for each origin and splits the URLs into those that may be tested and
// results for those that may not.
func filterRobots(ctx context.Context, cfg Config) (allowed []string, skipped []PageResult, err error) {
	ua := telemetry.RequestUserAgent(cfg.Identify)
	byOrigin := map[string]*robots.Result{}
	for _, raw := range cfg.URLs {
		origin, oerr := robots.Origin(raw)
		if oerr != nil {
			allowed = append(allowed, raw) // the page load reports a bad URL
			continue
		}
		res := byOrigin[origin]
		if res == nil {
			if res, err = robots.Fetch(ctx, cfg.HTTPClient, raw, telemetry.RobotsToken, ua); err != nil {
				return nil, nil, err
			}
			byOrigin[origin] = res
			if cfg.Log != nil {
				_, _ = fmt.Fprintf(cfg.Log, "robots: %s: %s\n", origin, res.Status)
			}
		}
		u, _ := url.Parse(raw)
		if u != nil && !res.Rules.Allowed(u.RequestURI()) {
			skipped = append(skipped, PageResult{URL: raw, Browser: cfg.Browser, Skipped: "robots.txt disallows DummyRun (" + res.Status + ")",
				Violations: []Finding{}, NeedsReview: []Finding{}})
			continue
		}
		allowed = append(allowed, raw)
	}
	return allowed, skipped, nil
}

func sweepScheme(ctx context.Context, cfg Config, scheme string, textScale int, rep *Report) error {
	args, prefs, err := ColourSchemeLaunch(cfg.Browser, scheme)
	if err != nil {
		return err
	}
	lo := bidi.LaunchOptions{Browser: cfg.Browser, Headless: cfg.Headless, Args: args, FirefoxPrefs: prefs}
	if lo, err = bidi.WithTextScale(lo, textScale); err != nil {
		return err
	}
	b, err := bidi.Launch(ctx, lo)
	if err != nil {
		return fmt.Errorf("a11y: launch %s: %w", cfg.Browser, err)
	}
	defer b.Close()
	rep.BrowserVersion = b.Version
	if err := access.Attach(ctx, b.Conn, cfg.Access); err != nil {
		return fmt.Errorf("a11y: set up preview access: %w", err)
	}
	if cfg.Session != nil {
		if err := applySession(ctx, b.Conn, cfg); err != nil {
			return err
		}
	}
	cx, err := b.Conn.CreateContext(ctx)
	if err != nil {
		return fmt.Errorf("a11y: open a tab: %w", err)
	}
	if cfg.Identify {
		// The browser's own user agent stays in front, so pages render as they do for the browser.
		v, err := b.Conn.Evaluate(ctx, cx, "navigator.userAgent")
		if err != nil {
			return fmt.Errorf("a11y: read the user agent: %w", err)
		}
		browserUA, _ := v.String()
		if err := b.Conn.SetUserAgentOverride(ctx, telemetry.UserAgent(browserUA, true), []string{cx}); err != nil {
			return fmt.Errorf("a11y: set the user agent: %w", err)
		}
	}
	for _, vp := range cfg.Viewports {
		if err := b.Conn.SetViewport(ctx, cx, vp.Width, vp.Height); err != nil {
			return fmt.Errorf("a11y: set the %s viewport: %w", vp.Name, err)
		}
		for _, u := range cfg.URLs {
			if err := ctx.Err(); err != nil {
				return err
			}
			res := testPage(ctx, b.Conn, cx, cfg, u, vp, scheme, textScale, rep)
			if res.Error != "" && res.SchemeActual == "" && ctx.Err() == nil {
				// A page can stall once on a busy machine. One more try separates that from a real fault.
				res = testPage(ctx, b.Conn, cx, cfg, u, vp, scheme, textScale, rep)
			}
			rep.Results = append(rep.Results, res)
			if cfg.Log != nil {
				status := "ok"
				switch {
				case res.Error != "":
					status = "ERROR " + res.Error
				case res.Failing() > 0:
					status = fmt.Sprintf("%d failing", res.Failing())
				}
				shade := scheme
				if textScale > 100 {
					shade += fmt.Sprintf(" text %d%%", textScale)
				}
				_, _ = fmt.Fprintf(cfg.Log, "%s %-7s %-5s %s: %s\n", cfg.Browser, vp.Name, shade, u, status)
			}
		}
	}
	return nil
}

// applySession loads the session for the hosts the sweep will visit. A session that loads nothing is
// an error, so a sweep never runs signed out by accident.
func applySession(ctx context.Context, conn signin.StorageConn, cfg Config) error {
	hosts := map[string]bool{}
	for _, raw := range cfg.URLs {
		if u, err := url.Parse(raw); err == nil {
			hosts[strings.ToLower(u.Hostname())] = true
		}
	}
	got, err := cfg.Session.Apply(ctx, conn, func(h string) bool { return hosts[strings.ToLower(h)] }, time.Now())
	if err != nil {
		return fmt.Errorf("a11y: load the session: %w", err)
	}
	if got.Cookies == 0 && got.Storage == 0 {
		return errors.New("a11y: the session has no cookie or local storage that is unexpired and for a host in the URLs (a cookie also needs the Secure flag unless the host is local)")
	}
	return nil
}

type axeNode = string

type axeRule struct {
	ID      string    `json:"id"`
	Impact  string    `json:"impact"`
	Help    string    `json:"help"`
	HelpURL string    `json:"helpUrl"`
	Tags    []string  `json:"tags"`
	Nodes   []axeNode `json:"nodes"`
}

type axeOut struct {
	Version    string    `json:"version"`
	Scheme     string    `json:"scheme"`
	Violations []axeRule `json:"violations"`
	Incomplete []axeRule `json:"incomplete"`
}

func testPage(ctx context.Context, conn *bidi.Conn, cx string, cfg Config, u string, vp Viewport, scheme string, textScale int, rep *Report) (res PageResult) {
	started := time.Now()
	res = PageResult{URL: u, Browser: cfg.Browser, Viewport: vp.Name, Width: vp.Width, Scheme: scheme, TextScale: textScaleField(textScale), Violations: []Finding{}, NeedsReview: []Finding{}}
	defer func() { res.DurationMS = time.Since(started).Milliseconds() }()
	pctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if _, err := conn.Navigate(pctx, cx, u); err != nil {
		res.Error = "load: " + firstLine(err.Error())
		return res
	}
	// Fonts and late layout can change contrast and target sizes, so wait for them.
	_, _ = conn.Evaluate(pctx, cx, "document.fonts ? document.fonts.ready.then(() => true) : true")
	select {
	case <-time.After(cfg.Settle):
	case <-pctx.Done():
	}
	var loaded bool
	if err := conn.CallFunctionJSON(pctx, cx, "function(){ if (!window.axe) { "+page.AxeSource()+"\n } return !!window.axe; }", &loaded); err != nil || !loaded {
		res.Error = "inject axe-core: " + errText(err, "axe-core did not load")
		return res
	}
	var out axeOut
	if err := conn.CallFunctionJSON(pctx, cx, axeRunJS, &out, tagsArg()); err != nil {
		res.Error = "axe.run: " + firstLine(err.Error())
		return res
	}
	if out.Version != "" {
		rep.AxeVersion = out.Version
	}
	res.SchemeActual = out.Scheme
	if out.Scheme != scheme {
		res.Error = fmt.Sprintf("the browser rendered the %s colour scheme, not %s", out.Scheme, scheme)
		return res
	}
	path := pagePath(u)
	for _, r := range out.Violations {
		if adviceOnly(r.Tags) {
			res.Advice = append(res.Advice, finding(r, path, cfg.Allow))
			continue
		}
		res.Violations = append(res.Violations, finding(r, path, cfg.Allow))
	}
	for _, r := range out.Incomplete {
		// A best-practice rule a person would have to check is noise, so only WCAG rules need review.
		if adviceOnly(r.Tags) {
			continue
		}
		res.NeedsReview = append(res.NeedsReview, finding(r, path, nil))
	}
	if cfg.Engines {
		r, err := page.RunEngines(pctx, conn, cx, axeNodes(res.Violations), CriteriaAA())
		if err != nil {
			res.Error = firstLine(err.Error())
			return res
		}
		res.EnginesRan, res.EngineGroupsOmitted = r.Ran, r.Omitted
		if len(r.Errors) > 0 {
			res.EngineErrors = r.Errors
		}
		if msg := enginesFailed(r); msg != "" {
			res.Error = firstLine(msg)
			return res
		}
		agreed, cands := engineFindings(r, res.Violations, path, cfg.Allow)
		res.Violations = append(res.Violations, agreed...)
		res.Candidates = cands
	}
	if cfg.PlainLanguage {
		plainCheck(pctx, conn, cx, &res, path, cfg.Allow)
	}
	if cfg.Consent {
		c, err := page.AnswerConsent(pctx, conn, cx, page.ConsentWait)
		if err != nil {
			res.Error = firstLine(err.Error())
			return res
		}
		res.Consent = c
		if msg := consentFailed(c); msg != "" {
			res.Error = msg
			return res
		}
		res.Advice = append(res.Advice, consentFindings(c, path, cfg.Allow)...)
	}
	if cfg.ImageContrast {
		targets := imageContrastTargets(res.NeedsReview)
		ms, err := page.MeasureImageContrast(pctx, conn, cx, targets)
		if err != nil {
			res.Error = firstLine(err.Error())
			return res
		}
		res.ImageContrastChecked, res.ImageContrast = true, ms
		res.ImageContrastUnmeasured = len(targets) - len(ms)
		res.Violations = append(res.Violations, imageContrastFindings(ms, path, cfg.Allow)...)
	}
	if cfg.Layout {
		l, err := page.MeasureLayout(pctx, conn, cx, true)
		if err != nil {
			res.Error = "layout checks: " + firstLine(err.Error())
			return res
		}
		res.LayoutChecked = true
		found, boxes := layoutFindings(l, path, cfg.Allow)
		res.Violations = append(res.Violations, found...)
		shoot(pctx, conn, cx, cfg.ScreenshotDir, &res, boxes)
	}
	if cfg.Keyboard {
		w, err := page.WalkTabs(pctx, conn, cx, page.MaxTabStops)
		if err != nil {
			res.Error = "keyboard checks: " + firstLine(err.Error())
			return res
		}
		res.KeyboardChecked = true
		res.KeyboardPartial = !w.Completed
		res.Violations = append(res.Violations, keyboardFindings(w, path, cfg.Allow)...)
	}
	if cfg.Markup {
		m, err := page.RunMarkupChecks(pctx, conn, cx)
		if err != nil {
			res.Error = "markup checks: " + firstLine(err.Error())
			return res
		}
		res.MarkupChecked = true
		res.Violations = append(res.Violations, markupFindings(m, path, cfg.Allow)...)
	}
	return res
}

// adviceOnly is true for a rule that has none of the WCAG A and AA level tags the sweep tests: a best-practice
// rule, which is advice and not a failure. The sweep runs no AAA tag, so an AAA rule is here only when it is also
// a best-practice rule.
func adviceOnly(tags []string) bool {
	for _, t := range tags {
		if slices.Contains(Tags, t) {
			return false
		}
	}
	return true
}

// textScaleField is the value kept in a result: 100 is left out.
func textScaleField(n int) int {
	if n > 100 {
		return n
	}
	return 0
}

func finding(r axeRule, path string, allow *AllowList) Finding {
	f := Finding{Rule: r.ID, Impact: r.Impact, WCAG: Criteria(r.Tags), Help: r.Help, HelpURL: r.HelpURL, Evidence: record.EvidenceEngineRule, Nodes: []Node{}}
	for _, t := range r.Nodes {
		n := Node{Target: t, Confidence: rate(record.EvidenceEngineRule, r.ID, []string{"axe"})}
		if e := allow.Match(r.ID, path, t); e != nil {
			n.Allowed, n.Reason = true, e.Reason
		}
		f.Nodes = append(f.Nodes, n)
	}
	return f
}

func pagePath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Path == "" {
		return "/"
	}
	return u.Path
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func errText(err error, fallback string) string {
	if err != nil {
		return firstLine(err.Error())
	}
	return fallback
}

func (r *Report) summarise(cfg Config) {
	pages := map[string]bool{}
	for _, p := range r.Results {
		if p.Skipped != "" {
			r.Summary.Skipped++
			continue
		}
		pages[p.URL] = true
		r.Summary.Runs++
		if p.Error != "" {
			r.Summary.PageErrors++
		}
		for _, f := range p.Violations {
			for _, n := range f.Nodes {
				if n.Allowed {
					r.Summary.Allowed++
				} else {
					r.Summary.Failing++
				}
			}
		}
		r.Summary.NeedsReview += len(p.NeedsReview)
		for _, f := range p.Advice {
			for _, n := range f.Nodes {
				if !n.Allowed {
					r.Summary.Advice++
				}
			}
		}
		for _, f := range p.Candidates {
			for _, n := range f.Nodes {
				if !n.Allowed {
					r.Summary.Candidates++
				}
			}
		}
	}
	r.Summary.Pages = len(pages)
	r.Summary.UnusedAllowed = len(cfg.Allow.Unused())
}

// axeRunJS runs axe-core in the page and returns plain data. Each node is its selector, with the
// parts of a selector that crosses an iframe or shadow root joined by " >> ".
//
//nolint:misspell // the CSS media feature name is prefers-color-scheme
const axeRunJS = `(tagsJSON) => {
  const flat = (t) => (Array.isArray(t) ? t.map(flat).join(" >> ") : String(t));
  const rules = (rs) => rs.map((v) => ({ id: v.id, impact: v.impact || "", help: v.help, helpUrl: v.helpUrl, tags: v.tags, nodes: v.nodes.map((n) => flat(n.target)) }));
  return window.axe.run(document, { runOnly: { type: "tag", values: JSON.parse(tagsJSON) }, resultTypes: ["violations", "incomplete"] }).then((r) => ({
    version: window.axe.version,
    scheme: window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light",
    violations: rules(r.violations),
    incomplete: rules(r.incomplete),
  }));
}`

// tagsArg is the rule tags as a JSON string, passed to the page as an argument rather than spliced
// into the script.
func tagsArg() string {
	b, _ := json.Marshal(append(append([]string{}, Tags...), AdviceTags...))
	return string(b)
}
