package service

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

// makeTestJPEG creates a solid-color JPEG at the given dimensions.
func makeTestJPEG(t *testing.T, w, h int) *bytes.Buffer {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: 255, G: 128, B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return &buf
}

// makeTestPNG creates a solid-color PNG at the given dimensions.
func makeTestPNG(t *testing.T, w, h int) *bytes.Buffer {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: 0, G: 128, B: 255, A: 200})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return &buf
}

func TestProcessImage_SmallJPEG_NoResize(t *testing.T) {
	src := makeTestJPEG(t, 800, 600)
	result, err := ProcessImage(src, "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}
	if result.ContentType != "image/jpeg" {
		t.Errorf("content type = %q, want image/jpeg", result.ContentType)
	}
	if result.Extension != ".jpg" {
		t.Errorf("extension = %q, want .jpg", result.Extension)
	}

	// Decode the output to check dimensions are preserved.
	img, _, err := image.Decode(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != 800 || bounds.Dy() != 600 {
		t.Errorf("dimensions = %dx%d, want 800x600", bounds.Dx(), bounds.Dy())
	}
}

func TestProcessImage_LargeJPEG_Resized(t *testing.T) {
	src := makeTestJPEG(t, 4000, 3000)
	origSize := src.Len()

	result, err := ProcessImage(src, "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}

	img, _, err := image.Decode(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	bounds := img.Bounds()

	// Should be resized to 1200 wide, height proportional (900).
	if bounds.Dx() != maxImageWidth {
		t.Errorf("width = %d, want %d", bounds.Dx(), maxImageWidth)
	}
	expectedH := 3000 * maxImageWidth / 4000 // 900
	if bounds.Dy() != expectedH {
		t.Errorf("height = %d, want %d", bounds.Dy(), expectedH)
	}

	// Output should be significantly smaller than a 4000x3000 JPEG.
	outputSize := result.Data.Len()
	if outputSize >= origSize {
		t.Errorf("output (%d bytes) should be smaller than input (%d bytes)", outputSize, origSize)
	}
}

func TestProcessImage_TallImage_HeightCapped(t *testing.T) {
	// Very tall image: 800 wide × 3200 tall.
	src := makeTestJPEG(t, 800, 3200)
	result, err := ProcessImage(src, "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}

	img, _, err := image.Decode(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	bounds := img.Bounds()

	// Height should be capped at 1600, width adjusted proportionally.
	if bounds.Dy() != maxImageHeight {
		t.Errorf("height = %d, want %d", bounds.Dy(), maxImageHeight)
	}
	expectedW := 800 * maxImageHeight / 3200 // 400
	if bounds.Dx() != expectedW {
		t.Errorf("width = %d, want %d", bounds.Dx(), expectedW)
	}
}

func TestProcessImage_PNG_StaysPNG(t *testing.T) {
	src := makeTestPNG(t, 500, 400)
	result, err := ProcessImage(src, "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if result.ContentType != "image/png" {
		t.Errorf("content type = %q, want image/png", result.ContentType)
	}
	if result.Extension != ".png" {
		t.Errorf("extension = %q, want .png", result.Extension)
	}
}

func TestProcessImage_WebP_BecomesJPEG(t *testing.T) {
	// WebP input should produce JPEG output.
	// We can't easily create a WebP in pure Go, so we test with a JPEG
	// but pass "image/webp" content type to verify the output logic.
	// The decode will still work since image.Decode auto-detects format.
	src := makeTestJPEG(t, 600, 400)
	result, err := ProcessImage(src, "image/webp")
	if err != nil {
		t.Fatal(err)
	}
	if result.ContentType != "image/jpeg" {
		t.Errorf("content type = %q, want image/jpeg", result.ContentType)
	}
	if result.Extension != ".jpg" {
		t.Errorf("extension = %q, want .jpg", result.Extension)
	}
}

func TestProcessImage_InvalidData_Error(t *testing.T) {
	src := bytes.NewReader([]byte("not an image"))
	_, err := ProcessImage(src, "image/jpeg")
	if err == nil {
		t.Error("expected error for invalid image data, got nil")
	}
}

// makeBombPNGHeader crafts a minimal PNG file (signature + IHDR) declaring
// dimensions w × h. image.DecodeConfig inspects only the IHDR chunk, so this
// is enough to exercise the declared-dimensions guard without allocating the
// corresponding pixel buffer.
func makeBombPNGHeader(w, h uint32) []byte {
	buf := new(bytes.Buffer)
	// PNG signature.
	buf.Write([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a})
	// IHDR payload: 4 width + 4 height + 5 metadata bytes.
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], w)
	binary.BigEndian.PutUint32(ihdr[4:8], h)
	ihdr[8] = 8  // bit depth
	ihdr[9] = 2  // color type (RGB)
	ihdr[10] = 0 // compression
	ihdr[11] = 0 // filter
	ihdr[12] = 0 // interlace
	writePNGChunk(buf, "IHDR", ihdr)
	return buf.Bytes()
}

func writePNGChunk(buf *bytes.Buffer, typ string, data []byte) {
	_ = binary.Write(buf, binary.BigEndian, uint32(len(data)))
	buf.WriteString(typ)
	buf.Write(data)
	crc := crc32.NewIEEE()
	crc.Write([]byte(typ))
	crc.Write(data)
	_ = binary.Write(buf, binary.BigEndian, crc.Sum32())
}

func TestProcessImage_RejectsDeclaredBomb(t *testing.T) {
	bomb := makeBombPNGHeader(65535, 65535) // ~4.3 billion pixels
	_, err := ProcessImage(bytes.NewReader(bomb), "image/png")
	if err == nil {
		t.Fatal("expected error for oversized dimensions")
	}
	if !strings.Contains(err.Error(), "exceed maximum pixels") {
		t.Errorf("wrong error message: %v", err)
	}
}

func TestProcessImage_RejectsOversizePayload(t *testing.T) {
	big := bytes.Repeat([]byte{0x00}, maxInputBytes+1)
	_, err := ProcessImage(bytes.NewReader(big), "image/jpeg")
	if err == nil {
		t.Fatal("expected error for oversize payload")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("wrong error message: %v", err)
	}
}

func TestProcessImage_AcceptsAtLimit(t *testing.T) {
	// A modest 1000x1000 image is well under both limits and should still process.
	src := makeTestJPEG(t, 1000, 1000)
	if _, err := ProcessImage(src, "image/jpeg"); err != nil {
		t.Fatalf("unexpected error for normal image: %v", err)
	}
}
