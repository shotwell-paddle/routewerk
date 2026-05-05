package repository

import (
	"context"
	"fmt"
	"time"

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
		RETURNING id, status, created_at, updated_at`

	return r.db.QueryRow(ctx, query,
		s.LocationID, s.ScheduledDate, s.Notes, s.CreatedBy,
	).Scan(&s.ID, &s.Status, &s.CreatedAt, &s.UpdatedAt)
}

func (r *SessionRepo) GetByID(ctx context.Context, id string) (*model.SettingSession, error) {
	// Single query with LEFT JOIN to load session + assignments in one round trip
	query := `
		SELECT s.id, s.location_id, s.scheduled_date, s.status, s.notes, s.created_by, s.created_at, s.updated_at,
			a.id, a.session_id, a.setter_id, a.wall_id, a.target_grades, a.notes
		FROM setting_sessions s
		LEFT JOIN setting_session_assignments a ON a.session_id = s.id
		WHERE s.id = $1`

	rows, err := r.db.Query(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	defer rows.Close()

	var s *model.SettingSession
	for rows.Next() {
		var aID, aSessionID, aSetterID *string
		var aWallID, aNotes *string
		var aTargetGrades []string

		if s == nil {
			s = &model.SettingSession{}
		}

		if err := rows.Scan(
			&s.ID, &s.LocationID, &s.ScheduledDate, &s.Status, &s.Notes,
			&s.CreatedBy, &s.CreatedAt, &s.UpdatedAt,
			&aID, &aSessionID, &aSetterID, &aWallID, &aTargetGrades, &aNotes,
		); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}

		if aID != nil {
			s.Assignments = append(s.Assignments, model.SettingSessionAssignment{
				ID:           *aID,
				SessionID:    *aSessionID,
				SetterID:     *aSetterID,
				WallID:       aWallID,
				TargetGrades: aTargetGrades,
				Notes:        aNotes,
			})
		}
	}

	return s, nil
}

func (r *SessionRepo) ListByLocation(ctx context.Context, locationID string, limit, offset int) ([]model.SettingSession, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT id, location_id, scheduled_date, status, notes, created_by, created_at, updated_at
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
			&s.ID, &s.LocationID, &s.ScheduledDate, &s.Status, &s.Notes,
			&s.CreatedBy, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
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

// UpdateAssignmentWall sets the wall_id on an existing assignment (if currently null).
func (r *SessionRepo) UpdateAssignmentWall(ctx context.Context, id string, wallID string) error {
	_, err := r.db.Exec(ctx,
		"UPDATE setting_session_assignments SET wall_id = $2 WHERE id = $1 AND wall_id IS NULL",
		id, wallID)
	if err != nil {
		return fmt.Errorf("update assignment wall: %w", err)
	}
	return nil
}

func (r *SessionRepo) RemoveAssignment(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, "DELETE FROM setting_session_assignments WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("remove assignment: %w", err)
	}
	return nil
}

// SessionListItem holds a session with aggregate data for the list view.
type SessionListItem struct {
	model.SettingSession
	CreatorName     string `json:"creator_name"`
	AssignmentCount int    `json:"assignment_count"`
}

// ListByLocationWithDetails returns sessions enriched with creator name and
// assignment count, ordered by scheduled_date descending.
func (r *SessionRepo) ListByLocationWithDetails(ctx context.Context, locationID string, limit, offset int, statusFilter ...string) ([]SessionListItem, error) {
	if limit <= 0 {
		limit = 50
	}

	filter := ""
	if len(statusFilter) > 0 {
		filter = statusFilter[0]
	}

	// Build query with optional status filter to avoid three near-identical copies.
	// The assignment count uses a LEFT JOIN + GROUP BY instead of a correlated subquery.
	query := `
		SELECT s.id, s.location_id, s.scheduled_date, s.status, s.notes, s.created_by,
			s.created_at, s.updated_at,
			COALESCE(u.display_name, 'Unknown') AS creator_name,
			COUNT(a.id)::int AS assignment_count
		FROM setting_sessions s
		LEFT JOIN users u ON u.id = s.created_by
		LEFT JOIN setting_session_assignments a ON a.session_id = s.id
		WHERE s.location_id = $1`

	args := []interface{}{locationID}
	switch filter {
	case "open":
		query += ` AND s.status != 'complete'`
	case "complete":
		query += ` AND s.status = 'complete'`
	}

	query += `
		GROUP BY s.id, u.display_name
		ORDER BY s.scheduled_date DESC
		LIMIT $2 OFFSET $3`
	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list sessions with details: %w", err)
	}
	defer rows.Close()

	var items []SessionListItem
	for rows.Next() {
		var item SessionListItem
		if err := rows.Scan(
			&item.ID, &item.LocationID, &item.ScheduledDate, &item.Status, &item.Notes,
			&item.CreatedBy, &item.CreatedAt, &item.UpdatedAt,
			&item.CreatorName, &item.AssignmentCount,
		); err != nil {
			return nil, fmt.Errorf("scan session item: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// SessionAssignmentDetail holds an assignment enriched with user/wall names.
type SessionAssignmentDetail struct {
	model.SettingSessionAssignment
	SetterName string  `json:"setter_name"`
	WallName   *string `json:"wall_name,omitempty"`
}

// GetByIDWithDetails returns a session with fully-loaded assignment details.
func (r *SessionRepo) GetByIDWithDetails(ctx context.Context, id string) (*model.SettingSession, []SessionAssignmentDetail, error) {
	query := `
		SELECT s.id, s.location_id, s.scheduled_date, s.status, s.notes, s.created_by,
			s.created_at, s.updated_at,
			a.id, a.session_id, a.setter_id, a.wall_id, a.target_grades, a.notes,
			COALESCE(u.display_name, 'Unknown'),
			w.name
		FROM setting_sessions s
		LEFT JOIN setting_session_assignments a ON a.session_id = s.id
		LEFT JOIN users u ON u.id = a.setter_id
		LEFT JOIN walls w ON w.id = a.wall_id
		WHERE s.id = $1`

	rows, err := r.db.Query(ctx, query, id)
	if err != nil {
		return nil, nil, fmt.Errorf("get session with details: %w", err)
	}
	defer rows.Close()

	var s *model.SettingSession
	var details []SessionAssignmentDetail
	for rows.Next() {
		var aID, aSessionID, aSetterID *string
		var aWallID, aNotes *string
		var aTargetGrades []string
		var setterName *string
		var wallName *string

		if s == nil {
			s = &model.SettingSession{}
		}

		if err := rows.Scan(
			&s.ID, &s.LocationID, &s.ScheduledDate, &s.Status, &s.Notes,
			&s.CreatedBy, &s.CreatedAt, &s.UpdatedAt,
			&aID, &aSessionID, &aSetterID, &aWallID, &aTargetGrades, &aNotes,
			&setterName, &wallName,
		); err != nil {
			return nil, nil, fmt.Errorf("scan session detail: %w", err)
		}

		if aID != nil {
			detail := SessionAssignmentDetail{
				SettingSessionAssignment: model.SettingSessionAssignment{
					ID:           *aID,
					SessionID:    *aSessionID,
					SetterID:     *aSetterID,
					WallID:       aWallID,
					TargetGrades: aTargetGrades,
					Notes:        aNotes,
				},
				WallName: wallName,
			}
			if setterName != nil {
				detail.SetterName = *setterName
			}
			details = append(details, detail)
		}
	}

	return s, details, nil
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

// ── Strip Targets ──────────────────────────────────────────

// StripTargetDetail holds a strip target with wall and route display info.
type StripTargetDetail struct {
	model.SessionStripTarget
	WallName   string  `json:"wall_name"`
	WallType   string  `json:"wall_type"`
	RouteGrade *string `json:"route_grade,omitempty"`
	RouteColor *string `json:"route_color,omitempty"`
	RouteName  *string `json:"route_name,omitempty"`
	RouteType  *string `json:"route_type,omitempty"`
}

// AddStripTarget adds a wall or individual route as a strip target for a session.
func (r *SessionRepo) AddStripTarget(ctx context.Context, t *model.SessionStripTarget) error {
	if t.RouteID == nil {
		// Whole-wall target — use the partial unique index
		return r.db.QueryRow(ctx, `
			INSERT INTO session_strip_targets (session_id, wall_id)
			VALUES ($1, $2)
			ON CONFLICT (session_id, wall_id) WHERE route_id IS NULL DO NOTHING
			RETURNING id, created_at`,
			t.SessionID, t.WallID,
		).Scan(&t.ID, &t.CreatedAt)
	}
	// Individual route target
	return r.db.QueryRow(ctx, `
		INSERT INTO session_strip_targets (session_id, wall_id, route_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (session_id, wall_id, route_id) DO NOTHING
		RETURNING id, created_at`,
		t.SessionID, t.WallID, t.RouteID,
	).Scan(&t.ID, &t.CreatedAt)
}

// RemoveStripTarget deletes a strip target by ID.
func (r *SessionRepo) RemoveStripTarget(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM session_strip_targets WHERE id = $1`, id)
	return err
}

