package webhandler

import (
	"net/http"
	"testing"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

// ── filterQuestsByDomain ───────────────────────────────────

func TestFilterQuestsByDomain(t *testing.T) {
	quests := []repository.QuestListItem{
		{Quest: model.Quest{DomainID: "d-1", Name: "Slab Master"}},
		{Quest: model.Quest{DomainID: "d-2", Name: "Overhang Explorer"}},
		{Quest: model.Quest{DomainID: "d-1", Name: "Slab Novice"}},
		{Quest: model.Quest{DomainID: "d-3", Name: "Crimp King"}},
	}

	tests := []struct {
		name     string
		domainID string
		wantLen  int
		wantNames []string
	}{
		{
			name:      "filter to domain 1",
			domainID:  "d-1",
			wantLen:   2,
			wantNames: []string{"Slab Master", "Slab Novice"},
		},
		{
			name:      "filter to domain 2",
			domainID:  "d-2",
			wantLen:   1,
			wantNames: []string{"Overhang Explorer"},
		},
		{
			name:     "filter to nonexistent domain",
			domainID: "d-999",
			wantLen:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterQuestsByDomain(quests, tt.domainID)
			if len(got) != tt.wantLen {
				t.Fatalf("len = %d, want %d", len(got), tt.wantLen)
			}
			for i, name := range tt.wantNames {
				if got[i].Name != name {
					t.Errorf("got[%d].Name = %q, want %q", i, got[i].Name, name)
				}
			}
		})
	}
}

func TestFilterQuestsByDomain_EmptySlice(t *testing.T) {
	got := filterQuestsByDomain(nil, "d-1")
	if len(got) != 0 {
		t.Errorf("expected empty result, got %d", len(got))
	}
}

// ── filterQuestsBySkill ────────────────────────────────────

func TestFilterQuestsBySkill(t *testing.T) {
	quests := []repository.QuestListItem{
		{Quest: model.Quest{SkillLevel: "beginner", Name: "First Steps"}},
		{Quest: model.Quest{SkillLevel: "intermediate", Name: "Mid Challenge"}},
		{Quest: model.Quest{SkillLevel: "advanced", Name: "Pro Mode"}},
		{Quest: model.Quest{SkillLevel: "beginner", Name: "Easy Warmup"}},
		{Quest: model.Quest{SkillLevel: "intermediate", Name: "Technique Focus"}},
	}

	tests := []struct {
		name     string
		skill    string
		wantLen  int
		wantNames []string
	}{
		{
			name:      "filter beginner",
			skill:     "beginner",
			wantLen:   2,
			wantNames: []string{"First Steps", "Easy Warmup"},
		},
		{
			name:      "filter intermediate",
			skill:     "intermediate",
			wantLen:   2,
			wantNames: []string{"Mid Challenge", "Technique Focus"},
		},
		{
			name:      "filter advanced",
			skill:     "advanced",
			wantLen:   1,
			wantNames: []string{"Pro Mode"},
		},
		{
			name:    "filter unknown skill",
			skill:   "expert",
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterQuestsBySkill(quests, tt.skill)
			if len(got) != tt.wantLen {
				t.Fatalf("len = %d, want %d", len(got), tt.wantLen)
			}
			for i, name := range tt.wantNames {
				if got[i].Name != name {
					t.Errorf("got[%d].Name = %q, want %q", i, got[i].Name, name)
				}
			}
		})
	}
}

func TestFilterQuestsBySkill_EmptySlice(t *testing.T) {
	got := filterQuestsBySkill(nil, "beginner")
	if len(got) != 0 {
		t.Errorf("expected empty result, got %d", len(got))
	}
}

// ── filterSuggestionsByDomain ──────────────────────────────

