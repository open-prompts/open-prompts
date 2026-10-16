package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"open-prompts/backend/internal/models"
)

// ErrAlreadyDecided is returned when a decision is attempted on a
// submission that is no longer pending. The service maps it to
// FailedPrecondition.
var ErrAlreadyDecided = errors.New("submission already decided")

// ErrSubmissionNotFound is returned when a submission id is unknown.
var ErrSubmissionNotFound = errors.New("submission not found")

// ReviewQueueFilters carries the queue listing filters.
type ReviewQueueFilters struct {
	Category         string // "" = any
	AgingOnly        bool   // only submissions older than the aging window
	ResubmissionOnly bool   // only attempt_number >= 2
	AgingHours       int    // aging window in hours (from platform settings)
	Limit            int
	Offset           int
}

// ReviewRepository defines the interface for pre-publication review data
// access: the submission queue, platform settings, and creator gating state.
type ReviewRepository interface {
	CreateSubmission(ctx context.Context, s *models.ReviewSubmission) error
	GetSubmission(ctx context.Context, id string) (*models.ReviewSubmission, error)
	GetLatestSubmission(ctx context.Context, templateID string) (*models.ReviewSubmission, error)
	ListQueue(ctx context.Context, f ReviewQueueFilters) ([]*models.ReviewQueueRow, int64, error)
	GetSubmissionDetail(ctx context.Context, id string) (*models.ReviewSubmissionDetail, error)
	ListChain(ctx context.Context, templateID string) ([]*models.ReviewSubmission, error)
	// DecideSubmission atomically moves a pending submission to a decided
	// status. Returns ErrAlreadyDecided when the row is not pending and
	// ErrSubmissionNotFound when the id is unknown.
	DecideSubmission(ctx context.Context, id string, status string, decidedBy string,
		reasonCode, note *string) (*models.ReviewSubmission, error)
	GetMetrics(ctx context.Context) (*models.ReviewMetrics, error)
	// Platform settings
	GetSettings(ctx context.Context) ([]*models.PlatformSetting, error)
	GetSetting(ctx context.Context, key string) (string, error)
	UpsertSetting(ctx context.Context, key, value string) error
	// Gating state
	GetUserGatingState(ctx context.Context, userID string) (approved int32, gatedRemaining int32, err error)
	IncrementApprovedCount(ctx context.Context, userID string) error
	DecrementGatedRemaining(ctx context.Context, userID string) error
	SetGatedRemaining(ctx context.Context, userID string, value int32) error
}

// reviewRepository implements ReviewRepository.
type reviewRepository struct {
	db *sql.DB
}

// NewReviewRepository creates a new instance of ReviewRepository.
func NewReviewRepository(db *sql.DB) ReviewRepository {
	return &reviewRepository{db: db}
}

// CreateSubmission inserts a new review submission row.
func (r *reviewRepository) CreateSubmission(ctx context.Context, s *models.ReviewSubmission) error {
	query := `
		INSERT INTO review_submissions (
			template_id, creator_id, attempt_number, status, submitted_at
		) VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`
	err := r.db.QueryRowContext(ctx, query,
		s.TemplateID, s.CreatorID, s.AttemptNumber, s.Status, s.SubmittedAt,
	).Scan(&s.ID)
	if err != nil {
		return fmt.Errorf("failed to create review submission: %w", err)
	}
	return nil
}

// submissionColumns is the shared SELECT column list for review_submissions.
const submissionColumns = `
	rs.id, rs.template_id, rs.creator_id, rs.attempt_number, rs.status,
	rs.submitted_at, rs.decided_at, rs.decided_by,
	rs.reject_reason_code, rs.reject_note
`

// scanSubmission scans a submission row into a models.ReviewSubmission.
func scanSubmission(scanner interface {
	Scan(dest ...interface{}) error
}) (*models.ReviewSubmission, error) {
	var s models.ReviewSubmission
	err := scanner.Scan(
		&s.ID, &s.TemplateID, &s.CreatorID, &s.AttemptNumber, &s.Status,
		&s.SubmittedAt, &s.DecidedAt, &s.DecidedBy,
		&s.RejectReasonCode, &s.RejectNote,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetSubmission retrieves a submission by id.
func (r *reviewRepository) GetSubmission(ctx context.Context, id string) (*models.ReviewSubmission, error) {
	query := `SELECT ` + submissionColumns + ` FROM review_submissions rs WHERE rs.id = $1`
	s, err := scanSubmission(r.db.QueryRowContext(ctx, query, id))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrSubmissionNotFound
		}
		return nil, fmt.Errorf("failed to get review submission: %w", err)
	}
	return s, nil
}

