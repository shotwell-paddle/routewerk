package config

import (
	"os"
	"time"
)

type Config struct {
	Port                string
	Env                 string
	DatabaseURL         string
	JWTSecret           string
	JWTExpiry           time.Duration
	RefreshTokenExpiry  time.Duration
	StorageEndpoint     string
	StorageBucket       string
	StorageAccessKey    string
	StorageSecretKey    string
	FCMProjectID        string
	FCMCredentialsFile  string
	FrontendURL         string
}

func Load() *Config {
	jwtExpiry, _ := time.ParseDuration(getEnv("JWT_EXPIRY", "15m"))
	refreshExpiry, _ := time.ParseDuration(getEnv("REFRESH_TOKEN_EXPIRY", "720h"))

	return &Config{
		Port:               getEnv("PORT", "8080"),
		Env:                getEnv("ENV", "development"),
		DatabaseURL:        getEnv("DATABASE_URL", "postgres://routewerk:password@localhost:5432/routewerk?sslmode=disable"),
		JWTSecret:          getEnv("JWT_SECRET", "change-me"),
		JWTExpiry:          jwtExpiry,
		RefreshTokenExpiry: refreshExpiry,
		StorageEndpoint:    getEnv("STORAGE_ENDPOINT", ""),
		StorageBucket:      getEnv("STORAGE_BUCKET", "routewerk-images"),
		StorageAccessKey:   getEnv("STORAGE_ACCESS_KEY", ""),
		StorageSecretKey:   getEnv("STORAGE_SECRET_KEY", ""),
		FCMProjectID:       getEnv("FCM_PROJECT_ID", ""),
		FCMCredentialsFile: getEnv("FCM_CREDENTIALS_FILE", ""),
		FrontendURL:        getEnv("FRONTEND_URL", "http://localhost:3000"),
	}
}

func (c *Config) IsDev() bool {
	return c.Env == "development"
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
