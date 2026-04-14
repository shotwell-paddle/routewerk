package webhandler

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

// progressionsGated checks the location's progressions_enabled flag and
// renders a 404 if disabled. Returns true when the request should be
// short-circuited (flag off, no location selected, or load error). All
// climber-facing quest/badge/activity routes call this first so the
// feature is hidden until a gym opts in via the admin UI.
func (h *Handler) progressionsGated(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusNotFound, "Not found", "This page is not available.")
		return true
	}
	loc, err := h.locationRepo.GetByID(ctx, locationID)
	if err != nil || loc == nil || !loc.ProgressionsEnabled {
		h.renderError(w, r, http.StatusNotFound, "Not found", "This page is not available.")
		return true
	}
	return false
}

// ============================================================
// Quest Browser — lists available quests + suggestions
// ============================================================

// QuestBrowser renders the quest browser page.
// GET /quests
func (h *Handler) QuestBrowser(w http.ResponseWriter, r *http.Request) {
	if h.progressionsGated(w, r) {
		return
	}
	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a gym first.")
		return
	}

	// Load domains for filter chips
	domains, err := h.questRepo.ListDomains(ctx, locationID)
	if err != nil {
		slog.Error("list quest domains failed", "error", err)
	}

	// Load available quests
	available, err := h.questSvc.ListAvailable(ctx, locationID)
	if err != nil {
		slog.Error("list available quests failed", "error", err)
	}

	// Load suggestions for this user
	suggestions, err := h.questSvc.SuggestQuests(ctx, user.ID, locationID, 5)
	if err != nil {
		slog.Error("quest suggestions failed", "error", err)
	}

	// Load active quests to show enrollment status
	activeQuests, err := h.questSvc.ListUserQuests(ctx, user.ID, "active")
	if err != nil {
		slog.Error("list active quests failed", "error", err)
	}

	// Domain progress for radar chart
	domainProgress, err := h.questSvc.UserDomainProgress(ctx, user.ID, locationID)
	if err != nil {
		slog.Error("domain progress failed", "error", err)
	}

	// Filter by domain if query param is set
	domainFilter := r.URL.Query().Get("domain")
	if domainFilter != "" {
		available = filterQuestsByDomain(available, domainFilter)
		suggestions = filterSuggestionsByDomain(suggestions, domainFilter)
	}

	// Filter by skill level if query param is set
	skillFilter := r.URL.Query().Get("skill")
	if skillFilter != "" {
		available = filterQuestsBySkill(available, skillFilter)
		suggestions = filterSuggestionsBySkill(suggestions, skillFilter)
	}

	// Build active quest lookup for progress bars in the browser
	activeQuestMap := make(map[string]*model.ClimberQuest, len(activeQuests))
	for i := range activeQuests {
		activeQuestMap[activeQuests[i].QuestID] = &activeQuests[i]
	}

	data := &PageData{
		TemplateData:     templateDataFromContext(r, "quests"),
		QuestDomains:     domains,
		DomainFilter:     domainFilter,
		SkillFilter:      skillFilter,
		AvailableQuests:  available,
		QuestSuggestions: suggestions,
		ActiveQuests:     activeQuests,
		ActiveQuestMap:   activeQuestMap,
		DomainProgress:   domainProgress,
	}
	h.render(w, r, "climber/quests.html", data)
}

// ============================================================
// Quest Detail — view a single quest
// ============================================================

