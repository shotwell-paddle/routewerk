package service

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"strings"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"github.com/jung-kurt/gofpdf/v2"
	qrcode "github.com/skip2/go-qrcode"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/shotwell-paddle/routewerk/internal/model"
)

// ============================================================
// Fonts
//
// We ship the Go typeface (Bigelow & Holmes) embedded in the binary via
// golang.org/x/image/font/gofont. That keeps rendering identical across
// Linux/macOS/Windows hosts and the Alpine Docker image, where DejaVu isn't
// installed — the previous host-filesystem lookup would silently fall back
// to the Go fonts in production anyway. Shipping bytes also lets the
// vector PDF renderer in cardsheet embed the same font via
// gofpdf.AddUTF8FontFromBytes.
// ============================================================

var (
	// Raw TTF bytes, exported at package scope for sibling packages
	// (cardsheet) that need to embed them in PDFs.
	FontRegularTTF = goregular.TTF
	FontBoldTTF    = gobold.TTF

	fontRegular *truetype.Font
	fontBold    *truetype.Font
)

func init() {
	fontRegular, _ = truetype.Parse(FontRegularTTF)
	fontBold, _ = truetype.Parse(FontBoldTTF)
}

// setFont sets the current font face for screen-resolution rendering (the
// digital + small print-card variants).
//
// Hinting=Full snaps outlines to the pixel grid so stems and crossbars land
// on whole pixels instead of being anti-aliased across two rows. That keeps
// body copy and tag labels from looking soft at OG-card display sizes, where
// browsers typically downscale the 2x canvas to fit article previews. We
// previously used HintingNone for "smoother curves" but it visibly fuzzes
// everything below ~20pt — crispness wins at this scale.
//
// Size is treated as pixels (DPI 72 → 1pt == 1px) because the rest of the
// drawing code uses pixel coordinates on a high-DPI canvas. Sheet cards use
// a separate setPrintFont (see cardgen_sheet_face.go) tuned for 300 DPI
// print, where grid-snapping lines up with the physical pixel density.
func setFont(dc *gg.Context, f *truetype.Font, size float64) {
	face := truetype.NewFace(f, &truetype.Options{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	dc.SetFontFace(face)
}

// ============================================================
// Card data
// ============================================================

type CardData struct {
	Route        *model.Route
	WallName     string
	LocationName string
	SetterName   string
	QRTargetURL  string
}

// IsCircuit returns true if this route uses a circuit/color-based system
// rather than a numeric grade. Exported so sibling packages (cardsheet) can
// branch on it when drawing card faces directly into PDFs.
func (d CardData) IsCircuit() bool {
	if d.Route.CircuitColor != nil && *d.Route.CircuitColor != "" {
		return true
	}
	return d.Route.GradingSystem == "circuit"
}

// ColorLabel returns a human-readable color name for accessibility.
// Uses CircuitColor if set, otherwise derives from the hex Color field.
func (d CardData) ColorLabel() string {
	if d.Route.CircuitColor != nil && *d.Route.CircuitColor != "" {
		return titleCase(strings.ToLower(*d.Route.CircuitColor))
	}
	return hexToName(d.Route.Color)
}

// ============================================================
// CardGenerator
// ============================================================

type CardGenerator struct {
	frontendURL string
}

func NewCardGenerator(frontendURL string) *CardGenerator {
	return &CardGenerator{frontendURL: strings.TrimRight(frontendURL, "/")}
}

func (g *CardGenerator) RouteURL(locationID, routeID string) string {
	return fmt.Sprintf("%s/locations/%s/routes/%s", g.frontendURL, locationID, routeID)
}

// ============================================================
// PRINT CARD — graded routes
//
// Hangs next to the route on the wall. Grade readable from 5 ft.
// Color name labeled for colorblind accessibility. No volatile data.
// Compact 400×300 (~4"×3" at 100 DPI).
// ============================================================

const (
	printW = 400
	printH = 300
)

// GeneratePrintPNG auto-selects graded vs circuit layout.
func (g *CardGenerator) GeneratePrintPNG(data CardData) ([]byte, error) {
	if data.IsCircuit() {
		return g.generateCircuitPrintPNG(data)
	}
	return g.generateGradedPrintPNG(data)
}

func (g *CardGenerator) GeneratePrintPDF(data CardData) ([]byte, error) {
	return g.wrapPDF(data, g.GeneratePrintPNG, printW, printH)
}

func (g *CardGenerator) generateGradedPrintPNG(data CardData) ([]byte, error) {
	dc := gg.NewContext(printW, printH)
	routeColor := parseHexColor(data.Route.Color)

	// Dark background — NRC-inspired
	dc.SetColor(color.RGBA{20, 20, 18, 255})
	dc.Clear()

	// -- Route color accent block (left side) --
	blockW := 110.0
	dc.SetColor(routeColor)
	dc.DrawRoundedRectangle(20, 20, blockW, 180, 14)
	dc.Fill()

	// -- Grade inside color block --
	gradeText := data.Route.Grade
	fontSize := gradeSize(gradeText)
	dc.SetColor(contrastColor(routeColor))
	setFont(dc, fontBold, fontSize)
	gw, gh := dc.MeasureString(gradeText)
	dc.DrawString(gradeText, 20+(blockW-gw)/2, 20+90+(gh/2))

	// -- Color name below grade in block --
	dc.SetColor(withAlpha(contrastColor(routeColor), 180))
	setFont(dc, fontBold, 10)
	colorLabel := strings.ToUpper(data.ColorLabel())
	clw, _ := dc.MeasureString(colorLabel)
	dc.DrawString(colorLabel, 20+(blockW-clw)/2, 170)

	// -- Route info (right of block) --
	textX := 150.0
	infoY := 46.0

	// Route name
	if data.Route.Name != nil && *data.Route.Name != "" {
		dc.SetColor(color.RGBA{255, 255, 255, 255})
		setFont(dc, fontBold, 24)
		dc.DrawString(truncateText(dc, *data.Route.Name, float64(printW)-textX-20), textX, infoY)
		infoY += 32
	}

	// Wall name
	dc.SetColor(color.RGBA{180, 175, 170, 255})
	setFont(dc, fontRegular, 16)
	dc.DrawString(data.WallName, textX, infoY)
	infoY += 24

	// Route type
	if data.Route.RouteType != "" {
		dc.SetColor(color.RGBA{120, 115, 110, 255})
		setFont(dc, fontRegular, 13)
		dc.DrawString(formatRouteType(data.Route.RouteType), textX, infoY)
	}

	// -- Setter + date (right side, lower) --
	footerY := 170.0
	if data.SetterName != "" {
		dc.SetColor(color.RGBA{140, 135, 130, 255})
		setFont(dc, fontRegular, 12)
		dc.DrawString("Set by "+data.SetterName, textX, footerY)
		footerY += 18
	}
	dc.SetColor(color.RGBA{100, 96, 92, 255})
	setFont(dc, fontRegular, 11)
	dc.DrawString(data.Route.DateSet.Format("Jan 2, 2006"), textX, footerY)

	// -- QR code (bottom right) --
	qrImg, err := generateQRImage(data.QRTargetURL, 80)
	if err == nil {
		dc.DrawImage(qrImg, printW-100, printH-118)
		dc.SetColor(color.RGBA{100, 96, 92, 255})
		setFont(dc, fontRegular, 8)
		dc.DrawStringAnchored("Scan to log", float64(printW)-60, float64(printH)-30, 0.5, 0.5)
	}

	// -- Branding --
	dc.SetColor(color.RGBA{70, 67, 64, 255})
	setFont(dc, fontBold, 9)
	dc.DrawString("ROUTEWERK", 20, float64(printH)-14)

	return encodePNG(dc)
}

// ============================================================
// PRINT CARD — circuit routes
//
// Circuit gyms identify routes by color, not grade. The route color
// fills the card as the dominant visual. Color name is written large
// and clear for colorblind accessibility. Grade shown small if present.
//
// Layout (400×300):
//   ┌──────────────────────────────────┐
//   │██████████████████████████████████│
//   │██                              ██│
//   │██    RED CIRCUIT               ██│ ← color name, huge
//   │██    The Overhang              ██│ ← wall name
//   │██    Boulder                   ██│ ← route type
//   │██████████████████████████████████│
//   │                                  │
//   │   Set by Chris S.       [QR]     │
//   │   Mar 15, 2026          [QR]     │
//   │   routewerk                      │
//   └──────────────────────────────────┘
// ============================================================

func (g *CardGenerator) generateCircuitPrintPNG(data CardData) ([]byte, error) {
	dc := gg.NewContext(printW, printH)
	routeColor := parseHexColor(data.Route.Color)

	// Dark background with bold color stripe — cheaper to print than full-color
	dc.SetColor(color.RGBA{20, 20, 18, 255})
	dc.Clear()

	// -- Bold color stripe (left edge) --
	stripeW := 28.0
	dc.SetColor(routeColor)
	dc.DrawRectangle(0, 0, stripeW, float64(printH))
	dc.Fill()

	// -- Circuit color name — huge, the primary identifier --
	textX := stripeW + 24
	circuitLabel := strings.ToUpper(data.ColorLabel())
	dc.SetColor(color.RGBA{255, 255, 255, 255})
	setFont(dc, fontBold, 52)
	dc.DrawString(circuitLabel, textX, 72)

	// "CIRCUIT" subtitle
	dc.SetColor(color.RGBA{120, 115, 110, 255})
	setFont(dc, fontBold, 14)
	dc.DrawString("CIRCUIT", textX, 94)

	// -- Route name (if set) --
	infoY := 130.0
	if data.Route.Name != nil && *data.Route.Name != "" {
		dc.SetColor(color.RGBA{220, 215, 210, 255})
		setFont(dc, fontBold, 20)
		dc.DrawString(*data.Route.Name, textX, infoY)
		infoY += 28
	}

	// -- Wall name --
	dc.SetColor(color.RGBA{180, 175, 170, 255})
	setFont(dc, fontRegular, 16)
	dc.DrawString(data.WallName, textX, infoY)
	infoY += 22

	// -- Grade (if present, secondary) --
	if data.Route.Grade != "" {
		dc.SetColor(color.RGBA{120, 115, 110, 255})
		setFont(dc, fontRegular, 13)
		dc.DrawString(formatRouteType(data.Route.RouteType)+"  ·  "+data.Route.Grade, textX, infoY)
	}

	// -- QR code (bottom right) --
	qrImg, err := generateQRImage(data.QRTargetURL, 80)
	if err == nil {
		dc.DrawImage(qrImg, printW-100, printH-118)
		dc.SetColor(color.RGBA{100, 96, 92, 255})
		setFont(dc, fontRegular, 8)
		dc.DrawStringAnchored("Scan to log", float64(printW)-60, float64(printH)-30, 0.5, 0.5)
	}

	// -- Setter + branding --
	if data.SetterName != "" {
		dc.SetColor(color.RGBA{140, 135, 130, 255})
		setFont(dc, fontRegular, 11)
		dc.DrawString("Set by "+data.SetterName+"  ·  "+data.Route.DateSet.Format("Jan 2, 2006"), textX, float64(printH)-34)
	}

	dc.SetColor(color.RGBA{70, 67, 64, 255})
	setFont(dc, fontBold, 9)
	dc.DrawString("ROUTEWERK", textX, float64(printH)-14)

	return encodePNG(dc)
}


// ============================================================
// DIGITAL CARD — shareable, landscape 1200×630 (OG-standard)
//
// Both graded and circuit routes render through the same unified
// layout: a full-bleed color hero on the left carries the route's
// primary identifier (grade or color name), and a dark info panel
// on the right carries name, location, setter, tags, and stats.
// The two variants differ only in what fills the hero zone — every
// other measurement is identical so they feel like the same product.
// ============================================================

// Digital cards render at 2x the nominal OG size (1200×630) so browsers have
// retina-quality pixels to downscale from. Every coordinate below is written
// in the 2x pixel space directly — there is no "scale" multiplier threaded
// through the code because that would invite drift between layout math and
// constants.
const (
	digitalW     = 2400
	digitalH     = 1260
	heroW        = 960 // width of the left color hero zone
	infoPadLeft  = 112 // left padding inside the info panel
	infoPadRight = 96  // right padding inside the info panel
	infoPadTopY  = 144 // top padding inside the info panel
	infoPadBot   = 80  // bottom padding inside the info panel
)

func (g *CardGenerator) GenerateDigitalPNG(data CardData) ([]byte, error) {
	return g.generateDigitalPNG(data)
}

func (g *CardGenerator) GenerateDigitalPDF(data CardData) ([]byte, error) {
	return g.wrapPDF(data, g.GenerateDigitalPNG, digitalW, digitalH)
}

// generateDigitalPNG renders a unified layout for graded + circuit routes.
// Only the hero zone contents differ between variants; everything else is
// laid out the same way so the two feel like the same card family.
func (g *CardGenerator) generateDigitalPNG(data CardData) ([]byte, error) {
	dc := gg.NewContext(digitalW, digitalH)
	routeColor := parseHexColor(data.Route.Color)
	onHero := contrastColor(routeColor)

	// ---- Panel backgrounds ----
	// Info panel (right): warm near-black. Drawn first so hero sits on top.
	panelBG := color.RGBA{20, 18, 16, 255}
	dc.SetColor(panelBG)
	dc.Clear()

	// Hero panel (left): full-bleed route color.
	dc.SetColor(routeColor)
	dc.DrawRectangle(0, 0, heroW, digitalH)
	dc.Fill()

	// Subtle vertical shadow on the panel seam so the hero feels slightly
	// elevated over the info panel.
	for i := 0; i < 48; i++ {
		alpha := uint8(60 - i)
		if alpha > 60 {
			alpha = 0
		}
		dc.SetColor(color.RGBA{0, 0, 0, alpha})
		dc.DrawLine(float64(heroW+i), 0, float64(heroW+i), digitalH)
		dc.Stroke()
	}

	// ---- Hero content ----
	// Primary identifier: grade for graded routes, color name for circuits.
	// Auto-size down if it overflows the hero zone.
	var primary, secondary string
	var primaryStart float64
	if data.IsCircuit() {
		primary = strings.ToUpper(data.ColorLabel())
		secondary = "CIRCUIT"
		primaryStart = 360 // color names run wider than V-grades
	} else {
		primary = data.Route.Grade
		secondary = strings.ToUpper(data.ColorLabel())
		primaryStart = 440
	}
	heroMax := float64(heroW) - 160
	primaryPt := fitWidth(dc, fontBold, primary, heroMax, primaryStart, 144)
	setFont(dc, fontBold, primaryPt)
	pw, _ := dc.MeasureString(primary)
	// Use cap-height (~0.72×em) to center visually; gg's MeasureString
	// returns em-advance height which over-pads the block.
	primaryCap := primaryPt * 0.72
	centerY := float64(digitalH) / 2
	primaryBaseline := centerY + primaryCap*0.35
	dc.SetColor(onHero)
	dc.DrawString(primary, (float64(heroW)-pw)/2, primaryBaseline)

	// Secondary label sits below the primary baseline, letter-spaced so it
	// reads as a small-caps subtitle.
	setFont(dc, fontBold, 40)
	spaced := letterSpace(secondary, 2)
	sw, _ := dc.MeasureString(spaced)
	dc.SetColor(withAlpha(onHero, 200))
	dc.DrawString(spaced, (float64(heroW)-sw)/2, primaryBaseline+96)

	// ---- Info panel content ----
	textX := float64(heroW + infoPadLeft)
	textRight := float64(digitalW - infoPadRight)
	textMaxW := textRight - textX
	y := float64(infoPadTopY)

	// No eyebrow — the hero already communicates what kind of route this is
	// (a grade for graded, a color name + CIRCUIT for circuits). The eyebrow
	// above the title added little beyond visual noise.

	// H1 — route name, up to two lines.
	if data.Route.Name != nil && *data.Route.Name != "" {
		setFont(dc, fontBold, 112)
		dc.SetColor(color.RGBA{246, 242, 236, 255})
		lines := wrapString(dc, *data.Route.Name, textMaxW, 2)
		for _, line := range lines {
			y += 116
			dc.DrawString(line, textX, y)
		}
		y += 40
	}

	// Location line — wall · gym.
	setFont(dc, fontRegular, 44)
	dc.SetColor(color.RGBA{188, 180, 170, 255})
	loc := data.WallName
	if data.LocationName != "" {
		if loc != "" {
			loc += "  ·  " + data.LocationName
		} else {
			loc = data.LocationName
		}
	}
	if loc != "" {
		y += 24
		dc.DrawString(truncateText(dc, loc, textMaxW), textX, y)
	}

	// Setter + date line.
	if data.SetterName != "" {
		y += 56
		setFont(dc, fontRegular, 36)
		dc.SetColor(color.RGBA{130, 124, 116, 255})
		dc.DrawString("Set by "+data.SetterName+"  ·  "+data.Route.DateSet.Format("Jan 2, 2006"), textX, y)
	}

	// Thin rule across the panel, separating identity from tags/stats.
	ruleY := float64(digitalH - 392)
	dc.SetColor(color.RGBA{60, 54, 48, 255})
	dc.SetLineWidth(2)
	dc.DrawLine(textX, ruleY, textRight, ruleY)
	dc.Stroke()

	// Tag pills — just below the rule.
	if len(data.Route.Tags) > 0 {
		tagY := ruleY + 72
		tagX := textX
		setFont(dc, fontBold, 28)
		for _, tag := range data.Route.Tags {
			name := tag.Name
			tw, _ := dc.MeasureString(name)
			pillW := tw + 48
			if tagX+pillW > textRight {
				break
			}
			dc.SetColor(color.RGBA{38, 34, 30, 255})
			drawPill(dc, tagX, tagY-32, pillW, 52, 26)
			dc.Fill()
			dc.SetColor(color.RGBA{220, 212, 202, 255})
			setFont(dc, fontBold, 28)
			dc.DrawString(name, tagX+24, tagY+6)
			tagX += pillW + 16
		}
	}

	// Stats row — always pinned to a fixed distance from the bottom so the
	// footer has a predictable clearance.
	statsY := float64(digitalH - 144)
	stats := buildStatPairs(data)
	if len(stats) > 0 {
		statX := textX
		for i, s := range stats {
			setFont(dc, fontBold, 68)
			dc.SetColor(color.RGBA{246, 242, 236, 255})
			dc.DrawString(s.value, statX, statsY)
			vw, _ := dc.MeasureString(s.value)

			setFont(dc, fontRegular, 28)
			dc.SetColor(color.RGBA{140, 132, 122, 255})
			dc.DrawString(s.label, statX+vw+16, statsY-4)
			lw, _ := dc.MeasureString(s.label)

			statX += vw + lw + 48
			if i < len(stats)-1 {
				dc.SetColor(color.RGBA{90, 82, 74, 255})
				setFont(dc, fontRegular, 56)
				dc.DrawString("·", statX-16, statsY-8)
			}
		}
	}

	// Footer — brand left, URL right, muted tones.
	footY := float64(digitalH - 56)
	setFont(dc, fontBold, 24)
	dc.SetColor(color.RGBA{120, 112, 102, 255})
	dc.DrawString(letterSpace("ROUTEWERK", 3), textX, footY)

	if data.QRTargetURL != "" {
		setFont(dc, fontRegular, 22)
		dc.SetColor(color.RGBA{100, 92, 84, 255})
		dc.DrawStringAnchored(
			truncateText(dc, data.QRTargetURL, textMaxW*0.6),
			textRight, footY-2, 1.0, 0.5,
		)
	}

	return encodePNG(dc)
}

// ============================================================
// Shared rendering components
// ============================================================

type statPair struct{ label, value string }

func buildStatPairs(data CardData) []statPair {
	stats := []statPair{}
	stats = append(stats, statPair{"sends", fmt.Sprintf("%d", data.Route.AscentCount)})
	if data.Route.RatingCount > 0 {
		stats = append(stats, statPair{"rating", fmt.Sprintf("%.1f", data.Route.AvgRating)})
	}
	if data.Route.AttemptCount > 0 {
		stats = append(stats, statPair{"attempts", fmt.Sprintf("%d", data.Route.AttemptCount)})
	}
	return stats
}

func drawDigitalStats(dc *gg.Context, data CardData, textX float64) {
	statsY := float64(digitalH) - 56
	stats := buildStatPairs(data)

	statX := textX
	for i, s := range stats {
		setFont(dc, fontBold, 20)
		dc.SetColor(color.RGBA{255, 255, 255, 255})
		dc.DrawString(s.value, statX, statsY)
		vw, _ := dc.MeasureString(s.value)

		setFont(dc, fontRegular, 10)
		dc.SetColor(color.RGBA{120, 115, 110, 255})
		dc.DrawString(s.label, statX+vw+5, statsY)
		lw, _ := dc.MeasureString(s.label)

		statX += vw + lw + 10
		if i < len(stats)-1 {
			dc.SetColor(color.RGBA{80, 77, 74, 255})
			setFont(dc, fontRegular, 10)
			dc.DrawString("·", statX, statsY)
			statX += 14
		}
	}
}

func drawDigitalTags(dc *gg.Context, data CardData, textX float64) {
	if len(data.Route.Tags) == 0 {
		return
	}
	tagY := float64(digitalH) - 80
	tagX := textX
	for _, tag := range data.Route.Tags {
		setFont(dc, fontBold, 10)
		tw, _ := dc.MeasureString(tag.Name)
		pillW := tw + 14

		// Light pill on dark bg
		dc.SetColor(color.RGBA{255, 255, 255, 25})
		drawPill(dc, tagX, tagY-10, pillW, 20, 10)
		dc.Fill()

		dc.SetColor(color.RGBA{220, 215, 210, 255})
		setFont(dc, fontBold, 10)
		dc.DrawString(tag.Name, tagX+7, tagY+3)

		tagX += pillW + 6
		if tagX > float64(digitalW)-80 {
			break
		}
	}
}

func truncateText(dc *gg.Context, text string, maxWidth float64) string {
	w, _ := dc.MeasureString(text)
	if w <= maxWidth {
		return text
	}
	for i := len(text) - 1; i > 0; i-- {
		candidate := text[:i] + "…"
		cw, _ := dc.MeasureString(candidate)
		if cw <= maxWidth {
			return candidate
		}
	}
	return text
}

// fitWidth returns the largest point size ≤ startPt (stepping in 1pt
// increments) at which the given text measures ≤ maxWidth under font f.
// Used to auto-shrink the hero identifier so long color names don't overflow.
func fitWidth(dc *gg.Context, f *truetype.Font, text string, maxWidth, startPt, minPt float64) float64 {
	pt := startPt
	for pt >= minPt {
		setFont(dc, f, pt)
		w, _ := dc.MeasureString(text)
		if w <= maxWidth {
			return pt
		}
		pt -= 1
	}
	return minPt
}

// letterSpace inserts spaces between runes so gg renders the text with
// pseudo-tracking. gg has no tracking API, so this is the simplest way to get
// the small-caps eyebrow look without shipping a glyph-layout pass.
func letterSpace(text string, gap int) string {
	if gap <= 0 || text == "" {
		return text
	}
	runes := []rune(text)
	var b strings.Builder
	b.Grow(len(text) * (gap + 1))
	for i, r := range runes {
		b.WriteRune(r)
		if i < len(runes)-1 {
			for j := 0; j < gap; j++ {
				b.WriteByte(' ')
			}
		}
	}
	return b.String()
}

// wrapString greedy-wraps text to at most maxLines lines whose widths do not
// exceed maxWidth under the currently-set font. The final line is ellipsis-
// truncated if overflow remains after the line budget is exhausted.
func wrapString(dc *gg.Context, text string, maxWidth float64, maxLines int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	cur := ""
	for _, w := range words {
		trial := w
		if cur != "" {
			trial = cur + " " + w
		}
		tw, _ := dc.MeasureString(trial)
		if tw <= maxWidth {
			cur = trial
			continue
		}
		if cur != "" {
			lines = append(lines, cur)
		}
		cur = w
		if len(lines) >= maxLines-1 {
			break
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	if n := len(lines); n > 0 {
		lw, _ := dc.MeasureString(lines[n-1])
		if lw > maxWidth {
			lines[n-1] = truncateText(dc, lines[n-1], maxWidth)
		}
	}
	return lines
}

// ============================================================
// PDF wrapper
// ============================================================

type pngFunc func(CardData) ([]byte, error)

func (g *CardGenerator) wrapPDF(data CardData, genPNG pngFunc, w, h int) ([]byte, error) {
	pngBytes, err := genPNG(data)
	if err != nil {
		return nil, err
	}

	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, fmt.Errorf("decode png for pdf: %w", err)
	}
	bounds := img.Bounds()
	imgW := float64(bounds.Dx())
	imgH := float64(bounds.Dy())

	dpi := 150.0
	pageW := imgW / dpi * 25.4
	pageH := imgH / dpi * 25.4

	pdf := gofpdf.NewCustom(&gofpdf.InitType{
		UnitStr: "mm",
		Size:    gofpdf.SizeType{Wd: pageW, Ht: pageH},
	})
	pdf.SetMargins(0, 0, 0)
	pdf.AddPage()

	pdf.RegisterImageOptionsReader("card", gofpdf.ImageOptions{ImageType: "PNG"}, bytes.NewReader(pngBytes))
	pdf.ImageOptions("card", 0, 0, pageW, pageH, false, gofpdf.ImageOptions{}, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("encode pdf: %w", err)
	}
	return buf.Bytes(), nil
}

// ============================================================
// Drawing primitives
// ============================================================

func encodePNG(dc *gg.Context) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, dc.Image()); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}


func drawPill(dc *gg.Context, x, y, w, h, r float64) {
	dc.NewSubPath()
	dc.DrawArc(x+r, y+r, r, math.Pi, 1.5*math.Pi)
	dc.LineTo(x+w-r, y)
	dc.DrawArc(x+w-r, y+r, r, 1.5*math.Pi, 2*math.Pi)
	dc.LineTo(x+w, y+h-r)
	dc.DrawArc(x+w-r, y+h-r, r, 0, 0.5*math.Pi)
	dc.LineTo(x+r, y+h)
	dc.DrawArc(x+r, y+h-r, r, 0.5*math.Pi, math.Pi)
	dc.ClosePath()
}

func generateQRImage(url string, size int) (image.Image, error) {
	qr, err := qrcode.New(url, qrcode.Medium)
	if err != nil {
		return nil, err
	}
	qr.DisableBorder = true
	return qr.Image(size), nil
}

// ============================================================
// Color utilities
// ============================================================

func parseHexColor(hex string) color.Color {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if len(hex) != 6 {
		return color.RGBA{100, 100, 100, 255}
	}
	var r, g, b uint8
	fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	return color.RGBA{r, g, b, 255}
}

func contrastColor(c color.Color) color.Color {
	r, g, b, _ := c.RGBA()
	lum := 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8)
	if lum > 140 {
		return color.RGBA{30, 30, 30, 255}
	}
	return color.RGBA{255, 255, 255, 255}
}

