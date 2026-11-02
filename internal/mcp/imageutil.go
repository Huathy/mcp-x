package mcp

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

// scaleImage downsamples src so its width is at most maxWidth.
//
// It uses area averaging (a box filter over the source footprint of each
// destination pixel) rather than point sampling. Screenshots of desktop UI are
// mostly small text, and point sampling drops most of the glyph pixels, which
// makes the result unreadable — exactly when the AI most needs to read it.
// The stdlib's image/draw scalers are nearest-neighbour style, so the
// reduction is done here instead.
func scaleImage(src image.Image, maxWidth int) image.Image {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if maxWidth <= 0 || sw <= maxWidth {
		return src
	}
	dw := maxWidth
	dh := int((int64(sh)*int64(dw) + int64(sw)/2) / int64(sw))
	if dh < 1 {
		dh = 1
	}

	// Cache the source as RGBA once: desktop captures arrive as *image.RGBA,
	// but a generic path keeps this correct for any image implementation.
	srcRGBA := toRGBA(src)

	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	// Precompute per-destination-pixel source bounds and their 1-based
	// lengths so the inner loop stays integer-only.
	type span struct{ x0, y0, x1, y1 int }
	xs := make([]span, dw)
	for x := 0; x < dw; x++ {
		x0 := b.Min.X + x*sw/dw
		x1 := b.Min.X + (x+1)*sw/dw
		if x1 <= x0 {
			x1 = x0 + 1
		}
		xs[x] = span{x0, 0, x1, x1 - x0}
	}
	for y := 0; y < dh; y++ {
		y0 := b.Min.Y + y*sh/dh
		y1 := b.Min.Y + (y+1)*sh/dh
		if y1 <= y0 {
			y1 = y0 + 1
		}
		rows := y1 - y0
		for x := 0; x < dw; x++ {
			var r, g, bl, a uint64
			for sy := y0; sy < y1; sy++ {
				for sx := xs[x].x0; sx < xs[x].x1; sx++ {
					i := srcRGBA.PixOffset(sx-b.Min.X, sy-b.Min.Y)
					// Alpha is averaged, not premultiplied; the source is
					// opaque for a screen capture, and premultiplying here
					// would only add cost.
					r += uint64(srcRGBA.Pix[i+0])
					g += uint64(srcRGBA.Pix[i+1])
					bl += uint64(srcRGBA.Pix[i+2])
					a += uint64(srcRGBA.Pix[i+3])
				}
			}
			n := uint64(xs[x].x1-xs[x].x0) * uint64(rows)
			i := dst.PixOffset(x, y)
			dst.Pix[i+0] = uint8(r / n)
			dst.Pix[i+1] = uint8(g / n)
			dst.Pix[i+2] = uint8(bl / n)
			dst.Pix[i+3] = uint8(a / n)
		}
	}
	return dst
}

func toRGBA(src image.Image) *image.RGBA {
	if r, ok := src.(*image.RGBA); ok {
		return r
	}
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			cr, cg, cb, ca := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			i := dst.PixOffset(x, y)
			dst.Pix[i+0] = uint8(cr >> 8)
			dst.Pix[i+1] = uint8(cg >> 8)
			dst.Pix[i+2] = uint8(cb >> 8)
			dst.Pix[i+3] = uint8(ca >> 8)
		}
	}
	return dst
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}

func encodeTo(img image.Image, path string) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create dir: %w", err)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 88}); err != nil {
			return fmt.Errorf("encode jpeg: %w", err)
		}
	default:
		if err := png.Encode(f, img); err != nil {
			return fmt.Errorf("encode png: %w", err)
		}
	}
	return nil
}
