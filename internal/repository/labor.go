package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

type LaborRepo struct {
	db *pgxpool.Pool
}

func NewLaborRepo(db *pgxpool.Pool) *LaborRepo {
	return &LaborRepo{db: db}
}

func (r *LaborRepo) Create(ctx context.Context, l *model.SetterLaborLog) error {
	query := `
		INSERT INTO setter_labor_logs (user_id, location_id, session_id, date, hours_worked, routes_set, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`

	return r.db.QueryRow(ctx, query,
		l.UserID, l.LocationID, l.SessionID, l.Date,
		l.HoursWorked, l.RoutesSet, l.Notes,
	).Scan(&l.ID, &l.CreatedAt, &l.UpdatedAt)
}

func (r *LaborRepo) ListByLocation(ctx context.Context, locationID string, limit, offset int) ([]LaborWithUser, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT l.id, l.user_id, l.location_id, l.session_id, l.date, l.hours_worked,
			l.routes_set, l.notes, l.created_at, l.updated_at,
			u.display_name
		FROM setter_labor_logs l
		JOIN users u ON u.id = l.user_id
		WHERE l.location_id = $1
		ORDER BY l.date DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.Query(ctx, query, locationID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list labor: %w", err)
	}
	defer rows.Close()

	var logs []LaborWithUser
	for rows.Next() {
		var l LaborWithUser
		if err := rows.Scan(
			&l.ID, &l.UserID, &l.LocationID, &l.SessionID, &l.Date,
			&l.HoursWorked, &l.RoutesSet, &l.Notes,
			&l.CreatedAt, &l.UpdatedAt, &l.SetterName,
		); err != nil {
			return nil, fmt.Errorf("scan labor: %w", err)
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func (r *LaborRepo) ListBySetter(ctx context.Context, userID string, limit, offset int) ([]model.SetterLaborLog, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT id, user_id, location_id, session_id, date, hours_worked, routes_set, notes, created_at, updated_at
		FROM setter_labor_logs
		WHERE user_id = $1
		ORDER BY date DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.Query(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list setter labor: %w", err)
	}
	defer rows.Close()

	var logs []model.SetterLaborLog
	for rows.Next() {
		var l model.SetterLaborLog
		if err := rows.Scan(
			&l.ID, &l.UserID, &l.LocationID, &l.SessionID, &l.Date,
			&l.HoursWorked, &l.RoutesSet, &l.Notes, &l.CreatedAt, &l.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan labor: %w", err)
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

type LaborWithUser struct {
	model.SetterLaborLog
	SetterName string `json:"setter_name"`
}
