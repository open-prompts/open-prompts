package models

import (
	"database/sql"
	"time"
)

// Notification is an engagement event notification for a template owner.
// It maps to the "notifications" table.
type Notification struct {
	ID               string     `json:"id"`
	RecipientID      string     `json:"recipient_id"`
	ActorID          string     `json:"actor_id"`
	TemplateID       string     `json:"template_id"`
	Type             string     `json:"type"` // "like" | "favorite" | "fork"
	ActorDisplayName string     `json:"actor_display_name"`
	ReadAt           *time.Time `json:"read_at"`
	CreatedAt        time.Time  `json:"created_at"`

	// Snippet is a ≤100-char excerpt; only populated for comment_reply events.
	Snippet string `json:"snippet"`

	// Joined at read time (not stored):
	ActorAvatar   sql.NullString `json:"actor_avatar"`
	TemplateTitle string         `json:"template_title"`
}
