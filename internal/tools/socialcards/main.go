// Command socialcards draws the link-preview card for each main page of the site.
//
//	go run ./internal/tools/socialcards          write site/static/social/*.png
//	go run ./internal/tools/socialcards -check   fail when a card differs from what the program draws
//
// Each card is 1200 by 630 pixels: amber ground, the Viewfinder mark, the Dummy mascot, and the page
// title and description in Space Grotesk. The shapes are the ones in brand/logo/mark.svg and
// brand/mascot/dummy-head.svg. The check compares pixels, not file bytes, so a change in the PNG encoder
// does not fail it, and allows a small difference per channel on at most two percent of the pixels for
// rounding between architectures.
package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

//go:embed fonts/SpaceGrotesk-Bold.ttf
var boldTTF []byte

//go:embed fonts/SpaceGrotesk-Medium.ttf
var mediumTTF []byte

const (
	width  = 1200
	height = 630
	margin = 72
)

var (
	amber = color.RGBA{0xfb, 0xad, 0x18, 0xff}
	ink   = color.RGBA{0x1d, 0x1d, 0x1b, 0xff}
	dummy = color.RGBA{0x24, 0x22, 0x1e, 0xff}
	paper = color.RGBA{0xf3, 0xf1, 0xec, 0xff}

	amberDark = color.RGBA{0xdb, 0x8a, 0x06, 0xff}
)

// card is one page's preview. Name is the file name without .png and matches the page path with
// slashes turned into hyphens.
type card struct {
	Name, Title, Text string
}

var cards = []card{
	{"home", "AI plays your users before your customers do", "Fictional users with real roles try your preview and report what broke."},
	{"how-it-works", "How a run works", "A persona signs in, tries to get a job done and tells you where it got stuck."},
	{"start", "Get started", "Install the command, send one persona to a site in read-only mode and read what it found."},
	{"personas", "The persona library", "Ready-made personas for the users every site has. Copy the command that runs each one."},
	{"for", "Who it is for", "Pick the job closest to yours and see what Dummy Run gives you."},
	{"for-qa-lead", "For QA leads", "Exploratory testing by role on every pull request, with evidence behind each finding."},
	{"for-developer", "For developers", "Your product used the way a customer uses it, on your laptop or in CI."},
	{"for-product-and-ux", "For product and UX", "Friction and confusion, each tied to the step and screenshot where it happened."},
	{"for-compliance-and-security", "For compliance and security", "What each role can see or do beyond its rights, and what leaves your environment."},
	{"for-executive", "For executives", "A clear report of what is broken, who it affects and the screenshot that shows it."},
	{"for-agency", "For agencies", "The same personas for every client, with the model in the region each client needs."},
	{"docs", "Docs", "Install the engine, run personas, read the report and connect the tools you already use."},
}

func main() {
	check := flag.Bool("check", false, "fail when a card is out of date, and write nothing")
	out := flag.String("out", "site/static/social", "directory for the cards")
	flag.Parse()
	if err := run(*out, *check); err != nil {
		fmt.Fprintln(os.Stderr, "socialcards:", err)
		os.Exit(1)
	}
}

func run(dir string, check bool) error {
	bold, err := opentype.Parse(boldTTF)
	if err != nil {
		return err
	}
	medium, err := opentype.Parse(mediumTTF)
	if err != nil {
		return err
	}
	var stale []string
	for _, c := range cards {
		img, err := draw1(c, bold, medium)
		if err != nil {
			return fmt.Errorf("%s: %w", c.Name, err)
		}
		path := filepath.Join(dir, c.Name+".png")
		if check {
			if !sameAsFile(img, path) {
				stale = append(stale, path)
			}
			continue
		}
		var buf bytes.Buffer
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		if err := enc.Encode(&buf, img); err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
			return err
		}
	}
	if len(stale) > 0 {
		return fmt.Errorf("out of date, run go run ./internal/tools/socialcards: %s", strings.Join(stale, ", "))
	}
	return nil
}

