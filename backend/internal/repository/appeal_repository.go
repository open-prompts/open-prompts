package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"open-prompts/backend/internal/models"

	"github.com/lib/pq"
)

// Appeal repository error sentinels.
var (
	// ErrAppealNotFound is returned when an appeal id is unknown.
	ErrAppealNotFound = errors.New("appeal not found")
	// ErrActionNotFound is returned when a moderation action id is unknown.
	ErrActionNotFound = errors.New("moderation action not found")
	// ErrAppealAlreadyResolved is returned when resolving an already-resolved
	// appeal (the state guard aborts the transaction).
	ErrAppealAlreadyResolved = errors.New("appeal already resolved")
	// ErrAppealAlreadyExists is returned when a second appeal is created for
	// the same moderation action (UNIQUE constraint backstop, FR-3).
	ErrAppealAlreadyExists = errors.New("appeal already exists for this action")
)

// AppealRepository is the data-access interface for moderation appeals and
// the moderation_actions table they reference.
type AppealRepository interface {
	CreateAppeal(ctx context.Context, a *models.Appeal) error
	GetAppeal(ctx context.Context, appealID string) (*models.Appeal, error)
	GetAppealByAction(ctx context.Context, actionID string) (*models.Appeal, error)
	ListAppeals(ctx context.Context, appellantID string, page, perPage int) ([]models.Appeal, int64, error)
	GetModerationAction(ctx context.Context, actionID string) (*models.ModerationAction, error)
	ListModerationActionsForTemplate(ctx context.Context, templateID string) ([]models.ModerationAction, error)
	ListQueue(ctx context.Context, state string, page, perPage int) ([]models.Appeal, int64, error)
	CountByState(ctx context.Context) (open, underReview, resolved int, err error)
	SetState(ctx context.Context, appealID, state string) error
	ResolveAppeal(ctx context.Context, appealID, outcome, note, resolverID string) error
	MarkActionSuperseded(ctx context.Context, actionID string) error
}

type appealRepository struct {
	db *sql.DB
}

// NewAppealRepository creates a new AppealRepository.
func NewAppealRepository(db *sql.DB) AppealRepository {
	return &appealRepository{db: db}
}

// appealColumns is the shared SELECT column list for the appeals table.
const appealColumns = `
	a.id, a.moderation_action_id, a.appellant_id, a.statement, a.state,
	a.resolver_id, COALESCE(a.resolution_note, ''), a.created_at, a.updated_at, a.resolved_at
`

// scanAppeal scans an appeals row into a models.Appeal.
func scanAppeal(scanner interface {
	Scan(dest ...interface{}) error
}) (*models.Appeal, error) {
	var a models.Appeal
	var resolverID sql.NullString
	var resolvedAt sql.NullTime
	err := scanner.Scan(
		&a.ID, &a.ModerationActionID, &a.AppellantID, &a.Statement, &a.State,
		&resolverID, &a.ResolutionNote, &a.CreatedAt, &a.UpdatedAt, &resolvedAt,
	)
	if err != nil {
		return nil, err
	}
	if resolverID.Valid {
		a.ResolverID = resolverID.String
	}
	if resolvedAt.Valid {
		t := resolvedAt.Time
		a.ResolvedAt = &t
	}
	return &a, nil
}

// CreateAppeal inserts a new open appeal. The UNIQUE (moderation_action_id)
// constraint surfaces a 23505 violation, mapped to ErrAppealAlreadyExists.
func (r *appealRepository) CreateAppeal(ctx context.Context, a *models.Appeal) error {
	query := `
		INSERT INTO appeals (moderation_action_id, appellant_id, statement)
		VALUES ($1, $2, $3)
		RETURNING id, created_at
	`
	err := r.db.QueryRowContext(ctx, query, a.ModerationActionID, a.AppellantID, a.Statement).
		Scan(&a.ID, &a.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrAppealAlreadyExists
		}
		return fmt.Errorf("failed to create appeal: %w", err)
	}
	return nil
}

// GetAppeal retrieves an appeal by id.
func (r *appealRepository) GetAppeal(ctx context.Context, appealID string) (*models.Appeal, error) {
	query := `SELECT ` + appealColumns + ` FROM appeals a WHERE a.id = $1`
	a, err := scanAppeal(r.db.QueryRowContext(ctx, query, appealID))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrAppealNotFound
		}
		return nil, fmt.Errorf("failed to get appeal: %w", err)
	}
	return a, nil
}

// GetAppealByAction retrieves the appeal for a moderation action, if any.
func (r *appealRepository) GetAppealByAction(ctx context.Context, actionID string) (*models.Appeal, error) {
	query := `SELECT ` + appealColumns + ` FROM appeals a WHERE a.moderation_action_id = $1`
	a, err := scanAppeal(r.db.QueryRowContext(ctx, query, actionID))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrAppealNotFound
		}
		return nil, fmt.Errorf("failed to get appeal by action: %w", err)
	}
	return a, nil
}

