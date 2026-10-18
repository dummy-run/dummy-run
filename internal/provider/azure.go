package provider

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/azure"
	"github.com/openai/openai-go/v3/shared"
)

// Azure is Azure OpenAI through openai-go's azure package. Authentication is
// the developer's Azure CLI sign-in or any other DefaultAzureCredential source;
// the engine never handles an API key.
type Azure struct {
	client openai.Client
	model  string
	region string
}

// DefaultAPIVersion is the Azure OpenAI API version used when none is configured.
const DefaultAPIVersion = "2025-04-01-preview"

// NewAzure builds the Azure provider. Endpoint is https://<resource>.openai.azure.com.
func NewAzure(cfg Config) (*Azure, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("provider: azure needs an endpoint, such as https://<resource>.openai.azure.com")
	}
	if cfg.Model == "" {
		return nil, errors.New("provider: azure needs a deployment name as the model")
	}
	if cfg.Region == "" {
		return nil, errors.New("provider: azure needs the deployment's region, such as uksouth or global")
	}
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("provider: azure credential: %w", err)
	}
	version := cfg.APIVersion
	if version == "" {
		version = DefaultAPIVersion
	}
	client := openai.NewClient(azure.WithEndpoint(strings.TrimSuffix(cfg.Endpoint, "/"), version), azure.WithTokenCredential(cred))
	return &Azure{client: client, model: cfg.Model, region: cfg.Region}, nil
}

// Kind is the run record's provider.kind for Azure OpenAI.
func (a *Azure) Kind() string { return "azure-foundry" }

// Model is the deployment name.
func (a *Azure) Model() string { return a.model }

// Region is the deployment's region as configured.
func (a *Azure) Region() string { return a.region }

// Chat makes one chat completion call.
func (a *Azure) Chat(ctx context.Context, req Request) (Response, error) {
	params := chatParams(a.model, req)
	completion, err := a.client.Chat.Completions.New(ctx, params)
	if err != nil {
		var apiErr *openai.Error
		if errors.As(err, &apiErr) && apiErr.Message != "" {
			return Response{}, fmt.Errorf("provider: azure: %d %s: %s (%w)", apiErr.StatusCode, apiErr.Code, apiErr.Message, err)
		}
		return Response{}, fmt.Errorf("provider: azure: %w", err)
	}
	return fromCompletion(completion)
}

// chatParams maps a Request onto the chat completions API.
func chatParams(model string, req Request) openai.ChatCompletionNewParams {
	msgs := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Turns)+1)
	if req.System != "" {
		msgs = append(msgs, openai.SystemMessage(req.System))
	}
	for _, t := range req.Turns {
		switch t.Role {
		case RoleUser:
			parts := []openai.ChatCompletionContentPartUnionParam{openai.TextContentPart(t.Text)}
			for _, img := range t.Images {
				parts = append(parts, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
					URL:    "data:image/png;base64," + base64.StdEncoding.EncodeToString(img.PNG),
					Detail: "auto",
				}))
			}
			msgs = append(msgs, openai.UserMessage(parts))
		case RoleAssistant:
			m := openai.ChatCompletionAssistantMessageParam{}
			if t.Text != "" {
				m.Content.OfString = openai.String(t.Text)
			}
			for _, c := range t.ToolCalls {
				m.ToolCalls = append(m.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
					OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
						ID: c.ID,
						Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
							Name: c.Name, Arguments: string(c.Args),
						},
					},
				})
			}
			msgs = append(msgs, openai.ChatCompletionMessageParamUnion{OfAssistant: &m})
		case RoleTool:
			msgs = append(msgs, openai.ToolMessage(t.Text, t.ToolCallID))
		}
	}
	params := openai.ChatCompletionNewParams{Model: model, Messages: msgs}
	if req.MaxOutputTokens > 0 {
		params.MaxCompletionTokens = openai.Int(int64(req.MaxOutputTokens))
	}
	for _, tool := range req.Tools {
		def := shared.FunctionDefinitionParam{Name: tool.Name, Description: openai.String(tool.Description)}
		if tool.Parameters != nil {
			def.Parameters = shared.FunctionParameters(tool.Parameters)
		}
		params.Tools = append(params.Tools, openai.ChatCompletionFunctionTool(def))
	}
	if len(req.Tools) > 0 {
		if req.ForceTool != "" {
			params.ToolChoice = openai.ToolChoiceOptionFunctionToolChoice(openai.ChatCompletionNamedToolChoiceFunctionParam{Name: req.ForceTool})
		} else {
			params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: openai.String("required")}
		}
	}
	return params
}

func fromCompletion(c *openai.ChatCompletion) (Response, error) {
	resp := Response{Usage: Usage{InputTokens: int(c.Usage.PromptTokens), OutputTokens: int(c.Usage.CompletionTokens)}}
	if len(c.Choices) == 0 {
		return resp, ErrNoChoice
	}
	choice := c.Choices[0]
	resp.FinishReason = choice.FinishReason
	resp.Text = choice.Message.Content
	for _, tc := range choice.Message.ToolCalls {
		if tc.Type != "function" && tc.Type != "" {
			continue
		}
		fn := tc.AsFunction()
		resp.ToolCalls = append(resp.ToolCalls, ToolCall{ID: fn.ID, Name: fn.Function.Name, Args: []byte(fn.Function.Arguments)})
	}
	return resp, nil
}
