# Content Reporting — Detailed Design

| | |
|---|---|
| **Feature ID** | content-reporting |
| **Version** | 1.0 |
| **Status** | Approved for implementation |
| **Author** | Architect Agent |
| **Upstream** | [Architecture](content-reporting-architecture.md) |

## 1. Proto Contract

### 1.1 `backend/api/proto/v1/prompt.proto`

Append to the `PromptService` service block (after `GetAppeal`, line 144):

```protobuf
service PromptService {
    // ... existing RPCs ...
    rpc CreateReport(CreateReportRequest) returns (Report);
    rpc GetMyReport(GetMyReportRequest) returns (MyReport);
    rpc WithdrawReport(WithdrawReportRequest) returns (WithdrawReportResponse);
}

message CreateReportRequest {
    string item_type = 1; // "template" | "comment"
    string item_id = 2;
    string reason = 3;    // spam | offensive | misleading | privacy | other
    string note = 4;      // optional, <= 500 chars
}

message Report {
    string id = 1;
    string item_type = 2;
    string item_id = 3;
    string reason = 4;
    string note = 5;
    string status = 6;    // open | resolved
    google.protobuf.Timestamp created_at = 7;
}

message GetMyReportRequest {
    string item_type = 1;
    string item_id = 2;
}

message MyReport {
    Report report = 1;    // null when the caller has no open report on the item
}

message WithdrawReportRequest {
    string item_type = 1;
    string item_id = 2;
}

message WithdrawReportResponse {
    bool success = 1;
}
```

### 1.2 `backend/api/proto/v1/admin.proto`

Append to the `AdminService` service block (after `ResolveAppeal`, line 96):

```protobuf
service AdminService {
    // ... existing RPCs ...
    rpc ListReportQueue(ListReportQueueRequest) returns (ListReportQueueResponse);
    rpc GetReportDetail(GetReportDetailRequest) returns (AdminReportDetail);
    rpc ResolveReportGroup(ResolveReportGroupRequest) returns (ResolveReportGroupResponse);
}

message ListReportQueueRequest {
    string status = 1;    // "" | "open" | "resolved"
    string item_type = 2; // "" | "template" | "comment"
    string reason = 3;    // "" | spam | offensive | misleading | privacy | other
    int32 page = 4;       // 1-based, default 1
    int32 per_page = 5;   // default 50, max 100
}

message ListReportQueueResponse {
    repeated AdminReportGroup groups = 1;
    int64 total = 2;
    int32 page = 3;
    int32 per_page = 4;
}

message AdminReportGroup {
    string item_type = 1;
    string item_id = 2;
    string item_title = 3;        // template title or comment snippet
    string author_display_name = 4;
    int32 open_count = 5;
    repeated string reasons = 6;  // distinct reason codes, with counts client-side
    google.protobuf.Timestamp latest_at = 7;
    string status = 8;            // open | resolved
}

message GetReportDetailRequest {
    string item_type = 1;
    string item_id = 2;
}

message AdminReportDetail {
    string item_type = 1;
    string item_id = 2;
    string item_title = 3;
    string item_content = 4;      // template preview or comment text
    string author_id = 5;
    string author_display_name = 6;
    repeated AdminReportEntry reports = 7;
}

message AdminReportEntry {
    string id = 1;
    string reporter_id = 2;
    string reporter_display_name = 3;
    string reason = 4;
    string note = 5;
    google.protobuf.Timestamp created_at = 6;
}

message ResolveReportGroupRequest {
    string item_type = 1;
    string item_id = 2;
    string resolution = 3; // "dismissed" | "actioned"
    string reason = 4;     // 3-500 chars, required
}

message ResolveReportGroupResponse {
    bool success = 1;
    int32 resolved_count = 2;
}
```

Add `open_reports` to `GetAdminStatsResponse` (field 9):

```protobuf
message GetAdminStatsResponse {
  // ... existing fields 1-8 ...
  int64 open_reports = 9; // open report count for the dashboard KPI
}
```

Regenerate:

```bash
cd backend && protoc --go_out=. --go-grpc_out=. api/proto/v1/prompt.proto api/proto/v1/admin.proto
```

## 2. Database DDL

Append to **both** `deployment/init.sql` and
`deployment/chart/open-prompts/init.sql`:

