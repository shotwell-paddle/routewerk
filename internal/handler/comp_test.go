package handler

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/shotwell-paddle/routewerk/internal/api"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

// ── isUUID + isValidStatus ─────────────────────────────────

func TestIsUUID(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"00000000-0000-0000-0000-000000000000", true},
		{uuid.New().String(), true},
		{"not-a-uuid", false},
		{"", false},
		{"12345", false},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := isUUID(tc.in); got != tc.want {
				t.Errorf("isUUID(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsValidStatus(t *testing.T) {
	for _, s := range []string{"draft", "open", "live", "closed", "archived"} {
		if !isValidStatus(s) {
			t.Errorf("isValidStatus(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "DRAFT", "running", "garbage"} {
		if isValidStatus(s) {
			t.Errorf("isValidStatus(%q) = true, want false", s)
		}
	}
}

func TestSlugPattern(t *testing.T) {
	good := []string{"a", "abc", "abc-def", "abc-123", "1abc", "abcdef-2026"}
	for _, s := range good {
		if !slugPattern.MatchString(s) {
			t.Errorf("slug %q should match", s)
		}
	}
	bad := []string{"", "-abc", "ABC", "abc def", "abc.def", "abc/def", strings.Repeat("a", 65)}
	for _, s := range bad {
		if slugPattern.MatchString(s) {
			t.Errorf("slug %q should not match", s)
		}
	}
}

// ── validateCreate ─────────────────────────────────────────

func TestValidateCreate(t *testing.T) {
	now := time.Now()
	later := now.Add(2 * time.Hour)

	base := func() api.CompetitionCreate {
		return api.CompetitionCreate{
			Name:        "Spring League",
			Slug:        "spring-league",
			Format:      api.CompetitionFormat("single"),
			ScoringRule: "top_zone",
			StartsAt:    now,
			EndsAt:      later,
		}
	}

	tests := []struct {
		name    string
		mutate  func(*api.CompetitionCreate)
		wantErr string // substring; "" = expect no error
	}{
		{name: "valid", mutate: func(c *api.CompetitionCreate) {}},
		{name: "missing name", mutate: func(c *api.CompetitionCreate) { c.Name = "" }, wantErr: "name is required"},
		{name: "bad slug", mutate: func(c *api.CompetitionCreate) { c.Slug = "Bad Slug" }, wantErr: "slug"},
		{name: "missing rule", mutate: func(c *api.CompetitionCreate) { c.ScoringRule = "" }, wantErr: "scoring_rule is required"},
		{name: "ends before starts", mutate: func(c *api.CompetitionCreate) { c.EndsAt = c.StartsAt.Add(-time.Hour) }, wantErr: "ends_at must be after"},
		{name: "ends equals starts", mutate: func(c *api.CompetitionCreate) { c.EndsAt = c.StartsAt }, wantErr: "ends_at must be after"},
		{name: "bad format", mutate: func(c *api.CompetitionCreate) { c.Format = api.CompetitionFormat("league") }, wantErr: "format must be"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := base()
			tc.mutate(&c)
			err := validateCreate(&c)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("want no error, got %v", err)
			case tc.wantErr != "" && err == nil:
				t.Fatalf("want error containing %q, got nil", tc.wantErr)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// ── createToModel ──────────────────────────────────────────

func TestCreateToModel_DefaultsAndOverrides(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	cfg := map[string]interface{}{"flash_bonus": float64(50)}
	in := api.CompetitionCreate{
		Name:          "Fall League",
		Slug:          "fall-league",
		Format:        api.CompetitionFormat("series"),
		ScoringRule:   "fixed",
		ScoringConfig: &cfg,
		StartsAt:      now,
		EndsAt:        now.Add(time.Hour),
	}
	loc := uuid.New().String()
	got := createToModel(loc, &in)

	if got.LocationID != loc {
		t.Errorf("LocationID = %q, want %q", got.LocationID, loc)
	}
	if got.Status != "draft" {
		t.Errorf("Status = %q, want default 'draft'", got.Status)
	}
	if got.LeaderboardVis != "public" {
		t.Errorf("LeaderboardVis = %q, want default 'public'", got.LeaderboardVis)
	}
	if string(got.Aggregation) != "{}" {
		t.Errorf("Aggregation = %q, want '{}'", got.Aggregation)
	}
	var roundtrip map[string]interface{}
	if err := json.Unmarshal(got.ScoringConfig, &roundtrip); err != nil {
		t.Fatalf("ScoringConfig didn't roundtrip: %v (raw: %s)", err, got.ScoringConfig)
	}
	if roundtrip["flash_bonus"] != float64(50) {
		t.Errorf("ScoringConfig flash_bonus = %v, want 50", roundtrip["flash_bonus"])
	}
}

func TestCreateToModel_ExplicitStatusAndVisibility(t *testing.T) {
	status := api.CompetitionStatus("open")
	vis := api.LeaderboardVisibility("registrants")
	now := time.Now()
	in := api.CompetitionCreate{
		Name: "x", Slug: "x", Format: "single", ScoringRule: "fixed",
		StartsAt: now, EndsAt: now.Add(time.Hour),
		Status:                &status,
		LeaderboardVisibility: &vis,
	}
	got := createToModel(uuid.New().String(), &in)
	if got.Status != "open" {
		t.Errorf("Status = %q, want 'open'", got.Status)
	}
	if got.LeaderboardVis != "registrants" {
		t.Errorf("LeaderboardVis = %q, want 'registrants'", got.LeaderboardVis)
	}
}

// ── modelToAPI ─────────────────────────────────────────────

func TestModelToAPI_RoundTripsCoreFields(t *testing.T) {
	id := uuid.New().String()
	loc := uuid.New().String()
	now := time.Now().UTC().Truncate(time.Second)
	regOpens := now.Add(time.Hour)

	in := &model.Competition{
		ID:                  id,
		LocationID:          loc,
		Name:                "Comp",
		Slug:                "comp",
		Format:              "series",
		Aggregation:         json.RawMessage(`{"method":"sum_drop_n","drop":1}`),
		ScoringRule:         "decay",
		ScoringConfig:       json.RawMessage(`{"base_points":1000}`),
		Status:              "live",
		LeaderboardVis:      "public",
		StartsAt:            now,
		EndsAt:              now.Add(2 * time.Hour),
		RegistrationOpensAt: pgtype.Timestamptz{Time: regOpens, Valid: true},
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	out, err := modelToAPI(in)
	if err != nil {
		t.Fatalf("modelToAPI: %v", err)
	}
	if out.Id.String() != id {
		t.Errorf("Id = %s, want %s", out.Id, id)
	}
	if out.LocationId.String() != loc {
		t.Errorf("LocationId = %s, want %s", out.LocationId, loc)
	}
	if string(out.Format) != "series" {
		t.Errorf("Format = %q, want 'series'", out.Format)
	}
	if out.Aggregation.Method == nil || string(*out.Aggregation.Method) != "sum_drop_n" {
		t.Errorf("Aggregation.Method roundtrip failed: %+v", out.Aggregation)
	}
	if out.Aggregation.Drop == nil || *out.Aggregation.Drop != 1 {
		t.Errorf("Aggregation.Drop = %v, want 1", out.Aggregation.Drop)
	}
	if out.ScoringConfig["base_points"] != float64(1000) {
		t.Errorf("ScoringConfig base_points = %v, want 1000", out.ScoringConfig["base_points"])
	}
	if out.RegistrationOpensAt == nil || !out.RegistrationOpensAt.Equal(regOpens) {
		t.Errorf("RegistrationOpensAt = %v, want %v", out.RegistrationOpensAt, regOpens)
	}
	if out.RegistrationClosesAt != nil {
		t.Errorf("RegistrationClosesAt = %v, want nil (was unset)", out.RegistrationClosesAt)
	}
}

func TestModelToAPI_BadIDReturnsError(t *testing.T) {
	in := &model.Competition{ID: "not-a-uuid", LocationID: uuid.New().String()}
	if _, err := modelToAPI(in); err == nil {
		t.Error("expected error on non-UUID ID, got nil")
	}
}

func TestModelToAPI_EmptyAggregationDecodesAsZeroValue(t *testing.T) {
	in := &model.Competition{
		ID:          uuid.New().String(),
		LocationID:  uuid.New().String(),
		Aggregation: json.RawMessage(`{}`),
	}
	out, err := modelToAPI(in)
	if err != nil {
		t.Fatalf("modelToAPI: %v", err)
	}
	if out.Aggregation.Method != nil {
		t.Errorf("Aggregation.Method = %v, want nil for empty jsonb", out.Aggregation.Method)
	}
}

// ── applyUpdate ────────────────────────────────────────────

func TestApplyUpdate_PartialFields(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	existing := &model.Competition{
		Name:           "Old Name",
		Slug:           "old-slug",
		Format:         "single",
		ScoringRule:    "fixed",
		Status:         "draft",
		LeaderboardVis: "public",
		StartsAt:       now,
		EndsAt:         now.Add(time.Hour),
	}
	newName := "New Name"
	newStatus := api.CompetitionStatus("live")
	body := api.CompetitionUpdate{
		Name:   &newName,
		Status: &newStatus,
	}
	if err := applyUpdate(existing, &body); err != nil {
		t.Fatalf("applyUpdate: %v", err)
	}
	if existing.Name != "New Name" {
		t.Errorf("Name = %q, want 'New Name'", existing.Name)
	}
	if existing.Status != "live" {
		t.Errorf("Status = %q, want 'live'", existing.Status)
	}
	// Untouched fields should be preserved.
	if existing.Slug != "old-slug" {
		t.Errorf("Slug = %q, want untouched 'old-slug'", existing.Slug)
	}
}

func TestApplyUpdate_RejectsBadSlugAndFormat(t *testing.T) {
	now := time.Now()
	existing := &model.Competition{StartsAt: now, EndsAt: now.Add(time.Hour)}
	bad := "Not A Slug"
	body := api.CompetitionUpdate{Slug: &bad}
	if err := applyUpdate(existing, &body); err == nil {
		t.Error("expected error on bad slug, got nil")
	}

	badFmt := api.CompetitionFormat("league")
	body = api.CompetitionUpdate{Format: &badFmt}
	if err := applyUpdate(existing, &body); err == nil {
		t.Error("expected error on bad format, got nil")
	}
}

func TestApplyUpdate_RejectsEndsBeforeStarts(t *testing.T) {
	now := time.Now()
	existing := &model.Competition{StartsAt: now, EndsAt: now.Add(time.Hour)}
	earlier := now.Add(-time.Hour)
	body := api.CompetitionUpdate{EndsAt: &earlier}
	if err := applyUpdate(existing, &body); err == nil {
		t.Error("expected error when EndsAt < StartsAt, got nil")
	}
}
