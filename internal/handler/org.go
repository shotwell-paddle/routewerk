package handler

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

type OrgHandler struct {
	orgs *repository.OrgRepo
}

func NewOrgHandler(orgs *repository.OrgRepo) *OrgHandler {
	return &OrgHandler{orgs: orgs}
}

type createOrgRequest struct {
	Name    string  `json:"name"`
	Slug    string  `json:"slug"`
	LogoURL *string `json:"logo_url,omitempty"`
}

func (h *OrgHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createOrgRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		Error(w, http.StatusBadRequest, "name is required")
		return
	}

	if req.Slug == "" {
		req.Slug = slugify(req.Name)
	}

	// Check slug uniqueness
	existing, err := h.orgs.GetBySlug(r.Context(), req.Slug)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if existing != nil {
		Error(w, http.StatusConflict, "organization slug already taken")
		return
	}

	org := &model.Organization{
		Name:    req.Name,
		Slug:    req.Slug,
		LogoURL: req.LogoURL,
	}
	if err := h.orgs.Create(r.Context(), org); err != nil {
		Error(w, http.StatusInternalServerError, "failed to create organization")
		return
	}

	// Make the creating user an org_admin
	userID := middleware.GetUserID(r.Context())
	membership := &model.UserMembership{
		UserID: userID,
		OrgID:  org.ID,
		Role:   "org_admin",
	}
	if err := h.orgs.AddMember(r.Context(), membership); err != nil {
		Error(w, http.StatusInternalServerError, "failed to add org admin")
		return
	}

	JSON(w, http.StatusCreated, org)
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

	var req createOrgRequest
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

	JSON(w, http.StatusOK, org)
}

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonAlphaNum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}
