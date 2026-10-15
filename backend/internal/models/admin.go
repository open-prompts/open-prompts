package models

import (
	"database/sql"
	"time"

	"github.com/lib/pq"
)

// AdminAuditLog is an append-only record of an administrative mutation.
// It maps to the "admin_audit_logs" table.
type AdminAuditLog struct {
	ID         string    `json:"id"`
	ActorID    string    `json:"actor_id"`
	ActorEmail string    `json:"actor_email"`
	Action     string    `json:"action"` // ban | unban | role_change
	TargetType string    `json:"target_type"`
	TargetID   string    `json:"target_id"`
	Reason     string    `json:"reason"`
	CreatedAt  time.Time `json:"created_at"`
}

// AdminStats holds the dashboard KPI values.
type AdminStats struct {
	TotalUsers      int64 `json:"total_users"`
	ActiveUsers     int64 `json:"active_users"`
	BannedUsers     int64 `json:"banned_users"`
	TotalTemplates  int64 `json:"total_templates"`
	PublicTemplates int64 `json:"public_templates"`
	PromptsCreated  int64 `json:"prompts_created"`
}

// UserStats holds per-user aggregates shown on the admin user detail page.
type UserStats struct {
	Templates         int64 `json:"templates"`
	LikesReceived     int64 `json:"likes_received"`
	FavoritesReceived int64 `json:"favorites_received"`
	PromptsCreated    int64 `json:"prompts_created"`
	FollowerCount     int64 `json:"follower_count"`
	FollowingCount    int64 `json:"following_count"`
}

// OAuthIdentity is a linked OAuth provider for a user.
type OAuthIdentity struct {
	Provider  string    `json:"provider"`
	CreatedAt time.Time `json:"created_at"`
}

// AdminTemplate is the admin moderation view of a template (includes private).
type AdminTemplate struct {
	ID               string         `json:"id"`
	OwnerID          string         `json:"owner_id"`
	OwnerDisplayName string         `json:"owner_display_name"`
	Title            string         `json:"title"`
	Description      sql.NullString `json:"description"`
	Visibility       string         `json:"visibility"`
	Type             string         `json:"type"`
	Language         string         `json:"language"`
	Tags             pq.StringArray `json:"tags"`
	LikeCount        int32          `json:"like_count"`
	FavoriteCount    int32          `json:"favorite_count"`
	Category         string         `json:"category"`
	ContentWarnings  pq.StringArray `json:"content_warnings"`
	LockedWarnings   pq.StringArray `json:"locked_warnings"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

// AdminTemplateVersion is a template version with its alias (if any) for the
// admin template detail view.
type AdminTemplateVersion struct {
	ID         int32          `json:"id"`
	Version    int32          `json:"version"`
	Content    string         `json:"content"`
	CreatedAt  time.Time      `json:"created_at"`
	AliasName  string         `json:"alias_name"` // "" if no alias points at this version
	ChangeNote sql.NullString `json:"change_note"`
}
