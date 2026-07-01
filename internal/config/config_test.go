package config

import (
	"strings"
	"testing"
	"time"
)

func validProdConfig() *Config {
	return &Config{
		Env:                "production",
		Port:               "8080",
		DatabaseURL:        "postgres://routewerk:secret@db.example.com:5432/routewerk?sslmode=require",
		JWTSecret:          "a-very-long-production-jwt-secret-key-1234",
		JWTExpiry:          15 * time.Minute,
		RefreshTokenExpiry: 720 * time.Hour,
		SessionSecret:      "a-very-long-production-session-secret-1234",
		SessionMaxAge:      720 * time.Hour,
		FrontendURL:        "https://app.routewerk.com",
	}
}

func TestValidate_DevAlwaysPasses(t *testing.T) {
	c := &Config{Env: "development"} // all defaults
	if problems := c.Validate(); len(problems) > 0 {
		t.Errorf("dev config should pass validation, got %v", problems)
	}
}

func TestValidate_ValidProdPasses(t *testing.T) {
	c := validProdConfig()
	if problems := c.Validate(); len(problems) > 0 {
		t.Errorf("valid prod config should pass, got %v", problems)
	}
}

func TestValidate_MagicLinkEnabledWithoutSMTPFails(t *testing.T) {
	c := validProdConfig()
	c.MagicLinkEnabled = true // no SMTP_HOST / SMTP_FROM set
	problems := c.Validate()
	found := false
	for _, p := range problems {
		if strings.Contains(p, "MAGIC_LINK_ENABLED") {
			found = true
		}
	}
	if !found {
		t.Error("expected MAGIC_LINK_ENABLED problem when SMTP unconfigured, got none")
	}
}

func TestValidate_MagicLinkEnabledWithSMTPPasses(t *testing.T) {
	c := validProdConfig()
	c.MagicLinkEnabled = true
	c.SMTPHost = "smtp.postmarkapp.com"
	c.SMTPFrom = "noreply@routewerk.com"
	if problems := c.Validate(); len(problems) > 0 {
		t.Errorf("magic link with SMTP configured should pass, got %v", problems)
	}
}

func TestValidate_DefaultDatabaseURLFails(t *testing.T) {
	c := validProdConfig()
	c.DatabaseURL = "postgres://routewerk:password@localhost:5432/routewerk?sslmode=disable"
	problems := c.Validate()
	found := false
	for _, p := range problems {
		if strings.Contains(p, "DATABASE_URL") {
			found = true
		}
	}
	if !found {
		t.Error("expected DATABASE_URL problem, got none")
	}
}

func TestValidate_SSLModeDisableFails(t *testing.T) {
	c := validProdConfig()
	c.DatabaseURL = "postgres://user:pass@prod-host/db?sslmode=disable"
	problems := c.Validate()
	found := false
	for _, p := range problems {
		if strings.Contains(p, "sslmode=disable") {
			found = true
		}
	}
	if !found {
		t.Error("expected sslmode problem")
	}
}

func TestValidate_WeakJWTSecretFails(t *testing.T) {
	c := validProdConfig()
	c.JWTSecret = "change-me"
	problems := c.Validate()
	found := false
	for _, p := range problems {
		if strings.Contains(p, "JWT_SECRET") {
			found = true
		}
	}
	if !found {
		t.Error("expected JWT_SECRET problem")
	}
}

func TestValidate_ShortJWTSecretFails(t *testing.T) {
	c := validProdConfig()
	c.JWTSecret = "short"
	problems := c.Validate()
	found := false
	for _, p := range problems {
		if strings.Contains(p, "JWT_SECRET") {
			found = true
		}
	}
	if !found {
		t.Error("expected JWT_SECRET length problem")
	}
}

func TestValidate_WeakSessionSecretFails(t *testing.T) {
	c := validProdConfig()
	c.SessionSecret = "change-me-session"
	problems := c.Validate()
	found := false
	for _, p := range problems {
		if strings.Contains(p, "SESSION_SECRET") {
			found = true
		}
	}
	if !found {
		t.Error("expected SESSION_SECRET problem")
	}
}

func TestValidate_HTTPFrontendURLFails(t *testing.T) {
	c := validProdConfig()
	c.FrontendURL = "http://app.routewerk.com"
	problems := c.Validate()
	found := false
	for _, p := range problems {
		if strings.Contains(p, "HTTPS") {
			found = true
		}
	}
	if !found {
		t.Error("expected HTTPS problem for FRONTEND_URL")
	}
}

func TestValidate_ExcessiveJWTExpiryFails(t *testing.T) {
	c := validProdConfig()
	c.JWTExpiry = 24 * time.Hour
	problems := c.Validate()
	found := false
	for _, p := range problems {
		if strings.Contains(p, "JWT_EXPIRY") {
			found = true
		}
	}
	if !found {
		t.Error("expected JWT_EXPIRY problem for >1h")
	}
}

