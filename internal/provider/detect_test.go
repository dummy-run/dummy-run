package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func ollamaWith(models ...string) func(context.Context, string) ([]string, error) {
	return func(context.Context, string) ([]string, error) { return models, nil }
}

func noOllama(context.Context, string) ([]string, error) {
	return nil, errors.New("connection refused")
}

func TestDetectOrder(t *testing.T) {
	all := map[string]string{"DUMMYRUN_AZURE_ENDPOINT": "https://x.openai.azure.com", "OPENAI_API_KEY": "k", "ANTHROPIC_API_KEY": "k"}
	for _, tc := range []struct {
		name     string
		env      map[string]string
		ollama   func(context.Context, string) ([]string, error)
		provider string
		label    string
	}{
		{"azure wins, as it was the only default before", all, ollamaWith("gemma4:e4b"), NameAzure, "Azure OpenAI"},
		{"openai", map[string]string{"OPENAI_API_KEY": "k", "ANTHROPIC_API_KEY": "k"}, ollamaWith("gemma4:e4b"), NameOpenAICompatible, "OpenAI"},
		{"anthropic", map[string]string{"ANTHROPIC_API_KEY": "k"}, ollamaWith("gemma4:e4b"), NameAnthropic, "Anthropic"},
		{"ollama on the default port", nil, ollamaWith("gemma4:e4b"), NameOpenAICompatible, "Ollama"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Detect(context.Background(), Env{Getenv: envOf(tc.env), Ollama: tc.ollama})
			if err != nil {
				t.Fatal(err)
			}
			if c.Provider != tc.provider || c.Label != tc.label {
				t.Errorf("got %s (%s), want %s (%s)", c.Label, c.Provider, tc.label, tc.provider)
			}
		})
	}
}

func TestDetectDefaults(t *testing.T) {
	c, _ := Detect(context.Background(), Env{Getenv: envOf(map[string]string{"OPENAI_API_KEY": "secret"}), Ollama: noOllama})
	if c.Model != "gpt-6-luna" || c.Endpoint != "https://api.openai.com/v1" || c.APIKeyEnv != "OPENAI_API_KEY" || c.ReasoningEffort != "none" {
		t.Errorf("openai choice %+v", c)
	}
	if got := c.Summary(""); got != "Using OpenAI gpt-6-luna (OPENAI_API_KEY is set)" || strings.Contains(got, "secret") {
		t.Errorf("summary %q", got)
	}
	c, _ = Detect(context.Background(), Env{Getenv: envOf(map[string]string{"ANTHROPIC_API_KEY": "secret"}), Ollama: noOllama})
	if c.Model != "claude-haiku-5-5" || c.Summary("") != "Using Anthropic claude-haiku-5-5 (ANTHROPIC_API_KEY is set)" {
		t.Errorf("anthropic choice %+v", c)
	}
	if got := c.Summary("claude-opus-5-5"); !strings.Contains(got, "claude-opus-5-5") {
		t.Errorf("a model from the flag should show in the summary: %q", got)
	}
}

func TestDetectNothing(t *testing.T) {
	_, err := Detect(context.Background(), Env{Getenv: envOf(nil), Ollama: noOllama})
	if !errors.Is(err, ErrNoProvider) {
		t.Fatalf("got %v, want ErrNoProvider", err)
	}
	for _, want := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "ollama pull gemma4:e4b", "DUMMYRUN_AZURE_ENDPOINT"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not mention %s", want)
		}
	}
}

func TestDetectOllamaHost(t *testing.T) {
	var asked string
	list := func(_ context.Context, base string) ([]string, error) {
		asked = base
		return []string{"gemma4:e4b"}, nil
	}
	c, err := Detect(context.Background(), Env{Getenv: envOf(map[string]string{"OLLAMA_HOST": "0.0.0.0:9999"}), Ollama: list})
	if err != nil {
		t.Fatal(err)
	}
	if asked != "http://127.0.0.1:9999" || c.Endpoint != "http://127.0.0.1:9999/v1" || c.Region != "local" {
		t.Errorf("asked %q, choice %+v", asked, c)
	}
	// A host that is named but silent is an error, not a quiet fall through.
	_, err = Detect(context.Background(), Env{Getenv: envOf(map[string]string{"OLLAMA_HOST": "box:11434"}), Ollama: noOllama})
	if err == nil || errors.Is(err, ErrNoProvider) || !strings.Contains(err.Error(), "ollama serve") {
		t.Errorf("got %v, want an error that names ollama serve", err)
	}
}

func TestDetectOllamaModelNotPulled(t *testing.T) {
	c, err := Detect(context.Background(), Env{Getenv: envOf(nil), Ollama: ollamaWith("llama3.2:latest")})
	var miss *ModelMissingError
	if !errors.As(err, &miss) {
		t.Fatalf("got %v, want a ModelMissingError", err)
	}
	if !strings.Contains(err.Error(), "ollama pull gemma4:e4b") || !strings.Contains(err.Error(), "--model llama3.2:latest") {
		t.Errorf("message %q", err)
	}
	if c.Provider != NameOpenAICompatible {
		t.Errorf("the choice should still be filled in: %+v", c)
	}
}

func TestHasOllamaModel(t *testing.T) {
	if !HasOllamaModel([]string{"gemma4:e4b"}, "gemma4:e4b") || HasOllamaModel([]string{"gemma4:e2b"}, "gemma4:e4b") || HasOllamaModel(nil, "x") {
		t.Error("HasOllamaModel gave a wrong answer")
	}
	if !HasOllamaModel([]string{"mistral:latest"}, "mistral") {
		t.Error("the :latest tag should match a bare name")
	}
}

func TestOllamaBase(t *testing.T) {
	for in, want := range map[string]string{
		"localhost":             "http://localhost:11434",
		"localhost:11434":       "http://localhost:11434",
		"0.0.0.0":               "http://127.0.0.1:11434",
		":11500":                "http://127.0.0.1:11500",
		"http://box.lan:8080":   "http://box.lan:8080",
		"https://ollama.my.lan": "https://ollama.my.lan:11434",
	} {
		got, err := OllamaBase(in)
		if err != nil || got != want {
			t.Errorf("OllamaBase(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestOllamaModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"models":[{"name":"gemma4:e4b"},{"name":"qwen3.5:4b"}]}`))
	}))
	defer srv.Close()
	got, err := OllamaModels(context.Background(), srv.URL)
	if err != nil || len(got) != 2 || got[0] != "gemma4:e4b" {
		t.Errorf("got %v, %v", got, err)
	}
	html := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>")) }))
	defer html.Close()
	if _, err := OllamaModels(context.Background(), html.URL); err == nil {
		t.Error("a server that is not Ollama was accepted")
	}
}

func TestDefaultsAreComplete(t *testing.T) {
	for _, d := range Defaults() {
		if d.Label == "" || d.Provider == "" || d.Trigger == "" || d.Why == "" {
			t.Errorf("default %q is missing a field: %+v", d.Label, d)
		}
		if d.Provider != NameAzure && d.Model == "" {
			t.Errorf("default %q has no model", d.Label)
		}
	}
	if DefaultModelFor(NameAnthropic) != DefaultAnthropic.Model || DefaultModelFor(NameGemini) != "" {
		t.Error("DefaultModelFor is wrong")
	}
}
