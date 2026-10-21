package models

import "time"

// ModerationAction records a moderation action (unpublish/lock/soft-delete)
// on a template. It maps to the "moderation_actions" table, which is
// populated by admin-template-moderation when it lands; the appeals feature
// only reads it.
type ModerationAction struct {
	ID           string     `json:"id"`
	TemplateID   string     `json:"template_id"`
	ActionType   string     `json:"action_type"` // "unpublish" | "lock" | "soft_delete"
	ActorID      string     `json:"actor_id"`
	ActorEmail   string     `json:"actor_email"`
	Reason       string     `json:"reason"`
	Status       string     `json:"status"` // "active" | "superseded" | "reversed"
	CreatedAt    time.Time  `json:"created_at"`
	SupersededAt *time.Time `json:"superseded_at"`
}

// Appeal is a creator's contest of a moderation action. It maps to the
// "appeals" table. State transitions are admin-only except the initial
// create.
type Appeal struct {
	ID                 string     `json:"id"`
	ModerationActionID string     `json:"moderation_action_id"`
	AppellantID        string     `json:"appellant_id"`
	Statement          string     `json:"statement"`
	State              string     `json:"state"` // open | under_review | resolved_upheld | resolved_reversed
	ResolverID         string     `json:"resolver_id"`
	ResolutionNote     string     `json:"resolution_note"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	ResolvedAt         *time.Time `json:"resolved_at"`

	// Joined at read time (not stored):
	TemplateID           string    `json:"template_id"`
	TemplateTitle        string    `json:"template_title"`
	ActionType           string    `json:"action_type"`
	ActionReason         string    `json:"action_reason"`
	ActorEmail           string    `json:"actor_email"`
	ActionCreatedAt      time.Time `json:"action_created_at"`
	ActionStatus         string    `json:"action_status"`
	AppellantDisplayName string    `json:"appellant_display_name"`
}