package handler

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

// emailRegex is a basic email format check — not exhaustive, but filters garbage.
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

type AuthHandler struct {
	auth *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{auth: authService}
}

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" || req.DisplayName == "" {
		Error(w, http.StatusBadRequest, "email, password, and display_name are required")
		return
	}

	// Normalize email
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	if !emailRegex.MatchString(req.Email) {
		Error(w, http.StatusBadRequest, "invalid email format")
		return
	}

	if len(req.Email) > 254 {
		Error(w, http.StatusBadRequest, "email too long")
		return
	}

	if len(req.Password) < 8 {
		Error(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	// bcrypt silently truncates at 72 bytes — reject longer passwords so users
	// aren't surprised that only the first 72 bytes matter.
	if len(req.Password) > 72 {
		Error(w, http.StatusBadRequest, "password must be at most 72 characters")
		return
	}

	if len(req.DisplayName) > 100 {
		Error(w, http.StatusBadRequest, "display name too long")
		return
	}

	result, err := h.auth.Register(r.Context(), req.Email, req.Password, req.DisplayName)
	if err != nil {
		if errors.Is(err, service.ErrEmailTaken) {
			Error(w, http.StatusConflict, "email already registered")
			return
		}
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	JSON(w, http.StatusCreated, result)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		Error(w, http.StatusBadRequest, "email and password are required")
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	result, err := h.auth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrAccountLocked) {
			Error(w, http.StatusTooManyRequests, "account temporarily locked, try again later")
			return
		}
		if errors.Is(err, service.ErrInvalidCredentials) {
			Error(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	JSON(w, http.StatusOK, result)
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.RefreshToken == "" {
		Error(w, http.StatusBadRequest, "refresh_token is required")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		Error(w, http.StatusUnauthorized, "authentication required")
		return
	}

	result, err := h.auth.Refresh(r.Context(), userID, req.RefreshToken)
	if err != nil {
		if errors.Is(err, service.ErrInvalidRefresh) {
			Error(w, http.StatusUnauthorized, "invalid refresh token")
			return
		}
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	JSON(w, http.StatusOK, result)
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	user, memberships, err := h.auth.GetProfile(r.Context(), userID)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			Error(w, http.StatusNotFound, "user not found")
			return
		}
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"user":        user,
		"memberships": memberships,
	})
}

// updateMeRequest patches the caller's user profile. All fields are optional;
// only fields present in the request body are applied. avatar_url and bio
// accept null to clear the current value (the indirection here distinguishes
// "field omitted" from "field present but null"); display_name cannot be
// blanked because the schema requires it.
type updateMeRequest struct {
	DisplayName *string `json:"display_name,omitempty"`
	AvatarURL   *string `json:"avatar_url,omitempty"`
	Bio         *string `json:"bio,omitempty"`
	// Boolean flags for "explicitly clear this field" — needed because the
	// pointer above can't disambiguate "not in JSON" from "JSON null" once
	// json.Unmarshal has run. The SPA settings form sends these when the
	// user blanks out an input.
	ClearAvatarURL bool `json:"clear_avatar_url,omitempty"`
	ClearBio       bool `json:"clear_bio,omitempty"`
}

// UpdateMe — PATCH /me. Updates editable profile fields on the calling user.
func (h *AuthHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req updateMeRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.DisplayName != nil {
		trimmed := strings.TrimSpace(*req.DisplayName)
		if trimmed == "" {
			Error(w, http.StatusBadRequest, "display_name cannot be empty")
			return
		}
		req.DisplayName = &trimmed
	}

	var avatar **string
	if req.ClearAvatarURL {
		var nilStr *string
		avatar = &nilStr
	} else if req.AvatarURL != nil {
		avatar = &req.AvatarURL
	}

	var bio **string
	if req.ClearBio {
		var nilStr *string
		bio = &nilStr
	} else if req.Bio != nil {
		bio = &req.Bio
	}

	user, err := h.auth.UpdateProfile(r.Context(), userID, req.DisplayName, avatar, bio)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			Error(w, http.StatusNotFound, "user not found")
			return
		}
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{"user": user})
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// ChangePassword — POST /me/password. Verifies the old password, hashes the
// new one, and revokes outstanding refresh tokens so other sessions must
// re-authenticate.
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req changePasswordRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.OldPassword == "" || req.NewPassword == "" {
		Error(w, http.StatusBadRequest, "old_password and new_password are required")
		return
	}
	if len(req.NewPassword) < 8 {
		Error(w, http.StatusBadRequest, "new_password must be at least 8 characters")
		return
	}

	if err := h.auth.ChangePassword(r.Context(), userID, req.OldPassword, req.NewPassword); err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			Error(w, http.StatusUnauthorized, "old password is incorrect")
			return
		}
		if errors.Is(err, service.ErrUserNotFound) {
			Error(w, http.StatusNotFound, "user not found")
			return
		}
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
