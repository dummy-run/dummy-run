package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dummy-run/dummy-run/internal/report"
	"github.com/dummy-run/dummy-run/internal/telemetry"
)

const reportHelp = `Usage: dummyrun report --runs DIR|FILE [--runs ...] [--a11y FILE]... [--title TEXT] [--client TEXT]
    [--format html|md|json|sarif|junit|badge|shields] [--include pending] [--baseline FILE] [--build SHA] [--date YYYY-MM-DD] [--out FILE]

Turns run records and accessibility sweep results into an audit report. The default is one self-contained
HTML file. Only reviewer-confirmed findings appear. --include pending adds the others, marked not reviewed.
Rejected findings never appear. The report is a sample. It is not a WCAG conformance claim.
--format badge writes a small SVG badge and --format shields writes the shields.io endpoint JSON for it.
Both say how many findings are open and the date of the newest run.`

func reportCmd(args []string) int {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var runs, sweeps listFlag
	fs.Var(&runs, "runs", "directory or file of run records; repeat the flag to read several (record paths may also follow the flags)")
	fs.Var(&sweeps, "a11y", "JSON file from dummyrun a11y --json; repeatable")
	title := fs.String("title", "", "report title (default \"Accessibility and usability audit\")")
	client := fs.String("client", "", "free text naming who the report is for")
	format := fs.String("format", "html", "html, md, json, sarif, junit, badge (SVG) or shields (shields.io endpoint JSON)")
	include := fs.String("include", "", "\"pending\" also includes findings no reviewer has confirmed, marked not reviewed (default: confirmed only)")
	baseline := fs.String("baseline", "", "an earlier report in JSON form; each finding is marked new or still present, and fixed ones are listed")
	build := fs.String("build", "", "commit or build hash to report on (at least 7 characters); default is every record given")
	date := fs.String("date", "", "report date as YYYY-MM-DD (default today)")
	out := fs.String("out", "", "write the report to this file instead of standard output")
	fs.Usage = func() { fmt.Fprintln(os.Stderr, reportHelp); fs.PrintDefaults() }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	runs = append(runs, fs.Args()...)
	if len(runs) == 0 && len(sweeps) == 0 {
		fmt.Fprintln(os.Stderr, "dummyrun report: give --runs, --a11y or both")
		return exitRefused
	}
	if !contains(report.Formats, *format) {
		fmt.Fprintf(os.Stderr, "dummyrun report: --format must be one of %s, not %q\n", strings.Join(report.Formats, ", "), *format)
		return exitRefused
	}
	if *include != "" && *include != "pending" {
		fmt.Fprintf(os.Stderr, "dummyrun report: --include takes \"pending\", not %q\n", *include)
		return exitRefused
	}
	opt := report.Options{Title: *title, Client: *client, IncludePending: *include == "pending", Build: *build, ToolVersion: telemetry.Version}
	if *date != "" {
		d, err := time.Parse("2006-01-02", *date)
		if err != nil {
			fmt.Fprintf(os.Stderr, "dummyrun report: --date wants YYYY-MM-DD, not %q\n", *date)
			return exitRefused
		}
		opt.Date = d
	}
	var loaded []*report.Run
	if len(runs) > 0 {
		var err error
		if loaded, err = report.LoadRuns(runs...); err != nil {
			fmt.Fprintln(os.Stderr, "dummyrun report:", err)
			return exitRefused
		}
	}
	for _, p := range sweeps {
		rep, err := report.LoadA11y(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, "dummyrun report:", err)
			return exitRefused
		}
		opt.A11y = append(opt.A11y, rep)
	}
	if *baseline != "" {
		b, err := report.LoadBaseline(*baseline)
		if err != nil {
			fmt.Fprintln(os.Stderr, "dummyrun report:", err)
			return exitRefused
		}
		opt.Baseline = b
	}
	rep, err := report.Build(loaded, opt)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dummyrun report:", err)
		return exitRefused
	}
	var buf bytes.Buffer
	if err := report.Render(&buf, rep, *format); err != nil {
		return fail(err)
	}
	if *out == "" {
		_, _ = os.Stdout.Write(buf.Bytes())
		return 0
	}
	if err := os.WriteFile(*out, buf.Bytes(), 0o644); err != nil { //nolint:gosec // a report is meant to be read
		return fail(err)
	}
	return 0
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
