package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

type DifficultyRepo struct {
	db *pgxpool.Pool
}

func NewDifficultyRepo(db *pgxpool.Pool) *DifficultyRepo {
	return &DifficultyRepo{db: db}
}

// Upsert records or updates a user's difficulty vote on a route.
func (r *DifficultyRepo) Upsert(ctx context.Context, v *model.DifficultyVote) error {
	query := `
		INSERT INTO difficulty_votes (user_id, route_id, vote)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, route_id)
		DO UPDATE SET vote = $3, updated_at = NOW()
		RETURNING id, created_at, updated_at`

	err := r.db.QueryRow(ctx, query,
		v.UserID, v.RouteID, v.Vote,
	).Scan(&v.ID, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert difficulty vote: %w", err)
	}
	return nil
}

// GetByUserAndRoute returns the user's existing difficulty vote for a route, or nil.
func (r *DifficultyRepo) GetByUserAndRoute(ctx context.Context, userID, routeID string) (*model.DifficultyVote, error) {
	query := `SELECT id, user_id, route_id, vote, created_at, updated_at
		FROM difficulty_votes WHERE user_id = $1 AND route_id = $2`

	var v model.DifficultyVote
	err := r.db.QueryRow(ctx, query, userID, routeID).Scan(
		&v.ID, &v.UserID, &v.RouteID, &v.Vote, &v.CreatedAt, &v.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get difficulty vote: %w", err)
	}
	return &v, nil
}

// RouteCounts returns the tally of easy/right/hard votes for a route.
func (r *DifficultyRepo) RouteCounts(ctx context.Context, routeID string) (*DifficultyTally, error) {
	query := `
		SELECT
			COUNT(*) FILTER (WHERE vote = 'easy')  AS easy,
			COUNT(*) FILTER (WHERE vote = 'right') AS "right",
			COUNT(*) FILTER (WHERE vote = 'hard')  AS hard
		FROM difficulty_votes
		WHERE route_id = $1`

	t := &DifficultyTally{}
	err := r.db.QueryRow(ctx, query, routeID).Scan(&t.Easy, &t.Right, &t.Hard)
	if err != nil {
		return nil, fmt.Errorf("difficulty counts: %w", err)
	}
	t.Total = t.Easy + t.Right + t.Hard
	return t, nil
}

// DifficultyTally holds aggregate vote counts for a route.
type DifficultyTally struct {
	Easy  int `json:"easy"`
	Right int `json:"right"`
	Hard  int `json:"hard"`
	Total int `json:"total"`
}
