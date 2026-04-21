package webhandler

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
	"github.com/shotwell-paddle/routewerk/internal/service/cardsheet"
)

// ── Route Card Batches ──────────────────────────────────────────
//
// These handlers drive the "print a batch of route cards on an 8-up sheet"
// flow. A batch is a persisted selection of route IDs; rendering happens
// synchronously on download (re-resolved from live route data each time) so
// reprints always reflect the current grade / setter / name.
//
// The MVP skips background rendering + storage caching — they live behind
// CardBatchRepo.StorageKey, which we leave nil and populate in a follow-up.

// CardBatchList renders the batch history page (GET /card-batches).
func (h *Handler) CardBatchList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a location.")
		return
	}

	batches, err := h.cardBatchRepo.ListByLocation(ctx, locationID, 50)
	if err != nil {
		slog.Error("card batch list failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not load card batches.")
		return
	}

	// Hydrate creator display names. Batches typically come from 1-3 setters
	// so a per-row lookup is fine; swap for a JOIN in the repo if listings
	// start hitting dozens of distinct creators.
	creatorNames := map[string]string{}
	items := make([]CardBatchListItem, 0, len(batches))
	for _, b := range batches {
		name, ok := creatorNames[b.CreatedBy]
		if !ok {
			if u, uErr := h.userRepo.GetByID(ctx, b.CreatedBy); uErr == nil && u != nil {
				name = u.DisplayName
			}
			creatorNames[b.CreatedBy] = name
		}
		items = append(items, CardBatchListItem{
			CardBatch:   b,
			CreatorName: name,
			CardCount:   len(b.RouteIDs),
			PageCount:   cardsheet.PageCount(len(b.RouteIDs)),
		})
	}

	data := &PageData{
		TemplateData: templateDataFromContext(r, "card-batches"),
		CardBatches:  items,
	}
	h.render(w, r, "setter/card-batches.html", data)
}

// CardBatchNewForm renders the new-batch form (GET /card-batches/new).
// Defaults the candidate route pool to "routes at this location that have
// never been included in a ready batch" — that's the most common setter
// workflow ("print cards for newly-set routes").
func (h *Handler) CardBatchNewForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a location.")
		return
	}

	candidates, err := h.loadBatchCandidates(r, locationID)
	if err != nil {
		slog.Error("card batch new: load candidates failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not load routes.")
		return
	}

	data := &PageData{
		TemplateData: templateDataFromContext(r, "card-batches"),
		BatchForm: CardBatchFormValues{
			Theme:           model.CardThemeTradingCard,
			CutterProfile:   model.CutterSilhouetteType2,
			CandidateRoutes: candidates,
			ShowAll:         r.URL.Query().Get("all") == "1",
		},
	}
	h.render(w, r, "setter/card-batch-form.html", data)
}

// CardBatchCreate handles the new-batch form POST (POST /card-batches/new).
// Validates the route IDs against the current location, persists the batch,
// and redirects to its detail page. A zero-route submission is a form error.
func (h *Handler) CardBatchCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" || user == nil {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a location.")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid form", "Could not parse form data.")
		return
	}

	theme := r.FormValue("theme")
	if theme == "" {
		theme = model.CardThemeTradingCard
	}
	cutter := r.FormValue("cutter_profile")
	if cutter == "" {
		cutter = model.CutterSilhouetteType2
	}
	routeIDs := r.Form["route_ids"]

	if len(routeIDs) == 0 {
		h.renderBatchFormError(w, r, locationID, "Select at least one route to include on the sheet.", CardBatchFormValues{
			Theme:         theme,
			CutterProfile: cutter,
			RouteIDs:      routeIDs,
		})
		return
	}

	// Filter to routes that actually live at this location. A bogus ID from a
	// hand-edited form would otherwise sneak into the stored UUID[] and
	// silently skip at render time — we'd rather fail the form upfront.
	valid, err := h.batchService.ValidateRouteIDs(ctx, locationID, routeIDs)
	if err != nil {
		slog.Error("card batch: validate route ids failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not validate selected routes.")
		return
	}
	if len(valid) == 0 {
		h.renderBatchFormError(w, r, locationID, "None of the selected routes are valid for this location.", CardBatchFormValues{
			Theme:         theme,
			CutterProfile: cutter,
			RouteIDs:      routeIDs,
		})
		return
	}

	batch := &model.CardBatch{
		LocationID:    locationID,
		CreatedBy:     user.ID,
		RouteIDs:      valid,
		Theme:         theme,
		CutterProfile: cutter,
		Status:        model.CardBatchStatusPending,
	}
	if err := h.cardBatchRepo.Create(ctx, batch); err != nil {
		slog.Error("card batch create failed", "error", err)
		h.renderBatchFormError(w, r, locationID, "Could not save batch. Please try again.", CardBatchFormValues{
			Theme:         theme,
			CutterProfile: cutter,
			RouteIDs:      valid,
		})
		return
	}

	// Audit log: mirrors the JSON API so batch creation is traceable
	// regardless of which surface the setter used.
	if h.auditSvc != nil {
		h.auditSvc.Record(r, service.AuditCardBatchCreate, "card_batch", batch.ID, "", map[string]interface{}{
			"location_id":    locationID,
			"route_count":    len(valid),
			"theme":          theme,
			"cutter_profile": cutter,
		})
	}

	http.Redirect(w, r, "/card-batches/"+batch.ID, http.StatusSeeOther)
}

