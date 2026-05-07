package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// ProgressionsAdminHandler exposes admin CRUD on quests, badges, and quest
// domains for the SPA at /settings/progressions. The climber-side
// QuestHandler (handler/quest.go) handles browse/start/log/abandon; this
// handler is purely for staff catalog management. Mirrors the HTMX
// /settings/progressions surface.
type ProgressionsAdminHandler struct {
	quests *repository.QuestRepo
	badges *repository.BadgeRepo
	authz  *middleware.Authorizer
}

func NewProgressionsAdminHandler(quests *repository.QuestRepo, badges *repository.BadgeRepo, authz *middleware.Authorizer) *ProgressionsAdminHandler {
	return &ProgressionsAdminHandler{quests: quests, badges: badges, authz: authz}
}

// requireHeadSetterAt looks up the caller's role at the given location
// and writes a 403 (or 404 / 500 as appropriate) if they're not at
// least head_setter. Used by the by-id update/delete handlers that
// don't have {locationID} on the URL — without this the chi parent
// middleware can't enforce per-location rank, so any authenticated
// user could mutate any quest/badge/domain by guessing an ID.
func (h *ProgressionsAdminHandler) requireHeadSetterAt(w http.ResponseWriter, r *http.Request, locationID string) bool {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		Error(w, http.StatusUnauthorized, "not authenticated")
		return false
	}
	m, err := h.authz.LookupLocationMembership(r.Context(), userID, locationID)
	if err != nil || m == nil {
		// Hide cross-org access under 404 so the API doesn't leak the
		// existence of rows in other gyms.
		Error(w, http.StatusNotFound, "not found")
		return false
	}
	if middleware.RoleRankValue(m.Role) < middleware.RoleRankValue(middleware.RoleHeadSetter) {
		Error(w, http.StatusForbidden, "head_setter or above required")
		return false
	}
	return true
}

// ── Quest admin ──────────────────────────────────────────

