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

	// CORS: AllowedOrigins "*" with AllowCredentials true violates the spec and
	// leaks cookies to any origin. In dev we allow all origins without credentials;
	// in production, set CORS_ORIGINS to the real Flutter/web app domains.
	allowedOrigins := []string{"http://localhost:3000", "http://localhost:8080"}
	if cfg.IsDev() {
		allowedOrigins = []string{"*"}
	}
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: !cfg.IsDev(), // credentials only when origins are explicit
		MaxAge:           300,
	}))

	// Rate limiter for auth endpoints: 20 requests per minute per IP
	authLimiter := middleware.NewRateLimiter(20, 1*time.Minute)

	// Repositories
	userRepo := repository.NewUserRepo(db)
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

	// Services
	authService := service.NewAuthService(userRepo, cfg)

	// Handlers
	healthHandler := handler.NewHealthHandler(db)
	authHandler := handler.NewAuthHandler(authService)
	orgHandler := handler.NewOrgHandler(orgRepo)
	locationHandler := handler.NewLocationHandler(locationRepo)
	wallHandler := handler.NewWallHandler(wallRepo)
	routeHandler := handler.NewRouteHandler(routeRepo)
	ascentHandler := handler.NewAscentHandler(ascentRepo)
	ratingHandler := handler.NewRatingHandler(ratingRepo)
	sessionHandler := handler.NewSessionHandler(sessionRepo)
	laborHandler := handler.NewLaborHandler(laborRepo)
	tagHandler := handler.NewTagHandler(tagRepo)
	followHandler := handler.NewFollowHandler(followRepo)
	trainingHandler := handler.NewTrainingHandler(trainingRepo)
	partnerHandler := handler.NewPartnerHandler(partnerRepo)
	analyticsHandler := handler.NewAnalyticsHandler(analyticsRepo)

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

		// Authenticated
		r.Group(func(r chi.Router) {
			r.Use(middleware.Authenticate(cfg.JWTSecret))

			// Auth & profile
			r.Post("/auth/refresh", authHandler.Refresh)
			r.Get("/me", authHandler.Me)
			r.Get("/me/ascents", ascentHandler.MyAscents)
			r.Get("/me/stats", ascentHandler.MyStats)
			r.Get("/me/feed", followHandler.Feed)
			r.Get("/me/labor", laborHandler.MyLabor)
			r.Get("/me/training-plans", trainingHandler.MyPlans)
			r.Get("/me/partner-profile", partnerHandler.MyProfile)

			// Organizations
			r.Route("/orgs", func(r chi.Router) {
				r.Post("/", orgHandler.Create)
				r.Get("/", orgHandler.List)
				r.Route("/{orgID}", func(r chi.Router) {
					r.Get("/", orgHandler.Get)
					r.Put("/", orgHandler.Update)
					r.Get("/analytics/overview", analyticsHandler.OrgOverview)

					// Locations (nested under org for creation)
					r.Route("/locations", func(r chi.Router) {
						r.Post("/", locationHandler.Create)
						r.Get("/", locationHandler.List)
					})

					// Tags (org-scoped)
					r.Route("/tags", func(r chi.Router) {
						r.Post("/", tagHandler.Create)
						r.Get("/", tagHandler.List)
						r.Delete("/{tagID}", tagHandler.Delete)
					})
				})
			})

			// Locations (direct access)
			r.Route("/locations/{locationID}", func(r chi.Router) {
				r.Get("/", locationHandler.Get)
				r.Put("/", locationHandler.Update)

				// Walls
				r.Route("/walls", func(r chi.Router) {
					r.Post("/", wallHandler.Create)
					r.Get("/", wallHandler.List)
					r.Route("/{wallID}", func(r chi.Router) {
						r.Get("/", wallHandler.Get)
						r.Put("/", wallHandler.Update)
						r.Delete("/", wallHandler.Delete)
					})
				})

				// Routes (climbs)
				r.Route("/routes", func(r chi.Router) {
					r.Post("/", routeHandler.Create)
					r.Get("/", routeHandler.List)
					r.Post("/bulk-archive", routeHandler.BulkArchive)
					r.Route("/{routeID}", func(r chi.Router) {
						r.Get("/", routeHandler.Get)
						r.Put("/", routeHandler.Update)
						r.Patch("/status", routeHandler.UpdateStatus)
						r.Post("/ascent", ascentHandler.Log)
						r.Get("/ascents", ascentHandler.RouteAscents)
						r.Post("/rate", ratingHandler.Rate)
						r.Get("/ratings", ratingHandler.RouteRatings)
					})
				})

				// Setting sessions
				r.Route("/sessions", func(r chi.Router) {
					r.Post("/", sessionHandler.Create)
					r.Get("/", sessionHandler.List)
					r.Route("/{sessionID}", func(r chi.Router) {
						r.Get("/", sessionHandler.Get)
						r.Put("/", sessionHandler.Update)
						r.Post("/assignments", sessionHandler.Assign)
					})
				})

				// Setter labor
				r.Route("/labor", func(r chi.Router) {
					r.Post("/", laborHandler.Log)
					r.Get("/", laborHandler.ListByLocation)
				})

				// Training plans
				r.Route("/training-plans", func(r chi.Router) {
					r.Post("/", trainingHandler.Create)
					r.Get("/", trainingHandler.List)
					r.Route("/{planID}", func(r chi.Router) {
						r.Get("/", trainingHandler.Get)
						r.Put("/", trainingHandler.Update)
						r.Post("/items", trainingHandler.AddItem)
						r.Patch("/items/{itemID}", trainingHandler.UpdateItem)
					})
				})

				// Partner matching
				r.Route("/partners", func(r chi.Router) {
					r.Get("/", partnerHandler.Search)
					r.Put("/profile", partnerHandler.UpdateProfile)
				})

				// Analytics
				r.Route("/analytics", func(r chi.Router) {
					r.Get("/grade-distribution", analyticsHandler.GradeDistribution)
					r.Get("/route-lifecycle", analyticsHandler.RouteLifecycle)
					r.Get("/engagement", analyticsHandler.Engagement)
					r.Get("/setter-productivity", analyticsHandler.SetterProductivity)
				})
			})

			// Social
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