// CardBatchDetail renders the review page for a batch (GET /card-batches/{id}).
func (h *Handler) CardBatchDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	batchID := chi.URLParam(r, "batchID")

	batch, err := h.cardBatchRepo.GetByID(ctx, batchID)
	if err != nil {
		slog.Error("card batch detail lookup failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not load batch.")
		return
	}
	if batch == nil {
		h.renderError(w, r, http.StatusNotFound, "Batch not found", "That batch doesn't exist.")
		return
	}
	if !h.checkLocationOwnership(w, r, batch.LocationID) {
		return
	}

	// Hydrate route previews + creator name for the detail view.
	previews := h.loadBatchRoutePreviews(r, batch.RouteIDs)
	creatorName := ""
	if u, uErr := h.userRepo.GetByID(ctx, batch.CreatedBy); uErr == nil && u != nil {
		creatorName = u.DisplayName
	}

	missing := len(batch.RouteIDs) - len(previews)
	if missing < 0 {
		missing = 0
	}
	view := &CardBatchDetailView{
		CardBatch:    *batch,
		CreatorName:  creatorName,
		Routes:       previews,
		PageCount:    cardsheet.PageCount(len(previews)),
		MissingCount: missing,
	}

	data := &PageData{
		TemplateData:    templateDataFromContext(r, "card-batches"),
		CardBatchDetail: view,
	}
	h.render(w, r, "setter/card-batch-detail.html", data)
}

