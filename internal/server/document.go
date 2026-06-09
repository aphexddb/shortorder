package server

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"net/http"
	"strings"

	"shortorder/internal/escpos"
)

// The document model composes a whole receipt as one ordered list of elements
// rendered into a single print job with a single cut at the end. It is the
// difference between issuing isolated print commands (each its own job, each
// cutting the paper) and laying out a real receipt: a logo, a header, an
// itemized table with prices flush-right, rules, totals, a barcode, a footer.
//
// Layout is done in the printer's native monospace font over a fixed character
// grid (see escpos.Cols / the layout helpers), so receipt text stays crisp and
// selectable rather than rasterized. Column-aware elements (row, columns,
// table, rule) are accurate at the default 1x text size; enlarged headers
// should be plain centered text.

// docColumn describes one column of a columns or table element.
type docColumn struct {
	Width int    `json:"width"` // character cells; 0 = auto (share remaining width)
	Align string `json:"align"` // left|center|right
}

// docElement is one item in a document. The type field selects which fields
// apply; unused fields are ignored. Supported types:
//
//	text     styled, word-wrapped paragraph (align, bold, underline, width/height size)
//	row      label left + value right, justified to the line ("Item        $4.99")
//	columns  one row of N cells with per-column width and alignment
//	table    many rows sharing one set of column definitions
//	rule     a horizontal rule across the line (char, default '-')
//	feed     blank vertical space (lines)
//	qr       a QR code (data, scale, recovery, align, caption)
//	barcode  a 1D/2D barcode (data, format, wide, hri, align, caption)
//	image    a base64 PNG/JPEG/GIF raster (image_base64, align)
type docElement struct {
	Type string `json:"type"`

	// text / shared styling
	Text      string `json:"text"`
	Align     string `json:"align"`
	Bold      bool   `json:"bold"`
	Underline byte   `json:"underline"`
	Width     int    `json:"width"`  // text size magnification 1..8
	Height    int    `json:"height"` // text size magnification 1..8

	// row
	Left  string `json:"left"`
	Right string `json:"right"`

	// rule
	Char string `json:"char"`

	// feed
	Lines int `json:"lines"`

	// columns / table
	Columns []docColumn `json:"columns"`
	Cells   []string    `json:"cells"` // columns: one row of cell texts
	Rows    [][]string  `json:"rows"`  // table: many rows of cell texts
	Gap     int         `json:"gap"`   // blank cells between columns (default 1)

	// qr / barcode
	Data     string `json:"data"`
	Scale    int    `json:"scale"`
	Recovery string `json:"recovery"`
	Caption  string `json:"caption"`
	Format   string `json:"format"`
	Wide     bool   `json:"wide"`
	HRI      bool   `json:"hri"`

	// image
	ImageBase64 string `json:"image_base64"`
}

// documentRequest is the body for POST /api/print/document.
type documentRequest struct {
	Columns  int          `json:"columns"` // line width in characters; 0 = derive from head width
	Elements []docElement `json:"elements"`
	Feed     int          `json:"feed"` // extra blank lines after the document
	Cut      *bool        `json:"cut"`  // cut after printing (default true)
}

func (s *Server) handleDocument(w http.ResponseWriter, r *http.Request) {
	var req documentRequest
	if !decode(w, r, &req) {
		return
	}
	if len(req.Elements) == 0 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("elements is required and must be non-empty"))
		return
	}
	data, err := buildDocument(req, s.cfg.Width)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.dispatch(w, "shortorder-document", data)
}

// buildDocument renders a document to ESC/POS at the given head width. Pure (no
// I/O) so the HTTP handler and the MCP tool share one code path. Every element
// is rendered into a single builder; feed and cut apply once, at the end.
func buildDocument(req documentRequest, headWidth int) ([]byte, error) {
	cols := req.Columns
	if cols <= 0 {
		cols = escpos.Cols(headWidth)
	}
	b := escpos.New()
	for i, el := range req.Elements {
		if err := writeElement(b, el, cols, headWidth); err != nil {
			return nil, fmt.Errorf("element %d (type %q): %w", i, el.Type, err)
		}
	}
	if req.Feed > 0 {
		b.Feed(req.Feed)
	}
	if cutOrDefault(req.Cut) {
		b.Cut()
	}
	return b.Bytes(), nil
}

