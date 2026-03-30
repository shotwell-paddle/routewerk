package service

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
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
