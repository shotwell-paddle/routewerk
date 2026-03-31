package webhandler

import (
	"net/url"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// TemplateData is the shared context for every page render.
type TemplateData struct {
	ActiveNav    string
	IsSetter     bool
	IsHeadSetter bool
	IsOrgAdmin   bool
	IsAppAdmin   bool
	UserName     string
	UserInitial string
	UserRole    string
	Location    *model.Location
	CSRFToken   string

	// Location switcher
	UserLocations []repository.UserLocationItem
	HasMultipleLocations bool

	// View-as-role switcher (for admins/managers/head setters)
	RealRole      string // the user's actual role before any override
	ViewAsRole    string // the currently active role override (empty = no override)
	CanViewAs     bool   // true if user has a role higher than setter
	ViewAsOptions []ViewAsOption
}

// ViewAsOption represents a role the user can "view as" in the switcher.
type ViewAsOption struct {
	Value   string // role key (e.g. "setter", "climber")
	Label   string // display name
	Active  bool   // currently selected
}

// PageData embeds TemplateData so templates can access layout fields at top level.
type PageData struct {
	TemplateData
	User *model.User // current user (for profile pages)

	// Setter dashboard
	Stats          *DashboardStats
	Walls          []WallWithRoutes
	RecentActivity []ActivityItem

	// Climber route browser
	Routes      []RouteView
	TotalRoutes int
	RouteType   string
	GradeGroups []GradeGroup
	HasMore     bool
	NextOffset  int

	// Route detail
	Route         *RouteView
	WallName      string
	SetterName    string
	RecentAscents []AscentView
	Consensus     *ConsensusData

	// Route form (create / edit)
	FormWalls   []model.Wall
	FormTags    []model.Tag
	FormValues  RouteFormValues
	FormError   string
	FormSuccess string
	HoldColors  []HoldColor
	VScaleGrades   []string
	YDSGrades      []string
	CircuitColors  []string

	// Manage routes
	StatusFilter string
	WallFilter   string
	GradeFilter  string
	DateFrom     string
	DateTo       string
	TypeFilter   string

	// Manage row partial (for HTMX swap of a single row)
	RowRoute *RouteView

	// Wall management
	WallList       []WallView
	Wall           *WallView
	WallFormValues WallFormValues
	WallFormError  string

	// Climber interactions
	UserRating    int  // the logged-in user's star rating (1-5, 0 = not yet rated)
	CanFlash      bool // true if user has no prior ascents on this route (flash = first try)
	HasCompleted  bool // true if user already has a send/flash on this route

	// Climber profile
	ClimberStats     *repository.UserClimbingStats
	TickList         []TickListItem
	TickListTotal    int
	GradePyramid     []PyramidBar
	TickFilterType   string // "boulder" or "route" — current filter
	TickFilterAscent string // "send", "flash", etc. — current filter
	TickSort         string // "date" or "grade" — current sort
	UserSettings     model.UserSettings

	// Gym search / join
	GymResults  []repository.LocationSearchResult
	GymQuery    string
	JoinSuccess string
	JoinError   string

	// Setting sessions
	Sessions           []repository.SessionListItem
	SessionFilter      string
	Session            *model.SettingSession
	SessionAssignments []repository.SessionAssignmentDetail
	SessionFormValues  SessionFormValues
	SessionFormError   string
	Setters            []repository.SetterAtLocation
	StripTargets       []repository.StripTargetDetail
	WallsWithRoutes    []repository.WallWithActiveRoutes
	ChecklistItems     []repository.ChecklistItemWithUser
	SessionRoutes      []repository.SessionRouteDetail

	// Grading context
	BoulderMethod        string // "v_scale", "circuit", or "both"
	ShowGradesOnCircuit  bool   // whether climbers see V-grade on circuit boulders
	RouteFields          RouteFieldsData

	// Community tags (user-submitted)
	CommunityTags []repository.AggregatedTag

	// Route photos
	Photos         []repository.PhotoWithUploader
	PhotosEnabled  bool // true if S3 storage is configured
	CanUploadPhoto bool // true if user can upload photos for this route
	PhotosUploaded int  // count of session routes that already have a photo
	PhotosPercent  int  // percentage of session routes with photos (0-100)

	// Team management
	TeamMembers    []repository.LocationMember
	TeamTotalCount int
	TeamPage       int
	TeamTotalPages int
	TeamQuery      string
	TeamRoleFilter string

	// Settings pages
	GymSettings     *model.LocationSettings
	OrgPermissions  *model.OrgPermissions
	OrgSettingsData *model.OrgSettings
	OrgName         string
	OrgLocations    []model.Location
	EditLocation    *model.Location
	GymForm         GymFormValues
	IsManager       bool
	SettingsSuccess bool

	// Error page
	ErrorCode    int
	ErrorTitle   string
	ErrorMessage string

	// Admin — health dashboard
	HealthChecks []HealthCheckResult
	OverallHealth string // "healthy", "degraded", "down"

	// Admin — metrics dashboard
	MetricsData *middleware.MetricsSnapshot
	Uptime      string // human-readable uptime
	JobStats    map[string]int64
}

// HealthCheckResult represents one dependency check.
type HealthCheckResult struct {
	Name    string // "Database", "Storage", "Job Queue", etc.
	Status  string // "ok", "error", "not_configured"
	Details string // human-readable detail
	Icon    string // SVG path data for the icon
}

// ArchiveFilterURL builds a clean /archive URL preserving the current date,
// type, and wall filters while setting the grade filter to the given value.
// This avoids inline URL construction in templates which html/template's
// context-aware escaping rejects as ambiguous.
func (pd *PageData) ArchiveFilterURL(grade string) string {
	params := url.Values{}
	if grade != "" {
		params.Set("grade", grade)
	}
	if pd.TypeFilter != "" {
		params.Set("type", pd.TypeFilter)
	}
	if pd.WallFilter != "" {
		params.Set("wall", pd.WallFilter)
	}
	if pd.DateFrom != "" {
		params.Set("from", pd.DateFrom)
	}
	if pd.DateTo != "" {
		params.Set("to", pd.DateTo)
	}
	if len(params) == 0 {
		return "/archive"
	}
	return "/archive?" + params.Encode()
}

type ConsensusData struct {
	EasyCount  int
	RightCount int
	HardCount  int
	TotalVotes int
	EasyPct    int
	RightPct   int
	HardPct    int
}

type DashboardStats struct {
	ActiveRoutes int
	ActiveDelta  int
	TotalSends   int
	AvgRating    float64
	DueForStrip  int
}

type WallWithRoutes struct {
	model.Wall
	RouteCount int
	Routes     []RouteView
}

type RouteView struct {
	model.Route
	WallName         string
	SetterName       string
	HideCircuitGrade bool // when true, CircuitVGrade() returns "" (hides V-grade from climbers)
}

// DisplayGrade returns the circuit color name for circuit routes, or the
// grade string for graded routes. This is the primary identifier shown on
// route cards and detail pages.
func (rv RouteView) DisplayGrade() string {
	if rv.IsCircuit() && rv.CircuitColor != nil && *rv.CircuitColor != "" {
		return titleCase(*rv.CircuitColor)
	}
	return rv.Grade
}

// IsCircuit returns true when this route uses circuit color grading.
func (rv RouteView) IsCircuit() bool {
	return rv.GradingSystem == "circuit"
}

// CircuitVGrade returns the V-grade for a circuit boulder, or "" if none
// was assigned. A circuit route stores the color name in CircuitColor and
// the optional V-grade in Grade. When Grade equals CircuitColor there is
// no distinct V-grade. Respects HideCircuitGrade for climber visibility.
func (rv RouteView) CircuitVGrade() string {
	if rv.HideCircuitGrade {
		return ""
	}
	if !rv.IsCircuit() || rv.CircuitColor == nil {
		return ""
	}
	if rv.Grade != "" && rv.Grade != *rv.CircuitColor {
		return rv.Grade
	}
	return ""
}

// SafeColor returns a validated hex color, safe for CSS injection.
func (rv RouteView) SafeColor() string {
	return sanitizeColor(rv.Color)
}

type ActivityItem struct {
	UserName     string
	UserInitial  string
	ActionText   string // plain string, auto-escaped by html/template
	Time         time.Time
	RoutePreview *RoutePreviewData
}

type RoutePreviewData struct {
	Color string
	Grade string
	Name  string
}

// SafeColor returns a validated hex color for the route preview.
func (rp RoutePreviewData) SafeColor() string {
	return sanitizeColor(rp.Color)
}

type GradeGroup struct {
	Label   string
	Value   string
	Count   int
	Color   string // hex color for circuit filter chips (empty for graded)
	IsColor bool   // true = circuit color filter, false = grade range filter
}

// SafeColor returns a validated hex color for the filter chip.
func (gg GradeGroup) SafeColor() string {
	return sanitizeColor(gg.Color)
}

type AscentView struct {
	model.Ascent
	UserName    string
	UserInitial string
	AscentType  string
	Notes       *string
}

// WallView extends a wall with aggregate route counts for display.
type WallView struct {
	model.Wall
	ActiveRoutes   int
	FlaggedRoutes  int
	ArchivedRoutes int
	TotalRoutes    int
}

// WallFormValues holds form state for wall create/edit.
type WallFormValues struct {
	Name         string
	WallType     string
	Angle        string
	HeightMeters string
	NumAnchors   string
	SurfaceType  string
	SortOrder    string
}

// SessionFormValues holds form state for session create/edit.
type SessionFormValues struct {
	ScheduledDate string
	Notes         string
}

// TickListItem represents one ascent in the climber's tick list.
type TickListItem struct {
	model.Ascent
	RouteGrade string
	RouteName  string
	RouteColor string
	RouteType  string
	WallID     string
	TypeLabel  string // human-friendly ("sent", "flashed", etc.)
}

// SafeColor returns a validated hex color for the tick list entry.
func (t TickListItem) SafeColor() string {
	return sanitizeColor(t.RouteColor)
}

// PyramidBar represents one bar in the grade pyramid chart.
type PyramidBar struct {
	Grade    string
	System   string
	Count    int
	WidthPct int // percentage width (relative to max count)
}

// RouteFormValues holds form state for create/edit — survives validation errors.
type RouteFormValues struct {
	WallID             string
	SetterID           string
	Slug               string
	RouteType          string
	GradingSystem      string
	Grade              string
	CircuitColor       string
	Name               string
	Color              string
	Description        string
	DateSet            string
	ProjectedStripDate string
	TagIDs             map[string]bool // for checkbox state
}

// HasTag returns true if the given tag ID is selected.
func (v RouteFormValues) HasTag(id string) bool {
	return v.TagIDs[id]
}

// GymFormValues holds form state for creating/editing a gym location.
type GymFormValues struct {
	Name         string
	Slug         string
	Address      string
	Timezone     string
	WebsiteURL   string
	Phone        string
	DayPassInfo  string
	CustomDomain string
}

// HoldColor represents a color swatch in the route form.
type HoldColor struct {
	Name string
	Hex  string
}

// RouteFieldsData provides the data for the server-rendered route form fields partial.
// Built from wall type + location grading settings + selected route type.
type RouteFieldsData struct {
	SessionID        string
	WallID           string
	WallType         string // "boulder" or "route" — empty means no wall selected
	TypeOptions      []SelectOption
	GradeLabel       string
	GradePlaceholder string
	GradeOptions     []string
	ShowVGrade       bool
	VScaleGrades     []string
	HoldColors       []HoldColor
	Setters          []repository.SetterAtLocation
}

// SelectOption is a generic <option> for template rendering.
type SelectOption struct {
	Value    string
	Label    string
	Selected bool
}