func withAlpha(c color.Color, a uint8) color.Color {
	r, g, b, _ := c.RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), a}
}

// hexToName maps common climbing hold hex colors to readable names.
// Falls back to a nearest-match hue description.
func hexToName(hex string) string {
	hex = strings.ToLower(strings.TrimPrefix(hex, "#"))

	// Exact matches for common gym hold colors
	known := map[string]string{
		"e53935": "Red", "f44336": "Red", "d32f2f": "Red", "ff0000": "Red",
		"c62828": "Dark Red", "b71c1c": "Dark Red",
		"ff9800": "Orange", "fb8c00": "Orange", "ef6c00": "Orange", "ff6d00": "Orange",
		"ffeb3b": "Yellow", "fdd835": "Yellow", "f9a825": "Yellow", "ffff00": "Yellow",
		"4caf50": "Green", "43a047": "Green", "2e7d32": "Green", "00ff00": "Green",
		"66bb6a": "Green", "388e3c": "Green",
		"2196f3": "Blue", "1e88e5": "Blue", "1565c0": "Blue", "0000ff": "Blue",
		"42a5f5": "Blue", "1976d2": "Blue",
		"9c27b0": "Purple", "8e24aa": "Purple", "7b1fa2": "Purple",
		"e91e63": "Pink", "d81b60": "Pink", "ec407a": "Pink", "ff69b4": "Pink",
		"000000": "Black", "212121": "Black", "333333": "Black",
		"ffffff": "White", "fafafa": "White", "f5f5f5": "White",
		"9e9e9e": "Gray", "757575": "Gray", "bdbdbd": "Gray",
		"795548": "Brown", "6d4c41": "Brown", "5d4037": "Brown",
		"00bcd4": "Teal", "0097a7": "Teal", "00838f": "Teal",
		"ff5722": "Burnt Orange", "e64a19": "Burnt Orange",
		"cddc39": "Lime", "c0ca33": "Lime",
	}

	if name, ok := known[hex]; ok {
		return name
	}

	// Fallback: classify by hue
	c := parseHexColor(hex)
	r, g, b, _ := c.RGBA()
	rf, gf, bf := float64(r>>8), float64(g>>8), float64(b>>8)

	// Check for near-grayscale
	maxC := math.Max(rf, math.Max(gf, bf))
	minC := math.Min(rf, math.Min(gf, bf))
	if maxC-minC < 30 {
		if maxC < 60 {
			return "Black"
		}
		if maxC > 200 {
			return "White"
		}
		return "Gray"
	}

	// Hue-based classification
	h := hue(rf, gf, bf)
	switch {
	case h < 15 || h >= 345:
		return "Red"
	case h < 45:
		return "Orange"
	case h < 70:
		return "Yellow"
	case h < 160:
		return "Green"
	case h < 200:
		return "Teal"
	case h < 260:
		return "Blue"
	case h < 300:
		return "Purple"
	default:
		return "Pink"
	}
}

func hue(r, g, b float64) float64 {
	maxC := math.Max(r, math.Max(g, b))
	minC := math.Min(r, math.Min(g, b))
	d := maxC - minC
	if d == 0 {
		return 0
	}
	var h float64
	switch maxC {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	case b:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h
}

func gradeSize(grade string) float64 {
	switch {
	case len(grade) <= 2:
		return 64
	case len(grade) <= 4:
		return 52
	default:
		return 42
	}
}

// titleCase capitalises the first letter of each word (replaces deprecated strings.Title).
func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func formatRouteType(rt string) string {
	switch rt {
	case "boulder":
		return "BOULDER"
	case "lead":
		return "LEAD"
	case "toprope", "top_rope":
		return "TOP ROPE"
	case "auto_belay":
		return "AUTO BELAY"
	default:
		return strings.ToUpper(rt)
	}
}
