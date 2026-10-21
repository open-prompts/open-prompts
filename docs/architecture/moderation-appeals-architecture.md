# Moderation Appeals — Architecture Design

| | |
|---|---|
| **Feature ID** | moderation-appeals |
| **Version** | 1.0 |
| **Status** | Approved for implementation |
| **Author** | Architect Agent |
| **Upstream** | [Requirements](../design/moderation-appeals-requirements.md), [UI/UX Design](../design/moderation-appeals-ui-ux-design.md) |
| **Downstream** | [Detailed Design](moderation-appeals-detailed-design.md) |

## 1. Overview

Moderation appeals close the loop that `admin-template-moderation` left
out-of-band: a creator whose template was actioned (unpublished, locked, or
soft-deleted) can contest the decision in-product with a structured, tracked
appeal, and an admin can triage a FIFO queue and reverse or uphold with a
required resolution note. The design follows the YouTube model — **one appeal
per moderation action**, a single case statement (no threaded dialogue), and
explicit states (`open` → `under_review` → `resolved_upheld` |
`resolved_reversed`).

The feature introduces a dedicated **`moderation_actions`** table (OQ-1) that
records every moderation action with a stable id. Appeals reference these
action ids, which gives a clean lifecycle for the FR-14 "superseded" state
(an action undone by a later direct admin restore can no longer be appealed).
Because `admin-template-moderation` is a backlog feature not yet implemented,
this design defines the `moderation_actions` table and the write path that
`admin-template-moderation` will populate when it lands; the appeals feature
itself only reads it. The reversal remedy (FR-7) reuses the restore semantics
that `admin-template-moderation` FR-5 defines.

### 1.1 Resolved Open Questions

| # | Question (from requirements §10) | Decision |
|---|---|---|
| 1 | Reference audit-log entry ids vs a dedicated `moderation_actions` table | **Dedicated `moderation_actions` table.** Audit entries are append-only and immutable; appeals need a mutable lifecycle (`active` → `superseded`/`reversed`) that audit rows cannot express. The table is defined here and populated by `admin-template-moderation` when it lands. |
| 2 | Is `under_review` required before resolve? | **No — an admin may resolve directly from `open`.** Two states is ceremony for a small team; the `under_review` transition remains available as an optional triage step (FR-6). |
| 3 | Distinct audit code for appeal reversal vs reusing `template.restore` | **Reuse `template.restore` with an appeal reference in the reason.** The restore path is identical; a distinct code would duplicate the remedy. The audit reason carries `appeal=<id>` so the reversal is traceable. |
| 4 | Is the 5-open-appeals limit configurable? | **Yes — server-side env `APPEAL_MAX_OPEN` (default 5).** Mirrors the `RATING_RATE_PER_MINUTE` config pattern; no platform_settings row needed in v1. |
| 5 | Notification note excerpt length | **140-char excerpt in the notification, plus a deep link to the appeal detail.** The excerpt gives immediate context; the deep link is the source of truth. |

### 1.2 Non-Goals

- Multi-round dialogue / threaded back-and-forth (one statement + one note).
- Attachments / evidence uploads; third-party arbitrators; SLA timers or
  auto-escalation.
- Appeals of account-level actions (bans — `admin-user-management` territory).
- Appeals of comment deletions (template actions only).
- Admin moderation actions themselves (unpublish/lock/soft-delete) — those
  belong to `admin-template-moderation`; this feature only reads them.
- **Monetization of any kind** — payments, pricing, purchases, earnings,
  sales, or paid-ranking surfaces are out of scope and excluded from this
  architecture.

## 2. System Context

```mermaid
flowchart LR
    subgraph Client
        FE[React Frontend<br/>AppealCtaSlot / AppealFormModal / UserAppealList]
        ADM[Admin Console<br/>AdminAppealQueue / AdminAppealDetail]
    end
    subgraph Gateway
        GW[REST Gateway :8080<br/>/api/v1/appeals... /api/v1/admin/appeals...]
    end
    subgraph Backend
        PS[PromptService<br/>CreateAppeal / ListAppeals / GetAppeal]
        AS[AdminService<br/>ListAppealQueue / ReviewAppeal / ResolveAppeal]
        AR[AppealRepository<br/>appeals + moderation_actions reads]
        NR[NotificationRepository<br/>appeal_resolved]
    end
    subgraph Stores
        PG[(PostgreSQL)]
        RD[(Redis<br/>open-appeal rate counter)]
    end
    FE -->|POST/GET /api/v1/appeals| GW --> PS
    ADM -->|GET/POST /api/v1/admin/appeals| GW --> AS
    PS --> AR --> PG
    AS --> AR --> PG
    AS --> NR --> PG
    PS -->|appeal_open:{user}| RD
```

## 3. Components Involved

