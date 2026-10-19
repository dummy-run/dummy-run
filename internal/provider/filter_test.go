package provider

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/openai/openai-go/v3"
)

func TestFilterReasons(t *testing.T) {
	for _, s := range []string{"content_filter", "refusal", "SAFETY", "incomplete: content_filter", "guardrail_intervened", "content_filtered", "PROHIBITED_CONTENT"} {
		if !isFilterReason(s) {
			t.Errorf("%q not treated as a content filter reason", s)
		}
	}
	for _, s := range []string{"stop", "tool_calls", "length", "max_tokens", "end_turn", "STOP", "", "other"} {
		if isFilterReason(s) {
			t.Errorf("%q treated as a content filter reason", s)
		}
	}
	if !isFilterAPIError("", "The response was filtered due to the prompt triggering Azure OpenAI's content management policy.") {
		t.Error("Azure message not recognised")
	}
	if isFilterAPIError("rate_limit", "slow down") {
		t.Error("rate limit treated as a filter")
	}
}

func completionWithFinish(reason string) *openai.ChatCompletion {
	var c openai.ChatCompletion
	body := `{"choices":[{"index":0,"finish_reason":"` + reason + `","message":{"role":"assistant","content":""}}],"usage":{"prompt_tokens":5,"completion_tokens":0}}`
	if err := json.Unmarshal([]byte(body), &c); err != nil {
		panic(err)
	}
	return &c
}

func TestFromCompletionFiltered(t *testing.T) {
	_, err := fromCompletion(completionWithFinish("content_filter"))
	if !errors.Is(err, ErrContentFiltered) {
		t.Fatalf("err = %v", err)
	}
	if _, err := fromCompletion(completionWithFinish("stop")); err != nil {
		t.Fatal(err)
	}
}
