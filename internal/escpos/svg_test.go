package escpos

import (
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
