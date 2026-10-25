package page

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"math"
	"slices"
	"strings"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

// ImageContrastMax is the most elements one page tries to measure.
const ImageContrastMax = 20

// imageContrastJS prepares one element for a picture of what is behind its text. Called with (op, selector):
//
//	"hide"  returns the text colour, the size the text needs and the element's box in document coordinates, then
//	        makes the text and its shadow transparent, so a screenshot shows only what is behind it
//	"show"  puts the text back
const imageContrastJS = `(op, selector) => {
  const id = "dummyrun-image-contrast";
  if (op === "show") {
    document.getElementById(id)?.remove();
    for (const el of document.querySelectorAll("[data-dummyrun-hide]")) el.removeAttribute("data-dummyrun-hide");
    return null;
  }
  let el = null;
  try { el = document.querySelector(selector); } catch (e) { return null; }
  if (!el) return null;
  const cs = getComputedStyle(el);
  const m = /rgba?\(([\d.]+),\s*([\d.]+),\s*([\d.]+)(?:,\s*([\d.]+))?\)/.exec(cs.color);
  if (!m || (m[4] !== undefined && Number(m[4]) < 1)) return null;
  const size = parseFloat(cs.fontSize), weight = Number(cs.fontWeight) || 400;
  // WCAG large text: 18pt (24 CSS px) or 14pt (about 18.66 CSS px) bold.
  const large = size >= 24 || (size >= 18.66 && weight >= 700);
  const r = el.getBoundingClientRect();
  if (r.width < 2 || r.height < 2) return null;
  el.setAttribute("data-dummyrun-hide", "");
  const st = document.createElement("style");
  st.id = id;
  st.textContent = "[data-dummyrun-hide], [data-dummyrun-hide] * { color: transparent !important; -webkit-text-fill-color: transparent !important; text-shadow: none !important; caret-color: transparent !important; }";
  document.documentElement.appendChild(st);
  return { rgb: [Number(m[1]), Number(m[2]), Number(m[3])], need: large ? 3 : 4.5, large,
    box: { x: r.left + scrollX, y: r.top + scrollY, width: Math.min(r.width, 1200), height: Math.min(r.height, 800) } };
}`

// ImageContrast is the contrast of one element's text against the pixels behind it.
type ImageContrast struct {
	Selector string `json:"selector"`
	// Text is the text colour as #rrggbb.
	Text string `json:"text"`
	// Need is the contrast ratio WCAG 1.4.3 asks for: 4.5, or 3 for large text.
	Need float64 `json:"need"`
	// Median is the contrast against the middle background pixel, and Worst against the worst tenth of them.
	Median float64 `json:"median"`
	Worst  float64 `json:"worst"`
	// Spread is the standard deviation of the background's luminance, from 0 to 255. Above 15 it is an image or
	// a gradient rather than one colour.
	Spread float64 `json:"spread"`
	Pixels int     `json:"pixels"`
}

// Fails says most of the text sits on a background with too little contrast.
func (c ImageContrast) Fails() bool { return c.Median < c.Need }

// MeasureImageContrast measures each element's text against what is behind it: it hides the text, takes a picture of
// the element's box and compares the text colour with every pixel. Elements it cannot measure are left out, and
// the caller counts them.
func MeasureImageContrast(ctx context.Context, conn *bidi.Conn, cx string, selectors []string) ([]ImageContrast, error) {
	var out []ImageContrast
	tried := 0
	for _, sel := range selectors {
		// A target inside a shadow root or frame joins its parts with " >> ", which querySelector can't resolve.
		if strings.Contains(sel, " >> ") {
			continue
		}
		// Failed attempts count too, so a page full of elements that can't be measured still finishes in time.
		if tried >= ImageContrastMax {
			break
		}
		tried++
		var prep *struct {
			RGB  [3]float64 `json:"rgb"`
			Need float64    `json:"need"`
			Box  LayoutBox  `json:"box"`
		}
		if err := conn.CallFunctionJSON(ctx, cx, imageContrastJS, &prep, "hide", sel); err != nil {
			if ctx.Err() != nil {
				return nil, fmt.Errorf("image contrast: %w", ctx.Err())
			}
			continue
		}
		if prep == nil {
			continue
		}
		data, err := conn.CaptureScreenshotClip(ctx, cx, bidi.Clip{X: prep.Box.X, Y: prep.Box.Y, Width: prep.Box.Width, Height: prep.Box.Height})
		var none any
		_ = conn.CallFunctionJSON(ctx, cx, imageContrastJS, &none, "show", "")
		if err != nil {
			if ctx.Err() != nil {
				return nil, fmt.Errorf("image contrast: %w", ctx.Err())
			}
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			continue
		}
		img, err := png.Decode(bytes.NewReader(raw))
		if err != nil {
			continue
		}
		c, ok := contrastBehind(img, prep.RGB)
		if !ok {
			continue
		}
		c.Selector, c.Need = sel, prep.Need
		out = append(out, c)
	}
	return out, nil
}

// contrastBehind compares a text colour with every pixel of a picture of the background.
func contrastBehind(img image.Image, text [3]float64) (ImageContrast, bool) {
	b := img.Bounds()
	if b.Dx() < 1 || b.Dy() < 1 {
		return ImageContrast{}, false
	}
	lt := luminance(text[0], text[1], text[2])
	ratios := make([]float64, 0, b.Dx()*b.Dy())
	var sum, sq float64
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			R, G, B := float64(r>>8), float64(g>>8), float64(bl>>8)
			grey := 0.299*R + 0.587*G + 0.114*B
			sum += grey
			sq += grey * grey
			ratios = append(ratios, ratio(lt, luminance(R, G, B)))
		}
	}
	n := float64(len(ratios))
	slices.Sort(ratios)
	mean := sum / n
	return ImageContrast{
		Text:   fmt.Sprintf("#%02x%02x%02x", int(text[0]), int(text[1]), int(text[2])),
		Median: round2(ratios[len(ratios)/2]),
		Worst:  round2(ratios[len(ratios)/10]),
		Spread: round2(math.Sqrt(math.Max(sq/n-mean*mean, 0))),
		Pixels: len(ratios),
	}, true
}

// luminance is the WCAG relative luminance of an sRGB colour with channels from 0 to 255.
func luminance(r, g, b float64) float64 {
	ch := func(c float64) float64 {
		c /= 255
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(r) + 0.7152*ch(g) + 0.0722*ch(b)
}

// ratio is the WCAG contrast ratio of two luminances.
func ratio(a, b float64) float64 {
	if a < b {
		a, b = b, a
	}
	return (a + 0.05) / (b + 0.05)
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }
