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

	// Web session manager (cookie-based auth for HTMX frontend)
	sessionMgr := middleware.NewSessionManager(webSessionRepo, userRepo, cfg.IsDev())

	// Handlers
	storageSvc := service.NewStorageService(cfg)
	healthHandler := handler.NewHealthHandler(db, storageSvc)
	authHandler := handler.NewAuthHandler(authService)
	magicAuthHandler := handler.NewMagicAuthHandler(magicLinkSvc)
	magicVerifyHandler := webhandler.NewMagicVerifyHandler(magicLinkSvc, webSessionRepo, userRepo, sessionMgr, cfg)
	orgHandler := handler.NewOrgHandler(orgRepo, auditService)
	locationHandler := handler.NewLocationHandler(locationRepo)
	wallHandler := handler.NewWallHandler(wallRepo, auditService)
	routeHandler := handler.NewRouteHandler(routeRepo, auditService)
	ascentHandler := handler.NewAscentHandler(ascentRepo)
	ratingHandler := handler.NewRatingHandler(ratingRepo)
	sessionHandler := handler.NewSessionHandler(sessionRepo)
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
	// index.html and the client router takes over. Phase 0 only mounts a
	// smoke-test prefix; Phase 1 will add /comp/* and /staff/comp/*.
	r.Group(func(r chi.Router) {
		r.Use(middleware.Gzip)
		r.Get("/favicon.svg", func(w http.ResponseWriter, req *http.Request) {
			spa.AssetServer().ServeHTTP(w, req)
		})
		// Mount fallback at both /spa-test and /spa-test/* so SvelteKit's
		// trailingSlash='never' default (URL ends up as /spa-test) doesn't
		// 404 on reload. The two registrations point at the same handler.
		r.Handle("/spa-test", spa.FallbackHandler())
		r.Handle("/spa-test/*", spa.FallbackHandler())
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

		// Authenticated — all routes below require a valid (non-expired) JWT
		r.Group(func(r chi.Router) {
			r.Use(middleware.Authenticate(cfg.JWTSecret, cfg.EnforceJWTAudience))

			// ── User's own data (no org context needed) ─────────────
			r.Get("/me", authHandler.Me)
			r.Get("/me/ascents", ascentHandler.MyAscents)
			r.Get("/me/stats", ascentHandler.MyStats)
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
					r.Delete("/{batchID}", cardBatchHandler.Delete)
				})

				// Setting sessions — head_setter or above
				r.Route("/sessions", func(r chi.Router) {
					r.Use(authz.RequireLocationRole("setter"))

					r.Get("/", sessionHandler.List)
					r.Get("/{sessionID}", sessionHandler.Get)

					r.Group(func(r chi.Router) {
						r.Use(authz.RequireLocationRole("head_setter"))
						r.Post("/", sessionHandler.Create)
						r.Put("/{sessionID}", sessionHandler.Update)
						r.Post("/{sessionID}/assignments", sessionHandler.Assign)
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
			})

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
