package webhandler

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

// ============================================================
// Quest Domains — CRUD
// ============================================================

// ProgressionsAdminPage is the main admin page listing domains and quests.
// GET /settings/progressions
func (h *Handler) ProgressionsAdminPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a location.")
		return
	}

	realRole := middleware.GetWebRole(ctx)
	if middleware.RoleRankValue(realRole) < middleware.RoleRankValue("head_setter") {
		h.renderError(w, r, http.StatusForbidden, "Not authorized", "Progressions admin requires head setter access.")
		return
	}

	domains, err := h.questRepo.ListDomains(ctx, locationID)
	if err != nil {
		slog.Error("list quest domains failed", "error", err)
		domains = nil
	}

	quests, err := h.questRepo.ListByLocation(ctx, locationID)
	if err != nil {
		slog.Error("list quests failed", "error", err)
		quests = nil
	}

	badges, err := h.badgeRepo.ListByLocation(ctx, locationID)
	if err != nil {
		slog.Error("list badges failed", "error", err)
		badges = nil
	}

	coverage, err := h.routeSkillTagRepo.TagCoverage(ctx, locationID)
	if err != nil {
		slog.Error("tag coverage failed", "error", err)
		coverage = nil
	}

	data := &PageData{
		TemplateData:     templateDataFromContext(r, "progressions"),
		QuestDomains:     domains,
		Quests:           quests,
		Badges:           badges,
		SkillTagCoverage: coverage,
	}
	h.render(w, r, "setter/progressions.html", data)
}

// DomainCreateForm renders the domain creation form.
// GET /settings/progressions/domains/new
func (h *Handler) DomainCreateForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireProgressionsAdmin(w, r) {
		return
	}

	_ = ctx
	data := &PageData{
		TemplateData:     templateDataFromContext(r, "progressions"),
		DomainFormValues: DomainFormValues{SortOrder: "0"},
	}
	h.render(w, r, "setter/domain-form.html", data)
}

// DomainCreate handles domain creation.
// POST /settings/progressions/domains/new
func (h *Handler) DomainCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireProgressionsAdmin(w, r) {
		return
	}

	locationID := middleware.GetWebLocationID(ctx)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid form", "Could not parse form data.")
		return
	}

	fv := parseDomainForm(r)
	sortOrder, _ := strconv.Atoi(fv.SortOrder)

	d := &model.QuestDomain{
		LocationID:  locationID,
		Name:        fv.Name,
		Description: strPtrIfNotEmpty(fv.Description),
		Color:       strPtrIfNotEmpty(fv.Color),
		Icon:        strPtrIfNotEmpty(fv.Icon),
		SortOrder:   sortOrder,
	}

	if d.Name == "" {
		data := &PageData{
			TemplateData:     templateDataFromContext(r, "progressions"),
			DomainFormValues: fv,
			DomainFormError:  "Name is required.",
		}
		h.render(w, r, "setter/domain-form.html", data)
		return
	}

	if err := h.questRepo.CreateDomain(ctx, d); err != nil {
		slog.Error("create domain failed", "error", err)
		data := &PageData{
			TemplateData:     templateDataFromContext(r, "progressions"),
			DomainFormValues: fv,
			DomainFormError:  "Could not create domain. Please try again.",
		}
		h.render(w, r, "setter/domain-form.html", data)
		return
	}

	w.Header().Set("HX-Redirect", "/settings/progressions")
	w.WriteHeader(http.StatusOK)
}

// DomainEditForm renders the domain edit form.
// GET /settings/progressions/domains/{domainID}/edit
func (h *Handler) DomainEditForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireProgressionsAdmin(w, r) {
		return
	}

	domainID := chi.URLParam(r, "domainID")
	d, err := h.questRepo.GetDomainByID(ctx, domainID)
	if err != nil || d == nil {
		h.renderError(w, r, http.StatusNotFound, "Not found", "Domain not found.")
		return
	}

	if !h.checkLocationOwnership(w, r, d.LocationID) {
		return
	}

	data := &PageData{
		TemplateData: templateDataFromContext(r, "progressions"),
		QuestDomain:  d,
		DomainFormValues: DomainFormValues{
			Name:        d.Name,
			Description: derefString(d.Description),
			Color:       derefString(d.Color),
			Icon:        derefString(d.Icon),
			SortOrder:   strconv.Itoa(d.SortOrder),
		},
	}
	h.render(w, r, "setter/domain-form.html", data)
}

