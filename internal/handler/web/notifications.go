package webhandler

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
)

// NotificationBadge returns just the sidebar unread-count badge.
// GET /notifications/badge
//
// Polled by HTMX off the sidebar so the unread count doesn't sit on the
// authenticated page-load critical path — see perf audit 2026-04-22
// finding #1. Fast indexed lookup against
// idx_notifications_user_unread; returns an empty body when the count
// is zero so HTMX just clears the badge element.
func (h *Handler) NotificationBadge(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	if user == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	count, err := h.notifRepo.UnreadCount(ctx, user.ID)
	if err != nil {
		slog.Error("notification badge: unread count failed", "user_id", user.ID, "error", err)
		// Soft-fail: returning no content is better than a broken nav.
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Short client cache so rapid navigation doesn't re-hit; HTMX poll
	// still refreshes every ~60s via hx-trigger.
	w.Header().Set("Cache-Control", "private, max-age=10")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if count > 0 {
		if _, err := fmt.Fprintf(w, `<span class="notif-badge">%d</span>`, count); err != nil {
			slog.Error("notification badge write failed", "error", err)
		}
	}
}

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
