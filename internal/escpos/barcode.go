package escpos

import (
	"fmt"
	"image"
	"strings"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/codabar"
	"github.com/boombuler/barcode/code128"
	"github.com/boombuler/barcode/code39"
	"github.com/boombuler/barcode/code93"
	"github.com/boombuler/barcode/datamatrix"
	"github.com/boombuler/barcode/ean"
	"github.com/boombuler/barcode/pdf417"
	"github.com/boombuler/barcode/twooffive"
)

// BarcodeFormats lists the symbologies BarcodeImage understands. The names are
// what callers pass as the format argument (case-insensitive). The list mixes
// 1D codes and the 2D codes (datamatrix, pdf417) reported by barcode2D.
var BarcodeFormats = []string{
	"code128", "gs1-128", "code39", "code93",
	"ean13", "ean8", "upca",
	"itf", "itf14", "standard2of5", "codabar",
	"datamatrix", "pdf417",
}

// BarcodeImage renders data as a barcode of the given symbology and returns it
// as a raster image.
//
// For 1D codes the image is scaled to width×height dots. A width <= 0 picks a
// sensible default module width — two dots per module normally, or four when
// wide is set — never narrower than one dot per module; an explicit width > 0
// overrides both. A height <= 0 defaults to 80 dots.
//
// For 2D codes (DataMatrix, PDF417) height is ignored and the modules are
// scaled uniformly (preserving aspect): ~6 dots per module normally, ~10 when
// wide is set, or scaled to an explicit width when one is given.
//
// As with QRImage, rendering to a raster (rather than the printer's native
// GS k barcode commands) keeps the output identical across the many ESC/POS
// clones, some of which implement only a subset of the native symbologies.
func BarcodeImage(data, format string, width, height int, wide bool) (image.Image, error) {
	if data == "" {
		return nil, fmt.Errorf("data is required")
	}
	bc, err := encodeBarcode(format, data)
	if err != nil {
		return nil, err
	}
	b := bc.Bounds()
	nw, nh := b.Dx(), b.Dy()
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}

	if barcode2D(format) {
		// 2D: scale modules uniformly so the code stays square / keeps its
		// aspect ratio. width overrides the per-module default; wide enlarges.
		scale := 6
		if wide {
			scale = 10
		}
		tw, th := nw*scale, nh*scale
		if width > 0 {
			tw = width
			th = nh * width / nw
		}
		if tw < nw {
			tw = nw
		}
		if th < nh {
			th = nh
		}
		scaled, err := barcode.Scale(bc, tw, th)
		if err != nil {
			return nil, fmt.Errorf("scale barcode: %w", err)
		}
		return scaled, nil
	}

	// 1D: the unscaled barcode is one module per pixel wide; use that as the
	// floor so scaling never collapses modules together.
	if height < 1 {
		height = 80
	}
	if width < 1 {
		perModule := 2
		if wide {
			perModule = 4
		}
		width = nw * perModule
	}
	if width < nw {
		width = nw
	}
	scaled, err := barcode.Scale(bc, width, height)
	if err != nil {
		return nil, fmt.Errorf("scale barcode: %w", err)
	}
	return scaled, nil
}

// BarcodeHRI returns the human-readable digit string for the EAN/UPC family,
// grouped the conventional way (EAN-8 as 4+4, EAN-13 as 1+6+6, UPC-A as
// 1+5+5+1). It is derived from the barcode's canonical content, which includes
// any auto-computed check digit, so it matches what a scanner reads. For other
// symbologies — or when the content doesn't match an expected length — it
// returns data unchanged.
func BarcodeHRI(format, data string) string {
	f := normalizeFormat(format)
	switch f {
	case "ean8", "ean-8", "ean13", "ean-13", "ean", "upca", "upc-a", "upc":
		bc, err := encodeBarcode(f, data)
		if err != nil {
			return data
		}
		return groupEANUPC(f, bc.Content())
	default:
		return data
	}
}

