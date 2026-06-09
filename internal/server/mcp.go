package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"strconv"

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
		mcpserver.WithResourceCapabilities(false, false),
		mcpserver.WithPromptCapabilities(false),
		mcpserver.WithInstructions(
			"shortorder prints to an Epson-compatible ESC/POS USB thermal receipt printer. "+
				"Call list_printers to confirm a device is connected, then use "+
				"print_text, print_qr, print_barcode, or print_image to print, and cut to feed and "+
				"cut the paper. To lay out a complete receipt (header, itemized rows with prices "+
				"flush-right, rules, totals, codes, footer) in one job, prefer print_document. "+
				"For crisp receipt text prefer native text (print_text / print_document) over "+
				"embedding text in an image. To print a layout the character grid can't express "+
				"(custom fonts, logos, free positioning, shapes), render it as SVG and use print_svg. "+
				"Read the `shortorder://capabilities` resource for machine-readable limits "+
				"(head width, supported barcode formats, fonts). "+
				"The `receipt`, `logo_header`, and `loyalty_qr` prompts provide ready-made, "+
				"copy-pasteable payload templates for these common jobs.",
		),
	)

	m.AddTool(mcp.NewTool("list_printers",
		mcp.WithDescription("List the connected, supported Epson-compatible ESC/POS USB thermal printer(s) and the supported models."),
	), s.mcpListPrinters)

	m.AddTool(mcp.NewTool("print_text",
		mcp.WithDescription("Print text to the thermal receipt printer, then optionally cut the paper. "+
			"For uniform styling, pass text with the top-level style fields. To mix alignment, "+
			"sizes, and emphasis line by line in one receipt, pass lines instead."),
		mcp.WithString("text", mcp.Description(`Text to print. Use \n for line breaks. Required unless "lines" is given; ignored when "lines" is given.`)),
		mcp.WithString("align", mcp.Enum("left", "center", "right"), mcp.Description("Horizontal alignment: left | center | right (default left).")),
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

	m.AddTool(mcp.NewTool("print_document",
		mcp.WithDescription("Print a whole receipt as one job: an ordered list of layout elements rendered "+
			"top to bottom with a single cut at the end. This is the way to lay out a real receipt — header, "+
			"itemized rows with prices flush-right, rules, totals, a barcode/QR, footer — using the printer's "+
			"crisp native text. Element types: text (wrapped paragraph), row (label left + value right), "+
			"columns (one row of N cells), table (many rows sharing column defs), rule (horizontal line), "+
			"feed (blank space), qr, barcode, image. Column layout (row/columns/table/rule) is accurate at the "+
			"default text size; use enlarged text only for centered headers, not columns."),
		mcp.WithNumber("columns", mcp.Description("Line width in characters. Default derives from the head width (80mm=48, 58mm=32).")),
		mcp.WithArray("elements", mcp.Required(),
			mcp.Description("Ordered list of layout elements. Each item has a \"type\" plus the fields for that type."),
			mcp.Items(docElementSchema()),
		),
		mcp.WithNumber("feed", mcp.Description("Extra blank lines fed after the document (default 0).")),
		mcp.WithBoolean("cut", mcp.Description("Cut the paper after printing (default true).")),
	), s.mcpPrintDocument)

	m.AddTool(mcp.NewTool("print_qr",
		mcp.WithDescription("Render a QR code from text/URL and print it, then optionally cut the paper."),
		mcp.WithString("data", mcp.Required(), mcp.Description("Text or URL to encode in the QR code.")),
		mcp.WithNumber("scale", mcp.Description("Module pixel size, ~6-10 prints cleanly (default 8).")),
		mcp.WithString("recovery", mcp.Enum("low", "medium", "high", "highest"), mcp.Description("Error-correction level: low | medium | high | highest (default medium).")),
		mcp.WithString("align", mcp.Enum("left", "center", "right"), mcp.Description("Horizontal alignment: left | center | right (default center).")),
		mcp.WithString("caption", mcp.Description("Optional text printed under the QR code.")),
		mcp.WithBoolean("cut", mcp.Description("Cut the paper after printing (default true).")),
	), s.mcpPrintQR)

	m.AddTool(mcp.NewTool("print_barcode",
		mcp.WithDescription("Render a barcode and print it, then optionally cut the paper. Supports 1D codes (CODE128, GS1-128, CODE39, CODE93, EAN-13/8, UPC-A, ITF, ITF-14, Standard 2 of 5, Codabar) and 2D codes (DataMatrix, PDF417)."),
		mcp.WithString("data", mcp.Required(), mcp.Description("Content to encode. Numeric symbologies (ean13, ean8, upca, itf, itf14, standard2of5) accept digits only.")),
		mcp.WithString("format", mcp.Enum(escpos.BarcodeFormats...), mcp.Description("Symbology: code128 | gs1-128 | code39 | code93 | ean13 | ean8 | upca | itf | itf14 | standard2of5 | codabar | datamatrix | pdf417 (default code128).")),
		mcp.WithNumber("width", mcp.Description("Total width in dots (1D: default ~2 dots/module; 2D: scales the whole symbol). Capped to the head width.")),
		mcp.WithNumber("height", mcp.Description("Bar height in dots for 1D codes (default 80). Ignored for 2D codes.")),
		mcp.WithBoolean("wide", mcp.Description("Print larger modules (1D ~4 dots/module, 2D ~10) for dense symbologies or finicky scanners (default false). Ignored when width is set.")),
		mcp.WithBoolean("hri", mcp.Description("Print the human-readable number under the code, grouped per symbology (EAN-8 4+4, EAN-13 1+6+6, UPC-A 1+5+5+1). Ignored if caption is set.")),
		mcp.WithString("align", mcp.Enum("left", "center", "right"), mcp.Description("Horizontal alignment: left | center | right (default center).")),
		mcp.WithString("caption", mcp.Description("Optional text printed under the barcode; overrides hri when set.")),
		mcp.WithBoolean("cut", mcp.Description("Cut the paper after printing (default true).")),
	), s.mcpPrintBarcode)

	m.AddTool(mcp.NewTool("print_svg",
		mcp.WithDescription("Render SVG markup to a raster and print it — the universal layout escape hatch. "+
			"Use this to print any layout the native character grid can't express: logos, free positioning, "+
			"rotation, shapes, rules, gradients, embedded images. Rendered as a 1-bit dithered raster. "+
			"Text renders in fonts bundled in the binary for determinism (no system fonts): Roboto (sans-serif), "+
			"Gelasio (serif), Go Mono (monospace). Generic families and common named fonts map onto these; an "+
			"unrecognized font falls back to sans-serif. font-weight/font-style are NOT differentiated, so for crisp "+
			"bold or styled receipt text prefer print_document / print_text. Reach for SVG only when the grid is not enough."),
		mcp.WithString("svg", mcp.Required(), mcp.Description("SVG markup. The root <svg> must declare a width and height (or a viewBox) so it has an intrinsic size.")),
		mcp.WithNumber("width", mcp.Description("Target raster width in dots. Defaults to and is capped at the head width; a smaller value prints narrower and is positioned by align.")),
		mcp.WithString("align", mcp.Enum("left", "center", "right"), mcp.Description("Horizontal alignment when narrower than the head: left | center | right (default center).")),
		mcp.WithBoolean("cut", mcp.Description("Cut the paper after printing (default true).")),
	), s.mcpPrintSVG)

	m.AddTool(mcp.NewTool("print_sample_receipt",
		mcp.WithDescription("Print the built-in sample receipt: a fully laid-out, itemized receipt (header, rows with "+
			"prices flush-right, rules, totals, a code, footer) built into the binary. Takes no arguments. Use it to "+
			"see what print_document produces without authoring a payload."),
	), s.mcpPrintSampleReceipt)

	m.AddTool(mcp.NewTool("print_sample_svg",
		mcp.WithDescription("Print the built-in SVG showcase: a rich SVG demonstrating fonts, shapes, and layout, built "+
			"into the binary. Takes no arguments. Use it to see what print_svg produces without authoring a payload."),
	), s.mcpPrintSampleSVG)

	m.AddTool(mcp.NewTool("print_image",
		mcp.WithDescription("Print a base64-encoded image (PNG/JPEG/GIF) as a dithered raster, scaled to fit the head, then optionally cut."),
		mcp.WithString("image_base64", mcp.Required(), mcp.Description("Base64-encoded PNG, JPEG, or GIF image data.")),
		mcp.WithString("align", mcp.Enum("left", "center", "right"), mcp.Description("Horizontal alignment: left | center | right (default center).")),
		mcp.WithBoolean("cut", mcp.Description("Cut the paper after printing (default true).")),
	), s.mcpPrintImage)

	m.AddTool(mcp.NewTool("cut",
		mcp.WithDescription("Feed a few lines clear of the head and perform a partial cut."),
	), s.mcpCut)

	// A single static resource that publishes the printer's capabilities as
	// machine-readable JSON so agents can discover limits (head width, barcode
	// formats, fonts, image formats, detected device) without parsing prose.
	m.AddResource(mcp.NewResource(
		"shortorder://capabilities",
		"capabilities",
		mcp.WithResourceDescription("Machine-readable printer capabilities: head width and columns, "+
			"supported/detected devices, barcode formats, QR recovery levels, SVG fonts and caveats, "+
			"image formats, and text size ranges."),
		mcp.WithMIMEType("application/json"),
	), s.mcpCapabilities)

	// Prompts: reusable payload templates an agent can list and expand. They
	// complement the print_sample_* tools (which print) by teaching the agent
	// how to AUTHOR a payload for the common "what's possible" jobs.
	m.AddPrompt(mcp.NewPrompt("receipt",
		mcp.WithPromptDescription("Lay out a complete itemized receipt."),
		mcp.WithArgument("store", mcp.ArgumentDescription("Store / header name printed at the top (default ACME CAFE).")),
		mcp.WithArgument("items", mcp.ArgumentDescription("Free-text description of the line items to fill into the table.")),
	), s.mcpPromptReceipt)

	m.AddPrompt(mcp.NewPrompt("logo_header",
		mcp.WithPromptDescription("Print a centered logo/wordmark header with SVG."),
		mcp.WithArgument("name", mcp.ArgumentDescription("The wordmark text to center in the header (default SHORT ORDER).")),
	), s.mcpPromptLogoHeader)

	m.AddPrompt(mcp.NewPrompt("loyalty_qr",
		mcp.WithPromptDescription("Print a scannable QR with a caption."),
		mcp.WithArgument("url", mcp.ArgumentDescription("The link to encode in the QR code (default https://example.com).")),
		mcp.WithArgument("caption", mcp.ArgumentDescription("Text printed under the QR code (default Scan me).")),
	), s.mcpPromptLoyaltyQR)

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

func (s *Server) mcpPrintDocument(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	elements := parseDocElements(req.GetArguments()["elements"])
	if len(elements) == 0 {
		return mcp.NewToolResultError("elements is required and must be non-empty"), nil
	}
	cut := req.GetBool("cut", true)
	job := documentRequest{
		Columns:  req.GetInt("columns", 0),
		Elements: elements,
		Feed:     req.GetInt("feed", 0),
		Cut:      &cut,
	}
	data, err := buildDocument(job, s.cfg.Width)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return s.mcpPrint("shortorder-document", data)
}

// parseDocElements converts the MCP "elements" argument (a JSON array of
// objects) into docElements, mirroring how the HTTP handler decodes the same
// shape from JSON. Non-object entries are skipped; missing fields take their
// zero value.
func parseDocElements(v any) []docElement {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]docElement, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, docElement{
			Type:        mapString(m, "type"),
			Text:        mapString(m, "text"),
			Align:       mapString(m, "align"),
			Bold:        mapBool(m, "bold"),
			Underline:   byte(mapInt(m, "underline")),
			Width:       mapInt(m, "width"),
			Height:      mapInt(m, "height"),
			Left:        mapString(m, "left"),
			Right:       mapString(m, "right"),
			Char:        mapString(m, "char"),
			Lines:       mapInt(m, "lines"),
			Columns:     parseDocColumns(m["columns"]),
			Cells:       toStringSlice(m["cells"]),
			Rows:        parseRows(m["rows"]),
			Gap:         mapInt(m, "gap"),
			Data:        mapString(m, "data"),
			Scale:       mapInt(m, "scale"),
			Recovery:    mapString(m, "recovery"),
			Caption:     mapString(m, "caption"),
			Format:      mapString(m, "format"),
			Wide:        mapBool(m, "wide"),
			HRI:         mapBool(m, "hri"),
			ImageBase64: mapString(m, "image_base64"),
		})
	}
	return out
}

