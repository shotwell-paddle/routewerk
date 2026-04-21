package cardsheet

// card_draw.go renders a single card face directly into a gofpdf page as
// VECTOR content — text through embedded fonts, color zones as filled
// rectangles, QR code as the only raster element. This is the path that
// matters for print quality: rasterizing cards to PNG @300 DPI and embedding
// as images (the previous approach) baked in hinted glyph shapes that went
// rough at any DPI mismatch between the renderer and the printer. Native PDF
// text stays sharp all the way to the platen.
//
// Cards are *designed* in portrait (2" × 3.5") because that's how a climber
// naturally holds a trading-card-style route card. On the letter sheet each
// card occupies a landscape 88.9 × 50.8 mm slot, and drawCardVector applies
// a 90° rotation so the portrait design lines up with that slot. The cutter
// follows a landscape rectangle; after cutting, the user rotates the card in
// hand and reads it in portrait — matching the sheet-face PNG used in the
// web preview (see service.GenerateSheetCardPNG).
//
//   ┌──────────────────┐                    ┌───────────────┬──────────────────────────┐
//   │                  │                    │               │                          │
//   │       V5         │      90° CCW       │    Crimson    │                          │
//   │       RED        │  ───────────────▶  │    Crush      │ ··· (sheet slot) ···     │
//   │                  │                    │               │                          │
//   │  Crimson Crush   │                    │ ... metadata  │                          │
//   │                  │                    │               │                          │
//   │  WALL    SETTER  │                    │   V5 rotated  │                          │
//   │  The Cave  Chris │                    │   ↓           │                          │
//   │                  │                    │               │                          │
//   │  SET       [QR]  │                    │               │                          │
//   │  Mar 15          │                    │               │                          │
//   └──────────────────┘                    └───────────────┴──────────────────────────┘
//       portrait                                           landscape sheet slot
//       (50.8 × 88.9)                                      (88.9 × 50.8)

import (
	"fmt"
	"strings"

	"github.com/jung-kurt/gofpdf/v2"
	qrcode "github.com/skip2/go-qrcode"

	"github.com/shotwell-paddle/routewerk/internal/service"
)

// cardFontFamily is the gofpdf family name for the embedded Routewerk body
// font. Must match the string passed to AddUTF8FontFromBytes.
const cardFontFamily = "routewerk"

// Portrait card layout constants — all in mm.
//
// The card is designed as a trading-card "split" layout: a color zone
// occupies the top ~55% of the card, the info zone occupies the remaining
// ~45%. Coordinates inside drawCardPortrait are expressed relative to the
// portrait top-left (ox, oy); drawCardVector rotates this whole drawing to
// land inside the landscape sheet slot.
const (
	portraitW = 50.8 // 2.0" — card short axis
	portraitH = 88.9 // 3.5" — card long axis

	colorZoneH      = 49.0 // top color zone height (≈ 55% of 88.9)
	infoPadX        = 3.0  // left padding inside info zone
	infoPadTop      = 5.0  // gap between color split and first name baseline
	infoBottomPad   = 1.8  // distance from card bottom to the ROUTEWERK footer baseline

	qrSizeMM       = 14.0
	qrMarginRight  = 2.5
	qrMarginBottom = 3.0
)

// drawCardVector places the portrait card design into a landscape slot at
// (x, y) with dimensions (w, h) = (88.9, 50.8) on the sheet. The card is
// drawn by drawCardPortrait in portrait coordinates; this wrapper applies a
// 90° CCW rotation (via gofpdf's transform stack) so the color zone — which
// sits on TOP of the portrait design — ends up on the LEFT of the landscape
// slot, matching the visual orientation the old raster path produced.
//
// uniqueKey must be unique within the whole PDF — it names the per-card QR
// image registration.
//
// Transform math (portrait corners → landscape slot):
//
//	portrait TL (0, 0)         → slot bottom-left  (x,        y + h)
//	portrait TR (portraitW, 0) → slot top-left     (x,        y)
//	portrait BL (0, portraitH) → slot bottom-right (x + w,    y + h)
//	portrait BR (portraitW, H) → slot top-right    (x + w,    y)
//
// gofpdf's TransformRotate measures angle counter-clockwise from the 3
// o'clock position in PDF-native (Y-up) space. Because gofpdf flips Y to
// present a top-left-origin user space, +90° in gofpdf reads as a CCW
// rotation *visually* on the page — east maps to north. Pairing that with a
// translate to the slot's bottom-left (x, y+h) produces the mapping above:
// the transforms compose such that user-space points are first rotated and
// then translated.
func drawCardVector(pdf *gofpdf.Fpdf, data service.CardData, x, y, w, h float64, uniqueKey string) {
	pdf.TransformBegin()
	pdf.TransformTranslate(x, y+h)
	pdf.TransformRotate(90, 0, 0)
	drawCardPortrait(pdf, data, 0, 0, uniqueKey)
	pdf.TransformEnd()
}

