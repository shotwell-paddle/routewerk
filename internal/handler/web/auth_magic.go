package webhandler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/shotwell-paddle/routewerk/internal/config"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

// MagicVerifyHandler handles GET /verify-magic — the URL the climber
// clicks in their email. On success it consumes the token, mints a web
// session (same pattern as POST /login), sets the cookie, rotates CSRF,
// and redirects to either the saved next path or /dashboard.
//
// On failure (invalid, expired, or already-consumed token) it redirects
// to /login. Phase 2 polish: a dedicated "this link no longer works,
// request a new one" page would be friendlier than a silent bounce.
//
// Standalone struct (not a method on the giant web.Handler) so the
// dependency surface stays minimal — magic-link auth doesn't need any
// of the route/wall/quest repos.
type MagicVerifyHandler struct {
	svc            *service.MagicLinkService
	webSessionRepo *repository.WebSessionRepo
	userRepo       *repository.UserRepo
	sessionMgr     *middleware.SessionManager
	cfg            *config.Config
}

func NewMagicVerifyHandler(
	svc *service.MagicLinkService,
	webSessionRepo *repository.WebSessionRepo,
	userRepo *repository.UserRepo,
	sessionMgr *middleware.SessionManager,
	cfg *config.Config,
) *MagicVerifyHandler {
	return &MagicVerifyHandler{
		svc:            svc,
		webSessionRepo: webSessionRepo,
		userRepo:       userRepo,
		sessionMgr:     sessionMgr,
		cfg:            cfg,
	}
}

// Verify handles GET /verify-magic?token=…&next=/path.
//
// Token validation, expiry, single-use, and user-still-exists checks
// all collapse into ErrMagicLinkInvalid; we redirect to /login on any
// of them rather than leak which case applied.
func (h *MagicVerifyHandler) Verify(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	next := safeRedirect(r.URL.Query().Get("next"), "/dashboard")

	user, _, err := h.svc.Consume(r.Context(), token)
	if err != nil {
		if errors.Is(err, service.ErrMagicLinkInvalid) {
			slog.Info("magic link verify: invalid or expired")
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		slog.Error("magic link verify: infrastructure error", "error", err)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := h.mintSession(r, w, user); err != nil {
		slog.Error("magic link verify: session creation failed", "user_id", user.ID, "error", err)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	slog.Info("magic link verified", "user_id", user.ID)
	http.Redirect(w, r, next, http.StatusSeeOther)
}

// mintSession creates a web session and writes the session + CSRF
// cookies. Mirrors LoginSubmit step-for-step (same expiry, same limit
// enforcement, same CSRF rotation) so the post-magic-link state is
// indistinguishable from a fresh password login.
func (h *MagicVerifyHandler) mintSession(r *http.Request, w http.ResponseWriter, user *model.User) error {
	token, tokenHash, err := middleware.GenerateSessionToken()
	if err != nil {
		return err
	}

	// Pick the user's default location — first membership with one wins.
	// Magic-link users hitting /verify-magic from an email don't have a
	// host context to drive locationForHost(), so we fall back to the
	// first membership directly.
	memberships, memErr := h.userRepo.GetMemberships(r.Context(), user.ID)
	if memErr != nil {
		// Non-fatal: a user with no memberships still gets a session
		// (they'll land on /join-gym or similar). Log and continue.
		slog.Info("magic link: load memberships failed", "user_id", user.ID, "error", memErr)
	}
	var locationID *string
	for _, m := range memberships {
		if m.LocationID != nil {
			locationID = m.LocationID
			break
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
		return err
	}
	if err := h.webSessionRepo.EnforceLimit(r.Context(), user.ID); err != nil {
		// Non-fatal: the session was created; we just couldn't trim the
		// older ones. Log and proceed.
		slog.Error("magic link: session limit enforcement failed", "user_id", user.ID, "error", err)
	}

	h.sessionMgr.SetSessionCookie(w, token, h.cfg.SessionMaxAge)

	// Rotate CSRF — same rationale as LoginSubmit: any token planted
	// before the auth event must stop being valid.
	if _, err := middleware.RotateCSRFToken(w, !h.cfg.IsDev()); err != nil {
		slog.Error("magic link: csrf rotate failed", "user_id", user.ID, "error", err)
	}
	return nil
}
