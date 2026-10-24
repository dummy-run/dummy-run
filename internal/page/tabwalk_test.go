package page

import (
	"image"
	"image/color"
	"testing"
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