// drawCardPortrait renders the portrait design at local top-left (ox, oy)
// inside the current (possibly transformed) drawing space. Dimensions are
// fixed at portraitW × portraitH — caller-supplied (ox, oy) only translate
// the drawing, they don't resize it.
func drawCardPortrait(pdf *gofpdf.Fpdf, data service.CardData, ox, oy float64, uniqueKey string) {
	routeR, routeG, routeB := hexToRGB(data.Route.Color)
	onR, onG, onB := contrastRGB(routeR, routeG, routeB)

	// ---- Color zone (top) ----
	pdf.SetFillColor(routeR, routeG, routeB)
	pdf.Rect(ox, oy, portraitW, colorZoneH, "F")

	// Primary identifier: grade for graded routes, color name for circuits.
	// Auto-shrink until it fits the card width with a small gutter.
	identifier := data.Route.Grade
	startPt := 34.0
	if data.IsCircuit() {
		identifier = strings.ToUpper(data.ColorLabel())
		// Color names run wider than V-grades; start smaller so the fit
		// loop has somewhere to go.
		startPt = 24.0
	}
	pdf.SetTextColor(onR, onG, onB)
	ptSize := fitFontSize(pdf, identifier, portraitW-6, startPt, 10)
	pdf.SetFont(cardFontFamily, "B", ptSize)
	idW := pdf.GetStringWidth(identifier)
	// Vertical center within color zone. pt→mm ≈ 0.3528; cap height ≈ 0.72×em.
	idCapMM := ptSize * 25.4 / 72.0 * 0.72
	idBaseline := oy + colorZoneH*0.52 + idCapMM/2
	pdf.Text(ox+(portraitW-idW)/2, idBaseline, identifier)

	// Color subtitle — only for graded routes, where the color name adds
	// information beyond the grade number.
	if !data.IsCircuit() {
		label := strings.ToUpper(data.ColorLabel())
		pdf.SetFont(cardFontFamily, "B", 8)
		// Slightly dim the on-color ink so the subtitle recedes behind the
		// grade visually.
		pdf.SetTextColor(dim(onR, 210), dim(onG, 210), dim(onB, 210))
		lw := pdf.GetStringWidth(label)
		pdf.Text(ox+(portraitW-lw)/2, idBaseline+8, label)
	}

	// ---- Info zone (bottom) ----
	infoTop := oy + colorZoneH
	infoX := ox + infoPadX
	infoW := portraitW - 2*infoPadX

	// Route name — bold, wrapped to 2 lines max.
	namePt := 11.0
	nameLine := 4.4
	pdf.SetFont(cardFontFamily, "B", namePt)
	pdf.SetTextColor(25, 25, 25)
	currentY := infoTop + infoPadTop
	if data.Route.Name != nil && *data.Route.Name != "" {
		lines := wrapTextPDF(pdf, *data.Route.Name, infoW, 2)
		for _, line := range lines {
			pdf.Text(infoX, currentY, line)
			currentY += nameLine
		}
	}
	// Pin the metadata grid to a consistent Y regardless of name line count,
	// so one-liner names don't cause the whole block to float up.
	minMetaY := infoTop + infoPadTop + 2*nameLine + 1.2
	if currentY < minMetaY {
		currentY = minMetaY
	} else {
		currentY += 1.0
	}

	// Metadata grid — 2 columns. Left column gets wall + date, right column
	// gets setter. Wall names are more variable in length than setter names
	// (gyms often use long descriptors like "Competition Wall South" while
	// setters go by first name), so we give WALL the larger slot. A 1.5mm
	// gutter prevents "fully-truncated" wall values from colliding with the
	// SETTER column — that was the overlap that "broke the design" on long
	// wall names.
	//
	//   |── col1W ──|·gutter·|── col2W ──|
	//   col1X                 col2X
	//
	// QR occupies the bottom-right ~14mm so the right column must not extend
	// below ~oy + portraitH - qrSizeMM.
	const (
		colGutter = 1.5
	)
	col1X := infoX
	col1W := portraitW*0.48 - infoPadX // ≈ 21.4mm, enough for "Competition Wall" at 9pt
	col2X := col1X + col1W + colGutter // ≈ 25.9mm
	col2W := portraitW - infoPadX - (col2X - ox)

	// Row 1: labels
	pdf.SetFont(cardFontFamily, "B", 5.5)
	pdf.SetTextColor(150, 150, 150)
	pdf.Text(col1X, currentY, "WALL")
	pdf.Text(col2X, currentY, "SETTER")
	currentY += 3.6

	// Row 1: values. Auto-shrink the font before giving up and truncating —
	// a wall name like "Main Overhang Boulder" reads better at 7.5pt than at
	// 9pt+ellipsis. Floor at 7pt; truncate only if even 7pt still overflows.
	pdf.SetTextColor(40, 40, 40)
	wallPt := fitFontSizeStyle(pdf, data.WallName, col1W, 9.0, 7.0, "")
	pdf.SetFont(cardFontFamily, "", wallPt)
	pdf.Text(col1X, currentY, truncatePDF(pdf, data.WallName, col1W))
	if data.SetterName != "" {
		setterPt := fitFontSizeStyle(pdf, data.SetterName, col2W, 9.0, 7.0, "")
		pdf.SetFont(cardFontFamily, "", setterPt)
		pdf.Text(col2X, currentY, truncatePDF(pdf, data.SetterName, col2W))
	}
	currentY += 4.8

	// Row 2: date only. The QR sits below-right and the setter row above-right
	// already uses col2X, so we leave col2 empty here for breathing room.
	pdf.SetFont(cardFontFamily, "B", 5.5)
	pdf.SetTextColor(150, 150, 150)
	pdf.Text(col1X, currentY, "SET")
	currentY += 3.6

	pdf.SetFont(cardFontFamily, "", 9)
	pdf.SetTextColor(40, 40, 40)
	pdf.Text(col1X, currentY, data.Route.DateSet.Format("Jan 2, 2006"))

	// QR — bottom right of the info zone, drawn as native PDF vector rects
	// (one filled rect per dark module). The previous approach encoded a
	// 300×300 PNG per card and fed it to pdf.RegisterImageOptionsReader,
	// which retains the decoded pixel data for every card in a single
	// *Fpdf until Output() — for a large batch that's tens of MB held
	// across the entire render and was the primary OOM contributor when
	// routewerk-dev crashed (256MB VM). Vector rendering keeps the QR at
	// effectively infinite DPI, shrinks the resulting PDF, and frees the
	// per-card QR memory with the page's content stream instead of
	// accumulating it globally.
	if data.QRTargetURL != "" {
		drawQRVector(
			pdf,
			data.QRTargetURL,
			ox+portraitW-qrSizeMM-qrMarginRight,
			oy+portraitH-qrSizeMM-qrMarginBottom,
			qrSizeMM,
		)
	}
	// uniqueKey is retained in the signature so callers don't have to change;
	// we no longer need it now that there's no per-card image registration.
	_ = uniqueKey

	// ROUTEWERK footer — bottom-left of the info zone.
	pdf.SetFont(cardFontFamily, "B", 5)
	pdf.SetTextColor(170, 170, 170)
	pdf.Text(infoX, oy+portraitH-infoBottomPad, "ROUTEWERK")
}

