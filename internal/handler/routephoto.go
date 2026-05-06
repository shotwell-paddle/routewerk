package handler

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

// RoutePhotoHandler exposes the JSON variant of the HTMX route-photo
// pipeline. The underlying validation (content-type sniffing, size cap,
// image processing, storage upload, photo-cap-per-route) is the same as
// internal/handler/web/photos.go — kept in lockstep so an SPA upload and
// an HTMX upload land equivalent rows in route_photos.
type RoutePhotoHandler struct {
	routes  *repository.RouteRepo
	photos  *repository.RoutePhotoRepo
	storage *service.StorageService
	// uploadSem caps in-flight image processing so a burst of large
	// uploads can't OOM the 256 MB Fly VM. Shared with the HTMX handler
	// would be ideal, but keeping a separate semaphore here is fine —
	// total in-flight across both code paths is still bounded by the
	// http.Server's concurrent-connection limit upstream.
	uploadSem chan struct{}
}

// Mirror the limits + allow-list from the HTMX handler so JSON uploads
// can't sneak past constraints the HTMX form respects.
const (
	jsonPhotoMaxUpload  = 5 << 20 // 5 MB
	jsonPhotoMaxPerRoot = 20      // matches webhandler photo cap
)

var jsonAllowedImageTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

func NewRoutePhotoHandler(
	routes *repository.RouteRepo,
	photos *repository.RoutePhotoRepo,
	storage *service.StorageService,
	uploadConcurrency int,
) *RoutePhotoHandler {
	if uploadConcurrency <= 0 {
		uploadConcurrency = 4
	}
	return &RoutePhotoHandler{
		routes:    routes,
		photos:    photos,
		storage:   storage,
		uploadSem: make(chan struct{}, uploadConcurrency),
	}
}