// GetLatestSubmission retrieves the newest submission for a template.
func (r *reviewRepository) GetLatestSubmission(ctx context.Context, templateID string) (*models.ReviewSubmission, error) {
	query := `SELECT ` + submissionColumns + `
		FROM review_submissions rs
		WHERE rs.template_id = $1
		ORDER BY rs.attempt_number DESC
		LIMIT 1`
	s, err := scanSubmission(r.db.QueryRowContext(ctx, query, templateID))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrSubmissionNotFound
		}
		return nil, fmt.Errorf("failed to get latest review submission: %w", err)
	}
	return s, nil
}

// queueWhere builds the shared queue WHERE clause. agingHours is bound as
// $1; category $2; aging_only $3; resubmission_only $4.
func queueWhere(f ReviewQueueFilters) (string, []interface{}) {
	args := []interface{}{f.AgingHours, f.Category, f.AgingOnly, f.ResubmissionOnly}
	return `
		WHERE rs.status = 'pending'
		  AND ($2 = '' OR t.category = $2)
		  AND (NOT $3 OR rs.submitted_at < now() - ($1 * interval '1 hour'))
		  AND (NOT $4 OR rs.attempt_number >= 2)
	`, args
}

// queueOrderBy is aging-first, then FIFO (oldest submitted first).
const queueOrderBy = `
		ORDER BY (rs.submitted_at < now() - ($1 * interval '1 hour')) DESC,
		         rs.submitted_at ASC
`

// queueRowColumns is the SELECT list for queue rows (joins templates/users).
const queueRowColumns = `
	rs.id, rs.template_id, t.title, rs.creator_id, u.display_name,
	rs.attempt_number, rs.submitted_at, COALESCE(t.category, ''),
	(rs.submitted_at < now() - ($1 * interval '1 hour')) AS aging,
	u.approved_template_count,
	(SELECT COUNT(*) FROM review_submissions x
	 WHERE x.creator_id = rs.creator_id AND x.status = 'rejected') AS prior_rejections,
	(u.status = 'banned') AS creator_banned
`

// scanQueueRow scans one queue row.
func scanQueueRow(scanner interface {
	Scan(dest ...interface{}) error
}) (*models.ReviewQueueRow, error) {
	var row models.ReviewQueueRow
	err := scanner.Scan(
		&row.ID, &row.TemplateID, &row.TemplateTitle, &row.CreatorID, &row.CreatorName,
		&row.AttemptNumber, &row.SubmittedAt, &row.TemplateCategory,
		&row.Aging, &row.CreatorApproved, &row.PriorRejections, &row.CreatorBanned,
	)
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// ListQueue returns one page of pending submissions (aging-first, then
// FIFO) plus the total matching count.
func (r *reviewRepository) ListQueue(ctx context.Context, f ReviewQueueFilters) ([]*models.ReviewQueueRow, int64, error) {
	where, args := queueWhere(f)

	query := `SELECT ` + queueRowColumns + `
		FROM review_submissions rs
		JOIN templates t ON t.id = rs.template_id
		JOIN users u ON u.id = rs.creator_id
	` + where + queueOrderBy + " LIMIT $5 OFFSET $6"
	args = append(args, f.Limit, f.Offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list review queue: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []*models.ReviewQueueRow
	for rows.Next() {
		row, err := scanQueueRow(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan review queue row: %w", err)
		}
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}

	// Count query shares the WHERE (minus LIMIT/OFFSET).
	countQuery := `SELECT COUNT(*)
		FROM review_submissions rs
		JOIN templates t ON t.id = rs.template_id
		JOIN users u ON u.id = rs.creator_id
	` + where
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args[:4]...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count review queue: %w", err)
	}

	return items, total, nil
}

// GetSubmissionDetail assembles the full detail payload for one submission:
// template metadata, latest version content, creator summary, and the
// template's decision chain.
func (r *reviewRepository) GetSubmissionDetail(ctx context.Context, id string) (*models.ReviewSubmissionDetail, error) {
	query := `
		SELECT
			rs.id, rs.template_id, rs.creator_id, rs.attempt_number, rs.status,
			rs.submitted_at, rs.decided_at, rs.decided_by,
			rs.reject_reason_code, rs.reject_note,
			t.title, t.description, t.type, t.tags, COALESCE(t.category, ''), t.language,
			t.created_at, t.like_count, t.favorite_count, t.comment_count,
			u.display_name, u.created_at, u.approved_template_count,
			(SELECT COUNT(*) FROM review_submissions x
			 WHERE x.creator_id = rs.creator_id AND x.status = 'rejected') AS prior_rejections,
			(u.status = 'banned') AS creator_banned,
			tv.version, tv.content
		FROM review_submissions rs
		JOIN templates t ON t.id = rs.template_id
		JOIN users u ON u.id = rs.creator_id
		LEFT JOIN template_versions tv ON tv.id = (
			SELECT tv2.id FROM template_versions tv2
			WHERE tv2.template_id = t.id
			ORDER BY tv2.version DESC LIMIT 1
		)
		WHERE rs.id = $1
	`
	var d models.ReviewSubmissionDetail
	var versionNumber sql.NullInt32
	var versionContent sql.NullString
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&d.ID, &d.TemplateID, &d.CreatorID, &d.AttemptNumber, &d.Status,
		&d.SubmittedAt, &d.DecidedAt, &d.DecidedBy,
		&d.RejectReasonCode, &d.RejectNote,
		&d.TemplateTitle, &d.TemplateDescription, &d.TemplateType, &d.TemplateTags,
		&d.TemplateCategory, &d.TemplateLanguage,
		&d.TemplateCreatedAt, &d.TemplateLikeCount, &d.TemplateFavoriteCount, &d.TemplateCommentCount,
		&d.CreatorName, &d.CreatorCreatedAt, &d.CreatorApproved,
		&d.PriorRejections, &d.CreatorBanned,
		&versionNumber, &versionContent,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrSubmissionNotFound
		}
		return nil, fmt.Errorf("failed to get review submission detail: %w", err)
	}
	if versionNumber.Valid {
		d.VersionNumber = versionNumber.Int32
	}
	d.VersionContent = versionContent.String

	// Decision chain for this template (all attempts, oldest first).
	chain, err := r.ListChain(ctx, d.TemplateID)
	if err != nil {
		return nil, err
	}
	d.History = chain

	return &d, nil
}

