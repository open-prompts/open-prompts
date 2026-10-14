# Pre-Publication Review — Detailed Design

| | |
|---|---|
| **Feature ID** | pre-publication-review |
| **Version** | 1.0 |
| **Status** | Approved for implementation |
| **Author** | Architect Agent |
| **Upstream** | [Architecture](pre-publication-review-architecture.md) |

## 1. Proto Contract

### 1.1 `backend/api/proto/v1/prompt.proto`

`Template` field numbers are a coordinated block across designs: 17 =
`comment_count` (shipped), 18/19 reserved by follow-creators
(`owner_display_name`/`owner_avatar`), 20–22 by template-ratings, 23/24 by
prompt-analytics. **This feature takes 25/26 — do not reuse 18–24.**

```protobuf
message Template {
    // ... existing fields 1-17 ...
    // Moderation state: "normal" | "in_review" | "rejected".
    // Only meaningful to the owner and admins; hidden templates 404 for
    // everyone else so the value never leaks.
    string moderation_status = 25;
    // Approval time of the first review approval; null/absent until then.
    google.protobuf.Timestamp published_at = 26;
}
```

`templateModelToProto` maps both (`ModerationStatus: m.ModerationStatus`,
`PublishedAt: timestamppb.New(m.PublishedAt.Time)` when valid). No other
prompt.proto changes — gating reuses `CreateTemplateRequest`/
`UpdateTemplateRequest` unchanged; the response `Template` carries the new
fields so the frontend can react.

### 1.2 `backend/api/proto/v1/admin.proto`

Seven new RPCs appended to `AdminService` (existing 13 untouched):

```protobuf
service AdminService {
    // ... existing RPCs 1-13 ...
    rpc ListReviewQueue(ListReviewQueueRequest) returns (ListReviewQueueResponse);
    rpc GetReviewSubmission(GetReviewSubmissionRequest) returns (GetReviewSubmissionResponse);
    rpc ApproveReviewSubmission(ApproveReviewSubmissionRequest) returns (ReviewDecisionResponse);
    rpc RejectReviewSubmission(RejectReviewSubmissionRequest) returns (ReviewDecisionResponse);
    rpc GetReviewMetrics(GetReviewMetricsRequest) returns (ReviewMetrics);
    rpc GetPlatformSettings(GetPlatformSettingsRequest) returns (GetPlatformSettingsResponse);
    rpc UpdatePlatformSetting(UpdatePlatformSettingRequest) returns (PlatformSettingEntry);
}

message ListReviewQueueRequest {
    string category = 1;         // "" = any
    bool aging_only = 2;         // only submissions older than the aging window
    bool resubmission_only = 3;  // only attempt_number >= 2
    int32 page = 4;
    int32 per_page = 5;
}

message ReviewQueueItem {
    string id = 1;                 // submission id
    string template_id = 2;
    string template_title = 3;
    string creator_id = 4;
    string creator_display_name = 5;
    int32 attempt_number = 6;      // >= 2 renders the "(nth)" chip
    bool is_resubmission = 7;      // attempt_number >= 2
    google.protobuf.Timestamp submitted_at = 8;
    string category = 9;
    bool aging = 10;               // submitted_at older than the aging window
    int32 creator_approved_count = 11;   // users.approved_template_count
    int32 creator_prior_rejections = 12; // rejected submissions by this creator
    bool creator_banned = 13;      // OQ-4: banned-mid-review flag
}

message ListReviewQueueResponse {
    repeated ReviewQueueItem items = 1;
    int64 total = 2;
    int32 page = 3;
    int32 per_page = 4;
}

message GetReviewSubmissionRequest {
    string id = 1; // submission id
}

message ReviewChainEntry {
    int32 attempt_number = 1;
    string status = 2;             // "approved" | "rejected"
    google.protobuf.Timestamp submitted_at = 3;
    google.protobuf.Timestamp decided_at = 4;
    string reject_reason_code = 5; // empty for approvals
    string reject_note = 6;
}

message ReviewSubmissionDetail {
    string id = 1;
    string template_id = 2;
    string template_title = 3;
    string template_description = 4;
    string template_content = 5;   // latest version content
    repeated string tags = 6;
    string category = 7;
    string language = 8;
    string creator_id = 9;
    string creator_display_name = 10;
    google.protobuf.Timestamp creator_joined_at = 11;
    int32 creator_published_count = 12;   // approved templates
    int32 creator_prior_rejections = 13;
    bool creator_banned = 14;
    int32 attempt_number = 15;
    google.protobuf.Timestamp submitted_at = 16;
    repeated ReviewChainEntry chain = 17; // prior attempts, oldest first
}

message GetReviewSubmissionResponse {
    ReviewSubmissionDetail submission = 1;
}

message ApproveReviewSubmissionRequest {
    string id = 1; // submission id
}

message RejectReviewSubmissionRequest {
    string id = 1;          // submission id
    string reason_code = 2; // required: spam | plagiarism | harmful | lowQuality | other
    string note = 3;        // optional, <= 500 chars
}

message ReviewDecisionResponse {
    string submission_id = 1;
    string template_id = 2;
    string status = 3; // "approved" | "rejected"
}

message GetReviewMetricsRequest {}

message RejectionRateEntry {
    string reason_code = 1;
    int32 count = 2;
    double rate = 3; // share of rejections (not of all decisions)
}

message ReviewMetrics {
    int64 open_count = 1;
    // Median hours a currently-open submission has been waiting.
    double median_time_in_review_hours = 2;
    int64 decisions_today = 3;
    repeated RejectionRateEntry rejection_rate_by_reason = 4;
}

message GetPlatformSettingsRequest {}

message PlatformSettingEntry {
    string key = 1;
    string value = 2;
    google.protobuf.Timestamp updated_at = 3;
}

message GetPlatformSettingsResponse {
    repeated PlatformSettingEntry settings = 1;
}

message UpdatePlatformSettingRequest {
    string key = 1;    // must exist in the known-keys whitelist
    string value = 2;  // integer >= 1 as decimal string
    string reason = 3; // 3-500 chars, audit-logged
}
```

