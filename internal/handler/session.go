package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

type SessionHandler struct {
	sessions *repository.SessionRepo
	// Used by Publish to archive routes targeted by strip-targets
	// before flipping the session to complete. Mirrors the HTMX
	// flow at internal/handler/web/sessions_lifecycle.go.
	routes *repository.RouteRepo
}

func NewSessionHandler(sessions *repository.SessionRepo, routes *repository.RouteRepo) *SessionHandler {
	return &SessionHandler{sessions: sessions, routes: routes}
}

type createSessionRequest struct {
	ScheduledDate string  `json:"scheduled_date"`
	Notes         *string `json:"notes,omitempty"`
}

type assignRequest struct {
	SetterID     string   `json:"setter_id"`
	WallID       *string  `json:"wall_id,omitempty"`
	TargetGrades []string `json:"target_grades,omitempty"`
	Notes        *string  `json:"notes,omitempty"`
}

func (h *SessionHandler) Create(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	userID := middleware.GetUserID(r.Context())

	var req createSessionRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ScheduledDate == "" {
		Error(w, http.StatusBadRequest, "scheduled_date is required")
		return
	}

	date, err := time.Parse("2006-01-02", req.ScheduledDate)
	if err != nil {
		Error(w, http.StatusBadRequest, "scheduled_date must be YYYY-MM-DD format")
		return
	}

	session := &model.SettingSession{
		LocationID:    locationID,
		ScheduledDate: date,
		Notes:         req.Notes,
		CreatedBy:     userID,
	}

	if err := h.sessions.Create(r.Context(), session); err != nil {
		Error(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	JSON(w, http.StatusCreated, session)
}

func (h *SessionHandler) List(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	sessions, err := h.sessions.ListByLocation(r.Context(), locationID, limit, offset)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	if sessions == nil {
		sessions = []model.SettingSession{}
	}

	JSON(w, http.StatusOK, sessions)
}

func (h *SessionHandler) Get(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")

	session, err := h.sessions.GetByID(r.Context(), sessionID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if session == nil {
		Error(w, http.StatusNotFound, "session not found")
		return
	}

	JSON(w, http.StatusOK, session)
}

func (h *SessionHandler) Update(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")

	session, err := h.sessions.GetByID(r.Context(), sessionID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if session == nil {
		Error(w, http.StatusNotFound, "session not found")
		return
	}

	var req createSessionRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ScheduledDate != "" {
		date, err := time.Parse("2006-01-02", req.ScheduledDate)
		if err == nil {
			session.ScheduledDate = date
		}
	}
	if req.Notes != nil {
		session.Notes = req.Notes
	}

	if err := h.sessions.Update(r.Context(), session); err != nil {
		Error(w, http.StatusInternalServerError, "failed to update session")
		return
	}

	session, _ = h.sessions.GetByID(r.Context(), sessionID)
	JSON(w, http.StatusOK, session)
}

func (h *SessionHandler) Assign(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")

	var req assignRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.SetterID == "" {
		Error(w, http.StatusBadRequest, "setter_id is required")
		return
	}

	assignment := &model.SettingSessionAssignment{
		SessionID:    sessionID,
		SetterID:     req.SetterID,
		WallID:       req.WallID,
		TargetGrades: req.TargetGrades,
		Notes:        req.Notes,
	}

	if err := h.sessions.AddAssignment(r.Context(), assignment); err != nil {
		Error(w, http.StatusInternalServerError, "failed to add assignment")
		return
	}

	JSON(w, http.StatusCreated, assignment)
}

// Unassign — DELETE /sessions/{sessionID}/assignments/{assignmentID}.
// head_setter+ enforced by router middleware.
func (h *SessionHandler) Unassign(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "assignmentID")
	if err := h.sessions.RemoveAssignment(r.Context(), id); err != nil {
		Error(w, http.StatusInternalServerError, "failed to remove assignment")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type sessionStatusRequest struct {
	Status string `json:"status"`
}

// UpdateStatus — POST /sessions/{sessionID}/status. Body: {status}. Allowed
// status flips: planning → in_progress → complete (or back). Note that
// the HTMX SessionPublish endpoint additionally publishes draft routes +
// runs strip targets when transitioning to complete; this JSON endpoint
// is the simple state flip only. The SPA can call this for plain
// planning/in_progress/cancelled transitions; for full publish-and-strip
// the SPA should redirect to the existing /sessions/{id}/complete HTMX
// view (which has the multi-step flow).
func (h *SessionHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	var req sessionStatusRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	allowed := map[string]bool{
		"planning":    true,
		"in_progress": true,
		"complete":    true,
		"cancelled":   true,
	}
	if !allowed[req.Status] {
		Error(w, http.StatusBadRequest, "invalid status")
		return
	}
	if err := h.sessions.UpdateStatus(r.Context(), sessionID, req.Status); err != nil {
		Error(w, http.StatusInternalServerError, "failed to update status")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Delete — DELETE /sessions/{sessionID}. Soft delete (deleted_at).
// head_setter+ enforced by router middleware.
func (h *SessionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	if err := h.sessions.Delete(r.Context(), sessionID); err != nil {
		Error(w, http.StatusInternalServerError, "failed to delete session")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Publish — POST /sessions/{sessionID}/publish.
//
// One-shot completion: archives any strip-targets (whole-wall +
// individual routes), then activates every draft route on the session,
// then flips the session to status=complete. Mirrors the HTMX flow
// (internal/handler/web/sessions_lifecycle.go::SessionPublish).
//
// head_setter+ enforced by router middleware. Returns the route counts
// + new session status so the SPA can show a confirmation summary
// without re-fetching.
type publishResult struct {
	StrippedRouteCount int    `json:"stripped_route_count"`
	PublishedRoutes    int    `json:"published_routes"`
	Status             string `json:"status"`
}

func (h *SessionHandler) Publish(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	if !isUUID(sessionID) {
		Error(w, http.StatusBadRequest, "invalid session id")
		return
	}

	// Order matters: archive THEN publish, otherwise a whole-wall strip
	// would catch the freshly-published routes too.
	stripTargets, err := h.sessions.ListStripTargets(r.Context(), sessionID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "could not load strip targets")
		return
	}
	stripped := 0
	var routeIDs []string
	for _, t := range stripTargets {
		if t.RouteID == nil {
			n, archErr := h.routes.BulkArchiveByWall(r.Context(), t.WallID)
			if archErr != nil {
				Error(w, http.StatusInternalServerError, "failed to archive wall routes")
				return
			}
			stripped += n
		} else {
			routeIDs = append(routeIDs, *t.RouteID)
		}
	}
	if len(routeIDs) > 0 {
		n, archErr := h.routes.BulkArchive(r.Context(), routeIDs)
		if archErr != nil {
			Error(w, http.StatusInternalServerError, "failed to archive routes")
			return
		}
		stripped += n
	}

	published, err := h.sessions.PublishSessionRoutes(r.Context(), sessionID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "failed to publish routes")
		return
	}

	if err := h.sessions.UpdateStatus(r.Context(), sessionID, "complete"); err != nil {
		Error(w, http.StatusInternalServerError, "failed to update session status")
		return
	}

	JSON(w, http.StatusOK, publishResult{
		StrippedRouteCount: stripped,
		PublishedRoutes:    published,
		Status:             "complete",
	})
}

// ── Strip targets ────────────────────────────────────────

// ListStripTargets — GET /sessions/{sessionID}/strip-targets.
func (h *SessionHandler) ListStripTargets(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	targets, err := h.sessions.ListStripTargets(r.Context(), sessionID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	JSON(w, http.StatusOK, targets)
}

type addStripTargetRequest struct {
	WallID  string  `json:"wall_id"`
	RouteID *string `json:"route_id,omitempty"`
}

// AddStripTarget — POST /sessions/{sessionID}/strip-targets. wall_id is
// required; route_id is optional (nil means "strip the whole wall").
// head_setter+ enforced by router middleware.
func (h *SessionHandler) AddStripTarget(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	var req addStripTargetRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.WallID == "" {
		Error(w, http.StatusBadRequest, "wall_id is required")
		return
	}
	target := &model.SessionStripTarget{
		SessionID: sessionID,
		WallID:    req.WallID,
		RouteID:   req.RouteID,
	}
	if err := h.sessions.AddStripTarget(r.Context(), target); err != nil {
		Error(w, http.StatusInternalServerError, "failed to add strip target")
		return
	}
	JSON(w, http.StatusCreated, target)
}

// RemoveStripTarget — DELETE /sessions/{sessionID}/strip-targets/{targetID}.
func (h *SessionHandler) RemoveStripTarget(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "targetID")
	if err := h.sessions.RemoveStripTarget(r.Context(), id); err != nil {
		Error(w, http.StatusInternalServerError, "failed to remove strip target")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Checklist ────────────────────────────────────────────

// ListChecklist — GET /sessions/{sessionID}/checklist. Returns the
// session's checklist items joined with the user who completed each
// (NULL for items that aren't done yet).
func (h *SessionHandler) ListChecklist(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	items, err := h.sessions.ListChecklistItems(r.Context(), sessionID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	JSON(w, http.StatusOK, items)
}

// ToggleChecklistItem — POST /sessions/{sessionID}/checklist/{itemID}/toggle.
// Marks an item done/undone for the calling user. Returns the new
// completion count.
func (h *SessionHandler) ToggleChecklistItem(w http.ResponseWriter, r *http.Request) {
	itemID := chi.URLParam(r, "itemID")
	userID := middleware.GetUserID(r.Context())
	count, err := h.sessions.ToggleChecklistItem(r.Context(), itemID, userID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "failed to toggle item")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"completion_count": count})
}
