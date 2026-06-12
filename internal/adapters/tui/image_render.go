package tui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strings"

	"charm.land/lipgloss/v2"
)

// decodeImageFromPath reads + decodes an image file. The decoders for PNG,
// JPEG, and GIF are registered via blank imports above so image.Decode can
// dispatch by sniffing the file header — no extension parsing needed.
//
// Returns a friendly error string when the format isn't supported (e.g. SVG,
// WebP, AVIF) so the viewer can render a "preview not available" panel
// instead of dumping a stack trace.
func decodeImageFromPath(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return img, nil
}

// fitDimensions computes the output image size that fits within (maxW pixels,
// maxH pixels) while preserving the source aspect ratio. Both dimensions are
// floored at 1 so callers don't accidentally scale to zero.
//
// Half-block rendering packs 2 vertical pixels per terminal row, so the
// caller passes maxH = (innerH * 2) and reads out the result height likewise.
func fitDimensions(srcW, srcH, maxW, maxH int) (int, int) {
	if srcW <= 0 || srcH <= 0 {
		return 1, 1
	}
	if maxW <= 0 {
		maxW = 1
	}
	if maxH <= 0 {
		maxH = 1
	}
	rw := float64(maxW) / float64(srcW)
	rh := float64(maxH) / float64(srcH)
	r := rw
	if rh < r {
		r = rh
	}
	// Only scale down. An already-small image stays at its native size —
	// upscaling with nearest-neighbor produces blocky garbage and the
	// terminal cell-size limit is what we're trying to respect, not a
	// minimum-display-size mandate.
	if r > 1 {
		r = 1
	}
	w := int(float64(srcW) * r)
	h := int(float64(srcH) * r)
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return w, h
}

// scaleNearest does a nearest-neighbor resize from src to dstW × dstH. The
// result is a paletted-free RGBA so renderHalfBlock can read pixels without
// caring about the source color model.
//
// Nearest-neighbor produces blocky output but is fast and dependency-free.
// We can swap in catmull-rom from x/image/draw later without changing the
// public API — callers just see a smaller image.Image.
func scaleNearest(src image.Image, dstW, dstH int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	srcB := src.Bounds()
	srcW := srcB.Dx()
	srcH := srcB.Dy()
	if srcW <= 0 || srcH <= 0 {
		return dst
	}
	for y := range dstH {
		sy := y * srcH / dstH
		for x := range dstW {
			sx := x * srcW / dstW
			dst.Set(x, y, src.At(srcB.Min.X+sx, srcB.Min.Y+sy))
		}
	}
	return dst
}

// renderHalfBlock turns an RGBA image into an ANSI-colored string where each
// terminal cell encodes two stacked pixels via the upper-half-block glyph:
//
//	"▀" with fg = top pixel, bg = bottom pixel.
//
// The output has ceil(H/2) rows and W columns. When H is odd, the final row
// pairs the last pixel row against transparent (rendered as just the fg —
// the bg-less top half). Returns the string ready for lipgloss composition;
// lipgloss measures the half-block as width-1, so border math stays correct.
func renderHalfBlock(img *image.RGBA) string {
	b := img.Bounds()
	w := b.Dx()
	h := b.Dy()
	if w <= 0 || h <= 0 {
		return ""
	}
	var sb strings.Builder
	for y := 0; y < h; y += 2 {
		for x := range w {
			top := img.RGBAAt(b.Min.X+x, b.Min.Y+y)
			var line string
			if y+1 < h {
				bot := img.RGBAAt(b.Min.X+x, b.Min.Y+y+1)
				line = lipgloss.NewStyle().
					Foreground(rgbColor(top)).
					Background(rgbColor(bot)).
					Render("▀")
			} else {
				line = lipgloss.NewStyle().
					Foreground(rgbColor(top)).
					Render("▀")
			}
			sb.WriteString(line)
		}
		if y+2 < h {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// rgbColor builds a lipgloss color from an RGBA sample, ignoring alpha. We
// drop alpha because terminals don't composite — a half-transparent pixel
// would render against whatever lipgloss thinks the panel background is,
// which is unpredictable. Treat alpha=0 as black so transparent regions are
// at least consistent.
func rgbColor(c color.RGBA) color.Color {
	if c.A == 0 {
		return lipgloss.Color("#000000")
	}
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B))
}

// renderImagePreview is the full pipeline: decode → fit → scale → halfblock.
// Used by the viewer when an image file is opened. Returns a preview string
// sized to fit (maxCellW × maxCellRows) terminal cells, plus a metadata line
// describing the source image (dimensions, format-aware byte size).
//
// On decode failure returns ("", err) so the viewer can render an error panel.
func renderImagePreview(data []byte, maxCellW, maxCellRows int) (string, image.Point, error) {
	img, err := decodeImageFromPath(data)
	if err != nil {
		return "", image.Point{}, err
	}
	src := img.Bounds()
	// Each terminal row carries 2 vertical pixels; each column = 1 pixel.
	dstW, dstH := fitDimensions(src.Dx(), src.Dy(), maxCellW, maxCellRows*2)
	scaled := scaleNearest(img, dstW, dstH)
	out := renderHalfBlock(scaled)
	return out, image.Point{X: src.Dx(), Y: src.Dy()}, nil
}
