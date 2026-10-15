# Content Warnings — Detailed Design

| | |
|---|---|
| **Feature ID** | content-warnings |
| **Version** | 1.0 |
| **Status** | Approved for implementation |
| **Author** | Architect Agent |
| **Upstream** | [Architecture](content-warnings-architecture.md) |

## 1. Proto Contract

### 1.1 `backend/api/proto/v1/prompt.proto`

`Template` field numbers are a coordinated block across designs: 17 =
`comment_count` (shipped), 18/19 reserved by follow-creators, 20–22 by
template-ratings, 23/24 by prompt-analytics, **25/26 taken by
pre-publication-review (`moderation_status`/`published_at`)**. **This feature
takes 27/28 — do not reuse 18–26.**

```protobuf
message Template {
    // ... existing fields 1-17 ...
    // Moderation state: "normal" | "in_review" | "rejected" (pre-publication-review).
    string moderation_status = 25;
    // Approval time of the first review approval (pre-publication-review).
    google.protobuf.Timestamp published_at = 26;
    // Active content warnings (self-declared + moderator-applied), keys from
    // the config-driven taxonomy (mature | violence | profanity | sensitive).
    repeated string content_warnings = 27;
    // The subset of content_warnings applied by moderation; creator-immutable.
    // Only meaningful to the owner and admins.
    repeated string locked_warnings = 28;
}
```

`CreateTemplateRequest` gains `content_warnings` (10) and
`UpdateTemplateRequest` gains `content_warnings` (11):

```protobuf
message CreateTemplateRequest {
    // ... existing fields 1-9 ...
    repeated string content_warnings = 10; // keys from the taxonomy; validated server-side
}

message UpdateTemplateRequest {
    // ... existing fields 1-10 ...
    repeated string content_warnings = 11; // full desired set; locked warnings enforced
}
```

`ListTemplatesRequest` gains `exclude_warned` (12):

```protobuf
message ListTemplatesRequest {
    // ... existing fields 1-11 ...
    bool exclude_warned = 12; // when true, filter out templates with any content warning
}
```

`templateModelToProto` maps both new fields
(`ContentWarnings: m.ContentWarnings`, `LockedWarnings: m.LockedWarnings`).

### 1.2 `backend/api/proto/v1/admin.proto`

`AdminTemplateSummary` gains `content_warnings` (15) and `locked_warnings`
(16) — fields 13/14 are contested by ratings/import-export, so start at 15:

```protobuf
message AdminTemplateSummary {
    // ... existing fields 1-12 ...
    repeated string content_warnings = 15;
    repeated string locked_warnings = 16;
}
```

`ListAdminTemplatesRequest` gains `warning` (7):

```protobuf
message ListAdminTemplatesRequest {
    // ... existing fields 1-6 ...
    string warning = 7; // "" = any | "none" | a taxonomy key (mature|violence|profanity|sensitive)
}
```

Two new RPCs appended to `AdminService` (existing 20 untouched):