| Component | Change |
|---|---|
| `backend/api/proto/v1/prompt.proto` | `CreateAppeal`/`ListAppeals`/`GetAppeal` RPCs on `PromptService`; `Appeal` message. |
| `backend/api/proto/v1/admin.proto` | `ListAppealQueue`/`GetAppeal`/`ReviewAppeal`/`ResolveAppeal` RPCs on `AdminService`; `AdminAppealEntry`/`AdminAppealDetail` messages. |
| `backend/internal/models/appeal.go` | **New** — `ModerationAction` and `Appeal` GORM models. |
| `backend/internal/repository/appeal_repository.go` | **New** — appeal CRUD, queue queries, moderation-action reads, atomic resolve+restore. |
| `backend/internal/service/prompt.go` | `CreateAppeal`/`ListAppeals`/`GetAppeal` implementations (owner + visibility checks, rate limit). |
| `backend/internal/service/admin.go` | `ListAppealQueue`/`GetAppeal`/`ReviewAppeal`/`ResolveAppeal` implementations (self-guard, audit, notification). |
| `backend/cmd/server/main.go` | `/api/v1/appeals` and `/api/v1/appeals/{id}` branches; `/api/v1/admin/appeals` and `/api/v1/admin/appeals/{id}` branches. |
| `frontend/src/components/AppealCtaSlot.js`, `AppealFormModal.js`, `AppealStateChip.js` | **New.** |
| `frontend/src/pages/AppealsPage.js`, `frontend/src/pages/AppealDetailPage.js` | **New** user routes. |
| `frontend/src/pages/admin/AppealsPage.js`, `frontend/src/pages/admin/AppealDetailPage.js` | **New** admin routes. |
| `frontend/src/services/api.js` | Appeal API client methods. |
| `frontend/src/locales/{en,zh}/translation.json` | `appeal.*` namespace. |
| `deployment/init.sql` (+ chart copy) | `moderation_actions` + `appeals` tables, audit/notification CHECK widenings. |

## 4. API Design

### 4.1 User Appeal APIs

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/api/v1/appeals` | Required | Body `{moderation_action_id, statement}`. Creates an `open` appeal. Returns the created appeal. |
| `GET` | `/api/v1/appeals` | Required | The caller's own appeals, paginated 20/page, newest first. |
| `GET` | `/api/v1/appeals/{id}` | Required | A single appeal (owner-only). |

Rules:

- **Not authenticated** → `401 UNAUTHENTICATED`.
- **Action not found / not the caller's template** → `404 NOT_FOUND` (the
  action's existence is not revealed to non-owners).
- **Action already appealed** → `409 ALREADY_EXISTS` ("This action has already
  been appealed").
- **Action superseded** (undone by a later direct admin restore) → `409
  ALREADY_EXISTS` with a "superseded" message; the UI hides the entry point.
- **Statement outside 50–2000 chars** → `400 INVALID_ARGUMENT`.
- **Open-appeal limit reached** (default 5) → `429 RESOURCE_EXHAUSTED` with
  `Retry-After` and the limit message.
- **Appeal not owned by caller** → `404 NOT_FOUND` (no existence leak).

### 4.2 Admin Appeal APIs

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/v1/admin/appeals?state=open\|under_review\|resolved&page=...` | Admin | Queue, oldest-first within state, with per-state counts. |
| `GET` | `/api/v1/admin/appeals/{id}` | Admin | Appeal detail + the original action's audit context. |
| `POST` | `/api/v1/admin/appeals/{id}/review` | Admin | `open` → `under_review`. |
| `POST` | `/api/v1/admin/appeals/{id}/resolve` | Admin | Body `{outcome: upheld\|reversed, note}`. Resolves the appeal; reversed restores the template atomically. |

Rules:

- **Non-admin** → `403 PERMISSION_DENIED`.
- **Resolver is the original action's actor** → `403 PERMISSION_DENIED`
  ("The original action's actor cannot resolve this appeal").
- **Note outside 20–1000 chars** → `400 INVALID_ARGUMENT`.
- **Appeal already resolved** → `409 ALREADY_EXISTS`.
- **Resolve on a superseded action** → `409 ALREADY_EXISTS` (the action was
  already undone; nothing to reverse).

### 4.3 Error Codes

| HTTP | gRPC | Condition |
|---|---|---|
| 400 | `INVALID_ARGUMENT` | Statement/note length violation; invalid outcome. |
| 401 | `UNAUTHENTICATED` | Missing/invalid token. |
| 403 | `PERMISSION_DENIED` | Non-admin; resolver is the original actor. |
| 404 | `NOT_FOUND` | Action/appeal not found or not owned. |
| 409 | `ALREADY_EXISTS` | Action already appealed; appeal already resolved; superseded action. |
| 429 | `RESOURCE_EXHAUSTED` | Open-appeal limit reached (`Retry-After`). |

## 5. Data Model

### 5.1 New Table: `moderation_actions`

```sql
CREATE TABLE IF NOT EXISTS moderation_actions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    action_type TEXT NOT NULL CHECK (action_type IN ('unpublish', 'lock', 'soft_delete')),
    actor_id TEXT NOT NULL,
    actor_email TEXT NOT NULL,
    reason TEXT NOT NULL CHECK (char_length(reason) BETWEEN 3 AND 500),
    status TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'superseded', 'reversed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    superseded_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_moderation_actions_template
    ON moderation_actions (template_id);
CREATE INDEX IF NOT EXISTS idx_moderation_actions_status_created
    ON moderation_actions (status, created_at);
```

