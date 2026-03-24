package models

import (
	"database/sql"
	"time"
)

type APIKey struct {
	ID         string       `json:"id"`
	UserID     string       `json:"user_id"`
	Name       string       `json:"name"`
	KeyHash    string       `json:"-"` // Never return the hash
	Prefix     string       `json:"prefix"`
	CreatedAt  time.Time    `json:"created_at"`
	ExpiresAt  sql.NullTime `json:"expires_at"`
	LastUsedAt sql.NullTime `json:"last_used_at"`
	IsActive   bool         `json:"is_active"`
}
