package service

import (
	"context"
	"database/sql"
	"fmt"
)

// TemplateCascadeService permanently removes a template and its owned
// children (versions, aliases, comments on it, likes/favorites/ratings).
// It is shared by the 30-day hard-delete ticker (admin-template-moderation)
// and account-deletion. The schema has no fork_source_id / template_watches /
// template_presets tables, so the cascade covers the tables that exist.
type TemplateCascadeService struct {
	db *sql.DB
}

// NewTemplateCascadeService creates a TemplateCascadeService.
func NewTemplateCascadeService(db *sql.DB) *TemplateCascadeService {
	return &TemplateCascadeService{db: db}
}

// HardDeleteTemplate permanently removes a template and its owned children in
// a single transaction. Idempotent: deleting an already-deleted template is a
// no-op.
func (s *TemplateCascadeService) HardDeleteTemplate(ctx context.Context, templateID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin cascade transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Owned children (FK cascade would handle most, but explicit deletes keep
	// the ordering deterministic and independent of FK definitions).
	for _, table := range []string{
		"template_versions",
		"template_aliases",
		"template_comments",
		"template_likes",
		"template_favorites",
		"template_ratings",
	} {
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM "+table+" WHERE template_id = $1", templateID); err != nil {
			return fmt.Errorf("failed to delete %s: %w", table, err)
		}
	}

	// The template row itself.
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM templates WHERE id = $1", templateID); err != nil {
		return fmt.Errorf("failed to delete template: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit cascade: %w", err)
	}
	return nil
}
