package server

import (
	"bytes"
	"strings"
	"testing"
)

const svgDoc = `<svg xmlns="http://www.w3.org/2000/svg" width="120" height="60" viewBox="0 0 120 60">
  <rect width="120" height="60" fill="white"/>
  <text x="60" y="38" font-family="sans-serif" font-size="24" text-anchor="middle">OK</text>
</svg>`

// A valid SVG renders to a raster (GS v 0) and ends with a single cut.
func TestBuildSVG(t *testing.T) {
	got, err := buildSVG(svgRequest{SVG: svgDoc, Cut: ptrBool(true)}, 576)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte{gs, 'v', '0'}) {
		t.Fatal("expected a GS v 0 raster image in the stream")
	}
	if !bytes.HasSuffix(got, []byte{gs, 'V', 66, 0}) {
		t.Fatal("expected a trailing partial cut")
	}
}

func TestBuildSVGNoCut(t *testing.T) {
	got, err := buildSVG(svgRequest{SVG: svgDoc, Cut: ptrBool(false)}, 576)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(got, []byte{gs, 'V', 66, 0}) {
		t.Fatal("did not expect a cut")
	}
}

func TestBuildSVGError(t *testing.T) {
	if _, err := buildSVG(svgRequest{SVG: "<svg>nope</svg"}, 576); err == nil {
		t.Fatal("expected error for malformed svg")
	}
}

// A width narrower than the head is honored (and the image left-aligns rather
// than being centered) without error.
func TestBuildSVGNarrowWidth(t *testing.T) {
	got, err := buildSVG(svgRequest{SVG: svgDoc, Width: 200, Align: "left", Cut: ptrBool(false)}, 576)
	if err != nil {
		t.Fatal(err)
	}
	// align left is ESC a 0; the raster should still be present.
	if !bytes.Contains(got, []byte{esc, 'a', 0}) {
		t.Fatal("expected left alignment")
	}
	if !bytes.Contains(got, []byte{gs, 'v', '0'}) {
		t.Fatal("expected raster image")
	}
}

func TestBuildSVGEmptyRejected(t *testing.T) {
	if _, err := buildSVG(svgRequest{SVG: "  "}, 576); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("want empty-svg error, got %v", err)
	}
}
