package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"open-prompts/backend/internal/models"
)

// FollowRepository is the data-access interface for the follow graph.
type FollowRepository interface {
	// Follow inserts a follow edge (idempotent) and bumps both counters in
	// one transaction. Returns the followee's new follower_count and whether
	// a new edge was actually inserted.
	Follow(ctx context.Context, followerID, followeeID string) (followerCount int32, inserted bool, err error)
	// Unfollow deletes a follow edge (idempotent) and decrements both
	// counters in one transaction. Returns the followee's new follower_count
	// and whether an edge was actually removed.
	Unfollow(ctx context.Context, followerID, followeeID string) (followerCount int32, removed bool, err error)
	// IsFollowing reports whether followerID currently follows followeeID.
	IsFollowing(ctx context.Context, followerID, followeeID string) (bool, error)
	// GetFollowCounts returns the user's follower and following counts.
	GetFollowCounts(ctx context.Context, userID string) (followers, following int32, err error)
	// ListFollowers returns a page of the user's followers (newest edge
	// first) plus the total count.
	ListFollowers(ctx context.Context, userID string, page, pageSize int) ([]*models.UserCard, int, error)
	// ListFollowing returns a page of the users the given user follows
	// (newest edge first) plus the total count.
	ListFollowing(ctx context.Context, userID string, page, pageSize int) ([]*models.UserCard, int, error)
	// GetFollowingFeed returns public templates from the creators the user
	// follows, newest first, keyset-paginated by created_at. Banned creators
	// and private templates are excluded in SQL.
	GetFollowingFeed(ctx context.Context, userID string, pageSize int, beforeCreatedAt time.Time) ([]*models.Template, int, error)
	// TopCreatorsByFollowers returns the creators with the most followers
	// (public user-card shape only).
	TopCreatorsByFollowers(ctx context.Context, limit int) ([]*models.UserCard, error)
}

type followRepository struct {
	db *sql.DB
}

// NewFollowRepository creates a new FollowRepository.
func NewFollowRepository(db *sql.DB) FollowRepository {
	return &followRepository{db: db}
}

// Follow inserts a follow edge and bumps both counters transactionally.
// The edge insert is idempotent (ON CONFLICT DO NOTHING); counters move only
// when a row was actually inserted.
func (r *followRepository) Follow(ctx context.Context, followerID, followeeID string) (int32, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, fmt.Errorf("failed to begin follow tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO user_follows (follower_id, followee_id) VALUES ($1, $2)
		 ON CONFLICT (follower_id, followee_id) DO NOTHING`,
		followerID, followeeID)
	if err != nil {
		return 0, false, fmt.Errorf("failed to insert follow edge: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, false, fmt.Errorf("failed to read follow insert result: %w", err)
	}
	inserted := rows > 0
	if inserted {
		if _, err := tx.ExecContext(ctx,
			`UPDATE users SET follower_count = follower_count + 1 WHERE id = $1`, followeeID); err != nil {
			return 0, false, fmt.Errorf("failed to bump follower_count: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE users SET following_count = following_count + 1 WHERE id = $1`, followerID); err != nil {
			return 0, false, fmt.Errorf("failed to bump following_count: %w", err)
		}
	}

	var count int32
	if err := tx.QueryRowContext(ctx,
		`SELECT follower_count FROM users WHERE id = $1`, followeeID).Scan(&count); err != nil {
		return 0, false, fmt.Errorf("failed to read follower_count: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, false, fmt.Errorf("failed to commit follow tx: %w", err)
	}
	return count, inserted, nil
}

