package escpos

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

const sampleSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="50" viewBox="0 0 100 50">
  <rect x="0" y="0" width="100" height="50" fill="white"/>
  <text x="50" y="32" font-family="sans-serif" font-size="20" text-anchor="middle">Hi</text>
  <line x1="0" y1="48" x2="100" y2="48" stroke="black" stroke-width="2"/>
</svg>`

func TestSVGImageScalesToWidth(t *testing.T) {
	img, err := SVGImage(sampleSVG, 200)
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	// width should land at the requested 200 dots (allow rounding slack)...
	if dx := b.Dx(); dx < 198 || dx > 202 {
		t.Fatalf("width = %d, want ~200", dx)
	}
	// ...and height preserve the 2:1 aspect ratio (~100).
	if dy := b.Dy(); dy < 96 || dy > 104 {
		t.Fatalf("height = %d, want ~100 (aspect preserved)", dy)
	}
}

// The rendered SVG must actually contain ink — a renderer that silently drops
// <text>/shapes would produce an all-white image.
func TestSVGImageRendersInk(t *testing.T) {
	img, err := SVGImage(sampleSVG, 200)
	if err != nil {
		t.Fatal(err)
	}
	raster := pack(img)
	if raster == nil {
		t.Fatal("pack returned nil")
	}
	dark := 0
	for _, by := range raster.data {
		if by != 0 {
			dark++
		}
	}
	if dark == 0 {
		t.Fatal("rendered SVG has no black pixels — text/shapes were dropped")
	}
}

// An unresolvable font must NOT crash the process (canvas panics internally).
// Instead SVGImage logs a warning and aliases the family to the bundled sans
// fallback, so text still renders. Verifies both the fallback render and that
// the miss is logged. Uses a guaranteed-absent family unique to this test so the
// alias/log state isn't shared with other tests.
func TestSVGImageUnknownFontFallsBackAndLogs(t *testing.T) {
	prev := slog.Default()
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	defer slog.SetDefault(prev)

	const fam = "zzz-unique-missing-font-7c4e1a"
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="40" viewBox="0 0 100 40">` +
		`<text x="4" y="28" font-family="` + fam + `" font-size="20">Hi</text></svg>`
	img, err := SVGImage(svg, 200)
	if err != nil {
		t.Fatalf("unknown font should fall back and render, got %v", err)
	}
	if pack(img) == nil {
		t.Fatal("expected a rendered image")
	}
	logged := buf.String()
	if !strings.Contains(logged, "font not found") || !strings.Contains(logged, fam) {
		t.Fatalf("expected a 'font not found' warning naming %q; log was:\n%s", fam, logged)
	}
}

// Generic families resolve to the bundled fonts (so text renders even on a host
// with no fonts installed). monospace maps to the bundled mono face.
func TestSVGImageGenericFamiliesRenderInk(t *testing.T) {
	for _, fam := range []string{"sans-serif", "serif", "monospace", "Helvetica, Arial, sans-serif"} {
		svg := `<svg xmlns="http://www.w3.org/2000/svg" width="120" height="40" viewBox="0 0 120 40">` +
			`<text x="4" y="28" font-family="` + fam + `" font-size="22">Ab12</text></svg>`
		img, err := SVGImage(svg, 240)
		if err != nil {
			t.Fatalf("family %q should render via bundled fonts, got %v", fam, err)
		}
		raster := pack(img)
		dark := 0
		for _, by := range raster.data {
			if by != 0 {
				dark++
			}
		}
		if dark == 0 {
			t.Fatalf("family %q produced no ink", fam)
		}
	}
}

func TestSVGImageEmpty(t *testing.T) {
	if _, err := SVGImage("   ", 200); err == nil {
		t.Fatal("expected error for empty svg")
	}
}

func TestSVGImageBadMarkup(t *testing.T) {
	if _, err := SVGImage("<svg><rect></svg", 200); err == nil {
		t.Fatal("expected parse error for malformed svg")
	}
}

func TestSVGImageContentBoundsFallback(t *testing.T) {
	// No width/height/viewBox, but with content: canvas falls back to the
	// content bounding box, so this renders fine and scales to the target width.
	img, err := SVGImage(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="10" height="10"/></svg>`, 200)
	if err != nil {
		t.Fatalf("content-bounds fallback should render, got %v", err)
	}
	if dx := img.Bounds().Dx(); dx < 198 || dx > 202 {
		t.Fatalf("width = %d, want ~200", dx)
	}
}

func TestSVGImageNoIntrinsicSize(t *testing.T) {
	// Empty document: nothing to size or scale -> a clear error rather than a
	// zero-pixel raster.
	_, err := SVGImage(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`, 200)
	if err == nil || !strings.Contains(err.Error(), "intrinsic size") {
		t.Fatalf("want intrinsic-size error, got %v", err)
	}
}

func TestSVGImageHeightCapped(t *testing.T) {
	// A 1x100000 viewBox scaled to width 200 would be 20,000,000 dots tall.
	tall := `<svg xmlns="http://www.w3.org/2000/svg" width="1" height="100000" viewBox="0 0 1 100000"><rect width="1" height="100000"/></svg>`
	_, err := SVGImage(tall, 200)
	if err == nil || !strings.Contains(err.Error(), "max") {
		t.Fatalf("want height-cap error, got %v", err)
	}
}