// DomainUpdate handles domain edits.
// POST /settings/progressions/domains/{domainID}/edit
func (h *Handler) DomainUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireProgressionsAdmin(w, r) {
		return
	}

	locationID := middleware.GetWebLocationID(ctx)
	domainID := chi.URLParam(r, "domainID")
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid form", "Could not parse form data.")
		return
	}

	fv := parseDomainForm(r)
	sortOrder, _ := strconv.Atoi(fv.SortOrder)

	d := &model.QuestDomain{
		ID:          domainID,
		LocationID:  locationID,
		Name:        fv.Name,
		Description: strPtrIfNotEmpty(fv.Description),
		Color:       strPtrIfNotEmpty(fv.Color),
		Icon:        strPtrIfNotEmpty(fv.Icon),
		SortOrder:   sortOrder,
	}

	if d.Name == "" {
		data := &PageData{
			TemplateData:     templateDataFromContext(r, "progressions"),
			DomainFormValues: fv,
			DomainFormError:  "Name is required.",
		}
		h.render(w, r, "setter/domain-form.html", data)
		return
	}

	if err := h.questRepo.UpdateDomain(ctx, d); err != nil {
		slog.Error("update domain failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Update failed", "Could not update domain.")
		return
	}

	w.Header().Set("HX-Redirect", "/settings/progressions")
	w.WriteHeader(http.StatusOK)
}

// DomainDelete removes a domain.
// POST /settings/progressions/domains/{domainID}/delete
func (h *Handler) DomainDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireProgressionsAdmin(w, r) {
		return
	}

	locationID := middleware.GetWebLocationID(ctx)
	domainID := chi.URLParam(r, "domainID")

	if err := h.questRepo.DeleteDomain(ctx, domainID, locationID); err != nil {
		slog.Error("delete domain failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Delete failed", "Could not delete domain. It may have quests assigned.")
		return
	}

	w.Header().Set("HX-Redirect", "/settings/progressions")
	w.WriteHeader(http.StatusOK)
}

// ============================================================
// Quests — CRUD
// ============================================================

// QuestCreateForm renders the quest creation form.
// GET /settings/progressions/quests/new
func (h *Handler) QuestCreateForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireProgressionsAdmin(w, r) {
		return
	}

	locationID := middleware.GetWebLocationID(ctx)
	domains, _ := h.questRepo.ListDomains(ctx, locationID)
	badges, _ := h.badgeRepo.ListByLocation(ctx, locationID)

	data := &PageData{
		TemplateData: templateDataFromContext(r, "progressions"),
		QuestDomains: domains,
		Badges:       badges,
		QuestFormValues: QuestFormValues{
			IsActive:   "true",
			SkillLevel: "beginner",
			QuestType:  "route_count",
			SortOrder:  "0",
		},
	}
	h.render(w, r, "setter/quest-form.html", data)
}

// QuestCreate handles quest creation.
// POST /settings/progressions/quests/new
func (h *Handler) QuestCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireProgressionsAdmin(w, r) {
		return
	}

	locationID := middleware.GetWebLocationID(ctx)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid form", "Could not parse form data.")
		return
	}

	fv := parseQuestForm(r)
	q, formErr := buildQuestFromForm(fv, locationID)
	if formErr != "" {
		domains, _ := h.questRepo.ListDomains(ctx, locationID)
		badges, _ := h.badgeRepo.ListByLocation(ctx, locationID)
		data := &PageData{
			TemplateData:    templateDataFromContext(r, "progressions"),
			QuestDomains:    domains,
			Badges:          badges,
			QuestFormValues: fv,
			QuestFormError:  formErr,
		}
		h.render(w, r, "setter/quest-form.html", data)
		return
	}

	if err := h.questRepo.Create(ctx, q); err != nil {
		slog.Error("create quest failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Create failed", "Could not create quest.")
		return
	}

	w.Header().Set("HX-Redirect", "/settings/progressions")
	w.WriteHeader(http.StatusOK)
}

