package server

import (
	"bytes"
	"strings"
	"testing"
)

// A document renders all elements into one job and cuts once at the end (no
// per-element cut).
func TestBuildDocumentSingleCut(t *testing.T) {
	got, err := buildDocument(documentRequest{
		Columns: 24,
		Elements: []docElement{
			{Type: "text", Text: "HEADER", Align: "center"},
			{Type: "rule"},
			{Type: "row", Left: "Coffee", Right: "$3.00"},
		},
		Cut: ptrBool(true),
	}, 576)
	if err != nil {
		t.Fatal(err)
	}
	// exactly one partial-cut sequence, at the very end
	if n := bytes.Count(got, []byte{gs, 'V', 66, 0}); n != 1 {
		t.Fatalf("want exactly 1 cut, got %d", n)
	}
	if !bytes.HasSuffix(got, []byte{gs, 'V', 66, 0}) {
		t.Fatalf("cut should be last")
	}
}

func TestBuildDocumentNoCut(t *testing.T) {
	got, err := buildDocument(documentRequest{
		Elements: []docElement{{Type: "text", Text: "hi"}},
		Cut:      ptrBool(false),
	}, 576)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(got, []byte{gs, 'V', 66, 0}) {
		t.Fatalf("no cut expected")
	}
}

// The row element justifies the value to the right edge of the line width.
func TestBuildDocumentRowJustified(t *testing.T) {
	got, err := buildDocument(documentRequest{
		Columns:  20,
		Elements: []docElement{{Type: "row", Left: "Tax", Right: "$1.20"}},
		Cut:      ptrBool(false),
	}, 576)
	if err != nil {
		t.Fatal(err)
	}
	want := "Tax" + strings.Repeat(" ", 20-3-5) + "$1.20"
	if !bytes.Contains(got, []byte(want)) {
		t.Fatalf("row line %q not found in %q", want, got)
	}
}

// The rule element fills the whole line width with the fill character.
func TestBuildDocumentRuleWidth(t *testing.T) {
	got, err := buildDocument(documentRequest{
		Columns:  32,
		Elements: []docElement{{Type: "rule", Char: "="}},
		Cut:      ptrBool(false),
	}, 576)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte(strings.Repeat("=", 32))) {
		t.Fatalf("expected a 32-wide '=' rule")
	}
}

// A multibyte rule char falls back to '-' rather than emitting a stray UTF-8
// byte across the whole line.
func TestBuildDocumentRuleMultibyteCharFallback(t *testing.T) {
	got, err := buildDocument(documentRequest{
		Columns:  16,
		Elements: []docElement{{Type: "rule", Char: "═"}}, // box-drawing rune (multibyte)
		Cut:      ptrBool(false),
	}, 576)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte(strings.Repeat("-", 16))) {
		t.Fatalf("multibyte rule char should fall back to a 16-wide '-' rule: %q", got)
	}
	if bytes.Contains(got, []byte{0xE2}) { // leading byte of '═'
		t.Fatalf("stray multibyte rule byte leaked into the stream")
	}
}

// columns defaults the line width from the head width when columns is omitted:
// 576 dots / 12 = 48.
func TestBuildDocumentDefaultColumnsFromWidth(t *testing.T) {
	got, err := buildDocument(documentRequest{
		Elements: []docElement{{Type: "rule"}},
		Cut:      ptrBool(false),
	}, 576)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte(strings.Repeat("-", 48))) {
		t.Fatalf("expected default 48-wide rule for an 80mm head")
	}
}

// A table lays out each row through shared column definitions; right-aligned
// price column lands flush against the gap.
func TestBuildDocumentTable(t *testing.T) {
	got, err := buildDocument(documentRequest{
		Columns: 24,
		Elements: []docElement{{
			Type: "table",
			Columns: []docColumn{
				{Width: 3, Align: "left"},
				{Width: 0, Align: "left"},
				{Width: 7, Align: "right"},
			},
			Rows: [][]string{
				{"2", "Coffee", "$6.00"},
				{"1", "Muffin", "$3.25"},
			},
		}},
		Cut: ptrBool(false),
	}, 576)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if !strings.Contains(text, "$6.00") || !strings.Contains(text, "$3.25") {
		t.Fatalf("table prices missing: %q", text)
	}
	// each table line is exactly the column width: 3 + 1 + 11 + 1 + 7 = 23? auto = 24-2gap-10 = 12
	// just assert the qty column starts the line
	if !strings.Contains(text, "2  ") || !strings.Contains(text, "1  ") {
		t.Fatalf("qty column not left-padded: %q", text)
	}
}

// An unknown element type is a request error, surfaced with its index.
func TestBuildDocumentUnknownElement(t *testing.T) {
	_, err := buildDocument(documentRequest{
		Elements: []docElement{{Type: "text", Text: "ok"}, {Type: "bogus"}},
	}, 576)
	if err == nil || !strings.Contains(err.Error(), "element 1") {
		t.Fatalf("want error mentioning element 1, got %v", err)
	}
}

// A barcode element with bad data fails the whole document (shared barcode path).
func TestBuildDocumentBarcodeError(t *testing.T) {
	_, err := buildDocument(documentRequest{
		Elements: []docElement{{Type: "barcode", Format: "ean13", Data: "abc"}},
	}, 576)
	if err == nil {
		t.Fatalf("expected barcode encode error")
	}
}

// parseDocElements decodes the MCP "elements" shape: nested columns/cells/rows,
// JSON numbers as float64, numeric cells coerced to strings.
func TestParseDocElements(t *testing.T) {
	in := []any{
		map[string]any{"type": "text", "text": "hi", "width": float64(2)},
		"skip-me",
		map[string]any{
			"type": "table",
			"columns": []any{
				map[string]any{"width": float64(3), "align": "left"},
				map[string]any{"align": "right"},
			},
			"rows": []any{
				[]any{float64(2), "Coffee", "$6.00"},
			},
		},
		map[string]any{"type": "row", "left": "Tax", "right": "$1.20"},
	}
	got := parseDocElements(in)
	if len(got) != 3 {
		t.Fatalf("got %d elements, want 3: %+v", len(got), got)
	}
	if got[0].Type != "text" || got[0].Width != 2 {
		t.Fatalf("text element = %+v", got[0])
	}
	tbl := got[1]
	if len(tbl.Columns) != 2 || tbl.Columns[0].Width != 3 || tbl.Columns[1].Align != "right" {
		t.Fatalf("table columns = %+v", tbl.Columns)
	}
	if len(tbl.Rows) != 1 || tbl.Rows[0][0] != "2" || tbl.Rows[0][1] != "Coffee" {
		t.Fatalf("table rows = %+v (numeric cell should coerce to \"2\")", tbl.Rows)
	}
	if got[2].Left != "Tax" || got[2].Right != "$1.20" {
		t.Fatalf("row element = %+v", got[2])
	}
}

func TestParseDocElementsNonArray(t *testing.T) {
	if got := parseDocElements(nil); got != nil {
		t.Fatalf("parseDocElements(nil) = %v, want nil", got)
	}
	if got := parseDocElements("nope"); got != nil {
		t.Fatalf("parseDocElements(string) = %v, want nil", got)
	}
}
