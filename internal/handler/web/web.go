package webhandler

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/web"
)

// TemplateData is the shared context for every page render.
type TemplateData struct {
	ActiveNav   string
	IsSetter    bool
	UserName    string
	UserInitial string
	UserRole    string
	Location    *model.Location
}

// PageData embeds TemplateData so templates can access layout fields at top level.
type PageData struct {
	TemplateData

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
	WallName   string
	SetterName string
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

type ActivityItem struct {
	UserName     string
	UserInitial  string
	ActionText   template.HTML
	Time         time.Time
	RoutePreview *RoutePreviewData
}

type RoutePreviewData struct {
	Color string
	Grade string
	Name  string
}

type GradeGroup struct {
	Label    string
	Value    string
	Count    int
	Color    string // hex color for circuit filter chips (empty for graded)
	IsColor  bool   // true = circuit color filter, false = grade range filter
}

type AscentView struct {
	model.Ascent
	UserName    string
	UserInitial string
	AscentType  string
	Notes       *string
}

// Handler serves the HTMX-powered web frontend.
type Handler struct {
	templates    map[string]*template.Template
	routeRepo    *repository.RouteRepo
	wallRepo     *repository.WallRepo
	locationRepo *repository.LocationRepo
	userRepo     *repository.UserRepo
}

func NewHandler(
	routeRepo *repository.RouteRepo,
	wallRepo *repository.WallRepo,
	locationRepo *repository.LocationRepo,
	userRepo *repository.UserRepo,
) *Handler {
	h := &Handler{
		routeRepo:    routeRepo,
		wallRepo:     wallRepo,
		locationRepo: locationRepo,
		userRepo:     userRepo,
	}
	h.loadTemplates()
	return h
}

var funcMap = template.FuncMap{
	"deref":   derefString,
	"title":   titleCase,
	"reltime": relativeTime,
	"abs":     absInt,
	"seq":     seq,
	"printf":  fmt.Sprintf,
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
		"climber/routes.html",
		"climber/route-detail.html",
	}

	for _, page := range pages {
		pageBytes := mustRead(tFS, page)
		tmplText := shared + "\n" + string(pageBytes)

		tmpl, parseErr := template.New(page).Funcs(funcMap).Parse(tmplText)
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
		http.Error(w, fmt.Sprintf("template %q not found", page), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	isHTMX := r.Header.Get("HX-Request") == "true"

	if isHTMX {
		// HTMX partial swap — render just the "content" block
		if err := tmpl.ExecuteTemplate(w, "content", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Full page — render base.html which includes sidebar + content
	if err := tmpl.ExecuteTemplate(w, page, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// StaticHandler returns an http.Handler for /static/ files.
func StaticHandler() http.Handler {
	sub, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		panic("cannot access static FS: " + err.Error())
	}
	return http.StripPrefix("/static/", http.FileServer(http.FS(sub)))
}

// ── Route Handlers ────────────────────────────────────────────

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	data := &PageData{
		TemplateData: TemplateData{
			ActiveNav:   "dashboard",
			IsSetter:    true,
			UserName:    "Chris S.",
			UserInitial: "C",
			UserRole:    "Head Setter",
		},
		Stats: &DashboardStats{
			ActiveRoutes: 47,
			ActiveDelta:  3,
			TotalSends:   1284,
			AvgRating:    3.8,
			DueForStrip:  5,
		},
		Walls: []WallWithRoutes{
			{
				Wall:       model.Wall{ID: "wall-1", Name: "The Cave", WallType: "boulder", Angle: strPtr("45°")},
				RouteCount: 8,
				Routes: []RouteView{
					{Route: model.Route{ID: "r1", GradingSystem: "circuit", Color: "#e53935", CircuitColor: strPtr("red"), Name: strPtr("Crimson Crush"), Status: "active", RouteType: "boulder", DateSet: time.Now().Add(-48 * time.Hour), AscentCount: 23, AvgRating: 4.2}, WallName: "The Cave", SetterName: "Chris S."},
					{Route: model.Route{ID: "r2", GradingSystem: "circuit", Color: "#1565c0", CircuitColor: strPtr("blue"), Name: strPtr("Blue Monday"), Status: "active", RouteType: "boulder", DateSet: time.Now().Add(-72 * time.Hour), AscentCount: 45, AvgRating: 3.9}, WallName: "The Cave", SetterName: "Alex M."},
					{Route: model.Route{ID: "r3", GradingSystem: "circuit", Color: "#2e7d32", CircuitColor: strPtr("green"), Status: "active", RouteType: "boulder", DateSet: time.Now().Add(-24 * time.Hour), AscentCount: 8, AvgRating: 4.5}, WallName: "The Cave", SetterName: "Chris S."},
				},
			},
			{
				Wall:       model.Wall{ID: "wall-2", Name: "Main Slab", WallType: "sport", Angle: strPtr("5°")},
				RouteCount: 6,
				Routes: []RouteView{
					{Route: model.Route{ID: "r4", GradingSystem: "yds", Grade: "5.11-", Color: "#7b1fa2", Name: strPtr("Purple Rain"), Status: "active", RouteType: "sport", DateSet: time.Now().Add(-96 * time.Hour), AscentCount: 31, AvgRating: 4.0}, WallName: "Main Slab", SetterName: "Jordan K."},
					{Route: model.Route{ID: "r5", GradingSystem: "yds", Grade: "5.10+", Color: "#f9a825", Name: strPtr("Gold Rush"), Status: "active", RouteType: "sport", DateSet: time.Now().Add(-120 * time.Hour), AscentCount: 52, AvgRating: 3.6}, WallName: "Main Slab", SetterName: "Chris S."},
				},
			},
			{
				Wall:       model.Wall{ID: "wall-3", Name: "Overhang Alley", WallType: "boulder", Angle: strPtr("30°")},
				RouteCount: 4,
				Routes: []RouteView{
					{Route: model.Route{ID: "r6", GradingSystem: "circuit", Color: "#e91e8a", CircuitColor: strPtr("pink"), Name: strPtr("Pink Panther"), Status: "active", RouteType: "boulder", DateSet: time.Now().Add(-36 * time.Hour), AscentCount: 18, AvgRating: 4.1}, WallName: "Overhang Alley", SetterName: "Alex M."},
					{Route: model.Route{ID: "r7", GradingSystem: "circuit", Color: "#00897b", CircuitColor: strPtr("teal"), Name: strPtr("Teal Deal"), Status: "draft", RouteType: "boulder", DateSet: time.Now().Add(-6 * time.Hour), AscentCount: 0, AvgRating: 0.0}, WallName: "Overhang Alley", SetterName: "Chris S."},
				},
			},
		},
		RecentActivity: []ActivityItem{
			{UserName: "Sarah L.", UserInitial: "S", ActionText: "sent", Time: time.Now().Add(-15 * time.Minute), RoutePreview: &RoutePreviewData{Color: "#e53935", Grade: "Red", Name: "Crimson Crush"}},
			{UserName: "Mike T.", UserInitial: "M", ActionText: "flashed", Time: time.Now().Add(-42 * time.Minute), RoutePreview: &RoutePreviewData{Color: "#1565c0", Grade: "Blue", Name: "Blue Monday"}},
			{UserName: "Alex M.", UserInitial: "A", ActionText: "set a new route", Time: time.Now().Add(-2 * time.Hour), RoutePreview: &RoutePreviewData{Color: "#00897b", Grade: "Teal", Name: "Teal Deal"}},
			{UserName: "Jordan K.", UserInitial: "J", ActionText: "rated ★★★★★", Time: time.Now().Add(-3 * time.Hour), RoutePreview: &RoutePreviewData{Color: "#2e7d32", Grade: "Green"}},
			{UserName: "Chris S.", UserInitial: "C", ActionText: "archived 3 routes on The Cave", Time: time.Now().Add(-5 * time.Hour)},
		},
	}
	h.render(w, r, "setter/dashboard.html", data)
}

func (h *Handler) Routes(w http.ResponseWriter, r *http.Request) {
	routeType := r.URL.Query().Get("type")

	allRoutes := []RouteView{
		// Circuit boulders — identified by color, not V-grade
		{Route: model.Route{ID: "r1", GradingSystem: "circuit", Color: "#e53935", CircuitColor: strPtr("red"), Name: strPtr("Crimson Crush"), Status: "active", RouteType: "boulder", DateSet: time.Now().Add(-48 * time.Hour), AscentCount: 23, AvgRating: 4.2}, WallName: "The Cave", SetterName: "Chris S."},
		{Route: model.Route{ID: "r2", GradingSystem: "circuit", Color: "#1565c0", CircuitColor: strPtr("blue"), Name: strPtr("Blue Monday"), Status: "active", RouteType: "boulder", DateSet: time.Now().Add(-72 * time.Hour), AscentCount: 45, AvgRating: 3.9}, WallName: "The Cave", SetterName: "Alex M."},
		{Route: model.Route{ID: "r3", GradingSystem: "circuit", Color: "#2e7d32", CircuitColor: strPtr("green"), Status: "active", RouteType: "boulder", DateSet: time.Now().Add(-24 * time.Hour), AscentCount: 8, AvgRating: 4.5}, WallName: "The Cave", SetterName: "Chris S."},
		{Route: model.Route{ID: "r6", GradingSystem: "circuit", Color: "#e91e8a", CircuitColor: strPtr("pink"), Name: strPtr("Pink Panther"), Status: "active", RouteType: "boulder", DateSet: time.Now().Add(-36 * time.Hour), AscentCount: 18, AvgRating: 4.1}, WallName: "Overhang Alley", SetterName: "Alex M."},
		{Route: model.Route{ID: "r7", GradingSystem: "circuit", Color: "#00897b", CircuitColor: strPtr("teal"), Name: strPtr("Teal Deal"), Status: "active", RouteType: "boulder", DateSet: time.Now().Add(-6 * time.Hour), AscentCount: 3, AvgRating: 3.5}, WallName: "Overhang Alley", SetterName: "Chris S."},
		{Route: model.Route{ID: "r9", GradingSystem: "circuit", Color: "#f9a825", CircuitColor: strPtr("yellow"), Name: strPtr("Sunshine Slab"), Status: "active", RouteType: "boulder", DateSet: time.Now().Add(-12 * time.Hour), AscentCount: 34, AvgRating: 3.7}, WallName: "The Cave", SetterName: "Jordan K."},
		{Route: model.Route{ID: "r10", GradingSystem: "circuit", Color: "#fc5200", CircuitColor: strPtr("orange"), Name: strPtr("Warm Up"), Status: "active", RouteType: "boulder", DateSet: time.Now().Add(-168 * time.Hour), AscentCount: 89, AvgRating: 3.2}, WallName: "The Cave", SetterName: "Jordan K."},
		{Route: model.Route{ID: "r11", GradingSystem: "circuit", Color: "#0a0a0a", CircuitColor: strPtr("black"), Name: strPtr("Dark Arts"), Status: "active", RouteType: "boulder", DateSet: time.Now().Add(-18 * time.Hour), AscentCount: 5, AvgRating: 4.8}, WallName: "Overhang Alley", SetterName: "Chris S."},
		{Route: model.Route{ID: "r12", GradingSystem: "circuit", Color: "#ffffff", CircuitColor: strPtr("white"), Name: strPtr("Ghost"), Status: "active", RouteType: "boulder", DateSet: time.Now().Add(-30 * time.Hour), AscentCount: 27, AvgRating: 3.4}, WallName: "The Cave", SetterName: "Alex M."},
		// Graded sport routes
		{Route: model.Route{ID: "r4", GradingSystem: "yds", Grade: "5.11-", Color: "#7b1fa2", Name: strPtr("Purple Rain"), Status: "active", RouteType: "sport", DateSet: time.Now().Add(-96 * time.Hour), AscentCount: 31, AvgRating: 4.0}, WallName: "Main Slab", SetterName: "Jordan K."},
		{Route: model.Route{ID: "r5", GradingSystem: "yds", Grade: "5.10+", Color: "#f9a825", Name: strPtr("Gold Rush"), Status: "active", RouteType: "sport", DateSet: time.Now().Add(-120 * time.Hour), AscentCount: 52, AvgRating: 3.6}, WallName: "Main Slab", SetterName: "Chris S."},
	}

	var filtered []RouteView
	for _, rv := range allRoutes {
		if routeType == "" || rv.RouteType == routeType {
			filtered = append(filtered, rv)
		}
	}

	data := &PageData{
		TemplateData: TemplateData{
			ActiveNav:   "routes",
			IsSetter:    true,
			UserName:    "Chris S.",
			UserInitial: "C",
			UserRole:    "Head Setter",
		},
		Routes:      filtered,
		TotalRoutes: len(filtered),
		RouteType:   routeType,
		GradeGroups: []GradeGroup{
			// Circuit colors (boulders)
			{Label: "Orange", Value: "orange", Count: 3, Color: "#fc5200", IsColor: true},
			{Label: "Red", Value: "red", Count: 4, Color: "#e53935", IsColor: true},
			{Label: "Blue", Value: "blue", Count: 5, Color: "#1565c0", IsColor: true},
			{Label: "Green", Value: "green", Count: 3, Color: "#2e7d32", IsColor: true},
			{Label: "Yellow", Value: "yellow", Count: 2, Color: "#f9a825", IsColor: true},
			{Label: "Pink", Value: "pink", Count: 3, Color: "#e91e8a", IsColor: true},
			{Label: "Black", Value: "black", Count: 2, Color: "#0a0a0a", IsColor: true},
			{Label: "White", Value: "white", Count: 2, Color: "#ffffff", IsColor: true},
			// Graded sport/top rope routes (plus/minus notation)
			{Label: "5.8 & under", Value: "5.8-under", Count: 4},
			{Label: "5.9", Value: "5.9", Count: 6},
			{Label: "5.10", Value: "5.10", Count: 8},
			{Label: "5.11", Value: "5.11", Count: 5},
			{Label: "5.12+", Value: "5.12-up", Count: 2},
		},
	}
	h.render(w, r, "climber/routes.html", data)
}

func (h *Handler) RouteDetail(w http.ResponseWriter, r *http.Request) {
	routeID := chi.URLParam(r, "routeID")

	rv := RouteView{
		Route: model.Route{
			ID:            routeID,
			GradingSystem: "circuit",
			Color:         "#e53935",
			CircuitColor:  strPtr("red"),
			Name:          strPtr("Crimson Crush"),
			Status:        "active",
			RouteType:     "boulder",
			DateSet:       time.Now().Add(-48 * time.Hour),
			AscentCount:   23,
			AttemptCount:  41,
			AvgRating:     4.2,
			RatingCount:   15,
			Description:   strPtr("Powerful start on crimps leads to a dynamic move to a big sloper. Technical footwork required on the slab section. Great for working contact strength."),
			Tags: []model.Tag{
				{Name: "Crimps"},
				{Name: "Dynamic"},
				{Name: "Slab finish"},
			},
		},
		WallName:   "The Cave",
		SetterName: "Chris S.",
	}

	// Consensus difficulty voting data
	consensus := &ConsensusData{
		EasyCount:  4,
		RightCount: 12,
		HardCount:  7,
		TotalVotes: 23,
	}
	consensus.EasyPct = consensus.EasyCount * 100 / consensus.TotalVotes
	consensus.HardPct = consensus.HardCount * 100 / consensus.TotalVotes
	consensus.RightPct = 100 - consensus.EasyPct - consensus.HardPct

	data := &PageData{
		TemplateData: TemplateData{
			ActiveNav:   "routes",
			IsSetter:    true,
			UserName:    "Chris S.",
			UserInitial: "C",
			UserRole:    "Head Setter",
		},
		Route:      &rv,
		WallName:   rv.WallName,
		SetterName: rv.SetterName,
		Consensus:  consensus,
		RecentAscents: []AscentView{
			{Ascent: model.Ascent{ClimbedAt: time.Now().Add(-15 * time.Minute), Attempts: 1}, UserName: "Sarah L.", UserInitial: "S", AscentType: "flashed"},
			{Ascent: model.Ascent{ClimbedAt: time.Now().Add(-2 * time.Hour), Attempts: 3}, UserName: "Mike T.", UserInitial: "M", AscentType: "sent", Notes: strPtr("Took a few tries to figure out the beta on the start. Key is right heel hook.")},
			{Ascent: model.Ascent{ClimbedAt: time.Now().Add(-5 * time.Hour), Attempts: 1}, UserName: "Jordan K.", UserInitial: "J", AscentType: "sent"},
		},
	}
	h.render(w, r, "climber/route-detail.html", data)
}

// ── Template Helpers ──────────────────────────────────────────

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%dh ago", h)
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "yesterday"
		}
		return fmt.Sprintf("%dd ago", days)
	default:
		return t.Format("Jan 2")
	}
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func seq(start, end int) []int {
	s := make([]int, 0, end-start+1)
	for i := start; i <= end; i++ {
		s = append(s, i)
	}
	return s
}

func strPtr(s string) *string { return &s }
