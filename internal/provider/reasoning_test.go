package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestReasoningEffortOnChatCompletions(t *testing.T) {
	srv := newReplay(t)
	srv.serve(t, 200, "openai-compatible", "tool.json")
	p, err := NewOpenAICompatible(Config{Endpoint: srv.URL + "/v1", Model: "m", Region: "local", ReasoningEffort: "none"})
	must(t, err)
	resp, err := p.Chat(context.Background(), contractRequest())
	must(t, err)
	var body map[string]any
	must(t, json.Unmarshal([]byte(srv.last(t).body), &body))
	if body["reasoning_effort"] != "none" {
		t.Errorf("reasoning_effort %v", body["reasoning_effort"])
	}
	if resp.Usage.CachedInputTokens != 0 {
		t.Errorf("cached tokens %d in the recorded fixture", resp.Usage.CachedInputTokens)
	}

	// Without the option nothing is sent.
	q, err := NewOpenAICompatible(Config{Endpoint: srv.URL + "/v1", Model: "m", Region: "local"})
	must(t, err)
	_, err = q.Chat(context.Background(), contractRequest())
	must(t, err)
	if strings.Contains(srv.last(t).body, "reasoning_effort") {
		t.Error("reasoning_effort sent when not asked for")
	}
}

func TestResponsesAPI(t *testing.T) {
	srv := newReplay(t)
	srv.serve(t, 200, "openai-compatible", "responses_tool.json")
	p, err := NewOpenAICompatible(Config{Endpoint: srv.URL + "/v1", Model: "m", Region: "local", API: APIResponses, ReasoningEffort: "low"})
	must(t, err)
	req := contractRequest()
	req.ForceTool = "finish"
	resp, err := p.Chat(context.Background(), req)
	must(t, err)
	got := srv.last(t)
	if !strings.HasSuffix(got.path, "/v1/responses") {
		t.Errorf("path %s", got.path)
	}
	var body map[string]any
	must(t, json.Unmarshal([]byte(got.body), &body))
	if body["store"] != false || body["instructions"] == nil || body["max_output_tokens"] != float64(512) {
		t.Errorf("body %v", body)
	}
	if body["reasoning"].(map[string]any)["effort"] != "low" {
		t.Errorf("reasoning %v", body["reasoning"])
	}
	if tc := body["tool_choice"].(map[string]any); tc["name"] != "finish" {
		t.Errorf("tool_choice %v", tc)
	}
	for _, want := range []string{"function_call_output", "input_image", "call_1", "TOOLMARK", "zzargs"} {
		if !strings.Contains(got.body, want) {
			t.Errorf("request lacks %q", want)
		}
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID != "call_resp_1" || resp.ToolCalls[0].Name != "look" {
		t.Errorf("tool calls %+v", resp.ToolCalls)
	}
	if resp.Usage.InputTokens != 200 || resp.Usage.CachedInputTokens != 128 || resp.Usage.OutputTokens != 12 {
		t.Errorf("usage %+v", resp.Usage)
	}

	srv.serve(t, 200, "openai-compatible", "responses_text.json")
	resp, err = p.Chat(context.Background(), contractRequest())
	must(t, err)
	if resp.Text != "Hello there, friend." {
		t.Errorf("text %q", resp.Text)
	}
	if strings.Contains(srv.last(t).body, `"tool_choice":{`) {
		t.Error("a tool was forced without being asked")
	}
}

func TestCachedTokensDecode(t *testing.T) {
	r := decodeCompletion(t, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"x"}}],
	  "usage":{"prompt_tokens":2000,"prompt_tokens_details":{"cached_tokens":1536},"completion_tokens":5}}`)
	if r.Usage.InputTokens != 2000 || r.Usage.CachedInputTokens != 1536 {
		t.Errorf("usage %+v", r.Usage)
	}
	a := decodeAnthropic(t, `{"content":[{"type":"text","text":"x"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"cache_read_input_tokens":90,"output_tokens":1}}`)
	if a.Usage.InputTokens != 100 || a.Usage.CachedInputTokens != 90 {
		t.Errorf("anthropic usage %+v", a.Usage)
	}
}

func TestBadAPIOrEffortRejected(t *testing.T) {
	base := Config{Endpoint: "http://localhost:1/v1", Model: "m", Region: "local"}
	bad := base
	bad.API = "legacy"
	if _, err := NewOpenAICompatible(bad); err == nil {
		t.Error("unknown API accepted")
	}
	bad = base
	bad.ReasoningEffort = "extreme"
	if _, err := NewOpenAICompatible(bad); err == nil {
		t.Error("unknown effort accepted")
	}
}

func TestCapabilityTableAndAdapt(t *testing.T) {
	for kind, c := range capabilityTable {
		if !c.ToolCalling || !c.Images {
			t.Errorf("%s must support tool calling and images", kind)
		}
	}
	req := Request{Turns: []Turn{{Role: RoleUser, Text: "x"}}, ForceTool: "finish"}
	got, note := Adapt(Capabilities{}, req)
	if got.ForceTool != "" || note == "" || len(got.Turns) != 2 || !strings.Contains(got.Turns[1].Text, "finish") {
		t.Errorf("adapted %+v note %q", got, note)
	}
	if len(req.Turns) != 1 || req.ForceTool != "finish" {
		t.Error("Adapt changed the caller's request")
	}
	same, note := Adapt(Capabilities{ForcedToolChoice: true}, req)
	if same.ForceTool != "finish" || note != "" || len(same.Turns) != 1 {
		t.Errorf("a backend that can force a tool was adapted: %+v", same)
	}
	if c := CapabilitiesOf(nil); !c.ForcedToolChoice {
		t.Error("the default for a provider that does not say should keep the forced tool")
	}
}

func TestResponsesKeepsEncryptedReasoningBetweenCalls(t *testing.T) {
	srv := newReplay(t)
	srv.serve(t, 200, "openai-compatible", "responses_reasoning_tool.json")
	p, err := NewOpenAICompatible(Config{Endpoint: srv.URL + "/v1", Model: "m", Region: "local", API: APIResponses, ReasoningEffort: "medium"})
	must(t, err)
	resp, err := p.Chat(context.Background(), contractRequest())
	must(t, err)
	if !strings.Contains(srv.last(t).body, "reasoning.encrypted_content") {
		t.Error("encrypted reasoning was not requested")
	}
	if len(resp.ToolCalls) != 1 || len(resp.ToolCalls[0].State) == 0 {
		t.Fatalf("reasoning was not kept: %+v", resp.ToolCalls)
	}
	// The next request carries it back, before the function call.
	next := contractRequest()
	next.Turns[1].ToolCalls[0].State = resp.ToolCalls[0].State
	_, err = p.Chat(context.Background(), next)
	must(t, err)
	body := srv.last(t).body
	if !strings.Contains(body, "ENCRYPTEDBLOB") || strings.Index(body, "ENCRYPTEDBLOB") > strings.Index(body, `"function_call"`) {
		t.Errorf("reasoning not sent back before its call: %s", body)
	}

	// With effort none nothing is requested or kept.
	q, err := NewOpenAICompatible(Config{Endpoint: srv.URL + "/v1", Model: "m", Region: "local", API: APIResponses, ReasoningEffort: "none"})
	must(t, err)
	_, err = q.Chat(context.Background(), contractRequest())
	must(t, err)
	if strings.Contains(srv.last(t).body, "encrypted_content") {
		t.Error("encrypted reasoning requested with effort none")
	}
}
