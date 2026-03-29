package service

import (
	"context"
	"errors"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/auth"
	"github.com/shotwell-paddle/routewerk/internal/config"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

var (
	ErrEmailTaken       = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidRefresh   = errors.New("invalid refresh token")
	ErrUserNotFound     = errors.New("user not found")
)

type AuthService struct {
	users *repository.UserRepo
	cfg   *config.Config
}

func NewAuthService(users *repository.UserRepo, cfg *config.Config) *AuthService {
	return &AuthService{users: users, cfg: cfg}
}

type AuthResult struct {
	User         *model.User   `json:"user"`
	AccessToken  string        `json:"access_token"`
	RefreshToken string        `json:"refresh_token"`
	ExpiresAt    time.Time     `json:"expires_at"`
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
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, ErrInvalidCredentials
	}

	if !auth.CheckPassword(password, u.PasswordHash) {
		return nil, ErrInvalidCredentials
	}

	return s.generateResult(ctx, u)
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
