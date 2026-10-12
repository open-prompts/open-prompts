package repository

import (
	"context"
	"database/sql"
	"fmt"

	"open-prompts/backend/internal/models"
)

// CommentRepository is the data-access interface for template comments.
type CommentRepository interface {
	// List returns a page of a template's comments (newest first) plus the
	// template's total comment count.
	List(ctx context.Context, templateID string, limit, offset int) ([]*models.Comment, int64, error)
	// Get loads a single comment by id.
	Get(ctx context.Context, id string) (*models.Comment, error)
	// Create inserts a comment and increments templates.comment_count in one
	// transaction.
	Create(ctx context.Context, c *models.Comment) error
	// Update sets content and updated_at (never created_at, so list position
	// is stable) and returns the updated comment.
	Update(ctx context.Context, id, content string) (*models.Comment, error)
	// Delete removes the comment and recomputes templates.comment_count in
	// one transaction. Returns (deleted, newCount, error); deleted=false when
	// the row does not exist.
	Delete(ctx context.Context, id string) (bool, int64, error)
}

type commentRepository struct {
	db *sql.DB
}

// NewCommentRepository creates a new CommentRepository.
func NewCommentRepository(db *sql.DB) CommentRepository {
	return &commentRepository{db: db}
}

const commentListQuery = `
	SELECT c.id, c.template_id, c.user_id, c.content, c.created_at, c.updated_at,
	       u.display_name, COALESCE(u.avatar, '')
	FROM template_comments c
	JOIN users u ON u.id = c.user_id
	WHERE c.template_id = $1
	ORDER BY c.created_at DESC
	LIMIT $2 OFFSET $3`

// List returns a page of comments (newest first) plus the total count.
func (r *commentRepository) List(ctx context.Context, templateID string, limit, offset int) ([]*models.Comment, int64, error) {
	rows, err := r.db.QueryContext(ctx, commentListQuery, templateID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list comments: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var comments []*models.Comment
	for rows.Next() {
		c := &models.Comment{}
		if err := rows.Scan(
			&c.ID, &c.TemplateID, &c.UserID, &c.Content, &c.CreatedAt, &c.UpdatedAt,
			&c.AuthorDisplayName, &c.AuthorAvatar,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan comment: %w", err)
		}
		comments = append(comments, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate comments: %w", err)
	}

	var total int64
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM template_comments WHERE template_id = $1`, templateID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count comments: %w", err)
	}
	return comments, total, nil
}

const commentGetQuery = `
	SELECT c.id, c.template_id, c.user_id, c.content, c.created_at, c.updated_at,
	       u.display_name, COALESCE(u.avatar, '')
	FROM template_comments c
	JOIN users u ON u.id = c.user_id
	WHERE c.id = $1`

// Get loads a single comment by id (author identity joined).
func (r *commentRepository) Get(ctx context.Context, id string) (*models.Comment, error) {
	c := &models.Comment{}
	err := r.db.QueryRowContext(ctx, commentGetQuery, id).Scan(
		&c.ID, &c.TemplateID, &c.UserID, &c.Content, &c.CreatedAt, &c.UpdatedAt,
		&c.AuthorDisplayName, &c.AuthorAvatar,
	)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get comment: %w", err)
	}
	return c, nil
}

// Create inserts a comment and increments the template's counter in one
// transaction (ToggleLike pattern).
func (r *commentRepository) Create(ctx context.Context, c *models.Comment) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin comment tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	err = tx.QueryRowContext(ctx, `
		INSERT INTO template_comments (template_id, user_id, content)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at`,
		c.TemplateID, c.UserID, c.Content).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert comment: %w", err)
	}

	if _, err = tx.ExecContext(ctx,
		`UPDATE templates SET comment_count = comment_count + 1 WHERE id = $1`,
		c.TemplateID); err != nil {
		return fmt.Errorf("failed to bump comment_count: %w", err)
	}
	return tx.Commit()
}

// Update sets content and updated_at only — never created_at, so list
// position is stable (FR-3).
func (r *commentRepository) Update(ctx context.Context, id, content string) (*models.Comment, error) {
	c := &models.Comment{}
	err := r.db.QueryRowContext(ctx, `
		UPDATE template_comments SET content = $2, updated_at = NOW()
		WHERE id = $1
		RETURNING id, template_id, user_id, content, created_at, updated_at`,
		id, content).Scan(
		&c.ID, &c.TemplateID, &c.UserID, &c.Content, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("failed to update comment: %w", err)
	}
	return c, nil
}

// Delete removes the comment and recomputes templates.comment_count in one
// transaction (authoritative recount, avoids drift).
func (r *commentRepository) Delete(ctx context.Context, id string) (bool, int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, 0, fmt.Errorf("failed to begin delete tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var templateID string
	err = tx.QueryRowContext(ctx,
		`DELETE FROM template_comments WHERE id = $1 RETURNING template_id`, id,
	).Scan(&templateID)
	if err == sql.ErrNoRows {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, fmt.Errorf("failed to delete comment: %w", err)
	}

	var count int64
	if err = tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM template_comments WHERE template_id = $1`, templateID,
	).Scan(&count); err != nil {
		return false, 0, fmt.Errorf("failed to recount comments: %w", err)
	}
	if _, err = tx.ExecContext(ctx,
		`UPDATE templates SET comment_count = $1 WHERE id = $2`, count, templateID); err != nil {
		return false, 0, fmt.Errorf("failed to update comment_count: %w", err)
	}
	return true, count, tx.Commit()
}
