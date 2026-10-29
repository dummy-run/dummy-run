package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dummy-run/dummy-run/internal/provider"
	"github.com/dummy-run/dummy-run/internal/record"
)

func env(m map[string]string) provider.Env {
	return provider.Env{Getenv: func(k string) string { return m[k] },
		Ollama: func(context.Context, string) ([]string, error) { return nil, errors.New("down") }}
}

func TestResolveProviderDetects(t *testing.T) {
	cfg, note, err := resolveProvider(context.Background(), provider.Config{}, env(map[string]string{"OPENAI_API_KEY": "k"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != "openai-compatible" || cfg.Model != "gpt-6-luna" || cfg.APIKeyEnv != "OPENAI_API_KEY" || cfg.Region != "global" {
		t.Errorf("config %+v", cfg)
	}
	if note != "Using OpenAI gpt-6-luna (OPENAI_API_KEY is set)" {
		t.Errorf("note %q", note)
	}
}

func TestResolveProviderKeepsYourFlags(t *testing.T) {
	cfg, note, err := resolveProvider(context.Background(), provider.Config{Model: "gpt-6.1-sol", Region: "eu"}, env(map[string]string{"OPENAI_API_KEY": "k"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "gpt-6.1-sol" || cfg.Region != "eu" || !strings.Contains(note, "gpt-6.1-sol") {
		t.Errorf("config %+v, note %q", cfg, note)
	}
}

func TestResolveProviderExplicit(t *testing.T) {
	cfg, note, err := resolveProvider(context.Background(), provider.Config{Provider: "anthropic"}, env(nil))
	if err != nil || cfg.Model != "claude-haiku-5-5" || !strings.Contains(note, "claude-haiku-5-5") {
		t.Errorf("got %+v, %q, %v", cfg, note, err)
	}
	cfg, note, err = resolveProvider(context.Background(), provider.Config{Provider: "gemini", Model: "m"}, env(nil))
	if err != nil || note != "" || cfg.Model != "m" {
		t.Errorf("a fully named provider should print no choice: %+v, %q, %v", cfg, note, err)
	}
	if _, _, err = resolveProvider(context.Background(), provider.Config{Provider: "bedrock"}, env(nil)); err == nil || !strings.Contains(err.Error(), "--model") {
		t.Errorf("got %v, want a --model error", err)
	}
}

func TestResolveProviderAzure(t *testing.T) {
	_, _, err := resolveProvider(context.Background(), provider.Config{}, env(map[string]string{"DUMMYRUN_AZURE_ENDPOINT": "https://r.openai.azure.com"}))
	if err == nil || !strings.Contains(err.Error(), "deployment") {
		t.Errorf("got %v, want a message about the deployment name", err)
	}
	cfg, _, err := resolveProvider(context.Background(), provider.Config{Provider: "azure", Model: "d", Region: "uksouth"}, env(map[string]string{"DUMMYRUN_AZURE_ENDPOINT": "https://r.openai.azure.com"}))
	if err != nil || cfg.Endpoint != "https://r.openai.azure.com" {
		t.Errorf("an explicit azure run should read the endpoint variable: %+v, %v", cfg, err)
	}
}

func TestResolveProviderNothing(t *testing.T) {
	_, _, err := resolveProvider(context.Background(), provider.Config{}, env(nil))
	if !errors.Is(err, provider.ErrNoProvider) {
		t.Errorf("got %v", err)
	}
}

func TestResolveProviderOllamaWithOwnModel(t *testing.T) {
	e := provider.Env{Getenv: func(string) string { return "" },
		Ollama: func(context.Context, string) ([]string, error) { return []string{"qwen3.5:4b"}, nil }}
	if _, _, err := resolveProvider(context.Background(), provider.Config{}, e); err == nil || !strings.Contains(err.Error(), "ollama pull gemma4:e4b") {
		t.Errorf("got %v, want the pull command", err)
	}
	cfg, _, err := resolveProvider(context.Background(), provider.Config{Model: "qwen3.5:4b"}, e)
	if err != nil || cfg.Model != "qwen3.5:4b" || cfg.Endpoint != "http://localhost:11434/v1" {
		t.Errorf("a named model should go ahead: %+v, %v", cfg, err)
	}
}

func TestTryTarget(t *testing.T) {
	tgt, err := tryTarget("example.com/pricing")
	if err != nil {
		t.Fatal(err)
	}
	if tgt.BaseURL != "https://example.com/pricing" || !tgt.ReadOnly || tgt.Environment != "try" {
		t.Errorf("target %+v", tgt)
	}
	if len(tgt.AllowedHosts) != 2 || tgt.AllowedHosts[0] != "example.com" || tgt.AllowedHosts[1] != "www.example.com" {
		t.Errorf("hosts %v", tgt.AllowedHosts)
	}
	tgt, _ = tryTarget("https://www.example.com/")
	if len(tgt.AllowedHosts) != 2 || tgt.AllowedHosts[1] != "example.com" {
		t.Errorf("hosts %v", tgt.AllowedHosts)
	}
	tgt, err = tryTarget("http://localhost:3000")
	if err != nil || len(tgt.AllowedHosts) != 1 || tgt.AllowedHosts[0] != "localhost:3000" {
		t.Errorf("localhost: %+v, %v", tgt, err)
	}
	for _, bad := range []string{"ftp://example.com", "http://example.com", "https://", "not a url at all %%"} {
		if _, err := tryTarget(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestTryPersonaLoads(t *testing.T) {
	p, sc, cleanup, err := tryPersona()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if p.ID != "first-time-visitor" || sc.ID != "look-around" || sc.Device != "desktop" {
		t.Errorf("persona %s, scenario %+v", p.ID, sc)
	}
}

func TestTryReport(t *testing.T) {
	var b bytes.Buffer
	rec := &record.Record{ID: "run-1", Outcome: record.OutcomeCompleted, Steps: make([]record.Step, 3),
		Findings: []record.Finding{{Severity: "major", Title: "No price anywhere", Steps: []int{2, 3}}}}
	tryReport(&b, rec, "/out/run-1.json", "/out")
	for _, want := range []string{"3 steps, 1 findings", "[major] No price anywhere (step 2, 3)", "/out/run-1/report.md",
		"/out/run-1/screens", "/out/run-1.json", "dummyrun review /out"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, b.String())
		}
	}
}

func TestPlainTextStripsEscapes(t *testing.T) {
	got := plainText("ok\x1b]0;owned\x07 and \x1b[2Jmore\r\nnext")
	if strings.ContainsAny(got, "\x1b\x07\r\n") {
		t.Errorf("control characters survived: %q", got)
	}
	if !strings.Contains(got, "ok") || !strings.Contains(got, "more") {
		t.Errorf("text was lost: %q", got)
	}
	var b bytes.Buffer
	rec := &record.Record{ID: "r", Outcome: record.OutcomeCompleted,
		Findings: []record.Finding{{Severity: "minor", Title: "bad \x1b[31mred"}}}
	tryReport(&b, rec, "/o/r.json", "/o")
	if strings.Contains(b.String(), "\x1b") {
		t.Errorf("tryReport printed an escape: %q", b.String())
	}
}

func TestNewCommandsRejectBadArguments(t *testing.T) {
	for name, args := range map[string][]string{
		"try with nothing":          {"try"},
		"try with two addresses":    {"try", "a.example", "b.example"},
		"try with a bad address":    {"try", "ftp://example.com"},
		"browsers with nothing":     {"browsers"},
		"browsers unknown command":  {"browsers", "remove"},
		"browsers install unknown":  {"browsers", "install", "safari"},
		"browsers install too many": {"browsers", "install", "chrome", "firefox"},
	} {
		if code := run(args); code != 2 {
			t.Errorf("%s: exit %d, want 2", name, code)
		}
	}
	for _, args := range [][]string{{"try", "-h"}, {"browsers", "-h"}, {"browsers", "list", "-h"}} {
		if code := run(args); code != 0 {
			t.Errorf("%v: exit %d, want 0", args, code)
		}
	}
}

func TestBrowsersListWorksOnAnEmptyCache(t *testing.T) {
	t.Setenv("DUMMYRUN_BROWSER_CACHE", t.TempDir())
	if code := run([]string{"browsers", "list"}); code != 0 {
		t.Errorf("exit %d, want 0", code)
	}
}

func TestTryAcceptsSelfCheckFlag(t *testing.T) {
	// A bad address fails after the flags parse, so the error names the address, not the flag.
	var stderr strings.Builder
	code := captureStderr(t, &stderr, func() int { return run([]string{"try", "ftp://example.com", "--self-check=false"}) })
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Fatalf("--self-check is not a try flag: %s", stderr.String())
	}
}
