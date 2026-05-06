package handler

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
	"github.com/shotwell-paddle/routewerk/internal/service/cardbatch"
	"github.com/shotwell-paddle/routewerk/internal/service/cardsheet"
)

// CardBatchHandler serves the JSON API for print-and-cut card batches.
// Mirrors the web flow (create → list → download PDF) but returns JSON
// everywhere except /pdf, which streams the rendered document.
type CardBatchHandler struct {
	batches *repository.CardBatchRepo
	svc     *cardbatch.Service
	audit   *service.AuditService
}

func NewCardBatchHandler(batches *repository.CardBatchRepo, svc *cardbatch.Service, audit *service.AuditService) *CardBatchHandler {
	return &CardBatchHandler{batches: batches, svc: svc, audit: audit}
}

// ── Request / response shapes ────────────────────────────────

// createBatchRequest is the JSON body for POST /locations/{id}/card-batches.
type createBatchRequest struct {
	RouteIDs      []string `json:"route_ids"`
	Theme         string   `json:"theme,omitempty"`
	CutterProfile string   `json:"cutter_profile,omitempty"`
}

// batchResponse is the API representation of a single batch. Mirrors the
// CardBatch model verbatim today; kept as its own type so changes to the
// storage model don't leak into the API contract.
type batchResponse struct {
	ID            string   `json:"id"`
	LocationID    string   `json:"location_id"`
	CreatedBy     string   `json:"created_by"`
	RouteIDs      []string `json:"route_ids"`
	Theme         string   `json:"theme"`
	CutterProfile string   `json:"cutter_profile"`
	Status        string   `json:"status"`
	StorageKey    *string  `json:"storage_key,omitempty"`
	ErrorMessage  *string  `json:"error_message,omitempty"`
	PageCount     int      `json:"page_count"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
}

func toBatchResponse(b model.CardBatch) batchResponse {
	return batchResponse{
		ID:            b.ID,
		LocationID:    b.LocationID,
		CreatedBy:     b.CreatedBy,
		RouteIDs:      b.RouteIDs,
		Theme:         b.Theme,
		CutterProfile: b.CutterProfile,
		Status:        b.Status,
		StorageKey:    b.StorageKey,
		ErrorMessage:  b.ErrorMessage,
		PageCount:     cardsheet.PageCount(len(b.RouteIDs)),
		CreatedAt:     b.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:     b.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// ── Handlers ──────────────────────────────────────────────────

// List returns the most recent batches for a location (GET /locations/{id}/card-batches).
func (h *CardBatchHandler) List(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	batches, err := h.batches.ListByLocation(r.Context(), locationID, 50)
	if err != nil {
		slog.Error("card batch list: db error", "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]batchResponse, 0, len(batches))
	for _, b := range batches {
		out = append(out, toBatchResponse(b))
	}
	JSON(w, http.StatusOK, map[string]any{"batches": out})
}

// Create persists a new batch (POST /locations/{id}/card-batches).
// The request body lists route IDs + theme/cutter; invalid IDs are filtered
// out silently but an empty resulting set is rejected as a 422.
func (h *CardBatchHandler) Create(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req createBatchRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.RouteIDs) == 0 {
		Error(w, http.StatusBadRequest, "route_ids is required")
		return
	}
	if len(req.RouteIDs) > cardbatch.MaxBatchCards {
		// Mirror the web form cap. 413 tells API clients "your payload is
		// too big"; the message tells humans what to do about it.
		Error(w, http.StatusRequestEntityTooLarge, fmt.Sprintf(
			"route_ids exceeds max batch size (%d, max %d) — split into multiple batches",
			len(req.RouteIDs), cardbatch.MaxBatchCards,
		))
		return
	}
	if req.Theme == "" {
		req.Theme = model.CardThemeTradingCard
	}
	if req.CutterProfile == "" {
		req.CutterProfile = model.CutterSilhouetteType2
	}

	valid, err := h.svc.ValidateRouteIDs(r.Context(), locationID, req.RouteIDs)
	if err != nil {
		slog.Error("card batch create: validate failed", "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if len(valid) == 0 {
		Error(w, http.StatusUnprocessableEntity, "no valid routes for this location")
		return
	}

	batch := &model.CardBatch{
		LocationID:    locationID,
		CreatedBy:     userID,
		RouteIDs:      valid,
		Theme:         req.Theme,
		CutterProfile: req.CutterProfile,
		Status:        model.CardBatchStatusPending,
	}
	if err := h.batches.Create(r.Context(), batch); err != nil {
		slog.Error("card batch create: db error", "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Audit log: setters can trace a print batch back to who requested it and
	// with how many routes. Cheap record with no sensitive data.
	if h.audit != nil {
		h.audit.Record(r, service.AuditCardBatchCreate, "card_batch", batch.ID, "", map[string]interface{}{
			"location_id":    locationID,
			"route_count":    len(valid),
			"theme":          req.Theme,
			"cutter_profile": req.CutterProfile,
		})
	}

	JSON(w, http.StatusCreated, toBatchResponse(*batch))
}

// Get returns a single batch (GET /locations/{id}/card-batches/{batchID}).
func (h *CardBatchHandler) Get(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	batchID := chi.URLParam(r, "batchID")

	b, err := h.batches.GetByID(r.Context(), batchID)
	if err != nil {
		slog.Error("card batch get: db error", "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if b == nil || b.LocationID != locationID {
		// Hide cross-location access under 404 to avoid existence disclosure.
		Error(w, http.StatusNotFound, "batch not found")
		return
	}
	JSON(w, http.StatusOK, toBatchResponse(*b))
}

// Download renders the batch to PDF and streams it back
// (GET /locations/{id}/card-batches/{batchID}/pdf).
//
// Rendering is synchronous. Batches are small enough (≤16 cards in practice)
// that a single-request render is cheaper than the machinery required for
// async+poll. Once we cache to Tigris, this handler will redirect to a
// pre-signed URL instead.
func (h *CardBatchHandler) Download(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	batchID := chi.URLParam(r, "batchID")

	b, err := h.batches.GetByID(r.Context(), batchID)
	if err != nil {
		slog.Error("card batch download: db error", "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if b == nil || b.LocationID != locationID {
		Error(w, http.StatusNotFound, "batch not found")
		return
	}

	// Render to a tmpfile instead of an in-memory buffer — see the
	// equivalent block in internal/handler/web/route_card_batches.go for
	// rationale. Same 256MB-VM-on-Fly concern applies to the JSON API path.
	tmp, err := os.CreateTemp("", "routewerk-cardbatch-*.pdf")
	if err != nil {
		slog.Error("card batch download: tmpfile create failed", "batch_id", batchID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}()

	count, err := h.svc.RenderBatch(r.Context(), b.LocationID, b.RouteIDs, cardsheet.SheetConfig{
		Cutter: cardsheet.CutterProfile(b.CutterProfile),
		Theme:  b.Theme,
	}, tmp)
	if err != nil {
		slog.Error("card batch download: render failed", "batch_id", batchID, "error", err)
		if mErr := h.batches.MarkFailed(r.Context(), batchID, err.Error()); mErr != nil {
			slog.Error("card batch mark-failed failed", "batch_id", batchID, "error", mErr)
		}
		Error(w, http.StatusInternalServerError, "render failed")
		return
	}
	if count == 0 {
		Error(w, http.StatusUnprocessableEntity, "no cards rendered")
		return
	}

	if _, err := tmp.Seek(0, 0); err != nil {
		slog.Error("card batch download: tmpfile seek failed", "batch_id", batchID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	filename := fmt.Sprintf("routewerk-cards-%s.pdf", b.ID)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Cache-Control", "no-store")
	if _, err := io.Copy(w, tmp); err != nil {
		slog.Error("card batch download: write failed", "batch_id", batchID, "error", err)
	}
}

// Delete removes a batch (DELETE /locations/{id}/card-batches/{batchID}).
//
// Authorization: creator or head_setter+ only. The route-level authz chain
// already ensures the actor is at least a setter at this location, so this
// check is strictly about "not your batch" guardrails.
func (h *CardBatchHandler) Delete(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	batchID := chi.URLParam(r, "batchID")

	b, err := h.batches.GetByID(r.Context(), batchID)
	if err != nil {
		slog.Error("card batch delete: db error", "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if b == nil || b.LocationID != locationID {
		Error(w, http.StatusNotFound, "batch not found")
		return
	}

	actorID := middleware.GetUserID(r.Context())
	actorRole := ""
	if m := middleware.GetMembership(r.Context()); m != nil {
		actorRole = m.Role
	}
	if !CanDeleteCardBatch(b.CreatedBy, actorID, actorRole) {
		Error(w, http.StatusForbidden, "only the creator or a head setter can delete this batch")
		return
	}

	if err := h.batches.Delete(r.Context(), batchID); err != nil {
		slog.Error("card batch delete: db error", "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	if h.audit != nil {
		h.audit.Record(r, service.AuditCardBatchDelete, "card_batch", batchID, "", map[string]interface{}{
			"location_id": locationID,
			"creator_id":  b.CreatedBy,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

type updateBatchRequest struct {
	RouteIDs []string `json:"route_ids"`
}

// Update — PATCH /api/v1/locations/{id}/card-batches/{id}.
//
// Replaces the batch's route_ids and resets the storage key so the next
// download re-renders. Mirrors the HTMX edit flow at
// internal/handler/web/route_card_batches.go::CardBatchUpdate. Authz:
// creator or head_setter+ (same gate as Delete).
func (h *CardBatchHandler) Update(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	batchID := chi.URLParam(r, "batchID")

	b, err := h.batches.GetByID(r.Context(), batchID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if b == nil || b.LocationID != locationID {
		Error(w, http.StatusNotFound, "batch not found")
		return
	}

	actorID := middleware.GetUserID(r.Context())
	actorRole := ""
	if m := middleware.GetMembership(r.Context()); m != nil {
		actorRole = m.Role
	}
	if !CanDeleteCardBatch(b.CreatedBy, actorID, actorRole) {
		Error(w, http.StatusForbidden, "only the creator or a head setter can edit this batch")
		return
	}

	var req updateBatchRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.RouteIDs) == 0 {
		Error(w, http.StatusBadRequest, "route_ids is required")
		return
	}
	if len(req.RouteIDs) > cardbatch.MaxBatchCards {
		Error(w, http.StatusRequestEntityTooLarge, fmt.Sprintf(
			"route_ids exceeds max batch size (%d, max %d)",
			len(req.RouteIDs), cardbatch.MaxBatchCards,
		))
		return
	}

	valid, err := h.svc.ValidateRouteIDs(r.Context(), b.LocationID, req.RouteIDs)
	if err != nil {
		slog.Error("card batch update: validate failed", "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if len(valid) == 0 {
		Error(w, http.StatusUnprocessableEntity, "no valid routes for this location")
		return
	}

	if err := h.batches.UpdateRouteIDs(r.Context(), b.ID, valid); err != nil {
		slog.Error("card batch update: db error", "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	if h.audit != nil {
		h.audit.Record(r, service.AuditCardBatchCreate, "card_batch", b.ID, "", map[string]interface{}{
			"location_id": locationID,
			"route_count": len(valid),
			"edit":        true,
		})
	}

	// Re-fetch so the response includes the new route count + reset
	// status the caller can use to refresh their view.
	updated, err := h.batches.GetByID(r.Context(), b.ID)
	if err != nil || updated == nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	JSON(w, http.StatusOK, toBatchResponse(*updated))
}

// CanDeleteCardBatch encodes the shared "creator-or-head_setter+" rule used
// by both the JSON API and the web handler. Exported for use from the web
// package; the duplication there is intentional to keep web-package tests
// from reaching across package boundaries.
func CanDeleteCardBatch(creatorID, actorID, actorRole string) bool {
	if creatorID != "" && creatorID == actorID {
		return true
	}
	return middleware.RoleRankValue(actorRole) >= middleware.RoleRankValue(middleware.RoleHeadSetter)
}