```protobuf
service AdminService {
    // ... existing RPCs ...
    rpc UpdateTemplateWarnings(UpdateTemplateWarningsRequest) returns (UpdateTemplateWarningsResponse);
    rpc ListWarningHistory(ListWarningHistoryRequest) returns (ListWarningHistoryResponse);
}

message UpdateTemplateWarningsRequest {
    string template_id = 1;
    repeated string add = 2;    // warnings to add (taxonomy keys)
    repeated string remove = 3; // warnings to remove (taxonomy keys)
    string reason = 4;          // required, 3-500 chars, audit-logged
}

message UpdateTemplateWarningsResponse {
    string template_id = 1;
    repeated string content_warnings = 2; // post-change full set
    repeated string locked_warnings = 3;  // post-change locked set
}

message ListWarningHistoryRequest {
    string template_id = 1;
    int32 page = 2;    // 1-based, default 1
    int32 per_page = 3; // default 20, max 100
}

message WarningHistoryEntry {
    string id = 1;
    string actor_id = 2;
    string actor_email = 3;
    string action = 4; // "template.warning_add" | "template.warning_remove"
    repeated string warnings = 5; // the warnings added/removed in this change
    string reason = 6;
    google.protobuf.Timestamp created_at = 7;
}

message ListWarningHistoryResponse {
    repeated WarningHistoryEntry entries = 1;
    int64 total = 2;
    int32 page = 3;
    int32 per_page = 4;
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
-- Feature: content-warnings
-- Description: config-driven warning taxonomy, per-template warning arrays,
--              moderator lock, audit + notification widenings.
-- -----------------------------------------------------------------------------

-- Warning arrays on templates (OQ-1: two parallel TEXT[] columns; the lock is
-- the intersection). Default empty — no backfill needed for existing rows.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='templates' AND column_name='content_warnings') THEN
        ALTER TABLE templates ADD COLUMN content_warnings TEXT[] NOT NULL DEFAULT '{}';
        COMMENT ON COLUMN templates.content_warnings IS
            'Active content warnings (self-declared + moderator-applied), keys from the config-driven taxonomy';
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='templates' AND column_name='locked_warnings') THEN
        ALTER TABLE templates ADD COLUMN locked_warnings TEXT[] NOT NULL DEFAULT '{}';
        COMMENT ON COLUMN templates.locked_warnings IS
            'Subset of content_warnings applied by moderation; creator-immutable (FR-8)';
    END IF;
END $$;

-- GIN index serves exclude_warned on public lists and the admin warning-state
-- filter (NFR-6).
CREATE INDEX IF NOT EXISTS idx_templates_content_warnings
    ON templates USING GIN(content_warnings);

-- Taxonomy seed (NFR-2: config-driven, admin-editable without deploy).
INSERT INTO platform_settings (key, value) VALUES
    ('content_warning_taxonomy', '["mature","violence","profanity","sensitive"]')
ON CONFLICT (key) DO NOTHING;

-- Widen admin_audit_logs.action CHECK for warning changes (guarded, idempotent).
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'admin_audit_logs'::regclass AND conname = 'admin_audit_logs_action_check';
    IF def IS NOT NULL AND def NOT LIKE '%template.warning_add%' THEN
        ALTER TABLE admin_audit_logs DROP CONSTRAINT admin_audit_logs_action_check;
        ALTER TABLE admin_audit_logs ADD CONSTRAINT admin_audit_logs_action_check
            CHECK (action IN ('ban', 'unban', 'role_change', 'delete_comment',
                              'review_approve', 'review_reject', 'settings_change',
                              'template.warning_add', 'template.warning_remove'));
    END IF;
END $$;

-- Widen notifications.type CHECK for warning-change notifications (guarded).
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'notifications'::regclass AND conname = 'notifications_type_check';
    IF def IS NOT NULL AND def NOT LIKE '%template_warning_changed%' THEN
        ALTER TABLE notifications DROP CONSTRAINT notifications_type_check;
        ALTER TABLE notifications ADD CONSTRAINT notifications_type_check
            CHECK (type IN ('like', 'favorite', 'fork', 'comment',
                            'template_approved', 'template_rejected',
                            'template_warning_changed'));
    END IF;
END $$;
```

## 3. Backend Layers

### 3.1 Models

```go
// models/template.go — add:
ContentWarnings pq.StringArray `json:"content_warnings"` // active warnings (taxonomy keys)
LockedWarnings  pq.StringArray `json:"locked_warnings"`  // moderator-applied subset (creator-immutable)

// models/warning.go — new file:
type WarningHistoryEntry struct {
    ID        string    `json:"id"`
    ActorID   string    `json:"actor_id"`
    ActorEmail string   `json:"actor_email"`
    Action    string    `json:"action"` // "template.warning_add" | "template.warning_remove"
    Warnings  pq.StringArray `json:"warnings"`
    Reason    string    `json:"reason"`
    CreatedAt time.Time `json:"created_at"`
}
```

### 3.2 `TemplateRepository` changes

