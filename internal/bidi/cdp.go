package bidi

import (
	"context"
	"encoding/json"
	"fmt"
)

// CDPSession returns the Chrome DevTools Protocol session for a browsing context, through
// chromedriver's goog:cdp extension to WebDriver BiDi. Firefox has no such extension, so it
// returns an error there, and callers fall back to what BiDi alone can do.
func (c *Conn) CDPSession(ctx context.Context, context string) (string, error) {
	var out struct {
		Session string `json:"session"`
	}
	if err := c.Call(ctx, "goog:cdp.getSession", map[string]any{"context": context}, &out); err != nil {
		return "", err
	}
	if out.Session == "" {
		return "", fmt.Errorf("bidi: goog:cdp.getSession gave no session for %s", context)
	}
	return out.Session, nil
}

// CDP sends one Chrome DevTools Protocol command on a session from CDPSession and decodes its
// result into out, which may be nil.
func (c *Conn) CDP(ctx context.Context, session, method string, params any, out any) error {
	if params == nil {
		params = map[string]any{}
	}
	var res struct {
		Result json.RawMessage `json:"result"`
	}
	if err := c.Call(ctx, "goog:cdp.sendCommand", map[string]any{"method": method, "params": params, "session": session}, &res); err != nil {
		return err
	}
	if out == nil || len(res.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(res.Result, out); err != nil {
		return fmt.Errorf("bidi: decode %s result: %w", method, err)
	}
	return nil
}
