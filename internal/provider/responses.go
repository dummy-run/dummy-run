package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

// The OpenAI API choices for Config.API.
const (
	// APIChat is the chat completions API, the default.
	APIChat = "chat"
	// APIResponses is the Responses API, which some models need for tool calls.
	APIResponses = "responses"
)

// checkAPI validates Config.API and Config.ReasoningEffort.
func checkAPI(cfg Config) error {
	switch cfg.API {
	case "", APIChat, APIResponses:
	default:
		return fmt.Errorf("provider: unknown API %q (%s or %s)", cfg.API, APIChat, APIResponses)
	}
	switch cfg.ReasoningEffort {
	case "", "none", "minimal", "low", "medium", "high", "xhigh":
	default:
		return fmt.Errorf("provider: unknown reasoning effort %q (none, minimal, low, medium, high or xhigh)", cfg.ReasoningEffort)
	}
	return nil
}

// responsesParams maps a Request onto the Responses API. The conversation is
// sent whole on every call and is not stored by the service (store is false),
// so nothing outlives the call.
func responsesParams(model string, req Request, choice, effort string) responses.ResponseNewParams {
	var items responses.ResponseInputParam
	for _, t := range req.Turns {
		switch t.Role {
		case RoleUser:
			var parts responses.ResponseInputMessageContentListParam
			if t.Text != "" {
				parts = append(parts, responses.ResponseInputContentParamOfInputText(t.Text))
			}
			for _, img := range t.Images {
				parts = append(parts, responses.ResponseInputContentUnionParam{OfInputImage: &responses.ResponseInputImageParam{
					Detail:   responses.ResponseInputImageDetailAuto,
					ImageURL: openai.String("data:image/png;base64," + base64.StdEncoding.EncodeToString(img.PNG)),
				}})
			}
			if len(parts) > 0 {
				items = append(items, responses.ResponseInputItemParamOfMessage(parts, responses.EasyInputMessageRoleUser))
			}
		case RoleAssistant:
			if t.Text != "" {
				items = append(items, responses.ResponseInputItemParamOfMessage(t.Text, responses.EasyInputMessageRoleAssistant))
			}
			for i, c := range t.ToolCalls {
				if i == 0 {
					items = append(items, reasoningItems(c.State)...)
				}
				items = append(items, responses.ResponseInputItemParamOfFunctionCall(string(normaliseArgs(string(c.Args))), c.ID, c.Name))
			}
		case RoleTool:
			item := responses.ResponseInputItemParamOfFunctionCallOutput(orText(t.Text, "(no output)"))
			item.OfFunctionCallOutput.CallID = openai.String(t.ToolCallID)
			items = append(items, item)
		}
	}
	params := responses.ResponseNewParams{
		Model: model, Store: openai.Bool(false), ParallelToolCalls: openai.Bool(false),
		Input: responses.ResponseNewParamsInputUnion{OfInputItemList: items},
	}
	if req.System != "" {
		params.Instructions = openai.String(req.System)
	}
	if req.MaxOutputTokens > 0 {
		params.MaxOutputTokens = openai.Int(int64(req.MaxOutputTokens))
	}
	if effort != "" {
		params.Reasoning = shared.ReasoningParam{Effort: shared.ReasoningEffort(effort)}
	}
	if keepsReasoning(effort) {
		// The service stores nothing, so the model's reasoning comes back encrypted and
		// goes into the next request next to the tool call it belongs to.
		params.Include = []responses.ResponseIncludable{responses.ResponseIncludableReasoningEncryptedContent}
	}
	for _, tool := range req.Tools {
		params.Tools = append(params.Tools, responses.ToolUnionParam{OfFunction: &responses.FunctionToolParam{
			Name: tool.Name, Description: openai.String(tool.Description), Parameters: schemaOf(tool)}})
	}
	if len(req.Tools) > 0 {
		if req.ForceTool != "" {
			params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{OfFunctionTool: &responses.ToolChoiceFunctionParam{Name: req.ForceTool}}
		} else {
			params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{OfToolChoiceMode: openai.Opt(responses.ToolChoiceOptions(choice))}
		}
	}
	return params
}

func fromResponse(r *responses.Response) (Response, error) {
	if r == nil {
		return Response{}, ErrNoChoice
	}
	resp := Response{FinishReason: string(r.Status), Usage: Usage{
		InputTokens: int(r.Usage.InputTokens), OutputTokens: int(r.Usage.OutputTokens),
		CachedInputTokens: int(r.Usage.InputTokensDetails.CachedTokens)}}
	if r.Status == responses.ResponseStatusIncomplete && r.IncompleteDetails.Reason != "" {
		resp.FinishReason = "incomplete: " + r.IncompleteDetails.Reason
	}
	if r.Error.Message != "" {
		return resp, fmt.Errorf("provider: the model reported an error: %s %s", r.Error.Code, r.Error.Message)
	}
	if len(r.Output) == 0 {
		return resp, ErrNoChoice
	}
	var reasoning []json.RawMessage
	for _, item := range r.Output {
		switch item.Type {
		case "reasoning":
			// Only reasoning the service sent encrypted can be sent back without stored state.
			var probe struct {
				Encrypted string `json:"encrypted_content"`
			}
			if json.Unmarshal([]byte(item.RawJSON()), &probe) == nil && probe.Encrypted != "" {
				reasoning = append(reasoning, json.RawMessage(item.RawJSON()))
			}
		case "message":
			for _, c := range item.Content {
				if c.Type == "output_text" {
					resp.Text += c.Text
				}
			}
		case "function_call":
			id := item.CallID
			if id == "" {
				id = fmt.Sprintf("call_%d", len(resp.ToolCalls)+1)
			}
			resp.ToolCalls = append(resp.ToolCalls, ToolCall{ID: id, Name: item.Name, Args: normaliseArgs(item.Arguments.OfString)})
		}
	}
	if len(reasoning) > 0 && len(resp.ToolCalls) > 0 {
		resp.ToolCalls[0].State = encodeArgs(reasoning)
	}
	return resp, nil
}

// keepsReasoning says whether encrypted reasoning is requested and passed back.
func keepsReasoning(effort string) bool { return effort != "" && effort != "none" }

// reasoningItems decodes the reasoning items kept in a tool call's State.
func reasoningItems(state []byte) []responses.ResponseInputItemUnionParam {
	var raws []json.RawMessage
	if len(state) == 0 || json.Unmarshal(state, &raws) != nil {
		return nil
	}
	var items []responses.ResponseInputItemUnionParam
	for _, raw := range raws {
		var item responses.ResponseInputItemUnionParam
		if json.Unmarshal(raw, &item) == nil {
			items = append(items, item)
		}
	}
	return items
}

// callResponses makes one Responses API call and wraps any error under name.
func callResponses(ctx context.Context, client *openai.Client, name string, params responses.ResponseNewParams) (Response, error) {
	out, err := client.Responses.New(ctx, params)
	if err != nil {
		var apiErr *openai.Error
		if errors.As(err, &apiErr) && apiErr.Message != "" {
			return Response{}, fmt.Errorf("provider: %s: %d %s: %s (%w)", name, apiErr.StatusCode, apiErr.Code, apiErr.Message, err)
		}
		return Response{}, fmt.Errorf("provider: %s: %w", name, err)
	}
	return fromResponse(out)
}
