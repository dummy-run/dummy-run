package page

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"slices"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

//go:embed tabwalk.js
var tabwalkJS string

const tabSandbox = "dummyrun-keyboard"

// TabStop is the element that had keyboard focus after one press of Tab.
type TabStop struct {
	ID           int       `json:"id"`
	Selector     string    `json:"selector"`
	Tag          string    `json:"tag"`
	Name         string    `json:"name,omitempty"`
	Box          LayoutBox `json:"box"`
	Hidden       bool      `json:"hidden"`
	Offscreen    bool      `json:"offscreen"`
	Outline      bool      `json:"outline"`
	StyleChanged bool      `json:"style_changed"`
	Frame        bool      `json:"frame"`
	Modal        bool      `json:"modal"`
	Body         bool      `json:"body"`
	// NoIndicator is true when the pixels around the element are the same with and without focus.
	NoIndicator bool `json:"no_indicator,omitempty"`
	// Checked is true when the pixel comparison ran for this stop.
	Checked bool `json:"checked,omitempty"`
}

// SkipLink describes a skip link found at the first stop.
type SkipLink struct {
	Selector string `json:"selector"`
	Text     string `json:"text"`
	Href     string `json:"href"`
	Found    bool   `json:"found"`
	Rendered bool   `json:"rendered"`
}

// TabWalk is the result of pressing Tab through a page.
type TabWalk struct {
	Focusables int       `json:"focusables"`
	Stops      []TabStop `json:"stops"`
	Trap       []TabStop `json:"trap,omitempty"`
	Skip       *SkipLink `json:"skip,omitempty"`
	Completed  bool      `json:"completed"`
}

// MaxTabStops is the most Tab presses one walk makes.
const MaxTabStops = 80

// maxIndicatorChecks is the most stops whose focus indicator is checked with pixels.
const maxIndicatorChecks = 30

// minChangedPixels is how many pixels must differ before the screenshots count as showing a focus indicator.
const minChangedPixels = 6

func (w *TabWalk) call(ctx context.Context, conn *bidi.Conn, cx, op string, arg any, out any) error {
	return conn.CallFunctionJSONIn(ctx, cx, tabSandbox, tabwalkJS, out, op, arg)
}

// WalkTabs presses Tab from the top of the page and records where focus goes. It needs a loaded page.
func WalkTabs(ctx context.Context, conn *bidi.Conn, cx string, limit int) (*TabWalk, error) {
	w := &TabWalk{}
	var init struct {
		Focusables int `json:"focusables"`
	}
	if err := w.call(ctx, conn, cx, "init", nil, &init); err != nil {
		return nil, fmt.Errorf("tab walk: %w", err)
	}
	w.Focusables = init.Focusables
	tab, _ := bidi.Key("Tab")
	esc, _ := bidi.Key("Escape")
	seen := map[int]int{} // element id -> index in Stops
	stuck := 0
	for i := 0; i < limit; i++ {
		if err := conn.PressKey(ctx, cx, tab); err != nil {
			return nil, fmt.Errorf("tab walk: %w", err)
		}
		var st TabStop
		if err := w.call(ctx, conn, cx, "state", nil, &st); err != nil {
			return nil, fmt.Errorf("tab walk: %w", err)
		}
		if st.Body {
			// Nothing on the page takes focus and a real Tab press found nothing either, such as a frame.
			if len(w.Stops) == 0 && w.Focusables == 0 {
				w.Completed = true
				break
			}
			if len(w.Stops) > 0 {
				w.Completed = true // focus left the page's elements, as it does after the last one
				break
			}
			continue
		}
		if len(w.Stops) == 0 {
			var sk *SkipLink
			if err := w.call(ctx, conn, cx, "skip", nil, &sk); err == nil {
				w.Skip = sk
			}
		}
		if n := len(w.Stops); n > 0 && w.Stops[n-1].ID == st.ID {
			if st.Frame {
				continue // focus moved inside a frame or a media player the walk cannot see into
			}
			stuck++
			if stuck >= 2 {
				if !st.Modal {
					t, err := trapped(ctx, conn, cx, w, []TabStop{st}, tab, esc)
					if err != nil {
						return nil, err
					}
					if t {
						w.Trap = []TabStop{st}
					}
				}
				return w.finish(ctx, conn, cx)
			}
			continue
		}
		stuck = 0
		if j, ok := seen[st.ID]; ok {
			if j == 0 {
				w.Completed = true // back at the first stop: the walk went all the way round
				break
			}
			loop := slices.Clone(w.Stops[j:])
			modal := slices.ContainsFunc(loop, func(s TabStop) bool { return s.Modal })
			if !modal {
				t, err := trapped(ctx, conn, cx, w, loop, tab, esc)
				if err != nil {
					return nil, err
				}
				if t {
					w.Trap = loop
				}
			}
			if !modal || w.Trap != nil {
				return w.finish(ctx, conn, cx)
			}
			break
		}
		seen[st.ID] = len(w.Stops)
		w.Stops = append(w.Stops, st)
	}
	return w.finish(ctx, conn, cx)
}

