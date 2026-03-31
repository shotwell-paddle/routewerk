package webhandler

import (
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/config"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
	"github.com/shotwell-paddle/routewerk/web"
)

// ── Validation ───────────────────────────────────────────────────

// validHexColor matches #RGB or #RRGGBB (case-insensitive).
var validHexColor = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}){1,2}$`)

// validRouteTypes is the allow-list for the ?type= query param.
var validRouteTypes = map[string]bool{
	"":         true,
	"boulder":  true,
	"sport":    true,
	"top_rope": true,
}

// validUUID matches UUID v4 or similar slug IDs used in the app.
var validRouteID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// sanitizeColor returns the color if it's a valid hex color, or a safe default.
func sanitizeColor(color string) string {
	if validHexColor.MatchString(color) {
		return color
	}
	return "#999999"
}

// ── Template Data Types ──────────────────────────────────────────

// TemplateData is the shared context for every page render.
type TemplateData struct {
	ActiveNav    string
	IsSetter     bool
	IsHeadSetter bool
	IsOrgAdmin   bool
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

// ── Wall View ────────────────────────────────────────────────────

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

// ── Session Form Data ─────────────────────────────────────────

// SessionFormValues holds form state for session create/edit.
type SessionFormValues struct {
	ScheduledDate string
	Notes         string
}

// ── Climber Profile Types ─────────────────────────────────────

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

// Wall types for form dropdown.
var wallTypes = []struct{ Value, Label string }{
	{"boulder", "Boulder"},
	{"route", "Route (Sport / Top Rope)"},
}

// Common wall angles.
var wallAngles = []string{
	"Slab", "Vertical", "Slight overhang", "Overhang", "Steep overhang", "Roof",
}

// Surface types.
var surfaceTypes = []string{
	"Textured plywood", "Smooth plywood", "Concrete", "Brick", "Natural rock",
}

// ── Route Form Data ──────────────────────────────────────────────

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

// buildRouteFields assembles the route form fields based on wall type, boulder method, and selected route type.
func buildRouteFields(sessionID, wallID, wallType, boulderMethod, selectedType string, settings model.LocationSettings, setters []repository.SetterAtLocation) RouteFieldsData {
	rf := RouteFieldsData{
		SessionID:    sessionID,
		WallID:       wallID,
		WallType:     wallType,
		VScaleGrades: vScaleGrades,
		HoldColors:   holdColorsFromSettings(settings),
		Setters:      setters,
	}

	if wallType == "" {
		return rf
	}

	// Build type options based on wall type + boulder method
	if wallType == "boulder" {
		switch boulderMethod {
		case "circuit":
			rf.TypeOptions = []SelectOption{{Value: "boulder_circuit", Label: "Boulder (Circuit)"}}
			if selectedType == "" {
				selectedType = "boulder_circuit"
			}
		case "both":
			rf.TypeOptions = []SelectOption{
				{Value: "boulder", Label: "Boulder"},
				{Value: "boulder_circuit", Label: "Boulder (Circuit)"},
			}
			if selectedType == "" {
				selectedType = "boulder"
			}
		default: // v_scale
			rf.TypeOptions = []SelectOption{{Value: "boulder", Label: "Boulder"}}
			if selectedType == "" {
				selectedType = "boulder"
			}
		}
	} else {
		// Route wall — only rope climbing types
		rf.TypeOptions = []SelectOption{{Value: "route", Label: "Route"}}
		selectedType = "route"
	}

	// Mark selected
	for i := range rf.TypeOptions {
		if rf.TypeOptions[i].Value == selectedType {
			rf.TypeOptions[i].Selected = true
		}
	}

	// Build grade options based on selected type
	switch selectedType {
	case "boulder_circuit":
		rf.GradeLabel = "Circuit Color"
		rf.GradePlaceholder = "color"
		rf.ShowVGrade = true
		// Use circuit colors from settings
		for _, c := range settings.Circuits.Colors {
			rf.GradeOptions = append(rf.GradeOptions, c.Name)
		}
	case "route":
		rf.GradeLabel = "Grade"
		rf.GradePlaceholder = "grade"
		rf.GradeOptions = ydsGrades
	default: // boulder
		rf.GradeLabel = "Grade"
		rf.GradePlaceholder = "grade"
		rf.GradeOptions = vScaleGrades
	}

	return rf
}

// holdColorsFromSettings returns HoldColors from gym settings, falling back to defaults.
func holdColorsFromSettings(settings model.LocationSettings) []HoldColor {
	if len(settings.HoldColors.Colors) > 0 {
		colors := make([]HoldColor, len(settings.HoldColors.Colors))
		for i, c := range settings.HoldColors.Colors {
			colors[i] = HoldColor{Name: c.Name, Hex: c.Hex}
		}
		return colors
	}
	return defaultHoldColors
}

// Standard hold colors used across climbing gyms (fallback).
var defaultHoldColors = []HoldColor{
	{"Red", "#e53935"},
	{"Orange", "#fc5200"},
	{"Yellow", "#f9a825"},
	{"Green", "#2e7d32"},
	{"Blue", "#1565c0"},
	{"Purple", "#7b1fa2"},
	{"Pink", "#e91e8a"},
	{"Black", "#0a0a0a"},
	{"White", "#e0e0e0"},
	{"Teal", "#00897b"},
}

// Grade lists for form dropdowns.
var vScaleGrades = []string{
	"VB", "V0", "V1", "V2", "V3", "V4", "V5", "V6", "V7", "V8", "V9", "V10", "V11", "V12",
}

var ydsGrades = []string{
	"5.5", "5.6", "5.7", "5.8-", "5.8", "5.8+",
	"5.9-", "5.9", "5.9+",
	"5.10-", "5.10", "5.10+",
	"5.11-", "5.11", "5.11+",
	"5.12-", "5.12", "5.12+",
	"5.13-", "5.13", "5.13+",
	"5.14-", "5.14",
}

var circuitColors = []string{
	"red", "orange", "yellow", "green", "blue", "purple", "pink", "white", "black",
}

// ── Handler ──────────────────────────────────────────────────────

// Handler serves the HTMX-powered web frontend.
type Handler struct {
	templates      map[string]*template.Template
	routeRepo      *repository.RouteRepo
	wallRepo       *repository.WallRepo
	locationRepo   *repository.LocationRepo
	userRepo       *repository.UserRepo
	tagRepo        *repository.TagRepo
	ascentRepo     *repository.AscentRepo
	ratingRepo     *repository.RatingRepo
	difficultyRepo *repository.DifficultyRepo
	orgRepo        *repository.OrgRepo
	sessionRepo    *repository.SessionRepo
	analyticsRepo  *repository.AnalyticsRepo
	webSessionRepo *repository.WebSessionRepo
	photoRepo      *repository.RoutePhotoRepo
	settingsRepo   *repository.SettingsRepo
	authService    *service.AuthService
	storageService *service.StorageService
	cardGen        *service.CardGenerator
	userTagRepo    *repository.UserTagRepo
	profanity      *service.ProfanityFilter
	sessionMgr     *middleware.SessionManager
	cfg            *config.Config
	uploadSem      chan struct{} // limits concurrent image processing
	db             *pgxpool.Pool
}

func NewHandler(
	routeRepo *repository.RouteRepo,
	wallRepo *repository.WallRepo,
	locationRepo *repository.LocationRepo,
	userRepo *repository.UserRepo,
	tagRepo *repository.TagRepo,
	ascentRepo *repository.AscentRepo,
	ratingRepo *repository.RatingRepo,
	difficultyRepo *repository.DifficultyRepo,
	orgRepo *repository.OrgRepo,
	sessionRepo *repository.SessionRepo,
	analyticsRepo *repository.AnalyticsRepo,
	webSessionRepo *repository.WebSessionRepo,
	photoRepo *repository.RoutePhotoRepo,
	settingsRepo *repository.SettingsRepo,
	userTagRepo *repository.UserTagRepo,
	authService *service.AuthService,
	storageService *service.StorageService,
	cardGen *service.CardGenerator,
	sessionMgr *middleware.SessionManager,
	cfg *config.Config,
	db *pgxpool.Pool,
) *Handler {
	h := &Handler{
		routeRepo:      routeRepo,
		wallRepo:       wallRepo,
		locationRepo:   locationRepo,
		userRepo:       userRepo,
		tagRepo:        tagRepo,
		ascentRepo:     ascentRepo,
		ratingRepo:     ratingRepo,
		difficultyRepo: difficultyRepo,
		orgRepo:        orgRepo,
		sessionRepo:    sessionRepo,
		analyticsRepo:  analyticsRepo,
		webSessionRepo: webSessionRepo,
		photoRepo:      photoRepo,
		settingsRepo:   settingsRepo,
		userTagRepo:    userTagRepo,
		profanity:      service.NewProfanityFilter(),
		authService:    authService,
		storageService: storageService,
		cardGen:        cardGen,
		sessionMgr:     sessionMgr,
		cfg:            cfg,
		uploadSem:      make(chan struct{}, 3), // limit concurrent image processing
		db:             db,
	}
	h.loadTemplates()
	return h
}

var funcMap = template.FuncMap{
	"deref":      derefString,
	"derefFloat": derefFloat64,
	"derefInt":   derefInt,
	"title":      titleCase,
	"reltime":    relativeTime,
	"abs":        absInt,
	"seq":        seq,
	"printf":     fmt.Sprintf,
	"staticPath": StaticPath,
	"safeCSS": func(s string) template.CSS {
		return template.CSS(s)
	},
	"roleName": roleDisplayName,
	"add": func(a, b int) int { return a + b },
	"sub": func(a, b int) int { return a - b },
	"initial": func(s string) string {
		if len(s) == 0 {
			return "?"
		}
		return strings.ToUpper(s[:1])
	},
}

func (h *Handler) loadTemplates() {
	h.templates = make(map[string]*template.Template)

	tFS, err := fs.Sub(web.TemplateFS, "templates")
	if err != nil {
		panic("cannot access template FS: " + err.Error())
	}

	// Read shared layout files
	baseBytes := mustRead(tFS, "base.html")
	sidebarBytes := mustRead(tFS, "partials/sidebar.html")
	routeCardBytes := mustRead(tFS, "partials/route-card.html")

	shared := string(baseBytes) + "\n" + string(sidebarBytes) + "\n" + string(routeCardBytes)

	// Parse each page template with the shared layout
	pages := []string{
		"setter/dashboard.html",
		"setter/route-form.html",
		"setter/route-manage.html",
		"setter/walls.html",
		"setter/wall-form.html",
		"setter/wall-detail.html",
		"setter/sessions.html",
		"setter/session-detail.html",
		"setter/session-form.html",
		"setter/session-complete.html",
		"setter/session-photos.html",
		"setter/settings.html",
		"setter/org-settings.html",
		"setter/team.html",
		"setter/org-team.html",
		"setter/gym-new.html",
		"setter/gym-edit.html",
		"climber/routes.html",
		"climber/archive.html",
		"climber/route-detail.html",
		"climber/walls.html",
		"climber/profile.html",
		"climber/profile-settings.html",
		"climber/join-gym.html",
		"error.html",
	}

	for _, page := range pages {
		pageBytes := mustRead(tFS, page)

		// Parse shared layout first, then the page template in a second Parse
		// call. This allows the page's {{define "title"}} to override the
		// {{block "title"}} default from base.html without a "multiple
		// definition" error (Go's template engine allows overrides across
		// separate Parse calls but not within a single one).
		tmpl, parseErr := template.New(page).Funcs(funcMap).Parse(shared)
		if parseErr != nil {
			panic(fmt.Sprintf("failed to parse shared layout for %s: %v", page, parseErr))
		}
		if _, parseErr = tmpl.Parse(string(pageBytes)); parseErr != nil {
			panic(fmt.Sprintf("failed to parse template %s: %v", page, parseErr))
		}
		h.templates[page] = tmpl
	}

	// Standalone pages (no sidebar/base layout)
	standalone := []string{
		"auth/login.html",
		"auth/register.html",
		"auth/setup.html",
	}
	for _, page := range standalone {
		pageBytes := mustRead(tFS, page)
		tmpl, parseErr := template.New(page).Funcs(funcMap).Parse(string(pageBytes))
		if parseErr != nil {
			panic(fmt.Sprintf("failed to parse template %s: %v", page, parseErr))
		}
		h.templates[page] = tmpl
	}
}

func mustRead(fsys fs.FS, name string) []byte {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		panic(fmt.Sprintf("cannot read %s: %v", name, err))
	}
	return data
}

// render executes a page template. HTMX requests get just the content block;
// full page loads get the complete HTML shell.
func (h *Handler) render(w http.ResponseWriter, r *http.Request, page string, data *PageData) {
	tmpl, ok := h.templates[page]
	if !ok {
		slog.Error("template not found", "page", page)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Please try again later.")
		return
	}

	// Inject CSRF token into every render
	data.CSRFToken = middleware.TokenFromRequest(r)

	// Populate location and view-as data for the sidebar
	h.enrichTemplateData(r, &data.TemplateData)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	isHTMX := r.Header.Get("HX-Request") == "true"

	if isHTMX {
		// HTMX partial swap — render just the "content" block
		if err := tmpl.ExecuteTemplate(w, "content", data); err != nil {
			slog.Error("template render failed", "page", page, "error", err)
			http.Error(w, "Something went wrong", http.StatusInternalServerError)
		}
		return
	}

	// Full page — render base.html which includes sidebar + content
	if err := tmpl.ExecuteTemplate(w, page, data); err != nil {
		slog.Error("template render failed", "page", page, "error", err)
		http.Error(w, "Something went wrong", http.StatusInternalServerError)
	}
}

// renderError renders a user-friendly error page. Does not expose internals.
func (h *Handler) renderError(w http.ResponseWriter, r *http.Request, code int, title, message string) {
	tmpl, ok := h.templates["error.html"]
	if !ok {
		// Last resort: plain text
		http.Error(w, title, code)
		return
	}

	data := &PageData{
		TemplateData: TemplateData{
			ActiveNav:   "",
			UserName:    "Guest",
			UserInitial: "?",
		},
		ErrorCode:    code,
		ErrorTitle:   title,
		ErrorMessage: message,
	}
	data.CSRFToken = middleware.TokenFromRequest(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)

	if err := tmpl.ExecuteTemplate(w, "error.html", data); err != nil {
		slog.Error("error template render failed", "error", err)
		http.Error(w, title, code)
	}
}

// checkLocationOwnership verifies a resource belongs to the user's active location.
// Returns true if the check passes (locations match or no location context).
// Returns false and renders a 404 if the resource belongs to a different location.
func (h *Handler) checkLocationOwnership(w http.ResponseWriter, r *http.Request, resourceLocationID string) bool {
	locationID := middleware.GetWebLocationID(r.Context())
	if locationID != "" && resourceLocationID != locationID {
		h.renderError(w, r, http.StatusNotFound, "Not found", "This resource doesn't exist.")
		return false
	}
	return true
}

// StaticHandler is defined in static.go

// ── Route Handlers ────────────────────────────────────────────

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a location to view the dashboard.")
		return
	}

	// Dashboard stats
	dbStats, err := h.analyticsRepo.LocationDashboardStats(ctx, locationID)
	if err != nil {
		slog.Error("dashboard stats failed", "location_id", locationID, "error", err)
		dbStats = &repository.LocationDashboard{}
	}

	stats := &DashboardStats{
		ActiveRoutes: dbStats.ActiveRoutes,
		ActiveDelta:  dbStats.ActiveDelta,
		TotalSends:   dbStats.TotalSends30d,
		AvgRating:    dbStats.AvgRating,
		DueForStrip:  dbStats.DueForStrip,
	}

	// Walls + active routes in two queries (no N+1)
	walls, err := h.wallRepo.ListByLocation(ctx, locationID)
	if err != nil {
		slog.Error("dashboard walls failed", "location_id", locationID, "error", err)
	}

	allRoutes, err := h.routeRepo.ListActiveByLocation(ctx, locationID)
	if err != nil {
		slog.Error("dashboard routes failed", "location_id", locationID, "error", err)
	}

	// Group routes by wall ID
	routesByWall := make(map[string][]RouteView)
	for _, rd := range allRoutes {
		if len(routesByWall[rd.WallID]) >= 6 {
			continue // dashboard shows max 6 per wall
		}
		routesByWall[rd.WallID] = append(routesByWall[rd.WallID], RouteView{
			Route:      rd.Route,
			WallName:   rd.WallName,
			SetterName: rd.SetterName,
		})
	}

	var wallViews []WallWithRoutes
	for _, wall := range walls {
		rv := routesByWall[wall.ID]
		if len(rv) == 0 {
			continue
		}
		wallViews = append(wallViews, WallWithRoutes{
			Wall:       wall,
			RouteCount: len(rv),
			Routes:     rv,
		})
	}

	// Recent activity
	recentEntries, err := h.analyticsRepo.RecentActivity(ctx, locationID, 8)
	if err != nil {
		slog.Error("dashboard activity failed", "location_id", locationID, "error", err)
	}

	var activityItems []ActivityItem
	for _, e := range recentEntries {
		initial := "?"
		if len(e.UserName) > 0 {
			initial = strings.ToUpper(e.UserName[:1])
		}

		grade := e.RouteGrade
		if e.RouteGradingSystem == "v_scale" && e.RouteCircuitColor != nil {
			grade = titleCase(*e.RouteCircuitColor)
		}
		name := ""
		if e.RouteName != nil {
			name = *e.RouteName
		}

		activityItems = append(activityItems, ActivityItem{
			UserName:    e.UserName,
			UserInitial: initial,
			ActionText:  e.AscentType,
			Time:        e.Time,
			RoutePreview: &RoutePreviewData{
				Color: e.RouteColor,
				Grade: grade,
				Name:  name,
			},
		})
	}

	data := &PageData{
		TemplateData:   templateDataFromContext(r, "dashboard"),
		Stats:          stats,
		Walls:          wallViews,
		RecentActivity: activityItems,
	}
	h.render(w, r, "setter/dashboard.html", data)
}

func (h *Handler) Routes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a location.")
		return
	}

	routeType := r.URL.Query().Get("type")
	if !validRouteTypes[routeType] {
		routeType = ""
	}
	wallID := r.URL.Query().Get("wall")
	gradeFilter := r.URL.Query().Get("grade")

	// Map frontend route types to DB route_type enum
	dbRouteType := routeType
	if routeType == "sport" || routeType == "top_rope" {
		dbRouteType = "route" // DB uses 'route' for all roped climbs
	}

	filter := repository.RouteFilter{
		LocationID: locationID,
		WallID:     wallID,
		Status:     "active",
		RouteType:  dbRouteType,
		Limit:      50,
	}

	// Expand grade filter chip values into SQL-friendly filter fields
	if gradeFilter != "" {
		if cc, ok := strings.CutPrefix(gradeFilter, "circuit:"); ok {
			filter.CircuitColor = cc
		} else {
			filter.GradeIn = expandGradeRange(gradeFilter)
		}
	}

	routes, total, err := h.routeRepo.ListWithDetails(ctx, filter)
	if err != nil {
		slog.Error("routes list failed", "location_id", locationID, "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not load routes.")
		return
	}

	// Load location settings for circuit grade visibility and filter chips
	locSettings, settErr := h.settingsRepo.GetLocationSettings(ctx, locationID)
	if settErr != nil {
		locSettings = model.DefaultLocationSettings()
	}

	// Determine if circuit grades should be hidden from this user
	effectiveRole := middleware.GetWebRole(ctx)
	isSetter := effectiveRole != "climber"
	hideCircuitGrade := !isSetter && !locSettings.Grading.ShowGradesOnCircuit

	var routeViews []RouteView
	for _, rd := range routes {
		routeViews = append(routeViews, RouteView{
			Route:            rd.Route,
			WallName:         rd.WallName,
			SetterName:       rd.SetterName,
			HideCircuitGrade: hideCircuitGrade,
		})
	}

	// Build grade groups from the grade distribution query
	gradeDist, err := h.analyticsRepo.GradeDistribution(ctx, locationID, "", "active")
	if err != nil {
		slog.Error("grade distribution failed", "location_id", locationID, "error", err)
	}

	gradeGroups := buildGradeGroups(gradeDist, &locSettings, isSetter)

	// Load walls for filter dropdown
	walls, wallErr := h.wallRepo.ListByLocation(ctx, locationID)
	if wallErr != nil {
		slog.Error("load walls for route filter failed", "error", wallErr)
	}

	data := &PageData{
		TemplateData:        templateDataFromContext(r, "routes"),
		Routes:              routeViews,
		TotalRoutes:         total,
		RouteType:           routeType,
		GradeGroups:         gradeGroups,
		GradeFilter:         gradeFilter,
		FormWalls:           walls,
		WallFilter:          wallID,
		ShowGradesOnCircuit: locSettings.Grading.ShowGradesOnCircuit,
	}
	h.render(w, r, "climber/routes.html", data)
}

// Archive renders the archived routes browser (GET /archive).
func (h *Handler) Archive(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a location.")
		return
	}

	routeType := r.URL.Query().Get("type")
	if !validRouteTypes[routeType] {
		routeType = ""
	}
	wallID := r.URL.Query().Get("wall")
	gradeFilter := r.URL.Query().Get("grade")
	dateFrom := r.URL.Query().Get("from")
	dateTo := r.URL.Query().Get("to")

	// Validate date format if provided
	if dateFrom != "" {
		if _, err := time.Parse("2006-01-02", dateFrom); err != nil {
			dateFrom = ""
		}
	}
	if dateTo != "" {
		if _, err := time.Parse("2006-01-02", dateTo); err != nil {
			dateTo = ""
		}
	}

	filter := repository.RouteFilter{
		LocationID: locationID,
		WallID:     wallID,
		Status:     "archived",
		RouteType:  routeType,
		DateFrom:   dateFrom,
		DateTo:     dateTo,
		Limit:      50,
	}

	if gradeFilter != "" {
		if cc, ok := strings.CutPrefix(gradeFilter, "circuit:"); ok {
			filter.CircuitColor = cc
		} else {
			filter.GradeIn = expandGradeRange(gradeFilter)
		}
	}

	routes, total, err := h.routeRepo.ListWithDetails(ctx, filter)
	if err != nil {
		slog.Error("archive list failed", "location_id", locationID, "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not load archived routes.")
		return
	}

	locSettings, settErr := h.settingsRepo.GetLocationSettings(ctx, locationID)
	if settErr != nil {
		locSettings = model.DefaultLocationSettings()
	}

	effectiveRole := middleware.GetWebRole(ctx)
	isSetter := effectiveRole != "climber"
	hideCircuitGrade := !isSetter && !locSettings.Grading.ShowGradesOnCircuit

	var routeViews []RouteView
	for _, rd := range routes {
		routeViews = append(routeViews, RouteView{
			Route:            rd.Route,
			WallName:         rd.WallName,
			SetterName:       rd.SetterName,
			HideCircuitGrade: hideCircuitGrade,
		})
	}

	// Load walls for filter dropdown
	walls, wallErr := h.wallRepo.ListByLocation(ctx, locationID)
	if wallErr != nil {
		slog.Error("load walls for archive filter failed", "error", wallErr)
	}

	// Build grade groups from the actual archived routes so chips reflect
	// what's in the archive, not what's currently active on the walls.
	gradeDist, err := h.analyticsRepo.GradeDistribution(ctx, locationID, "", "archived")
	if err != nil {
		slog.Error("archive grade distribution failed", "location_id", locationID, "error", err)
	}
	gradeGroups := buildGradeGroups(gradeDist, &locSettings, isSetter)

	data := &PageData{
		TemplateData:        templateDataFromContext(r, "archive"),
		Routes:              routeViews,
		TotalRoutes:         total,
		RouteType:           routeType,
		GradeGroups:         gradeGroups,
		GradeFilter:         gradeFilter,
		FormWalls:           walls,
		WallFilter:          wallID,
		DateFrom:            dateFrom,
		DateTo:              dateTo,
		TypeFilter:          routeType,
		ShowGradesOnCircuit: locSettings.Grading.ShowGradesOnCircuit,
	}
	h.render(w, r, "climber/archive.html", data)
}

func (h *Handler) RouteDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	routeID := chi.URLParam(r, "routeID")

	if !validRouteID.MatchString(routeID) {
		h.renderError(w, r, http.StatusBadRequest, "Invalid route", "The route ID format is not valid.")
		return
	}

	rt, err := h.routeRepo.GetByID(ctx, routeID)
	if err != nil {
		slog.Error("route detail failed", "route_id", routeID, "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not load route.")
		return
	}
	if rt == nil {
		h.renderError(w, r, http.StatusNotFound, "Route not found", "This route doesn't exist or has been removed.")
		return
	}

	if !h.checkLocationOwnership(w, r, rt.LocationID) {
		return
	}

	effectiveRole := middleware.GetWebRole(ctx)
	isSetter := effectiveRole != "climber"
	user := middleware.GetWebUser(ctx)

	// Parallelize all independent queries using a WaitGroup.
	var wg sync.WaitGroup

	var wallName string
	wg.Add(1)
	go func() {
		defer wg.Done()
		if wall, wErr := h.wallRepo.GetByID(ctx, rt.WallID); wErr == nil && wall != nil {
			wallName = wall.Name
		}
	}()

	setterName := "Unknown"
	wg.Add(1)
	go func() {
		defer wg.Done()
		if rt.SetterID != nil {
			if setter, uErr := h.userRepo.GetByID(ctx, *rt.SetterID); uErr == nil && setter != nil {
				setterName = setter.DisplayName
			}
		}
	}()

	var locSettings model.LocationSettings
	wg.Add(1)
	go func() {
		defer wg.Done()
		ls, settErr := h.settingsRepo.GetLocationSettings(ctx, rt.LocationID)
		if settErr != nil {
			locSettings = model.DefaultLocationSettings()
		} else {
			locSettings = ls
		}
	}()

	var recentAscents []AscentView
	wg.Add(1)
	go func() {
		defer wg.Done()
		viewerID := ""
		if user != nil {
			viewerID = user.ID
		}
		ascents, ascentErr := h.ascentRepo.ListByRouteForViewer(ctx, routeID, viewerID, 10, 0)
		if ascentErr != nil {
			slog.Error("load ascents for route detail failed", "route_id", routeID, "error", ascentErr)
		}
		for _, a := range ascents {
			initial := "?"
			if len(a.UserDisplayName) > 0 {
				initial = strings.ToUpper(a.UserDisplayName[:1])
			}
			recentAscents = append(recentAscents, AscentView{
				Ascent:      a.Ascent,
				UserName:    a.UserDisplayName,
				UserInitial: initial,
				AscentType:  ascentTypeLabel(a.AscentType),
				Notes:       a.Notes,
			})
		}
	}()

	var consensus *ConsensusData
	wg.Add(1)
	go func() {
		defer wg.Done()
		consensus = loadConsensus(ctx, h.difficultyRepo, routeID)
	}()

	userRating := 0
	canFlash := true
	hasCompleted := false
	if user != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if existing, rErr := h.ratingRepo.GetByUserAndRoute(ctx, user.ID, routeID); rErr == nil && existing != nil {
				userRating = existing.Rating
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			if hasAny, completed, sErr := h.ascentRepo.RouteAscentStatus(ctx, user.ID, routeID); sErr == nil {
				canFlash = !hasAny
				hasCompleted = completed
			}
		}()
	}

	var photos []repository.PhotoWithUploader
	wg.Add(1)
	go func() {
		defer wg.Done()
		var err error
		photos, err = h.photoRepo.ListByRoute(ctx, routeID)
		if err != nil {
			slog.Error("load route photos failed", "route_id", routeID, "error", err)
		}
	}()

	var communityTags []repository.AggregatedTag
	wg.Add(1)
	go func() {
		defer wg.Done()
		viewerID := ""
		if user != nil {
			viewerID = user.ID
		}
		var err error
		communityTags, err = h.userTagRepo.ListByRoute(ctx, routeID, viewerID)
		if err != nil {
			slog.Error("load community tags failed", "route_id", routeID, "error", err)
		}
	}()

	wg.Wait()

	photosEnabled := h.storageService.IsConfigured()

	rv := RouteView{
		Route:            *rt,
		WallName:         wallName,
		SetterName:       setterName,
		HideCircuitGrade: !isSetter && !locSettings.Grading.ShowGradesOnCircuit,
	}

	data := &PageData{
		TemplateData:        templateDataFromContext(r, "routes"),
		Route:               &rv,
		WallName:            wallName,
		SetterName:          setterName,
		RecentAscents:       recentAscents,
		Consensus:           consensus,
		UserRating:          userRating,
		CanFlash:            canFlash,
		HasCompleted:        hasCompleted,
		CommunityTags:       communityTags,
		Photos:              photos,
		PhotosEnabled:       photosEnabled,
		CanUploadPhoto:      photosEnabled,
		ShowGradesOnCircuit: locSettings.Grading.ShowGradesOnCircuit,
	}
	h.render(w, r, "climber/route-detail.html", data)
}

// Auth handlers are in auth.go
// Helper functions are in helpers.go
