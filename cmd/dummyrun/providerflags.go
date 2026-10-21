package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dummy-run/dummy-run/internal/provider"
)

// providerFlags are the flags that choose the model, shared by "run" and "try".
type providerFlags struct {
	prov, model, region, endpoint, apiKeyEnv, effort, apiKind, project, apiVersion *string
}

func addProviderFlags(fs *flag.FlagSet) *providerFlags {
	return &providerFlags{
		prov:       fs.String("provider", "", "model provider: "+strings.Join(provider.Names, ", ")+". Left out, it is chosen from your environment: DUMMYRUN_AZURE_ENDPOINT, OPENAI_API_KEY, ANTHROPIC_API_KEY, then Ollama"),
		model:      fs.String("model", "", "deployment or model name (optional where a default exists; see docs/PROVIDERS.md)"),
		region:     fs.String("region", "", "region the model runs in, such as uksouth or global (optional where the provider has a default)"),
		endpoint:   fs.String("endpoint", "", "base URL of the provider (Azure endpoint, or the OpenAI-compatible base URL)"),
		apiKeyEnv:  fs.String("api-key-env", "", "name of the environment variable that holds the API key"),
		effort:     fs.String("reasoning-effort", "", "reasoning effort for OpenAI-style models: none, minimal, low, medium, high or xhigh"),
		apiKind:    fs.String("openai-api", "", "OpenAI-style API: chat (default) or responses, for models that need it to call tools"),
		project:    fs.String("project", "", "Google Cloud project for the vertex provider"),
		apiVersion: fs.String("api-version", "", "Azure OpenAI API version (default "+provider.DefaultAPIVersion+")"),
	}
}

func (f *providerFlags) config() provider.Config {
	return provider.Config{Provider: *f.prov, Model: *f.model, Region: *f.region, Endpoint: *f.endpoint,
		APIVersion: *f.apiVersion, APIKeyEnv: *f.apiKeyEnv, Project: *f.project, ReasoningEffort: *f.effort, API: *f.apiKind}
}

// build chooses the provider when --provider is not given, prints what it chose as the first line of the run
// output, and builds the provider.
func (f *providerFlags) build(ctx context.Context, w io.Writer) (provider.Provider, error) {
	cfg, note, err := resolveProvider(ctx, f.config(), provider.Env{Getenv: os.Getenv})
	if err != nil {
		return nil, err
	}
	if note != "" {
		fmt.Fprintln(w, note)
	}
	return provider.New(cfg)
}

// resolveProvider fills in what the flags leave out. With no provider named it detects one from the
// environment. It returns a line that says what was chosen, or "" when everything was given.
func resolveProvider(ctx context.Context, cfg provider.Config, env provider.Env) (provider.Config, string, error) {
	note := ""
	if cfg.Provider == "" {
		choice, err := provider.Detect(ctx, env)
		var missing *provider.ModelMissingError
		switch {
		case errors.As(err, &missing) && cfg.Model != "":
			// The default model is not pulled, but the caller named another.
		case err != nil:
			return cfg, "", err
		}
		cfg.Provider = choice.Provider
		note = choice.Summary(cfg.Model)
		fill(&cfg.Model, choice.Model)
		fill(&cfg.Endpoint, choice.Endpoint)
		fill(&cfg.Region, choice.Region)
		fill(&cfg.APIKeyEnv, choice.APIKeyEnv)
		fill(&cfg.ReasoningEffort, choice.ReasoningEffort)
	} else if cfg.Model == "" {
		if m := provider.DefaultModelFor(cfg.Provider); m != "" {
			cfg.Model = m
			note = fmt.Sprintf("Using %s %s (the default for --provider %s)", cfg.Provider, m, cfg.Provider)
		}
	}
	if cfg.Provider == provider.NameAzure {
		fill(&cfg.Endpoint, env.Getenv("DUMMYRUN_AZURE_ENDPOINT"))
	}
	if cfg.Model == "" {
		if cfg.Provider == provider.NameAzure {
			return cfg, "", errors.New("the azure provider needs the name of your deployment: --model <deployment> --region <region>")
		}
		return cfg, "", fmt.Errorf("--model is required with --provider %s", cfg.Provider)
	}
	return cfg, note, nil
}

func fill(dst *string, v string) {
	if *dst == "" {
		*dst = v
	}
}
