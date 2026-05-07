package handler

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/rbac"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

// emailRegex is a basic email format check — not exhaustive, but filters garbage.
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

type AuthHandler struct {
	auth   *service.AuthService
	secure bool // Sets the Secure flag on outbound cookies (false in dev).
}

func NewAuthHandler(authService *service.AuthService, secure bool) *AuthHandler {
	return &AuthHandler{auth: authService, secure: secure}
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

	// Surface the active view-as role (if any) so the SPA can render a
	// "viewing as X" badge + clear-button without having to read the
	// HttpOnly cookie. Empty string means no override is active.
	var viewAs string
	if c, err := r.Cookie(middleware.ViewAsCookieName); err == nil {
		viewAs = c.Value
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"user":         user,
		"memberships":  memberships,
		"view_as_role": viewAs,
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

// SetViewAs — PUT /me/view-as { role?: string }.
//
// JSON wrapper around the existing /switch-view-as form endpoint. Sets
// (or clears, with empty/missing role) the same _rw_view_as cookie that
// the cookie-or-JWT auth middleware honors. Authorization rules mirror
// switchers.go: caller's real role must be head_setter+ AND the target
// role must rank strictly below the caller's. Body shape:
//
//	{ "role": "climber" }   // downgrade
//	{ "role": "" }          // clear (or just send {})
//
// Returns 204 on success, 4xx on policy violation.
type setViewAsRequest struct {
	Role string `json:"role"`
}

func (h *AuthHandler) SetViewAs(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		Error(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req setViewAsRequest
	// Empty body is fine (means "clear"); decode errors only when body is
	// non-empty and malformed.
	if r.ContentLength > 0 {
		if err := Decode(r, &req); err != nil {
			Error(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	// Compute the caller's true max role across memberships (matches the
	// HTMX bestRole logic). is_app_admin promotes to org_admin.
	_, memberships, err := h.auth.GetProfile(r.Context(), userID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	realRank := 0
	realRole := ""
	for _, m := range memberships {
		rank := rbac.RankValue(m.Role)
		if rank > realRank {
			realRank = rank
			realRole = m.Role
		}
	}
	if realRank < rbac.RankValue(rbac.RoleHeadSetter) {
		Error(w, http.StatusForbidden, "view-as requires head_setter or above")
		return
	}

	target := strings.TrimSpace(req.Role)
	if target == "" {
		// Clear override.
		http.SetCookie(w, &http.Cookie{
			Name:     middleware.ViewAsCookieName,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   h.secure,
			SameSite: http.SameSiteLaxMode,
		})
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Validate target role exists in the allowed set (climber..gym_manager)
	// and ranks strictly below the caller's real role.
	targetRank := rbac.RankValue(target)
	if targetRank == 0 || target == rbac.RoleOrgAdmin {
		Error(w, http.StatusBadRequest, "invalid view-as role")
		return
	}
	if targetRank >= realRank {
		Error(w, http.StatusForbidden, "view-as can only target roles below your own ("+realRole+")")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     middleware.ViewAsCookieName,
		Value:    target,
		Path:     "/",
		MaxAge:   int((1 * time.Hour).Seconds()),
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}
