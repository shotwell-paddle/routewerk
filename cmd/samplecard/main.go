package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/service"
	"github.com/shotwell-paddle/routewerk/internal/service/cardsheet"
)

func main() {
	sheet := flag.Bool("sheet", false,
		"also render an 8-up Silhouette Cameo print-and-cut sheet to <outDir>/sample_sheet.pdf")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: samplecard [--sheet] <outDir>\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	outDir := flag.Arg(0)

	gen := service.NewCardGenerator("https://app.routewerk.com")

	// ── Graded route ──
	name := "Crimson Crush"
	setter := "setter-123"
	graded := service.CardData{
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

	// ── Circuit route ──
	circuitName := "The Slab Problem"
	circuitSetter := "setter-456"
	circuitColor := "Red"
	circuit := service.CardData{
		Route: &model.Route{
			ID:            "route-circuit-1",
			LocationID:    "loc-123",
			WallID:        "wall-789",
			SetterID:      &circuitSetter,
			RouteType:     "boulder",
			Status:        "active",
			GradingSystem: "circuit",
			Grade:         "",
			CircuitColor:  &circuitColor,
			Name:          &circuitName,
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

	samples := []struct {
		name string
		fn   func() ([]byte, error)
	}{
		{"graded_print.png", func() ([]byte, error) { return gen.GeneratePrintPNG(graded) }},
		{"graded_digital.png", func() ([]byte, error) { return gen.GenerateDigitalPNG(graded) }},
		{"circuit_print.png", func() ([]byte, error) { return gen.GeneratePrintPNG(circuit) }},
		{"circuit_digital.png", func() ([]byte, error) { return gen.GenerateDigitalPNG(circuit) }},
		{"graded_sheet_face.png", func() ([]byte, error) { return gen.GenerateSheetCardPNG(graded) }},
		{"circuit_sheet_face.png", func() ([]byte, error) { return gen.GenerateSheetCardPNG(circuit) }},
	}

	for _, s := range samples {
		data, err := s.fn()
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR %s: %v\n", s.name, err)
			os.Exit(1)
		}
		path := outDir + "/" + s.name
		if err := os.WriteFile(path, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s (%d bytes)\n", path, len(data))
	}

	// ── Print-and-cut sheet (optional) ──
	//
	// Fills an 8-up sheet by alternating graded + circuit cards so every
	// rendering branch ends up on the first physical test cut. Output lands
	// next to the PNG samples so a tester can run:
	//   silhouette studio → open sample_sheet.pdf → Cut by Color → red
	if *sheet {
		composer := cardsheet.NewComposer(gen)
		cards := []service.CardData{
			graded, circuit, graded, circuit,
			graded, circuit, graded, circuit,
		}
		path := outDir + "/sample_sheet.pdf"
		f, err := os.Create(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "create %s: %v\n", path, err)
			os.Exit(1)
		}
		if err := composer.Render(f, cards, cardsheet.SheetConfig{}); err != nil {
			f.Close()
			fmt.Fprintf(os.Stderr, "render sheet: %v\n", err)
			os.Exit(1)
		}
		if err := f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "close %s: %v\n", path, err)
			os.Exit(1)
		}
		info, _ := os.Stat(path)
		fmt.Printf("wrote %s (%d bytes)\n", path, info.Size())
	}
}
