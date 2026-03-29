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

	dc.SetColor(color.White)
	dc.Clear()

	// -- Color band across top --
	bandH := 48.0
	dc.SetColor(routeColor)
	roundedRectTop(dc, 0, 0, float64(printW), bandH, 10)
	dc.Fill()

	// -- Color name on the band (accessibility) --
	dc.SetColor(contrastColor(routeColor))
	setFont(dc, fontBold, 15)
	dc.DrawString(strings.ToUpper(data.colorLabel())+" HOLDS", 20, bandH-16)

	// -- Grade — massive, left-aligned --
	gradeY := bandH + 28
	dc.SetColor(darken(routeColor, 0.15))
	fontSize := gradeSize(data.Route.Grade)
	setFont(dc, fontBold, fontSize)
	dc.DrawString(data.Route.Grade, 28, gradeY+fontSize*0.8)

	gw, _ := dc.MeasureString(data.Route.Grade)
	textX := 28 + gw + 18
	if textX < 140 {
		textX = 140
	}

	// -- Route name --
	infoY := bandH + 44
	if data.Route.Name != nil && *data.Route.Name != "" {
		dc.SetColor(color.RGBA{30, 30, 30, 255})
		setFont(dc, fontBold, 22)
		dc.DrawString(*data.Route.Name, textX, infoY)
		infoY += 28
	}

	// -- Wall name --
	dc.SetColor(color.RGBA{90, 90, 90, 255})
	setFont(dc, fontRegular, 17)
	dc.DrawString(data.WallName, textX, infoY)
	infoY += 24

	// -- Route type badge --
	if data.Route.RouteType != "" {
		typeLabel := formatRouteType(data.Route.RouteType)
		setFont(dc, fontRegular, 13)
		tw, _ := dc.MeasureString(typeLabel)
		dc.SetColor(color.RGBA{235, 235, 235, 255})
		drawPill(dc, textX, infoY-12, tw+16, 22, 11)
		dc.Fill()
		dc.SetColor(color.RGBA{90, 90, 90, 255})
		setFont(dc, fontRegular, 13)
		dc.DrawString(typeLabel, textX+8, infoY+3)
	}

	// -- QR code --
	drawPrintFooter(dc, data, 100)

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
	textOnColor := contrastColor(routeColor)

	dc.SetColor(color.White)
	dc.Clear()

	// -- Color hero fills top ~60% --
	heroH := 180.0
	dc.SetColor(routeColor)
	roundedRectTop(dc, 0, 0, float64(printW), heroH, 10)
	dc.Fill()

	// -- Circuit color name — the primary identifier --
	circuitLabel := strings.ToUpper(data.colorLabel()) + " CIRCUIT"
	dc.SetColor(textOnColor)
	setFont(dc, fontBold, 38)
	dc.DrawString(circuitLabel, 28, 64)

	// -- Route name (if set) --
	infoY := 98.0
	if data.Route.Name != nil && *data.Route.Name != "" {
		dc.SetColor(withAlpha(textOnColor, 220))
		setFont(dc, fontBold, 20)
		dc.DrawString(*data.Route.Name, 28, infoY)
		infoY += 28
	}

	// -- Wall name --
	dc.SetColor(withAlpha(textOnColor, 190))
	setFont(dc, fontRegular, 17)
	dc.DrawString(data.WallName, 28, infoY)
	infoY += 24

	// -- Route type + grade (if present, shown small) --
	dc.SetColor(withAlpha(textOnColor, 160))
	setFont(dc, fontRegular, 14)
	meta := formatRouteType(data.Route.RouteType)
	if data.Route.Grade != "" {
		meta += "  •  " + data.Route.Grade
	}
	dc.DrawString(meta, 28, infoY)

	// -- QR code + footer --
	drawPrintFooter(dc, data, 90)

	return encodePNG(dc)
}

