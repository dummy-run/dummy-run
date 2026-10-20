package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/aws/smithy-go"
)

// Bedrock is the Amazon Bedrock Converse API through the AWS SDK for Go v2.
// Credentials come from the SDK's default chain (environment, shared config,
// SSO, container or instance role); the engine never handles an access key
// itself. The region decides where the call runs and defaults to eu-west-2
// (London). The model is a model ID or an inference profile ID.
type Bedrock struct {
	client *bedrockruntime.Client
	model  string
	region string
}

// NewBedrock builds the provider.
func NewBedrock(cfg Config) (*Bedrock, error) {
	return newBedrock(cfg, nil, nil)
}

func newBedrock(cfg Config, load []func(*config.LoadOptions) error, client []func(*bedrockruntime.Options)) (*Bedrock, error) {
	if cfg.Model == "" {
		return nil, errors.New("provider: bedrock needs a model ID or inference profile ID")
	}
	region := cfg.Region
	if region == "" {
		region = capabilityTable["bedrock"].DefaultRegion
	}
	load = append([]func(*config.LoadOptions) error{config.WithRegion(region)}, load...)
	awsCfg, err := config.LoadDefaultConfig(context.Background(), load...)
	if err != nil {
		return nil, fmt.Errorf("provider: bedrock credentials: %w", err)
	}
	if cfg.Endpoint != "" {
		client = append(client, func(o *bedrockruntime.Options) { o.BaseEndpoint = &cfg.Endpoint })
	}
	return &Bedrock{client: bedrockruntime.NewFromConfig(awsCfg, client...), model: cfg.Model, region: region}, nil
}

// Kind is the run record's provider.kind.
func (b *Bedrock) Kind() string { return "bedrock" }

// Model is the model or inference profile ID.
func (b *Bedrock) Model() string { return b.model }

// Region is the AWS region the call goes to.
func (b *Bedrock) Region() string { return b.region }

// Capabilities reads the capability table.
func (b *Bedrock) Capabilities() Capabilities { return capabilityTable[b.Kind()] }

// Chat makes one Converse call.
func (b *Bedrock) Chat(ctx context.Context, req Request) (resp Response, err error) {
	defer guardReply(b.Kind(), &resp, &err)
	ctx, cancel := context.WithTimeout(ctx, 2*RequestTimeout)
	defer cancel()
	out, err := b.client.Converse(ctx, bedrockInput(b.model, req))
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			return Response{}, fmt.Errorf("provider: bedrock: %s: %s (%w)", apiErr.ErrorCode(), apiErr.ErrorMessage(), err)
		}
		return Response{}, fmt.Errorf("provider: bedrock: %w", err)
	}
	return fromBedrock(out)
}

// bedrockInput maps a Request onto Converse. ForceTool is ignored here: Adapt
// turns it into an instruction first, since forced tool use is not available on every model.
func bedrockInput(model string, req Request) *bedrockruntime.ConverseInput {
	in := &bedrockruntime.ConverseInput{ModelId: &model}
	if req.System != "" {
		in.System = []types.SystemContentBlock{&types.SystemContentBlockMemberText{Value: req.System}}
	}
	if req.MaxOutputTokens > 0 {
		n := int32(min(req.MaxOutputTokens, 1<<30)) //nolint:gosec // bounded just above
		in.InferenceConfig = &types.InferenceConfiguration{MaxTokens: &n}
	}
	add := func(role types.ConversationRole, blocks ...types.ContentBlock) {
		if len(blocks) == 0 {
			return
		}
		if n := len(in.Messages); n > 0 && in.Messages[n-1].Role == role {
			in.Messages[n-1].Content = append(in.Messages[n-1].Content, blocks...)
			return
		}
		in.Messages = append(in.Messages, types.Message{Role: role, Content: blocks})
	}
	for _, t := range req.Turns {
		switch t.Role {
		case RoleUser:
			var blocks []types.ContentBlock
			if t.Text != "" {
				blocks = append(blocks, &types.ContentBlockMemberText{Value: t.Text})
			}
			for _, img := range t.Images {
				blocks = append(blocks, &types.ContentBlockMemberImage{Value: types.ImageBlock{
					Format: types.ImageFormatPng, Source: &types.ImageSourceMemberBytes{Value: img.PNG}}})
			}
			add(types.ConversationRoleUser, blocks...)
		case RoleAssistant:
			var blocks []types.ContentBlock
			if t.Text != "" {
				blocks = append(blocks, &types.ContentBlockMemberText{Value: t.Text})
			}
			for _, c := range t.ToolCalls {
				id, name := c.ID, c.Name
				blocks = append(blocks, &types.ContentBlockMemberToolUse{Value: types.ToolUseBlock{
					ToolUseId: &id, Name: &name, Input: document.NewLazyDocument(argsObject(c.Args))}})
			}
			add(types.ConversationRoleAssistant, blocks...)
		case RoleTool:
			id := t.ToolCallID
			add(types.ConversationRoleUser, &types.ContentBlockMemberToolResult{Value: types.ToolResultBlock{
				ToolUseId: &id, Content: []types.ToolResultContentBlock{&types.ToolResultContentBlockMemberText{Value: orText(t.Text, "(no output)")}}}})
		}
	}
	if len(req.Tools) > 0 {
		tc := &types.ToolConfiguration{ToolChoice: &types.ToolChoiceMemberAuto{}}
		for _, tool := range req.Tools {
			name, desc := tool.Name, tool.Description
			spec := types.ToolSpecification{Name: &name,
				InputSchema: &types.ToolInputSchemaMemberJson{Value: document.NewLazyDocument(schemaOf(tool))}}
			if desc != "" {
				spec.Description = &desc
			}
			tc.Tools = append(tc.Tools, &types.ToolMemberToolSpec{Value: spec})
		}
		in.ToolConfig = tc
	}
	return in
}

func fromBedrock(out *bedrockruntime.ConverseOutput) (Response, error) {
	if out == nil {
		return Response{}, ErrNoChoice
	}
	resp := Response{FinishReason: string(out.StopReason)}
	if u := out.Usage; u != nil {
		if u.InputTokens != nil {
			resp.Usage.InputTokens = int(*u.InputTokens)
		}
		if u.CacheReadInputTokens != nil {
			resp.Usage.CachedInputTokens = int(*u.CacheReadInputTokens)
		}
		if u.OutputTokens != nil {
			resp.Usage.OutputTokens = int(*u.OutputTokens)
		}
	}
	if isFilterReason(resp.FinishReason) {
		return resp, filtered("bedrock", "stop reason "+resp.FinishReason, nil)
	}
	msg, ok := out.Output.(*types.ConverseOutputMemberMessage)
	if !ok || msg == nil {
		return resp, ErrNoChoice
	}
	for _, block := range msg.Value.Content {
		switch b := block.(type) {
		case *types.ContentBlockMemberText:
			resp.Text += b.Value
		case *types.ContentBlockMemberToolUse:
			var args any
			if b.Value.Input != nil {
				if err := b.Value.Input.UnmarshalSmithyDocument(&args); err != nil {
					args = nil
				}
			}
			call := ToolCall{ID: deref(b.Value.ToolUseId), Name: deref(b.Value.Name), Args: encodeArgs(args)}
			if call.ID == "" {
				call.ID = fmt.Sprintf("call_%d", len(resp.ToolCalls)+1)
			}
			resp.ToolCalls = append(resp.ToolCalls, call)
		}
	}
	return resp, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
