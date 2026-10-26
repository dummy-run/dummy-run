// Package journey finds the journeys a page offers, such as a search, a menu, a basket or a contact form, and drives
// each one by mouse and by keyboard. A journey has a success test that must be false when the page loads and true
// after the action, so each verdict is computed, not judged. Nothing is ever submitted. No model is involved.
package journey

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/dummy-run/dummy-run/internal/bidi"
	"github.com/dummy-run/dummy-run/internal/tech"
)

//go:embed journey.js
var journeyJS string

// Kinds of journey, in the order they are tried.
var Kinds = []string{"search", "menu", "signup", "basket", "booking", "contact"}

// techKinds maps the journeys a technology suggests to the kinds here.
var techKinds = map[string]string{"search": "search", "basket": "basket", "checkout": "basket", "booking": "booking", "contact form": "contact"}

// Outcomes of one way of trying a journey.
const (
	Pass        = "pass"        // the success test was false before and true after
	Fail        = "fail"        // the action happened and the success test stayed false
	Unreachable = "unreachable" // the mouse couldn't hit it or Tab never reached it
	Already     = "already"     // the success test was true before anything was done, so the journey proves nothing
	NotFound    = "not found"   // a technology suggested the journey, but the page shows no way in
)

// Verdict is how one way of trying went.
type Verdict struct {
	Outcome string `json:"outcome"`
	Detail  string `json:"detail,omitempty"`
}

// Journey is one journey the page offers, and how it went by mouse and by keyboard.
type Journey struct {
	Kind     string `json:"kind"`
	Selector string `json:"selector,omitempty"`
	Label    string `json:"label,omitempty"`
	Href     string `json:"href,omitempty"`
	// Source says how the journey was found: dom, schema.org, or tech:<name> for one a technology suggested.
	Source   string   `json:"source"`
	Mouse    *Verdict `json:"mouse,omitempty"`
	Keyboard *Verdict `json:"keyboard,omitempty"`
}

// KeyboardShortfall reports whether the journey works by mouse but not by keyboard.
func (j Journey) KeyboardShortfall() bool {
	return j.Mouse != nil && j.Keyboard != nil && j.Mouse.Outcome == Pass && j.Keyboard.Outcome != Pass
}

// Driver runs journeys in one tab.
type Driver struct {
	Conn *bidi.Conn
	CX   string
	// Settle is how long to wait after an action before the success test. Default 800 ms.
	Settle time.Duration
}

// searchText is what the search journey types. It is never sent.
const searchText = "opening hours"

// maxTabs is how many times the keyboard presses Tab looking for the journey's start.
const maxTabs = 250

func (d *Driver) call(ctx context.Context, op string, arg any, out any) error {
	b, err := json.Marshal(arg)
	if err != nil {
		return err
	}
	return d.Conn.CallFunctionJSON(ctx, d.CX, journeyJS, out, op, string(b))
}

// load opens the page fresh, with focus inside it, and stops its forms from being sent.
func (d *Driver) load(ctx context.Context, url string) error {
	if _, err := d.Conn.Navigate(ctx, d.CX, url); err != nil {
		return err
	}
	// Firefox can leave focus in its own window after an earlier Tab walk ran off the page. Pressing Tab brings it
	// back, and loading again starts the page from the top with focus inside it.
	var has bool
	for i := 0; i < 10; i++ {
		if err := d.Conn.CallFunctionJSON(ctx, d.CX, "() => document.hasFocus()", &has); err != nil || has {
			break
		}
		_ = d.Conn.PressKey(ctx, d.CX, bidi.KeyTab)
	}
	if _, err := d.Conn.Navigate(ctx, d.CX, url); err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	var ok bool
	return d.call(ctx, "guard", struct{}{}, &ok)
}