// ListStripTargets returns all strip targets for a session with wall/route details.
func (r *SessionRepo) ListStripTargets(ctx context.Context, sessionID string) ([]StripTargetDetail, error) {
	query := `
		SELECT st.id, st.session_id, st.wall_id, st.route_id, st.created_at,
			w.name, w.wall_type,
			rt.grade, rt.color, rt.name, rt.route_type
		FROM session_strip_targets st
		JOIN walls w ON w.id = st.wall_id
		LEFT JOIN routes rt ON rt.id = st.route_id
		WHERE st.session_id = $1
		ORDER BY w.sort_order, rt.grade`

	rows, err := r.db.Query(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list strip targets: %w", err)
	}
	defer rows.Close()

	var targets []StripTargetDetail
	for rows.Next() {
		var t StripTargetDetail
		if err := rows.Scan(
			&t.ID, &t.SessionID, &t.WallID, &t.RouteID, &t.CreatedAt,
			&t.WallName, &t.WallType,
			&t.RouteGrade, &t.RouteColor, &t.RouteName, &t.RouteType,
		); err != nil {
			return nil, fmt.Errorf("scan strip target: %w", err)
		}
		targets = append(targets, t)
	}
	return targets, nil
}

// ActiveRoutesByWall returns active routes grouped by wall for a location.
// Used by the strip target picker.
type WallWithActiveRoutes struct {
	Wall   model.Wall
	Routes []model.Route
}

