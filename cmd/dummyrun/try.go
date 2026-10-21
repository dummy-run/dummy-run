package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"

	"github.com/dummy-run/dummy-run/casts"
	"github.com/dummy-run/dummy-run/internal/cast"
	"github.com/dummy-run/dummy-run/internal/policy"
	"github.com/dummy-run/dummy-run/internal/record"
	"github.com/dummy-run/dummy-run/internal/runner"
)

const tryUsage = `Usage:
  dummyrun try URL [--browser chrome|firefox] [--out DIR] [--steps N] [--provider PROVIDER --model MODEL ...]

Plays the first-time visitor from the persona library against one page, for a few minutes, in read-only mode.
The browser blocks every request that could change data and every request to another host. Then it
prints what the visitor found and where the run is saved.

You need a browser (run "dummyrun browsers install" once) and a model. With no --provider, the model comes from
your environment, in this order: DUMMYRUN_AZURE_ENDPOINT (then --model and --region are needed), OPENAI_API_KEY,
ANTHROPIC_API_KEY, then a running Ollama. The first line of the output says which one it chose.
docs/PROVIDERS.md lists the defaults.

Flags:
  --browser NAME   chrome or firefox (default chrome)
  --out DIR        where run records go (default ~/.local/share/dummyrun/runs/try)
  --steps N        step budget (default 15, up to 40)
  --basic-auth-env USER_ENV:PASS_ENV
                   answer HTTP Basic auth on a protected preview, with the names of two environment variables
  --header-env 'Name:ENV'
                   send a request header whose value is in an environment variable, such as
                   x-vercel-protection-bypass. Repeat for more. Both go only to the site's own hosts
  --headful        show the browser window
  The model flags are the same as for "dummyrun run".`

// tryDefaultSteps keeps a first run short. A scenario cannot ask for fewer than the engine's default of 25, so the
// command sets the budget itself.
const tryDefaultSteps = 15

// tryScenario is the built-in scenario: short, and about the one thing every site owes a new visitor.
const tryScenario = `# Look around for five minutes

Device: desktop. Start at the page you were given.

## Given

Alex has just landed here from a link with no explanation. The browser blocks anything that could change data, so Alex stops at any form.

## Goal

"Work out what this is, who it is for, what it costs and how to start, using only what the site shows."

## Steps Alex would take

1. Read the first screen without scrolling and write down what Alex thinks this is.
2. Look for the price, or a plain statement that there is none.
3. Look for the clearest way to start. Follow it, read what it asks for and do not submit.
4. Finish with what Alex could and could not work out.

## Success looks like

- Alex could say what it is and who it is for from the first screen.
- Alex found the price or knew there was none.
- There was one clear way to start.

## What to report

Alex's one-sentence guess, then every place where Alex got stuck, with step numbers. Report only problems Alex hit, not ones Alex can imagine.
`

func tryCmd(args []string) int {
	fs := flag.NewFlagSet("try", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprintln(os.Stderr, tryUsage) }
	browser := fs.String("browser", "chrome", "chrome or firefox")
	out := fs.String("out", "", "directory for run records (default ~/.local/share/dummyrun/runs/try)")
	steps := fs.Int("steps", tryDefaultSteps, "step budget (up to 40)")
	headful := fs.Bool("headful", false, "show the browser window")
	pf := addProviderFlags(fs)
	var preview authFlags
	preview.register(fs)
	// Allow the URL before or after the flags.
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
	} else if fs.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "dummyrun try: want one URL")
		fmt.Fprintln(os.Stderr, tryUsage)
		return 2
	}
	if target == "" {
		fmt.Fprintln(os.Stderr, "dummyrun try: give the address of your site, such as: dummyrun try https://your-site")
		return 2
	}
	tgt, err := tryTarget(target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dummyrun try:", err)
		return 2
	}
	if tgt.Auth, err = preview.auth(nil); err != nil {
		fmt.Fprintln(os.Stderr, "dummyrun try:", err)
		return 2
	}
	if *out == "" {
		*out = defaultTryDir()
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// The model first, so its choice is the first line printed and a missing key fails before a browser starts.
	p, err := pf.build(ctx, os.Stderr)
	if err != nil {
		return fail(err)
	}
	persona, scenario, cleanup, err := tryPersona()
	if err != nil {
		return fail(err)
	}
	defer cleanup()
	fmt.Fprintf(os.Stderr, "Reading %s as a first-time visitor, read-only: requests that could change data are blocked.\n", tgt.BaseURL)
	res, err := runner.Run(ctx, runner.Config{
		Persona: persona, Scenario: scenario, Target: tgt, Provider: p, Browser: *browser, Headless: !*headful,
		OutDir: *out, StepBudget: *steps, Log: os.Stderr, HTTPClient: requestClient(false), SelfCheck: true,
	})
	if err != nil {
		if res != nil {
			fmt.Fprintf(os.Stderr, "dummyrun: the run stopped early and was recorded at %s\n", res.Path)
		}
		if strings.Contains(err.Error(), "exceeds the available context size") {
			fmt.Fprintln(os.Stderr, "dummyrun: the local model ran out of room. Start Ollama with a larger context and try again: OLLAMA_CONTEXT_LENGTH=16384 ollama serve")
		}
		if isBrowserMissing(err) {
			fmt.Fprintln(os.Stderr, "dummyrun: no browser is set up yet. Run this once, then try again: dummyrun browsers install")
		}
		return fail(err)
	}
	tryReport(os.Stdout, res.Record, res.Path, *out)
	return 0
}

