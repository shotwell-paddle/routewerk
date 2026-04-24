package service

// ============================================================
// SHEET-READY CARD FACE
//
// Renders a single 600×1050 px portrait card face (2" × 3.5" @ 300 DPI)
// suitable for placement on an 8-up print-and-cut sheet. Implements
// Design D (trading-card split) — the variant we ship by default.
//
// This is separate from generateGradedPrintPNG (400×300 landscape) because
// the digital/web endpoints need a different aspect ratio and density than
// the physical card cutter needs.
//
// Font rendering here is tuned for 300 DPI print output:
//   - Sizes are expressed in real typographic points; DPI 300 in the
//     truetype.Options converts them to the correct pixel metrics for the
//     canvas. Means a 13pt body is 54 px tall, which lines up with "body
//     copy on a 3.5" card" intuition.
//   - Hinting is HintingFull. Without it freetype's output for small body
//     type looks fuzzy on an inkjet (visible as soft edges / ink bleed on
//     the first physical test cuts).
//   - Label / value sizes are deliberately larger than the on-screen print
//     cards — a wall-mounted card has to be readable at arm's length, not
//     laptop-distance.
// ============================================================

import (
	"image/color"
	"strings"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
)

const (
	sheetCardW = 600  // px — 2" @ 300 DPI
	sheetCardH = 1050 // px — 3.5" @ 300 DPI

	// Pixel-accurate mirrors of the PDF path's mm-based constants so the
	// PNG sheet face stays visually identical to what the production PDF
	// renderer draws. 1 mm = 1050/88.9 ≈ 11.811 px at 300 DPI.
	// Must stay in sync with colorZoneH / bottomStripeH / identifierZoneH
	// in internal/service/cardsheet/card_draw.go.
	sheetCardColorBarPx   = 260 // 22mm top colour bar
	sheetCardStripePx     = 35  // 3mm bottom colour stripe
	sheetCardIdentifierPx = 236 // 20mm identifier band inside the white zone

	// sheetCardDPI is the output density we render for. Must match the
	// physical print resolution to keep hinting aligned to the pixel grid;
	// the Silhouette Cameo prints at 300 DPI by default.
	sheetCardDPI = 300
)

// GenerateSheetCardPNG produces the portrait card face used by the sheet
// composer. The returned PNG is 600×1050 RGBA.
//
// Graded routes (V-scale, YDS) show their grade with a small tape-color
// subtitle. Circuit routes show the color name in place of the grade
// (and suppress the redundant subtitle) — matching the gym convention.
func (g *CardGenerator) GenerateSheetCardPNG(data CardData) ([]byte, error) {
	dc := gg.NewContext(sheetCardW, sheetCardH)

	dc.SetColor(color.White)
	dc.Clear()

	routeColor := parseHexColor(data.Route.Color)
	isCircuit := data.IsCircuit()

	// Flipped layout — matches the PDF path. Colour sits at the bottom
	// of the card so it's always visible when the tag is tucked under
	// a start hold. Top of the card is plain white (tuck zone).

	// ---- Bottom colour bar ----
	dc.SetColor(routeColor)
	dc.DrawRectangle(0, float64(sheetCardH-sheetCardColorBarPx),
		float64(sheetCardW), float64(sheetCardColorBarPx))
	dc.Fill()

	// ---- Primary identifier INSIDE the colour bar ----
	// Graded routes show the grade ("V5", "5.11+"). Circuits show the
	// colour name ("RED", "YELLOW"). Either way, pure black or pure
	// white based on background luminance — hard contrast on every
	// palette swatch (canary yellow, off-white, red, black).
	var label string
	var startPt float64
	if isCircuit {
		label = strings.ToUpper(data.ColorLabel())
		startPt = 56.0
	} else {
		label = data.Route.Grade
		startPt = 72.0
	}
	rr, gg, bb, _ := routeColor.RGBA()
	lum := 0.299*float64(rr>>8) + 0.587*float64(gg>>8) + 0.114*float64(bb>>8)
	if lum > 128 {
		dc.SetColor(color.RGBA{0, 0, 0, 255})
	} else {
		dc.SetColor(color.RGBA{255, 255, 255, 255})
	}
	fitPrintFont(dc, label, fontBold, float64(sheetCardW)-80, startPt, 18)
	lw, lh := dc.MeasureString(label)
	barTop := float64(sheetCardH - sheetCardColorBarPx)
	barMid := barTop + float64(sheetCardColorBarPx)/2
	dc.DrawString(label, (float64(sheetCardW)-lw)/2, barMid+lh/2)

	// ---- Info flow (route name, metadata) ----
	// Clusters tight against the top of the colour bar. Mirrors the
	// PDF path's anchor: infoTopY (37mm) + infoPadTop (5mm) = 42mm
	// ≈ 496 px at 300 DPI. Everything above this is intentional
	// whitespace — the tuck zone.
	nameLineHeight := 68.0
	y := 496.0
	if data.Route.Name != nil && *data.Route.Name != "" {
		dc.SetColor(color.RGBA{25, 25, 25, 255})
		setPrintFont(dc, fontBold, 18)
		lines := wrapLines(dc, *data.Route.Name, float64(sheetCardW)-60, 2)
		for _, line := range lines {
			dc.DrawString(line, 30, y)
			y += nameLineHeight
		}
		y += 14 // gap before the metadata grid
	}

	// 2-column metadata grid. Labels are small, uppercase, gray; values are
	// 13pt regular in near-black. Spacing between the label baseline and the
	// value baseline has to exceed the value's ascent (~40 px for 13pt at
	// 300 DPI) — earlier revisions used 36 px, which overlapped the two.
	//
	// Column width budget: col1 spans 30–290 (260 px), col2 spans 310–570.
	// Setter values have to fit ABOVE the QR (QR top at y=840), not beside
	// it — the right-hand column shares its x-range with the QR lower down.
	col1X := 30.0
	col2X := float64(sheetCardW)/2 + 10
	labelToValue := 52.0 // baseline gap: label (9pt, ~9 px ascent) → value (13pt, ~40 px ascent) + ~3 px clearance
	rowGap := 38.0       // baseline gap: value → next label (12 px descender + ~26 px clearance)

	// Row 1 — WALL + SETTER
	dc.SetColor(color.RGBA{150, 150, 150, 255})
	setPrintFont(dc, fontBold, 9)
	dc.DrawString("WALL", col1X, y)
	dc.DrawString("SETTER", col2X, y)
	y += labelToValue
	dc.SetColor(color.RGBA{40, 40, 40, 255})
	setPrintFont(dc, fontRegular, 13)
	dc.DrawString(truncateText(dc, data.WallName, 260), col1X, y)
	if data.SetterName != "" {
		dc.DrawString(truncateText(dc, data.SetterName, 240), col2X, y)
	}
	y += rowGap

	// Row 2 — SET (date). Value sits under WALL in col1; col2 is left empty
	// because the QR lands there, so crowding the date next to a scan code
	// reads as noise.
	dc.SetColor(color.RGBA{150, 150, 150, 255})
	setPrintFont(dc, fontBold, 9)
	dc.DrawString("SET", col1X, y)
	y += labelToValue
	dc.SetColor(color.RGBA{40, 40, 40, 255})
	setPrintFont(dc, fontRegular, 13)
	dc.DrawString(data.Route.DateSet.Format("Jan 2, 2006"), col1X, y)

	// QR — right-aligned, vertically positioned so its bottom sits
	// ~24 px (2mm) above the colour bar. Lives in the right strip
	// that the 2-column metadata grid deliberately keeps clear.
	qrPx := 160
	if qrImg, err := generateQRImage(data.QRTargetURL, qrPx); err == nil {
		qrY := sheetCardH - sheetCardColorBarPx - qrPx - 24
		dc.DrawImage(qrImg, sheetCardW-qrPx-30, qrY)
	}

	// Branding dropped in the 2026-04 flip — the info cluster is tight
	// against the colour bar with no vertical slack for a footer.

	return encodePNG(dc)
}

