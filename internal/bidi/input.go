package bidi

import (
	"context"
	"fmt"
	"strings"
)

// Key codes from the WebDriver spec, section "Keyboard actions". A key action's
// value is one of these or a single printable character.
const (
	KeyBackspace  = ""
	KeyTab        = ""
	KeyEnter      = ""
	KeyShift      = ""
	KeyControl    = ""
	KeyAlt        = ""
	KeyEscape     = ""
	KeySpace      = ""
	KeyPageUp     = ""
	KeyPageDown   = ""
	KeyEnd        = ""
	KeyHome       = ""
	KeyArrowLeft  = ""
	KeyArrowUp    = ""
	KeyArrowRight = ""
	KeyArrowDown  = ""
	KeyDelete     = ""
	KeyMeta       = ""
)

// keyNames maps the names a persona may use (lower case, no spaces) to codes.
var keyNames = map[string]string{
	"enter": KeyEnter, "return": KeyEnter, "escape": KeyEscape, "esc": KeyEscape, "tab": KeyTab,
	"backspace": KeyBackspace, "delete": KeyDelete, "space": KeySpace, "home": KeyHome, "end": KeyEnd,
	"pageup": KeyPageUp, "pagedown": KeyPageDown,
	"arrowdown": KeyArrowDown, "arrowup": KeyArrowUp, "arrowleft": KeyArrowLeft, "arrowright": KeyArrowRight,
	"down": KeyArrowDown, "up": KeyArrowUp, "left": KeyArrowLeft, "right": KeyArrowRight,
}

// KeyNames lists the key names Key accepts, for help text.
var KeyNames = []string{"ArrowDown", "ArrowUp", "ArrowLeft", "ArrowRight", "Enter", "Escape", "Tab",
	"Backspace", "Delete", "Space", "Home", "End", "PageUp", "PageDown"}

// Key resolves a key name such as "ArrowDown" or "enter" to its code. A single
// character is returned as itself.
func Key(name string) (string, error) {
	if len([]rune(name)) == 1 {
		return name, nil
	}
	code, ok := keyNames[strings.ToLower(strings.NewReplacer(" ", "", "_", "", "-", "").Replace(name))]
	if !ok {
		return "", fmt.Errorf("bidi: unknown key %q; use one of %s", name, strings.Join(KeyNames, ", "))
	}
	return code, nil
}

// Click moves the mouse to (x, y) in the viewport and clicks.
func (c *Conn) Click(ctx context.Context, context string, x, y float64) error {
	return c.Call(ctx, "input.performActions", map[string]any{
		"context": context,
		"actions": []any{map[string]any{
			"type": "pointer", "id": "mouse", "parameters": map[string]any{"pointerType": "mouse"},
			"actions": []any{
				map[string]any{"type": "pointerMove", "x": int(x), "y": int(y)},
				map[string]any{"type": "pointerDown", "button": 0},
				map[string]any{"type": "pointerUp", "button": 0},
			},
		}},
	}, nil)
}

// TypeText presses and releases one key per character of text. Keys go to
// whatever has focus.
func (c *Conn) TypeText(ctx context.Context, context, text string) error {
	acts := make([]any, 0, 2*len(text))
	for _, r := range text {
		v := string(r)
		if r == '\n' {
			v = KeyEnter
		}
		acts = append(acts,
			map[string]any{"type": "keyDown", "value": v},
			map[string]any{"type": "keyUp", "value": v})
	}
	return c.keyActions(ctx, context, acts)
}

// PressKey presses and releases one key by code.
func (c *Conn) PressKey(ctx context.Context, context, code string) error {
	return c.keyActions(ctx, context, []any{
		map[string]any{"type": "keyDown", "value": code},
		map[string]any{"type": "keyUp", "value": code},
	})
}

// PressKeyTimes presses one key n times.
func (c *Conn) PressKeyTimes(ctx context.Context, context, code string, n int) error {
	acts := make([]any, 0, 2*n)
	for range n {
		acts = append(acts,
			map[string]any{"type": "keyDown", "value": code},
			map[string]any{"type": "keyUp", "value": code})
	}
	return c.keyActions(ctx, context, acts)
}

func (c *Conn) keyActions(ctx context.Context, context string, acts []any) error {
	if len(acts) == 0 {
		return nil
	}
	return c.Call(ctx, "input.performActions", map[string]any{
		"context": context,
		"actions": []any{map[string]any{"type": "key", "id": "keyboard", "actions": acts}},
	}, nil)
}

// ReleaseActions releases any key or button still held.
func (c *Conn) ReleaseActions(ctx context.Context, context string) error {
	return c.Call(ctx, "input.releaseActions", map[string]any{"context": context}, nil)
}