Regenerate:

```bash
cd backend && protoc --go_out=. --go-grpc_out=. api/proto/v1/prompt.proto api/proto/v1/admin.proto
```

## 2. Database DDL

Append to **both** `deployment/init.sql` and
`deployment/chart/open-prompts/init.sql` (byte-identical):

```sql
-- -----------------------------------------------------------------------------
-- Feature: pre-publication-review
-- Description: gating state, review submissions, platform settings.
-- -----------------------------------------------------------------------------

-- Gating state on users (OQ-1: denormalized counters). The backfill runs
-- inside the column-add guard, so it executes exactly once — on first
-- migration onto this schema.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='users' AND column_name='approved_template_count') THEN
        ALTER TABLE users ADD COLUMN approved_template_count INT NOT NULL DEFAULT 0;
        COMMENT ON COLUMN users.approved_template_count IS
            'Approved templates maintained at review decision time (gating threshold N)';
        -- Backfill: existing public templates count as approved.
        UPDATE users u
        SET approved_template_count = (
            SELECT COUNT(*) FROM templates t
            WHERE t.owner_id = u.id AND t.visibility = 'public'
        );
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='users' AND column_name='review_gated_remaining') THEN
        ALTER TABLE users ADD COLUMN review_gated_remaining INT NOT NULL DEFAULT 0;
        COMMENT ON COLUMN users.review_gated_remaining IS
            'Publishes still gated after a moderation takedown/ban (re-gate count M)';
    END IF;
END $$;

-- Moderation state + first-approval time on templates. The published_at
-- backfill runs inside the column-add guard (once, on first migration).
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='templates' AND column_name='moderation_status') THEN
        ALTER TABLE templates ADD COLUMN moderation_status TEXT NOT NULL DEFAULT 'normal';
        ALTER TABLE templates ADD CONSTRAINT templates_moderation_status_check
            CHECK (moderation_status IN ('normal', 'in_review', 'rejected'));
        COMMENT ON COLUMN templates.moderation_status IS
            'normal | in_review | rejected (admin-template-moderation widens this list later)';
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='templates' AND column_name='published_at') THEN
        ALTER TABLE templates ADD COLUMN published_at TIMESTAMPTZ;
        COMMENT ON COLUMN templates.published_at IS 'First review-approval time; NULL until approved';
        -- Backfill: existing public templates are considered published at creation.
        UPDATE templates SET published_at = created_at
        WHERE visibility = 'public' AND published_at IS NULL;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_templates_moderation_status
    ON templates(moderation_status);

-- Review submissions (OQ-2: separate table; chain via UNIQUE(template_id, attempt_number)).
CREATE TABLE IF NOT EXISTS review_submissions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    creator_id TEXT NOT NULL,
    attempt_number INT NOT NULL CHECK (attempt_number >= 1),
    status TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'rejected')),
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    decided_at TIMESTAMPTZ,
    decided_by TEXT,
    reject_reason_code TEXT,
    reject_note TEXT,
    UNIQUE (template_id, attempt_number)
);

COMMENT ON TABLE review_submissions IS 'Pre-publication review queue; one row per publish attempt';
COMMENT ON COLUMN review_submissions.attempt_number IS '1 = first submission; >= 2 = resubmission';
COMMENT ON COLUMN review_submissions.status IS 'pending | approved | rejected';
COMMENT ON COLUMN review_submissions.reject_reason_code IS 'spam | plagiarism | harmful | lowQuality | other (NULL for approvals)';

-- Partial index serves the queue: pending only, aging-first + FIFO order.
CREATE INDEX IF NOT EXISTS idx_review_submissions_pending
    ON review_submissions (status, submitted_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_review_submissions_template
    ON review_submissions (template_id);
CREATE INDEX IF NOT EXISTS idx_review_submissions_creator
    ON review_submissions (creator_id, status);

-- Platform settings (NFR-5: N/M/aging configurable without deploy).
CREATE TABLE IF NOT EXISTS platform_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE platform_settings IS 'Admin-editable platform settings (key-value)';

INSERT INTO platform_settings (key, value) VALUES
    ('review_approval_threshold', '3'),
    ('review_regate_count', '3'),
    ('review_aging_hours', '48')
ON CONFLICT (key) DO NOTHING;

-- Widen admin_audit_logs CHECKs for review decisions (guarded, idempotent).
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'admin_audit_logs'::regclass AND conname = 'admin_audit_logs_action_check';
    IF def IS NOT NULL AND def NOT LIKE '%review_approve%' THEN
        ALTER TABLE admin_audit_logs DROP CONSTRAINT admin_audit_logs_action_check;
        ALTER TABLE admin_audit_logs ADD CONSTRAINT admin_audit_logs_action_check
            CHECK (action IN ('ban', 'unban', 'role_change', 'delete_comment',
                              'review_approve', 'review_reject', 'settings_change'));
    END IF;
END $$;

DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'admin_audit_logs'::regclass AND conname = 'admin_audit_logs_target_type_check';
    IF def IS NOT NULL AND def NOT LIKE '%template%' THEN
        ALTER TABLE admin_audit_logs DROP CONSTRAINT admin_audit_logs_target_type_check;
        ALTER TABLE admin_audit_logs ADD CONSTRAINT admin_audit_logs_target_type_check
            CHECK (target_type IN ('user', 'comment', 'template', 'platform'));
    END IF;
END $$;

-- Widen notifications.type CHECK for decision notifications (guarded).
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'notifications'::regclass AND conname = 'notifications_type_check';
    IF def IS NOT NULL AND def NOT LIKE '%template_approved%' THEN
        ALTER TABLE notifications DROP CONSTRAINT notifications_type_check;
        ALTER TABLE notifications ADD CONSTRAINT notifications_type_check
            CHECK (type IN ('like', 'favorite', 'fork', 'comment', 'template_approved', 'template_rejected'));
    END IF;
END $$;
```

