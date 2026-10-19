package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"google.golang.org/genai"
)

// FuzzDecodeResponses feeds arbitrary bytes to every provider as the service's
// reply. Decoding must never panic, and a tool call that comes out must have an ID
// (the reply to it needs one) and, for the providers that rebuild the
// arguments, valid JSON.
func FuzzDecodeResponses(f *testing.F) {
	for _, dir := range []string{"openai-compatible", "anthropic", "gemini", "bedrock"} {
		files, _ := filepath.Glob(filepath.Join("testdata", dir, "*.json"))
		for _, file := range files {
			data, err := os.ReadFile(file)
			if err == nil {
				f.Add(data)
			}
		}
	}
	var mu sync.Mutex
	var reply []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		body := reply
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	f.Cleanup(srv.Close)

	f.Setenv("DUMMYRUN_FUZZ_KEY", "k")
	os.Setenv("AWS_EC2_METADATA_DISABLED", "true") //nolint:errcheck,gosec // test setup
	f.Setenv("AWS_CONFIG_FILE", os.DevNull)
	f.Setenv("AWS_SHARED_CREDENTIALS_FILE", os.DevNull)
	compat, err := NewOpenAICompatible(Config{Endpoint: srv.URL + "/v1", Model: "m", Region: "local", APIKeyEnv: "DUMMYRUN_FUZZ_KEY"})
	if err != nil {
		f.Fatal(err)
	}
	responses, err := NewOpenAICompatible(Config{Endpoint: srv.URL + "/v1", Model: "m", Region: "local", API: APIResponses})
	if err != nil {
		f.Fatal(err)
	}
	claude, err := NewAnthropic(Config{Endpoint: srv.URL, Model: "m", APIKeyEnv: "DUMMYRUN_FUZZ_KEY"})
	if err != nil {
		f.Fatal(err)
	}
	gem, err := newGemini(Config{Provider: NameGemini, Model: "m", APIKeyEnv: "DUMMYRUN_FUZZ_KEY"},
		func(c *genai.ClientConfig) { c.HTTPOptions.BaseURL = srv.URL })
	if err != nil {
		f.Fatal(err)
	}
	static := aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
		return aws.Credentials{AccessKeyID: "AKIATESTONLY", SecretAccessKey: "s"}, nil
	})
	bed, err := newBedrock(Config{Model: "m", Endpoint: srv.URL}, []func(*config.LoadOptions) error{config.WithCredentialsProvider(static)}, nil)
	if err != nil {
		f.Fatal(err)
	}
	providers := []struct {
		p         Provider
		validArgs bool
	}{{compat, false}, {responses, false}, {claude, true}, {gem, true}, {bed, true}}

	f.Fuzz(func(t *testing.T, data []byte) {
		mu.Lock()
		reply = data
		mu.Unlock()
		for _, pr := range providers {
			resp, err := pr.p.Chat(context.Background(), Request{Turns: []Turn{{Role: RoleUser, Text: "x"}}, Tools: []Tool{{Name: "look"}}})
			if err != nil {
				continue
			}
			for _, c := range resp.ToolCalls {
				if c.ID == "" {
					t.Fatalf("%s: tool call without an ID: %+v", pr.p.Kind(), c)
				}
				if pr.validArgs && !json.Valid(c.Args) {
					t.Fatalf("%s: tool arguments are not JSON: %q", pr.p.Kind(), c.Args)
				}
			}
		}
	})
}
