package router

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shotwell-paddle/routewerk/internal/config"
	"github.com/shotwell-paddle/routewerk/internal/event"
	"github.com/shotwell-paddle/routewerk/internal/handler"
	webhandler "github.com/shotwell-paddle/routewerk/internal/handler/web"
	"github.com/shotwell-paddle/routewerk/internal/jobs"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
	"github.com/shotwell-paddle/routewerk/internal/service/cardbatch"
	"github.com/shotwell-paddle/routewerk/internal/service/cardsheet"
	"github.com/shotwell-paddle/routewerk/internal/sse"
	"github.com/shotwell-paddle/routewerk/web/spa"
)

// Deps holds dependencies passed from main.
type Deps struct {
	JobQueue *jobs.Queue
	EventBus event.Bus
	NotifSvc *service.NotificationService
	QuestSvc *service.QuestService
}

func New(cfg *config.Config, db *pgxpool.Pool, deps *Deps) *chi.Mux {
	r := chi.NewRouter()

	// Request metrics (lightweight in-process counters)
	metrics := middleware.NewMetrics()

	// Global middleware (applied to ALL routes — API and web)
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(metrics.Collect)
	r.Use(middleware.HSTS(cfg.IsDev()))
	r.Use(middleware.Logger)
	r.Use(middleware.Recovery)

	// CORS — always use explicit origins (never wildcard) so credentials work safely.
	// In dev, allow common local dev ports; in production, allow the configured frontend
	// plus any additional serving domains (Fly default hostname, other custom domains).
	allowedOrigins := []string{cfg.FrontendURL}
	if cfg.IsDev() {
		allowedOrigins = []string{"http://localhost:3000", "http://localhost:8080", "http://127.0.0.1:3000", "http://127.0.0.1:8080"}
	} else {
		for _, extra := range cfg.ExtraOrigins {
			if extra != "" {
				allowedOrigins = append(allowedOrigins, extra)
			}
		}
	}
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Rate limiter for auth endpoints: 20 requests per minute per IP
	authLimiter := middleware.NewRateLimiter(20, 1*time.Minute)

	// Authorization middleware — enforces org/location membership + roles
	authz := middleware.NewAuthorizer(db)

	// Repositories
	userRepo := repository.NewUserRepo(db)
	loginAttemptRepo := repository.NewLoginAttemptRepo(db)
	orgRepo := repository.NewOrgRepo(db)
	locationRepo := repository.NewLocationRepo(db)
	wallRepo := repository.NewWallRepo(db)
	routeRepo := repository.NewRouteRepo(db)
	ascentRepo := repository.NewAscentRepo(db)
	ratingRepo := repository.NewRatingRepo(db)
	sessionRepo := repository.NewSessionRepo(db)
	laborRepo := repository.NewLaborRepo(db)
	tagRepo := repository.NewTagRepo(db)
	followRepo := repository.NewFollowRepo(db)
	trainingRepo := repository.NewTrainingRepo(db)
	partnerRepo := repository.NewPartnerRepo(db)
	analyticsRepo := repository.NewAnalyticsRepo(db)
	webSessionRepo := repository.NewWebSessionRepo(db)

	// Audit — queue-backed so state-changing requests don't wait on the
	// audit_logs insert. Queue is the same Postgres DB, so enqueue + commit
	// gives the same durability guarantee as an inline insert. See S6 in
	// the 2026-04-22 perf audit.
	auditRepo := repository.NewAuditRepo(db)
	auditService := service.NewAuditService(auditRepo, deps.JobQueue)

	// Services
	authService := service.NewAuthService(userRepo, loginAttemptRepo, cfg)
	cardGen := service.NewCardGenerator(cfg.FrontendURL)

	// Magic-link auth: passwordless sign-in. Email send goes through the
	// shared EmailService job handler (registered in main.go); the
	// service here just orchestrates token gen / persistence / enqueue.
	magicLinkRepo := repository.NewMagicLinkRepo(db)
	magicLinkSvc := service.NewMagicLinkService(magicLinkRepo, userRepo, deps.JobQueue, cfg.FrontendURL)

	// Competitions module — all five waves of Phase 1f wired here.
	// Hub is in-process; if we ever scale horizontally, swap for a
	// Postgres LISTEN/NOTIFY adapter behind the same Subscribe/Publish
	// interface (see internal/sse/hub.go).
	compRepo := repository.NewCompetitionRepo(db)
	compRegRepo := repository.NewCompetitionRegistrationRepo(db)
	compAttemptRepo := repository.NewCompetitionAttemptRepo(db)
	compHub := sse.New()
	compHandler := handler.NewCompHandler(compRepo, compRegRepo, compAttemptRepo, userRepo, authz, compHub)

	// Web session manager (cookie-based auth for HTMX frontend)
	sessionMgr := middleware.NewSessionManager(webSessionRepo, userRepo, cfg.IsDev())

	// Handlers
	storageSvc := service.NewStorageService(cfg)
	healthHandler := handler.NewHealthHandler(db, storageSvc)
	authHandler := handler.NewAuthHandler(authService, !cfg.IsDev())
	magicAuthHandler := handler.NewMagicAuthHandler(magicLinkSvc)
	magicVerifyHandler := webhandler.NewMagicVerifyHandler(magicLinkSvc, webSessionRepo, userRepo, sessionMgr, cfg)
	orgHandler := handler.NewOrgHandler(orgRepo, auditService)
	locationHandler := handler.NewLocationHandler(locationRepo)
	wallHandler := handler.NewWallHandler(wallRepo, auditService)
	routeHandler := handler.NewRouteHandler(routeRepo, auditService)
	teamHandler := handler.NewTeamHandler(userRepo)
	questHandler := handler.NewQuestHandler(deps.QuestSvc)
	// notifHandler is initialized lower, after notifRepo is constructed
	// for the web handler bundle.
	ascentHandler := handler.NewAscentHandler(ascentRepo)
	ratingHandler := handler.NewRatingHandler(ratingRepo)
	sessionHandler := handler.NewSessionHandler(sessionRepo, routeRepo)
	laborHandler := handler.NewLaborHandler(laborRepo)
	tagHandler := handler.NewTagHandler(tagRepo, auditService)
	followHandler := handler.NewFollowHandler(followRepo)
	trainingHandler := handler.NewTrainingHandler(trainingRepo)
	partnerHandler := handler.NewPartnerHandler(partnerRepo)
	analyticsHandler := handler.NewAnalyticsHandler(analyticsRepo)
	cardHandler := handler.NewCardHandler(routeRepo, wallRepo, locationRepo, userRepo, cardGen)

	// Card batch pipeline: the sheet composer wraps cardGen for 8-up print-and-cut
	// PDF rendering; CardBatchService coordinates ID → CardData lookups and drives
	// the composer. Both web and JSON API handlers share the same service.
	cardBatchRepo := repository.NewCardBatchRepo(db)
	sheetComposer := cardsheet.NewComposer(cardGen)
	batchSvc := cardbatch.NewService(routeRepo, wallRepo, locationRepo, userRepo, sheetComposer, cardGen)
	cardBatchHandler := handler.NewCardBatchHandler(cardBatchRepo, batchSvc, auditService)

	// Health check — public (Fly.io probes need this), but pool details
	// are only returned for internal IPs (see health.go).
	r.Get("/health", healthHandler.Check)

	// ── Web Frontend (HTMX) ────────────────────────────────────
	difficultyRepo := repository.NewDifficultyRepo(db)
	photoRepo := repository.NewRoutePhotoRepo(db)
	settingsRepo := repository.NewCachedSettingsRepo(repository.NewSettingsRepo(db))
	userTagRepo := repository.NewUserTagRepo(db)

	// Progressions repos (created in main.go for service layer, but we also
	// need them here for the web handler; create local instances since repos
	// are stateless wrappers around the DB pool).
	questRepo := repository.NewQuestRepo(db)
	badgeRepo := repository.NewBadgeRepo(db)
	activityRepo := repository.NewActivityRepo(db)
	routeSkillTagRepo := repository.NewRouteSkillTagRepo(db)
	notifRepo := repository.NewNotificationRepo(db)
	notifHandler := handler.NewNotificationHandler(notifRepo)
	dashboardHandler := handler.NewDashboardHandler(analyticsRepo)
	settingsHandler := handler.NewSettingsHandler(settingsRepo)
	progressionsAdminHandler := handler.NewProgressionsAdminHandler(questRepo, badgeRepo)
	// JSON variant of the HTMX route-photo upload pipeline (multipart upload,
	// image processing, S3 upload, route_photos row insert). Cap concurrent
	// processing at 4 to bound peak memory on the 256 MB VM.
	routePhotoHandler := handler.NewRoutePhotoHandler(routeRepo, photoRepo, storageSvc, 4)
	// Community tags — same UserTagRepo as the HTMX side. Profanity filter
	// is stateless so a fresh instance here is fine; both filters share
	// the same blocklist source.
	routeTagHandler := handler.NewRouteTagHandler(routeRepo, userTagRepo, service.NewProfanityFilter())
	// Climber difficulty consensus — same difficulty_votes table as the
	// HTMX feedback flow.
	routeDifficultyHandler := handler.NewRouteDifficultyHandler(routeRepo, difficultyRepo)
	// Climber-facing badge showcase — pairs the location's catalog with
	// the caller's earned set in one round-trip.
	badgeShowcaseHandler := handler.NewBadgeShowcaseHandler(badgeRepo)
	// Location-wide activity feed (quest progress, badges earned, route
	// sets). Mirrors the HTMX /quests/activity page but is not gated by
	// the progressions feature flag — staff often want activity while
	// the flag is still off.
	activityHandler := handler.NewActivityHandler(activityRepo)
	// Setter playbook — default checklist template applied to new
	// sessions. Mirrors HTMX /settings/playbook.
	playbookHandler := handler.NewPlaybookHandler(sessionRepo)

	webHandler := webhandler.NewHandler(routeRepo, wallRepo, locationRepo, userRepo, tagRepo, ascentRepo, ratingRepo, difficultyRepo, orgRepo, sessionRepo, analyticsRepo, webSessionRepo, photoRepo, settingsRepo, userTagRepo, questRepo, badgeRepo, activityRepo, routeSkillTagRepo, notifRepo, deps.QuestSvc, deps.EventBus, authService, storageSvc, cardGen, cardBatchRepo, batchSvc, auditService, sessionMgr, cfg, db)

	// Rate limiter for web pages: 120 requests per minute per IP
	webLimiter := middleware.NewRateLimiter(120, 1*time.Minute)

	// Stricter rate limiter for login: 10 requests per minute per IP
	loginLimiter := middleware.NewRateLimiter(10, 1*time.Minute)

	// Per-user throttle for card-batch creation: 10 per hour. PDF rendering
	// is CPU-heavy (8 cards × fontdraw + PDF encode) and a shared setter
	// account behind one gym IP shouldn't collapse into a single bucket —
	// key on user id. Applied to both the web form POST and the JSON API.
	batchCreateLimiter := middleware.NewRateLimiter(10, 1*time.Hour)
	batchCreateLimitByUser := batchCreateLimiter.LimitByKey(func(r *http.Request) string {
		if u := middleware.GetWebUser(r.Context()); u != nil {
			return "web:" + u.ID
		}
		if id := middleware.GetUserID(r.Context()); id != "" {
			return "api:" + id
		}
		return ""
	})

	// CSRF protection for state-changing requests
	csrf := middleware.NewCSRFProtection(cfg.IsDev())

	// Static assets — immutable cache headers, gzip compressed
	r.Group(func(r chi.Router) {
		r.Use(middleware.SecureHeadersStatic)
		r.Use(middleware.Gzip)
		r.Handle("/static/*", webhandler.StaticHandler())
		// SPA build assets are content-hashed under /_app/* — same caching
		// policy. The SvelteKit build emits absolute paths, so we mount at
		// the same root path the build references.
		r.Handle("/_app/*", spa.AssetServer())
	})

	// SPA fallback: any unmatched path under an SPA-owned prefix returns
	// index.html and the client router takes over. Each SPA-owned prefix
	// is mounted at both /prefix and /prefix/* so SvelteKit's
	// trailingSlash='never' default (URL ends up as /prefix) doesn't
	// 404 on reload. All registrations point at the same handler.
	//
	// Phase 1g adds /comp/*; Phase 1h adds /staff/comp/*. /spa-test
	// stays around as a smoke-test landing.
	r.Group(func(r chi.Router) {
		r.Use(middleware.Gzip)
		r.Get("/favicon.svg", func(w http.ResponseWriter, req *http.Request) {
			spa.AssetServer().ServeHTTP(w, req)
		})
		r.Handle("/spa-test", spa.FallbackHandler())
		r.Handle("/spa-test/*", spa.FallbackHandler())
		r.Handle("/comp", spa.FallbackHandler())
		r.Handle("/comp/*", spa.FallbackHandler())
		// SPA's magic-link sign-in lives at /sign-in (not /login —
		// /login is taken by the HTMX password-auth page used by staff).
		r.Handle("/sign-in", spa.FallbackHandler())
		r.Handle("/sign-in/*", spa.FallbackHandler())
		// /staff/comp/* used to host the comp staff UI; it now lives
		// under /app/competitions/* so it shares the workspace shell
		// (sidebar, location picker, role pill). 308 redirects preserve
		// any bookmarks pointing at the old paths.
		r.Get("/staff/comp", func(w http.ResponseWriter, req *http.Request) {
			http.Redirect(w, req, "/app/competitions", http.StatusPermanentRedirect)
		})
		r.Get("/staff/comp/*", func(w http.ResponseWriter, req *http.Request) {
			rest := chi.URLParam(req, "*")
			http.Redirect(w, req, "/app/competitions/"+rest, http.StatusPermanentRedirect)
		})
		// Phase 2 (rebuild): the new SPA shell lives at /app while we
		// migrate the existing HTMX surfaces page-by-page. Once a page
		// has a SPA equivalent ready, its old HTMX route can be swapped
		// to point here as well.
		r.Handle("/app", spa.FallbackHandler())
		r.Handle("/app/*", spa.FallbackHandler())
	})

	// Web pages — web-specific CSP, CSRF, rate limiting, gzip, query timeout.
	// 10 MB body cap is generous enough for our two multipart upload paths
	// (avatar + route photos — 5 MB file + form overhead) while still
	// preventing a slow-streaming caller from pinning a goroutine with
	// megabytes of form keys. See S3 in the 2026-04-22 perf audit.
	r.Group(func(r chi.Router) {
		r.Use(middleware.SecureHeadersWeb(cfg.StorageEndpoint))
		r.Use(middleware.Gzip)
		r.Use(webLimiter.Limit)
		r.Use(middleware.RequestTimeout(cfg.QueryTimeout))
		r.Use(middleware.LimitBody(10 << 20)) // 10 MB
		r.Use(csrf.Protect)

		// Public auth routes (no session required, stricter rate limit)
		r.Group(func(r chi.Router) {
			r.Use(loginLimiter.Limit)
			r.Get("/login", webHandler.LoginPage)
			r.Post("/login", webHandler.LoginSubmit)
			r.Get("/register", webHandler.RegisterPage)
			r.Post("/register", webHandler.RegisterSubmit)
			// Magic-link verify: GET so clicking a link from email works.
			// CSRF doesn't apply (GET); auth not required (the token is
			// the credential). The handler sets the session cookie on
			// success and redirects to next/dashboard.
			r.Get("/verify-magic", magicVerifyHandler.Verify)
		})

		// Authenticated web routes
		r.Group(func(r chi.Router) {
			r.Use(sessionMgr.RequireSession)

			r.Get("/", func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
			})
			// First-run setup (only works when no orgs exist)
			r.Get("/setup", webHandler.SetupPage)
			r.Post("/setup", webHandler.SetupSubmit)

			// Gym discovery (works even without a location)
			r.Get("/join-gym", webHandler.JoinGymPage)
			r.Get("/join-gym/search", webHandler.JoinGymSearch)
			r.Post("/join-gym", webHandler.JoinGymSubmit)

			// Location and role switchers
			r.Post("/switch-location", webHandler.SwitchLocation)
			r.Post("/switch-view-as", webHandler.SwitchViewAs)

			// Climber routes — any authenticated user
			r.Get("/explore/walls", webHandler.ClimberWalls)
			r.Get("/routes", webHandler.Routes)
			r.Get("/archive", webHandler.Archive)
			r.Get("/routes/{routeID}", webHandler.RouteDetail)
			r.Get("/routes/{routeID}/card/print.png", webHandler.RouteCardPrintPNG)
			r.Get("/routes/{routeID}/card/print.pdf", webHandler.RouteCardPrintPDF)
			r.Get("/routes/{routeID}/card/share.png", webHandler.RouteCardSharePNG)
			r.Get("/routes/{routeID}/card/share.pdf", webHandler.RouteCardSharePDF)
			r.Post("/routes/{routeID}/ascent", webHandler.LogAscent)
			r.Get("/routes/{routeID}/ascents-feed", webHandler.AscentsFeed)
			r.Post("/routes/{routeID}/rate", webHandler.RateRoute)
			r.Post("/routes/{routeID}/difficulty", webHandler.DifficultyVote)
			r.Post("/routes/{routeID}/tags", webHandler.AddCommunityTag)
			r.Post("/routes/{routeID}/tags/remove", webHandler.RemoveCommunityTag)
			r.Post("/routes/{routeID}/tags/delete", webHandler.DeleteCommunityTag)
			r.Post("/routes/{routeID}/photos", webHandler.PhotoUpload)
			r.Post("/routes/{routeID}/photos/{photoID}/delete", webHandler.PhotoDelete)
			r.Get("/profile", webHandler.ClimberProfile)
			r.Get("/profile/ticks", webHandler.ProfileTicks)
			r.Get("/profile/settings", webHandler.ProfileSettings)
			r.Post("/profile/settings", webHandler.ProfileSettingsSave)
			r.Post("/profile/password", webHandler.PasswordChange)
			r.Get("/profile/ticks/{ascentID}/edit", webHandler.TickEditForm)
			r.Post("/profile/ticks/{ascentID}", webHandler.TickUpdate)
			r.Post("/profile/ticks/{ascentID}/delete", webHandler.TickDelete)
			r.Post("/logout", webHandler.Logout)

			// Notifications
			r.Get("/notifications", webHandler.Notifications)
			r.Get("/notifications/badge", webHandler.NotificationBadge)
			r.Post("/notifications/read-all", webHandler.NotificationMarkAllRead)
			r.Post("/notifications/{notifID}/read", webHandler.NotificationMarkRead)

			// Progressions — climber-facing quest system
			r.Get("/quests", webHandler.QuestBrowser)
			r.Get("/quests/mine", webHandler.MyQuests)
			r.Get("/quests/badges", webHandler.BadgeShowcase)
			r.Get("/quests/activity", webHandler.QuestActivity)
			r.Get("/quests/{questID}", webHandler.QuestDetailPage)
			r.Post("/quests/{questID}/start", webHandler.QuestStart)
			r.Post("/quests/{questID}/log", webHandler.QuestLogProgress)
			r.Post("/quests/{questID}/complete", webHandler.QuestComplete)
			r.Post("/quests/{questID}/abandon", webHandler.QuestAbandon)

			// Setter routes — require setter role or above
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireSetterSession)

				r.Get("/dashboard", webHandler.Dashboard)

				// Route creation, editing, status management
				r.Get("/routes/manage", webHandler.RouteManage)
				r.Get("/routes/new", webHandler.RouteNew)
				r.Get("/routes/new/fields", webHandler.RouteFormFields)
				r.Post("/routes/new", webHandler.RouteCreate)
				r.Get("/routes/{routeID}/edit", webHandler.RouteEdit)
				r.Post("/routes/{routeID}/edit", webHandler.RouteUpdate)
				r.Post("/routes/{routeID}/status", webHandler.RouteStatusUpdate)

				// Wall management
				r.Get("/walls", webHandler.WallList)
				r.Get("/walls/new", webHandler.WallNew)
				r.Post("/walls/new", webHandler.WallCreate)
				r.Get("/walls/{wallID}", webHandler.WallDetail)
				r.Get("/walls/{wallID}/edit", webHandler.WallEdit)
				r.Post("/walls/{wallID}/edit", webHandler.WallUpdate)
				r.Post("/walls/{wallID}/archive", webHandler.WallArchive)
				r.Post("/walls/{wallID}/unarchive", webHandler.WallUnarchive)
				r.Post("/walls/{wallID}/delete", webHandler.WallDelete)

				// Setting sessions
				r.Get("/sessions", webHandler.SessionList)
				r.Get("/sessions/new", webHandler.SessionNew)
				r.Post("/sessions/new", webHandler.SessionCreate)
				r.Get("/sessions/{sessionID}", webHandler.SessionDetail)
				r.Get("/sessions/{sessionID}/edit", webHandler.SessionEdit)
				r.Post("/sessions/{sessionID}/edit", webHandler.SessionUpdate)
				r.Post("/sessions/{sessionID}/assign", webHandler.SessionAddAssignment)
				r.Post("/sessions/{sessionID}/unassign/{assignmentID}", webHandler.SessionRemoveAssignment)
				r.Post("/sessions/{sessionID}/strip", webHandler.SessionAddStripTarget)
				r.Post("/sessions/{sessionID}/strip/{targetID}/delete", webHandler.SessionRemoveStripTarget)
				r.Post("/sessions/{sessionID}/checklist/{itemID}/toggle", webHandler.SessionToggleChecklist)
				r.Post("/sessions/{sessionID}/delete", webHandler.SessionDelete)
				r.Get("/sessions/{sessionID}/complete", webHandler.SessionComplete)
				r.Get("/sessions/{sessionID}/photos", webHandler.SessionPhotos)
				r.Post("/sessions/{sessionID}/publish", webHandler.SessionPublish)
				r.Post("/sessions/{sessionID}/reopen", webHandler.SessionReopen)
				r.Get("/sessions/{sessionID}/route-fields", webHandler.SessionRouteFields)
				r.Post("/sessions/{sessionID}/routes", webHandler.SessionAddRoute)
				r.Post("/sessions/{sessionID}/routes/{routeID}/edit", webHandler.SessionEditRoute)
				r.Post("/sessions/{sessionID}/routes/{routeID}/delete", webHandler.SessionDeleteRoute)

				// Route card print batches — 8-up print-and-cut sheets.
				// Download and detail are behind the setter group (same as the
				// session-management tooling) because the batch picker exposes
				// the full route inventory and the PDF is intended for the
				// setter's print workflow.
				r.Get("/card-batches", webHandler.CardBatchList)
				r.Get("/card-batches/new", webHandler.CardBatchNewForm)
				r.With(batchCreateLimitByUser).Post("/card-batches/new", webHandler.CardBatchCreate)
				r.Get("/card-batches/{batchID}", webHandler.CardBatchDetail)
				r.Get("/card-batches/{batchID}/edit", webHandler.CardBatchEditForm)
				r.Post("/card-batches/{batchID}/edit", webHandler.CardBatchUpdate)
				r.Get("/card-batches/{batchID}/download.pdf", webHandler.CardBatchDownload)
				r.Get("/card-batches/{batchID}/cutlines.dxf", webHandler.CardBatchCutlines)
				r.Get("/card-batches/{batchID}/preview.png", webHandler.CardBatchPreview)
				r.With(batchCreateLimitByUser).Post("/card-batches/{batchID}/retry", webHandler.CardBatchRetry)
				r.Post("/card-batches/{batchID}/delete", webHandler.CardBatchDelete)

				// Gym settings — head_setter or above (handler checks role internally)
				r.Get("/settings", webHandler.GymSettingsPage)
				r.Post("/settings", webHandler.GymSettingsSave)
				r.Post("/settings/circuits/add", webHandler.GymSettingsAddCircuit)
				r.Post("/settings/circuits/{colorName}/delete", webHandler.GymSettingsRemoveCircuit)
				r.Post("/settings/hold-colors/add", webHandler.GymSettingsAddHoldColor)
				r.Post("/settings/hold-colors/{colorName}/delete", webHandler.GymSettingsRemoveHoldColor)
				r.Post("/settings/palette-preset", webHandler.GymSettingsApplyPalettePreset)
				r.Get("/settings/team", webHandler.TeamPage)
				r.Post("/settings/team/{membershipID}/role", webHandler.TeamUpdateRole)

				// Playbook editor — head_setter or above (handler checks role internally)
				r.Get("/settings/playbook", webHandler.PlaybookEditPage)
				r.Post("/settings/playbook/add", webHandler.PlaybookCreate)
				r.Post("/settings/playbook/{stepID}/edit", webHandler.PlaybookUpdate)
				r.Post("/settings/playbook/{stepID}/delete", webHandler.PlaybookDelete)
				r.Post("/settings/playbook/{stepID}/move", webHandler.PlaybookMove)

				// Progressions feature toggle — gym_manager or above (handler checks role internally)
				r.Post("/settings/progressions-toggle", webHandler.ProgressionsToggle)

				// Organization settings — gym_manager or above (handler checks role internally)
				r.Get("/settings/organization", webHandler.OrgSettingsPage)
				r.Post("/settings/organization", webHandler.OrgSettingsSave)
				r.Get("/settings/organization/gyms/new", webHandler.GymNewPage)
				r.Post("/settings/organization/gyms/new", webHandler.GymCreate)
				r.Get("/settings/organization/gyms/{gymID}/edit", webHandler.GymEditPage)
				r.Post("/settings/organization/gyms/{gymID}/edit", webHandler.GymUpdate)
				r.Get("/settings/organization/team", webHandler.OrgTeamPage)
				r.Post("/settings/organization/team/{membershipID}/role", webHandler.OrgTeamUpdateRole)

				// Progressions admin — head_setter or above (handler checks role internally)
				r.Get("/settings/progressions", webHandler.ProgressionsAdminPage)

				r.Get("/settings/progressions/domains/new", webHandler.DomainCreateForm)
				r.Post("/settings/progressions/domains/new", webHandler.DomainCreate)
				r.Get("/settings/progressions/domains/{domainID}/edit", webHandler.DomainEditForm)
				r.Post("/settings/progressions/domains/{domainID}/edit", webHandler.DomainUpdate)
				r.Post("/settings/progressions/domains/{domainID}/delete", webHandler.DomainDelete)

				r.Get("/settings/progressions/quests/new", webHandler.QuestCreateForm)
				r.Post("/settings/progressions/quests/new", webHandler.QuestCreate)
				r.Get("/settings/progressions/quests/{questID}/edit", webHandler.QuestEditForm)
				r.Post("/settings/progressions/quests/{questID}/edit", webHandler.QuestUpdate)
				r.Post("/settings/progressions/quests/{questID}/deactivate", webHandler.QuestDeactivate)
				r.Post("/settings/progressions/quests/{questID}/duplicate", webHandler.QuestDuplicate)

				r.Get("/settings/progressions/badges/new", webHandler.BadgeCreateForm)
				r.Post("/settings/progressions/badges/new", webHandler.BadgeCreate)
				r.Get("/settings/progressions/badges/{badgeID}/edit", webHandler.BadgeEditForm)
				r.Post("/settings/progressions/badges/{badgeID}/edit", webHandler.BadgeUpdate)
				r.Post("/settings/progressions/badges/{badgeID}/delete", webHandler.BadgeDelete)

			})

			// App admin routes — require is_app_admin flag on user
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireAppAdmin)

				adminDeps := &webhandler.AdminDeps{
					Metrics:  metrics,
					JobQueue: deps.JobQueue,
				}
				r.Get("/admin/health", webHandler.AdminHealthPage(adminDeps))
				r.Get("/admin/metrics", webHandler.AdminMetricsPage(adminDeps))
			})
		})
	})

	// API v1 — JSON API with restrictive CSP and query timeout.
	// 1 MB body cap: the API only accepts JSON payloads and never needs
	// multipart. Any future upload endpoint should mount under /web or
	// override this limit in its own handler. See S3 in the 2026-04-22
	// perf audit.
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.SecureHeaders)
		r.Use(middleware.RequestTimeout(cfg.QueryTimeout))
		r.Use(middleware.LimitBody(1 << 20)) // 1 MB
		// Public — rate-limited auth endpoints
		r.Group(func(r chi.Router) {
			r.Use(authLimiter.Limit)
			r.Post("/auth/register", authHandler.Register)
			r.Post("/auth/login", authHandler.Login)
			// Magic-link request: per-IP limit comes from authLimiter
			// above; per-email throttle (3 / 15 min) is enforced inside
			// the service against the DB so it survives process restarts
			// and isn't bypassed by hitting different API instances.
			r.Post("/auth/magic/request", magicAuthHandler.Request)
		})

		// Refresh token — accepts expired access tokens (signature still verified).
		// This must be outside the normal Authenticate middleware because the
		// whole point of refresh is that the access token has expired.
		r.Group(func(r chi.Router) {
			r.Use(authLimiter.Limit)
			r.Use(middleware.AuthenticateAllowExpired(cfg.JWTSecret, cfg.EnforceJWTAudience))
			r.Post("/auth/refresh", authHandler.Refresh)
		})

		// Authenticated — all routes below accept either a valid web
		// session cookie (SvelteKit SPA, same-origin) OR a valid JWT
		// (mobile / API clients). The dual-auth middleware tries cookie
		// first; mobile flows that send only Authorization continue to
		// work unchanged.
		r.Group(func(r chi.Router) {
			r.Use(middleware.AuthenticateCookieOrJWT(sessionMgr, cfg.JWTSecret, cfg.EnforceJWTAudience))

			// ── User's own data (no org context needed) ─────────────
			r.Get("/me", authHandler.Me)
			r.Patch("/me", authHandler.UpdateMe)
			r.Post("/me/password", authHandler.ChangePassword)
			r.Put("/me/view-as", authHandler.SetViewAs)
			r.Get("/me/ascents", ascentHandler.MyAscents)
			r.Patch("/me/ascents/{ascentID}", ascentHandler.UpdateMine)
			r.Delete("/me/ascents/{ascentID}", ascentHandler.DeleteMine)
			r.Get("/me/stats", ascentHandler.MyStats)
			r.Get("/me/quests", questHandler.MyQuests)
			r.Get("/me/notifications", notifHandler.List)
			r.Get("/me/notifications/unread-count", notifHandler.UnreadCount)
			r.Post("/me/notifications/{notifID}/read", notifHandler.MarkRead)
			r.Post("/me/notifications/read-all", notifHandler.MarkAllRead)
			r.Get("/me/feed", followHandler.Feed)
			r.Get("/me/labor", laborHandler.MyLabor)
			r.Get("/me/training-plans", trainingHandler.MyPlans)
			r.Get("/me/partner-profile", partnerHandler.MyProfile)

			// ── Organizations ───────────────────────────────────────
			// POST /orgs is removed from the public API.
			// Orgs are created via `routewerk-admin create-org`.
			r.Get("/orgs", orgHandler.List) // returns only orgs the user belongs to

			r.Route("/orgs/{orgID}", func(r chi.Router) {
				// All org routes require membership
				r.Use(authz.RequireOrgMember)

				r.Get("/", orgHandler.Get)

				// Org admin only
				r.Group(func(r chi.Router) {
					r.Use(authz.RequireOrgRole("org_admin"))
					r.Put("/", orgHandler.Update)
				})

				// Analytics overview — managers only
				r.Group(func(r chi.Router) {
					r.Use(authz.RequireOrgRole("org_admin", "head_setter"))
					r.Get("/analytics/overview", analyticsHandler.OrgOverview)
				})

				// Org-wide team list — org_admin only. Same response
				// shape as the location-scoped /locations/{id}/team so
				// the SPA can swap the source URL without rerendering.
				r.Group(func(r chi.Router) {
					r.Use(authz.RequireOrgRole("org_admin"))
					r.Get("/team", teamHandler.ListOrg)
				})

				// Locations (nested under org for creation)
				r.Route("/locations", func(r chi.Router) {
					r.Get("/", locationHandler.List) // any member can list

					// Creating locations requires org_admin
					r.Group(func(r chi.Router) {
						r.Use(authz.RequireOrgRole("org_admin"))
						r.Post("/", locationHandler.Create)
					})
				})

				// Tags (org-scoped)
				r.Route("/tags", func(r chi.Router) {
					r.Get("/", tagHandler.List) // any member can view tags

					// Managing tags requires setter or above
					r.Group(func(r chi.Router) {
						r.Use(authz.RequireOrgRole("setter"))
						r.Post("/", tagHandler.Create)
						r.Delete("/{tagID}", tagHandler.Delete)
					})
				})
			})

			// ── Locations (direct access) ───────────────────────────
			r.Route("/locations/{locationID}", func(r chi.Router) {
				// All location routes require membership in the owning org
				r.Use(authz.RequireLocationMember)

				r.Get("/", locationHandler.Get) // any member

				// Update location — org_admin only
				r.Group(func(r chi.Router) {
					r.Use(authz.RequireLocationRole("org_admin"))
					r.Put("/", locationHandler.Update)
				})

				// Progressions feature flag — gym_manager+ matches the
				// HTMX policy at internal/handler/web/settings.go::ProgressionsToggle.
				r.Group(func(r chi.Router) {
					r.Use(authz.RequireLocationRole("gym_manager"))
					r.Post("/progressions-toggle", locationHandler.SetProgressions)
				})

				// Walls — setter or above to manage, any member to view
				r.Route("/walls", func(r chi.Router) {
					r.Get("/", wallHandler.List)

					r.Group(func(r chi.Router) {
						r.Use(authz.RequireLocationRole("setter"))
						r.Post("/", wallHandler.Create)
					})

					r.Route("/{wallID}", func(r chi.Router) {
						r.Get("/", wallHandler.Get) // any member

						r.Group(func(r chi.Router) {
							r.Use(authz.RequireLocationRole("setter"))
							r.Put("/", wallHandler.Update)
						})
						r.Group(func(r chi.Router) {
							r.Use(authz.RequireLocationRole("head_setter"))
							r.Delete("/", wallHandler.Delete)
							r.Post("/archive", wallHandler.Archive)
							r.Post("/unarchive", wallHandler.Unarchive)
						})
					})
				})

				// Routes (climbs)
				r.Route("/routes", func(r chi.Router) {
					r.Get("/", routeHandler.List) // any member can browse routes

					// Creating/editing routes — setter or above
					r.Group(func(r chi.Router) {
						r.Use(authz.RequireLocationRole("setter"))
						r.Post("/", routeHandler.Create)
					})

					// Bulk archive — head_setter or above
					r.Group(func(r chi.Router) {
						r.Use(authz.RequireLocationRole("head_setter"))
						r.Post("/bulk-archive", routeHandler.BulkArchive)
					})

					r.Route("/{routeID}", func(r chi.Router) {
						r.Get("/", routeHandler.Get) // any member
						r.Get("/ascents", ascentHandler.RouteAscents)
						r.Get("/ratings", ratingHandler.RouteRatings)

						// Route info cards (any member)
						r.Get("/card/print.png", cardHandler.PrintPNG)
						r.Get("/card/print.pdf", cardHandler.PrintPDF)
						r.Get("/card/share.png", cardHandler.DigitalPNG)
						r.Get("/card/share.pdf", cardHandler.DigitalPDF)

						// Climber actions — any member can log ascents and rate
						r.Post("/ascent", ascentHandler.Log)
						r.Post("/rate", ratingHandler.Rate)

						// Route photos — any member can upload + list; delete is
						// scoped to setter+ or the photo's original uploader
						// (enforced in the handler). Mirrors the HTMX policy.
						r.Get("/photos", routePhotoHandler.List)
						r.Post("/photos", routePhotoHandler.Upload)
						r.Delete("/photos/{photoID}", routePhotoHandler.Delete)

						// Community tags — any member can vote / unvote their
						// own. The /tags/all moderate endpoint scrubs every
						// vote for a name; head_setter+ via the inline group.
						r.Get("/tags", routeTagHandler.List)
						r.Post("/tags", routeTagHandler.Add)
						r.Delete("/tags", routeTagHandler.Remove)
						r.Group(func(r chi.Router) {
							r.Use(authz.RequireLocationRole("head_setter"))
							r.Delete("/tags/all", routeTagHandler.Moderate)
						})

						// Difficulty consensus vote — easy / right / hard.
						// Any member with location access; one vote per
						// (user, route), upserted on resubmit.
						r.Get("/difficulty", routeDifficultyHandler.Get)
						r.Post("/difficulty", routeDifficultyHandler.Vote)

						// Edit route — setter or above
						r.Group(func(r chi.Router) {
							r.Use(authz.RequireLocationRole("setter"))
							r.Put("/", routeHandler.Update)
							r.Patch("/status", routeHandler.UpdateStatus)
						})
					})
				})

				// Card print batches — setter or above can manage. List and
				// download are shared across the setter team; there's no per-
				// creator ownership check because the print pipeline is
				// inherently collaborative (anyone on shift might re-run a
				// prior batch after adding a new route to it).
				r.Route("/card-batches", func(r chi.Router) {
					r.Use(authz.RequireLocationRole("setter"))

					r.Get("/", cardBatchHandler.List)
					r.With(batchCreateLimitByUser).Post("/", cardBatchHandler.Create)
					r.Get("/{batchID}", cardBatchHandler.Get)
					r.Get("/{batchID}/pdf", cardBatchHandler.Download)
					r.Patch("/{batchID}", cardBatchHandler.Update)
					r.Delete("/{batchID}", cardBatchHandler.Delete)
				})

				// Setting sessions — head_setter or above
				r.Route("/sessions", func(r chi.Router) {
					r.Use(authz.RequireLocationRole("setter"))

					r.Get("/", sessionHandler.List)
					r.Get("/{sessionID}", sessionHandler.Get)
					r.Get("/{sessionID}/routes", sessionHandler.ListRoutes)
					r.Get("/{sessionID}/strip-targets", sessionHandler.ListStripTargets)
					r.Get("/{sessionID}/checklist", sessionHandler.ListChecklist)
					// Setter+ can tick checklist items off (own assignments,
					// shared shop tasks, etc.). The HTMX side has the same
					// permission level.
					r.Post("/{sessionID}/checklist/{itemID}/toggle", sessionHandler.ToggleChecklistItem)

					r.Group(func(r chi.Router) {
						r.Use(authz.RequireLocationRole("head_setter"))
						r.Post("/", sessionHandler.Create)
						r.Put("/{sessionID}", sessionHandler.Update)
						r.Delete("/{sessionID}", sessionHandler.Delete)
						r.Post("/{sessionID}/status", sessionHandler.UpdateStatus)
						r.Post("/{sessionID}/assignments", sessionHandler.Assign)
						r.Delete("/{sessionID}/assignments/{assignmentID}", sessionHandler.Unassign)
						r.Post("/{sessionID}/strip-targets", sessionHandler.AddStripTarget)
						r.Delete("/{sessionID}/strip-targets/{targetID}", sessionHandler.RemoveStripTarget)
						// Combined publish: archives strip-targets, activates
						// drafts, flips status. Mirrors HTMX /sessions/{id}/publish.
						r.Post("/{sessionID}/publish", sessionHandler.Publish)
					})
				})

				// Setter labor
				r.Route("/labor", func(r chi.Router) {
					// Setters can log their own labor
					r.Group(func(r chi.Router) {
						r.Use(authz.RequireLocationRole("setter"))
						r.Post("/", laborHandler.Log)
					})
					// Viewing labor — head_setter or above
					r.Group(func(r chi.Router) {
						r.Use(authz.RequireLocationRole("head_setter"))
						r.Get("/", laborHandler.ListByLocation)
					})
				})

				// Training plans — setter or above can create/manage
				r.Route("/training-plans", func(r chi.Router) {
					r.Use(authz.RequireLocationRole("setter"))

					r.Post("/", trainingHandler.Create)
					r.Get("/", trainingHandler.List)
					r.Route("/{planID}", func(r chi.Router) {
						r.Get("/", trainingHandler.Get)
						r.Put("/", trainingHandler.Update)
						r.Post("/items", trainingHandler.AddItem)
						r.Patch("/items/{itemID}", trainingHandler.UpdateItem)
					})
				})

				// Partner matching — any member
				r.Route("/partners", func(r chi.Router) {
					r.Get("/", partnerHandler.Search)
					r.Put("/profile", partnerHandler.UpdateProfile)
				})

				// Analytics — managers only (org_admin, head_setter)
				r.Route("/analytics", func(r chi.Router) {
					r.Use(authz.RequireLocationRole("head_setter"))

					r.Get("/grade-distribution", analyticsHandler.GradeDistribution)
					r.Get("/route-lifecycle", analyticsHandler.RouteLifecycle)
					r.Get("/engagement", analyticsHandler.Engagement)
					r.Get("/setter-productivity", analyticsHandler.SetterProductivity)
				})

				// Competitions — list any member; create head_setter+.
				// Read/update by id live below at /competitions/{id}.
				r.Route("/competitions", func(r chi.Router) {
					r.Get("/", compHandler.List)

					r.Group(func(r chi.Router) {
						r.Use(authz.RequireLocationRole("head_setter"))
						r.Post("/", compHandler.Create)
					})
				})

				// Team list — head_setter+. Returns memberships at this
				// location (or org-wide) joined with user info for the SPA
				// /app/team page (Phase 2.7).
				r.Route("/team", func(r chi.Router) {
					r.Use(authz.RequireLocationRole("head_setter"))
					r.Get("/", teamHandler.List)
				})

				// Quests catalog — Phase 2.8. Any location member can browse
				// the active quests at this location.
				r.Get("/quests", questHandler.ListAvailable)

				// Climber badge showcase — catalog + caller's earned set.
				// Any member can read; gated by location membership above.
				r.Get("/badges/showcase", badgeShowcaseHandler.Get)

				// Location-wide activity feed. Any member can read.
				r.Get("/activity", activityHandler.List)

				// Setter dashboard summary (stats + recent activity). Setter+
				// only because the HTMX /dashboard requires the same.
				r.Group(func(r chi.Router) {
					r.Use(authz.RequireLocationRole("setter"))
					r.Get("/dashboard", dashboardHandler.Stats)
				})

				// Location settings (circuits, hold-colors, grading, display,
				// session defaults). Read for setter+ so the SPA route form
				// can restrict grading systems / color pickers to what this
				// gym actually stocks; write for head_setter+ to match the
				// HTMX gym-settings policy at internal/handler/web/settings.go.
				r.Group(func(r chi.Router) {
					r.Use(authz.RequireLocationRole("setter"))
					r.Get("/settings", settingsHandler.GetLocationSettings)
					// Static catalog of named palette presets the SPA can
					// render as one-click apply buttons. Same role gate
					// (setter+) since the data is location-agnostic but
					// the only callers are the gym-settings UI.
					r.Get("/settings/palette-presets", settingsHandler.ListPalettePresets)
					// Playbook list — setter+ since the SPA's session
					// lifecycle UI peeks at this to know which steps to
					// pre-populate. Writes are gated below.
					r.Get("/playbook", playbookHandler.List)
				})
				r.Group(func(r chi.Router) {
					r.Use(authz.RequireLocationRole("head_setter"))
					r.Put("/settings", settingsHandler.UpdateLocationSettings)
					// Apply a named palette preset — head_setter+ matches the
					// HTMX flow. Replaces circuits + hold-color lists in one shot.
					r.Post("/settings/palette-preset", settingsHandler.ApplyPalettePreset)
					// Playbook writes — head_setter+ matches HTMX policy
					// (internal/handler/web/sessions_lifecycle.go::PlaybookEdit*).
					r.Post("/playbook", playbookHandler.Create)
					r.Patch("/playbook/{stepID}", playbookHandler.Update)
					r.Delete("/playbook/{stepID}", playbookHandler.Delete)
					r.Post("/playbook/{stepID}/move", playbookHandler.Move)
				})

				// Progressions admin — quest / badge / domain CRUD for
				// head_setter+. Mirrors HTMX /settings/progressions.
				r.Group(func(r chi.Router) {
					r.Use(authz.RequireLocationRole("head_setter"))
					r.Get("/admin/quests", progressionsAdminHandler.ListAllQuests)
					r.Post("/admin/quests", progressionsAdminHandler.CreateQuest)
					r.Post("/admin/quests/{questID}/deactivate", progressionsAdminHandler.DeactivateQuest)
					r.Post("/admin/quests/{questID}/duplicate", progressionsAdminHandler.DuplicateQuest)

					r.Get("/admin/quest-domains", progressionsAdminHandler.ListDomains)
					r.Post("/admin/quest-domains", progressionsAdminHandler.CreateDomain)
					r.Delete("/admin/quest-domains/{domainID}", progressionsAdminHandler.DeleteDomain)

					r.Get("/admin/badges", progressionsAdminHandler.ListBadges)
					r.Post("/admin/badges", progressionsAdminHandler.CreateBadge)
				})
			})

			// Quest / domain / badge updates by id (no location prefix).
			// Authorization is enforced by the handler via the location_id
			// stored on each row, not the URL.
			r.Put("/quests/{questID}", progressionsAdminHandler.UpdateQuest)
			r.Put("/quest-domains/{domainID}", progressionsAdminHandler.UpdateDomain)
			r.Put("/badges/{badgeID}", progressionsAdminHandler.UpdateBadge)
			r.Delete("/badges/{badgeID}", progressionsAdminHandler.DeleteBadge)

			// Membership writes (Phase 2.7) — no location prefix because the
			// caller might not be acting "at" the membership's location
			// directly. The handler resolves the membership's org and looks
			// up the caller's effective role within that org before allowing
			// the write. gym_manager+ minimum.
			r.Patch("/memberships/{membershipID}", teamHandler.UpdateMembership)
			r.Delete("/memberships/{membershipID}", teamHandler.RemoveMembership)

			// Quest enrollment + log entries (Phase 2.8). Each handler
			// verifies the caller owns the enrollment before mutating.
			r.Get("/quests/{questID}", questHandler.Get)
			r.Post("/quests/{questID}/start", questHandler.Start)
			r.Post("/climber-quests/{climberQuestID}/log", questHandler.LogProgress)
			r.Delete("/climber-quests/{climberQuestID}", questHandler.Abandon)

			// ── Competitions by id (no location prefix) ─────────────
			// Read is open to any authenticated user; the leaderboard
			// visibility check (Phase 1f wave 4) gates downstream
			// resources. Write authz happens inside the handler via the
			// shared requireCompRole helper since the {locationID} chi
			// param isn't on these URLs.
			//
			// /competitions/by-slug/{slug} MUST register before the
			// {id} routes — chi matches in registration order and
			// "by-slug" would otherwise be caught by the {id} pattern.
			r.Get("/competitions/by-slug/{slug}", compHandler.GetBySlug)

			// Everything under /competitions/{id} lives in one sub-router.
			// Chi can't have both `r.Get("/competitions/{id}", h)` AND
			// `r.Route("/competitions/{id}", ...)` registered at the same
			// node — the sub-router takes over the subtree and the bare
			// GET 404s. Put the bare GET/PATCH inside the sub-router as
			// "/" instead. (Reproduced 2026-05-06: comp detail page 404'd
			// while child resources like /events and /categories worked.)
			r.Route("/competitions/{id}", func(r chi.Router) {
				r.Get("/", compHandler.Get)
				r.Patch("/", compHandler.Update)
				r.Get("/events", compHandler.ListEvents)
				r.Post("/events", compHandler.CreateEvent)
				r.Get("/categories", compHandler.ListCategories)
				r.Post("/categories", compHandler.CreateCategory)

				// Phase 1f wave 3: registrations + the unified action endpoint.
				r.Get("/registrations", compHandler.ListRegistrations)
				r.Post("/registrations", compHandler.CreateRegistration)
				r.Post("/actions", compHandler.SubmitActions)
			})
			r.Route("/events/{id}", func(r chi.Router) {
				r.Patch("/", compHandler.UpdateEvent)
				r.Get("/problems", compHandler.ListProblems)
				r.Post("/problems", compHandler.CreateProblem)
			})
			r.Patch("/problems/{id}", compHandler.UpdateProblem)
			r.Delete("/registrations/{id}", compHandler.WithdrawRegistration)
			r.Get("/registrations/{id}/attempts", compHandler.ListRegistrationAttempts)

			// Phase 1f wave 4: staff verify/override + leaderboard read.
			// Verify/override authz happens inside the handler since the
			// {locationID} chi param isn't on these URLs.
			r.Post("/attempts/{id}/verify", compHandler.VerifyAttempt)
			r.Post("/attempts/{id}/override", compHandler.OverrideAttempt)
			r.Get("/competitions/{id}/leaderboard", compHandler.GetLeaderboard)
			r.Get("/competitions/{id}/leaderboard/stream", compHandler.StreamLeaderboard)

			// ── Social (no org context) ─────────────────────────────
			r.Route("/users/{userID}", func(r chi.Router) {
				r.Post("/follow", followHandler.Follow)
				r.Delete("/follow", followHandler.Unfollow)
				r.Get("/followers", followHandler.Followers)
				r.Get("/following", followHandler.Following)
			})
		})
	})

	return r
}
