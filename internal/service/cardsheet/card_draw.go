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
// Layout is INVERTED relative to a trading card — colour at the BOTTOM,
// not the top. Route tags get taped next to the start hold and the top
// of the tag slides partially under the hold, hiding anything drawn
// there. The primary identifier (grade for graded routes, colour name
// for circuits) prints INSIDE the bottom colour bar, in pure black or
// white depending on background contrast. Everything else clusters
// tight against the top of the bar in the lower ~30mm of the card,
// leaving the upper half as intentional whitespace — the "tuck zone"
// where content gets harmlessly hidden when the tag rides under a
// hold.
//
// Vertical stack, top → bottom (mm):
//
//	0    ─ tuck zone (blank white — anything here can be hidden)
//	~37  ─ info cluster: route name, wall, setter, date, QR.
//	       Laid out so the last row (date) sits ~3mm above the
//	       bottom colour bar.
//	66.9 ─ bottom colour bar (22mm): route colour + the primary
//	       identifier text (grade or colour name) centred in it.
//	88.9 ─ card bottom
//
// Total ink coverage ~25% (22mm bottom bar, full width) + text ink;
// well under Terra Slate's ~50% polymer-toner ceiling so the printer
// lays down a clean fuse without per-job density tuning.
//
// Coordinates inside drawCardPortrait are expressed relative to the
// portrait top-left (ox, oy); drawCardVector rotates this whole drawing
// to land inside the landscape sheet slot.
const (
	portraitW = 50.8 // 2.0" — card short axis
	portraitH = 88.9 // 3.5" — card long axis

	bottomBarH = 22.0 // main colour bar at the bottom
	infoPadX   = 3.0  // left padding inside the info zone

	qrSizeMM      = 14.0
	qrMarginRight = 2.5

	// Derived anchors.
	bottomBarY = portraitH - bottomBarH // 66.9

	// Info cluster starts 37mm below the card top so the stack of
	// {route name + 2 metadata rows + SET + date} lands with the date
	// baseline ~3mm above the colour bar. Above y=infoTopY the card
	// is intentional whitespace — the tuck zone.
	infoTopY = 37.0
	// Gap between infoTopY and the first name baseline. Matches the
	// legacy infoPadTop value.
	infoPadTop = 5.0
)

// drawCardVector places the portrait card design into a landscape slot at
// (x, y) with dimensions (w, h) = (88.9, 50.8) on the sheet. The card is
// drawn by drawCardPortrait in portrait coordinates; this wrapper applies a
// 90° CCW rotation (via gofpdf's transform stack) so the color zone — which
// sits on TOP of the portrait design — ends up on the LEFT of the landscape
// slot, matching the visual orientation the old raster path produced.
//
// bleedMM is the distance (in mm) the coloured zone extends past the
// portrait's top/left/right edges so registration drift doesn't expose
// white paper along the cut. 0 disables bleed.
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
func drawCardVector(pdf *gofpdf.Fpdf, data service.CardData, x, y, w, h, bleedMM float64, uniqueKey string) {
	pdf.TransformBegin()
	pdf.TransformTranslate(x, y+h)
	pdf.TransformRotate(90, 0, 0)
	drawCardPortrait(pdf, data, 0, 0, bleedMM, uniqueKey)
	pdf.TransformEnd()
}