// ListChain returns all submissions for a template, oldest attempt first.
func (r *reviewRepository) ListChain(ctx context.Context, templateID string) ([]*models.ReviewSubmission, error) {
	query := `SELECT ` + submissionColumns + `
		FROM review_submissions rs
		WHERE rs.template_id = $1
		ORDER BY rs.attempt_number ASC`
	rows, err := r.db.QueryContext(ctx, query, templateID)
	if err != nil {
		return nil, fmt.Errorf("failed to list review chain: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []*models.ReviewSubmission
	for rows.Next() {
		s, err := scanSubmission(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan review chain row: %w", err)
		}
		items = append(items, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return items, nil
}

// DecideSubmission atomically moves a pending submission to a decided
// status. The WHERE status='pending' guard makes the transition idempotent
// under races: exactly one caller wins.
func (r *reviewRepository) DecideSubmission(ctx context.Context, id string, status string, decidedBy string,
	reasonCode, note *string) (*models.ReviewSubmission, error) {
	query := `
		UPDATE review_submissions
		SET status = $2, decided_at = now(), decided_by = $3,
		    reject_reason_code = $4, reject_note = $5
		WHERE id = $1 AND status = 'pending'
		RETURNING template_id, creator_id, attempt_number, submitted_at
	`
	var templateID, creatorID string
	var attemptNumber int32
	var submittedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, query, id, status, decidedBy, reasonCode, note).Scan(
		&templateID, &creatorID, &attemptNumber, &submittedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			// Distinguish unknown id from already-decided.
			var exists bool
			if err := r.db.QueryRowContext(ctx,
				`SELECT EXISTS(SELECT 1 FROM review_submissions WHERE id = $1)`, id,
			).Scan(&exists); err == nil && exists {
				return nil, ErrAlreadyDecided
			}
			return nil, ErrSubmissionNotFound
		}
		return nil, fmt.Errorf("failed to decide review submission: %w", err)
	}
	return &models.ReviewSubmission{
		ID:               id,
		TemplateID:       templateID,
		CreatorID:        creatorID,
		AttemptNumber:    attemptNumber,
		Status:           status,
		SubmittedAt:      submittedAt.Time,
		DecidedAt:        sql.NullTime{Time: time.Now(), Valid: true},
		DecidedBy:        sql.NullString{String: decidedBy, Valid: true},
		RejectReasonCode: nullString(reasonCode),
		RejectNote:       nullString(note),
	}, nil
}

// nullString converts a *string to sql.NullString.
func nullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

// GetMetrics aggregates queue statistics.
func (r *reviewRepository) GetMetrics(ctx context.Context) (*models.ReviewMetrics, error) {
	m := &models.ReviewMetrics{}

	// Open count + median wait of currently-pending submissions.
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*),
		       COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY
		           EXTRACT(EPOCH FROM (now() - submitted_at)) / 3600.0), 0)
		FROM review_submissions WHERE status = 'pending'
	`).Scan(&m.PendingCount, &m.MedianHours)
	if err != nil {
		return nil, fmt.Errorf("failed to get review metrics: %w", err)
	}

	// Decisions today (approved + rejected).
	var approvedToday, rejectedToday int32
	err = r.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN status = 'approved' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'rejected' THEN 1 ELSE 0 END), 0)
		FROM review_submissions
		WHERE status <> 'pending' AND decided_at >= date_trunc('day', now())
	`).Scan(&approvedToday, &rejectedToday)
	if err != nil {
		return nil, fmt.Errorf("failed to get review decisions today: %w", err)
	}
	m.ApprovedToday = approvedToday
	m.RejectedToday = rejectedToday

	// Oldest pending age in whole hours.
	err = r.db.QueryRowContext(ctx, `
		SELECT COALESCE(EXTRACT(EPOCH FROM (now() - MIN(submitted_at))) / 3600.0, 0)::int
		FROM review_submissions WHERE status = 'pending'
	`).Scan(&m.OldestPendingHours)
	if err != nil {
		return nil, fmt.Errorf("failed to get oldest pending age: %w", err)
	}

	// Rejection rate by reason code (share of all decided rejections).
	rows, err := r.db.QueryContext(ctx, `
		SELECT reject_reason_code, COUNT(*),
		       COUNT(*)::float / NULLIF(SUM(COUNT(*)) OVER (), 0)
		FROM review_submissions
		WHERE status = 'rejected' AND reject_reason_code IS NOT NULL
		GROUP BY reject_reason_code
		ORDER BY COUNT(*) DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to get rejection rates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var e models.RejectionRateEntry
		if err := rows.Scan(&e.ReasonCode, &e.Count, &e.Rate); err != nil {
			return nil, fmt.Errorf("failed to scan rejection rate: %w", err)
		}
		m.RejectionRates = append(m.RejectionRates, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return m, nil
}

// GetSettings returns all platform settings.
func (r *reviewRepository) GetSettings(ctx context.Context) ([]*models.PlatformSetting, error) {
	query := `SELECT key, value, updated_at FROM platform_settings ORDER BY key`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get platform settings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []*models.PlatformSetting
	for rows.Next() {
		var s models.PlatformSetting
		if err := rows.Scan(&s.Key, &s.Value, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan platform setting: %w", err)
		}
		items = append(items, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return items, nil
}

// GetSetting returns the value for one platform setting key.
func (r *reviewRepository) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := r.db.QueryRowContext(ctx,
		`SELECT value FROM platform_settings WHERE key = $1`, key).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("setting %q not found", key)
		}
		return "", fmt.Errorf("failed to get platform setting: %w", err)
	}
	return value, nil
}

// UpsertSetting inserts or updates a platform setting.
func (r *reviewRepository) UpsertSetting(ctx context.Context, key, value string) error {
	query := `
		INSERT INTO platform_settings (key, value, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = now()
	`
	if _, err := r.db.ExecContext(ctx, query, key, value); err != nil {
		return fmt.Errorf("failed to upsert platform setting: %w", err)
	}
	return nil
}

// GetUserGatingState returns the creator's gating counters.
func (r *reviewRepository) GetUserGatingState(ctx context.Context, userID string) (int32, int32, error) {
	var approved, gatedRemaining int32
	err := r.db.QueryRowContext(ctx, `
		SELECT approved_template_count, review_gated_remaining
		FROM users WHERE id = $1
	`, userID).Scan(&approved, &gatedRemaining)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, 0, fmt.Errorf("user not found")
		}
		return 0, 0, fmt.Errorf("failed to get user gating state: %w", err)
	}
	return approved, gatedRemaining, nil
}

// IncrementApprovedCount bumps approved_template_count by one.
func (r *reviewRepository) IncrementApprovedCount(ctx context.Context, userID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET approved_template_count = approved_template_count + 1 WHERE id = $1`, userID)
	if err != nil {
		return fmt.Errorf("failed to increment approved count: %w", err)
	}
	return nil
}

// DecrementGatedRemaining decrements review_gated_remaining when > 0.
func (r *reviewRepository) DecrementGatedRemaining(ctx context.Context, userID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET review_gated_remaining = review_gated_remaining - 1
		 WHERE id = $1 AND review_gated_remaining > 0`, userID)
	if err != nil {
		return fmt.Errorf("failed to decrement gated remaining: %w", err)
	}
	return nil
}

// SetGatedRemaining sets review_gated_remaining to an exact value.
func (r *reviewRepository) SetGatedRemaining(ctx context.Context, userID string, value int32) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET review_gated_remaining = $2 WHERE id = $1`, userID, value)
	if err != nil {
		return fmt.Errorf("failed to set gated remaining: %w", err)
	}
	return nil
}