func (r *SessionRepo) ActiveRoutesByWall(ctx context.Context, locationID string) ([]WallWithActiveRoutes, error) {
	query := `
		SELECT w.id, w.name, w.wall_type, w.sort_order,
			rt.id, rt.grade, rt.color, rt.name, rt.route_type, rt.date_set
		FROM walls w
		LEFT JOIN routes rt ON rt.wall_id = w.id AND rt.status = 'active' AND rt.deleted_at IS NULL
		WHERE w.location_id = $1 AND w.deleted_at IS NULL
		ORDER BY w.sort_order, rt.grade`

	rows, err := r.db.Query(ctx, query, locationID)
	if err != nil {
		return nil, fmt.Errorf("active routes by wall: %w", err)
	}
	defer rows.Close()

	wallMap := map[string]*WallWithActiveRoutes{}
	var order []string

	for rows.Next() {
		var wID, wName, wType string
		var wSort int
		var rID, rGrade, rColor, rRouteType *string
		var rName *string
		var rDateSet *time.Time

		if err := rows.Scan(
			&wID, &wName, &wType, &wSort,
			&rID, &rGrade, &rColor, &rName, &rRouteType, &rDateSet,
		); err != nil {
			return nil, fmt.Errorf("scan wall routes: %w", err)
		}

		wg, exists := wallMap[wID]
		if !exists {
			wg = &WallWithActiveRoutes{
				Wall: model.Wall{
					ID:        wID,
					Name:      wName,
					WallType:  wType,
					SortOrder: wSort,
				},
			}
			wallMap[wID] = wg
			order = append(order, wID)
		}

		if rID != nil {
			route := model.Route{
				ID:        *rID,
				WallID:    wID,
				RouteType: derefOr(rRouteType, ""),
				Grade:     derefOr(rGrade, ""),
				Color:     derefOr(rColor, ""),
				Name:      rName,
			}
			if rDateSet != nil {
				route.DateSet = *rDateSet
			}
			wg.Routes = append(wg.Routes, route)
		}
	}

	result := make([]WallWithActiveRoutes, 0, len(order))
	for _, id := range order {
		result = append(result, *wallMap[id])
	}
	return result, nil
}

