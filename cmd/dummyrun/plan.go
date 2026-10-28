package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	"github.com/dummy-run/dummy-run/casts"
	"github.com/dummy-run/dummy-run/internal/access"
	"github.com/dummy-run/dummy-run/internal/plan"
	"github.com/dummy-run/dummy-run/internal/policy"
	"github.com/dummy-run/dummy-run/internal/provider"
)

const planHelp = `Usage: dummyrun plan URL --out DIR [--personas N | --persona ID...] [--steps N] [--min-steps N] [--pages URL]... [--pages-file FILE]
    [--library DIR] [--json FILE] [--no-model] [--signin] [--identify] [--respect-robots] [--provider PROVIDER --model MODEL ...]
    [--basic-auth-env USER_ENV:PASS_ENV] [--header-env 'Name:ENV']...

Works out what your site is for and writes a cast that fits it, ready for "dummyrun run".

First it reads up to 10 pages over plain HTTP, with no browser: titles, headings, menus, forms, and signs such
as a basket, prices, a sign-in form or a contact link. It only sends GET requests, and only to the site's own
hosts. Then one short model call names the kind of site, who it is for and its two to four key journeys, picks
personas from the library that fit them and gives each its journeys, starting pages and what not to do. How
each persona goes about a journey comes from the persona itself: the keyboard user counts Tab presses, the
screen reader user moves by headings and landmarks, and each has its own minimum number of steps. It never
asks about prices unless the site sells something.

DIR gets one folder per persona (its profile and prompt, copied from the library, and scenarios/01-<id>.md)
and plan.json. Give an empty folder or one an earlier plan wrote: its personas are replaced. Play a persona with:
  dummyrun run --cast DIR --persona ID --scenario ID --target CONFIG --out RUNS ...
and add --plan DIR/plan.json to "dummyrun report" to show what was tested and why.

If the model call fails, or its answer does not hold up, the plan comes from the signals alone and plan.json says
why. --no-model skips the call. Exit status: 0 when the cast is written, 2 for a usage error (such as a --persona
that is not in the library), 3 when robots.txt disallows the start page (with --respect-robots), 130 when
interrupted; nothing is written then.

Flags:
  --out DIR          where the cast goes (required)
  --personas N       how many personas to pick, 1 to 4 (default 1)
  --persona ID       use this library persona instead of letting the plan pick; repeat it for several, in order
  --steps N          each persona's step budget (default 12)
  --min-steps N      each persona's minimum steps (default the persona's own, always at least 3 below --steps)
  --pages URL        a page to read instead of the sitemap; repeat it, up to 10
  --pages-file FILE  a file of pages to read, one address per line
  --library DIR      the persona library to choose from (default the one built into the program)
  --json FILE        also write plan.json here
  --no-model         plan from the signals alone, with no model call
  --signin           the target has test users, so a persona that signs in may be picked
  --identify         name the scanner page in the user agent
  --respect-robots   read robots.txt and skip what it disallows for DummyRun or *
  --basic-auth-env USER_ENV:PASS_ENV
                     answer HTTP Basic auth on a protected preview, with the names of two environment variables
  --header-env 'Name:ENV'
                     send a request header whose value is in an environment variable, such as
                     x-vercel-protection-bypass. Repeat for more. Both go only to the site's own hosts
  The model flags are the same as for "dummyrun run".`

