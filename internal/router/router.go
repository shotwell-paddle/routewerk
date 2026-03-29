package router

import (
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
	r.Use(middleware.Logger)
	r.Use(chimw.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"}, // tighten in production
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Repositories
	userRepo := repository.NewUserRepo(db)
	orgRepo := repository.NewOrgRepo(db)
	locationRepo := repository.NewLocationRepo(db)
	wallRepo := repository.NewWallRepo(db)
	routeRepo := repository.NewRouteRepo(db)

	// Services
	authService := service.NewAuthService(userRepo, cfg)

	// Handlers
	healthHandler := handler.NewHealthHandler(db)
	authHandler := handler.NewAuthHandler(authService)
	orgHandler := handler.NewOrgHandler(orgRepo)
	locationHandler := handler.NewLocationHandler(locationRepo)
	wallHandler := handler.NewWallHandler(wallRepo)
	routeHandler := handler.NewRouteHandler(routeRepo)

	// Health check (unauthenticated)
	r.Get("/health", healthHandler.Check)

	// API v1
	r.Route("/api/v1", func(r chi.Router) {
		// --- Public routes ---
		r.Post("/auth/register", authHandler.Register)
		r.Post("/auth/login", authHandler.Login)

		// --- Authenticated routes ---
		r.Group(func(r chi.Router) {
			r.Use(middleware.Authenticate(cfg.JWTSecret))

			// Auth
			r.Post("/auth/refresh", authHandler.Refresh)
			r.Get("/me", authHandler.Me)

			// Organizations
			r.Route("/orgs", func(r chi.Router) {
				r.Post("/", orgHandler.Create)
				r.Get("/", orgHandler.List)
				r.Route("/{orgID}", func(r chi.Router) {
					r.Get("/", orgHandler.Get)
					r.Put("/", orgHandler.Update)

					// Locations (nested under org)
					r.Route("/locations", func(r chi.Router) {
						r.Post("/", locationHandler.Create)
						r.Get("/", locationHandler.List)
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
					})
				})
			})

			// Future endpoints (stubbed in comments for reference):
			// Setting sessions: POST/GET /locations/{id}/sessions
			// Setter labor:     POST/GET /locations/{id}/labor
			// Tags:             POST/GET/DELETE /orgs/{id}/tags
			// Ascents:          POST /locations/{id}/routes/{id}/ascent
			// Ratings:          POST /locations/{id}/routes/{id}/rate
			// Follows:          POST/DELETE /users/{id}/follow
			// Training plans:   POST/GET /locations/{id}/training-plans
			// Partner matching: GET/PUT /locations/{id}/partners
			// Analytics:        GET /locations/{id}/analytics/*
			// QR codes:         GET /routes/{id}/qr, /routes/{id}/card
		})
	})

	return r
}
