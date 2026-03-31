package main

import (
	"os"
	"testing"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

// TestSampleCardGeneration verifies that the sample card data produces
// valid PNG output via the CardGenerator. This is an end-to-end smoke
// test for the card generation pipeline.

func TestGradedCardGeneration(t *testing.T) {
	gen := service.NewCardGenerator("https://app.routewerk.com")

	name := "Crimson Crush"
	setter := "setter-123"
	data := service.CardData{
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
			},
		},
		WallName:     "The Cave",
		LocationName: "LEF Boulder",
		SetterName:   "Chris S.",
		QRTargetURL:  "https://app.routewerk.com/locations/loc-123/routes/route-abc",
	}

	// Test print PNG generation
	printPNG, err := gen.GeneratePrintPNG(data)
	if err != nil {
		t.Fatalf("GeneratePrintPNG: %v", err)
	}
	if len(printPNG) == 0 {
		t.Error("GeneratePrintPNG returned empty data")
	}
	// PNG magic bytes: \x89PNG
	if len(printPNG) > 4 && string(printPNG[:4]) != "\x89PNG" {
		t.Error("GeneratePrintPNG output is not a valid PNG")
	}

	// Test digital PNG generation
	digitalPNG, err := gen.GenerateDigitalPNG(data)
	if err != nil {
		t.Fatalf("GenerateDigitalPNG: %v", err)
	}
	if len(digitalPNG) == 0 {
		t.Error("GenerateDigitalPNG returned empty data")
	}
	if len(digitalPNG) > 4 && string(digitalPNG[:4]) != "\x89PNG" {
		t.Error("GenerateDigitalPNG output is not a valid PNG")
	}
}

func TestCircuitCardGeneration(t *testing.T) {
	gen := service.NewCardGenerator("https://app.routewerk.com")

	name := "The Slab Problem"
	setter := "setter-456"
	color := "Red"
	data := service.CardData{
		Route: &model.Route{
			ID:            "route-circuit-1",
			LocationID:    "loc-123",
			WallID:        "wall-789",
			SetterID:      &setter,
			RouteType:     "boulder",
			Status:        "active",
			GradingSystem: "circuit",
			Grade:         "",
			CircuitColor:  &color,
			Name:          &name,
			Color:         "#e53935",
			DateSet:       time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
			AvgRating:     3.8,
			RatingCount:   12,
			AscentCount:   34,
			AttemptCount:  89,
			Tags: []model.Tag{
				{Name: "Slab"},
			},
		},
		WallName:     "The Slab Wall",
		LocationName: "LEF Boulder",
		SetterName:   "Alex M.",
		QRTargetURL:  "https://app.routewerk.com/locations/loc-123/routes/route-circuit-1",
	}

	printPNG, err := gen.GeneratePrintPNG(data)
	if err != nil {
		t.Fatalf("GeneratePrintPNG (circuit): %v", err)
	}
	if len(printPNG) == 0 {
		t.Error("circuit GeneratePrintPNG returned empty data")
	}

	digitalPNG, err := gen.GenerateDigitalPNG(data)
	if err != nil {
		t.Fatalf("GenerateDigitalPNG (circuit): %v", err)
	}
	if len(digitalPNG) == 0 {
		t.Error("circuit GenerateDigitalPNG returned empty data")
	}
}

func TestCardWriteToFile(t *testing.T) {
	gen := service.NewCardGenerator("https://app.routewerk.com")

	name := "Test Route"
	data := service.CardData{
		Route: &model.Route{
			ID:            "route-test",
			LocationID:    "loc-1",
			WallID:        "wall-1",
			RouteType:     "boulder",
			Status:        "active",
			GradingSystem: "v_scale",
			Grade:         "V3",
			Name:          &name,
			Color:         "#1565c0",
			DateSet:       time.Now(),
		},
		WallName:     "Test Wall",
		LocationName: "Test Gym",
		QRTargetURL:  "https://example.com",
	}

	png, err := gen.GenerateDigitalPNG(data)
	if err != nil {
		t.Fatalf("GenerateDigitalPNG: %v", err)
	}

	// Write to temp file and verify
	tmpFile, err := os.CreateTemp("", "card-test-*.png")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := tmpFile.Write(png); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Verify file was written
	info, err := os.Stat(tmpFile.Name())
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() == 0 {
		t.Error("written file is empty")
	}
	if info.Size() != int64(len(png)) {
		t.Errorf("file size = %d, want %d", info.Size(), len(png))
	}
}
