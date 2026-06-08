// Package escpos builds ESC/POS command streams for thermal receipt printers.
//
// ESC/POS is the de-facto command language for receipt printers (Epson and the
// many compatible clones, including the Volcora v-WRP2-A1W). A print job is just
// a byte stream: printable text is emitted verbatim, and control sequences
// (mostly ESC = 0x1B and GS = 0x1D prefixed) toggle formatting, render bitmaps,
// and drive the cutter.
package escpos

import (
	"bytes"
	"image"
)

// Control bytes.
const (
	esc = 0x1B
	gs  = 0x1D
	lf  = 0x0A
)

// Align is a horizontal justification mode.
type Align byte

const (
	AlignLeft   Align = 0
	AlignCenter Align = 1
	AlignRight  Align = 2
)

// Builder accumulates an ESC/POS command stream.
type Builder struct {
	buf bytes.Buffer
}

// New returns a Builder that has already emitted the printer-init sequence
// (ESC @), which clears any leftover formatting state from a prior job.
func New() *Builder {
	b := &Builder{}
	b.Init()
	return b
}

// Init resets the printer to its power-on defaults (ESC @).
func (b *Builder) Init() *Builder {
	b.buf.Write([]byte{esc, '@'})
	return b
}

// Raw appends arbitrary bytes to the stream unchanged.
func (b *Builder) Raw(p []byte) *Builder {
	b.buf.Write(p)
	return b
}

// Text appends text without a trailing newline.
func (b *Builder) Text(s string) *Builder {
	b.buf.WriteString(s)
	return b
}

// Line appends text followed by a line feed.
func (b *Builder) Line(s string) *Builder {
	b.buf.WriteString(s)
	b.buf.WriteByte(lf)
	return b
}

// Align sets horizontal justification (ESC a n).
func (b *Builder) Align(a Align) *Builder {
	b.buf.Write([]byte{esc, 'a', byte(a)})
	return b
}

// Bold toggles emphasized printing (ESC E n).
func (b *Builder) Bold(on bool) *Builder {
	var n byte
	if on {
		n = 1
	}
	b.buf.Write([]byte{esc, 'E', n})
	return b
}

// Underline sets underline weight: 0 off, 1 thin, 2 thick (ESC - n).
func (b *Builder) Underline(weight byte) *Builder {
	if weight > 2 {
		weight = 2
	}
	b.buf.Write([]byte{esc, '-', weight})
	return b
}

// Size sets the character magnification. width and height are 1..8 (GS ! n),
// where the low nibble is height and the high nibble is width.
func (b *Builder) Size(width, height int) *Builder {
	width = clamp(width, 1, 8)
	height = clamp(height, 1, 8)
	n := byte((width-1)<<4 | (height - 1))
	b.buf.Write([]byte{gs, '!', n})
	return b
}

// Feed advances the paper n lines (ESC d n).
func (b *Builder) Feed(n int) *Builder {
	if n < 0 {
		n = 0
	}
	for n > 255 {
		b.buf.Write([]byte{esc, 'd', 255})
		n -= 255
	}
	b.buf.Write([]byte{esc, 'd', byte(n)})
	return b
}

// Cut feeds a few lines clear of the print head and performs a partial cut
// (GS V 66 n — function B feeds n dots then cuts, leaving a small tab).
func (b *Builder) Cut() *Builder {
	b.Feed(3)
	b.buf.Write([]byte{gs, 'V', 66, 0})
	return b
}

// FullCut performs a full cut after feeding clear of the head (GS V 65 n).
func (b *Builder) FullCut() *Builder {
	b.Feed(3)
	b.buf.Write([]byte{gs, 'V', 65, 0})
	return b
}

// Image renders a monochrome raster of img using the GS v 0 raster bit-image
// command. The image is converted to 1-bit (Floyd–Steinberg-free, simple
// luminance threshold) and packed MSB-first into rows. Pixels are printed at
// the head's native density; scale/fit the image before calling if needed.
func (b *Builder) Image(img image.Image) *Builder {
	raster := pack(img)
	if raster == nil {
		return b
	}
	widthBytes := raster.widthBytes
	height := raster.height

	// GS v 0 m xL xH yL yH d1...dk
	header := []byte{
		gs, 'v', '0', 0,
		byte(widthBytes & 0xFF), byte((widthBytes >> 8) & 0xFF),
		byte(height & 0xFF), byte((height >> 8) & 0xFF),
	}
	b.buf.Write(header)
	b.buf.Write(raster.data)
	return b
}

// Bytes returns the accumulated command stream.
func (b *Builder) Bytes() []byte {
	return b.buf.Bytes()
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
