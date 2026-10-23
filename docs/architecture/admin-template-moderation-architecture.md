# Admin Template Moderation — Architecture Design

| | |
|---|---|
| **Feature ID** | admin-template-moderation |
| **Version** | 1.0 |
| **Status** | Approved for implementation |
| **Author** | Architect Agent |
| **Upstream** | [Requirements](../design/admin-template-moderation-requirements.md), [UI/UX Design](../design/admin-template-moderation-ui-ux-design.md) |
| **Downstream** | [Detailed Design](admin-template-moderation-detailed-design.md) |

## 1. Overview

Admin template moderation gives the platform its **graduated, auditable
enforcement** on templates. Today an admin who finds a policy-violating
template can only look at it (the admin console's template pages are read-only
by design) or ban the owner (nuclear). This feature adds three **state
changes, not deletions** — **Unpublish** (public → private, reversible,
reason-required, creator notified), **Lock** (freeze edits + new comments/
forks/likes/ratings/watches while content stays public), and **Soft delete**
(hidden everywhere, 30-day restore window, then hard-delete cascade). All
three write to the existing `admin_audit_logs` with actor, target, reason, and
timestamp, and notify the creator via a new `moderation` notification type.

The feature reuses established patterns: the `moderation_status` column
(introduced by pre-publication-review with a narrower CHECK that this feature
widens), the `writeAudit` + `requireAdminContext` admin-service shape, the
`createEngagementNotification` best-effort notification hook, the
`buildTemplateConditions` banned-owner hiding pattern (extended to moderation
status with an owner bypass), the `moderation_actions` table (defined by
moderation-appeals, **populated here**), the reason-modal + type-to-confirm
pattern (from admin-user-management / account-deletion), and the
scheduler-sharing/webhooks DB-backed due-scan ticker pattern for the 30-day
hard-delete.

This feature also **closes the content-reporting loop**: the report queue's
"Act → unpublish template" flow (content-reporting FR-15) currently only
resolves the report group as `actioned` without actually unpublishing — this
feature provides the real unpublish endpoint that flow calls, and the
auto-resolve hook (content-reporting FR-12) fires in the same transaction.

### 1.1 Resolved Open Questions

