package escpos

import (
	"strings"
	"unicode/utf8"
)

// Text layout helpers for the column-oriented document model.
//
// These are pure string transforms over a fixed character grid: they wrap,
// pad, and arrange text into columns so a caller can build receipt layouts —
// itemized rows, tables, rules — out of the printer's native monospace font
// instead of rasterizing an image. They assume Font A (the default), whose
// glyphs are a fixed 12 dots wide, so a line holds a whole number of equal
// cells. Enlarged text (GS ! magnification) breaks that assumption, so column
// layout is only accurate at size 1×1; headers that need a bigger font should
// be plain centered text, not columns.

// DefaultCols is the column count assumed when the head width is unknown.
const DefaultCols = 48

// Cols converts a print-head width in dots to the number of Font A characters
// that fit on one line. Font A glyphs occupy 12 dots, so an 80mm head (576
// dots) is 48 columns and a 58mm head (384 dots) is 32. A non-positive width
// falls back to DefaultCols.
func Cols(dots int) int {
	if dots <= 0 {
		return DefaultCols
	}
	if c := dots / 12; c >= 1 {
		return c
	}
	return 1
}

// Column describes one column of a Columns or table layout.
type Column struct {
	Width int   // width in character cells; <= 0 means auto (resolved by ResolveWidths)
	Align Align // horizontal alignment of the cell text within the column
}

// Rule returns a horizontal rule cols cells wide made of ch (default '-').
func Rule(ch byte, cols int) string {
	if ch == 0 {
		ch = '-'
	}
	if cols < 1 {
		cols = 1
	}
	return strings.Repeat(string(ch), cols)
}

// Wrap breaks s into lines no wider than cols cells. Newlines already in s are
// honored as hard breaks (and blank lines are preserved). Within a paragraph
// words are kept whole where they fit; a single word longer than cols is
// hard-split across lines. A cols < 1 is treated as 1.
func Wrap(s string, cols int) []string {
	if cols < 1 {
		cols = 1
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		out = append(out, wrapParagraph(para, cols)...)
	}
	return out
}

func wrapParagraph(para string, cols int) []string {
	words := strings.Fields(para)
	if len(words) == 0 {
		return []string{""} // preserve a blank line
	}
	var lines []string
	var cur strings.Builder
	curLen := 0
	flush := func() {
		lines = append(lines, cur.String())
		cur.Reset()
		curLen = 0
	}
	for _, w := range words {
		// Hard-split a word that cannot fit on a line by itself.
		for dispLen(w) > cols {
			if curLen > 0 {
				flush()
			}
			head, tail := splitRunes(w, cols)
			lines = append(lines, head)
			w = tail
		}
		wl := dispLen(w)
		need := wl
		if curLen > 0 {
			need++ // leading space
		}
		if curLen+need > cols {
			flush()
		}
		if curLen > 0 {
			cur.WriteByte(' ')
			curLen++
		}
		cur.WriteString(w)
		curLen += wl
	}
	if curLen > 0 || len(lines) == 0 {
		flush()
	}
	return lines
}

// Row lays out a left-aligned label and a right-aligned value on a line cols
// cells wide — the canonical receipt line item ("Cheeseburger      $8.50").
// The left text wraps when it would collide with the value; the value is kept
// whole, flush to the right edge of the last line. If the value alone is as
// wide as the line, the label is stacked above it.
func Row(left, right string, cols int) []string {
	if cols < 1 {
		cols = 1
	}
	if right == "" {
		return Wrap(left, cols)
	}
	rl := dispLen(right)
	leftWidth := cols - rl - 1 // reserve the value plus one separating space
	if leftWidth < 1 {
		out := Wrap(left, cols)
		out = append(out, padCol(right, cols, AlignRight))
		return out
	}
	leftLines := Wrap(left, leftWidth)
	out := make([]string, 0, len(leftLines))
	for i, ll := range leftLines {
		if i < len(leftLines)-1 {
			out = append(out, ll)
			continue
		}
		gap := cols - dispLen(ll) - rl
		if gap < 1 {
			gap = 1
		}
		out = append(out, ll+strings.Repeat(" ", gap)+right)
	}
	return out
}

