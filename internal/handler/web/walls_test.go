package webhandler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/rbac"
)

// ── parseWallForm ──────────────────────────────────────────────────

func TestParseWallForm_TrimsAndCopies(t *testing.T) {
	form := url.Values{
		"name":          {"  The Cave  "},
		"wall_type":     {"boulder"},
		"angle":         {" 45° "},
		"height_meters": {" 4.5 "},
		"num_anchors":   {"3"},
		"surface_type":  {"plywood"},
		"sort_order":    {"2"},
	}
	r := httptest.NewRequest(http.MethodPost, "/walls/new", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_ = r.ParseForm()

	fv := parseWallForm(r)

	cases := []struct {
		got, want, name string
	}{
		{fv.Name, "The Cave", "Name"},
		{fv.WallType, "boulder", "WallType"},
		{fv.Angle, "45°", "Angle"},
		{fv.HeightMeters, "4.5", "HeightMeters"},
		{fv.NumAnchors, "3", "NumAnchors"},
		{fv.SurfaceType, "plywood", "SurfaceType"},
		{fv.SortOrder, "2", "SortOrder"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

// ── wallFromFormValues ─────────────────────────────────────────────

func TestWallFromFormValues_OptionalFields(t *testing.T) {
	t.Run("all populated", func(t *testing.T) {
		fv := WallFormValues{
			Name:         "Wall A",
			WallType:     "route",
			Angle:        "vertical",
			HeightMeters: "12.5",
			NumAnchors:   "4",
			SurfaceType:  "fiberglass",
			SortOrder:    "7",
		}
		w := wallFromFormValues("loc-1", fv)
		if w.LocationID != "loc-1" || w.Name != "Wall A" || w.WallType != "route" {
			t.Errorf("basic fields wrong: %+v", w)
		}
		if w.Angle == nil || *w.Angle != "vertical" {
			t.Errorf("Angle pointer wrong: %v", w.Angle)
		}
		if w.HeightMeters == nil || *w.HeightMeters != 12.5 {
			t.Errorf("HeightMeters wrong: %v", w.HeightMeters)
		}
		if w.NumAnchors == nil || *w.NumAnchors != 4 {
			t.Errorf("NumAnchors wrong: %v", w.NumAnchors)
		}
		if w.SurfaceType == nil || *w.SurfaceType != "fiberglass" {
			t.Errorf("SurfaceType wrong: %v", w.SurfaceType)
		}
		if w.SortOrder != 7 {
			t.Errorf("SortOrder = %d, want 7", w.SortOrder)
		}
	})

	t.Run("optional fields absent", func(t *testing.T) {
		w := wallFromFormValues("loc-1", WallFormValues{Name: "Bare", WallType: "boulder"})
		if w.Angle != nil || w.HeightMeters != nil || w.NumAnchors != nil || w.SurfaceType != nil {
			t.Errorf("expected nil optional pointers, got %+v", w)
		}
		if w.SortOrder != 0 {
			t.Errorf("SortOrder default = %d, want 0", w.SortOrder)
		}
	})

	t.Run("malformed numerics silently dropped", func(t *testing.T) {
		w := wallFromFormValues("loc-1", WallFormValues{
			Name: "N", WallType: "boulder",
			HeightMeters: "not-a-number",
			NumAnchors:   "NaN",
			SortOrder:    "abc",
		})
		if w.HeightMeters != nil {
			t.Errorf("HeightMeters should be nil on parse fail, got %v", *w.HeightMeters)
		}
		if w.NumAnchors != nil {
			t.Errorf("NumAnchors should be nil on parse fail, got %v", *w.NumAnchors)
		}
		if w.SortOrder != 0 {
			t.Errorf("SortOrder should default to 0 on parse fail, got %d", w.SortOrder)
		}
	})
}

// ── requireHeadSetter ──────────────────────────────────────────────

func TestRequireHeadSetter(t *testing.T) {
	h := &Handler{} // zero-value is fine; helper reads only request context

	tests := []struct {
		name       string
		role       string
		htmx       bool
		wantAllow  bool
		wantStatus int
	}{
		{"climber blocked", rbac.RoleClimber, false, false, http.StatusForbidden},
		{"setter blocked", rbac.RoleSetter, false, false, http.StatusForbidden},
		{"head_setter allowed", rbac.RoleHeadSetter, false, true, http.StatusOK},
		{"gym_manager allowed", rbac.RoleGymManager, false, true, http.StatusOK},
		{"org_admin allowed", rbac.RoleOrgAdmin, false, true, http.StatusOK},
		{"unknown role blocked", "", false, false, http.StatusForbidden},
		{"htmx setter gets HX-Redirect", rbac.RoleSetter, true, false, http.StatusForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/walls/w1/archive", nil)
			ctx := middleware.SetWebRole(r.Context(), tc.role)
			r = r.WithContext(ctx)
			if tc.htmx {
				r.Header.Set("HX-Request", "true")
			}
			w := httptest.NewRecorder()

			got := h.requireHeadSetter(w, r)
			if got != tc.wantAllow {
				t.Fatalf("requireHeadSetter = %v, want %v", got, tc.wantAllow)
			}
			if !tc.wantAllow && w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if tc.htmx && !tc.wantAllow {
				if w.Header().Get("HX-Redirect") == "" {
					t.Error("expected HX-Redirect header for HTMX forbidden response")
				}
			}
		})
	}
}
