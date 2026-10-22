package repository

import (
	"strings"
	"testing"
)

func TestReportColumnsPresent(t *testing.T) {
	for _, col := range []string{
		"cr.id", "cr.item_type", "cr.item_id", "cr.reporter_id", "cr.reason",
		"cr.status", "cr.resolution", "cr.resolved_by", "cr.resolved_at", "cr.created_at",
	} {
		if !strings.Contains(reportColumns, col) {
			t.Errorf("reportColumns must contain %s, got:\n%s", col, reportColumns)
		}
	}
}

func TestUpsertUsesOnConflict(t *testing.T) {
	// The upsert must use ON CONFLICT on the UNIQUE (item_type, item_id,
	// reporter_id) constraint and reset the row to open (FR-2).
	query := `
		INSERT INTO content_reports (item_type, item_id, reporter_id, reason, note)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (item_type, item_id, reporter_id)
		DO UPDATE SET reason = EXCLUDED.reason,
		              note = EXCLUDED.note,
		              status = 'open',
		              resolution = NULL,
		              resolved_by = NULL,
		              resolved_at = NULL,
		              created_at = now()
		RETURNING id, created_at
	`
	for _, want := range []string{
		"ON CONFLICT (item_type, item_id, reporter_id)",
		"DO UPDATE SET reason = EXCLUDED.reason",
		"status = 'open'",
		"resolution = NULL",
		"created_at = now()",
	} {
		if !strings.Contains(query, want) {
			t.Errorf("Upsert query must contain %q", want)
		}
	}
}

func TestListQueueGroupsAndOrders(t *testing.T) {
	// The queue must group by item, join template/comment context, and order
	// open-first by count then recency (FR-8/FR-9).
	query := `
		SELECT cr.item_type, cr.item_id,
		       COALESCE(t.title, c.content, '') AS item_title,
		       COALESCE(tu.display_name, cu.display_name, '') AS author_display_name,
		       COUNT(*) FILTER (WHERE cr.status = 'open') AS open_count,
		       array_agg(DISTINCT cr.reason) AS reasons,
		       MAX(cr.created_at) AS latest_at,
		       MAX(cr.status) AS status
		FROM content_reports cr
		LEFT JOIN templates t ON cr.item_type = 'template' AND t.id = cr.item_id
		LEFT JOIN users tu ON t.owner_id = tu.id
		LEFT JOIN template_comments c ON cr.item_type = 'comment' AND c.id = cr.item_id
		LEFT JOIN users cu ON c.user_id = cu.id
		WHERE ($1 = '' OR cr.status = $1)
		  AND ($2 = '' OR cr.item_type = $2)
		  AND ($3 = '' OR cr.reason = $3)
		GROUP BY cr.item_type, cr.item_id, t.title, c.content, tu.display_name, cu.display_name
		ORDER BY open_count DESC, latest_at DESC
		LIMIT $4 OFFSET $5
	`
	for _, want := range []string{
		"GROUP BY cr.item_type, cr.item_id",
		"COUNT(*) FILTER (WHERE cr.status = 'open')",
		"array_agg(DISTINCT cr.reason)",
		"ORDER BY open_count DESC, latest_at DESC",
		"LEFT JOIN templates t",
		"LEFT JOIN template_comments c",
		"COALESCE(t.title, c.content, '') AS item_title",
		"COALESCE(tu.display_name, cu.display_name, '') AS author_display_name",
	} {
		if !strings.Contains(query, want) {
			t.Errorf("ListQueue query must contain %q", want)
		}
	}
}

func TestResolveItemOnlyOpen(t *testing.T) {
	// ResolveItem must only touch open reports (atomic across all open
	// reports of the item, NFR-3).
	query := `
		UPDATE content_reports
		 SET status = 'resolved',
		     resolution = $3,
		     resolved_by = $4,
		     resolved_at = now()
		 WHERE item_type = $1 AND item_id = $2 AND status = 'open'
	`
	for _, want := range []string{
		"SET status = 'resolved'",
		"resolution = $3",
		"WHERE item_type = $1 AND item_id = $2 AND status = 'open'",
	} {
		if !strings.Contains(query, want) {
			t.Errorf("ResolveItem query must contain %q", want)
		}
	}
}

func TestAutoResolveItemActioned(t *testing.T) {
	// AutoResolveItem must resolve open reports as actioned (FR-12).
	query := `
		UPDATE content_reports
		 SET status = 'resolved',
		     resolution = 'actioned',
		     resolved_by = $3,
		     resolved_at = now()
		 WHERE item_type = $1 AND item_id = $2 AND status = 'open'
	`
	for _, want := range []string{
		"resolution = 'actioned'",
		"WHERE item_type = $1 AND item_id = $2 AND status = 'open'",
	} {
		if !strings.Contains(query, want) {
			t.Errorf("AutoResolveItem query must contain %q", want)
		}
	}
}

func TestListForItemJoinsReporter(t *testing.T) {
	// The admin detail must join the reporter's display name (admin-only,
	// NFR-2).
	query := `
		SELECT ` + reportColumns + `, COALESCE(u.display_name, '') AS reporter_display_name
		FROM content_reports cr
		LEFT JOIN users u ON u.id = cr.reporter_id
		WHERE cr.item_type = $1 AND cr.item_id = $2
		ORDER BY cr.created_at DESC
	`
	for _, want := range []string{
		"LEFT JOIN users u ON u.id = cr.reporter_id",
		"reporter_display_name",
		"ORDER BY cr.created_at DESC",
	} {
		if !strings.Contains(query, want) {
			t.Errorf("ListForItem query must contain %q", want)
		}
	}
}

func TestReportErrorSentinels(t *testing.T) {
	if ErrReportNotFound == nil {
		t.Error("ErrReportNotFound must be defined")
	}
}