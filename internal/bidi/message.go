// Package bidi is a typed WebDriver BiDi client for Chrome and Firefox.
//
// It covers the commands the Dummy Run engine uses: session, browsingContext,
// script, input, log, network and emulation. The protocol is one JSON message
// per WebSocket frame, as https://w3c.github.io/webdriver-bidi/ defines it.
package bidi

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Message is one frame from the remote end: a command result, a command
// error or an event.
type Message struct {
	// ID is the command ID a result or error answers. Events have none.
	ID *int64 `json:"id,omitempty"`
	// Type is "success", "error" or "event".
	Type string `json:"type"`
	// Method names the event, such as "log.entryAdded".
	Method string `json:"method,omitempty"`
	// Params carries an event's payload.
	Params json.RawMessage `json:"params,omitempty"`
	// Result carries a successful command's payload.
	Result json.RawMessage `json:"result,omitempty"`
	// Error is the protocol error code, such as "no such element".
	Error string `json:"error,omitempty"`
	// ErrorMessage is the remote end's explanation of the error.
	ErrorMessage string `json:"message,omitempty"`
}

// ErrDecode marks a frame the client could not make sense of.
var ErrDecode = errors.New("bidi: cannot decode message")

// Decode parses one frame and checks that it is a well formed result,
// error or event. The remote end's text is never trusted beyond that.
func Decode(data []byte) (Message, error) {
	var m Message
	if err := json.Unmarshal(data, &m); err != nil {
		return Message{}, fmt.Errorf("%w: %w", ErrDecode, err)
	}
	switch m.Type {
	case "success":
		if m.ID == nil {
			return Message{}, fmt.Errorf("%w: success without an id", ErrDecode)
		}
	case "error":
		if m.ID == nil && m.Error == "" {
			return Message{}, fmt.Errorf("%w: error without an id or a code", ErrDecode)
		}
	case "event":
		if m.Method == "" {
			return Message{}, fmt.Errorf("%w: event without a method", ErrDecode)
		}
	default:
		return Message{}, fmt.Errorf("%w: unknown type %q", ErrDecode, m.Type)
	}
	return m, nil
}

// CommandError is an error the remote end returned for a command.
type CommandError struct {
	Method  string
	Code    string
	Message string
}

func (e *CommandError) Error() string {
	return fmt.Sprintf("bidi: %s failed: %s: %s", e.Method, e.Code, e.Message)
}

// IsCommandError reports whether err is a CommandError with the given code.
func IsCommandError(err error, code string) bool {
	var ce *CommandError
	return errors.As(err, &ce) && ce.Code == code
}