func isBrowserMissing(err error) bool {
	m := err.Error()
	return strings.Contains(m, "chromedriver not found") || strings.Contains(m, "firefox not found")
}

// tryTarget builds the read-only target for one URL. A bare host name gets https.
func tryTarget(raw string) (policy.Target, error) {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return policy.Target{}, fmt.Errorf("%q is not a web address; try something like https://your-site", raw)
	}
	hosts := []string{u.Host}
	// Sites often redirect between the bare domain and www, so allow the other form of the same name.
	if h := u.Hostname(); u.Port() == "" && strings.Contains(h, ".") && notIP(h) {
		if strings.HasPrefix(h, "www.") {
			hosts = append(hosts, strings.TrimPrefix(h, "www."))
		} else {
			hosts = append(hosts, "www."+h)
		}
	}
	t := policy.Target{Adapter: "url", Environment: "try", BaseURL: u.String(), AllowedHosts: hosts, ReadOnly: true}
	if err := t.Validate(); err != nil {
		return policy.Target{}, err
	}
	return t, nil
}

func notIP(h string) bool {
	for _, r := range h {
		if (r < '0' || r > '9') && r != '.' && r != ':' {
			return true
		}
	}
	return false
}

func defaultTryDir() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "dummyrun", "runs", "try")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("dummyrun-runs", "try")
	}
	return filepath.Join(home, ".local", "share", "dummyrun", "runs", "try")
}

// tryPersona loads the first-time visitor from the library inside the program and adds the built-in scenario.
// The library is copied to a temporary folder because the cast loader reads from disk; cleanup removes it.
func tryPersona() (*cast.Persona, cast.Scenario, func(), error) {
	dir, err := os.MkdirTemp("", "dummyrun-try-")
	if err != nil {
		return nil, cast.Scenario{}, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) } // a folder this command made itself
	lib, err := fs.Sub(casts.Library, "library")
	if err == nil {
		err = os.CopyFS(dir, lib)
	}
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, "first-time-visitor", "scenarios", "00-look-around.md"), []byte(tryScenario), 0o600)
	}
	if err != nil {
		cleanup()
		return nil, cast.Scenario{}, func() {}, err
	}
	persona, err := cast.Load(dir, "first-time-visitor")
	if err != nil {
		cleanup()
		return nil, cast.Scenario{}, func() {}, err
	}
	sc, err := persona.Scenario("look-around")
	if err != nil {
		cleanup()
		return nil, cast.Scenario{}, func() {}, err
	}
	return persona, sc, cleanup, nil
}

// tryReport prints what the run found and where it is saved.
func tryReport(w interface{ Write([]byte) (int, error) }, rec *record.Record, recordPath, outDir string) {
	fmt.Fprintf(w, "\n%s: %d steps, %d findings.\n", rec.Outcome, len(rec.Steps), len(rec.Findings))
	if rec.Outcome != record.OutcomeCompleted && rec.OutcomeReason != "" {
		fmt.Fprintf(w, "%s\n", plainText(rec.OutcomeReason))
	}
	for i, f := range rec.Findings {
		fmt.Fprintf(w, "  %d. [%s] %s", i+1, plainText(f.Severity), plainText(f.Title))
		if len(f.Steps) > 0 {
			fmt.Fprintf(w, " (step %s)", joinInts(f.Steps))
		}
		fmt.Fprintln(w)
	}
	if len(rec.Findings) == 0 {
		fmt.Fprintln(w, "  The visitor found nothing to report.")
	}
	runDir := filepath.Join(outDir, rec.ID)
	fmt.Fprintf(w, "\nThe visitor's own report: %s\n", filepath.Join(runDir, "report.md"))
	fmt.Fprintf(w, "Screenshots of every step: %s\n", filepath.Join(runDir, "screens"))
	fmt.Fprintf(w, "The run record: %s\n", recordPath)
	if len(rec.Findings) > 0 {
		fmt.Fprintf(w, "\nEvery finding starts as pending. Check each against its screenshots, then confirm or reject it: dummyrun review %s\n", outDir)
	}
}

func joinInts(n []int) string {
	s := make([]string, len(n))
	for i, v := range n {
		s[i] = fmt.Sprint(v)
	}
	return strings.Join(s, ", ")
}

// plainText makes text from the model safe to print on a terminal. A page can steer the model into writing
// escape sequences, which could move the cursor, rewrite earlier lines or set the window title. Control
// characters become spaces.
func plainText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}
