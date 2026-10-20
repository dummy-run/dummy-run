package provider

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
)

// OpenAICompatible talks to any service that offers the OpenAI chat
// completions API: OpenAI, OpenRouter, Mistral, Scaleway, Nebius, OVHcloud,
// Together, Fireworks, DeepInfra, Groq, and local servers such as Ollama, vLLM
// and the llama.cpp server. The API key comes from a named environment
// variable at run time. A local server needs none.
type OpenAICompatible struct {
	client openai.Client
	model  string
	region string
	// legacyTokens sends max_tokens, which every compatible server accepts,
	// instead of max_completion_tokens, which only OpenAI's own API needs for newer models.
	legacyTokens bool
	effort       string
	api          string
}

// NewOpenAICompatible builds the provider. cfg.Endpoint is the base URL up to
// and including /v1 (or the vendor's equivalent). cfg.APIKeyEnv is optional; when
// set, the variable must hold a key.
func NewOpenAICompatible(cfg Config) (*OpenAICompatible, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("provider: openai-compatible needs --endpoint, the base URL such as https://api.openai.com/v1 or http://localhost:11434/v1")
	}
	u, err := url.Parse(cfg.Endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("provider: openai-compatible endpoint %q is not an http or https URL", cfg.Endpoint)
	}
	if err := checkAPI(cfg); err != nil {
		return nil, err
	}
	if cfg.Model == "" {
		return nil, errors.New("provider: openai-compatible needs a model name")
	}
	if cfg.Region == "" {
		return nil, errors.New("provider: openai-compatible needs --region, where the service runs, such as eu-fr-par, global or local")
	}
	opts := []option.RequestOption{
		option.WithBaseURL(strings.TrimSuffix(cfg.Endpoint, "/") + "/"),
		option.WithRequestTimeout(RequestTimeout), option.WithMaxRetries(3),
		// The SDK reads OPENAI_API_KEY, OPENAI_ORG_ID and OPENAI_PROJECT_ID from the
		// environment. They belong to OpenAI, so they are cleared here and the
		// key comes only from the variable the user named.
		option.WithAPIKey(""), option.WithOrganization(""), option.WithProject(""),
	}
	if cfg.APIKeyEnv != "" {
		key, err := keyFromEnv(cfg.APIKeyEnv)
		if err != nil {
			return nil, err
		}
		opts = append(opts, option.WithAPIKey(key))
	} else {
		opts = append(opts, option.WithHeaderDel("Authorization"))
	}
	return &OpenAICompatible{
		client: openai.NewClient(opts...), model: cfg.Model, region: cfg.Region,
		legacyTokens: u.Hostname() != "api.openai.com", effort: cfg.ReasoningEffort, api: cfg.API,
	}, nil
}

// Kind is the run record's provider.kind.
func (o *OpenAICompatible) Kind() string { return "openai-compatible" }

// Model is the model name sent to the service.
func (o *OpenAICompatible) Model() string { return o.model }

// Region is where the service runs, as the user declared it.
func (o *OpenAICompatible) Region() string { return o.region }

// Capabilities reads the capability table.
func (o *OpenAICompatible) Capabilities() Capabilities {
	c := capabilityTable[o.Kind()]
	c.Seed = c.Seed && o.api != APIResponses // the Responses API has no seed
	return c
}

// Chat makes one chat completion call.
func (o *OpenAICompatible) Chat(ctx context.Context, req Request) (Response, error) {
	if o.api == APIResponses {
		return callResponses(ctx, &o.client, "openai-compatible", responsesParams(o.model, req, "auto", o.effort))
	}
	params := chatParamsWith(o.model, req, "auto")
	if o.effort != "" {
		params.ReasoningEffort = shared.ReasoningEffort(o.effort)
	}
	if o.legacyTokens && req.MaxOutputTokens > 0 {
		params.MaxCompletionTokens = param.Opt[int64]{}
		params.MaxTokens = openai.Int(int64(req.MaxOutputTokens))
	}
	completion, err := o.client.Chat.Completions.New(ctx, params)
	if err != nil {
		var apiErr *openai.Error
		if errors.As(err, &apiErr) && isFilterAPIError(apiErr.Code, apiErr.Message) {
			return Response{}, filtered("openai-compatible", apiErr.Message, err)
		}
		if errors.As(err, &apiErr) && apiErr.Message != "" {
			return Response{}, fmt.Errorf("provider: openai-compatible: %d %s: %s (%w)", apiErr.StatusCode, apiErr.Code, apiErr.Message, err)
		}
		return Response{}, fmt.Errorf("provider: openai-compatible: %w", err)
	}
	return fromCompletion(completion)
}
