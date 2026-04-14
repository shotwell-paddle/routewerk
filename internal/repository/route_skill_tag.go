package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

// RouteSkillTagRepo manages the route_skill_tags table — free-form skill
// labels attached to routes for quest matching (e.g. "slab", "overhang",
// "crimps", "dynamic"). These are separate from the org-level tag system
// used for route categorization.
type RouteSkillTagRepo struct {
	db *pgxpool.Pool
}

func NewRouteSkillTagRepo(db *pgxpool.Pool) *RouteSkillTagRepo {
	return &RouteSkillTagRepo{db: db}
}

// SetTags replaces all skill tags for a route in a single transaction.
// Pass an empty slice to clear all tags.
func (r *RouteSkillTagRepo) SetTags(ctx context.Context, routeID string, tags []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM route_skill_tags WHERE route_id = $1`, routeID); err != nil {
		return fmt.Errorf("clear skill tags: %w", err)
	}

	for _, tag := range tags {
		if _, err := tx.Exec(ctx,
			`INSERT INTO route_skill_tags (route_id, tag) VALUES ($1, $2)`,
			routeID, tag,
		); err != nil {
			return fmt.Errorf("insert skill tag %q: %w", tag, err)
		}
	}

	return tx.Commit(ctx)
}

// GetTags returns all skill tags for a single route.
func (r *RouteSkillTagRepo) GetTags(ctx context.Context, routeID string) ([]model.RouteSkillTag, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, route_id, tag FROM route_skill_tags WHERE route_id = $1 ORDER BY tag`,
		routeID,
	)
	if err != nil {
		return nil, fmt.Errorf("get skill tags: %w", err)
	}
	defer rows.Close()

	var tags []model.RouteSkillTag
	for rows.Next() {
		var t model.RouteSkillTag
		if err := rows.Scan(&t.ID, &t.RouteID, &t.Tag); err != nil {
			return nil, fmt.Errorf("scan skill tag: %w", err)
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

// GetTagStrings returns just the tag names for a route (convenience).
func (r *RouteSkillTagRepo) GetTagStrings(ctx context.Context, routeID string) ([]string, error) {
	rows, err := r.db.Query(ctx,
		`SELECT tag FROM route_skill_tags WHERE route_id = $1 ORDER BY tag`,
		routeID,
	)
	if err != nil {
		return nil, fmt.Errorf("get skill tag strings: %w", err)
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, fmt.Errorf("scan skill tag string: %w", err)
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

// ListRoutesByTag returns route IDs that have a given skill tag at a location.
func (r *RouteSkillTagRepo) ListRoutesByTag(ctx context.Context, locationID, tag string) ([]string, error) {
	rows, err := r.db.Query(ctx,
		`SELECT rst.route_id
		 FROM route_skill_tags rst
		 JOIN routes ro ON ro.id = rst.route_id
		 WHERE ro.location_id = $1 AND rst.tag = $2`,
		locationID, tag,
	)
	if err != nil {
		return nil, fmt.Errorf("list routes by skill tag: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan route id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// TagCoverage returns a map of tag → count for all skill tags at a location.
// Useful for the admin dashboard to see which skills are well-represented.
func (r *RouteSkillTagRepo) TagCoverage(ctx context.Context, locationID string) (map[string]int, error) {
	rows, err := r.db.Query(ctx,
		`SELECT rst.tag, COUNT(*)
		 FROM route_skill_tags rst
		 JOIN routes ro ON ro.id = rst.route_id
		 WHERE ro.location_id = $1
		 GROUP BY rst.tag
		 ORDER BY COUNT(*) DESC`,
		locationID,
	)
	if err != nil {
		return nil, fmt.Errorf("tag coverage: %w", err)
	}
	defer rows.Close()

	coverage := make(map[string]int)
	for rows.Next() {
		var tag string
		var count int
		if err := rows.Scan(&tag, &count); err != nil {
			return nil, fmt.Errorf("scan coverage: %w", err)
		}
		coverage[tag] = count
	}
	return coverage, rows.Err()
}

// AllTagsForLocation returns distinct skill tags used at a location, sorted alphabetically.
func (r *RouteSkillTagRepo) AllTagsForLocation(ctx context.Context, locationID string) ([]string, error) {
	rows, err := r.db.Query(ctx,
		`SELECT DISTINCT rst.tag
		 FROM route_skill_tags rst
		 JOIN routes ro ON ro.id = rst.route_id
		 WHERE ro.location_id = $1
		 ORDER BY rst.tag`,
		locationID,
	)
	if err != nil {
		return nil, fmt.Errorf("all tags for location: %w", err)
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}