func TestFilterSuggestionsByDomain(t *testing.T) {
	suggestions := []service.QuestSuggestion{
		{
			Quest:  repository.QuestListItem{Quest: model.Quest{DomainID: "d-1", Name: "Slab Master"}},
			Score:  0.9,
			Reason: "Popular",
		},
		{
			Quest:  repository.QuestListItem{Quest: model.Quest{DomainID: "d-2", Name: "Overhang"}},
			Score:  0.7,
			Reason: "New domain",
		},
		{
			Quest:  repository.QuestListItem{Quest: model.Quest{DomainID: "d-1", Name: "Slab Novice"}},
			Score:  0.5,
			Reason: "Good fit",
		},
	}

	got := filterSuggestionsByDomain(suggestions, "d-1")
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Quest.Name != "Slab Master" {
		t.Errorf("got[0].Quest.Name = %q, want Slab Master", got[0].Quest.Name)
	}
	if got[1].Quest.Name != "Slab Novice" {
		t.Errorf("got[1].Quest.Name = %q, want Slab Novice", got[1].Quest.Name)
	}
	// Verify score and reason are preserved
	if got[0].Score != 0.9 {
		t.Errorf("got[0].Score = %f, want 0.9", got[0].Score)
	}
	if got[0].Reason != "Popular" {
		t.Errorf("got[0].Reason = %q, want Popular", got[0].Reason)
	}
}

func TestFilterSuggestionsByDomain_EmptySlice(t *testing.T) {
	got := filterSuggestionsByDomain(nil, "d-1")
	if len(got) != 0 {
		t.Errorf("expected empty result, got %d", len(got))
	}
}

func TestFilterSuggestionsByDomain_NoMatch(t *testing.T) {
	suggestions := []service.QuestSuggestion{
		{Quest: repository.QuestListItem{Quest: model.Quest{DomainID: "d-1"}}},
	}
	got := filterSuggestionsByDomain(suggestions, "d-999")
	if len(got) != 0 {
		t.Errorf("expected 0 results, got %d", len(got))
	}
}

// ── filterSuggestionsBySkill ───────────────────────────────

func TestFilterSuggestionsBySkill(t *testing.T) {
	suggestions := []service.QuestSuggestion{
		{
			Quest:  repository.QuestListItem{Quest: model.Quest{SkillLevel: "beginner", Name: "Easy Start"}},
			Score:  0.8,
			Reason: "Good for beginners",
		},
		{
			Quest:  repository.QuestListItem{Quest: model.Quest{SkillLevel: "advanced", Name: "Hard Mode"}},
			Score:  0.6,
			Reason: "Challenge yourself",
		},
		{
			Quest:  repository.QuestListItem{Quest: model.Quest{SkillLevel: "beginner", Name: "Warmup"}},
			Score:  0.4,
			Reason: "Popular",
		},
	}

	got := filterSuggestionsBySkill(suggestions, "beginner")
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Quest.Name != "Easy Start" {
		t.Errorf("got[0].Quest.Name = %q, want Easy Start", got[0].Quest.Name)
	}
	if got[1].Quest.Name != "Warmup" {
		t.Errorf("got[1].Quest.Name = %q, want Warmup", got[1].Quest.Name)
	}

	// Filter advanced
	got2 := filterSuggestionsBySkill(suggestions, "advanced")
	if len(got2) != 1 {
		t.Fatalf("advanced len = %d, want 1", len(got2))
	}
	if got2[0].Quest.Name != "Hard Mode" {
		t.Errorf("got[0].Quest.Name = %q, want Hard Mode", got2[0].Quest.Name)
	}
}

func TestFilterSuggestionsBySkill_EmptySlice(t *testing.T) {
	got := filterSuggestionsBySkill(nil, "beginner")
	if len(got) != 0 {
		t.Errorf("expected empty result, got %d", len(got))
	}
}

// ── Combined filters ───────────────────────────────────────
// These tests verify that domain + skill filters compose correctly
// when applied sequentially (as the handler does).

func TestCombinedDomainAndSkillFilter(t *testing.T) {
	quests := []repository.QuestListItem{
		{Quest: model.Quest{DomainID: "d-1", SkillLevel: "beginner", Name: "Slab Beginner"}},
		{Quest: model.Quest{DomainID: "d-1", SkillLevel: "advanced", Name: "Slab Pro"}},
		{Quest: model.Quest{DomainID: "d-2", SkillLevel: "beginner", Name: "Overhang Beginner"}},
		{Quest: model.Quest{DomainID: "d-2", SkillLevel: "advanced", Name: "Overhang Pro"}},
	}

	// First filter by domain d-1
	filtered := filterQuestsByDomain(quests, "d-1")
	if len(filtered) != 2 {
		t.Fatalf("after domain filter: len = %d, want 2", len(filtered))
	}

	// Then filter by skill beginner
	filtered = filterQuestsBySkill(filtered, "beginner")
	if len(filtered) != 1 {
		t.Fatalf("after skill filter: len = %d, want 1", len(filtered))
	}
	if filtered[0].Name != "Slab Beginner" {
		t.Errorf("Name = %q, want Slab Beginner", filtered[0].Name)
	}
}

func TestCombinedFilters_NoResults(t *testing.T) {
	quests := []repository.QuestListItem{
		{Quest: model.Quest{DomainID: "d-1", SkillLevel: "beginner", Name: "Easy One"}},
		{Quest: model.Quest{DomainID: "d-2", SkillLevel: "advanced", Name: "Hard Two"}},
	}

	// Filter domain d-1 + skill advanced = no results
	filtered := filterQuestsByDomain(quests, "d-1")
	filtered = filterQuestsBySkill(filtered, "advanced")
	if len(filtered) != 0 {
		t.Errorf("expected 0, got %d", len(filtered))
	}
}

func TestCombinedSuggestionFilters(t *testing.T) {
	suggestions := []service.QuestSuggestion{
		{Quest: repository.QuestListItem{Quest: model.Quest{DomainID: "d-1", SkillLevel: "beginner", Name: "A"}}},
		{Quest: repository.QuestListItem{Quest: model.Quest{DomainID: "d-1", SkillLevel: "advanced", Name: "B"}}},
		{Quest: repository.QuestListItem{Quest: model.Quest{DomainID: "d-2", SkillLevel: "beginner", Name: "C"}}},
	}

	filtered := filterSuggestionsByDomain(suggestions, "d-1")
	filtered = filterSuggestionsBySkill(filtered, "beginner")
	if len(filtered) != 1 {
		t.Fatalf("len = %d, want 1", len(filtered))
	}
	if filtered[0].Quest.Name != "A" {
		t.Errorf("Name = %q, want A", filtered[0].Quest.Name)
	}
}

// ── buildQuestFromForm ─────────────────────────────────────

func TestBuildQuestFromForm_ValidFull(t *testing.T) {
	fv := QuestFormValues{
		DomainID:              "d-1",
		BadgeID:               "b-1",
		Name:                  "Slab Master",
		Description:           "Complete 10 slab routes",
		QuestType:             "permanent",
		CompletionCriteria:    "Send 10 slab routes of any grade",
		TargetCount:           "10",
		SuggestedDurationDays: "30",
		SkillLevel:            "beginner",
		RequiresCertification: "lead",
		RouteTagFilter:        "slab, balance, footwork",
		IsActive:              "true",
		SortOrder:             "5",
	}

	q, errMsg := buildQuestFromForm(fv, "loc-1")
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	if q.LocationID != "loc-1" {
		t.Errorf("LocationID = %q, want loc-1", q.LocationID)
	}
	if q.DomainID != "d-1" {
		t.Errorf("DomainID = %q, want d-1", q.DomainID)
	}
	if q.Name != "Slab Master" {
		t.Errorf("Name = %q, want Slab Master", q.Name)
	}
	if q.BadgeID == nil || *q.BadgeID != "b-1" {
		t.Errorf("BadgeID = %v, want b-1", q.BadgeID)
	}
	if q.TargetCount == nil || *q.TargetCount != 10 {
		t.Errorf("TargetCount = %v, want 10", q.TargetCount)
	}
	if q.SuggestedDurationDays == nil || *q.SuggestedDurationDays != 30 {
		t.Errorf("SuggestedDurationDays = %v, want 30", q.SuggestedDurationDays)
	}
	if q.RequiresCertification == nil || *q.RequiresCertification != "lead" {
		t.Errorf("RequiresCertification = %v, want lead", q.RequiresCertification)
	}
	if len(q.RouteTagFilter) != 3 {
		t.Fatalf("RouteTagFilter len = %d, want 3", len(q.RouteTagFilter))
	}
	if q.RouteTagFilter[0] != "slab" || q.RouteTagFilter[1] != "balance" || q.RouteTagFilter[2] != "footwork" {
		t.Errorf("RouteTagFilter = %v, want [slab balance footwork]", q.RouteTagFilter)
	}
	if !q.IsActive {
		t.Error("IsActive should be true")
	}
	if q.SortOrder != 5 {
		t.Errorf("SortOrder = %d, want 5", q.SortOrder)
	}
	if q.SkillLevel != "beginner" {
		t.Errorf("SkillLevel = %q, want beginner", q.SkillLevel)
	}
}