// ── Duration env parsing (Load) ─────────────────────────────

// TestLoad_DurationParsing verifies that duration env vars parse correctly,
// that unset vars silently use defaults, and that a typo'd value falls back
// to the default (never 0) while recording a problem naming the variable.
func TestLoad_DurationParsing(t *testing.T) {
	tests := []struct {
		name        string
		envKey      string
		envValue    string
		get         func(*Config) time.Duration
		want        time.Duration
		wantProblem bool
	}{
		{
			name:   "valid QUERY_TIMEOUT",
			envKey: "QUERY_TIMEOUT", envValue: "10s",
			get:  func(c *Config) time.Duration { return c.QueryTimeout },
			want: 10 * time.Second,
		},
		{
			name:   "unset QUERY_TIMEOUT uses default silently",
			envKey: "QUERY_TIMEOUT", envValue: "",
			get:  func(c *Config) time.Duration { return c.QueryTimeout },
			want: 5 * time.Second,
		},
		{
			name:   "typo'd QUERY_TIMEOUT falls back to default, not 0",
			envKey: "QUERY_TIMEOUT", envValue: "5sec",
			get:  func(c *Config) time.Duration { return c.QueryTimeout },
			want: 5 * time.Second, wantProblem: true,
		},
		{
			name:   "typo'd JWT_EXPIRY falls back to default, not 0",
			envKey: "JWT_EXPIRY", envValue: "15minutes",
			get:  func(c *Config) time.Duration { return c.JWTExpiry },
			want: 15 * time.Minute, wantProblem: true,
		},
		{
			name:   "typo'd SESSION_MAX_AGE falls back to default, not 0",
			envKey: "SESSION_MAX_AGE", envValue: "30 days",
			get:  func(c *Config) time.Duration { return c.SessionMaxAge },
			want: 720 * time.Hour, wantProblem: true,
		},
		{
			name:   "bare number is not a duration",
			envKey: "DB_MAX_CONN_LIFETIME", envValue: "30",
			get:  func(c *Config) time.Duration { return c.DBMaxConnLifetime },
			want: 30 * time.Minute, wantProblem: true,
		},
		{
			name:   "valid DB_HEALTH_CHECK_PERIOD",
			envKey: "DB_HEALTH_CHECK_PERIOD", envValue: "1m30s",
			get:  func(c *Config) time.Duration { return c.DBHealthCheckPeriod },
			want: 90 * time.Second,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.envKey, tc.envValue)

			cfg := Load()
			if got := tc.get(cfg); got != tc.want {
				t.Errorf("%s=%q: got %s, want %s", tc.envKey, tc.envValue, got, tc.want)
			}

			found := false
			for _, p := range cfg.loadProblems {
				if strings.Contains(p, tc.envKey) {
					found = true
				}
			}
			if found != tc.wantProblem {
				t.Errorf("%s=%q: problem recorded = %v, want %v (problems: %v)",
					tc.envKey, tc.envValue, found, tc.wantProblem, cfg.loadProblems)
			}
		})
	}
}

func TestValidate_LoadProblemsFailProduction(t *testing.T) {
	c := validProdConfig()
	c.QueryTimeout = 5 * time.Second
	c.loadProblems = []string{`QUERY_TIMEOUT: invalid duration "5sec"; using default 5s`}
	problems := c.Validate()
	found := false
	for _, p := range problems {
		if strings.Contains(p, "QUERY_TIMEOUT") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected QUERY_TIMEOUT parse problem to fail production validation, got %v", problems)
	}
}

func TestValidate_LoadProblemsIgnoredInDev(t *testing.T) {
	c := &Config{Env: "development"}
	c.loadProblems = []string{`QUERY_TIMEOUT: invalid duration "5sec"; using default 5s`}
	if problems := c.Validate(); len(problems) > 0 {
		t.Errorf("dev should not fail on load problems (they are logged + defaulted), got %v", problems)
	}
}

func TestMask(t *testing.T) {
	if got := mask("abcdefgh"); got != "abcd****" {
		t.Errorf("mask(abcdefgh) = %q", got)
	}
	if got := mask("ab"); got != "****" {
		t.Errorf("mask(ab) = %q", got)
	}
}

func TestRedactURL(t *testing.T) {
	got := redactURL("postgres://user:secret@host:5432/db")
	if strings.Contains(got, "secret") {
		t.Errorf("password should be redacted: %q", got)
	}
	if !strings.Contains(got, "****") {
		t.Errorf("should contain mask: %q", got)
	}
}

