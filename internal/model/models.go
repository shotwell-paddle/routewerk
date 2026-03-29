package model

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// ============================================================
// Core entities
// ============================================================

type Organization struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	Slug      string             `json:"slug"`
	LogoURL   *string            `json:"logo_url,omitempty"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
	DeletedAt pgtype.Timestamptz `json:"-"`
}

type Location struct {
	ID                 string             `json:"id"`
	OrgID              string             `json:"org_id"`
	Name               string             `json:"name"`
	Slug               string             `json:"slug"`
	Address            *string            `json:"address,omitempty"`
	Timezone           string             `json:"timezone"`
	WebsiteURL         *string            `json:"website_url,omitempty"`
	Phone              *string            `json:"phone,omitempty"`
	HoursJSON          []byte             `json:"hours,omitempty"`
	DayPassInfo        *string            `json:"day_pass_info,omitempty"`
	WaiverURL          *string            `json:"waiver_url,omitempty"`
	AllowSharedSetters bool               `json:"allow_shared_setters"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
	DeletedAt          pgtype.Timestamptz `json:"-"`
}

type User struct {
	ID           string             `json:"id"`
	Email        string             `json:"email"`
	PasswordHash string             `json:"-"`
	DisplayName  string             `json:"display_name"`
	AvatarURL    *string            `json:"avatar_url,omitempty"`
	Bio          *string            `json:"bio,omitempty"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
	DeletedAt    pgtype.Timestamptz `json:"-"`
}

type UserMembership struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	OrgID       string    `json:"org_id"`
	LocationID  *string   `json:"location_id,omitempty"`
	Role        string    `json:"role"`
	Specialties []string  `json:"specialties,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ============================================================
// Walls & Routes
// ============================================================

type Wall struct {
	ID           string             `json:"id"`
	LocationID   string             `json:"location_id"`
	Name         string             `json:"name"`
	WallType     string             `json:"wall_type"`
	Angle        *string            `json:"angle,omitempty"`
	HeightMeters *float64           `json:"height_meters,omitempty"`
	NumAnchors   *int               `json:"num_anchors,omitempty"`
	SurfaceType  *string            `json:"surface_type,omitempty"`
	SortOrder    int                `json:"sort_order"`
	MapX         *float64           `json:"map_x,omitempty"`
	MapY         *float64           `json:"map_y,omitempty"`
	MapWidth     *float64           `json:"map_width,omitempty"`
	MapHeight    *float64           `json:"map_height,omitempty"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
	DeletedAt    pgtype.Timestamptz `json:"-"`
}

type Route struct {
	ID                 string             `json:"id"`
	LocationID         string             `json:"location_id"`
	WallID             string             `json:"wall_id"`
	SetterID           *string            `json:"setter_id,omitempty"`
	RouteType          string             `json:"route_type"`
	Status             string             `json:"status"`
	GradingSystem      string             `json:"grading_system"`
	Grade              string             `json:"grade"`
	GradeLow           *string            `json:"grade_low,omitempty"`
	GradeHigh          *string            `json:"grade_high,omitempty"`
	CircuitColor       *string            `json:"circuit_color,omitempty"`
	Name               *string            `json:"name,omitempty"`
	Color              string             `json:"color"`
	Description        *string            `json:"description,omitempty"`
	PhotoURL           *string            `json:"photo_url,omitempty"`
	DateSet            time.Time          `json:"date_set"`
	ProjectedStripDate *time.Time         `json:"projected_strip_date,omitempty"`
	DateStripped       *time.Time         `json:"date_stripped,omitempty"`
	AvgRating          float64            `json:"avg_rating"`
	AscentCount        int                `json:"ascent_count"`
	AttemptCount       int                `json:"attempt_count"`
	Tags               []Tag              `json:"tags,omitempty"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
	DeletedAt          pgtype.Timestamptz `json:"-"`
}

type Tag struct {
	ID       string  `json:"id"`
	OrgID    string  `json:"org_id"`
	Category string  `json:"category"`
	Name     string  `json:"name"`
	Color    *string `json:"color,omitempty"`
}

// ============================================================
// Setting Sessions & Labor
// ============================================================

type SettingSession struct {
	ID            string                     `json:"id"`
	LocationID    string                     `json:"location_id"`
	ScheduledDate time.Time                  `json:"scheduled_date"`
	Notes         *string                    `json:"notes,omitempty"`
	CreatedBy     string                     `json:"created_by"`
	Assignments   []SettingSessionAssignment `json:"assignments,omitempty"`
	CreatedAt     time.Time                  `json:"created_at"`
	UpdatedAt     time.Time                  `json:"updated_at"`
}

type SettingSessionAssignment struct {
	ID           string   `json:"id"`
	SessionID    string   `json:"session_id"`
	SetterID     string   `json:"setter_id"`
	WallID       *string  `json:"wall_id,omitempty"`
	TargetGrades []string `json:"target_grades,omitempty"`
	Notes        *string  `json:"notes,omitempty"`
}

type SetterLaborLog struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	LocationID string    `json:"location_id"`
	SessionID  *string   `json:"session_id,omitempty"`
	Date       time.Time `json:"date"`
	HoursWorked *float64 `json:"hours_worked,omitempty"`
	RoutesSet  int       `json:"routes_set"`
	Notes      *string   `json:"notes,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ============================================================
// Climber
// ============================================================

type Ascent struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	RouteID    string    `json:"route_id"`
	AscentType string    `json:"ascent_type"`
	Attempts   int       `json:"attempts"`
	Notes      *string   `json:"notes,omitempty"`
	ClimbedAt  time.Time `json:"climbed_at"`
	CreatedAt  time.Time `json:"created_at"`
}

type RouteRating struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	RouteID   string    `json:"route_id"`
	Rating    int       `json:"rating"`
	Comment   *string   `json:"comment,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ============================================================
// Community
// ============================================================

type Follow struct {
	FollowerID  string    `json:"follower_id"`
	FollowingID string    `json:"following_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type AchievementDefinition struct {
	ID           string  `json:"id"`
	OrgID        *string `json:"org_id,omitempty"`
	Slug         string  `json:"slug"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	IconURL      *string `json:"icon_url,omitempty"`
	CriteriaJSON []byte  `json:"criteria"`
}

type UserAchievement struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id"`
	AchievementID string    `json:"achievement_id"`
	EarnedAt      time.Time `json:"earned_at"`
}