// Upload — POST /api/v1/locations/{locationID}/routes/{routeID}/photos.
//
// Multipart form with a single `photo` part. Any authenticated member of
// the location can upload (matches the HTMX policy: climbers send beta
// shots too). Returns 201 with the new photo's URL + ID.
func (h *RoutePhotoHandler) Upload(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	routeID := chi.URLParam(r, "routeID")
	if !isUUID(locationID) || !isUUID(routeID) {
		Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		Error(w, http.StatusUnauthorized, "authentication required")
		return
	}

	if !h.storage.IsConfigured() {
		Error(w, http.StatusServiceUnavailable, "photo uploads are not currently available")
		return
	}

	// Verify the route exists and belongs to the URL's location. Cross-
	// location uploads are blocked here, in addition to the location-
	// member middleware on the parent route.
	route, err := h.routes.GetByID(r.Context(), routeID)
	if err != nil || route == nil {
		Error(w, http.StatusNotFound, "route not found")
		return
	}
	if route.LocationID != locationID {
		Error(w, http.StatusNotFound, "route not found")
		return
	}

	if err := r.ParseMultipartForm(jsonPhotoMaxUpload); err != nil {
		if errors.Is(err, http.ErrNotMultipart) {
			Error(w, http.StatusBadRequest, "upload must be multipart/form-data")
			return
		}
		Error(w, http.StatusBadRequest, "file too large (max 5 MB)")
		return
	}

	file, header, err := r.FormFile("photo")
	if err != nil {
		Error(w, http.StatusBadRequest, "missing photo file")
		return
	}
	defer file.Close()

	// Sniff the content type rather than trusting the multipart header —
	// matches the HTMX handler. The sniffed type is what we hand to
	// ProcessImage downstream.
	sniff := make([]byte, 512)
	n, sniffErr := io.ReadFull(file, sniff)
	if sniffErr != nil && sniffErr != io.ErrUnexpectedEOF && sniffErr != io.EOF {
		Error(w, http.StatusBadRequest, "could not read upload")
		return
	}
	sniff = sniff[:n]
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		slog.Error("photo upload: seek failed", "route_id", routeID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	contentType := http.DetectContentType(sniff)
	if !jsonAllowedImageTypes[contentType] {
		Error(w, http.StatusBadRequest, "unsupported image format — use JPEG, PNG, or WebP")
		return
	}

	if header.Size > jsonPhotoMaxUpload {
		Error(w, http.StatusRequestEntityTooLarge, "file too large (max 5 MB)")
		return
	}

	count, err := h.photos.CountByRoute(r.Context(), routeID)
	if err != nil {
		slog.Error("photo upload: count failed", "route_id", routeID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if count >= jsonPhotoMaxPerRoot {
		Error(w, http.StatusUnprocessableEntity, fmt.Sprintf("maximum %d photos per route", jsonPhotoMaxPerRoot))
		return
	}

	select {
	case h.uploadSem <- struct{}{}:
		defer func() { <-h.uploadSem }()
	case <-r.Context().Done():
		Error(w, http.StatusServiceUnavailable, "request cancelled")
		return
	}

	processed, err := service.ProcessImage(file, contentType)
	if err != nil {
		slog.Error("photo upload: image processing failed", "route_id", routeID, "error", err)
		Error(w, http.StatusBadRequest, "could not process image — please try a different file")
		return
	}

	uploadFilename := strings.TrimSuffix(header.Filename, ".webp") + processed.Extension
	storageKey, photoURL, err := h.storage.Upload(r.Context(), routeID, uploadFilename, processed.ContentType, processed.Data)
	if err != nil {
		slog.Error("photo upload: storage failed", "route_id", routeID, "error", err)
		Error(w, http.StatusInternalServerError, "upload failed — please try again")
		return
	}

	sortOrder, _ := h.photos.NextSortOrder(r.Context(), routeID)
	uploaderID := userID
	photo := &model.RoutePhoto{
		RouteID:    routeID,
		PhotoURL:   photoURL,
		StorageKey: &storageKey,
		UploadedBy: &uploaderID,
		SortOrder:  sortOrder,
	}
	if err := h.photos.Create(r.Context(), photo); err != nil {
		slog.Error("photo upload: db insert failed", "route_id", routeID, "error", err)
		// Orphan cleanup — drop the just-uploaded blob if the row insert failed.
		_ = h.storage.Delete(r.Context(), storageKey)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	// First photo on a route also becomes the primary (route.PhotoURL).
	// Mirrors the HTMX setter create flow.
	if route.PhotoURL == nil || *route.PhotoURL == "" {
		route.PhotoURL = &photoURL
		if err := h.routes.Update(r.Context(), route); err != nil {
			slog.Error("photo upload: set primary failed", "route_id", routeID, "error", err)
		}
	}

	JSON(w, http.StatusCreated, map[string]any{
		"id":         photo.ID,
		"route_id":   photo.RouteID,
		"photo_url":  photo.PhotoURL,
		"sort_order": photo.SortOrder,
		"created_at": photo.CreatedAt,
	})
}

// List — GET /api/v1/locations/{locationID}/routes/{routeID}/photos.
//
// Returns all photos for a route in display order. Any authenticated
// location member can read.
func (h *RoutePhotoHandler) List(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	routeID := chi.URLParam(r, "routeID")
	if !isUUID(locationID) || !isUUID(routeID) {
		Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	route, err := h.routes.GetByID(r.Context(), routeID)
	if err != nil || route == nil || route.LocationID != locationID {
		Error(w, http.StatusNotFound, "route not found")
		return
	}

	photos, err := h.photos.ListByRoute(r.Context(), routeID)
	if err != nil {
		slog.Error("photo list failed", "route_id", routeID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]map[string]any, 0, len(photos))
	for _, p := range photos {
		out = append(out, map[string]any{
			"id":            p.ID,
			"photo_url":     p.PhotoURL,
			"sort_order":    p.SortOrder,
			"caption":       p.Caption,
			"uploaded_by":   p.UploadedBy,
			"uploader_name": p.UploaderName,
			"created_at":    p.CreatedAt,
		})
	}
	JSON(w, http.StatusOK, out)
}

// Delete — DELETE /api/v1/locations/{locationID}/routes/{routeID}/photos/{photoID}.
//
// Setter+ at the location, OR the photo's uploader, can delete. Mirrors
// the HTMX handler's policy at internal/handler/web/photos.go::PhotoDelete.
func (h *RoutePhotoHandler) Delete(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	routeID := chi.URLParam(r, "routeID")
	photoID := chi.URLParam(r, "photoID")
	if !isUUID(locationID) || !isUUID(routeID) || !isUUID(photoID) {
		Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		Error(w, http.StatusUnauthorized, "authentication required")
		return
	}

	route, err := h.routes.GetByID(r.Context(), routeID)
	if err != nil || route == nil || route.LocationID != locationID {
		Error(w, http.StatusNotFound, "route not found")
		return
	}

	photo, err := h.photos.GetByID(r.Context(), photoID)
	if err != nil || photo == nil || photo.RouteID != routeID {
		Error(w, http.StatusNotFound, "photo not found")
		return
	}

	// Authz: setter+ at the location OR the original uploader.
	m := middleware.GetMembership(r.Context())
	isStaff := m != nil && middleware.RoleRankValue(m.Role) >= 2
	isOwner := photo.UploadedBy != nil && *photo.UploadedBy == userID
	if !isStaff && !isOwner {
		Error(w, http.StatusForbidden, "you can't delete this photo")
		return
	}

	if err := h.photos.Delete(r.Context(), photoID); err != nil {
		slog.Error("photo delete: db failed", "photo_id", photoID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Drop the underlying blob if we have the key. Older rows didn't store
	// a key, so fall back to URL parsing — keeps cleanup parity with the
	// HTMX handler.
	if photo.StorageKey != nil && *photo.StorageKey != "" {
		_ = h.storage.Delete(r.Context(), *photo.StorageKey)
	} else if key := h.storage.KeyFromURL(photo.PhotoURL); key != "" {
		_ = h.storage.Delete(r.Context(), key)
	}

	// If we just deleted the primary photo, fall back to the next photo
	// in the list (or null if none remain).
	if route.PhotoURL != nil && *route.PhotoURL == photo.PhotoURL {
		remaining, _ := h.photos.ListByRoute(r.Context(), routeID)
		if len(remaining) > 0 {
			next := remaining[0].PhotoURL
			route.PhotoURL = &next
		} else {
			route.PhotoURL = nil
		}
		if err := h.routes.Update(r.Context(), route); err != nil {
			slog.Error("photo delete: route primary update failed", "route_id", routeID, "error", err)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}