// QuestEditForm renders the quest edit form.
// GET /settings/progressions/quests/{questID}/edit
func (h *Handler) QuestEditForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireProgressionsAdmin(w, r) {
		return
	}

	questID := chi.URLParam(r, "questID")
	q, err := h.questRepo.GetByID(ctx, questID)
	if err != nil || q == nil {
		h.renderError(w, r, http.StatusNotFound, "Not found", "Quest not found.")
		return
	}
	if !h.checkLocationOwnership(w, r, q.LocationID) {
		return
	}

	locationID := middleware.GetWebLocationID(ctx)
	domains, _ := h.questRepo.ListDomains(ctx, locationID)
	badges, _ := h.badgeRepo.ListByLocation(ctx, locationID)

	data := &PageData{
		TemplateData: templateDataFromContext(r, "progressions"),
		QuestDetail:  q,
		QuestDomains: domains,
		Badges:       badges,
		QuestFormValues: questToFormValues(q),
	}
	h.render(w, r, "setter/quest-form.html", data)
}

// QuestUpdate handles quest edits.
// POST /settings/progressions/quests/{questID}/edit
func (h *Handler) QuestUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireProgressionsAdmin(w, r) {
		return
	}

	locationID := middleware.GetWebLocationID(ctx)
	questID := chi.URLParam(r, "questID")
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid form", "Could not parse form data.")
		return
	}

	fv := parseQuestForm(r)
	q, formErr := buildQuestFromForm(fv, locationID)
	if formErr != "" {
		domains, _ := h.questRepo.ListDomains(ctx, locationID)
		badges, _ := h.badgeRepo.ListByLocation(ctx, locationID)
		data := &PageData{
			TemplateData:    templateDataFromContext(r, "progressions"),
			QuestDomains:    domains,
			Badges:          badges,
			QuestFormValues: fv,
			QuestFormError:  formErr,
		}
		h.render(w, r, "setter/quest-form.html", data)
		return
	}
	q.ID = questID

	if err := h.questRepo.Update(ctx, q); err != nil {
		slog.Error("update quest failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Update failed", "Could not update quest.")
		return
	}

	w.Header().Set("HX-Redirect", "/settings/progressions")
	w.WriteHeader(http.StatusOK)
}

// QuestDeactivate hides a quest from the browser without deleting it.
// POST /settings/progressions/quests/{questID}/deactivate
func (h *Handler) QuestDeactivate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireProgressionsAdmin(w, r) {
		return
	}

	locationID := middleware.GetWebLocationID(ctx)
	questID := chi.URLParam(r, "questID")

	if err := h.questRepo.Deactivate(ctx, questID, locationID); err != nil {
		slog.Error("deactivate quest failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Failed", "Could not deactivate quest.")
		return
	}

	w.Header().Set("HX-Redirect", "/settings/progressions")
	w.WriteHeader(http.StatusOK)
}

// QuestDuplicate clones a quest for seasonal rotation.
// POST /settings/progressions/quests/{questID}/duplicate
func (h *Handler) QuestDuplicate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireProgressionsAdmin(w, r) {
		return
	}

	locationID := middleware.GetWebLocationID(ctx)
	questID := chi.URLParam(r, "questID")

	_, err := h.questRepo.Duplicate(ctx, questID, locationID)
	if err != nil {
		slog.Error("duplicate quest failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Failed", "Could not duplicate quest.")
		return
	}

	w.Header().Set("HX-Redirect", "/settings/progressions")
	w.WriteHeader(http.StatusOK)
}

// ============================================================
// Badges — CRUD
// ============================================================

// BadgeCreateForm renders the badge creation form.
// GET /settings/progressions/badges/new
func (h *Handler) BadgeCreateForm(w http.ResponseWriter, r *http.Request) {
	if !h.requireProgressionsAdmin(w, r) {
		return
	}

	data := &PageData{
		TemplateData:    templateDataFromContext(r, "progressions"),
		BadgeFormValues: BadgeFormValues{Color: "#FFD700", Icon: "award"},
	}
	h.render(w, r, "setter/badge-form.html", data)
}

