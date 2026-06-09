package server

import (
	"fmt"
	"net/http"
	"strings"

	"shortorder/internal/escpos"
)

// The SVG endpoint is the universal layout escape hatch. Where /api/print/text
// and /api/print/document lay out receipts in the printer's native character
// grid, this renders arbitrary SVG markup to a raster and prints it through the
// dither pipeline — so an agent can print any layout it can draw: custom fonts,
// logos, free positioning, shapes, anything the grid can't express. The cost is
// the usual raster trade-off (1-bit dithered, not selectable text, larger
// payload), so prefer native text when the grid is enough.

// svgRequest is the body for POST /api/print/svg.
type svgRequest struct {
	SVG   string `json:"svg"`   // SVG markup; the root <svg> must have a width/height or viewBox
	Width int    `json:"width"` // target raster width in dots; default and cap is the head width
	Align string `json:"align"` // left|center|right, used when width < head width
	Cut   *bool  `json:"cut"`   // cut after printing (default true)
}

func (s *Server) handleSVG(w http.ResponseWriter, r *http.Request) {
	var req svgRequest
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.SVG) == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("svg is required"))
		return
	}
	data, err := buildSVG(req, s.cfg.Width)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.dispatch(w, "shortorder-svg", data)
}

// buildSVG renders an SVG job to ESC/POS at the given head width. Pure (no I/O)
// so the HTTP handler and the MCP tool share one code path. The markup is
// rasterized to the target width (defaulting to, and capped at, the head width)
// and printed through the same dither/align path as any other image.
func buildSVG(req svgRequest, headWidth int) ([]byte, error) {
	target := headWidth
	if req.Width > 0 && req.Width < headWidth {
		target = req.Width
	}
	img, err := escpos.SVGImage(req.SVG, target)
	if err != nil {
		return nil, err
	}
	b := escpos.New()
	writeImage(b, img, headWidth, parseAlignDefault(req.Align, escpos.AlignCenter))
	if cutOrDefault(req.Cut) {
		b.Cut()
	}
	return b.Bytes(), nil
}
