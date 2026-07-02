package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port               string
	Env                string
	DatabaseURL        string
	JWTSecret          string
	JWTExpiry          time.Duration
	RefreshTokenExpiry time.Duration
	// EnforceJWTAudience gates the audience-claim check on the read path.
	// New tokens always write the audience claim; once old tokens have drained
	// (≥ JWTExpiry past rollout) this can be flipped to true to make the
	// stricter rule binding. Env: JWT_ENFORCE_AUDIENCE.
	EnforceJWTAudience bool
	StorageEndpoint    string
	StorageBucket      string
	StorageAccessKey   string
	StorageSecretKey   string
	// Server-side database backups (see service/backup.go). Reuses the
	// STORAGE_* credentials; BackupBucket defaults to StorageBucket with
	// dumps under BackupPrefix. BackupRunOnBoot fires one backup at
	// startup — set on staging so every deploy smoke-tests the pipeline.
	BackupEnabled       bool
	BackupBucket        string
	BackupPrefix        string
	BackupHourUTC       int
	BackupRetentionDays int
	BackupRunOnBoot     bool
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
	DBHealthCheckPeriod time.Duration
	QueryTimeout        time.Duration

	// SMTP (optional — if not configured, emails are logged to stdout in
	// dev. In production, an unconfigured send is a hard error: see
	// EmailService.send and the MagicLinkEnabled guardrail in Validate.)
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string

	// MagicLinkEnabled gates the passwordless sign-in flow. Default false:
	// the magic-link request endpoint and UI entry points stay off until
	// email delivery is actually wired up. When true in production, Validate
	// requires SMTP to be configured so links can never be silently dropped.
	// Env: MAGIC_LINK_ENABLED.
	MagicLinkEnabled bool

	// loadProblems collects env parse errors detected during Load (e.g. a
	// typo'd duration like QUERY_TIMEOUT=5sec). Load logs them loudly and
	// falls back to the built-in default; Validate escalates them to fatal
	// outside dev so production fails fast instead of silently running with
	// a value the operator didn't set.
	loadProblems []string
}