// setPrintFont sets the current font face for print-DPI rendering (300 DPI).
// `pt` is treated as real typographic points — a 12pt body renders at
// 12 × 300/72 = 50 px on the 300 DPI canvas. Full hinting keeps glyph edges
// crisp at the target pixel density; without it small labels (9pt) smear on
// the first inkjet pass.
func setPrintFont(dc *gg.Context, f *truetype.Font, pt float64) {
	face := truetype.NewFace(f, &truetype.Options{
		Size:    pt,
		DPI:     sheetCardDPI,
		Hinting: font.HintingFull,
	})
	dc.SetFontFace(face)
}

// fitPrintFont is the print-DPI variant of the size-to-fit helper used for
// the grade / circuit-color identifier. Sizes are in points. Starts at
// startPt and steps down by 2pt until the measured width fits maxWidth or
// minPt is reached, then leaves dc with that face.
//
// Step size is 2pt rather than 4pt because at print DPI a 4pt drop is a
// visible ~17 px jump — tight enough that we want finer granularity to
// avoid leaving visible whitespace around long identifiers like "PURPLE".
func fitPrintFont(dc *gg.Context, text string, f *truetype.Font, maxWidth, startPt, minPt float64) {
	var last font.Face
	pt := startPt
	for pt >= minPt {
		face := truetype.NewFace(f, &truetype.Options{
			Size:    pt,
			DPI:     sheetCardDPI,
			Hinting: font.HintingFull,
		})
		dc.SetFontFace(face)
		w, _ := dc.MeasureString(text)
		last = face
		if w <= maxWidth {
			return
		}
		pt -= 2
	}
	if last != nil {
		dc.SetFontFace(last)
	}
}

// wrapLines greedy-wraps text to at most maxLines lines that fit within
// maxWidth using the currently-set font face on dc. Overflow on the last
// line is truncated with an ellipsis.
func wrapLines(dc *gg.Context, text string, maxWidth float64, maxLines int) []string {
	words := strings.Fields(text)
	var lines []string
	var cur []string
	for _, word := range words {
		trial := strings.Join(append(cur, word), " ")
		tw, _ := dc.MeasureString(trial)
		if tw <= maxWidth {
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
	// Ellipsis-truncate the last line if still too wide for maxWidth.
	if len(lines) > 0 {
		last := lines[len(lines)-1]
		if lw, _ := dc.MeasureString(last); lw > maxWidth {
			for len(last) > 0 {
				if ew, _ := dc.MeasureString(last + "…"); ew <= maxWidth {
					break
				}
				last = last[:len(last)-1]
			}
			lines[len(lines)-1] = last + "…"
		}
	}
	return lines
}