func TestBuildQuestFromForm_Validation(t *testing.T) {
	tests := []struct {
		name    string
		fv      QuestFormValues
		wantErr string
	}{
		{
			name:    "missing name",
			fv:      QuestFormValues{DomainID: "d-1", Description: "desc"},
			wantErr: "Name is required.",
		},
		{
			name:    "missing domain",
			fv:      QuestFormValues{Name: "Test", Description: "desc"},
			wantErr: "Domain is required.",
		},
		{
			name:    "missing description",
			fv:      QuestFormValues{Name: "Test", DomainID: "d-1"},
			wantErr: "Description is required.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, errMsg := buildQuestFromForm(tt.fv, "loc-1")
			if q != nil {
				t.Error("expected nil quest on validation failure")
			}
			if errMsg != tt.wantErr {
				t.Errorf("error = %q, want %q", errMsg, tt.wantErr)
			}
		})
	}
}

func TestBuildQuestFromForm_OptionalFields(t *testing.T) {
	fv := QuestFormValues{
		Name:        "Minimal Quest",
		DomainID:    "d-1",
		Description: "A simple quest",
		QuestType:   "permanent",
		SkillLevel:  "intermediate",
	}

	q, errMsg := buildQuestFromForm(fv, "loc-1")
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	if q.BadgeID != nil {
		t.Errorf("BadgeID should be nil, got %v", q.BadgeID)
	}
	if q.TargetCount != nil {
		t.Errorf("TargetCount should be nil, got %v", q.TargetCount)
	}
	if q.SuggestedDurationDays != nil {
		t.Errorf("SuggestedDurationDays should be nil, got %v", q.SuggestedDurationDays)
	}
	if q.RequiresCertification != nil {
		t.Errorf("RequiresCertification should be nil, got %v", q.RequiresCertification)
	}
	if len(q.RouteTagFilter) != 0 {
		t.Errorf("RouteTagFilter should be empty, got %v", q.RouteTagFilter)
	}
	if q.IsActive {
		t.Error("IsActive should be false when not set to 'true'")
	}
}

func TestBuildQuestFromForm_InvalidNumbers(t *testing.T) {
	fv := QuestFormValues{
		Name:                  "Test",
		DomainID:              "d-1",
		Description:           "desc",
		TargetCount:           "abc",
		SuggestedDurationDays: "-5",
		SortOrder:             "xyz",
	}

	q, errMsg := buildQuestFromForm(fv, "loc-1")
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	// Invalid target count → nil
	if q.TargetCount != nil {
		t.Errorf("TargetCount should be nil for invalid input, got %v", q.TargetCount)
	}
	// Negative duration → nil (n > 0 check)
	if q.SuggestedDurationDays != nil {
		t.Errorf("SuggestedDurationDays should be nil for negative, got %v", q.SuggestedDurationDays)
	}
	// Invalid sort order → stays 0
	if q.SortOrder != 0 {
		t.Errorf("SortOrder should be 0 for invalid input, got %d", q.SortOrder)
	}
}

