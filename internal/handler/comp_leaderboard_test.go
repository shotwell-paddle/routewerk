package handler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/shotwell-paddle/routewerk/internal/api"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

// ── aggregateLeaderboard with each builtin scorer ─────────

// fixture builds one comp + N registrations + a problem set + per-reg
// attempts. Used by every aggregateLeaderboard test below.
func leaderboardFixture(scoringRule string, scoringConfig string) (*model.Competition, []model.CompetitionRegistration, []model.CompetitionProblem, []model.CompetitionAttempt) {
	cfg := json.RawMessage("{}")
	if scoringConfig != "" {
		cfg = json.RawMessage(scoringConfig)
	}
	comp := &model.Competition{
		ID:            uuid.New().String(),
		LocationID:    uuid.New().String(),
		Name:          "League",
		Slug:          "league",
		Format:        "single",
		ScoringRule:   scoringRule,
		ScoringConfig: cfg,
		Status:        model.CompStatusLive,
	}
	categoryID := uuid.New().String()

	regA := model.CompetitionRegistration{
		ID:            uuid.New().String(),
		CompetitionID: comp.ID,
		CategoryID:    categoryID,
		UserID:        uuid.New().String(),
		DisplayName:   "Alice",
	}
	regB := model.CompetitionRegistration{
		ID:            uuid.New().String(),
		CompetitionID: comp.ID,
		CategoryID:    categoryID,
		UserID:        uuid.New().String(),
		DisplayName:   "Bob",
	}

	pts100 := 100.0
	pts200 := 200.0
	problems := []model.CompetitionProblem{
		{ID: uuid.New().String(), Label: "P1", SortOrder: 1, Points: &pts100},
		{ID: uuid.New().String(), Label: "P2", SortOrder: 2, Points: &pts200},
	}
	zone1 := 1
	zone2 := 2
	attempts := []model.CompetitionAttempt{
		// Alice: tops both, 1 attempt each
		{RegistrationID: regA.ID, ProblemID: problems[0].ID, Attempts: 1, ZoneAttempts: &zone1, ZoneReached: true, TopReached: true},
		{RegistrationID: regA.ID, ProblemID: problems[1].ID, Attempts: 1, ZoneAttempts: &zone1, ZoneReached: true, TopReached: true},
		// Bob: tops P1 in 3 attempts, zones P2 in 2 (no top)
		{RegistrationID: regB.ID, ProblemID: problems[0].ID, Attempts: 3, ZoneAttempts: &zone1, ZoneReached: true, TopReached: true},
		{RegistrationID: regB.ID, ProblemID: problems[1].ID, Attempts: 4, ZoneAttempts: &zone2, ZoneReached: true, TopReached: false},
	}
	return comp, []model.CompetitionRegistration{regA, regB}, problems, attempts
}