// Unfollow deletes a follow edge and decrements both counters
// transactionally. Counters floor at 0 to stay robust against drift.
func (r *followRepository) Unfollow(ctx context.Context, followerID, followeeID string) (int32, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, fmt.Errorf("failed to begin unfollow tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`DELETE FROM user_follows WHERE follower_id = $1 AND followee_id = $2`,
		followerID, followeeID)
	if err != nil {
		return 0, false, fmt.Errorf("failed to delete follow edge: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, false, fmt.Errorf("failed to read unfollow delete result: %w", err)
	}
	removed := rows > 0
	if removed {
		if _, err := tx.ExecContext(ctx,
			`UPDATE users SET follower_count = GREATEST(follower_count - 1, 0) WHERE id = $1`, followeeID); err != nil {
			return 0, false, fmt.Errorf("failed to decrement follower_count: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE users SET following_count = GREATEST(following_count - 1, 0) WHERE id = $1`, followerID); err != nil {
			return 0, false, fmt.Errorf("failed to decrement following_count: %w", err)
		}
	}

	var count int32
	if err := tx.QueryRowContext(ctx,
		`SELECT follower_count FROM users WHERE id = $1`, followeeID).Scan(&count); err != nil {
		return 0, false, fmt.Errorf("failed to read follower_count: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, false, fmt.Errorf("failed to commit unfollow tx: %w", err)
	}
	return count, removed, nil
}

// IsFollowing reports whether followerID currently follows followeeID.
func (r *followRepository) IsFollowing(ctx context.Context, followerID, followeeID string) (bool, error) {
	var exists bool
	if err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM user_follows WHERE follower_id = $1 AND followee_id = $2)`,
		followerID, followeeID).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check follow edge: %w", err)
	}
	return exists, nil
}

// GetFollowCounts returns the user's follower and following counts.
func (r *followRepository) GetFollowCounts(ctx context.Context, userID string) (int32, int32, error) {
	var followers, following int32
	if err := r.db.QueryRowContext(ctx,
		`SELECT follower_count, following_count FROM users WHERE id = $1`, userID,
	).Scan(&followers, &following); err != nil {
		return 0, 0, fmt.Errorf("failed to get follow counts: %w", err)
	}
	return followers, following, nil
}

const userCardColumns = `u.id, u.display_name, COALESCE(u.avatar, ''), u.follower_count`

func scanUserCard(row interface{ Scan(...interface{}) error }) (*models.UserCard, error) {
	var c models.UserCard
	if err := row.Scan(&c.ID, &c.DisplayName, &c.Avatar, &c.FollowerCount); err != nil {
		return nil, err
	}
	return &c, nil
}

// ListFollowers returns a page of the user's followers (newest edge first).
func (r *followRepository) ListFollowers(ctx context.Context, userID string, page, pageSize int) ([]*models.UserCard, int, error) {
	offset := (page - 1) * pageSize
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+userCardColumns+`
		 FROM user_follows f
		 JOIN users u ON u.id = f.follower_id
		 WHERE f.followee_id = $1
		 ORDER BY f.created_at DESC
		 LIMIT $2 OFFSET $3`,
		userID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list followers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var cards []*models.UserCard
	for rows.Next() {
		c, err := scanUserCard(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan follower: %w", err)
		}
		cards = append(cards, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate followers: %w", err)
	}

	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_follows WHERE followee_id = $1`, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count followers: %w", err)
	}
	return cards, total, nil
}

// ListFollowing returns a page of the users the given user follows.
func (r *followRepository) ListFollowing(ctx context.Context, userID string, page, pageSize int) ([]*models.UserCard, int, error) {
	offset := (page - 1) * pageSize
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+userCardColumns+`
		 FROM user_follows f
		 JOIN users u ON u.id = f.followee_id
		 WHERE f.follower_id = $1
		 ORDER BY f.created_at DESC
		 LIMIT $2 OFFSET $3`,
		userID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list following: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var cards []*models.UserCard
	for rows.Next() {
		c, err := scanUserCard(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan following: %w", err)
		}
		cards = append(cards, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate following: %w", err)
	}

	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_follows WHERE follower_id = $1`, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count following: %w", err)
	}
	return cards, total, nil
}

// GetFollowingFeed returns public templates from followed creators, newest
// first, keyset-paginated by created_at. Banned creators and private
// templates are excluded in SQL.
func (r *followRepository) GetFollowingFeed(ctx context.Context, userID string, pageSize int, beforeCreatedAt time.Time) ([]*models.Template, int, error) {
	query := `
		SELECT t.id, t.owner_id, t.title, t.description, t.visibility, t.type, t.tags, t.category, t.language,
		       t.like_count, t.favorite_count, t.comment_count, t.view_count, t.use_count,
		       t.moderation_status, t.published_at,
		       t.content_warnings, t.locked_warnings,
		       t.created_at, t.updated_at,
		       u.display_name AS owner_display_name, COALESCE(u.avatar, '') AS owner_avatar
		FROM user_follows f
		JOIN templates t ON t.owner_id = f.followee_id
		JOIN users u ON u.id = t.owner_id
		WHERE f.follower_id = $1
		  AND t.visibility = 'public'
		  AND t.created_at < $2
		  AND t.moderation_status = 'normal'
		  AND NOT EXISTS (SELECT 1 FROM users bu WHERE bu.id = t.owner_id AND bu.status = 'banned')
		ORDER BY t.created_at DESC
		LIMIT $3`
	rows, err := r.db.QueryContext(ctx, query, userID, beforeCreatedAt, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query following feed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var templates []*models.Template
	for rows.Next() {
		var t models.Template
		if err := rows.Scan(
			&t.ID, &t.OwnerID, &t.Title, &t.Description, &t.Visibility, &t.Type,
			&t.Tags, &t.Category, &t.Language, &t.LikeCount, &t.FavoriteCount, &t.CommentCount,
			&t.ViewCount, &t.UseCount,
			&t.ModerationStatus, &t.PublishedAt, &t.ContentWarnings, &t.LockedWarnings,
			&t.CreatedAt, &t.UpdatedAt,
			&t.OwnerDisplayName, &t.OwnerAvatar,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan feed template: %w", err)
		}
		templates = append(templates, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate feed templates: %w", err)
	}

	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*)
		 FROM user_follows f
		 JOIN templates t ON t.owner_id = f.followee_id
		 JOIN users u ON u.id = t.owner_id
		 WHERE f.follower_id = $1
		   AND t.visibility = 'public'
		   AND t.created_at < $2
		   AND t.moderation_status = 'normal'
		   AND NOT EXISTS (SELECT 1 FROM users bu WHERE bu.id = t.owner_id AND bu.status = 'banned')`,
		userID, beforeCreatedAt).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count following feed: %w", err)
	}
	return templates, total, nil
}

// TopCreatorsByFollowers returns the creators with the most followers.
func (r *followRepository) TopCreatorsByFollowers(ctx context.Context, limit int) ([]*models.UserCard, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+userCardColumns+`
		 FROM users u
		 WHERE u.status <> 'banned'
		 ORDER BY u.follower_count DESC, u.created_at ASC
		 LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list top creators: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var cards []*models.UserCard
	for rows.Next() {
		c, err := scanUserCard(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan top creator: %w", err)
		}
		cards = append(cards, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate top creators: %w", err)
	}
	return cards, nil
}
