// Package visual compares a screenshot with the one taken last time: how many pixels changed, by a port of
// pixelmatch, and how far apart the two pictures are by a difference hash. It reports the counts and judges nothing.
package visual

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"math/bits"
	"os"
	"path/filepath"
)

// Threshold is pixelmatch's colour threshold, from 0 to 1. Its default, 0.1, ignores slight shifts in colour.
const Threshold = 0.1

// Change is how a screenshot differs from the one before. All counts are pixels of the picture.
type Change struct {
	// Changed counts the pixels that differ, leaving out anti-aliasing, of Total.
	Changed int `json:"changed"`
	Total   int `json:"total"`
	// Ratio is Changed over Total, rounded to four places.
	Ratio float64 `json:"ratio"`
	// HashDistance is how many of the 64 bits of the two pictures' difference hashes differ: 0 for pictures that
	// look the same at a glance, and more as their shapes and shading change.
	HashDistance int `json:"hash_distance"`
	// SizeChanged says the two pictures have different sizes, so no pixels were compared.
	SizeChanged bool `json:"size_changed,omitempty"`
	// Diff is a picture of the changes, written next to the screenshot when any pixel changed.
	Diff string `json:"diff,omitempty"`
}

// Compare compares the PNG at oldPath with img, writes a picture of the differences to diffPath when there are
// any, and returns what changed.
func Compare(oldPath string, img image.Image, diffPath string) (*Change, error) {
	f, err := os.Open(oldPath) //nolint:gosec // a file the sweep wrote in the directory you chose
	if err != nil {
		return nil, err
	}
	old, err := png.Decode(f)
	_ = f.Close()
	if err != nil {
		return nil, fmt.Errorf("read the earlier screenshot: %w", err)
	}
	a, b := toRGBA(old), toRGBA(img)
	c := &Change{HashDistance: bits.OnesCount64(dhash(a) ^ dhash(b))}
	if a.Rect.Size() != b.Rect.Size() {
		c.SizeChanged = true
		return c, nil
	}
	out := image.NewRGBA(a.Rect)
	c.Total = a.Rect.Dx() * a.Rect.Dy()
	c.Changed = pixelmatch(a, b, out, Threshold)
	c.Ratio = math.Round(float64(c.Changed)/float64(c.Total)*1e4) / 1e4
	if c.Changed > 0 && diffPath != "" {
		if err := writePNG(diffPath, out); err != nil {
			return c, err
		}
		c.Diff = diffPath
	}
	return c, nil
}

func toRGBA(img image.Image) *image.RGBA {
	if r, ok := img.(*image.RGBA); ok && r.Rect.Min == (image.Point{}) {
		return r
	}
	out := image.NewRGBA(image.Rect(0, 0, img.Bounds().Dx(), img.Bounds().Dy()))
	draw.Draw(out, out.Rect, img, img.Bounds().Min, draw.Src)
	return out
}

// dhash is a 64-bit difference hash: the picture shrunk to 9 by 8 grey cells by averaging, with one bit for each
// pair of neighbouring cells, set when the left one is brighter.
func dhash(img *image.RGBA) uint64 {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w == 0 || h == 0 {
		return 0
	}
	var cells [8][9]float64
	for cy := range 8 {
		for cx := range 9 {
			x0, x1 := cx*w/9, max((cx+1)*w/9, cx*w/9+1)
			y0, y1 := cy*h/8, max((cy+1)*h/8, cy*h/8+1)
			sum, n := 0.0, 0
			for y := y0; y < min(y1, h); y++ {
				for x := x0; x < min(x1, w); x++ {
					i := y*img.Stride + x*4
					sum += 0.299*float64(img.Pix[i]) + 0.587*float64(img.Pix[i+1]) + 0.114*float64(img.Pix[i+2])
					n++
				}
			}
			if n > 0 {
				cells[cy][cx] = sum / float64(n)
			}
		}
	}
	var hash uint64
	for cy := range 8 {
		for cx := range 8 {
			hash <<= 1
			if cells[cy][cx] > cells[cy][cx+1] {
				hash |= 1
			}
		}
	}
	return hash
}

// WriteFile writes img as a PNG to path through a new file renamed into place, so a write that fails part way leaves
// the earlier screenshot whole.
func WriteFile(path string, img image.Image) error { return writePNG(path, img) }

func writePNG(path string, img image.Image) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".screen-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	werr := png.Encode(f, img)
	if err := errors.Join(werr, f.Close()); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
