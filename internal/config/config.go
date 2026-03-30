package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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
	ExtraOrigins        []string // additional allowed CORS origins
	SessionSecret       string
	SessionMaxAge       time.Duration
	DBMaxConns          int32
	DBMinConns          int32
	DBMaxConnLifetime   time.Duration
	DBMaxConnIdleTime   time.Duration
	QueryTimeout        time.Duration
}

func Load() *Config {
	jwtExpiry, _ := time.ParseDuration(getEnv("JWT_EXPIRY", "15m"))
	refreshExpiry, _ := time.ParseDuration(getEnv("REFRESH_TOKEN_EXPIRY", "720h"))
	sessionMaxAge, _ := time.ParseDuration(getEnv("SESSION_MAX_AGE", "720h")) // 30 days
	dbMaxConns := getEnvInt("DB_MAX_CONNS", 10)
	dbMinConns := getEnvInt("DB_MIN_CONNS", 2)
	dbMaxConnLifetime, _ := time.ParseDuration(getEnv("DB_MAX_CONN_LIFETIME", "1h"))
	dbMaxConnIdleTime, _ := time.ParseDuration(getEnv("DB_MAX_CONN_IDLE_TIME", "30m"))
	queryTimeout, _ := time.ParseDuration(getEnv("QUERY_TIMEOUT", "5s"))

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
		ExtraOrigins:       parseOrigins(getEnv("EXTRA_ORIGINS", "")),
		SessionSecret:      getEnv("SESSION_SECRET", "change-me-session"),
		SessionMaxAge:      sessionMaxAge,
		DBMaxConns:         int32(dbMaxConns),
		DBMinConns:         int32(dbMinConns),
		DBMaxConnLifetime:  dbMaxConnLifetime,
		DBMaxConnIdleTime:  dbMaxConnIdleTime,
		QueryTimeout:       queryTimeout,
	}
}

func (c *Config) IsDev() bool {
	return c.Env == "development"
}

// Validate checks for production-critical configuration errors.
// Returns a list of human-readable problems. An empty slice means the
// config is valid for the target environment.
func (c *Config) Validate() []string {
	if c.IsDev() {
		return nil // dev defaults are fine for local work
	}

	var problems []string
	check := func(ok bool, msg string) {
		if !ok {
			problems = append(problems, msg)
		}
	}

	// Database
	check(c.DatabaseURL != "" &&
		c.DatabaseURL != "postgres://routewerk:password@localhost:5432/routewerk?sslmode=disable",
		"DATABASE_URL must be set to a real connection string")

	// On Fly.io, Postgres is accessed over a private WireGuard network (*.flycast)
	// that is already encrypted end-to-end. sslmode=disable is safe and expected
	// because Fly's managed Postgres doesn't expose TLS on the internal interface.
	onFlyNetwork := os.Getenv("FLY_APP_NAME") != "" &&
		strings.Contains(c.DatabaseURL, ".flycast")
	if !onFlyNetwork {
		check(!strings.Contains(strings.ToLower(c.DatabaseURL), "sslmode=disable"),
			"DATABASE_URL must not use sslmode=disable in production")
	}

	// Auth secrets
	check(c.JWTSecret != "change-me" && len(c.JWTSecret) >= 32,
		"JWT_SECRET must be at least 32 characters and not the default value")
	check(c.SessionSecret != "change-me-session" && len(c.SessionSecret) >= 32,
		"SESSION_SECRET must be at least 32 characters and not the default value")

	// Durations
	check(c.JWTExpiry > 0 && c.JWTExpiry <= 1*time.Hour,
		"JWT_EXPIRY should be between 1s and 1h (got "+c.JWTExpiry.String()+")")
	check(c.RefreshTokenExpiry >= 24*time.Hour,
		"REFRESH_TOKEN_EXPIRY should be at least 24h (got "+c.RefreshTokenExpiry.String()+")")
	check(c.SessionMaxAge >= 1*time.Hour,
		"SESSION_MAX_AGE should be at least 1h (got "+c.SessionMaxAge.String()+")")

	// Frontend URL
	check(c.FrontendURL != "" && c.FrontendURL != "http://localhost:3000",
		"FRONTEND_URL must be set to the production origin")
	check(!strings.HasPrefix(c.FrontendURL, "http://"),
		"FRONTEND_URL must use HTTPS in production")

	return problems
}

// MustValidate calls Validate and panics if any problems are found.
// Intended for use at startup to fail fast.
func (c *Config) MustValidate() {
	problems := c.Validate()
	if len(problems) == 0 {
		return
	}
	msg := "configuration errors:\n"
	for _, p := range problems {
		msg += "  - " + p + "\n"
	}
	panic(msg)
}

// String returns a redacted summary of the configuration for logging.
// Secrets are masked; only the first 4 characters are shown.
func (c *Config) String() string {
	return fmt.Sprintf(
		"env=%s port=%s db=%s jwt=%s session=%s frontend=%s",
		c.Env, c.Port, redactURL(c.DatabaseURL),
		mask(c.JWTSecret), mask(c.SessionSecret), c.FrontendURL,
	)
}

func mask(s string) string {
	if len(s) <= 4 {
		return "****"
	}
	return s[:4] + "****"
}

func redactURL(u string) string {
	// Hide password in postgres://user:PASSWORD@host/db
	at := strings.Index(u, "@")
	if at < 0 {
		return u
	}
	colon := strings.LastIndex(u[:at], ":")
	if colon < 0 {
		return u
	}
	return u[:colon+1] + "****" + u[at:]
}

// parseOrigins splits a comma-separated list of origins (e.g.
// "https://a.example.com,https://b.example.com") into a slice.
func parseOrigins(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			origins = append(origins, p)
		}
	}
	return origins
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
