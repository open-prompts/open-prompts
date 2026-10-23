# Admin Template Moderation — Detailed Design

| | |
|---|---|
| **Feature ID** | admin-template-moderation |
| **Version** | 1.0 |
| **Status** | Approved for implementation |
| **Author** | Architect Agent |
| **Upstream** | [Architecture](admin-template-moderation-architecture.md) |

## 1. Proto Contract

### 1.1 `backend/api/proto/v1/admin.proto`

Append to the `AdminService` service block (after `ResolveReportGroup`, line 108):

```protobuf
service AdminService {
    // ... existing RPCs ...
    rpc UnpublishTemplate(ModerationActionRequest) returns (ModerationActionResponse);
    rpc LockTemplate(ModerationActionRequest) returns (ModerationActionResponse);
    rpc SoftDeleteTemplate(ModerationActionRequest) returns (ModerationActionResponse);
    rpc RestoreTemplate(ModerationActionRequest) returns (ModerationActionResponse);
}

message ModerationActionRequest {
    string template_id = 1;
    string reason = 2; // 3-500 chars, required
}

message ModerationActionResponse {
    string template_id = 1;
    string moderation_status = 2; // the new status
    google.protobuf.Timestamp moderated_at = 3;
}
```

Add fields to `AdminTemplateSummary` (after `rating_count = 17`):

```protobuf
message AdminTemplateSummary {
    // ... existing fields 1-17 ...
    // Moderation state: "normal" | "locked" | "unpublished_by_admin" |
    // "soft_deleted" | "in_review" | "rejected".
    string moderation_status = 18;
    // Timestamp of the last moderation action (NULL until first action).
    google.protobuf.Timestamp moderated_at = 19;
    // Admin id of the last moderation action (NULL until first action).
    string moderated_by = 20;
    // Reason of the last moderation action (admin-only, shown to owner too).
    string moderation_reason = 21;
}
```

Add a filter to `ListAdminTemplatesRequest` (after `warning = 7`):

```protobuf
message ListAdminTemplatesRequest {
    // ... existing fields 1-7 ...
    string moderation_status = 8; // "" | normal | locked | unpublished_by_admin | soft_deleted
}
```

Regenerate:

```bash
cd backend && protoc --go_out=. --go-grpc_out=. api/proto/v1/admin.proto
```

## 2. Database DDL

Append to **both** `deployment/init.sql` and
`deployment/chart/open-prompts/init.sql`:

```sql
-- ===== admin-template-moderation =====
-- Widen the moderation_status CHECK (guarded, idempotent). The column and its
-- original CHECK were introduced by pre-publication-review.
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'templates'::regclass AND conname = 'templates_moderation_status_check';
    IF def IS NOT NULL AND def NOT LIKE '%unpublished_by_admin%' THEN
        ALTER TABLE templates DROP CONSTRAINT templates_moderation_status_check;
        ALTER TABLE templates ADD CONSTRAINT templates_moderation_status_check
            CHECK (moderation_status IN ('normal', 'in_review', 'rejected',
                                         'locked', 'unpublished_by_admin', 'soft_deleted'));
    END IF;
END $$;

-- Admin-only moderation metadata (NULL until first action).
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='templates' AND column_name='moderated_at') THEN
        ALTER TABLE templates ADD COLUMN moderated_at TIMESTAMPTZ;
        ALTER TABLE templates ADD COLUMN moderated_by TEXT;
        ALTER TABLE templates ADD COLUMN moderation_reason TEXT;
        COMMENT ON COLUMN templates.moderated_at IS 'Timestamp of the last moderation action';
        COMMENT ON COLUMN templates.moderated_by IS 'Admin id of the last moderation action';
        COMMENT ON COLUMN templates.moderation_reason IS 'Reason of the last moderation action (shown to owner + admins only)';
    END IF;
END $$;

-- Soft-delete restore-window start (NULL until soft-deleted). The 30-day
-- hard-delete ticker scans on this timestamp.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='templates' AND column_name='deleted_at') THEN
        ALTER TABLE templates ADD COLUMN deleted_at TIMESTAMPTZ;
        COMMENT ON COLUMN templates.deleted_at IS 'Soft-delete time; starts the 30-day restore window (NULL otherwise)';
    END IF;
END $$;

-- Widen admin_audit_logs.action CHECK for moderation actions (guarded).
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'admin_audit_logs'::regclass AND conname = 'admin_audit_logs_action_check';
    IF def IS NOT NULL AND def NOT LIKE '%template.unpublish%' THEN
        ALTER TABLE admin_audit_logs DROP CONSTRAINT admin_audit_logs_action_check;
        ALTER TABLE admin_audit_logs ADD CONSTRAINT admin_audit_logs_action_check
            CHECK (action IN ('ban', 'unban', 'role_change', 'delete_comment',
                              'review_approve', 'review_reject', 'settings_change',
                              'template.warning_add', 'template.warning_remove',
                              'appeal.create', 'appeal.review', 'appeal.resolve',
                              'report.dismiss', 'report.actioned',
                              'template.unpublish', 'template.lock',
                              'template.soft_delete', 'template.restore',
                              'template.republished_after_moderation'));
    END IF;
END $$;

-- Widen notifications.type CHECK for the moderation event type (guarded).
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'notifications'::regclass AND conname = 'notifications_type_check';
    IF def IS NOT NULL AND def NOT LIKE '%moderation%' THEN
        ALTER TABLE notifications DROP CONSTRAINT notifications_type_check;
        ALTER TABLE notifications ADD CONSTRAINT notifications_type_check
            CHECK (type IN ('like', 'favorite', 'fork', 'comment',
                            'template_approved', 'template_rejected',
                            'template_warning_changed', 'follow',
                            'appeal_resolved', 'comment_reply',
                            'moderation'));
    END IF;
END $$;
```

