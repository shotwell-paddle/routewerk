package webhandler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/middleware"
)


// SwitchLocation handles POST /switch-location.
// Updates the web session's location_id and redirects to the dashboard.
func (h *Handler) SwitchLocation(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	locationID := r.FormValue("location_id")
	if locationID == "" {
		http.Error(w, "Missing location", http.StatusBadRequest)
		return
	}

	// Validate the ID format
	if !validRouteID.MatchString(locationID) {
		http.Error(w, "Invalid location", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	session := middleware.GetWebSession(ctx)
	if user == nil || session == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Verify the user actually has access to this location
	locations, err := h.locationRepo.ListForUser(ctx, user.ID)
	if err != nil {
		slog.Error("failed to list user locations", "user_id", user.ID, "error", err)
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}

	found := false
	for _, loc := range locations {
		if loc.ID == locationID {
			found = true
			break
		}
	}
	if !found {
		http.Error(w, "Location not found", http.StatusForbidden)
		return
	}

	// Update the session's location in the database
	if err := h.webSessionRepo.UpdateLocation(ctx, session.ID, locationID); err != nil {
		slog.Error("failed to switch location", "session_id", session.ID, "location_id", locationID, "error", err)
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}

	// Clear any view-as override when switching locations (role may differ)
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.ViewAsCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	// Redirect — HTMX or full page
	redirect := "/dashboard"
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", redirect)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// SwitchViewAs handles POST /switch-view-as.
// Sets a cookie to override the user's visible role for testing/demo purposes.
func (h *Handler) SwitchViewAs(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	targetRole := r.FormValue("role")
	realRole := middleware.GetWebRealRole(r.Context())
	realRank := middleware.RoleRankValue(realRole)

	// Only users with head_setter+ can use view-as
	if realRank < 3 {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if targetRole == "" {
		// Clear override — revert to actual role
		http.SetCookie(w, &http.Cookie{
			Name:     middleware.ViewAsCookieName,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
	} else {
		// Can only view as a role lower than your actual role
		targetRank := middleware.RoleRankValue(targetRole)
		if targetRank == 0 || targetRank >= realRank {
			http.Error(w, "Invalid role", http.StatusBadRequest)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     middleware.ViewAsCookieName,
			Value:    targetRole,
			Path:     "/",
			MaxAge:   int((24 * time.Hour).Seconds()), // expires after 24h
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
	}

	// Redirect — the page will re-render with the new effective role
	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = "/dashboard"
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", referer)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, referer, http.StatusSeeOther)
}
