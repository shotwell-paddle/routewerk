package service

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"github.com/jung-kurt/gofpdf/v2"
	qrcode "github.com/skip2/go-qrcode"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/shotwell-paddle/routewerk/internal/model"
)

// ============================================================
// Fonts
// ============================================================

var (
	fontRegular *truetype.Font
	fontBold    *truetype.Font
)

func init() {
	fontRegular = loadSystemFont("/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf")
	if fontRegular == nil {
		fontRegular, _ = truetype.Parse(goregular.TTF)
	}
	fontBold = loadSystemFont("/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf")
	if fontBold == nil {
		fontBold, _ = truetype.Parse(gobold.TTF)
	}
}

func loadSystemFont(path string) *truetype.Font {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	f, err := truetype.Parse(data)
	if err != nil {
		return nil
	}
	return f
}

func setFont(dc *gg.Context, font *truetype.Font, size float64) {
	face := truetype.NewFace(font, &truetype.Options{Size: size, DPI: 72})
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

// isCircuit returns true if this route uses a circuit/color-based system
// rather than a numeric grade.
func (d CardData) isCircuit() bool {
	if d.Route.CircuitColor != nil && *d.Route.CircuitColor != "" {
		return true
	}
	return d.Route.GradingSystem == "circuit"
}

// colorLabel returns a human-readable color name for accessibility.
// Uses CircuitColor if set, otherwise derives from the hex Color field.
func (d CardData) colorLabel() string {
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
	if data.isCircuit() {
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
	colorLabel := strings.ToUpper(data.colorLabel())
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
	circuitLabel := strings.ToUpper(data.colorLabel())
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
// DIGITAL CARD — shareable, landscape 640×360
//
// Auto-selects graded vs circuit layout. Both include live stats
// and tags since digital cards are generated on demand.
// ============================================================

const (
	digitalW = 640
	digitalH = 360
)

func (g *CardGenerator) GenerateDigitalPNG(data CardData) ([]byte, error) {
	if data.isCircuit() {
		return g.generateCircuitDigitalPNG(data)
	}
	return g.generateGradedDigitalPNG(data)
}

func (g *CardGenerator) GenerateDigitalPDF(data CardData) ([]byte, error) {
	return g.wrapPDF(data, g.GenerateDigitalPNG, digitalW, digitalH)
}

func (g *CardGenerator) generateGradedDigitalPNG(data CardData) ([]byte, error) {
	dc := gg.NewContext(digitalW, digitalH)
	routeColor := parseHexColor(data.Route.Color)

	// Dark background
	bgColor := color.RGBA{20, 20, 18, 255}
	dc.SetColor(bgColor)
	dc.Clear()

	// -- Left color accent block --
	blockW := 120.0
	blockH := float64(digitalH) - 80
	dc.SetColor(routeColor)
	dc.DrawRoundedRectangle(28, 28, blockW, blockH, 16)
	dc.Fill()

	// -- Grade inside color block --
	textOnColor := contrastColor(routeColor)
	gradeText := data.Route.Grade
	setFont(dc, fontBold, 48)
	gw, gh := dc.MeasureString(gradeText)
	dc.SetColor(textOnColor)
	dc.DrawString(gradeText, 28+(blockW-gw)/2, 28+(blockH/2)-(gh/2)+gh*0.3)

	// -- Color name below grade --
	dc.SetColor(withAlpha(textOnColor, 160))
	setFont(dc, fontBold, 10)
	colorLabel := strings.ToUpper(data.colorLabel())
	clw, _ := dc.MeasureString(colorLabel)
	dc.DrawString(colorLabel, 28+(blockW-clw)/2, 28+blockH-16)

	// -- Route info (right side) --
	textX := 172.0
	infoY := 52.0

	// Route type label
	if data.Route.RouteType != "" {
		dc.SetColor(color.RGBA{252, 82, 0, 255}) // accent orange
		setFont(dc, fontBold, 10)
		dc.DrawString(formatRouteType(data.Route.RouteType), textX, infoY)
		infoY += 22
	}

	// Route name
	if data.Route.Name != nil && *data.Route.Name != "" {
		dc.SetColor(color.RGBA{255, 255, 255, 255})
		setFont(dc, fontBold, 24)
		dc.DrawString(truncateText(dc, *data.Route.Name, float64(digitalW)-textX-30), textX, infoY)
		infoY += 32
	}

	// Wall + Location
	dc.SetColor(color.RGBA{180, 175, 170, 255})
	setFont(dc, fontRegular, 14)
	loc := data.WallName
	if data.LocationName != "" {
		loc = data.WallName + "  ·  " + data.LocationName
	}
	dc.DrawString(loc, textX, infoY)
	infoY += 22

	// Setter + date
	if data.SetterName != "" {
		dc.SetColor(color.RGBA{120, 115, 110, 255})
		setFont(dc, fontRegular, 12)
		dc.DrawString("Set by "+data.SetterName+"  ·  "+data.Route.DateSet.Format("Jan 2, 2006"), textX, infoY)
	}

	// -- Stats row --
	drawDigitalStats(dc, data, textX)

	// -- Tags --
	drawDigitalTags(dc, data, textX)

	// -- Route link (bottom right, replacing QR — QR is useless on phones) --
	if data.QRTargetURL != "" {
		dc.SetColor(color.RGBA{90, 86, 82, 255})
		setFont(dc, fontRegular, 9)
		dc.DrawStringAnchored(data.QRTargetURL, float64(digitalW)-20, float64(digitalH)-16, 1.0, 0.5)
	}

	// -- Branding --
	dc.SetColor(color.RGBA{70, 67, 64, 255})
	setFont(dc, fontBold, 9)
	dc.DrawString("ROUTEWERK", 28, float64(digitalH)-14)

	return encodePNG(dc)
}

func (g *CardGenerator) generateCircuitDigitalPNG(data CardData) ([]byte, error) {
	dc := gg.NewContext(digitalW, digitalH)
	routeColor := parseHexColor(data.Route.Color)

	// Full route color background — color IS the identity
	dc.SetColor(routeColor)
	dc.Clear()

	// Gradient overlay at bottom for depth + text readability
	for y := float64(digitalH) - 120; y < float64(digitalH); y++ {
		alpha := uint8((y - (float64(digitalH) - 120)) / 120 * 100)
		dc.SetColor(color.RGBA{0, 0, 0, alpha})
		dc.DrawLine(0, y, float64(digitalW), y)
		dc.Stroke()
	}

	textOnColor := contrastColor(routeColor)

	// -- Circuit color name — huge, the primary identifier --
	circuitLabel := strings.ToUpper(data.colorLabel())
	dc.SetColor(textOnColor)
	setFont(dc, fontBold, 56)
	dc.DrawString(circuitLabel, 36, 80)

	// "CIRCUIT" subtitle
	dc.SetColor(withAlpha(textOnColor, 160))
	setFont(dc, fontBold, 14)
	dc.DrawString("CIRCUIT", 38, 104)

	// -- Route name --
	nameY := 140.0
	if data.Route.Name != nil && *data.Route.Name != "" {
		dc.SetColor(withAlpha(textOnColor, 240))
		setFont(dc, fontBold, 20)
		dc.DrawString(*data.Route.Name, 36, nameY)
		nameY += 28
	}

	// -- Wall + Location --
	dc.SetColor(withAlpha(textOnColor, 190))
	setFont(dc, fontRegular, 14)
	loc := data.WallName
	if data.LocationName != "" {
		loc = data.WallName + "  ·  " + data.LocationName
	}
	dc.DrawString(loc, 36, nameY)

	// -- Stats row (bottom left) --
	statsY := float64(digitalH) - 56
	stats := buildStatPairs(data)
	statX := 36.0
	for i, s := range stats {
		setFont(dc, fontBold, 18)
		dc.SetColor(withAlpha(textOnColor, 240))
		dc.DrawString(s.value, statX, statsY)
		vw, _ := dc.MeasureString(s.value)

		setFont(dc, fontRegular, 10)
		dc.SetColor(withAlpha(textOnColor, 140))
		dc.DrawString(s.label, statX+vw+5, statsY)
		lw, _ := dc.MeasureString(s.label)

		statX += vw + lw + 10
		if i < len(stats)-1 {
			dc.SetColor(withAlpha(textOnColor, 80))
			setFont(dc, fontRegular, 10)
			dc.DrawString("·", statX, statsY)
			statX += 14
		}
	}

	// -- Tags (bottom, above stats) --
	if len(data.Route.Tags) > 0 {
		tagY := float64(digitalH) - 80
		tagX := 36.0
		for _, tag := range data.Route.Tags {
			setFont(dc, fontBold, 10)
			tw, _ := dc.MeasureString(tag.Name)
			pillW := tw + 14

			// Dark semi-transparent pill
			dc.SetColor(color.RGBA{0, 0, 0, 80})
			drawPill(dc, tagX, tagY-10, pillW, 20, 10)
			dc.Fill()

			dc.SetColor(withAlpha(textOnColor, 220))
			setFont(dc, fontBold, 10)
			dc.DrawString(tag.Name, tagX+7, tagY+3)

			tagX += pillW + 6
			if tagX > float64(digitalW)-80 {
				break
			}
		}
	}

	// -- Route link (bottom right, replacing QR) --
	if data.QRTargetURL != "" {
		dc.SetColor(withAlpha(textOnColor, 80))
		setFont(dc, fontRegular, 9)
		dc.DrawStringAnchored(data.QRTargetURL, float64(digitalW)-20, float64(digitalH)-16, 1.0, 0.5)
	}

	// -- Branding --
	dc.SetColor(withAlpha(textOnColor, 60))
	setFont(dc, fontBold, 9)
	dc.DrawString("ROUTEWERK", 36, float64(digitalH)-14)

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
