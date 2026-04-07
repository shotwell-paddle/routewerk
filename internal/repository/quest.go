package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

type QuestRepo struct {
	db *pgxpool.Pool
}

func NewQuestRepo(db *pgxpool.Pool) *QuestRepo {
	return &QuestRepo{db: db}
}

// ============================================================
// Quest Domains
// ============================================================

func (r *QuestRepo) CreateDomain(ctx context.Context, d *model.QuestDomain) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO quest_domains (location_id, name, description, color, icon, sort_order)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, created_at, updated_at`,
		d.LocationID, d.Name, d.Description, d.Color, d.Icon, d.SortOrder,
	).Scan(&d.ID, &d.CreatedAt, &d.UpdatedAt)
}

func (r *QuestRepo) UpdateDomain(ctx context.Context, d *model.QuestDomain) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE quest_domains
		 SET name = $1, description = $2, color = $3, icon = $4, sort_order = $5, updated_at = NOW()
		 WHERE id = $6 AND location_id = $7`,
		d.Name, d.Description, d.Color, d.Icon, d.SortOrder, d.ID, d.LocationID,
	)
	if err != nil {
		return fmt.Errorf("update quest domain: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("quest domain %s not found", d.ID)
	}
	return nil
}

func (r *QuestRepo) DeleteDomain(ctx context.Context, id, locationID string) error {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM quest_domains WHERE id = $1 AND location_id = $2`,
		id, locationID,
	)
	if err != nil {
		return fmt.Errorf("delete quest domain: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("quest domain %s not found", id)
	}
	return nil
}

func (r *QuestRepo) ListDomains(ctx context.Context, locationID string) ([]model.QuestDomain, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, location_id, name, description, color, icon, sort_order, created_at, updated_at
		 FROM quest_domains
		 WHERE location_id = $1
		 ORDER BY sort_order, name`,
		locationID,
	)
	if err != nil {
		return nil, fmt.Errorf("list quest domains: %w", err)
	}
	defer rows.Close()

	var out []model.QuestDomain
	for rows.Next() {
		var d model.QuestDomain
		if err := rows.Scan(&d.ID, &d.LocationID, &d.Name, &d.Description, &d.Color, &d.Icon, &d.SortOrder, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *QuestRepo) GetDomainByID(ctx context.Context, id string) (*model.QuestDomain, error) {
	var d model.QuestDomain
	err := r.db.QueryRow(ctx,
		`SELECT id, location_id, name, description, color, icon, sort_order, created_at, updated_at
		 FROM quest_domains WHERE id = $1`,
		id,
	).Scan(&d.ID, &d.LocationID, &d.Name, &d.Description, &d.Color, &d.Icon, &d.SortOrder, &d.CreatedAt, &d.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get quest domain: %w", err)
	}
	return &d, nil
}

// ============================================================
// Quests
// ============================================================

func (r *QuestRepo) Create(ctx context.Context, q *model.Quest) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO quests (location_id, domain_id, badge_id, name, description,
			quest_type, completion_criteria, target_count, suggested_duration_days,
			available_from, available_until, skill_level, requires_certification,
			route_tag_filter, is_active, sort_order)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		 RETURNING id, created_at, updated_at`,
		q.LocationID, q.DomainID, q.BadgeID, q.Name, q.Description,
		q.QuestType, q.CompletionCriteria, q.TargetCount, q.SuggestedDurationDays,
		q.AvailableFrom, q.AvailableUntil, q.SkillLevel, q.RequiresCertification,
		q.RouteTagFilter, q.IsActive, q.SortOrder,
	).Scan(&q.ID, &q.CreatedAt, &q.UpdatedAt)
}

func (r *QuestRepo) Update(ctx context.Context, q *model.Quest) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE quests
		 SET domain_id = $1, badge_id = $2, name = $3, description = $4,
		     quest_type = $5, completion_criteria = $6, target_count = $7,
		     suggested_duration_days = $8, available_from = $9, available_until = $10,
		     skill_level = $11, requires_certification = $12, route_tag_filter = $13,
		     is_active = $14, sort_order = $15, updated_at = NOW()
		 WHERE id = $16 AND location_id = $17`,
		q.DomainID, q.BadgeID, q.Name, q.Description,
		q.QuestType, q.CompletionCriteria, q.TargetCount,
		q.SuggestedDurationDays, q.AvailableFrom, q.AvailableUntil,
		q.SkillLevel, q.RequiresCertification, q.RouteTagFilter,
		q.IsActive, q.SortOrder, q.ID, q.LocationID,
	)
	if err != nil {
		return fmt.Errorf("update quest: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("quest %s not found", q.ID)
	}
	return nil
}