func TestBuildQuestFromForm_ZeroTargetCount(t *testing.T) {
	fv := QuestFormValues{
		Name:        "Test",
		DomainID:    "d-1",
		Description: "desc",
		TargetCount: "0",
	}

	q, _ := buildQuestFromForm(fv, "loc-1")
	// Zero target count → nil (n > 0 check)
	if q.TargetCount != nil {
		t.Errorf("TargetCount should be nil for zero, got %v", q.TargetCount)
	}
}

// ── questToFormValues ──────────────────────────────────────

func TestQuestToFormValues_FullQuest(t *testing.T) {
	badgeID := "b-1"
	target := 10
	duration := 30
	cert := "lead"
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

	q := &model.Quest{
		DomainID:              "d-1",
		BadgeID:               &badgeID,
		Name:                  "Slab Master",
		Description:           "Complete 10 slab routes",
		QuestType:             "permanent",
		CompletionCriteria:    "Send 10 slab routes",
		TargetCount:           &target,
		SuggestedDurationDays: &duration,
		AvailableFrom:         &from,
		AvailableUntil:        &until,
		SkillLevel:            "beginner",
		RequiresCertification: &cert,
		RouteTagFilter:        []string{"slab", "balance"},
		IsActive:              true,
		SortOrder:             5,
	}

	fv := questToFormValues(q)

	if fv.DomainID != "d-1" {
		t.Errorf("DomainID = %q, want d-1", fv.DomainID)
	}
	if fv.BadgeID != "b-1" {
		t.Errorf("BadgeID = %q, want b-1", fv.BadgeID)
	}
	if fv.Name != "Slab Master" {
		t.Errorf("Name = %q, want Slab Master", fv.Name)
	}
	if fv.TargetCount != "10" {
		t.Errorf("TargetCount = %q, want 10", fv.TargetCount)
	}
	if fv.SuggestedDurationDays != "30" {
		t.Errorf("SuggestedDurationDays = %q, want 30", fv.SuggestedDurationDays)
	}
	if fv.AvailableFrom != "2026-01-01" {
		t.Errorf("AvailableFrom = %q, want 2026-01-01", fv.AvailableFrom)
	}
	if fv.AvailableUntil != "2026-06-30" {
		t.Errorf("AvailableUntil = %q, want 2026-06-30", fv.AvailableUntil)
	}
	if fv.RequiresCertification != "lead" {
		t.Errorf("RequiresCertification = %q, want lead", fv.RequiresCertification)
	}
	if fv.RouteTagFilter != "slab, balance" {
		t.Errorf("RouteTagFilter = %q, want 'slab, balance'", fv.RouteTagFilter)
	}
	if fv.IsActive != "true" {
		t.Errorf("IsActive = %q, want true", fv.IsActive)
	}
	if fv.SortOrder != "5" {
		t.Errorf("SortOrder = %q, want 5", fv.SortOrder)
	}
}

func TestQuestToFormValues_MinimalQuest(t *testing.T) {
	q := &model.Quest{
		DomainID:    "d-1",
		Name:        "Simple Quest",
		Description: "desc",
		QuestType:   "permanent",
		SkillLevel:  "beginner",
	}

	fv := questToFormValues(q)

	if fv.BadgeID != "" {
		t.Errorf("BadgeID should be empty, got %q", fv.BadgeID)
	}
	if fv.TargetCount != "" {
		t.Errorf("TargetCount should be empty, got %q", fv.TargetCount)
	}
	if fv.SuggestedDurationDays != "" {
		t.Errorf("SuggestedDurationDays should be empty, got %q", fv.SuggestedDurationDays)
	}
	if fv.AvailableFrom != "" {
		t.Errorf("AvailableFrom should be empty, got %q", fv.AvailableFrom)
	}
	if fv.AvailableUntil != "" {
		t.Errorf("AvailableUntil should be empty, got %q", fv.AvailableUntil)
	}
	if fv.RequiresCertification != "" {
		t.Errorf("RequiresCertification should be empty, got %q", fv.RequiresCertification)
	}
	if fv.RouteTagFilter != "" {
		t.Errorf("RouteTagFilter should be empty, got %q", fv.RouteTagFilter)
	}
	// IsActive defaults to false → empty string
	if fv.IsActive != "" {
		t.Errorf("IsActive should be empty for inactive quest, got %q", fv.IsActive)
	}
}