func planCmd(args []string) int {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprintln(os.Stderr, planHelp) }
	out := fs.String("out", "", "where the cast goes")
	personas := fs.Int("personas", 1, "how many personas to pick, 1 to 4")
	var use listFlag
	fs.Var(&use, "persona", "use this library persona instead of letting the plan pick; repeatable")
	minSteps := fs.Int("min-steps", 0, "each persona's minimum steps (default the persona's own)")
	steps := fs.Int("steps", 12, "each persona's step budget")
	var pages listFlag
	fs.Var(&pages, "pages", "a page to read; repeatable")
	pagesFile := fs.String("pages-file", "", "a file of pages, one per line")
	libDir := fs.String("library", "", "persona library folder")
	jsonOut := fs.String("json", "", "also write plan.json here")
	noModel := fs.Bool("no-model", false, "plan from the signals alone")
	signin := fs.Bool("signin", false, "the target has test users")
	identify := fs.Bool("identify", false, "name the scanner page in the user agent")
	respect := fs.Bool("respect-robots", false, "honour robots.txt")
	var preview authFlags
	preview.register(fs)
	pf := addProviderFlags(fs)
	var target string
	rest := args
	if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		target, rest = rest[0], rest[1:]
	}
	if err := fs.Parse(rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if target == "" && fs.NArg() == 1 {
		target = fs.Arg(0)
	}
	if target == "" || *out == "" || *personas < 1 || *personas > 4 || *steps < 4 {
		fmt.Fprintln(os.Stderr, "dummyrun plan: give one URL and --out DIR, with --personas from 1 to 4 and --steps of 4 or more")
		return 2
	}
	if *pagesFile != "" {
		more, err := readLines(*pagesFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "dummyrun plan:", err)
			return 2
		}
		pages = append(pages, more...)
	}
	tgt, err := tryTarget(target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dummyrun plan:", err)
		return 2
	}
	if tgt.Auth, err = preview.auth(nil); err != nil {
		fmt.Fprintln(os.Stderr, "dummyrun plan:", err)
		return 2
	}
	lib, err := libraryFS(*libDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dummyrun plan:", err)
		return 2
	}
	if people, err := plan.LoadLibrary(lib); err != nil {
		fmt.Fprintln(os.Stderr, "dummyrun plan:", err)
		return 2
	} else if err := knownPersonas(use, people); err != nil {
		fmt.Fprintln(os.Stderr, "dummyrun plan:", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var p provider.Provider
	if !*noModel {
		if p, err = pf.build(ctx, os.Stderr); err != nil {
			return fail(err)
		}
	}
	pl, err := makePlan(ctx, planInput{Target: tgt, Pages: pages, Library: lib, Provider: p,
		Personas: *personas, Use: use, Steps: *steps, MinSteps: *minSteps, SignIn: *signin, Identify: *identify, Respect: *respect, Out: *out})
	if errors.Is(err, plan.ErrRobots) {
		fmt.Fprintln(os.Stderr, "dummyrun plan: robots.txt disallows DummyRun from the start page, so nothing was read")
		return exitRobots
	}
	if err != nil {
		return fail(err)
	}
	if *jsonOut != "" {
		if err := writePlanJSON(*jsonOut, pl); err != nil {
			return fail(err)
		}
	}
	printPlan(os.Stdout, pl, *out)
	return 0
}

// planInput is what makePlan needs. try and plan share it.
type planInput struct {
	// Target gives the start page, the hosts recon may read and the preview access it needs to read them.
	Target   policy.Target
	Pages    []string
	Library  fs.FS
	Provider provider.Provider
	Personas int
	Use      []string
	Steps    int
	MinSteps int
	SignIn   bool
	Identify bool
	Respect  bool
	Out      string
}

// makePlan reads the site, plans the scan and writes the cast into in.Out. When the caller stops it, it
// returns the context's error and writes nothing, so no cast comes from a scan that was cut short.
func makePlan(ctx context.Context, in planInput) (*plan.Plan, error) {
	people, err := plan.LoadLibrary(in.Library)
	if err != nil {
		return nil, err
	}
	// The preview secrets go only to the target's own hosts, as they do for the browser.
	pa, err := access.Resolve(in.Target.Auth, os.Getenv, access.TargetScope(in.Target))
	if err != nil {
		return nil, err
	}
	rec, err := plan.Run(ctx, plan.ReconOptions{Client: access.WrapClient(requestClient(in.Identify), pa), StartURL: in.Target.BaseURL,
		Pages: in.Pages, AllowedHosts: hostsOnly(in.Target.AllowedHosts), Identify: in.Identify, RespectRobots: in.Respect})
	if err != nil {
		return nil, err
	}
	pl := plan.Make(ctx, rec, plan.Options{Provider: in.Provider, Library: people, MaxPersonas: in.Personas, Personas: in.Use, Steps: in.Steps,
		MinSteps: in.MinSteps, SignIn: in.SignIn})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := plan.WriteCast(in.Out, in.Library, pl, people); err != nil {
		return nil, err
	}
	return pl, nil
}

// knownPersonas refuses an id that is not in the library, naming the ones that are, so a typo never turns into
// a persona the plan chose on its own.
func knownPersonas(ids []string, lib []plan.LibraryPersona) error {
	var have []string
	for _, l := range lib {
		have = append(have, l.ID)
	}
	for _, id := range ids {
		if !slices.Contains(have, id) {
			return fmt.Errorf("no persona %q in the library; it has %s", id, strings.Join(have, ", "))
		}
	}
	return nil
}

// hostsOnly drops ports, since recon compares host names.
func hostsOnly(hosts []string) []string {
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		if i := strings.LastIndex(h, ":"); i > 0 && !strings.Contains(h[i:], "]") {
			out = append(out, h[:i], h)
			continue
		}
		out = append(out, h)
	}
	return out
}

func libraryFS(dir string) (fs.FS, error) {
	if dir != "" {
		return os.DirFS(dir), nil
	}
	return fs.Sub(casts.Library, "library")
}

func writePlanJSON(path string, pl *plan.Plan) error {
	data, err := json.MarshalIndent(pl, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644) //nolint:gosec // the plan is meant to be read
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path) //nolint:gosec // the path comes from the command line on purpose
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if l := strings.TrimSpace(sc.Text()); l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	return out, sc.Err()
}

// printPlan says what the plan is, in a few lines.
func printPlan(w io.Writer, pl *plan.Plan, dir string) {
	fmt.Fprintf(w, "Your site looks like a %s", plan.SiteWords(pl.SiteType))
	if pl.Audience != "" {
		fmt.Fprintf(w, " for %s", plainText(pl.Audience))
	}
	fmt.Fprintln(w, ".")
	fmt.Fprintln(w, "Journeys to try:")
	for _, j := range pl.Journeys {
		fmt.Fprintf(w, "  - %s\n", plainText(j.Name))
	}
	for _, p := range pl.Personas {
		fmt.Fprintf(w, "Persona %s (%s), scenario %s: %s\n", p.ID, plainText(p.Name), p.Scenario.ID, plainText(p.Why))
	}
	if pl.Source == "fallback" {
		fmt.Fprintf(w, "This plan comes from what the pages show, without a model: %s.\n", plainText(pl.FallbackReason))
	}
	fmt.Fprintf(w, "Cast written to %s\n", dir)
}