// Deactivate hides a quest from the browser. Active enrollments continue.
func (r *QuestRepo) Deactivate(ctx context.Context, id, locationID string) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE quests SET is_active = false, updated_at = NOW()
		 WHERE id = $1 AND location_id = $2`,
		id, locationID,
	)
	if err != nil {
		return fmt.Errorf("deactivate quest: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("quest %s not found", id)
	}
	return nil
}

func (r *QuestRepo) GetByID(ctx context.Context, id string) (*model.Quest, error) {
	var q model.Quest
	var domain model.QuestDomain
	var badge model.Badge
	var badgeID, badgeName, badgeDesc, badgeIcon, badgeColor *string

	err := r.db.QueryRow(ctx,
		`SELECT q.id, q.location_id, q.domain_id, q.badge_id, q.name, q.description,
			q.quest_type, q.completion_criteria, q.target_count, q.suggested_duration_days,
			q.available_from, q.available_until, q.skill_level, q.requires_certification,
			q.route_tag_filter, q.is_active, q.sort_order, q.created_at, q.updated_at,
			d.id, d.location_id, d.name, d.description, d.color, d.icon, d.sort_order, d.created_at, d.updated_at,
			b.id, b.name, b.description, b.icon, b.color
		 FROM quests q
		 JOIN quest_domains d ON d.id = q.domain_id
		 LEFT JOIN badges b ON b.id = q.badge_id
		 WHERE q.id = $1`,
		id,
	).Scan(
		&q.ID, &q.LocationID, &q.DomainID, &q.BadgeID, &q.Name, &q.Description,
		&q.QuestType, &q.CompletionCriteria, &q.TargetCount, &q.SuggestedDurationDays,
		&q.AvailableFrom, &q.AvailableUntil, &q.SkillLevel, &q.RequiresCertification,
		&q.RouteTagFilter, &q.IsActive, &q.SortOrder, &q.CreatedAt, &q.UpdatedAt,
		&domain.ID, &domain.LocationID, &domain.Name, &domain.Description, &domain.Color, &domain.Icon, &domain.SortOrder, &domain.CreatedAt, &domain.UpdatedAt,
		&badgeID, &badgeName, &badgeDesc, &badgeIcon, &badgeColor,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get quest by id: %w", err)
	}

	q.Domain = &domain
	if badgeID != nil {
		badge.ID = *badgeID
		badge.LocationID = q.LocationID
		badge.Name = *badgeName
		badge.Description = badgeDesc
		badge.Icon = *badgeIcon
		badge.Color = *badgeColor
		q.Badge = &badge
	}

	return &q, nil
}

// QuestListItem includes social proof counts for the quest browser.
type QuestListItem struct {
	model.Quest
	ActiveCount    int `json:"active_count"`    // climbers currently working on this
	CompletedCount int `json:"completed_count"` // total completions
}

// ListAvailable returns active quests that are currently within their
// availability window, with social proof counts. This is the quest browser.
func (r *QuestRepo) ListAvailable(ctx context.Context, locationID string) ([]QuestListItem, error) {
	rows, err := r.db.Query(ctx,
		`SELECT q.id, q.location_id, q.domain_id, q.badge_id, q.name, q.description,
			q.quest_type, q.completion_criteria, q.target_count, q.suggested_duration_days,
			q.available_from, q.available_until, q.skill_level, q.requires_certification,
			q.route_tag_filter, q.is_active, q.sort_order, q.created_at, q.updated_at,
			d.id, d.name, d.color, d.icon,
			COALESCE(active.cnt, 0) AS active_count,
			COALESCE(completed.cnt, 0) AS completed_count
		 FROM quests q
		 JOIN quest_domains d ON d.id = q.domain_id
		 LEFT JOIN (
			SELECT quest_id, COUNT(*) AS cnt FROM climber_quests WHERE status = 'active' GROUP BY quest_id
		 ) active ON active.quest_id = q.id
		 LEFT JOIN (
			SELECT quest_id, COUNT(*) AS cnt FROM climber_quests WHERE status = 'completed' GROUP BY quest_id
		 ) completed ON completed.quest_id = q.id
		 WHERE q.location_id = $1
		   AND q.is_active = true
		   AND (q.available_from IS NULL OR q.available_from <= NOW())
		   AND (q.available_until IS NULL OR q.available_until >= NOW())
		 ORDER BY d.sort_order, q.sort_order, q.name`,
		locationID,
	)
	if err != nil {
		return nil, fmt.Errorf("list available quests: %w", err)
	}
	defer rows.Close()

	var out []QuestListItem
	for rows.Next() {
		var item QuestListItem
		var domain model.QuestDomain
		if err := rows.Scan(
			&item.ID, &item.LocationID, &item.DomainID, &item.BadgeID, &item.Name, &item.Description,
			&item.QuestType, &item.CompletionCriteria, &item.TargetCount, &item.SuggestedDurationDays,
			&item.AvailableFrom, &item.AvailableUntil, &item.SkillLevel, &item.RequiresCertification,
			&item.RouteTagFilter, &item.IsActive, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt,
			&domain.ID, &domain.Name, &domain.Color, &domain.Icon,
			&item.ActiveCount, &item.CompletedCount,
		); err != nil {
			return nil, err
		}
		item.Domain = &domain
		out = append(out, item)
	}
	return out, rows.Err()
}

// ListByLocation returns all quests for admin views (including inactive).
func (r *QuestRepo) ListByLocation(ctx context.Context, locationID string) ([]QuestListItem, error) {
	rows, err := r.db.Query(ctx,
		`SELECT q.id, q.location_id, q.domain_id, q.badge_id, q.name, q.description,
			q.quest_type, q.completion_criteria, q.target_count, q.suggested_duration_days,
			q.available_from, q.available_until, q.skill_level, q.requires_certification,
			q.route_tag_filter, q.is_active, q.sort_order, q.created_at, q.updated_at,
			d.id, d.name, d.color, d.icon,
			COALESCE(active.cnt, 0) AS active_count,
			COALESCE(completed.cnt, 0) AS completed_count
		 FROM quests q
		 JOIN quest_domains d ON d.id = q.domain_id
		 LEFT JOIN (
			SELECT quest_id, COUNT(*) AS cnt FROM climber_quests WHERE status = 'active' GROUP BY quest_id
		 ) active ON active.quest_id = q.id
		 LEFT JOIN (
			SELECT quest_id, COUNT(*) AS cnt FROM climber_quests WHERE status = 'completed' GROUP BY quest_id
		 ) completed ON completed.quest_id = q.id
		 WHERE q.location_id = $1
		 ORDER BY q.is_active DESC, d.sort_order, q.sort_order, q.name`,
		locationID,
	)
	if err != nil {
		return nil, fmt.Errorf("list quests by location: %w", err)
	}
	defer rows.Close()

	var out []QuestListItem
	for rows.Next() {
		var item QuestListItem
		var domain model.QuestDomain
		if err := rows.Scan(
			&item.ID, &item.LocationID, &item.DomainID, &item.BadgeID, &item.Name, &item.Description,
			&item.QuestType, &item.CompletionCriteria, &item.TargetCount, &item.SuggestedDurationDays,
			&item.AvailableFrom, &item.AvailableUntil, &item.SkillLevel, &item.RequiresCertification,
			&item.RouteTagFilter, &item.IsActive, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt,
			&domain.ID, &domain.Name, &domain.Color, &domain.Icon,
			&item.ActiveCount, &item.CompletedCount,
		); err != nil {
			return nil, err
		}
		item.Domain = &domain
		out = append(out, item)
	}
	return out, rows.Err()
}

// Duplicate clones a quest with blank dates and inactive status.
// Used for seasonal rotation.
func (r *QuestRepo) Duplicate(ctx context.Context, id, locationID string) (*model.Quest, error) {
	var q model.Quest
	err := r.db.QueryRow(ctx,
		`INSERT INTO quests (location_id, domain_id, badge_id, name, description,
			quest_type, completion_criteria, target_count, suggested_duration_days,
			skill_level, requires_certification, route_tag_filter, is_active, sort_order)
		 SELECT location_id, domain_id, badge_id, name, description,
			quest_type, completion_criteria, target_count, suggested_duration_days,
			skill_level, requires_certification, route_tag_filter, false, sort_order
		 FROM quests
		 WHERE id = $1 AND location_id = $2
		 RETURNING id, location_id, domain_id, badge_id, name, description,
			quest_type, completion_criteria, target_count, suggested_duration_days,
			available_from, available_until, skill_level, requires_certification,
			route_tag_filter, is_active, sort_order, created_at, updated_at`,
		id, locationID,
	).Scan(
		&q.ID, &q.LocationID, &q.DomainID, &q.BadgeID, &q.Name, &q.Description,
		&q.QuestType, &q.CompletionCriteria, &q.TargetCount, &q.SuggestedDurationDays,
		&q.AvailableFrom, &q.AvailableUntil, &q.SkillLevel, &q.RequiresCertification,
		&q.RouteTagFilter, &q.IsActive, &q.SortOrder, &q.CreatedAt, &q.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("quest %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("duplicate quest: %w", err)
	}
	return &q, nil
}

// ============================================================
// Climber Quests (enrollment + progress)
// ============================================================

func (r *QuestRepo) StartQuest(ctx context.Context, userID, questID string) (*model.ClimberQuest, error) {
	var cq model.ClimberQuest
	err := r.db.QueryRow(ctx,
		`INSERT INTO climber_quests (user_id, quest_id)
		 VALUES ($1, $2)
		 RETURNING id, user_id, quest_id, status, progress_count, started_at, completed_at`,
		userID, questID,
	).Scan(&cq.ID, &cq.UserID, &cq.QuestID, &cq.Status, &cq.ProgressCount, &cq.StartedAt, &cq.CompletedAt)
	if err != nil {
		return nil, fmt.Errorf("start quest: %w", err)
	}
	return &cq, nil
}

func (r *QuestRepo) GetClimberQuest(ctx context.Context, id string) (*model.ClimberQuest, error) {
	var cq model.ClimberQuest
	err := r.db.QueryRow(ctx,
		`SELECT id, user_id, quest_id, status, progress_count, started_at, completed_at
		 FROM climber_quests WHERE id = $1`,
		id,
	).Scan(&cq.ID, &cq.UserID, &cq.QuestID, &cq.Status, &cq.ProgressCount, &cq.StartedAt, &cq.CompletedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get climber quest: %w", err)
	}
	return &cq, nil
}

// CompleteQuest marks a climber quest as completed. Returns the updated record.
func (r *QuestRepo) CompleteQuest(ctx context.Context, id string) (*model.ClimberQuest, error) {
	var cq model.ClimberQuest
	err := r.db.QueryRow(ctx,
		`UPDATE climber_quests
		 SET status = 'completed', completed_at = NOW()
		 WHERE id = $1 AND status = 'active'
		 RETURNING id, user_id, quest_id, status, progress_count, started_at, completed_at`,
		id,
	).Scan(&cq.ID, &cq.UserID, &cq.QuestID, &cq.Status, &cq.ProgressCount, &cq.StartedAt, &cq.CompletedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("climber quest %s not found or not active", id)
	}
	if err != nil {
		return nil, fmt.Errorf("complete quest: %w", err)
	}
	return &cq, nil
}

// AbandonQuest marks a climber quest as abandoned.
func (r *QuestRepo) AbandonQuest(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE climber_quests SET status = 'abandoned'
		 WHERE id = $1 AND status = 'active'`,
		id,
	)
	if err != nil {
		return fmt.Errorf("abandon quest: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("climber quest %s not found or not active", id)
	}
	return nil
}

