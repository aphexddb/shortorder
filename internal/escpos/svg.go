package escpos

import (
	"fmt"
	"image"
	"strings"

	"github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/renderers/rasterizer"
)

// MaxSVGHeight caps a rendered SVG's height in dots. It bounds memory and paper
// use when a pathological aspect ratio (a tall, narrow viewBox) would otherwise
// rasterize into meters of receipt. 20000 dots is ~2.6m of 80mm paper — far
// beyond any real receipt, but a hard stop against runaway input.
const MaxSVGHeight = 20000

// SVGImage renders SVG markup to a raster image scaled so its width is width
// dots, preserving aspect ratio. It uses tdewolff/canvas's pure-Go renderer and
// bundled fonts — no system fonts, no external binary, no network — so the same
// markup rasterizes identically on any host (dev laptop or appliance).
//
// This is the universal layout path: shapes, rules, gradients, free positioning,
// rotation, embedded raster images, and text — anything an agent can express as
// SVG — renders here and prints through the same 1-bit dither pipeline (Image)
// as photographs. SVG is the escape hatch for layouts the character grid can't
// express; native text endpoints stay crisper and smaller.
//
// Fonts: for determinism the host's fonts are NOT used. All text renders in the
// bundled Go typeface — every family (serif, sans-serif, cursive, a named font,
// ...) maps to it, and font-weight/font-style are not differentiated (no real
// bold or italic). Map font-family to a generic (sans-serif / monospace); for
// crisp, truly bold or styled receipt text use the text/document endpoints.
func SVGImage(data string, width int) (img image.Image, err error) {
	if strings.TrimSpace(data) == "" {
		return nil, fmt.Errorf("svg is empty")
	}
	// Use bundled fonts so text renders deterministically without host fonts.
	ensureBundledFonts()
	// canvas panics on an unresolvable font (and a few other malformed inputs);
	// turn that into an error so a bad SVG can never crash the process — which
	// would be a remote DoS, fatal under the stdio MCP transport.
	defer func() {
		if r := recover(); r != nil {
			img = nil
			err = fmt.Errorf("render svg: %v (if you used a specific font-family, switch to a generic one such as sans-serif, serif, or monospace)", r)
		}
	}()
	c, err := canvas.ParseSVG(strings.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse svg: %w", err)
	}
	// canvas measures the document in millimeters (SVG user units at 96 DPI).
	// A document with no intrinsic size can't be scaled to a pixel width.
	if c.W <= 0 || c.H <= 0 {
		return nil, fmt.Errorf("svg has no intrinsic size; set width and height (or a viewBox) on the root <svg>")
	}
	if width < 1 {
		width = 1
	}
	pxPerMM := float64(width) / c.W
	if h := c.H * pxPerMM; h > MaxSVGHeight {
		return nil, fmt.Errorf("rendered svg would be %.0f dots tall (max %d); reduce its height-to-width ratio or target width", h, MaxSVGHeight)
	}
	img = rasterizer.Draw(c, canvas.DPMM(pxPerMM), canvas.DefaultColorSpace)
	return img, nil
}
