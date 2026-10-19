package bidi

import "context"

// ContextInfo describes one browsing context (a tab or frame).
type ContextInfo struct {
	Context  string        `json:"context"`
	URL      string        `json:"url"`
	Parent   string        `json:"parent,omitempty"`
	Children []ContextInfo `json:"children,omitempty"`
}

// GetTree lists the open browsing contexts.
func (c *Conn) GetTree(ctx context.Context) ([]ContextInfo, error) {
	var out struct {
		Contexts []ContextInfo `json:"contexts"`
	}
	err := c.Call(ctx, "browsingContext.getTree", nil, &out)
	return out.Contexts, err
}

// CreateContext opens a new tab and returns its context ID.
func (c *Conn) CreateContext(ctx context.Context) (string, error) {
	var out struct {
		Context string `json:"context"`
	}
	err := c.Call(ctx, "browsingContext.create", map[string]any{"type": "tab"}, &out)
	return out.Context, err
}

// CloseContext closes a tab.
func (c *Conn) CloseContext(ctx context.Context, context string) error {
	return c.Call(ctx, "browsingContext.close", map[string]any{"context": context}, nil)
}

// NavigateResult is the result of browsingContext.navigate.
type NavigateResult struct {
	Navigation string `json:"navigation"`
	URL        string `json:"url"`
}

// Navigate loads url in the context and waits for the load event.
func (c *Conn) Navigate(ctx context.Context, context, url string) (NavigateResult, error) {
	var out NavigateResult
	err := c.Call(ctx, "browsingContext.navigate", map[string]any{
		"context": context, "url": url, "wait": "complete",
	}, &out)
	return out, err
}

// Reload reloads the context and waits for the load event.
func (c *Conn) Reload(ctx context.Context, context string) error {
	return c.Call(ctx, "browsingContext.reload", map[string]any{"context": context, "wait": "complete"}, nil)
}

// TraverseHistory moves delta entries through the context's history. -1 is back.
func (c *Conn) TraverseHistory(ctx context.Context, context string, delta int) error {
	return c.Call(ctx, "browsingContext.traverseHistory", map[string]any{"context": context, "delta": delta}, nil)
}

// SetViewport sets the size of the viewport in CSS pixels.
func (c *Conn) SetViewport(ctx context.Context, context string, width, height int) error {
	return c.Call(ctx, "browsingContext.setViewport", map[string]any{
		"context":  context,
		"viewport": map[string]any{"width": width, "height": height},
	}, nil)
}

// CaptureScreenshot returns the viewport as a PNG, base64 encoded.
func (c *Conn) CaptureScreenshot(ctx context.Context, context string) (string, error) {
	var out struct {
		Data string `json:"data"`
	}
	err := c.Call(ctx, "browsingContext.captureScreenshot", map[string]any{
		"context": context,
		"origin":  "viewport",
		"format":  map[string]any{"type": "image/png"},
	}, &out)
	return out.Data, err
}

// NodeRef is a node handle returned by locateNodes, usable as a script argument.
type NodeRef struct {
	SharedID string `json:"sharedId"`
}

// LocateNodes finds elements by a CSS selector and returns their shared IDs.
func (c *Conn) LocateNodes(ctx context.Context, context, css string, limit int) ([]NodeRef, error) {
	params := map[string]any{
		"context": context,
		"locator": map[string]any{"type": "css", "value": css},
	}
	if limit > 0 {
		params["maxNodeCount"] = limit
	}
	var out struct {
		Nodes []struct {
			SharedID string `json:"sharedId"`
		} `json:"nodes"`
	}
	if err := c.Call(ctx, "browsingContext.locateNodes", params, &out); err != nil {
		return nil, err
	}
	refs := make([]NodeRef, 0, len(out.Nodes))
	for _, n := range out.Nodes {
		refs = append(refs, NodeRef{SharedID: n.SharedID})
	}
	return refs, nil
}

// SetUserAgentOverride replaces navigator.userAgent for the given contexts
// through emulation.setUserAgentOverride.
func (c *Conn) SetUserAgentOverride(ctx context.Context, userAgent string, contexts []string) error {
	params := map[string]any{"userAgent": userAgent}
	if len(contexts) > 0 {
		params["contexts"] = contexts
	}
	return c.Call(ctx, "emulation.setUserAgentOverride", params, nil)
}

// SetViewportScaled sets the viewport to width by height CSS pixels and the
// device pixel ratio to ratio. A ratio of 2 with a viewport half the window's
// size is how a browser behaves at 200 percent page zoom: the page lays out in
// fewer CSS pixels and each one covers more screen pixels. The command is
// browsingContext.setViewport with its devicePixelRatio parameter; a ratio of
// 0 resets the ratio to the browser's own.
func (c *Conn) SetViewportScaled(ctx context.Context, context string, width, height int, ratio float64) error {
	params := map[string]any{
		"context":  context,
		"viewport": map[string]any{"width": width, "height": height},
	}
	if ratio > 0 {
		params["devicePixelRatio"] = ratio
	} else {
		params["devicePixelRatio"] = nil
	}
	return c.Call(ctx, "browsingContext.setViewport", params, nil)
}
