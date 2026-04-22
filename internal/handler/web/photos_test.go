package webhandler

import (
	"testing"
)

// makeFtypHeader builds a 12-byte slice that looks like the start of an
// ISOBMFF file: a 4-byte big-endian box size, the literal "ftyp", then a
// 4-byte major brand. The sniffer only reads the first 12 bytes, so this
// is enough to exercise it.
func makeFtypHeader(brand string) []byte {
	if len(brand) != 4 {
		panic("brand must be 4 bytes")
	}
	out := make([]byte, 12)
	// Box size (arbitrary; sniffer ignores it).
	out[0], out[1], out[2], out[3] = 0x00, 0x00, 0x00, 0x20
	copy(out[4:8], "ftyp")
	copy(out[8:12], brand)
	return out
}

func TestIsHEIC(t *testing.T) {
	tests := []struct {
		name   string
		input  []byte
		expect bool
	}{
		// HEIC brands — iPhone still ships "heic" and "heix" most often;
		// live-photo burst variants use "hevc"/"hevx".
		{"iPhone HEIC (heic)", makeFtypHeader("heic"), true},
		{"iPhone HEIC (heix)", makeFtypHeader("heix"), true},
		{"HEVC live photo (hevc)", makeFtypHeader("hevc"), true},
		{"HEVC multi-frame (hevx)", makeFtypHeader("hevx"), true},
		{"HEIC image mosaic (heim)", makeFtypHeader("heim"), true},
		{"HEIC image sequence (heis)", makeFtypHeader("heis"), true},
		{"HEVC mosaic (hevm)", makeFtypHeader("hevm"), true},
		{"HEVC sequence (hevs)", makeFtypHeader("hevs"), true},

		// Generic HEIF brands.
		{"HEIF image (mif1)", makeFtypHeader("mif1"), true},
		{"HEIF sequence (msf1)", makeFtypHeader("msf1"), true},

		// Non-HEIC/HEIF ISOBMFF brands — MP4, 3GP, etc. The sniffer must
		// return false so we don't misdiagnose these as HEIC and show the
		// user an irrelevant "browser didn't convert" message.
		{"MP4 (isom)", makeFtypHeader("isom"), false},
		{"MP4 (mp42)", makeFtypHeader("mp42"), false},
		{"3GPP (3gp4)", makeFtypHeader("3gp4"), false},
		{"QuickTime (qt  )", makeFtypHeader("qt  "), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isHEIC(tc.input); got != tc.expect {
				t.Errorf("isHEIC(%q brand) = %v, want %v",
					string(tc.input[8:12]), got, tc.expect)
			}
		})
	}
}

func TestIsHEIC_ShortInput(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{"nil", nil},
		{"empty", []byte{}},
		{"11 bytes (one short of header)", make([]byte, 11)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isHEIC(tc.input); got {
				t.Errorf("isHEIC(short) = true, want false")
			}
		})
	}
}

func TestIsHEIC_NoFtypBox(t *testing.T) {
	// Valid length but the magic isn't "ftyp" — a JPEG starts with FF D8,
	// not a box structure. Must return false rather than misreading bytes
	// 8–12 as a brand.
	jpegLike := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01}
	if isHEIC(jpegLike) {
		t.Errorf("isHEIC(jpeg-like) = true, want false")
	}

	// A PNG header — no "ftyp" box.
	pngLike := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 'I', 'H', 'D', 'R'}
	if isHEIC(pngLike) {
		t.Errorf("isHEIC(png-like) = true, want false")
	}
}

func TestAllowedImageTypes(t *testing.T) {
	// The allowlist deliberately excludes HEIC — the server has no pure-Go
	// way to decode it, so the browser's native image pipeline (createImage
	// Bitmap + canvas in app.js) is responsible for turning HEIC into JPEG
	// before upload. This test guards against accidentally re-adding HEIC
	// to the allowlist, which would waive the pre-decode check and let
	// goheif-less ProcessImage fail with an unhelpful "decode image" error.
	required := []string{"image/jpeg", "image/png", "image/webp"}
	for _, ct := range required {
		if !allowedImageTypes[ct] {
			t.Errorf("allowedImageTypes is missing %q", ct)
		}
	}

	forbidden := []string{
		"image/heic", "image/heif", // handled client-side only
		"image/gif", "image/svg+xml", "application/pdf", "text/html", "",
	}
	for _, ct := range forbidden {
		if allowedImageTypes[ct] {
			t.Errorf("allowedImageTypes accepts forbidden type %q", ct)
		}
	}
}
