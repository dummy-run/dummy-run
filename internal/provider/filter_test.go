package provider

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
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

func TestFromCompletionRefusalField(t *testing.T) {
	var c openai.ChatCompletion
	body := `{"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":null,"refusal":"I cannot help with that."}}],"usage":{"prompt_tokens":5,"completion_tokens":3}}`
	if err := json.Unmarshal([]byte(body), &c); err != nil {
		t.Fatal(err)
	}
	resp, err := fromCompletion(&c)
	if !errors.Is(err, ErrContentFiltered) {
		t.Fatalf("err = %v", err)
	}
	if resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 3 {
		t.Errorf("usage %+v", resp.Usage)
	}
}

func TestFromResponseRefusalPart(t *testing.T) {
	var r responses.Response
	body := `{"status":"completed","output":[{"type":"message","id":"m","role":"assistant","status":"completed","content":[{"type":"refusal","refusal":"No."}]}],"usage":{"input_tokens":7,"output_tokens":2}}`
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatal(err)
	}
	resp, err := fromResponse(&r)
	if !errors.Is(err, ErrContentFiltered) {
		t.Fatalf("err = %v", err)
	}
	if resp.Usage.InputTokens != 7 {
		t.Errorf("usage %+v", resp.Usage)
	}
}

func TestFromBedrockKeepsUsageWhenFiltered(t *testing.T) {
	in, out := int32(11), int32(4)
	resp, err := fromBedrock(&bedrockruntime.ConverseOutput{StopReason: types.StopReasonGuardrailIntervened,
		Usage: &types.TokenUsage{InputTokens: &in, OutputTokens: &out}})
	if !errors.Is(err, ErrContentFiltered) {
		t.Fatalf("err = %v", err)
	}
	if resp.Usage.InputTokens != 11 || resp.Usage.OutputTokens != 4 {
		t.Errorf("usage %+v", resp.Usage)
	}
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