func parseDocColumns(v any) []docColumn {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]docColumn, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, docColumn{Width: mapInt(m, "width"), Align: mapString(m, "align")})
	}
	return out
}

func parseRows(v any) [][]string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([][]string, 0, len(arr))
	for _, item := range arr {
		out = append(out, toStringSlice(item))
	}
	return out
}

// toStringSlice coerces a JSON array into []string, stringifying numbers and
// bools so a caller that passes a numeric cell (e.g. a quantity) still works.
func toStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		switch s := item.(type) {
		case string:
			out = append(out, s)
		case float64:
			out = append(out, strconv.FormatFloat(s, 'f', -1, 64))
		case bool:
			out = append(out, strconv.FormatBool(s))
		case nil:
			out = append(out, "")
		default:
			out = append(out, fmt.Sprint(item))
		}
	}
	return out
}

// docElementSchema is the JSON Schema for one element in the print_document
// "elements" array. It is intentionally permissive (no per-type required
// fields) since one schema covers every element type.
func docElementSchema() map[string]any {
	align := map[string]any{"type": "string", "enum": []string{"left", "center", "right"}}
	column := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"width": map[string]any{"type": "number", "description": "Column width in characters; 0 or omitted = auto (share remaining width)."},
			"align": align,
		},
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"type":         map[string]any{"type": "string", "enum": []string{"text", "row", "columns", "table", "rule", "feed", "qr", "barcode", "image"}, "description": "Element type (default text)."},
			"text":         map[string]any{"type": "string", "description": "text element: content; \\n for hard breaks. Wrapped to the line width at default size."},
			"align":        align,
			"bold":         map[string]any{"type": "boolean"},
			"underline":    map[string]any{"type": "number", "enum": []int{0, 1, 2}},
			"width":        map[string]any{"type": "number", "description": "text element: width size magnification 1-8 (enlarged text isn't column-wrapped)."},
			"height":       map[string]any{"type": "number", "description": "text element: height size magnification 1-8."},
			"left":         map[string]any{"type": "string", "description": "row element: left/label text."},
			"right":        map[string]any{"type": "string", "description": "row element: right/value text, kept flush to the right edge."},
			"char":         map[string]any{"type": "string", "description": "rule element: single fill character (default '-')."},
			"lines":        map[string]any{"type": "number", "description": "feed element: number of blank lines (default 1)."},
			"columns":      map[string]any{"type": "array", "items": column, "description": "columns/table element: per-column width and alignment. Omit for equal auto columns."},
			"cells":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "columns element: one row of cell texts."},
			"rows":         map[string]any{"type": "array", "items": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "description": "table element: rows of cell texts."},
			"gap":          map[string]any{"type": "number", "description": "columns/table element: blank cells between columns (default 1)."},
			"data":         map[string]any{"type": "string", "description": "qr/barcode element: content to encode."},
			"scale":        map[string]any{"type": "number", "description": "qr element: module pixel size (default 8)."},
			"recovery":     map[string]any{"type": "string", "enum": []string{"low", "medium", "high", "highest"}, "description": "qr element: error-correction level."},
			"caption":      map[string]any{"type": "string", "description": "qr/barcode element: text printed under the code."},
			"format":       map[string]any{"type": "string", "description": "barcode element: symbology (code128, ean13, upca, datamatrix, pdf417, ...; default code128)."},
			"wide":         map[string]any{"type": "boolean", "description": "barcode element: larger modules for dense codes / finicky scanners."},
			"hri":          map[string]any{"type": "boolean", "description": "barcode element: print the human-readable number under the code."},
			"image_base64": map[string]any{"type": "string", "description": "image element: base64 PNG/JPEG/GIF, scaled to fit the head."},
		},
		"required": []string{"type"},
	}
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

