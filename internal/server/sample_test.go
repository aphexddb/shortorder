package server

import (
	"encoding/json"
	"testing"
)

// The embedded complex receipt unmarshals into a documentRequest and renders
// through the document pipeline without error.
func TestSampleReceiptBuilds(t *testing.T) {
	var req documentRequest
	if err := json.Unmarshal(sampleReceiptJSON, &req); err != nil {
		t.Fatalf("embedded sample receipt is invalid JSON: %v", err)
	}
	if len(req.Elements) == 0 {
		t.Fatal("embedded sample receipt has no elements")
	}
	got, err := buildDocument(req, 576)
	if err != nil {
		t.Fatalf("buildDocument: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("buildDocument produced no bytes")
	}
}

// The embedded showcase SVG renders through the SVG pipeline without error.
func TestSampleSVGBuilds(t *testing.T) {
	got, err := buildSVG(svgRequest{SVG: string(sampleShowcaseSVG)}, 576)
	if err != nil {
		t.Fatalf("buildSVG: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("buildSVG produced no bytes")
	}
}
