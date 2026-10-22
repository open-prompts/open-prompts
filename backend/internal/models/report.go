package models

import "time"

// ContentReport maps to the "content_reports" table. It records a user's
// report on a public template or comment. One report per user per item
// (UNIQUE); re-reporting upserts (FR-2). Resolution is atomic across all
// open reports of an item (NFR-3).
type ContentReport struct {
	ID         string     `json:"id"`
	ItemType   string     `json:"item_type"` // template | comment
	ItemID     string     `json:"item_id"`
	ReporterID string     `json:"reporter_id"`
	Reason     string     `json:"reason"` // spam | offensive | misleading | privacy | other
	Note       string     `json:"note"`
	Status     string     `json:"status"`     // open | resolved
	Resolution string     `json:"resolution"` // dismissed | actioned
	ResolvedBy string     `json:"resolved_by"`
	ResolvedAt *time.Time `json:"resolved_at"`
	CreatedAt  time.Time  `json:"created_at"`

	// Joined at read time (admin-only, NFR-2):
	ReporterDisplayName string `json:"reporter_display_name"`
}

// ReportGroup is one row of the admin report queue: a reported item grouped
// with its open count, distinct reasons, and latest report time.
type ReportGroup struct {
	ItemType          string    `json:"item_type"`
	ItemID            string    `json:"item_id"`
	ItemTitle         string    `json:"item_title"`          // template title or comment snippet
	AuthorDisplayName string    `json:"author_display_name"` // "" when the item is gone
	OpenCount         int64     `json:"open_count"`
	Reasons           []string  `json:"reasons"` // distinct reason codes
	LatestAt          time.Time `json:"latest_at"`
	Status            string    `json:"status"` // open | resolved
}
