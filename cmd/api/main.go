package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/config"
	"github.com/shotwell-paddle/routewerk/internal/database"
	"github.com/shotwell-paddle/routewerk/internal/router"
)

func main() {
	cfg := config.Load()

	// Validate critical config in production
	if !cfg.IsDev() {
		if cfg.JWTSecret == "change-me" || len(cfg.JWTSecret) < 32 {
			log.Fatal("FATAL: JWT_SECRET must be set to a strong value (>=32 chars) in production")
		}
		if cfg.DatabaseURL == "postgres://routewerk:password@localhost:5432/routewerk?sslmode=disable" {
			log.Fatal("FATAL: DATABASE_URL must be set in production")
		}
	}

	// Connect to database
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()
	log.Println("connected to database")

	// Build router
	r := router.New(cfg, db)

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
		log.Println("shutting down server...")

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("forced shutdown: %v", err)
		}
	}()

	log.Printf("routewerk api listening on %s (env=%s)", addr, cfg.Env)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
