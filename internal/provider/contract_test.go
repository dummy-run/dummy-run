package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"google.golang.org/genai"
)

// The contract test runs every provider against a local server that replays a
// fixture from testdata. No test here needs a key or the network. The
// openai-compatible fixtures were recorded from a local Ollama server; the
// others follow each vendor's documented response format.

const testKey = "test-key-DO-NOT-LEAK-7f3a"

// backend describes one provider for the contract test.
type backend struct {
	name string
	dir  string
	// build returns the provider talking to the server at url.
	build func(t *testing.T, url string) Provider
	// keyHeader is the request header that carries the key, or "" when none does.
	keyHeader string
	// path is a fragment the request path must contain.
	path string
}

func backends() []backend {
	return []backend{
		{name: "openai-compatible", dir: "openai-compatible", keyHeader: "Authorization", path: "/v1/chat/completions",
			build: func(t *testing.T, url string) Provider {
				t.Setenv("DUMMYRUN_TEST_KEY", testKey)
				p, err := NewOpenAICompatible(Config{Endpoint: url + "/v1", Model: "model-x", Region: "local", APIKeyEnv: "DUMMYRUN_TEST_KEY"})
				must(t, err)
				return p
			}},
		{name: "anthropic", dir: "anthropic", keyHeader: "X-Api-Key", path: "/v1/messages",
			build: func(t *testing.T, url string) Provider {
				t.Setenv("DUMMYRUN_TEST_KEY", testKey)
				p, err := NewAnthropic(Config{Endpoint: url, Model: "claude-test", APIKeyEnv: "DUMMYRUN_TEST_KEY"})
				must(t, err)
				return p
			}},
		{name: "gemini-api", dir: "gemini", keyHeader: "X-Goog-Api-Key", path: "/models/gemini-test:generateContent",
			build: func(t *testing.T, url string) Provider {
				t.Setenv("DUMMYRUN_TEST_KEY", testKey)
				p, err := newGemini(Config{Provider: NameGemini, Model: "gemini-test", APIKeyEnv: "DUMMYRUN_TEST_KEY"},
					func(c *genai.ClientConfig) { c.HTTPOptions.BaseURL = url })
				must(t, err)
				return p
			}},
		{name: "vertex-gemini", dir: "gemini", path: "/projects/test-project/locations/europe-west2/publishers/google/models/gemini-test:generateContent",
			build: func(t *testing.T, url string) Provider {
				t.Setenv("GOOGLE_API_KEY", "")
				t.Setenv("GEMINI_API_KEY", "")
				p, err := newGemini(Config{Provider: NameVertex, Model: "gemini-test", Project: "test-project", Region: "europe-west2"},
					func(c *genai.ClientConfig) {
						c.HTTPOptions.BaseURL = url
						// A plain client stands in for Application Default Credentials.
						c.HTTPClient = http.DefaultClient
					})
				must(t, err)
				return p
			}},
		{name: "bedrock", dir: "bedrock", path: "/model/test.model-v1:0/converse",
			build: func(t *testing.T, url string) Provider {
				t.Setenv("AWS_CONFIG_FILE", os.DevNull)
				t.Setenv("AWS_SHARED_CREDENTIALS_FILE", os.DevNull)
				t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
				static := aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
					return aws.Credentials{AccessKeyID: "AKIATESTONLY", SecretAccessKey: "test-secret"}, nil
				})
				p, err := newBedrock(Config{Model: "test.model-v1:0", Endpoint: url},
					[]func(*config.LoadOptions) error{config.WithCredentialsProvider(static)}, nil)
				must(t, err)
				return p
			}},
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// replay is a server that records requests and answers with one fixture.
type replay struct {
	*httptest.Server
	mu     sync.Mutex
	status int
	body   []byte
	reqs   []recorded
}

type recorded struct {
	path   string
	header http.Header
	body   string
}

func newReplay(t *testing.T) *replay {
	r := &replay{status: http.StatusOK}
	r.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		data, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.reqs = append(r.reqs, recorded{path: req.URL.Path, header: req.Header.Clone(), body: string(data)})
		status, body := r.status, r.body
		r.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(r.Close)
	return r
}

func (r *replay) serve(t *testing.T, status int, dir, file string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", dir, file))
	must(t, err)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status, r.body, r.reqs = status, data, nil
}

func (r *replay) last(t *testing.T) recorded {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.reqs) == 0 {
		t.Fatal("no request reached the server")
	}
	return r.reqs[len(r.reqs)-1]
}

var testPNG = []byte("\x89PNG\r\n\x1a\ncontract-test-image")

