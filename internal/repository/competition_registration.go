package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shotwell-paddle/routewerk/internal/database"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

var ErrRegistrationNotFound = errors.New("registration not found")

type CompetitionRegistrationRepo struct {
	db *pgxpool.Pool
}

func NewCompetitionRegistrationRepo(db *pgxpool.Pool) *CompetitionRegistrationRepo {
	return &CompetitionRegistrationRepo{db: db}
}

func (r *CompetitionRegistrationRepo) Create(ctx context.Context, reg *model.CompetitionRegistration) error {
	query := `
		INSERT INTO competition_registrations (
			competition_id, category_id, user_id, display_name,
			bib_number, waiver_signed_at, paid_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at`

	return r.db.QueryRow(ctx, query,
		reg.CompetitionID, reg.CategoryID, reg.UserID, reg.DisplayName,
		reg.BibNumber, reg.WaiverSignedAt, reg.PaidAt,
	).Scan(&reg.ID, &reg.CreatedAt)
}

func (r *CompetitionRegistrationRepo) GetByID(ctx context.Context, id string) (*model.CompetitionRegistration, error) {
	ctx, cancel := database.QueryTimeout(ctx, database.TimeoutFast)
	defer cancel()

	query := `
		SELECT id, competition_id, category_id, user_id, display_name,
			bib_number, waiver_signed_at, paid_at, withdrawn_at, created_at
		FROM competition_registrations
		WHERE id = $1`

	reg := &model.CompetitionRegistration{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&reg.ID, &reg.CompetitionID, &reg.CategoryID, &reg.UserID, &reg.DisplayName,
		&reg.BibNumber, &reg.WaiverSignedAt, &reg.PaidAt, &reg.WithdrawnAt, &reg.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRegistrationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get registration: %w", err)
	}
	return reg, nil
}

// GetByCompAndUser is the lookup the climber UI uses on page load to
// determine "is this user already registered for this comp, and if so
// what's their registration ID?".
func (r *CompetitionRegistrationRepo) GetByCompAndUser(ctx context.Context, competitionID, userID string) (*model.CompetitionRegistration, error) {
	ctx, cancel := database.QueryTimeout(ctx, database.TimeoutFast)
	defer cancel()

	query := `
		SELECT id, competition_id, category_id, user_id, display_name,
			bib_number, waiver_signed_at, paid_at, withdrawn_at, created_at
		FROM competition_registrations
		WHERE competition_id = $1 AND user_id = $2`

	reg := &model.CompetitionRegistration{}
	err := r.db.QueryRow(ctx, query, competitionID, userID).Scan(
		&reg.ID, &reg.CompetitionID, &reg.CategoryID, &reg.UserID, &reg.DisplayName,
		&reg.BibNumber, &reg.WaiverSignedAt, &reg.PaidAt, &reg.WithdrawnAt, &reg.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRegistrationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get registration by comp+user: %w", err)
	}
	return reg, nil
}

// ListByCompetition returns all registrations for a comp, including
// withdrawn ones (staff UI needs to see them). Pass categoryID = "" to
// list across all categories.
func (r *CompetitionRegistrationRepo) ListByCompetition(ctx context.Context, competitionID, categoryID string) ([]model.CompetitionRegistration, error) {
	ctx, cancel := database.QueryTimeout(ctx, database.TimeoutFast)
	defer cancel()

	var (
		rows pgx.Rows
		err  error
	)
	base := `
		SELECT id, competition_id, category_id, user_id, display_name,
			bib_number, waiver_signed_at, paid_at, withdrawn_at, created_at
		FROM competition_registrations
		WHERE competition_id = $1`
	if categoryID == "" {
		rows, err = r.db.Query(ctx, base+` ORDER BY created_at`, competitionID)
	} else {
		rows, err = r.db.Query(ctx, base+` AND category_id = $2 ORDER BY created_at`, competitionID, categoryID)
	}
	if err != nil {
		return nil, fmt.Errorf("list registrations: %w", err)
	}
	defer rows.Close()

	var out []model.CompetitionRegistration
	for rows.Next() {
		var reg model.CompetitionRegistration
		if err := rows.Scan(
			&reg.ID, &reg.CompetitionID, &reg.CategoryID, &reg.UserID, &reg.DisplayName,
			&reg.BibNumber, &reg.WaiverSignedAt, &reg.PaidAt, &reg.WithdrawnAt, &reg.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan registration: %w", err)
		}
		out = append(out, reg)
	}
	return out, rows.Err()
}

// Withdraw marks a registration as withdrawn (does not delete). Frees the
// bib_number for reuse via the partial unique index on the table.
func (r *CompetitionRegistrationRepo) Withdraw(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE competition_registrations
		    SET withdrawn_at = now()
		  WHERE id = $1 AND withdrawn_at IS NULL`,
		id,
	)
	if err != nil {
		return fmt.Errorf("withdraw registration: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrRegistrationNotFound
	}
	return nil
}

// UpdateBib sets a new bib number. Returns an error if the bib clashes
// with another active registration in the same comp (Postgres surfaces
// the unique-index violation; caller can convert to a friendly message).
func (r *CompetitionRegistrationRepo) UpdateBib(ctx context.Context, id string, bib *int) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE competition_registrations SET bib_number = $2 WHERE id = $1`,
		id, bib,
	)
	if err != nil {
		return fmt.Errorf("update bib: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrRegistrationNotFound
	}
	return nil
}
