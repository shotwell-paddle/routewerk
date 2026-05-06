package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// NotificationHandler exposes the unread-notification surface as JSON
// for the SPA at /app/notifications. The HTMX side at /notifications
// uses the same NotificationRepo underneath; this handler is the
// parallel JSON entry point.
type NotificationHandler struct {
	notifs *repository.NotificationRepo
}

func NewNotificationHandler(notifs *repository.NotificationRepo) *NotificationHandler {
	return &NotificationHandler{notifs: notifs}
}

// List — GET /me/notifications. Returns the caller's unread notifications,
// newest first. Mirrors the HTMX /notifications page (same repo call).
func (h *NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	notifs, err := h.notifs.ListUnread(r.Context(), userID, limit)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if notifs == nil {
		notifs = []repository.Notification{}
	}
	JSON(w, http.StatusOK, map[string]interface{}{"notifications": notifs})
}

// UnreadCount — GET /me/notifications/unread-count. Cheap endpoint for
// the sidebar badge poll. The HTMX page polls every 60s; the SPA layout
// can do the same.
func (h *NotificationHandler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	count, err := h.notifs.UnreadCount(r.Context(), userID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"count": count})
}

// MarkRead — POST /me/notifications/{notifID}/read. The repo includes
// user_id in the WHERE clause so a caller can't mark someone else's
// notifications as read.
func (h *NotificationHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	idStr := chi.URLParam(r, "notifID")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		Error(w, http.StatusBadRequest, "invalid notification id")
		return
	}
	if err := h.notifs.MarkRead(r.Context(), id, userID); err != nil {
		// Repo returns "not found or already read" — surface as 404 so the
		// SPA can decide whether to silently dedupe (already read) or warn.
		Error(w, http.StatusNotFound, "notification not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MarkAllRead — POST /me/notifications/read-all. Returns count of rows
// affected so the SPA can confirm "5 marked read" if desired.
func (h *NotificationHandler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	count, err := h.notifs.MarkAllRead(r.Context(), userID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"marked": count})
}
