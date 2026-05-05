package webhandler

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
)

// ── Playbook editor (head_setter and above, per gym) ─────────
//
// The playbook is the per-location template that's copied into each new
// session's checklist. Setters and head setters check the boxes during
// a session (handled in sessions_ops.go). Head setters and above edit
// the template here. Edits do NOT propagate into existing sessions —
// each session keeps the snapshot it was initialised with.

const playbookTitleMaxLen = 200

// PlaybookEditPage renders the playbook editor (GET /settings/playbook).
func (h *Handler) PlaybookEditPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a location.")
		return
	}
	if !h.requireHeadSetter(w, r) {
		return
	}

	steps, err := h.sessionRepo.ListPlaybookSteps(ctx, locationID)
	if err != nil {
		slog.Error("list playbook steps failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not load the playbook.")
		return
	}

	data := &PageData{
		TemplateData:    templateDataFromContext(r, "settings"),
		PlaybookSteps:   steps,
		SettingsSuccess: r.URL.Query().Get("saved") == "1",
	}
	h.render(w, r, "setter/playbook.html", data)
}

// PlaybookCreate appends a new step to the gym's playbook
// (POST /settings/playbook/add).
func (h *Handler) PlaybookCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		http.Error(w, "No location", http.StatusBadRequest)
		return
	}
	if !h.requireHeadSetter(w, r) {
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	title, ok := normaliseStepTitle(r.FormValue("title"))
	if !ok {
		http.Error(w, "Step title is required (max 200 chars)", http.StatusBadRequest)
		return
	}

	if _, err := h.sessionRepo.CreatePlaybookStep(ctx, locationID, title); err != nil {
		slog.Error("create playbook step failed", "error", err)
		http.Error(w, "Save failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/settings/playbook?saved=1")
	w.WriteHeader(http.StatusOK)
}

// PlaybookUpdate replaces a step's title
// (POST /settings/playbook/{stepID}/edit).
func (h *Handler) PlaybookUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireHeadSetter(w, r) {
		return
	}

	stepID := chi.URLParam(r, "stepID")
	step, err := h.sessionRepo.GetPlaybookStep(ctx, stepID)
	if err != nil {
		slog.Error("get playbook step failed", "error", err)
		http.Error(w, "Lookup failed", http.StatusInternalServerError)
		return
	}
	if step == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if !h.checkLocationOwnership(w, r, step.LocationID) {
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	title, ok := normaliseStepTitle(r.FormValue("title"))
	if !ok {
		http.Error(w, "Step title is required (max 200 chars)", http.StatusBadRequest)
		return
	}

	if err := h.sessionRepo.UpdatePlaybookStep(ctx, stepID, title); err != nil {
		slog.Error("update playbook step failed", "error", err)
		http.Error(w, "Save failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/settings/playbook?saved=1")
	w.WriteHeader(http.StatusOK)
}

// PlaybookDelete removes a step (POST /settings/playbook/{stepID}/delete).
func (h *Handler) PlaybookDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireHeadSetter(w, r) {
		return
	}

	stepID := chi.URLParam(r, "stepID")
	step, err := h.sessionRepo.GetPlaybookStep(ctx, stepID)
	if err != nil {
		slog.Error("get playbook step failed", "error", err)
		http.Error(w, "Lookup failed", http.StatusInternalServerError)
		return
	}
	if step == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if !h.checkLocationOwnership(w, r, step.LocationID) {
		return
	}

	if err := h.sessionRepo.DeletePlaybookStep(ctx, stepID); err != nil {
		slog.Error("delete playbook step failed", "error", err)
		http.Error(w, "Delete failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/settings/playbook?saved=1")
	w.WriteHeader(http.StatusOK)
}

// PlaybookMove swaps a step with its neighbour
// (POST /settings/playbook/{stepID}/move?dir=up|down).
func (h *Handler) PlaybookMove(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireHeadSetter(w, r) {
		return
	}

	stepID := chi.URLParam(r, "stepID")
	step, err := h.sessionRepo.GetPlaybookStep(ctx, stepID)
	if err != nil {
		slog.Error("get playbook step failed", "error", err)
		http.Error(w, "Lookup failed", http.StatusInternalServerError)
		return
	}
	if step == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if !h.checkLocationOwnership(w, r, step.LocationID) {
		return
	}

	dir := r.URL.Query().Get("dir")
	if dir != "up" && dir != "down" {
		http.Error(w, "Invalid direction", http.StatusBadRequest)
		return
	}

	if err := h.sessionRepo.MovePlaybookStep(ctx, stepID, dir); err != nil {
		slog.Error("move playbook step failed", "error", err)
		http.Error(w, "Move failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/settings/playbook")
	w.WriteHeader(http.StatusOK)
}

// normaliseStepTitle trims, validates length, and reports whether the
// resulting title is acceptable.
func normaliseStepTitle(raw string) (string, bool) {
	title := strings.TrimSpace(raw)
	if title == "" {
		return "", false
	}
	if len(title) > playbookTitleMaxLen {
		return "", false
	}
	return title, true
}
