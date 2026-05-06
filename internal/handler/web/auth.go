package webhandler

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

// emailRegex is a basic email format check (same pattern as the API handler).
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// ── Login ────────────────────────────────────────────────────

// LoginPage renders the login form (GET /login).
func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	// Auth pages are standalone HTML documents (no base layout). If requested
	// via HTMX (sidebar link), force a full page navigation so the sidebar
	// doesn't wrap the login card.
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/login")
		w.WriteHeader(http.StatusOK)
		return
	}

	// Already authenticated — redirect to dashboard
	if cookie, err := r.Cookie(middleware.SessionCookieName); err == nil && cookie.Value != "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	tmpl, ok := h.templates["auth/login.html"]
	if !ok {
		http.Error(w, "login template not found", http.StatusInternalServerError)
		return
	}

	data := struct {
		CSRFToken string
		Email     string
		Error     string
	}{
		CSRFToken: middleware.TokenFromRequest(r),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		slog.Error("login template render failed", "error", err)
	}
}

// LoginSubmit processes the login form (POST /login).
func (h *Handler) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.loginError(w, r, "Invalid form data.", "")
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	if email == "" || password == "" {
		h.loginError(w, r, "Email and password are required.", email)
		return
	}

	// Authenticate via the auth service (reuses bcrypt check + lockout).
	// ValidateCredentials only checks the password — no JWT/refresh token overhead.
	user, err := h.authService.ValidateCredentials(r.Context(), email, password)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidCredentials):
			h.loginError(w, r, "Invalid email or password.", email)
		case errors.Is(err, service.ErrAccountLocked):
			h.loginError(w, r, "Account temporarily locked. Try again later.", email)
		default:
			slog.Error("login failed", "email", email, "error", err)
			h.loginError(w, r, "Something went wrong. Please try again.", email)
		}
		return
	}

	// Create a web session
	token, tokenHash, err := middleware.GenerateSessionToken()
	if err != nil {
		slog.Error("session token generation failed", "error", err)
		h.loginError(w, r, "Something went wrong. Please try again.", email)
		return
	}

	// Pick the user's default location — prefer the domain-matched gym,
	// then fall back to the first membership with a location.
	memberships, memErr := h.userRepo.GetMemberships(r.Context(), user.ID)
	if memErr != nil {
		slog.Error("load memberships after login failed", "user_id", user.ID, "error", memErr)
	}

	locationID := h.locationForHost(r)
	if locationID == nil {
		for _, m := range memberships {
			if m.LocationID != nil {
				locationID = m.LocationID
				break
			}
		}
	}

	ip := realIP(r)
	ua := truncateUA(r.UserAgent())
	sess := &model.WebSession{
		UserID:     user.ID,
		LocationID: locationID,
		TokenHash:  tokenHash,
		IPAddress:  &ip,
		UserAgent:  &ua,
		ExpiresAt:  repository.SessionExpiry(h.cfg.SessionMaxAge),
	}

	if err := h.webSessionRepo.Create(r.Context(), sess); err != nil {
		slog.Error("session creation failed", "user_id", user.ID, "error", err)
		h.loginError(w, r, "Something went wrong. Please try again.", email)
		return
	}

	// Enforce session limit (delete oldest if over max)
	if err := h.webSessionRepo.EnforceLimit(r.Context(), user.ID); err != nil {
		slog.Error("session limit enforcement failed", "user_id", user.ID, "error", err)
	}

	// Set the session cookie
	h.sessionMgr.SetSessionCookie(w, token, h.cfg.SessionMaxAge)

	// Rotate the CSRF cookie so any token planted before login (session
	// fixation) stops being valid. Failure here is logged but non-fatal —
	// the user still has an old-but-working token, and the next page load
	// will redisplay it correctly.
	if _, err := middleware.RotateCSRFToken(w, !h.cfg.IsDev()); err != nil {
		slog.Error("csrf rotate after login failed", "user_id", user.ID, "error", err)
	}

	// Users with no location need to pick a gym or set up the first one
	if locationID == nil {
		http.Redirect(w, r, h.postAuthRedirect(r), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// loginError re-renders the login page with an error message.
func (h *Handler) loginError(w http.ResponseWriter, r *http.Request, msg, email string) {
	tmpl, ok := h.templates["auth/login.html"]
	if !ok {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	data := struct {
		CSRFToken string
		Email     string
		Error     string
	}{
		CSRFToken: middleware.TokenFromRequest(r),
		Email:     email,
		Error:     msg,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnprocessableEntity)
	if err := tmpl.Execute(w, data); err != nil {
		slog.Error("login error template render failed", "error", err)
	}
}

// ── Logout ───────────────────────────────────────────────────

// Logout destroys the session and redirects to login (POST /logout).
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetWebSession(r.Context())
	if session != nil {
		if err := h.webSessionRepo.Delete(r.Context(), session.ID); err != nil {
			slog.Error("session delete failed", "session_id", session.ID, "error", err)
		}
	}

	h.sessionMgr.ClearSessionCookie(w)

	// Rotate the CSRF cookie too — on a shared device, the next user
	// picking up the browser should not inherit the previous user's
	// pre-logout CSRF token.
	if _, err := middleware.RotateCSRFToken(w, !h.cfg.IsDev()); err != nil {
		slog.Error("csrf rotate after logout failed", "error", err)
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/login")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// ── Registration ─────────────────────────────────────────────

// RegisterPage renders the registration form (GET /register).
func (h *Handler) RegisterPage(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/register")
		w.WriteHeader(http.StatusOK)
		return
	}

	tmpl, ok := h.templates["auth/register.html"]
	if !ok {
		http.Error(w, "register template not found", http.StatusInternalServerError)
		return
	}

	data := struct {
		CSRFToken   string
		Email       string
		DisplayName string
		Error       string
	}{
		CSRFToken: middleware.TokenFromRequest(r),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		slog.Error("register template render failed", "error", err)
	}
}

// RegisterSubmit processes the registration form (POST /register).
func (h *Handler) RegisterSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.registerError(w, r, "Invalid form data.", "", "")
		return
	}

	displayName := strings.TrimSpace(r.FormValue("display_name"))
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	password := r.FormValue("password")
	passwordConfirm := r.FormValue("password_confirm")

	// Validation
	if displayName == "" || email == "" || password == "" {
		h.registerError(w, r, "All fields are required.", email, displayName)
		return
	}

	if len(displayName) > 100 {
		h.registerError(w, r, "Display name is too long.", email, displayName)
		return
	}

	if !emailRegex.MatchString(email) {
		h.registerError(w, r, "Please enter a valid email address.", email, displayName)
		return
	}

	if len(password) < 8 {
		h.registerError(w, r, "Password must be at least 8 characters.", email, displayName)
		return
	}

	if len(password) > 72 {
		h.registerError(w, r, "Password must be at most 72 characters.", email, displayName)
		return
	}

	if password != passwordConfirm {
		h.registerError(w, r, "Passwords do not match.", email, displayName)
		return
	}

	// Create account via auth service (ignores JWT tokens — we create a web session instead)
	_, err := h.authService.Register(r.Context(), email, password, displayName)
	if err != nil {
		if errors.Is(err, service.ErrEmailTaken) {
			h.registerError(w, r, "An account with that email already exists.", email, displayName)
			return
		}
		slog.Error("registration failed", "email", email, "error", err)
		h.registerError(w, r, "Something went wrong. Please try again.", email, displayName)
		return
	}

	// Authenticate the new user to create a web session
	user, err := h.authService.ValidateCredentials(r.Context(), email, password)
	if err != nil {
		slog.Error("post-register auth failed", "email", email, "error", err)
		// Account was created — send them to login
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Create web session (same logic as LoginSubmit)
	token, tokenHash, err := middleware.GenerateSessionToken()
	if err != nil {
		slog.Error("session token generation failed", "error", err)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Pick the user's default location — prefer the domain-matched gym,
	// then fall back to the first membership with a location.
	memberships, memErr := h.userRepo.GetMemberships(r.Context(), user.ID)
	if memErr != nil {
		slog.Error("load memberships after register failed", "user_id", user.ID, "error", memErr)
	}

	locationID := h.locationForHost(r)
	if locationID == nil {
		for _, m := range memberships {
			if m.LocationID != nil {
				locationID = m.LocationID
				break
			}
		}
	}

	ip := realIP(r)
	ua := truncateUA(r.UserAgent())
	sess := &model.WebSession{
		UserID:     user.ID,
		LocationID: locationID,
		TokenHash:  tokenHash,
		IPAddress:  &ip,
		UserAgent:  &ua,
		ExpiresAt:  repository.SessionExpiry(h.cfg.SessionMaxAge),
	}

	if err := h.webSessionRepo.Create(r.Context(), sess); err != nil {
		slog.Error("session creation failed", "user_id", user.ID, "error", err)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := h.webSessionRepo.EnforceLimit(r.Context(), user.ID); err != nil {
		slog.Error("session limit enforcement failed", "user_id", user.ID, "error", err)
	}

	h.sessionMgr.SetSessionCookie(w, token, h.cfg.SessionMaxAge)

	// Rotate CSRF on session creation — same fixation rationale as LoginSubmit.
	if _, err := middleware.RotateCSRFToken(w, !h.cfg.IsDev()); err != nil {
		slog.Error("csrf rotate after register failed", "user_id", user.ID, "error", err)
	}

	// New users have no gym yet — send them to setup or gym selection
	if locationID == nil {
		http.Redirect(w, r, h.postAuthRedirect(r), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// locationForHost resolves the request hostname to a location via the
// custom_domain column. Returns nil when no match is found (falls through
// to the normal membership-based location selection).
func (h *Handler) locationForHost(r *http.Request) *string {
	host := r.Host
	// Strip port if present (e.g. "localhost:8080")
	if idx := strings.LastIndex(host, ":"); idx > 0 {
		host = host[:idx]
	}
	if host == "" || host == "localhost" {
		return nil
	}

	loc, err := h.locationRepo.GetByCustomDomain(r.Context(), host)
	if err != nil {
		slog.Error("domain lookup failed", "host", host, "error", err)
		return nil
	}
	if loc == nil {
		return nil
	}
	slog.Info("domain-matched location", "host", host, "location_id", loc.ID, "name", loc.Name)
	return &loc.ID
}

// postAuthRedirect returns the URL to redirect to after login/register
// when the user has no location. If no organizations exist yet (first-run),
// send them to /setup; otherwise to /join-gym.
func (h *Handler) postAuthRedirect(r *http.Request) string {
	if h.needsSetup(r) {
		return "/setup"
	}
	return "/join-gym"
}

// registerError re-renders the registration page with an error message.
func (h *Handler) registerError(w http.ResponseWriter, r *http.Request, msg, email, displayName string) {
	tmpl, ok := h.templates["auth/register.html"]
	if !ok {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	data := struct {
		CSRFToken   string
		Email       string
		DisplayName string
		Error       string
	}{
		CSRFToken:   middleware.TokenFromRequest(r),
		Email:       email,
		DisplayName: displayName,
		Error:       msg,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnprocessableEntity)
	if err := tmpl.Execute(w, data); err != nil {
		slog.Error("register error template render failed", "error", err)
	}
}