func derefOr(s *string, fallback string) string {
	if s != nil {
		return *s
	}
	return fallback
}

// ── Session Lifecycle ──────────────────────────────────────

// UpdateStatus changes a session's status (planning, in_progress, complete).
func (r *SessionRepo) UpdateStatus(ctx context.Context, id, status string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE setting_sessions SET status = $2, updated_at = NOW() WHERE id = $1`,
		id, status,
	)
	return err
}

// Delete removes a session and cascade-deletes its routes (that were created in this session).
// Runs inside a transaction so a partial failure doesn't leave orphaned records.
func (r *SessionRepo) Delete(ctx context.Context, id string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// First delete any draft routes linked to this session
	if _, err := tx.Exec(ctx,
		`DELETE FROM routes WHERE session_id = $1 AND status = 'draft'`, id); err != nil {
		return fmt.Errorf("delete session routes: %w", err)
	}

	// Unlink any published routes (keep them but clear session_id)
	if _, err := tx.Exec(ctx,
		`UPDATE routes SET session_id = NULL WHERE session_id = $1`, id); err != nil {
		return fmt.Errorf("unlink session routes: %w", err)
	}

	// Delete assignments (no ON DELETE CASCADE on this FK)
	if _, err := tx.Exec(ctx,
		`DELETE FROM setting_session_assignments WHERE session_id = $1`, id); err != nil {
		return fmt.Errorf("delete session assignments: %w", err)
	}

	// Delete the session (strip targets + checklist cascade automatically)
	if _, err := tx.Exec(ctx, `DELETE FROM setting_sessions WHERE id = $1`, id); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return tx.Commit(ctx)
}

// ── Session Routes ─────────────────────────────────────────

// SessionRouteDetail holds a route with setter and wall names for session view.
type SessionRouteDetail struct {
	model.Route
	SetterName string `json:"setter_name"`
	WallName   string `json:"wall_name"`
}

// ListSessionRoutes returns all routes linked to a session.
func (r *SessionRepo) ListSessionRoutes(ctx context.Context, sessionID string) ([]SessionRouteDetail, error) {
	query := `
		SELECT rt.id, rt.location_id, rt.wall_id, rt.setter_id, rt.route_type, rt.status,
			rt.grading_system, rt.grade, rt.color, rt.name, rt.date_set, rt.session_id,
			rt.circuit_color, rt.photo_url,
			rt.created_at, rt.updated_at,
			COALESCE(u.display_name, 'Unknown') AS setter_name,
			w.name AS wall_name
		FROM routes rt
		LEFT JOIN users u ON u.id = rt.setter_id
		JOIN walls w ON w.id = rt.wall_id
		WHERE rt.session_id = $1 AND rt.deleted_at IS NULL
		ORDER BY w.sort_order, rt.grade`

	rows, err := r.db.Query(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list session routes: %w", err)
	}
	defer rows.Close()

	var routes []SessionRouteDetail
	for rows.Next() {
		var d SessionRouteDetail
		if err := rows.Scan(
			&d.ID, &d.LocationID, &d.WallID, &d.SetterID, &d.RouteType, &d.Status,
			&d.GradingSystem, &d.Grade, &d.Color, &d.Name, &d.DateSet, &d.SessionID,
			&d.CircuitColor, &d.PhotoURL,
			&d.CreatedAt, &d.UpdatedAt,
			&d.SetterName, &d.WallName,
		); err != nil {
			return nil, fmt.Errorf("scan session route: %w", err)
		}
		routes = append(routes, d)
	}
	return routes, rows.Err()
}

// PublishSessionRoutes sets all draft routes in a session to 'active'.
func (r *SessionRepo) PublishSessionRoutes(ctx context.Context, sessionID string) (int, error) {
	tag, err := r.db.Exec(ctx,
		`UPDATE routes SET status = 'active', updated_at = NOW()
		 WHERE session_id = $1 AND status = 'draft' AND deleted_at IS NULL`,
		sessionID,
	)
	if err != nil {
		return 0, fmt.Errorf("publish session routes: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// DeleteSessionRoute hard-deletes a draft route by ID (only if it belongs to the session).
func (r *SessionRepo) DeleteSessionRoute(ctx context.Context, sessionID, routeID string) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM routes WHERE id = $1 AND session_id = $2 AND status = 'draft'`,
		routeID, sessionID,
	)
	return err
}