func Load() *Config {
	// duration parses a duration env var. Unset → fallback. Set but invalid
	// → fallback plus a recorded problem, so a typo like QUERY_TIMEOUT=5sec
	// can never silently become 0 (which callers would treat as "no timeout"
	// or instant expiry). Validate escalates recorded problems to fatal
	// outside dev; in dev the loud log + default is the whole story.
	var problems []string
	duration := func(key, fallback string) time.Duration {
		raw := getEnv(key, fallback)
		d, err := time.ParseDuration(raw)
		if err == nil {
			return d
		}
		def, derr := time.ParseDuration(fallback)
		if derr != nil {
			// Fallbacks are compile-time constants; a bad one is a bug here,
			// not an operator error.
			panic(fmt.Sprintf("config: built-in default for %s is not a valid duration: %q", key, fallback))
		}
		problems = append(problems, fmt.Sprintf(
			"%s: invalid duration %q (%v); using default %s", key, raw, err, def))
		slog.Error("invalid duration in environment, using default",
			"var", key, "value", raw, "default", def.String())
		return def
	}

	jwtExpiry := duration("JWT_EXPIRY", "15m")
	refreshExpiry := duration("REFRESH_TOKEN_EXPIRY", "720h")
	sessionMaxAge := duration("SESSION_MAX_AGE", "720h") // 30 days
	dbMaxConns := getEnvInt("DB_MAX_CONNS", 5)
	dbMinConns := getEnvInt("DB_MIN_CONNS", 1)
	dbMaxConnLifetime := duration("DB_MAX_CONN_LIFETIME", "30m")
	dbMaxConnIdleTime := duration("DB_MAX_CONN_IDLE_TIME", "5m")
	dbHealthCheckPeriod := duration("DB_HEALTH_CHECK_PERIOD", "30s")
	queryTimeout := duration("QUERY_TIMEOUT", "5s")

	return &Config{
		Port:                getEnv("PORT", "8080"),
		Env:                 getEnv("ENV", "development"),
		DatabaseURL:         getEnv("DATABASE_URL", "postgres://routewerk:password@localhost:5432/routewerk?sslmode=disable"),
		JWTSecret:           getEnv("JWT_SECRET", "change-me"),
		JWTExpiry:           jwtExpiry,
		RefreshTokenExpiry:  refreshExpiry,
		EnforceJWTAudience:  getEnvBool("JWT_ENFORCE_AUDIENCE", false),
		StorageEndpoint:     getEnv("STORAGE_ENDPOINT", ""),
		StorageBucket:       getEnv("STORAGE_BUCKET", "routewerk-images"),
		StorageAccessKey:    getEnv("STORAGE_ACCESS_KEY", ""),
		StorageSecretKey:    getEnv("STORAGE_SECRET_KEY", ""),
		BackupEnabled:       getEnvBool("BACKUP_ENABLED", true),
		BackupBucket:        getEnv("BACKUP_BUCKET", ""),
		BackupPrefix:        getEnv("BACKUP_PREFIX", "backups/"),
		BackupHourUTC:       getEnvInt("BACKUP_HOUR_UTC", 9),
		BackupRetentionDays: getEnvInt("BACKUP_RETENTION_DAYS", 35),
		BackupRunOnBoot:     getEnvBool("BACKUP_RUN_ON_BOOT", false),
		FCMProjectID:        getEnv("FCM_PROJECT_ID", ""),
		FCMCredentialsFile:  getEnv("FCM_CREDENTIALS_FILE", ""),
		FrontendURL:         getEnv("FRONTEND_URL", "http://localhost:3000"),
		ExtraOrigins:        parseOrigins(getEnv("EXTRA_ORIGINS", "")),
		SessionSecret:       getEnv("SESSION_SECRET", "change-me-session"),
		SessionMaxAge:       sessionMaxAge,
		DBMaxConns:          int32(dbMaxConns),
		DBMinConns:          int32(dbMinConns),
		DBMaxConnLifetime:   dbMaxConnLifetime,
		DBMaxConnIdleTime:   dbMaxConnIdleTime,
		DBHealthCheckPeriod: dbHealthCheckPeriod,
		QueryTimeout:        queryTimeout,

		SMTPHost:     getEnv("SMTP_HOST", ""),
		SMTPPort:     getEnv("SMTP_PORT", "587"),
		SMTPUsername: getEnv("SMTP_USERNAME", ""),
		SMTPPassword: getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:     getEnv("SMTP_FROM", ""),

		MagicLinkEnabled: getEnvBool("MAGIC_LINK_ENABLED", false),

		loadProblems: problems,
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
		// Dev defaults are fine for local work. Env parse errors recorded in
		// Load (loadProblems) were already logged loudly there and fell back
		// to defaults — no need to block local startup on them.
		return nil
	}

	var problems []string
	check := func(ok bool, msg string) {
		if !ok {
			problems = append(problems, msg)
		}
	}

	// Escalate env parse errors from Load. In production a typo'd duration
	// must fail startup with the variable named, not run with a fallback
	// the operator didn't choose.
	problems = append(problems, c.loadProblems...)

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

	// Magic-link delivery. The flow generates a token, stores it, and emails
	// the link via SMTP. If MAGIC_LINK_ENABLED is on but SMTP isn't set, every
	// request would silently succeed (202 "check your email") while delivering
	// nothing — so refuse to boot. Keep the flag off until email is wired.
	if c.MagicLinkEnabled {
		check(c.SMTPHost != "" && c.SMTPFrom != "",
			"MAGIC_LINK_ENABLED requires SMTP_HOST and SMTP_FROM (magic links can't be delivered without them)")
	}

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

// getEnvBool parses common truthy/falsy strings. Anything unparseable or
// empty returns the fallback so misconfiguration can't silently flip a flag.
func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