// ListAppeals returns the caller's appeals, newest first, paginated.
func (r *appealRepository) ListAppeals(ctx context.Context, appellantID string, page, perPage int) ([]models.Appeal, int64, error) {
	offset := (page - 1) * perPage
	if offset < 0 {
		offset = 0
	}
	query := `
		SELECT a.id, a.moderation_action_id, a.appellant_id, a.statement, a.state,
		       a.resolver_id, COALESCE(a.resolution_note, ''), a.created_at, a.updated_at, a.resolved_at,
		       ma.template_id, ma.action_type, ma.reason AS action_reason,
		       ma.actor_email, ma.created_at AS action_created_at, ma.status AS action_status,
		       t.title AS template_title
		FROM appeals a
		JOIN moderation_actions ma ON ma.id = a.moderation_action_id
		JOIN templates t ON t.id = ma.template_id
		WHERE a.appellant_id = $1
		ORDER BY a.created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.QueryContext(ctx, query, appellantID, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list appeals: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []models.Appeal
	for rows.Next() {
		var a models.Appeal
		var resolverID sql.NullString
		var resolvedAt sql.NullTime
		var actionCreatedAt sql.NullTime
		if err := rows.Scan(
			&a.ID, &a.ModerationActionID, &a.AppellantID, &a.Statement, &a.State,
			&resolverID, &a.ResolutionNote, &a.CreatedAt, &a.UpdatedAt, &resolvedAt,
			&a.TemplateID, &a.ActionType, &a.ActionReason,
			&a.ActorEmail, &actionCreatedAt, &a.ActionStatus,
			&a.TemplateTitle,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan appeal row: %w", err)
		}
		if resolverID.Valid {
			a.ResolverID = resolverID.String
		}
		if resolvedAt.Valid {
			t := resolvedAt.Time
			a.ResolvedAt = &t
		}
		if actionCreatedAt.Valid {
			a.ActionCreatedAt = actionCreatedAt.Time
		}
		items = append(items, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}

	var total int64
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM appeals WHERE appellant_id = $1`, appellantID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count appeals: %w", err)
	}
	return items, total, nil
}

// GetModerationAction retrieves a moderation action by id.
func (r *appealRepository) GetModerationAction(ctx context.Context, actionID string) (*models.ModerationAction, error) {
	query := `
		SELECT id, template_id, action_type, actor_id, actor_email, reason, status,
		       created_at, superseded_at
		FROM moderation_actions
		WHERE id = $1
	`
	var ma models.ModerationAction
	var supersededAt sql.NullTime
	err := r.db.QueryRowContext(ctx, query, actionID).Scan(
		&ma.ID, &ma.TemplateID, &ma.ActionType, &ma.ActorID, &ma.ActorEmail, &ma.Reason, &ma.Status,
		&ma.CreatedAt, &supersededAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrActionNotFound
		}
		return nil, fmt.Errorf("failed to get moderation action: %w", err)
	}
	if supersededAt.Valid {
		t := supersededAt.Time
		ma.SupersededAt = &t
	}
	return &ma, nil
}

