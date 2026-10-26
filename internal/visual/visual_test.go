package visual

import (
	"image"
	"image/color"
	"image/png"
	"math/bits"
	"os"
	"path/filepath"
	"testing"
)

func fill(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func box(img *image.RGBA, r image.Rectangle, c color.RGBA) *image.RGBA {
	out := image.NewRGBA(img.Rect)
	copy(out.Pix, img.Pix)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			out.SetRGBA(x, y, c)
		}
	}
	return out
}

var (
	white = color.RGBA{255, 255, 255, 255}
	black = color.RGBA{0, 0, 0, 255}
)

func TestPixelmatchCountsChangedPixels(t *testing.T) {
	a := fill(40, 30, white)
	if n := pixelmatch(a, a, image.NewRGBA(a.Rect), Threshold); n != 0 {
		t.Errorf("the same picture: %d pixels changed, want 0", n)
	}
	b := box(a, image.Rect(10, 10, 20, 15), black)
	if n := pixelmatch(a, b, image.NewRGBA(a.Rect), Threshold); n != 50 {
		t.Errorf("a black 10 by 5 box: %d pixels changed, want 50", n)
	}
	faint := box(a, image.Rect(10, 10, 20, 15), color.RGBA{250, 250, 250, 255})
	if n := pixelmatch(a, faint, image.NewRGBA(a.Rect), Threshold); n != 0 {
		t.Errorf("a box a shade off white: %d pixels changed, want 0 under the threshold", n)
	}
}

func TestPixelmatchLeavesOutAntiAliasing(t *testing.T) {
	// A black box on white, then the same box with one grey pixel blending its edge, as anti-aliasing does.
	a := box(fill(20, 20, white), image.Rect(5, 5, 15, 15), black)
	b := box(a, image.Rect(15, 9, 16, 10), color.RGBA{128, 128, 128, 255})
	out := image.NewRGBA(a.Rect)
	if n := pixelmatch(a, b, out, Threshold); n != 0 {
		t.Errorf("%d pixels changed, want 0 for an anti-aliased edge", n)
	}
	if got := out.RGBAAt(15, 9); got.R != 255 || got.G != 255 || got.B != 0 {
		t.Errorf("the diff marks the anti-aliased pixel %v, want yellow", got)
	}
}

func TestDhash(t *testing.T) {
	a := box(fill(90, 80, white), image.Rect(0, 0, 45, 80), black)
	if d := bits.OnesCount64(dhash(a) ^ dhash(a)); d != 0 {
		t.Errorf("the same picture is %d bits apart", d)
	}
	b := box(fill(90, 80, white), image.Rect(45, 0, 90, 80), black)
	if d := bits.OnesCount64(dhash(a) ^ dhash(b)); d < 8 {
		t.Errorf("mirrored halves are %d bits apart, want at least 8", d)
	}
	if dhash(image.NewRGBA(image.Rect(0, 0, 0, 0))) != 0 {
		t.Error("an empty picture should hash to 0")
	}
}

func TestCompare(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "page.png")
	a := fill(40, 30, white)
	if err := WriteFile(old, a); err != nil {
		t.Fatal(err)
	}
	diff := filepath.Join(dir, "page.diff.png")
	c, err := Compare(old, a, diff)
	if err != nil {
		t.Fatal(err)
	}
	if c.Changed != 0 || c.Total != 1200 || c.Diff != "" {
		t.Errorf("same picture: %+v", c)
	}
	if _, err := os.Stat(diff); !os.IsNotExist(err) {
		t.Errorf("a diff was written for no change: %v", err)
	}

	c, err = Compare(old, box(a, image.Rect(0, 0, 12, 10), black), diff)
	if err != nil {
		t.Fatal(err)
	}
	if c.Changed != 120 || c.Ratio != 0.1 || c.Diff != diff {
		t.Errorf("changed picture: %+v", c)
	}
	f, err := os.Open(diff)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	d, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if got := color.RGBAModel.Convert(d.At(1, 1)).(color.RGBA); got.R != 255 || got.G != 0 {
		t.Errorf("the diff marks a changed pixel %v, want red", got)
	}

	c, err = Compare(old, fill(41, 30, white), diff)
	if err != nil {
		t.Fatal(err)
	}
	if !c.SizeChanged || c.Total != 0 {
		t.Errorf("resized picture: %+v", c)
	}

	if _, err := Compare(filepath.Join(dir, "none.png"), a, ""); !os.IsNotExist(err) {
		t.Errorf("missing earlier picture: %v", err)
	}
	if err := os.WriteFile(old, []byte("not a png"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Compare(old, a, ""); err == nil {
		t.Error("a broken earlier picture should be an error")
	}
}

func TestWriteFileLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	if err := WriteFile(filepath.Join(dir, "a.png"), fill(2, 2, white)); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(filepath.Join(dir, "missing", "a.png"), fill(2, 2, white)); err == nil {
		t.Error("writing into a missing directory should fail")
	}
	es, _ := os.ReadDir(dir)
	if len(es) != 1 {
		t.Errorf("files left: %v", es)
	}
}