// QuestDetail renders the detail page for a single quest.
// GET /quests/{questID}
func (h *Handler) QuestDetailPage(w http.ResponseWriter, r *http.Request) {
	if h.progressionsGated(w, r) {
		return
	}
	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	locationID := middleware.GetWebLocationID(ctx)
	questID := chi.URLParam(r, "questID")

	quest, err := h.questSvc.GetQuest(ctx, questID)
	if err != nil {
		if errors.Is(err, service.ErrQuestNotFound) {
			h.renderError(w, r, http.StatusNotFound, "Quest not found", "This quest doesn't exist.")
			return
		}
		slog.Error("get quest failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Please try again.")
		return
	}
	if !h.checkLocationOwnership(w, r, quest.LocationID) {
		return
	}

	// Check if user is enrolled
	activeQuests, err := h.questSvc.ListUserQuests(ctx, user.ID, "active")
	if err != nil {
		slog.Error("list active quests failed", "error", err)
	}
	var enrollment *model.ClimberQuest
	for i := range activeQuests {
		if activeQuests[i].QuestID == questID {
			enrollment = &activeQuests[i]
			break
		}
	}

	// If enrolled, load logs
	var questLogs []model.QuestLog
	if enrollment != nil {
		questLogs, err = h.questSvc.ListLogs(ctx, user.ID, enrollment.ID)
		if err != nil {
			slog.Error("list quest logs failed", "error", err)
		}
	}

	// Domain progress
	domainProgress, err := h.questSvc.UserDomainProgress(ctx, user.ID, locationID)
	if err != nil {
		slog.Error("domain progress failed", "error", err)
	}

	data := &PageData{
		TemplateData:   templateDataFromContext(r, "quests"),
		QuestDetail:    quest,
		ClimberQuest:   enrollment,
		QuestLogs:      questLogs,
		DomainProgress: domainProgress,
	}
	h.render(w, r, "climber/quest-detail.html", data)
}

// ============================================================
// Quest Actions — start, log, complete, abandon
// ============================================================

// QuestStart enrolls the climber in a quest.
// POST /quests/{questID}/start
func (h *Handler) QuestStart(w http.ResponseWriter, r *http.Request) {
	if h.progressionsGated(w, r) {
		return
	}
	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	locationID := middleware.GetWebLocationID(ctx)
	questID := chi.URLParam(r, "questID")

	_, err := h.questSvc.StartQuest(ctx, user.ID, questID, locationID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrQuestNotFound):
			h.renderError(w, r, http.StatusNotFound, "Quest not found", "This quest doesn't exist.")
		case errors.Is(err, service.ErrQuestNotActive):
			h.renderError(w, r, http.StatusBadRequest, "Quest unavailable", "This quest is not currently active.")
		case errors.Is(err, service.ErrQuestNotAvailable):
			h.renderError(w, r, http.StatusBadRequest, "Quest unavailable", "This quest is outside its availability window.")
		case errors.Is(err, service.ErrAlreadyEnrolled):
			// Not really an error — redirect to the quest
			w.Header().Set("HX-Redirect", "/quests/"+questID)
			w.WriteHeader(http.StatusOK)
		default:
			slog.Error("start quest failed", "error", err)
			h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not start quest. Please try again.")
		}
		return
	}

	w.Header().Set("HX-Redirect", "/quests/"+questID)
	w.WriteHeader(http.StatusOK)
}

// QuestLogProgress records a progress entry.
// POST /quests/{questID}/log
func (h *Handler) QuestLogProgress(w http.ResponseWriter, r *http.Request) {
	if h.progressionsGated(w, r) {
		return
	}
	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	questID := chi.URLParam(r, "questID")

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid form", "Could not parse form data.")
		return
	}

	// Find the user's active enrollment for this quest
	activeQuests, err := h.questSvc.ListUserQuests(ctx, user.ID, "active")
	if err != nil {
		slog.Error("list active quests failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Please try again.")
		return
	}
	var enrollment *model.ClimberQuest
	for i := range activeQuests {
		if activeQuests[i].QuestID == questID {
			enrollment = &activeQuests[i]
			break
		}
	}
	if enrollment == nil {
		h.renderError(w, r, http.StatusBadRequest, "Not enrolled", "You need to start this quest first.")
		return
	}

	logType := strings.TrimSpace(r.FormValue("log_type"))
	if logType == "" {
		logType = "general"
	}
	notes := strings.TrimSpace(r.FormValue("notes"))
	routeID := strings.TrimSpace(r.FormValue("route_id"))

	log := &model.QuestLog{
		LogType: logType,
	}
	if notes != "" {
		log.Notes = &notes
	}
	if routeID != "" {
		log.RouteID = &routeID
	}

	if err := h.questSvc.LogProgress(ctx, user.ID, enrollment.ID, log); err != nil {
		slog.Error("log quest progress failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not log progress. Please try again.")
		return
	}

	w.Header().Set("HX-Redirect", "/quests/"+questID)
	w.WriteHeader(http.StatusOK)
}

