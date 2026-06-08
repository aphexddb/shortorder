package escpos

import (
	"image"

	"golang.org/x/image/draw"
)

// rasterImage is a 1-bit packed bitmap ready for GS v 0.
type rasterImage struct {
	widthBytes int    // bytes per row = ceil(width/8)
	height     int    // rows
	data       []byte // widthBytes*height bytes, MSB-first, 1 = black
}

// FitWidth scales img down to fit within maxWidth dots, preserving aspect
// ratio. Images already narrower than maxWidth are returned unchanged. This
// keeps wide source images within the print head's dot width (e.g. 576 for an
// 80mm printer) instead of being clipped.
func FitWidth(img image.Image, maxWidth int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxWidth || maxWidth <= 0 {
		return img
	}
	nh := h * maxWidth / w
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, maxWidth, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst
}

// pack converts img to a 1-bit raster using Floyd–Steinberg error-diffusion
// dithering. Compared to a hard threshold this renders grayscale and
// anti-aliased text faithfully on a 1-bit head: solid blacks stay solid, light
// grays become a sparse stipple instead of dropping out (faint) or filling in
// (dense). Content that is already pure black/white (e.g. a QR raster) has zero
// quantization error to diffuse, so it stays crisp. Transparent pixels are
// treated as white.
func pack(img image.Image) *rasterImage {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil
	}
	widthBytes := (w + 7) / 8
	data := make([]byte, widthBytes*h)

	// Grayscale working buffer in 0..255 (float for accumulated error).
	gray := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			if a == 0 {
				gray[y*w+x] = 255 // transparent -> white
				continue
			}
			// Rec. 601 luma; channels are 16-bit, scale to 0..255.
			luma := (299*float64(r) + 587*float64(g) + 114*float64(bl)) / 1000 / 257
			gray[y*w+x] = luma
		}
	}

	diffuse := func(x, y int, err, factor float64) {
		if x < 0 || x >= w || y < 0 || y >= h {
			return
		}
		gray[y*w+x] += err * factor / 16
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			old := gray[y*w+x]
			var newv float64
			if old >= 128 {
				newv = 255 // white
			} else {
				newv = 0 // black
				data[y*widthBytes+x/8] |= 0x80 >> uint(x%8)
			}
			err := old - newv
			diffuse(x+1, y, err, 7)
			diffuse(x-1, y+1, err, 3)
			diffuse(x, y+1, err, 5)
			diffuse(x+1, y+1, err, 1)
		}
	}
	return &rasterImage{widthBytes: widthBytes, height: h, data: data}
}
