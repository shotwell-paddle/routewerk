package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/auth"
	"github.com/shotwell-paddle/routewerk/internal/config"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

const (
	maxLoginAttempts = 5
	lockoutDuration  = 15 * time.Minute
)

var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidRefresh     = errors.New("invalid refresh token")
	ErrUserNotFound       = errors.New("user not found")
	ErrAccountLocked      = errors.New("account temporarily locked")
)

type AuthService struct {
	users    *repository.UserRepo
	attempts *repository.LoginAttemptRepo
	cfg      *config.Config
}

func NewAuthService(users *repository.UserRepo, attempts *repository.LoginAttemptRepo, cfg *config.Config) *AuthService {
	return &AuthService{users: users, attempts: attempts, cfg: cfg}
}

type AuthResult struct {
	User         *model.User          `json:"user"`
	AccessToken  string               `json:"access_token"`
	RefreshToken string               `json:"refresh_token"`
	ExpiresAt    time.Time            `json:"expires_at"`
	Memberships  []model.UserMembership `json:"memberships,omitempty"`
}

func (s *AuthService) Register(ctx context.Context, email, password, displayName string) (*AuthResult, error) {
	existing, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrEmailTaken
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}

	u := &model.User{
		Email:        email,
		PasswordHash: hash,
		DisplayName:  displayName,
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, err
	}

	return s.generateResult(ctx, u)
}

func (s *AuthService) Login(ctx context.Context, email, password string) (*AuthResult, error) {
	// Check account lockout
	locked, err := s.attempts.IsLocked(ctx, email)
	if err != nil {
		slog.Error("lockout check failed", "email", email, "error", err)
		// Fail open on DB error — don't block legitimate users because
		// the login_attempts table is unavailable. The IP rate limiter
		// still protects against brute force.
	} else if locked {
		slog.Warn("login attempt on locked account", "email", email)
		return nil, ErrAccountLocked
	}

	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, ErrInvalidCredentials
	}

	if !auth.CheckPassword(password, u.PasswordHash) {
		// Record failed attempt
		count, lockedUntil, recErr := s.attempts.RecordFailure(ctx, email, maxLoginAttempts, lockoutDuration)
		if recErr != nil {
			slog.Error("failed to record login failure", "email", email, "error", recErr)
		} else if !lockedUntil.IsZero() {
			slog.Warn("account locked after failed attempts",
				"email", email,
				"attempts", count,
				"locked_until", lockedUntil,
			)
		}
		return nil, ErrInvalidCredentials
	}

	// Successful login — clear failure counter
	if err := s.attempts.ClearFailures(ctx, email); err != nil {
		slog.Error("failed to clear login failures", "email", email, "error", err)
	}

	return s.generateResult(ctx, u)
}

// ValidateCredentials checks the user's email and password without generating
// any tokens. Used by the web session login flow which doesn't need JWTs.
// Returns the verified User on success.
func (s *AuthService) ValidateCredentials(ctx context.Context, email, password string) (*model.User, error) {
	// Check account lockout
	locked, err := s.attempts.IsLocked(ctx, email)
	if err != nil {
		slog.Error("lockout check failed", "email", email, "error", err)
	} else if locked {
		slog.Warn("login attempt on locked account", "email", email)
		return nil, ErrAccountLocked
	}

	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, ErrInvalidCredentials
	}

	if !auth.CheckPassword(password, u.PasswordHash) {
		count, lockedUntil, recErr := s.attempts.RecordFailure(ctx, email, maxLoginAttempts, lockoutDuration)
		if recErr != nil {
			slog.Error("failed to record login failure", "email", email, "error", recErr)
		} else if !lockedUntil.IsZero() {
			slog.Warn("account locked after failed attempts",
				"email", email,
				"attempts", count,
				"locked_until", lockedUntil,
			)
		}
		return nil, ErrInvalidCredentials
	}

	// Successful — clear failure counter
	if err := s.attempts.ClearFailures(ctx, email); err != nil {
		slog.Error("failed to clear login failures", "email", email, "error", err)
	}

	return u, nil
}

func (s *AuthService) Refresh(ctx context.Context, userID, refreshToken string) (*AuthResult, error) {
	hashes, err := s.users.GetActiveRefreshTokens(ctx, userID)
	if err != nil {
		return nil, err
	}

	valid := false
	for _, hash := range hashes {
		if auth.CheckRefreshToken(refreshToken, hash) {
			valid = true
			break
		}
	}
	if !valid {
		return nil, ErrInvalidRefresh
	}

	if err := s.users.RevokeRefreshTokens(ctx, userID); err != nil {
		return nil, err
	}

	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, ErrUserNotFound
	}

	return s.generateResult(ctx, u)
}

func (s *AuthService) GetProfile(ctx context.Context, userID string) (*model.User, []model.UserMembership, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	if u == nil {
		return nil, nil, ErrUserNotFound
	}

	memberships, err := s.users.GetMemberships(ctx, userID)
	if err != nil {
		return nil, nil, err
	}

	return u, memberships, nil
}

func (s *AuthService) generateResult(ctx context.Context, u *model.User) (*AuthResult, error) {
	accessToken, expiresAt, err := auth.GenerateAccessToken(
		u.ID, u.Email, s.cfg.JWTSecret, s.cfg.JWTExpiry,
	)
	if err != nil {
		return nil, err
	}

	refreshToken, err := auth.GenerateRefreshToken()
	if err != nil {
		return nil, err
	}

	refreshHash := auth.HashRefreshToken(refreshToken)
	refreshExpiry := time.Now().Add(s.cfg.RefreshTokenExpiry)
	if err := s.users.SaveRefreshToken(ctx, u.ID, refreshHash, refreshExpiry); err != nil {
		return nil, err
	}

	return &AuthResult{
		User:         u,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
	}, nil
}
