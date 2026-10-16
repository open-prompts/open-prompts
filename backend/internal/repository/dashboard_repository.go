package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// DashboardActivityType enumerates recent-activity item kinds.
type DashboardActivityType int

const (
	DashboardActivityTemplate DashboardActivityType = iota + 1
	DashboardActivityPrompt
	DashboardActivityLike
	DashboardActivityFavorite
)

// DashboardActivity is one entry in the recent-activity feed.
type DashboardActivity struct {
	Type        DashboardActivityType
	EntityID    string
	EntityTitle string
	Timestamp   time.Time
	TemplateID  string // prompts only
}

// DashboardOverview aggregates a user's workspace counts and activity.
type DashboardOverview struct {
	TemplateCount   int32
	PromptCount     int32
	CollectionCount int32
	APIKeyCount     int32
	Activity        []DashboardActivity
}

// DashboardRepository aggregates a user's workspace counts and recent
// activity across templates, prompts, likes, favorites, and (optionally)
// collections. It is a read-only aggregation over existing tables.
type DashboardRepository struct {
	db *sql.DB
}

// NewDashboardRepository creates a new DashboardRepository.
func NewDashboardRepository(db *sql.DB) *DashboardRepository {
	return &DashboardRepository{db: db}
}

// GetOverview returns the user's resource counts and the 10 most recent
// activity items. The collections count degrades to 0 (with a warning log)
// when the collections table is absent (42P01), never a 500.
func (r *DashboardRepository) GetOverview(ctx context.Context, userID string) (*DashboardOverview, error) {
	overview := &DashboardOverview{}

	// template_count
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM templates WHERE owner_id = $1`, userID,
	).Scan(&overview.TemplateCount); err != nil {
		return nil, fmt.Errorf("failed to count templates: %w", err)
	}

	// prompt_count
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM prompts WHERE owner_id = $1`, userID,
	).Scan(&overview.PromptCount); err != nil {
		return nil, fmt.Errorf("failed to count prompts: %w", err)
	}

	// collection_count (feature-detection: 42P01 -> 0)
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM collections WHERE owner_id = $1 AND status = 'active'`, userID,
	).Scan(&overview.CollectionCount); err != nil {
		if isUndefinedTable(err) {
			zap.S().Warn("collections table absent; collection_count=0")
			overview.CollectionCount = 0
		} else {
			return nil, fmt.Errorf("failed to count collections: %w", err)
		}
	}

	// api_key_count
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM api_keys
		 WHERE user_id = $1 AND is_active = true
		   AND (expires_at IS NULL OR expires_at > NOW())`, userID,
	).Scan(&overview.APIKeyCount); err != nil {
		return nil, fmt.Errorf("failed to count api keys: %w", err)
	}

	// recent activity (UNION ALL, newest first, LIMIT 10)
	rows, err := r.db.QueryContext(ctx, `
		SELECT * FROM (
			SELECT 'template' AS type, t.id::text AS entity_id, t.title AS entity_title,
			       t.updated_at AS timestamp, '' AS template_id
			FROM templates t
			WHERE t.owner_id = $1
			UNION ALL
			SELECT 'prompt', p.id::text, t.title, p.created_at, t.id::text
			FROM prompts p
			JOIN templates t ON t.id = p.template_id
			WHERE p.owner_id = $1
			UNION ALL
			SELECT 'like', t.id::text, t.title, l.created_at, ''
			FROM template_likes l
			JOIN templates t ON t.id = l.template_id
			WHERE t.owner_id = $1
			UNION ALL
			SELECT 'favorite', t.id::text, t.title, f.created_at, ''
			FROM template_favorites f
			JOIN templates t ON t.id = f.template_id
			WHERE t.owner_id = $1
		) activity
		ORDER BY timestamp DESC
		LIMIT 10
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list recent activity: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var typ string
		var a DashboardActivity
		if err := rows.Scan(&typ, &a.EntityID, &a.EntityTitle, &a.Timestamp, &a.TemplateID); err != nil {
			return nil, fmt.Errorf("failed to scan activity row: %w", err)
		}
		switch typ {
		case "template":
			a.Type = DashboardActivityTemplate
		case "prompt":
			a.Type = DashboardActivityPrompt
		case "like":
			a.Type = DashboardActivityLike
		case "favorite":
			a.Type = DashboardActivityFavorite
		default:
			continue
		}
		overview.Activity = append(overview.Activity, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return overview, nil
}

// isUndefinedTable reports whether err is a Postgres 42P01 (undefined_table)
// error, used for graceful feature-detection of the collections table.
func isUndefinedTable(err error) bool {
	type pgError interface {
		SQLState() string
	}
	if pe, ok := err.(pgError); ok {
		return pe.SQLState() == "42P01"
	}
	return false
}