`Create` INSERT gains `content_warnings`, `locked_warnings` columns; `Update`
SET clause gains both; `Get`/`List`/`CountTemplates` SELECTs gain both (scan
into the new model fields).

**`exclude_warned` filter** in `buildTemplateConditions` (public stream only;
private stream untouched — a private template's warnings are only visible to
its owner):

```sql
-- When exclude_warned is true, add to the public-stream WHERE:
AND NOT (t.content_warnings && ARRAY['mature','violence','profanity','sensitive'])
```

The taxonomy array is passed as a parameter (from the cached settings), never
inlined. `CountTemplates` shares the builder, so counts stay consistent.

**Admin warning-state filter** (in the admin list query, `ListAdminTemplates`):

```sql
-- warning = "" (any): no extra clause
-- warning = "none":  AND (t.content_warnings = '{}' OR t.content_warnings IS NULL)
-- warning = "mature" (or any key): AND (t.content_warnings @> ARRAY[$1])
```

### 3.3 `PromptService` changes

`PromptService` gains a helper to read the taxonomy through the settings
cache:

```go
// taxonomy returns the config-driven warning keys (from platform_settings,
// cached 60s). Falls back to the built-in default on cache miss.
func (s *PromptService) taxonomy(ctx context.Context) []string
```

**`CreateTemplate`** — after validation, before `TemplateRepo.Create`:

```go
// Validate each requested warning against the taxonomy.
for _, w := range req.ContentWarnings {
    if !slices.Contains(taxonomy, w) {
        return nil, status.Errorf(codes.InvalidArgument, "unknown content warning %q", w)
    }
}
template.ContentWarnings = req.ContentWarnings
template.LockedWarnings = nil // a fresh template has no moderator-applied warnings
```

**`UpdateTemplate`** — enforce the creator-immutable lock (FR-8):

```go
// Load the existing template (already fetched for the update).
locked := existing.LockedWarnings
incoming := req.ContentWarnings
// A locked warning must remain present in the incoming set.
for _, lw := range locked {
    if !slices.Contains(incoming, lw) {
        return nil, status.Errorf(codes.PermissionDenied,
            "warning %q was applied by moderation and cannot be removed", lw)
    }
}
// Validate the incoming set against the taxonomy.
for _, w := range incoming {
    if !slices.Contains(taxonomy, w) {
        return nil, status.Errorf(codes.InvalidArgument, "unknown content warning %q", w)
    }
}
template.ContentWarnings = incoming
template.LockedWarnings = locked // preserved; creator cannot change it
```

**`ListTemplates`** — pass `exclude_warned` from the request into the
repository filter.

### 3.4 `AdminService` warning RPCs

`AdminService` gains a `TemplateRepo` reference (or reuses the existing admin
template repository) and a taxonomy helper. Both RPCs start with
`requireAdminContext`.

**`UpdateTemplateWarnings`** (one transaction where possible; notification
and audit are best-effort per existing patterns):

1. Validate `reason` (3–500 chars) → `InvalidArgument` when missing/too short.
2. Validate every key in `add`/`remove` against the taxonomy →
   `InvalidArgument` when unknown.
3. Load the template; compute the new sets:
   - `newContent = (current ∪ add) − remove`
   - `newLocked = (currentLocked ∪ add) − remove` (a moderator can remove a
     warning they or another moderator applied; removing a self-declared
     warning just drops it from `content_warnings`).
4. `UPDATE templates SET content_warnings = $1, locked_warnings = $2 WHERE id = $3`
   (guard `WHERE id = $1`; zero rows → `NotFound`).
5. `writeAudit(ctx, actor, "template.warning_add", "template", templateID,
   reason)` for each added warning, and `"template.warning_remove"` for each
   removed one (or one audit row per change with the warning list in the
   reason — the history list reads these rows).
6. Notification (best-effort, self-suppressed — see 3.5).

**`ListWarningHistory`** — read-only; queries `admin_audit_logs` filtered by
`target_type = 'template'`, `target_id = templateID`, and
`action IN ('template.warning_add','template.warning_remove')`, ordered by
`created_at DESC`, paginated. Maps rows to `WarningHistoryEntry`.