// ── buildQuestFromForm ↔ questToFormValues round-trip ──────

func TestQuestFormRoundTrip(t *testing.T) {
	// Build a quest from form values, convert back, verify losslessness
	fv := QuestFormValues{
		DomainID:              "d-1",
		BadgeID:               "b-1",
		Name:                  "Round Trip Quest",
		Description:           "Testing round trip",
		QuestType:             "seasonal",
		CompletionCriteria:    "Do the thing",
		TargetCount:           "5",
		SuggestedDurationDays: "14",
		SkillLevel:            "advanced",
		RequiresCertification: "belay",
		RouteTagFilter:        "crimp, pinch",
		IsActive:              "true",
		SortOrder:             "3",
	}

	q, errMsg := buildQuestFromForm(fv, "loc-1")
	if errMsg != "" {
		t.Fatalf("build failed: %s", errMsg)
	}

	fv2 := questToFormValues(q)

	if fv2.DomainID != fv.DomainID {
		t.Errorf("DomainID round trip: %q != %q", fv2.DomainID, fv.DomainID)
	}
	if fv2.BadgeID != fv.BadgeID {
		t.Errorf("BadgeID round trip: %q != %q", fv2.BadgeID, fv.BadgeID)
	}
	if fv2.Name != fv.Name {
		t.Errorf("Name round trip: %q != %q", fv2.Name, fv.Name)
	}
	if fv2.TargetCount != fv.TargetCount {
		t.Errorf("TargetCount round trip: %q != %q", fv2.TargetCount, fv.TargetCount)
	}
	if fv2.SuggestedDurationDays != fv.SuggestedDurationDays {
		t.Errorf("SuggestedDurationDays round trip: %q != %q", fv2.SuggestedDurationDays, fv.SuggestedDurationDays)
	}
	if fv2.SkillLevel != fv.SkillLevel {
		t.Errorf("SkillLevel round trip: %q != %q", fv2.SkillLevel, fv.SkillLevel)
	}
	if fv2.RequiresCertification != fv.RequiresCertification {
		t.Errorf("RequiresCertification round trip: %q != %q", fv2.RequiresCertification, fv.RequiresCertification)
	}
	if fv2.IsActive != fv.IsActive {
		t.Errorf("IsActive round trip: %q != %q", fv2.IsActive, fv.IsActive)
	}
	if fv2.SortOrder != fv.SortOrder {
		t.Errorf("SortOrder round trip: %q != %q", fv2.SortOrder, fv.SortOrder)
	}
}

// ── strPtrIfNotEmpty ───────────────────────────────────────

func TestStrPtrIfNotEmpty(t *testing.T) {
	if strPtrIfNotEmpty("") != nil {
		t.Error("empty string should return nil")
	}
	p := strPtrIfNotEmpty("hello")
	if p == nil || *p != "hello" {
		t.Errorf("strPtrIfNotEmpty(hello) = %v, want hello", p)
	}
}

// ── EarnedBadgeIDs map building ────────────────────────────
// This logic is used in BadgeShowcase to build the earned lookup.

func TestEarnedBadgeIDsMap(t *testing.T) {
	earnedBadges := []model.ClimberBadge{
		{BadgeID: "b-1"},
		{BadgeID: "b-3"},
		{BadgeID: "b-5"},
	}

	earnedIDs := make(map[string]bool, len(earnedBadges))
	for _, cb := range earnedBadges {
		earnedIDs[cb.BadgeID] = true
	}

	if !earnedIDs["b-1"] {
		t.Error("b-1 should be in earned set")
	}
	if !earnedIDs["b-3"] {
		t.Error("b-3 should be in earned set")
	}
	if !earnedIDs["b-5"] {
		t.Error("b-5 should be in earned set")
	}
	if earnedIDs["b-2"] {
		t.Error("b-2 should NOT be in earned set")
	}
	if earnedIDs["b-4"] {
		t.Error("b-4 should NOT be in earned set")
	}
}

