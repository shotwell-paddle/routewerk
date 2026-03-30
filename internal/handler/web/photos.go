package webhandler

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

const maxUploadSize = 5 << 20 // 5 MB

// allowedImageTypes is the allow-list of MIME types for photo uploads.
var allowedImageTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

// PhotoUpload handles POST /routes/{routeID}/photos.
// Any authenticated user can upload a photo for a route at their active location.
func (h *Handler) PhotoUpload(w http.ResponseWriter, r *http.Request) {
	routeID := chi.URLParam(r, "routeID")
	if routeID == "" || !validRouteID.MatchString(routeID) {
		http.Error(w, "Invalid route", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Verify the route exists and belongs to the user's location
	route, err := h.routeRepo.GetByID(ctx, routeID)
	if err != nil || route == nil {
		h.renderError(w, r, http.StatusNotFound, "Route not found", "This route doesn't exist.")
		return
	}
	if !h.checkLocationOwnership(w, r, route.LocationID) {
		return
	}

	// Check storage is configured
	if !h.storageService.IsConfigured() {
		slog.Warn("photo upload attempted but storage not configured")
		http.Error(w, "Photo uploads are not currently available", http.StatusServiceUnavailable)
		return
	}

	// Parse multipart form
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		http.Error(w, "File too large (max 5 MB)", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("photo")
	if err != nil {
		http.Error(w, "Missing photo file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Validate content type
	contentType := header.Header.Get("Content-Type")
	if !allowedImageTypes[contentType] {
		http.Error(w, "Only JPEG, PNG, and WebP images are allowed", http.StatusBadRequest)
		return
	}

	// Validate file size
	if header.Size > maxUploadSize {
		http.Error(w, "File too large (max 5 MB)", http.StatusBadRequest)
		return
	}

	// Limit photos per route (prevent abuse)
	count, err := h.photoRepo.CountByRoute(ctx, routeID)
	if err != nil {
		slog.Error("failed to count route photos", "route_id", routeID, "error", err)
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}
	if count >= 20 {
		http.Error(w, "Maximum 20 photos per route", http.StatusBadRequest)
		return
	}

	// Acquire upload semaphore to cap concurrent image processing memory.
	select {
	case h.uploadSem <- struct{}{}:
		defer func() { <-h.uploadSem }()
	case <-ctx.Done():
		http.Error(w, "Request cancelled", http.StatusServiceUnavailable)
		return
	}

	// Resize and compress the image before uploading.
	processed, err := service.ProcessImage(file, contentType)
	if err != nil {
		slog.Error("image processing failed", "route_id", routeID, "error", err)
		http.Error(w, "Could not process image — please try a different file", http.StatusBadRequest)
		return
	}

	// Use the processed image's content type and extension for the upload key.
	uploadFilename := strings.TrimSuffix(header.Filename, ".webp") + processed.Extension

	// Upload to S3
	photoURL, err := h.storageService.Upload(ctx, routeID, uploadFilename, processed.ContentType, processed.Data)
	if err != nil {
		slog.Error("photo upload failed", "route_id", routeID, "error", err)
		http.Error(w, "Upload failed — please try again", http.StatusInternalServerError)
		return
	}

	// Get next sort order
	sortOrder, err := h.photoRepo.NextSortOrder(ctx, routeID)
	if err != nil {
		slog.Error("failed to get next sort order", "route_id", routeID, "error", err)
		sortOrder = 0
	}

	// Caption from form
	caption := strings.TrimSpace(r.FormValue("caption"))
	var captionPtr *string
	if caption != "" {
		captionPtr = &caption
	}

	// Save to database
	photo := &model.RoutePhoto{
		RouteID:    routeID,
		PhotoURL:   photoURL,
		Caption:    captionPtr,
		UploadedBy: &user.ID,
		SortOrder:  sortOrder,
	}
	if err := h.photoRepo.Create(ctx, photo); err != nil {
		slog.Error("failed to save photo record", "route_id", routeID, "error", err)
		// Try to clean up the uploaded file
		if delErr := h.storageService.Delete(ctx, photoURL); delErr != nil {
			slog.Error("failed to clean up orphaned S3 photo", "route_id", routeID, "url", photoURL, "error", delErr)
		}
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}

	// If this is the first photo, also set it as the route's primary photo_url
	if count == 0 {
		route.PhotoURL = &photoURL
		if err := h.routeRepo.Update(ctx, route); err != nil {
			slog.Error("failed to set route primary photo", "route_id", routeID, "error", err)
		}
	}

	// Redirect back to the page the upload came from.
	// If the referer is a session photos page, go back there.
	redirect := fmt.Sprintf("/routes/%s", routeID)
	if ref := r.Header.Get("Referer"); ref != "" {
		if idx := strings.Index(ref, "/sessions/"); idx >= 0 {
			tail := ref[idx:]
			if strings.HasSuffix(tail, "/photos") || strings.Contains(tail, "/photos?") {
				redirect = tail
			}
		}
	}
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", redirect)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// PhotoDelete handles POST /routes/{routeID}/photos/{photoID}/delete.
// The uploader or any setter can delete a photo.
func (h *Handler) PhotoDelete(w http.ResponseWriter, r *http.Request) {
	routeID := chi.URLParam(r, "routeID")
	photoID := chi.URLParam(r, "photoID")
	if routeID == "" || photoID == "" {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Load the photo
	photo, err := h.photoRepo.GetByID(ctx, photoID)
	if err != nil || photo == nil {
		h.renderError(w, r, http.StatusNotFound, "Photo not found", "This photo doesn't exist.")
		return
	}

	// Verify it belongs to the right route
	if photo.RouteID != routeID {
		h.renderError(w, r, http.StatusNotFound, "Photo not found", "This photo doesn't exist.")
		return
	}

	// Verify the route belongs to the user's location
	route, err := h.routeRepo.GetByID(ctx, photo.RouteID)
	if err != nil || route == nil {
		h.renderError(w, r, http.StatusNotFound, "Route not found", "This route doesn't exist.")
		return
	}
	if !h.checkLocationOwnership(w, r, route.LocationID) {
		return
	}

	// Authorization: uploader can delete their own, setters can delete any
	role := middleware.GetWebRole(ctx)
	isUploader := photo.UploadedBy != nil && *photo.UploadedBy == user.ID
	isSetter := middleware.RoleRankValue(role) >= middleware.RoleRankValue("setter")

	if !isUploader && !isSetter {
		h.renderError(w, r, http.StatusForbidden, "Not allowed", "You can only delete photos you uploaded.")
		return
	}

	// Delete from S3
	if h.storageService.IsConfigured() {
		if err := h.storageService.Delete(ctx, photo.PhotoURL); err != nil {
			slog.Error("failed to delete photo from storage", "photo_id", photoID, "error", err)
			// Continue with DB deletion even if S3 fails
		}
	}

	// Delete from database
	if err := h.photoRepo.Delete(ctx, photoID); err != nil {
		slog.Error("failed to delete photo record", "photo_id", photoID, "error", err)
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}

	// Redirect back to route detail
	redirect := fmt.Sprintf("/routes/%s", routeID)
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", redirect)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}
