package event

// Each event type has a specific payload struct so listeners can
// type-assert rather than parsing `any` blindly. Payloads carry
// enough context for listeners to act WITHOUT querying the database.
// This means including names, colors, icons — everything needed to
// write to the activity log or notification table in a single INSERT.

// QuestStartedPayload is published when a climber enrolls in a quest.
type QuestStartedPayload struct {
	ClimberQuestID string
	QuestID        string
	QuestName      string
	DomainName     string
	DomainColor    string
}

// QuestCompletedPayload is published when a climber completes a quest,
// either by reaching the target count or by self-completing.
type QuestCompletedPayload struct {
	ClimberQuestID string
	QuestID        string
	QuestName      string
	DomainName     string
	DomainColor    string
	BadgeID        string // empty if quest has no badge
	BadgeName      string
	BadgeIcon      string
	BadgeColor     string
}

// QuestAbandonedPayload is published when a climber abandons a quest.
type QuestAbandonedPayload struct {
	ClimberQuestID string
	QuestID        string
	QuestName      string
}

// ProgressLoggedPayload is published when a climber logs progress
// on an active quest (route climbed, session logged, reflection, etc.).
type ProgressLoggedPayload struct {
	ClimberQuestID string
	QuestID        string
	QuestName      string
	LogType        string  // route_climbed, session_logged, reflection, etc.
	RouteID        string  // empty if not route-specific
	ProgressCount  int     // current count after this log
	TargetCount    *int    // nil if quest has no numeric target
}

// RouteSentPayload is published when a climber logs a send or flash on
// a route. Quest listeners use this to auto-progress active route_count quests.
type RouteSentPayload struct {
	AscentID   string
	RouteID    string
	RouteName  string
	RouteGrade string
	AscentType string // "send" or "flash"
	LocationID string
}

// BadgeEarnedPayload is published when a badge is awarded to a climber,
// typically by the sync AwardBadge listener after quest completion.
type BadgeEarnedPayload struct {
	BadgeID   string
	BadgeName string
	BadgeIcon string
	BadgeColor string
	QuestName string
}
