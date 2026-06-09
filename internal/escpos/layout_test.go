package escpos

import (
	"strings"
	"testing"
)

func TestCols(t *testing.T) {
	cases := []struct {
		dots, want int
	}{
		{576, 48}, // 80mm
		{384, 32}, // 58mm
		{0, DefaultCols},
		{-5, DefaultCols},
		{6, 1}, // sub-cell width floors at 1
	}
	for _, c := range cases {
		if got := Cols(c.dots); got != c.want {
			t.Errorf("Cols(%d) = %d, want %d", c.dots, got, c.want)
		}
	}
}

func TestRule(t *testing.T) {
	if got := Rule('-', 5); got != "-----" {
		t.Errorf("Rule('-',5) = %q", got)
	}
	if got := Rule(0, 3); got != "---" {
		t.Errorf("Rule(0,3) default char = %q, want ---", got)
	}
	if got := Rule('=', 0); got != "=" {
		t.Errorf("Rule('=',0) = %q, want single =", got)
	}
}

func TestWrapWordBoundaries(t *testing.T) {
	got := Wrap("the quick brown fox", 9)
	want := []string{"the quick", "brown fox"}
	assertLines(t, "Wrap words", got, want)
}

func TestWrapHonorsNewlines(t *testing.T) {
	got := Wrap("a\n\nb", 10)
	want := []string{"a", "", "b"}
	assertLines(t, "Wrap newlines", got, want)
}

func TestWrapHardSplitsLongWord(t *testing.T) {
	got := Wrap("supercalifragilistic", 8)
	want := []string{"supercal", "ifragili", "stic"}
	assertLines(t, "Wrap long word", got, want)
}

func TestRowJustifies(t *testing.T) {
	got := Row("Cheeseburger", "$8.50", 24)
	if len(got) != 1 {
		t.Fatalf("Row 1 line expected, got %v", got)
	}
	if ln := got[0]; len(ln) != 24 || !strings.HasPrefix(ln, "Cheeseburger") || !strings.HasSuffix(ln, "$8.50") {
		t.Fatalf("Row justify = %q (len %d)", ln, len(ln))
	}
}

func TestRowWrapsLeftKeepsValueOnLastLine(t *testing.T) {
	got := Row("A very long item description here", "$12.00", 20)
	if len(got) < 2 {
		t.Fatalf("expected wrap, got %v", got)
	}
	last := got[len(got)-1]
	if !strings.HasSuffix(last, "$12.00") || len(last) != 20 {
		t.Fatalf("value not flush-right on last line: %q (len %d)", last, len(last))
	}
	for _, ln := range got[:len(got)-1] {
		if strings.Contains(ln, "$12.00") {
			t.Fatalf("value leaked onto non-final line: %q", ln)
		}
	}
}

func TestColumnsAlignAndWrap(t *testing.T) {
	cols := []Column{
		{Width: 3, Align: AlignLeft},
		{Width: 10, Align: AlignLeft},
		{Width: 6, Align: AlignRight},
	}
	got := Columns([]string{"2", "Cappuccino", "$9.00"}, cols, 1)
	// 3 + 1 + 10 + 1 + 6 = 21 cells per line
	for _, ln := range got {
		if len(ln) != 21 {
			t.Fatalf("column line width = %d, want 21: %q", len(ln), ln)
		}
	}
	if !strings.HasSuffix(got[0], " $9.00") {
		t.Fatalf("right column not right-aligned: %q", got[0])
	}
}

func TestColumnsWrapsTallCell(t *testing.T) {
	cols := []Column{{Width: 4, Align: AlignLeft}, {Width: 6, Align: AlignLeft}}
	got := Columns([]string{"x", "alpha beta gamma"}, cols, 1)
	if len(got) < 2 {
		t.Fatalf("expected multi-line wrap, got %v", got)
	}
	// First column only present on first physical line; padded blank after.
	if !strings.HasPrefix(got[0], "x   ") {
		t.Fatalf("first cell not left-padded: %q", got[0])
	}
	if !strings.HasPrefix(got[1], "    ") {
		t.Fatalf("continuation row should blank the first column: %q", got[1])
	}
}

func TestResolveWidthsDistributes(t *testing.T) {
	// One fixed (4) + two autos, total 20, gap 1 (2 gaps = 2).
	// budget for autos = 20 - 2 - 4 = 14 -> 7 and 7.
	cols := []Column{{Width: 4}, {Width: 0}, {Width: 0}}
	got := ResolveWidths(cols, 20, 1)
	if got[0].Width != 4 || got[1].Width != 7 || got[2].Width != 7 {
		t.Fatalf("ResolveWidths = %+v, want [4 7 7]", got)
	}
}

func TestResolveWidthsRemainderToEarliest(t *testing.T) {
	// Two autos, total 9, no fixed, gap 1 (1 gap). budget = 8 -> 4 and 4.
	// total 10 -> budget 9 -> 5 and 4.
	cols := []Column{{Width: 0}, {Width: 0}}
	got := ResolveWidths(cols, 10, 1)
	if got[0].Width != 5 || got[1].Width != 4 {
		t.Fatalf("ResolveWidths remainder = %+v, want [5 4]", got)
	}
}

func TestResolveWidthsOverflowFloorsAtOne(t *testing.T) {
	cols := []Column{{Width: 30}, {Width: 0}}
	got := ResolveWidths(cols, 20, 1)
	if got[1].Width != 1 {
		t.Fatalf("auto column under pressure = %d, want 1", got[1].Width)
	}
}

func assertLines(t *testing.T, name string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %d lines %q, want %d %q", name, len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s line %d = %q, want %q", name, i, got[i], want[i])
		}
	}
}
