package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dummy-run/dummy-run/internal/mcpserver"
	"github.com/dummy-run/dummy-run/internal/provider"
	"github.com/dummy-run/dummy-run/internal/telemetry"
)

const mcpUsage = `Usage:
  dummyrun mcp [--allow-origin ORIGIN]... [--cast DIR]... [--out DIR] [--browser chrome|firefox] [flags]

Serves four tools to a coding agent over standard input and output, using the Model Context Protocol:
list_personas, a11y_sweep, run_persona and get_run. Standard output carries only the protocol.

Sites:
  --allow-origin ORIGIN   a site the tools may test, besides localhost, 127.0.0.1 and private-network hosts.
                          A host, a wildcard such as *.example.com or an origin such as https://preview.example.com.
                          Repeat the flag, or set DUMMYRUN_MCP_ALLOW_ORIGINS to a comma-separated list.

Files:
  --cast DIR              a folder of local personas. Repeat for more. The default is each folder in ./casts
                          except library. The built-in library is always available.
  --out DIR               where run records go, and the only place get_run reads (default
                          ~/.local/share/dummyrun/runs/mcp)

Limits:
  --max-steps N           step cap for one run_persona call (default 25; a call asks for 10 unless it says more)
  --max-tokens N          token cap for one run_persona call (default 400000)
  --session-tokens N      total model tokens for the session (default 1000000)
  --max-runs N            run_persona calls per session (default 10)
  --max-sweeps N          a11y_sweep calls per session (default 40)

Model provider, for run_persona only. Each flag also reads the variable shown:
  --provider NAME         DUMMYRUN_PROVIDER (default azure)
  --model NAME            DUMMYRUN_MODEL
  --region NAME           DUMMYRUN_REGION
  --endpoint URL          DUMMYRUN_ENDPOINT, and DUMMYRUN_AZURE_ENDPOINT for azure
  --api-key-env NAME      DUMMYRUN_API_KEY_ENV
  --project ID            DUMMYRUN_PROJECT
  --reasoning-effort L    DUMMYRUN_REASONING_EFFORT
  --openai-api KIND       DUMMYRUN_OPENAI_API
  --api-version V         DUMMYRUN_API_VERSION

Other:
  --browser NAME          chrome or firefox (default chrome)
  --verbose               write the engine's progress lines to standard error`

func envOr(flagValue, name string) string {
	if flagValue != "" {
		return flagValue
	}
	return os.Getenv(name)
}

