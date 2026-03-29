package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

type OrgHandler struct {
	orgs  *repository.OrgRepo
	audit *service.AuditService
}

func NewOrgHandler(orgs *repository.OrgRepo, audit *service.AuditService) *OrgHandler {
	return &OrgHandler{orgs: orgs, audit: audit}
}

type updateOrgRequest struct {
	Name    string  `json:"name"`
	Slug    string  `json:"slug"`
	LogoURL *string `json:"logo_url,omitempty"`
}

func (h *OrgHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	orgs, err := h.orgs.ListByUser(r.Context(), userID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	if orgs == nil {
		orgs = []model.Organization{}
	}

	JSON(w, http.StatusOK, orgs)
}

func (h *OrgHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")

	org, err := h.orgs.GetByID(r.Context(), orgID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if org == nil {
		Error(w, http.StatusNotFound, "organization not found")
		return
	}

	JSON(w, http.StatusOK, org)
}

func (h *OrgHandler) Update(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")

	org, err := h.orgs.GetByID(r.Context(), orgID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if org == nil {
		Error(w, http.StatusNotFound, "organization not found")
		return
	}

	var req updateOrgRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name != "" {
		org.Name = req.Name
	}
	if req.Slug != "" {
		org.Slug = req.Slug
	}
	if req.LogoURL != nil {
		org.LogoURL = req.LogoURL
	}

	if err := h.orgs.Update(r.Context(), org); err != nil {
		Error(w, http.StatusInternalServerError, "failed to update organization")
		return
	}

	h.audit.Record(r, service.AuditOrgUpdate, "org", orgID, orgID, map[string]interface{}{
		"name": org.Name,
		"slug": org.Slug,
	})

	JSON(w, http.StatusOK, org)
}
