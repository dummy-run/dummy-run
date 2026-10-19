package provider

import (
	"encoding/json"
	"fmt"
	"strings"
)

// schemaOf returns a tool's parameters as a plain JSON Schema object. A tool
// without parameters gets an empty object schema, which every backend needs.
func schemaOf(t Tool) map[string]any {
	empty := map[string]any{"type": "object", "properties": map[string]any{}}
	if t.Parameters == nil {
		return empty
	}
	data, err := json.Marshal(t.Parameters)
	if err != nil {
		return empty
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil || out == nil {
		return empty
	}
	if _, ok := out["type"]; !ok {
		out["type"] = "object"
	}
	if _, ok := out["properties"]; !ok {
		out["properties"] = map[string]any{}
	}
	return out
}

// argsObject decodes tool-call arguments as a JSON object. Arguments that are
// not an object become an empty one, so a replay of the turn stays valid.
func argsObject(raw json.RawMessage) map[string]any {
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

// orText returns s, or fallback when s is blank. Some backends reject empty text.
func orText(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

// encodeArgs renders decoded arguments back to JSON for a tool call.
func encodeArgs(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil || string(data) == "null" {
		return json.RawMessage("{}")
	}
	return data
}

// guardReply turns a panic inside a vendor SDK into an error. Some SDKs panic on
// a malformed reply, such as a JSON null; a bad reply from the service must not end the process.
// Use it as: defer guardReply(kind, &resp, &err).
func guardReply(kind string, resp *Response, err *error) {
	if r := recover(); r != nil {
		*resp, *err = Response{}, fmt.Errorf("provider: %s: could not read the reply: %v", kind, r)
	}
}
