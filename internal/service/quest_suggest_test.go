package service

import (
	"testing"

	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

func makeQuestItem(id, domainID, skillLevel string, activeCount, completedCount int) repository.QuestListItem {
	return repository.QuestListItem{
		Quest: model.Quest{
			ID:         id,
			DomainID:   domainID,
			SkillLevel: skillLevel,
			IsActive:   true,
		},
		ActiveCount:    activeCount,
		CompletedCount: completedCount,
	}
}

func TestScoreQuest_NewUserBeginnerBoost(t *testing.T) {
	// Brand new user (0 completions) + beginner quest = big boost
	q := makeQuestItem("q1", "domain-slab", "beginner", 0, 0)
	domainCompleted := map[string]int{}

	score, reason := scoreQuest(q, domainCompleted, 0, 0)

	// Should get: 20 (no completions anywhere) + 0 (no social) + 25 (beginner boost)
	if score != 45 {
		t.Errorf("new user beginner score = %v, want 45", score)
	}
	if reason != "great for getting started" {
		t.Errorf("reason = %q, want %q", reason, "great for getting started")
	}
}

func TestScoreQuest_NewUserAdvancedNoBoost(t *testing.T) {
	// Brand new user + advanced quest = no beginner or advanced boost
	q := makeQuestItem("q1", "domain-lead", "advanced", 0, 0)
	domainCompleted := map[string]int{}

	score, reason := scoreQuest(q, domainCompleted, 0, 0)

	// Should get: 20 (no completions) + 0 (no social) + 0 (no boost)
	if score != 20 {
		t.Errorf("new user advanced score = %v, want 20", score)
	}
	if reason != "recommended for you" {
		t.Errorf("reason = %q, want %q", reason, "recommended for you")
	}
}

func TestScoreQuest_ExperiencedUserAdvancedBoost(t *testing.T) {
	// Experienced user (5+ completions) + advanced quest in new domain
	q := makeQuestItem("q1", "domain-lead", "advanced", 0, 0)
	domainCompleted := map[string]int{
		"domain-slab": 5,
		"domain-lead": 0,
	}

	score, reason := scoreQuest(q, domainCompleted, 5, 5)

	// Domain gap: (1 - 0/5) * 40 = 40
	// Social: 0
	// Advanced boost: 15
	// Total: 55
	if score != 55 {
		t.Errorf("experienced user advanced score = %v, want 55", score)
	}
	if reason != "explore a new domain" {
		t.Errorf("reason = %q, want %q", reason, "explore a new domain")
	}
}

func TestScoreQuest_DomainGapScoring(t *testing.T) {
	tests := []struct {
		name            string
		domainID        string
		domainCompleted map[string]int
		maxCompleted    int
		totalCompleted  int
		wantGapScore    float64
	}{
		{
			name:            "untouched domain gets full gap score",
			domainID:        "domain-lead",
			domainCompleted: map[string]int{"domain-slab": 5, "domain-lead": 0},
			maxCompleted:    5,
			totalCompleted:  5,
			wantGapScore:    40, // (1 - 0/5) * 40
		},
		{
			name:            "most completed domain gets zero gap score",
			domainID:        "domain-slab",
			domainCompleted: map[string]int{"domain-slab": 5, "domain-lead": 0},
			maxCompleted:    5,
			totalCompleted:  5,
			wantGapScore:    0, // (1 - 5/5) * 40
		},
		{
			name:            "half completed gets half gap score",
			domainID:        "domain-overhang",
			domainCompleted: map[string]int{"domain-slab": 4, "domain-overhang": 2},
			maxCompleted:    4,
			totalCompleted:  6,
			wantGapScore:    20, // (1 - 2/4) * 40
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := makeQuestItem("q1", tt.domainID, "intermediate", 0, 0)
			score, _ := scoreQuest(q, tt.domainCompleted, tt.maxCompleted, tt.totalCompleted)

			// Score includes gap + social (0 here) + skill boost (none for intermediate with <5 or >=5)
			// For experienced users with intermediate, only gap matters
			if score != tt.wantGapScore {
				t.Errorf("gap score = %v, want %v", score, tt.wantGapScore)
			}
		})
	}
}

func TestScoreQuest_SocialProof(t *testing.T) {
	tests := []struct {
		name           string
		activeCount    int
		completedCount int
		wantSocial     float64
	}{
		{"no activity", 0, 0, 0},
		{"10 total", 5, 5, 1},
		{"50 total", 20, 30, 5},
		{"200 total caps at 20", 100, 100, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := makeQuestItem("q1", "domain-slab", "intermediate", tt.activeCount, tt.completedCount)
			// Use max domain completed > 0 so we get deterministic gap score
			domainCompleted := map[string]int{"domain-slab": 3}
			score, _ := scoreQuest(q, domainCompleted, 3, 3)

			// Gap score: (1 - 3/3) * 40 = 0
			// So score == social score only
			if score != tt.wantSocial {
				t.Errorf("social score = %v, want %v", score, tt.wantSocial)
			}
		})
	}
}

func TestScoreQuest_PopularReason(t *testing.T) {
	// Quest with >5 active and no gap reason → "popular with other climbers"
	q := makeQuestItem("q1", "domain-slab", "intermediate", 10, 5)
	domainCompleted := map[string]int{"domain-slab": 3}

	_, reason := scoreQuest(q, domainCompleted, 3, 3)

	if reason != "popular with other climbers" {
		t.Errorf("reason = %q, want %q", reason, "popular with other climbers")
	}
}

func TestScoreQuest_EarlyUserBeginnerBoost(t *testing.T) {
	// User with 1-2 completions still gets small beginner boost
	q := makeQuestItem("q1", "domain-slab", "beginner", 0, 0)
	domainCompleted := map[string]int{"domain-slab": 2}

	score, _ := scoreQuest(q, domainCompleted, 2, 2)

	// Gap: (1 - 2/2) * 40 = 0
	// Social: 0
	// Beginner boost (totalCompleted < 3): 10
	if score != 10 {
		t.Errorf("early user beginner score = %v, want 10", score)
	}
}

func TestScoreQuest_DefaultReason(t *testing.T) {
	// Intermediate quest, no gap, no social, no boost → default reason
	q := makeQuestItem("q1", "domain-slab", "intermediate", 0, 0)
	domainCompleted := map[string]int{"domain-slab": 3}

	_, reason := scoreQuest(q, domainCompleted, 3, 3)

	if reason != "recommended for you" {
		t.Errorf("reason = %q, want %q", reason, "recommended for you")
	}
}

func TestScoreQuest_GapReasonPriority(t *testing.T) {
	// High gap score → "explore a new domain" takes priority over social
	q := makeQuestItem("q1", "domain-lead", "intermediate", 10, 10)
	domainCompleted := map[string]int{"domain-slab": 5, "domain-lead": 0}

	_, reason := scoreQuest(q, domainCompleted, 5, 5)

	// Gap score > 0.7, so gap reason wins even though social is high
	if reason != "explore a new domain" {
		t.Errorf("reason = %q, want %q", reason, "explore a new domain")
	}
}
