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
		// Fail closed — if we can't verify lockout state, deny the
		// attempt. The IP rate limiter provides a secondary layer, but
		// we don't want to bypass account lockout on DB hiccups.
		return nil, ErrAccountLocked
	}
	if locked {
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
		return nil, ErrAccountLocked
	}
	if locked {
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

	// Find the matching bcrypt hash for this token.
	var matchedHash string
	for _, hash := range hashes {
		if auth.CheckRefreshToken(refreshToken, hash) {
			matchedHash = hash
			break
		}
	}
	if matchedHash == "" {
		return nil, ErrInvalidRefresh
	}

	// Atomically revoke this specific token. If another request already
	// consumed it (race), the UPDATE affects zero rows and we fail.
	revoked, err := s.users.RevokeRefreshToken(ctx, matchedHash)
	if err != nil {
		return nil, err
	}
	if !revoked {
		return nil, ErrInvalidRefresh
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

// ChangePassword verifies the old password and sets a new one.
func (s *AuthService) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if u == nil {
		return ErrUserNotFound
	}

	if !auth.CheckPassword(oldPassword, u.PasswordHash) {
		return ErrInvalidCredentials
	}

	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}

	if err := s.users.UpdatePassword(ctx, userID, hash); err != nil {
		return err
	}

	// Revoke all refresh tokens so other sessions must re-authenticate
	if err := s.users.RevokeRefreshTokens(ctx, userID); err != nil {
		slog.Error("revoke tokens after password change failed", "user_id", userID, "error", err)
	}

	return nil
}

// ResetPassword sets a new password without requiring the old one.
// Intended for admin-initiated resets.
func (s *AuthService) ResetPassword(ctx context.Context, userID, newPassword string) error {
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}

	if err := s.users.UpdatePassword(ctx, userID, hash); err != nil {
		return err
	}

	if err := s.users.RevokeRefreshTokens(ctx, userID); err != nil {
		slog.Error("revoke tokens after password reset failed", "user_id", userID, "error", err)
	}
	return nil
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
