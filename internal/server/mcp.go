package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"

	mcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"shortorder/internal/escpos"
	"shortorder/internal/printer"
)

// MCPServer builds a Model Context Protocol server exposing the printer as a set
// of typed tools. Agents that speak MCP discover these tools automatically and
// call them without any glue code. The same server is used for both the stdio
// transport (`shortorder mcp`) and the HTTP transport (`/mcp`).
func (s *Server) MCPServer() *mcpserver.MCPServer {
	m := mcpserver.NewMCPServer(
		"shortorder",
		s.cfg.Version,
		mcpserver.WithToolCapabilities(true),
		mcpserver.WithInstructions(
			"shortorder prints to a USB thermal receipt printer (ESC/POS). "+
				"Call list_printers to confirm a device is connected, then use "+
				"print_text, print_qr, or print_image to print, and cut to feed and "+
				"cut the paper. For crisp receipt text prefer print_text over embedding "+
				"text in an image.",
		),
	)

	m.AddTool(mcp.NewTool("list_printers",
		mcp.WithDescription("List the connected, supported USB thermal printer(s) and the supported models."),
	), s.mcpListPrinters)

	m.AddTool(mcp.NewTool("print_text",
		mcp.WithDescription("Print text to the thermal receipt printer, then optionally cut the paper. "+
			"For uniform styling, pass text with the top-level style fields. To mix alignment, "+
			"sizes, and emphasis line by line in one receipt, pass lines instead."),
		mcp.WithString("text", mcp.Description(`Text to print. Use \n for line breaks. Required unless "lines" is given; ignored when "lines" is given.`)),
		mcp.WithString("align", mcp.Description("Horizontal alignment: left | center | right (default left).")),
		mcp.WithBoolean("bold", mcp.Description("Bold / emphasized text (default false).")),
		mcp.WithNumber("underline", mcp.Description("Underline weight: 0 off, 1 thin, 2 thick (default 0).")),
		mcp.WithNumber("width", mcp.Description("Character width magnification, 1-8 (default 1).")),
		mcp.WithNumber("height", mcp.Description("Character height magnification, 1-8 (default 1).")),
		mcp.WithArray("lines",
			mcp.Description("Optional per-line styling. Each item is one styled line; when set, the top-level "+
				"text and style fields are ignored. Use this to mix alignment, sizes, and emphasis in one receipt."),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text":      map[string]any{"type": "string", "description": `Text for this line. Use \n for line breaks.`},
					"align":     map[string]any{"type": "string", "enum": []string{"left", "center", "right"}, "description": "Horizontal alignment (default left)."},
					"bold":      map[string]any{"type": "boolean", "description": "Bold / emphasized text (default false)."},
					"underline": map[string]any{"type": "number", "enum": []int{0, 1, 2}, "description": "Underline weight: 0 off, 1 thin, 2 thick (default 0)."},
					"width":     map[string]any{"type": "number", "description": "Character width magnification, 1-8 (default 1)."},
					"height":    map[string]any{"type": "number", "description": "Character height magnification, 1-8 (default 1)."},
				},
				"required": []string{"text"},
			}),
		),
		mcp.WithNumber("feed", mcp.Description("Extra blank lines fed after the text (default 0).")),
		mcp.WithBoolean("cut", mcp.Description("Cut the paper after printing (default true).")),
	), s.mcpPrintText)

	m.AddTool(mcp.NewTool("print_qr",
		mcp.WithDescription("Render a QR code from text/URL and print it, then optionally cut the paper."),
		mcp.WithString("data", mcp.Required(), mcp.Description("Text or URL to encode in the QR code.")),
		mcp.WithNumber("scale", mcp.Description("Module pixel size, ~6-10 prints cleanly (default 8).")),
		mcp.WithString("recovery", mcp.Description("Error-correction level: low | medium | high | highest (default medium).")),
		mcp.WithString("align", mcp.Description("Horizontal alignment: left | center | right (default center).")),
		mcp.WithString("caption", mcp.Description("Optional text printed under the QR code.")),
		mcp.WithBoolean("cut", mcp.Description("Cut the paper after printing (default true).")),
	), s.mcpPrintQR)

	m.AddTool(mcp.NewTool("print_image",
		mcp.WithDescription("Print a base64-encoded image (PNG/JPEG/GIF) as a dithered raster, scaled to fit the head, then optionally cut."),
		mcp.WithString("image_base64", mcp.Required(), mcp.Description("Base64-encoded PNG, JPEG, or GIF image data.")),
		mcp.WithString("align", mcp.Description("Horizontal alignment: left | center | right (default center).")),
		mcp.WithBoolean("cut", mcp.Description("Cut the paper after printing (default true).")),
	), s.mcpPrintImage)

	m.AddTool(mcp.NewTool("cut",
		mcp.WithDescription("Feed a few lines clear of the head and perform a partial cut."),
	), s.mcpCut)

	return m
}

