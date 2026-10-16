package repository

import (
	"strings"
	"testing"
)

// TestQueueRowColumnsCoalesceCategory guards Issue #3: the review-queue scan
// must never receive a NULL for t.category (a template created without a
// category stores NULL). Scanning NULL into the plain-string
// ReviewQueueRow.TemplateCategory crashes the whole GET /admin/review queue
// with "converting NULL to string is unsupported". COALESCE(t.category, ”)
// turns NULL into "" so the scan always succeeds.
func TestQueueRowColumnsCoalesceCategory(t *testing.T) {
	if !strings.Contains(queueRowColumns, "COALESCE(t.category, '')") {
		t.Errorf("queueRowColumns must COALESCE t.category to '' for NULL-safe scan, got:\n%s", queueRowColumns)
	}
}
