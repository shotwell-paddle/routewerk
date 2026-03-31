package webhandler

import (
	"context"
	"fmt"
	"html/template"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/config"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

// SettingsStore abstracts settings access so the handler can work with
// either a direct SettingsRepo or a CachedSettingsRepo.
type SettingsStore interface {
	GetLocationSettings(ctx context.Context, locationID string) (model.LocationSettings, error)
	UpdateLocationSettings(ctx context.Context, locationID string, settings model.LocationSettings) error
	GetOrgSettings(ctx context.Context, orgID string) (model.OrgSettings, error)
	UpdateOrgSettings(ctx context.Context, orgID string, settings model.OrgSettings) error
	GetUserSettings(ctx context.Context, userID string) (model.UserSettings, error)
	UpdateUserSettings(ctx context.Context, userID string, settings model.UserSettings) error
}

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
	settingsRepo   SettingsStore
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
	settingsRepo SettingsStore,
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

// Auth handlers are in auth.go
// Helper functions are in helpers.go
