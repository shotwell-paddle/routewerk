package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

// CardHandler generates route info cards with QR codes.
type CardHandler struct {
	routes    *repository.RouteRepo
	walls     *repository.WallRepo
	locations *repository.LocationRepo
	users     *repository.UserRepo
	cardGen   *service.CardGenerator
}

func NewCardHandler(
	routes *repository.RouteRepo,
	walls *repository.WallRepo,
	locations *repository.LocationRepo,
	users *repository.UserRepo,
	cardGen *service.CardGenerator,
) *CardHandler {
	return &CardHandler{
		routes:    routes,
		walls:     walls,
		locations: locations,
		users:     users,
		cardGen:   cardGen,
	}
}

// PrintPNG serves the print-ready route card (no volatile data).
func (h *CardHandler) PrintPNG(w http.ResponseWriter, r *http.Request) {
	data, ok := h.resolveCardData(w, r)
	if !ok {
		return
	}
	pngBytes, err := h.cardGen.GeneratePrintPNG(data)
	if err != nil {
		Error(w, http.StatusInternalServerError, "failed to generate card")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(pngBytes) //nolint:errcheck
}

// PrintPDF serves the print-ready route card as PDF.
func (h *CardHandler) PrintPDF(w http.ResponseWriter, r *http.Request) {
	data, ok := h.resolveCardData(w, r)
	if !ok {
		return
	}
	pdfBytes, err := h.cardGen.GeneratePrintPDF(data)
	if err != nil {
		Error(w, http.StatusInternalServerError, "failed to generate card")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline; filename=\"route-card.pdf\"")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(pdfBytes) //nolint:errcheck
}

// DigitalPNG serves the shareable digital card (includes live stats).
func (h *CardHandler) DigitalPNG(w http.ResponseWriter, r *http.Request) {
	data, ok := h.resolveCardData(w, r)
	if !ok {
		return
	}
	pngBytes, err := h.cardGen.GenerateDigitalPNG(data)
	if err != nil {
		Error(w, http.StatusInternalServerError, "failed to generate card")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=300") // shorter cache — live stats
	w.Write(pngBytes) //nolint:errcheck
}

// DigitalPDF serves the shareable digital card as PDF.
func (h *CardHandler) DigitalPDF(w http.ResponseWriter, r *http.Request) {
	data, ok := h.resolveCardData(w, r)
	if !ok {
		return
	}
	pdfBytes, err := h.cardGen.GenerateDigitalPDF(data)
	if err != nil {
		Error(w, http.StatusInternalServerError, "failed to generate card")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline; filename=\"route-card-share.pdf\"")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Write(pdfBytes) //nolint:errcheck
}

// resolveCardData loads route, wall, location, and setter info.
func (h *CardHandler) resolveCardData(w http.ResponseWriter, r *http.Request) (service.CardData, bool) {
	locationID := chi.URLParam(r, "locationID")
	routeID := chi.URLParam(r, "routeID")

	rt, err := h.routes.GetByID(r.Context(), routeID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return service.CardData{}, false
	}
	if rt == nil {
		Error(w, http.StatusNotFound, "route not found")
		return service.CardData{}, false
	}

	wallName := ""
	wall, err := h.walls.GetByID(r.Context(), rt.WallID)
	if err == nil && wall != nil {
		wallName = wall.Name
	}

	locationName := ""
	loc, err := h.locations.GetByID(r.Context(), locationID)
	if err == nil && loc != nil {
		locationName = loc.Name
	}

	setterName := ""
	if rt.SetterID != nil {
		setter, err := h.users.GetByID(r.Context(), *rt.SetterID)
		if err == nil && setter != nil {
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