// drawCardPortrait renders the portrait design at local top-left (ox, oy)
// inside the current (possibly transformed) drawing space. Dimensions are
// fixed at portraitW × portraitH — caller-supplied (ox, oy) only translate
// the drawing, they don't resize it.
//
// bleedMM extends the bottom colour bar past its cut line (bottom, left,
// right). The top of the card is plain white so no top bleed is needed.
//
// The primary identifier (grade for graded routes, colour name for
// circuit routes) prints INSIDE the colour bar, in pure black or white
// chosen for contrast. Route name + metadata + QR cluster tight against
// the top of the bar; everything above that is intentional whitespace.
func drawCardPortrait(pdf *gofpdf.Fpdf, data service.CardData, ox, oy, bleedMM float64, uniqueKey string) {
	routeR, routeG, routeB := hexToRGB(data.Route.Color)

	// ---- Bottom colour bar ----
	// The main colour ID. Bleed expands down, left, and right.
	pdf.SetFillColor(routeR, routeG, routeB)
	pdf.Rect(ox-bleedMM, oy+bottomBarY, portraitW+2*bleedMM, bottomBarH+bleedMM, "F")

	// ---- Primary identifier inside the colour bar ----
	// Graded routes: show the grade ("V5", "5.11+"). Circuits: show
	// the colour name ("RED", "YELLOW"). Either way, pure black or
	// pure white depending on luminance — hard contrast reads clean
	// from across the gym on every swatch including canary yellow
	// and off-white.
	var label string
	var startPt float64
	if data.IsCircuit() {
		label = strings.ToUpper(data.ColorLabel())
		startPt = 24.0 // colour names run wider than V-grades
	} else {
		label = data.Route.Grade
		startPt = 34.0
	}
	ptSize := fitFontSize(pdf, label, portraitW-8, startPt, 10)
	pdf.SetFont(cardFontFamily, "B", ptSize)
	textR, textG, textB := onColorInkRGB(routeR, routeG, routeB)
	pdf.SetTextColor(textR, textG, textB)
	lw := pdf.GetStringWidth(label)
	capMM := ptSize * 25.4 / 72.0 * 0.72
	barBaseline := oy + bottomBarY + bottomBarH*0.5 + capMM/2
	pdf.Text(ox+(portraitW-lw)/2, barBaseline, label)

	// ---- Info flow (route name, metadata, QR) ----
	// Clusters tight against the top of the colour bar — the LAST row
	// (date) sits ~3mm above the bar. Everything above infoTopY is
	// blank white, the tuck zone where a hold can hide content
	// harmlessly.
	infoTop := oy + infoTopY
	infoX := ox + infoPadX
	// textColW is the route-name column width. Narrower than infoW so
	// the name doesn't flow under the right-aligned QR.
	textColW := portraitW - 2*infoPadX - qrSizeMM - 2.0

	// uniqueKey is retained in the signature so callers don't have to
	// change; we no longer need it now that there's no per-card image
	// registration.
	_ = uniqueKey

	// Route name — bold, wrapped to 2 lines max. Columns are narrow so
	// the name doesn't flow into the QR's vertical strip on the right.
	namePt := 11.0
	nameLine := 4.4
	pdf.SetFont(cardFontFamily, "B", namePt)
	pdf.SetTextColor(0, 0, 0)
	currentY := infoTop + infoPadTop
	if data.Route.Name != nil && *data.Route.Name != "" {
		lines := wrapTextPDF(pdf, *data.Route.Name, textColW, 2)
		for _, line := range lines {
			pdf.Text(infoX, currentY, line)
			currentY += nameLine
		}
	}
	// Pin the metadata grid to a consistent Y regardless of name line
	// count, so one-liner names (or missing names, as on circuit cards)
	// don't cause the whole block to float up. The metadata rows always
	// sit at the same height relative to the colour bar.
	minMetaY := infoTop + infoPadTop + 2*nameLine + 1.2
	if currentY < minMetaY {
		currentY = minMetaY
	} else {
		currentY += 1.0
	}

	// Metadata grid — two columns, constrained so column 2 ends before
	// the right-aligned QR. col1X=3mm, col2X ≈ 21.5mm, col2 ends ≈
	// 32.8mm — leaves ~1.5mm clearance before the QR left edge at
	// ox + portraitW - qrSizeMM - qrMarginRight = 34.3.
	const (
		colGutter = 1.5
		col1W     = 17.0 // room for "The Cave" at 9pt, or longer at 7pt
		col2W     = 11.5 // room for "Chris" at 9pt
	)
	col1X := infoX
	col2X := col1X + col1W + colGutter

	// Row 1: WALL / SETTER labels
	pdf.SetFont(cardFontFamily, "B", 5.5)
	pdf.SetTextColor(0, 0, 0)
	pdf.Text(col1X, currentY, "WALL")
	pdf.Text(col2X, currentY, "SETTER")
	currentY += 3.6

	// Row 1: values. Auto-shrink before truncating.
	pdf.SetTextColor(0, 0, 0)
	wallPt := fitFontSizeStyle(pdf, data.WallName, col1W, 9.0, 7.0, "")
	pdf.SetFont(cardFontFamily, "", wallPt)
	pdf.Text(col1X, currentY, truncatePDF(pdf, data.WallName, col1W))
	if data.SetterName != "" {
		setterPt := fitFontSizeStyle(pdf, data.SetterName, col2W, 9.0, 7.0, "")
		pdf.SetFont(cardFontFamily, "", setterPt)
		pdf.Text(col2X, currentY, truncatePDF(pdf, data.SetterName, col2W))
	}
	currentY += 4.4

	// Row 2: SET / date.
	pdf.SetFont(cardFontFamily, "B", 5.5)
	pdf.SetTextColor(0, 0, 0)
	pdf.Text(col1X, currentY, "SET")
	currentY += 3.6

	pdf.SetFont(cardFontFamily, "", 9)
	pdf.SetTextColor(0, 0, 0)
	pdf.Text(col1X, currentY, data.Route.DateSet.Format("Jan 2, 2006"))

	// QR — right-aligned, vertically positioned so its bottom sits 2mm
	// above the colour bar. Lives in the strip of the info zone that
	// the 2-column metadata grid deliberately leaves clear.
	if data.QRTargetURL != "" {
		qrX := ox + portraitW - qrSizeMM - qrMarginRight
		qrY := oy + bottomBarY - qrSizeMM - 2.0
		drawQRVector(pdf, data.QRTargetURL, qrX, qrY, qrSizeMM)
	}

	// ROUTEWERK footer — dropped in the 2026-04 redesign. The info
	// cluster now sits tight against the colour bar with no vertical
	// slack for a footer, and the brand mark is the lowest-priority
	// piece of ink on the card. If branding needs to come back, the
	// natural home is top-of-card inside the tuck zone at ~y=5.
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

// subtitleInkRGB picks the ink colour for the colour-name subtitle drawn
// under the primary identifier on graded routes. The subtitle lives in
// the white zone, so the default is the route colour itself — that ties
// the subtitle visually to the bottom colour bar. For low-contrast route
// colours (the off-white "white hold" swatch, canary yellow, anything
// else with luminance > 200 that would disappear on white) we fall
// back to a medium-dark grey that reads cleanly on white.
func subtitleInkRGB(r, g, b int) (int, int, int) {
	lum := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
	if lum > 200 {
		return 85, 85, 85 // medium-dark grey
	}
	return r, g, b
}

// onColorInkRGB picks pure black or pure white for text drawn ON the
// route-colour background of the bottom colour bar. Used for the
// circuit-card colour-name label. ITU-R BT.601 luminance > 128 flips
// to black so "brighter" backgrounds (yellow, pale blue, white) read
// with dark text and darker backgrounds (red, blue, purple, black,
// etc.) read with white text. No grey fallback — the gym prefers
// hard black-or-white contrast on the bar for unambiguous legibility
// at distance.
func onColorInkRGB(r, g, b int) (int, int, int) {
	lum := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
	if lum > 128 {
		return 0, 0, 0
	}
	return 255, 255, 255
}