### 3.5 Warning-change notifications

Reuse the `createEngagementNotification` shape in `AdminService` (actor =
deciding admin, `ActorDisplayName` = admin display name):

- Type `template_warning_changed`.
- The notification's `ActorDisplayName` carries the delta sentence payload
  (added/removed warning names) so the frontend can render
  `warnings.notify.changed` without a separate lookup. Concretely, the
  `ActorDisplayName` field is set to a compact JSON `{"added":[...],"removed":[...]}`
  that `NotificationItem` parses into the localized sentence (OQ-5).

Self-decision edge case (an admin changing warnings on their own template):
skip and log a warning — consistent with the existing self-suppression and
the `no_self_notify` CHECK backstop. Failures are logged and swallowed; the
warning change itself is never affected.

### 3.6 Gateway (`backend/cmd/server/main.go`)

Two routes, both behind `requireAdmin`, following the existing
`/api/v1/admin/templates/` TrimPrefix pattern:

```go
// POST   /api/v1/admin/templates/{id}/warnings   (JSON: add, remove, reason)
// DELETE /api/v1/admin/templates/{id}/warnings   (JSON: add, remove, reason)
// GET    /api/v1/admin/templates/{id}/warnings/history
http.HandleFunc("/api/v1/admin/templates/", func(w, r) {
    // existing GET detail handler for len(parts)==1 ...
    // extend the switch:
    rest := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/templates/")
    parts := strings.Split(rest, "/")
    switch {
    case len(parts) == 1 && r.Method == http.MethodGet: // existing GetTemplate
    case len(parts) == 2 && parts[1] == "warnings" && r.Method == http.MethodPost:
        // UpdateTemplateWarnings (add)
    case len(parts) == 2 && parts[1] == "warnings" && r.Method == http.MethodDelete:
        // UpdateTemplateWarnings (remove)
    case len(parts) == 3 && parts[1] == "warnings" && parts[2] == "history" && r.Method == http.MethodGet:
        // ListWarningHistory
    default: 404/405
    }
})
```

The public list handler (`/api/v1/templates`) maps the `exclude_warned` query
param (`== "true"`) into `ListTemplatesRequest.ExcludeWarned`. The admin list
handler (`/api/v1/admin/templates`) maps the `warning` query param into
`ListAdminTemplatesRequest.Warning`.

## 4. Frontend

### 4.1 Routes

No new routes. The admin warning surfaces live inside the existing
`/admin/templates` and `/admin/templates/:id` pages.

### 4.2 New Files

| File | Purpose |
|---|---|
| `frontend/src/components/warnings/WarningCheckboxGroup.js` + `.scss` | Multi-select from the taxonomy; locked warnings render as disabled, checked chips with a lock icon + tooltip. |
| `frontend/src/components/warnings/WarningChip.js` + `.scss` | Small chip with ⚠ icon + localized reason name; tooltip lists all reasons when multiple. |
| `frontend/src/components/warnings/BlurredPreview.js` + `.scss` | Card preview area blurred (CSS `filter: blur(12px)`); chip overlays bottom; title outside blur. |
| `frontend/src/components/warnings/ContentInterstitial.js` + `.scss` | Full-width panel above content: icon, reasons list, Show content / Go back; focus-trapped, Escape = back. |
| `frontend/src/components/warnings/HideWarnedFilterChip.js` + `.scss` | Toggle chip in the filter row; persists per user; URL-synced (`?hideWarned=1`). |
| `frontend/src/components/warnings/ShowWarnedToggle.js` + `.scss` | Settings switch with description; persists server-side. |
| `frontend/src/components/warnings/WarningActionsPanel.js` + `.scss` | Admin panel: current warnings with origin badges, add dropdown, remove buttons, reason input, history list. |

### 4.3 Modified Files