// drawPrintFooter renders the QR code, setter, date, and branding
// shared by both graded and circuit print cards.
func drawPrintFooter(dc *gg.Context, data CardData, qrSize int) {
	// QR code — bottom right
	qrImg, err := generateQRImage(data.QRTargetURL, qrSize)
	if err == nil {
		qrX := printW - qrSize - 20
		qrY := printH - qrSize - 36
		dc.DrawImage(qrImg, qrX, qrY)

		dc.SetColor(color.RGBA{140, 140, 140, 255})
		setFont(dc, fontRegular, 9)
		dc.DrawStringAnchored("Scan to log climb", float64(qrX)+float64(qrSize)/2, float64(qrY+qrSize)+12, 0.5, 0.5)
	}

	// Setter + date — bottom left
	footerY := float64(printH) - 52
	if data.SetterName != "" {
		dc.SetColor(color.RGBA{110, 110, 110, 255})
		setFont(dc, fontRegular, 13)
		dc.DrawString("Set by "+data.SetterName, 28, footerY)
		footerY += 18
	}
	dc.SetColor(color.RGBA{150, 150, 150, 255})
	setFont(dc, fontRegular, 12)
	dc.DrawString(data.Route.DateSet.Format("Jan 2, 2006"), 28, footerY)

	// Branding
	dc.SetColor(color.RGBA{200, 200, 200, 255})
	setFont(dc, fontRegular, 9)
	dc.DrawString("routewerk", 28, float64(printH)-12)

	// Border
	dc.SetColor(color.RGBA{210, 210, 210, 255})
	dc.SetLineWidth(1)
	dc.DrawRoundedRectangle(0.5, 0.5, float64(printW)-1, float64(printH)-1, 10)
	dc.Stroke()
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

	dc.SetColor(color.White)
	dc.Clear()

	// -- Color hero section (top ~47%) --
	heroH := 170.0
	dc.SetColor(routeColor)
	dc.DrawRectangle(0, 0, float64(digitalW), heroH)
	dc.Fill()
	drawGradientBottom(dc, heroH, float64(digitalW))

	textOnColor := contrastColor(routeColor)

	// -- Color name label (accessibility) — top-left --
	dc.SetColor(withAlpha(textOnColor, 160))
	setFont(dc, fontBold, 10)
	dc.DrawString(strings.ToUpper(data.colorLabel())+" HOLDS", 36, 24)

	// -- Grade — large, left side --
	dc.SetColor(textOnColor)
	setFont(dc, fontBold, 52)
	dc.DrawString(data.Route.Grade, 36, 88)

	gw, _ := dc.MeasureString(data.Route.Grade)
	labelX := 36 + gw + 24

	// -- Route name --
	nameY := 62.0
	if data.Route.Name != nil && *data.Route.Name != "" {
		dc.SetColor(textOnColor)
		setFont(dc, fontBold, 22)
		dc.DrawString(*data.Route.Name, labelX, nameY)
		nameY += 28
	}

	// -- Wall + Location --
	dc.SetColor(withAlpha(textOnColor, 200))
	setFont(dc, fontRegular, 14)
	loc := data.WallName
	if data.LocationName != "" {
		loc = data.WallName + "  •  " + data.LocationName
	}
	dc.DrawString(loc, labelX, nameY)
	nameY += 22

	// -- Setter + date --
	if data.SetterName != "" {
		dc.SetColor(withAlpha(textOnColor, 170))
		setFont(dc, fontRegular, 12)
		dc.DrawString("Set by "+data.SetterName+"  •  "+data.Route.DateSet.Format("Jan 2, 2006"), labelX, nameY)
	}

	// -- Route type pill --
	drawTypePill(dc, data.Route.RouteType, textOnColor)

	// -- Stats + tags + footer (shared) --
	drawDigitalBottom(dc, data, routeColor, heroH)

	return encodePNG(dc)
}

func (g *CardGenerator) generateCircuitDigitalPNG(data CardData) ([]byte, error) {
	dc := gg.NewContext(digitalW, digitalH)
	routeColor := parseHexColor(data.Route.Color)

	dc.SetColor(color.White)
	dc.Clear()

	// -- Color hero — taller for circuit since color is the identity --
	heroH := 180.0
	dc.SetColor(routeColor)
	dc.DrawRectangle(0, 0, float64(digitalW), heroH)
	dc.Fill()
	drawGradientBottom(dc, heroH, float64(digitalW))

	textOnColor := contrastColor(routeColor)

	// -- Circuit color name — the primary identifier, huge --
	circuitLabel := strings.ToUpper(data.colorLabel()) + " CIRCUIT"
	dc.SetColor(textOnColor)
	setFont(dc, fontBold, 36)
	dc.DrawString(circuitLabel, 36, 64)

	// -- Route name --
	nameY := 92.0
	if data.Route.Name != nil && *data.Route.Name != "" {
		dc.SetColor(withAlpha(textOnColor, 220))
		setFont(dc, fontBold, 18)
		dc.DrawString(*data.Route.Name, 36, nameY)
		nameY += 26
	}

	// -- Wall + Location --
	dc.SetColor(withAlpha(textOnColor, 190))
	setFont(dc, fontRegular, 14)
	loc := data.WallName
	if data.LocationName != "" {
		loc = data.WallName + "  •  " + data.LocationName
	}
	dc.DrawString(loc, 36, nameY)
	nameY += 22

	// -- Setter + grade (if present, secondary) --
	meta := ""
	if data.SetterName != "" {
		meta = "Set by " + data.SetterName
	}
	if data.Route.Grade != "" {
		if meta != "" {
			meta += "  •  "
		}
		meta += data.Route.Grade
	}
	if meta != "" {
		dc.SetColor(withAlpha(textOnColor, 160))
		setFont(dc, fontRegular, 12)
		dc.DrawString(meta, 36, nameY)
	}

	// -- Route type pill --
	drawTypePill(dc, data.Route.RouteType, textOnColor)

	// -- Stats + tags + footer --
	drawDigitalBottom(dc, data, routeColor, heroH)

	return encodePNG(dc)
}

// ============================================================
// Shared rendering components
// ============================================================

