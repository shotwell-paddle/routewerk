package cardsheet

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/jung-kurt/gofpdf/v2"

	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

// ────────────────────────────────────────────────────────────
// Test fixtures
// ────────────────────────────────────────────────────────────

func testCardData(idSuffix string) service.CardData {
	name := "Crimson Crush " + idSuffix
	setter := "setter-123"
	return service.CardData{
		Route: &model.Route{
			ID:            "route-" + idSuffix,
			LocationID:    "loc-123",
			WallID:        "wall-456",
			SetterID:      &setter,
			RouteType:     "boulder",
			Status:        "active",
			GradingSystem: "v_scale",
			Grade:         "V5",
			Name:          &name,
			Color:         "#e53935",
			DateSet:       time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
		},
		WallName:    "The Cave",
		SetterName:  "Chris S.",
		QRTargetURL: "https://app.routewerk.com/locations/loc-123/routes/route-" + idSuffix,
	}
}

func testCards(n int) []service.CardData {
	out := make([]service.CardData, n)
	for i := 0; i < n; i++ {
		out[i] = testCardData(string(rune('a' + i)))
	}
	return out
}

// ────────────────────────────────────────────────────────────
// Pure helpers
// ────────────────────────────────────────────────────────────

func TestPageCount(t *testing.T) {
	tests := []struct {
		in, want int
	}{
		{0, 0},
		{-1, 0},
		{1, 1},
		{7, 1},
		{8, 1},
		{9, 2},
		{16, 2},
		{17, 3},
		{80, 10},
	}
	for _, tc := range tests {
		if got := PageCount(tc.in); got != tc.want {
			t.Errorf("PageCount(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// (TestRotatePNG90CCW removed along with rotatePNG90CCW — the vector
// card-draw path never called it; rotation is handled natively by
// gofpdf's transform stack in drawCardVector.)

// ────────────────────────────────────────────────────────────
// Cut path color — must be pure RGB(255,0,0)
//
// Silhouette Studio's "Cut by Color" only matches exactly red, so if we
// ever regress to #fe0101 or similar, physical cuts silently stop working.
// Test by rendering into an uncompressed PDF and grepping the content
// stream for the exact color operator gofpdf emits for SetDrawColor(255,0,0).
// ────────────────────────────────────────────────────────────

func TestDrawCutPathIsPureRed(t *testing.T) {
	pdf := gofpdf.NewCustom(&gofpdf.InitType{
		UnitStr: "mm",
		Size:    gofpdf.SizeType{Wd: pageWMM, Ht: pageHMM},
	})
	pdf.SetMargins(0, 0, 0)
	pdf.SetCompression(false)
	pdf.AddPage()
	drawCutPath(pdf, 20, 50)

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("pdf output: %v", err)
	}

	// gofpdf emits stroke color as "R G B RG" with three decimal places for
	// each channel. Pure red → "1.000 0.000 0.000 RG".
	want := "1.000 0.000 0.000 RG"
	if !bytes.Contains(out.Bytes(), []byte(want)) {
		t.Errorf("cut path color operator %q not found in PDF", want)
	}
}

// TestRegistrationMarksArePureBlack mirrors the cut-path check for the
// registration mark fill — the Cameo's optical sensor is most reliable on
// solid black, and any lift toward gray will make it start losing marks.
//
// Two quirks of gofpdf drive the shape of this test:
//
//  1. SetFillColor calls that don't change the current color are suppressed.
//     Since PDF's default fill IS black, SetFillColor(0,0,0) on a fresh page
//     wouldn't emit anything. We dirty the fill to magenta first so the
//     function has to reset it.
//
//  2. When r == g == b, gofpdf collapses to the single-channel grayscale
//     operator: "0.000 g" rather than "0.000 0.000 0.000 rg". Assert on the
//     collapsed form — it's still pure black in the rendered output.
func TestRegistrationMarksArePureBlack(t *testing.T) {
	pdf := gofpdf.NewCustom(&gofpdf.InitType{
		UnitStr: "mm",
		Size:    gofpdf.SizeType{Wd: pageWMM, Ht: pageHMM},
	})
	pdf.SetMargins(0, 0, 0)
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFillColor(200, 50, 150) // anything non-black
	drawRegistrationMarks(pdf)

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("pdf output: %v", err)
	}

	want := "0.000 g"
	if !bytes.Contains(out.Bytes(), []byte(want)) {
		t.Errorf("reg-mark fill color operator %q not found in PDF", want)
	}
}

// ────────────────────────────────────────────────────────────
// End-to-end Render
// ────────────────────────────────────────────────────────────

func TestRender_EmptyReturnsError(t *testing.T) {
	c := NewComposer(service.NewCardGenerator("https://app.routewerk.com"))
	var buf bytes.Buffer
	err := c.Render(&buf, nil, SheetConfig{})
	if err == nil {
		t.Fatal("expected error for empty card list")
	}
	if !strings.Contains(err.Error(), "no cards") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRender_UnsupportedCutterReturnsError(t *testing.T) {
	c := NewComposer(service.NewCardGenerator("https://app.routewerk.com"))
	var buf bytes.Buffer
	err := c.Render(&buf, testCards(1), SheetConfig{Cutter: "acme_5000"})
	if err == nil {
		t.Fatal("expected error for unsupported cutter")
	}
}

func TestRender_ValidPDF(t *testing.T) {
	c := NewComposer(service.NewCardGenerator("https://app.routewerk.com"))
	var buf bytes.Buffer
	if err := c.Render(&buf, testCards(3), SheetConfig{}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("empty PDF output")
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")) {
		t.Errorf("not a valid PDF (missing %%PDF- header)")
	}
	t.Logf("3-card sheet: %d bytes", buf.Len())
}

// TestRender_Paginates renders 12 cards, which must span two sheets.
// gofpdf emits a /Count entry in the Pages object we can check, and also
// marks each page with "/Type /Page" — counting those is the simplest
// page-count assertion that doesn't require a full PDF parser.
func TestRender_Paginates(t *testing.T) {
	tests := []struct {
		cards     int
		wantPages int
	}{
		{1, 1},
		{8, 1},
		{9, 2},
		{12, 2},
		{17, 3},
	}
	for _, tc := range tests {
		c := NewComposer(service.NewCardGenerator("https://app.routewerk.com"))
		var buf bytes.Buffer
		if err := c.Render(&buf, testCards(tc.cards), SheetConfig{}); err != nil {
			t.Fatalf("%d cards: Render: %v", tc.cards, err)
		}
		// Count "/Type /Page" occurrences, subtracting the single
		// "/Type /Pages" entry that names the page tree root. This is
		// tolerant of whether gofpdf puts a space, newline, or nothing
		// immediately after "/Page".
		all := bytes.Count(buf.Bytes(), []byte("/Type /Page"))
		tree := bytes.Count(buf.Bytes(), []byte("/Type /Pages"))
		pages := all - tree
		if pages != tc.wantPages {
			t.Errorf("%d cards: got %d pages, want %d", tc.cards, pages, tc.wantPages)
		}
	}
}
