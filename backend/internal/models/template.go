package models

import (
	"database/sql"
	"time"

	"github.com/lib/pq"
)

// Template represents the template model in the database.
// It maps to the "templates" table.
type Template struct {
	ID               string         `json:"id"`
	OwnerID          string         `json:"owner_id"`
	Title            string         `json:"title"`
	Description      sql.NullString `json:"description"`
	Visibility       string         `json:"visibility"`
	Type             string         `json:"type"`
	Tags             pq.StringArray `json:"tags"`
	Category         sql.NullString `json:"category"`
	Language         string         `json:"language"`
	LikeCount        int32          `json:"like_count"`
	FavoriteCount    int32          `json:"favorite_count"`
	CommentCount     int32          `json:"comment_count"`
	ModerationStatus string         `json:"moderation_status"` // "normal" | "in_review" | "rejected"
	PublishedAt      sql.NullTime   `json:"published_at"`      // first review-approval time; NULL until approved
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`

	// Transient fields (not in templates table)
	IsLiked     bool `json:"is_liked"`
	IsFavorited bool `json:"is_favorited"`
}

// TemplateVersion represents a version of a template.
// It maps to the "template_versions" table.
type TemplateVersion struct {
	ID          int32          `json:"id"`
	TemplateID  string         `json:"template_id"`
	Version     int32          `json:"version"`
	Content     string         `json:"content"`
	ChangeNote  sql.NullString `json:"change_note"` // NULL-able; empty string in proto
	CreatedAt   time.Time      `json:"created_at"`
	PromptCount int            `json:"prompt_count"` // computed at read time, not stored
}

// TemplateAlias represents an alias pointing to a specific template version.
// It maps to the "template_aliases" table.
type TemplateAlias struct {
	ID         string    `json:"id"`
	TemplateID string    `json:"template_id"`
	AliasName  string    `json:"alias_name"`
	VersionID  int32     `json:"version_id"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
