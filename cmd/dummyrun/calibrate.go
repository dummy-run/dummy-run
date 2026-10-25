package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/dummy-run/dummy-run/internal/confidence"
)

func calibrateCmd(args []string) int { return calibrateTo(args, os.Stdout) }

// calibrateTo runs dummyrun calibrate and writes the report to stdout when no --report file is given.
func calibrateTo(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("calibrate", flag.ContinueOnError)
	var labels, eval listFlag
	fs.Var(&labels, "labels", "a JSON file of labelled findings, each with its inputs and whether it was true; repeat the flag to read several")
	fs.Var(&eval, "eval", "labelled findings to check the table against instead of the ones it was built from; repeatable")
	table := fs.String("table", "", "rate the --eval findings with this table instead of building one from --labels")
	outTable := fs.String("out-table", "", "write the rebuilt confidence table here (JSON)")
	report := fs.String("report", "", "write the calibration report here (Markdown) instead of standard output")
	minSamples := fs.Int("min-samples", confidence.DefaultMinSamples, "the fewest labelled findings a bucket needs before it is used")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, `Usage: dummyrun calibrate --labels FILE... [--eval FILE...] [--table FILE] [--out-table FILE] [--report FILE] [--min-samples N]

Rebuild the confidence table from labelled findings and report how well its bands match what the labels show.

A labelled finding is {"inputs": {...}, "true": true|false, "id": "...", "source": "..."}, where inputs is the
finding's inputs block from a run record: evidence_kind, engines, check, on_journey and, with repeated runs,
reproduced. The table groups findings by those inputs and records how often each group was right, with a 95
percent interval. The report shows each band's observed accuracy, each bucket's stated accuracy against the
labels it rated, and whether the bands discriminate.

The engine ships with internal/confidence/table.json, built from internal/confidence/labels/*.json by this
command. To check a table against findings it was not built from, pass them with --eval.`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if len(labels) == 0 && *table == "" {
		fmt.Fprintln(os.Stderr, "dummyrun calibrate: give --labels to build a table, or --table to check one")
		return exitRefused
	}
	var t *confidence.Table
	var built []confidence.Labelled
	if len(labels) > 0 {
		for _, p := range labels {
			l, err := confidence.ReadLabels(p)
			if err != nil {
				fmt.Fprintln(os.Stderr, "dummyrun calibrate:", err)
				return exitRefused
			}
			built = append(built, l...)
		}
		t = confidence.Calibrate(built, *minSamples)
	}
	if *table != "" {
		var err error
		if t, err = confidence.ReadTable(*table); err != nil {
			fmt.Fprintln(os.Stderr, "dummyrun calibrate:", err)
			return exitRefused
		}
	}
	if *outTable != "" {
		data, err := t.Marshal()
		if err != nil {
			fmt.Fprintln(os.Stderr, "dummyrun calibrate:", err)
			return 1
		}
		if err := os.WriteFile(*outTable, data, 0o600); err != nil {
			fmt.Fprintln(os.Stderr, "dummyrun calibrate:", err)
			return 1
		}
	}
	check := built
	if len(eval) > 0 {
		check = nil
		for _, p := range eval {
			l, err := confidence.ReadLabels(p)
			if err != nil {
				fmt.Fprintln(os.Stderr, "dummyrun calibrate:", err)
				return exitRefused
			}
			check = append(check, l...)
		}
	}
	if len(check) == 0 {
		fmt.Fprintln(os.Stderr, "dummyrun calibrate: nothing to check the table against; give --labels or --eval")
		return exitRefused
	}
	md := confidence.Evaluate(t, check).Markdown()
	if *report == "" {
		if _, err := io.WriteString(stdout, md); err != nil {
			fmt.Fprintln(os.Stderr, "dummyrun calibrate: write the report:", err)
			return 1
		}
		return 0
	}
	if err := os.WriteFile(*report, []byte(md), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "dummyrun calibrate:", err)
		return 1
	}
	return 0
}