// ListAllQuests — GET /locations/{id}/admin/quests. Lists every quest
// (active + inactive) at the location, with social-proof counts. Used by
// the admin index. Climbers use the regular ListAvailable on QuestHandler.
func (h *ProgressionsAdminHandler) ListAllQuests(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	items, err := h.quests.ListByLocation(r.Context(), locationID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if items == nil {
		items = []repository.QuestListItem{}
	}
	JSON(w, http.StatusOK, map[string]interface{}{"quests": items})
}

// CreateQuest — POST /locations/{id}/admin/quests. The body matches the
// Quest model; LocationID is taken from the URL.
func (h *ProgressionsAdminHandler) CreateQuest(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	var q model.Quest
	if err := Decode(r, &q); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	q.LocationID = locationID
	if q.Name == "" || q.QuestType == "" || q.DomainID == "" {
		Error(w, http.StatusBadRequest, "name, quest_type, and domain_id are required")
		return
	}
	if err := h.quests.Create(r.Context(), &q); err != nil {
		Error(w, http.StatusInternalServerError, "failed to create quest")
		return
	}
	JSON(w, http.StatusCreated, q)
}

// UpdateQuest — PUT /quests/{id}. Replaces the quest's mutable fields.
// LocationID + ID + CreatedAt aren't touched. Authz: head_setter+ at
// the quest's owning location (looked up from the row, NOT the body).
func (h *ProgressionsAdminHandler) UpdateQuest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "questID")
	existing, err := h.quests.GetByID(r.Context(), id)
	if err != nil {
		slog.Error("update quest: lookup failed", "quest_id", id, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if existing == nil {
		Error(w, http.StatusNotFound, "quest not found")
		return
	}
	if !h.requireHeadSetterAt(w, r, existing.LocationID) {
		return
	}

	var q model.Quest
	if err := Decode(r, &q); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// Pin id + location to the existing row — body can't relocate the
	// quest into a gym the caller has rights at by setting location_id.
	q.ID = id
	q.LocationID = existing.LocationID
	if err := h.quests.Update(r.Context(), &q); err != nil {
		Error(w, http.StatusInternalServerError, "failed to update quest")
		return
	}
	JSON(w, http.StatusOK, q)
}

// DeactivateQuest — POST /locations/{id}/admin/quests/{questID}/deactivate.
// Sets is_active=false. Doesn't delete (preserves enrollment history).
func (h *ProgressionsAdminHandler) DeactivateQuest(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	id := chi.URLParam(r, "questID")
	if err := h.quests.Deactivate(r.Context(), id, locationID); err != nil {
		Error(w, http.StatusInternalServerError, "failed to deactivate quest")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DuplicateQuest — POST /locations/{id}/admin/quests/{questID}/duplicate.
// Creates a copy with " (copy)" suffix on the name and is_active=false.
// Useful when a head setter wants to clone last season's quest with tweaks.
func (h *ProgressionsAdminHandler) DuplicateQuest(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	id := chi.URLParam(r, "questID")
	q, err := h.quests.Duplicate(r.Context(), id, locationID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "failed to duplicate quest")
		return
	}
	JSON(w, http.StatusCreated, q)
}

// ── Domain admin ─────────────────────────────────────────

// ListDomains — GET /locations/{id}/admin/quest-domains.
func (h *ProgressionsAdminHandler) ListDomains(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	domains, err := h.quests.ListDomains(r.Context(), locationID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if domains == nil {
		domains = []model.QuestDomain{}
	}
	JSON(w, http.StatusOK, domains)
}

// CreateDomain — POST /locations/{id}/admin/quest-domains.
func (h *ProgressionsAdminHandler) CreateDomain(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	var d model.QuestDomain
	if err := Decode(r, &d); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	d.LocationID = locationID
	if d.Name == "" {
		Error(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := h.quests.CreateDomain(r.Context(), &d); err != nil {
		Error(w, http.StatusInternalServerError, "failed to create domain")
		return
	}
	JSON(w, http.StatusCreated, d)
}

// UpdateDomain — PUT /quest-domains/{id}. Authz: head_setter+ at the
// domain's owning location, looked up from the row.
func (h *ProgressionsAdminHandler) UpdateDomain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "domainID")
	existing, err := h.quests.GetDomainByID(r.Context(), id)
	if err != nil {
		slog.Error("update domain: lookup failed", "domain_id", id, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if existing == nil {
		Error(w, http.StatusNotFound, "domain not found")
		return
	}
	if !h.requireHeadSetterAt(w, r, existing.LocationID) {
		return
	}

	var d model.QuestDomain
	if err := Decode(r, &d); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	d.ID = id
	d.LocationID = existing.LocationID
	if err := h.quests.UpdateDomain(r.Context(), &d); err != nil {
		Error(w, http.StatusInternalServerError, "failed to update domain")
		return
	}
	JSON(w, http.StatusOK, d)
}

// DeleteDomain — DELETE /locations/{id}/admin/quest-domains/{domainID}.
// Repo enforces the location_id match so cross-org deletion is impossible.
func (h *ProgressionsAdminHandler) DeleteDomain(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	id := chi.URLParam(r, "domainID")
	if err := h.quests.DeleteDomain(r.Context(), id, locationID); err != nil {
		Error(w, http.StatusInternalServerError, "failed to delete domain")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Badge admin ──────────────────────────────────────────

// ListBadges — GET /locations/{id}/admin/badges.
func (h *ProgressionsAdminHandler) ListBadges(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	badges, err := h.badges.ListByLocation(r.Context(), locationID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if badges == nil {
		badges = []model.Badge{}
	}
	JSON(w, http.StatusOK, badges)
}

// CreateBadge — POST /locations/{id}/admin/badges.
func (h *ProgressionsAdminHandler) CreateBadge(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	var b model.Badge
	if err := Decode(r, &b); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	b.LocationID = locationID
	if b.Name == "" {
		Error(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := h.badges.Create(r.Context(), &b); err != nil {
		Error(w, http.StatusInternalServerError, "failed to create badge")
		return
	}
	JSON(w, http.StatusCreated, b)
}

// UpdateBadge — PUT /badges/{id}. Authz: head_setter+ at the badge's
// owning location, looked up from the row.
func (h *ProgressionsAdminHandler) UpdateBadge(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "badgeID")
	existing, err := h.badges.GetByID(r.Context(), id)
	if err != nil {
		slog.Error("update badge: lookup failed", "badge_id", id, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if existing == nil {
		Error(w, http.StatusNotFound, "badge not found")
		return
	}
	if !h.requireHeadSetterAt(w, r, existing.LocationID) {
		return
	}

	var b model.Badge
	if err := Decode(r, &b); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	b.ID = id
	b.LocationID = existing.LocationID
	if err := h.badges.Update(r.Context(), &b); err != nil {
		Error(w, http.StatusInternalServerError, "failed to update badge")
		return
	}
	JSON(w, http.StatusOK, b)
}

// DeleteBadge — DELETE /badges/{id}. Authz: head_setter+ at the
// badge's owning location, looked up from the row.
func (h *ProgressionsAdminHandler) DeleteBadge(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "badgeID")
	existing, err := h.badges.GetByID(r.Context(), id)
	if err != nil {
		slog.Error("delete badge: lookup failed", "badge_id", id, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if existing == nil {
		Error(w, http.StatusNotFound, "badge not found")
		return
	}
	if !h.requireHeadSetterAt(w, r, existing.LocationID) {
		return
	}

	if err := h.badges.Delete(r.Context(), id); err != nil {
		Error(w, http.StatusInternalServerError, "failed to delete badge")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
