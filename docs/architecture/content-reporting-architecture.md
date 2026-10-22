# Content Reporting — Architecture Design

| | |
|---|---|
| **Feature ID** | content-reporting |
| **Version** | 1.0 |
| **Status** | Approved for implementation |
| **Author** | Architect Agent |
| **Upstream** | [Requirements](../design/content-reporting-requirements.md), [UI/UX Design](../design/content-reporting-ui-ux-design.md) |
| **Downstream** | [Detailed Design](content-reporting-detailed-design.md) |

## 1. Overview

Content reporting gives the platform its **inbound abuse signal**: a logged-in
user can flag a public template or a public comment with a fixed reason and an
optional note, and an admin works a prioritized queue grouped by item, with a
review drawer that carries full context and one-click resolution (dismiss or
act). The design follows the Reddit/FlowGPT model — a small fixed reason list,
one report per user per item (re-report replaces), a queue sorted by open count
and recency, and **no auto-takedown** (every consequence requires an explicit
admin action, NFR-4).

The feature introduces a single new table **`content_reports`** (OQ-1) that
records every report. The queue groups by `(item_type, item_id)` with
`COUNT(*)` and `MAX(created_at)` computed on read (OQ-2) — no denormalized
summary table is needed at the expected scale. Resolution is atomic across all
open reports of an item (NFR-3). Reporters are silent-to-reporter (FR-17): no
notifications, no case tracking — the feature's job is routing attention, not
building a case-management system.

### 1.1 Resolved Open Questions

| # | Question (from requirements §10) | Decision |
|---|---|---|
| 1 | Item identity: comments need a stable public id | **Comment ids are already exposed** in `Comment` proto (`id = 1`) and in `ListCommentsResponse`/`ListCommentRepliesResponse` payloads. Templates use their existing UUID. No new id scheme needed. |
| 2 | Grouping query vs a denormalized `report_groups` summary table | **Compute on read.** The `(status, item_type, item_id)` index (FR-1) serves the grouped queue query; at the stated scale (1,000 open reports, 50/page) a single indexed aggregate is well under the 1 s NFR-1 budget. A summary table would add write-path complexity for no measured benefit. |
| 3 | Auto-resolve triggers (FR-12): event-driven vs sweep on queue read | **Service-layer hook at unpublish/delete time.** When a template is unpublished or a comment deleted by any path, the service resolves that item's open reports as `actioned` in the same transaction. No sweep job; no dangling queue rows. |
| 4 | Withdraw semantics: hard delete vs mark withdrawn | **Hard delete.** `DELETE /reports/mine` removes the row entirely; the `UNIQUE (item_type, item_id, reporter_id)` constraint then allows clean re-reporting. Simplest and matches the "no trace" intent. |
| 5 | Rate limit storage: in-memory vs redis | **Redis counter with 1-hour TTL** (`report_rate:{user_id}`), fail-open on Redis errors. Mirrors the existing comment rate-limit pattern (`checkCommentRate`). |
| 6 | Audit action naming | **New audit action codes `report.dismiss` and `report.actioned`.** The `admin_audit_logs.action` CHECK is widened (idempotent) to include both. `target_type` reuses the existing `template`/`comment` values. |

### 1.2 Non-Goals

- Auto-takedown or auto-hide at report thresholds (human decision always in v1).
- Reporter-facing case tracking or resolution notifications (silent-to-reporter).
- Reporting users/profiles (user abuse goes through the existing ban flow).
- Reporting chains, collections, or teams.
- Moderation workspaces, assignment/claiming, SLA timers.
- **Monetization of any kind** — payments, pricing, purchases, earnings, sales,
  or paid-ranking surfaces are out of scope and excluded from this architecture.

## 2. System Context

```mermaid
flowchart LR
    subgraph Client
        FE[React Frontend<br/>ReportMenu / ReportModal / ReportedMenuItem]
        ADM[Admin Console<br/>ReportsPage / ReviewDrawer / OpenReportsKPI]
    end
    subgraph Gateway
        GW[REST Gateway :8080<br/>/api/v1/reports... /api/v1/admin/reports...]
    end
    subgraph Backend
        PS[PromptService<br/>CreateReport / GetMyReport / WithdrawReport]
        AS[AdminService<br/>ListReportQueue / GetReportDetail / ResolveReportGroup]
        RR[ReportRepository<br/>content_reports CRUD + grouped queue]
        AR[AdminRepository<br/>audit + unpublish/delete hooks]
    end
    subgraph Stores
        PG[(PostgreSQL)]
        RD[(Redis<br/>report_rate:{user} counter)]
    end
    FE -->|POST/GET/DELETE /api/v1/reports| GW --> PS
    ADM -->|GET/POST /api/v1/admin/reports| GW --> AS
    PS --> RR --> PG
    AS --> RR --> PG
    AS --> AR --> PG
    PS -->|report_rate:{user}| RD
```

