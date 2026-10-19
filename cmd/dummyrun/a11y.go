package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/dummy-run/dummy-run/internal/a11y"
)

const a11yUsage = `Usage:
  dummyrun a11y [--sitemap URL] [--rebase URL] [URL...] [flags]

Runs the axe-core WCAG 2.2 A and AA rules on each page in a real browser. No model is called.

Flags:
  --sitemap URL     a sitemap (or sitemap index) listing the pages to test
  --rebase URL      move every sitemap URL onto this origin, to test a local copy of a site
  --browser NAME    chrome or firefox (default chrome)
  --viewport NAME   desktop (1280 wide), phone (375 wide) or both (default both)
  --scheme NAME     light, dark or both (default both)
  --allow FILE      accepted violations, one per line: rule | page | selector | reason
  --json FILE       write the full report as JSON; "-" writes it to standard output
  --headful         show the browser window

Exit status: 0 when nothing fails, 1 when a violation is not in the allow-list or a page cannot be
tested, 2 for a usage error.`

func a11yCmd(args []string) int {
	fs := flag.NewFlagSet("a11y", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprintln(os.Stderr, a11yUsage) }
	sitemap := fs.String("sitemap", "", "sitemap URL")
	rebase := fs.String("rebase", "", "origin to move sitemap URLs onto")
	browser := fs.String("browser", "chrome", "chrome or firefox")
	viewport := fs.String("viewport", "both", "desktop, phone or both")
	scheme := fs.String("scheme", "both", "light, dark or both")
	allowPath := fs.String("allow", "", "allow-list file")
	jsonPath := fs.String("json", "", "JSON report file, or - for standard output")
	headful := fs.Bool("headful", false, "show the browser window")
	// Flags may come after the URLs, so parse in a loop that peels off positional arguments.
	var urls []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
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
		"desktop": {a11y.Desktop}, "phone": {a11y.Phone}, "both": {a11y.Desktop, a11y.Phone}})
	if !ok {
		fmt.Fprintf(os.Stderr, "dummyrun a11y: --viewport %q: want desktop, phone or both\n", *viewport)
		return 2
	}
	schemes, ok := pick(*scheme, map[string][]string{
		a11y.Light: {a11y.Light}, a11y.Dark: {a11y.Dark}, "both": {a11y.Light, a11y.Dark}})
	if !ok {
		fmt.Fprintf(os.Stderr, "dummyrun a11y: --scheme %q: want light, dark or both\n", *scheme)
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *sitemap != "" {
		pages, err := a11y.FetchSitemap(ctx, nil, *sitemap)
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
	rep, err := a11y.Run(ctx, a11y.Config{
		URLs: urls, Browser: *browser, Viewports: viewports, Schemes: schemes, Allow: allow,
		Headless: !*headful, Log: os.Stderr,
	})
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
