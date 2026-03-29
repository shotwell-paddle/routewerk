package service

import (
	"bytes"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/model"
)

func testCardData() CardData {
	name := "Crimson Crush"
	setter := "setter-123"
	return CardData{
		Route: &model.Route{
			ID:            "route-abc",
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
			AvgRating:     4.2,
			RatingCount:   18,
			AscentCount:   47,
			AttemptCount:  123,
			Tags: []model.Tag{
				{Name: "Crimpy"},
				{Name: "Overhang"},
				{Name: "Competition"},
			},
		},
		WallName:     "The Cave",
		LocationName: "LEF Boulder",
		SetterName:   "Chris S.",
		QRTargetURL:  "https://app.routewerk.com/locations/loc-123/routes/route-abc",
	}
}

func TestGeneratePrintPNG(t *testing.T) {
	gen := NewCardGenerator("https://app.routewerk.com")
	data := testCardData()

	pngBytes, err := gen.GeneratePrintPNG(data)
	if err != nil {
		t.Fatalf("GeneratePrintPNG failed: %v", err)
	}
	if len(pngBytes) == 0 {
		t.Fatal("empty output")
	}

	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("invalid PNG: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != printW || b.Dy() != printH {
		t.Errorf("print card: expected %dx%d, got %dx%d", printW, printH, b.Dx(), b.Dy())
	}
	t.Logf("Print PNG: %d bytes (%dx%d)", len(pngBytes), b.Dx(), b.Dy())
}

func TestGenerateDigitalPNG(t *testing.T) {
	gen := NewCardGenerator("https://app.routewerk.com")
	data := testCardData()

	pngBytes, err := gen.GenerateDigitalPNG(data)
	if err != nil {
		t.Fatalf("GenerateDigitalPNG failed: %v", err)
	}
	if len(pngBytes) == 0 {
		t.Fatal("empty output")
	}

	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("invalid PNG: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != digitalW || b.Dy() != digitalH {
		t.Errorf("digital card: expected %dx%d, got %dx%d", digitalW, digitalH, b.Dx(), b.Dy())
	}
	t.Logf("Digital PNG: %d bytes (%dx%d)", len(pngBytes), b.Dx(), b.Dy())
}

func TestGeneratePrintPDF(t *testing.T) {
	gen := NewCardGenerator("https://app.routewerk.com")
	pdfBytes, err := gen.GeneratePrintPDF(testCardData())
	if err != nil {
		t.Fatalf("GeneratePrintPDF failed: %v", err)
	}
	if !strings.HasPrefix(string(pdfBytes[:5]), "%PDF-") {
		t.Error("not a valid PDF")
	}
	t.Logf("Print PDF: %d bytes", len(pdfBytes))
}

func TestGenerateDigitalPDF(t *testing.T) {
	gen := NewCardGenerator("https://app.routewerk.com")
	pdfBytes, err := gen.GenerateDigitalPDF(testCardData())
	if err != nil {
		t.Fatalf("GenerateDigitalPDF failed: %v", err)
	}
	if !strings.HasPrefix(string(pdfBytes[:5]), "%PDF-") {
		t.Error("not a valid PDF")
	}
	t.Logf("Digital PDF: %d bytes", len(pdfBytes))
}

func TestRouteURL(t *testing.T) {
	gen := NewCardGenerator("https://app.routewerk.com/")
	url := gen.RouteURL("loc-123", "route-abc")
	expected := "https://app.routewerk.com/locations/loc-123/routes/route-abc"
	if url != expected {
		t.Errorf("RouteURL = %q, want %q", url, expected)
	}
}

func TestParseHexColor(t *testing.T) {
	tests := []string{"#e53935", "e53935", "#fff", "invalid", ""}
	for _, tc := range tests {
		c := parseHexColor(tc)
		if c == nil {
			t.Errorf("parseHexColor(%q) returned nil", tc)
		}
	}
}

func TestNoNameRoute(t *testing.T) {
	gen := NewCardGenerator("https://app.routewerk.com")
	data := testCardData()
	data.Route.Name = nil // unnamed route

	_, err := gen.GeneratePrintPNG(data)
	if err != nil {
		t.Fatalf("print with no name: %v", err)
	}
	_, err = gen.GenerateDigitalPNG(data)
	if err != nil {
		t.Fatalf("digital with no name: %v", err)
	}
}

func TestNoSetterRoute(t *testing.T) {
	gen := NewCardGenerator("https://app.routewerk.com")
	data := testCardData()
	data.Route.SetterID = nil
	data.SetterName = ""

	_, err := gen.GeneratePrintPNG(data)
	if err != nil {
		t.Fatalf("print with no setter: %v", err)
	}
	_, err = gen.GenerateDigitalPNG(data)
	if err != nil {
		t.Fatalf("digital with no setter: %v", err)
	}
}

// ── Circuit route tests ──────────────────────────────────────────

func testCircuitCardData() CardData {
	name := "The Slab Problem"
	setter := "setter-456"
	circuitColor := "Red"
	return CardData{
		Route: &model.Route{
			ID:            "route-circuit-1",
			LocationID:    "loc-123",
			WallID:        "wall-789",
			SetterID:      &setter,
			RouteType:     "boulder",
			Status:        "active",
			GradingSystem: "circuit",
			Grade:         "",
			CircuitColor:  &circuitColor,
			Name:          &name,
			Color:         "#e53935",
			DateSet:       time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
			AvgRating:     3.8,
			RatingCount:   12,
			AscentCount:   34,
			AttemptCount:  89,
			Tags: []model.Tag{
				{Name: "Slab"},
				{Name: "Balance"},
			},
		},
		WallName:     "The Slab Wall",
		LocationName: "LEF Boulder",
		SetterName:   "Alex M.",
		QRTargetURL:  "https://app.routewerk.com/locations/loc-123/routes/route-circuit-1",
	}
}

