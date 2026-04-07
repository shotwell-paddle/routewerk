package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/config"
	"github.com/shotwell-paddle/routewerk/internal/database"
	"github.com/shotwell-paddle/routewerk/internal/event"
	"github.com/shotwell-paddle/routewerk/internal/jobs"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/router"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

func main() {
	cfg := config.Load()

	// Structured logging: JSON in production, human-readable in dev
	initLogger(cfg.IsDev())

	// Validate config — fails fast in production if secrets or URLs are missing
	if problems := cfg.Validate(); len(problems) > 0 {
		for _, p := range problems {
			slog.Error("config problem", "issue", p)
		}
		log.Fatal("FATAL: fix configuration errors before starting in production")
	}
	slog.Debug("config loaded", "summary", cfg.String())

	// Run pending migrations before opening the connection pool.
	// This ensures the schema is always up to date on startup.
	slog.Info("running database migrations")
	if err := database.Migrate(cfg.DatabaseURL); err != nil {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}

	// Connect to database
	db, err := database.Connect(cfg.DatabaseURL, cfg.IsDev(), database.PoolConfig{
		MaxConns:          cfg.DBMaxConns,
		MinConns:          cfg.DBMinConns,
		MaxConnLifetime:   cfg.DBMaxConnLifetime,
		MaxConnIdleTime:   cfg.DBMaxConnIdleTime,
		HealthCheckPeriod: cfg.DBHealthCheckPeriod,
	})
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	slog.Info("connected to database")

	// Background housekeeping — clean up expired web sessions every hour
	go cleanupExpiredSessions(db)

	// Job queue — lightweight Postgres-backed async processing
	jobQueue := jobs.NewQueue(db)

	// Email service — sends transactional emails via SMTP (logs in dev)
	emailSvc := service.NewEmailService(service.EmailConfig{
		Host:     cfg.SMTPHost,
		Port:     cfg.SMTPPort,
		Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword,
		From:     cfg.SMTPFrom,
	}, cfg.FrontendURL)
	emailSvc.RegisterHandlers(jobQueue)

	// Notification service — in-app notifications backed by job queue
	notifRepo := repository.NewNotificationRepo(db)
	notifSvc := service.NewNotificationService(notifRepo, jobQueue)
	notifSvc.RegisterHandlers(jobQueue)

	// Event bus — in-memory pub/sub for progressions and future features
	bus := event.NewMemoryBus(slog.Default())

	// Progressions services
	questRepo := repository.NewQuestRepo(db)
	badgeRepo := repository.NewBadgeRepo(db)
	activityRepo := repository.NewActivityRepo(db)
	questSvc := service.NewQuestService(questRepo, badgeRepo, bus)

	// Register event listeners (badge awards, activity feed, notifications)
	questListeners := service.NewQuestListeners(badgeRepo, questRepo, activityRepo, notifSvc, bus)
	questListeners.Register()

	// Start job queue worker
	stopJobs := jobQueue.Start(context.Background())
	defer stopJobs()

	// Build router
	r := router.New(cfg, db, &router.Deps{
		JobQueue: jobQueue,
		EventBus: bus,
		NotifSvc: notifSvc,
		QuestSvc: questSvc,
	})

	// Start server with timeouts to prevent slowloris and resource exhaustion
	addr := fmt.Sprintf(":%s", cfg.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
	}

	// Graceful shutdown — drain existing connections before exiting
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("shutting down server")

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("forced shutdown", "error", err)
		}
		// Drain in-flight async event handlers
		if err := bus.Shutdown(ctx); err != nil {
			slog.Error("event bus shutdown timeout", "error", err)
		}
	}()

	slog.Info("routewerk api starting", "addr", addr, "env", cfg.Env)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

// cleanupExpiredSessions runs every hour to delete expired web sessions.
func cleanupExpiredSessions(db *pgxpool.Pool) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		tag, err := db.Exec(ctx, `DELETE FROM web_sessions WHERE expires_at <= NOW()`)
		cancel()
		if err != nil {
			slog.Error("session cleanup failed", "error", err)
			continue
		}
		if n := tag.RowsAffected(); n > 0 {
			slog.Info("cleaned up expired sessions", "count", n)
		}
	}
}

func initLogger(isDev bool) {
	var handler slog.Handler
	if isDev {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	} else {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	}
	slog.SetDefault(slog.New(handler))
}