func TestString_NoSecretLeak(t *testing.T) {
	c := validProdConfig()
	s := c.String()
	if strings.Contains(s, c.JWTSecret) {
		t.Error("String() should not contain full JWT secret")
	}
	if strings.Contains(s, "secret") {
		t.Error("String() should not contain database password")
	}
}

// ── Additional validation edge cases ────────────────────────

func TestValidate_ShortRefreshTokenExpiryFails(t *testing.T) {
	c := validProdConfig()
	c.RefreshTokenExpiry = 1 * time.Hour // < 24h
	problems := c.Validate()
	found := false
	for _, p := range problems {
		if strings.Contains(p, "REFRESH_TOKEN_EXPIRY") {
			found = true
		}
	}
	if !found {
		t.Error("expected REFRESH_TOKEN_EXPIRY problem for <24h")
	}
}

func TestValidate_ShortSessionMaxAgeFails(t *testing.T) {
	c := validProdConfig()
	c.SessionMaxAge = 30 * time.Minute // < 1h
	problems := c.Validate()
	found := false
	for _, p := range problems {
		if strings.Contains(p, "SESSION_MAX_AGE") {
			found = true
		}
	}
	if !found {
		t.Error("expected SESSION_MAX_AGE problem for <1h")
	}
}

func TestValidate_DefaultFrontendURLFails(t *testing.T) {
	c := validProdConfig()
	c.FrontendURL = "http://localhost:3000"
	problems := c.Validate()
	found := false
	for _, p := range problems {
		if strings.Contains(p, "FRONTEND_URL") {
			found = true
		}
	}
	if !found {
		t.Error("expected FRONTEND_URL problem for default value")
	}
}

func TestValidate_EmptyFrontendURLFails(t *testing.T) {
	c := validProdConfig()
	c.FrontendURL = ""
	problems := c.Validate()
	found := false
	for _, p := range problems {
		if strings.Contains(p, "FRONTEND_URL") {
			found = true
		}
	}
	if !found {
		t.Error("expected FRONTEND_URL problem for empty value")
	}
}

func TestValidate_ZeroJWTExpiryFails(t *testing.T) {
	c := validProdConfig()
	c.JWTExpiry = 0
	problems := c.Validate()
	found := false
	for _, p := range problems {
		if strings.Contains(p, "JWT_EXPIRY") {
			found = true
		}
	}
	if !found {
		t.Error("expected JWT_EXPIRY problem for 0 duration")
	}
}

func TestValidate_MultipleProblemsCombined(t *testing.T) {
	c := &Config{
		Env:                "production",
		DatabaseURL:        "postgres://routewerk:password@localhost:5432/routewerk?sslmode=disable",
		JWTSecret:          "short",
		JWTExpiry:          0,
		RefreshTokenExpiry: 0,
		SessionSecret:      "tiny",
		SessionMaxAge:      0,
		FrontendURL:        "",
	}
	problems := c.Validate()
	if len(problems) < 5 {
		t.Errorf("expected at least 5 problems, got %d: %v", len(problems), problems)
	}
}

func TestIsDev(t *testing.T) {
	dev := &Config{Env: "development"}
	if !dev.IsDev() {
		t.Error("IsDev should return true for development")
	}

	prod := &Config{Env: "production"}
	if prod.IsDev() {
		t.Error("IsDev should return false for production")
	}

	other := &Config{Env: "staging"}
	if other.IsDev() {
		t.Error("IsDev should return false for staging")
	}
}

func TestMustValidate_PanicsOnBadConfig(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustValidate should panic on bad production config")
		}
	}()

	c := &Config{
		Env:       "production",
		JWTSecret: "short",
	}
	c.MustValidate()
}

func TestMustValidate_NoPanicOnValidConfig(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("MustValidate should not panic on valid config: %v", r)
		}
	}()

	validProdConfig().MustValidate()
}

func TestRedactURL_NoPassword(t *testing.T) {
	// URL without password should pass through unchanged
	url := "postgres://localhost:5432/db"
	got := redactURL(url)
	if got != url {
		t.Errorf("redactURL with no password = %q, want %q", got, url)
	}
}

func TestRedactURL_NoAtSign(t *testing.T) {
	url := "just-a-string"
	got := redactURL(url)
	if got != url {
		t.Errorf("redactURL with no @ = %q, want %q", got, url)
	}
}

func TestMask_ExactlyFourChars(t *testing.T) {
	got := mask("abcd")
	if got != "****" {
		t.Errorf("mask(abcd) = %q, want ****", got)
	}
}

func TestMask_Empty(t *testing.T) {
	got := mask("")
	if got != "****" {
		t.Errorf("mask(\"\") = %q, want ****", got)
	}
}

func TestMask_FiveChars(t *testing.T) {
	got := mask("abcde")
	if got != "abcd****" {
		t.Errorf("mask(abcde) = %q, want abcd****", got)
	}
}
