package models

import (
	"time"

	"github.com/lib/pq"
)

// WarningHistoryEntry is a single moderator warning change on a template,
// read from the admin_audit_logs table (action template.warning_add /
// template.warning_remove).
type WarningHistoryEntry struct {
	ID         string         `json:"id"`
	ActorID    string         `json:"actor_id"`
	ActorEmail string         `json:"actor_email"`
	Action     string         `json:"action"` // "template.warning_add" | "template.warning_remove"
	Warnings   pq.StringArray `json:"warnings"`
	Reason     string         `json:"reason"`
	CreatedAt  time.Time      `json:"created_at"`
}
