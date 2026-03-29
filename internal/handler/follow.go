package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

type FollowHandler struct {
	follows *repository.FollowRepo
}

func NewFollowHandler(follows *repository.FollowRepo) *FollowHandler {
	return &FollowHandler{follows: follows}
}

func (h *FollowHandler) Follow(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	targetID := chi.URLParam(r, "userID")

	if userID == targetID {
		Error(w, http.StatusBadRequest, "cannot follow yourself")
		return
	}

	if err := h.follows.Follow(r.Context(), userID, targetID); err != nil {
		Error(w, http.StatusInternalServerError, "failed to follow")
		return
	}

	JSON(w, http.StatusOK, map[string]string{"status": "following"})
}

func (h *FollowHandler) Unfollow(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	targetID := chi.URLParam(r, "userID")

	if err := h.follows.Unfollow(r.Context(), userID, targetID); err != nil {
		Error(w, http.StatusInternalServerError, "failed to unfollow")
		return
	}

	JSON(w, http.StatusOK, map[string]string{"status": "unfollowed"})
}

func (h *FollowHandler) Followers(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "userID")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	users, err := h.follows.Followers(r.Context(), targetID, limit, offset)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	JSON(w, http.StatusOK, users)
}

func (h *FollowHandler) Following(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "userID")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	users, err := h.follows.Following(r.Context(), targetID, limit, offset)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	JSON(w, http.StatusOK, users)
}

func (h *FollowHandler) Feed(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	items, err := h.follows.ActivityFeed(r.Context(), userID, limit, offset)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	JSON(w, http.StatusOK, items)
}
