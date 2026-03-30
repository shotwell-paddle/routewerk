package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

// SettingsRepo handles JSONB settings for locations and organizations.
type SettingsRepo struct {
	db *pgxpool.Pool
}

// NewSettingsRepo creates a new SettingsRepo.
func NewSettingsRepo(db *pgxpool.Pool) *SettingsRepo {
	return &SettingsRepo{db: db}
}

// ── Location Settings ────────────────────────────────────────────

// GetLocationSettings loads and parses the settings for a location.
// Returns defaults if the column is empty or unparseable.
func (r *SettingsRepo) GetLocationSettings(ctx context.Context, locationID string) (model.LocationSettings, error) {
	var raw []byte
	err := r.db.QueryRow(ctx,
		`SELECT COALESCE(settings_json, '{}'::jsonb) FROM locations WHERE id = $1 AND deleted_at IS NULL`,
		locationID,
	).Scan(&raw)
	if err != nil {
		return model.DefaultLocationSettings(), fmt.Errorf("get location settings: %w", err)
	}

	settings := model.DefaultLocationSettings()
	if len(raw) > 2 { // more than just "{}"
		if err := json.Unmarshal(raw, &settings); err != nil {
			return model.DefaultLocationSettings(), fmt.Errorf("parse location settings: %w", err)
		}
	}
	return settings, nil
}

// UpdateLocationSettings writes the full settings JSON for a location.
func (r *SettingsRepo) UpdateLocationSettings(ctx context.Context, locationID string, settings model.LocationSettings) error {
	raw, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal location settings: %w", err)
	}

	_, err = r.db.Exec(ctx,
		`UPDATE locations SET settings_json = $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL`,
		raw, locationID,
	)
	if err != nil {
		return fmt.Errorf("update location settings: %w", err)
	}
	return nil
}

// ── Organization Settings ────────────────────────────────────────

// GetOrgSettings loads and parses the settings for an organization.
func (r *SettingsRepo) GetOrgSettings(ctx context.Context, orgID string) (model.OrgSettings, error) {
	var raw []byte
	err := r.db.QueryRow(ctx,
		`SELECT COALESCE(settings_json, '{}'::jsonb) FROM organizations WHERE id = $1 AND deleted_at IS NULL`,
		orgID,
	).Scan(&raw)
	if err != nil {
		return model.DefaultOrgSettings(), fmt.Errorf("get org settings: %w", err)
	}

	settings := model.DefaultOrgSettings()
	if len(raw) > 2 {
		if err := json.Unmarshal(raw, &settings); err != nil {
			return model.DefaultOrgSettings(), fmt.Errorf("parse org settings: %w", err)
		}
	}
	return settings, nil
}

// UpdateOrgSettings writes the full settings JSON for an organization.
func (r *SettingsRepo) UpdateOrgSettings(ctx context.Context, orgID string, settings model.OrgSettings) error {
	raw, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal org settings: %w", err)
	}

	_, err = r.db.Exec(ctx,
		`UPDATE organizations SET settings_json = $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL`,
		raw, orgID,
	)
	if err != nil {
		return fmt.Errorf("update org settings: %w", err)
	}
	return nil
}

// ── User Settings ────────────────────────────────────────────

// GetUserSettings loads and parses privacy/preference settings for a user.
func (r *SettingsRepo) GetUserSettings(ctx context.Context, userID string) (model.UserSettings, error) {
	var raw []byte
	err := r.db.QueryRow(ctx,
		`SELECT COALESCE(settings_json, '{}'::jsonb) FROM users WHERE id = $1 AND deleted_at IS NULL`,
		userID,
	).Scan(&raw)
	if err != nil {
		return model.DefaultUserSettings(), fmt.Errorf("get user settings: %w", err)
	}

	settings := model.DefaultUserSettings()
	if len(raw) > 2 {
		if err := json.Unmarshal(raw, &settings); err != nil {
			return model.DefaultUserSettings(), fmt.Errorf("parse user settings: %w", err)
		}
	}
	return settings, nil
}

// UpdateUserSettings writes the full settings JSON for a user.
func (r *SettingsRepo) UpdateUserSettings(ctx context.Context, userID string, settings model.UserSettings) error {
	raw, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal user settings: %w", err)
	}

	_, err = r.db.Exec(ctx,
		`UPDATE users SET settings_json = $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL`,
		raw, userID,
	)
	if err != nil {
		return fmt.Errorf("update user settings: %w", err)
	}
	return nil
}
