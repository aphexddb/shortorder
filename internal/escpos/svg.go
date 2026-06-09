package escpos

import (
	"fmt"
	"image"
	"log/slog"
	"regexp"
	"strings"

	"github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/renderers/rasterizer"
)

// MaxSVGHeight caps a rendered SVG's height in dots. It bounds memory and paper
// use when a pathological aspect ratio (a tall, narrow viewBox) would otherwise
// rasterize into meters of receipt. 20000 dots is ~2.6m of 80mm paper — far
// beyond any real receipt, but a hard stop against runaway input.
const MaxSVGHeight = 20000

// maxFontRetries bounds the alias-and-retry loop, so an SVG that names many
// unresolvable fonts still terminates.
const maxFontRetries = 16

// missingFontRe extracts the family name from canvas's "failed to find font
// 'NAME'" panic so it can be aliased to a fallback.
var missingFontRe = regexp.MustCompile(`font '([^']*)'`)

// SVGImage renders SVG markup to a raster image scaled so its width is width
// dots, preserving aspect ratio. It uses tdewolff/canvas's pure-Go renderer and
// fonts bundled into the binary — no system fonts, no external binary, no
// network — so the same markup rasterizes identically on any host (dev laptop or
// appliance).
//
// This is the universal layout path: shapes, rules, gradients, free positioning,
// rotation, embedded raster images, and text — anything an agent can express as
// SVG — renders here and prints through the same 1-bit dither pipeline (Image)
// as photographs. SVG is the escape hatch for layouts the character grid can't
// express; native text endpoints stay crisper and smaller.
//
// Fonts: the host's fonts are not used. Text renders in the bundled faces —
// Roboto (sans-serif) and Gelasio (serif), plus Go Mono (monospace). Generic
// families and common named fonts map onto these; a family we don't recognize is
// logged and aliased to the sans fallback so text still renders. font-weight and
// font-style are not differentiated (no real bold/italic) — for crisp bold or
// styled receipt text use the text/document endpoints.
func SVGImage(data string, width int) (image.Image, error) {
	if strings.TrimSpace(data) == "" {
		return nil, fmt.Errorf("svg is empty")
	}
	ensureBundledFonts()

	// Rendering mutates the shared canvas font index when aliasing a missing
	// font, so serialize it (the printer is single-slot, so this is free).
	fontMu.Lock()
	defer fontMu.Unlock()

	for attempt := 0; attempt <= maxFontRetries; attempt++ {
		img, err := renderSVGOnce(data, width)
		if err == nil {
			return img, nil
		}
		// Only a missing font is recoverable by aliasing; anything else is a real
		// error (bad markup, no intrinsic size, too tall) — return it as-is.
		name, ok := missingFontName(err)
		if !ok {
			return nil, err
		}
		if !aliasFont(name) {
			// No fallback available, or we already aliased this name and it still
			// failed: log and give up rather than loop.
			slog.Warn("svg fonts: font not found and no usable fallback; cannot render text", "font", name)
			return nil, err
		}
		slog.Warn("svg fonts: font not found, falling back to bundled sans-serif", "font", name)
	}
	return nil, fmt.Errorf("render svg: too many unresolved fonts (after %d fallbacks)", maxFontRetries)
}

// renderSVGOnce parses and rasterizes the SVG once, converting canvas's panics
// (notably an unresolvable font) into an error so a bad SVG can never crash the
// process — which would be a remote DoS, fatal under the stdio MCP transport.
func renderSVGOnce(data string, width int) (img image.Image, err error) {
	defer func() {
		if r := recover(); r != nil {
			img, err = nil, fmt.Errorf("render svg: %v", r)
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

// missingFontName reports whether err is canvas's "failed to find font 'NAME'"
// failure, returning the family name when it is.
func missingFontName(err error) (string, bool) {
	if err == nil || !strings.Contains(err.Error(), "find font") {
		return "", false
	}
	m := missingFontRe.FindStringSubmatch(err.Error())
	if len(m) != 2 {
		return "", false
	}
	return m[1], true
}