| File | Change |
|---|---|
| `frontend/src/services/api.js` | `createTemplate`/`updateTemplate` accept `content_warnings`; `getTemplates` accepts `exclude_warned`. |
| `frontend/src/services/adminApi.js` | `updateTemplateWarnings(id, {add, remove, reason})`, `getWarningHistory(id, params)`. |
| `frontend/src/components/CreateTemplateModal.js` | Add `WarningCheckboxGroup` to the form; include `content_warnings` in the create payload. |
| `frontend/src/pages/TemplateDetails.js` | Render `ContentInterstitial` before content when the template has warnings and `show_warned_content` is off; edit form gains `WarningCheckboxGroup` with locked display. |
| `frontend/src/components/PromptCard.js` | Render `BlurredPreview` + `WarningChip` when the template has warnings and the viewer hasn't revealed/opted out. |
| `frontend/src/pages/Home.js` | Add `HideWarnedFilterChip` to the filter row; pass `exclude_warned` to the list query; persist per user + URL sync. |
| `frontend/src/pages/Profile.js` | Add a "Content preferences" section with `ShowWarnedToggle`. |
| `frontend/src/pages/admin/TemplatesPage.js` | Add `AdminWarningFilter` dropdown (Any / None / Mature / Violence / Profanity / Sensitive). |
| `frontend/src/pages/admin/TemplateDetailPage.js` | Add `WarningActionsPanel` (current warnings, add/remove with reason, history). |
| `frontend/src/components/notifications/NotificationItem.js` | `EVENT_ICONS` += `template_warning_changed`; parse the delta payload into `warnings.notify.changed`. |
| `frontend/src/locales/{en,zh}/translation.json` | `warnings.*` keys verbatim from UI/UX §8. |

**FR-12 (embeds):** the `template-embedding-widget` surface is a backlog
feature (not yet implemented). This feature makes `ContentInterstitial` a
reusable, self-contained component (it takes the warning reasons and the
reveal callback as props and owns its own `sessionStorage` reveal key). When
the embedding widget ships, it renders `ContentInterstitial` before the embed
content, scoped to its own reveal key so revealing inside an embed does not
reveal the template page elsewhere (AC-10). No embed code changes are part of
this feature.

### 4.4 Redux

No new slice: viewer preferences (`show_warned_content`) are fetched with the
profile and stored in the existing auth/user state; `exclude_warned` is
page-local + persisted via localStorage (logged-in) or URL (anonymous),
matching the existing filter persistence pattern. Reveal state is
`sessionStorage` (page-local, no Redux).

### 4.5 i18n Keys (EN/ZH parity)

`warnings.*` verbatim from UI/UX §8:

```json
{
  "warnings": {
    "group": { "title": "Content warnings (optional)", "help": "Help viewers know what to expect before opening." },
    "type": { "mature": "Mature themes", "violence": "Graphic violence", "profanity": "Strong language", "sensitive": "Sensitive topics" },
    "locked": { "appliedBy": "Applied by moderation on {{date}}: {{reason}}", "label": "Applied by moderation (locked)" },
    "chip": { "multiple": "{{first}} +{{count}} more" },
    "interstitial": { "title": "Content warning", "body": "This template covers: {{reasons}}", "subtext": "You can view it — we just want you to know first.", "show": "Show content", "back": "Go back" },
    "filter": { "hideWarned": "Hide warned content" },
    "settings": { "title": "Content preferences", "showAlways": "Always show warned content", "showAlwaysHelp": "Skip the blur and warning screen for flagged templates." },
    "admin": {
      "filter": { "label": "Warnings", "any": "Any", "none": "None" },
      "panel": { "title": "Content warnings", "current": "Current", "origin": { "creator": "creator", "moderation": "moderation" }, "add": "Add warning", "remove": "Remove", "reason": "Reason (required)", "history": "History", "historyEmpty": "No warning history" }
    },
    "notify": { "changed": "Content warnings on \"{{title}}\" were changed: {{change}}", "added": "added {{warning}}", "removed": "removed {{warning}}" }
  }
}
```

