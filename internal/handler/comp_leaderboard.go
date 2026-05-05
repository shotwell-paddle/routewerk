package handler

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/shotwell-paddle/routewerk/internal/api"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/service/competition"
)

// Phase 1f wave 4 — leaderboard read endpoint.
//
//   GET /api/v1/competitions/{id}/leaderboard?category=<uuid>
//
// Pulls all relevant attempts + problems + registrations, runs them
// through the comp's scorer (per `competitions.scoring_rule`), assembles
// the ranked entries with display_name + bib_number, and returns. A
// short server-side cache (~2 seconds) keyed by (compID, categoryID)
// keeps repeated polls cheap even if the SPA misbehaves; the action
// endpoint busts the cache on every successful write.
const leaderboardCacheTTL = 2 * time.Second

// GetLeaderboard handles GET /api/v1/competitions/{id}/leaderboard.
func (h *CompHandler) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
	compID := chi.URLParam(r, "id")
	if !isUUID(compID) {
		Error(w, http.StatusBadRequest, "invalid competition id")
		return
	}
	categoryID := r.URL.Query().Get("category")
	if categoryID != "" && !isUUID(categoryID) {
		Error(w, http.StatusBadRequest, "invalid category id")
		return
	}

	// Cache check — also short-circuits comp lookup on a hot path.
	if cached, ok := h.cache.get(compID, categoryID); ok {
		JSON(w, http.StatusOK, cached)
		return
	}

	comp, ok := h.loadComp(w, r, compID)
	if !ok {
		return
	}

	// Pull problems by walking events. League comps are small enough
	// that we'd rather avoid a JOIN-heavy SQL than premature optimize.
	events, err := h.repo.ListEvents(r.Context(), compID)
	if err != nil {
		slog.Error("leaderboard: list events", "competition_id", compID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	var problems []model.CompetitionProblem
	for i := range events {
		ps, err := h.repo.ListProblems(r.Context(), events[i].ID)
		if err != nil {
			slog.Error("leaderboard: list problems", "event_id", events[i].ID, "error", err)
			Error(w, http.StatusInternalServerError, "internal error")
			return
		}
		problems = append(problems, ps...)
	}

	regs, err := h.regRepo.ListByCompetition(r.Context(), compID, categoryID)
	if err != nil {
		slog.Error("leaderboard: list registrations", "competition_id", compID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	attempts, err := h.attemptRepo.ListByCompetition(r.Context(), compID, categoryID)
	if err != nil {
		slog.Error("leaderboard: list attempts", "competition_id", compID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	entries, err := aggregateLeaderboard(comp, regs, problems, attempts)
	if err != nil {
		slog.Error("leaderboard: aggregate", "competition_id", compID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	respCompID, _ := uuid.Parse(compID)
	out := api.Leaderboard{
		CompetitionId: respCompID,
		ScoringRule:   comp.ScoringRule,
		GeneratedAt:   time.Now(),
		Entries:       entries,
	}
	if categoryID != "" {
		c, _ := uuid.Parse(categoryID)
		out.CategoryId = &c
	}

	h.cache.put(compID, categoryID, out)
	JSON(w, http.StatusOK, out)
}

// aggregateLeaderboard is the pure scoring/ranking core, factored out
// for testing. Given a comp + its active registrations + the problem set
// + every attempt by an active registration, it returns ranked entries.
//
// The function is intentionally signature-light (no DB, no http) so the
// scorer behavior is testable in isolation.
func aggregateLeaderboard(
	comp *model.Competition,
	regs []model.CompetitionRegistration,
	problems []model.CompetitionProblem,
	attempts []model.CompetitionAttempt,
) ([]api.LeaderboardEntry, error) {
	scorer, ok := competition.Get(comp.ScoringRule)
	if !ok {
		// Unknown scorer name — return an empty leaderboard rather than
		// erroring; staff can fix the comp's scoring_rule and re-poll.
		// Surfacing as 500 would block the SPA from rendering anything.
		return []api.LeaderboardEntry{}, nil
	}

	// Index attempts by registration_id for per-climber Score calls.
	attemptsByReg := map[string][]competition.Attempt{}
	for i := range attempts {
		a := &attempts[i]
		attemptsByReg[a.RegistrationID] = append(attemptsByReg[a.RegistrationID], competition.Attempt{
			RegistrationID: a.RegistrationID,
			ProblemID:      a.ProblemID,
			Attempts:       a.Attempts,
			ZoneAttempts:   a.ZoneAttempts,
			ZoneReached:    a.ZoneReached,
			TopReached:     a.TopReached,
		})
	}

	// Convert problems to scorer-shape once.
	scorerProblems := make([]competition.Problem, 0, len(problems))
	for i := range problems {
		p := &problems[i]
		scorerProblems = append(scorerProblems, competition.Problem{
			ID:         p.ID,
			Label:      p.Label,
			Points:     p.Points,
			ZonePoints: p.ZonePoints,
			SortOrder:  p.SortOrder,
		})
	}

	scores := make([]competition.ClimberScore, 0, len(regs))
	for i := range regs {
		if !regs[i].IsActive() {
			// Defense in depth — repo already filters withdrawn, but if
			// the caller passed unfiltered registrations, exclude here.
			continue
		}
		s, err := scorer.Score(regs[i].ID, attemptsByReg[regs[i].ID], scorerProblems, comp.ScoringConfig)
		if err != nil {
			return nil, err
		}
		scores = append(scores, s)
	}

	ranked := scorer.Rank(scores, comp.ScoringConfig)

	// Index registrations for display_name + bib + category lookup.
	regByID := map[string]*model.CompetitionRegistration{}
	for i := range regs {
		regByID[regs[i].ID] = &regs[i]
	}

	entries := make([]api.LeaderboardEntry, 0, len(ranked))
	for _, rs := range ranked {
		reg, ok := regByID[rs.RegistrationID]
		if !ok {
			// Score for a registration we don't have metadata for —
			// shouldn't happen, but skip rather than crash.
			continue
		}
		regUUID, _ := uuid.Parse(rs.RegistrationID)
		catUUID, _ := uuid.Parse(reg.CategoryID)
		entries = append(entries, api.LeaderboardEntry{
			Rank:           rs.Rank,
			RegistrationId: regUUID,
			DisplayName:    reg.DisplayName,
			BibNumber:      reg.BibNumber,
			CategoryId:     &catUUID,
			Points:         float32(rs.Points),
			Tops:           rs.Tops,
			Zones:          rs.Zones,
			AttemptsToTop:  rs.AttemptsToTop,
			AttemptsToZone: rs.AttemptsToZone,
		})
	}
	return entries, nil
}

// ── Cache ──────────────────────────────────────────────────

// leaderboardCache is a tiny TTL cache keyed by (compID, categoryID).
// Entries auto-expire (lazy eviction on get); the action endpoint also
// invalidates entries for a comp on every successful write.
type leaderboardCache struct {
	mu      sync.RWMutex
	entries map[string]cachedLeaderboard
}

type cachedLeaderboard struct {
	payload   api.Leaderboard
	expiresAt time.Time
}

func newLeaderboardCache() *leaderboardCache {
	return &leaderboardCache{entries: map[string]cachedLeaderboard{}}
}

func (c *leaderboardCache) key(compID, categoryID string) string {
	return compID + "|" + categoryID
}

func (c *leaderboardCache) get(compID, categoryID string) (api.Leaderboard, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[c.key(compID, categoryID)]
	if !ok {
		return api.Leaderboard{}, false
	}
	if time.Now().After(e.expiresAt) {
		return api.Leaderboard{}, false
	}
	return e.payload, true
}

func (c *leaderboardCache) put(compID, categoryID string, payload api.Leaderboard) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[c.key(compID, categoryID)] = cachedLeaderboard{
		payload:   payload,
		expiresAt: time.Now().Add(leaderboardCacheTTL),
	}
}

// invalidate drops every cached leaderboard for a competition,
// regardless of category. Called by the action endpoint and by
// verify/override on every successful write.
func (c *leaderboardCache) invalidate(compID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prefix := compID + "|"
	for k := range c.entries {
		if strings.HasPrefix(k, prefix) {
			delete(c.entries, k)
		}
	}
}