func (s *Server) mcpPrintSVG(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	svg, err := req.RequireString("svg")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	cut := req.GetBool("cut", true)
	job := svgRequest{
		SVG:   svg,
		Width: req.GetInt("width", 0),
		Align: req.GetString("align", ""),
		Cut:   &cut,
	}
	data, err := buildSVG(job, s.cfg.Width)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return s.mcpPrint("shortorder-svg", data)
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

func (s *Server) mcpPrintSampleReceipt(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var job documentRequest
	if err := json.Unmarshal(sampleReceiptJSON, &job); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("embedded sample receipt is invalid: %v", err)), nil
	}
	data, err := buildDocument(job, s.cfg.Width)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return s.mcpPrint("shortorder-sample-receipt", data)
}

func (s *Server) mcpPrintSampleSVG(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	data, err := buildSVG(svgRequest{SVG: string(sampleShowcaseSVG)}, s.cfg.Width)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return s.mcpPrint("shortorder-sample-svg", data)
}

func (s *Server) mcpCut(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.mcpPrint("shortorder-cut", escpos.New().Cut().Bytes())
}

// ---- resource handlers ---------------------------------------------------

// mcpCapabilities serves the shortorder://capabilities resource: a single JSON
// object describing what this printer can do (head geometry, supported/detected
// devices, barcode/QR/SVG/image/text limits). Values are pulled from the same
// sources the tools use so the resource never drifts from real behavior.
func (s *Server) mcpCapabilities(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	// Columns derive from head width exactly as buildDocument does (dots/12:
	// 576 dots -> 48 cols for 80mm, 384 dots -> 32 cols for 58mm).
	cols := escpos.Cols(s.cfg.Width)
	// Cols(dots) crosses from 32 to 48 columns around 512 dots, the same
	// threshold that distinguishes a 58mm head from an 80mm head.
	widthMm := 58
	if s.cfg.Width >= 512 {
		widthMm = 80
	}

	// Detect() is best-effort: an agent reading capabilities with no printer
	// plugged in should still get the static facts, so swallow the error and
	// report an empty detected list rather than failing the resource.
	detected, _ := printer.Detect()
	if detected == nil {
		detected = []printer.Info{}
	}

	caps := map[string]any{
		"head": map[string]any{
			"widthDots": s.cfg.Width,
			"columns":   cols,
			"widthMm":   widthMm,
		},
		"device": map[string]any{
			"supported": printer.SupportedModels(),
			"detected":  detected,
		},
		"barcode": map[string]any{
			"formats": escpos.BarcodeFormats,
			"twoD":    []string{"datamatrix", "pdf417"},
			"hri":     "human-readable number can be printed under 1D codes, grouped per symbology",
		},
		"qr": map[string]any{
			"recovery":     []string{"low", "medium", "high", "highest"},
			"defaultScale": 8,
		},
		"svg": map[string]any{
			"fonts": []map[string]any{
				{"name": "Roboto", "family": "sans-serif"},
				{"name": "Gelasio", "family": "serif"},
				{"name": "Go Mono", "family": "monospace"},
			},
			"raster": "1-bit Floyd–Steinberg dithered",
			"caveats": []string{
				"font-weight/font-style not differentiated — use print_text/print_document for genuine bold",
			},
		},
		"image": map[string]any{
			"formats":   []string{"png", "jpeg", "gif"},
			"rendering": "dithered, scaled to fit head",
		},
		"text": map[string]any{
			"sizeRange": map[string]any{"min": 1, "max": 8},
			"underline": []int{0, 1, 2},
		},
	}

	data, err := json.Marshal(caps)
	if err != nil {
		return nil, err
	}
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "shortorder://capabilities",
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}

