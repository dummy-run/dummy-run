package provider

import (
	"slices"
	"strings"
)

// Capabilities says what a backend can do. The runner reads it to choose
// fallbacks, so it never branches on a provider's name.
type Capabilities struct {
	// ToolCalling is true when the model can call the supplied tools.
	ToolCalling bool
	// ForcedToolChoice is true when the backend honours Request.ForceTool.
	// When false, Adapt replaces it with an instruction.
	ForcedToolChoice bool
	// ImagesInToolResults is true when a tool turn may carry images. When
	// false, the runner sends a screenshot as a user turn after the tool turns.
	ImagesInToolResults bool
	// Images is true when a user turn may carry images.
	Images bool
	// StructuredOutput is true when the backend can constrain a reply to a JSON Schema.
	StructuredOutput bool
	// ReasoningEffort is true when --reasoning-effort is sent to the backend.
	ReasoningEffort bool
	// ResponsesAPI is true when the backend offers the OpenAI Responses API,
	// chosen with --openai-api responses, for models that need it to call tools.
	ResponsesAPI bool
	// CachedTokens is true when the backend reports cached input tokens in usage.
	CachedTokens bool
	// DefaultRegion is what the run record gets when no region is given. Empty
	// means the caller must say, because the backend cannot tell where it runs.
	DefaultRegion string
	// RegionSelectable is true when the region decides where the call goes.
	RegionSelectable bool
}

// Capable is implemented by providers that report their capabilities.
type Capable interface {
	Capabilities() Capabilities
}

// capabilityTable holds each provider kind's capabilities. Every provider's
// Capabilities method reads this table.
//
// openai-compatible is cautious on purpose: backends differ on forced and
// named tool choice, so the finish step uses an instruction there, and images
// go in user turns because the chat completions format has no image in a tool message.
var capabilityTable = map[string]Capabilities{
	"azure-foundry":     {ToolCalling: true, ForcedToolChoice: true, Images: true, StructuredOutput: true, ReasoningEffort: true, ResponsesAPI: true, CachedTokens: true},
	"openai-compatible": {ToolCalling: true, Images: true, StructuredOutput: true, ReasoningEffort: true, ResponsesAPI: true, CachedTokens: true},
	"anthropic":         {ToolCalling: true, ImagesInToolResults: true, Images: true, StructuredOutput: true, CachedTokens: true, DefaultRegion: "global"},
	"gemini-api":        {ToolCalling: true, ForcedToolChoice: true, Images: true, StructuredOutput: true, CachedTokens: true, DefaultRegion: "global"},
	"vertex-gemini":     {ToolCalling: true, ForcedToolChoice: true, Images: true, StructuredOutput: true, CachedTokens: true, RegionSelectable: true},
	"bedrock":           {ToolCalling: true, Images: true, StructuredOutput: true, CachedTokens: true, RegionSelectable: true, DefaultRegion: "eu-west-2"},
}

// Kinds lists the provider kinds in the capability table, sorted.
func Kinds() []string {
	kinds := make([]string, 0, len(capabilityTable))
	for k := range capabilityTable {
		kinds = append(kinds, k)
	}
	slices.Sort(kinds)
	return kinds
}

// CapabilitiesFor returns the table entry for a provider kind.
func CapabilitiesFor(kind string) (Capabilities, bool) {
	c, ok := capabilityTable[kind]
	return c, ok
}

// CapabilitiesOf reports a provider's capabilities. A provider that does not
// say gets the interface's original contract: tool calling, a forced tool
// choice and images in user turns.
func CapabilitiesOf(p Provider) Capabilities {
	if c, ok := p.(Capable); ok {
		return c.Capabilities()
	}
	return Capabilities{ToolCalling: true, ForcedToolChoice: true, Images: true}
}

// FinishNowInstruction is sent in place of a forced tool choice.
const FinishNowInstruction = "Stop here. Call the %s tool now and do not call any other tool."

// Adapt rewrites a request for a backend that cannot honour it. When the
// request forces a tool and the backend cannot force one, the force is dropped
// and an instruction to call that tool is added as a user turn. The note says
// what changed. Call it before Chat.
func Adapt(c Capabilities, req Request) (Request, string) {
	if req.ForceTool == "" || c.ForcedToolChoice {
		return req, ""
	}
	name := req.ForceTool
	req.Turns = slices.Concat(req.Turns, []Turn{{Role: RoleUser, Text: strings.Replace(FinishNowInstruction, "%s", name, 1)}})
	req.ForceTool = ""
	return req, "this backend cannot force a tool, so the model was told to call " + name
}
