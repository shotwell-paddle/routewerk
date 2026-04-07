package model

import "time"

// ActivityLogEntry represents a single event in the activity feed.
// Metadata is denormalized JSONB — carries everything needed to
// render a feed item without joining back to source tables.
type ActivityLogEntry struct {
	ID           string         `json:"id"`
	LocationID   string         `json:"location_id"`
	UserID       string         `json:"user_id"`
	ActivityType string         `json:"activity_type"`
	EntityType   string         `json:"entity_type"`
	EntityID     string         `json:"entity_id"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"created_at"`

	// Joined fields — populated by feed queries that join to users.
	UserDisplayName string  `json:"user_display_name,omitempty"`
	UserAvatarURL   *string `json:"user_avatar_url,omitempty"`
}
