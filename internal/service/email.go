package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/mail"
	"net/smtp"
	"strings"

	"github.com/shotwell-paddle/routewerk/internal/jobs"
)

// EmailConfig holds SMTP connection settings.
type EmailConfig struct {
	Host     string // SMTP host (e.g. "smtp.postmarkapp.com")
	Port     string // SMTP port (e.g. "587")
	Username string // SMTP username / API key
	Password string // SMTP password / API token
	From     string // sender address (e.g. "noreply@routewerk.com")
}

// IsConfigured returns true if SMTP settings are provided.
func (c EmailConfig) IsConfigured() bool {
	return c.Host != "" && c.From != ""
}

// EmailService sends transactional emails via SMTP. In dev mode (no SMTP
// configured), it logs emails to stdout instead of sending them.
type EmailService struct {
	cfg         EmailConfig
	frontendURL string
}

// NewEmailService creates an email service. If cfg is not configured,
// emails are logged instead of sent.
func NewEmailService(cfg EmailConfig, frontendURL string) *EmailService {
	return &EmailService{cfg: cfg, frontendURL: frontendURL}
}

// ── Email Types ─────────────────────────────────────────────────

// PasswordResetPayload is the job payload for password reset emails.
type PasswordResetPayload struct {
	UserEmail   string `json:"user_email"`
	DisplayName string `json:"display_name"`
	ResetToken  string `json:"reset_token"`
}

// InvitePayload is the job payload for org invite emails.
type InvitePayload struct {
	UserEmail    string `json:"user_email"`
	InviterName  string `json:"inviter_name"`
	OrgName      string `json:"org_name"`
	LocationName string `json:"location_name"`
	InviteToken  string `json:"invite_token"`
}

// WelcomePayload is the job payload for welcome emails.
type WelcomePayload struct {
	UserEmail   string `json:"user_email"`
	DisplayName string `json:"display_name"`
}

// ── Job Handlers ────────────────────────────────────────────────

// RegisterHandlers registers email job handlers with the job queue.
func (s *EmailService) RegisterHandlers(q *jobs.Queue) {
	q.Register("email.password_reset", s.handlePasswordReset)
	q.Register("email.invite", s.handleInvite)
	q.Register("email.welcome", s.handleWelcome)
	q.Register("email.magic_link", s.handleMagicLink)
}

func (s *EmailService) handlePasswordReset(_ context.Context, job jobs.Job) error {
	var p PasswordResetPayload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	resetURL := fmt.Sprintf("%s/reset-password?token=%s", s.frontendURL, p.ResetToken)

	body, err := renderTemplate(passwordResetTmpl, map[string]string{
		"DisplayName": p.DisplayName,
		"ResetURL":    resetURL,
	})
	if err != nil {
		return err
	}

	return s.send(p.UserEmail, "Reset your Routewerk password", body)
}

func (s *EmailService) handleInvite(_ context.Context, job jobs.Job) error {
	var p InvitePayload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	inviteURL := fmt.Sprintf("%s/accept-invite?token=%s", s.frontendURL, p.InviteToken)

	body, err := renderTemplate(inviteTmpl, map[string]string{
		"InviterName":  p.InviterName,
		"OrgName":      p.OrgName,
		"LocationName": p.LocationName,
		"InviteURL":    inviteURL,
	})
	if err != nil {
		return err
	}

	return s.send(p.UserEmail, fmt.Sprintf("You've been invited to %s on Routewerk", p.OrgName), body)
}

func (s *EmailService) handleMagicLink(_ context.Context, job jobs.Job) error {
	var p MagicLinkPayload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	verifyURL := fmt.Sprintf("%s/verify-magic?token=%s", s.frontendURL, p.Token)
	if p.NextPath != "" {
		verifyURL += "&next=" + p.NextPath
	}

	body, err := renderTemplate(magicLinkTmpl, map[string]string{
		"DisplayName": p.DisplayName,
		"VerifyURL":   verifyURL,
	})
	if err != nil {
		return err
	}
	return s.send(p.UserEmail, "Sign in to Routewerk", body)
}

func (s *EmailService) handleWelcome(_ context.Context, job jobs.Job) error {
	var p WelcomePayload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	body, err := renderTemplate(welcomeTmpl, map[string]string{
		"DisplayName": p.DisplayName,
		"LoginURL":    s.frontendURL + "/login",
	})
	if err != nil {
		return err
	}

	return s.send(p.UserEmail, "Welcome to Routewerk", body)
}

