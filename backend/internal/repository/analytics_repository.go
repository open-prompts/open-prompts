package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"open-prompts/backend/internal/models"
)

// AnalyticsRepository is the data-access layer for prompt usage analytics.
// It reads/writes the template_usage_events table and aggregates the
// denormalized counters and the like/favorite series.
type AnalyticsRepository struct {
	db *sql.DB
}

// NewAnalyticsRepository creates a new AnalyticsRepository.
func NewAnalyticsRepository(db *sql.DB) *AnalyticsRepository {
	return &AnalyticsRepository{db: db}
}

// InsertEvent appends one usage event. Empty user/session ids are stored as
// NULL via NULLIF.
func (r *AnalyticsRepository) InsertEvent(ctx context.Context, templateID, eventType, source, userID, sessionID string) error {
	query := `
		INSERT INTO template_usage_events (template_id, event_type, source, user_id, session_id)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''))
	`
	if _, err := r.db.ExecContext(ctx, query, templateID, eventType, source, userID, sessionID); err != nil {
		return fmt.Errorf("failed to insert usage event: %w", err)
	}
	return nil
}

// RecentViewExists reports a view event for (template, session) newer than
// window. DB fallback for the Redis dedup key.
func (r *AnalyticsRepository) RecentViewExists(ctx context.Context, templateID, sessionID string, window time.Duration) (bool, error) {
	query := `
		SELECT 1 FROM template_usage_events
		WHERE template_id = $1 AND session_id = $2 AND event_type = 'view'
		  AND created_at > NOW() - $3::interval
		LIMIT 1
	`
	var one int
	err := r.db.QueryRowContext(ctx, query, templateID, sessionID, window.String()).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check recent view: %w", err)
	}
	return true, nil
}

// IncrementCounter bumps a denormalized counter. column is whitelisted by
// the caller ("view_count" | "use_count"); never interpolated from input.
func (r *AnalyticsRepository) IncrementCounter(ctx context.Context, templateID, column string) error {
	query := fmt.Sprintf("UPDATE templates SET %s = %s + 1 WHERE id = $1", column, column)
	if _, err := r.db.ExecContext(ctx, query, templateID); err != nil {
		return fmt.Errorf("failed to increment %s: %w", column, err)
	}
	return nil
}

