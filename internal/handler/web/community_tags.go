package webhandler

import (
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
)

// AddCommunityTag handles POST /routes/{routeID}/tags — adds a user-submitted tag.
// Returns the updated community-tags partial for HTMX swap.
func (h *Handler) AddCommunityTag(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	routeID := chi.URLParam(r, "routeID")
	user := middleware.GetWebUser(ctx)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if !validRouteID.MatchString(routeID) {
		http.Error(w, "Invalid route", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	raw := strings.TrimSpace(r.FormValue("tag_name"))

	// Normalize: lowercase, collapse whitespace
	tagName := strings.ToLower(raw)
	tagName = strings.Join(strings.Fields(tagName), " ")

	// Validate length
	if tagName == "" || utf8.RuneCountInString(tagName) > 30 {
		http.Error(w, "Tag must be 1-30 characters", http.StatusBadRequest)
		return
	}

	// Profanity check (whole-word matching)
	if !h.profanity.IsClean(tagName) {
		http.Error(w, "Tag contains inappropriate language", http.StatusBadRequest)
		return
	}

	if err := h.userTagRepo.Add(ctx, routeID, user.ID, tagName); err != nil {
		slog.Error("add community tag failed", "route_id", routeID, "user_id", user.ID, "error", err)
		http.Error(w, "Failed to add tag", http.StatusInternalServerError)
		return
	}

	h.renderCommunityTags(w, r, routeID, user.ID)
}

// RemoveCommunityTag handles DELETE /routes/{routeID}/tags — removes the current
// user's vote for a tag. If the user is a head_setter+, removes all instances.
func (h *Handler) RemoveCommunityTag(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	routeID := chi.URLParam(r, "routeID")
	user := middleware.GetWebUser(ctx)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if !validRouteID.MatchString(routeID) {
		http.Error(w, "Invalid route", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	tagName := strings.TrimSpace(r.FormValue("tag_name"))
	if tagName == "" {
		http.Error(w, "Missing tag name", http.StatusBadRequest)
		return
	}

	if err := h.userTagRepo.Remove(ctx, routeID, user.ID, tagName); err != nil {
		slog.Error("remove community tag failed", "route_id", routeID, "error", err)
		http.Error(w, "Failed to remove tag", http.StatusInternalServerError)
		return
	}

	h.renderCommunityTags(w, r, routeID, user.ID)
}

// DeleteCommunityTag handles POST /routes/{routeID}/tags/delete — moderator action
// to remove ALL instances of a tag from a route. Requires head_setter or above.
func (h *Handler) DeleteCommunityTag(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	routeID := chi.URLParam(r, "routeID")
	user := middleware.GetWebUser(ctx)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if !validRouteID.MatchString(routeID) {
		http.Error(w, "Invalid route", http.StatusBadRequest)
		return
	}

	// Check permission: head_setter, manager, or admin
	role := middleware.GetWebRole(ctx)
	if role != "head_setter" && role != "manager" && role != "admin" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	tagName := strings.TrimSpace(r.FormValue("tag_name"))
	if tagName == "" {
		http.Error(w, "Missing tag name", http.StatusBadRequest)
		return
	}

	if err := h.userTagRepo.DeleteTagFromRoute(ctx, routeID, tagName); err != nil {
		slog.Error("delete community tag failed", "route_id", routeID, "error", err)
		http.Error(w, "Failed to delete tag", http.StatusInternalServerError)
		return
	}

	h.renderCommunityTags(w, r, routeID, user.ID)
}

// renderCommunityTags renders the community-tags-section partial.
func (h *Handler) renderCommunityTags(w http.ResponseWriter, r *http.Request, routeID, viewerID string) {
	ctx := r.Context()
	tags, err := h.userTagRepo.ListByRoute(ctx, routeID, viewerID)
	if err != nil {
		slog.Error("load community tags failed", "route_id", routeID, "error", err)
		tags = nil
	}

	effectiveRole := middleware.GetWebRole(ctx)
	isHeadSetter := effectiveRole == "head_setter" || effectiveRole == "manager" || effectiveRole == "admin"

	data := &PageData{
		TemplateData:  templateDataFromContext(r, "routes"),
		CommunityTags: tags,
	}
	data.TemplateData.IsHeadSetter = isHeadSetter

	// Pass the routeID for form action URLs
	if rt, rErr := h.routeRepo.GetByID(ctx, routeID); rErr == nil && rt != nil {
		rv := RouteView{Route: *rt}
		data.Route = &rv
	}

	tmpl, ok := h.templates["climber/route-detail.html"]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "community-tags-section", data); err != nil {
		slog.Error("render community tags partial failed", "error", err)
	}
}