`status` lifecycle: `active` (appealable) → `superseded` (undone by a later
direct admin restore, no longer appealable) or `reversed` (undone by an appeal
resolution). `admin-template-moderation` writes rows here on unpublish/lock/
soft-delete and flips `active` → `superseded` on a direct restore.

### 5.2 New Table: `appeals`

```sql
CREATE TABLE IF NOT EXISTS appeals (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    moderation_action_id UUID NOT NULL REFERENCES moderation_actions(id) ON DELETE CASCADE,
    appellant_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    statement TEXT NOT NULL CHECK (char_length(statement) BETWEEN 50 AND 2000),
    state TEXT NOT NULL DEFAULT 'open'
        CHECK (state IN ('open', 'under_review', 'resolved_upheld', 'resolved_reversed')),
    resolver_id TEXT,
    resolution_note TEXT CHECK (resolution_note IS NULL OR char_length(resolution_note) BETWEEN 20 AND 1000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    UNIQUE (moderation_action_id)
);
CREATE INDEX IF NOT EXISTS idx_appeals_appellant_created
    ON appeals (appellant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_appeals_state_created
    ON appeals (state, created_at);
```

`UNIQUE (moderation_action_id)` enforces FR-3 (one appeal per action) at the
database level. `state` transitions are admin-only except the initial create.

### 5.3 CHECK Widenings (idempotent, both init.sql copies)

- `admin_audit_logs.action` gains `appeal.create`, `appeal.review`,
  `appeal.resolve`.
- `notifications.type` gains `appeal_resolved`.

## 6. Auth & Authorization

- **User writes** require session/API-key auth; user id from context, never
  the body. The appellant must own the template the action targeted.
- **Admin writes** require the admin JWT (`requireAdminContext`). The
  resolver must not be the original action's actor (FR-9, self-guard).
- **Visibility**: appeals and actions are never revealed to non-owners /
  non-admins (404 on foreign resources).
- **Rate limiting**: open-appeal creation limited to `APPEAL_MAX_OPEN` (5)
  simultaneous open appeals per user, enforced via a Redis counter
  (`appeal_open:{user_id}`) that decrements on resolution. Fail-open on Redis
  errors.

## 7. Frontend

- **AppealCtaSlot** (owner banner / removed-notice): CTA when no appeal,
  status chip when open/under_review, outcome chip + note excerpt when
  resolved; hidden when the action is superseded.
- **AppealFormModal**: read-only action context + statement textarea
  (50–2000) with live counter; 409 → replaced by the existing appeal's status
  card.
- **AppealStateChip**: open / under review / upheld / reversed with text
  labels.
- **AppealsPage** (`/appeals`): the caller's appeals with state chips, action
  context, timestamps; resolution note excerpt when resolved.
- **AppealDetailPage** (`/appeals/:id`): statement + action context +
  resolution block.
- **AdminAppealsPage** (`/admin/appeals`): state filter tabs with counts,
  oldest-first rows, statement excerpt.
- **AdminAppealDetailPage** (`/admin/appeals/:id`): two-pane appeal + audit
  entry; Review button; Resolve form (outcome radio + required note) with
  self-guard disabled state.
- **i18n**: `appeal.*` namespace; EN/ZH parity per the UI/UX doc §8.

## 8. Configuration

| Variable | Default | Description |
|---|---|---|
| `APPEAL_MAX_OPEN` | `5` | Max simultaneous open appeals per user. |

Added to `deployment/docker-compose.yml` and the Helm chart
(`values.yaml` + `templates/deployment-backend.yaml`), mirroring
`RATING_RATE_PER_MINUTE`.

## 9. Security Considerations

- **Privacy**: appeals and actions are owner/admin-only; no existence leak to
  others (404).
- **Integrity**: `UNIQUE (moderation_action_id)` enforces one-appeal-per-action;
  resolve+restore is atomic in one transaction; state transitions validated
  server-side.
- **Abuse**: open-appeal rate limit; self-guard prevents self-review; plain-text
  statements/notes (no HTML/Markdown) rendered as text (XSS-safe by
  construction, NFR-5).
- **Injection**: parameterized SQL only.

## 10. Deployment Notes

- DDL is additive and idempotent; both init.sql copies updated in the same
  commit.
- No new services or infrastructure; one new Redis counter key.
- `backend/server` is a tracked build artifact — never commit changes to it.

## 11. Test Strategy

- **FVT** (`tests/fvt.py`): `test_moderation_appeals` — create/list/get,
  one-appeal-per-action 409, superseded 409, statement validation 400,
  open-limit 429, admin queue ordering + counts, review transition, resolve
  upheld/reversed, reversed restores template atomically, self-guard 403,
  non-admin 403, notification on resolve, audit entries.
- **Unit**: repository state-transition matrix, resolve+restore transaction,
  rate-limit counter, service validation + self-guard.
- **E2E** (test-agent): form modal flow, 409 → status card, queue tabs,
  resolve confirm, self-guard disabled state, i18n parity.