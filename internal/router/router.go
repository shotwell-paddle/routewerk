package router

import (
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shotwell-paddle/routewerk/internal/config"
	"github.com/shotwell-paddle/routewerk/internal/handler"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
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

	// Health check (unauthenticated)
	health := handler.NewHealthHandler(db)
	r.Get("/health", health.Check)

	// API v1
	r.Route("/api/v1", func(r chi.Router) {
		// --- Public routes ---
		// r.Post("/auth/register", authHandler.Register)
		// r.Post("/auth/login", authHandler.Login)
		// r.Post("/auth/refresh", authHandler.Refresh)

		// --- Authenticated routes ---
		r.Group(func(r chi.Router) {
			r.Use(middleware.Authenticate(cfg.JWTSecret))

			// Organizations
			// r.Route("/orgs", func(r chi.Router) {
			// 	r.Post("/", orgHandler.Create)
			// 	r.Get("/", orgHandler.List)
			// 	r.Route("/{orgID}", func(r chi.Router) {
			// 		r.Get("/", orgHandler.Get)
			// 		r.Put("/", orgHandler.Update)
			// 	})
			// })

			// Locations
			// r.Route("/orgs/{orgID}/locations", func(r chi.Router) {
			// 	r.Post("/", locationHandler.Create)
			// 	r.Get("/", locationHandler.List)
			// 	r.Route("/{locationID}", func(r chi.Router) {
			// 		r.Get("/", locationHandler.Get)
			// 		r.Put("/", locationHandler.Update)
			// 	})
			// })

			// Walls
			// r.Route("/locations/{locationID}/walls", func(r chi.Router) {
			// 	r.Post("/", wallHandler.Create)
			// 	r.Get("/", wallHandler.List)
			// 	r.Route("/{wallID}", func(r chi.Router) {
			// 		r.Get("/", wallHandler.Get)
			// 		r.Put("/", wallHandler.Update)
			// 		r.Delete("/", wallHandler.Delete)
			// 	})
			// })

			// Routes (climbs)
			// r.Route("/locations/{locationID}/routes", func(r chi.Router) {
			// 	r.Post("/", routeHandler.Create)
			// 	r.Get("/", routeHandler.List)
			// 	r.Route("/{routeID}", func(r chi.Router) {
			// 		r.Get("/", routeHandler.Get)
			// 		r.Put("/", routeHandler.Update)
			// 		r.Patch("/status", routeHandler.UpdateStatus)
			// 		r.Post("/rate", ratingHandler.Rate)
			// 		r.Post("/ascent", ascentHandler.Log)
			// 	})
			// })

			// Bulk operations
			// r.Post("/locations/{locationID}/routes/bulk-archive", routeHandler.BulkArchive)
			// r.Patch("/locations/{locationID}/routes/bulk-grade", routeHandler.BulkGradeUpdate)

			// Setting sessions
			// r.Route("/locations/{locationID}/sessions", func(r chi.Router) {
			// 	r.Post("/", sessionHandler.Create)
			// 	r.Get("/", sessionHandler.List)
			// 	r.Route("/{sessionID}", func(r chi.Router) {
			// 		r.Get("/", sessionHandler.Get)
			// 		r.Put("/", sessionHandler.Update)
			// 		r.Post("/assignments", sessionHandler.Assign)
			// 	})
			// })

			// Setter labor
			// r.Route("/locations/{locationID}/labor", func(r chi.Router) {
			// 	r.Post("/", laborHandler.Log)
			// 	r.Get("/", laborHandler.List)
			// })

			// Tags
			// r.Route("/orgs/{orgID}/tags", func(r chi.Router) {
			// 	r.Post("/", tagHandler.Create)
			// 	r.Get("/", tagHandler.List)
			// 	r.Delete("/{tagID}", tagHandler.Delete)
			// })

			// Climber profile & activity
			// r.Get("/me", userHandler.Me)
			// r.Put("/me", userHandler.UpdateProfile)
			// r.Get("/me/ascents", ascentHandler.MyAscents)
			// r.Get("/me/stats", ascentHandler.MyStats)
			// r.Get("/me/feed", feedHandler.ActivityFeed)

			// Follows
			// r.Post("/users/{userID}/follow", followHandler.Follow)
			// r.Delete("/users/{userID}/follow", followHandler.Unfollow)
			// r.Get("/users/{userID}/followers", followHandler.Followers)
			// r.Get("/users/{userID}/following", followHandler.Following)

			// Training / coaching
			// r.Route("/locations/{locationID}/training-plans", func(r chi.Router) {
			// 	r.Post("/", trainingHandler.Create)
			// 	r.Get("/", trainingHandler.List)
			// 	r.Route("/{planID}", func(r chi.Router) {
			// 		r.Get("/", trainingHandler.Get)
			// 		r.Put("/", trainingHandler.Update)
			// 		r.Post("/items", trainingHandler.AddItem)
			// 		r.Patch("/items/{itemID}", trainingHandler.UpdateItem)
			// 	})
			// })

			// Partner matching
			// r.Route("/locations/{locationID}/partners", func(r chi.Router) {
			// 	r.Get("/", partnerHandler.Search)
			// 	r.Put("/profile", partnerHandler.UpdateProfile)
			// })

			// Analytics (manager/head setter)
			// r.Route("/locations/{locationID}/analytics", func(r chi.Router) {
			// 	r.Get("/grade-distribution", analyticsHandler.GradeDistribution)
			// 	r.Get("/route-lifecycle", analyticsHandler.RouteLifecycle)
			// 	r.Get("/engagement", analyticsHandler.Engagement)
			// 	r.Get("/setter-productivity", analyticsHandler.SetterProductivity)
			// })

			// Org-level analytics
			// r.Get("/orgs/{orgID}/analytics/overview", analyticsHandler.OrgOverview)

			// QR code generation
			// r.Get("/routes/{routeID}/qr", qrHandler.Generate)
			// r.Get("/routes/{routeID}/card", qrHandler.RouteCard)
		})
	})

	return r
}