// QuestComplete manually completes a quest.
// POST /quests/{questID}/complete
func (h *Handler) QuestComplete(w http.ResponseWriter, r *http.Request) {
	if h.progressionsGated(w, r) {
		return
	}
	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	questID := chi.URLParam(r, "questID")

	// Find enrollment
	activeQuests, err := h.questSvc.ListUserQuests(ctx, user.ID, "active")
	if err != nil {
		slog.Error("list active quests failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Please try again.")
		return
	}
	var enrollment *model.ClimberQuest
	for i := range activeQuests {
		if activeQuests[i].QuestID == questID {
			enrollment = &activeQuests[i]
			break
		}
	}
	if enrollment == nil {
		h.renderError(w, r, http.StatusBadRequest, "Not enrolled", "You need to start this quest first.")
		return
	}

	if err := h.questSvc.CompleteQuest(ctx, user.ID, enrollment.ID); err != nil {
		slog.Error("complete quest failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not complete quest. Please try again.")
		return
	}

	w.Header().Set("HX-Redirect", "/quests/"+questID)
	w.WriteHeader(http.StatusOK)
}

// QuestAbandon lets a climber drop out of a quest.
// POST /quests/{questID}/abandon
func (h *Handler) QuestAbandon(w http.ResponseWriter, r *http.Request) {
	if h.progressionsGated(w, r) {
		return
	}
	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	questID := chi.URLParam(r, "questID")

	// Find enrollment
	activeQuests, err := h.questSvc.ListUserQuests(ctx, user.ID, "active")
	if err != nil {
		slog.Error("list active quests failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Please try again.")
		return
	}
	var enrollment *model.ClimberQuest
	for i := range activeQuests {
		if activeQuests[i].QuestID == questID {
			enrollment = &activeQuests[i]
			break
		}
	}
	if enrollment == nil {
		h.renderError(w, r, http.StatusBadRequest, "Not enrolled", "You're not enrolled in this quest.")
		return
	}

	if err := h.questSvc.AbandonQuest(ctx, user.ID, enrollment.ID); err != nil {
		slog.Error("abandon quest failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not abandon quest. Please try again.")
		return
	}

	w.Header().Set("HX-Redirect", "/quests")
	w.WriteHeader(http.StatusOK)
}

// ============================================================
// My Quests — active + completed
// ============================================================

// MyQuests shows the climber's active and completed quests.
// GET /quests/mine
func (h *Handler) MyQuests(w http.ResponseWriter, r *http.Request) {
	if h.progressionsGated(w, r) {
		return
	}
	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a gym first.")
		return
	}

	activeQuests, err := h.questSvc.ListUserQuests(ctx, user.ID, "active")
	if err != nil {
		slog.Error("list active quests failed", "error", err)
	}

	completedQuests, err := h.questSvc.ListUserQuests(ctx, user.ID, "completed")
	if err != nil {
		slog.Error("list completed quests failed", "error", err)
	}

	// Badges for this location
	badges, err := h.badgeRepo.ListUserBadgesForLocation(ctx, user.ID, locationID)
	if err != nil {
		slog.Error("list climber badges failed", "error", err)
	}

	// Domain progress
	domainProgress, err := h.questSvc.UserDomainProgress(ctx, user.ID, locationID)
	if err != nil {
		slog.Error("domain progress failed", "error", err)
	}

	data := &PageData{
		TemplateData:    templateDataFromContext(r, "quests"),
		ActiveQuests:    activeQuests,
		CompletedQuests: completedQuests,
		ClimberBadges:   badges,
		DomainProgress:  domainProgress,
	}
	h.render(w, r, "climber/my-quests.html", data)
}