## 3. Backend Layers

### 3.1 Models

```go
// models/template.go — add:
ModerationStatus string         `json:"moderation_status"` // "normal" | "in_review" | "rejected"
PublishedAt      sql.NullTime   `json:"published_at"`

// models/user.go — add:
ApprovedTemplateCount int32 `json:"approved_template_count"`
ReviewGatedRemaining  int32 `json:"review_gated_remaining"`

// models/review.go — new file:
type ReviewSubmission struct {
    ID               string     `json:"id"`
    TemplateID       string     `json:"template_id"`
    CreatorID        string     `json:"creator_id"`
    AttemptNumber    int32      `json:"attempt_number"`
    Status           string     `json:"status"` // "pending" | "approved" | "rejected"
    SubmittedAt      time.Time  `json:"submitted_at"`
    DecidedAt        *time.Time `json:"decided_at"`
    DecidedBy        *string    `json:"decided_by"`
    RejectReasonCode *string    `json:"reject_reason_code"`
    RejectNote       *string    `json:"reject_note"`
}

type PlatformSetting struct {
    Key       string    `json:"key"`
    Value     string    `json:"value"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

### 3.2 `TemplateRepository` changes

`Create` INSERT gains `moderation_status`, `published_at` columns; `Update`
SET clause gains both; `Get`/`List`/`CountTemplates` SELECTs gain both (scan
into the new model fields).

**`Get` hiding rule** (extends the existing banned-owner block):

```go
// in_review/rejected templates are visible only to their owner (admins use
// the admin repository, which does not filter). 404-equivalent for everyone
// else — no status leak.
if t.ModerationStatus != "normal" && t.OwnerID != currentUserID {
    return nil, fmt.Errorf("template not found")
}
```

**`buildTemplateConditions`** — the public-stream moderation filter mirrors
the banned-owner pattern and applies on the same condition (visibility absent
or public), with an owner bypass via the already-bound `current_user_id`
placeholder `$1`:

```sql
AND (
    t.moderation_status = 'normal'
    OR t.owner_id = $1   -- $1 is current_user_id ("" for anonymous)
)
```

Because `$1` is `''` for anonymous callers, the bypass never matches and
anonymous/public streams see only `normal` templates; the owner's mixed view
sees their own gated templates. `CountTemplates` shares the builder, so
counts stay consistent. The private-stream branch (`visibility = 'private'`)
is untouched — private templates are never gated.

### 3.3 `ReviewRepository` (new file)

