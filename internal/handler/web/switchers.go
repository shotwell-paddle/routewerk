package webhandler

import (
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/middleware"
)

// safeRedirect returns path only if it is a relative URL on the same host.
// Falls back to fallback for empty, absolute, or protocol-relative URLs.
func safeRedirect(raw, fallback string) string {
	if raw == "" {
		return fallback
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host != "" || u.Scheme != "" || !pathStartsWithSlash(raw) {
		return fallback
	}
	return u.Path
}

func pathStartsWithSlash(s string) bool {
	return len(s) > 0 && s[0] == '/'
}


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
		Secure:   !h.cfg.IsDev(),
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
			Secure:   !h.cfg.IsDev(),
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
			MaxAge:   int((1 * time.Hour).Seconds()), // expires after 1h (was 24h)
			HttpOnly: true,
			Secure:   !h.cfg.IsDev(),
			SameSite: http.SameSiteLaxMode,
		})
	}

	// Redirect — validate referer to prevent open redirects
	redirect := safeRedirect(r.Header.Get("Referer"), "/dashboard")

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", redirect)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}
