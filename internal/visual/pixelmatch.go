// This file is a Go port of pixelmatch 7.1.0 (https://github.com/mapbox/pixelmatch), whose licence follows.
//
// ISC License
//
// Copyright (c) 2025, Mapbox
//
// Permission to use, copy, modify, and/or distribute this software for any purpose
// with or without fee is hereby granted, provided that the above copyright notice
// and this permission notice appear in all copies.
//
// THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES WITH
// REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND
// FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY SPECIAL, DIRECT,
// INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES WHATSOEVER RESULTING FROM LOSS
// OF USE, DATA OR PROFITS, WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER
// TORTIOUS ACTION, ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF
// THIS SOFTWARE.

package visual

import (
	"image"
	"math"
)

// pixelmatch counts the pixels that differ between two pictures of the same size, by their colour difference in the
// YIQ space, and leaves out pixels that only differ by anti-aliasing, at Threshold, which is
// pixelmatch's default. When out is not nil it gets a picture of the differences: the first picture faded to grey,
// with differing pixels in red and anti-aliased ones in yellow.
func pixelmatch(a, b *image.RGBA, out *image.RGBA) int {
	w, h := a.Rect.Dx(), a.Rect.Dy()
	maxDelta := 35215 * Threshold * Threshold
	diff := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			pos := y*a.Stride + x*4
			delta := 0.0
			if !samePixel(a.Pix, b.Pix, pos) {
				delta = colorDelta(a.Pix, b.Pix, pos, pos, false)
			}
			if math.Abs(delta) > maxDelta {
				if antialiased(a, b, x, y) || antialiased(b, a, x, y) {
					if out != nil {
						drawPixel(out.Pix, pos, 255, 255, 0)
					}
					continue
				}
				if out != nil {
					drawPixel(out.Pix, pos, 255, 0, 0)
				}
				diff++
			} else if out != nil {
				drawGrayPixel(a.Pix, pos, 0.1, out.Pix)
			}
		}
	}
	return diff
}

func samePixel(p, q []uint8, i int) bool {
	return p[i] == q[i] && p[i+1] == q[i+1] && p[i+2] == q[i+2] && p[i+3] == q[i+3]
}

// antialiased says whether the pixel at x, y in img looks like anti-aliasing: few neighbours of the same colour, and
// its darkest or brightest neighbour sitting in a flat area in both pictures.
func antialiased(img, other *image.RGBA, x1, y1 int) bool {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	x0, y0 := max(x1-1, 0), max(y1-1, 0)
	x2, y2 := min(x1+1, w-1), min(y1+1, h-1)
	pos := y1*img.Stride + x1*4
	zeroes := 0
	if x1 == x0 || x1 == x2 || y1 == y0 || y1 == y2 {
		zeroes = 1
	}
	var lo, hi float64
	var loX, loY, hiX, hiY int
	for x := x0; x <= x2; x++ {
		for y := y0; y <= y2; y++ {
			if x == x1 && y == y1 {
				continue
			}
			delta := colorDelta(img.Pix, img.Pix, pos, y*img.Stride+x*4, true)
			switch {
			case delta == 0:
				zeroes++
				if zeroes > 2 {
					return false
				}
			case delta < lo:
				lo, loX, loY = delta, x, y
			case delta > hi:
				hi, hiX, hiY = delta, x, y
			}
		}
	}
	if lo == 0 || hi == 0 {
		return false
	}
	return (hasManySiblings(img, loX, loY) && hasManySiblings(other, loX, loY)) ||
		(hasManySiblings(img, hiX, hiY) && hasManySiblings(other, hiX, hiY))
}

// hasManySiblings says whether more than two neighbours of the pixel have exactly its colour.
func hasManySiblings(img *image.RGBA, x1, y1 int) bool {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	x0, y0 := max(x1-1, 0), max(y1-1, 0)
	x2, y2 := min(x1+1, w-1), min(y1+1, h-1)
	pos := y1*img.Stride + x1*4
	zeroes := 0
	if x1 == x0 || x1 == x2 || y1 == y0 || y1 == y2 {
		zeroes = 1
	}
	for x := x0; x <= x2; x++ {
		for y := y0; y <= y2; y++ {
			if x == x1 && y == y1 {
				continue
			}
			if j := y*img.Stride + x*4; img.Pix[j] == img.Pix[pos] && img.Pix[j+1] == img.Pix[pos+1] && img.Pix[j+2] == img.Pix[pos+2] && img.Pix[j+3] == img.Pix[pos+3] {
				zeroes++
			}
			if zeroes > 2 {
				return true
			}
		}
	}
	return false
}

// colorDelta is the squared YIQ distance between two pixels, negative when the first is brighter, or the brightness
// difference alone when yOnly is set. Pixels that aren't opaque are blended with a background that varies by
// position, so transparency still shows as a difference.
func colorDelta(p, q []uint8, k, m int, yOnly bool) float64 {
	r1, g1, b1, a1 := float64(p[k]), float64(p[k+1]), float64(p[k+2]), float64(p[k+3])
	r2, g2, b2, a2 := float64(q[m]), float64(q[m+1]), float64(q[m+2]), float64(q[m+3])
	dr, dg, db, da := r1-r2, g1-g2, b1-b2, a1-a2
	if dr == 0 && dg == 0 && db == 0 && da == 0 {
		return 0
	}
	if a1 < 255 || a2 < 255 {
		rb := 48 + 159*float64(k%2)
		gb := 48 + 159*float64(int(float64(k)/1.618033988749895)%2)
		bb := 48 + 159*float64(int(float64(k)/2.618033988749895)%2)
		dr = (r1*a1 - r2*a2 - rb*da) / 255
		dg = (g1*a1 - g2*a2 - gb*da) / 255
		db = (b1*a1 - b2*a2 - bb*da) / 255
	}
	y := dr*0.29889531 + dg*0.58662247 + db*0.11448223
	if yOnly {
		return y
	}
	i := dr*0.59597799 - dg*0.27417610 - db*0.32180189
	qq := dr*0.21147017 - dg*0.52261711 + db*0.31114694
	delta := 0.5053*y*y + 0.299*i*i + 0.1957*qq*qq
	if y > 0 {
		return -delta
	}
	return delta
}

func drawPixel(out []uint8, pos int, r, g, b uint8) {
	out[pos], out[pos+1], out[pos+2], out[pos+3] = r, g, b, 255
}

func drawGrayPixel(img []uint8, i int, alpha float64, out []uint8) {
	v := 255 + (float64(img[i])*0.29889531+float64(img[i+1])*0.58662247+float64(img[i+2])*0.11448223-255)*alpha*float64(img[i+3])/255
	g := uint8(math.Max(0, math.Min(255, v)))
	drawPixel(out, i, g, g, g)
}