// CardBatchDownload streams the rendered 8-up PDF for a batch
// (GET /card-batches/{id}/download.pdf).
//
// MVP: renders synchronously on each request. The response is still cheap for
// the typical 1-16 card batch (a few hundred ms). Once we have Tigris-backed
// caching we'll flip to "render once, serve storage_key".
func (h *Handler) CardBatchDownload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	batchID := chi.URLParam(r, "batchID")

	batch, err := h.cardBatchRepo.GetByID(ctx, batchID)
	if err != nil {
		slog.Error("card batch download lookup failed", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if batch == nil {
		http.Error(w, "batch not found", http.StatusNotFound)
		return
	}
	if !h.checkLocationOwnership(w, r, batch.LocationID) {
		return
	}

	// Rendering is CPU-heavy (PNG per card + PDF encode). Share the upload
	// semaphore so batch downloads can't starve the route-photo pipeline and
	// vice versa.
	if !h.acquireUploadSem(w, r) {
		return
	}
	defer h.releaseUploadSem()

	// Buffer the PDF so failures in the composer translate to a proper 5xx
	// rather than a truncated body with 200 OK.
	var buf bytes.Buffer
	count, err := h.batchService.RenderBatch(ctx, batch.LocationID, batch.RouteIDs, cardsheet.SheetConfig{
		Cutter: cardsheet.CutterProfile(batch.CutterProfile),
		Theme:  batch.Theme,
	}, &buf)
	if err != nil {
		slog.Error("card batch render failed", "batch_id", batchID, "error", err)
		// Best-effort: mark failed so the UI can explain why the download 5xxed.
		if mErr := h.cardBatchRepo.MarkFailed(ctx, batchID, err.Error()); mErr != nil {
			slog.Error("card batch mark-failed failed", "batch_id", batchID, "error", mErr)
		}
		http.Error(w, "render failed", http.StatusInternalServerError)
		return
	}
	if count == 0 {
		http.Error(w, "no cards rendered", http.StatusUnprocessableEntity)
		return
	}

	filename := fmt.Sprintf("routewerk-cards-%s.pdf", batch.ID)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	// No Cache-Control: the PDF is route-state-dependent and we want setters
	// to always pull a fresh copy after a re-print.
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write(buf.Bytes()); err != nil {
		slog.Error("card batch write failed", "batch_id", batchID, "error", err)
	}
}

// CardBatchEditForm renders the edit form for a batch (GET /card-batches/{id}/edit).
// Pre-selects the current route_ids and shows the full candidate pool —
// including already-carded routes — so setters can add or remove routes
// without having to re-create the batch. Same authz as delete/retry.
func (h *Handler) CardBatchEditForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	batchID := chi.URLParam(r, "batchID")

	batch, err := h.cardBatchRepo.GetByID(ctx, batchID)
	if err != nil {
		slog.Error("card batch edit lookup failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not load batch.")
		return
	}
	if batch == nil {
		h.renderError(w, r, http.StatusNotFound, "Batch not found", "That batch doesn't exist.")
		return
	}
	if !h.checkLocationOwnership(w, r, batch.LocationID) {
		return
	}

	user := middleware.GetWebUser(ctx)
	role := middleware.GetWebRole(ctx)
	if user == nil || !canDeleteCardBatch(batch.CreatedBy, user.ID, role) {
		h.renderError(w, r, http.StatusForbidden, "Not allowed",
			"Only the setter who created this batch or a head setter can edit it.")
		return
	}

	// For edit mode we always want the full active-route pool so setters can
	// both add new routes and see the ones already on the batch — not just
	// "routes without a card yet".
	q := r.URL.Query()
	q.Set("all", "1")
	r.URL.RawQuery = q.Encode()

	candidates, err := h.loadBatchCandidates(r, batch.LocationID)
	if err != nil {
		slog.Error("card batch edit: load candidates failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not load routes.")
		return
	}

	data := &PageData{
		TemplateData: templateDataFromContext(r, "card-batches"),
		BatchForm: CardBatchFormValues{
			Theme:           batch.Theme,
			CutterProfile:   batch.CutterProfile,
			RouteIDs:        batch.RouteIDs,
			CandidateRoutes: candidates,
			EditBatchID:     batch.ID,
			ShowAll:         true,
		},
	}
	h.render(w, r, "setter/card-batch-form.html", data)
}

// CardBatchUpdate handles the edit form POST (POST /card-batches/{id}/edit).
// Replaces the batch's route_ids with the validated selection and resets
// status/storage_key so the next download re-renders.
func (h *Handler) CardBatchUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	batchID := chi.URLParam(r, "batchID")

	batch, err := h.cardBatchRepo.GetByID(ctx, batchID)
	if err != nil {
		slog.Error("card batch update lookup failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not load batch.")
		return
	}
	if batch == nil {
		h.renderError(w, r, http.StatusNotFound, "Batch not found", "That batch doesn't exist.")
		return
	}
	if !h.checkLocationOwnership(w, r, batch.LocationID) {
		return
	}

	user := middleware.GetWebUser(ctx)
	role := middleware.GetWebRole(ctx)
	if user == nil || !canDeleteCardBatch(batch.CreatedBy, user.ID, role) {
		h.renderError(w, r, http.StatusForbidden, "Not allowed",
			"Only the setter who created this batch or a head setter can edit it.")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid form", "Could not parse form data.")
		return
	}
	routeIDs := r.Form["route_ids"]
	if len(routeIDs) == 0 {
		h.renderBatchEditError(w, r, batch, "Select at least one route.", routeIDs)
		return
	}

	valid, err := h.batchService.ValidateRouteIDs(ctx, batch.LocationID, routeIDs)
	if err != nil {
		slog.Error("card batch update: validate failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not validate selected routes.")
		return
	}
	if len(valid) == 0 {
		h.renderBatchEditError(w, r, batch, "None of the selected routes are valid for this location.", routeIDs)
		return
	}

	if err := h.cardBatchRepo.UpdateRouteIDs(ctx, batch.ID, valid); err != nil {
		slog.Error("card batch update failed", "error", err)
		h.renderBatchEditError(w, r, batch, "Could not save changes. Please try again.", valid)
		return
	}

	if h.auditSvc != nil {
		h.auditSvc.Record(r, service.AuditCardBatchCreate, "card_batch", batch.ID, "", map[string]interface{}{
			"location_id": batch.LocationID,
			"route_count": len(valid),
			"edit":        true,
		})
	}

	http.Redirect(w, r, "/card-batches/"+batch.ID, http.StatusSeeOther)
}

