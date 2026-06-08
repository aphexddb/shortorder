package escpos

import (
	"image"

	qrcode "github.com/skip2/go-qrcode"
)

// QRImage renders data as a QR code image. scale is the module (pixel) size;
// the resulting image side is roughly (modules * scale). A scale of 6–10 prints
// cleanly on an 80mm head. recovery selects error-correction redundancy.
//
// Rendering the QR to a raster (rather than using the printer's native GS ( k
// QR commands) keeps output identical across the many ESC/POS clones, some of
// which implement the native QR commands incompletely.
func QRImage(data string, scale int, recovery qrcode.RecoveryLevel) (image.Image, error) {
	if scale < 1 {
		scale = 8
	}
	q, err := qrcode.New(data, recovery)
	if err != nil {
		return nil, err
	}
	q.DisableBorder = false
	// Image(size) renders a square of the given pixel side; derive it from the
	// module count so each module is `scale` pixels.
	modules := len(q.Bitmap())
	side := modules * scale
	return q.Image(side), nil
}
