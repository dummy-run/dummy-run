package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/dummy-run/dummy-run/internal/coverage"
)

// listFlag is a flag that can be given more than once.
type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error { *l = append(*l, v); return nil }

// Exit codes of dummyrun coverage.
const (
	exitRefused    = 2
	exitNotStopped = 3
)

func coverageCmd(args []string) int {
	fs := flag.NewFlagSet("coverage", flag.ContinueOnError)
	var runs, clusters, thresholds listFlag
	fs.Var(&runs, "runs", "directory or file of run records; repeat the flag to read several")
	build := fs.String("build", "", "commit or build hash to count (at least 7 characters); needed when the records come from more than one build")
	strata := fs.String("strata", "device", "what the stopping rule treats as separate groups: device or persona")
	confirmed := fs.Bool("confirmed-only", true, "count only reviewer-confirmed findings; --confirmed-only=false also counts pending ones and labels the report a demonstration")
	fs.Var(&thresholds, "threshold", "policy number as name=value, repeatable: coverage (default 0.90), min-runs (4), no-new (3), kappa (0.6)")
	fs.Var(&clusters, "clusters", "a reviewer's grouping of findings into problems, JSON {\"clusters\": [[\"run/finding\", ...]]}; give two files for kappa")
	format := fs.String("format", "markdown", "json or markdown")
	out := fs.String("out", "", "write the report to this file instead of standard output")
	failUnless := fs.Bool("fail-unless-stopped", false, "exit 3 when the stopping rule is not satisfied, so a CI job can use it")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if len(runs) == 0 {
		fmt.Fprintln(os.Stderr, "dummyrun coverage: missing --runs")
		return exitRefused
	}
	if *format != "json" && *format != "markdown" {
		fmt.Fprintf(os.Stderr, "dummyrun coverage: --format must be json or markdown, not %q\n", *format)
		return exitRefused
	}
	th, err := parseThresholds(thresholds)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dummyrun coverage:", err)
		return exitRefused
	}
	opt := coverage.Options{Build: *build, Strata: *strata, ConfirmedOnly: *confirmed, Thresholds: th}
	for _, c := range clusters {
		cf, err := coverage.ReadClusters(c)
		if err != nil {
			fmt.Fprintln(os.Stderr, "dummyrun coverage:", err)
			return exitRefused
		}
		opt.Clusters = append(opt.Clusters, cf)
	}
	if len(opt.Clusters) > 2 {
		fmt.Fprintln(os.Stderr, "dummyrun coverage: give at most two --clusters files")
		return exitRefused
	}
	recs, err := coverage.Load(runs...)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dummyrun coverage:", err)
		return exitRefused
	}
	rep, err := coverage.Build(recs, opt)
	if err != nil {
		var ref *coverage.Refusal
		if errors.As(err, &ref) {
			fmt.Fprintln(os.Stderr, "dummyrun coverage: refused:", ref.Msg)
			return exitRefused
		}
		return fail(err)
	}
	var text string
	if *format == "json" {
		data, err := rep.JSON()
		if err != nil {
			return fail(err)
		}
		text = string(data)
	} else {
		text = rep.Markdown()
	}
	if *out != "" {
		if err := os.WriteFile(*out, []byte(text), 0o644); err != nil { //nolint:gosec // a report is meant to be read
			return fail(err)
		}
	} else {
		fmt.Print(text)
	}
	if *failUnless && !rep.Stopping.Stop {
		fmt.Fprintln(os.Stderr, "dummyrun coverage: the stopping rule is not satisfied:", rep.Stopping.Summary)
		return exitNotStopped
	}
	return 0
}

func parseThresholds(list []string) (coverage.Thresholds, error) {
	th := coverage.DefaultThresholds()
	for _, kv := range list {
		name, val, ok := strings.Cut(kv, "=")
		if !ok {
			return th, fmt.Errorf("--threshold %q: want name=value", kv)
		}
		switch name {
		case "coverage", "kappa":
			f, err := strconv.ParseFloat(val, 64)
			if err != nil || f <= 0 || f > 1 {
				return th, fmt.Errorf("--threshold %s needs a number above 0 and at most 1, not %q", name, val)
			}
			if name == "coverage" {
				th.Coverage = f
			} else {
				th.Kappa = f
			}
		case "min-runs", "no-new":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 {
				return th, fmt.Errorf("--threshold %s needs a whole number of at least 1, not %q", name, val)
			}
			if name == "min-runs" {
				th.MinRuns = n
			} else {
				th.NoNew = n
			}
		default:
			return th, fmt.Errorf("--threshold %q: the names are coverage, min-runs, no-new and kappa", name)
		}
	}
	return th, nil
}