// ServeStdioMCP runs the MCP server over stdio (for `shortorder mcp`). It blocks
// until stdin closes. Diagnostics go to stderr; stdout carries the protocol.
func (s *Server) ServeStdioMCP() error {
	return mcpserver.ServeStdio(s.MCPServer())
}

// ---- tool handlers -------------------------------------------------------

func (s *Server) mcpListPrinters(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	found, err := printer.Detect()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultJSON(map[string]any{
		"supported": printer.SupportedModels(),
		"detected":  found,
	})
}

func (s *Server) mcpPrintText(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	lines := parseSegments(req.GetArguments()["lines"])
	text := req.GetString("text", "")
	if text == "" && len(lines) == 0 {
		return mcp.NewToolResultError("text or lines is required"), nil
	}
	cut := req.GetBool("cut", true)
	job := textRequest{
		Text:      text,
		Align:     req.GetString("align", ""),
		Bold:      req.GetBool("bold", false),
		Underline: byte(req.GetInt("underline", 0)),
		Width:     req.GetInt("width", 0),
		Height:    req.GetInt("height", 0),
		Lines:     lines,
		Feed:      req.GetInt("feed", 0),
		Cut:       &cut,
	}
	return s.mcpPrint("shortorder-text", buildText(job))
}

// parseSegments converts the MCP "lines" argument (a JSON array of objects) into
// textSegments. Non-object entries are skipped and missing fields take their
// zero value, mirroring how the HTTP handler decodes the same shape from JSON.
func parseSegments(v any) []textSegment {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]textSegment, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, textSegment{
			Text:      mapString(m, "text"),
			Align:     mapString(m, "align"),
			Bold:      mapBool(m, "bold"),
			Underline: byte(mapInt(m, "underline")),
			Width:     mapInt(m, "width"),
			Height:    mapInt(m, "height"),
		})
	}
	return out
}

func mapString(m map[string]any, k string) string {
	if s, ok := m[k].(string); ok {
		return s
	}
	return ""
}

func mapBool(m map[string]any, k string) bool {
	if b, ok := m[k].(bool); ok {
		return b
	}
	return false
}

// mapInt reads an integer-valued field. JSON numbers decode to float64, but
// accept the integer kinds too for callers that pass them directly.
func mapInt(m map[string]any, k string) int {
	switch n := m[k].(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return 0
}

func (s *Server) mcpPrintQR(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	data, err := req.RequireString("data")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	cut := req.GetBool("cut", true)
	job := qrRequest{
		Data:     data,
		Scale:    req.GetInt("scale", 0),
		Recovery: req.GetString("recovery", ""),
		Align:    req.GetString("align", ""),
		Caption:  req.GetString("caption", ""),
		Cut:      &cut,
	}
	payload, err := buildQR(job, s.cfg.Width)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return s.mcpPrint("shortorder-qr", payload)
}

func (s *Server) mcpPrintImage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	b64, err := req.RequireString("image_base64")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("decode image_base64: %v", err)), nil
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("decode image (png/jpeg/gif): %v", err)), nil
	}
	align := parseAlignDefault(req.GetString("align", ""), escpos.AlignCenter)
	cut := req.GetBool("cut", true)
	return s.mcpPrint("shortorder-image", buildImageRaster(img, s.cfg.Width, align, cut))
}

func (s *Server) mcpCut(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.mcpPrint("shortorder-cut", escpos.New().Cut().Bytes())
}

// mcpPrint sends an ESC/POS stream and returns a tool result describing the
// outcome. Printer/transport failures are returned as tool errors (isError) so
// the calling agent sees and can react to them.
func (s *Server) mcpPrint(jobName string, data []byte) (*mcp.CallToolResult, error) {
	t, err := s.send(jobName, data)
	if err != nil {
		s.log.Warn("mcp print failed", "job", jobName, "err", err)
		return mcp.NewToolResultError(err.Error()), nil
	}
	s.log.Info("mcp print", "job", jobName, "printer", t.Name, "bytes", len(data))
	return mcp.NewToolResultText(fmt.Sprintf("Printed %d bytes to %s (%s).", len(data), t.Name, t.Model)), nil
}