// ---- prompt handlers -----------------------------------------------------

// mcpPromptReceipt expands the `receipt` prompt: a guide plus a concrete,
// copy-pasteable print_document payload that lays out a full itemized receipt.
func (s *Server) mcpPromptReceipt(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	store := req.Params.Arguments["store"]
	if store == "" {
		store = "ACME CAFE"
	}
	items := req.Params.Arguments["items"]
	if items == "" {
		items = "(describe the line items here; one table row per item: quantity, name, price)"
	}

	text := "Lay out a complete itemized receipt by calling the `print_document` tool. " +
		"Pass an ordered `elements` array; column-aware elements (row, table, rule) are accurate " +
		"at the default text size, so only enlarge text for centered headers.\n\n" +
		"Header: `" + store + "`. Line items to fill into the table rows below: " + items + "\n\n" +
		"Replace the example rows with your items (each row is [quantity, name, price]), recompute " +
		"the TOTAL, and set the barcode `data` to your order id. Example `print_document` arguments:\n\n" +
		"```json\n" +
		"{\n" +
		"  \"elements\": [\n" +
		"    { \"type\": \"text\", \"text\": \"" + store + "\", \"align\": \"center\", \"bold\": true, \"width\": 2, \"height\": 2 },\n" +
		"    { \"type\": \"text\", \"text\": \"123 Main St\", \"align\": \"center\" },\n" +
		"    { \"type\": \"rule\", \"char\": \"=\" },\n" +
		"    { \"type\": \"row\", \"left\": \"Order #1042\", \"right\": \"2026-06-08 14:32\" },\n" +
		"    { \"type\": \"rule\" },\n" +
		"    { \"type\": \"table\",\n" +
		"      \"columns\": [ { \"width\": 3, \"align\": \"left\" }, { \"width\": 0, \"align\": \"left\" }, { \"width\": 9, \"align\": \"right\" } ],\n" +
		"      \"rows\": [\n" +
		"        [ \"2\", \"Cappuccino\", \"$9.00\" ],\n" +
		"        [ \"1\", \"Avocado Toast\", \"$12.50\" ],\n" +
		"        [ \"3\", \"Drip Coffee\", \"$7.50\" ]\n" +
		"      ]\n" +
		"    },\n" +
		"    { \"type\": \"rule\" },\n" +
		"    { \"type\": \"row\", \"left\": \"Subtotal\", \"right\": \"$29.00\" },\n" +
		"    { \"type\": \"row\", \"left\": \"Tax\", \"right\": \"$2.68\" },\n" +
		"    { \"type\": \"rule\", \"char\": \"=\" },\n" +
		"    { \"type\": \"row\", \"left\": \"TOTAL\", \"right\": \"$31.68\", \"bold\": true },\n" +
		"    { \"type\": \"feed\", \"lines\": 1 },\n" +
		"    { \"type\": \"barcode\", \"format\": \"code128\", \"data\": \"ORD-2026-1042\", \"hri\": true, \"align\": \"center\" },\n" +
		"    { \"type\": \"feed\", \"lines\": 1 },\n" +
		"    { \"type\": \"text\", \"text\": \"Thank you for dining with us!\", \"align\": \"center\" }\n" +
		"  ],\n" +
		"  \"cut\": true\n" +
		"}\n" +
		"```"

	return mcp.NewGetPromptResult(
		"A print_document payload template for a full itemized receipt.",
		[]mcp.PromptMessage{mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(text))},
	), nil
}