// IncrementProgress atomically increments the progress count and returns
// the new count. Used after logging a quest entry.
func (r *QuestRepo) IncrementProgress(ctx context.Context, climberQuestID string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx,
		`UPDATE climber_quests
		 SET progress_count = progress_count + 1
		 WHERE id = $1 AND status = 'active'
		 RETURNING progress_count`,
		climberQuestID,
	).Scan(&count)
	if err == pgx.ErrNoRows {
		return 0, fmt.Errorf("climber quest %s not found or not active", climberQuestID)
	}
	if err != nil {
		return 0, fmt.Errorf("increment progress: %w", err)
	}
	return count, nil
}

// ListUserQuests returns all quests for a user, with quest details joined.
func (r *QuestRepo) ListUserQuests(ctx context.Context, userID string, status string) ([]model.ClimberQuest, error) {
	query := `
		SELECT cq.id, cq.user_id, cq.quest_id, cq.status, cq.progress_count, cq.started_at, cq.completed_at,
			q.id, q.location_id, q.domain_id, q.name, q.description, q.quest_type,
			q.completion_criteria, q.target_count, q.is_active,
			d.id, d.name, d.color, d.icon
		FROM climber_quests cq
		JOIN quests q ON q.id = cq.quest_id
		JOIN quest_domains d ON d.id = q.domain_id
		WHERE cq.user_id = $1`

	args := []any{userID}
	if status != "" {
		query += ` AND cq.status = $2`
		args = append(args, status)
	}
	query += ` ORDER BY cq.started_at DESC`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list user quests: %w", err)
	}
	defer rows.Close()

	var out []model.ClimberQuest
	for rows.Next() {
		var cq model.ClimberQuest
		var q model.Quest
		var d model.QuestDomain
		if err := rows.Scan(
			&cq.ID, &cq.UserID, &cq.QuestID, &cq.Status, &cq.ProgressCount, &cq.StartedAt, &cq.CompletedAt,
			&q.ID, &q.LocationID, &q.DomainID, &q.Name, &q.Description, &q.QuestType,
			&q.CompletionCriteria, &q.TargetCount, &q.IsActive,
			&d.ID, &d.Name, &d.Color, &d.Icon,
		); err != nil {
			return nil, err
		}
		q.Domain = &d
		cq.Quest = &q
		out = append(out, cq)
	}
	return out, rows.Err()
}

