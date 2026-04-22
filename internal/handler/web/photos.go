package webhandler

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
	"github.com/shotwell-paddle/routewerk/internal/service"
)

const maxUploadSize = 5 << 20 // 5 MB

// allowedImageTypes is the allow-list of MIME types the server can actually
// decode. HEIC is NOT on this list — decoding HEIC requires libde265 via CGO,
// which would break our CGO_ENABLED=0 build. Instead, the browser decodes
// HEIC via the native createImageBitmap pipeline in app.js and re-encodes
// it as JPEG before upload, so by the time bytes reach the server they're
// already one of these three formats.
var allowedImageTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

// heifBrands is the set of ISOBMFF `ftyp` major brands that indicate a HEIC
// or HEIF file. http.DetectContentType returns "application/octet-stream"
// for HEIC, so we identify the format ourselves purely to produce a
// *helpful error message* ("HEIC couldn't be converted in your browser")
// rather than the generic "unsupported format". Reference: ISO/IEC
// 14496-12 + 23008-12.
var heifBrands = map[string]bool{
	"heic": true, "heix": true, "heim": true, "heis": true,
	"hevc": true, "hevx": true, "hevm": true, "hevs": true,
	"mif1": true, "msf1": true,
}

// isHEIC reports whether a byte buffer looks like a HEIC/HEIF file based on
// its ISOBMFF `ftyp` box brand. An ISOBMFF file starts with a 4-byte
// big-endian box size, then the literal "ftyp", then a 4-byte major brand.
func isHEIC(b []byte) bool {
	if len(b) < 12 {
		return false
	}
	if string(b[4:8]) != "ftyp" {
		return false
	}
	return heifBrands[string(b[8:12])]
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

	// Parse multipart form. ErrNotMultipart gets its own message because
	// "File too large" was actively misleading the last time an upload form
	// shipped without hx-encoding="multipart/form-data" and hx-boost silently
	// URL-encoded the body — the request landed here looking like a form
	// POST with no file at all.
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		if errors.Is(err, http.ErrNotMultipart) {
			slog.Warn("photo upload: request was not multipart",
				"route_id", routeID, "content_type", r.Header.Get("Content-Type"))
			http.Error(w, "Upload must be multipart/form-data", http.StatusBadRequest)
			return
		}
		http.Error(w, "File too large (max 5 MB)", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("photo")
	if err != nil {
		http.Error(w, "Missing photo file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Validate content type by sniffing the first 512 bytes rather than
	// trusting the multipart header (which is attacker-controlled). The
	// sniffed type is also what we pass to ProcessImage below, so any
	// mismatch between the declared and actual type is immaterial.
	sniff := make([]byte, 512)
	n, err := io.ReadFull(file, sniff)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		http.Error(w, "Could not read upload", http.StatusBadRequest)
		return
	}
	sniff = sniff[:n]
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		slog.Error("failed to rewind upload", "error", err)
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
		return
	}
	contentType := http.DetectContentType(sniff)
	// Detect HEIC specifically so we can return a useful message. The
	// browser is supposed to decode HEIC via createImageBitmap and re-
	// encode as JPEG before the upload fires; if raw HEIC is reaching the
	// server, the client-side pipeline failed (most likely a desktop
	// browser without native HEIC decode, e.g. Chrome/Firefox).
	if contentType == "application/octet-stream" && isHEIC(sniff) {
		slog.Warn("photo upload: raw HEIC reached server (client decode skipped?)",
			"route_id", routeID,
			"declared", header.Header.Get("Content-Type"),
			"filename", header.Filename)
		http.Error(w, "HEIC couldn't be decoded in your browser. Upload directly from your iPhone, or save the photo as JPEG first.", http.StatusBadRequest)
		return
	}
	if !allowedImageTypes[contentType] {
		// Log so the next unsupported format is visible in fly logs — the
		// previous version silently 400'd and the response text was hidden
		// behind a generic HTMX toast.
		slog.Warn("photo upload: unsupported content type",
			"route_id", routeID,
			"sniffed", contentType,
			"declared", header.Header.Get("Content-Type"),
			"filename", header.Filename)
		http.Error(w, "Unsupported image format — use JPEG, PNG, or WebP", http.StatusBadRequest)
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

	// Upload to S3 — capture BOTH the stable storage key (for future Delete)
	// and the public URL (for rendering). Storing only the URL means a CDN or
	// endpoint rotation silently breaks deletes.
	storageKey, photoURL, err := h.storageService.Upload(ctx, routeID, uploadFilename, processed.ContentType, processed.Data)
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
		StorageKey: &storageKey,
		Caption:    captionPtr,
		UploadedBy: &user.ID,
		SortOrder:  sortOrder,
	}
	if err := h.photoRepo.Create(ctx, photo); err != nil {
		slog.Error("failed to save photo record", "route_id", routeID, "error", err)
		// Try to clean up the uploaded file (we still have the key in memory,
		// so we don't need to round-trip through the DB or parse a URL).
		if delErr := h.storageService.Delete(ctx, storageKey); delErr != nil {
			slog.Error("failed to clean up orphaned S3 photo", "route_id", routeID, "key", storageKey, "error", delErr)
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

	// Delete from S3 by stable storage key. Legacy rows (inserted before
	// migration 28) have storage_key = NULL, so fall back to deriving the key
	// from the URL — that fallback only works while StorageEndpoint matches
	// what was configured when the row was written, so we want it to fade out.
	if h.storageService.IsConfigured() {
		var keyToDelete string
		if photo.StorageKey != nil && *photo.StorageKey != "" {
			keyToDelete = *photo.StorageKey
		} else {
			keyToDelete = h.storageService.KeyFromURL(photo.PhotoURL)
		}
		if keyToDelete == "" {
			slog.Warn("photo delete: could not determine storage key", "photo_id", photoID, "url", photo.PhotoURL)
		} else if err := h.storageService.Delete(ctx, keyToDelete); err != nil {
			slog.Error("failed to delete photo from storage", "photo_id", photoID, "key", keyToDelete, "error", err)
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
