package models

import (
	"time"
)

// Prompt represents the prompt model in the database.
// It maps to the "prompts" table.
type Prompt struct {
	ID         string            `json:"id"`
	TemplateID string            `json:"template_id"`
	VersionID  int32             `json:"version_id"`
	OwnerID    string            `json:"owner_id"`
	Variables  map[string]string `json:"variables"` // Stored as JSONB in DB
	Content    string            `json:"content"`   // The actual rendered content
	CreatedAt  time.Time         `json:"created_at"`
	// TemplateTitle is the denormalized title of the source template,
	// populated by the list query's JOIN (empty on old payloads).
	TemplateTitle string `json:"template_title"`
}
