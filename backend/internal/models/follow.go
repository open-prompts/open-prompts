package models

import "time"

// Follow represents a directed follow edge (follower -> followee).
// It maps to the "user_follows" table.
type Follow struct {
	FollowerID string
	FolloweeID string
	CreatedAt  time.Time
}

// UserCard is the public identity of a user in followers/following lists and
// creator suggestions. It deliberately excludes email/mobile/role.
type UserCard struct {
	ID            string
	DisplayName   string
	Avatar        string
	FollowerCount int32
	IsFollowing   bool
}
