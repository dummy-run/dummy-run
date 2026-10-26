package page

import (
	"image"
	"image/color"
	"testing"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

func flat(w int, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func TestChangedPixels(t *testing.T) {
	white, black := color.White, color.Black
	a := flat(10, white)
	if n := changedPixels(a, flat(10, white)); n != 0 {
		t.Errorf("identical pictures differ in %d pixels", n)
	}
	// The boundary: one pixel short of minChangedPixels is not an indicator, minChangedPixels is.
	for _, c := range []struct {
		changed int
		want    bool // true when the pixels count as a focus indicator
	}{{minChangedPixels - 1, false}, {minChangedPixels, true}} {
		b := flat(10, white)
		for i := 0; i < c.changed; i++ {
			b.Set(i, 0, black)
		}
		n := changedPixels(a, b)
		if n != c.changed {
			t.Errorf("changed %d pixels, counted %d", c.changed, n)
		}
		if got := n >= minChangedPixels; got != c.want {
			t.Errorf("%d changed pixels: indicator %v, want %v", c.changed, got, c.want)
		}
	}
	// Pictures of different sizes differ everywhere.
	if n := changedPixels(a, flat(12, white)); n != 100 {
		t.Errorf("different sizes counted %d, want 100", n)
	}
}

// ring draws a picture of a w by h clip at the given density, white with a line k pixels thick of colour c round
// its edge.
func ring(w, h, k, density int, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w*density, h*density))
	for y := 0; y < h*density; y++ {
		for x := 0; x < w*density; x++ {
			edge := x < k*density || y < k*density || x >= (w-k)*density || y >= (h-k)*density
			if edge {
				img.Set(x, y, c)
			} else {
				img.Set(x, y, color.White)
			}
		}
	}
	return img
}

func TestMeasureFocus(t *testing.T) {
	clip := bidi.Clip{Width: 52, Height: 32}
	box := LayoutBox{Width: 40, Height: 20}
	plain := func(d int) *image.RGBA { return ring(52, 32, 0, d, color.White) }
	for _, d := range []int{1, 2} {
		// A black line 2 pixels thick round a 52 by 32 clip covers 2*2*(52+32)-16 = 320 CSS pixels.
		a := measureFocus(ring(52, 32, 2, d, color.Black), plain(d), clip, box)
		if a == nil || a.Area != 320 || a.Changed != 320 || a.Contrast != 21 || a.Need != 240 {
			t.Errorf("density %d, black ring: %+v", d, a)
		}
	}
	faint := measureFocus(ring(52, 32, 2, 1, color.Gray{0xe6}), plain(1), clip, box)
	if faint == nil || faint.Area != 0 || faint.Changed != 320 || faint.Contrast >= 3 {
		t.Errorf("faint ring: %+v", faint)
	}
	if same := measureFocus(plain(1), plain(1), clip, box); same == nil || same.Changed != 0 || same.Contrast != 1 {
		t.Errorf("no change: %+v", same)
	}
	if measureFocus(plain(1), plain(2), clip, box) != nil {
		t.Error("pictures of different sizes were compared")
	}
}