// mcpPromptLogoHeader expands the `logo_header` prompt: a guide plus a concrete
// print_svg payload that centers a wordmark over a white background.
func (s *Server) mcpPromptLogoHeader(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	name := req.Params.Arguments["name"]
	if name == "" {
		name = "SHORT ORDER"
	}

	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="384" height="120" viewBox="0 0 384 120">` +
		`<rect width="384" height="120" fill="white"/>` +
		`<text x="192" y="78" font-family="serif" font-size="48" text-anchor="middle">` + name + `</text>` +
		`</svg>`

	text := "Print a centered logo/wordmark header by calling the `print_svg` tool. SVG is the escape " +
		"hatch for layout the character grid can't express (custom fonts, free positioning, shapes); for " +
		"genuinely bold body text, `print_document` is crisper.\n\n" +
		"Pass this as the `svg` argument (it centers \"" + name + "\" over a white background):\n\n" +
		"```xml\n" + svg + "\n```"

	return mcp.NewGetPromptResult(
		"A print_svg payload template for a centered wordmark header.",
		[]mcp.PromptMessage{mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(text))},
	), nil
}

// mcpPromptLoyaltyQR expands the `loyalty_qr` prompt: a guide plus the minimal
// print_qr argument shape for a scannable QR with a caption.
func (s *Server) mcpPromptLoyaltyQR(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	url := req.Params.Arguments["url"]
	if url == "" {
		url = "https://example.com"
	}
	caption := req.Params.Arguments["caption"]
	if caption == "" {
		caption = "Scan me"
	}

	text := "Print a scannable QR with a caption by calling the `print_qr` tool. The minimal arguments " +
		"are `data` (the link to encode) and `caption`; `scale`, `recovery`, and `align` are optional.\n\n" +
		"```json\n" +
		"{\n" +
		"  \"data\": \"" + url + "\",\n" +
		"  \"caption\": \"" + caption + "\",\n" +
		"  \"align\": \"center\"\n" +
		"}\n" +
		"```"

	return mcp.NewGetPromptResult(
		"A print_qr payload template for a captioned, scannable QR code.",
		[]mcp.PromptMessage{mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(text))},
	), nil
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