## 3. Components Involved

| Component | Change |
|---|---|
| `backend/api/proto/v1/prompt.proto` | `CreateReport`/`GetMyReport`/`WithdrawReport` RPCs on `PromptService`; `Report`/`MyReport` messages. |
| `backend/api/proto/v1/admin.proto` | `ListReportQueue`/`GetReportDetail`/`ResolveReportGroup` RPCs on `AdminService`; `AdminReportGroup`/`AdminReportDetail` messages; `open_reports` field on `GetAdminStatsResponse`. |
| `backend/internal/models/report.go` | **New** — `ContentReport` GORM model. |
| `backend/internal/repository/report_repository.go` | **New** — report CRUD, grouped queue, detail, atomic resolve, auto-resolve hooks. |
| `backend/internal/service/prompt.go` | `CreateReport`/`GetMyReport`/`WithdrawReport` implementations (auth, banned check, rate limit). |
| `backend/internal/service/admin.go` | `ListReportQueue`/`GetReportDetail`/`ResolveReportGroup` implementations (audit, unpublish/delete hooks, KPI count). |
| `backend/cmd/server/main.go` | `/api/v1/reports` and `/api/v1/reports/mine` branches; `/api/v1/admin/reports` and `/api/v1/admin/reports/{item_type}/{item_id}` branches. |
| `frontend/src/components/ReportMenu.js`, `ReportModal.js`, `ReportedMenuItem.js` | **New.** |
| `frontend/src/pages/admin/ReportsPage.js`, `frontend/src/components/admin/ReviewDrawer.js`, `ReasonPopover.js` | **New** admin surfaces. |
| `frontend/src/services/api.js`, `frontend/src/services/adminApi.js` | Report API client methods. |
| `frontend/src/locales/{en,zh}/translation.json` | `reports.*` and `admin.reports.*` namespaces. |
| `deployment/init.sql` (+ chart copy) | `content_reports` table, audit CHECK widening. |

## 4. API Design

### 4.1 User Report APIs

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/api/v1/reports` | Required | Body `{item_type, item_id, reason, note}`. Creates or updates the caller's report on the item. Returns the report. |
| `GET` | `/api/v1/reports/mine?item_type=&item_id=` | Required | The caller's open report for an item (drives the checkmark state). |
| `DELETE` | `/api/v1/reports/mine` | Required | Body `{item_type, item_id}`. Withdraws (hard-deletes) the caller's report. |

Rules:

- **Not authenticated** → `401 UNAUTHENTICATED`.
- **Banned user** → `403 PERMISSION_DENIED`.
- **Invalid `item_type`** (not `template`/`comment`) → `400 INVALID_ARGUMENT`.
- **Item not found / not public** → `404 NOT_FOUND` (reporting private or
  non-existent content is indistinguishable).
- **Reporting own content** → `400 INVALID_ARGUMENT` (FR-4 hides the action;
  server enforces as a backstop).
- **Invalid reason** (not in the 5-code taxonomy) → `400 INVALID_ARGUMENT`.
- **Note > 500 chars** → `400 INVALID_ARGUMENT`.
- **Rate limit exceeded** (20/hour) → `429 RESOURCE_EXHAUSTED` with
  `Retry-After`.

### 4.2 Admin Report APIs

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/v1/admin/reports?status=&item_type=&reason=&page=` | Admin | Grouped queue, open-first by count then recency, 50/page. |
| `GET` | `/api/v1/admin/reports/{item_type}/{item_id}` | Admin | Detail: item context + every report (reporter, reason, note, time). |
| `POST` | `/api/v1/admin/reports/{item_type}/{item_id}/resolve` | Admin | Body `{resolution: dismissed\|actioned, reason}`. Resolves all open reports on the item atomically. |

Rules:

- **Non-admin** → `403 PERMISSION_DENIED`.
- **Reason outside 3–500 chars** → `400 INVALID_ARGUMENT`.
- **Invalid resolution** → `400 INVALID_ARGUMENT`.
- **Item not found** → `404 NOT_FOUND`.
- **No open reports** → `409 ALREADY_EXISTS` (nothing to resolve).

The "act" path (unpublish template / delete comment) reuses the existing admin
endpoints; the report queue's resolve endpoint records the resolution and the
underlying action is performed by the existing flow (FR-10, NFR-7).

### 4.3 Error Codes

| HTTP | gRPC | Condition |
|---|---|---|
| 400 | `INVALID_ARGUMENT` | Invalid item_type/reason; note > 500; reporting own content; reason length. |
| 401 | `UNAUTHENTICATED` | Missing/invalid token. |
| 403 | `PERMISSION_DENIED` | Banned user; non-admin admin route. |
| 404 | `NOT_FOUND` | Item not found/not public; report not found. |
| 409 | `ALREADY_EXISTS` | No open reports to resolve. |
| 429 | `RESOURCE_EXHAUSTED` | Report rate limit (`Retry-After`). |

