package bidi

import (
	"context"
	"encoding/json"
	"strings"
)

// Header is one request header as the network module serialises it.
type Header struct {
	Name  string     `json:"name"`
	Value BytesValue `json:"value"`
}

// BytesValue is a header value. This client only sends string values.
type BytesValue struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// StringHeader builds a header with a string value.
func StringHeader(name, value string) Header {
	return Header{Name: name, Value: BytesValue{Type: "string", Value: value}}
}

// RequestData describes the request in a network event.
type RequestData struct {
	Request string   `json:"request"`
	URL     string   `json:"url"`
	Method  string   `json:"method"`
	Headers []Header `json:"headers"`
	// Destination is the fetch destination, such as document, iframe, image, style or script, where the
	// browser reports it. It is empty when it does not.
	Destination string `json:"destination,omitempty"`
	// InitiatorType says what started the request, such as fetch, xmlhttprequest, link, img or script, where the
	// browser reports it. It is empty when it does not.
	InitiatorType string `json:"initiatorType,omitempty"`
}

// BeforeRequestSent is a network.beforeRequestSent event.
type BeforeRequestSent struct {
	Context     string      `json:"context"`
	IsBlocked   bool        `json:"isBlocked"`
	Navigation  string      `json:"navigation"`
	Request     RequestData `json:"request"`
	Intercepts  []string    `json:"intercepts"`
	RedirectCnt int         `json:"redirectCount"`
}

// ResponseCompleted is a network.responseCompleted event.
type ResponseCompleted struct {
	Context    string      `json:"context"`
	Navigation string      `json:"navigation"`
	Request    RequestData `json:"request"`
	Response   struct {
		URL      string `json:"url"`
		Status   int    `json:"status"`
		MimeType string `json:"mimeType"`
	} `json:"response"`
}

// FetchError is a network.fetchError event: a request that never got a response.
type FetchError struct {
	Context   string      `json:"context"`
	Request   RequestData `json:"request"`
	ErrorText string      `json:"errorText"`
}

// AddIntercept registers an intercept for the beforeRequestSent phase on the
// given hostnames. With no hostnames, every request is intercepted.
func (c *Conn) AddIntercept(ctx context.Context, hostnames []string) (string, error) {
	return c.AddInterceptPhases(ctx, hostnames, "beforeRequestSent")
}

// AddInterceptPhases is AddIntercept for the given phases, such as "beforeRequestSent" and "authRequired".
// A blocked authRequired request waits for ContinueWithAuth.
func (c *Conn) AddInterceptPhases(ctx context.Context, hostnames []string, phases ...string) (string, error) {
	params := map[string]any{"phases": phases}
	if len(hostnames) > 0 {
		patterns := make([]any, 0, len(hostnames))
		for _, h := range hostnames {
			patterns = append(patterns, map[string]any{"type": "pattern", "hostname": h})
		}
		params["urlPatterns"] = patterns
	}
	var out struct {
		Intercept string `json:"intercept"`
	}
	err := c.Call(ctx, "network.addIntercept", params, &out)
	return out.Intercept, err
}

// AddInterceptInContexts is AddIntercept for every request in the given top-level browsing contexts and the
// frames inside them.
func (c *Conn) AddInterceptInContexts(ctx context.Context, contexts []string, phases ...string) (string, error) {
	var out struct {
		Intercept string `json:"intercept"`
	}
	err := c.Call(ctx, "network.addIntercept", map[string]any{"phases": phases, "contexts": contexts}, &out)
	return out.Intercept, err
}

// RemoveIntercept removes an intercept.
func (c *Conn) RemoveIntercept(ctx context.Context, intercept string) error {
	return c.Call(ctx, "network.removeIntercept", map[string]any{"intercept": intercept}, nil)
}

// ContinueRequest lets a blocked request through with the merged header list.
//
// The spec says the headers parameter replaces the request's headers, and
// Firefox does exactly that: send only the new headers and Host is gone. The
// original headers are therefore merged with the additions, so the request
// keeps everything the browser set.
func (c *Conn) ContinueRequest(ctx context.Context, request string, original, set []Header) error {
	return c.Call(ctx, "network.continueRequest", map[string]any{
		"request": request,
		"headers": MergeHeaders(original, set),
	}, nil)
}