func sameAsFile(img *image.RGBA, path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	got, err := png.Decode(f)
	if err != nil || got.Bounds() != img.Bounds() {
		return false
	}
	// Rasteriser edge pixels may differ a little between architectures, but a flat area may not. A
	// brand colour change moves most of the card, so the number of pixels that differ at all is capped.
	differing := 0
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			g, w := got.At(x, y), img.At(x, y)
			if !nearly(g, w) {
				return false
			}
			if !exact(g, w) {
				differing++
			}
		}
	}
	return differing <= width*height/50
}

func exact(a, b color.Color) bool {
	r1, g1, b1, a1 := a.RGBA()
	r2, g2, b2, a2 := b.RGBA()
	return r1 == r2 && g1 == g2 && b1 == b2 && a1 == a2
}

// nearly reports whether two pixels differ by at most a few levels in each channel. The rasteriser's
// edge pixels can differ in the last bits between processor architectures.
func nearly(a, b color.Color) bool {
	r1, g1, b1, a1 := a.RGBA()
	r2, g2, b2, a2 := b.RGBA()
	const tol = 24 << 8
	d := func(x, y uint32) bool {
		if x > y {
			return x-y <= tol
		}
		return y-x <= tol
	}
	return d(r1, r2) && d(g1, g2) && d(b1, b2) && d(a1, a2)
}