// groupEANUPC inserts the conventional spaces into an EAN/UPC content string.
func groupEANUPC(format, content string) string {
	switch format {
	case "ean8", "ean-8":
		if len(content) == 8 {
			return content[:4] + " " + content[4:]
		}
	case "ean13", "ean-13", "ean":
		if len(content) == 13 {
			return content[:1] + " " + content[1:7] + " " + content[7:]
		}
	case "upca", "upc-a", "upc":
		// UPC-A is encoded as a 13-digit EAN-13 with a leading zero; strip it
		// back to 12 digits and group as 1+5+5+1.
		u := content
		if len(u) == 13 && u[0] == '0' {
			u = u[1:]
		}
		if len(u) == 12 {
			return u[:1] + " " + u[1:6] + " " + u[6:11] + " " + u[11:]
		}
	}
	return content
}

// normalizeFormat lower-cases and trims a format name for matching.
func normalizeFormat(format string) string {
	return strings.ToLower(strings.TrimSpace(format))
}

// barcode2D reports whether a format is a 2D (stacked/matrix) symbology, which
// is scaled differently from the 1D codes.
func barcode2D(format string) bool {
	switch normalizeFormat(format) {
	case "datamatrix", "dm", "pdf417", "pdf-417":
		return true
	}
	return false
}

// encodeBarcode dispatches to the boombuler encoder for the named symbology.
// An empty format defaults to CODE128, the most general-purpose 1D code.
func encodeBarcode(format, data string) (barcode.Barcode, error) {
	switch normalizeFormat(format) {
	case "", "code128", "128":
		return code128.Encode(data)
	case "gs1-128", "gs1128", "ucc128", "ean128":
		// GS1-128 (a.k.a. UCC/EAN-128) is Code 128 with a leading FNC1 that
		// flags the data as GS1 element strings.
		return code128.Encode(string(code128.FNC1) + data)
	case "code39", "39":
		// includeChecksum=false, fullASCII=true — accept the full ASCII range.
		return code39.Encode(data, false, true)
	case "code93", "93":
		return code93.Encode(data, false, true)
	case "ean8", "ean-8":
		return ean.Encode(data)
	case "ean13", "ean-13", "ean":
		return ean.Encode(data)
	case "upca", "upc-a", "upc":
		return encodeUPCA(data)
	case "itf", "i2of5", "interleaved2of5", "2of5", "25":
		// Interleaved 2 of 5 (digits only, even count); no check digit is added,
		// so a scanner reads back exactly the digits encoded.
		return twooffive.Encode(data, true)
	case "itf14", "itf-14":
		return encodeITF14(data)
	case "standard2of5", "2of5std", "s2of5", "code25", "industrial2of5":
		// Standard (non-interleaved) 2 of 5.
		return twooffive.Encode(data, false)
	case "codabar":
		return codabar.Encode(data)
	case "datamatrix", "dm":
		return datamatrix.Encode(data)
	case "pdf417", "pdf-417":
		// securityLevel 2 — a moderate error-correction level.
		return pdf417.Encode(data, 2)
	default:
		return nil, fmt.Errorf("unsupported barcode format %q (supported: %s)", format, strings.Join(BarcodeFormats, ", "))
	}
}

// encodeUPCA encodes an 11- or 12-digit UPC-A. UPC-A is the 13-digit EAN-13
// with a leading zero, so we prepend the zero and let the EAN encoder validate
// or compute the check digit. An 11-digit input is treated as the data digits
// (check digit computed); a 12-digit input is treated as a complete UPC-A.
func encodeUPCA(data string) (barcode.Barcode, error) {
	d := strings.TrimSpace(data)
	if !allDigits(d) || (len(d) != 11 && len(d) != 12) {
		return nil, fmt.Errorf("upca requires 11 or 12 digits, got %q", data)
	}
	return ean.Encode("0" + d)
}

// encodeITF14 encodes a 14-digit GTIN-14 as Interleaved 2 of 5 (ITF-14). The
// digit count must be even for interleaving; 14 satisfies that. A bearer bar is
// not drawn (it is optional for in-house receipt use).
func encodeITF14(data string) (barcode.Barcode, error) {
	d := strings.TrimSpace(data)
	if !allDigits(d) || len(d) != 14 {
		return nil, fmt.Errorf("itf14 requires exactly 14 digits, got %q", data)
	}
	return twooffive.Encode(d, true)
}

// allDigits reports whether s is non-empty and contains only ASCII digits.
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
