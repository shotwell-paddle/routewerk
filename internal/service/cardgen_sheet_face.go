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

	sheetCardSplitFrac = 0.55 // top color zone occupies 55% of height

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
	onColor := contrastColor(routeColor)
	isCircuit := data.IsCircuit()

	splitY := float64(sheetCardH) * sheetCardSplitFrac

	// ---- Top color zone ----
	dc.SetColor(routeColor)
	dc.DrawRectangle(0, 0, float64(sheetCardW), splitY)
	dc.Fill()

	// Primary identifier: "V5" / "5.11+" for graded routes,
	// or the color name for circuit routes. Sized in points so the
	// physical grade is roughly 3/4" tall regardless of the string length.
	identifier := data.Route.Grade
	startPt := 56.0
	if isCircuit {
		identifier = strings.ToUpper(data.ColorLabel())
		// "PURPLE" is wider than "V5" — start smaller so fitPrintFont has
		// somewhere to go instead of immediately capping at startPt.
		startPt = 42.0
	}
	dc.SetColor(onColor)
	fitPrintFont(dc, identifier, fontBold, float64(sheetCardW)-80, startPt, 18)
	iw, ih := dc.MeasureString(identifier)
	dc.DrawString(identifier, (float64(sheetCardW)-iw)/2, (splitY+ih)/2)

	// Secondary tape-color subtitle — only when it adds information
	// (i.e. not for circuit routes, where the identifier IS the color).
	if !isCircuit {
		label := strings.ToUpper(data.ColorLabel())
		dc.SetColor(withAlpha(onColor, 230))
		setPrintFont(dc, fontBold, 10)
		lw, _ := dc.MeasureString(label)
		dc.DrawString(label, (float64(sheetCardW)-lw)/2, splitY-44)
	}

	// ---- Bottom info zone (white) ----
	//
	// Layout uses pixel coordinates (gg is pixel-based) but font sizes are in
	// points at 300 DPI. Conversion: 1pt = 300/72 px ≈ 4.17 px.
	//
	// Vertical budget (bottom zone ≈ 472 px):
	//   name block (up to 2 × 68 + 18 gap)  ~154
	//   row 1 label (9pt, 26 px after gap)    26
	//   row 1 value (13pt, 54 px after gap)   54
	//   gap                                   34
	//   row 2 label                           26
	//   row 2 value                           54
	//   branding footer + slack               ~124 (QR lives here)

	// Route name — bold, wraps to 2 lines max. Left-aligned, 18pt reads
	// cleanly on a 3.5" card held at arm's length.
	nameLineHeight := 68.0
	y := splitY + 52 // first baseline: ascent-padded below the color split
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

	// QR — bottom right. 160 px = ~13.5 mm on the printed card; medium error
	// correction + that size scans reliably from 18" with a phone camera.
	// Positioned so the SETTER value row (ends ~y=829) clears the QR top
	// (y=840) with ~10 px of daylight.
	qrPx := 160
	if qrImg, err := generateQRImage(data.QRTargetURL, qrPx); err == nil {
		dc.DrawImage(qrImg, sheetCardW-qrPx-30, sheetCardH-qrPx-50)
	}

	// Branding
	dc.SetColor(color.RGBA{170, 170, 170, 255})
	setPrintFont(dc, fontBold, 9)
	dc.DrawString("ROUTEWERK", 30, float64(sheetCardH)-22)

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