// ── Session Checklist ──────────────────────────────────────

// ChecklistItemWithUser enriches a checklist item with the completer's name.
type ChecklistItemWithUser struct {
	model.SessionChecklistItem
	CompletedByName *string `json:"completed_by_name,omitempty"`
}

// ListChecklistItems returns checklist items for a session, ordered by sort_order.
func (r *SessionRepo) ListChecklistItems(ctx context.Context, sessionID string) ([]ChecklistItemWithUser, error) {
	query := `
		SELECT ci.id, ci.session_id, ci.sort_order, ci.title,
			ci.completed, ci.completed_by, ci.completed_at, ci.created_at,
			u.display_name
		FROM session_checklist_items ci
		LEFT JOIN users u ON u.id = ci.completed_by
		WHERE ci.session_id = $1
		ORDER BY ci.sort_order`

	rows, err := r.db.Query(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list checklist items: %w", err)
	}
	defer rows.Close()

	var items []ChecklistItemWithUser
	for rows.Next() {
		var item ChecklistItemWithUser
		if err := rows.Scan(
			&item.ID, &item.SessionID, &item.SortOrder, &item.Title,
			&item.Completed, &item.CompletedBy, &item.CompletedAt, &item.CreatedAt,
			&item.CompletedByName,
		); err != nil {
			return nil, fmt.Errorf("scan checklist item: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ToggleChecklistItem toggles the completed state of a checklist item.
// If marking complete, stores who completed it and when.
func (r *SessionRepo) ToggleChecklistItem(ctx context.Context, itemID, userID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE session_checklist_items
		SET completed = NOT completed,
			completed_by = CASE WHEN NOT completed THEN $2 ELSE NULL END,
			completed_at = CASE WHEN NOT completed THEN NOW() ELSE NULL END
		WHERE id = $1`,
		itemID, userID,
	)
	return err
}

// InitializeChecklist copies the location's playbook steps into a session's checklist.
// Skips if the session already has checklist items.
func (r *SessionRepo) InitializeChecklist(ctx context.Context, sessionID, locationID string) error {
	// Don't duplicate if already initialized
	var count int
	if err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM session_checklist_items WHERE session_id = $1`,
		sessionID,
	).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	_, err := r.db.Exec(ctx, `
		INSERT INTO session_checklist_items (session_id, sort_order, title)
		SELECT $1, sort_order, title
		FROM location_playbook_steps
		WHERE location_id = $2
		ORDER BY sort_order`,
		sessionID, locationID,
	)
	return err
}

// ── Location Playbook Steps ────────────────────────────────
//
// These manage the per-gym playbook template that's copied into each new
// session's checklist (see InitializeChecklist). Edits here only affect
// future sessions — existing sessions keep the snapshot they were
// initialised with.

// ListPlaybookSteps returns the playbook template for a location, ordered
// by sort_order.
func (r *SessionRepo) ListPlaybookSteps(ctx context.Context, locationID string) ([]model.LocationPlaybookStep, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, location_id, sort_order, title, created_at
		FROM location_playbook_steps
		WHERE location_id = $1
		ORDER BY sort_order, created_at`,
		locationID,
	)
	if err != nil {
		return nil, fmt.Errorf("list playbook steps: %w", err)
	}
	defer rows.Close()

	var steps []model.LocationPlaybookStep
	for rows.Next() {
		var s model.LocationPlaybookStep
		if err := rows.Scan(&s.ID, &s.LocationID, &s.SortOrder, &s.Title, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan playbook step: %w", err)
		}
		steps = append(steps, s)
	}
	return steps, rows.Err()
}

// GetPlaybookStep fetches a single step. Returns nil (no error) if not found.
func (r *SessionRepo) GetPlaybookStep(ctx context.Context, stepID string) (*model.LocationPlaybookStep, error) {
	var s model.LocationPlaybookStep
	err := r.db.QueryRow(ctx, `
		SELECT id, location_id, sort_order, title, created_at
		FROM location_playbook_steps
		WHERE id = $1`,
		stepID,
	).Scan(&s.ID, &s.LocationID, &s.SortOrder, &s.Title, &s.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get playbook step: %w", err)
	}
	return &s, nil
}

// CreatePlaybookStep appends a step to the end of the location's playbook.
// The new step's sort_order is one greater than the current max.
func (r *SessionRepo) CreatePlaybookStep(ctx context.Context, locationID, title string) (*model.LocationPlaybookStep, error) {
	var s model.LocationPlaybookStep
	err := r.db.QueryRow(ctx, `
		INSERT INTO location_playbook_steps (location_id, sort_order, title)
		VALUES (
			$1,
			COALESCE((SELECT MAX(sort_order) + 1 FROM location_playbook_steps WHERE location_id = $1), 0),
			$2
		)
		RETURNING id, location_id, sort_order, title, created_at`,
		locationID, title,
	).Scan(&s.ID, &s.LocationID, &s.SortOrder, &s.Title, &s.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create playbook step: %w", err)
	}
	return &s, nil
}

// UpdatePlaybookStep replaces a step's title.
func (r *SessionRepo) UpdatePlaybookStep(ctx context.Context, stepID, title string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE location_playbook_steps SET title = $2 WHERE id = $1`,
		stepID, title,
	)
	if err != nil {
		return fmt.Errorf("update playbook step: %w", err)
	}
	return nil
}

// DeletePlaybookStep removes a step. Existing session checklist items are
// unaffected — they're snapshots, not foreign-keyed to the template.
func (r *SessionRepo) DeletePlaybookStep(ctx context.Context, stepID string) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM location_playbook_steps WHERE id = $1`,
		stepID,
	)
	if err != nil {
		return fmt.Errorf("delete playbook step: %w", err)
	}
	return nil
}

// MovePlaybookStep swaps the sort_order of a step with its neighbour in the
// given direction ("up" or "down"). No-op if the step is already at the
// edge in that direction. Done in a transaction so the swap is atomic.
func (r *SessionRepo) MovePlaybookStep(ctx context.Context, stepID, direction string) error {
	if direction != "up" && direction != "down" {
		return fmt.Errorf("invalid direction %q", direction)
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback on commit is no-op

	var locationID string
	var sortOrder int
	if err := tx.QueryRow(ctx,
		`SELECT location_id, sort_order FROM location_playbook_steps WHERE id = $1 FOR UPDATE`,
		stepID,
	).Scan(&locationID, &sortOrder); err != nil {
		return fmt.Errorf("lock step: %w", err)
	}

	// Find neighbour
	var neighbourQuery string
	if direction == "up" {
		neighbourQuery = `
			SELECT id, sort_order FROM location_playbook_steps
			WHERE location_id = $1 AND sort_order < $2
			ORDER BY sort_order DESC LIMIT 1
			FOR UPDATE`
	} else {
		neighbourQuery = `
			SELECT id, sort_order FROM location_playbook_steps
			WHERE location_id = $1 AND sort_order > $2
			ORDER BY sort_order ASC LIMIT 1
			FOR UPDATE`
	}

	var neighbourID string
	var neighbourOrder int
	if err := tx.QueryRow(ctx, neighbourQuery, locationID, sortOrder).
		Scan(&neighbourID, &neighbourOrder); err != nil {
		if err == pgx.ErrNoRows {
			// already at the edge; nothing to do
			return tx.Commit(ctx)
		}
		return fmt.Errorf("find neighbour: %w", err)
	}

	// Swap. Sort_order has no UNIQUE constraint, so a direct two-step
	// update is fine without a temporary value.
	if _, err := tx.Exec(ctx,
		`UPDATE location_playbook_steps SET sort_order = $1 WHERE id = $2`,
		neighbourOrder, stepID,
	); err != nil {
		return fmt.Errorf("update step order: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE location_playbook_steps SET sort_order = $1 WHERE id = $2`,
		sortOrder, neighbourID,
	); err != nil {
		return fmt.Errorf("update neighbour order: %w", err)
	}

	return tx.Commit(ctx)
}