```sql
-- ===== content-reporting =====
-- User reports on public templates and comments. One report per user per item
-- (UNIQUE); re-reporting upserts (FR-2). Resolution is atomic across all open
-- reports of an item (NFR-3).
CREATE TABLE IF NOT EXISTS content_reports (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    item_type TEXT NOT NULL CHECK (item_type IN ('template', 'comment')),
    item_id UUID NOT NULL,
    reporter_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reason TEXT NOT NULL CHECK (reason IN ('spam', 'offensive', 'misleading', 'privacy', 'other')),
    note TEXT CHECK (note IS NULL OR char_length(note) <= 500),
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved')),
    resolution TEXT CHECK (resolution IS NULL OR resolution IN ('dismissed', 'actioned')),
    resolved_by TEXT REFERENCES users(id),
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (item_type, item_id, reporter_id)
);

COMMENT ON TABLE content_reports IS 'User reports on public templates/comments; one per user per item';
COMMENT ON COLUMN content_reports.item_type IS 'template | comment';
COMMENT ON COLUMN content_reports.reason IS 'spam | offensive | misleading | privacy | other';
COMMENT ON COLUMN content_reports.status IS 'open | resolved';
COMMENT ON COLUMN content_reports.resolution IS 'dismissed | actioned (NULL while open)';

CREATE INDEX IF NOT EXISTS idx_content_reports_status_item
    ON content_reports (status, item_type, item_id);
CREATE INDEX IF NOT EXISTS idx_content_reports_reporter
    ON content_reports (reporter_id);

-- Widen admin_audit_logs.action CHECK for report actions (guarded, idempotent).
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'admin_audit_logs'::regclass AND conname = 'admin_audit_logs_action_check';
    IF def IS NOT NULL AND def NOT LIKE '%report.dismiss%' THEN
        ALTER TABLE admin_audit_logs DROP CONSTRAINT admin_audit_logs_action_check;
        ALTER TABLE admin_audit_logs ADD CONSTRAINT admin_audit_logs_action_check
            CHECK (action IN ('ban', 'unban', 'role_change', 'delete_comment',
                              'review_approve', 'review_reject', 'settings_change',
                              'template.warning_add', 'template.warning_remove',
                              'appeal.create', 'appeal.review', 'appeal.resolve',
                              'report.dismiss', 'report.actioned'));
    END IF;
END $$;
```

`admin_audit_logs.target_type` already includes `template` and `comment` — no
widening needed.

## 3. Backend Layers

### 3.1 `models` (`backend/internal/models/report.go`)

```go
package models

import "time"

// ContentReport maps to the "content_reports" table.
type ContentReport struct {
    ID         string     `json:"id"`
    ItemType   string     `json:"item_type"`   // template | comment
    ItemID     string     `json:"item_id"`
    ReporterID string     `json:"reporter_id"`
    Reason     string     `json:"reason"`      // spam | offensive | misleading | privacy | other
    Note       string     `json:"note"`
    Status     string     `json:"status"`      // open | resolved
    Resolution string     `json:"resolution"`  // dismissed | actioned
    ResolvedBy string     `json:"resolved_by"`
    ResolvedAt *time.Time `json:"resolved_at"`
    CreatedAt  time.Time  `json:"created_at"`
}
```

### 3.2 `ReportRepository` (`backend/internal/repository/report_repository.go`)

```go
type ReportRepository interface {
    Upsert(ctx context.Context, r *models.ContentReport) error // FR-2: re-report replaces
    GetMine(ctx context.Context, reporterID, itemType, itemID string) (*models.ContentReport, error)
    DeleteMine(ctx context.Context, reporterID, itemType, itemID string) error
    ListQueue(ctx context.Context, status, itemType, reason string, limit, offset int) ([]*models.ReportGroup, int64, error)
    ListForItem(ctx context.Context, itemType, itemID string) ([]*models.ContentReport, error)
    ResolveItem(ctx context.Context, itemType, itemID, resolution, resolvedBy string) (int64, error) // atomic
    CountOpen(ctx context.Context) (int64, error) // dashboard KPI
    AutoResolveItem(ctx context.Context, itemType, itemID string) error // FR-12 hook
}
```

`Upsert` SQL (re-report replaces reason/note and bumps recency; resets to open):

```sql
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
RETURNING id, created_at;
```

`ListQueue` (grouped by item, open-first by count then recency):

```sql
SELECT cr.item_type, cr.item_id,
       COALESCE(t.title, c.content) AS item_title,
       COALESCE(tu.display_name, cu.display_name) AS author_display_name,
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
LIMIT $4 OFFSET $5;
```

`ResolveItem` — atomic across all open reports of the item:

```sql
UPDATE content_reports
SET status = 'resolved',
    resolution = $3,
    resolved_by = $4,
    resolved_at = now()
WHERE item_type = $1 AND item_id = $2 AND status = 'open';
```

