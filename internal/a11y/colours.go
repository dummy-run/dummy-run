package a11y

import (
	"fmt"

	"github.com/dummy-run/dummy-run/internal/colour"
)

// Colours are the colours of text that fails a contrast rule, and the nearest colours that would meet the ratio. A suggestion
// changes only the lightness of one colour in OKLCH, so it keeps the hue of the design. TextFix keeps the
// background, and BackgroundFix keeps the text. Either is empty when no lightness at that hue reaches the ratio.
type Colours struct {
	Text          string  `json:"text"`
	Background    string  `json:"background"`
	Ratio         float64 `json:"ratio"`
	Need          float64 `json:"need"`
	TextFix       string  `json:"text_fix,omitempty"`
	BackgroundFix string  `json:"background_fix,omitempty"`
}

// suggestColours turns what axe measured into Colours, or nil when axe gave colours that can't be read.
func suggestColours(c axeColours) *Colours {
	fg, err1 := colour.Parse(c.FG)
	bg, err2 := colour.Parse(c.BG)
	if err1 != nil || err2 != nil || c.Need <= 1 {
		return nil
	}
	out := &Colours{Text: fg.Hex(), Background: bg.Hex(), Ratio: c.Ratio, Need: c.Need}
	if out.Ratio == 0 {
		out.Ratio = colour.Ratio(fg, bg)
	}
	if t, ok := colour.Nearest(fg, bg, c.Need); ok && t != fg {
		out.TextFix = t.Hex()
	}
	if b, ok := colour.Nearest(bg, fg, c.Need); ok && b != bg {
		out.BackgroundFix = b.Hex()
	}
	return out
}

// Detail says what was measured and what would meet the ratio, in words.
func (c *Colours) Detail() string {
	d := fmt.Sprintf("%s on %s is %.2f:1, and needs %g:1.", c.Text, c.Background, c.Ratio, c.Need)
	switch {
	case c.TextFix != "" && c.BackgroundFix != "":
		d += fmt.Sprintf(" Text %s on the same background meets it, or the same text on %s.", c.TextFix, c.BackgroundFix)
	case c.TextFix != "":
		d += fmt.Sprintf(" Text %s on the same background meets it.", c.TextFix)
	case c.BackgroundFix != "":
		d += fmt.Sprintf(" The same text on %s meets it.", c.BackgroundFix)
	}
	return d
}
