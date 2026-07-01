package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

type LocationHandler struct {
	locations *repository.LocationRepo
	audit     *service.AuditService
}

func NewLocationHandler(locations *repository.LocationRepo, audit *service.AuditService) *LocationHandler {
	return &LocationHandler{locations: locations, audit: audit}
}

// record is a nil-safe audit shim — tests construct the handler without
// an audit service.
func (h *LocationHandler) record(r *http.Request, action, locationID, orgID string, meta map[string]interface{}) {
	if h.audit != nil {
		h.audit.Record(r, action, "location", locationID, orgID, meta)
	}
}

type createLocationRequest struct {
	Name               string  `json:"name"`
	Slug               string  `json:"slug"`
	Address            *string `json:"address,omitempty"`
	Timezone           string  `json:"timezone"`
	WebsiteURL         *string `json:"website_url,omitempty"`
	Phone              *string `json:"phone,omitempty"`
	DayPassInfo        *string `json:"day_pass_info,omitempty"`
	WaiverURL          *string `json:"waiver_url,omitempty"`
	AllowSharedSetters bool    `json:"allow_shared_setters"`
}

func (h *LocationHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")

	var req createLocationRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		Error(w, http.StatusBadRequest, "name is required")
		return
	}

	if req.Slug == "" {
		req.Slug = slugify(req.Name)
	}

	if req.Timezone == "" {
		req.Timezone = "America/New_York"
	}

	loc := &model.Location{
		OrgID:              orgID,
		Name:               req.Name,
		Slug:               req.Slug,
		Address:            req.Address,
		Timezone:           req.Timezone,
		WebsiteURL:         req.WebsiteURL,
		Phone:              req.Phone,
		DayPassInfo:        req.DayPassInfo,
		WaiverURL:          req.WaiverURL,
		AllowSharedSetters: req.AllowSharedSetters,
	}

	if err := h.locations.Create(r.Context(), loc); err != nil {
		InternalError(w, r, "failed to create location", err)
		return
	}

	h.record(r, service.AuditLocationCreate, loc.ID, orgID, map[string]interface{}{
		"name": loc.Name,
	})

	JSON(w, http.StatusCreated, loc)
}

func (h *LocationHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")

	locations, err := h.locations.ListByOrg(r.Context(), orgID)
	if err != nil {
		InternalError(w, r, "internal error", err)
		return
	}

	if locations == nil {
		locations = []model.Location{}
	}

	JSON(w, http.StatusOK, locations)
}

func (h *LocationHandler) Get(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")

	loc, err := h.locations.GetByID(r.Context(), locationID)
	if err != nil {
		InternalError(w, r, "internal error", err)
		return
	}
	if loc == nil {
		Error(w, http.StatusNotFound, "location not found")
		return
	}

	JSON(w, http.StatusOK, loc)
}

func (h *LocationHandler) Update(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")

	loc, err := h.locations.GetByID(r.Context(), locationID)
	if err != nil {
		InternalError(w, r, "internal error", err)
		return
	}
	if loc == nil {
		Error(w, http.StatusNotFound, "location not found")
		return
	}

	var req createLocationRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name != "" {
		loc.Name = req.Name
	}
	if req.Slug != "" {
		loc.Slug = req.Slug
	}
	if req.Address != nil {
		loc.Address = req.Address
	}
	if req.Timezone != "" {
		loc.Timezone = req.Timezone
	}
	if req.WebsiteURL != nil {
		loc.WebsiteURL = req.WebsiteURL
	}
	if req.Phone != nil {
		loc.Phone = req.Phone
	}
	if req.DayPassInfo != nil {
		loc.DayPassInfo = req.DayPassInfo
	}
	if req.WaiverURL != nil {
		loc.WaiverURL = req.WaiverURL
	}
	loc.AllowSharedSetters = req.AllowSharedSetters

	if err := h.locations.Update(r.Context(), loc); err != nil {
		InternalError(w, r, "failed to update location", err)
		return
	}

	h.record(r, service.AuditLocationUpdate, loc.ID, loc.OrgID, nil)

	JSON(w, http.StatusOK, loc)
}

type progressionsToggleRequest struct {
	Enabled bool `json:"enabled"`
}

// SetProgressions — POST /locations/{locationID}/progressions-toggle.
//
// Flips the climber-facing progressions feature for a location.
// Mirrors the HTMX endpoint at internal/handler/web/settings.go::ProgressionsToggle.
// gym_manager+ enforced by router middleware (matches HTMX policy).
func (h *LocationHandler) SetProgressions(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	var req progressionsToggleRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.locations.SetProgressionsEnabled(r.Context(), locationID, req.Enabled); err != nil {
		InternalError(w, r, "failed to toggle progressions", err)
		return
	}
	h.record(r, service.AuditLocationUpdate, locationID, "", map[string]interface{}{
		"progressions_enabled": req.Enabled,
	})
	w.WriteHeader(http.StatusNoContent)
}
