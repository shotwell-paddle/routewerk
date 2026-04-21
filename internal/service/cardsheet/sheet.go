// Package cardsheet composes one or more portrait card faces produced by
// service.CardGenerator.GenerateSheetCardPNG into letter-sized, 8-up
// print-and-cut PDF sheets suitable for a Silhouette Cameo 5 / Pro Mk II.
//
// Each page carries:
//   - Silhouette Type 2 registration marks (L top-left, 5mm squares at
//     top-right and bottom-left, 10mm inset from the page edges).
//   - Up to 8 cards in a 2×4 grid. Cards are stored portrait (2" × 3.5")
//     by the renderer and rotated to landscape for the sheet so a single
//     rotation on the cutter is all that's needed.
//   - Pure-RGB-red hairline cut-path rectangles drawn at each card's edge.
//     Silhouette Studio's "Cut by Color" setting picks these up verbatim.
//
// Sheet geometry mirrors the Python reference in tmp/card-designs/gen_sheet.py;
// that script is what the first physical test cuts were tuned against.
package cardsheet

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"

	"github.com/jung-kurt/gofpdf/v2"

	"github.com/shotwell-paddle/routewerk/internal/service"
)

// CutterProfile selects the registration-mark + cut-path conventions for a
// specific cutter. Only Silhouette Type 2 is supported today; the type exists
// so future profiles can be added without changing callers.
type CutterProfile string

const (
	SilhouetteType2 CutterProfile = "silhouette_type2"
)

// Letter page size in millimetres. gofpdf runs in mm throughout this package
// so the numbers line up 1:1 with Silhouette Studio.
const (
	pageWMM = 215.9
	pageHMM = 279.4
)

// Silhouette Type 2 registration marks. See Silhouette "Cutting Mats &
// Registration Marks" reference — a Cameo 5 requires a ≥10mm clear zone
// around each mark, and the top-left L is what anchors page orientation.
const (
	markInsetMM  = 10.0 // mark origin distance from page edge
	markSquareMM = 5.0  // size of the two square marks
	lLegMM       = 19.0 // length of each leg of the top-left L
	lThickMM     = 0.5  // thickness of each leg of the top-left L
)

// Card grid. Cards are placed landscape-on-page; a 2×4 grid of 88.9×50.8mm
// cards fits inside the Cameo's cut window with room for reg marks.
const (
	cardWMM      = 88.9 // 3.5" long axis — on the sheet this is horizontal
	cardHMM      = 50.8 // 2.0" short axis
	gridCols     = 2
	gridRows     = 4
	cardsPerPage = gridCols * gridRows
	gridXMM      = 19.05 // leaves ~9mm clear zone right of the top-left L
	gridYMM      = 45.6  // leaves ~9mm clear zone below the top-left L
)

// Cut-path stroke. Must be pure RGB red (255,0,0) — Silhouette Studio's
// "Cut by Color" matches exactly on that value. The stroke is a hairline so
// it doesn't show on the printed card.
const (
	cutR        = 255
	cutG        = 0
	cutB        = 0
	cutStrokePT = 0.1

	// cardCornerRadiusMM rounds the four corners of every card's cut path.
	// Climbing-gym route tags live on plastic holds that are gripped,
	// brushed, and yanked dozens of times a day; sharp corners fray and
	// peel first. 4mm is pronounced enough to give meaningful durability
	// (the rounded arc distributes handling stress instead of
	// concentrating it at a point), reads clearly as "rounded" visually,
	// and is well within the Silhouette's min-radius tolerance. The cut
	// path is what the cutter physically follows — this is the durability
	// win, not a decorative outline.
	cardCornerRadiusMM = 4.0
)

const defaultBleedMM = 0.5

// SheetConfig controls sheet rendering. All fields are optional and the zero
// value is a valid Silhouette Type 2 sheet with default bleed.
type SheetConfig struct {
	// Cutter selects the registration-mark profile. Defaults to SilhouetteType2.
	Cutter CutterProfile

	// Theme is passed through to the card renderer (future use: swap Design D
	// for other variants). Empty means "whatever the renderer's default is".
	Theme string

	// Bleed is the distance (mm) the card background extends past the cut
	// path, hiding registration drift. Set to 0 to disable the bleed layer.
	// Defaults to defaultBleedMM.
	Bleed float64
}

// Composer renders slices of service.CardData to 8-up print-and-cut PDFs.
// It wraps *service.CardGenerator so sheet composition stays independent of
// per-card rendering — that split is what lets us theme cards without
// touching registration-mark / cut-path code.
type Composer struct {
	cards *service.CardGenerator
}

// NewComposer returns a Composer that uses c to render each card face.
func NewComposer(c *service.CardGenerator) *Composer {
	return &Composer{cards: c}
}

