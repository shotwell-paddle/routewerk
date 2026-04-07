package webhandler

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

// ============================================================
// Quest Browser — lists available quests + suggestions
// ============================================================

// QuestBrowser renders the quest browser page.
// GET /quests
func (h *Handler) QuestBrowser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a gym first.")
		return
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

	// Filter by domain or skill level if query params are set
	domainFilter := r.URL.Query().Get("domain")
	levelFilter := r.URL.Query().Get("level")
	if domainFilter != "" || levelFilter != "" {
		filtered := make([]service.QuestSuggestion, 0, len(suggestions))
		for _, s := range suggestions {
			if domainFilter != "" && s.Quest.DomainID != domainFilter {
				continue
			}
			if levelFilter != "" && s.Quest.SkillLevel != levelFilter {
				continue
			}
			filtered = append(filtered, s)
		}
		suggestions = filtered
	}

	data := &PageData{
		TemplateData:     templateDataFromContext(r, "quests"),
		AvailableQuests:  available,
		QuestSuggestions: suggestions,
		ActiveQuests:     activeQuests,
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
// Activity Feed
// ============================================================

// QuestActivity renders the activity feed for the location.
// GET /quests/activity
func (h *Handler) QuestActivity(w http.ResponseWriter, r *http.Request) {
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
