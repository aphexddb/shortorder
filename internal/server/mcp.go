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
				"print_text, print_qr, print_barcode, or print_image to print, and cut to feed and "+
				"cut the paper. For crisp receipt text prefer print_text over embedding "+
				"text in an image.",
		),
	)

	m.AddTool(mcp.NewTool("list_printers",
		mcp.WithDescription("List the connected, supported USB thermal printer(s) and the supported models."),
	), s.mcpListPrinters)

	m.AddTool(mcp.NewTool("print_text",
		mcp.WithDescription("Print text to the thermal receipt printer, then optionally cut the paper."),
		mcp.WithString("text", mcp.Required(), mcp.Description(`Text to print. Use \n for line breaks.`)),
		mcp.WithString("align", mcp.Description("Horizontal alignment: left | center | right (default left).")),
		mcp.WithBoolean("bold", mcp.Description("Bold / emphasized text (default false).")),
		mcp.WithNumber("underline", mcp.Description("Underline weight: 0 off, 1 thin, 2 thick (default 0).")),
		mcp.WithNumber("width", mcp.Description("Character width magnification, 1-8 (default 1).")),
		mcp.WithNumber("height", mcp.Description("Character height magnification, 1-8 (default 1).")),
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

	m.AddTool(mcp.NewTool("print_barcode",
		mcp.WithDescription("Render a barcode and print it, then optionally cut the paper. Supports 1D codes (CODE128, GS1-128, CODE39, CODE93, EAN-13/8, UPC-A, ITF, ITF-14, Standard 2 of 5, Codabar) and 2D codes (DataMatrix, PDF417)."),
		mcp.WithString("data", mcp.Required(), mcp.Description("Content to encode. Numeric symbologies (ean13, ean8, upca, itf, itf14, standard2of5) accept digits only.")),
		mcp.WithString("format", mcp.Description("Symbology: code128 | gs1-128 | code39 | code93 | ean13 | ean8 | upca | itf | itf14 | standard2of5 | codabar | datamatrix | pdf417 (default code128).")),
		mcp.WithNumber("width", mcp.Description("Total width in dots (1D: default ~2 dots/module; 2D: scales the whole symbol). Capped to the head width.")),
		mcp.WithNumber("height", mcp.Description("Bar height in dots for 1D codes (default 80). Ignored for 2D codes.")),
		mcp.WithBoolean("wide", mcp.Description("Print larger modules (1D ~4 dots/module, 2D ~10) for dense symbologies or finicky scanners (default false). Ignored when width is set.")),
		mcp.WithBoolean("hri", mcp.Description("Print the human-readable number under the code, grouped per symbology (EAN-8 4+4, EAN-13 1+6+6, UPC-A 1+5+5+1). Ignored if caption is set.")),
		mcp.WithString("align", mcp.Description("Horizontal alignment: left | center | right (default center).")),
		mcp.WithString("caption", mcp.Description("Optional text printed under the barcode; overrides hri when set.")),
		mcp.WithBoolean("cut", mcp.Description("Cut the paper after printing (default true).")),
	), s.mcpPrintBarcode)

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
	text, err := req.RequireString("text")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	cut := req.GetBool("cut", true)
	job := textRequest{
		Text:      text,
		Align:     req.GetString("align", ""),
		Bold:      req.GetBool("bold", false),
		Underline: byte(req.GetInt("underline", 0)),
		Width:     req.GetInt("width", 0),
		Height:    req.GetInt("height", 0),
		Feed:      req.GetInt("feed", 0),
		Cut:       &cut,
	}
	return s.mcpPrint("shortorder-text", buildText(job))
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

func (s *Server) mcpPrintBarcode(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	data, err := req.RequireString("data")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	cut := req.GetBool("cut", true)
	job := barcodeRequest{
		Data:    data,
		Format:  req.GetString("format", ""),
		Width:   req.GetInt("width", 0),
		Height:  req.GetInt("height", 0),
		Wide:    req.GetBool("wide", false),
		HRI:     req.GetBool("hri", false),
		Align:   req.GetString("align", ""),
		Caption: req.GetString("caption", ""),
		Cut:     &cut,
	}
	payload, err := buildBarcode(job, s.cfg.Width)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return s.mcpPrint("shortorder-barcode", payload)
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
