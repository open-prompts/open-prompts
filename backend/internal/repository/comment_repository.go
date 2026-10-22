package repository

import (
	"context"
	"database/sql"
	"fmt"

	"open-prompts/backend/internal/models"
)

// CommentRepository is the data-access interface for template comments.
type CommentRepository interface {
	// List returns a page of TOP-LEVEL comments (newest first) plus the
	// template's total top-level comment count. Each comment carries
	// ReplyCount and HasCreatorReply (computed via subquery, no N+1).
	List(ctx context.Context, templateID string, limit, offset int) ([]*models.Comment, int64, error)
	// ListReplies returns all replies to a top-level comment, oldest-first.
	ListReplies(ctx context.Context, parentID string) ([]*models.Comment, error)
	// Get loads a single comment by id.
	Get(ctx context.Context, id string) (*models.Comment, error)
	// Create inserts a comment (top-level or reply) and maintains the
	// appropriate counter in one transaction: comment_count for top-level,
	// parent.reply_count for replies.
	Create(ctx context.Context, c *models.Comment) error
	// Update sets content and updated_at (never created_at, so list position
	// is stable) and returns the updated comment.
	Update(ctx context.Context, id, content string) (*models.Comment, error)
	// Delete removes a comment. If it is a top-level comment, its replies are
	// deleted in the same transaction and comment_count is recomputed. If it
	// is a reply, its parent's reply_count is decremented. Returns
	// (deleted, newCount, error); deleted=false when the row does not exist.
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
	       u.display_name, COALESCE(u.avatar, ''),
	       c.reply_count,
	       EXISTS (
	           SELECT 1 FROM template_comments r
	           WHERE r.parent_comment_id = c.id AND r.user_id = t.owner_id
	       ) AS has_creator_reply
	FROM template_comments c
	JOIN users u ON u.id = c.user_id
	JOIN templates t ON t.id = c.template_id
	WHERE c.template_id = $1 AND c.parent_comment_id IS NULL
	ORDER BY c.created_at DESC
	LIMIT $2 OFFSET $3`

// List returns a page of TOP-LEVEL comments (newest first) plus the total
// top-level comment count. reply_count and has_creator_reply are computed via
// correlated subqueries (no N+1; list pages ≤ 50 rows).
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
			&c.ReplyCount, &c.HasCreatorReply,
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
		`SELECT COUNT(*) FROM template_comments WHERE template_id = $1 AND parent_comment_id IS NULL`, templateID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count comments: %w", err)
	}
	return comments, total, nil
}

const commentListRepliesQuery = `
	SELECT c.id, c.template_id, c.user_id, c.content, c.created_at, c.updated_at,
	       u.display_name, COALESCE(u.avatar, ''),
	       c.reply_count
	FROM template_comments c
	JOIN users u ON u.id = c.user_id
	WHERE c.parent_comment_id = $1
	ORDER BY c.created_at ASC`

// ListReplies returns all replies to a top-level comment, oldest-first
// (conversational order), bounded by the 50-reply thread cap.
func (r *commentRepository) ListReplies(ctx context.Context, parentID string) ([]*models.Comment, error) {
	rows, err := r.db.QueryContext(ctx, commentListRepliesQuery, parentID)
	if err != nil {
		return nil, fmt.Errorf("failed to list replies: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var replies []*models.Comment
	for rows.Next() {
		c := &models.Comment{}
		if err := rows.Scan(
			&c.ID, &c.TemplateID, &c.UserID, &c.Content, &c.CreatedAt, &c.UpdatedAt,
			&c.AuthorDisplayName, &c.AuthorAvatar,
			&c.ReplyCount,
		); err != nil {
			return nil, fmt.Errorf("failed to scan reply: %w", err)
		}
		replies = append(replies, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate replies: %w", err)
	}
	return replies, nil
}

const commentGetQuery = `
	SELECT c.id, c.template_id, c.user_id, c.content, c.created_at, c.updated_at,
	       u.display_name, COALESCE(u.avatar, ''),
	       COALESCE(c.parent_comment_id::text, ''), c.reply_count
	FROM template_comments c
	JOIN users u ON u.id = c.user_id
	WHERE c.id = $1`

// Get loads a single comment by id (author identity joined).
func (r *commentRepository) Get(ctx context.Context, id string) (*models.Comment, error) {
	c := &models.Comment{}
	err := r.db.QueryRowContext(ctx, commentGetQuery, id).Scan(
		&c.ID, &c.TemplateID, &c.UserID, &c.Content, &c.CreatedAt, &c.UpdatedAt,
		&c.AuthorDisplayName, &c.AuthorAvatar,
		&c.ParentCommentID, &c.ReplyCount,
	)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get comment: %w", err)
	}
	return c, nil
}

// Create inserts a comment (top-level or reply) and maintains the
// appropriate counter in one transaction: comment_count for top-level,
// parent.reply_count for replies (ToggleLike pattern).
func (r *commentRepository) Create(ctx context.Context, c *models.Comment) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin comment tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	err = tx.QueryRowContext(ctx, `
		INSERT INTO template_comments (template_id, user_id, content, parent_comment_id)
		VALUES ($1, $2, $3, NULLIF($4, '')::uuid)
		RETURNING id, created_at, updated_at`,
		c.TemplateID, c.UserID, c.Content, c.ParentCommentID).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert comment: %w", err)
	}

	if c.ParentCommentID == "" {
		// Top-level: bump templates.comment_count.
		if _, err = tx.ExecContext(ctx,
			`UPDATE templates SET comment_count = comment_count + 1 WHERE id = $1`,
			c.TemplateID); err != nil {
			return fmt.Errorf("failed to bump comment_count: %w", err)
		}
	} else {
		// Reply: bump the parent's reply_count.
		if _, err = tx.ExecContext(ctx,
			`UPDATE template_comments SET reply_count = reply_count + 1 WHERE id = $1`,
			c.ParentCommentID); err != nil {
			return fmt.Errorf("failed to bump reply_count: %w", err)
		}
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

// Delete removes a comment. If it is a top-level comment, its replies are
// deleted in the same transaction and comment_count is recomputed
// (authoritative recount, avoids drift). If it is a reply, its parent's
// reply_count is decremented (floor at 0).
func (r *commentRepository) Delete(ctx context.Context, id string) (bool, int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, 0, fmt.Errorf("failed to begin delete tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var templateID, parentID string
	err = tx.QueryRowContext(ctx,
		`DELETE FROM template_comments WHERE id = $1 RETURNING template_id, COALESCE(parent_comment_id::text, '')`,
		id,
	).Scan(&templateID, &parentID)
	if err == sql.ErrNoRows {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, fmt.Errorf("failed to delete comment: %w", err)
	}

	if parentID == "" {
		// Top-level: delete its replies, then recount comment_count.
		if _, err = tx.ExecContext(ctx,
			`DELETE FROM template_comments WHERE parent_comment_id = $1`, id); err != nil {
			return false, 0, fmt.Errorf("failed to cascade delete replies: %w", err)
		}
		var count int64
		if err = tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM template_comments WHERE template_id = $1 AND parent_comment_id IS NULL`,
			templateID).Scan(&count); err != nil {
			return false, 0, fmt.Errorf("failed to recount comments: %w", err)
		}
		if _, err = tx.ExecContext(ctx,
			`UPDATE templates SET comment_count = $1 WHERE id = $2`, count, templateID); err != nil {
			return false, 0, fmt.Errorf("failed to update comment_count: %w", err)
		}
	} else {
		// Reply: decrement the parent's reply_count (floor at 0).
		if _, err = tx.ExecContext(ctx,
			`UPDATE template_comments SET reply_count = GREATEST(reply_count - 1, 0) WHERE id = $1`,
			parentID); err != nil {
			return false, 0, fmt.Errorf("failed to decrement reply_count: %w", err)
		}
	}
	return true, 0, tx.Commit()
}
