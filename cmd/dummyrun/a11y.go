package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	"github.com/dummy-run/dummy-run/internal/a11y"
	"github.com/dummy-run/dummy-run/internal/access"
	"github.com/dummy-run/dummy-run/internal/cast"
	"github.com/dummy-run/dummy-run/internal/signin"
)

const a11yUsage = `Usage:
  dummyrun a11y [--sitemap URL] [--rebase URL] [URL...] [flags]

Runs the axe-core WCAG 2.2 A and AA rules on each page in a real browser, plus axe's best-practice rules, which
are reported apart as advice and never fail the sweep. No model is called.

Flags:
  --sitemap URL     a sitemap (or sitemap index) listing the pages to test
  --rebase URL      move every sitemap URL onto this origin, to test a local copy of a site
  --browser NAME    chrome or firefox (default chrome)
  --viewport NAME   desktop (1280), phone (375), small (320) or both (desktop and phone, the default)
  --scheme NAME     light, dark or both (default both)
  --text-scale N    the system text size in percent, 100 to 300, such as 200. A comma-separated list such as
                    100,200 tests each size. The browser's default font size is raised and the page is not zoomed,
                    so rem and em text grows and px text may not. A size above 100 turns on --layout
  --all             run every check listed below in the same sweep, including the layout checks at 320 px and at
                    200 percent text
  --consent         also answer a cookie banner by rejecting, with DuckDuckGo autoconsent, before the layout checks.
                    A banner with an accept button and no reject button is advice (dummyrun-consent-no-reject)
  --engines         also run IBM Equal Access, HTML_CodeSniffer and Siteimprove Alfa beside axe-core. An element two
                    or more engines fail for the same WCAG criterion has high confidence. A failure axe misses and
                    two others agree on is a violation (rule dummyrun-engines-<criterion>); one that only one engine
                    reports is a candidate, kept in the JSON and never failing the sweep
  --image-contrast  also measure the text axe could not judge for contrast, usually text on an image or gradient:
                    hide the text, take a picture of what is behind it and compare the text colour with every pixel.
                    Text that fails at the middle pixel is a violation (dummyrun-contrast-over-image, WCAG 1.4.3)
  --keyboard        also press Tab through each page and report focus you cannot see, focus that gets stuck and a
                    skip link that goes nowhere
  --layout          also run the layout checks, measured from what the browser lays out and paints: sideways
                    scrolling, text cut off by its box, a control covered by another, targets under 24 by 24 CSS
                    px and a control hidden behind other content when it has focus. Rules dummyrun-reflow,
                    dummyrun-clipped-text, dummyrun-overlap, dummyrun-target-size and dummyrun-focus-obscured.
                    They fail the sweep like axe rules and the allow-list can accept them. Unless you set
                    --viewport or --text-scale, it adds the 320 px window and a run at 200 percent text, the
                    two settings WCAG 1.4.10 and 1.4.4 name
  --markup          also run the markup checks: a control whose visible words are missing from its name, and a field for
                    your own details with no purpose-specific autocomplete value
  --plain-language  also check the main content of each page with Vale: Flesch reading ease and grade, sentences
                    over 25 words, words the GOV.UK style guide asks you to avoid and Latin abbreviations. The
                    results are advice. Needs vale on the PATH or DUMMYRUN_VALE
  --screenshots DIR write a PNG crop of the elements the layout checks flag into DIR, at most 12 for each page and
                    setting
  --allow FILE      accepted violations, one per line: rule | page | selector | reason
  --json FILE       write the full report as JSON; "-" writes it to standard output
  --identify        add "DummyRun/<version> (+https://dummyrun.dev/scanner/)" to the browser's user agent
  --respect-robots  fetch robots.txt from each page's origin and skip pages it disallows for DummyRun or *
  --session FILE    a saved session (see "dummyrun session save"), loaded before the first page so the pages are
                    tested signed in. The file holds cookies: keep it private
  --basic-auth-env USER_ENV:PASS_ENV
                    answer HTTP Basic auth on a protected preview with the user name and password in these
                    environment variables. Give the variable names, never the secrets
  --header-env 'Name:ENV'
                    send a request header whose value is in an environment variable, such as
                    x-vercel-protection-bypass or CF-Access-Client-Id. Repeat for more. Both go only to the
                    origins of the URLs, the sitemap and --rebase, never to another host, and never into the report
  --headful         show the browser window

Exit status: 0 when nothing fails, 1 when a violation is not in the allow-list or a page cannot be
tested, 2 for a usage error, 3 when robots.txt disallows every page. Pages skipped for robots.txt do not fail the sweep.`

