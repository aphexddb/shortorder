package server

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
)

// Built-in sample print jobs. The UI "Try it" panel exposes one-click buttons
// that print a fully laid-out receipt and a rich SVG showcase, so a user can see
// what the document and SVG endpoints produce without authoring a payload. The
// sample definitions are embedded into the binary (the same //go:embed approach
// used for the bundled fonts), so they ship with the app and need no files on
// disk at runtime. Each handler reuses the exact builder of its real endpoint —
// buildDocument / buildSVG — so the sample output is byte-identical to what an
// agent would get from /api/print/document or /api/print/svg.

//go:embed sample/complex-receipt.json
var sampleReceiptJSON []byte

//go:embed sample/showcase.svg
var sampleShowcaseSVG []byte

// handleSampleReceipt prints the embedded complex receipt through the document
// pipeline. Body is ignored; the receipt definition lives in the binary.
func (s *Server) handleSampleReceipt(w http.ResponseWriter, r *http.Request) {
	var req documentRequest
	if err := json.Unmarshal(sampleReceiptJSON, &req); err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("embedded sample receipt is invalid: %w", err))
		return
	}
	data, err := buildDocument(req, s.cfg.Width)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("rendering sample receipt: %w", err))
		return
	}
	s.dispatch(w, "shortorder-sample-receipt", data)
}

// handleSampleSVG prints the embedded showcase SVG through the SVG pipeline.
// Body is ignored; the SVG markup lives in the binary.
func (s *Server) handleSampleSVG(w http.ResponseWriter, r *http.Request) {
	data, err := buildSVG(svgRequest{SVG: string(sampleShowcaseSVG)}, s.cfg.Width)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("rendering sample SVG: %w", err))
		return
	}
	s.dispatch(w, "shortorder-sample-svg", data)
}
