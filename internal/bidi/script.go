package bidi

import (
	"context"
	"encoding/json"
	"fmt"
)

// RemoteValue is a script result as the protocol serialises it.
type RemoteValue struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value,omitempty"`
}

// EvaluateResult is the result of script.evaluate or script.callFunction.
type EvaluateResult struct {
	Type             string      `json:"type"`
	Result           RemoteValue `json:"result"`
	ExceptionDetails struct {
		Text      string      `json:"text"`
		Exception RemoteValue `json:"exception"`
	} `json:"exceptionDetails"`
}

// ScriptError is a script that threw.
type ScriptError struct {
	Text string
}

func (e *ScriptError) Error() string { return "bidi: script threw: " + e.Text }

func target(context string) map[string]any {
	return map[string]any{"context": context}
}

// Evaluate runs an expression in the context and returns its serialised value.
func (c *Conn) Evaluate(ctx context.Context, context, expression string) (RemoteValue, error) {
	var out EvaluateResult
	err := c.Call(ctx, "script.evaluate", map[string]any{
		"expression":      expression,
		"target":          target(context),
		"awaitPromise":    true,
		"resultOwnership": "none",
	}, &out)
	if err != nil {
		return RemoteValue{}, err
	}
	if out.Type == "exception" {
		return RemoteValue{}, &ScriptError{Text: exceptionText(out)}
	}
	return out.Result, nil
}

// CallFunction calls a function declaration with arguments. Arguments are
// local values (string, number, bool) or node references.
func (c *Conn) CallFunction(ctx context.Context, context, declaration string, args ...any) (RemoteValue, error) {
	return c.callFunction(ctx, target(context), declaration, args...)
}

// CallFunctionIn is CallFunction in a sandbox: a realm of its own that shares the page's DOM but none of its
// scripts' globals, so page code can neither see nor change what runs there. The same name reuses the realm
// for as long as the document lives.
func (c *Conn) CallFunctionIn(ctx context.Context, context, sandbox, declaration string, args ...any) (RemoteValue, error) {
	t := target(context)
	t["sandbox"] = sandbox
	return c.callFunction(ctx, t, declaration, args...)
}

func (c *Conn) callFunction(ctx context.Context, tgt map[string]any, declaration string, args ...any) (RemoteValue, error) {
	var out EvaluateResult
	err := c.Call(ctx, "script.callFunction", map[string]any{
		"functionDeclaration": declaration,
		"target":              tgt,
		"arguments":           localValues(args),
		"awaitPromise":        true,
		"resultOwnership":     "none",
	}, &out)
	if err != nil {
		return RemoteValue{}, err
	}
	if out.Type == "exception" {
		return RemoteValue{}, &ScriptError{Text: exceptionText(out)}
	}
	return out.Result, nil
}

func exceptionText(out EvaluateResult) string {
	if out.ExceptionDetails.Text != "" {
		return out.ExceptionDetails.Text
	}
	return string(out.ExceptionDetails.Exception.Value)
}

// localValues turns Go values into protocol LocalValue objects.
func localValues(args []any) []any {
	vals := make([]any, 0, len(args))
	for _, a := range args {
		switch v := a.(type) {
		case NodeRef:
			vals = append(vals, map[string]any{"sharedId": v.SharedID})
		case string:
			vals = append(vals, map[string]any{"type": "string", "value": v})
		case bool:
			vals = append(vals, map[string]any{"type": "boolean", "value": v})
		case int:
			vals = append(vals, map[string]any{"type": "number", "value": v})
		case float64:
			vals = append(vals, map[string]any{"type": "number", "value": v})
		default:
			vals = append(vals, map[string]any{"type": "undefined"})
		}
	}
	return vals
}

// String returns the value when it is a string, and an error otherwise.
func (v RemoteValue) String() (string, error) {
	if v.Type != "string" {
		return "", fmt.Errorf("bidi: expected a string result, got %s", v.Type)
	}
	var s string
	if err := json.Unmarshal(v.Value, &s); err != nil {
		return "", fmt.Errorf("bidi: decode string result: %w", err)
	}
	return s, nil
}

// EvaluateJSON runs an expression whose value is passed through
// JSON.stringify in the page, and decodes it into out. The page's own JSON
// serialiser avoids the protocol's nested remote values for plain data.
func (c *Conn) EvaluateJSON(ctx context.Context, context, expression string, out any) error {
	v, err := c.Evaluate(ctx, context, "JSON.stringify((()=>{ return ("+expression+"); })())")
	if err != nil {
		return err
	}
	if v.Type == "undefined" {
		return nil
	}
	s, err := v.String()
	if err != nil {
		return err
	}
	if s == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(s), out); err != nil {
		return fmt.Errorf("bidi: decode page JSON: %w", err)
	}
	return nil
}

// CallFunctionJSON calls a function whose return value is JSON.stringify'd in
// the page, and decodes it into out.
func (c *Conn) CallFunctionJSON(ctx context.Context, context, declaration string, out any, args ...any) error {
	return c.callFunctionJSON(ctx, target(context), declaration, out, args...)
}

// CallFunctionJSONIn is CallFunctionJSON in a sandbox (see CallFunctionIn).
func (c *Conn) CallFunctionJSONIn(ctx context.Context, context, sandbox, declaration string, out any, args ...any) error {
	t := target(context)
	t["sandbox"] = sandbox
	return c.callFunctionJSON(ctx, t, declaration, out, args...)
}

func (c *Conn) callFunctionJSON(ctx context.Context, tgt map[string]any, declaration string, out any, args ...any) error {
	wrapped := "function(...a){ const r = (" + declaration + ").apply(this, a); " +
		"return Promise.resolve(r).then((x) => JSON.stringify(x === undefined ? null : x)); }"
	v, err := c.callFunction(ctx, tgt, wrapped, args...)
	if err != nil {
		return err
	}
	s, err := v.String()
	if err != nil {
		return err
	}
	if s == "" || s == "null" {
		return nil
	}
	if err := json.Unmarshal([]byte(s), out); err != nil {
		return fmt.Errorf("bidi: decode page JSON: %w", err)
	}
	return nil
}
