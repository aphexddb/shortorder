package escpos

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

func TestNewEmitsInit(t *testing.T) {
	got := New().Bytes()
	want := []byte{esc, '@'}
	if !bytes.Equal(got, want) {
		t.Fatalf("New() = %v, want init sequence %v", got, want)
	}
}

func TestTextAndCut(t *testing.T) {
	got := New().Align(AlignCenter).Bold(true).Line("hi").Bold(false).Bytes()
	// ESC @, ESC a 1, ESC E 1, "hi\n", ESC E 0
	want := []byte{esc, '@', esc, 'a', 1, esc, 'E', 1, 'h', 'i', lf, esc, 'E', 0}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
}

func TestCutEmitsGSV(t *testing.T) {
	got := New().Cut().Bytes()
	// must end with the GS V 66 0 partial-cut sequence
	if !bytes.HasSuffix(got, []byte{gs, 'V', 66, 0}) {
		t.Fatalf("Cut() did not end with GS V 66 0: %v", got)
	}
}

func TestSizeNibbles(t *testing.T) {
	// width=2,height=3 -> high nibble (2-1)=1, low nibble (3-1)=2 -> 0x12
	got := (&Builder{}).Size(2, 3).Bytes()
	want := []byte{gs, '!', 0x12}
	if !bytes.Equal(got, want) {
		t.Fatalf("Size(2,3) = %v, want %v", got, want)
	}
}

func TestImageRasterHeader(t *testing.T) {
	// 16x2 image: widthBytes = 2, height = 2.
	img := image.NewRGBA(image.Rect(0, 0, 16, 2))
	for x := 0; x < 16; x++ {
		img.Set(x, 0, color.Black)
		img.Set(x, 1, color.White)
	}
	got := (&Builder{}).Image(img).Bytes()
	// GS v 0 m xL xH yL yH ...
	header := got[:8]
	want := []byte{gs, 'v', '0', 0, 2, 0, 2, 0}
	if !bytes.Equal(header, want) {
		t.Fatalf("raster header = %v, want %v", header, want)
	}
	// total = 8-byte header + widthBytes*height = 8 + 4 data bytes
	if len(got) != 12 {
		t.Fatalf("raster len = %d, want 12", len(got))
	}
	// first row all black -> 0xFF 0xFF
	if got[8] != 0xFF || got[9] != 0xFF {
		t.Fatalf("black row = %x %x, want ff ff", got[8], got[9])
	}
}
