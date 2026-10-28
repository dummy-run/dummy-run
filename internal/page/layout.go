package page

import (
	"context"
	_ "embed"
	"fmt"
	"regexp"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

//go:embed layout.js
var layoutJS string

// Layout limits. They match WCAG 2.2 SC 2.5.8 and the 2 px that the checks ignore.
const (
	// MinTargetPx is the smallest side of a target in CSS pixels (WCAG 2.5.8, level AA).
	MinTargetPx = 24
	// LayoutTolerancePx is how far two boxes may overlap, or text may be clipped, before it counts.
	LayoutTolerancePx = 2
	// LayoutMaxItems limits each list of facts in one measurement.
	LayoutMaxItems = 50
)

// LayoutBox is a rectangle in CSS pixels of the whole page, from the top left of the document.
type LayoutBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// LayoutElement names one element: its selector, its @number from the last look if it has one, and its text.
type LayoutElement struct {
	Selector string `json:"selector"`
	Ref      string `json:"ref,omitempty"`
	Label    string `json:"label,omitempty"`
}

// LayoutOverflow says the page scrolls sideways.
type LayoutOverflow struct {
	ScrollWidth int `json:"scroll_width"`
	ClientWidth int `json:"client_width"`
	// Excess is how many CSS pixels the page is wider than the window.
	Excess float64 `json:"excess"`
	// Culprits are the widest outermost elements that reach past the window.
	Culprits []struct {
		LayoutElement
		Right float64 `json:"right"`
	} `json:"culprits"`
}

// LayoutClipped is an element whose text is partly cut off by a box that hides its overflow. When the text is in a
// control, the element is the control and Control is true.
type LayoutClipped struct {
	LayoutElement
	Control bool      `json:"control,omitempty"`
	Detail  string    `json:"detail"`
	Box     LayoutBox `json:"box"`
}

// LayoutOverlap is a pair of controls where B is painted over A: at Covered of the Sampled points on A's text, or on
// its whole box when Text is false, the browser says B is on top.
type LayoutOverlap struct {
	A       LayoutElement `json:"a"`
	B       LayoutElement `json:"b"`
	Covered int           `json:"covered"`
	Sampled int           `json:"sampled"`
	Text    bool          `json:"text"`
	Box     LayoutBox     `json:"box"`
}

// LayoutObscured is a control that, once it has focus, is entirely hidden by By, an opaque element (WCAG 2.4.11).
// ByPosition is By's CSS position, such as sticky or fixed for a header that stays on screen.
type LayoutObscured struct {
	LayoutElement
	By         LayoutElement `json:"by"`
	ByPosition string        `json:"by_position"`
	Box        LayoutBox     `json:"box"`
}

// LayoutSmall is an interactive element below the minimum target size.
type LayoutSmall struct {
	LayoutElement
	Width  float64   `json:"width"`
	Height float64   `json:"height"`
	Box    LayoutBox `json:"box"`
}

// Layout is what the geometry checks found on one page at one window size and text size.
type Layout struct {
	Viewport   []int           `json:"viewport"`
	RootFontPx float64         `json:"root_font_px"`
	Controls   int             `json:"controls"`
	Overflow   *LayoutOverflow `json:"overflow"`
	// ReflowExempt counts elements past the window that WCAG 1.4.10 lets keep a two-dimensional layout, such as a
	// data table. When they are the only ones, Overflow is nil.
	ReflowExempt int             `json:"reflow_exempt"`
	Clipped      []LayoutClipped `json:"clipped"`
	ClippedN     int             `json:"clipped_count"`
	Overlaps     []LayoutOverlap `json:"overlaps"`
	OverlapN     int             `json:"overlap_count"`
	Small        []LayoutSmall   `json:"small"`
	SmallN       int             `json:"small_count"`
	// FocusChecked says focus was moved to each control (at most 150) to look for WCAG 2.4.11 failures, and
	// Focused how many took it.
	FocusChecked bool             `json:"focus_checked"`
	Focused      int              `json:"focused"`
	Obscured     []LayoutObscured `json:"obscured"`
	ObscuredN    int              `json:"obscured_count"`
}

// Problems is the number of things wrong: sideways overflow counts once.
func (l *Layout) Problems() int {
	if l == nil {
		return 0
	}
	n := l.ClippedN + l.OverlapN + l.SmallN + l.ObscuredN
	if l.Overflow != nil {
		n++
	}
	return n
}

// MeasureLayout runs the geometry checks in the tab and returns what they found. The page should
// have settled first. Elements that carry a data-dummyrun-ref from the last look keep that @number. With focus, it
// also moves focus to each control to look for focus hidden behind something else, which a page can react to, so a
// persona's look never asks for it.
func MeasureLayout(ctx context.Context, conn *bidi.Conn, cx string, focus bool) (*Layout, error) {
	var out Layout
	if err := conn.CallFunctionJSON(ctx, cx, layoutJS, &out, LayoutMaxItems, MinTargetPx, LayoutTolerancePx, focus); err != nil {
		return nil, fmt.Errorf("measure layout: %w", err)
	}
	return &out, nil
}

// LayoutNote is what an observation says about the layout, in terms a persona can act on. It names
// controls by their @number and never repeats a selector or a label, which come from the page.
type LayoutNote struct {
	// OverflowPx is how far the page is wider than the window, 0 when it is not.
	OverflowPx int `json:"overflow_px,omitempty"`
	// Overlaps counts pairs of controls where one is painted over the other's text.
	Overlaps int `json:"overlaps,omitempty"`
	// OverlapRefs lists some of those pairs as "@4 @7" for controls that have an @number.
	OverlapRefs []string `json:"overlap_refs,omitempty"`
	// Clipped counts controls whose text is cut off. Text cut off outside a control is left out.
	Clipped int `json:"clipped,omitempty"`
	// ClippedRefs lists the @numbers of some of them.
	ClippedRefs []string `json:"clipped_refs,omitempty"`
}

// refPattern is the @number the engine gives a control: digits only.
var refPattern = regexp.MustCompile(`^[0-9]{1,4}$`)

// Note reduces the layout to the facts an observation shows. It returns nil when there are none.
// Small targets are left out: on a phone they are common and would be noise on every step.
func (l *Layout) Note() *LayoutNote {
	if l == nil {
		return nil
	}
	n := &LayoutNote{Overlaps: l.OverlapN}
	if l.Overflow != nil {
		n.OverflowPx = int(l.Overflow.Excess + 0.5)
	}
	for _, o := range l.Overlaps {
		if len(n.OverlapRefs) < 5 && refPattern.MatchString(o.A.Ref) && refPattern.MatchString(o.B.Ref) {
			n.OverlapRefs = append(n.OverlapRefs, "@"+o.A.Ref+" @"+o.B.Ref)
		}
	}
	for _, c := range l.Clipped {
		if !c.Control {
			continue
		}
		n.Clipped++
		if len(n.ClippedRefs) < 5 && refPattern.MatchString(c.Ref) {
			n.ClippedRefs = append(n.ClippedRefs, "@"+c.Ref)
		}
	}
	if n.OverflowPx == 0 && n.Overlaps == 0 && n.Clipped == 0 {
		return nil
	}
	return n
}

// addLayout adds the layout facts to a snapshot. A page that cannot be measured keeps the rest.
func (p *Page) addLayout(ctx context.Context, snap *Snapshot) {
	l, err := MeasureLayout(ctx, p.conn, p.cx, false)
	if err != nil {
		return
	}
	snap.Layout = l.Note()
}
