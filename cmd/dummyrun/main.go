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
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/dummy-run/dummy-run/internal/a11y"
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
	case "run":
		return runCmd(args[1:])
	case "a11y":
		return a11yCmd(args[1:])
	case "coverage":
		return coverageCmd(args[1:])
	case "report":
		return reportCmd(args[1:])
	case "doctor":
		return doctorCmd(args[1:])
	case "audit":
		if len(args) > 1 && args[1] == "a11y" {
			return a11yCmd(args[2:])
		}
		fmt.Fprintln(os.Stderr, "dummyrun audit: want \"a11y\"")
		return 2
	case "version":
		fmt.Println("dummyrun", telemetry.Version)
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

func usage() {
	fmt.Fprintln(os.Stderr, `Usage:
  dummyrun run --cast DIR --persona ID --scenario ID --target CONFIG --out DIR \
      --browser chrome|firefox --provider PROVIDER --model MODEL [--region REGION] [--endpoint URL] \
      [--api-key-env NAME] [--project ID] [--reasoning-effort LEVEL] [--openai-api chat|responses] \
      [--batch ID] [--seed N] [--repeat-index N] [--universe sitemap|URL]
  dummyrun coverage --runs DIR [--build SHA] [--strata device|persona] [--confirmed-only=false] [--threshold NAME=VALUE]... \
      [--clusters FILE]... [--format json|markdown] [--out FILE] [--fail-unless-stopped]
  dummyrun report --runs DIR [--a11y FILE]... [--title TEXT] [--client TEXT] [--format html|md|json|sarif|junit] [--include pending] \
      [--baseline FILE] [--build SHA] [--date YYYY-MM-DD] [--out FILE]
  dummyrun a11y [--sitemap URL] [URL...] [--browser chrome|firefox] [--allow FILE] [--json FILE]
  dummyrun doctor [--stop]
  dummyrun version

Run "dummyrun run -h", "dummyrun coverage -h", "dummyrun report -h", "dummyrun a11y -h" or "dummyrun doctor -h" for every flag. "dummyrun audit a11y" is the same as "dummyrun a11y".

Providers:
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

An API key is read from the environment when the run starts. It is never printed or written to a run record.`)
}

func runCmd(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	castDir := fs.String("cast", "", "cast directory")
	personaID := fs.String("persona", "", "persona ID")
	scenarioID := fs.String("scenario", "", "scenario ID")
	targetPath := fs.String("target", "", "target configuration (JSON)")
	out := fs.String("out", "", "directory for run records")
	browser := fs.String("browser", "chrome", "chrome or firefox")
	prov := fs.String("provider", "azure", "model provider: "+strings.Join(provider.Names, ", "))
	model := fs.String("model", "", "deployment or model name")
	region := fs.String("region", "", "region the model runs in, such as uksouth or global (optional where the provider has a default)")
	endpoint := fs.String("endpoint", "", "base URL of the provider (Azure endpoint, or the OpenAI-compatible base URL)")
	apiKeyEnv := fs.String("api-key-env", "", "name of the environment variable that holds the API key")
	effort := fs.String("reasoning-effort", "", "reasoning effort for OpenAI-style models: none, minimal, low, medium, high or xhigh")
	apiKind := fs.String("openai-api", "", "OpenAI-style API: chat (default) or responses, for models that need it to call tools")
	project := fs.String("project", "", "Google Cloud project for the vertex provider")
	apiVersion := fs.String("api-version", "", "Azure OpenAI API version (default "+provider.DefaultAPIVersion+")")
	steps := fs.Int("steps", 0, "step budget override (up to 40)")
	tokens := fs.Int("tokens", 0, "token budget (input plus output)")
	batch := fs.String("batch", "", "batch ID grouping runs started together")
	seed := fs.Int64("seed", -1, "sampling seed sent to the model API where the provider takes one (0 or more); recorded as seed, null where it is not sent")
	repeat := fs.Int("repeat-index", -1, "repeat count for this persona, scenario and model in the batch (default: counted from the records in --out)")
	universe := fs.String("universe", "", "where the routes that exist come from, for route coverage: \"sitemap\" (base_url/sitemap.xml) or a sitemap URL")
	headful := fs.Bool("headful", false, "show the browser window")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	missing := []string{}
	for name, v := range map[string]string{"cast": *castDir, "persona": *personaID, "scenario": *scenarioID,
		"target": *targetPath, "out": *out, "model": *model} {
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
	persona, err := cast.Load(*castDir, *personaID)
	if err != nil {
		return fail(err)
	}
	scenario, err := persona.Scenario(*scenarioID)
	if err != nil {
		return fail(err)
	}
	if *endpoint == "" && *prov == provider.NameAzure {
		*endpoint = os.Getenv("DUMMYRUN_AZURE_ENDPOINT")
	}
	p, err := provider.New(provider.Config{Provider: *prov, Model: *model, Region: *region, Endpoint: *endpoint,
		APIVersion: *apiVersion, APIKeyEnv: *apiKeyEnv, Project: *project, ReasoningEffort: *effort, API: *apiKind})
	if err != nil {
		return fail(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg := runner.Config{
		Persona: persona, Scenario: scenario, Target: target, Provider: p, Browser: *browser, Headless: !*headful,
		OutDir: *out, StepBudget: *steps, TokenBudget: *tokens, Batch: *batch, Log: os.Stderr,
	}
	if *seed >= 0 {
		cfg.Seed = seed
	}
	if *repeat >= 0 {
		cfg.RepeatIndex = repeat
	}
	if *universe != "" {
		if cfg.Universe, err = fetchUniverse(ctx, *universe, target.BaseURL); err != nil {
			return fail(err)
		}
	}
	res, err := runner.Run(ctx, cfg)
	if err != nil {
		if res != nil {
			fmt.Fprintf(os.Stderr, "dummyrun: run errored and was recorded at %s\n", res.Path)
		}
		return fail(err)
	}
	summary(res.Record)
	return 0
}

// fetchUniverse reads a sitemap and returns its normalised routes, each tagged with the source "sitemap".
func fetchUniverse(ctx context.Context, spec, baseURL string) ([]record.RouteUniverseEntry, error) {
	loc := spec
	if spec == "sitemap" {
		loc = strings.TrimSuffix(baseURL, "/") + "/sitemap.xml"
	}
	pages, err := a11y.FetchSitemap(ctx, nil, loc)
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
	if err := t.Validate(); err != nil {
		return policy.Target{}, fmt.Errorf("%s: %w", path, err)
	}
	return t, nil
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
