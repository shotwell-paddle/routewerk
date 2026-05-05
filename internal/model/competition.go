// Competition tracking domain types. See docs/competitions-handoff.md.
//
// Layered like the schema (migrations 000032–000034):
//
//	Competition
//	  ├── CompetitionEvent (1+, weighted, optional per-event scorer override)
//	  │     └── CompetitionProblem (the things climbers attempt)
//	  ├── CompetitionCategory (groups: men/women, masters, etc.)
//	  └── CompetitionRegistration (one per user per comp; mandatory account)
//	        └── CompetitionAttempt (current state, one per (reg, problem))
//	              └── CompetitionAttemptLog (append-only history)
//
// jsonb columns are exposed as json.RawMessage so they round-trip cleanly
// through the JSON API without base64 encoding (which is what `[]byte`
// would do under encoding/json).
package model

import (
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// ── Competition format + status ────────────────────────────

const (
	CompFormatSingle = "single"
	CompFormatSeries = "series"
)

const (
	CompStatusDraft    = "draft"
	CompStatusOpen     = "open"
	CompStatusLive     = "live"
	CompStatusClosed   = "closed"
	CompStatusArchived = "archived"
)

const (
	LeaderboardVisibilityPublic      = "public"
	LeaderboardVisibilityMembers     = "members"
	LeaderboardVisibilityRegistrants = "registrants"
)

// AggregationMethod values that the API accepts in Competition.Aggregation.
// Validation lives in (Aggregation).Validate().
const (
	AggMethodSum            = "sum"
	AggMethodSumDropN       = "sum_drop_n"
	AggMethodWeightedFinals = "weighted_finals"
	AggMethodBestN          = "best_n"
)

// ── Competition ────────────────────────────────────────────

type Competition struct {
	ID         string `json:"id"`
	LocationID string `json:"location_id"`
	Name       string `json:"name"`
	Slug       string `json:"slug"`

	Format         string          `json:"format"`
	Aggregation    json.RawMessage `json:"aggregation"`
	ScoringRule    string          `json:"scoring_rule"`
	ScoringConfig  json.RawMessage `json:"scoring_config"`
	Status         string          `json:"status"`
	LeaderboardVis string          `json:"leaderboard_visibility"`

	StartsAt             time.Time          `json:"starts_at"`
	EndsAt               time.Time          `json:"ends_at"`
	RegistrationOpensAt  pgtype.Timestamptz `json:"registration_opens_at,omitempty"`
	RegistrationClosesAt pgtype.Timestamptz `json:"registration_closes_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Aggregation describes how scores from multiple events combine into a
// comp-level standing. The schema stores this as freeform jsonb; this
// struct is the canonical decoded form for app-side handling.
//
// Method-specific fields (Drop, Weights, FinalsEventID) are only meaningful
// for some methods. (Aggregation).Validate() enforces required combinations.
type Aggregation struct {
	Method        string   `json:"method"`
	Drop          int      `json:"drop,omitempty"`
	Weights       []int    `json:"weights,omitempty"`
	FinalsEventID *string  `json:"finals_event_id,omitempty"`
	BestN         int      `json:"best_n,omitempty"`
	Extra         struct{} `json:"-"` // reserved for future fields without breaking JSON
}

// ── Competition event / category / problem ─────────────────

type CompetitionEvent struct {
	ID            string  `json:"id"`
	CompetitionID string  `json:"competition_id"`
	Name          string  `json:"name"`
	Sequence      int     `json:"sequence"`
	StartsAt      time.Time `json:"starts_at"`
	EndsAt        time.Time `json:"ends_at"`
	Weight        float64 `json:"weight"`

	// Optional per-event scorer override. NULL means "use the comp's
	// scoring_rule + scoring_config".
	ScoringRuleOverride   *string         `json:"scoring_rule_override,omitempty"`
	ScoringConfigOverride json.RawMessage `json:"scoring_config_override,omitempty"`
}

// EffectiveScoringRule returns the scorer name that applies to this event:
// the override if set, otherwise the comp-level default. Pass the parent
// competition's ScoringRule.
func (e CompetitionEvent) EffectiveScoringRule(compRule string) string {
	if e.ScoringRuleOverride != nil && *e.ScoringRuleOverride != "" {
		return *e.ScoringRuleOverride
	}
	return compRule
}

// EffectiveScoringConfig returns the scorer config that applies to this
// event. The override is used as-is when present, even if the override
// rule name is the same as the comp's — overrides are explicit choices.
func (e CompetitionEvent) EffectiveScoringConfig(compConfig json.RawMessage) json.RawMessage {
	if e.ScoringRuleOverride != nil && *e.ScoringRuleOverride != "" {
		return e.ScoringConfigOverride
	}
	return compConfig
}

type CompetitionCategory struct {
	ID            string          `json:"id"`
	CompetitionID string          `json:"competition_id"`
	Name          string          `json:"name"`
	SortOrder     int             `json:"sort_order"`
	Rules         json.RawMessage `json:"rules"`
}

type CompetitionProblem struct {
	ID         string   `json:"id"`
	EventID    string   `json:"event_id"`
	RouteID    *string  `json:"route_id,omitempty"`
	Label      string   `json:"label"`
	Points     *float64 `json:"points,omitempty"`
	ZonePoints *float64 `json:"zone_points,omitempty"`
	Grade      *string  `json:"grade,omitempty"`
	Color      *string  `json:"color,omitempty"`
	SortOrder  int      `json:"sort_order"`
}

// ── Registration ───────────────────────────────────────────

type CompetitionRegistration struct {
	ID             string             `json:"id"`
	CompetitionID  string             `json:"competition_id"`
	CategoryID     string             `json:"category_id"`
	UserID         string             `json:"user_id"`
	DisplayName    string             `json:"display_name"`
	BibNumber      *int               `json:"bib_number,omitempty"`
	WaiverSignedAt pgtype.Timestamptz `json:"waiver_signed_at,omitempty"`
	PaidAt         pgtype.Timestamptz `json:"paid_at,omitempty"`
	WithdrawnAt    pgtype.Timestamptz `json:"withdrawn_at,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
}

// IsActive reports whether the registration is still in-comp (not withdrawn).
func (r CompetitionRegistration) IsActive() bool {
	return !r.WithdrawnAt.Valid
}

// ── Attempt + log ──────────────────────────────────────────

type CompetitionAttempt struct {
	ID             string             `json:"id"`
	RegistrationID string             `json:"registration_id"`
	ProblemID      string             `json:"problem_id"`
	Attempts       int                `json:"attempts"`
	ZoneAttempts   *int               `json:"zone_attempts,omitempty"`
	ZoneReached    bool               `json:"zone_reached"`
	TopReached     bool               `json:"top_reached"`
	Notes          *string            `json:"notes,omitempty"`
	LoggedAt       time.Time          `json:"logged_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
	VerifiedBy     *string            `json:"verified_by,omitempty"`
	VerifiedAt     pgtype.Timestamptz `json:"verified_at,omitempty"`
}

// IsVerified reports whether a setter+ has signed off on this attempt.
func (a CompetitionAttempt) IsVerified() bool {
	return a.VerifiedAt.Valid
}

// Action types accepted by the unified action endpoint and recorded in
// competition_attempt_log.action.
const (
	CompActionIncrement = "increment"
	CompActionZone      = "zone"
	CompActionTop       = "top"
	CompActionUndo      = "undo"
	CompActionReset     = "reset"
	CompActionVerify    = "verify"
	CompActionOverride  = "override"
)

// CompetitionAttemptLog is one append-only entry per state-changing action
// on a CompetitionAttempt. `before` and `after` snapshot the attempt state
// for auditing and undo. `idempotency_key` dedupes accidental retries from
// the climber's phone (action queue + intermittent connectivity).
type CompetitionAttemptLog struct {
	ID             int64           `json:"id"`
	AttemptID      string          `json:"attempt_id"`
	ActorUserID    *string         `json:"actor_user_id,omitempty"`
	Action         string          `json:"action"`
	Before         json.RawMessage `json:"before,omitempty"`
	After          json.RawMessage `json:"after,omitempty"`
	IdempotencyKey *string         `json:"idempotency_key,omitempty"`
	At             time.Time       `json:"at"`
}