```go
type ReviewQueueFilters struct {
    Category        string
    AgingOnly       bool
    ResubmissionOnly bool
    AgingHours      int
    Limit           int
    Offset          int
}

type ReviewRepository interface {
    CreateSubmission(ctx context.Context, s *models.ReviewSubmission) error
    GetSubmission(ctx context.Context, id string) (*models.ReviewSubmission, error)
    GetLatestSubmission(ctx context.Context, templateID string) (*models.ReviewSubmission, error)
    ListQueue(ctx context.Context, f ReviewQueueFilters) ([]*models.ReviewQueueRow, int64, error)
    GetSubmissionDetail(ctx context.Context, id string) (*models.ReviewSubmissionDetail, error)
    ListChain(ctx context.Context, templateID string) ([]*models.ReviewSubmission, error)
    DecideSubmission(ctx context.Context, id string, status string, decidedBy string,
        reasonCode, note *string) (*models.ReviewSubmission, error) // tx: UPDATE ... WHERE status='pending'
    GetMetrics(ctx context.Context) (*models.ReviewMetrics, error)
    // Platform settings
    GetSettings(ctx context.Context) ([]*models.PlatformSetting, error)
    GetSetting(ctx context.Context, key string) (string, error)
    UpsertSetting(ctx context.Context, key, value string) error
    // Gating state
    GetUserGatingState(ctx context.Context, userID string) (approved int32, gatedRemaining int32, err error)
    IncrementApprovedCount(ctx context.Context, userID string) error
    DecrementGatedRemaining(ctx context.Context, userID string) error
    SetGatedRemaining(ctx context.Context, userID string, value int32) error
}
```

**Queue SQL** (aging-first, then FIFO — OQ-5):

```sql
SELECT rs.id, rs.template_id, t.title, rs.creator_id, u.display_name,
       rs.attempt_number, rs.submitted_at, t.category,
       (rs.submitted_at < now() - ($1 * interval '1 hour')) AS aging,
       u.approved_template_count,
       (SELECT COUNT(*) FROM review_submissions x
        WHERE x.creator_id = rs.creator_id AND x.status = 'rejected') AS prior_rejections,
       (u.status = 'banned') AS creator_banned
FROM review_submissions rs
JOIN templates t ON t.id = rs.template_id
JOIN users u ON u.id = rs.creator_id
WHERE rs.status = 'pending'
  AND ($2 = '' OR t.category = $2)                       -- category filter
  AND (NOT $3 OR rs.submitted_at < now() - ($1 * interval '1 hour'))  -- aging_only
  AND (NOT $4 OR rs.attempt_number >= 2)                 -- resubmission_only
ORDER BY (rs.submitted_at < now() - ($1 * interval '1 hour')) DESC,
         rs.submitted_at ASC
LIMIT $5 OFFSET $6;
```

(The count query shares the WHERE, minus LIMIT/OFFSET.)

**Metrics SQL:**

```sql
-- open_count + median wait (percentile_cont over pending ages)
SELECT COUNT(*) AS open_count,
       COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY
           EXTRACT(EPOCH FROM (now() - submitted_at)) / 3600.0), 0) AS median_hours
FROM review_submissions WHERE status = 'pending';

-- decisions today
SELECT COUNT(*) FROM review_submissions
WHERE status <> 'pending' AND decided_at >= date_trunc('day', now());

-- rejection rate by reason (share of rejections)
SELECT reject_reason_code, COUNT(*) AS cnt,
       COUNT(*)::float / NULLIF(SUM(COUNT(*)) OVER (), 0) AS rate
FROM review_submissions
WHERE status = 'rejected' AND reject_reason_code IS NOT NULL
GROUP BY reject_reason_code;
```

**Decision transaction** (`DecideSubmission`):

```sql
UPDATE review_submissions
SET status = $2, decided_at = now(), decided_by = $3,
    reject_reason_code = $4, reject_note = $5
WHERE id = $1 AND status = 'pending'
RETURNING template_id, creator_id, attempt_number;
```

Zero rows → `ErrAlreadyDecided` (service maps to `FailedPrecondition`).
The service then updates `templates` and `users` in the same transaction
scope (see 3.5).

### 3.4 `PromptService` gating hooks

New helper on `PromptService` (which gains a `ReviewRepo
repository.ReviewRepository` field):

```go
// isGated returns true when the creator's next publish must enter review.
// Gated iff approved_template_count < N OR review_gated_remaining > 0.
func (s *PromptService) isGated(ctx context.Context, userID string) bool
```

Settings are read through the 60s cache (`review_approval_threshold`,
`review_regate_count`, `review_aging_hours`).

**`CreateTemplate`** — after the visibility mapping, before `TemplateRepo.Create`:

```go
moderationStatus := "normal"
var publishedAt sql.NullTime
if visibility == "public" && s.isGated(ctx, userID) {
    moderationStatus = "in_review"
} else if visibility == "public" {
    publishedAt = sql.NullTime{Time: time.Now(), Valid: true}
}
template.ModerationStatus = moderationStatus
template.PublishedAt = publishedAt
```

After the version-1 creation succeeds and `moderationStatus == "in_review"`:
create the submission (attempt 1) and decrement `review_gated_remaining`
when > 0 — both best-effort-logged but *inside* the same request; a failure
rolls the whole publish into an error so a gated template can never exist
without its queue row.

**`UpdateTemplate`** — at the visibility transition (private→public, or
rejected→public resubmission):