The `moderation_actions` table is **already defined** by moderation-appeals in
`deployment/init.sql` — this feature only **populates** it (no DDL change).

## 3. Backend Layers

### 3.1 `models` (`backend/internal/models/template.go`)

Add to the `Template` model:

```go
type Template struct {
    // ... existing fields ...
    ModerationStatus string       `json:"moderation_status"` // normal | in_review | rejected | locked | unpublished_by_admin | soft_deleted
    PublishedAt      sql.NullTime `json:"published_at"`
    ModeratedAt      sql.NullTime `json:"moderated_at"`
    ModeratedBy      string       `json:"moderated_by"`
    ModerationReason string       `json:"moderation_reason"`
    DeletedAt        sql.NullTime `json:"deleted_at"` // soft-delete window start
}
```

### 3.2 `AdminRepository` (`backend/internal/repository/admin_repository.go`)

Add to the `AdminRepository` interface:

```go
type AdminRepository interface {
    // ... existing methods ...
    // SetTemplateModeration transitions a template's moderation status.
    // publish=true sets moderation_status='normal' + published_at=now();
    // publish=false sets moderation_status='rejected'. Both are guarded by
    // WHERE moderation_status='in_review' so concurrent decisions are safe.
    SetTemplateModeration(ctx context.Context, templateID string, status string, publish bool) error
    // SetModerationState sets an arbitrary moderation_status + moderation
    // metadata (moderated_at/by/reason) on a template. Returns
    // ErrTemplateNotFound when the id is unknown.
    SetModerationState(ctx context.Context, templateID, status, actorID, reason string) error
    // InsertModerationAction records a moderation action for the appeals
    // lifecycle. Returns the new action id.
    InsertModerationAction(ctx context.Context, a *models.ModerationAction) (string, error)
    // MarkModerationActionSuperseded flips the latest active action for a
    // template to 'superseded' (used on restore).
    MarkModerationActionSuperseded(ctx context.Context, templateID string) error
    // ListAuditByTarget returns the audit entries for a target (reused for
    // the moderation history).
    ListAuditByTarget(ctx context.Context, targetID string, limit int) ([]*models.AdminAuditLog, error)
}
```

`SetModerationState` SQL:

```sql
UPDATE templates
SET moderation_status = $1,
    moderated_at = now(),
    moderated_by = $2,
    moderation_reason = $3,
    updated_at = now()
WHERE id = $4
RETURNING id;
```

`InsertModerationAction` SQL:

```sql
INSERT INTO moderation_actions (template_id, action_type, actor_id, actor_email, reason)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;
```

`MarkModerationActionSuperseded` SQL:

