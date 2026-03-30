package service

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"

	// Register WebP decoder so image.Decode can handle it.
	_ "golang.org/x/image/webp"

	"golang.org/x/image/draw"
)

const (
	// maxImageWidth is the maximum width for uploaded photos.
	// 1200px is a good balance: sharp on high-DPI mobile screens, small file size.
	maxImageWidth = 1200

	// maxImageHeight prevents absurdly tall panoramic images.
	maxImageHeight = 1600

	// jpegQuality controls compression. 82 gives excellent visual quality
	// at roughly 60-70% size reduction from default.
	jpegQuality = 82
)

// ProcessedImage is the result of resizing and compressing an uploaded image.
type ProcessedImage struct {
	Data        *bytes.Reader
	ContentType string
	Extension   string
}

// ProcessImage decodes an uploaded image, resizes it if larger than the max
// dimensions, and re-encodes as JPEG (for JPEG/WebP inputs) or PNG (for PNG
// inputs with transparency). All outputs are optimized for web delivery.
func ProcessImage(src io.Reader, contentType string) (*ProcessedImage, error) {
	img, _, err := image.Decode(src)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	bounds := img.Bounds()
	origW := bounds.Dx()
	origH := bounds.Dy()

	// Calculate new dimensions, preserving aspect ratio.
	newW, newH := origW, origH
	if newW > maxImageWidth {
		newH = newH * maxImageWidth / newW
		newW = maxImageWidth
	}
	if newH > maxImageHeight {
		newW = newW * maxImageHeight / newH
		newH = maxImageHeight
	}

	// Resize if needed using high-quality CatmullRom interpolation.
	if newW != origW || newH != origH {
		dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)
		img = dst
	}

	// Encode. We convert everything to JPEG unless the input was PNG
	// (which may have transparency the user cares about).
	var buf bytes.Buffer
	outType := "image/jpeg"
	outExt := ".jpg"

	if contentType == "image/png" {
		enc := &png.Encoder{CompressionLevel: png.BestCompression}
		if err := enc.Encode(&buf, img); err != nil {
			return nil, fmt.Errorf("encode png: %w", err)
		}
		outType = "image/png"
		outExt = ".png"
	} else {
		// JPEG and WebP inputs both become JPEG output.
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
			return nil, fmt.Errorf("encode jpeg: %w", err)
		}
	}

	return &ProcessedImage{
		Data:        bytes.NewReader(buf.Bytes()),
		ContentType: outType,
		Extension:   outExt,
	}, nil
}
