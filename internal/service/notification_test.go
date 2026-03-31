package service

import (
	"testing"

	"github.com/shotwell-paddle/routewerk/internal/repository"
)

func TestNotification_StrPtr(t *testing.T) {
	s := strPtr("hello")
	if s == nil {
		t.Fatal("strPtr returned nil")
	}
	if *s != "hello" {
		t.Errorf("*s = %q, want hello", *s)
	}
}

func TestNotification_Model(t *testing.T) {
	n := repository.Notification{
		UserID: "user-123",
		Type:   "route.rated",
		Title:  "Someone rated your route",
		Body:   "Chris gave Test Route 5 stars",
		Link:   strPtr("/routes/abc"),
	}

	if n.UserID != "user-123" {
		t.Errorf("UserID = %q", n.UserID)
	}
	if n.Type != "route.rated" {
		t.Errorf("Type = %q", n.Type)
	}
	if n.Link == nil || *n.Link != "/routes/abc" {
		t.Errorf("Link = %v", n.Link)
	}
}

func TestNewNotificationService(t *testing.T) {
	svc := NewNotificationService(nil, nil)
	if svc == nil {
		t.Fatal("NewNotificationService returned nil")
	}
}
