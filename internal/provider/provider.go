// Package provider is the model behind a persona. One interface, several
// clouds: Azure OpenAI is implemented; Anthropic and Gemini fit the same shape.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
}

// Turn is one message in the conversation.
type Turn struct {
	Role Role
	// Text is the message, or the tool result for RoleTool.
	Text string
	// Images accompany a user turn.
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
}

// Usage is the token count of one call.
type Usage struct {
	InputTokens  int
	OutputTokens int
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

// Provider plays a persona.
type Provider interface {
	// Kind is the run record's provider.kind: azure-foundry, anthropic, vertex-gemini or openai-compatible.
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

// Config selects and configures a provider.
type Config struct {
	// Provider is "azure".
	Provider string
	// Model is the deployment or model name.
	Model string
	// Region is recorded in the run; Azure deployments do not report it.
	Region string
	// Endpoint is the provider's base URL, such as https://<resource>.openai.azure.com.
	Endpoint string
	// APIVersion is the Azure OpenAI API version.
	APIVersion string
}

// New builds a provider from its configuration.
func New(cfg Config) (Provider, error) {
	switch cfg.Provider {
	case "azure":
		return NewAzure(cfg)
	case "anthropic", "gemini", "vertex":
		return nil, fmt.Errorf("provider: %s is not implemented yet; the interface is ready for it", cfg.Provider)
	default:
		return nil, fmt.Errorf("provider: unknown provider %q (azure)", cfg.Provider)
	}
}