// ============================================================
// Coaching
// ============================================================

type TrainingPlan struct {
	ID          string             `json:"id"`
	CoachID     string             `json:"coach_id"`
	ClimberID   string             `json:"climber_id"`
	LocationID  string             `json:"location_id"`
	Name        string             `json:"name"`
	Description *string            `json:"description,omitempty"`
	Active      bool               `json:"active"`
	Items       []TrainingPlanItem `json:"items,omitempty"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

type TrainingPlanItem struct {
	ID          string     `json:"id"`
	PlanID      string     `json:"plan_id"`
	RouteID     *string    `json:"route_id,omitempty"`
	SortOrder   int        `json:"sort_order"`
	Title       string     `json:"title"`
	Notes       *string    `json:"notes,omitempty"`
	Completed   bool       `json:"completed"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ============================================================
// Partner Matching
// ============================================================

type PartnerProfile struct {
	ID            string   `json:"id"`
	UserID        string   `json:"user_id"`
	LocationID    string   `json:"location_id"`
	LookingFor    []string `json:"looking_for"`
	ClimbingTypes []string `json:"climbing_types"`
	GradeRange    *string  `json:"grade_range,omitempty"`
	Availability  []byte   `json:"availability,omitempty"`
	Bio           *string  `json:"bio,omitempty"`
	Active        bool     `json:"active"`
	UpdatedAt     time.Time `json:"updated_at"`
}