// ListModerationActionsForTemplate returns the moderation actions for a
// template, newest first.
func (r *appealRepository) ListModerationActionsForTemplate(ctx context.Context, templateID string) ([]models.ModerationAction, error) {
	query := `
		SELECT id, template_id, action_type, actor_id, actor_email, reason, status,
		       created_at, superseded_at
		FROM moderation_actions
		WHERE template_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, templateID)
	if err != nil {
		return nil, fmt.Errorf("failed to list moderation actions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []models.ModerationAction
	for rows.Next() {
		var ma models.ModerationAction
		var supersededAt sql.NullTime
		if err := rows.Scan(
			&ma.ID, &ma.TemplateID, &ma.ActionType, &ma.ActorID, &ma.ActorEmail, &ma.Reason, &ma.Status,
			&ma.CreatedAt, &supersededAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan moderation action: %w", err)
		}
		if supersededAt.Valid {
			t := supersededAt.Time
			ma.SupersededAt = &t
		}
		items = append(items, ma)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return items, nil
}

// ListQueue returns the admin appeal queue, oldest first within state,
// paginated. state "" returns all states.
func (r *appealRepository) ListQueue(ctx context.Context, state string, page, perPage int) ([]models.Appeal, int64, error) {
	offset := (page - 1) * perPage
	if offset < 0 {
		offset = 0
	}
	query := `
		SELECT a.id, a.moderation_action_id, a.appellant_id, a.statement, a.state,
		       a.resolver_id, COALESCE(a.resolution_note, ''), a.created_at, a.updated_at, a.resolved_at,
		       ma.template_id, ma.action_type, t.title AS template_title,
		       u.display_name AS appellant_display_name
		FROM appeals a
		JOIN moderation_actions ma ON ma.id = a.moderation_action_id
		JOIN templates t ON t.id = ma.template_id
		JOIN users u ON u.id = a.appellant_id
		WHERE ($1 = '' OR a.state = $1)
		ORDER BY a.created_at ASC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.QueryContext(ctx, query, state, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list appeal queue: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []models.Appeal
	for rows.Next() {
		var a models.Appeal
		var resolverID sql.NullString
		var resolvedAt sql.NullTime
		if err := rows.Scan(
			&a.ID, &a.ModerationActionID, &a.AppellantID, &a.Statement, &a.State,
			&resolverID, &a.ResolutionNote, &a.CreatedAt, &a.UpdatedAt, &resolvedAt,
			&a.TemplateID, &a.ActionType, &a.TemplateTitle,
			&a.AppellantDisplayName,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan queue row: %w", err)
		}
		if resolverID.Valid {
			a.ResolverID = resolverID.String
		}
		if resolvedAt.Valid {
			t := resolvedAt.Time
			a.ResolvedAt = &t
		}
		items = append(items, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}

	var total int64
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM appeals WHERE ($1 = '' OR state = $1)`, state,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count queue: %w", err)
	}
	return items, total, nil
}

// CountByState returns the per-state appeal counts for the queue tabs.
func (r *appealRepository) CountByState(ctx context.Context) (open, underReview, resolved int, err error) {
	query := `
		SELECT
			COUNT(*) FILTER (WHERE state = 'open'),
			COUNT(*) FILTER (WHERE state = 'under_review'),
			COUNT(*) FILTER (WHERE state IN ('resolved_upheld', 'resolved_reversed'))
		FROM appeals
	`
	if err := r.db.QueryRowContext(ctx, query).Scan(&open, &underReview, &resolved); err != nil {
		return 0, 0, 0, fmt.Errorf("failed to count appeals by state: %w", err)
	}
	return open, underReview, resolved, nil
}

// SetState transitions an appeal's state (admin-only). Returns
// ErrAppealNotFound when the id is unknown.
func (r *appealRepository) SetState(ctx context.Context, appealID, state string) error {
	query := `UPDATE appeals SET state = $2, updated_at = now() WHERE id = $1`
	res, err := r.db.ExecContext(ctx, query, appealID, state)
	if err != nil {
		return fmt.Errorf("failed to set appeal state: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to set appeal state: %w", err)
	}
	if rows == 0 {
		return ErrAppealNotFound
	}
	return nil
}

// ResolveAppeal atomically resolves an appeal and, when the outcome is
// "reversed", restores the template and flips the action status. The
// transaction locks the appeal row and re-checks state so a concurrent
// resolve is safe (FR-7).
func (r *appealRepository) ResolveAppeal(ctx context.Context, appealID, outcome, note, resolverID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin resolve transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 1) Lock the appeal row and re-check state.
	var state string
	var actionID string
	var templateID string
	err = tx.QueryRowContext(ctx,
		`SELECT a.state, a.moderation_action_id, ma.template_id
		 FROM appeals a JOIN moderation_actions ma ON ma.id = a.moderation_action_id
		 WHERE a.id = $1 FOR UPDATE`,
		appealID,
	).Scan(&state, &actionID, &templateID)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrAppealNotFound
		}
		return fmt.Errorf("failed to lock appeal: %w", err)
	}
	if state != "open" && state != "under_review" {
		return ErrAppealAlreadyResolved
	}

	// 2) Update the appeal.
	newState := "resolved_upheld"
	if outcome == "reversed" {
		newState = "resolved_reversed"
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE appeals
		 SET state = $2, resolver_id = $3, resolution_note = $4,
		     resolved_at = now(), updated_at = now()
		 WHERE id = $1`,
		appealID, newState, resolverID, note,
	); err != nil {
		return fmt.Errorf("failed to update appeal: %w", err)
	}

	// 3) If reversed, restore the template and mark the action reversed.
	if outcome == "reversed" {
		if _, err := tx.ExecContext(ctx,
			`UPDATE moderation_actions SET status = 'reversed' WHERE id = $1`, actionID,
		); err != nil {
			return fmt.Errorf("failed to mark action reversed: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE templates SET moderation_status = 'normal' WHERE id = $1`, templateID,
		); err != nil {
			return fmt.Errorf("failed to restore template: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit resolve transaction: %w", err)
	}
	return nil
}

// MarkActionSuperseded flips a moderation action to superseded (undone by a
// direct admin restore). Returns ErrActionNotFound when the id is unknown.
func (r *appealRepository) MarkActionSuperseded(ctx context.Context, actionID string) error {
	query := `UPDATE moderation_actions SET status = 'superseded', superseded_at = now() WHERE id = $1`
	res, err := r.db.ExecContext(ctx, query, actionID)
	if err != nil {
		return fmt.Errorf("failed to mark action superseded: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to mark action superseded: %w", err)
	}
	if rows == 0 {
		return ErrActionNotFound
	}
	return nil
}

// isUniqueViolation reports whether err is a Postgres 23505 unique violation.
func isUniqueViolation(err error) bool {
	var pgErr *pq.Error
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}