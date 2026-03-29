package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

type SessionRepo struct {
	db *pgxpool.Pool
}

func NewSessionRepo(db *pgxpool.Pool) *SessionRepo {
	return &SessionRepo{db: db}
}

func (r *SessionRepo) Create(ctx context.Context, s *model.SettingSession) error {
	query := `
		INSERT INTO setting_sessions (location_id, scheduled_date, notes, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`

	return r.db.QueryRow(ctx, query,
		s.LocationID, s.ScheduledDate, s.Notes, s.CreatedBy,
	).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)
}

func (r *SessionRepo) GetByID(ctx context.Context, id string) (*model.SettingSession, error) {
	query := `
		SELECT id, location_id, scheduled_date, notes, created_by, created_at, updated_at
		FROM setting_sessions
		WHERE id = $1`

	s := &model.SettingSession{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&s.ID, &s.LocationID, &s.ScheduledDate, &s.Notes,
		&s.CreatedBy, &s.CreatedAt, &s.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	assignments, err := r.GetAssignments(ctx, s.ID)
	if err != nil {
		return nil, err
	}
	s.Assignments = assignments

	return s, nil
}

func (r *SessionRepo) ListByLocation(ctx context.Context, locationID string, limit, offset int) ([]model.SettingSession, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT id, location_id, scheduled_date, notes, created_by, created_at, updated_at
		FROM setting_sessions
		WHERE location_id = $1
		ORDER BY scheduled_date DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.Query(ctx, query, locationID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []model.SettingSession
	for rows.Next() {
		var s model.SettingSession
		if err := rows.Scan(
			&s.ID, &s.LocationID, &s.ScheduledDate, &s.Notes,
			&s.CreatedBy, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func (r *SessionRepo) Update(ctx context.Context, s *model.SettingSession) error {
	query := `
		UPDATE setting_sessions
		SET scheduled_date = $2, notes = $3
		WHERE id = $1
		RETURNING updated_at`

	return r.db.QueryRow(ctx, query,
		s.ID, s.ScheduledDate, s.Notes,
	).Scan(&s.UpdatedAt)
}

func (r *SessionRepo) AddAssignment(ctx context.Context, a *model.SettingSessionAssignment) error {
	query := `
		INSERT INTO setting_session_assignments (session_id, setter_id, wall_id, target_grades, notes)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`

	return r.db.QueryRow(ctx, query,
		a.SessionID, a.SetterID, a.WallID, a.TargetGrades, a.Notes,
	).Scan(&a.ID)
}

func (r *SessionRepo) RemoveAssignment(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, "DELETE FROM setting_session_assignments WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("remove assignment: %w", err)
	}
	return nil
}

func (r *SessionRepo) GetAssignments(ctx context.Context, sessionID string) ([]model.SettingSessionAssignment, error) {
	query := `
		SELECT id, session_id, setter_id, wall_id, target_grades, notes
		FROM setting_session_assignments
		WHERE session_id = $1`

	rows, err := r.db.Query(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get assignments: %w", err)
	}
	defer rows.Close()

	var assignments []model.SettingSessionAssignment
	for rows.Next() {
		var a model.SettingSessionAssignment
		if err := rows.Scan(
			&a.ID, &a.SessionID, &a.SetterID, &a.WallID, &a.TargetGrades, &a.Notes,
		); err != nil {
			return nil, fmt.Errorf("scan assignment: %w", err)
		}
		assignments = append(assignments, a)
	}
	return assignments, nil
}
