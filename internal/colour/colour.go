// Package colour suggests colours that pass a WCAG contrast threshold. It works in OKLCH, the polar form of
// Björn Ottosson's OKLab colour space, and changes only lightness, so a suggestion keeps the hue and, where the
// screen can show it, the chroma of the colour it replaces. The formulas are the published ones; no code is
// copied from another library.
package colour

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// RGB is an sRGB colour with channels from 0 to 255.
type RGB struct{ R, G, B uint8 }

// Parse reads a #rgb or #rrggbb colour.
func Parse(s string) (RGB, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return RGB{}, fmt.Errorf("colour: %q is not #rgb or #rrggbb", s)
	}
	var c [3]uint8
	for i := range c {
		v, err := strconv.ParseUint(s[2*i:2*i+2], 16, 8)
		if err != nil {
			return RGB{}, fmt.Errorf("colour: %q is not #rgb or #rrggbb", s)
		}
		c[i] = uint8(v)
	}
	return RGB{c[0], c[1], c[2]}, nil
}

// Hex writes the colour as #rrggbb.
func (c RGB) Hex() string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

func linear(c float64) float64 {
	c /= 255
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func encode(c float64) float64 {
	if c <= 0.0031308 {
		return 12.92 * c * 255
	}
	return (1.055*math.Pow(c, 1/2.4) - 0.055) * 255
}

// Luminance is the WCAG relative luminance.
func (c RGB) Luminance() float64 {
	return 0.2126*linear(float64(c.R)) + 0.7152*linear(float64(c.G)) + 0.0722*linear(float64(c.B))
}

// Ratio is the WCAG contrast ratio between two colours, from 1 to 21.
func Ratio(a, b RGB) float64 {
	la, lb := a.Luminance(), b.Luminance()
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// LCH is a colour in OKLCH: lightness from 0 to 1, chroma, and hue in radians.
type LCH struct{ L, C, H float64 }

// ToLCH converts an sRGB colour to OKLCH.
func ToLCH(c RGB) LCH {
	r, g, b := linear(float64(c.R)), linear(float64(c.G)), linear(float64(c.B))
	l := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	m := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	s := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	L := 0.2104542553*l + 0.7936177850*m - 0.0040720468*s
	A := 1.9779984951*l - 2.4285922050*m + 0.4505937099*s
	B := 0.0259040371*l + 0.7827717662*m - 0.8086757660*s
	// A grey's a and b are rounding noise, so it gets no chroma and stays grey when its lightness changes.
	if C := math.Hypot(A, B); C > 1e-4 {
		return LCH{L, C, math.Atan2(B, A)}
	}
	return LCH{L: L}
}

// toLinear converts OKLCH to linear sRGB, which can fall outside 0 to 1 for a colour the screen can't show.
func (p LCH) toLinear() (float64, float64, float64) {
	A, B := p.C*math.Cos(p.H), p.C*math.Sin(p.H)
	l := p.L + 0.3963377774*A + 0.2158037573*B
	m := p.L - 0.1055613458*A - 0.0638541728*B
	s := p.L - 0.0894841775*A - 1.2914855480*B
	l, m, s = l*l*l, m*m*m, s*s*s
	return 4.0767416621*l - 3.3077115913*m + 0.2309699292*s,
		-1.2684380046*l + 2.6097574011*m - 0.3413193965*s,
		-0.0041960863*l - 0.7034186147*m + 1.7076147010*s
}

// ToRGB converts OKLCH to sRGB. A colour the screen can't show loses chroma, keeping its lightness and hue,
// until it can.
func (p LCH) ToRGB() RGB {
	in := func(q LCH) bool {
		r, g, b := q.toLinear()
		const e = 1e-6
		return r >= -e && r <= 1+e && g >= -e && g <= 1+e && b >= -e && b <= 1+e
	}
	if !in(p) {
		lo, hi := 0.0, p.C
		for i := 0; i < 30; i++ {
			mid := (lo + hi) / 2
			if in(LCH{p.L, mid, p.H}) {
				lo = mid
			} else {
				hi = mid
			}
		}
		p.C = lo
	}
	r, g, b := p.toLinear()
	ch := func(v float64) uint8 {
		return uint8(math.Round(math.Max(0, math.Min(255, encode(math.Max(0, math.Min(1, v)))))))
	}
	return RGB{ch(r), ch(g), ch(b)}
}

// Nearest returns the colour closest in lightness to c that reaches need against other, and whether there is one.
// It tries darker and lighter, and keeps whichever moves lightness less.
func Nearest(c, other RGB, need float64) (RGB, bool) {
	if Ratio(c, other) >= need {
		return c, true
	}
	p := ToLCH(c)
	var best RGB
	bestMove, found := math.Inf(1), false
	for _, end := range []float64{0, 1} {
		at := LCH{end, p.C, p.H}.ToRGB()
		if Ratio(at, other) < need {
			continue // even black or white, at this hue, isn't enough
		}
		// The ratio rises steadily from c's lightness towards the end, so the first lightness that passes is found
		// by halving.
		near, far := p.L, end
		for i := 0; i < 40; i++ {
			mid := (near + far) / 2
			if Ratio(LCH{mid, p.C, p.H}.ToRGB(), other) >= need {
				far = mid
			} else {
				near = mid
			}
		}
		cand := LCH{far, p.C, p.H}.ToRGB()
		if Ratio(cand, other) < need {
			continue
		}
		if move := math.Abs(far - p.L); move < bestMove {
			best, bestMove, found = cand, move, true
		}
	}
	return best, found
}