// BadgeCreate handles badge creation.
// POST /settings/progressions/badges/new
func (h *Handler) BadgeCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireProgressionsAdmin(w, r) {
		return
	}

	locationID := middleware.GetWebLocationID(ctx)
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid form", "Could not parse form data.")
		return
	}

	fv := parseBadgeForm(r)
	if fv.Name == "" {
		data := &PageData{
			TemplateData:    templateDataFromContext(r, "progressions"),
			BadgeFormValues: fv,
			BadgeFormError:  "Name is required.",
		}
		h.render(w, r, "setter/badge-form.html", data)
		return
	}

	b := &model.Badge{
		LocationID:  locationID,
		Name:        fv.Name,
		Description: strPtrIfNotEmpty(fv.Description),
		Icon:        fv.Icon,
		Color:       fv.Color,
	}

	if err := h.badgeRepo.Create(ctx, b); err != nil {
		slog.Error("create badge failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Create failed", "Could not create badge.")
		return
	}

	w.Header().Set("HX-Redirect", "/settings/progressions")
	w.WriteHeader(http.StatusOK)
}

// BadgeEditForm renders the badge edit form.
// GET /settings/progressions/badges/{badgeID}/edit
func (h *Handler) BadgeEditForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireProgressionsAdmin(w, r) {
		return
	}

	badgeID := chi.URLParam(r, "badgeID")
	b, err := h.badgeRepo.GetByID(ctx, badgeID)
	if err != nil || b == nil {
		h.renderError(w, r, http.StatusNotFound, "Not found", "Badge not found.")
		return
	}
	if !h.checkLocationOwnership(w, r, b.LocationID) {
		return
	}

	data := &PageData{
		TemplateData: templateDataFromContext(r, "progressions"),
		BadgeDetail:  b,
		BadgeFormValues: BadgeFormValues{
			Name:        b.Name,
			Description: derefString(b.Description),
			Icon:        b.Icon,
			Color:       b.Color,
		},
	}
	h.render(w, r, "setter/badge-form.html", data)
}

// BadgeUpdate handles badge edits.
// POST /settings/progressions/badges/{badgeID}/edit
func (h *Handler) BadgeUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireProgressionsAdmin(w, r) {
		return
	}

	badgeID := chi.URLParam(r, "badgeID")
	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid form", "Could not parse form data.")
		return
	}

	fv := parseBadgeForm(r)
	if fv.Name == "" {
		data := &PageData{
			TemplateData:    templateDataFromContext(r, "progressions"),
			BadgeFormValues: fv,
			BadgeFormError:  "Name is required.",
		}
		h.render(w, r, "setter/badge-form.html", data)
		return
	}

	b := &model.Badge{
		ID:          badgeID,
		Name:        fv.Name,
		Description: strPtrIfNotEmpty(fv.Description),
		Icon:        fv.Icon,
		Color:       fv.Color,
	}

	if err := h.badgeRepo.Update(ctx, b); err != nil {
		slog.Error("update badge failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Update failed", "Could not update badge.")
		return
	}

	w.Header().Set("HX-Redirect", "/settings/progressions")
	w.WriteHeader(http.StatusOK)
}

// BadgeDelete removes a badge.
// POST /settings/progressions/badges/{badgeID}/delete
func (h *Handler) BadgeDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireProgressionsAdmin(w, r) {
		return
	}

	badgeID := chi.URLParam(r, "badgeID")

	if err := h.badgeRepo.Delete(ctx, badgeID); err != nil {
		slog.Error("delete badge failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Delete failed", "Could not delete badge. It may be assigned to quests.")
		return
	}

	w.Header().Set("HX-Redirect", "/settings/progressions")
	w.WriteHeader(http.StatusOK)
}

// ============================================================
// Helpers
// ============================================================

// requireProgressionsAdmin checks head_setter+ role and active location.
// Returns false (and renders error) if the check fails.
func (h *Handler) requireProgressionsAdmin(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a location.")
		return false
	}
	realRole := middleware.GetWebRole(ctx)
	if middleware.RoleRankValue(realRole) < middleware.RoleRankValue("head_setter") {
		h.renderError(w, r, http.StatusForbidden, "Not authorized", "Progressions admin requires head setter access.")
		return false
	}
	return true
}

func parseDomainForm(r *http.Request) DomainFormValues {
	return DomainFormValues{
		Name:        strings.TrimSpace(r.FormValue("name")),
		Description: strings.TrimSpace(r.FormValue("description")),
		Color:       strings.TrimSpace(r.FormValue("color")),
		Icon:        strings.TrimSpace(r.FormValue("icon")),
		SortOrder:   r.FormValue("sort_order"),
	}
}

