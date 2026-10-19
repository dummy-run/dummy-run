// Command dummyrun runs one persona through one scenario against a target.
//
//	dummyrun run --cast DIR --persona ID --scenario ID --target CONFIG --out DIR \
//	  --browser chrome|firefox --provider azure --model DEPLOYMENT --region REGION
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/dummy-run/dummy-run/internal/cast"
	"github.com/dummy-run/dummy-run/internal/policy"
	"github.com/dummy-run/dummy-run/internal/provider"
	"github.com/dummy-run/dummy-run/internal/record"
	"github.com/dummy-run/dummy-run/internal/runner"
	"github.com/dummy-run/dummy-run/internal/telemetry"
)

func main() {
	os.Exit(run(os.Args[1:]))
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
      --browser chrome|firefox --provider azure --model DEPLOYMENT --region REGION [--endpoint URL]
  dummyrun a11y [--sitemap URL] [URL...] [--browser chrome|firefox] [--allow FILE] [--json FILE]
  dummyrun version

The Azure endpoint comes from --endpoint or DUMMYRUN_AZURE_ENDPOINT. The engine authenticates with
azidentity's DefaultAzureCredential (environment, workload identity, managed identity, then "az login")
and never reads an API key.`)
}

func runCmd(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	castDir := fs.String("cast", "", "cast directory")
	personaID := fs.String("persona", "", "persona ID")
	scenarioID := fs.String("scenario", "", "scenario ID")
	targetPath := fs.String("target", "", "target configuration (JSON)")
	out := fs.String("out", "", "directory for run records")
	browser := fs.String("browser", "chrome", "chrome or firefox")
	prov := fs.String("provider", "azure", "model provider")
	model := fs.String("model", "", "deployment or model name")
	region := fs.String("region", "", "region the model runs in, such as uksouth or global")
	endpoint := fs.String("endpoint", os.Getenv("DUMMYRUN_AZURE_ENDPOINT"), "Azure OpenAI endpoint")
	apiVersion := fs.String("api-version", "", "Azure OpenAI API version (default "+provider.DefaultAPIVersion+")")
	steps := fs.Int("steps", 0, "step budget override (up to 40)")
	tokens := fs.Int("tokens", 0, "token budget (input plus output)")
	batch := fs.String("batch", "", "batch ID grouping runs started together")
	headful := fs.Bool("headful", false, "show the browser window")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	missing := []string{}
	for name, v := range map[string]string{"cast": *castDir, "persona": *personaID, "scenario": *scenarioID,
		"target": *targetPath, "out": *out, "model": *model, "region": *region} {
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
	p, err := provider.New(provider.Config{Provider: *prov, Model: *model, Region: *region, Endpoint: *endpoint, APIVersion: *apiVersion})
	if err != nil {
		return fail(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	res, err := runner.Run(ctx, runner.Config{
		Persona: persona, Scenario: scenario, Target: target, Provider: p, Browser: *browser, Headless: !*headful,
		OutDir: *out, StepBudget: *steps, TokenBudget: *tokens, Batch: *batch, Log: os.Stderr,
	})
	if err != nil {
		if res != nil {
			fmt.Fprintf(os.Stderr, "dummyrun: run errored and was recorded at %s\n", res.Path)
		}
		return fail(err)
	}
	summary(res.Record)
	return 0
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
