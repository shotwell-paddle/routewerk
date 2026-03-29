package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

type PartnerHandler struct {
	partners *repository.PartnerRepo
}

func NewPartnerHandler(partners *repository.PartnerRepo) *PartnerHandler {
	return &PartnerHandler{partners: partners}
}

type updatePartnerProfileRequest struct {
	LookingFor    []string `json:"looking_for"`
	ClimbingTypes []string `json:"climbing_types"`
	GradeRange    *string  `json:"grade_range,omitempty"`
	Availability  []byte   `json:"availability,omitempty"`
	Bio           *string  `json:"bio,omitempty"`
	Active        bool     `json:"active"`
}

func (h *PartnerHandler) Search(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")

	partners, err := h.partners.Search(r.Context(), locationID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	JSON(w, http.StatusOK, partners)
}

func (h *PartnerHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	userID := middleware.GetUserID(r.Context())

	var req updatePartnerProfileRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	profile := &model.PartnerProfile{
		UserID:        userID,
		LocationID:    locationID,
		LookingFor:    req.LookingFor,
		ClimbingTypes: req.ClimbingTypes,
		GradeRange:    req.GradeRange,
		Availability:  req.Availability,
		Bio:           req.Bio,
		Active:        req.Active,
	}

	if err := h.partners.Upsert(r.Context(), profile); err != nil {
		Error(w, http.StatusInternalServerError, "failed to update profile")
		return
	}

	JSON(w, http.StatusOK, profile)
}

func (h *PartnerHandler) MyProfile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	profile, err := h.partners.GetByUser(r.Context(), userID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if profile == nil {
		Error(w, http.StatusNotFound, "no partner profile")
		return
	}

	JSON(w, http.StatusOK, profile)
}