func parseQuestForm(r *http.Request) QuestFormValues {
	return QuestFormValues{
		DomainID:              r.FormValue("domain_id"),
		BadgeID:               r.FormValue("badge_id"),
		Name:                  strings.TrimSpace(r.FormValue("name")),
		Description:           strings.TrimSpace(r.FormValue("description")),
		QuestType:             r.FormValue("quest_type"),
		CompletionCriteria:    strings.TrimSpace(r.FormValue("completion_criteria")),
		TargetCount:           r.FormValue("target_count"),
		SuggestedDurationDays: r.FormValue("suggested_duration_days"),
		AvailableFrom:         r.FormValue("available_from"),
		AvailableUntil:        r.FormValue("available_until"),
		SkillLevel:            r.FormValue("skill_level"),
		RequiresCertification: strings.TrimSpace(r.FormValue("requires_certification")),
		RouteTagFilter:        strings.TrimSpace(r.FormValue("route_tag_filter")),
		IsActive:              r.FormValue("is_active"),
		SortOrder:             r.FormValue("sort_order"),
	}
}

func parseBadgeForm(r *http.Request) BadgeFormValues {
	return BadgeFormValues{
		Name:        strings.TrimSpace(r.FormValue("name")),
		Description: strings.TrimSpace(r.FormValue("description")),
		Icon:        strings.TrimSpace(r.FormValue("icon")),
		Color:       strings.TrimSpace(r.FormValue("color")),
	}
}

func buildQuestFromForm(fv QuestFormValues, locationID string) (*model.Quest, string) {
	if fv.Name == "" {
		return nil, "Name is required."
	}
	if fv.DomainID == "" {
		return nil, "Domain is required."
	}
	if fv.Description == "" {
		return nil, "Description is required."
	}

	q := &model.Quest{
		LocationID:         locationID,
		DomainID:           fv.DomainID,
		Name:               fv.Name,
		Description:        fv.Description,
		QuestType:          fv.QuestType,
		CompletionCriteria: fv.CompletionCriteria,
		SkillLevel:         fv.SkillLevel,
		IsActive:           fv.IsActive == "true",
	}

	if fv.BadgeID != "" {
		q.BadgeID = &fv.BadgeID
	}
	if fv.TargetCount != "" {
		if n, err := strconv.Atoi(fv.TargetCount); err == nil && n > 0 {
			q.TargetCount = &n
		}
	}
	if fv.SuggestedDurationDays != "" {
		if n, err := strconv.Atoi(fv.SuggestedDurationDays); err == nil && n > 0 {
			q.SuggestedDurationDays = &n
		}
	}
	if fv.RequiresCertification != "" {
		q.RequiresCertification = &fv.RequiresCertification
	}
	if fv.RouteTagFilter != "" {
		tags := strings.Split(fv.RouteTagFilter, ",")
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}
		q.RouteTagFilter = tags
	}
	if fv.SortOrder != "" {
		if n, err := strconv.Atoi(fv.SortOrder); err == nil {
			q.SortOrder = n
		}
	}

	return q, ""
}

func questToFormValues(q *model.Quest) QuestFormValues {
	fv := QuestFormValues{
		DomainID:           q.DomainID,
		Name:               q.Name,
		Description:        q.Description,
		QuestType:          q.QuestType,
		CompletionCriteria: q.CompletionCriteria,
		SkillLevel:         q.SkillLevel,
		SortOrder:          strconv.Itoa(q.SortOrder),
	}
	if q.IsActive {
		fv.IsActive = "true"
	}
	if q.BadgeID != nil {
		fv.BadgeID = *q.BadgeID
	}
	if q.TargetCount != nil {
		fv.TargetCount = strconv.Itoa(*q.TargetCount)
	}
	if q.SuggestedDurationDays != nil {
		fv.SuggestedDurationDays = strconv.Itoa(*q.SuggestedDurationDays)
	}
	if q.RequiresCertification != nil {
		fv.RequiresCertification = *q.RequiresCertification
	}
	if len(q.RouteTagFilter) > 0 {
		fv.RouteTagFilter = strings.Join(q.RouteTagFilter, ", ")
	}
	if q.AvailableFrom != nil {
		fv.AvailableFrom = q.AvailableFrom.Format("2006-01-02")
	}
	if q.AvailableUntil != nil {
		fv.AvailableUntil = q.AvailableUntil.Format("2006-01-02")
	}
	return fv
}

func strPtrIfNotEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
