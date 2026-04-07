package model

import "time"

// ============================================================
// Quest Domains
// ============================================================

type QuestDomain struct {
	ID         string    `json:"id"`
	LocationID string    `json:"location_id"`
	Name       string    `json:"name"`
	Description *string  `json:"description,omitempty"`
	Color      *string   `json:"color,omitempty"`
	Icon       *string   `json:"icon,omitempty"`
	SortOrder  int       `json:"sort_order"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ============================================================
// Badges
// ============================================================

type Badge struct {
	ID          string    `json:"id"`
	LocationID  string    `json:"location_id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	Icon        string    `json:"icon"`
	Color       string    `json:"color"`
	CreatedAt   time.Time `json:"created_at"`
}

// ============================================================
// Quests
// ============================================================

type Quest struct {
	ID                    string     `json:"id"`
	LocationID            string     `json:"location_id"`
	DomainID              string     `json:"domain_id"`
	BadgeID               *string    `json:"badge_id,omitempty"`
	Name                  string     `json:"name"`
	Description           string     `json:"description"`
	QuestType             string     `json:"quest_type"`
	CompletionCriteria    string     `json:"completion_criteria"`
	TargetCount           *int       `json:"target_count,omitempty"`
	SuggestedDurationDays *int       `json:"suggested_duration_days,omitempty"`
	AvailableFrom         *time.Time `json:"available_from,omitempty"`
	AvailableUntil        *time.Time `json:"available_until,omitempty"`
	SkillLevel            string     `json:"skill_level"`
	RequiresCertification *string    `json:"requires_certification,omitempty"`
	RouteTagFilter        []string   `json:"route_tag_filter,omitempty"`
	IsActive              bool       `json:"is_active"`
	SortOrder             int        `json:"sort_order"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`

	// Joined fields — populated by some queries, not stored in quests table.
	Domain *QuestDomain `json:"domain,omitempty"`
	Badge  *Badge       `json:"badge,omitempty"`
}

// ============================================================
// Climber Quests (enrollment + progress)
// ============================================================

type ClimberQuest struct {
	ID            string     `json:"id"`
	UserID        string     `json:"user_id"`
	QuestID       string     `json:"quest_id"`
	Status        string     `json:"status"`
	ProgressCount int        `json:"progress_count"`
	StartedAt     time.Time  `json:"started_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`

	// Joined fields
	Quest *Quest `json:"quest,omitempty"`
}

// ============================================================
// Quest Logs (progress entries)
// ============================================================

type QuestLog struct {
	ID             string     `json:"id"`
	ClimberQuestID string     `json:"climber_quest_id"`
	LogType        string     `json:"log_type"`
	RouteID        *string    `json:"route_id,omitempty"`
	Notes          *string    `json:"notes,omitempty"`
	Rating         *int       `json:"rating,omitempty"`
	LoggedAt       time.Time  `json:"logged_at"`
}

// ============================================================
// Climber Badges (awarded)
// ============================================================

type ClimberBadge struct {
	ID       string    `json:"id"`
	UserID   string    `json:"user_id"`
	BadgeID  string    `json:"badge_id"`
	EarnedAt time.Time `json:"earned_at"`

	// Joined fields
	Badge *Badge `json:"badge,omitempty"`
}

// ============================================================
// Route Skill Tags
// ============================================================

type RouteSkillTag struct {
	ID      string `json:"id"`
	RouteID string `json:"route_id"`
	Tag     string `json:"tag"`
}