// Render writes one or more letter-sized, 8-up print-and-cut sheets to w.
// The PDF is paginated automatically for more than 8 cards. Card order on
// the sheet matches the input order, filled left→right then top→bottom.
//
// An empty data slice is treated as an error — there's no useful PDF to
// produce and callers (handlers, CLI) always want to surface that rather
// than silently emit a blank document.
func (c *Composer) Render(w io.Writer, data []service.CardData, cfg SheetConfig) error {
	if len(data) == 0 {
		return fmt.Errorf("cardsheet: no cards to render")
	}
	if cfg.Cutter == "" {
		cfg.Cutter = SilhouetteType2
	}
	if cfg.Cutter != SilhouetteType2 {
		return fmt.Errorf("cardsheet: unsupported cutter profile %q", cfg.Cutter)
	}
	if cfg.Bleed == 0 {
		cfg.Bleed = defaultBleedMM
	}

	pdf := gofpdf.NewCustom(&gofpdf.InitType{
		UnitStr: "mm",
		Size:    gofpdf.SizeType{Wd: pageWMM, Ht: pageHMM},
	})
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetTitle("Routewerk route cards", true)
	pdf.SetCreator("Routewerk", true)

	// Register the embedded Routewerk body font once. drawCardVector relies
	// on this family being available for every SetFont call — registering
	// per-card would balloon the PDF with duplicate font descriptors.
	registerCardFonts(pdf)

	total := len(data)
	for start := 0; start < total; start += cardsPerPage {
		pdf.AddPage()
		drawRegistrationMarks(pdf)

		end := start + cardsPerPage
		if end > total {
			end = total
		}
		for i := start; i < end; i++ {
			slot := i - start
			row := slot / gridCols
			col := slot % gridCols
			x := gridXMM + float64(col)*cardWMM
			y := gridYMM + float64(row)*cardHMM

			// Native vector draw — no PNG rasterization. Text stays sharp at
			// whatever DPI the target printer runs at. uniqueKey must be unique
			// within the whole PDF since it names the per-card QR image.
			drawCardVector(pdf, data[i], x, y, cardWMM, cardHMM, fmt.Sprintf("c%d", i))
			drawCutPath(pdf, x, y)
		}
	}

	if err := pdf.Output(w); err != nil {
		return fmt.Errorf("cardsheet: encode pdf: %w", err)
	}
	return nil
}

// PageCount returns the number of sheets Render will produce for n cards.
// Used by handlers that want to tell the user "this will be 3 sheets" before
// kicking off the render.
func PageCount(n int) int {
	if n <= 0 {
		return 0
	}
	return (n + cardsPerPage - 1) / cardsPerPage
}

// drawRegistrationMarks stamps the Silhouette Type 2 pattern on the current
// page: L-mark at top-left, 5mm squares at top-right and bottom-left. All
// marks are pure black — the Cameo's optical sensor calibrates on that.
func drawRegistrationMarks(pdf *gofpdf.Fpdf) {
	pdf.SetFillColor(0, 0, 0)

	// Top-left L: horizontal leg along the top edge + vertical leg down the
	// left edge. They meet at the inner corner (markInsetMM, markInsetMM).
	pdf.Rect(markInsetMM, markInsetMM, lLegMM, lThickMM, "F")
	pdf.Rect(markInsetMM, markInsetMM, lThickMM, lLegMM, "F")

	// Top-right 5×5 square.
	pdf.Rect(
		pageWMM-markInsetMM-markSquareMM,
		markInsetMM,
		markSquareMM, markSquareMM,
		"F",
	)

	// Bottom-left 5×5 square.
	pdf.Rect(
		markInsetMM,
		pageHMM-markInsetMM-markSquareMM,
		markSquareMM, markSquareMM,
		"F",
	)
}

// drawCutPath strokes a hairline pure-red rounded rectangle exactly at the
// card edge. Silhouette Studio's "Cut by Color" picks up RGB(255,0,0) as the
// cut geometry; anything off that value is ignored, so do not tweak the
// constants without re-testing on the cutter.
//
// Corners are rounded at cardCornerRadiusMM (see the constant for rationale).
// Because the cutter follows the drawn path, this produces physically
// rounded cards out of the Silhouette — no separate corner-round step.
func drawCutPath(pdf *gofpdf.Fpdf, x, y float64) {
	pdf.SetDrawColor(cutR, cutG, cutB)
	// gofpdf line width is in user units (mm here); convert from points.
	pdf.SetLineWidth(cutStrokePT * 25.4 / 72.0)
	// "1234" = round all four corners.
	pdf.RoundedRect(x, y, cardWMM, cardHMM, cardCornerRadiusMM, "1234", "D")
}

// rotatePNG90CCW decodes a PNG, rotates the pixel data 90° counter-clockwise,
// and re-encodes. We rotate pixel data rather than use gofpdf's transform
// stack because it keeps image placement math straightforward and avoids
// any chance of the cut-path rectangle landing on rotated coordinates.
func rotatePNG90CCW(pngBytes []byte) ([]byte, error) {
	src, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, fmt.Errorf("decode png: %w", err)
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// 90° CCW: source (x, y) → destination (y, w-1-x).
			dst.Set(y, w-1-x, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return nil, fmt.Errorf("encode rotated png: %w", err)
	}
	return out.Bytes(), nil
}