`AutoResolveItem` (FR-12 hook, called on unpublish/delete):

```sql
UPDATE content_reports
SET status = 'resolved',
    resolution = 'actioned',
    resolved_by = $3,
    resolved_at = now()
WHERE item_type = $1 AND item_id = $2 AND status = 'open';
```

### 3.3 `PromptService` (`backend/internal/service/prompt.go`)

`CreateReport`:

1. `GetUserIDFromContext` → `Unauthenticated` on failure.
2. Load the user; if `user.Status == "banned"` → `PermissionDenied` (FR-3).
3. Validate `item_type` ∈ {template, comment} and `reason` ∈ the 5-code
   taxonomy → else `InvalidArgument`.
4. Validate `note` ≤ 500 chars → else `InvalidArgument`.
5. Load the item (template or comment); if not found or not public → `NotFound`.
   If the item's owner/author is the caller → `InvalidArgument` (FR-4 backstop).
6. Rate limit (below) → `ResourceExhausted` on limit.
7. `ReportRepo.Upsert` → `Report` proto.

`GetMyReport`: auth, then `ReportRepo.GetMine` → `MyReport` (null when none).

`WithdrawReport`: auth, then `ReportRepo.DeleteMine` → `WithdrawReportResponse`.

**Rate limiting** (Redis counter, fail-open):

```
key:    report_rate:{user_id}
value:  "{count}:{window_expires_unix}"
window: sliding, limit REPORT_RATE_PER_HOUR (default 20)
```

Increment on each `CreateReport`; when the count reaches the limit, return
`429 RESOURCE_EXHAUSTED` with `Retry-After`.

### 3.4 `AdminService` (`backend/internal/service/admin.go`)

`ListReportQueue`: `requireAdminContext`, then `ReportRepo.ListQueue` →
`ListReportQueueResponse`.

`GetReportDetail`: `requireAdminContext`, then `ReportRepo.ListForItem` +
item context (template title/content or comment text) → `AdminReportDetail`.

`ResolveReportGroup`:

1. `requireAdminContext`.
2. Validate `resolution` ∈ {dismissed, actioned} and `reason` length ∈ [3, 500]
   → else `InvalidArgument`.
3. `ReportRepo.ResolveItem` → if 0 rows resolved → `AlreadyExists` (no open
   reports).
