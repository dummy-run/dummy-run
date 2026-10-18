package bidi

import "context"

// Capabilities is the WebDriver capabilities object passed to session.new.
type Capabilities struct {
	AlwaysMatch map[string]any `json:"alwaysMatch,omitempty"`
}

// SessionNewResult is the result of session.new.
type SessionNewResult struct {
	SessionID    string         `json:"sessionId"`
	Capabilities map[string]any `json:"capabilities"`
}

// SessionNew starts a session on a remote end that is not yet in one, such as
// Firefox launched directly. A chromedriver connection already has a session.
func (c *Conn) SessionNew(ctx context.Context, caps Capabilities) (SessionNewResult, error) {
	var out SessionNewResult
	err := c.Call(ctx, "session.new", map[string]any{"capabilities": caps}, &out)
	return out, err
}

// SessionStatus is the result of session.status.
type SessionStatus struct {
	Ready   bool   `json:"ready"`
	Message string `json:"message"`
}

// Status asks the remote end whether it can take a new session.
func (c *Conn) Status(ctx context.Context) (SessionStatus, error) {
	var out SessionStatus
	err := c.Call(ctx, "session.status", nil, &out)
	return out, err
}

// Subscribe asks for the named events. Without contexts, events for every
// browsing context arrive.
func (c *Conn) Subscribe(ctx context.Context, events []string, contexts ...string) error {
	params := map[string]any{"events": events}
	if len(contexts) > 0 {
		params["contexts"] = contexts
	}
	return c.Call(ctx, "session.subscribe", params, nil)
}

// SessionEnd ends the session. The browser closes with it.
func (c *Conn) SessionEnd(ctx context.Context) error {
	return c.Call(ctx, "session.end", nil, nil)
}

// BrowserClose closes the browser.
func (c *Conn) BrowserClose(ctx context.Context) error {
	return c.Call(ctx, "browser.close", nil, nil)
}
