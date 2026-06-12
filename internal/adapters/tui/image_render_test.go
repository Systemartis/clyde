package tui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

// makePNG encodes a tiny image with the given pixels (column-major) into PNG
// bytes — keeps test fixtures self-contained.
func makePNG(t *testing.T, w, h int, pixels []color.RGBA) []byte {
	t.Helper()
	if len(pixels) != w*h {
		t.Fatalf("pixel count mismatch: w=%d h=%d len=%d", w, h, len(pixels))
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i, px := range pixels {
		x := i % w
		y := i / w
		img.Set(x, y, px)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

// TestDecodeImageFromPath_PNG verifies a stdlib-encoded PNG round-trips
// through decodeImageFromPath without errors and returns the right bounds.
func TestDecodeImageFromPath_PNG(t *testing.T) {
	t.Parallel()
	red := color.RGBA{R: 255, A: 255}
	data := makePNG(t, 2, 2, []color.RGBA{red, red, red, red})

	img, err := decodeImageFromPath(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := img.Bounds().Dx(); got != 2 {
		t.Errorf("width = %d, want 2", got)
	}
	if got := img.Bounds().Dy(); got != 2 {
		t.Errorf("height = %d, want 2", got)
	}
}

// TestDecodeImageFromPath_BadInput verifies non-image bytes return an error,
// not a panic — the viewer's "preview not available" path depends on this.
func TestDecodeImageFromPath_BadInput(t *testing.T) {
	t.Parallel()
	_, err := decodeImageFromPath([]byte("not an image"))
	if err == nil {
		t.Error("expected error for non-image bytes")
	}
}

// TestFitDimensions verifies aspect-preserving scaling for a few common cases.
func TestFitDimensions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                   string
		srcW, srcH, maxW, maxH int
		wantW, wantH           int
	}{
		// Wide image fits to width.
		{"wide-fits-width", 200, 100, 100, 100, 100, 50},
		// Tall image fits to height.
		{"tall-fits-height", 100, 200, 100, 100, 50, 100},
		// Already-smaller stays the same when the cap is generous.
		{"smaller-fits", 50, 25, 100, 100, 50, 25},
		// Square fills square.
		{"square", 100, 100, 60, 60, 60, 60},
		// Degenerate input should not return zero.
		{"zero-src", 0, 0, 100, 100, 1, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotW, gotH := fitDimensions(tc.srcW, tc.srcH, tc.maxW, tc.maxH)
			if gotW != tc.wantW || gotH != tc.wantH {
				t.Errorf("got (%d, %d), want (%d, %d)", gotW, gotH, tc.wantW, tc.wantH)
			}
		})
	}
}

// TestScaleNearest_Bounds verifies the scaler returns an image with the exact
// requested dimensions, regardless of source size.
func TestScaleNearest_Bounds(t *testing.T) {
	t.Parallel()
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	dst := scaleNearest(src, 8, 16)
	if got := dst.Bounds().Dx(); got != 8 {
		t.Errorf("width = %d, want 8", got)
	}
	if got := dst.Bounds().Dy(); got != 16 {
		t.Errorf("height = %d, want 16", got)
	}
}

// TestRenderHalfBlock_Dimensions verifies the rendered string has the right
// number of rows (ceil(H/2)) and that each row contains exactly W half-block
// glyphs. Strips ANSI codes before counting.
func TestRenderHalfBlock_Dimensions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		w, h     int
		wantRows int
	}{
		{"4x4-square", 4, 4, 2},
		{"4x5-odd-h", 4, 5, 3},
		{"1x1-tiny", 1, 1, 1},
		{"6x2-wide", 6, 2, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			img := image.NewRGBA(image.Rect(0, 0, tc.w, tc.h))
			// Fill with red so the renderer hits the styled-cell path.
			for y := range tc.h {
				for x := range tc.w {
					img.Set(x, y, color.RGBA{R: 200, A: 255})
				}
			}
			out := renderHalfBlock(img)
			rows := strings.Split(out, "\n")
			if len(rows) != tc.wantRows {
				t.Fatalf("rows = %d, want %d (out=%q)", len(rows), tc.wantRows, out)
			}
			for i, row := range rows {
				if got := strings.Count(row, "▀"); got != tc.w {
					t.Errorf("row[%d] glyph count = %d, want %d", i, got, tc.w)
				}
			}
		})
	}
}

// TestRenderImagePreview_EndToEnd verifies the full decode → fit → scale →
// render pipeline produces a non-empty string with the expected cell-grid
// dimensions for a known input. The original bounds are returned for the
// viewer header, so we check that too.
func TestRenderImagePreview_EndToEnd(t *testing.T) {
	t.Parallel()
	// 4×4 PNG, all blue.
	blue := color.RGBA{B: 200, A: 255}
	pixels := make([]color.RGBA, 16)
	for i := range pixels {
		pixels[i] = blue
	}
	data := makePNG(t, 4, 4, pixels)

	out, srcSize, err := renderImagePreview(data, 8, 4) // 8 cols × 4 rows = 8 px wide × 8 px tall budget
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if out == "" {
		t.Fatal("expected non-empty preview")
	}
	if !strings.Contains(out, "▀") {
		t.Errorf("expected half-block glyph in output, got %q", out[:min(60, len(out))])
	}
	if srcSize.X != 4 || srcSize.Y != 4 {
		t.Errorf("srcSize = %v, want (4, 4)", srcSize)
	}
}

// TestRenderImagePreview_DistinctImagesProduceDistinctOutput is the regression
// test for the "every image looks the same" bug: the old viewer rendered a
// procedural ASCII pattern based only on grid coordinates, ignoring the actual
// pixels. Two images with very different content must render differently.
func TestRenderImagePreview_DistinctImagesProduceDistinctOutput(t *testing.T) {
	t.Parallel()
	red := color.RGBA{R: 255, A: 255}
	green := color.RGBA{G: 255, A: 255}

	redData := makePNG(t, 2, 2, []color.RGBA{red, red, red, red})
	greenData := makePNG(t, 2, 2, []color.RGBA{green, green, green, green})

	redOut, _, err := renderImagePreview(redData, 4, 2)
	if err != nil {
		t.Fatalf("render red: %v", err)
	}
	greenOut, _, err := renderImagePreview(greenData, 4, 2)
	if err != nil {
		t.Fatalf("render green: %v", err)
	}
	if redOut == greenOut {
		t.Error("red and green images rendered identical output — pixels are being ignored")
	}
}
