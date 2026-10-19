package a11y

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
	"github.com/dummy-run/dummy-run/internal/page"
	"github.com/dummy-run/dummy-run/internal/signin"
)

// Tags are the axe-core rule tags the sweep runs: WCAG 2.0, 2.1 and 2.2 at levels A and AA.
var Tags = []string{"wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"}

// Viewport is a named window size in CSS pixels.
type Viewport struct {
	Name          string
	Width, Height int
}

// Viewport presets. They match the sizes the site's earlier pa11y check used.
var (
	Desktop = Viewport{Name: "desktop", Width: 1280, Height: 900}
	Phone   = Viewport{Name: "phone", Width: 375, Height: 740}
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
	Headless  bool
	// Settle is how long to wait after the page loads, for late layout. Default 300 ms.
	Settle time.Duration
	// Log receives one progress line per page. Nil is silent.
	Log io.Writer
	// Session, when set, is loaded into each browser before the first page, so the sweep sees the
	// pages as a signed-in user does. Cookies for hosts that none of the URLs use are skipped.
	Session *signin.Session
}

// Node is one element that failed a rule.
type Node struct {
	Target  string `json:"target"`
	Allowed bool   `json:"allowed,omitempty"`
	Reason  string `json:"allowed_reason,omitempty"`
}

// Finding is one axe rule that failed, or needs review, on one page.
type Finding struct {
	Rule    string   `json:"rule"`
	Impact  string   `json:"impact,omitempty"`
	WCAG    []string `json:"wcag,omitempty"`
	Help    string   `json:"help"`
	HelpURL string   `json:"help_url"`
	Nodes   []Node   `json:"nodes"`
}

// PageResult is one page in one browser, viewport and colour scheme.
type PageResult struct {
	URL          string    `json:"url"`
	Browser      string    `json:"browser"`
	Viewport     string    `json:"viewport"`
	Width        int       `json:"width"`
	Scheme       string    `json:"scheme"`
	Error        string    `json:"error,omitempty"`
	Violations   []Finding `json:"violations"`
	NeedsReview  []Finding `json:"needs_review"`
	DurationMS   int64     `json:"duration_ms"`
	SchemeActual string    `json:"scheme_applied,omitempty"`
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
	Tool           string       `json:"tool"`
	Browser        string       `json:"browser"`
	BrowserVersion string       `json:"browser_version,omitempty"`
	AxeVersion     string       `json:"axe_version,omitempty"`
	Tags           []string     `json:"tags"`
	Results        []PageResult `json:"results"`
	Summary        Summary      `json:"summary"`
}

// Summary totals a report.
type Summary struct {
	Pages         int `json:"pages"`
	Runs          int `json:"runs"`
	Failing       int `json:"failing_nodes"`
	Allowed       int `json:"allowed_nodes"`
	NeedsReview   int `json:"needs_review_rules"`
	PageErrors    int `json:"page_errors"`
	UnusedAllowed int `json:"unused_allow_entries"`
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
	if cfg.Settle == 0 {
		cfg.Settle = 300 * time.Millisecond
	}
	rep := &Report{Tool: "dummyrun a11y", Browser: cfg.Browser, Tags: Tags}
	for _, scheme := range cfg.Schemes {
		if err := sweepScheme(ctx, cfg, scheme, rep); err != nil {
			return rep, err
		}
	}
	rep.summarise(cfg)
	return rep, nil
}

func sweepScheme(ctx context.Context, cfg Config, scheme string, rep *Report) error {
	args, prefs, err := ColourSchemeLaunch(cfg.Browser, scheme)
	if err != nil {
		return err
	}
	b, err := bidi.Launch(ctx, bidi.LaunchOptions{Browser: cfg.Browser, Headless: cfg.Headless, Args: args, FirefoxPrefs: prefs})
	if err != nil {
		return fmt.Errorf("a11y: launch %s: %w", cfg.Browser, err)
	}
	defer b.Close()
	rep.BrowserVersion = b.Version
	if cfg.Session != nil {
		if err := applySession(ctx, b.Conn, cfg); err != nil {
			return err
		}
	}
	cx, err := b.Conn.CreateContext(ctx)
	if err != nil {
		return fmt.Errorf("a11y: open a tab: %w", err)
	}
	for _, vp := range cfg.Viewports {
		if err := b.Conn.SetViewport(ctx, cx, vp.Width, vp.Height); err != nil {
			return fmt.Errorf("a11y: set the %s viewport: %w", vp.Name, err)
		}
		for _, u := range cfg.URLs {
			if err := ctx.Err(); err != nil {
				return err
			}
			res := testPage(ctx, b.Conn, cx, cfg, u, vp, scheme, rep)
			if res.Error != "" && res.SchemeActual == "" && ctx.Err() == nil {
				// A page can stall once on a busy machine. One more try separates that from a real fault.
				res = testPage(ctx, b.Conn, cx, cfg, u, vp, scheme, rep)
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
				_, _ = fmt.Fprintf(cfg.Log, "%s %-7s %-5s %s: %s\n", cfg.Browser, vp.Name, scheme, u, status)
			}
		}
	}
	return nil
}

// applySession loads the session for the hosts the sweep will visit. A session that loads nothing is
// an error, so a sweep never runs signed out by accident.
func applySession(ctx context.Context, conn *bidi.Conn, cfg Config) error {
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
	if got.Cookies == 0 && len(cfg.Session.Origins) == 0 {
		return errors.New("a11y: the session has no cookie that is unexpired and for a host in the URLs")
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

func testPage(ctx context.Context, conn *bidi.Conn, cx string, cfg Config, u string, vp Viewport, scheme string, rep *Report) (res PageResult) {
	started := time.Now()
	res = PageResult{URL: u, Browser: cfg.Browser, Viewport: vp.Name, Width: vp.Width, Scheme: scheme, Violations: []Finding{}, NeedsReview: []Finding{}}
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
		res.Violations = append(res.Violations, finding(r, path, cfg.Allow))
	}
	for _, r := range out.Incomplete {
		res.NeedsReview = append(res.NeedsReview, finding(r, path, nil))
	}
	return res
}

func finding(r axeRule, path string, allow *AllowList) Finding {
	f := Finding{Rule: r.ID, Impact: r.Impact, WCAG: Criteria(r.Tags), Help: r.Help, HelpURL: r.HelpURL, Nodes: []Node{}}
	for _, t := range r.Nodes {
		n := Node{Target: t}
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
	b, _ := json.Marshal(Tags)
	return string(b)
}
