package models

import (
	"database/sql"
	"time"

	"github.com/lib/pq"
)

// ReviewSubmission represents one publish attempt in the pre-publication
// review queue. It maps to the "review_submissions" table.
type ReviewSubmission struct {
	ID               string         `json:"id"`
	TemplateID       string         `json:"template_id"`
	CreatorID        string         `json:"creator_id"`
	AttemptNumber    int32          `json:"attempt_number"` // 1 = first submission; >= 2 = resubmission
	Status           string         `json:"status"`         // "pending" | "approved" | "rejected"
	SubmittedAt      time.Time      `json:"submitted_at"`
	DecidedAt        sql.NullTime   `json:"decided_at"`
	DecidedBy        sql.NullString `json:"decided_by"`
	RejectReasonCode sql.NullString `json:"reject_reason_code"` // spam | plagiarism | harmful | lowQuality | other
	RejectNote       sql.NullString `json:"reject_note"`
}

// PlatformSetting represents an admin-editable platform setting.
// It maps to the "platform_settings" table.
type PlatformSetting struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ReviewQueueRow is one row of the admin review queue listing.
type ReviewQueueRow struct {
	ReviewSubmission
	TemplateTitle    string    `json:"template_title"`
	TemplateCategory string    `json:"template_category"`
	CreatorName      string    `json:"creator_name"`
	CreatorCreatedAt time.Time `json:"creator_created_at"`
	CreatorApproved  int32     `json:"creator_approved"` // approved_template_count at submit time
	PriorRejections  int32     `json:"prior_rejections"` // rejected submissions by this creator
	Aging            bool      `json:"aging"`            // older than the aging window
	CreatorBanned    bool      `json:"creator_banned"`   // OQ-4: banned-mid-review flag
}

// ReviewSubmissionDetail is the full detail payload for the review detail page.
type ReviewSubmissionDetail struct {
	ReviewSubmission
	TemplateTitle         string         `json:"template_title"`
	TemplateDescription   sql.NullString `json:"template_description"`
	TemplateType          string         `json:"template_type"`
	TemplateTags          pq.StringArray `json:"template_tags"`
	TemplateCategory      sql.NullString `json:"template_category"`
	TemplateLanguage      string         `json:"template_language"`
	TemplateCreatedAt     time.Time      `json:"template_created_at"`
	TemplateLikeCount     int32          `json:"template_like_count"`
	TemplateFavoriteCount int32          `json:"template_favorite_count"`
	TemplateCommentCount  int32          `json:"template_comment_count"`
	CreatorName           string         `json:"creator_name"`
	CreatorCreatedAt      time.Time      `json:"creator_created_at"`
	CreatorApproved       int32          `json:"creator_approved"`
	PriorRejections       int32          `json:"prior_rejections"`
	CreatorBanned         bool           `json:"creator_banned"`
	// Latest version content for review.
	VersionNumber  int32  `json:"version_number"`
	VersionContent string `json:"version_content"`
	// Previous decision history for this template (resubmission context).
	History []*ReviewSubmission `json:"history"`
}

// RejectionRateEntry is the share of rejections for one reason code.
type RejectionRateEntry struct {
	ReasonCode string  `json:"reason_code"`
	Count      int32   `json:"count"`
	Rate       float64 `json:"rate"`
}

// ReviewMetrics aggregates queue statistics for the metrics bar.
type ReviewMetrics struct {
	PendingCount       int32                 `json:"pending_count"`
	MedianHours        float64               `json:"median_hours"`
	ApprovedToday      int32                 `json:"approved_today"`
	RejectedToday      int32                 `json:"rejected_today"`
	OldestPendingHours int32                 `json:"oldest_pending_hours"`
	RejectionRates     []*RejectionRateEntry `json:"rejection_rates"`
}
