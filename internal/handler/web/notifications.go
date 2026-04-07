package webhandler

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
)

// Notifications renders the notification list page.
// GET /notifications
func (h *Handler) Notifications(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.GetWebUser(ctx)

	notifs, err := h.notifRepo.ListUnread(ctx, user.ID, 50)
	if err != nil {
		slog.Error("list notifications failed", "error", err)
	}

	data := &PageData{
		TemplateData:  templateDataFromContext(r, "notifications"),
		Notifications: notifs,
	}
	h.render(w, r, "climber/notifications.html", data)
}

// NotificationMarkRead marks a single notification as read.
// POST /notifications/{id}/read
func (h *Handler) NotificationMarkRead(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.GetWebUser(ctx)

	idStr := chi.URLParam(r, "notifID")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid ID", "Invalid notification ID.")
		return
	}

	if err := h.notifRepo.MarkRead(ctx, id, user.ID); err != nil {
		slog.Error("mark notification read failed", "error", err)
	}

	// Return updated notification list via HTMX
	notifs, _ := h.notifRepo.ListUnread(ctx, user.ID, 50)
	data := &PageData{
		TemplateData:  templateDataFromContext(r, "notifications"),
		Notifications: notifs,
	}
	h.render(w, r, "climber/notifications.html", data)
}

// NotificationMarkAllRead marks all notifications as read.
// POST /notifications/read-all
func (h *Handler) NotificationMarkAllRead(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.GetWebUser(ctx)

	if _, err := h.notifRepo.MarkAllRead(ctx, user.ID); err != nil {
		slog.Error("mark all notifications read failed", "error", err)
	}

	w.Header().Set("HX-Redirect", "/notifications")
	w.WriteHeader(http.StatusOK)
}