```sql
UPDATE moderation_actions
SET status = 'superseded', superseded_at = now()
WHERE template_id = $1 AND status = 'active';
```

### 3.3 `TemplateRepository` (`backend/internal/repository/template_repository.go`)

Extend `buildTemplateConditions` to hide `unpublished_by_admin` and
`soft_deleted` templates from non-owners (OQ-1). The existing moderation
filter already hides `in_review`/`rejected`; widen it:

```go
// Moderation filter mirrors the banned-owner pattern: gated
// (in_review/rejected/unpublished_by_admin/soft_deleted) templates are
// hidden from every public stream except their owner's. $1 is
// current_user_id ("" for anonymous), so the bypass never matches for
// anonymous callers.
query += " AND (t.moderation_status = 'normal' OR t.owner_id = $1)"
```

This single clause now covers all four non-public moderation states. The
`locked` state is intentionally **not** hidden — locked templates stay
publicly visible (FR-3).

### 3.4 `AdminService` (`backend/internal/service/admin.go`)

Add a shared helper and the four RPCs:

```go
// moderationRateLimit enforces the per-admin write tier (FR-15).
func (s *AdminService) moderationRateLimit(ctx context.Context, adminID string) error {
    // Redis counter mod_action_rate:{adminID}, sliding window,
    // limit MOD_ACTION_RATE_PER_MINUTE (default 10), fail-open.
}

// applyModerationAction performs the shared state change: self-guard,
// transition guard, state update, moderation_actions insert, audit, report
// auto-resolve, notification.
func (s *AdminService) applyModerationAction(ctx context.Context, actor *models.User,
    req *pb.ModerationActionRequest, actionType, newStatus string) (*pb.ModerationActionResponse, error) {
    // 1) Load the template; missing -> NotFound.
    // 2) Self-guard: if template.OwnerID == actor.ID -> PermissionDenied.
    // 3) Transition guard: reject invalid transitions (below).
    // 4) moderationRateLimit -> ResourceExhausted on limit.
    // 5) AdminRepo.SetModerationState(templateID, newStatus, actor.ID, reason).
    // 6) AdminRepo.InsertModerationAction(...) -> moderation_actions row.
    // 7) writeAudit(actor, "template."+actionType, "template", templateID, reason).
    // 8) ReportRepo.AutoResolveItem("template", templateID, actor.ID) (FR-10).
    // 9) notifyModeration(actor, ownerID, templateID, actionType, reason).
    // 10) Return ModerationActionResponse.
}
```

`UnpublishTemplate`: `requireAdminContext`, validate reason ∈ [3,500], then
`applyModerationAction(..., "unpublish", "unpublished_by_admin")`.

`LockTemplate`: `requireAdminContext`, validate reason, then
`applyModerationAction(..., "lock", "locked")`.

`SoftDeleteTemplate`: `requireAdminContext`, validate reason, then
`applyModerationAction(..., "soft_delete", "soft_deleted")` — additionally
sets `deleted_at = now()` (the restore-window start).

`RestoreTemplate`: `requireAdminContext`, validate reason, then:

1. Load the template; missing → `NotFound`.
2. If `template.ModerationStatus == "normal"` → `AlreadyExists` (nothing to
   restore).
