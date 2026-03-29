package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

type TagHandler struct {
	tags  *repository.TagRepo
	audit *service.AuditService
}

func NewTagHandler(tags *repository.TagRepo, audit *service.AuditService) *TagHandler {
	return &TagHandler{tags: tags, audit: audit}
}

type createTagRequest struct {
	Category string  `json:"category"`
	Name     string  `json:"name"`
	Color    *string `json:"color,omitempty"`
}

func (h *TagHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")

	var req createTagRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Category == "" || req.Name == "" {
		Error(w, http.StatusBadRequest, "category and name are required")
		return
	}

	tag := &model.Tag{
		OrgID:    orgID,
		Category: req.Category,
		Name:     req.Name,
		Color:    req.Color,
	}

	if err := h.tags.Create(r.Context(), tag); err != nil {
		Error(w, http.StatusInternalServerError, "failed to create tag")
		return
	}

	h.audit.Record(r, service.AuditTagCreate, "tag", tag.ID, orgID, map[string]interface{}{
		"name":     tag.Name,
		"category": tag.Category,
	})

	JSON(w, http.StatusCreated, tag)
}

func (h *TagHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")
	category := r.URL.Query().Get("category")

	tags, err := h.tags.ListByOrg(r.Context(), orgID, category)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	if tags == nil {
		tags = []model.Tag{}
	}

	JSON(w, http.StatusOK, tags)
}

func (h *TagHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")
	tagID := chi.URLParam(r, "tagID")

	if err := h.tags.Delete(r.Context(), tagID); err != nil {
		Error(w, http.StatusInternalServerError, "failed to delete tag")
		return
	}

	h.audit.Record(r, service.AuditTagDelete, "tag", tagID, orgID, nil)

	JSON(w, http.StatusNoContent, nil)
}