// Find lists the journeys the page at url offers. found is what the page is built with: a journey a technology
// suggests that the page shows no way into is listed as not found.
func (d *Driver) Find(ctx context.Context, url string, found []tech.Found) ([]Journey, error) {
	if err := d.load(ctx, url); err != nil {
		return nil, err
	}
	var js []Journey
	if err := d.call(ctx, "find", struct{}{}, &js); err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, j := range js {
		have[j.Kind] = true
	}
	for _, f := range found {
		for _, s := range tech.Journeys([]tech.Found{f}) {
			k := techKinds[s]
			if k == "" || have[k] {
				continue
			}
			have[k] = true
			js = append(js, Journey{Kind: k, Source: "tech:" + f.Name, Mouse: &Verdict{Outcome: NotFound}, Keyboard: &Verdict{Outcome: NotFound}})
		}
	}
	slices.SortStableFunc(js, func(a, b Journey) int { return slices.Index(Kinds, a.Kind) - slices.Index(Kinds, b.Kind) })
	return js, nil
}

type spec struct {
	Kind  string `json:"kind"`
	Sel   string `json:"sel"`
	Href  string `json:"href,omitempty"`
	Typed string `json:"typed,omitempty"`
}

func (d *Driver) check(ctx context.Context, s spec) bool {
	var ok bool
	if err := d.call(ctx, "check", s, &ok); err != nil {
		return false
	}
	return ok
}

// Try drives the journey by mouse and then by keyboard, each from a fresh load of the page.
func (d *Driver) Try(ctx context.Context, url string, j *Journey) {
	if j.Mouse != nil {
		return // not found on the page
	}
	j.Mouse = d.try(ctx, url, *j, false)
	j.Keyboard = d.try(ctx, url, *j, true)
}

func (d *Driver) try(ctx context.Context, url string, j Journey, keyboard bool) *Verdict {
	if err := d.load(ctx, url); err != nil {
		return &Verdict{Outcome: Fail, Detail: "the page did not load: " + err.Error()}
	}
	// Some pages give elements a new id on every load, so the start is found again on this load.
	var fresh []Journey
	if err := d.call(ctx, "find", struct{}{}, &fresh); err == nil {
		for _, f := range fresh {
			if f.Kind == j.Kind {
				j.Selector = f.Selector
				break
			}
		}
	}
	s := spec{Kind: j.Kind, Sel: j.Selector, Href: j.Href}
	if j.Kind == "search" {
		s.Typed = searchText
	}
	if d.check(ctx, s) {
		return &Verdict{Outcome: Already}
	}
	if v := d.reach(ctx, j, keyboard); v != nil {
		return v
	}
	how := "clicked"
	if keyboard {
		how = "pressed Enter on"
	}
	switch j.Kind {
	case "search":
		if err := d.Conn.TypeText(ctx, d.CX, searchText); err != nil {
			return &Verdict{Outcome: Fail, Detail: "typing failed: " + err.Error()}
		}
		how = "typed into"
	default:
		var err error
		if keyboard {
			err = d.Conn.PressKey(ctx, d.CX, bidi.KeyEnter)
		} else {
			var at struct {
				X, Y float64
			}
			_ = d.call(ctx, "locate", spec{Sel: j.Selector}, &at)
			err = d.Conn.Click(ctx, d.CX, at.X, at.Y)
		}
		if err != nil {
			return &Verdict{Outcome: Fail, Detail: err.Error()}
		}
	}
	if !d.settle(ctx, s) {
		return &Verdict{Outcome: Fail, Detail: fmt.Sprintf("%s %q and %s", how, j.Label, failed(j.Kind))}
	}
	if j.Kind == "contact" {
		return d.fill(ctx, keyboard)
	}
	return &Verdict{Outcome: Pass}
}

// settle waits for the success test, up to a few seconds for a journey that loads another page.
func (d *Driver) settle(ctx context.Context, s spec) bool {
	wait := d.Settle
	if wait == 0 {
		wait = 800 * time.Millisecond
	}
	deadline := time.Now().Add(wait)
	if s.Href != "" {
		deadline = time.Now().Add(8 * time.Second)
	}
	for {
		time.Sleep(200 * time.Millisecond)
		if d.check(ctx, s) {
			return true
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return false
		}
	}
}

