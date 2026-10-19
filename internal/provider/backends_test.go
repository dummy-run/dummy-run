package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"google.golang.org/genai"
)

func jsonOf(t *testing.T, v any) map[string]any {
	t.Helper()
	data, err := json.Marshal(v)
	must(t, err)
	var out map[string]any
	must(t, json.Unmarshal(data, &out))
	return out
}

func TestOpenAICompatibleKeyHandling(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "ambient-key-must-not-be-sent")
	t.Setenv("OPENAI_ORG_ID", "org-ambient")
	srv := newReplay(t)
	srv.serve(t, 200, "openai-compatible", "text.json")

	// No variable named: no Authorization header, whatever the environment holds.
	local, err := NewOpenAICompatible(Config{Endpoint: srv.URL + "/v1", Model: "m", Region: "local"})
	must(t, err)
	_, err = local.Chat(context.Background(), contractRequest())
	must(t, err)
	h := srv.last(t).header
	if h.Get("Authorization") != "" || h.Get("OpenAI-Organi"+"zation") != "" {
		t.Errorf("ambient OpenAI settings were sent to another server: %v", h)
	}

	// A named variable is read at run time and used exactly.
	t.Setenv("MY_VENDOR_KEY", "vendor-key")
	hosted, err := NewOpenAICompatible(Config{Endpoint: srv.URL + "/v1/", Model: "m", Region: "eu-fr-par", APIKeyEnv: "MY_VENDOR_KEY"})
	must(t, err)
	_, err = hosted.Chat(context.Background(), contractRequest())
	must(t, err)
	if got := srv.last(t).header.Get("Authorization"); got != "Bearer vendor-key" {
		t.Errorf("Authorization %q", got)
	}

	// A named variable that is empty is an error that names the variable, not a value.
	_, err = NewOpenAICompatible(Config{Endpoint: srv.URL, Model: "m", Region: "x", APIKeyEnv: "NOT_SET_ANYWHERE_123"})
	if err == nil || !strings.Contains(err.Error(), "NOT_SET_ANYWHERE_123") {
		t.Errorf("unset key variable gave %v", err)
	}
}

func TestOpenAICompatibleRequestShape(t *testing.T) {
	srv := newReplay(t)
	srv.serve(t, 200, "openai-compatible", "tool.json")
	p, err := NewOpenAICompatible(Config{Endpoint: srv.URL + "/v1", Model: "m", Region: "local"})
	must(t, err)
	req := contractRequest()
	req.ForceTool = "finish"
	req, _ = Adapt(CapabilitiesOf(p), req)
	_, err = p.Chat(context.Background(), req)
	must(t, err)
	var body map[string]any
	must(t, json.Unmarshal([]byte(srv.last(t).body), &body))
	if body["tool_choice"] != "auto" {
		t.Errorf("tool_choice %v, want auto", body["tool_choice"])
	}
	if body["max_tokens"] != float64(512) {
		t.Errorf("max_tokens %v: compatible servers need max_tokens", body["max_tokens"])
	}
	if _, ok := body["max_completion_tokens"]; ok {
		t.Error("max_completion_tokens sent to a non-OpenAI host")
	}

	openAI, err := NewOpenAICompatible(Config{Endpoint: "https://api.openai.com/v1", Model: "m", Region: "global"})
	must(t, err)
	if openAI.legacyTokens {
		t.Error("api.openai.com should get max_completion_tokens")
	}
}

