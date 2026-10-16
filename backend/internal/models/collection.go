package models

import "time"

// Collection represents a row in the "collections" table.
type Collection struct {
	ID          string
	OwnerID     string
	Name        string
	Description string
	Visibility  string // "private" | "public"
	Status      string // "active" | "deleted"
	CreatedAt   time.Time
	UpdatedAt   time.Time

	// Computed at read time, never denormalized.
	TemplateCount int32
}

// CollectionMember is a template within a collection plus its position and
// visibility flag (owner view flags private / no-longer-public members).
type CollectionMember struct {
	Template Template
	Position int32
	IsPublic bool
}