func TestAggregateLeaderboard_TopZone(t *testing.T) {
	comp, regs, problems, attempts := leaderboardFixture("top_zone", "")
	entries, err := aggregateLeaderboard(comp, regs, problems, attempts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if entries[0].DisplayName != "Alice" || entries[0].Rank != 1 {
		t.Errorf("rank 1 = %s (rank %d), want Alice (rank 1)", entries[0].DisplayName, entries[0].Rank)
	}
	if entries[1].DisplayName != "Bob" || entries[1].Rank != 2 {
		t.Errorf("rank 2 = %s (rank %d), want Bob (rank 2)", entries[1].DisplayName, entries[1].Rank)
	}
	if entries[0].Tops != 2 {
		t.Errorf("Alice tops = %d, want 2", entries[0].Tops)
	}
	if entries[1].Tops != 1 {
		t.Errorf("Bob tops = %d, want 1", entries[1].Tops)
	}
}

func TestAggregateLeaderboard_Fixed(t *testing.T) {
	comp, regs, problems, attempts := leaderboardFixture("fixed", "")
	entries, err := aggregateLeaderboard(comp, regs, problems, attempts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Alice: 100 + 200 = 300. Bob: 100 (only topped P1) = 100.
	if entries[0].DisplayName != "Alice" || entries[0].Points != 300 {
		t.Errorf("rank 1 = %s pts %v, want Alice 300", entries[0].DisplayName, entries[0].Points)
	}
	if entries[1].DisplayName != "Bob" || entries[1].Points != 100 {
		t.Errorf("rank 2 = %s pts %v, want Bob 100", entries[1].DisplayName, entries[1].Points)
	}
}

func TestAggregateLeaderboard_Decay(t *testing.T) {
	comp, regs, problems, attempts := leaderboardFixture("decay", "")
	entries, err := aggregateLeaderboard(comp, regs, problems, attempts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Default decay: base=1000, rate=0.1.
	// Alice: 1000 (1 attempt) × 2 = 2000.
	// Bob: 1000 / (1 + 0.1*2) = ~833 for P1 only.
	if entries[0].DisplayName != "Alice" {
		t.Errorf("rank 1 = %s, want Alice", entries[0].DisplayName)
	}
	if entries[0].Points < 1900 || entries[0].Points > 2100 {
		t.Errorf("Alice points = %v, want ~2000", entries[0].Points)
	}
	if entries[1].DisplayName != "Bob" {
		t.Errorf("rank 2 = %s, want Bob", entries[1].DisplayName)
	}
	if entries[1].Points < 800 || entries[1].Points > 850 {
		t.Errorf("Bob points = %v, want ~833", entries[1].Points)
	}
}

func TestAggregateLeaderboard_UnknownScorerReturnsEmpty(t *testing.T) {
	comp, regs, problems, attempts := leaderboardFixture("nonsense", "")
	entries, err := aggregateLeaderboard(comp, regs, problems, attempts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %d, want 0 (unknown scorer)", len(entries))
	}
}

func TestAggregateLeaderboard_ExcludesWithdrawnRegistrations(t *testing.T) {
	comp, regs, problems, attempts := leaderboardFixture("fixed", "")
	// Mark Alice withdrawn — defense-in-depth check (caller normally
	// passes filtered regs).
	regs[0].WithdrawnAt.Valid = true
	regs[0].WithdrawnAt.Time = time.Now()

	entries, err := aggregateLeaderboard(comp, regs, problems, attempts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1 (withdrawn excluded)", len(entries))
	}
	if entries[0].DisplayName != "Bob" {
		t.Errorf("only entry should be Bob; got %s", entries[0].DisplayName)
	}
}

// ── leaderboardCache ──────────────────────────────────────

func TestLeaderboardCache_GetPutTTL(t *testing.T) {
	c := newLeaderboardCache()
	compID := uuid.New().String()
	cat := uuid.New().String()
	payload := api.Leaderboard{ScoringRule: "top_zone"}

	if _, ok := c.get(compID, cat); ok {
		t.Error("empty cache should miss")
	}

	c.put(compID, cat, payload)
	got, ok := c.get(compID, cat)
	if !ok || got.ScoringRule != "top_zone" {
		t.Errorf("get after put: ok=%v, ScoringRule=%q", ok, got.ScoringRule)
	}
}

func TestLeaderboardCache_Invalidate(t *testing.T) {
	c := newLeaderboardCache()
	comp1 := uuid.New().String()
	comp2 := uuid.New().String()
	c.put(comp1, "", api.Leaderboard{ScoringRule: "fixed"})
	c.put(comp1, uuid.New().String(), api.Leaderboard{ScoringRule: "fixed"})
	c.put(comp2, "", api.Leaderboard{ScoringRule: "decay"})

	c.invalidate(comp1)

	if _, ok := c.get(comp1, ""); ok {
		t.Error("comp1 / no-category should be invalidated")
	}
	if _, ok := c.get(comp2, ""); !ok {
		t.Error("comp2 should NOT be invalidated")
	}
}

func TestLeaderboardCache_TTLExpiry(t *testing.T) {
	c := newLeaderboardCache()
	compID := uuid.New().String()
	c.put(compID, "", api.Leaderboard{ScoringRule: "fixed"})

	// Forcibly expire.
	c.mu.Lock()
	for k, v := range c.entries {
		v.expiresAt = time.Now().Add(-time.Second)
		c.entries[k] = v
	}
	c.mu.Unlock()

	if _, ok := c.get(compID, ""); ok {
		t.Error("expired entry should miss")
	}
}
