package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

type TrainingRepo struct {
	db *pgxpool.Pool
}

func NewTrainingRepo(db *pgxpool.Pool) *TrainingRepo {
	return &TrainingRepo{db: db}
}

func (r *TrainingRepo) Create(ctx context.Context, p *model.TrainingPlan) error {
	query := `
		INSERT INTO training_plans (coach_id, climber_id, location_id, name, description, active)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at`

	return r.db.QueryRow(ctx, query,
		p.CoachID, p.ClimberID, p.LocationID, p.Name, p.Description, p.Active,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func (r *TrainingRepo) GetByID(ctx context.Context, id string) (*model.TrainingPlan, error) {
	// Single query with LEFT JOIN to load plan + items in one round trip
	query := `
		SELECT p.id, p.coach_id, p.climber_id, p.location_id, p.name, p.description, p.active, p.created_at, p.updated_at,
			i.id, i.plan_id, i.route_id, i.sort_order, i.title, i.notes, i.completed, i.completed_at, i.created_at
		FROM training_plans p
		LEFT JOIN training_plan_items i ON i.plan_id = p.id
		WHERE p.id = $1
		ORDER BY i.sort_order`

	rows, err := r.db.Query(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("get training plan: %w", err)
	}
	defer rows.Close()

	var p *model.TrainingPlan
	for rows.Next() {
		var itemID *string
		var itemPlanID, itemTitle *string
		var itemRouteID, itemNotes *string
		var itemSortOrder *int
		var itemCompleted *bool
		var itemCompletedAt *time.Time
		var itemCreatedAt *time.Time

		if p == nil {
			p = &model.TrainingPlan{}
		}

		if err := rows.Scan(
			&p.ID, &p.CoachID, &p.ClimberID, &p.LocationID,
			&p.Name, &p.Description, &p.Active, &p.CreatedAt, &p.UpdatedAt,
			&itemID, &itemPlanID, &itemRouteID, &itemSortOrder, &itemTitle,
			&itemNotes, &itemCompleted, &itemCompletedAt, &itemCreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan training plan: %w", err)
		}

		if itemID != nil {
			item := model.TrainingPlanItem{
				ID:          *itemID,
				PlanID:      *itemPlanID,
				RouteID:     itemRouteID,
				SortOrder:   *itemSortOrder,
				Title:       *itemTitle,
				Notes:       itemNotes,
				Completed:   *itemCompleted,
				CompletedAt: itemCompletedAt,
				CreatedAt:   *itemCreatedAt,
			}
			p.Items = append(p.Items, item)
		}
	}

	return p, nil
}

func (r *TrainingRepo) ListByClimber(ctx context.Context, climberID string) ([]model.TrainingPlan, error) {
	query := `
		SELECT id, coach_id, climber_id, location_id, name, description, active, created_at, updated_at
		FROM training_plans
		WHERE climber_id = $1
		ORDER BY active DESC, updated_at DESC`

	return r.queryPlans(ctx, query, climberID)
}

func (r *TrainingRepo) ListByCoach(ctx context.Context, coachID string) ([]model.TrainingPlan, error) {
	query := `
		SELECT id, coach_id, climber_id, location_id, name, description, active, created_at, updated_at
		FROM training_plans
		WHERE coach_id = $1
		ORDER BY active DESC, updated_at DESC`

	return r.queryPlans(ctx, query, coachID)
}

func (r *TrainingRepo) ListByLocation(ctx context.Context, locationID string) ([]model.TrainingPlan, error) {
	query := `
		SELECT id, coach_id, climber_id, location_id, name, description, active, created_at, updated_at
		FROM training_plans
		WHERE location_id = $1
		ORDER BY active DESC, updated_at DESC`

	return r.queryPlans(ctx, query, locationID)
}

func (r *TrainingRepo) Update(ctx context.Context, p *model.TrainingPlan) error {
	query := `
		UPDATE training_plans
		SET name = $2, description = $3, active = $4
		WHERE id = $1
		RETURNING updated_at`

	return r.db.QueryRow(ctx, query,
		p.ID, p.Name, p.Description, p.Active,
	).Scan(&p.UpdatedAt)
}

func (r *TrainingRepo) AddItem(ctx context.Context, item *model.TrainingPlanItem) error {
	query := `
		INSERT INTO training_plan_items (plan_id, route_id, sort_order, title, notes)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`

	return r.db.QueryRow(ctx, query,
		item.PlanID, item.RouteID, item.SortOrder, item.Title, item.Notes,
	).Scan(&item.ID, &item.CreatedAt)
}

func (r *TrainingRepo) UpdateItem(ctx context.Context, itemID string, completed bool, title, notes *string) error {
	if completed {
		query := `UPDATE training_plan_items SET completed = true, completed_at = NOW() WHERE id = $1`
		_, err := r.db.Exec(ctx, query, itemID)
		return err
	}

	query := `UPDATE training_plan_items SET completed = false, completed_at = NULL`
	args := []interface{}{}
	argN := 1

	if title != nil {
		query += fmt.Sprintf(", title = $%d", argN)
		args = append(args, *title)
		argN++
	}
	if notes != nil {
		query += fmt.Sprintf(", notes = $%d", argN)
		args = append(args, *notes)
		argN++
	}

	query += fmt.Sprintf(" WHERE id = $%d", argN)
	args = append(args, itemID)

	_, err := r.db.Exec(ctx, query, args...)
	return err
}

func (r *TrainingRepo) GetItems(ctx context.Context, planID string) ([]model.TrainingPlanItem, error) {
	query := `
		SELECT id, plan_id, route_id, sort_order, title, notes, completed, completed_at, created_at
		FROM training_plan_items
		WHERE plan_id = $1
		ORDER BY sort_order`

	rows, err := r.db.Query(ctx, query, planID)
	if err != nil {
		return nil, fmt.Errorf("get items: %w", err)
	}
	defer rows.Close()

	var items []model.TrainingPlanItem
	for rows.Next() {
		var item model.TrainingPlanItem
		if err := rows.Scan(
			&item.ID, &item.PlanID, &item.RouteID, &item.SortOrder,
			&item.Title, &item.Notes, &item.Completed, &item.CompletedAt, &item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan item: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *TrainingRepo) queryPlans(ctx context.Context, query, id string) ([]model.TrainingPlan, error) {
	rows, err := r.db.Query(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("list training plans: %w", err)
	}
	defer rows.Close()

	var plans []model.TrainingPlan
	for rows.Next() {
		var p model.TrainingPlan
		if err := rows.Scan(
			&p.ID, &p.CoachID, &p.ClimberID, &p.LocationID,
			&p.Name, &p.Description, &p.Active, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan plan: %w", err)
		}
		plans = append(plans, p)
	}
	return plans, rows.Err()
}