// A tool-only assistant turn has no text. Ollama rejects a missing content field with a 400,
// so the compatible provider sends an empty string. The Azure provider is left as it was.
func TestOpenAICompatibleSendsEmptyContentForToolOnlyTurns(t *testing.T) {
	srv := newReplay(t)
	srv.serve(t, 200, "openai-compatible", "tool.json")
	p, err := NewOpenAICompatible(Config{Endpoint: srv.URL + "/v1", Model: "m", Region: "local"})
	must(t, err)
	req := contractRequest()
	req.Turns[1].Text = ""
	_, err = p.Chat(context.Background(), req)
	must(t, err)
	var body struct {
		Messages []map[string]any `json:"messages"`
	}
	must(t, json.Unmarshal([]byte(srv.last(t).body), &body))
	found := false
	for _, m := range body.Messages {
		if m["role"] != "assistant" {
			continue
		}
		found = true
		if c, ok := m["content"].(string); !ok || c != "" {
			t.Errorf("assistant content %#v, want an empty string", m["content"])
		}
		if _, ok := m["tool_calls"]; !ok {
			t.Error("tool_calls missing")
		}
	}
	if !found {
		t.Fatal("no assistant message was sent")
	}
}

func TestOpenAICompatibleRejectsBadConfig(t *testing.T) {
	for name, cfg := range map[string]Config{
		"no endpoint":  {Model: "m", Region: "r"},
		"bad endpoint": {Endpoint: "localhost:11434", Model: "m", Region: "r"},
		"no model":     {Endpoint: "http://localhost:11434/v1", Region: "r"},
		"no region":    {Endpoint: "http://localhost:11434/v1", Model: "m"},
	} {
		if _, err := NewOpenAICompatible(cfg); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestFromCompletionFillsMissingIDsAndArgs(t *testing.T) {
	// Some local servers send no call ID and an empty argument string.
	resp := decodeCompletion(t, `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":"",
	  "tool_calls":[{"type":"function","function":{"name":"look","arguments":""}}]}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID == "" || string(resp.ToolCalls[0].Args) != "{}" {
		t.Errorf("tool calls %+v", resp.ToolCalls)
	}
}

func TestAnthropicParams(t *testing.T) {
	req := contractRequest()
	req.ForceTool = "finish" // never reaches the API: forced use returns 400
	req.Turns = append(req.Turns[:2:2],
		Turn{Role: RoleAssistant}, // an empty assistant turn must be dropped
		Turn{Role: RoleTool, ToolCallID: "call_1", Text: "TOOLMARK", Images: []Image{{PNG: testPNG}}},
		Turn{Role: RoleUser, Text: "Use one of the tools."})
	got := jsonOf(t, anthropicParams("claude-test", req))

	choice := got["tool_choice"].(map[string]any)
	if choice["type"] != "auto" || choice["disable_parallel_tool_use"] != true {
		t.Errorf("tool_choice %v", choice)
	}
	msgs := got["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("%d messages, want user, assistant, user: %v", len(msgs), msgs)
	}
	roles := []string{"user", "assistant", "user"}
	for i, m := range msgs {
		if m.(map[string]any)["role"] != roles[i] {
			t.Errorf("message %d role %v", i, m.(map[string]any)["role"])
		}
	}
	last := msgs[2].(map[string]any)["content"].([]any)
	if len(last) != 2 || last[0].(map[string]any)["type"] != "tool_result" {
		t.Fatalf("the tool result must come first in the user message: %v", last)
	}
	inner := last[0].(map[string]any)["content"].([]any)
	if len(inner) != 2 || inner[1].(map[string]any)["type"] != "image" {
		t.Errorf("the screenshot did not travel inside the tool result: %v", inner)
	}
	tools := got["tools"].([]any)
	look := tools[0].(map[string]any)["input_schema"].(map[string]any)
	if look["type"] != "object" {
		t.Errorf("a tool without parameters needs an object schema: %v", look)
	}
	finish := tools[1].(map[string]any)["input_schema"].(map[string]any)
	if r, _ := finish["required"].([]any); len(r) != 1 || r[0] != "verdict" {
		t.Errorf("required lost: %v", finish)
	}
	if got["max_tokens"] != float64(512) {
		t.Errorf("max_tokens %v", got["max_tokens"])
	}
	if sys := got["system"].([]any); len(sys) != 1 {
		t.Errorf("system %v", sys)
	}
}

func TestAnthropicNeedsAKey(t *testing.T) {
	t.Setenv(DefaultAnthropicKeyEnv, "")
	_, err := NewAnthropic(Config{Model: "claude-test"})
	if err == nil || !strings.Contains(err.Error(), DefaultAnthropicKeyEnv) {
		t.Errorf("missing key gave %v", err)
	}
	if _, err := NewAnthropic(Config{}); err == nil {
		t.Error("missing model accepted")
	}
}

func TestAnthropicDecodeKeepsToolInputValid(t *testing.T) {
	var m = decodeAnthropic(t, `{"content":[{"type":"tool_use","id":"","name":"click","input":null}],"stop_reason":"tool_use","usage":{"input_tokens":3,"output_tokens":1,"cache_read_input_tokens":10}}`)
	if len(m.ToolCalls) != 1 || m.ToolCalls[0].ID == "" || string(m.ToolCalls[0].Args) != "{}" {
		t.Errorf("tool calls %+v", m.ToolCalls)
	}
	if m.Usage.InputTokens != 13 {
		t.Errorf("cached input tokens not counted: %+v", m.Usage)
	}
}

func TestGeminiParams(t *testing.T) {
	req := contractRequest()
	req.ForceTool = "finish"
	req.Turns[1].ToolCalls[0].State = []byte("sig")
	req.Turns = append(req.Turns[:3:3],
		Turn{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call_1", Name: "click", Args: json.RawMessage(`{}`)}, {ID: "call_2", Name: "look", Args: json.RawMessage(`{}`)}}},
		Turn{Role: RoleTool, ToolCallID: "call_1", Text: "first"},
		Turn{Role: RoleTool, ToolCallID: "call_2", Text: "second"})
	contents, cfg := geminiParams(req)
	got := jsonOf(t, map[string]any{"contents": contents, "config": cfg})

	cs := got["contents"].([]any)
	var roles []string
	for _, c := range cs {
		roles = append(roles, c.(map[string]any)["role"].(string))
	}
	if strings.Join(roles, ",") != "user,model,user,model,user" {
		t.Errorf("roles %v", roles)
	}
	firstCall := cs[1].(map[string]any)["parts"].([]any)[1].(map[string]any)
	if firstCall["thoughtSignature"] == nil {
		t.Errorf("the thought signature was not sent back: %v", firstCall)
	}
	replies := cs[4].(map[string]any)["parts"].([]any)
	if len(replies) != 2 {
		t.Fatalf("replies to parallel calls must share a content: %v", replies)
	}
	if name := replies[0].(map[string]any)["functionResponse"].(map[string]any)["name"]; name != "click" {
		t.Errorf("reply named %v, want click", name)
	}
	fc := got["config"].(map[string]any)["toolConfig"].(map[string]any)["functionCallingConfig"].(map[string]any)
	if fc["mode"] != "ANY" || fc["allowedFunctionNames"].([]any)[0] != "finish" {
		t.Errorf("forced tool choice %v", fc)
	}
	if cfg.MaxOutputTokens != 512 {
		t.Errorf("max output tokens %d", cfg.MaxOutputTokens)
	}
}

func TestGeminiDecodeKeepsSignatureAndSkipsThoughts(t *testing.T) {
	var r genai.GenerateContentResponse
	must(t, json.Unmarshal([]byte(`{"candidates":[{"content":{"role":"model","parts":[
	  {"text":"private reasoning","thought":true},{"text":"visible"},
	  {"functionCall":{"name":"look","args":{"a":1}},"thoughtSignature":"c2ln"}]},"finishReason":"STOP"}]}`), &r))
	resp, err := fromGemini(&r)
	must(t, err)
	if resp.Text != "visible" {
		t.Errorf("text %q", resp.Text)
	}
	if len(resp.ToolCalls) != 1 || string(resp.ToolCalls[0].State) != "sig" || string(resp.ToolCalls[0].Args) != `{"a":1}` {
		t.Errorf("tool calls %+v", resp.ToolCalls)
	}
	var blocked genai.GenerateContentResponse
	must(t, json.Unmarshal([]byte(`{"promptFeedback":{"blockReason":"SAFETY"}}`), &blocked))
	if _, err := fromGemini(&blocked); !errors.Is(err, ErrNoChoice) || !strings.Contains(err.Error(), "SAFETY") {
		t.Errorf("blocked prompt gave %v", err)
	}
}

func TestGeminiConfigErrors(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv(DefaultGeminiKeyEnv, "")
	for name, cfg := range map[string]Config{
		"vertex without a project": {Provider: NameVertex, Model: "m", Region: "europe-west2"},
		"vertex without a region":  {Provider: NameVertex, Model: "m", Project: "p"},
		"gemini without a key":     {Provider: NameGemini, Model: "m"},
		"no model":                 {Provider: NameGemini},
	} {
		if _, err := NewGemini(cfg); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestVertexRecordsItsRegion(t *testing.T) {
	srv := newReplay(t)
	p, err := newGemini(Config{Provider: NameVertex, Model: "gemini-test", Project: "test-project", Region: "europe-west2"},
		func(c *genai.ClientConfig) { c.HTTPOptions.BaseURL = srv.URL; c.HTTPClient = http.DefaultClient })
	must(t, err)
	if p.Kind() != "vertex-gemini" || p.Region() != "europe-west2" {
		t.Errorf("kind %s region %s", p.Kind(), p.Region())
	}
}

func TestBedrockParamsAndDefaults(t *testing.T) {
	req := contractRequest()
	req.ForceTool = "finish"
	req.Turns = append(req.Turns[:3:3],
		Turn{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "c2", Name: "look", Args: json.RawMessage(`{}`)}}},
		Turn{Role: RoleTool, ToolCallID: "c2", Text: "second"})
	in := bedrockInput("m", req)
	if len(in.Messages) != 5 {
		t.Errorf("%d messages, want user, assistant, user, assistant, user", len(in.Messages))
	}
	if in.ToolConfig == nil || len(in.ToolConfig.Tools) != 2 {
		t.Fatalf("tool config %+v", in.ToolConfig)
	}
	got := jsonOf(t, in.ToolConfig.ToolChoice)
	if len(got) != 0 && got["tool"] != nil {
		t.Errorf("a tool was forced: %v", got)
	}
	if *in.InferenceConfig.MaxTokens != 512 {
		t.Errorf("max tokens %d", *in.InferenceConfig.MaxTokens)
	}

	t.Setenv("AWS_CONFIG_FILE", "/dev/null")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/dev/null")
	b, err := NewBedrock(Config{Model: "m"})
	must(t, err)
	if b.Region() != "eu-west-2" {
		t.Errorf("default region %q, want London", b.Region())
	}
	if _, err := NewBedrock(Config{}); err == nil {
		t.Error("missing model accepted")
	}
}

func TestNewDispatch(t *testing.T) {
	if _, err := New(Config{Provider: "nope"}); err == nil || !strings.Contains(err.Error(), "openai-compatible") {
		t.Errorf("unknown provider gave %v", err)
	}
	for _, name := range Names {
		if _, err := New(Config{Provider: name}); err == nil {
			t.Errorf("%s with an empty config was accepted", name)
		}
	}
}

func decodeCompletion(t *testing.T, raw string) Response {
	t.Helper()
	var c openai.ChatCompletion
	must(t, json.Unmarshal([]byte(raw), &c))
	resp, err := fromCompletion(&c)
	must(t, err)
	return resp
}

func decodeAnthropic(t *testing.T, raw string) Response {
	t.Helper()
	var m anthropic.Message
	must(t, json.Unmarshal([]byte(raw), &m))
	return fromAnthropic(&m)
}