func TestCircuitPrintPNG(t *testing.T) {
	gen := NewCardGenerator("https://app.routewerk.com")
	data := testCircuitCardData()

	if !data.isCircuit() {
		t.Fatal("expected isCircuit() == true")
	}

	pngBytes, err := gen.GeneratePrintPNG(data)
	if err != nil {
		t.Fatalf("Circuit PrintPNG failed: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("invalid PNG: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != printW || b.Dy() != printH {
		t.Errorf("circuit print card: expected %dx%d, got %dx%d", printW, printH, b.Dx(), b.Dy())
	}
	t.Logf("Circuit Print PNG: %d bytes (%dx%d)", len(pngBytes), b.Dx(), b.Dy())
}

func TestCircuitDigitalPNG(t *testing.T) {
	gen := NewCardGenerator("https://app.routewerk.com")
	data := testCircuitCardData()

	pngBytes, err := gen.GenerateDigitalPNG(data)
	if err != nil {
		t.Fatalf("Circuit DigitalPNG failed: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("invalid PNG: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != digitalW || b.Dy() != digitalH {
		t.Errorf("circuit digital card: expected %dx%d, got %dx%d", digitalW, digitalH, b.Dx(), b.Dy())
	}
	t.Logf("Circuit Digital PNG: %d bytes (%dx%d)", len(pngBytes), b.Dx(), b.Dy())
}

func TestCircuitPrintPDF(t *testing.T) {
	gen := NewCardGenerator("https://app.routewerk.com")
	pdfBytes, err := gen.GeneratePrintPDF(testCircuitCardData())
	if err != nil {
		t.Fatalf("Circuit PrintPDF failed: %v", err)
	}
	if !strings.HasPrefix(string(pdfBytes[:5]), "%PDF-") {
		t.Error("not a valid PDF")
	}
	t.Logf("Circuit Print PDF: %d bytes", len(pdfBytes))
}

func TestCircuitDigitalPDF(t *testing.T) {
	gen := NewCardGenerator("https://app.routewerk.com")
	pdfBytes, err := gen.GenerateDigitalPDF(testCircuitCardData())
	if err != nil {
		t.Fatalf("Circuit DigitalPDF failed: %v", err)
	}
	if !strings.HasPrefix(string(pdfBytes[:5]), "%PDF-") {
		t.Error("not a valid PDF")
	}
	t.Logf("Circuit Digital PDF: %d bytes", len(pdfBytes))
}

func TestCircuitWithGrade(t *testing.T) {
	gen := NewCardGenerator("https://app.routewerk.com")
	data := testCircuitCardData()
	data.Route.Grade = "V3" // circuit with optional grade

	_, err := gen.GeneratePrintPNG(data)
	if err != nil {
		t.Fatalf("circuit+grade print: %v", err)
	}
	_, err = gen.GenerateDigitalPNG(data)
	if err != nil {
		t.Fatalf("circuit+grade digital: %v", err)
	}
}

func TestCircuitNoName(t *testing.T) {
	gen := NewCardGenerator("https://app.routewerk.com")
	data := testCircuitCardData()
	data.Route.Name = nil

	_, err := gen.GeneratePrintPNG(data)
	if err != nil {
		t.Fatalf("circuit no-name print: %v", err)
	}
	_, err = gen.GenerateDigitalPNG(data)
	if err != nil {
		t.Fatalf("circuit no-name digital: %v", err)
	}
}

func TestIsCircuitDetection(t *testing.T) {
	// Circuit via CircuitColor field
	cc := "Blue"
	d1 := CardData{Route: &model.Route{CircuitColor: &cc, GradingSystem: "v_scale"}}
	if !d1.isCircuit() {
		t.Error("expected isCircuit() for CircuitColor set")
	}

	// Circuit via GradingSystem
	d2 := CardData{Route: &model.Route{GradingSystem: "circuit"}}
	if !d2.isCircuit() {
		t.Error("expected isCircuit() for GradingSystem=circuit")
	}

	// Graded route
	d3 := CardData{Route: &model.Route{GradingSystem: "v_scale"}}
	if d3.isCircuit() {
		t.Error("expected !isCircuit() for graded route")
	}
}

func TestColorLabel(t *testing.T) {
	cc := "ORANGE"
	d := CardData{Route: &model.Route{CircuitColor: &cc, Color: "#ff9800"}}
	if got := d.colorLabel(); got != "Orange" {
		t.Errorf("colorLabel() = %q, want %q", got, "Orange")
	}

	// Falls back to hex name
	d2 := CardData{Route: &model.Route{Color: "#e53935"}}
	if got := d2.colorLabel(); got != "Red" {
		t.Errorf("colorLabel() = %q, want %q", got, "Red")
	}
}

func TestTitleCase(t *testing.T) {
	tests := map[string]string{
		"hello world": "Hello World",
		"red":         "Red",
		"DARK GREEN":  "DARK GREEN",
		"":            "",
	}
	for in, want := range tests {
		if got := titleCase(in); got != want {
			t.Errorf("titleCase(%q) = %q, want %q", in, got, want)
		}
	}
}
