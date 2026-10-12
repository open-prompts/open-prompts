package repository

import (
	"context"
	"database/sql"
	"fmt"

	"open-prompts/backend/internal/models"
)

// NotificationRepository is the data-access interface for engagement
// notifications. All reads and mutations are scoped by recipientID.
type NotificationRepository interface {
	// Create inserts a notification unless an identical unread one exists
	// (toggle-spam dedupe). A deduped insert is NOT an error.
	Create(ctx context.Context, n *models.Notification) error
	// List returns a page of the recipient's notifications plus the
	// recipient's total unread count (badge reconciliation in one call).
	List(ctx context.Context, recipientID, filter string, limit, offset int) ([]*models.Notification, int, error)
	// CountUnread returns the recipient's unread count.
	CountUnread(ctx context.Context, recipientID string) (int, error)
	// MarkRead marks one notification read; returns false when the id is
	// unknown or owned by another user.
	MarkRead(ctx context.Context, recipientID, id string) (bool, error)
	// MarkAllRead marks every unread notification of the recipient read and
	// returns the number of rows updated.
	MarkAllRead(ctx context.Context, recipientID string) (int64, error)
	// Delete removes one notification; returns false when the id is unknown
	// or owned by another user.
	Delete(ctx context.Context, recipientID, id string) (bool, error)
	// PurgeExpired deletes read notifications older than readRetentionDays
	// and all notifications older than retentionDays.
	PurgeExpired(ctx context.Context, readRetentionDays, retentionDays int) error
}

type notificationRepository struct {
	db *sql.DB
}

// NewNotificationRepository creates a new NotificationRepository.
func NewNotificationRepository(db *sql.DB) NotificationRepository {
	return &notificationRepository{db: db}
}

// Create inserts a notification with toggle-spam dedupe: if an identical
// (recipient, actor, template, type) row is still unread, the insert is
// silently skipped (zero rows returned is not an error).
func (r *notificationRepository) Create(ctx context.Context, n *models.Notification) error {
	query := `
		INSERT INTO notifications (recipient_id, actor_id, template_id, type, actor_display_name)
		SELECT $1, $2, $3, $4, $5
		WHERE NOT EXISTS (
			SELECT 1 FROM notifications
			WHERE recipient_id = $1 AND actor_id = $2 AND template_id = $3
			  AND type = $4 AND read_at IS NULL
		)
		RETURNING id, created_at`
	err := r.db.QueryRowContext(ctx, query,
		n.RecipientID, n.ActorID, n.TemplateID, n.Type, n.ActorDisplayName,
	).Scan(&n.ID, &n.CreatedAt)
	if err == sql.ErrNoRows {
		// Deduped: an identical unread notification already exists.
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to insert notification: %w", err)
	}
	return nil
}

const notificationListQuery = `
	SELECT n.id, n.type, n.actor_id, n.actor_display_name, u.avatar,
	       n.template_id, t.title, n.read_at, n.created_at
	FROM notifications n
	LEFT JOIN users u ON u.id = n.actor_id
	LEFT JOIN templates t ON t.id = n.template_id
	WHERE n.recipient_id = $1
	  AND ($2 = 'unread' AND n.read_at IS NULL OR $2 = 'all')
	ORDER BY n.created_at DESC
	LIMIT $3 OFFSET $4`

// List returns a page of notifications plus the recipient's unread count.
func (r *notificationRepository) List(ctx context.Context, recipientID, filter string, limit, offset int) ([]*models.Notification, int, error) {
	rows, err := r.db.QueryContext(ctx, notificationListQuery, recipientID, filter, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list notifications: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var notifications []*models.Notification
	for rows.Next() {
		n := &models.Notification{}
		if err := rows.Scan(
			&n.ID, &n.Type, &n.ActorID, &n.ActorDisplayName, &n.ActorAvatar,
			&n.TemplateID, &n.TemplateTitle, &n.ReadAt, &n.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan notification: %w", err)
		}
		notifications = append(notifications, n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate notifications: %w", err)
	}

	unreadCount, err := r.CountUnread(ctx, recipientID)
	if err != nil {
		return nil, 0, err
	}
	return notifications, unreadCount, nil
}

// CountUnread returns the recipient's unread count via the partial index.
func (r *notificationRepository) CountUnread(ctx context.Context, recipientID string) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE recipient_id = $1 AND read_at IS NULL`,
		recipientID,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count unread notifications: %w", err)
	}
	return count, nil
}

// MarkRead marks one notification read (idempotent). Unknown or foreign ids
// return found=false so the service can answer 404 without leaking existence.
func (r *notificationRepository) MarkRead(ctx context.Context, recipientID, id string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET read_at = now()
		 WHERE id = $1 AND recipient_id = $2 AND read_at IS NULL`,
		id, recipientID)
	if err != nil {
		return false, fmt.Errorf("failed to mark notification read: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to read mark-read result: %w", err)
	}
	if rows == 0 {
		// Either already read (idempotent success) or not owned/missing.
		// Distinguish with an existence probe scoped to the recipient.
		var exists bool
		if err := r.db.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM notifications WHERE id = $1 AND recipient_id = $2)`,
			id, recipientID,
		).Scan(&exists); err != nil {
			return false, fmt.Errorf("failed to probe notification: %w", err)
		}
		return exists, nil
	}
	return true, nil
}

// MarkAllRead marks every unread notification of the recipient read.
func (r *notificationRepository) MarkAllRead(ctx context.Context, recipientID string) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET read_at = now()
		 WHERE recipient_id = $1 AND read_at IS NULL`,
		recipientID)
	if err != nil {
		return 0, fmt.Errorf("failed to mark all notifications read: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to read mark-all result: %w", err)
	}
	return rows, nil
}

// Delete removes one notification scoped to the recipient.
func (r *notificationRepository) Delete(ctx context.Context, recipientID, id string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM notifications WHERE id = $1 AND recipient_id = $2`,
		id, recipientID)
	if err != nil {
		return false, fmt.Errorf("failed to delete notification: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to read delete result: %w", err)
	}
	return rows > 0, nil
}

// PurgeExpired deletes expired notifications (lazy retention).
func (r *notificationRepository) PurgeExpired(ctx context.Context, readRetentionDays, retentionDays int) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM notifications
		 WHERE read_at < now() - ($1 || ' days')::interval
		    OR created_at < now() - ($2 || ' days')::interval`,
		readRetentionDays, retentionDays)
	if err != nil {
		return fmt.Errorf("failed to purge notifications: %w", err)
	}
	return nil
}
