package provider

import (
	"context"
	"errors"
	"fmt"
	"os"

	"google.golang.org/genai"
)

// DefaultGeminiKeyEnv is the variable the Gemini Developer API provider reads
// the API key from when --api-key-env is not given.
const DefaultGeminiKeyEnv = "GEMINI_API_KEY"

// Gemini is Google Gemini through the genai Go SDK, on one of two backends.
// The Vertex AI backend (provider "vertex") uses Application Default
// Credentials, a project and a region, and keeps data in that region. The
// Gemini Developer API backend (provider "gemini") uses an API key.
type Gemini struct {
	client *genai.Client
	model  string
	region string
	vertex bool
}

// NewGemini builds the provider for cfg.Provider "gemini" or "vertex".
func NewGemini(cfg Config) (*Gemini, error) {
	return newGemini(cfg, nil)
}

// newGemini lets a test change the client configuration, for example to point
// it at a local server.
func newGemini(cfg Config, tweak func(*genai.ClientConfig)) (*Gemini, error) {
	if cfg.Model == "" {
		return nil, errors.New("provider: " + cfg.Provider + " needs a model, such as gemini-2.5-flash")
	}
	retries := int32(3)
	timeout := RequestTimeout
	cc := &genai.ClientConfig{HTTPOptions: genai.HTTPOptions{Timeout: &timeout, RetryOptions: &genai.HTTPRetryOptions{Attempts: &retries}}}
	g := &Gemini{model: cfg.Model}
	switch cfg.Provider {
	case NameVertex:
		project := cfg.Project
		if project == "" {
			project = os.Getenv("GOOGLE_CLOUD_PROJECT")
		}
		if project == "" {
			return nil, errors.New("provider: vertex needs --project or GOOGLE_CLOUD_PROJECT")
		}
		if cfg.Region == "" {
			return nil, errors.New("provider: vertex needs --region, the Google Cloud location such as europe-west2")
		}
		// No APIKey: the SDK then uses Application Default Credentials.
		cc.Backend, cc.Project, cc.Location = genai.BackendVertexAI, project, cfg.Region
		g.vertex, g.region = true, cfg.Region
	case NameGemini:
		env := cfg.APIKeyEnv
		if env == "" {
			env = DefaultGeminiKeyEnv
		}
		key, err := keyFromEnv(env)
		if err != nil {
			return nil, err
		}
		cc.Backend, cc.APIKey = genai.BackendGeminiAPI, key
		g.region = cfg.Region
		if g.region == "" {
			g.region = capabilityTable["gemini-api"].DefaultRegion
		}
	default:
		return nil, fmt.Errorf("provider: %q is not a Gemini provider", cfg.Provider)
	}
	if cfg.Endpoint != "" {
		cc.HTTPOptions.BaseURL = cfg.Endpoint
	}
	if tweak != nil {
		tweak(cc)
	}
	client, err := genai.NewClient(context.Background(), cc)
	if err != nil {
		return nil, fmt.Errorf("provider: %s: %w", cfg.Provider, err)
	}
	g.client = client
	return g, nil
}

// Kind is the run record's provider.kind.
func (g *Gemini) Kind() string {
	if g.vertex {
		return "vertex-gemini"
	}
	return "gemini-api"
}

// Model is the model name.
func (g *Gemini) Model() string { return g.model }

// Region is the Vertex location, or global for the Developer API.
func (g *Gemini) Region() string { return g.region }

// Capabilities reads the capability table.
func (g *Gemini) Capabilities() Capabilities { return capabilityTable[g.Kind()] }

// Chat makes one generateContent call.
func (g *Gemini) Chat(ctx context.Context, req Request) (resp Response, err error) {
	defer guardReply(g.Kind(), &resp, &err)
	contents, cfg := geminiParams(req)
	out, err := g.client.Models.GenerateContent(ctx, g.model, contents, cfg)
	if err != nil {
		var apiErr genai.APIError
		if errors.As(err, &apiErr) {
			return Response{}, fmt.Errorf("provider: %s: %d %s: %s (%w)", g.Kind(), apiErr.Code, apiErr.Status, apiErr.Message, err)
		}
		return Response{}, fmt.Errorf("provider: %s: %w", g.Kind(), err)
	}
	return fromGemini(out)
}