// writeElement renders a single document element into the builder. cols is the
// line width in characters; headWidth is the head width in dots (for rasters).
func writeElement(b *escpos.Builder, el docElement, cols, headWidth int) error {
	switch strings.ToLower(strings.TrimSpace(el.Type)) {
	case "", "text":
		writeDocText(b, el, cols)
	case "row":
		writeStyledLines(b, escpos.Row(el.Left, el.Right, cols), el.Bold, el.Underline)
	case "columns":
		columns := resolveDocColumns(el.Columns, len(el.Cells), cols, docGap(el.Gap))
		writeStyledLines(b, escpos.Columns(el.Cells, columns, docGap(el.Gap)), el.Bold, el.Underline)
	case "table":
		writeDocTable(b, el, cols)
	case "rule", "divider", "hr":
		b.Align(escpos.AlignLeft)
		b.Line(escpos.Rule(ruleChar(el.Char), cols))
	case "feed", "space":
		n := el.Lines
		if n < 1 {
			n = 1
		}
		b.Feed(n)
	case "qr":
		return writeQR(b, qrRequest{
			Data:     el.Data,
			Scale:    el.Scale,
			Recovery: el.Recovery,
			Align:    el.Align,
			Caption:  el.Caption,
		}, headWidth)
	case "barcode":
		return writeBarcode(b, barcodeRequest{
			Data:    el.Data,
			Format:  el.Format,
			Wide:    el.Wide,
			HRI:     el.HRI,
			Align:   el.Align,
			Caption: el.Caption,
		}, headWidth)
	case "image":
		return writeDocImage(b, el, headWidth)
	default:
		return fmt.Errorf("unknown element type %q", el.Type)
	}
	return nil
}

// writeDocText renders a styled, word-wrapped paragraph. At the default 1x size
// the text is wrapped to the line width; enlarged text honors only explicit
// newlines, since wider glyphs no longer map to the character grid.
func writeDocText(b *escpos.Builder, el docElement, cols int) {
	b.Align(parseAlign(el.Align))
	sized := el.Width > 0 || el.Height > 0
	if sized {
		b.Size(orOne(el.Width), orOne(el.Height))
	}
	if el.Bold {
		b.Bold(true)
	}
	if el.Underline > 0 {
		b.Underline(el.Underline)
	}
	if sized {
		for _, ln := range strings.Split(el.Text, "\n") {
			b.Line(ln)
		}
	} else {
		for _, ln := range escpos.Wrap(el.Text, cols) {
			b.Line(ln)
		}
	}
	b.Bold(false).Underline(0).Size(1, 1)
}

// writeDocTable renders each row through the shared column definitions. el.Bold
// emphasizes the whole table (e.g. a totals block).
func writeDocTable(b *escpos.Builder, el docElement, cols int) {
	gap := docGap(el.Gap)
	columns := resolveDocColumns(el.Columns, maxRowLen(el.Rows), cols, gap)
	for _, row := range el.Rows {
		writeStyledLines(b, escpos.Columns(row, columns, gap), el.Bold, el.Underline)
	}
}

func writeDocImage(b *escpos.Builder, el docElement, headWidth int) error {
	raw, err := base64.StdEncoding.DecodeString(el.ImageBase64)
	if err != nil {
		return fmt.Errorf("decode image_base64: %w", err)
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("decode image (png/jpeg/gif): %w", err)
	}
	writeImage(b, img, headWidth, parseAlignDefault(el.Align, escpos.AlignCenter))
	return nil
}

// writeStyledLines prints already-laid-out lines left-aligned (their internal
// spacing is the layout), optionally bold/underlined for the whole block.
func writeStyledLines(b *escpos.Builder, lines []string, bold bool, underline byte) {
	b.Align(escpos.AlignLeft)
	if bold {
		b.Bold(true)
	}
	if underline > 0 {
		b.Underline(underline)
	}
	for _, ln := range lines {
		b.Line(ln)
	}
	if underline > 0 {
		b.Underline(0)
	}
	if bold {
		b.Bold(false)
	}
}

// resolveDocColumns turns the request's column definitions into resolved
// escpos.Columns. With no definitions, n equal auto columns are used (one per
// cell). Auto widths are then filled to span the line.
func resolveDocColumns(dcs []docColumn, nCells, total, gap int) []escpos.Column {
	var columns []escpos.Column
	if len(dcs) == 0 {
		n := nCells
		if n < 1 {
			n = 1
		}
		columns = make([]escpos.Column, n) // all auto, left-aligned
	} else {
		columns = make([]escpos.Column, len(dcs))
		for i, dc := range dcs {
			columns[i] = escpos.Column{Width: dc.Width, Align: parseAlign(dc.Align)}
		}
	}
	return escpos.ResolveWidths(columns, total, gap)
}

// docGap defaults the inter-column gap to 1 cell when unset (0).
func docGap(g int) int {
	if g <= 0 {
		return 1
	}
	return g
}

// ruleChar picks the single fill byte for a rule. The head renders its Font A
// code page one byte per cell, so only a single-byte (ASCII printable) char is
// used as given; an empty or multibyte value (e.g. a box-drawing rune, which
// would print as mojibake) falls back to '-'.
func ruleChar(s string) byte {
	if len(s) == 1 && s[0] >= 0x20 && s[0] < 0x7f {
		return s[0]
	}
	return '-'
}

func maxRowLen(rows [][]string) int {
	m := 0
	for _, r := range rows {
		if len(r) > m {
			m = len(r)
		}
	}
	return m
}
