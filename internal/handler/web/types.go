package webhandler

import (
	"net/url"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

// TemplateData is the shared context for every page render.
type TemplateData struct {
	ActiveNav    string
	IsSetter     bool
	IsHeadSetter bool
	IsOrgAdmin   bool
	IsAppAdmin   bool
	UserName     string
	UserInitial  string
	UserRole     string
	Location     *model.Location
	CSRFToken    string

	// ProgressionsEnabled mirrors Location.ProgressionsEnabled for templates.
	// False when no location is loaded. Gates the climber-facing /quests UI.
	ProgressionsEnabled bool

	// Location switcher
	UserLocations        []repository.UserLocationItem
	HasMultipleLocations bool

	// Notifications — count is loaded asynchronously via
	// /notifications/badge, not rendered inline on the server.

	// View-as-role switcher (for admins/managers/head setters)
	RealRole      string // the user's actual role before any override
	ViewAsRole    string // the currently active role override (empty = no override)
	CanViewAs     bool   // true if user has a role higher than setter
	ViewAsOptions []ViewAsOption
}

// ViewAsOption represents a role the user can "view as" in the switcher.
type ViewAsOption struct {
	Value  string // role key (e.g. "setter", "climber")
	Label  string // display name
	Active bool   // currently selected
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
	FormWalls     []model.Wall
	FormTags      []model.Tag
	FormValues    RouteFormValues
	FormError     string
	FormSuccess   string
	HoldColors    []HoldColor
	VScaleGrades  []string
	YDSGrades     []string
	CircuitColors []string

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
	UserRating   int  // the logged-in user's star rating (1-5, 0 = not yet rated)
	CanFlash     bool // true if user has no prior ascents on this route (flash = first try)
	HasCompleted bool // true if user already has a send/flash on this route

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
	BoulderMethod       string // "v_scale", "circuit", or "both"
	ShowGradesOnCircuit bool   // whether climbers see V-grade on circuit boulders
	RouteFields         RouteFieldsData

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
	PalettePresets  []model.PalettePreset // available palette presets for the gym settings UI

	// Progressions — admin
	QuestDomains     []model.QuestDomain
	QuestDomain      *model.QuestDomain
	DomainFormValues DomainFormValues
	DomainFormError  string
	Quests           []repository.QuestListItem
	QuestDetail      *model.Quest
	QuestFormValues  QuestFormValues
	QuestFormError   string
	Badges           []model.Badge
	BadgeDetail      *model.Badge
	BadgeFormValues  BadgeFormValues
	BadgeFormError   string
	SkillTagCoverage map[string]int

	// Progressions — climber
	DomainFilter     string
	SkillFilter      string
	AvailableQuests  []repository.QuestListItem
	QuestSuggestions []service.QuestSuggestion
	ActiveQuests     []model.ClimberQuest
	CompletedQuests  []model.ClimberQuest
	ClimberBadges    []model.ClimberBadge
	EarnedBadgeIDs   map[string]bool
	ActiveQuestMap   map[string]*model.ClimberQuest // keyed by quest ID
	DomainProgress   []repository.DomainProgress
	QuestLogs        []model.QuestLog
	ClimberQuest     *model.ClimberQuest
	ActivityFeed     []model.ActivityLogEntry
	Notifications    []repository.Notification

	// Error page
	ErrorCode    int
	ErrorTitle   string
	ErrorMessage string

	// Admin — health dashboard
	HealthChecks  []HealthCheckResult
	OverallHealth string // "healthy", "degraded", "down"

	// Admin — metrics dashboard
	MetricsData *middleware.MetricsSnapshot
	Uptime      string // human-readable uptime
	JobStats    map[string]int64

	// Card batches — print-and-cut sheet queue
	CardBatches     []CardBatchListItem
	CardBatchDetail *CardBatchDetailView
	BatchForm       CardBatchFormValues
	BatchFormError  string
}

// CardBatchListItem is a row on the batch-history page.
type CardBatchListItem struct {
	model.CardBatch
	CreatorName string
	CardCount   int
	PageCount   int
}

// CardBatchDetailView describes a single batch plus the routes it contains.
// Used by the "review before print" and "post-create confirmation" pages.
//
// MissingCount is (len(RouteIDs) - len(Routes)) — how many of the batch's
// routes could not be hydrated (deleted, moved to another location). Used
// by the template to explain why the sheet count shrunk.
type CardBatchDetailView struct {
	model.CardBatch
	CreatorName  string
	Routes       []CardBatchRoutePreview
	PageCount    int
	MissingCount int
}

// CardBatchRoutePreview is a thin route row shown in the batch picker and
// review pages. Pulled from repository.RouteRepo.GetByID — same fields as
// RouteView but scoped to what the batch UI needs.
type CardBatchRoutePreview struct {
	ID        string
	Name      string
	Grade     string
	Color     string
	WallName  string
	DateSet   time.Time
	IsCircuit bool
}

// SafeColor validates the hex color before CSS interpolation.
func (p CardBatchRoutePreview) SafeColor() string { return sanitizeColor(p.Color) }

// CardBatchFormValues holds form state for new-batch creation so validation
// errors can round-trip to the template without losing user input.
//
// The same struct powers the edit form; EditBatchID is non-empty only when
// the template should POST to the update endpoint rather than /new.
type CardBatchFormValues struct {
	Theme         string
	CutterProfile string
	RouteIDs      []string // selected route IDs, preserved on form re-render
	// CandidateRoutes is the pool of routes the user can pick from. Typically
	// "routes at this location with no card yet" but the form accepts any
	// valid routes at the location.
	CandidateRoutes []CardBatchRoutePreview
	// EditBatchID, when set, flips the form into edit mode — the template
	// POSTs to /card-batches/{id}/edit and changes its copy to match.
	EditBatchID string
	// ShowAll mirrors ?all=1 on the form URL. Drives the active state on
	// the "Uncarded" / "All active" filter chips and the empty-state copy.
	ShowAll bool
}

// CardBatchRouteGroup clusters candidate routes by wall so the picker can
// show a scannable "print cards for the overhang, then the slab" workflow
// instead of one flat 200-row list.
type CardBatchRouteGroup struct {
	WallName string
	Routes   []CardBatchRoutePreview
}

// IsEdit reports whether the form is being rendered in edit mode. Template
// helper so the card-batch-form.html can branch on form submission target.
func (v CardBatchFormValues) IsEdit() bool { return v.EditBatchID != "" }

// Selected returns a set-like map of the IDs already on the batch so the
// template can render pre-checked checkboxes without a linear scan per row.
// html/template can't do "is X in slice" cleanly, so we materialise a map.
func (v CardBatchFormValues) Selected() map[string]bool {
	set := make(map[string]bool, len(v.RouteIDs))
	for _, id := range v.RouteIDs {
		set[id] = true
	}
	return set
}

// FormAction returns the POST target for this form — /new for create mode,
// /{id}/edit for edit mode. Used in the template's <form action=...>.
func (v CardBatchFormValues) FormAction() string {
	if v.EditBatchID != "" {
		return "/card-batches/" + v.EditBatchID + "/edit"
	}
	return "/card-batches/new"
}

// GroupByWall clusters CandidateRoutes by wall name while preserving the
// slice order (the handler sorts wall → date desc → grade before passing
// them in, so the template just walks the existing order). Used by the
// picker template to render "wall sections" with headers and per-wall
// "select all in wall" affordances.
func (v CardBatchFormValues) GroupByWall() []CardBatchRouteGroup {
	if len(v.CandidateRoutes) == 0 {
		return nil
	}
	groups := make([]CardBatchRouteGroup, 0, 8)
	idx := map[string]int{}
	for _, rt := range v.CandidateRoutes {
		name := rt.WallName
		if name == "" {
			name = "Unassigned"
		}
		i, ok := idx[name]
		if !ok {
			idx[name] = len(groups)
			groups = append(groups, CardBatchRouteGroup{WallName: name, Routes: []CardBatchRoutePreview{rt}})
			continue
		}
		groups[i].Routes = append(groups[i].Routes, rt)
	}
	return groups
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

// ── Progressions Form Values ─────────────────────────────────

// DomainFormValues holds form state for quest domain create/edit.
type DomainFormValues struct {
	Name        string
	Description string
	Color       string
	Icon        string
	SortOrder   string
}

// QuestFormValues holds form state for quest create/edit.
type QuestFormValues struct {
	DomainID              string
	BadgeID               string
	Name                  string
	Description           string
	QuestType             string
	CompletionCriteria    string
	TargetCount           string
	SuggestedDurationDays string
	AvailableFrom         string
	AvailableUntil        string
	SkillLevel            string
	RequiresCertification string
	RouteTagFilter        string // comma-separated
	IsActive              string
	SortOrder             string
}

// BadgeFormValues holds form state for badge create/edit.
type BadgeFormValues struct {
	Name        string
	Description string
	Icon        string
	Color       string
}