// renderBatchEditError re-renders the edit form with the user's values intact
// and a visible error. Mirrors renderBatchFormError but for the edit flow.
func (h *Handler) renderBatchEditError(w http.ResponseWriter, r *http.Request, batch *model.CardBatch, msg string, routeIDs []string) {
	q := r.URL.Query()
	q.Set("all", "1")
	r.URL.RawQuery = q.Encode()

	candidates, err := h.loadBatchCandidates(r, batch.LocationID)
	if err != nil {
		slog.Error("card batch edit form: reload candidates failed", "error", err)
	}
	data := &PageData{
		TemplateData: templateDataFromContext(r, "card-batches"),
		BatchForm: CardBatchFormValues{
			Theme:           batch.Theme,
			CutterProfile:   batch.CutterProfile,
			RouteIDs:        routeIDs,
			CandidateRoutes: candidates,
			EditBatchID:     batch.ID,
			ShowAll:         true,
		},
		BatchFormError: msg,
	}
	h.render(w, r, "setter/card-batch-form.html", data)
}

// CardBatchPreview renders a PNG preview of the first renderable card in
// the batch (GET /card-batches/{id}/preview.png). Drives the thumbnail on
// the batch detail page so setters can spot-check the output before
// committing paper to the cutter.
//
// Synchronously renders on each request. The render is cheap for a single
// card (~50 ms) and we set a short browser cache so the image doesn't
// re-fetch on every detail-page reload.
//
// Timeout handling: the global web RequestTimeout (QUERY_TIMEOUT, default 5s)
// is too tight for this endpoint — one cold-pool DB query on GetByID can
// easily eat most of that budget, leaving nothing for the remaining per-route
// lookups + PNG render. We decouple from the request deadline here and use a
// dedicated 15s budget, but still honor client cancellation via AfterFunc so
// a setter closing the page doesn't leak a render goroutine.
const previewRenderTimeout = 15 * time.Second