func draw1(c card, bold, medium *opentype.Font) (*image.RGBA, error) {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), image.NewUniform(amber), image.Point{}, draw.Src)

	// Viewfinder corners at the card edges, as on the mark.
	corners(img, 28, 28, width-28, height-28, 64, 8, ink)

	// The mascot sits on an ink disc at the right.
	cx, cy, r := 940.0, 318.0, 196.0
	fillPath(img, circle(cx, cy, r), ink)
	// The mascot's own viewBox is 64 units. Scale it to fit the disc.
	scale := 4.6
	ox, oy := cx-32*scale, cy-32*scale
	drawHead(img, ox, oy, scale)

	// The mark and name, top left.
	drawMark(img, margin, 76, 56)
	text(img, medium, 30, margin+56+18, 76+38, "Dummy Run", ink)

	// Title and description.
	titleFace, err := opentype.NewFace(bold, &opentype.FaceOptions{Size: 64, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil, err
	}
	descFace, err := opentype.NewFace(medium, &opentype.FaceOptions{Size: 30, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil, err
	}
	textWidth := 640
	y := 250
	tl := wrap(titleFace, c.Title, textWidth)
	size := 64
	if len(tl) > 3 {
		// A long title drops to a smaller size so it keeps three lines at most.
		size = 52
		titleFace, _ = opentype.NewFace(bold, &opentype.FaceOptions{Size: 52, DPI: 72, Hinting: font.HintingNone})
		tl = wrap(titleFace, c.Title, textWidth)
	}
	lh := size + 10
	if len(tl) >= 3 {
		y = 215
	}
	if len(tl) <= 1 {
		y = 290
	}
	for _, l := range tl {
		drawString(img, titleFace, margin, y, l, ink)
		y += lh
	}
	y += 14
	for _, l := range wrap(descFace, c.Text, textWidth) {
		drawString(img, descFace, margin, y, l, ink)
		y += 42
	}

	if y-42 > height-130 {
		return nil, fmt.Errorf("text runs into the footer, shorten it")
	}
	urlFace, _ := opentype.NewFace(bold, &opentype.FaceOptions{Size: 28, DPI: 72, Hinting: font.HintingNone})
	drawString(img, urlFace, margin, height-76, "dummyrun.dev", ink)
	return img, nil
}

func text(img *image.RGBA, f *opentype.Font, size float64, x, y int, s string, c color.Color) {
	face, _ := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	drawString(img, face, x, y, s, c)
}

func drawString(img *image.RGBA, face font.Face, x, y int, s string, c color.Color) {
	d := font.Drawer{Dst: img, Src: image.NewUniform(c), Face: face, Dot: fixed.P(x, y)}
	d.DrawString(s)
}

func wrap(face font.Face, s string, maxW int) []string {
	var lines []string
	cur := ""
	for _, w := range strings.Fields(s) {
		try := w
		if cur != "" {
			try = cur + " " + w
		}
		if font.MeasureString(face, try).Ceil() > maxW && cur != "" {
			lines = append(lines, cur)
			cur = w
		} else {
			cur = try
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// A shape is a closed outline made of line segments after flattening.
type shape struct{ segs []func(r *vector.Rasterizer) }

func fillPath(img *image.RGBA, s shape, c color.Color) {
	var r vector.Rasterizer
	r.Reset(width, height)
	for _, f := range s.segs {
		f(&r)
	}
	r.DrawOp = draw.Over
	r.Draw(img, img.Bounds(), image.NewUniform(c), image.Point{})
}

func circle(cx, cy, rad float64) shape {
	return arcShape(cx, cy, rad, 0, 2*math.Pi)
}

// arcShape is a pie slice from angle a0 to a1, or a whole disc when the sweep is a full turn.
func arcShape(cx, cy, rad, a0, a1 float64) shape {
	const n = 180
	full := a1-a0 >= 2*math.Pi-1e-9
	return shape{[]func(*vector.Rasterizer){func(r *vector.Rasterizer) {
		if !full {
			r.MoveTo(float32(cx), float32(cy))
			r.LineTo(float32(cx+rad*math.Cos(a0)), float32(cy+rad*math.Sin(a0)))
		} else {
			r.MoveTo(float32(cx+rad*math.Cos(a0)), float32(cy+rad*math.Sin(a0)))
		}
		for i := 1; i <= n; i++ {
			a := a0 + (a1-a0)*float64(i)/n
			r.LineTo(float32(cx+rad*math.Cos(a)), float32(cy+rad*math.Sin(a)))
		}
		r.ClosePath()
	}}}
}

func rect(x0, y0, x1, y1 float64) shape {
	return shape{[]func(*vector.Rasterizer){func(r *vector.Rasterizer) {
		r.MoveTo(float32(x0), float32(y0))
		r.LineTo(float32(x1), float32(y0))
		r.LineTo(float32(x1), float32(y1))
		r.LineTo(float32(x0), float32(y1))
		r.ClosePath()
	}}}
}

// corners draws four L-shaped camera corners with arm length arm and thickness t.
func corners(img *image.RGBA, x0, y0, x1, y1, arm, t float64, c color.Color) {
	for _, s := range []shape{
		rect(x0, y0, x0+arm, y0+t), rect(x0, y0, x0+t, y0+arm),
		rect(x1-arm, y0, x1, y0+t), rect(x1-t, y0, x1, y0+arm),
		rect(x0, y1-t, x0+arm, y1), rect(x0, y1-arm, x0+t, y1),
		rect(x1-arm, y1-t, x1, y1), rect(x1-t, y1-arm, x1, y1),
	} {
		fillPath(img, s, c)
	}
}

// drawMark draws the Viewfinder mark in a square of side size at (x, y), from mark.svg's 64 unit box.
func drawMark(img *image.RGBA, x, y, size float64) {
	k := size / 64
	px := func(v float64) float64 { return x + v*k }
	py := func(v float64) float64 { return y + v*k }
	// Target: ink ring, amber face, ink upper-left and lower-right quadrants.
	fillPath(img, circle(px(32), py(32), 19.5*k), ink)
	fillPath(img, circle(px(32), py(32), 16.5*k), amber)
	fillPath(img, arcShape(px(32), py(32), 17*k, math.Pi, 1.5*math.Pi), ink)
	fillPath(img, arcShape(px(32), py(32), 17*k, 0, 0.5*math.Pi), ink)
	// Corners, stroke 6 units, at the box edges.
	arm, t := 12.0, 6.0
	corners(img, px(0), py(0), px(64), py(64), arm*k, t*k, ink)
}

// drawHead draws the friendly head from brand/mascot/dummy-head.svg. The head's viewBox is 64 units,
// scaled by k and placed with its top left corner at ox, oy.
func drawHead(img *image.RGBA, ox, oy, k float64) {
	px := func(x float64) float64 { return ox + x*k }
	py := func(y float64) float64 { return oy + y*k }

	// Ears, then the rounded face.
	fillPath(img, union(circle(px(6.5), py(35), 4.5*k), circle(px(57.5), py(35), 4.5*k)), amberDark)
	face := []shape{rect(px(29), py(7), px(35), py(57)), rect(px(7), py(29), px(57), py(35))}
	for _, c := range [][2]float64{{29, 29}, {35, 29}, {29, 35}, {35, 35}} {
		face = append(face, circle(px(c[0]), py(c[1]), 22*k))
	}
	fillPath(img, union(face...), amber)

	// The target on the temple: amber disc, dark ring, dark top left and bottom right quadrants.
	tx, ty := px(19), py(21)
	fillPath(img, circle(tx, ty, 7.8*k), dummy)
	fillPath(img, circle(tx, ty, 5.2*k), amber)
	fillPath(img, union(arcShape(tx, ty, 6.5*k, math.Pi, 1.5*math.Pi), arcShape(tx, ty, 6.5*k, 0, 0.5*math.Pi)), dummy)

	// Eyes with a highlight each.
	fillPath(img, union(ellipse(px(25), py(36), 3.6*k, 4.4*k), ellipse(px(39), py(36), 3.6*k, 4.4*k)), dummy)
	fillPath(img, union(circle(px(26.2), py(34.3), 1.4*k), circle(px(40.2), py(34.3), 1.4*k)), paper)

	// The smile: a quadratic curve from (27, 46) through (32, 50.5) to (37, 46), stroked 3 units wide
	// as one outline with a round cap at each end. One outline keeps the fill count low.
	const steps = 24
	var left, right [steps + 1][2]float64
	for i := 0; i <= steps; i++ {
		t := float64(i) / steps
		x := (1-t)*(1-t)*27 + 2*(1-t)*t*32 + t*t*37
		y := (1-t)*(1-t)*46 + 2*(1-t)*t*50.5 + t*t*46
		dx := 2*(1-t)*(32-27) + 2*t*(37-32)
		dy := 2*(1-t)*(50.5-46) + 2*t*(46-50.5)
		n := math.Hypot(dx, dy)
		nx, ny := -dy/n*1.5, dx/n*1.5
		left[i] = [2]float64{px(x + nx), py(y + ny)}
		right[i] = [2]float64{px(x - nx), py(y - ny)}
	}
	fillPath(img, shape{[]func(*vector.Rasterizer){func(r *vector.Rasterizer) {
		r.MoveTo(float32(left[0][0]), float32(left[0][1]))
		for i := 1; i <= steps; i++ {
			r.LineTo(float32(left[i][0]), float32(left[i][1]))
		}
		for i := steps; i >= 0; i-- {
			r.LineTo(float32(right[i][0]), float32(right[i][1]))
		}
		r.ClosePath()
	}}}, dummy)
	fillPath(img, union(circle(px(27), py(46), 1.5*k), circle(px(37), py(46), 1.5*k)), dummy)
}

// union joins shapes that share one fill, so the rasteriser runs once for them. Every shape is drawn in
// the same direction, so overlaps stay filled.
func union(shapes ...shape) shape {
	var out shape
	for _, s := range shapes {
		out.segs = append(out.segs, s.segs...)
	}
	return out
}

func ellipse(cx, cy, rx, ry float64) shape {
	const n = 120
	return shape{[]func(*vector.Rasterizer){func(r *vector.Rasterizer) {
		r.MoveTo(float32(cx+rx), float32(cy))
		for i := 1; i <= n; i++ {
			a := 2 * math.Pi * float64(i) / n
			r.LineTo(float32(cx+rx*math.Cos(a)), float32(cy+ry*math.Sin(a)))
		}
		r.ClosePath()
	}}}
}
