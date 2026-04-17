package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

type RatingHandler struct {
	ratings *repository.RatingRepo
}

func NewRatingHandler(ratings *repository.RatingRepo) *RatingHandler {
	return &RatingHandler{ratings: ratings}
}

type rateRequest struct {
	Rating  int     `json:"rating"`
	Comment *string `json:"comment,omitempty"`
}

func (h *RatingHandler) Rate(w http.ResponseWriter, r *http.Request) {
	routeID := chi.URLParam(r, "routeID")
	userID := middleware.GetUserID(r.Context())

	var req rateRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Rating < 1 || req.Rating > 5 {
		Error(w, http.StatusBadRequest, "rating must be between 1 and 5")
		return
	}

	rating := &model.RouteRating{
		UserID:  userID,
		RouteID: routeID,
		Rating:  req.Rating,
		Comment: req.Comment,
	}

	if err := h.ratings.Upsert(r.Context(), rating); err != nil {
		Error(w, http.StatusInternalServerError, "failed to save rating")
		return
	}

	JSON(w, http.StatusOK, rating)
}

func (h *RatingHandler) RouteRatings(w http.ResponseWriter, r *http.Request) {
	// Scope ratings to the locationID the URL was nested under. The
	// RequireLocationMember middleware upstream has already verified the
	// caller has membership at this location; pinning the DB query to the
	// same locationID closes the cross-tenant probe via a stolen routeID.
	locationID := chi.URLParam(r, "locationID")
	routeID := chi.URLParam(r, "routeID")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	ratings, err := h.ratings.ListByRoute(r.Context(), routeID, locationID, limit, offset)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	JSON(w, http.StatusOK, ratings)
}
