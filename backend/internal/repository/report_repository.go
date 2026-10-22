package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"open-prompts/backend/internal/models"

	"github.com/lib/pq"
)

// Report repository error sentinels.
var (
	// ErrReportNotFound is returned when a report row is unknown.
	ErrReportNotFound = errors.New("report not found")
)

// ReportRepository is the data-access interface for content reports.
type ReportRepository interface {
	// Upsert creates or replaces the caller's report on an item (FR-2:
	// re-report replaces reason/note and bumps recency; resets to open).
	Upsert(ctx context.Context, r *models.ContentReport) error
	// GetMine returns the caller's report on an item, or ErrReportNotFound.
	GetMine(ctx context.Context, reporterID, itemType, itemID string) (*models.ContentReport, error)
	// DeleteMine hard-deletes the caller's report on an item (withdraw).
	DeleteMine(ctx context.Context, reporterID, itemType, itemID string) error
	// ListQueue returns the grouped admin queue, open-first by count then
	// recency, paginated.
	ListQueue(ctx context.Context, status, itemType, reason string, limit, offset int) ([]*models.ReportGroup, int64, error)
	// ListForItem returns every report on an item (admin detail).
	ListForItem(ctx context.Context, itemType, itemID string) ([]*models.ContentReport, error)
	// ResolveItem atomically resolves all open reports of an item. Returns
	// the number of rows resolved (0 -> nothing to resolve, 409).
	ResolveItem(ctx context.Context, itemType, itemID, resolution, resolvedBy string) (int64, error)
	// CountOpen returns the number of open reports (dashboard KPI, FR-13).
	CountOpen(ctx context.Context) (int64, error)
	// AutoResolveItem resolves all open reports of an item as actioned
	// (FR-12 hook on unpublish/comment-delete).
	AutoResolveItem(ctx context.Context, itemType, itemID, resolvedBy string) error
}

type reportRepository struct {
	db *sql.DB
}

// NewReportRepository creates a new ReportRepository.
func NewReportRepository(db *sql.DB) ReportRepository {
	return &reportRepository{db: db}
}

// reportColumns is the shared SELECT column list for the content_reports
// table (without the joined reporter display name).
const reportColumns = `
	cr.id, cr.item_type, cr.item_id, cr.reporter_id, cr.reason,
	COALESCE(cr.note, ''), cr.status, COALESCE(cr.resolution, ''),
	COALESCE(cr.resolved_by, ''), cr.resolved_at, cr.created_at
`

// scanReport scans a content_reports row into a models.ContentReport.
func scanReport(scanner interface {
	Scan(dest ...interface{}) error
}) (*models.ContentReport, error) {
	var r models.ContentReport
	var resolvedAt sql.NullTime
	err := scanner.Scan(
		&r.ID, &r.ItemType, &r.ItemID, &r.ReporterID, &r.Reason,
		&r.Note, &r.Status, &r.Resolution,
		&r.ResolvedBy, &resolvedAt, &r.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if resolvedAt.Valid {
		t := resolvedAt.Time
		r.ResolvedAt = &t
	}
	return &r, nil
}

// Upsert inserts a report or, on a UNIQUE (item_type, item_id, reporter_id)
// violation, replaces the existing row (FR-2). The 23505 backstop maps to an
// upsert, never an error.
func (r *reportRepository) Upsert(ctx context.Context, rep *models.ContentReport) error {
	query := `
		INSERT INTO content_reports (item_type, item_id, reporter_id, reason, note)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (item_type, item_id, reporter_id)
		DO UPDATE SET reason = EXCLUDED.reason,
		              note = EXCLUDED.note,
		              status = 'open',
		              resolution = NULL,
		              resolved_by = NULL,
		              resolved_at = NULL,
		              created_at = now()
		RETURNING id, created_at
	`
	err := r.db.QueryRowContext(ctx, query, rep.ItemType, rep.ItemID, rep.ReporterID, rep.Reason, rep.Note).
		Scan(&rep.ID, &rep.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			// The ON CONFLICT clause already handles the unique violation;
			// this branch is a defensive backstop.
			return fmt.Errorf("failed to upsert report: %w", err)
		}
		return fmt.Errorf("failed to upsert report: %w", err)
	}
	return nil
}

// GetMine returns the caller's report on an item, or ErrReportNotFound.
func (r *reportRepository) GetMine(ctx context.Context, reporterID, itemType, itemID string) (*models.ContentReport, error) {
	query := `SELECT ` + reportColumns + ` FROM content_reports cr
		WHERE cr.reporter_id = $1 AND cr.item_type = $2 AND cr.item_id = $3`
	rep, err := scanReport(r.db.QueryRowContext(ctx, query, reporterID, itemType, itemID))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrReportNotFound
		}
		return nil, fmt.Errorf("failed to get report: %w", err)
	}
	return rep, nil
}

