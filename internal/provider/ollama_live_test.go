package provider

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"strings"
	"testing"
	"time"
)

// TestOllamaLive is a smoke test against a local Ollama server with a vision
// model. It is skipped unless DUMMYRUN_OLLAMA_MODEL names a model, for example
// DUMMYRUN_OLLAMA_MODEL=gemma4:e4b. DUMMYRUN_OLLAMA_URL overrides the default
// http://localhost:11434/v1. CI never sets either variable.
func TestOllamaLive(t *testing.T) {
	model := os.Getenv("DUMMYRUN_OLLAMA_MODEL")
	if model == "" {
		t.Skip("set DUMMYRUN_OLLAMA_MODEL to run against a local Ollama")
	}
	url := os.Getenv("DUMMYRUN_OLLAMA_URL")
	if url == "" {
		url = "http://localhost:11434/v1"
	}
	img := image.NewRGBA(image.Rect(0, 0, 96, 96))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 220, 20, 20, 255
	}
	var buf bytes.Buffer
	must(t, png.Encode(&buf, img))

	p, err := NewOpenAICompatible(Config{Endpoint: url, Model: model, Region: "local"})
	must(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	start := time.Now()
	resp, err := p.Chat(ctx, Request{
		System: "You look at a screenshot and report what you see by calling the tool.",
		Turns:  []Turn{{Role: RoleUser, Text: "What single colour fills this image? Call report_colour with the colour name.", Images: []Image{{PNG: buf.Bytes()}}}},
		Tools: []Tool{{Name: "report_colour", Description: "Report the colour you see.", Parameters: map[string]any{
			"type": "object", "properties": map[string]any{"colour": map[string]any{"type": "string"}}, "required": []string{"colour"}}}},
		MaxOutputTokens: 1024,
	})
	must(t, err)
	t.Logf("%s answered in %s: text %q, tool calls %+v, usage %+v", model, time.Since(start).Round(time.Second), resp.Text, resp.ToolCalls, resp.Usage)
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "report_colour" {
		t.Fatalf("no tool call: %+v", resp)
	}
	if !strings.Contains(strings.ToLower(string(resp.ToolCalls[0].Args)), "red") {
		t.Errorf("the model did not see red: %s", resp.ToolCalls[0].Args)
	}
}
