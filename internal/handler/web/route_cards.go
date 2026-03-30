package webhandler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

// ── Route Cards ──────────────────────────────────────────────

// acquireUploadSem blocks until a slot is available in the shared image-processing
// semaphore, or returns false if the request context is cancelled.
func (h *Handler) acquireUploadSem(w http.ResponseWriter, r *http.Request) bool {
	select {
	case h.uploadSem <- struct{}{}:
		return true
	case <-r.Context().Done():
		http.Error(w, "Request cancelled", http.StatusServiceUnavailable)
		return false
	}
}

func (h *Handler) releaseUploadSem() { <-h.uploadSem }

// RouteCardPrintPNG serves the print-ready route card as PNG.
func (h *Handler) RouteCardPrintPNG(w http.ResponseWriter, r *http.Request) {
	data, ok := h.resolveCardData(w, r)
	if !ok {
		return
	}
	if !h.acquireUploadSem(w, r) {
		return
	}
	defer h.releaseUploadSem()

	pngBytes, err := h.cardGen.GeneratePrintPNG(data)
	if err != nil {
		slog.Error("print card generation failed", "error", err)
		http.Error(w, "card generation failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(pngBytes) //nolint:errcheck
}

// RouteCardPrintPDF serves the print-ready route card as PDF.
func (h *Handler) RouteCardPrintPDF(w http.ResponseWriter, r *http.Request) {
	data, ok := h.resolveCardData(w, r)
	if !ok {
		return
	}
	if !h.acquireUploadSem(w, r) {
		return
	}
	defer h.releaseUploadSem()

	pdfBytes, err := h.cardGen.GeneratePrintPDF(data)
	if err != nil {
		slog.Error("print card PDF generation failed", "error", err)
		http.Error(w, "card generation failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline; filename=\"route-card.pdf\"")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(pdfBytes) //nolint:errcheck
}

// RouteCardSharePNG serves the shareable digital route card as PNG.
func (h *Handler) RouteCardSharePNG(w http.ResponseWriter, r *http.Request) {
	data, ok := h.resolveCardData(w, r)
	if !ok {
		return
	}
	if !h.acquireUploadSem(w, r) {
		return
	}
	defer h.releaseUploadSem()

	pngBytes, err := h.cardGen.GenerateDigitalPNG(data)
	if err != nil {
		slog.Error("share card generation failed", "error", err)
		http.Error(w, "card generation failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Write(pngBytes) //nolint:errcheck
}

// RouteCardSharePDF serves the shareable digital route card as PDF.
func (h *Handler) RouteCardSharePDF(w http.ResponseWriter, r *http.Request) {
	data, ok := h.resolveCardData(w, r)
	if !ok {
		return
	}
	if !h.acquireUploadSem(w, r) {
		return
	}
	defer h.releaseUploadSem()

	pdfBytes, err := h.cardGen.GenerateDigitalPDF(data)
	if err != nil {
		slog.Error("share card PDF generation failed", "error", err)
		http.Error(w, "card generation failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline; filename=\"route-card-share.pdf\"")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Write(pdfBytes) //nolint:errcheck
}

// resolveCardData loads route, wall, location, and setter info for card generation.
func (h *Handler) resolveCardData(w http.ResponseWriter, r *http.Request) (service.CardData, bool) {
	ctx := r.Context()
	routeID := chi.URLParam(r, "routeID")
	locationID := middleware.GetWebLocationID(ctx)

	rt, err := h.routeRepo.GetByID(ctx, routeID)
	if err != nil {
		slog.Error("card: route lookup failed", "route_id", routeID, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return service.CardData{}, false
	}
	if rt == nil {
		http.Error(w, "route not found", http.StatusNotFound)
		return service.CardData{}, false
	}

	wallName := ""
	if wall, wErr := h.wallRepo.GetByID(ctx, rt.WallID); wErr == nil && wall != nil {
		wallName = wall.Name
	}

	locationName := ""
	if loc, lErr := h.locationRepo.GetByID(ctx, locationID); lErr == nil && loc != nil {
		locationName = loc.Name
	}

	setterName := ""
	if rt.SetterID != nil {
		if setter, uErr := h.userRepo.GetByID(ctx, *rt.SetterID); uErr == nil && setter != nil {
			setterName = setter.DisplayName
		}
	}

	return service.CardData{
		Route:        rt,
		WallName:     wallName,
		LocationName: locationName,
		SetterName:   setterName,
		QRTargetURL:  h.cardGen.RouteURL(locationID, routeID),
	}, true
}

// ── Climber Walls ────────────────────────────────────────────

// ClimberWalls renders the wall browser for climbers (GET /explore/walls).
func (h *Handler) ClimberWalls(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a location.")
		return
	}

	walls, err := h.wallRepo.ListWithCounts(ctx, locationID)
	if err != nil {
		slog.Error("climber walls failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not load walls.")
		return
	}

	var wallViews []WallView
	for _, wc := range walls {
		// Only show walls with active routes to climbers
		wallViews = append(wallViews, WallView{
			Wall:         wc.Wall,
			ActiveRoutes: wc.ActiveRoutes,
			TotalRoutes:  wc.TotalRoutes,
		})
	}

	data := &PageData{
		TemplateData: templateDataFromContext(r, "explore-walls"),
		WallList:     wallViews,
	}
	h.render(w, r, "climber/walls.html", data)
}

// ── Helpers ───────────────────────────────────────────────────

// loadConsensus fetches difficulty vote tallies and computes percentages.
func loadConsensus(ctx context.Context, repo *repository.DifficultyRepo, routeID string) *ConsensusData {
	if repo == nil {
		return nil
	}
	tally, err := repo.RouteCounts(ctx, routeID)
	if err != nil {
		slog.Error("load consensus failed", "route_id", routeID, "error", err)
		return nil
	}
	if tally.Total == 0 {
		return &ConsensusData{}
	}
	return &ConsensusData{
		EasyCount:  tally.Easy,
		RightCount: tally.Right,
		HardCount:  tally.Hard,
		TotalVotes: tally.Total,
		EasyPct:    tally.Easy * 100 / tally.Total,
		RightPct:   tally.Right * 100 / tally.Total,
		HardPct:    tally.Hard * 100 / tally.Total,
	}
}

func ascentTypeLabel(t string) string {
	switch t {
	case "send":
		return "sent"
	case "flash":
		return "flashed"
	case "attempt":
		return "attempted"
	case "project":
		return "projected"
	default:
		return t
	}
}

// buildPyramidBars converts raw GradePyramidEntry rows into display bars.
// Returns entries sorted by grade with a percentage width for rendering.
func buildPyramidBars(entries []repository.GradePyramidEntry) []PyramidBar {
	if len(entries) == 0 {
		return nil
	}

	maxCount := 0
	for _, e := range entries {
		if e.Count > maxCount {
			maxCount = e.Count
		}
	}

	var bars []PyramidBar
	for _, e := range entries {
		pct := 0
		if maxCount > 0 {
			pct = e.Count * 100 / maxCount
		}
		if pct < 5 {
			pct = 5 // minimum visible bar width
		}
		bars = append(bars, PyramidBar{
			Grade:    e.Grade,
			System:   e.GradingSystem,
			Count:    e.Count,
			WidthPct: pct,
		})
	}
	return bars
}