ZH mirrors every key exactly as specified in UI/UX §8 (内容警告（可选）/
帮助观众在打开前了解内容。/ 成人主题 / 血腥暴力 / 强烈语言 / 敏感话题 /
由审核于 {{date}} 施加：{{reason}} / 由审核施加（已锁定）/ {{first}} 等
{{count}} 项 / 内容警告 / 本模板涉及：{{reasons}} / 你可以查看 —— 我们只是
想让你先知情。/ 显示内容 / 返回 / 隐藏警告内容 / 内容偏好 / 始终显示警告
内容 / 跳过已标记模板的模糊与警告页。/ 警告 / 任意 / 无 / 内容警告 / 当前 /
创作者 / 审核 / 添加警告 / 移除 / 原因（必填）/ 历史 / 暂无警告历史 /
你的模板"{{title}}"的内容警告已变更：{{change}} / 已添加 {{warning}} /
已移除 {{warning}})。

## 5. Error Matrix

| Scenario | HTTP | gRPC | Frontend handling |
|---|---|---|---|
| Unknown warning type on create/update | 400 | `INVALID_ARGUMENT` | Inline error; checkbox group reverts |
| Creator removes a locked warning | 403 | `PERMISSION_DENIED` | Inline error; locked chips stay checked |
| Admin add/remove without reason or reason <3 chars | 400 | `INVALID_ARGUMENT` | Apply button pre-disabled; inline error if raced |
| Non-admin on warning admin routes | 401/403 | `UNAUTHENTICATED`/`PERMISSION_DENIED` | Admin login redirect |
| Template id unknown (admin warning change/history) | 404 | `NOT_FOUND` | Not-found page; back to list |
| Notification insert fails | — | — | Logged server-side; warning change unaffected |

## 6. Deployment

- DDL additive/idempotent; both init.sql copies byte-identical in the same
  commit; CHECK widenings guarded by `pg_get_constraintdef`; column adds
  guarded by column-existence checks; taxonomy seed uses
  `ON CONFLICT DO NOTHING`.
- No new services, ports, or infrastructure. No environment variables — the
  taxonomy lives in `platform_settings`.
- Helm chart: no template changes (init.sql reaches the chart via the
  existing ConfigMap); `values.yaml` untouched.
- Never commit `backend/server` (tracked build artifact).

## 7. SDK

No `python-sdk/openprompts` changes: the API returns `content_warnings` as a
plain array on `get_template`/`list_templates`, and clients decide whether to
gate (out of scope — SDK-side gating is explicitly excluded). The admin
warning surface is a web console feature; the SDK wraps the public template
API only.

## 8. Test Strategy

### 8.1 FVT (`tests/fvt.py`) — `test_content_warnings`

Users: `creator` (self-declares), `viewer` (observer), `admin` via
`ADMIN_BOOTSTRAP_*`.

1. **AC-1:** creator creates a template with `content_warnings: ["mature"]`
   → response `Template.content_warnings == ["mature"]`, `locked_warnings`
   empty; GET detail returns the same.
2. **AC-6:** creator updates to remove `mature` (self-declared) → 200,
   `content_warnings` empty.
3. **FR-8/AC-6:** admin adds `violence` with reason → audit log contains
   `template.warning_add` with actor/reason; creator notification contains
   `template_warning_changed`; creator GET shows `locked_warnings ==
   ["violence"]`; creator update removing `violence` → 403.
4. **FR-6/AC-4:** `GET /templates?exclude_warned=true` excludes the warned
   template; without the param it is present.
5. **FR-9/AC-7:** admin list `?warning=none` excludes warned; `?warning=mature`
   shows only mature-warned; `?warning=violence` shows only violence-warned.
6. **FR-2:** unknown warning type on create → 400.
7. **FR-11/AC-9:** a warned template's position in `sort=newest` and
   `sort=most_liked` is identical with and without warnings (create a warned
   and an unwarned template with the same timestamps/likes; assert relative
   order unchanged).
