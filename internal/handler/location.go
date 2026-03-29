package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

type LocationHandler struct {
	locations *repository.LocationRepo
}

func NewLocationHandler(locations *repository.LocationRepo) *LocationHandler {
	return &LocationHandler{locations: locations}
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
		Error(w, http.StatusInternalServerError, "failed to create location")
		return
	}

	JSON(w, http.StatusCreated, loc)
}

func (h *LocationHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")

	locations, err := h.locations.ListByOrg(r.Context(), orgID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
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
		Error(w, http.StatusInternalServerError, "internal error")
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
		Error(w, http.StatusInternalServerError, "internal error")
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
		Error(w, http.StatusInternalServerError, "failed to update location")
		return
	}

	JSON(w, http.StatusOK, loc)
}