// registerCardFonts embeds the Routewerk body font into pdf under the
// cardFontFamily name. Must run once per pdf, before any drawCardVector
// calls.
func registerCardFonts(pdf *gofpdf.Fpdf) {
	pdf.AddUTF8FontFromBytes(cardFontFamily, "", service.FontRegularTTF)
	pdf.AddUTF8FontFromBytes(cardFontFamily, "B", service.FontBoldTTF)
}

// ── helpers ──────────────────────────────────────────────────

// drawQRVector renders a QR code for content into a (size × size) mm square
// with its top-left at (x, y) using filled black rectangles — one per dark
// module in the encoded QR bitmap. The QR's built-in 4-module quiet zone is
// preserved (DisableBorder stays false) so scan reliability matches the
// previous raster output.
//
// Silent no-op on encode error: a failed QR shouldn't tank the whole sheet,
// the old raster path had the same "skip on error" semantics.
func drawQRVector(pdf *gofpdf.Fpdf, content string, x, y, size float64) {
	qr, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return
	}
	bitmap := qr.Bitmap()
	n := len(bitmap)
	if n == 0 {
		return
	}
	// Module size in mm. n already includes the quiet zone when
	// DisableBorder is false, which is the default and what we want.
	mod := size / float64(n)
	pdf.SetFillColor(0, 0, 0)
	for row := 0; row < n; row++ {
		// Cache the row slice to shave a bounds-check per module — noticeable
		// only in aggregate, but drawCardPortrait runs per card per page.
		cells := bitmap[row]
		for col := 0; col < n; col++ {
			if cells[col] {
				pdf.Rect(
					x+float64(col)*mod,
					y+float64(row)*mod,
					mod, mod,
					"F",
				)
			}
		}
	}
}

