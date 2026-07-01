package handler

import (
	"net/http"

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
		InternalError(w, r, "failed to follow", err)
		return
	}

	JSON(w, http.StatusOK, map[string]string{"status": "following"})
}

func (h *FollowHandler) Unfollow(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	targetID := chi.URLParam(r, "userID")

	if err := h.follows.Unfollow(r.Context(), userID, targetID); err != nil {
		InternalError(w, r, "failed to unfollow", err)
		return
	}

	JSON(w, http.StatusOK, map[string]string{"status": "unfollowed"})
}

func (h *FollowHandler) Followers(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "userID")
	limit, offset := clampPage(r, 50, 200)

	users, err := h.follows.Followers(r.Context(), targetID, limit, offset)
	if err != nil {
		InternalError(w, r, "internal error", err)
		return
	}

	JSON(w, http.StatusOK, users)
}

func (h *FollowHandler) Following(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "userID")
	limit, offset := clampPage(r, 50, 200)

	users, err := h.follows.Following(r.Context(), targetID, limit, offset)
	if err != nil {
		InternalError(w, r, "internal error", err)
		return
	}

	JSON(w, http.StatusOK, users)
}

func (h *FollowHandler) Feed(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	// 30, not 50: preserve the feed's pre-clampPage default (the repo's
	// own limit<=0 default was 30 and is now dead code behind this).
	limit, offset := clampPage(r, 30, 200)

	items, err := h.follows.ActivityFeed(r.Context(), userID, limit, offset)
	if err != nil {
		InternalError(w, r, "internal error", err)
		return
	}

	JSON(w, http.StatusOK, items)
}
