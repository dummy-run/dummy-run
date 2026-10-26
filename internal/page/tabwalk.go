package page

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"math"
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
	StyleChanged bool      `json:"styleChanged"`
	Frame        bool      `json:"frame"`
	Modal        bool      `json:"modal"`
	Body         bool      `json:"body"`
	// NoIndicator is true when the pixels around the element are the same with and without focus.
	NoIndicator bool `json:"no_indicator,omitempty"`
	// Checked is true when the pixel comparison ran for this stop.
	Checked bool `json:"checked,omitempty"`
	// Appearance is how the focus indicator measured, when the pictures with and without focus were compared.
	Appearance *FocusAppearance `json:"appearance,omitempty"`
}

// FocusAppearance measures a focus indicator from two pictures of the same place, with and without focus, by the
// method of WCAG 2.4.13: the pixels that change, and how much their contrast changes.
type FocusAppearance struct {
	// Changed is the area, in CSS pixels, of the pixels that differ at all.
	Changed float64 `json:"changed"`
	// Area is the area, in CSS pixels, of the pixels whose colours with and without focus have a contrast ratio of
	// at least 3:1.
	Area float64 `json:"area"`
	// Need is the area of a 2 CSS pixel thick line round the element, the least WCAG 2.4.13 asks for.
	Need float64 `json:"need"`
	// Contrast is the largest contrast ratio between a pixel's colours with and without focus.
	Contrast float64 `json:"contrast"`
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

// maxAppearanceChecks is the most stops with a focus style of their own whose indicator is measured with pixels.
const maxAppearanceChecks = 30

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
// their style, by comparing pictures. A focus or screenshot failure is not fatal and leaves the stop unchecked. A
// cancelled or expired context is, so a walk that ran out of time never reads as a clean result.
func (w *TabWalk) finish(ctx context.Context, conn *bidi.Conn, cx string) (*TabWalk, error) {
	// The walk does not follow focus into a frame, so a walk that met one has not seen everything.
	if slices.ContainsFunc(w.Stops, func(s TabStop) bool { return s.Frame }) {
		w.Completed = false
	}
	// Stops that show no focus style get the pictures first, so the measurements of styled ones can't use up the
	// check for an indicator that may be missing.
	checks, measured := 0, 0
	for i := range w.Stops {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("tab walk: %w", err)
		}
		s := &w.Stops[i]
		if s.Hidden || s.Offscreen || s.Frame {
			continue
		}
		styled := s.Outline || s.StyleChanged
		if (!styled && checks >= maxIndicatorChecks) || (styled && measured >= maxAppearanceChecks) {
			continue
		}
		if styled {
			measured++
		} else {
			checks++
		}
		clip := bidi.Clip{X: max(s.Box.X-6, 0), Y: max(s.Box.Y-6, 0), Width: min(s.Box.Width+12, 600), Height: min(s.Box.Height+12, 300)}
		var ok bool
		if err := w.call(ctx, conn, cx, "focus", s.ID, &ok); err != nil || !ok {
			if ctx.Err() != nil {
				return nil, fmt.Errorf("tab walk: %w", ctx.Err())
			}
			continue
		}
		var b bool
		_ = w.call(ctx, conn, cx, "settle", nil, &b)
		with, err1 := shot(ctx, conn, cx, clip)
		_ = w.call(ctx, conn, cx, "blur", nil, &b)
		_ = w.call(ctx, conn, cx, "settle", nil, &b)
		without, err2 := shot(ctx, conn, cx, clip)
		if err1 != nil || err2 != nil {
			if ctx.Err() != nil {
				return nil, fmt.Errorf("tab walk: %w", ctx.Err())
			}
			continue
		}
		if !styled {
			s.Checked = true
			s.NoIndicator = changedPixels(with, without) < minChangedPixels
		}
		s.Appearance = measureFocus(with, without, clip, s.Box)
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

// measureFocus compares pictures of the same clip with and without focus. A picture can have more pixels than the
// clip has CSS pixels, on a high density screen, so areas are scaled back to CSS pixels. The line WCAG 2.4.13 asks
// for goes round the part of the element inside the clip.
func measureFocus(with, without image.Image, clip bidi.Clip, box LayoutBox) *FocusAppearance {
	if with.Bounds() != without.Bounds() || clip.Width <= 0 || clip.Height <= 0 {
		return nil
	}
	bd := with.Bounds()
	scale := float64(bd.Dx()) / clip.Width * float64(bd.Dy()) / clip.Height
	if scale <= 0 {
		return nil
	}
	changed, strong, best := 0, 0, 1.0
	for y := bd.Min.Y; y < bd.Max.Y; y++ {
		for x := bd.Min.X; x < bd.Max.X; x++ {
			a, b := with.At(x, y), without.At(x, y)
			r1, g1, b1, _ := a.RGBA()
			r2, g2, b2, _ := b.RGBA()
			if r1 == r2 && g1 == g2 && b1 == b2 {
				continue
			}
			changed++
			c := ratio(luminance(float64(r1>>8), float64(g1>>8), float64(b1>>8)), luminance(float64(r2>>8), float64(g2>>8), float64(b2>>8)))
			if c >= 3 {
				strong++
			}
			best = max(best, c)
		}
	}
	w, h := min(box.Width, clip.Width), min(box.Height, clip.Height)
	return &FocusAppearance{Changed: round1(float64(changed) / scale), Area: round1(float64(strong) / scale),
		Need: round1(4 * (w + h)), Contrast: round2(best)}
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