| # | Question (from requirements §10) | Decision |
|---|---|---|
| 1 | Access-check composition: single guard vs per-endpoint | **Single repository-layer guard.** Extend the existing `buildTemplateConditions` (the one place every public read path flows through) to hide `unpublished_by_admin` and `soft_deleted` templates from non-owners, mirroring the banned-owner pattern. The owner bypass (`t.owner_id = $1`) already exists. This covers home, search, category, related rails, embeds, QR-share, following feeds, and the SDK's `get_template`/`get_prompt_by_alias` in one place — no per-endpoint risk of leaks. |
| 2 | Column vs separate table for current state | **Column on `templates.moderation_status`** (hot-path read, one extra column in the WHERE clause). History comes from `admin_audit_logs` (immutable) plus the `moderation_actions` table (mutable lifecycle for appeals). No separate current-state table. |
| 3 | Lock interplay with owner edit | **Block both edits and re-publish** (true freeze, FR-3). The owner may still view the template and its content; the lock banner explains why. This is the simplest correct semantics and matches the UI/UX "Editing and new interactions are disabled while under review." |
| 4 | Hard-delete reuse | **Extract a shared template-cascade service.** The 30-day expiry calls the same cascade routine that account-deletion defines (versions, aliases, comments on it go; others' forks survive with "deleted source" lineage). The routine is extracted into a reusable `TemplateCascadeService` so both account-deletion and this feature share it. |
| 5 | Re-publish without admin review (FR-2) | **Accept (option a) with an audit trail marker.** The owner can immediately re-publish after unpublish; a malicious owner could re-publish unchanged. Mitigation is (a) accept for v1 simplicity — the admin can re-unpublish, and repeated violations escalate to a ban — plus a new audit event `template.republished_after_moderation` written when a moderated template is re-published, so the pattern is visible in the audit trail. |

### 1.2 Non-Goals

- Bulk moderation actions (multi-template unpublish/delete) — explicitly out
  of scope (FR-15).
- Pre-publication review queue (submit → approve) — separate feature
  (pre-publication-review).
- Creator appeal workflow (in-app appeal form, case states) — separate feature
  (moderation-appeals); v1 is notify + out-of-band contact.
- Admin editing template content (only state changes; content editing stays
  with the owner).
- Age-restriction / content-warning overlays — separate feature
  (content-warnings).
- Shadow-banning or graduated visibility (search-delisting only).
- **Monetization of any kind** — payments, pricing, purchases, earnings,
  sales, or paid-ranking surfaces are out of scope and excluded from this
  architecture.

## 2. System Context

```mermaid
flowchart LR
    subgraph Client
        FE[React Frontend<br/>OwnerModerationBanner / LockBanner / RemovedNotice]
        AD[Admin Console<br/>TemplatesPage status filter / TemplateDetailPage ModerationPanel]
    end
    subgraph Gateway
        GW[REST Gateway :8080<br/>/api/v1/admin/templates/{id}/unpublish|lock|delete|restore]
    end
    subgraph Backend
        AS[AdminService<br/>UnpublishTemplate / LockTemplate / SoftDeleteTemplate / RestoreTemplate]
        TR[TemplateRepository<br/>moderation filters + access guard]
        AR[AdminRepository<br/>SetTemplateModeration / audit / moderation_actions insert]
        RR[ReportRepository<br/>AutoResolveItem hook]
        NR[NotificationRepository<br/>moderation notifications]
        TC[TemplateCascadeService<br/>30-day hard-delete]
    end
    subgraph Stores
        PG[(PostgreSQL)]
        RD[(Redis<br/>mod_action_rate:{admin} counter)]
    end
    FE -->|owner view| GW
    AD -->|POST /api/v1/admin/templates/{id}/...| GW --> AS
    AS --> TR --> PG
    AS --> AR --> PG
    AS --> RR --> PG
    AS --> NR
    TC -->|ticker scan| PG
    AS -->|mod_action_rate:{admin}| RD
```

## 3. Components Involved

| Component | Change |
|---|---|
| `backend/api/proto/v1/admin.proto` | 4 new RPCs on `AdminService`: `UnpublishTemplate`, `LockTemplate`, `SoftDeleteTemplate`, `RestoreTemplate`; new request/response messages; `moderation_status`/`moderated_at`/`moderated_by`/`moderation_reason` fields on `AdminTemplateSummary`; `moderation_status` filter on `ListAdminTemplatesRequest`. |
| `backend/internal/service/admin.go` | `UnpublishTemplate`/`LockTemplate`/`SoftDeleteTemplate`/`RestoreTemplate` implementations (self-guard, reason validation, audit, notification, moderation_actions insert, report auto-resolve hook). |
| `backend/internal/repository/admin_repository.go` | `SetTemplateModeration` widened to the new statuses; new `InsertModerationAction`; `ListAuditByTarget` reused for the moderation history. |
| `backend/internal/repository/template_repository.go` | `buildTemplateConditions` extended to hide `unpublished_by_admin`/`soft_deleted` from non-owners (owner bypass already present). |
| `backend/internal/repository/report_repository.go` | `AutoResolveItem` hook called from the unpublish/soft-delete paths (already defined by content-reporting). |
| `backend/internal/service/template_cascade.go` | **New** — shared hard-delete cascade service (extracted for account-deletion reuse). |
| `backend/internal/service/ticker.go` | **New** — 30-day hard-delete due-scan ticker (scheduler-sharing/webhooks pattern). |
| `backend/cmd/server/main.go` | 4 admin routes under `/api/v1/admin/templates/{id}/...` (all `requireAdmin`). |
| `frontend/src/pages/admin/TemplatesPage.js` | Status filter (public/private/locked/unpublished/soft-deleted) + status chip column + restore countdown. |
| `frontend/src/pages/admin/TemplateDetailPage.js` | **ModerationPanel** (status chip, actions, history). |
| `frontend/src/components/admin/ModerationPanel.js`, `ActionReasonModal.js`, `StatusChip.js`, `RestoreCountdown.js` | **New.** |
| `frontend/src/components/moderation/*` | **New** — `OwnerModerationBanner`, `LockBanner`, `RemovedNotice`. |
| `frontend/src/components/notifications/NotificationItem.js` | Render the `moderation` notification type. |
| `frontend/src/components/admin/ReviewDrawer.js` | "Act → unpublish" now calls the real unpublish endpoint (FR-10). |
| `frontend/src/locales/{en,zh}/translation.json` | `mod.*` namespace (verbatim from UI/UX §8). |
| `deployment/init.sql` (+ chart copy) | Widen `templates_moderation_status_check`; add `moderated_at`/`moderated_by`/`moderation_reason` columns; widen audit + notification CHECKs. |

## 4. API Design

### 4.1 Admin Moderation APIs

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/api/v1/admin/templates/{id}/unpublish` | Admin | Body `{reason}`. Sets `moderation_status = 'unpublished_by_admin'`. |
| `POST` | `/api/v1/admin/templates/{id}/lock` | Admin | Body `{reason}`. Sets `moderation_status = 'locked'`. |
| `POST` | `/api/v1/admin/templates/{id}/delete` | Admin | Body `{reason}`. Sets `moderation_status = 'soft_deleted'`, `deleted_at = now()`. |
| `POST` | `/api/v1/admin/templates/{id}/restore` | Admin | Body `{reason}`. Restores to `normal` (visibility preserved). |

Rules:

- **Non-admin** → `403 PERMISSION_DENIED`.
- **Reason outside 3–500 chars** → `400 INVALID_ARGUMENT`.
- **Admin moderating their own template** → `403 PERMISSION_DENIED` (FR-14).
- **Template not found** → `404 NOT_FOUND`.
- **Invalid transition** (e.g. unpublish a soft-deleted template, restore a
  `normal` template) → `409 ALREADY_EXISTS`.
- **Rate limit exceeded** (write tier, FR-15) → `429 RESOURCE_EXHAUSTED` with
  `Retry-After`.

### 4.2 Admin List Filter

`GET /api/v1/admin/templates` gains a `moderation_status` query param
(`"" | normal | locked | unpublished_by_admin | soft_deleted`). Response items
carry `moderation_status`, `moderated_at`, `moderated_by`,
`moderation_reason` (admin-only fields).

### 4.3 Error Codes

| HTTP | gRPC | Condition |
|---|---|---|
| 400 | `INVALID_ARGUMENT` | Reason length; invalid moderation_status filter. |
| 401 | `UNAUTHENTICATED` | Missing/invalid token. |
| 403 | `PERMISSION_DENIED` | Non-admin; admin moderating own template. |
| 404 | `NOT_FOUND` | Template not found. |
| 409 | `ALREADY_EXISTS` | Invalid state transition. |
| 429 | `RESOURCE_EXHAUSTED` | Moderation rate limit (`Retry-After`). |

## 5. Data Model

### 5.1 `templates` additions

```sql
-- Widen the moderation_status CHECK (guarded, idempotent).
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
```

### 5.2 `moderation_actions` (populated here)

The `moderation_actions` table is **defined by moderation-appeals** (already in
`deployment/init.sql`). This feature **populates** it: every unpublish/lock/
soft-delete inserts a row with `action_type`, `actor_id`, `actor_email`,
`reason`, `status = 'active'`. Restore does **not** insert a row (the
`action_type` CHECK only allows `('unpublish', 'lock', 'soft_delete')`); it
marks the prior action `superseded` (so appeals against it become
non-appealable, per moderation-appeals FR-14) and records the restore in the
audit log (`template.restore`).

### 5.3 CHECK Widenings (idempotent, both init.sql copies)

- `admin_audit_logs.action` gains `template.unpublish`, `template.lock`,
  `template.soft_delete`, `template.restore`,
  `template.republished_after_moderation`.
- `notifications.type` gains `moderation`.

## 6. Auth & Authorization

- All moderation routes sit behind `requireAdmin` (gateway) and
  `requireAdminContext` (service) — role=admin AND scope=admin, identical to
  the existing admin console endpoints.
- **Self-guard (FR-14):** an admin cannot moderate their own template — the
  service checks `template.OwnerID == actor.ID` → `403 PERMISSION_DENIED`.
- **Attribution:** every action writes an audit row (actor id/email, action,
  target `template`/template id, reason) and a `moderation_actions` row —
  satisfying NFR-2.
- **Transparency (NFR-3):** moderation reasons are shown to the template owner
  and admins only; the public lock banner shows a generic message, never the
  reason.

## 7. Frontend

- **Admin console:**
  - `TemplatesPage` gains a **status filter** (`moderation_status`) and a
    status chip column; soft-deleted rows show a `RestoreCountdown`
    ("N days left").
  - `TemplateDetailPage` gains a **ModerationPanel** (status chip, the three
    actions with consequence summaries, restore when applicable, and the
    moderation history from audit logs filtered by target).
  - `ActionReasonModal` (reason 3–500 chars + consequence checklist; delete
    adds type-to-confirm of the template title).
  - `ReviewDrawer` "Act → unpublish" now calls the real unpublish endpoint
    (FR-10) and auto-resolves the report group.
- **Creator surfaces:**
  - `OwnerModerationBanner` (unpublished: reason + "Edit template" CTA),
    `LockBanner` (generic, public), `RemovedNotice` (soft-deleted: reason +
    appeal hint).
  - `NotificationItem` renders the `moderation` type with action, title,
    reason excerpt, and (for unpublish) a "fix and re-publish" deep link.
- **i18n:** `mod.*` keys verbatim from UI/UX §8, EN/ZH parity.

## 8. Configuration

| Setting | Default | Description |
|---|---|---|
| `MOD_ACTION_RATE_PER_MINUTE` | `10` | Per-admin moderation write rate limit (FR-15), server-side env. |
| `MOD_SOFT_DELETE_WINDOW_DAYS` | `30` | Restore window before hard-delete (FR-4), server-side env. |

Both are server-side env vars (mirroring `APPEAL_MAX_OPEN`); no
`platform_settings` rows needed in v1.

## 9. Security & Privacy Considerations

- **No status leaks:** `unpublished_by_admin` and `soft_deleted` templates
  404 for non-owners on every path (detail, list, search, profile, fork
  source, comments, embeds, QR-share, SDK) — all flow through the same
  `buildTemplateConditions` visibility rule.
- **Locked templates stay public** but all mutations are blocked in the
  service layer (edit, comment, fork, like/favorite/rating, watch, preset
  save) — enforced per-mutation, not just in the UI.
- **Auditability:** every action, restore, and the scheduled hard-delete
  writes an immutable audit row; the moderation history on the detail page is
  complete.
- **Injection:** parameterized SQL only; reason length validated server-side;
  filter values never reach SQL text.
- **Transparency (NFR-4):** every action notifies the creator with the reason;
  the owner always sees why their content was actioned.

## 10. Deployment Notes

- DDL is additive and idempotent; both `deployment/init.sql` and
  `deployment/chart/open-prompts/init.sql` updated byte-identically in the
  same commit. CHECK widenings use the `pg_get_constraintdef` guard.
- No new services, ports, or infrastructure; no chart changes beyond the
  init.sql ConfigMap content.
- `backend/server` is a tracked build artifact — never commit changes to it.

## 11. Test Strategy

- **FVT** (`tests/fvt.py`): `test_admin_template_moderation` — unpublish →
  hidden from public surfaces + owner banner + audit + notification; lock →
  public visible + mutations blocked; soft-delete → 404 for non-owners +
  removed-notice + restore countdown; restore within window → previous
  visibility + audit; self-guard 403; reason validation; report auto-resolve
  on unpublish; rate limit 429. Covers AC-1..10.
- **Unit:** access-guard matrix (owner bypass, non-owner 404, anonymous),
  state-transition guards, cascade service, ticker idempotency, service
  validation matrix.
- **E2E** (test-agent): moderation panel rendering, action modals, status
  filter, owner banners, lock-blocked interactions, notification rendering,
  i18n EN/ZH parity.

## 12. Performance & Caching

- **NFR-1 (500ms p95 @ 1k open):** unpublish/lock take effect immediately on
  read paths — the moderation status is one extra column in the existing
  `buildTemplateConditions` WHERE clause; no cache invalidation beyond
  existing TTLs.
- **NFR-2 (Auditability):** every action writes an audit row; the moderation
  history query reuses `ListAuditByTarget` (indexed by target_id).
- **NFR-6 (10k open):** the admin list filter adds a `moderation_status`
  equality clause served by the existing `idx_templates_moderation_status`
  index; pagination unchanged.
- **Ticker:** the 30-day hard-delete scan is a single indexed query
  (`WHERE moderation_status = 'soft_deleted' AND deleted_at < now() -
  interval '30 days'`), idempotent and catch-up safe.