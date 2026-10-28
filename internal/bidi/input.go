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

// modifierNames maps the modifier names a persona may use (lower case) to codes.
var modifierNames = map[string]string{
	"shift": KeyShift, "control": KeyControl, "ctrl": KeyControl, "alt": KeyAlt, "option": KeyAlt,
	"meta": KeyMeta, "cmd": KeyMeta, "command": KeyMeta,
}

// ModifierNames lists the modifiers Chord accepts, for help text.
var ModifierNames = []string{"Shift", "Control", "Alt", "Meta"}

// Chord resolves a key name with any modifiers held before it, joined by +, such as "Shift+Tab" or
// "Control+Alt+Delete", to the modifiers' codes and the key's code. A key name alone, as Key takes it, is a chord
// with no modifiers, and "+" on its own, or after a modifier, is the plus key.
func Chord(name string) (mods []string, key string, err error) {
	keyName, rest, joined := name, "", false
	if len(name) > 1 {
		if i := strings.LastIndex(name[:len(name)-1], "+"); i >= 0 {
			rest, keyName, joined = name[:i], name[i+1:], true
		}
	}
	if joined {
		seen := map[string]bool{}
		for _, m := range strings.Split(rest, "+") {
			code, ok := modifierNames[strings.ToLower(strings.TrimSpace(m))]
			if !ok {
				return nil, "", fmt.Errorf("bidi: unknown modifier %q in %q; use %s", m, name, strings.Join(ModifierNames, ", "))
			}
			if seen[code] {
				return nil, "", fmt.Errorf("bidi: %q holds %s twice", name, strings.TrimSpace(m))
			}
			seen[code] = true
			mods = append(mods, code)
		}
	}
	if joined {
		keyName = strings.TrimSpace(keyName)
	}
	if key, err = Key(keyName); err != nil {
		return nil, "", err
	}
	return mods, key, nil
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

// MoveMouse moves the mouse to (x, y) in the viewport without pressing a button.
func (c *Conn) MoveMouse(ctx context.Context, context string, x, y float64) error {
	return c.pointer(ctx, context, map[string]any{"type": "pointerMove", "x": int(x), "y": int(y)})
}

// PressMouse clicks where the mouse already is.
func (c *Conn) PressMouse(ctx context.Context, context string) error {
	return c.pointer(ctx, context, map[string]any{"type": "pointerDown", "button": 0}, map[string]any{"type": "pointerUp", "button": 0})
}

func (c *Conn) pointer(ctx context.Context, context string, acts ...any) error {
	return c.Call(ctx, "input.performActions", map[string]any{
		"context": context,
		"actions": []any{map[string]any{
			"type": "pointer", "id": "mouse", "parameters": map[string]any{"pointerType": "mouse"},
			"actions": acts,
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

// PressChord holds the modifiers down in order, presses and releases the key, then lets the modifiers go in reverse.
// If the call fails part way, a modifier could stay held for every key after it, so PressChord releases what is
// held before it returns the error. If that release fails too, the error says so on the same line, since later keys
// may then come with a modifier still held and a page action reports only an error's first line.
func (c *Conn) PressChord(ctx context.Context, context string, mods []string, code string) error {
	err := c.keyActions(ctx, context, chordActions(mods, code))
	if err != nil && len(mods) > 0 {
		if rerr := c.releaseHeld(ctx, context); rerr != nil {
			err = fmt.Errorf("%w; bidi: release the held keys: %w", err, rerr)
		}
	}
	return err
}

// chordActions is the key actions PressChord sends.
func chordActions(mods []string, code string) []any {
	acts := make([]any, 0, 2*len(mods)+2)
	for _, m := range mods {
		acts = append(acts, map[string]any{"type": "keyDown", "value": m})
	}
	acts = append(acts,
		map[string]any{"type": "keyDown", "value": code},
		map[string]any{"type": "keyUp", "value": code})
	for i := len(mods) - 1; i >= 0; i-- {
		acts = append(acts, map[string]any{"type": "keyUp", "value": mods[i]})
	}
	return acts
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

// releaseHeld releases any key still held after a failed key sequence. Its own deadline outlives the failed call's,
// so the release is still sent when that call ran out of time.
func (c *Conn) releaseHeld(parent context.Context, cx string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), DefaultTimeout)
	defer cancel()
	return c.ReleaseActions(ctx, cx)
}