4. Write audit `report.dismiss` or `report.actioned` (target_type `template`/
   `comment`, target_id `item_id`, reason carries the admin's reason).
5. Return `ResolveReportGroupResponse`.

**Auto-resolve hooks (FR-12):** when a template is unpublished or a comment
deleted by any admin path, call `ReportRepo.AutoResolveItem` in the same
transaction. For templates, hook into the unpublish/moderation action path; for
comments, hook into `DeleteAdminComment` and the user-facing comment delete.

**Dashboard KPI (FR-13):** `GetAdminStats` additionally calls
`ReportRepo.CountOpen` and sets `open_reports` on the response.

### 3.5 Gateway (`backend/cmd/server/main.go`)

**User routes** — add after the `/api/v1/appeals/` handler:

```go
// /api/v1/reports — POST create/update (auth).
http.HandleFunc("/api/v1/reports", func(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodPost { // auth -> CreateReportRequest{ItemType, ItemId, Reason, Note}
    }
})

// /api/v1/reports/mine — GET (auth), DELETE (auth).
http.HandleFunc("/api/v1/reports/mine", func(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:   // auth -> GetMyReportRequest{ItemType, ItemId}
    case http.MethodDelete: // auth -> WithdrawReportRequest{ItemType, ItemId}
    }
})
```

**Admin routes** — add after the `/api/v1/admin/appeals/` handler:

```go
// /api/v1/admin/reports — GET queue (admin).
http.HandleFunc("/api/v1/admin/reports", func(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodGet { // admin -> ListReportQueueRequest{Status, ItemType, Reason, Page, PerPage}
    }
})

// /api/v1/admin/reports/{item_type}/{item_id} — GET detail, POST resolve (admin).
http.HandleFunc("/api/v1/admin/reports/", func(w http.ResponseWriter, r *http.Request) {
    rest := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/reports/")
    parts := strings.Split(rest, "/")
    if len(parts) != 2 { http.Error(w, "not found", http.StatusNotFound); return }
    itemType, itemID := parts[0], parts[1]
    switch r.Method {
    case http.MethodGet:  // admin -> GetReportDetailRequest{ItemType, ItemId}
    case http.MethodPost: // admin -> ResolveReportGroupRequest{ItemType, ItemId, Resolution, Reason}
    }
})
```

All routes require the Bearer-token auth dance (user or admin JWT as
appropriate). `Retry-After` is set on 429 responses.

## 4. Frontend

### 4.1 New Files

| File | Purpose |
|---|---|
| `frontend/src/components/ReportMenu.js` + `.scss` | ⋯ menu item: Report / You reported this (Update/Withdraw). |
| `frontend/src/components/ReportModal.js` + `.scss` | Five radio reasons + optional note; focus-trapped dialog. |
| `frontend/src/components/ReportedMenuItem.js` + `.scss` | Checkmark state with Update/Withdraw submenu. |
| `frontend/src/pages/admin/ReportsPage.js` + `.scss` | `/admin/reports` — grouped queue with filters. |
| `frontend/src/components/admin/ReviewDrawer.js` + `.scss` | Right drawer: item context + reports + resolution actions. |
| `frontend/src/components/admin/ReasonPopover.js` + `.scss` | Reason-required confirm popover for Dismiss/Act. |

### 4.2 Modified Files

| File | Change |
|---|---|
| `frontend/src/App.js` | Add `/admin/reports` admin route. |
| `frontend/src/services/api.js` | `createReport`, `getMyReport`, `withdrawReport`. |
| `frontend/src/services/adminApi.js` | `listAdminReports`, `getAdminReportDetail`, `resolveReportGroup`. |
| `frontend/src/components/admin/AdminLayout.js` | Add Reports nav item. |
| `frontend/src/pages/admin/DashboardPage.js` | Add OpenReportsKPI card (FR-13). |
| TemplateDetails / comment ⋯ menu | Mount `ReportMenu`. |

### 4.3 Redux

No new slice: report state arrives with API responses; the modal and queue
pages manage local state. `ReportMenu` fetches `getMyReport` on mount to decide
the checkmark state.

### 4.4 API Client

```js
createReport({ item_type, item_id, reason, note }) // POST /reports
getMyReport(item_type, item_id)                    // GET  /reports/mine
withdrawReport(item_type, item_id)                 // DELETE /reports/mine
listAdminReports(params)                           // GET  /admin/reports
getAdminReportDetail(item_type, item_id)           // GET  /admin/reports/:item_type/:item_id
resolveReportGroup(item_type, item_id, data)       // POST /admin/reports/:item_type/:item_id/resolve
```

### 4.5 i18n Keys (EN/ZH parity)

```json
{
  "reports": {
    "action": "Report",
    "modal": {
      "title": { "template": "Report this template", "comment": "Report this comment" },
      "question": "Why are you reporting this?",
      "noteLabel": "Additional details (optional)",
      "submit": "Submit report",
      "cancel": "Cancel"
    },
    "reason": {
      "spam": "Spam or scam",
      "offensive": "Offensive or hateful",
      "misleading": "Misleading or harmful",
      "privacy": "Privacy violation",
      "other": "Something else"
    },
    "toast": { "submitted": "Report submitted — thank you. Our moderators will review it.", "updated": "Your report was updated." },
    "reported": "You reported this",
    "update": "Update report…",
    "withdraw": "Withdraw report",
    "withdrawConfirm": "Withdraw your report?",
    "rateLimited": "You've submitted several reports recently — please try again later."
  },
  "admin": {
    "nav": { "reports": "Reports" },
    "reports": {
      "title": "Reports",
      "open": "Open",
      "resolved": "Resolved",
      "openCount": "Open reports",
      "review": "Review",
      "view": "View",
      "dismiss": "Dismiss reports",
      "reasonRequired": "A reason is required",
      "empty": "No open reports — all clear",
      "type": "Type",
      "item": "Item",
      "author": "Author",
      "reasons": "Reasons",
      "latest": "Latest",
      "status": "Status",
      "actions": "Actions"
    },
    "dashboard": { "open_reports": "Open reports" }
  }
}
```

ZH mirrors every key per the UI/UX doc §8 (`reports.action` = "举报",
`reports.reason.spam` = "垃圾信息或诈骗", `admin.reports.title` = "举报", etc.).

## 5. Error Matrix

| Scenario | HTTP | gRPC | Frontend handling |
|---|---|---|---|
| Invalid item_type/reason; note > 500; own content | 400 | `INVALID_ARGUMENT` | Field-level error; form unchanged |
| Not authenticated | 401 | `UNAUTHENTICATED` | Redirect to `/login` |
| Banned user | 403 | `PERMISSION_DENIED` | Toast; action hidden |
| Non-admin admin route | 403 | `PERMISSION_DENIED` | Admin console error toast |
| Item not found / not public | 404 | `NOT_FOUND` | 404 page |
| No open reports to resolve | 409 | `ALREADY_EXISTS` | Toast; queue refreshes |
| Report rate limit | 429 | `RESOURCE_EXHAUSTED` | Toast with rate-limit message |
| Redis down | — | — | Fail-open server-side; no user impact |

## 6. Deployment

- `REPORT_RATE_PER_HOUR=20` added to `deployment/docker-compose.yml` backend
  environment and the Helm chart (`values.yaml` `backend.env.reportRatePerHour` +
  `templates/deployment-backend.yaml` env entry).
- DDL additive/idempotent; both init.sql copies in the same commit.
- No new services, ports, or infrastructure.

## 7. SDK

No python-sdk changes: content reporting is a web-surface feature (user report
modal + admin console). The REST endpoints are documented in the API surface;
the SDK wraps template resolution only.

## 8. Test Strategy

### 8.1 FVT (`tests/fvt.py`) — `test_content_reporting`

Setup: alice (reporter), bob (admin), carol (creator). Seed a public template
by carol and a public comment on it.

1. alice reports the template → `open`; `GET /reports/mine` returns it.
2. alice re-reports with a different reason → upsert (no duplicate); toast
   "updated".
3. alice reports the comment → `open`.
4. Anonymous `POST /reports` → 401.
5. A banned user `POST /reports` → 403.
6. alice reports her own template → 400.
7. alice submits 21 reports in an hour → 429 on the 21st.
8. bob `GET /admin/reports` → grouped queue, open-first by count then recency,
   filters work.
9. bob `GET /admin/reports/template/{id}` → detail with all reports.
10. bob `POST /resolve {resolution: dismissed, reason}` → all open reports
    resolved; audit has `report.dismiss`; content untouched.
11. A second item: bob `POST /resolve {resolution: actioned, reason}` →
    resolved; audit has `report.actioned`.
12. Unpublishing a template (via the moderation path) auto-resolves its open
    reports as `actioned` (FR-12).
13. Deleting a comment auto-resolves its open reports as `actioned` (FR-12).
14. `GET /admin/stats` → `open_reports` KPI count correct.
15. Note containing `<script>` renders as literal text (no HTML interpretation).

### 8.2 Unit Tests

- Repository: `Upsert` upsert + unique-violation mapping, `ListQueue` grouping +
  ordering + filters, `ResolveItem` atomic (0-rows → no-op), `AutoResolveItem`,
  `CountOpen`.
- Service: validation matrix, authz (banned, own-content, non-admin), rate-limit
  counter (fake Redis), audit emission, auto-resolve hooks.
- Update `MockReportRepository` with the new methods.

### 8.3 E2E (test-agent, Nightwatch)

- Report modal: open from ⋯ menu, radio group, submit, success toast, reported
  state.
- Reported-state menu: Update (prefilled modal) and Withdraw (confirm).
- `/admin/reports` queue: grouping, filters, open-first sort, Review/View.
- Review drawer: item context, reports list, Dismiss/Act with reason popover.
- Dashboard KPI card links to the queue.
- i18n EN/ZH parity for all `reports.*` and `admin.reports.*` keys.

## 9. Implementation Order

1. DDL (both init.sql copies) + models + `ReportRepository` + unit tests.
2. Proto changes + regenerate.
3. `PromptService` user RPCs + rate limit + Redis counter wiring.
4. `AdminService` admin RPCs + audit + auto-resolve hooks + KPI count.
5. Gateway user + admin routes.
6. Frontend: api client → components → pages → routes → i18n.
7. FVT + E2E.

## 10. Constraints for Developer

1. **Never commit `backend/server`** — tracked build artifact; `git checkout
   -- backend/server` if modified.
2. Stage only your own files; never `git add -A`.
3. Both init.sql copies in sync in the same commit.
4. The `content_reports` UNIQUE constraint is the FR-1 backstop; map the `23505`
   violation to an upsert (FR-2), not an error.
5. `ResolveItem` must be atomic across all open reports of the item; return 409
   when 0 rows are resolved.
6. Auto-resolve (FR-12) must hook the unpublish and comment-delete paths in the
   same transaction — no sweep job.
7. Reporters are admin-only (NFR-2); never expose reporter identity to content
   authors or other users.
8. Notes and reasons are plain text — never render as HTML/Markdown (XSS-safe
   by construction, NFR-5).
9. No auto-takedown at any report threshold (NFR-4) — every consequence requires
   an explicit admin action.
10. No monetization surfaces of any kind.