// reach gets the journey's start element ready: in view and hittable for the mouse, or focused by Tab for the
// keyboard. It returns a verdict only when the element can't be reached.
func (d *Driver) reach(ctx context.Context, j Journey, keyboard bool) *Verdict {
	if keyboard {
		return d.tabTo(ctx, j)
	}
	var at struct {
		Found    bool `json:"found"`
		Hittable bool `json:"hittable"`
		X, Y     float64
	}
	if err := d.call(ctx, "locate", spec{Sel: j.Selector}, &at); err != nil || !at.Found {
		return &Verdict{Outcome: Unreachable, Detail: "the start of the journey wasn't on the page"}
	}
	if !at.Hittable {
		return &Verdict{Outcome: Unreachable, Detail: "something covers it, so a click lands elsewhere"}
	}
	if j.Kind == "search" {
		if err := d.Conn.Click(ctx, d.CX, at.X, at.Y); err != nil {
			return &Verdict{Outcome: Fail, Detail: err.Error()}
		}
	}
	return nil
}

// tabTo presses Tab from the top of the freshly loaded page until the journey's start has focus. Nothing is scrolled
// first: Chrome starts Tab from where the page was last scrolled to, so a scroll would skip what's above it.
func (d *Driver) tabTo(ctx context.Context, j Journey) *Verdict {
	var present bool
	if err := d.call(ctx, "present", spec{Sel: j.Selector}, &present); err != nil || !present {
		return &Verdict{Outcome: Unreachable, Detail: "the start of the journey wasn't on the page"}
	}
	for i := 0; i < maxTabs; i++ {
		if err := d.Conn.PressKey(ctx, d.CX, bidi.KeyTab); err != nil {
			return &Verdict{Outcome: Fail, Detail: err.Error()}
		}
		var on bool
		if err := d.call(ctx, "focused", spec{Sel: j.Selector}, &on); err == nil && on {
			return nil
		}
	}
	return &Verdict{Outcome: Unreachable, Detail: fmt.Sprintf("Tab never reached %q in %d presses", j.Label, maxTabs)}
}

// fill types sample text into each field of the contact form, by Tab or by click, and checks each took it. The
// form is never sent.
func (d *Driver) fill(ctx context.Context, keyboard bool) *Verdict {
	var fields []string
	if err := d.call(ctx, "fields", struct{}{}, &fields); err != nil || len(fields) == 0 {
		return &Verdict{Outcome: Pass, Detail: "reached the form; it has no text fields to fill"}
	}
	filled := 0
	for i, sel := range fields {
		if keyboard {
			reached := false
			for t := 0; t < 40 && !reached; t++ {
				if err := d.Conn.PressKey(ctx, d.CX, bidi.KeyTab); err != nil {
					break
				}
				_ = d.call(ctx, "focused", spec{Sel: sel}, &reached)
			}
			if !reached {
				continue
			}
		} else {
			var at struct {
				Found    bool `json:"found"`
				Hittable bool `json:"hittable"`
				X, Y     float64
			}
			if err := d.call(ctx, "locate", spec{Sel: sel}, &at); err != nil || !at.Found || !at.Hittable {
				continue
			}
			if err := d.Conn.Click(ctx, d.CX, at.X, at.Y); err != nil {
				continue
			}
		}
		text := fmt.Sprintf("Dummy Run %d", i+1)
		if err := d.Conn.TypeText(ctx, d.CX, text); err != nil {
			continue
		}
		var v string
		if err := d.call(ctx, "value", spec{Sel: sel}, &v); err == nil && v != "" {
			filled++
		}
	}
	detail := fmt.Sprintf("filled %d of %d fields, and did not send the form", filled, len(fields))
	if filled < len(fields) {
		return &Verdict{Outcome: Fail, Detail: detail}
	}
	return &Verdict{Outcome: Pass, Detail: detail}
}

func failed(kind string) string {
	switch kind {
	case "menu":
		return "the menu did not open"
	case "search":
		return "the text did not appear in the search box"
	case "signup":
		return "no sign-up form appeared"
	case "basket":
		return "no basket page appeared"
	case "booking":
		return "no booking form or calendar appeared"
	case "contact":
		return "no contact form appeared"
	}
	return "nothing changed"
}
