// Command dummyrun runs one persona through one scenario against a target.
//
//	dummyrun run --cast DIR --persona ID --scenario ID --target CONFIG --out DIR \
//	  --browser chrome|firefox --provider azure --model DEPLOYMENT --region REGION
//
// docs/CLI.md holds the help text of each command and is generated from it: run go generate ./cmd/dummyrun.
package main

//go:generate go run ../../internal/tools/docgen

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/dummy-run/dummy-run/internal/a11y"
	"github.com/dummy-run/dummy-run/internal/access"
	"github.com/dummy-run/dummy-run/internal/bidi"
	"github.com/dummy-run/dummy-run/internal/cast"
	"github.com/dummy-run/dummy-run/internal/identity"
	"github.com/dummy-run/dummy-run/internal/policy"
	"github.com/dummy-run/dummy-run/internal/provider"
	"github.com/dummy-run/dummy-run/internal/record"
	"github.com/dummy-run/dummy-run/internal/runner"
	"github.com/dummy-run/dummy-run/internal/telemetry"
)

func main() {
	code := run(os.Args[1:])
	// A path that skipped its own cleanup must still leave no browser running.
	bidi.CloseAll()
	os.Exit(code)
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "try":
		return tryCmd(args[1:])
	case "browsers":
		return browsersCmd(args[1:])
	case "run":
		return runCmd(args[1:])
	case "review":
		return reviewCmd(args[1:])
	case "a11y":
		return a11yCmd(args[1:])
	case "coverage":
		return coverageCmd(args[1:])
	case "calibrate":
		return calibrateCmd(args[1:])
	case "report":
		return reportCmd(args[1:])
	case "doctor":
		return doctorCmd(args[1:])
	case "session":
		return sessionCmd(args[1:])
	case "mcp":
		return mcpCmd(args[1:])
	case "audit":
		if len(args) > 1 && args[1] == "a11y" {
			return a11yCmd(args[2:])
		}
		fmt.Fprintln(os.Stderr, "dummyrun audit: want \"a11y\"")
		return 2
	case "version":
		fmt.Println(versionLine())
		return 0
	case "-h", "--help", "help":
		usage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "dummyrun: unknown command %q\n", args[0])
		usage()
		return 2
	}
}