```go
wasPublic := template.Visibility == "public"
// ... existing field updates ...
transitioningToPublic := !wasPublic && template.Visibility == "public"
if transitioningToPublic {
    if s.isGated(ctx, userID) {
        template.ModerationStatus = "in_review"
        template.PublishedAt = sql.NullTime{}
    } else {
        template.ModerationStatus = "normal"
        if !template.PublishedAt.Valid {
            template.PublishedAt = sql.NullTime{Time: time.Now(), Valid: true}
        }
    }
}
```

After `TemplateRepo.Update` succeeds and the template entered `in_review`:
`attempt = latestSubmission.AttemptNumber + 1` (or 1 when none), insert the
submission, decrement `review_gated_remaining` when > 0. Edits that do not
cross the publish boundary leave `moderation_status` untouched (FR-3/FR-6:
in_review/rejected stay editable; FR-14: exempt creators' published edits
stay live).

**`CreatePrompt` visibility guard** (pre-existing gap, fixed here): after
fetching the version, load its template via `TemplateRepo.Get(ctx,
version.TemplateID, userID)` and fail with `NotFound` when hidden — a gated
template's content must not be renderable through the prompt path.

### 3.5 `AdminService` review RPCs

`AdminService` gains a `ReviewRepo repository.ReviewRepository` field. All
seven RPCs start with `requireAdminContext`.

**`ApproveReviewSubmission`** (one transaction where possible; notification
and audit are best-effort per existing patterns):

1. `DecideSubmission(ctx, id, "approved", admin.ID, nil, nil)` →
   `NotFound` when unknown, `FailedPrecondition` when already decided.
2. Update the template: `moderation_status = 'normal'`,
   `published_at = now()` (only when currently `in_review` — guard with
   `WHERE id = $1 AND moderation_status = 'in_review'`).
3. `IncrementApprovedCount(creatorID)` (FR-7: counts toward N).
4. `writeAudit(ctx, actor, "review_approve", "template", templateID,
   "Approved in pre-publication review")` (≥3 chars, satisfies the CHECK).
5. Notification (best-effort, self-suppressed — see 3.6).

**`RejectReviewSubmission`:**

1. Validate `reason_code` against the whitelist
   (`spam|plagiarism|harmful|lowQuality|other`) → `InvalidArgument` when
   missing/unknown; `note` ≤ 500 chars → `InvalidArgument`.
2. `DecideSubmission(ctx, id, "rejected", admin.ID, &reasonCode, &note)`.
3. Update the template: `moderation_status = 'rejected'` (same
   `in_review` guard).
4. `writeAudit(ctx, actor, "review_reject", "template", templateID,
   reasonCode + ": " + note)` (FR-10: actor, timestamp, reason).
5. Notification with reason (best-effort).

**`ListReviewQueue` / `GetReviewSubmission` / `GetReviewMetrics`:**
read-only; map repository rows to the proto messages; `GetReviewSubmission`
assembles the chain via `ListChain` and the author summary (joined_at,
published count = `approved_template_count`, prior rejections).

**`GetPlatformSettings` / `UpdatePlatformSetting`:** list reads all rows;
update validates `key` against the known whitelist
(`review_approval_threshold|review_regate_count|review_aging_hours`) and
`value` as a positive integer, calls `UpsertSetting`, invalidates the
settings cache, and `writeAudit(ctx, actor, "settings_change", "platform",
key, reason)` — the `settings_change` action and `platform` target_type are
included in the §2 CHECK widenings.

**`BanUser` hook (FR-2/US-8):** after the existing ban succeeds, set
`review_gated_remaining = M` (`SetGatedRemaining`) so the user's publishes
are re-gated after reinstatement. Best-effort (logged, non-fatal) to keep
the ban path unchanged on review-store failures.

### 3.6 Decision notifications

Reuse the `createEngagementNotification` shape in `AdminService` (actor =
deciding admin, `ActorDisplayName` = admin display name):

- Approve → type `template_approved`.
- Reject → type `template_rejected`.

Self-decision edge case (an admin reviewing their own submission): skip and
log a warning — consistent with the existing self-suppression and the
`no_self_notify` CHECK backstop. Failures are logged and swallowed; the
decision itself is never affected (NFR-4 is covered by the status surfaces,
and the notification store is the delivery mechanism).

### 3.7 Gateway (`backend/cmd/server/main.go`)

Seven routes, all behind `requireAdmin`, following the existing
`/api/v1/admin/comments/` TrimPrefix pattern:

```go
// GET  /api/v1/admin/review            (query: category, aging, resubmission_only, page, per_page)
// GET  /api/v1/admin/review/metrics
http.HandleFunc("/api/v1/admin/review", func(w, r) { /* exact path only;
   "metrics" is handled by the /review/ prefix handler below to keep one
   registration — actually register /api/v1/admin/review for the list and
   let the prefix handler own /review/metrics, /review/{id}, ... */ })

// GET  /api/v1/admin/review/metrics
// GET  /api/v1/admin/review/{id}
// POST /api/v1/admin/review/{id}/approve
// POST /api/v1/admin/review/{id}/reject   (JSON body: reason_code, note)
http.HandleFunc("/api/v1/admin/review/", func(w, r) {
    rest := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/review/")
    parts := strings.Split(rest, "/")
    switch {
    case rest == "metrics" && r.Method == http.MethodGet: // GetReviewMetrics
    case len(parts) == 1 && r.Method == http.MethodGet:   // GetReviewSubmission{Id: parts[0]}
    case len(parts) == 2 && parts[1] == "approve" && r.Method == http.MethodPost:
    case len(parts) == 2 && parts[1] == "reject" && r.Method == http.MethodPost:
    default: 404/405
    }
})

// GET /api/v1/admin/settings   → GetPlatformSettings
// PUT /api/v1/admin/settings   → UpdatePlatformSetting (JSON: key, value, reason)
http.HandleFunc("/api/v1/admin/settings", func(w, r) { ... })
```