// geminiParams maps a Request onto generateContent. Gemini honours a forced tool.
func geminiParams(req Request) ([]*genai.Content, *genai.GenerateContentConfig) {
	names := map[string]string{} // call ID to tool name, for the turn being answered
	var contents []*genai.Content
	add := func(role genai.Role, parts ...*genai.Part) {
		if len(parts) == 0 {
			return
		}
		// Replies to parallel calls share one content, and roles alternate.
		if n := len(contents); n > 0 && contents[n-1].Role == string(role) {
			contents[n-1].Parts = append(contents[n-1].Parts, parts...)
			return
		}
		contents = append(contents, &genai.Content{Role: string(role), Parts: parts})
	}
	for _, t := range req.Turns {
		switch t.Role {
		case RoleUser:
			var parts []*genai.Part
			if t.Text != "" {
				parts = append(parts, genai.NewPartFromText(t.Text))
			}
			for _, img := range t.Images {
				parts = append(parts, genai.NewPartFromBytes(img.PNG, "image/png"))
			}
			add(genai.RoleUser, parts...)
		case RoleAssistant:
			var parts []*genai.Part
			if t.Text != "" {
				parts = append(parts, genai.NewPartFromText(t.Text))
			}
			clear(names)
			for _, c := range t.ToolCalls {
				names[c.ID] = c.Name
				p := genai.NewPartFromFunctionCall(c.Name, argsObject(c.Args))
				p.ThoughtSignature = c.State
				parts = append(parts, p)
			}
			add(genai.RoleModel, parts...)
		case RoleTool:
			// The reply is matched to its call by name and order, so no call ID is sent.
			add(genai.RoleUser, genai.NewPartFromFunctionResponse(names[t.ToolCallID], map[string]any{"output": t.Text}))
		}
	}
	cfg := &genai.GenerateContentConfig{}
	if req.System != "" {
		cfg.SystemInstruction = genai.NewContentFromText(req.System, genai.RoleUser)
	}
	if req.MaxOutputTokens > 0 {
		cfg.MaxOutputTokens = int32(min(req.MaxOutputTokens, 1<<30)) //nolint:gosec // bounded just above
	}
	if len(req.Tools) > 0 {
		decls := make([]*genai.FunctionDeclaration, 0, len(req.Tools))
		for _, tool := range req.Tools {
			decls = append(decls, &genai.FunctionDeclaration{Name: tool.Name, Description: tool.Description, ParametersJsonSchema: schemaOf(tool)})
		}
		cfg.Tools = []*genai.Tool{{FunctionDeclarations: decls}}
		fc := &genai.FunctionCallingConfig{Mode: genai.FunctionCallingConfigModeAny}
		if req.ForceTool != "" {
			fc.AllowedFunctionNames = []string{req.ForceTool}
		}
		cfg.ToolConfig = &genai.ToolConfig{FunctionCallingConfig: fc}
	}
	return contents, cfg
}

func fromGemini(r *genai.GenerateContentResponse) (Response, error) {
	if r == nil {
		return Response{}, ErrNoChoice
	}
	resp := Response{}
	if u := r.UsageMetadata; u != nil {
		resp.Usage = Usage{InputTokens: int(u.PromptTokenCount + u.ToolUsePromptTokenCount),
			OutputTokens: int(u.CandidatesTokenCount + u.ThoughtsTokenCount), CachedInputTokens: int(u.CachedContentTokenCount)}
	}
	if len(r.Candidates) == 0 || r.Candidates[0] == nil {
		if r.PromptFeedback != nil && r.PromptFeedback.BlockReason != "" {
			return resp, fmt.Errorf("provider: gemini blocked the prompt: %s (%w)", r.PromptFeedback.BlockReason, ErrNoChoice)
		}
		return resp, ErrNoChoice
	}
	cand := r.Candidates[0]
	resp.FinishReason = string(cand.FinishReason)
	if cand.Content == nil {
		return resp, nil
	}
	for _, p := range cand.Content.Parts {
		switch {
		case p == nil:
		case p.FunctionCall != nil:
			resp.ToolCalls = append(resp.ToolCalls, ToolCall{
				ID:   fmt.Sprintf("call_%d", len(resp.ToolCalls)+1), // Gemini sends no ID the engine can rely on
				Name: p.FunctionCall.Name, Args: encodeArgs(p.FunctionCall.Args), State: p.ThoughtSignature})
		case p.Text != "" && !p.Thought:
			resp.Text += p.Text
		}
	}
	return resp, nil
}
