package provider

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
)

func TestChatParamsShape(t *testing.T) {
	req := Request{
		System: "You are a persona.",
		Turns: []Turn{
			{Role: RoleUser, Text: "Begin.", Images: []Image{{PNG: []byte("\x89PNG")}}},
			{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call_1", Name: "click", Args: json.RawMessage(`{"target":"@1"}`)}}},
			{Role: RoleTool, ToolCallID: "call_1", Text: "step 1: clicked"},
		},
		Tools:           []Tool{{Name: "click", Description: "Click", Parameters: map[string]any{"type": "object", "properties": map[string]any{"target": map[string]any{"type": "string"}}, "required": []string{"target"}}}, {Name: "finish", Description: "End"}},
		MaxOutputTokens: 2048,
	}
	data, err := json.Marshal(chatParams("gpt-test", req))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "gpt-test" || got["max_completion_tokens"] != float64(2048) {
		t.Errorf("model or max tokens wrong: %v", got)
	}
	msgs := got["messages"].([]any)
	if len(msgs) != 4 {
		t.Fatalf("%d messages", len(msgs))
	}
	if msgs[0].(map[string]any)["role"] != "system" {
		t.Error("first message is not the system prompt")
	}
	user := msgs[1].(map[string]any)
	parts := user["content"].([]any)
	if len(parts) != 2 || parts[1].(map[string]any)["type"] != "image_url" {
		t.Errorf("user content %v", parts)
	}
	if !strings.HasPrefix(parts[1].(map[string]any)["image_url"].(map[string]any)["url"].(string), "data:image/png;base64,") {
		t.Error("image is not a PNG data URL")
	}
	assistant := msgs[2].(map[string]any)
	calls := assistant["tool_calls"].([]any)
	if len(calls) != 1 || calls[0].(map[string]any)["id"] != "call_1" {
		t.Errorf("assistant tool calls %v", calls)
	}
	tool := msgs[3].(map[string]any)
	if tool["role"] != "tool" || tool["tool_call_id"] != "call_1" {
		t.Errorf("tool message %v", tool)
	}
	tools := got["tools"].([]any)
	if len(tools) != 2 || tools[0].(map[string]any)["type"] != "function" {
		t.Errorf("tools %v", tools)
	}
	if got["tool_choice"] != "required" {
		t.Errorf("tool_choice %v, want required", got["tool_choice"])
	}
	if _, ok := got["temperature"]; ok {
		t.Error("temperature must not be set; reasoning deployments reject it")
	}
}

func TestChatParamsForcedTool(t *testing.T) {
	req := Request{Turns: []Turn{{Role: RoleUser, Text: "x"}}, Tools: []Tool{{Name: "finish"}}, ForceTool: "finish"}
	data, _ := json.Marshal(chatParams("m", req))
	var got map[string]any
	_ = json.Unmarshal(data, &got)
	tc, ok := got["tool_choice"].(map[string]any)
	if !ok || tc["type"] != "function" || tc["function"].(map[string]any)["name"] != "finish" {
		t.Errorf("tool_choice %v", got["tool_choice"])
	}
}

func TestFromCompletion(t *testing.T) {
	var c openai.ChatCompletion
	raw := `{"id":"x","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":"",
	  "tool_calls":[{"id":"call_9","type":"function","function":{"name":"look","arguments":"{}"}}]}}],
	  "usage":{"prompt_tokens":120,"completion_tokens":15,"total_tokens":135}}`
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatal(err)
	}
	resp, err := fromCompletion(&c)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Usage.InputTokens != 120 || resp.Usage.OutputTokens != 15 || resp.FinishReason != "tool_calls" {
		t.Errorf("response %+v", resp)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "look" || resp.ToolCalls[0].ID != "call_9" {
		t.Errorf("tool calls %+v", resp.ToolCalls)
	}
	var empty openai.ChatCompletion
	_ = json.Unmarshal([]byte(`{"id":"y","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":0}}`), &empty)
	if _, err := fromCompletion(&empty); !errors.Is(err, ErrNoChoice) {
		t.Errorf("empty reply gave %v", err)
	}
}

func TestNewRejectsUnknownProviders(t *testing.T) {
	if _, err := New(Config{Provider: "nope"}); err == nil {
		t.Error("unknown provider accepted")
	}
	if _, err := NewAzure(Config{Model: "m", Region: "r"}); err == nil {
		t.Error("azure without an endpoint accepted")
	}
	if _, err := NewAzure(Config{Endpoint: "https://x.openai.azure.com", Region: "r"}); err == nil {
		t.Error("azure without a model accepted")
	}
	if _, err := NewAzure(Config{Endpoint: "https://x.openai.azure.com", Model: "m"}); err == nil {
		t.Error("azure without a region accepted")
	}
}
