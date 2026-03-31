package service

import (
	"strings"
	"testing"
)

func TestEmailConfig_IsConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  EmailConfig
		want bool
	}{
		{"fully configured", EmailConfig{Host: "smtp.example.com", Port: "587", From: "noreply@test.com"}, true},
		{"no host", EmailConfig{From: "noreply@test.com"}, false},
		{"no from", EmailConfig{Host: "smtp.example.com"}, false},
		{"empty", EmailConfig{}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.IsConfigured(); got != tc.want {
				t.Errorf("IsConfigured() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBuildMIME(t *testing.T) {
	msg := buildMIME("from@test.com", "to@test.com", "Hello", "<p>Hi</p>")
	s := string(msg)

	if !strings.Contains(s, "From: from@test.com") {
		t.Error("missing From header")
	}
	if !strings.Contains(s, "To: to@test.com") {
		t.Error("missing To header")
	}
	if !strings.Contains(s, "Subject: Hello") {
		t.Error("missing Subject header")
	}
	if !strings.Contains(s, "Content-Type: text/html") {
		t.Error("missing Content-Type header")
	}
	if !strings.Contains(s, "<p>Hi</p>") {
		t.Error("missing body")
	}
}

func TestRenderTemplate(t *testing.T) {
	tmpl := "Hello {{.Name}}, welcome to {{.App}}!"
	result, err := renderTemplate(tmpl, map[string]string{
		"Name": "Chris",
		"App":  "Routewerk",
	})
	if err != nil {
		t.Fatalf("renderTemplate: %v", err)
	}
	if result != "Hello Chris, welcome to Routewerk!" {
		t.Errorf("result = %q", result)
	}
}

func TestRenderTemplate_InvalidTemplate(t *testing.T) {
	_, err := renderTemplate("{{.Missing}", map[string]string{})
	if err == nil {
		t.Error("expected error for invalid template")
	}
}

func TestPasswordResetTemplate(t *testing.T) {
	result, err := renderTemplate(passwordResetTmpl, map[string]string{
		"DisplayName": "Chris",
		"ResetURL":    "https://routewerk.com/reset?token=abc",
	})
	if err != nil {
		t.Fatalf("renderTemplate: %v", err)
	}
	if !strings.Contains(result, "Chris") {
		t.Error("template should contain display name")
	}
	if !strings.Contains(result, "https://routewerk.com/reset?token=abc") {
		t.Error("template should contain reset URL")
	}
	if !strings.Contains(result, "Reset Password") {
		t.Error("template should contain CTA text")
	}
}

func TestInviteTemplate(t *testing.T) {
	result, err := renderTemplate(inviteTmpl, map[string]string{
		"InviterName":  "Alex",
		"OrgName":      "LEF Climbing",
		"LocationName": "Boulder Gym",
		"InviteURL":    "https://routewerk.com/invite?token=xyz",
	})
	if err != nil {
		t.Fatalf("renderTemplate: %v", err)
	}
	if !strings.Contains(result, "Alex") {
		t.Error("template should contain inviter name")
	}
	if !strings.Contains(result, "LEF Climbing") {
		t.Error("template should contain org name")
	}
}

func TestWelcomeTemplate(t *testing.T) {
	result, err := renderTemplate(welcomeTmpl, map[string]string{
		"DisplayName": "Chris",
		"LoginURL":    "https://routewerk.com/login",
	})
	if err != nil {
		t.Fatalf("renderTemplate: %v", err)
	}
	if !strings.Contains(result, "Welcome to Routewerk") {
		t.Error("template should contain welcome text")
	}
}

func TestNewEmailService(t *testing.T) {
	svc := NewEmailService(EmailConfig{}, "https://routewerk.com")
	if svc == nil {
		t.Fatal("NewEmailService returned nil")
	}
}
