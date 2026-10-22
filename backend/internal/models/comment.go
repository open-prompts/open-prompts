package models

import "time"

// Comment represents a row in the "template_comments" table.
type Comment struct {
	ID         string    `json:"id"`
	TemplateID string    `json:"template_id"`
	UserID     string    `json:"user_id"`
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`

	// Threading: "" for top-level comments; otherwise the id of the
	// top-level comment this reply belongs to (single-level nesting).
	ParentCommentID string `json:"parent_comment_id"`
	// ReplyCount is the number of direct replies (top-level comments only).
	ReplyCount int32 `json:"reply_count"`
	// HasCreatorReply is true when the template owner has replied in this
	// thread (top-level comments only).
	HasCreatorReply bool `json:"has_creator_reply"`

	// Joined at read time (public identity only).
	AuthorDisplayName string `json:"author_display_name"`
	AuthorAvatar      string `json:"author_avatar"`

	// Computed in the service layer.
	IsEdited bool `json:"is_edited"`
}

// AdminComment is a comment with moderation context (admin list).
type AdminComment struct {
	ID               string    `json:"id"`
	TemplateID       string    `json:"template_id"`
	TemplateTitle    string    `json:"template_title"`
	TemplateOwnerID  string    `json:"template_owner_id"`
	OwnerDisplayName string    `json:"owner_display_name"`
	UserID           string    `json:"user_id"`
	UserDisplayName  string    `json:"user_display_name"`
	UserEmail        string    `json:"user_email"`
	Content          string    `json:"content"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`

	// ParentAuthorDisplayName is the display name of the parent comment's
	// author; empty for top-level comments (admin moderation view).
	ParentAuthorDisplayName string `json:"parent_author_display_name"`
}