func contractRequest() Request {
	return Request{
		System: "SYSMARK you are a persona testing a website.",
		Turns: []Turn{
			{Role: RoleUser, Text: "USERMARK begin.", Images: []Image{{PNG: testPNG}}},
			{Role: RoleAssistant, Text: "ASSISTMARK", ToolCalls: []ToolCall{{ID: "call_1", Name: "look", Args: json.RawMessage(`{"zzargs":1}`)}}},
			{Role: RoleTool, ToolCallID: "call_1", Text: "TOOLMARK step 1 looked."},
			{Role: RoleUser, Text: "Screenshot after step 1.", Images: []Image{{PNG: testPNG}}},
		},
		Tools: []Tool{
			{Name: "look", Description: "Look at the page."},
			{Name: "finish", Description: "End the run.", Parameters: map[string]any{"type": "object",
				"properties": map[string]any{"verdict": map[string]any{"type": "string"}}, "required": []string{"verdict"}}},
		},
		MaxOutputTokens: 512,
	}
}

// TestContract is what every provider must pass.
func TestContract(t *testing.T) {
	for _, b := range backends() {
		t.Run(b.name, func(t *testing.T) {
			srv := newReplay(t)
			p := b.build(t, srv.URL)

			// Identity and capabilities.
			if p.Kind() != b.name || p.Model() == "" || p.Region() == "" {
				t.Errorf("identity: kind %q model %q region %q", p.Kind(), p.Model(), p.Region())
			}
			caps := CapabilitiesOf(p)
			if want, ok := CapabilitiesFor(p.Kind()); !ok || caps != want {
				t.Errorf("capabilities %+v are not the table entry %+v", caps, want)
			}
			if !caps.ToolCalling || !caps.Images {
				t.Error("every provider must support tool calling and images")
			}

			// A prose reply.
			srv.serve(t, 200, b.dir, "text.json")
			resp, err := p.Chat(context.Background(), contractRequest())
			must(t, err)
			if !strings.Contains(resp.Text, "Hello") || len(resp.ToolCalls) != 0 {
				t.Errorf("text reply decoded as %+v", resp)
			}
			if resp.Usage.InputTokens == 0 || resp.Usage.OutputTokens == 0 {
				t.Errorf("usage missing: %+v", resp.Usage)
			}

			// What was sent.
			got := srv.last(t)
			if !strings.Contains(got.path, b.path) {
				t.Errorf("request path %q lacks %q", got.path, b.path)
			}
			for _, want := range []string{"SYSMARK", "USERMARK", "ASSISTMARK", "TOOLMARK", "zzargs", `"finish"`, "verdict", "call_1"} {
				if want == "call_1" && p.Kind() != "openai-compatible" && p.Kind() != "anthropic" && p.Kind() != "bedrock" {
					continue // Gemini matches a reply to its call by name and order
				}
				if !strings.Contains(got.body, want) {
					t.Errorf("request body lacks %q", want)
				}
			}
			if !strings.Contains(got.body, base64.StdEncoding.EncodeToString(testPNG)) {
				t.Error("the image did not reach the request as base64")
			}
			if strings.Contains(got.body, testKey) {
				t.Error("the API key is in the request body")
			}
			if b.keyHeader != "" && !strings.Contains(got.header.Get(b.keyHeader), testKey) {
				t.Errorf("header %s does not carry the key", b.keyHeader)
			}

			// A tool call.
			srv.serve(t, 200, b.dir, "tool.json")
			resp, err = p.Chat(context.Background(), contractRequest())
			must(t, err)
			if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "look" || resp.ToolCalls[0].ID == "" {
				t.Fatalf("tool reply decoded as %+v", resp)
			}
			if !json.Valid(resp.ToolCalls[0].Args) {
				t.Errorf("tool arguments %q are not JSON", resp.ToolCalls[0].Args)
			}
			if resp.FinishReason == "" {
				t.Error("finish reason missing")
			}

			// A forced finish becomes an instruction where the backend cannot force a tool.
			force := contractRequest()
			force.ForceTool = "finish"
			force, _ = Adapt(caps, force)
			_, err = p.Chat(context.Background(), force)
			must(t, err)
			sent := srv.last(t).body
			if !caps.ForcedToolChoice && !strings.Contains(sent, "Call the finish tool now") {
				t.Error("the finish instruction was not sent")
			}
			if caps.ForcedToolChoice && strings.Contains(sent, "Call the finish tool now") {
				t.Error("a backend that can force a tool was given the instruction too")
			}

			// An error keeps its reason and never contains the key.
			srv.serve(t, 400, b.dir, "error.json")
			_, err = p.Chat(context.Background(), contractRequest())
			if err == nil {
				t.Fatal("a 400 reply gave no error")
			}
			if strings.Contains(err.Error(), testKey) {
				t.Error("the error message contains the API key")
			}
		})
	}
}