func (h *Handler) CardBatchPreview(w http.ResponseWriter, r *http.Request) {
	// Detach from the 5s request deadline, but keep client cancel propagation.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), previewRenderTimeout)
	defer cancel()
	stop := context.AfterFunc(r.Context(), cancel)
	defer stop()

	batchID := chi.URLParam(r, "batchID")

	batch, err := h.cardBatchRepo.GetByID(ctx, batchID)
	if err != nil {
		slog.Error("card batch preview lookup failed", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if batch == nil {
		http.Error(w, "batch not found", http.StatusNotFound)
		return
	}
	if !h.checkLocationOwnership(w, r, batch.LocationID) {
		return
	}

	if !h.acquireUploadSem(w, r) {
		return
	}
	defer h.releaseUploadSem()

	var buf bytes.Buffer
	if err := h.batchService.RenderPreviewPNG(ctx, batch.LocationID, batch.RouteIDs, &buf); err != nil {
		slog.Error("card batch preview render failed", "batch_id", batchID, "error", err)
		http.Error(w, "preview unavailable", http.StatusUnprocessableEntity)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	// Short cache: preview reflects live route data, but within a single
	// sitting the same setter may hit refresh several times while queueing
	// prints. 60s is long enough to help, short enough to self-heal.
	w.Header().Set("Cache-Control", "private, max-age=60")
	if _, err := w.Write(buf.Bytes()); err != nil {
		slog.Error("card batch preview write failed", "batch_id", batchID, "error", err)
	}
}

// CardBatchRetry resets a failed batch back to status=pending so the next
// download attempt re-runs the render (POST /card-batches/{id}/retry).
// This is a manual equivalent of InvalidateStorageKey — useful when a
// transient error (storage blip, DB timeout) left the batch stuck in
// status=failed and setters want to try again without deleting and
// re-creating.
//
// Authorization mirrors delete: creator or head_setter+. We reuse that rule
// so setters can't reset each other's batches but head setters can clean
// up on behalf of the team.
func (h *Handler) CardBatchRetry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	batchID := chi.URLParam(r, "batchID")

	batch, err := h.cardBatchRepo.GetByID(ctx, batchID)
	if err != nil {
		slog.Error("card batch retry lookup failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not load batch.")
		return
	}
	if batch == nil {
		h.renderError(w, r, http.StatusNotFound, "Batch not found", "That batch doesn't exist.")
		return
	}
	if !h.checkLocationOwnership(w, r, batch.LocationID) {
		return
	}

	user := middleware.GetWebUser(ctx)
	role := middleware.GetWebRole(ctx)
	if user == nil || !canDeleteCardBatch(batch.CreatedBy, user.ID, role) {
		h.renderError(w, r, http.StatusForbidden, "Not allowed",
			"Only the setter who created this batch or a head setter can retry it.")
		return
	}

	if err := h.cardBatchRepo.InvalidateStorageKey(ctx, batchID); err != nil {
		slog.Error("card batch retry failed", "batch_id", batchID, "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not reset batch.")
		return
	}

	http.Redirect(w, r, "/card-batches/"+batchID, http.StatusSeeOther)
}

// CardBatchDelete removes a batch (POST /card-batches/{id}/delete).
// The rendered PDF in storage (if any) is NOT purged — the batch feature
// doesn't populate StorageKey in the MVP, so there's nothing to clean up yet.
//
// Authorization: only the batch's creator or a head_setter+ can delete.
// Rationale: the batch-create page lives in setter-and-above territory, so
// every setter can create — but rank-and-file setters shouldn't be able to
// clobber each other's batches. Head setters (and above) need delete
// authority for cleanup across the team.
func (h *Handler) CardBatchDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	batchID := chi.URLParam(r, "batchID")

	batch, err := h.cardBatchRepo.GetByID(ctx, batchID)
	if err != nil {
		slog.Error("card batch delete lookup failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not load batch.")
		return
	}
	if batch == nil {
		h.renderError(w, r, http.StatusNotFound, "Batch not found", "That batch doesn't exist.")
		return
	}
	if !h.checkLocationOwnership(w, r, batch.LocationID) {
		return
	}

	user := middleware.GetWebUser(ctx)
	role := middleware.GetWebRole(ctx)
	if user == nil || !canDeleteCardBatch(batch.CreatedBy, user.ID, role) {
		h.renderError(w, r, http.StatusForbidden, "Not allowed",
			"Only the setter who created this batch or a head setter can delete it.")
		return
	}

	if err := h.cardBatchRepo.Delete(ctx, batchID); err != nil {
		slog.Error("card batch delete failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not delete batch.")
		return
	}

	if h.auditSvc != nil {
		h.auditSvc.Record(r, service.AuditCardBatchDelete, "card_batch", batchID, "", map[string]interface{}{
			"location_id": batch.LocationID,
			"creator_id":  batch.CreatedBy,
		})
	}

	http.Redirect(w, r, "/card-batches", http.StatusSeeOther)
}

// canDeleteCardBatch encodes the "creator-or-head_setter+" delete rule so it
// can be unit-tested without hitting the DB or HTTP stack.
func canDeleteCardBatch(creatorID, actorID, actorRole string) bool {
	if creatorID != "" && creatorID == actorID {
		return true
	}
	return middleware.RoleRankValue(actorRole) >= middleware.RoleRankValue(middleware.RoleHeadSetter)
}

// ── Helpers ───────────────────────────────────────────────────

// loadBatchCandidates returns the pool of routes available to select when
// creating a new batch. Default behaviour: "routes with no card yet" at this
// location. When ?all=1 is set, returns every active route at the location
// so setters can reprint cards for already-carded routes after a regrade.
//
// The two filters are STRICT — "Uncarded" returning an empty list must stay
// empty so the template can show a real empty-state rather than silently
// swapping to the full active pool (which looks identical to "All active"
// and makes the filter chips feel broken).
//
// Routes are sorted wall-asc → date-set-desc → grade-asc so the picker
// groups by wall naturally and the printed sheet mirrors how setters walk
// the gym (wall by wall, newest routes first).
func (h *Handler) loadBatchCandidates(r *http.Request, locationID string) ([]CardBatchRoutePreview, error) {
	ctx := r.Context()

	showAll := r.URL.Query().Get("all") == "1"

	var routeIDs []string
	if showAll {
		activeRoutes, _, err := h.routeRepo.List(ctx, repository.RouteFilter{
			LocationID: locationID,
			Status:     "active",
			Limit:      200,
		})
		if err != nil {
			return nil, err
		}
		routeIDs = make([]string, 0, len(activeRoutes))
		for _, rt := range activeRoutes {
			routeIDs = append(routeIDs, rt.ID)
		}
	} else {
		ids, err := h.cardBatchRepo.RoutesWithoutCard(ctx, locationID)
		if err != nil {
			return nil, err
		}
		routeIDs = ids
	}

	previews := h.loadBatchRoutePreviews(r, routeIDs)
	sortBatchRoutePreviews(previews)
	return previews, nil
}

// sortBatchRoutePreviews orders rows for the picker: wall name ascending,
// then newest set date first within a wall, then grade ascending for
// stability. Grade comparison is lexical — good enough for "5.9", "5.10a",
// and circuit colors, and the wall grouping already dominates the visual
// order anyway.
func sortBatchRoutePreviews(previews []CardBatchRoutePreview) {
	sort.SliceStable(previews, func(i, j int) bool {
		if previews[i].WallName != previews[j].WallName {
			return previews[i].WallName < previews[j].WallName
		}
		if !previews[i].DateSet.Equal(previews[j].DateSet) {
			return previews[i].DateSet.After(previews[j].DateSet)
		}
		return previews[i].Grade < previews[j].Grade
	})
}

// loadBatchRoutePreviews hydrates a list of route IDs into preview rows. Wall
// names are cached across the list; missing routes (deleted since the batch
// was saved) are silently dropped so the review page stays renderable.
func (h *Handler) loadBatchRoutePreviews(r *http.Request, routeIDs []string) []CardBatchRoutePreview {
	ctx := r.Context()

	wallNames := map[string]string{}
	previews := make([]CardBatchRoutePreview, 0, len(routeIDs))
	for _, id := range routeIDs {
		rt, err := h.routeRepo.GetByID(ctx, id)
		if err != nil || rt == nil {
			continue
		}
		wname, ok := wallNames[rt.WallID]
		if !ok {
			if w, wErr := h.wallRepo.GetByID(ctx, rt.WallID); wErr == nil && w != nil {
				wname = w.Name
			}
			wallNames[rt.WallID] = wname
		}
		name := ""
		if rt.Name != nil {
			name = *rt.Name
		}
		grade := rt.Grade
		isCircuit := rt.GradingSystem == "circuit"
		if isCircuit && rt.CircuitColor != nil && *rt.CircuitColor != "" {
			// Display circuit routes by their color name so the picker matches
			// the on-wall convention (climbers don't see V-grades there).
			grade = strings.ToUpper(*rt.CircuitColor)
		}
		previews = append(previews, CardBatchRoutePreview{
			ID:        rt.ID,
			Name:      name,
			Grade:     grade,
			Color:     rt.Color,
			WallName:  wname,
			DateSet:   rt.DateSet,
			IsCircuit: isCircuit,
		})
	}
	return previews
}

// renderBatchFormError re-renders the batch form with a visible error and
// the user's previously submitted values intact so they don't have to
// re-check every checkbox.
func (h *Handler) renderBatchFormError(w http.ResponseWriter, r *http.Request, locationID, msg string, values CardBatchFormValues) {
	candidates, err := h.loadBatchCandidates(r, locationID)
	if err != nil {
		slog.Error("card batch form: reload candidates failed", "error", err)
	}
	values.CandidateRoutes = candidates
	values.ShowAll = r.URL.Query().Get("all") == "1"

	data := &PageData{
		TemplateData:   templateDataFromContext(r, "card-batches"),
		BatchForm:      values,
		BatchFormError: msg,
	}
	h.render(w, r, "setter/card-batch-form.html", data)
}