// ContinueRequestUnchanged lets a blocked request through as it is.
func (c *Conn) ContinueRequestUnchanged(ctx context.Context, request string) error {
	return c.Call(ctx, "network.continueRequest", map[string]any{"request": request}, nil)
}

// FailRequest makes a blocked request fail as a network error.
func (c *Conn) FailRequest(ctx context.Context, request string) error {
	return c.Call(ctx, "network.failRequest", map[string]any{"request": request}, nil)
}

// MergeHeaders returns original with each header in set added, replacing any
// original header of the same name. Header names compare case-insensitively.
// The order of original is kept; new names go at the end.
func MergeHeaders(original, set []Header) []Header {
	replace := make(map[string]Header, len(set))
	for _, h := range set {
		replace[strings.ToLower(h.Name)] = h
	}
	out := make([]Header, 0, len(original)+len(set))
	seen := make(map[string]bool, len(set))
	for _, h := range original {
		key := strings.ToLower(h.Name)
		if r, ok := replace[key]; ok {
			if !seen[key] {
				out = append(out, r)
				seen[key] = true
			}
			continue
		}
		out = append(out, h)
	}
	for _, h := range set {
		key := strings.ToLower(h.Name)
		if !seen[key] {
			out = append(out, h)
			seen[key] = true
		}
	}
	return out
}

// HeaderValue returns the value of the named header, case-insensitively, and
// whether it was present.
func HeaderValue(headers []Header, name string) (string, bool) {
	for _, h := range headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value.Value, true
		}
	}
	return "", false
}

// OnBeforeRequestSent calls f for every network.beforeRequestSent event. The returned function stops it.
func (c *Conn) OnBeforeRequestSent(f func(BeforeRequestSent)) (remove func()) {
	return c.OnEvent("network.beforeRequestSent", func(_ string, params json.RawMessage) {
		var e BeforeRequestSent
		if err := json.Unmarshal(params, &e); err != nil {
			return
		}
		f(e)
	})
}

// OnResponseCompleted calls f for every network.responseCompleted event.
func (c *Conn) OnResponseCompleted(f func(ResponseCompleted)) {
	c.OnEvent("network.responseCompleted", func(_ string, params json.RawMessage) {
		var e ResponseCompleted
		if err := json.Unmarshal(params, &e); err != nil {
			return
		}
		f(e)
	})
}

// OnFetchError calls f for every network.fetchError event.
func (c *Conn) OnFetchError(f func(FetchError)) {
	c.OnEvent("network.fetchError", func(_ string, params json.RawMessage) {
		var e FetchError
		if err := json.Unmarshal(params, &e); err != nil {
			return
		}
		f(e)
	})
}

// AuthRequired is a network.authRequired event: the server answered 401 or 407 with a challenge.
type AuthRequired struct {
	Context   string      `json:"context"`
	IsBlocked bool        `json:"isBlocked"`
	Request   RequestData `json:"request"`
	Response  struct {
		URL    string `json:"url"`
		Status int    `json:"status"`
	} `json:"response"`
}

// OnAuthRequired calls f for every network.authRequired event.
func (c *Conn) OnAuthRequired(f func(AuthRequired)) {
	c.OnEvent("network.authRequired", func(_ string, params json.RawMessage) {
		var e AuthRequired
		if err := json.Unmarshal(params, &e); err != nil {
			return
		}
		f(e)
	})
}

// ProvideCredentials answers a blocked authentication challenge with a user name and password.
func (c *Conn) ProvideCredentials(ctx context.Context, request, username, password string) error {
	return c.Call(ctx, "network.continueWithAuth", map[string]any{
		"request": request,
		"action":  "provideCredentials",
		"credentials": map[string]any{
			"type": "password", "username": username, "password": password,
		},
	}, nil)
}

// CancelAuth refuses a blocked authentication challenge. The page gets the 401 response.
func (c *Conn) CancelAuth(ctx context.Context, request string) error {
	return c.Call(ctx, "network.continueWithAuth", map[string]any{"request": request, "action": "cancel"}, nil)
}