// trapped presses Escape and then Tab a few more times. Focus that never reaches an element outside the loop is a trap.
// A key press or state read that fails is an error, so a probe that could not finish never reads as "no trap".
func trapped(ctx context.Context, conn *bidi.Conn, cx string, w *TabWalk, loop []TabStop, tab, esc string) (bool, error) {
	if err := conn.PressKey(ctx, cx, esc); err != nil {
		return false, fmt.Errorf("tab walk: trap probe: %w", err)
	}
	in := map[int]bool{}
	for _, s := range loop {
		in[s.ID] = true
	}
	// Escape alone may move focus out, as a menu that closes on Escape does.
	var after TabStop
	if err := w.call(ctx, conn, cx, "state", nil, &after); err != nil {
		return false, fmt.Errorf("tab walk: trap probe: %w", err)
	}
	if after.Body || !in[after.ID] {
		return false, nil
	}
	for range len(loop) + 3 {
		if err := conn.PressKey(ctx, cx, tab); err != nil {
			return false, fmt.Errorf("tab walk: trap probe: %w", err)
		}
		var st TabStop
		if err := w.call(ctx, conn, cx, "state", nil, &st); err != nil {
			return false, fmt.Errorf("tab walk: trap probe: %w", err)
		}
		if st.Body || !in[st.ID] {
			return false, nil
		}
	}
	return true, nil
}

// finish marks a walk that met a frame as partial, then checks the focus indicator of the stops that show none in
// their style, by comparing pictures. A focus or screenshot failure is not fatal and leaves the stop unchecked. A cancelled or expired context is, so a
// walk that ran out of time never reads as a clean result.
func (w *TabWalk) finish(ctx context.Context, conn *bidi.Conn, cx string) (*TabWalk, error) {
	// The walk does not follow focus into a frame, so a walk that met one has not seen everything.
	if slices.ContainsFunc(w.Stops, func(s TabStop) bool { return s.Frame }) {
		w.Completed = false
	}
	checks := 0
	for i := range w.Stops {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("tab walk: %w", err)
		}
		s := &w.Stops[i]
		if s.Outline || s.StyleChanged || s.Hidden || s.Offscreen || s.Frame || checks >= maxIndicatorChecks {
			continue
		}
		checks++
		clip := bidi.Clip{X: max(s.Box.X-6, 0), Y: max(s.Box.Y-6, 0), Width: min(s.Box.Width+12, 600), Height: min(s.Box.Height+12, 300)}
		var ok bool
		if err := w.call(ctx, conn, cx, "focus", s.ID, &ok); err != nil || !ok {
			if ctx.Err() != nil {
				return nil, fmt.Errorf("tab walk: %w", ctx.Err())
			}
			continue
		}
		with, err1 := shot(ctx, conn, cx, clip)
		var b bool
		_ = w.call(ctx, conn, cx, "blur", nil, &b)
		without, err2 := shot(ctx, conn, cx, clip)
		if err1 != nil || err2 != nil {
			if ctx.Err() != nil {
				return nil, fmt.Errorf("tab walk: %w", ctx.Err())
			}
			continue
		}
		s.Checked = true
		s.NoIndicator = changedPixels(with, without) < minChangedPixels
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("tab walk: %w", err)
	}
	return w, nil
}

func shot(ctx context.Context, conn *bidi.Conn, cx string, clip bidi.Clip) (image.Image, error) {
	data, err := conn.CaptureScreenshotClip(ctx, cx, clip)
	if err != nil {
		return nil, err
	}
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, err
	}
	return png.Decode(bytes.NewReader(raw))
}

// changedPixels counts the pixels that differ between two pictures. Pictures of different sizes differ everywhere.
func changedPixels(a, b image.Image) int {
	if a.Bounds() != b.Bounds() {
		return a.Bounds().Dx() * a.Bounds().Dy()
	}
	n := 0
	bd := a.Bounds()
	for y := bd.Min.Y; y < bd.Max.Y; y++ {
		for x := bd.Min.X; x < bd.Max.X; x++ {
			r1, g1, b1, a1 := a.At(x, y).RGBA()
			r2, g2, b2, a2 := b.At(x, y).RGBA()
			if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
				n++
			}
		}
	}
	return n
}
