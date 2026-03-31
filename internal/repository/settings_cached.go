package repository

import (
	"context"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/cache"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

// CachedSettingsRepo wraps SettingsRepo with an in-process TTL cache.
// Location and org settings are read on nearly every page load but change
// rarely, making them ideal caching targets.
//
// Cache invalidation: writes call Invalidate immediately. As a safety net,
// entries expire after the configured TTL (default 5 minutes).
type CachedSettingsRepo struct {
	inner          *SettingsRepo
	locationCache  *cache.Cache[model.LocationSettings]
	orgCache       *cache.Cache[model.OrgSettings]
	userCache      *cache.Cache[model.UserSettings]
}

// NewCachedSettingsRepo creates a settings repo with in-process caching.
func NewCachedSettingsRepo(inner *SettingsRepo) *CachedSettingsRepo {
	ttl := 5 * time.Minute
	return &CachedSettingsRepo{
		inner:         inner,
		locationCache: cache.New[model.LocationSettings](ttl),
		orgCache:      cache.New[model.OrgSettings](ttl),
		userCache:     cache.New[model.UserSettings](ttl),
	}
}

// ── Location Settings ────────────────────────────────────────────

func (r *CachedSettingsRepo) GetLocationSettings(ctx context.Context, locationID string) (model.LocationSettings, error) {
	if v, ok := r.locationCache.Get(locationID); ok {
		return v, nil
	}
	settings, err := r.inner.GetLocationSettings(ctx, locationID)
	if err != nil {
		return settings, err
	}
	r.locationCache.Set(locationID, settings)
	return settings, nil
}

func (r *CachedSettingsRepo) UpdateLocationSettings(ctx context.Context, locationID string, settings model.LocationSettings) error {
	err := r.inner.UpdateLocationSettings(ctx, locationID, settings)
	if err != nil {
		return err
	}
	r.locationCache.Invalidate(locationID)
	return nil
}

// ── Organization Settings ────────────────────────────────────────

func (r *CachedSettingsRepo) GetOrgSettings(ctx context.Context, orgID string) (model.OrgSettings, error) {
	if v, ok := r.orgCache.Get(orgID); ok {
		return v, nil
	}
	settings, err := r.inner.GetOrgSettings(ctx, orgID)
	if err != nil {
		return settings, err
	}
	r.orgCache.Set(orgID, settings)
	return settings, nil
}

func (r *CachedSettingsRepo) UpdateOrgSettings(ctx context.Context, orgID string, settings model.OrgSettings) error {
	err := r.inner.UpdateOrgSettings(ctx, orgID, settings)
	if err != nil {
		return err
	}
	r.orgCache.Invalidate(orgID)
	return nil
}

// ── User Settings ────────────────────────────────────────────

func (r *CachedSettingsRepo) GetUserSettings(ctx context.Context, userID string) (model.UserSettings, error) {
	if v, ok := r.userCache.Get(userID); ok {
		return v, nil
	}
	settings, err := r.inner.GetUserSettings(ctx, userID)
	if err != nil {
		return settings, err
	}
	r.userCache.Set(userID, settings)
	return settings, nil
}

func (r *CachedSettingsRepo) UpdateUserSettings(ctx context.Context, userID string, settings model.UserSettings) error {
	err := r.inner.UpdateUserSettings(ctx, userID, settings)
	if err != nil {
		return err
	}
	r.userCache.Invalidate(userID)
	return nil
}
