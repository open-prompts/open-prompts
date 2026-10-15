package models

import (
	"database/sql"
	"time"
)

type User struct {
	ID                    string         `json:"id"`
	Email                 string         `json:"email"`
	Mobile                sql.NullString `json:"mobile"`
	PasswordHash          string         `json:"-"`
	DisplayName           string         `json:"display_name"`
	Avatar                string         `json:"avatar"`
	Role                  string         `json:"role"`                    // "user" | "admin"
	Status                string         `json:"status"`                  // "active" | "banned"
	ApprovedTemplateCount int32          `json:"approved_template_count"` // gating threshold N
	ReviewGatedRemaining  int32          `json:"review_gated_remaining"`  // re-gate count M after takedown/ban
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
}
