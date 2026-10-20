package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// DefaultAnthropicKeyEnv is the variable the Anthropic provider reads the API
// key from when --api-key-env is not given.
const DefaultAnthropicKeyEnv = "ANTHROPIC_API_KEY"

// Anthropic is the Messages API through the official Go SDK. Tool choice is
// always auto: forced tool use returns 400 on current models, so the finish
// step is an instruction (see Adapt) and the runner asks again if the model
// answers in prose.
type Anthropic struct {
	client anthropic.Client
	model  string
	region string
}

// NewAnthropic builds the provider. The key is read from cfg.APIKeyEnv, or
// ANTHROPIC_API_KEY when that is empty.
func NewAnthropic(cfg Config) (*Anthropic, error) {
	if cfg.Model == "" {
		return nil, errors.New("provider: anthropic needs a model, such as claude-sonnet-5-5")
	}
	env := cfg.APIKeyEnv
	if env == "" {
		env = DefaultAnthropicKeyEnv
	}
	key, err := keyFromEnv(env)
	if err != nil {
		return nil, err
	}
	opts := []option.RequestOption{option.WithAPIKey(key), option.WithRequestTimeout(RequestTimeout), option.WithMaxRetries(3)}
	if cfg.Endpoint != "" {
		opts = append(opts, option.WithBaseURL(cfg.Endpoint))
	}
	region := cfg.Region
	if region == "" {
		region = capabilityTable["anthropic"].DefaultRegion
	}
	return &Anthropic{client: anthropic.NewClient(opts...), model: cfg.Model, region: region}, nil
}

// Kind is the run record's provider.kind.
func (a *Anthropic) Kind() string { return "anthropic" }

// Model is the model ID.
func (a *Anthropic) Model() string { return a.model }

// Region is global unless the user declared another.
func (a *Anthropic) Region() string { return a.region }

// Capabilities reads the capability table.
func (a *Anthropic) Capabilities() Capabilities { return capabilityTable[a.Kind()] }

// Chat makes one Messages API call.
func (a *Anthropic) Chat(ctx context.Context, req Request) (Response, error) {
	msg, err := a.client.Messages.New(ctx, anthropicParams(a.model, req))
	if err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			return Response{}, fmt.Errorf("provider: anthropic: %d: %s (%w)", apiErr.StatusCode, apiErr.Error(), err)
		}
		return Response{}, fmt.Errorf("provider: anthropic: %w", err)
	}
	resp := fromAnthropic(msg)
	if isFilterReason(resp.FinishReason) {
		return resp, filtered("anthropic", "stop reason "+resp.FinishReason, nil)
	}
	return resp, nil
}

// anthropicParams maps a Request onto the Messages API. ForceTool is ignored
// here: Adapt turns it into an instruction first.
func anthropicParams(model string, req Request) anthropic.MessageNewParams {
	maxTokens := int64(req.MaxOutputTokens)
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	params := anthropic.MessageNewParams{Model: model, MaxTokens: maxTokens}
	if req.System != "" {
		params.System = []anthropic.TextBlockParam{{Text: req.System}}
	}
	var msgs []anthropic.MessageParam
	add := func(role anthropic.MessageParamRole, blocks ...anthropic.ContentBlockParamUnion) {
		if len(blocks) == 0 {
			return
		}
		// The API wants alternating roles, and every tool result in one user message.
		if n := len(msgs); n > 0 && msgs[n-1].Role == role {
			msgs[n-1].Content = append(msgs[n-1].Content, blocks...)
			return
		}
		msgs = append(msgs, anthropic.MessageParam{Role: role, Content: blocks})
	}
	for _, t := range req.Turns {
		switch t.Role {
		case RoleUser:
			var blocks []anthropic.ContentBlockParamUnion
			if t.Text != "" {
				blocks = append(blocks, anthropic.NewTextBlock(t.Text))
			}
			for _, img := range t.Images {
				blocks = append(blocks, anthropic.NewImageBlockBase64("image/png", base64.StdEncoding.EncodeToString(img.PNG)))
			}
			add(anthropic.MessageParamRoleUser, blocks...)
		case RoleAssistant:
			var blocks []anthropic.ContentBlockParamUnion
			if t.Text != "" {
				blocks = append(blocks, anthropic.NewTextBlock(t.Text))
			}
			for _, c := range t.ToolCalls {
				blocks = append(blocks, anthropic.NewToolUseBlock(c.ID, argsObject(c.Args), c.Name))
			}
			add(anthropic.MessageParamRoleAssistant, blocks...)
		case RoleTool:
			content := []anthropic.ToolResultBlockParamContentUnion{{OfText: &anthropic.TextBlockParam{Text: orText(t.Text, "(no output)")}}}
			for _, img := range t.Images {
				content = append(content, anthropic.ToolResultBlockParamContentUnion{OfImage: &anthropic.ImageBlockParam{
					Source: anthropic.ImageBlockParamSourceUnion{OfBase64: &anthropic.Base64ImageSourceParam{
						MediaType: anthropic.Base64ImageSourceMediaTypeImagePNG, Data: base64.StdEncoding.EncodeToString(img.PNG)}},
				}})
			}
			add(anthropic.MessageParamRoleUser, anthropic.ContentBlockParamUnion{OfToolResult: &anthropic.ToolResultBlockParam{
				ToolUseID: t.ToolCallID, Content: content}})
		}
	}
	params.Messages = msgs
	for _, tool := range req.Tools {
		schema := schemaOf(tool)
		in := anthropic.ToolInputSchemaParam{Properties: schema["properties"], ExtraFields: map[string]any{}}
		if req, ok := schema["required"].([]any); ok {
			for _, r := range req {
				if s, ok := r.(string); ok {
					in.Required = append(in.Required, s)
				}
			}
		}
		for k, v := range schema {
			if k != "type" && k != "properties" && k != "required" {
				in.ExtraFields[k] = v
			}
		}
		params.Tools = append(params.Tools, anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{
			Name: tool.Name, Description: anthropic.String(tool.Description), InputSchema: in}})
	}
	if len(req.Tools) > 0 {
		// Auto, and one tool at a time: the runner runs one action per turn.
		params.ToolChoice = anthropic.ToolChoiceUnionParam{OfAuto: &anthropic.ToolChoiceAutoParam{
			DisableParallelToolUse: anthropic.Bool(true)}}
	}
	return params
}

func fromAnthropic(m *anthropic.Message) Response {
	if m == nil {
		return Response{}
	}
	resp := Response{
		FinishReason: string(m.StopReason),
		Usage: Usage{
			InputTokens:       int(m.Usage.InputTokens + m.Usage.CacheCreationInputTokens + m.Usage.CacheReadInputTokens),
			OutputTokens:      int(m.Usage.OutputTokens),
			CachedInputTokens: int(m.Usage.CacheReadInputTokens),
		},
	}
	for _, block := range m.Content {
		switch block.Type {
		case "text":
			resp.Text += block.Text
		case "tool_use":
			args := block.Input
			if len(args) == 0 || !json.Valid(args) {
				args = json.RawMessage("{}")
			}
			id := block.ID
			if id == "" {
				id = fmt.Sprintf("call_%d", len(resp.ToolCalls)+1)
			}
			resp.ToolCalls = append(resp.ToolCalls, ToolCall{ID: id, Name: block.Name, Args: args})
		}
	}
	return resp
}
