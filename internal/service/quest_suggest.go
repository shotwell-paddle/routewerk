package service

import (
	"context"
	"sort"

	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// QuestSuggestion wraps a quest with a relevance score and reason.
type QuestSuggestion struct {
	Quest  repository.QuestListItem `json:"quest"`
	Score  float64                  `json:"score"`
	Reason string                   `json:"reason"`
}

// SuggestQuests returns up to `limit` recommended quests for a climber at a
// location. The algorithm favors:
//  1. Domains where the climber has completed the fewest quests (fill gaps)
//  2. Quests that are popular (high active + completed count = social proof)
//  3. Beginner-friendly quests if the climber has no completions yet
//  4. Quests the climber hasn't already started
func (s *QuestService) SuggestQuests(ctx context.Context, userID, locationID string, limit int) ([]QuestSuggestion, error) {
	if limit <= 0 {
		limit = 5
	}

	// Get available quests
	available, err := s.quests.ListAvailable(ctx, locationID)
	if err != nil {
		return nil, err
	}
	if len(available) == 0 {
		return nil, nil
	}

	// Get user's current and past quests to exclude active ones
	activeQuests, err := s.quests.ListUserQuests(ctx, userID, "active")
	if err != nil {
		return nil, err
	}
	activeSet := make(map[string]bool, len(activeQuests))
	for _, cq := range activeQuests {
		activeSet[cq.QuestID] = true
	}

	// Get domain progress for gap scoring
	progress, err := s.quests.UserDomainProgress(ctx, userID, locationID)
	if err != nil {
		return nil, err
	}
	domainCompleted := make(map[string]int, len(progress))
	totalCompleted := 0
	for _, dp := range progress {
		domainCompleted[dp.DomainID] = dp.Completed
		totalCompleted += dp.Completed
	}

	// Find max completions across any domain (for normalization)
	maxDomainCompleted := 0
	for _, c := range domainCompleted {
		if c > maxDomainCompleted {
			maxDomainCompleted = c
		}
	}

	// Score each available quest
	var suggestions []QuestSuggestion
	for _, q := range available {
		if activeSet[q.ID] {
			continue // skip already-enrolled
		}

		score, reason := scoreQuest(q, domainCompleted, maxDomainCompleted, totalCompleted)

		suggestions = append(suggestions, QuestSuggestion{
			Quest:  q,
			Score:  score,
			Reason: reason,
		})
	}

	// Sort by score descending
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Score > suggestions[j].Score
	})

	// Cap at limit
	if len(suggestions) > limit {
		suggestions = suggestions[:limit]
	}

	return suggestions, nil
}

// scoreQuest is a pure function that scores a single quest based on user context.
// Extracted from SuggestQuests so it can be tested without a database.
func scoreQuest(q repository.QuestListItem, domainCompleted map[string]int, maxDomainCompleted, totalCompleted int) (float64, string) {
	score := 0.0
	reason := ""

	// 1. Domain gap score: favor domains with fewer completions
	domainCount := domainCompleted[q.DomainID]
	if maxDomainCompleted > 0 {
		// Inverse: 0 completions → full score, max completions → 0
		gapScore := 1.0 - (float64(domainCount) / float64(maxDomainCompleted))
		score += gapScore * 40 // up to 40 points
		if gapScore > 0.7 {
			reason = "explore a new domain"
		}
	} else {
		// No completions anywhere — all domains equal
		score += 20
	}

	// 2. Social proof: popular quests are more engaging
	socialScore := float64(q.ActiveCount+q.CompletedCount) / 10.0
	if socialScore > 20 {
		socialScore = 20
	}
	score += socialScore // up to 20 points
	if q.ActiveCount > 5 && reason == "" {
		reason = "popular with other climbers"
	}

	// 3. Beginner boost: if user is new, favor beginner quests
	if totalCompleted == 0 && q.SkillLevel == "beginner" {
		score += 25
		reason = "great for getting started"
	} else if totalCompleted < 3 && q.SkillLevel == "beginner" {
		score += 10
	}

	// 4. Intermediate/advanced boost for experienced climbers
	if totalCompleted >= 5 && q.SkillLevel == "advanced" {
		score += 15
		if reason == "" {
			reason = "ready for a challenge"
		}
	}

	if reason == "" {
		reason = "recommended for you"
	}

	return score, reason
}
