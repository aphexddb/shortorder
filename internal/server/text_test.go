package server

import (
	"bytes"
	"testing"
)

// ESC/POS control bytes, mirrored here so tests can assert exact streams
// without reaching into the escpos package's unexported constants.
const (
	esc = 0x1B
	gs  = 0x1D
	lf  = 0x0A
)

func ptrBool(b bool) *bool { return &b }

// A flat text request renders one styled segment, byte-identical to the
// pre-segment behavior: init, align, bold, line, then the style resets.
func TestBuildTextFlat(t *testing.T) {
	got := buildText(textRequest{Text: "hi", Bold: true, Cut: ptrBool(false)})
	want := []byte{
		esc, '@', // init
		esc, 'a', 0, // align left
		esc, 'E', 1, // bold on
		'h', 'i', lf, // line
		esc, 'E', 0, // bold off
		esc, '-', 0, // underline off
		gs, '!', 0, // size reset
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("buildText flat:\n got %v\nwant %v", got, want)
	}
}

// When lines is set, each segment is rendered with its own style and the
// top-level text is ignored.
func TestBuildTextLines(t *testing.T) {
	got := buildText(textRequest{
		Text: "IGNORED",
		Lines: []textSegment{
			{Text: "A", Align: "center"},
			{Text: "B"},
		},
		Cut: ptrBool(false),
	})
	want := []byte{
		esc, '@', // init
		// segment 1: centered "A"
		esc, 'a', 1,
		'A', lf,
		esc, 'E', 0, esc, '-', 0, gs, '!', 0,
		// segment 2: default "B"
		esc, 'a', 0,
		'B', lf,
		esc, 'E', 0, esc, '-', 0, gs, '!', 0,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("buildText lines:\n got %v\nwant %v", got, want)
	}
	if bytes.Contains(got, []byte("IGNORED")) {
		t.Fatalf("top-level text should be ignored when lines is set")
	}
}

// feed and cut apply once, after the segments.
func TestBuildTextLinesFeedAndCut(t *testing.T) {
	got := buildText(textRequest{
		Lines: []textSegment{{Text: "x"}},
		Feed:  2,
		Cut:   ptrBool(true),
	})
	// feed 2 then a partial cut (Cut() feeds 3 more, then GS V 66 0).
	if !bytes.Contains(got, []byte{esc, 'd', 2}) {
		t.Fatalf("expected feed of 2 lines; got %v", got)
	}
	if !bytes.HasSuffix(got, []byte{gs, 'V', 66, 0}) {
		t.Fatalf("expected trailing partial cut; got %v", got)
	}
}

// parseSegments decodes the MCP "lines" argument shape (JSON numbers arrive as
// float64) and skips anything that isn't an object.
func TestParseSegments(t *testing.T) {
	in := []any{
		map[string]any{
			"text":      "hdr",
			"align":     "right",
			"bold":      true,
			"underline": float64(2),
			"width":     float64(2),
			"height":    float64(3),
		},
		"not-an-object", // skipped
		map[string]any{"text": "body"},
	}
	got := parseSegments(in)
	want := []textSegment{
		{Text: "hdr", Align: "right", Bold: true, Underline: 2, Width: 2, Height: 3},
		{Text: "body"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d segments, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("segment %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseSegmentsNonArray(t *testing.T) {
	if got := parseSegments(nil); got != nil {
		t.Fatalf("parseSegments(nil) = %v, want nil", got)
	}
	if got := parseSegments("oops"); got != nil {
		t.Fatalf("parseSegments(non-array) = %v, want nil", got)
	}
}