func TestEarnedBadgeIDsMap_Empty(t *testing.T) {
	var earnedBadges []model.ClimberBadge
	earnedIDs := make(map[string]bool, len(earnedBadges))
	for _, cb := range earnedBadges {
		earnedIDs[cb.BadgeID] = true
	}

	if len(earnedIDs) != 0 {
		t.Errorf("expected empty map, got %d entries", len(earnedIDs))
	}
}

// ── ActiveQuestMap building ────────────────────────────────
// This logic is used in QuestBrowser to provide progress data.

func TestActiveQuestMap(t *testing.T) {
	activeQuests := []model.ClimberQuest{
		{ID: "cq-1", QuestID: "q-1", ProgressCount: 3},
		{ID: "cq-2", QuestID: "q-5", ProgressCount: 7},
	}

	m := make(map[string]*model.ClimberQuest, len(activeQuests))
	for i := range activeQuests {
		m[activeQuests[i].QuestID] = &activeQuests[i]
	}

	if m["q-1"] == nil {
		t.Fatal("q-1 should be in map")
	}
	if m["q-1"].ProgressCount != 3 {
		t.Errorf("q-1 ProgressCount = %d, want 3", m["q-1"].ProgressCount)
	}
	if m["q-5"] == nil {
		t.Fatal("q-5 should be in map")
	}
	if m["q-5"].ProgressCount != 7 {
		t.Errorf("q-5 ProgressCount = %d, want 7", m["q-5"].ProgressCount)
	}
	if m["q-999"] != nil {
		t.Error("q-999 should not be in map")
	}
}

// ── parseDomainForm ────────────────────────────────────────

func TestParseDomainForm(t *testing.T) {
	// Create a fake request with form values
	req, _ := http.NewRequest(http.MethodPost, "/settings/progressions/domains/new", nil)
	req.Form = map[string][]string{
		"name":        {"  Slab Technique  "},
		"description": {"  Smooth feet, quiet hands  "},
		"color":       {"  #2e7d32  "},
		"icon":        {"  foot  "},
		"sort_order":  {"3"},
	}

	fv := parseDomainForm(req)

	if fv.Name != "Slab Technique" {
		t.Errorf("Name = %q, want trimmed", fv.Name)
	}
	if fv.Description != "Smooth feet, quiet hands" {
		t.Errorf("Description = %q, want trimmed", fv.Description)
	}
	if fv.Color != "#2e7d32" {
		t.Errorf("Color = %q, want trimmed", fv.Color)
	}
	if fv.Icon != "foot" {
		t.Errorf("Icon = %q, want trimmed", fv.Icon)
	}
	if fv.SortOrder != "3" {
		t.Errorf("SortOrder = %q, want 3", fv.SortOrder)
	}
}

// ── parseBadgeForm ─────────────────────────────────────────

func TestParseBadgeForm(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "/settings/progressions/badges/new", nil)
	req.Form = map[string][]string{
		"name":        {"  Slab Master Badge  "},
		"description": {"  Earned by completing the Slab quest  "},
		"icon":        {"  award  "},
		"color":       {"  #FFD700  "},
	}

	fv := parseBadgeForm(req)

	if fv.Name != "Slab Master Badge" {
		t.Errorf("Name = %q, want trimmed", fv.Name)
	}
	if fv.Description != "Earned by completing the Slab quest" {
		t.Errorf("Description = %q, want trimmed", fv.Description)
	}
	if fv.Icon != "award" {
		t.Errorf("Icon = %q, want trimmed", fv.Icon)
	}
	if fv.Color != "#FFD700" {
		t.Errorf("Color = %q, want trimmed", fv.Color)
	}
}
