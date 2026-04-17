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

	// maxInputBytes bounds the encoded payload before any decode work happens.
	// Must match or exceed the handler's multipart cap (currently 5 MB).
	maxInputBytes = 5 * 1024 * 1024

	// maxInputPixels bounds declared image dimensions to prevent
	// decompression-bomb DoS. 40 megapixels accommodates modern phone cameras
	// (e.g. iPhone 48 MP Pro mode downsamples to < 40 MP JPEG by default) while
	// rejecting pathological 65535x65535 declarations that would allocate TBs.
	maxInputPixels = 40 * 1000 * 1000
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
//
// The function defends against decompression-bomb DoS by:
//   1. Buffering at most maxInputBytes+1 from src, rejecting anything larger.
//   2. Peeking at the image header via image.DecodeConfig (no pixel allocation)
//      and rejecting declarations larger than maxInputPixels.
//   3. Only then calling the full decoder.
func ProcessImage(src io.Reader, contentType string) (*ProcessedImage, error) {
	raw, err := io.ReadAll(io.LimitReader(src, maxInputBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}
	if len(raw) > maxInputBytes {
		return nil, fmt.Errorf("image exceeds %d bytes", maxInputBytes)
	}

	// Peek at declared dimensions before allocating pixel buffers.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decode image config: %w", err)
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxInputPixels {
		return nil, fmt.Errorf("image dimensions %dx%d exceed maximum pixels", cfg.Width, cfg.Height)
	}

	img, _, err := image.Decode(bytes.NewReader(raw))
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