8. **NFR-2:** PUT `/admin/settings` `content_warning_taxonomy` to a custom
   list → a new type is accepted; PUT back to default.
9. **Audit:** `/admin/audit-log?action=template.warning_add` and
   `action=template.warning_remove` contain the changes with actor + reason.

### 8.2 Unit Tests

- Taxonomy validation matrix (valid key, unknown key, empty array).
- Lock-enforcement matrix: locked warning removed → error; locked warning
  kept → ok; self-declared removed → ok; self-declared added → ok.
- `exclude_warned` filter SQL (warned excluded, unwarned kept, private
  stream untouched).
- Admin warning-state filter (`none`/`any`/specific key).
- `UpdateTemplateWarnings` set math (union/difference, locked preservation).
- Audit/notification best-effort (failure logged, change unaffected).
- Update mocks: `MockTemplateRepository` (new columns).

### 8.3 E2E (test-agent, Nightwatch)

- Card blur + warning chip rendering; title readable outside blur.
- Detail interstitial: reasons list, Show content reveals, Go back returns;
  reveal persists across reload in the same session (sessionStorage), a new
  session re-gates.
- `show_warned_content` on → no blur, no interstitial anywhere.
- `exclude_warned` filter: warned templates absent; state survives reload for
  logged-in users; URL `?hideWarned=1` sync.
- Admin panel: add/remove with reason-required (Apply disabled until reason
  ≥3 chars), history list renders, toast on success.
- Creator editor: locked warnings render disabled/checked with lock icon +
  tooltip; removing them is blocked.
- Notification: `template_warning_changed` row renders the delta sentence;
  deep-link to the template.
- i18n EN/ZH parity for all `warnings.*` keys.

## 9. Implementation Order

1. DDL (both init.sql copies) + models + taxonomy seed + unit tests.
2. Proto changes + regenerate + `templateModelToProto` mapping +
   repository SELECT/INSERT/UPDATE columns.
3. `exclude_warned` filter + admin warning-state filter.
4. Taxonomy validation + lock enforcement in `CreateTemplate`/`UpdateTemplate`.
5. Admin warning RPCs (`UpdateTemplateWarnings`, `ListWarningHistory`) +
   notifications + audit.
6. Gateway routes + query params.
7. FVT `test_content_warnings`.
8. Frontend viewer/creator (checkbox group, blur, interstitial, filter chip,
   settings toggle) + i18n en/zh.
9. Frontend admin (warning filter, actions panel) + NotificationItem.
10. Component tests + handoff to test-agent for e2e.

## 10. Constraints for Developer

1. **Never commit `backend/server`** — tracked build artifact; `git
   checkout -- backend/server` if modified.
2. Stage only your own files; never `git add -A` (the tree holds other
   agents' WIP).
3. Both init.sql copies byte-identical in the same commit; every DDL block
   idempotent (column-existence guards, `pg_get_constraintdef` for CHECK
   widenings, `ON CONFLICT DO NOTHING` for the taxonomy seed).
4. **`Template` proto field numbers 27/28 only.** 18–26 are reserved/used by
   follow-creators (18/19), template-ratings (20–22), prompt-analytics
   (23/24), and pre-publication-review (25/26) — never reuse them. Do not
   touch `AdminTemplateSummary` 13/14 (contested by ratings/import-export)
   — start new admin fields at 15.
5. **Do NOT overload `templates.moderation_status`** for warning state — it
   is `('normal','in_review','rejected')` and belongs to pre-publication
   review. Warnings live in `content_warnings`/`locked_warnings` only.
6. The lock rule is `locked_warnings ⊆ content_warnings`; the creator can
   never remove a locked warning (403). The moderator can remove any warning
   (locked or self-declared) with a reason.
7. `exclude_warned` applies to the public stream only; private streams are
   untouched. The taxonomy array is passed as a parameter, never inlined.
8. Warning types are validated against the server-side taxonomy whitelist —
   never trust the client's strings.
9. `pq.StringArray` for both warning columns; default empty arrays; no
   backfill needed.
10. No monetization surfaces of any kind.