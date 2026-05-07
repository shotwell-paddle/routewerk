package handler

import (
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

// RouteTagHandler exposes the JSON variant of the HTMX community-tag
// pipeline (internal/handler/web/community_tags.go). Climbers add their
// own tag votes; setters can also delete any tag from a route as a
// moderation action. Tags are normalized + profanity-filtered server-
// side so the SPA can hand off raw input.
type RouteTagHandler struct {
	routes    *repository.RouteRepo
	userTags  *repository.UserTagRepo
	profanity *service.ProfanityFilter
}

func NewRouteTagHandler(
	routes *repository.RouteRepo,
	userTags *repository.UserTagRepo,
	profanity *service.ProfanityFilter,
) *RouteTagHandler {
	return &RouteTagHandler{routes: routes, userTags: userTags, profanity: profanity}
}

// List — GET /api/v1/locations/{locationID}/routes/{routeID}/tags.
//
// Aggregated tags for a route, ordered by popularity. The current user's
// own votes are flagged via `user_added` so the SPA can light up the
// chips they've already added.
func (h *RouteTagHandler) List(w http.ResponseWriter, r *http.Request) {
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

	viewerID := middleware.GetUserID(r.Context())
	tags, err := h.userTags.ListByRoute(r.Context(), routeID, viewerID)
	if err != nil {
		slog.Error("list community tags", "route_id", routeID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Normalize nil → [] so the SPA can call `.length` without a guard.
	if tags == nil {
		tags = []repository.AggregatedTag{}
	}
	JSON(w, http.StatusOK, tags)
}

type addCommunityTagRequest struct {
	TagName string `json:"tag_name"`
}

// Add — POST /api/v1/locations/{locationID}/routes/{routeID}/tags
// { tag_name: "crimpy" }.
//
// Any authenticated location member can vote. Validation mirrors the
// HTMX handler exactly: trim, lowercase, collapse whitespace, 1-30
// runes, run through the profanity filter. Duplicate (route, user, tag)
// is a server-side no-op via ON CONFLICT.
func (h *RouteTagHandler) Add(w http.ResponseWriter, r *http.Request) {
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

	var req addCommunityTagRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tagName := normalizeTagName(req.TagName)
	if tagName == "" || utf8.RuneCountInString(tagName) > 30 {
		Error(w, http.StatusBadRequest, "tag must be 1-30 characters")
		return
	}
	if !h.profanity.IsClean(tagName) {
		Error(w, http.StatusBadRequest, "tag contains inappropriate language")
		return
	}

	route, err := h.routes.GetByID(r.Context(), routeID)
	if err != nil || route == nil || route.LocationID != locationID {
		Error(w, http.StatusNotFound, "route not found")
		return
	}

	if err := h.userTags.Add(r.Context(), routeID, userID, tagName); err != nil {
		slog.Error("add community tag failed", "route_id", routeID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	tags, _ := h.userTags.ListByRoute(r.Context(), routeID, userID)
	if tags == nil {
		tags = []repository.AggregatedTag{}
	}
	JSON(w, http.StatusOK, tags)
}

// Remove — DELETE /api/v1/locations/{locationID}/routes/{routeID}/tags
// { tag_name: "crimpy" }.
//
// Removes the caller's own vote for a tag. The HTMX handler also lets
// head_setter+ moderate tags via a separate /tags/delete endpoint —
// that's exposed below as `Moderate`.
func (h *RouteTagHandler) Remove(w http.ResponseWriter, r *http.Request) {
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

	var req addCommunityTagRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	tagName := normalizeTagName(req.TagName)
	if tagName == "" {
		Error(w, http.StatusBadRequest, "tag_name is required")
		return
	}

	route, err := h.routes.GetByID(r.Context(), routeID)
	if err != nil || route == nil || route.LocationID != locationID {
		Error(w, http.StatusNotFound, "route not found")
		return
	}

	if err := h.userTags.Remove(r.Context(), routeID, userID, tagName); err != nil {
		slog.Error("remove community tag failed", "route_id", routeID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	tags, _ := h.userTags.ListByRoute(r.Context(), routeID, userID)
	if tags == nil {
		tags = []repository.AggregatedTag{}
	}
	JSON(w, http.StatusOK, tags)
}

// Moderate — DELETE /api/v1/locations/{locationID}/routes/{routeID}/tags/all
// { tag_name: "..." }.
//
// head_setter+ scrubs every vote for a given tag from the route. Used
// when a tag is misleading or off-topic and the moderator wants it gone
// for everyone (mirrors HTMX `/tags/delete`).
func (h *RouteTagHandler) Moderate(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	routeID := chi.URLParam(r, "routeID")
	if !isUUID(locationID) || !isUUID(routeID) {
		Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	var req addCommunityTagRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	tagName := normalizeTagName(req.TagName)
	if tagName == "" {
		Error(w, http.StatusBadRequest, "tag_name is required")
		return
	}

	route, err := h.routes.GetByID(r.Context(), routeID)
	if err != nil || route == nil || route.LocationID != locationID {
		Error(w, http.StatusNotFound, "route not found")
		return
	}

	if err := h.userTags.DeleteTagFromRoute(r.Context(), routeID, tagName); err != nil {
		slog.Error("moderate community tag failed", "route_id", routeID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func normalizeTagName(raw string) string {
	t := strings.ToLower(strings.TrimSpace(raw))
	return strings.Join(strings.Fields(t), " ")
}