3. `AdminRepo.SetModerationState(templateID, "normal", actor.ID, reason)` —
   visibility is preserved (the owner's `visibility` column is untouched).
4. `AdminRepo.MarkModerationActionSuperseded(templateID)` — the prior action
   becomes non-appealable. **No new `moderation_actions` row is inserted for
   restore** — the `action_type` CHECK only allows
   `('unpublish', 'lock', 'soft_delete')`, and the restore is recorded in the
   audit log (`template.restore`).
5. `writeAudit(actor, "template.restore", "template", templateID, reason)`.
6. `notifyModeration(actor, ownerID, templateID, "restore", reason)`.

**Transition guard** (invalid transitions → `409 ALREADY_EXISTS`):

| From \ To | unpublish | lock | soft_delete | restore |
|---|---|---|---|---|
| normal | ✅ | ✅ | ✅ | ❌ |
| locked | ✅ | ❌ | ✅ | ✅ |
| unpublished_by_admin | ✅ | ✅ | ✅ | ✅ |
| soft_deleted | ❌ | ❌ | ❌ | ✅ (within window) |
| in_review / rejected | ❌ | ❌ | ❌ | ❌ |

**Re-publish marker (OQ-5):** when the owner re-publishes an
`unpublished_by_admin` template via the normal `UpdateTemplate` flow, the
service writes an audit event `template.republished_after_moderation`
(target `template`, target_id template id, reason `"owner re-published after
admin unpublish"`). This is a hook in `UpdateTemplate` (prompt.go) that fires
when `visibility` transitions to `public` and `moderation_status` was
`unpublished_by_admin`.

`notifyModeration` (mirrors `notifyReviewDecision`):

```go
func (s *AdminService) notifyModeration(ctx context.Context, actor *models.User,
    ownerID, templateID, actionType, reason string) {
    if s.NotificationRepo == nil { return }
    excerpt := reason
    if len([]rune(excerpt)) > 140 { excerpt = string([]rune(excerpt)[:140]) + "…" }
    if err := s.NotificationRepo.Create(ctx, &models.Notification{
        RecipientID:      ownerID,
        ActorID:          actor.ID,
        TemplateID:       templateID,
        Type:             "moderation",
        ActorDisplayName: actor.DisplayName,
        Snippet:          excerpt,
    }); err != nil {
        zap.S().Errorf("moderation notification: failed: %v", err)
    }
}
```

The `moderation` notification payload carries the action type and reason
excerpt; the deep link points to the template detail (owner view). For
unpublish, the frontend renders a "fix and re-publish" CTA.

### 3.5 `TemplateCascadeService` (`backend/internal/service/template_cascade.go`)

**New** — the shared hard-delete cascade (OQ-4), extracted for reuse by
account-deletion:

```go
type TemplateCascadeService struct {
    db *sql.DB
}

// HardDeleteTemplate permanently removes a template and its owned children
// (versions, aliases, comments on it, likes/favorites/ratings, watches,
// presets). Others' forks survive with "deleted source" lineage (their
// fork_source_id is nulled / marked deleted).
func (s *TemplateCascadeService) HardDeleteTemplate(ctx context.Context, templateID string) error {
    // BEGIN;
    // DELETE FROM template_versions WHERE template_id = $1;
    // DELETE FROM template_aliases WHERE template_id = $1;
    // DELETE FROM template_comments WHERE template_id = $1;
    // DELETE FROM template_likes WHERE template_id = $1;
    // DELETE FROM template_favorites WHERE template_id = $1;
    // DELETE FROM template_ratings WHERE template_id = $1;
    // DELETE FROM template_watches WHERE template_id = $1;
    // DELETE FROM template_presets WHERE template_id = $1;
    // UPDATE templates SET fork_source_id = NULL WHERE fork_source_id = $1;
    // DELETE FROM templates WHERE id = $1;
    // COMMIT;
}
```

### 3.6 Ticker (`backend/internal/service/ticker.go`)

**New** — the 30-day hard-delete due-scan (scheduler-sharing/webhooks pattern):

```go
// scanExpiredSoftDeletes runs on a 24h interval (and on startup, catch-up
// safe). It hard-deletes soft_deleted templates past the restore window.
func (s *TickerService) scanExpiredSoftDeletes(ctx context.Context) error {
    rows, err := s.db.QueryContext(ctx, `
        SELECT id FROM templates
        WHERE moderation_status = 'soft_deleted'
          AND deleted_at < now() - ($1::int * interval '1 day')
        LIMIT 1000`, modSoftDeleteWindowDays)
    // for each id: TemplateCascadeService.HardDeleteTemplate(id)
    // writeAudit(actor=system, "template.soft_delete", "template", id,
    //            "expired soft-delete window; hard-deleted")
}
```

Idempotent: the `WHERE moderation_status = 'soft_deleted'` guard means a
restored template is never picked up; a crash mid-batch is safe to re-run.

### 3.7 Gateway (`backend/cmd/server/main.go`)

Add to the existing `/api/v1/admin/templates/` handler (after the warnings
routes, before the fallthrough `Method not allowed`):

```go
// Moderation actions:
//   POST /api/v1/admin/templates/{id}/unpublish
//   POST /api/v1/admin/templates/{id}/lock
//   POST /api/v1/admin/templates/{id}/delete
//   POST /api/v1/admin/templates/{id}/restore
if len(parts) == 2 && r.Method == http.MethodPost {
    var body struct {
        Reason string `json:"reason"`
    }
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        http.Error(w, "Invalid request body", http.StatusBadRequest)
        return
    }
    var resp *pb.ModerationActionResponse
    var err error
    switch parts[1] {
    case "unpublish":
        resp, err = adminSvc.UnpublishTemplate(ctx, &pb.ModerationActionRequest{TemplateId: templateID, Reason: body.Reason})
    case "lock":
        resp, err = adminSvc.LockTemplate(ctx, &pb.ModerationActionRequest{TemplateId: templateID, Reason: body.Reason})
    case "delete":
        resp, err = adminSvc.SoftDeleteTemplate(ctx, &pb.ModerationActionRequest{TemplateId: templateID, Reason: body.Reason})
    case "restore":
        resp, err = adminSvc.RestoreTemplate(ctx, &pb.ModerationActionRequest{TemplateId: templateID, Reason: body.Reason})
    default:
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }
    if err != nil {
        writeError(w, err)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    b, _ := marshaler.Marshal(resp)
    _, _ = w.Write(b)
    return
}
```

Also add `moderation_status` to the `ListTemplates` service method: parse the
query param, validate against the allowed set, and pass it through
`TemplateFilters.ModerationStatus`.

## 4. Frontend

### 4.1 New Files

| File | Purpose |
|---|---|
| `frontend/src/components/admin/ModerationPanel.js` + `.scss` | Status chip + action buttons + history list on the admin template detail. |
| `frontend/src/components/admin/ActionReasonModal.js` + `.scss` | Consequence checklist + reason textarea (3–500) + optional type-to-confirm (delete). |
| `frontend/src/components/admin/StatusChip.js` + `.scss` | Colored chip per moderation_status. |
| `frontend/src/components/admin/RestoreCountdown.js` + `.scss` | "N days left" on soft-deleted rows. |
| `frontend/src/components/moderation/OwnerModerationBanner.js` + `.scss` | Owner-view banner (unpublished/locked/removed variants). |
| `frontend/src/components/moderation/LockBanner.js` + `.scss` | Public generic lock notice. |
| `frontend/src/components/moderation/RemovedNotice.js` + `.scss` | Owner soft-deleted notice with reason + appeal hint. |

### 4.2 Modified Files

| File | Change |
|---|---|
| `frontend/src/pages/admin/TemplatesPage.js` | Add `moderation_status` filter (AdminSelect) + status chip column + RestoreCountdown on soft-deleted rows. |
| `frontend/src/pages/admin/TemplateDetailPage.js` | Mount `ModerationPanel` (fetches moderation history via `getAdminAuditLogs` filtered by target). |
| `frontend/src/components/admin/ReviewDrawer.js` | "Act → unpublish" calls `unpublishTemplate(id, { reason })` then `resolveReportGroup(..., { resolution: 'actioned', reason })`. |
| `frontend/src/components/notifications/NotificationItem.js` | Render `moderation` type (icon + action + title + reason excerpt + deep link). |
| `frontend/src/pages/TemplateDetails.js` | Mount owner banners / lock banner / removed notice based on `moderation_status`. |
| `frontend/src/services/adminApi.js` | `unpublishTemplate`, `lockTemplate`, `softDeleteTemplate`, `restoreTemplate`. |
| `frontend/src/App.js` | No new routes (moderation lives on existing admin template pages). |

### 4.3 Redux

No new slice: moderation state arrives with API responses; the panel and
modals manage local state.

### 4.4 API Client

```js
unpublishTemplate(id, { reason })  // POST /admin/templates/:id/unpublish
lockTemplate(id, { reason })       // POST /admin/templates/:id/lock
softDeleteTemplate(id, { reason }) // POST /admin/templates/:id/delete
restoreTemplate(id, { reason })    // POST /admin/templates/:id/restore
```

### 4.5 i18n Keys (EN/ZH parity)

```json
{
  "mod": {
    "panel": { "title": "Moderation", "status": "Current status" },
    "status": {
      "normal": "Normal",
      "locked": "Locked",
      "unpublished": "Unpublished by admin",
      "soft_deleted": "Removed"
    },
    "action": {
      "unpublish": "Unpublish",
      "lock": "Lock",
      "delete": "Delete",
      "restore": "Restore"
    },
    "modal": {
      "unpublish": { "title": "Unpublish this template?" },
      "lock": { "title": "Lock this template?" },
      "delete": { "title": "Delete this template?" },
      "restore": { "title": "Restore this template?" },
      "consequence": {
        "public": "Removed from public surfaces (home, search, embeds, API)",
        "ownerKeeps": "The owner keeps access and can edit",
        "frozen": "Editing and new interactions are frozen",
        "notified": "The owner is notified with your reason",
        "reversible": "Reversible: restore any time",
        "window": "Hidden everywhere; 30-day restore window"
      },
      "reason": "Reason (required, visible to the owner)",
      "typeTitle": "Type the template title to confirm",
      "daysLeft": "{{count}} days left"
    },
    "history": { "title": "Moderation history", "empty": "No moderation actions yet" },
    "selfGuard": "You cannot moderate your own template",
    "banner": {
      "unpublished": "An administrator unpublished this template: {{reason}}. You can edit it and share it again.",
      "locked": "This template is locked by moderators. Editing and new interactions are disabled while under review.",
      "removed": "This template was removed by moderators: {{reason}}. If you believe this is a mistake, reply via your notification or contact support.",
      "editCta": "Edit template"
    },
    "lock": {
      "public": "This template is locked by moderators",
      "blocked": "This template is locked by moderators"
    },
    "notif": {
      "unpublish": "Your template \"{{title}}\" was unpublished: {{reason}}",
      "lock": "Your template \"{{title}}\" was locked: {{reason}}",
      "delete": "Your template \"{{title}}\" was removed: {{reason}}",
      "restore": "Your template \"{{title}}\" was restored"
    },
    "filter": { "status": "Status", "empty": "No templates match" },
    "toast": { "actioned": "Template unpublished; report resolved" }
  }
}
```

ZH mirrors every key per the UI/UX doc §8 (`mod.panel.title` = "审核",
`mod.status.normal` = "正常", etc.).

## 5. Error Matrix

| Scenario | HTTP | gRPC | Frontend handling |
|---|---|---|---|
| Reason length violation | 400 | `INVALID_ARGUMENT` | Field-level error; modal unchanged |
| Not authenticated | 401 | `UNAUTHENTICATED` | Redirect to `/login` |
| Non-admin admin route | 403 | `PERMISSION_DENIED` | Admin console error toast |
| Admin moderating own template | 403 | `PERMISSION_DENIED` | Actions disabled with self-guard tooltip |
| Template not found | 404 | `NOT_FOUND` | 404 page |
| Invalid state transition | 409 | `ALREADY_EXISTS` | Toast; panel refreshes |
| Moderation rate limit | 429 | `RESOURCE_EXHAUSTED` | Toast with limit message |
| Redis down | — | — | Fail-open server-side; no user impact |

## 6. Deployment

- `MOD_ACTION_RATE_PER_MINUTE=10` and `MOD_SOFT_DELETE_WINDOW_DAYS=30` added
  to `deployment/docker-compose.yml` backend environment and the Helm chart
  (`values.yaml` `backend.env.modActionRatePerMinute` +
  `backend.env.modSoftDeleteWindowDays` + `templates/deployment-backend.yaml`
  env entries).
- DDL additive/idempotent; both init.sql copies in the same commit.
- No new services, ports, or infrastructure.

## 7. SDK

No python-sdk changes: moderation is an admin-console + web-surface feature.
The REST endpoints are documented in the API surface; the SDK wraps template
resolution only. The SDK's `get_template`/`get_prompt_by_alias` automatically
respect the new visibility rule via the shared repository guard (no SDK code
change).

## 8. Test Strategy

### 8.1 FVT (`tests/fvt.py`) — `test_admin_template_moderation`

Setup: alice (creator), bob (admin), carol (admin). alice owns a public
template.

1. bob unpublishes alice's template with a reason → `moderation_status =
   'unpublished_by_admin'`; anonymous + carol (non-owner) `GET /templates/{id}`
   → 404; alice (owner) sees it with the banner; audit has `template.unpublish`;
   alice gets a `moderation` notification.
2. alice edits and re-shares → `moderation_status` returns to `normal`;
   template public again; audit has `template.republished_after_moderation`.
3. bob locks the template → public viewers see it (200) with the lock banner;
   alice's edit is blocked (403/409); a comment/fork/like attempt is blocked;
   existing comments remain visible.
4. bob soft-deletes (types the title to confirm) → non-owners 404; alice sees
   the removed-notice; admin list shows it under `soft_deleted` with a restore
   countdown.
5. bob restores on day 12 → template returns to previous visibility; audit has
   `template.restore`; the scheduled hard-delete is cancelled (status now
   `normal`).
6. A soft-deleted template past 30 days is hard-deleted by the ticker with
   cascade semantics; another user's fork survives with "deleted source"
   lineage.
7. bob filters `/admin/templates?moderation_status=locked` → only locked
   templates listed, each with moderated_at/by/reason.
8. From a content-reporting queue group, Act → unpublish → the report group
   auto-resolves as `actioned`; the moderation audit entry carries the same
   reason.
9. bob attempts to unpublish his own template → 403; audit log unchanged.
10. Reason < 3 or > 500 chars → 400.
11. Invalid transition (restore a `normal` template) → 409.
12. bob exceeds the moderation rate limit → 429 with `Retry-After`.

### 8.2 Unit

- Repository: `SetModerationState` (status + metadata), `InsertModerationAction`,
  `MarkModerationActionSuperseded`, `buildTemplateConditions` visibility matrix
  (owner bypass, non-owner 404, anonymous, locked stays public).
- Service: validation matrix, self-guard, transition guard, rate-limit counter
  (fake Redis), audit + notification emission, report auto-resolve hook.
- Cascade service: hard-delete cascade + fork lineage preservation.
- Ticker: idempotency, catch-up safety, window boundary.
- Update `MockAdminRepository` with the new methods.

### 8.3 E2E (test-agent, Nightwatch)

- Moderation panel rendering + action modals (reason validation, type-to-confirm
  for delete).
- Status filter on `/admin/templates`; status chips; restore countdown.
- Owner banners (unpublished/locked/removed) + lock-blocked interactions.
- `moderation` notification rendering with reason excerpt + deep link.
- Report queue "Act → unpublish" flow.
- i18n EN/ZH parity for all `mod.*` keys.

## 9. Implementation Order

1. DDL (both init.sql copies) + models + `AdminRepository` methods + unit tests.
2. Proto changes + regenerate.
3. `AdminService` RPCs + self-guard + transition guard + rate limit + audit +
   notification + report auto-resolve.
4. `TemplateRepository` visibility guard + `UpdateTemplate` re-publish marker.
5. `TemplateCascadeService` + ticker.
6. Gateway routes + `ListTemplates` filter.
7. Frontend: api client → components → pages → i18n.
8. FVT + E2E.

## 10. Constraints for Developer

1. **Never commit `backend/server`** — tracked build artifact; `git checkout
   -- backend/server` if modified.
2. Stage only your own files; never `git add -A`.
3. Both init.sql copies in sync in the same commit.
4. `moderation_actions` is **already defined** by moderation-appeals — do NOT
   re-create it; only insert rows. The `appeals` table reads it.
5. The `moderation_status` CHECK widening must be guarded by
   `pg_get_constraintdef` (idempotent) and must preserve the existing
   `in_review`/`rejected` values (pre-publication-review).
6. The visibility guard in `buildTemplateConditions` must keep `locked`
   templates public — only `unpublished_by_admin` and `soft_deleted` (plus the
   existing `in_review`/`rejected`) are hidden from non-owners.
7. Locked templates block ALL mutations in the service layer (edit, comment,
   fork, like/favorite/rating, watch, preset save) — enforce per-mutation, not
   just in the UI.
8. The re-publish marker (`template.republished_after_moderation`) is a hook
   in `UpdateTemplate` (prompt.go), not a new endpoint.
9. Reasons and notification excerpts are plain text — never render as
   HTML/Markdown (XSS-safe by construction, NFR-5).
10. No monetization surfaces of any kind.