// DailyEventSeries returns per-day view/use counts keyed by YYYY-MM-DD (UTC).
// Exactly one of templateID / ownerID is set (NULL scopes are skipped in SQL).
func (r *AnalyticsRepository) DailyEventSeries(ctx context.Context, templateID, ownerID string, from, to time.Time) (map[string][2]int32, error) {
	query := `
		SELECT to_char(date_trunc('day', e.created_at AT TIME ZONE 'UTC'), 'YYYY-MM-DD') AS day,
		       COUNT(*) FILTER (WHERE e.event_type = 'view')::int AS views,
		       COUNT(*) FILTER (WHERE e.event_type = 'use')::int AS uses
		FROM template_usage_events e
		JOIN templates t ON t.id = e.template_id
		WHERE e.created_at >= $1 AND e.created_at < $2
		  AND ($3::uuid IS NULL OR e.template_id = $3)
		  AND ($4::text  IS NULL OR t.owner_id = $4)
		GROUP BY 1
	`
	rows, err := r.db.QueryContext(ctx, query, from, to, nullUUID(templateID), nullText(ownerID))
	if err != nil {
		return nil, fmt.Errorf("failed to query daily event series: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string][2]int32)
	for rows.Next() {
		var day string
		var views, uses int32
		if err := rows.Scan(&day, &views, &uses); err != nil {
			return nil, fmt.Errorf("failed to scan daily event series: %w", err)
		}
		result[day] = [2]int32{views, uses}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error in daily event series: %w", err)
	}
	return result, nil
}

// DailyLikeSeries returns per-day like counts keyed by YYYY-MM-DD (UTC).
func (r *AnalyticsRepository) DailyLikeSeries(ctx context.Context, templateID, ownerID string, from, to time.Time) (map[string]int32, error) {
	query := `
		SELECT to_char(date_trunc('day', l.created_at AT TIME ZONE 'UTC'), 'YYYY-MM-DD') AS day,
		       COUNT(*)::int AS likes
		FROM template_likes l
		JOIN templates t ON t.id = l.template_id
		WHERE l.created_at >= $1 AND l.created_at < $2
		  AND ($3::uuid IS NULL OR l.template_id = $3)
		  AND ($4::text  IS NULL OR t.owner_id = $4)
		GROUP BY 1
	`
	return r.dailyCountSeries(ctx, query, from, to, templateID, ownerID, "daily like series")
}

// DailyFavoriteSeries returns per-day favorite counts keyed by YYYY-MM-DD (UTC).
func (r *AnalyticsRepository) DailyFavoriteSeries(ctx context.Context, templateID, ownerID string, from, to time.Time) (map[string]int32, error) {
	query := `
		SELECT to_char(date_trunc('day', f.created_at AT TIME ZONE 'UTC'), 'YYYY-MM-DD') AS day,
		       COUNT(*)::int AS favorites
		FROM template_favorites f
		JOIN templates t ON t.id = f.template_id
		WHERE f.created_at >= $1 AND f.created_at < $2
		  AND ($3::uuid IS NULL OR f.template_id = $3)
		  AND ($4::text  IS NULL OR t.owner_id = $4)
		GROUP BY 1
	`
	return r.dailyCountSeries(ctx, query, from, to, templateID, ownerID, "daily favorite series")
}

// dailyCountSeries runs a per-day COUNT query and returns the day->count map.
func (r *AnalyticsRepository) dailyCountSeries(ctx context.Context, query string, from, to time.Time, templateID, ownerID, label string) (map[string]int32, error) {
	rows, err := r.db.QueryContext(ctx, query, from, to, nullUUID(templateID), nullText(ownerID))
	if err != nil {
		return nil, fmt.Errorf("failed to query %s: %w", label, err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]int32)
	for rows.Next() {
		var day string
		var count int32
		if err := rows.Scan(&day, &count); err != nil {
			return nil, fmt.Errorf("failed to scan %s: %w", label, err)
		}
		result[day] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error in %s: %w", label, err)
	}
	return result, nil
}

// UseSourceSplit returns use-event counts per source for the window.
func (r *AnalyticsRepository) UseSourceSplit(ctx context.Context, templateID, ownerID string, from, to time.Time) (web, api int32, err error) {
	query := `
		SELECT e.source, COUNT(*)::int
		FROM template_usage_events e
		JOIN templates t ON t.id = e.template_id
		WHERE e.event_type = 'use' AND e.created_at >= $1 AND e.created_at < $2
		  AND ($3::uuid IS NULL OR e.template_id = $3)
		  AND ($4::text  IS NULL OR t.owner_id = $4)
		GROUP BY e.source
	`
	rows, err := r.db.QueryContext(ctx, query, from, to, nullUUID(templateID), nullText(ownerID))
	if err != nil {
		return 0, 0, fmt.Errorf("failed to query use source split: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var source string
		var count int32
		if err := rows.Scan(&source, &count); err != nil {
			return 0, 0, fmt.Errorf("failed to scan use source split: %w", err)
		}
		switch source {
		case "web":
			web = count
		case "api":
			api = count
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("rows error in use source split: %w", err)
	}
	return web, api, nil
}

// TopTemplatesByUses returns the owner's top templates in the window, ranked
// by uses then views.
func (r *AnalyticsRepository) TopTemplatesByUses(ctx context.Context, ownerID string, from, to time.Time, limit int) ([]models.TopTemplateRow, error) {
	query := `
		WITH ev AS (
		    SELECT template_id,
		           COUNT(*) FILTER (WHERE event_type = 'view')::int AS views,
		           COUNT(*) FILTER (WHERE event_type = 'use')::int AS uses
		    FROM template_usage_events
		    WHERE created_at >= $2 AND created_at < $3
		    GROUP BY template_id
		), lk AS (
		    SELECT template_id, COUNT(*)::int AS likes FROM template_likes
		    WHERE created_at >= $2 AND created_at < $3 GROUP BY template_id
		), fv AS (
		    SELECT template_id, COUNT(*)::int AS favorites FROM template_favorites
		    WHERE created_at >= $2 AND created_at < $3 GROUP BY template_id
		)
		SELECT t.id, t.title,
		       COALESCE(ev.views, 0), COALESCE(ev.uses, 0),
		       COALESCE(lk.likes, 0), COALESCE(fv.favorites, 0)
		FROM templates t
		LEFT JOIN ev ON ev.template_id = t.id
		LEFT JOIN lk ON lk.template_id = t.id
		LEFT JOIN fv ON fv.template_id = t.id
		WHERE t.owner_id = $1
		  AND (ev.template_id IS NOT NULL OR lk.template_id IS NOT NULL OR fv.template_id IS NOT NULL)
		ORDER BY COALESCE(ev.uses, 0) DESC, COALESCE(ev.views, 0) DESC
		LIMIT $4
	`
	rows, err := r.db.QueryContext(ctx, query, ownerID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top templates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var result []models.TopTemplateRow
	for rows.Next() {
		var row models.TopTemplateRow
		if err := rows.Scan(&row.ID, &row.Title, &row.Views, &row.Uses, &row.Likes, &row.Favorites); err != nil {
			return nil, fmt.Errorf("failed to scan top template: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error in top templates: %w", err)
	}
	return result, nil
}

// CountOwnerTemplates returns how many templates the user owns.
func (r *AnalyticsRepository) CountOwnerTemplates(ctx context.Context, ownerID string) (int32, error) {
	query := `SELECT COUNT(*)::int FROM templates WHERE owner_id = $1`
	var count int32
	if err := r.db.QueryRowContext(ctx, query, ownerID).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count owner templates: %w", err)
	}
	return count, nil
}

// nullUUID returns nil for an empty string so the SQL scope parameter
// disables the filter; otherwise the raw string (cast to uuid in SQL).
func nullUUID(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// nullText returns nil for an empty string so the SQL scope parameter
// disables the filter.
func nullText(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}