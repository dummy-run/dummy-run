package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/dummy-run/dummy-run/internal/review"
)

func reviewCmd(args []string) int {
	return reviewRun(args, os.Stdin, os.Stdout, os.Stderr, isTerminal(os.Stdin))
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// gitUserName returns git config user.name, or "" when git or the setting is missing.
func gitUserName() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "config", "user.name").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func reviewRun(args []string, stdin io.Reader, stdout, stderr io.Writer, tty bool) int {
	fs := flag.NewFlagSet("review", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var confirm, reject listFlag
	fs.Var(&confirm, "confirm", "confirm the pending findings with this fingerprint, a fingerprint prefix (6 or more characters), run/finding or a finding ID; repeatable")
	fs.Var(&reject, "reject", "reject the pending findings with this fingerprint, prefix, run/finding or finding ID; needs --reason; repeatable")
	reason := fs.String("reason", "", "why: required with --reject, optional with --confirm")
	reviewer := fs.String("reviewer", "", "who is reviewing (default: git config user.name)")
	list := fs.Bool("list", false, "list the problems and change nothing")
	verdict := fs.String("verdict", "pending", "with --list, which findings to show: pending, confirmed, rejected or all")
	asJSON := fs.Bool("json", false, "with --list, print JSON instead of a table")

	// Flags and paths may come in any order.
	var paths []string
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
		paths = append(paths, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	refuse := func(format string, a ...any) int {
		fmt.Fprintf(stderr, "dummyrun review: "+format+"\n", a...)
		return 2
	}
	if len(paths) == 0 {
		return refuse("give a directory of run records or one or more record files")
	}
	decisions := len(confirm) + len(reject)
	if *list && decisions > 0 {
		return refuse("--list changes nothing, so it cannot go with --confirm or --reject")
	}
	if *reason != "" && decisions == 0 {
		return refuse("--reason goes with --confirm or --reject")
	}
	if len(reject) > 0 && strings.TrimSpace(*reason) == "" {
		return refuse("--reject needs --reason")
	}
	switch *verdict {
	case "pending", "confirmed", "rejected", "all":
	default:
		return refuse("--verdict must be pending, confirmed, rejected or all, not %q", *verdict)
	}
	set, err := review.Load(paths...)
	if err != nil {
		fmt.Fprintln(stderr, "dummyrun review:", err)
		return 2
	}
	for _, s := range set.Skipped {
		fmt.Fprintln(stderr, "dummyrun review: skipped", s)
	}
	if *list {
		return reviewList(set, *verdict, *asJSON, stdout)
	}
	name := strings.TrimSpace(*reviewer)
	if name == "" {
		name = gitUserName()
	}
	if name == "" {
		return refuse("no reviewer: pass --reviewer or set git config user.name")
	}
	if decisions > 0 {
		return reviewDecide(set, name, confirm, reject, *reason, stdout, stderr)
	}
	if !tty {
		return refuse("standard input is not a terminal. Use --list to see the findings, then --confirm or --reject with --reason")
	}
	sess := &review.Session{In: bufio.NewReader(stdin), Out: stdout, Reviewer: name}
	if _, err := sess.Run(set); err != nil {
		fmt.Fprintln(stderr, "dummyrun review:", err)
		return 1
	}
	return 0
}

// reviewDecide resolves every selector first and writes only when all of them match, so a typo in
// the fourth selector does not leave three verdicts behind.
func reviewDecide(set *review.Set, reviewer string, confirm, reject []string, reason string, stdout, stderr io.Writer) int {
	type job struct {
		g       *review.Group
		verdict string
	}
	var jobs []job
	seen := map[string]string{}
	for _, c := range []struct {
		verdict string
		sels    []string
	}{{review.Confirmed, confirm}, {review.Rejected, reject}} {
		for _, sel := range c.sels {
			g, err := set.Find(sel)
			if err != nil {
				fmt.Fprintln(stderr, "dummyrun review:", err)
				return 2
			}
			if prev, dup := seen[g.Fingerprint]; dup {
				if prev != c.verdict {
					fmt.Fprintf(stderr, "dummyrun review: %s is both confirmed and rejected\n", g.Fingerprint)
					return 2
				}
				continue
			}
			seen[g.Fingerprint] = c.verdict
			jobs = append(jobs, job{g, c.verdict})
		}
	}
	now := time.Now()
	status := 0
	for _, j := range jobs {
		applied, err := review.Apply(j.g, review.Decision{Verdict: j.verdict, Reason: reason, Reviewer: reviewer, At: now})
		if len(applied) > 0 {
			fmt.Fprintf(stdout, "%s %s: %s\n", j.verdict, j.g.Fingerprint, review.Describe(applied))
		}
		if err != nil {
			fmt.Fprintln(stderr, "dummyrun review:", err)
			status = 1
		}
	}
	return status
}

type listOccurrence struct {
	Run      string `json:"run"`
	Finding  string `json:"finding"`
	Persona  string `json:"persona"`
	Scenario string `json:"scenario"`
	Verdict  string `json:"verdict"`
	File     string `json:"file"`
}

type listItem struct {
	Fingerprint string           `json:"fingerprint"`
	Severity    string           `json:"severity"`
	Kind        string           `json:"kind"`
	Class       string           `json:"class"`
	Title       string           `json:"title"`
	Route       string           `json:"route"`
	Locus       string           `json:"locus"`
	Symptom     string           `json:"symptom"`
	Pending     int              `json:"pending"`
	Confirmed   int              `json:"confirmed"`
	Rejected    int              `json:"rejected"`
	Occurrences []listOccurrence `json:"occurrences"`
}

func reviewList(set *review.Set, verdict string, asJSON bool, stdout io.Writer) int {
	filter := verdict
	if verdict == "all" {
		filter = ""
	}
	items := []listItem{}
	for _, g := range set.Groups(filter) {
		lead := g.Lead().Finding
		c := g.Counts()
		it := listItem{Fingerprint: g.Fingerprint, Severity: lead.Severity, Kind: lead.Kind, Class: lead.Class, Title: lead.Title,
			Route: lead.Route, Locus: lead.Locus, Symptom: lead.Symptom, Pending: c[review.Pending],
			Confirmed: c[review.Confirmed], Rejected: c[review.Rejected], Occurrences: []listOccurrence{}}
		for _, o := range g.Occurrences {
			it.Occurrences = append(it.Occurrences, listOccurrence{Run: o.Record.ID, Finding: o.Finding.ID, Persona: o.Record.Persona.ID,
				Scenario: o.Record.Scenario.ID, Verdict: o.Finding.Verdict, File: o.Path})
		}
		items = append(items, it)
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(items); err != nil {
			return 1
		}
		return 0
	}
	if len(items) == 0 {
		fmt.Fprintf(stdout, "No %s findings.\n", verdict)
		return 0
	}
	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "FINGERPRINT\tSEVERITY\tKIND\tPENDING\tCONFIRMED\tREJECTED\tROUTE\tTITLE")
	for _, it := range items {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%d\t%s\t%s\n", it.Fingerprint, it.Severity, it.Kind, it.Pending, it.Confirmed, it.Rejected,
			oneLine(it.Route), oneLine(it.Title))
	}
	_ = tw.Flush()
	return 0
}

// oneLine keeps a table row on one line and free of terminal escape sequences.
func oneLine(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
}
