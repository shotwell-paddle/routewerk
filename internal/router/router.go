package router

import (
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shotwell-paddle/routewerk/internal/config"
	"github.com/shotwell-paddle/routewerk/internal/handler"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

func New(cfg *config.Config, db *pgxpool.Pool) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(middleware.SecureHeaders)
	r.Use(middleware.Logger)
	r.Use(chimw.Recoverer)

	// CORS
	allowedOrigins := []string{"http://localhost:3000", "http://localhost:8080"}
	if cfg.IsDev() {
		allowedOrigins = []string{"*"}
	}
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: !cfg.IsDev(),
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

	// Audit
	auditRepo := repository.NewAuditRepo(db)
	auditService := service.NewAuditService(auditRepo)

	// Services
	authService := service.NewAuthService(userRepo, loginAttemptRepo, cfg)
	cardGen := service.NewCardGenerator(cfg.FrontendURL)

	// Handlers
	healthHandler := handler.NewHealthHandler(db)
	authHandler := handler.NewAuthHandler(authService)
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

	// Health check
	r.Get("/health", healthHandler.Check)

	// API v1
	r.Route("/api/v1", func(r chi.Router) {
		// Public — rate-limited auth endpoints
		r.Group(func(r chi.Router) {
			r.Use(authLimiter.Limit)
			r.Post("/auth/register", authHandler.Register)
			r.Post("/auth/login", authHandler.Login)
		})

		// Refresh token — accepts expired access tokens (signature still verified).
		// This must be outside the normal Authenticate middleware because the
		// whole point of refresh is that the access token has expired.
		r.Group(func(r chi.Router) {
			r.Use(authLimiter.Limit)
			r.Use(middleware.AuthenticateAllowExpired(cfg.JWTSecret))
			r.Post("/auth/refresh", authHandler.Refresh)
		})

		// Authenticated — all routes below require a valid (non-expired) JWT
		r.Group(func(r chi.Router) {
			r.Use(middleware.Authenticate(cfg.JWTSecret))

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
						r.Get("/", routeHandler.Get)       // any member
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
