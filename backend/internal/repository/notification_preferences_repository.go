package repository

import (
	"context"
	"database/sql"
	"fmt"

	"open-prompts/backend/internal/models"
)

// NotificationPreferencesRepository is the data-access interface for the
// per-user notification preferences table. Absent rows mean all-defaults.
type NotificationPreferencesRepository interface {
	// Get returns the effective preferences for a user. When no row exists,
	// it returns the all-defaults value with found=false (no error).
	Get(ctx context.Context, userID string) (*models.NotificationPreferences, bool, error)
	// Upsert creates the row on first change or updates the provided fields.
	// Only the fields with non-nil pointers are written (nil = leave
	// unchanged).
	Upsert(ctx context.Context, userID string, like, favorite, fork, comment, follow, paused *bool) error
}

type notificationPreferencesRepository struct {
	db *sql.DB
}

// NewNotificationPreferencesRepository creates a new
// NotificationPreferencesRepository.
func NewNotificationPreferencesRepository(db *sql.DB) NotificationPreferencesRepository {
	return &notificationPreferencesRepository{db: db}
}

// Get performs a single PK point read. Absent row → defaults (found=false).
func (r *notificationPreferencesRepository) Get(ctx context.Context, userID string) (*models.NotificationPreferences, bool, error) {
	prefs := &models.NotificationPreferences{UserID: userID}
	err := r.db.QueryRowContext(ctx,
		`SELECT user_id, like_enabled, favorite_enabled, fork_enabled,
		        comment_enabled, follow_enabled, paused_all, updated_at
		 FROM notification_preferences
		 WHERE user_id = $1`,
		userID,
	).Scan(&prefs.UserID, &prefs.LikeEnabled, &prefs.FavoriteEnabled, &prefs.ForkEnabled,
		&prefs.CommentEnabled, &prefs.FollowEnabled, &prefs.PausedAll, &prefs.UpdatedAt)
	if err == sql.ErrNoRows {
		return prefs, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to load notification preferences: %w", err)
	}
	return prefs, true, nil
}

// Upsert lazily creates the row on first change and applies a partial update.
// The caller passes *bool pointers (nil = leave unchanged); nil is converted
// to NULL so COALESCE keeps the existing value.
func (r *notificationPreferencesRepository) Upsert(ctx context.Context, userID string, like, favorite, fork, comment, follow, paused *bool) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO notification_preferences
		    (user_id, like_enabled, favorite_enabled, fork_enabled,
		     comment_enabled, follow_enabled, paused_all, updated_at)
		 VALUES ($1, COALESCE($2, true), COALESCE($3, true), COALESCE($4, true),
		         COALESCE($5, true), COALESCE($6, true), COALESCE($7, false), now())
		 ON CONFLICT (user_id) DO UPDATE SET
		    like_enabled = COALESCE($2, notification_preferences.like_enabled),
		    favorite_enabled = COALESCE($3, notification_preferences.favorite_enabled),
		    fork_enabled = COALESCE($4, notification_preferences.fork_enabled),
		    comment_enabled = COALESCE($5, notification_preferences.comment_enabled),
		    follow_enabled = COALESCE($6, notification_preferences.follow_enabled),
		    paused_all = COALESCE($7, notification_preferences.paused_all),
		    updated_at = now()`,
		userID, like, favorite, fork, comment, follow, paused)
	if err != nil {
		return fmt.Errorf("failed to upsert notification preferences: %w", err)
	}
	return nil
}