## 5. Data Model

### 5.1 New Table: `content_reports`

```sql
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
CREATE INDEX IF NOT EXISTS idx_content_reports_status_item
    ON content_reports (status, item_type, item_id);
CREATE INDEX IF NOT EXISTS idx_content_reports_reporter
    ON content_reports (reporter_id);
```

`UNIQUE (item_type, item_id, reporter_id)` enforces FR-1 (one report per user
per item) at the database level; the `23505` violation maps to an upsert
(re-report replaces, FR-2). `status`/`resolution` lifecycle: `open` →
`resolved` with `resolution` `dismissed` or `actioned`.

### 5.2 CHECK Widenings (idempotent, both init.sql copies)

- `admin_audit_logs.action` gains `report.dismiss`, `report.actioned`.
- `admin_audit_logs.target_type` already includes `template` and `comment` —
  no widening needed.

## 6. Auth & Authorization

- **User writes** require session/API-key auth; user id from context, never the
  body. Banned users are rejected (FR-3). Users cannot report their own content.
- **Admin writes** require the admin JWT (`requireAdminContext`).
- **Visibility**: reporters are visible only to admins (NFR-2); the user-facing
  `GET /reports/mine` returns only the caller's own report. Content authors
  never see who reported them.
- **Rate limiting**: report submission limited to 20/hour per user via a Redis
  counter (`report_rate:{user_id}`, 1-hour TTL), fail-open on Redis errors.

## 7. Frontend

- **ReportMenu** (⋯ menu item on templates and comments): "Report" when no open
  report; "You reported this" with Update/Withdraw submenu when the caller has
  an open report (FR-7). Hidden for the user's own content (FR-4).
- **ReportModal**: five radio reasons with descriptors + optional note (≤500),
  Cancel/Submit; Submit disabled until a reason is chosen; focus-trapped dialog
  (NFR-6). Title reads "Report this template"/"Report this comment".
- **ReportedMenuItem**: checkmark state with Update (reopens modal prefilled)
  and Withdraw (confirm popover).
- **ReportsPage** (`/admin/reports`): grouped queue table — type badge, item
  title/snippet, author, open count, reason chips, latest time, status,
  Review/View action; filters (status/type/reason); open-first sort by count
  then recency (FR-8/FR-9).
- **ReviewDrawer**: right drawer with item context card, author link, every
  report (reporter, reason, note, time), and resolution actions — Dismiss
  reports / Act (unpublish template or delete comment), each requiring a reason
  (FR-10). "Ban author" links to the admin user detail.
- **OpenReportsKPI**: dashboard card with the open-report count linking to
  `/admin/reports` (FR-13).
- **i18n**: `reports.*` and `admin.reports.*` namespaces; EN/ZH parity per the
  UI/UX doc §8.

## 8. Configuration

| Variable | Default | Description |
|---|---|---|
| `REPORT_RATE_PER_HOUR` | `20` | Max report submissions per user per hour. |

Added to `deployment/docker-compose.yml` and the Helm chart
(`values.yaml` + `templates/deployment-backend.yaml`), mirroring
`RATING_RATE_PER_MINUTE`.

## 9. Security Considerations

- **Privacy**: reporters are admin-only (NFR-2); notes are confidential
  moderation data; no existence leak to non-owners (404).
- **Integrity**: `UNIQUE (item_type, item_id, reporter_id)` enforces one-report-
  per-user-per-item; resolution is atomic across all open reports of an item
  (NFR-3); auto-resolve on unpublish/delete is transactional.
- **Abuse**: 20/hour rate limit; banned users blocked; no auto-takedown (NFR-4).
- **Injection**: parameterized SQL only.
- **XSS**: notes and reasons are plain text rendered with escaping only.

## 10. Deployment Notes

- DDL is additive and idempotent; both init.sql copies updated in the same
  commit.
- No new services or infrastructure; one new Redis counter key.
- `backend/server` is a tracked build artifact — never commit changes to it.

## 11. Test Strategy

- **FVT** (`tests/fvt.py`): `test_content_reporting` — create/update report,
  one-per-user upsert, withdraw, banned 403, anonymous 401, rate limit 429,
  admin queue grouping + ordering + filters, detail, dismiss/actioned resolve,
  auto-resolve on unpublish/delete, audit entries, KPI count.
- **Unit**: repository upsert + unique-violation mapping, grouped queue query,
  atomic resolve, auto-resolve hooks, rate-limit counter, service validation +
  authz.
- **E2E** (test-agent): report modal flow, reported-state menu, queue table,
  review drawer, dismiss/act, KPI card, i18n parity.