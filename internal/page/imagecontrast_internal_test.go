package page

import (
	"image"
	"image/color"
	"testing"
)

func fill(w, h int, f func(x, y int) color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, f(x, y))
		}
	}
	return img
}

func TestContrastBehind(t *testing.T) {
	white := [3]float64{255, 255, 255}
	black := fill(10, 10, func(int, int) color.RGBA { return color.RGBA{0, 0, 0, 255} })
	c, ok := contrastBehind(black, white)
	if !ok || c.Median != 21 || c.Worst != 21 || c.Spread != 0 || c.Text != "#ffffff" || c.Pixels != 100 {
		t.Errorf("white on black: %+v", c)
	}
	// A left-to-right gradient from black to white: the median pixel is mid grey and the worst tenth is near white.
	grad := fill(100, 4, func(x, _ int) color.RGBA { v := uint8(x * 255 / 99); return color.RGBA{v, v, v, 255} })
	c, _ = contrastBehind(grad, white)
	if c.Median > 4.5 || c.Worst > c.Median || c.Spread < 15 {
		t.Errorf("white on a grey gradient: %+v", c)
	}
	if !(ImageContrast{Median: 4.4, Need: 4.5}).Fails() || (ImageContrast{Median: 3.1, Need: 3}).Fails() {
		t.Error("Fails compares the median with the need")
	}
	// A ratio just under the threshold is rounded down, so it still fails and never shows as 4.50.
	if m := floor2(4.4999); m != 4.49 || !(ImageContrast{Median: m, Need: 4.5}).Fails() {
		t.Errorf("4.4999 should round down to 4.49 and fail, got %v", m)
	}
	if l := luminance(255, 255, 255); l != 1 {
		t.Errorf("white luminance %v", l)
	}
	if r := ratio(luminance(118, 118, 118), 1); r < 4.5 || r > 4.6 {
		t.Errorf("#767676 on white is about 4.54:1, got %v", r)
	}
}

func TestMeasureImageContrastSkipsNestedTargets(t *testing.T) {
	// A nested target never reaches the browser, so a nil connection is never used.
	ms, err := MeasureImageContrast(t.Context(), nil, "", []string{"#host >> button"})
	if err != nil || len(ms) != 0 {
		t.Fatalf("got %v, %v", ms, err)
	}
}