// Columns lays out one row of cells into physical lines. Each cell is wrapped
// within its column width and aligned; cells with fewer wrapped lines than the
// tallest are blank-padded so the columns stay aligned down the row. Adjacent
// columns are separated by gap blank cells. Column widths must be resolved
// (all > 0) — use ResolveWidths to fill in autos first.
func Columns(cells []string, cols []Column, gap int) []string {
	if gap < 0 {
		gap = 0
	}
	wrapped := make([][]string, len(cols))
	widths := make([]int, len(cols))
	rows := 1
	for i := range cols {
		text := ""
		if i < len(cells) {
			text = cells[i]
		}
		w := cols[i].Width
		if w < 1 {
			w = 1 // tolerate an unresolved width rather than dropping the cell
		}
		widths[i] = w
		wrapped[i] = Wrap(text, w)
		if len(wrapped[i]) > rows {
			rows = len(wrapped[i])
		}
	}
	sep := strings.Repeat(" ", gap)
	out := make([]string, 0, rows)
	for r := 0; r < rows; r++ {
		parts := make([]string, len(cols))
		for i := range cols {
			cell := ""
			if r < len(wrapped[i]) {
				cell = wrapped[i][r]
			}
			parts[i] = padCol(cell, widths[i], cols[i].Align)
		}
		out = append(out, strings.Join(parts, sep))
	}
	return out
}

// ResolveWidths fills in auto (<= 0) column widths so the columns plus the gaps
// between them exactly fill total cells. Fixed widths are kept as-is; the
// leftover space is split as evenly as possible among the auto columns, with
// any remainder going to the earliest autos. Auto columns never drop below 1.
func ResolveWidths(cols []Column, total, gap int) []Column {
	if gap < 0 {
		gap = 0
	}
	out := make([]Column, len(cols))
	copy(out, cols)
	autos := 0
	fixed := 0
	for _, c := range out {
		if c.Width > 0 {
			fixed += c.Width
		} else {
			autos++
		}
	}
	if autos == 0 {
		return out
	}
	gaps := 0
	if len(out) > 1 {
		gaps = gap * (len(out) - 1)
	}
	rem := total - gaps - fixed
	if rem < autos {
		rem = autos // guarantee at least 1 cell per auto column
	}
	each := rem / autos
	extra := rem % autos
	for i := range out {
		if out[i].Width <= 0 {
			out[i].Width = each
			if extra > 0 {
				out[i].Width++
				extra--
			}
		}
	}
	return out
}

// padCol pads or aligns s to exactly width cells, truncating if it is longer.
func padCol(s string, width int, a Align) string {
	if width <= 0 {
		return ""
	}
	n := dispLen(s)
	if n > width {
		return truncRunes(s, width)
	}
	gap := width - n
	switch a {
	case AlignRight:
		return strings.Repeat(" ", gap) + s
	case AlignCenter:
		left := gap / 2
		return strings.Repeat(" ", left) + s + strings.Repeat(" ", gap-left)
	default:
		return s + strings.Repeat(" ", gap)
	}
}

// dispLen is the number of display cells s occupies, assuming each rune is one
// Font A cell wide (true for ASCII and most Latin text).
func dispLen(s string) int {
	return utf8.RuneCountInString(s)
}

// splitRunes splits s after n runes (not bytes), so multibyte text isn't cut
// mid-rune.
func splitRunes(s string, n int) (head, tail string) {
	if n <= 0 {
		return "", s
	}
	r := []rune(s)
	if len(r) <= n {
		return s, ""
	}
	return string(r[:n]), string(r[n:])
}

// truncRunes returns the first n runes of s.
func truncRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
