package webhandler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
)

// SessionDelete removes a session and its draft routes (POST /sessions/{sessionID}/delete).
func (h *Handler) SessionDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := chi.URLParam(r, "sessionID")

	session, sErr := h.sessionRepo.GetByID(ctx, sessionID)
	if sErr != nil || session == nil {
		h.renderError(w, r, http.StatusNotFound, "Session not found", "That session doesn't exist.")
		return
	}
	if !h.checkLocationOwnership(w, r, session.LocationID) {
		return
	}

	if err := h.sessionRepo.Delete(ctx, sessionID); err != nil {
		slog.Error("delete session failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Delete failed", "Could not delete session.")
		return
	}

	http.Redirect(w, r, "/sessions", http.StatusSeeOther)
}

// SessionComplete renders the review/publish page (GET /sessions/{sessionID}/complete).
func (h *Handler) SessionComplete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := chi.URLParam(r, "sessionID")
	locationID := middleware.GetWebLocationID(ctx)

	session, assignments, err := h.sessionRepo.GetByIDWithDetails(ctx, sessionID)
	if err != nil || session == nil {
		h.renderError(w, r, http.StatusNotFound, "Session not found", "That session doesn't exist.")
		return
	}
	if !h.checkLocationOwnership(w, r, session.LocationID) {
		return
	}

	sessionRoutes, err := h.sessionRepo.ListSessionRoutes(ctx, sessionID)
	if err != nil {
		slog.Error("list session routes for complete failed", "error", err)
	}

	walls, err := h.wallRepo.ListByLocation(ctx, locationID)
	if err != nil {
		slog.Error("load walls for session complete failed", "location_id", locationID, "error", err)
	}
	setters, err := h.userRepo.ListSettersByLocation(ctx, locationID)
	if err != nil {
		slog.Error("load setters for session complete failed", "location_id", locationID, "error", err)
	}

	locSettings, err := h.settingsRepo.GetLocationSettings(ctx, locationID)
	if err != nil {
		slog.Error("load location settings for session complete failed", "location_id", locationID, "error", err)
		locSettings = defaultLocationSettings()
	}

	data := &PageData{
		TemplateData:       templateDataFromContext(r, "sessions"),
		Session:            session,
		SessionAssignments: assignments,
		SessionRoutes:      sessionRoutes,
		FormWalls:          walls,
		Setters:            setters,
		HoldColors:         holdColorsFromSettings(locSettings),
		VScaleGrades:       vScaleGrades,
		YDSGrades:          ydsGrades,
		CircuitColors:      circuitColors,
	}

	h.render(w, r, "setter/session-complete.html", data)
}

// SessionPublish publishes all draft routes and marks session complete (POST /sessions/{sessionID}/publish).
func (h *Handler) SessionPublish(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := chi.URLParam(r, "sessionID")

	session, sErr := h.sessionRepo.GetByID(ctx, sessionID)
	if sErr != nil || session == nil {
		h.renderError(w, r, http.StatusNotFound, "Session not found", "That session doesn't exist.")
		return
	}
	if !h.checkLocationOwnership(w, r, session.LocationID) {
		return
	}

	// Archive routes on the stripping list BEFORE publishing new routes,
	// so that whole-wall strips don't catch the freshly published ones.
	stripTargets, err := h.sessionRepo.ListStripTargets(ctx, sessionID)
	if err != nil {
		slog.Error("load strip targets failed", "error", err)
	} else {
		var routeIDs []string
		for _, t := range stripTargets {
			if t.RouteID == nil {
				// Whole-wall strip — archive all active routes on this wall
				n, archErr := h.routeRepo.BulkArchiveByWall(ctx, t.WallID)
				if archErr != nil {
					slog.Error("archive wall routes failed", "wall_id", t.WallID, "error", archErr)
				} else if n > 0 {
					slog.Info("archived wall routes", "session_id", sessionID, "wall_id", t.WallID, "count", n)
				}
			} else {
				// Individual route strip
				routeIDs = append(routeIDs, *t.RouteID)
			}
		}
		if len(routeIDs) > 0 {
			n, archErr := h.routeRepo.BulkArchive(ctx, routeIDs)
			if archErr != nil {
				slog.Error("archive stripped routes failed", "error", archErr)
			} else if n > 0 {
				slog.Info("archived stripped routes", "session_id", sessionID, "count", n)
			}
		}
	}

	// Publish all draft routes (after stripping, so new routes stay active)
	if _, err := h.sessionRepo.PublishSessionRoutes(ctx, sessionID); err != nil {
		slog.Error("publish session routes failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Publish failed", "Could not publish routes.")
		return
	}

	// Mark session complete
	if err := h.sessionRepo.UpdateStatus(ctx, sessionID, "complete"); err != nil {
		slog.Error("update session status failed", "error", err)
	}

	// Redirect to photos page so the setter can upload route photos.
	// Fall back to session detail if storage isn't configured.
	if h.storageService.IsConfigured() {
		http.Redirect(w, r, "/sessions/"+sessionID+"/photos", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/sessions/"+sessionID, http.StatusSeeOther)
}

// SessionReopen sets a completed session back to in_progress (POST /sessions/{sessionID}/reopen).
// Requires head_setter role or above.
func (h *Handler) SessionReopen(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := chi.URLParam(r, "sessionID")

	session, sErr := h.sessionRepo.GetByID(ctx, sessionID)
	if sErr != nil || session == nil {
		h.renderError(w, r, http.StatusNotFound, "Session not found", "That session doesn't exist.")
		return
	}
	if !h.checkLocationOwnership(w, r, session.LocationID) {
		return
	}

	// Only head_setter+ can reopen
	role := middleware.GetWebRole(ctx)
	if middleware.RoleRankValue(role) < 3 {
		h.renderError(w, r, http.StatusForbidden, "Not allowed", "Only head setters and above can reopen sessions.")
		return
	}

	if err := h.sessionRepo.UpdateStatus(ctx, sessionID, "in_progress"); err != nil {
		slog.Error("reopen session failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Reopen failed", "Could not reopen session.")
		return
	}

	http.Redirect(w, r, "/sessions/"+sessionID, http.StatusSeeOther)
}
