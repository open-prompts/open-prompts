package models

import "time"

// NotificationPreferences is the per-user notification opt-out state.
// An absent row means all-enabled / not-paused (defaults).
type NotificationPreferences struct {
	UserID          string    `json:"user_id"`
	LikeEnabled     bool      `json:"like_enabled"`
	FavoriteEnabled bool      `json:"favorite_enabled"`
	ForkEnabled     bool      `json:"fork_enabled"`
	CommentEnabled  bool      `json:"comment_enabled"`
	FollowEnabled   bool      `json:"follow_enabled"`
	PausedAll       bool      `json:"paused_all"`
	UpdatedAt       time.Time `json:"updated_at"`
}