// fitFontSize returns the largest font size ≤ startPt (stepping in 0.5pt
// increments) that measures ≤ maxWidth for text under the currently-set
// family+style.
func fitFontSize(pdf *gofpdf.Fpdf, text string, maxWidth, startPt, minPt float64) float64 {
	return fitFontSizeStyle(pdf, text, maxWidth, startPt, minPt, "B")
}

// fitFontSizeStyle is the style-aware variant used when we need to measure
// against a non-bold style (e.g., the wall/setter metadata values that render
// in regular weight). Behaves the same as fitFontSize otherwise.
func fitFontSizeStyle(pdf *gofpdf.Fpdf, text string, maxWidth, startPt, minPt float64, style string) float64 {
	pt := startPt
	for pt >= minPt {
		pdf.SetFont(cardFontFamily, style, pt)
		if pdf.GetStringWidth(text) <= maxWidth {
			return pt
		}
		pt -= 0.5
	}
	return minPt
}

// wrapTextPDF greedy-wraps text to at most maxLines lines whose widths do
// not exceed maxWidth under the currently-set font. The last line is
// ellipsis-truncated if overflow remains after the line budget is exhausted.
func wrapTextPDF(pdf *gofpdf.Fpdf, text string, maxWidth float64, maxLines int) []string {
	words := strings.Fields(text)
	var lines []string
	var cur []string
	for _, word := range words {
		trial := strings.Join(append(cur, word), " ")
		if pdf.GetStringWidth(trial) <= maxWidth {
			cur = append(cur, word)
			continue
		}
		if len(cur) > 0 {
			lines = append(lines, strings.Join(cur, " "))
		}
		cur = []string{word}
		if len(lines) == maxLines-1 {
			break
		}
	}
	if len(cur) > 0 {
		lines = append(lines, strings.Join(cur, " "))
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	if n := len(lines); n > 0 && pdf.GetStringWidth(lines[n-1]) > maxWidth {
		lines[n-1] = truncatePDF(pdf, lines[n-1], maxWidth)
	}
	return lines
}

// truncatePDF shortens text character-by-character until it fits maxWidth
// under the currently-set font, appending "…" when truncation happens.
func truncatePDF(pdf *gofpdf.Fpdf, text string, maxWidth float64) string {
	if pdf.GetStringWidth(text) <= maxWidth {
		return text
	}
	runes := []rune(text)
	for i := len(runes) - 1; i > 0; i-- {
		candidate := string(runes[:i]) + "…"
		if pdf.GetStringWidth(candidate) <= maxWidth {
			return candidate
		}
	}
	return text
}

// hexToRGB parses #RRGGBB / #RGB into three 0–255 integers. Returns a mid
// gray on any parse error.
func hexToRGB(hex string) (int, int, int) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if len(hex) != 6 {
		return 100, 100, 100
	}
	var r, g, b int
	if _, err := fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b); err != nil {
		return 100, 100, 100
	}
	return r, g, b
}

// contrastRGB returns a near-black or near-white depending on the relative
// luminance of the background. Threshold matches the PNG renderer so the
// two paths agree on ink selection.
func contrastRGB(r, g, b int) (int, int, int) {
	lum := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
	if lum > 140 {
		return 30, 30, 30
	}
	return 255, 255, 255
}

// dim scales an RGB channel toward black by the given alpha (0–255), used to
// knock back the color subtitle so it reads as secondary to the grade.
func dim(channel, alpha int) int {
	return channel * alpha / 255
}