func mcpCmd(args []string) int {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprintln(os.Stderr, mcpUsage) }
	var origins, casts listFlag
	fs.Var(&origins, "allow-origin", "a site the tools may test")
	fs.Var(&casts, "cast", "a folder of local personas")
	out := fs.String("out", "", "run record folder")
	browser := fs.String("browser", "chrome", "chrome or firefox")
	verbose := fs.Bool("verbose", false, "write progress lines to standard error")
	lim := mcpserver.DefaultLimits()
	maxSteps := fs.Int("max-steps", lim.MaxSteps, "step cap for one run")
	maxTokens := fs.Int("max-tokens", lim.MaxTokens, "token cap for one run")
	sessionTokens := fs.Int("session-tokens", lim.SessionTokens, "token cap for the session")
	maxRuns := fs.Int("max-runs", lim.MaxRuns, "run_persona calls per session")
	maxSweeps := fs.Int("max-sweeps", lim.MaxSweeps, "a11y_sweep calls per session")
	prov := fs.String("provider", "", "model provider")
	model := fs.String("model", "", "deployment or model name")
	region := fs.String("region", "", "region the model runs in")
	endpoint := fs.String("endpoint", "", "base URL of the provider")
	apiKeyEnv := fs.String("api-key-env", "", "name of the variable that holds the API key")
	project := fs.String("project", "", "Google Cloud project for vertex")
	effort := fs.String("reasoning-effort", "", "reasoning effort")
	apiKind := fs.String("openai-api", "", "chat or responses")
	apiVersion := fs.String("api-version", "", "Azure OpenAI API version")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "dummyrun mcp: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	if *browser != "chrome" && *browser != "firefox" {
		fmt.Fprintf(os.Stderr, "dummyrun mcp: --browser %q: want chrome or firefox\n", *browser)
		return 2
	}
	if *maxSteps < 1 || *maxTokens < 1 || *sessionTokens < 1 || *maxRuns < 1 || *maxSweeps < 1 {
		fmt.Fprintln(os.Stderr, "dummyrun mcp: the limits must be 1 or more")
		return 2
	}
	allow := []string(origins)
	if env := os.Getenv("DUMMYRUN_MCP_ALLOW_ORIGINS"); env != "" {
		allow = append(allow, strings.Split(env, ",")...)
	}
	parsed, err := mcpserver.ParseOrigins(allow)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dummyrun mcp:", err)
		return 2
	}
	if len(casts) == 0 {
		casts = defaultCasts()
	}
	runs := *out
	if runs == "" {
		if runs, err = defaultRunsDir(); err != nil {
			return fail(err)
		}
	}
	pcfg := provider.Config{Provider: envOr(*prov, "DUMMYRUN_PROVIDER"), Model: envOr(*model, "DUMMYRUN_MODEL"),
		Region: envOr(*region, "DUMMYRUN_REGION"), Endpoint: envOr(*endpoint, "DUMMYRUN_ENDPOINT"),
		APIVersion: envOr(*apiVersion, "DUMMYRUN_API_VERSION"), APIKeyEnv: envOr(*apiKeyEnv, "DUMMYRUN_API_KEY_ENV"),
		Project: envOr(*project, "DUMMYRUN_PROJECT"), ReasoningEffort: envOr(*effort, "DUMMYRUN_REASONING_EFFORT"),
		API: envOr(*apiKind, "DUMMYRUN_OPENAI_API")}
	if pcfg.Provider == "" {
		pcfg.Provider = provider.NameAzure
	}
	if pcfg.Endpoint == "" && pcfg.Provider == provider.NameAzure {
		pcfg.Endpoint = os.Getenv("DUMMYRUN_AZURE_ENDPOINT")
	}
	cfg := mcpserver.Config{
		Version: telemetry.Version, Origins: parsed, CastDirs: casts, RunsDir: runs, Browser: *browser,
		Limits: mcpserver.Limits{DefaultSteps: min(lim.DefaultSteps, *maxSteps), MaxSteps: *maxSteps,
			DefaultTokens: min(lim.DefaultTokens, *maxTokens), MaxTokens: *maxTokens,
			SessionTokens: *sessionTokens, MaxRuns: *maxRuns, MaxSweeps: *maxSweeps},
		SecretEnv: secretEnvNames(pcfg.APIKeyEnv),
		NewProvider: func() (provider.Provider, error) {
			if pcfg.Model == "" {
				return nil, errors.New("no model is set (--model or DUMMYRUN_MODEL)")
			}
			return provider.New(pcfg)
		},
	}
	if *verbose {
		cfg.Log = os.Stderr
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv, state := mcpserver.New(cfg)
	// Run returns when the client closes standard input or a signal arrives. Close then waits for any
	// tool call that is still running, so its browser is shut down before the program exits.
	err = srv.Run(ctx, &mcp.StdioTransport{})
	state.Close()
	if err != nil && ctx.Err() == nil {
		return fail(err)
	}
	return 0
}

// secretEnvNames lists the variables whose values must never appear in a result.
func secretEnvNames(custom string) []string {
	names := []string{"ANTHROPIC_API_KEY", "GEMINI_API_KEY", "OPENAI_API_KEY", "AZURE_OPENAI_API_KEY", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN"}
	if custom != "" {
		names = append(names, custom)
	}
	return names
}

// defaultCasts is every folder in ./casts except the library, which the program carries itself.
func defaultCasts() []string {
	entries, err := os.ReadDir("casts")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && e.Name() != "library" {
			out = append(out, filepath.Join("casts", e.Name()))
		}
	}
	return out
}

func defaultRunsDir() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "dummyrun", "runs", "mcp"), nil
}