Query params map exactly as in `ListAdminTemplates` (`page`, `per_page`
ints with defaults 1/20, capped at 100). JSON bodies decode into the request
structs; `writeError` maps the gRPC codes per the existing table.

## 4. Frontend

### 4.1 Routes

```js
// App.js — inside the /admin lazy block (AdminLayout):
<Route path="review" element={<ReviewPage />} />          // /admin/review
<Route path="review/:id" element={<ReviewDetailPage />} /> // /admin/review/:id
```

`AdminLayout` navItems gains `{ to: '/admin/review', label: t('admin.nav.review') }`
(after Templates).

### 4.2 New Files

| File | Purpose |
|---|---|
| `frontend/src/pages/admin/ReviewPage.js` + `.scss` | MetricsBar + QueueFilters + QueueTable; `j`/`k` row navigation. |
| `frontend/src/pages/admin/ReviewDetailPage.js` + `.scss` | SubmissionDetail + approve/reject actions + keyboard (`A`, `R`, `→`); auto-advance to next queue item after a decision. |
| `frontend/src/components/review/ReviewStatusCard.js` + `.scss` | Creator card: status icon, title, submitted time, expected window, View/Edit; rejected variant shows reason + resubmit CTA. |
| `frontend/src/components/review/PublishReviewModal.js` + `.scss` | Post-publish confirmation for gated creators. |
| `frontend/src/components/review/QueueTable.js` + `.scss` | Rows: aging badge (amber ⏳ + age), title, author, context stat, "(nth)" resubmission chip, Review button. |
| `frontend/src/components/review/QueueFilters.js` + `.scss` | Tabs All / Aging / Resubmissions + category select. |
| `frontend/src/components/review/MetricsBar.js` + `.scss` | Open count, median wait, decisions today. |
| `frontend/src/components/review/SubmissionDetail.js` + `.scss` | Content preview, author summary line, resubmission chain, actions. |
| `frontend/src/components/review/RejectDialog.js` + `.scss` | Fixed reason radios (required — Reject disabled until chosen), optional note, confirm. |

### 4.3 Modified Files

| File | Change |
|---|---|
| `frontend/src/services/adminApi.js` | `getReviewQueue(params)`, `getReviewMetrics()`, `getReviewSubmission(id)`, `approveReviewSubmission(id)`, `rejectReviewSubmission(id, reasonCode, note)`, `getPlatformSettings()`, `updatePlatformSetting(key, value, reason)`. |
| `frontend/src/pages/Home.js` | Render `ReviewStatusCard` list above the template grid for own `in_review`/`rejected` templates (from the mixed-view private stream + a status filter). |
| `frontend/src/pages/TemplateDetails.js` | `handleShare`/`handleUnshare`: when the update response's `template.moderation_status === 'in_review'`, show `PublishReviewModal` instead of the live toast. Owner banner on rejected templates (reason + "Fix and resubmit"). |
| `frontend/src/components/CreateTemplateModal.js` | `handleSubmit`: when the create response carries `moderation_status === 'in_review'`, show `PublishReviewModal` before `onSuccess()`. |
| `frontend/src/components/PromptCard.js` | Moderation badge next to the visibility badge for own `in_review`/`rejected` templates. |
| `frontend/src/components/notifications/NotificationItem.js` | `EVENT_ICONS` += `template_approved`/`template_rejected` entries; special-case the sentence to `review.notify.approved`/`review.notify.rejected` (actor-less). |
| `frontend/src/components/admin/AdminLayout.js` | Review nav entry. |
| `frontend/src/App.js` | Review routes. |
| `frontend/src/locales/{en,zh}/translation.json` | `review.*` keys + `admin.nav.review` + `admin.review.*` console keys. |

### 4.4 Redux

No new slice: queue state is page-local (`useState`/`useEffect` via
`adminApi`), matching the existing admin console pages; creator status
arrives with template payloads (`moderation_status` on the Template
message).

### 4.5 i18n Keys (EN/ZH parity)

`review.*` verbatim from UI/UX §8:

```json
{
  "review": {
    "status": { "inReview": "In review", "rejected": "Rejected", "approved": "Live now" },
    "card": { "submitted": "Submitted {{time}}", "expected": "Expected decision within 24 hours" },
    "modal": {
      "title": "Submitted for review",
      "body": "Your template will go live after a quick review — usually within 24 hours. You'll get a notification either way."
    },
    "queue": {
      "title": "Review queue", "open": "Open: {{count}}", "median": "Median wait: {{duration}}",
      "today": "Today: {{count}} decisions", "aging": "Aging >48h", "resubmissions": "Resubmissions",
      "empty": "Queue is clear"
    },
    "row": { "resubmission": "({{n}}th)" },
    "detail": {
      "author": "Joined {{date}} · {{published}} published · {{rejections}} prior rejections",
      "chain": "Resubmission history"
    },
    "action": { "approve": "Approve", "reject": "Reject…" },
    "reject": {
      "title": "Reject template", "reasonRequired": "Pick a reason to continue",
      "note": "Note to creator (optional)",
      "reason": {
        "spam": "Spam", "plagiarism": "Plagiarism", "harmful": "Harmful content",
        "lowQuality": "Low quality", "other": "Other"
      }
    },
    "notify": {
      "approved": "Your template \"{{title}}\" is live",
      "rejected": "Your template \"{{title}}\" was rejected: {{reason}}"
    },
    "cta": { "resubmit": "Fix and resubmit" },
    "notFound": "This page doesn't exist"
  },
  "admin": { "nav": { "review": "Review" } }
}
```

ZH mirrors every key exactly as specified in UI/UX §8 (审核中 / 已拒绝 /
已上线 / 提交于 {{time}} / 预计 24 小时内出结果 / 已提交审核 / … /
队列已清空 / （第 {{n}} 次）/ 注册于 … / 重新提交历史 / 通过 / 拒绝… /
拒绝模板 / 请选择原因后继续 / 给创作者的备注（可选）/ 垃圾内容 / 抄袭 /
有害内容 / 质量低下 / 其他 / 你的模板"{{title}}"已上线 /
你的模板"{{title}}"被拒绝：{{reason}} / 修改并重新提交 / 页面不存在 /
`admin.nav.review` = "审核")。Additional console keys
(`admin.review.settings.*` for the settings form, aging badge labels) follow
the same EN/ZH parity rule.

## 5. Error Matrix

| Scenario | HTTP | gRPC | Frontend handling |
|---|---|---|---|
| Reject without reason_code | 400 | `INVALID_ARGUMENT` | Reject button pre-disabled; inline error if raced |
| Unknown reason code / note >500 | 400 | `INVALID_ARGUMENT` | Inline error; dialog stays open |
| Invalid settings key/value | 400 | `INVALID_ARGUMENT` | Field error in settings form |
| Non-admin on any review/settings route | 401/403 | `UNAUTHENTICATED`/`PERMISSION_DENIED` | Admin login redirect |
| Submission id unknown | 404 | `NOT_FOUND` | Not-found page; back to queue |
| Submission already decided | 409 | `FAILED_PRECONDITION` | Toast "already decided"; refresh queue |
| Gated template accessed by non-owner (detail/fork/comment/prompt) | 404 | `NOT_FOUND` | Standard not-found page (no status leak) |
| Notification insert fails | — | — | Logged server-side; decision unaffected |

## 6. Deployment

- DDL additive/idempotent; both init.sql copies byte-identical in the same
  commit; CHECK widenings guarded by `pg_get_constraintdef`; backfills
  guarded by column-existence checks.
- No new services, ports, or infrastructure. No environment variables —
  thresholds live in `platform_settings`.
- Helm chart: no template changes (init.sql reaches the chart via the
  existing ConfigMap); `values.yaml` untouched.
- Never commit `backend/server` (tracked build artifact).

## 7. SDK

No `python-sdk/openprompts` changes: gated templates are never returned to
non-owners by `list_templates`/`get_template` (the repository filters
them), and `get_prompt_by_alias` 404s for hidden templates once the
`CreatePrompt`/alias visibility guard lands. The admin review surface is a
web console feature; the SDK wraps the public template API only.

## 8. Test Strategy

### 8.1 FVT (`tests/fvt.py`) — `test_pre_publication_review`

Users: `newbie` (0 approved), `established` (create 3 public templates
first — instant publishes), `second` (observer). Admin via
`ADMIN_BOOTSTRAP_*` (POST `/admin/login` → admin headers).

1. **AC-1/AC-2:** newbie publishes → response `moderation_status ==
   "in_review"`, `published_at` empty; established publishes →
   `moderation_status == "normal"`, `published_at` set, immediately in
   public list.
2. **AC-9:** newbie's template: anonymous GET → 404; second-user GET →
   404; public list/search (keyword = title) → absent; newbie GET → 200
   with `in_review`.
3. **FR-8/AC-8:** admin GET `/admin/review` → contains newbie's submission;
   ordering oldest-first; create an older submission (direct SQL or second
   template + time manipulation via submitted_at is not available — assert
   relative order of two submissions created in sequence); aging badge
   false for fresh items.
4. **AC-3:** approve → template `moderation_status == "normal"`,
   `published_at` ≈ now, visible to anonymous; newbie notification list
   contains `template_approved`; newbie's next publish still gated
   (approved count 1 < 3).
