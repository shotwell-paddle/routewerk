package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// RouteDistributionTargetHandler exposes per-location route-distribution
// targets to the SPA — the "what should our mix look like" goals that
// head-setters configure to plan setting work. The dashboard
// distribution charts overlay these as target markers; the settings
// page (head_setter+ only) edits the full set in one shot.
type RouteDistributionTargetHandler struct {
	targets *repository.RouteDistributionTargetRepo
}

func NewRouteDistributionTargetHandler(targets *repository.RouteDistributionTargetRepo) *RouteDistributionTargetHandler {
	return &RouteDistributionTargetHandler{targets: targets}
}

// List — GET /api/v1/locations/{locationID}/distribution-targets.
//
// Setter+ at the location can read (so the dashboard chart overlay
// works for everyone with chart access). Empty list when no targets
// have been configured yet.
func (h *RouteDistributionTargetHandler) List(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	if !isUUID(locationID) {
		Error(w, http.StatusBadRequest, "invalid location id")
		return
	}
	out, err := h.targets.ListByLocation(r.Context(), locationID)
	if err != nil {
		slog.Error("list distribution targets", "location_id", locationID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if out == nil {
		out = []repository.RouteDistributionTarget{}
	}
	JSON(w, http.StatusOK, out)
}

type replaceTargetsRequest struct {
	Targets []targetEntry `json:"targets"`
}

type targetEntry struct {
	RouteType   string `json:"route_type"`
	Grade       string `json:"grade"`
	TargetCount int    `json:"target_count"`
}

// Replace — PUT /api/v1/locations/{locationID}/distribution-targets.
//
// Wipes the existing target set for the location and inserts the
// caller-supplied set in one transaction. head_setter+ enforced by
// router middleware. target_count <= 0 entries are silently dropped
// (treat "0" as "no goal").
func (h *RouteDistributionTargetHandler) Replace(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	if !isUUID(locationID) {
		Error(w, http.StatusBadRequest, "invalid location id")
		return
	}
	var req replaceTargetsRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	allowed := map[string]bool{"boulder": true, "route": true, "circuit": true}
	for _, t := range req.Targets {
		if t.RouteType == "" || t.Grade == "" {
			Error(w, http.StatusBadRequest, "route_type and grade are required on every entry")
			return
		}
		if !allowed[t.RouteType] {
			Error(w, http.StatusBadRequest, "route_type must be 'boulder', 'route', or 'circuit'")
			return
		}
		if t.TargetCount < 0 {
			Error(w, http.StatusBadRequest, "target_count must be non-negative")
			return
		}
	}

	models := make([]repository.RouteDistributionTarget, 0, len(req.Targets))
	for _, t := range req.Targets {
		models = append(models, repository.RouteDistributionTarget{
			LocationID:  locationID,
			RouteType:   t.RouteType,
			Grade:       t.Grade,
			TargetCount: t.TargetCount,
		})
	}
	if err := h.targets.ReplaceAll(r.Context(), locationID, models); err != nil {
		slog.Error("replace distribution targets", "location_id", locationID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	out, err := h.targets.ListByLocation(r.Context(), locationID)
	if err != nil {
		slog.Error("re-list distribution targets after replace", "location_id", locationID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if out == nil {
		out = []repository.RouteDistributionTarget{}
	}
	JSON(w, http.StatusOK, out)
}
