package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/shotwell-paddle/routewerk/internal/api"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/rbac"
)

// Phase 1f wave 2 — competition categories CRUD.
//
// Categories are simpler than events — no time window, no scoring
// override, just a name + sort_order + freeform jsonb rules.
//
// Routes (wired in router.go):
//   GET  /api/v1/competitions/{id}/categories      any auth
//   POST /api/v1/competitions/{id}/categories      head_setter+

// ListCategories handles GET /api/v1/competitions/{id}/categories.
func (h *CompHandler) ListCategories(w http.ResponseWriter, r *http.Request) {
	compID := chi.URLParam(r, "id")
	if !isUUID(compID) {
		Error(w, http.StatusBadRequest, "invalid competition id")
		return
	}
	if _, ok := h.loadComp(w, r, compID); !ok {
		return
	}
	cats, err := h.repo.ListCategories(r.Context(), compID)
	if err != nil {
		slog.Error("list categories", "competition_id", compID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]api.CompetitionCategory, 0, len(cats))
	for i := range cats {
		c, err := categoryToAPI(&cats[i])
		if err != nil {
			slog.Error("category serialization", "id", cats[i].ID, "error", err)
			Error(w, http.StatusInternalServerError, "internal error")
			return
		}
		out = append(out, c)
	}
	JSON(w, http.StatusOK, out)
}

// CreateCategory handles POST /api/v1/competitions/{id}/categories.
func (h *CompHandler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	compID := chi.URLParam(r, "id")
	if !isUUID(compID) {
		Error(w, http.StatusBadRequest, "invalid competition id")
		return
	}
	comp, ok := h.loadComp(w, r, compID)
	if !ok {
		return
	}
	if !h.requireCompRole(w, r, comp, rbac.RoleHeadSetter) {
		return
	}
	var body api.CategoryCreate
	if err := Decode(r, &body); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Name == "" {
		Error(w, http.StatusBadRequest, "name is required")
		return
	}
	cat := categoryCreateToModel(compID, &body)
	if err := h.repo.CreateCategory(r.Context(), cat); err != nil {
		slog.Error("create category", "competition_id", compID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	out, err := categoryToAPI(cat)
	if err != nil {
		slog.Error("category serialization", "id", cat.ID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	JSON(w, http.StatusCreated, out)
}

// ── Conversion ─────────────────────────────────────────────

func categoryCreateToModel(compID string, b *api.CategoryCreate) *model.CompetitionCategory {
	c := &model.CompetitionCategory{
		CompetitionID: compID,
		Name:          b.Name,
		Rules:         json.RawMessage(`{}`),
	}
	if b.SortOrder != nil {
		c.SortOrder = *b.SortOrder
	}
	if b.Rules != nil {
		if buf, err := json.Marshal(*b.Rules); err == nil {
			c.Rules = buf
		}
	}
	return c
}

func categoryToAPI(c *model.CompetitionCategory) (api.CompetitionCategory, error) {
	id, err := uuid.Parse(c.ID)
	if err != nil {
		return api.CompetitionCategory{}, err
	}
	cid, err := uuid.Parse(c.CompetitionID)
	if err != nil {
		return api.CompetitionCategory{}, err
	}
	rules := map[string]interface{}{}
	if len(c.Rules) > 0 && string(c.Rules) != "null" {
		_ = json.Unmarshal(c.Rules, &rules)
	}
	return api.CompetitionCategory{
		Id:            id,
		CompetitionId: cid,
		Name:          c.Name,
		SortOrder:     c.SortOrder,
		Rules:         rules,
	}, nil
}

