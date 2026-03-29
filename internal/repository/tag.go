package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

type TagRepo struct {
	db *pgxpool.Pool
}

func NewTagRepo(db *pgxpool.Pool) *TagRepo {
	return &TagRepo{db: db}
}

func (r *TagRepo) Create(ctx context.Context, t *model.Tag) error {
	query := `
		INSERT INTO tags (org_id, category, name, color)
		VALUES ($1, $2, $3, $4)
		RETURNING id`

	return r.db.QueryRow(ctx, query,
		t.OrgID, t.Category, t.Name, t.Color,
	).Scan(&t.ID)
}

func (r *TagRepo) ListByOrg(ctx context.Context, orgID string, category string) ([]model.Tag, error) {
	var query string
	var args []interface{}

	if category != "" {
		query = `SELECT id, org_id, category, name, color FROM tags WHERE org_id = $1 AND category = $2 ORDER BY category, name`
		args = []interface{}{orgID, category}
	} else {
		query = `SELECT id, org_id, category, name, color FROM tags WHERE org_id = $1 ORDER BY category, name`
		args = []interface{}{orgID}
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()

	var tags []model.Tag
	for rows.Next() {
		var t model.Tag
		if err := rows.Scan(&t.ID, &t.OrgID, &t.Category, &t.Name, &t.Color); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tags = append(tags, t)
	}
	return tags, nil
}

func (r *TagRepo) Delete(ctx context.Context, id string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Remove from all routes first
	if _, err := tx.Exec(ctx, "DELETE FROM route_tags WHERE tag_id = $1", id); err != nil {
		return fmt.Errorf("remove tag from routes: %w", err)
	}

	if _, err := tx.Exec(ctx, "DELETE FROM tags WHERE id = $1", id); err != nil {
		return fmt.Errorf("delete tag: %w", err)
	}

	return tx.Commit(ctx)
}