// ============================================================
// Quest Logs
// ============================================================

func (r *QuestRepo) LogProgress(ctx context.Context, log *model.QuestLog) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO quest_logs (climber_quest_id, log_type, route_id, notes, rating)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, logged_at`,
		log.ClimberQuestID, log.LogType, log.RouteID, log.Notes, log.Rating,
	).Scan(&log.ID, &log.LoggedAt)
}

func (r *QuestRepo) ListLogs(ctx context.Context, climberQuestID string) ([]model.QuestLog, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, climber_quest_id, log_type, route_id, notes, rating, logged_at
		 FROM quest_logs
		 WHERE climber_quest_id = $1
		 ORDER BY logged_at DESC`,
		climberQuestID,
	)
	if err != nil {
		return nil, fmt.Errorf("list quest logs: %w", err)
	}
	defer rows.Close()

	var out []model.QuestLog
	for rows.Next() {
		var l model.QuestLog
		if err := rows.Scan(&l.ID, &l.ClimberQuestID, &l.LogType, &l.RouteID, &l.Notes, &l.Rating, &l.LoggedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// DomainProgress returns per-domain completion counts for a user.
// Used to render the radar chart / domain map.
type DomainProgress struct {
	DomainID   string `json:"domain_id"`
	DomainName string `json:"domain_name"`
	Color      string `json:"color"`
	Completed  int    `json:"completed"`
}

func (r *QuestRepo) UserDomainProgress(ctx context.Context, userID, locationID string) ([]DomainProgress, error) {
	rows, err := r.db.Query(ctx,
		`SELECT d.id, d.name, COALESCE(d.color, ''), COUNT(cq.id)
		 FROM quest_domains d
		 LEFT JOIN quests q ON q.domain_id = d.id
		 LEFT JOIN climber_quests cq ON cq.quest_id = q.id AND cq.user_id = $1 AND cq.status = 'completed'
		 WHERE d.location_id = $2
		 GROUP BY d.id, d.name, d.color, d.sort_order
		 ORDER BY d.sort_order`,
		userID, locationID,
	)
	if err != nil {
		return nil, fmt.Errorf("user domain progress: %w", err)
	}
	defer rows.Close()

	var out []DomainProgress
	for rows.Next() {
		var dp DomainProgress
		if err := rows.Scan(&dp.DomainID, &dp.DomainName, &dp.Color, &dp.Completed); err != nil {
			return nil, err
		}
		out = append(out, dp)
	}
	return out, rows.Err()
}
