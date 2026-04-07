package webhandler

import (
	"testing"

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
