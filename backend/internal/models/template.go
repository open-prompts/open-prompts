package models

import (
	"database/sql"
	"time"

	"github.com/lib/pq"
)

// Template represents the template model in the database.
// It maps to the "templates" table.
type Template struct {
	ID            string         `json:"id"`
	OwnerID       string         `json:"owner_id"`
	Title         string         `json:"title"`
	Description   sql.NullString `json:"description"`
	Visibility    string         `json:"visibility"`
	Type          string         `json:"type"`
	Tags          pq.StringArray `json:"tags"`
	Category      sql.NullString `json:"category"`
	Language      string         `json:"language"`
	LikeCount     int32          `json:"like_count"`
	FavoriteCount int32          `json:"favorite_count"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`

	// Transient fields (not in templates table)
	IsLiked     bool `json:"is_liked"`
	IsFavorited bool `json:"is_favorited"`
}

// TemplateVersion represents a version of a template.
// It maps to the "template_versions" table.
type TemplateVersion struct {
	ID         int32     `json:"id"`
	TemplateID string    `json:"template_id"`
	Version    int32     `json:"version"`
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"created_at"`
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