// ── SMTP Sending ────────────────────────────────────────────────

func (s *EmailService) send(to, subject, htmlBody string) error {
	if !s.cfg.IsConfigured() {
		// Dev mode: log the email
		slog.Info("email (dev mode, not sent)",
			"to", to,
			"subject", subject,
			"body_length", len(htmlBody),
		)
		return nil
	}

	if _, err := mail.ParseAddress(to); err != nil {
		return fmt.Errorf("invalid recipient %q: %w", to, err)
	}

	msg := buildMIME(s.cfg.From, to, subject, htmlBody)

	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	addr := s.cfg.Host + ":" + s.cfg.Port

	if err := smtp.SendMail(addr, auth, s.cfg.From, []string{to}, msg); err != nil {
		return fmt.Errorf("smtp send to %s: %w", to, err)
	}

	slog.Info("email sent", "to", to, "subject", subject)
	return nil
}

// sanitizeHeader removes CR and LF so user-controlled values cannot inject
// additional SMTP headers or split the body. Applied to every header value
// passed to buildMIME.
func sanitizeHeader(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

func buildMIME(from, to, subject, htmlBody string) []byte {
	var buf bytes.Buffer
	buf.WriteString("From: " + sanitizeHeader(from) + "\r\n")
	buf.WriteString("To: " + sanitizeHeader(to) + "\r\n")
	buf.WriteString("Subject: " + sanitizeHeader(subject) + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(htmlBody)
	return buf.Bytes()
}

// ── Templates ───────────────────────────────────────────────────

func renderTemplate(tmplStr string, data map[string]string) (string, error) {
	tmpl, err := template.New("email").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse email template: %w", err)
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute email template: %w", err)
	}
	return buf.String(), nil
}

const passwordResetTmpl = `<!DOCTYPE html>
<html>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px; color: #1a1a1a;">
  <h2 style="margin-bottom: 16px;">Reset your password</h2>
  <p>Hi {{.DisplayName}},</p>
  <p>We received a request to reset your Routewerk password. Click the button below to choose a new one:</p>
  <p style="margin: 24px 0;">
    <a href="{{.ResetURL}}" style="background: #f97316; color: white; padding: 12px 24px; border-radius: 6px; text-decoration: none; font-weight: 600;">Reset Password</a>
  </p>
  <p style="color: #6b7280; font-size: 14px;">This link expires in 1 hour. If you didn't request this, you can safely ignore this email.</p>
</body>
</html>`

const inviteTmpl = `<!DOCTYPE html>
<html>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px; color: #1a1a1a;">
  <h2 style="margin-bottom: 16px;">You're invited!</h2>
  <p>{{.InviterName}} has invited you to join <strong>{{.OrgName}}</strong> at <strong>{{.LocationName}}</strong> on Routewerk.</p>
  <p style="margin: 24px 0;">
    <a href="{{.InviteURL}}" style="background: #f97316; color: white; padding: 12px 24px; border-radius: 6px; text-decoration: none; font-weight: 600;">Accept Invite</a>
  </p>
  <p style="color: #6b7280; font-size: 14px;">This invitation expires in 7 days.</p>
</body>
</html>`

const magicLinkTmpl = `<!DOCTYPE html>
<html>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px; color: #1a1a1a;">
  <h2 style="margin-bottom: 16px;">Sign in to Routewerk</h2>
  <p>Hi {{.DisplayName}},</p>
  <p>Click the button below to sign in. No password needed.</p>
  <p style="margin: 24px 0;">
    <a href="{{.VerifyURL}}" style="background: #f97316; color: white; padding: 12px 24px; border-radius: 6px; text-decoration: none; font-weight: 600;">Sign In</a>
  </p>
  <p style="color: #6b7280; font-size: 14px;">This link expires in 15 minutes and can only be used once. If you didn't request this, you can safely ignore this email.</p>
</body>
</html>`

const welcomeTmpl = `<!DOCTYPE html>
<html>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px; color: #1a1a1a;">
  <h2 style="margin-bottom: 16px;">Welcome to Routewerk!</h2>
  <p>Hi {{.DisplayName}},</p>
  <p>Your account is all set. Routewerk helps climbers track their sends and helps setters manage their walls.</p>
  <p style="margin: 24px 0;">
    <a href="{{.LoginURL}}" style="background: #f97316; color: white; padding: 12px 24px; border-radius: 6px; text-decoration: none; font-weight: 600;">Get Started</a>
  </p>
</body>
</html>`
