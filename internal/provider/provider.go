// Package provider is the model behind a persona. One interface, many
// backends: Azure OpenAI, any OpenAI-compatible endpoint, Anthropic, Google
// Gemini (Vertex AI or the Gemini Developer API) and AWS Bedrock.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Role of a turn in the conversation.
type Role string

// The roles a turn can have.
const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Image is a PNG shown to the model.
type Image struct {
	PNG []byte
}

// ToolCall is the model asking for one tool.
type ToolCall struct {
	ID   string
	Name string
	Args json.RawMessage
	// State is opaque data a provider needs sent back with this call on the
	// next request, such as a Gemini thought signature. The runner keeps it as is.
	State []byte
}

// Turn is one message in the conversation.
type Turn struct {
	Role Role
	// Text is the message, or the tool result for RoleTool.
	Text string
	// Images accompany a user turn, or a tool turn when the provider's
	// Capabilities.ImagesInToolResults is true.
	Images []Image
	// ToolCalls are what an assistant turn asked for.
	ToolCalls []ToolCall
	// ToolCallID says which call a tool turn answers.
	ToolCallID string
}

// Tool is a function the model may call. Parameters is a JSON Schema object.
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// Request is one model call.
type Request struct {
	System string
	Turns  []Turn
	Tools  []Tool
	// ForceTool names the one tool the model must call, or "" for its choice.
	// Providers that return 400 for forced tool use fall back to auto and say
	// so in Response.Note.
	ForceTool string
	// MaxOutputTokens caps the reply.
	MaxOutputTokens int
	// Seed is the sampling seed for backends whose Capabilities.Seed is true, or nil. Backends
	// without a seed ignore it.
	Seed *int64
}

// Usage is the token count of one call.
type Usage struct {
	// InputTokens counts every input token, cached ones included.
	InputTokens  int
	OutputTokens int
	// CachedInputTokens is the part of InputTokens the service served from its
	// prompt cache, where the API reports it. It is zero otherwise.
	CachedInputTokens int
}

// Response is the model's reply.
type Response struct {
	Text      string
	ToolCalls []ToolCall
	Usage     Usage
	// FinishReason is the provider's own value, recorded for diagnosis.
	FinishReason string
	// Note carries a provider warning, such as a forced tool downgraded to auto.
	Note string
}

// Provider plays a persona. A provider may also implement Capable.
type Provider interface {
	// Kind is the run record's provider.kind: azure-foundry, openai-compatible, anthropic, vertex-gemini, gemini-api or bedrock.
	Kind() string
	// Model is the model or deployment name.
	Model() string
	// Region is where the model ran, or "global".
	Region() string
	// Chat makes one call.
	Chat(ctx context.Context, req Request) (Response, error)
}

// ErrNoChoice is returned when the model sent an empty reply.
var ErrNoChoice = errors.New("provider: the model returned no choice")

// Provider names accepted by New.
const (
	NameAzure            = "azure"
	NameOpenAICompatible = "openai-compatible"
	NameAnthropic        = "anthropic"
	NameGemini           = "gemini"
	NameVertex           = "vertex"
	NameBedrock          = "bedrock"
)

// Names lists the provider names New accepts.
var Names = []string{NameAzure, NameOpenAICompatible, NameAnthropic, NameGemini, NameVertex, NameBedrock}

// Config selects and configures a provider.
type Config struct {
	// Provider is one of Names.
	Provider string
	// Model is the deployment or model name.
	Model string
	// Region is recorded in the run. For vertex and bedrock it is also where the call goes.
	// Empty means the provider's DefaultRegion.
	Region string
	// Endpoint is the provider's base URL, such as https://<resource>.openai.azure.com
	// for Azure or https://api.openai.com/v1 for an OpenAI-compatible service.
	Endpoint string
	// APIVersion is the Azure OpenAI API version.
	APIVersion string
	// APIKeyEnv names the environment variable that holds the API key. The key is
	// read at run time and never stored or logged.
	APIKeyEnv string
	// ReasoningEffort sets the reasoning effort for the OpenAI-style APIs: none,
	// minimal, low, medium, high or xhigh. Empty leaves the model's default. Some
	// models only take tool calls over chat completions with it set to none.
	ReasoningEffort string
	// API picks the OpenAI-style API: "chat" (the default) or "responses", for
	// models that need the Responses API to call tools.
	API string
	// Project is the Google Cloud project for vertex.
	Project string
}

// New builds a provider from its configuration.
func New(cfg Config) (Provider, error) {
	switch cfg.Provider {
	case NameAzure:
		return NewAzure(cfg)
	case NameOpenAICompatible:
		return NewOpenAICompatible(cfg)
	case NameAnthropic:
		return NewAnthropic(cfg)
	case NameGemini, NameVertex:
		return NewGemini(cfg)
	case NameBedrock:
		return NewBedrock(cfg)
	default:
		return nil, fmt.Errorf("provider: unknown provider %q (%s)", cfg.Provider, strings.Join(Names, ", "))
	}
}

// keyFromEnv reads the API key from the named environment variable. The error
// names the variable, never the value.
func keyFromEnv(name string) (string, error) {
	if name == "" {
		return "", errors.New("provider: set --api-key-env to the name of the environment variable that holds the API key")
	}
	key := os.Getenv(name)
	if key == "" {
		return "", fmt.Errorf("provider: the environment variable %s is empty or not set", name)
	}
	return key, nil
}