// a11yRun runs the sweep. Tests replace it to see the configuration the flags make.
var a11yRun = a11y.Run

func a11yCmd(args []string) int {
	fs := flag.NewFlagSet("a11y", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprintln(os.Stderr, a11yUsage) }
	sitemap := fs.String("sitemap", "", "sitemap URL")
	rebase := fs.String("rebase", "", "origin to move sitemap URLs onto")
	browser := fs.String("browser", "chrome", "chrome or firefox")
	viewport := fs.String("viewport", "both", "desktop, phone, small or both")
	scheme := fs.String("scheme", "both", "light, dark or both")
	textScale := fs.String("text-scale", "", "system text size in percent, or a comma-separated list")
	all := fs.Bool("all", false, "run every check")
	layout := fs.Bool("layout", false, "run the layout checks")
	keyboard := fs.Bool("keyboard", false, "run the keyboard checks")
	markup := fs.Bool("markup", false, "run the markup checks")
	engines := fs.Bool("engines", false, "run the other rule engines beside axe-core")
	consent := fs.Bool("consent", false, "answer cookie banners by rejecting")
	plainLang := fs.Bool("plain-language", false, "check the main content with Vale")
	imageContrast := fs.Bool("image-contrast", false, "measure text contrast against the pixels behind it")
	shotDir := fs.String("screenshots", "", "directory for PNG crops of flagged elements")
	allowPath := fs.String("allow", "", "allow-list file")
	jsonPath := fs.String("json", "", "JSON report file, or - for standard output")
	sessionPath := fs.String("session", "", "saved session file to load before the sweep")
	headful := fs.Bool("headful", false, "show the browser window")
	identify := fs.Bool("identify", false, "name the scanner page in the user agent")
	respect := fs.Bool("respect-robots", false, "skip pages that robots.txt disallows for DummyRun or *")
	var preview authFlags
	preview.register(fs)
	// Flags may come after the URLs, so parse in a loop that peels off positional arguments.
	var urls []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			return 2
		}
		if fs.NArg() == 0 {
			break
		}
		urls = append(urls, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	if *browser != "chrome" && *browser != "firefox" {
		fmt.Fprintf(os.Stderr, "dummyrun a11y: --browser %q: want chrome or firefox\n", *browser)
		return 2
	}
	viewports, ok := pick(*viewport, map[string][]a11y.Viewport{
		"desktop": {a11y.Desktop}, "phone": {a11y.Phone}, "small": {a11y.Small}, "both": {a11y.Desktop, a11y.Phone}})
	if !ok {
		fmt.Fprintf(os.Stderr, "dummyrun a11y: --viewport %q: want desktop, phone, small or both\n", *viewport)
		return 2
	}
	schemes, ok := pick(*scheme, map[string][]string{
		a11y.Light: {a11y.Light}, a11y.Dark: {a11y.Dark}, "both": {a11y.Light, a11y.Dark}})
	if !ok {
		fmt.Fprintf(os.Stderr, "dummyrun a11y: --scheme %q: want light, dark or both\n", *scheme)
		return 2
	}
	if *all {
		// One switch for every check. A check added later joins this list.
		for _, on := range []*bool{layout, keyboard, markup, engines, consent, plainLang, imageContrast} {
			*on = true
		}
	}
	scales, err := parseTextScales(*textScale)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dummyrun a11y: --text-scale: %v\n", err)
		return 2
	}
	viewports, scales = layoutSettings(*layout, flagsSet(fs), viewports, scales)
	if err := checkScreenshotFlags(*shotDir, *layout, scales); err != nil {
		fmt.Fprintln(os.Stderr, "dummyrun a11y:", err)
		return 2
	}
	if *sitemap == "" && len(urls) == 0 {
		fmt.Fprintln(os.Stderr, "dummyrun a11y: give at least one URL or --sitemap")
		fmt.Fprintln(os.Stderr, a11yUsage)
		return 2
	}
	var allow *a11y.AllowList
	if *allowPath != "" {
		f, err := os.Open(*allowPath) //nolint:gosec // the path comes from the command line on purpose
		if err != nil {
			return fail(err)
		}
		allow, err = a11y.ParseAllowList(f)
		_ = f.Close()
		if err != nil {
			return fail(fmt.Errorf("%s: %w", *allowPath, err))
		}
	}
	var session *signin.Session
	if *sessionPath != "" {
		var err error
		if session, err = signin.LoadSession(*sessionPath); err != nil {
			return fail(err)
		}
	}
	auth, err := preview.auth(nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dummyrun a11y:", err)
		return 2
	}
	// The secrets go only to the origins you name: the URLs, the sitemap and the --rebase origin.
	pa, err := access.Resolve(auth, os.Getenv, access.OriginScope(append([]string{*sitemap, *rebase}, urls...)))
	if err != nil {
		return fail(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *sitemap != "" {
		pages, err := a11y.FetchSitemap(ctx, access.WrapClient(requestClient(*identify), pa), *sitemap, robotsGuard(access.WrapClient(requestClient(*identify), pa), *identify, *respect))
		if err != nil {
			return fail(err)
		}
		if *rebase != "" {
			if pages, err = a11y.Rebase(pages, *rebase); err != nil {
				return fail(err)
			}
		}
		urls = append(pages, urls...)
	}
	// With the JSON on standard output, progress goes to standard error so the JSON stays clean.
	rep, err := a11yRun(ctx, a11y.Config{
		URLs: urls, Browser: *browser, Viewports: viewports, Schemes: schemes, Allow: allow,
		TextScales: scales, Layout: *layout, Keyboard: *keyboard, Markup: *markup, Engines: *engines, Consent: *consent, PlainLanguage: *plainLang, ImageContrast: *imageContrast, ScreenshotDir: *shotDir,
		Headless: !*headful, Log: os.Stderr, Session: session, Identify: *identify, RespectRobots: *respect, Access: pa,
	})
	if errors.Is(err, a11y.ErrRobotsDisallow) {
		rep.WriteSummary(os.Stderr)
		if *jsonPath != "" {
			if werr := writeReport(rep, *jsonPath); werr != nil {
				return fail(werr)
			}
		}
		fmt.Fprintln(os.Stderr, "dummyrun a11y:", err)
		return exitRobots
	}
	if err != nil {
		return fail(err)
	}
	summaryOut := os.Stdout
	if *jsonPath == "-" {
		summaryOut = os.Stderr
	}
	rep.WriteSummary(summaryOut)
	for _, e := range allow.Unused() {
		fmt.Fprintf(summaryOut, "note: allow-list line %d (%s) matched nothing in this run\n", e.Line, e.Rule)
	}
	if *jsonPath != "" {
		if err := writeReport(rep, *jsonPath); err != nil {
			return fail(err)
		}
	}
	if rep.Failed() {
		return 1
	}
	return 0
}

// flagsSet names the flags given on the command line.
func flagsSet(fs *flag.FlagSet) map[string]bool {
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	return set
}

// layoutSettings adds the two settings the reflow and resize-text criteria name, a 320 px window (WCAG 1.4.10) and
// text at 200 percent (WCAG 1.4.4), when the layout checks are on and you chose neither the windows nor the sizes.
func layoutSettings(layout bool, set map[string]bool, viewports []a11y.Viewport, scales []int) ([]a11y.Viewport, []int) {
	if !layout {
		return viewports, scales
	}
	if !set["viewport"] && !slices.Contains(viewports, a11y.Small) {
		viewports = append(slices.Clone(viewports), a11y.Small)
	}
	if !set["text-scale"] && len(scales) == 0 {
		scales = []int{100, 200}
	}
	return viewports, scales
}

// checkScreenshotFlags refuses --screenshots when no layout checks will run, since nothing would be written.
func checkScreenshotFlags(dir string, layout bool, scales []int) error {
	if dir != "" && !layout && !slices.ContainsFunc(scales, func(n int) bool { return n > 100 }) {
		return errors.New("--screenshots needs --layout, or a --text-scale above 100, which run the checks that take them")
	}
	return nil
}

// parseTextScales reads "200" or "100,200" into percentages. Empty gives none, which means 100.
func parseTextScales(v string) ([]int, error) {
	if strings.TrimSpace(v) == "" {
		return nil, nil
	}
	var out []int
	for _, part := range strings.Split(v, ",") {
		n, err := cast.ParseTextScale(part)
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, fmt.Errorf("%q is empty; want a percentage such as 200", v)
		}
		if slices.Contains(out, n) {
			return nil, fmt.Errorf("%d is given twice", n)
		}
		out = append(out, n)
	}
	return out, nil
}

func pick[T any](name string, options map[string][]T) ([]T, bool) {
	v, ok := options[strings.ToLower(name)]
	return v, ok
}

func writeReport(rep *a11y.Report, path string) error {
	if path == "-" {
		return rep.WriteJSON(os.Stdout)
	}
	f, err := os.Create(path) //nolint:gosec // the path comes from the command line on purpose
	if err != nil {
		return err
	}
	if err := rep.WriteJSON(f); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