func drawGradientBottom(dc *gg.Context, heroH, w float64) {
	for y := heroH - 40; y < heroH; y++ {
		alpha := uint8((y - (heroH - 40)) / 40 * 50)
		dc.SetColor(color.RGBA{0, 0, 0, alpha})
		dc.DrawLine(0, y, w, y)
		dc.Stroke()
	}
}

func drawTypePill(dc *gg.Context, routeType string, textOnColor color.Color) {
	if routeType == "" {
		return
	}
	typeLabel := formatRouteType(routeType)
	setFont(dc, fontBold, 10)
	tw, _ := dc.MeasureString(typeLabel)
	pillX := float64(digitalW) - tw - 54
	pillY := 22.0
	dc.SetColor(color.RGBA{255, 255, 255, 40})
	drawPill(dc, pillX, pillY, tw+20, 22, 11)
	dc.Fill()
	dc.SetColor(withAlpha(textOnColor, 220))
	setFont(dc, fontBold, 10)
	dc.DrawString(typeLabel, pillX+10, pillY+15)
}

func drawDigitalBottom(dc *gg.Context, data CardData, routeColor color.Color, heroH float64) {
	// -- Stats row --
	statsY := heroH + 36

	stats := []struct{ label, value string }{}
	stats = append(stats, struct{ label, value string }{"ascents", fmt.Sprintf("%d", data.Route.AscentCount)})
	if data.Route.RatingCount > 0 {
		stats = append(stats, struct{ label, value string }{"avg rating", fmt.Sprintf("%.1f", data.Route.AvgRating)})
		stats = append(stats, struct{ label, value string }{"ratings", fmt.Sprintf("%d", data.Route.RatingCount)})
	}
	if data.Route.AttemptCount > 0 {
		stats = append(stats, struct{ label, value string }{"attempts", fmt.Sprintf("%d", data.Route.AttemptCount)})
	}

	statX := 36.0
	for i, s := range stats {
		setFont(dc, fontBold, 20)
		dc.SetColor(color.RGBA{40, 40, 40, 255})
		dc.DrawString(s.value, statX, statsY)
		vw, _ := dc.MeasureString(s.value)

		setFont(dc, fontRegular, 11)
		dc.SetColor(color.RGBA{140, 140, 140, 255})
		dc.DrawString(s.label, statX+vw+6, statsY)
		lw, _ := dc.MeasureString(s.label)

		statX += vw + lw + 12
		if i < len(stats)-1 {
			dc.SetColor(color.RGBA{200, 200, 200, 255})
			setFont(dc, fontRegular, 11)
			dc.DrawString("·", statX, statsY)
			statX += 16
		}
	}

	// -- Tags --
	if len(data.Route.Tags) > 0 {
		tagY := statsY + 36
		tagX := 36.0
		for _, tag := range data.Route.Tags {
			setFont(dc, fontRegular, 11)
			tw, _ := dc.MeasureString(tag.Name)
			pillW := tw + 16

			// White pill background
			dc.SetColor(color.White)
			drawPill(dc, tagX, tagY-11, pillW, 22, 11)
			dc.Fill()

			// Subtle border
			dc.SetColor(color.RGBA{200, 200, 200, 255})
			dc.SetLineWidth(1)
			drawPill(dc, tagX, tagY-11, pillW, 22, 11)
			dc.Stroke()

			// Dark text for readability
			dc.SetColor(color.RGBA{60, 60, 60, 255})
			setFont(dc, fontRegular, 11)
			dc.DrawString(tag.Name, tagX+8, tagY+4)

			tagX += pillW + 8
			if tagX > float64(digitalW)-40 {
				break
			}
		}
	}

	// -- QR code --
	qrImg, err := generateQRImage(data.QRTargetURL, 64)
	if err == nil {
		dc.DrawImage(qrImg, digitalW-84, digitalH-84)
	}

	// -- Branding --
	dc.SetColor(color.RGBA{190, 190, 190, 255})
	setFont(dc, fontRegular, 9)
	dc.DrawString("routewerk", 36, float64(digitalH)-16)

	// -- Border --
	dc.SetColor(color.RGBA{220, 220, 220, 255})
	dc.SetLineWidth(1)
	dc.DrawRoundedRectangle(0.5, 0.5, float64(digitalW)-1, float64(digitalH)-1, 8)
	dc.Stroke()
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

func roundedRectTop(dc *gg.Context, x, y, w, h, r float64) {
	dc.NewSubPath()
	dc.DrawArc(x+r, y+r, r, math.Pi, 1.5*math.Pi)
	dc.LineTo(x+w-r, y)
	dc.DrawArc(x+w-r, y+r, r, 1.5*math.Pi, 2*math.Pi)
	dc.LineTo(x+w, y+h)
	dc.LineTo(x, y+h)
	dc.ClosePath()
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

func darken(c color.Color, amount float64) color.Color {
	r, g, b, _ := c.RGBA()
	factor := 1.0 - amount
	return color.RGBA{
		uint8(float64(r>>8) * factor),
		uint8(float64(g>>8) * factor),
		uint8(float64(b>>8) * factor),
		255,
	}
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