// ============================================================
// Badge Showcase — all badges earned + available
// ============================================================

// BadgeShowcase renders the badge collection page.
// GET /quests/badges
func (h *Handler) BadgeShowcase(w http.ResponseWriter, r *http.Request) {
	if h.progressionsGated(w, r) {
		return
	}
	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a gym first.")
		return
	}

	// All badges available at this location
	allBadges, err := h.badgeRepo.ListByLocation(ctx, locationID)
	if err != nil {
		slog.Error("list badges failed", "error", err)
	}

	// Badges this climber has earned
	earnedBadges, err := h.badgeRepo.ListUserBadgesForLocation(ctx, user.ID, locationID)
	if err != nil {
		slog.Error("list climber badges failed", "error", err)
	}

	// Build earned lookup
	earnedIDs := make(map[string]bool, len(earnedBadges))
	for _, cb := range earnedBadges {
		earnedIDs[cb.BadgeID] = true
	}

	// Domain progress for sidebar
	domainProgress, err := h.questSvc.UserDomainProgress(ctx, user.ID, locationID)
	if err != nil {
		slog.Error("domain progress failed", "error", err)
	}

	data := &PageData{
		TemplateData:   templateDataFromContext(r, "quests"),
		Badges:         allBadges,
		ClimberBadges:  earnedBadges,
		EarnedBadgeIDs: earnedIDs,
		DomainProgress: domainProgress,
	}
	h.render(w, r, "climber/badge-showcase.html", data)
}

// ============================================================
// Activity Feed
// ============================================================

// QuestActivity renders the activity feed for the location.
// GET /quests/activity
func (h *Handler) QuestActivity(w http.ResponseWriter, r *http.Request) {
	if h.progressionsGated(w, r) {
		return
	}
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a gym first.")
		return
	}

	feed, err := h.activityRepo.ListByLocation(ctx, locationID, 50, 0)
	if err != nil {
		slog.Error("activity feed failed", "error", err)
	}

	data := &PageData{
		TemplateData: templateDataFromContext(r, "quests"),
		ActivityFeed: feed,
	}
	h.render(w, r, "climber/quest-activity.html", data)
}

// ============================================================
// Filter helpers — extracted for testability
// ============================================================

func filterQuestsByDomain(quests []repository.QuestListItem, domainID string) []repository.QuestListItem {
	out := make([]repository.QuestListItem, 0, len(quests))
	for _, q := range quests {
		if q.DomainID == domainID {
			out = append(out, q)
		}
	}
	return out
}

func filterSuggestionsByDomain(suggestions []service.QuestSuggestion, domainID string) []service.QuestSuggestion {
	out := make([]service.QuestSuggestion, 0, len(suggestions))
	for _, s := range suggestions {
		if s.Quest.DomainID == domainID {
			out = append(out, s)
		}
	}
	return out
}

func filterQuestsBySkill(quests []repository.QuestListItem, skill string) []repository.QuestListItem {
	out := make([]repository.QuestListItem, 0, len(quests))
	for _, q := range quests {
		if q.SkillLevel == skill {
			out = append(out, q)
		}
	}
	return out
}

func filterSuggestionsBySkill(suggestions []service.QuestSuggestion, skill string) []service.QuestSuggestion {
	out := make([]service.QuestSuggestion, 0, len(suggestions))
	for _, s := range suggestions {
		if s.Quest.SkillLevel == skill {
			out = append(out, s)
		}
	}
	return out
}