// DeleteMine hard-deletes the caller's report on an item (withdraw). It is
// idempotent: deleting a non-existent report is not an error.
func (r *reportRepository) DeleteMine(ctx context.Context, reporterID, itemType, itemID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM content_reports WHERE reporter_id = $1 AND item_type = $2 AND item_id = $3`,
		reporterID, itemType, itemID)
	if err != nil {
		return fmt.Errorf("failed to delete report: %w", err)
	}
	return nil
}

// ListQueue returns the grouped admin queue, open-first by open count then
// recency, paginated. Filters: status ("" = all), item_type ("" = all),
// reason ("" = all). The total counts the number of distinct items matching
// the filters.
func (r *reportRepository) ListQueue(ctx context.Context, status, itemType, reason string, limit, offset int) ([]*models.ReportGroup, int64, error) {
	where := `WHERE ($1 = '' OR cr.status = $1)
	          AND ($2 = '' OR cr.item_type = $2)
	          AND ($3 = '' OR cr.reason = $3)`
	query := `
		SELECT cr.item_type, cr.item_id,
		       COALESCE(t.title, c.content) AS item_title,
		       COALESCE(tu.display_name, cu.display_name) AS author_display_name,
		       COUNT(*) FILTER (WHERE cr.status = 'open') AS open_count,
		       array_agg(DISTINCT cr.reason) AS reasons,
		       MAX(cr.created_at) AS latest_at,
		       MAX(cr.status) AS status
		FROM content_reports cr
		LEFT JOIN templates t ON cr.item_type = 'template' AND t.id = cr.item_id
		LEFT JOIN users tu ON t.owner_id = tu.id
		LEFT JOIN template_comments c ON cr.item_type = 'comment' AND c.id = cr.item_id
		LEFT JOIN users cu ON c.user_id = cu.id
		` + where + `
		GROUP BY cr.item_type, cr.item_id, t.title, c.content, tu.display_name, cu.display_name
		ORDER BY open_count DESC, latest_at DESC
		LIMIT $4 OFFSET $5
	`
	rows, err := r.db.QueryContext(ctx, query, status, itemType, reason, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list report queue: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var groups []*models.ReportGroup
	for rows.Next() {
		var g models.ReportGroup
		var reasons pq.StringArray
		if err := rows.Scan(
			&g.ItemType, &g.ItemID, &g.ItemTitle, &g.AuthorDisplayName,
			&g.OpenCount, &reasons, &g.LatestAt, &g.Status,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan report group: %w", err)
		}
		g.Reasons = reasons
		groups = append(groups, &g)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}

	var total int64
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM (
			SELECT 1 FROM content_reports cr `+where+` GROUP BY cr.item_type, cr.item_id
		) sub`, status, itemType, reason,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count report queue: %w", err)
	}
	return groups, total, nil
}

// ListForItem returns every report on an item (admin detail), newest first,
// with the reporter's display name joined (admin-only, NFR-2).
func (r *reportRepository) ListForItem(ctx context.Context, itemType, itemID string) ([]*models.ContentReport, error) {
	query := `
		SELECT ` + reportColumns + `, COALESCE(u.display_name, '') AS reporter_display_name
		FROM content_reports cr
		LEFT JOIN users u ON u.id = cr.reporter_id
		WHERE cr.item_type = $1 AND cr.item_id = $2
		ORDER BY cr.created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, itemType, itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to list reports for item: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []*models.ContentReport
	for rows.Next() {
		var rep models.ContentReport
		var resolvedAt sql.NullTime
		if err := rows.Scan(
			&rep.ID, &rep.ItemType, &rep.ItemID, &rep.ReporterID, &rep.Reason,
			&rep.Note, &rep.Status, &rep.Resolution,
			&rep.ResolvedBy, &resolvedAt, &rep.CreatedAt,
			&rep.ReporterDisplayName,
		); err != nil {
			return nil, fmt.Errorf("failed to scan report: %w", err)
		}
		if resolvedAt.Valid {
			t := resolvedAt.Time
			rep.ResolvedAt = &t
		}
		items = append(items, &rep)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return items, nil
}

// ResolveItem atomically resolves all open reports of an item. Returns the
// number of rows resolved (0 -> nothing to resolve, mapped to 409).
func (r *reportRepository) ResolveItem(ctx context.Context, itemType, itemID, resolution, resolvedBy string) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE content_reports
		 SET status = 'resolved',
		     resolution = $3,
		     resolved_by = $4,
		     resolved_at = now()
		 WHERE item_type = $1 AND item_id = $2 AND status = 'open'`,
		itemType, itemID, resolution, resolvedBy)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve reports: %w", err)
	}
	return res.RowsAffected()
}

// CountOpen returns the number of open reports (dashboard KPI, FR-13).
func (r *reportRepository) CountOpen(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM content_reports WHERE status = 'open'`,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count open reports: %w", err)
	}
	return count, nil
}

// AutoResolveItem resolves all open reports of an item as actioned (FR-12
// hook on unpublish/comment-delete). Best-effort; failures are logged by the
// caller.
func (r *reportRepository) AutoResolveItem(ctx context.Context, itemType, itemID, resolvedBy string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE content_reports
		 SET status = 'resolved',
		     resolution = 'actioned',
		     resolved_by = $3,
		     resolved_at = now()
		 WHERE item_type = $1 AND item_id = $2 AND status = 'open'`,
		itemType, itemID, resolvedBy)
	if err != nil {
		return fmt.Errorf("failed to auto-resolve reports: %w", err)
	}
	return nil
}