// versionLine is what "dummyrun version" prints. A release build sets both values (see .goreleaser.yaml).
func versionLine() string {
	return fmt.Sprintf("dummyrun %s (commit %s)", telemetry.Version, telemetry.Commit)
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage:
  dummyrun browsers install [chrome|firefox|all]
  dummyrun browsers list
  dummyrun try URL [--browser chrome|firefox] [--out DIR] [--provider PROVIDER --model MODEL] \
      [--basic-auth-env USER_ENV:PASS_ENV] [--header-env 'Name:ENV']...
  dummyrun run --cast DIR --persona ID --scenario ID|--scenario-file FILE --target CONFIG --out DIR \
      --browser chrome|firefox [--provider PROVIDER --model MODEL] [--region REGION] [--endpoint URL] \
      [--api-key-env NAME] [--project ID] [--reasoning-effort LEVEL] [--openai-api chat|responses] \
      [--batch ID] [--seed N] [--repeat-index N] [--universe sitemap|URL] [--session FILE] [--identify] [--respect-robots] \
      [--basic-auth-env USER_ENV:PASS_ENV] [--header-env 'Name:ENV']...
  dummyrun coverage --runs DIR [--build SHA] [--strata device|persona] [--confirmed-only=false] [--threshold NAME=VALUE]... \
      [--clusters FILE]... [--format json|markdown] [--out FILE] [--fail-unless-stopped]
  dummyrun report --runs DIR [--a11y FILE]... [--title TEXT] [--client TEXT] [--format html|md|json|sarif|junit] [--include pending] \
      [--baseline FILE] [--build SHA] [--date YYYY-MM-DD] [--out FILE]
  dummyrun review RUNS_DIR|RECORD... [--list [--json] [--verdict V]] [--confirm ID]... [--reject ID]... [--reason TEXT] [--reviewer NAME]
  dummyrun calibrate --labels FILE... [--eval FILE...] [--table FILE] [--out-table FILE] [--report FILE] [--min-samples N]
  dummyrun a11y [--sitemap URL] [URL...] [--all] [--browser chrome|firefox] [--allow FILE] [--json FILE] [--session FILE] [--identify] [--respect-robots] \
      [--basic-auth-env USER_ENV:PASS_ENV] [--header-env 'Name:ENV']...
  dummyrun session save --target CONFIG --role ROLE --out FILE [--browser chrome|firefox] \
      [--basic-auth-env USER_ENV:PASS_ENV] [--header-env 'Name:ENV']...
  dummyrun doctor [--stop]
  dummyrun mcp [--allow-origin ORIGIN]... [--provider PROVIDER --model MODEL] [--cast DIR]... [--out DIR]
  dummyrun version

First run: "dummyrun browsers install" once, set one model key, then "dummyrun try https://your-site".
Run "dummyrun try -h", "dummyrun browsers -h", "dummyrun run -h", "dummyrun coverage -h", "dummyrun report -h", "dummyrun review -h", "dummyrun calibrate -h", "dummyrun a11y -h", "dummyrun session -h", "dummyrun mcp -h" or "dummyrun doctor -h" for every flag. "dummyrun audit a11y" is the same as "dummyrun a11y".

Providers (leave out --provider and --model to have them chosen from your environment, in this order:
DUMMYRUN_AZURE_ENDPOINT, OPENAI_API_KEY, ANTHROPIC_API_KEY, then Ollama on OLLAMA_HOST or localhost:11434;
the first line of the output says what was chosen, and docs/PROVIDERS.md lists the default models):
  azure              Azure OpenAI. --endpoint or DUMMYRUN_AZURE_ENDPOINT; sign in with "az login" or another
                     DefaultAzureCredential source. --region is required.
  openai-compatible  Any OpenAI-style service or local server. --endpoint is the base URL, such as
                     https://api.openai.com/v1 or http://localhost:11434/v1. --api-key-env names the variable
                     that holds the key (leave it out for a local server). --region is required (use "local").
  anthropic          Anthropic Messages API. The key is read from ANTHROPIC_API_KEY or --api-key-env.
  gemini             Gemini Developer API. The key is read from GEMINI_API_KEY or --api-key-env.
  vertex             Gemini on Vertex AI with Application Default Credentials. --project (or
                     GOOGLE_CLOUD_PROJECT) and --region are required.
  bedrock            Amazon Bedrock Converse with the AWS default credential chain. --region defaults to eu-west-2.

An API key is read from the environment when the run starts. It is never printed or written to a run record.
The same holds for the test-user credentials named in a target file's signin block and for the Basic auth and
header secrets of a protected preview (the auth block, --basic-auth-env and --header-env).`)
}

func runCmd(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	castDir := fs.String("cast", "", "cast directory")
	personaID := fs.String("persona", "", "persona ID")
	scenarioID := fs.String("scenario", "", "scenario ID")
	scenarioFile := fs.String("scenario-file", "", "a scenario file to play instead of --scenario, named NN-<id>.md; it is not looked up in the cast")
	targetPath := fs.String("target", "", "target configuration (JSON)")
	out := fs.String("out", "", "directory for run records")
	browser := fs.String("browser", "chrome", "chrome or firefox")
	pf := addProviderFlags(fs)
	steps := fs.Int("steps", 0, "step budget override (up to 40)")
	tokens := fs.Int("tokens", 0, "token budget (input plus output)")
	batch := fs.String("batch", "", "batch ID grouping runs started together")
	seed := fs.Int64("seed", -1, "sampling seed (0 or more), sent to providers that take one; the record shows null where it was not sent")
	repeat := fs.Int("repeat-index", -1, "which repeat this is for the persona, scenario and model in the batch; -1 counts the records already in --out")
	sessionFile := fs.String("session", "", "sign in from this saved session file (see \"dummyrun session save\") instead of the login form")
	universe := fs.String("universe", "", "where the routes that exist come from, for route coverage: \"sitemap\" (base_url/sitemap.xml) or a sitemap URL")
	headful := fs.Bool("headful", false, "show the browser window")
	identify := fs.Bool("identify", false, "add \"DummyRun/<version> (+"+telemetry.ScannerURL+")\" to the browser's user agent (also set by \"identify\": true in the target file)")
	respect := fs.Bool("respect-robots", false, "fetch robots.txt from the target origin and block what it disallows for DummyRun or * (also set by \"respect_robots\": true in the target file); a site-wide disallow stops the run before the browser starts")
	var preview authFlags
	preview.register(fs)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	missing := []string{}
	if *scenarioID != "" && *scenarioFile != "" {
		fmt.Fprintln(os.Stderr, "dummyrun run: use --scenario or --scenario-file, not both")
		return 2
	}
	scenarioNeed := *scenarioID
	if *scenarioFile != "" {
		scenarioNeed = *scenarioFile
	}
	for name, v := range map[string]string{"cast": *castDir, "persona": *personaID, "scenario": scenarioNeed,
		"target": *targetPath, "out": *out} {
		if v == "" {
			missing = append(missing, "--"+name)
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "dummyrun run: missing %v\n", missing)
		usage()
		return 2
	}
	target, err := loadTarget(*targetPath)
	if err != nil {
		return fail(err)
	}
	target.Identify = target.Identify || *identify
	target.RespectRobots = target.RespectRobots || *respect
	if target.Auth, err = preview.auth(target.Auth); err != nil {
		fmt.Fprintln(os.Stderr, "dummyrun run:", err)
		return 2
	}
	if err := target.Validate(); err != nil {
		return fail(err)
	}
	pa, err := access.Resolve(target.Auth, os.Getenv, access.TargetScope(target))
	if err != nil {
		return fail(err)
	}
	persona, err := cast.Load(*castDir, *personaID)
	if err != nil {
		return fail(err)
	}
	var scenario cast.Scenario
	if *scenarioFile != "" {
		scenario, err = cast.LoadScenario(*scenarioFile)
	} else {
		scenario, err = persona.Scenario(*scenarioID)
	}
	if err != nil {
		return fail(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	p, err := pf.build(ctx, os.Stderr)
	if err != nil {
		return fail(err)
	}
	cfg := runner.Config{
		Persona: persona, Scenario: scenario, Target: target, Provider: p, Browser: *browser, Headless: !*headful,
		OutDir: *out, SessionFile: *sessionFile, StepBudget: *steps, TokenBudget: *tokens, Batch: *batch, Log: os.Stderr,
		HTTPClient: access.WrapClient(requestClient(target.Identify), pa), Access: pa,
	}
	if *seed >= 0 {
		cfg.Seed = seed
	}
	if *repeat >= 0 {
		cfg.RepeatIndex = repeat
	}
	if *universe != "" {
		if cfg.Universe, err = fetchUniverse(ctx, cfg.HTTPClient, *universe, target.BaseURL, robotsGuard(cfg.HTTPClient, target.Identify, target.RespectRobots)); err != nil {
			return fail(err)
		}
	}
	res, err := runner.Run(ctx, cfg)
	if errors.Is(err, runner.ErrRobotsDisallow) {
		fmt.Fprintf(os.Stderr, "dummyrun: %v; the run was recorded as blocked at %s\n", err, res.Path)
		return exitRobots
	}
	if err != nil {
		if res != nil && errors.Is(err, provider.ErrContentFiltered) {
			fmt.Fprintf(os.Stderr, "dummyrun: the model provider's content filter stopped the run. It was recorded as provider_filtered at %s\n", res.Path)
		} else if res != nil {
			fmt.Fprintf(os.Stderr, "dummyrun: run errored and was recorded at %s\n", res.Path)
		}
		return fail(err)
	}
	summary(res.Record)
	return 0
}

// fetchUniverse reads a sitemap and returns its normalised routes, each tagged with the source "sitemap".
func fetchUniverse(ctx context.Context, client *http.Client, spec, baseURL string, allowed func(context.Context, string) error) ([]record.RouteUniverseEntry, error) {
	loc := spec
	if spec == "sitemap" {
		loc = strings.TrimSuffix(baseURL, "/") + "/sitemap.xml"
	}
	pages, err := a11y.FetchSitemap(ctx, client, loc, allowed)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []record.RouteUniverseEntry
	for _, p := range pages {
		r := identity.NormaliseRoute(p)
		if !seen[r] {
			seen[r] = true
			out = append(out, record.RouteUniverseEntry{Route: r, Source: "sitemap"})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s lists no pages", loc)
	}
	return out, nil
}

func loadTarget(path string) (policy.Target, error) {
	data, err := os.ReadFile(path) //nolint:gosec // the path comes from the command line on purpose
	if err != nil {
		return policy.Target{}, err
	}
	var t policy.Target
	if err := json.Unmarshal(data, &t); err != nil {
		return policy.Target{}, fmt.Errorf("%s: %w", path, err)
	}
	if t.Adapter == "" {
		t.Adapter = "url"
	}
	if t.Signin != nil {
		t.Signin.Dir = filepath.Dir(path)
	}
	if err := t.Validate(); err != nil {
		return policy.Target{}, fmt.Errorf("%s: %w", path, err)
	}
	return t, nil
}

// exitRobots is the exit status when robots.txt keeps Dummy Run off the site.
const exitRobots = 3

// requestClient is the HTTP client for the engine's own requests (sitemaps, the home page). With identify
// they carry the same suffix as the browser.
func requestClient(identify bool) *http.Client {
	c := &http.Client{Timeout: 30 * time.Second}
	if identify {
		c.Transport = uaTransport{ua: telemetry.RequestUserAgent(true), next: http.DefaultTransport}
	}
	return c
}

type uaTransport struct {
	ua   string
	next http.RoundTripper
}

func (t uaTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("User-Agent", t.ua)
	return t.next.RoundTrip(r)
}

func summary(rec *record.Record) {
	fmt.Printf("%s %s: %d steps, %d findings, %d calls, %d in / %d out tokens\n", rec.ID, rec.Outcome,
		len(rec.Steps), len(rec.Findings), rec.Usage.ModelCalls, rec.Usage.InputTokens, rec.Usage.OutputTokens)
}

func fail(err error) int {
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "dummyrun: interrupted")
		return 130
	}
	fmt.Fprintln(os.Stderr, "dummyrun:", err)
	return 1
}
