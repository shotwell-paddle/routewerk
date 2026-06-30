package handler

import (
	"testing"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/model"
)

func strp(s string) *string { return &s }

func TestBuildSessionRoute(t *testing.T) {
	session := &model.SettingSession{
		ID:            "sess-1",
		LocationID:    "loc-1",
		ScheduledDate: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	}

	tests := []struct {
		name    string
		req     sessionRouteRequest
		wantErr bool
		check   func(t *testing.T, rt *model.Route)
	}{
		{
			name: "valid boulder draft",
			req: sessionRouteRequest{
				WallID:        "wall-1",
				RouteType:     "boulder",
				GradingSystem: "V-scale",
				Grade:         "V4",
				Color:         "#ff0000",
				SetterID:      strp("setter-9"),
				Name:          strp("Crimpy Corner"),
			},
			check: func(t *testing.T, rt *model.Route) {
				if rt.Status != "draft" {
					t.Errorf("status = %q, want draft", rt.Status)
				}
				if rt.SessionID == nil || *rt.SessionID != "sess-1" {
					t.Errorf("session_id = %v, want sess-1", rt.SessionID)
				}
				if rt.LocationID != "loc-1" {
					t.Errorf("location_id = %q, want loc-1", rt.LocationID)
				}
				if !rt.DateSet.Equal(session.ScheduledDate) {
					t.Errorf("date_set = %v, want %v", rt.DateSet, session.ScheduledDate)
				}
				if rt.SetterID == nil || *rt.SetterID != "setter-9" {
					t.Errorf("setter_id = %v, want setter-9", rt.SetterID)
				}
				if rt.Name == nil || *rt.Name != "Crimpy Corner" {
					t.Errorf("name = %v, want Crimpy Corner", rt.Name)
				}
			},
		},
		{
			name: "circuit boulder keeps circuit_color",
			req: sessionRouteRequest{
				WallID:        "wall-2",
				RouteType:     "boulder",
				GradingSystem: "circuit",
				Grade:         "Red",
				Color:         "#000000",
				CircuitColor:  strp("Red"),
			},
			check: func(t *testing.T, rt *model.Route) {
				if rt.CircuitColor == nil || *rt.CircuitColor != "Red" {
					t.Errorf("circuit_color = %v, want Red", rt.CircuitColor)
				}
			},
		},
		{
			name: "blank setter_id normalizes to nil",
			req: sessionRouteRequest{
				WallID:        "wall-1",
				RouteType:     "route",
				GradingSystem: "YDS",
				Grade:         "5.10-",
				Color:         "#00ff00",
				SetterID:      strp("   "),
				Name:          strp(""),
			},
			check: func(t *testing.T, rt *model.Route) {
				if rt.SetterID != nil {
					t.Errorf("setter_id = %v, want nil", rt.SetterID)
				}
				if rt.Name != nil {
					t.Errorf("name = %v, want nil", rt.Name)
				}
			},
		},
		{
			name:    "missing wall_id is rejected",
			req:     sessionRouteRequest{RouteType: "boulder", GradingSystem: "V-scale", Grade: "V2", Color: "#fff"},
			wantErr: true,
		},
		{
			name:    "missing grade is rejected",
			req:     sessionRouteRequest{WallID: "wall-1", RouteType: "boulder", GradingSystem: "V-scale", Color: "#fff"},
			wantErr: true,
		},
		{
			name:    "missing color is rejected",
			req:     sessionRouteRequest{WallID: "wall-1", RouteType: "boulder", GradingSystem: "V-scale", Grade: "V2"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, msg := buildSessionRoute(tt.req, session)
			if tt.wantErr {
				if msg == "" {
					t.Fatalf("expected error message, got none")
				}
				if rt != nil {
					t.Fatalf("expected nil route on error, got %+v", rt)
				}
				return
			}
			if msg != "" {
				t.Fatalf("unexpected error: %s", msg)
			}
			if rt == nil {
				t.Fatal("expected route, got nil")
			}
			if tt.check != nil {
				tt.check(t, rt)
			}
		})
	}
}