5. **AC-5:** reject without `reason_code` → 400; UI-level: covered by e2e.
6. **AC-4:** reject with `spam` → template `rejected`, 404 for second
   user; newbie notification contains `template_rejected` with the reason;
   owner GET → 200 with `rejected`.
7. **AC-6:** newbie edits and re-publishes (update visibility public) →
   new submission `attempt_number == 2`; queue row `is_resubmission`;
   detail shows chain with prior reason.
8. **AC-10:** `/admin/review/metrics` → open_count matches pending;
   decisions_today ≥ 2; rejection_rate_by_reason contains spam.
9. **FR-2/AC-7:** admin bans a creator with a public template (or the
   established user) → unban → next publish gated
   (`review_gated_remaining` path via BanUser hook).
10. **NFR-5:** GET `/admin/settings` → three keys; PUT
    `review_approval_threshold` = 1 with reason → subsequent newbie
    publish (approved count 1) goes live instantly; PUT back to 3.
11. **Audit:** `/admin/audit-log?action=review_approve` and
    `action=review_reject` contain the decisions with actor + reason.

### 8.2 Unit Tests

- `isGated` matrix: threshold boundary (N-1 gated, N exempt), re-gate
  counter decrement, both conditions OR'd.
- Repository: queue ordering (aging-first then FIFO), filters
  (category/aging/resubmission), `DecideSubmission` concurrency (second
  decide → `ErrAlreadyDecided`), metrics math (median, rate shares).
- Service: reason-code whitelist, note length, settings key/value
  validation, `FailedPrecondition` mapping, notification self-suppression.
- Update mocks: `MockTemplateRepository` (new columns),
  `MockReviewRepository` (new interface).

### 8.3 E2E (test-agent, Nightwatch)

- Queue page: metrics bar renders, tabs filter, category select, row
  skeletons/empty/error states, `j`/`k` navigation.
- Detail: content preview, author summary line, chain entries, `A`
  approves with toast + auto-advance, `R` opens dialog.
- RejectDialog: disabled until a radio is chosen; note optional; submit
  advances.
- Creator: status cards on Home (in_review + rejected variants with
  reason), publish modal after gated publish, TemplateDetails owner banner,
  PromptCard badge.
- Notifications: approved/rejected rows render with the new icons and
  actor-less sentences; deep-link to the template.
- Direct URL: non-admin opening an in_review URL sees the not-found page.
- i18n EN/ZH parity for all `review.*` and `admin.nav.review` keys.

## 9. Implementation Order

1. DDL (both init.sql copies) + models + `ReviewRepository` + unit tests.
2. Proto changes + regenerate + `templateModelToProto` mapping +
   repository SELECT/INSERT/UPDATE columns.
3. Visibility filters: `Get` rule + `buildTemplateConditions` moderation
   filter + `CreatePrompt` guard.
4. Gating hooks in `CreateTemplate`/`UpdateTemplate` + settings cache.
5. Admin review RPCs + `BanUser` hook + notifications + audit.
6. Gateway routes.
7. FVT `test_pre_publication_review`.
8. Frontend admin (routes, pages, components, adminApi).
9. Frontend creator (Home cards, modals, banners, badges,
   NotificationItem) + i18n en/zh.
10. Component tests + handoff to test-agent for e2e.

## 10. Constraints for Developer

1. **Never commit `backend/server`** — tracked build artifact; `git
   checkout -- backend/server` if modified.
2. Stage only your own files; never `git add -A` (the tree holds other
   agents' WIP).
3. Both init.sql copies byte-identical in the same commit; every DDL block
   idempotent (column-existence guards, `pg_get_constraintdef` for CHECK
   widenings, `ON CONFLICT DO NOTHING` for settings seeds).
4. **`Template` proto field numbers 25/26 only.** 18–24 are reserved by
   follow-creators (18/19), template-ratings (20–22), and prompt-analytics
   (23/24) — never reuse them. Do not touch `AdminTemplateSummary` (13/14
   contested by ratings/import-export) or `GetTemplateResponse` (3/4
   claimed).
5. Hidden-template semantics: **404, never 403**, for non-owners on every
   read path — detail, list, search, fork, comments, prompts, aliases. No
   status leak anywhere.
6. The moderation filter must mirror the banned-owner pattern exactly:
   public-stream condition `AND (t.moderation_status = 'normal' OR
   t.owner_id = $current_user_id)`; private streams untouched.
7. Gated publish must be atomic with its submission row: if the submission
   insert fails, the publish fails — a gated template without a queue row
   is a bug.
8. Decisions are transactional where possible; notification and audit
   writes are best-effort (logged, non-fatal) per existing patterns.
9. `review_gated_remaining` decrements only when > 0 and only on a gated
   publish; `approved_template_count` increments only on approval of a
   pending submission (idempotence guarded by the `WHERE status='pending'`
   decide).
10. Reason codes are a server-side whitelist — never trust the client's
    string; settings keys likewise.
11. int32 for all counts; `published_at` is `sql.NullTime` (NULL until
    first approval).
12. No monetization surfaces of any kind.
