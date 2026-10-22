# Comment Replies & Threads — Architecture Design

| | |
|---|---|
| **Feature ID** | comment-replies-threads |
| **Version** | 1.0 |
| **Status** | Approved for implementation |
| **Author** | Architect Agent |
| **Upstream** | [Requirements](../design/comment-replies-threads-requirements.md), [UI/UX Design](../design/comment-replies-threads-ui-ux-design.md) |
| **Downstream** | [Detailed Design](comment-replies-threads-detailed-design.md) |

## 1. Overview

This feature adds **single-level threading** to the shipped flat comment
system (`template-comments`, commit 793a48f). It follows the GitHub/YouTube
pattern: top-level comments stay newest-first and paginated; replies indent
once under their top-level parent, load on demand in oldest-first
(conversational) order, and are capped at 50 per thread. Replying to a reply
flattens onto the same top-level thread (one level of nesting only). The
parent comment's author is notified with a new `comment_reply` event type.

Scope:

1. **Replying** — any authenticated user can reply to a top-level comment on a
   public template (banned users `403`); replies inherit all existing comment
   rules (1–2000 chars, 15-min edit window, author-or-owner-or-admin delete,
   rate limits 5/min + 50/day).
2. **Threads** — replies render indented under their parent; a collapsed
   "View N replies" bucket with lazy fetch + cache; `has_creator_reply` drives
   an "Answered" chip.
3. **Counters** — `template_comments.reply_count` (direct replies, maintained
   transactionally) and `templates.comment_count` (top-level comments only,
   unchanged semantics).
4. **Notifications** — a `comment_reply` event type notifies the parent
   comment's author (not the template owner, unlike `comment`); payload
   carries the replier's display name and a ≤100-char snippet.
5. **Moderation** — deleting a top-level comment cascades to its replies in
   the same transaction; deleting a reply decrements `reply_count`; the admin
   comments list gains a `parent` column.

### 1.1 Resolved Open Questions

| # | Question (from requirements §10) | Decision |
|---|---|---|
| 1 | `parent_comment_id` constraint: plain UUID column vs FK `REFERENCES template_comments(id) ON DELETE CASCADE` | **Plain UUID column, no FK.** Cascade is handled in the service layer (FR-11), matching the shipped delete flow which already recomputes counters transactionally. A self-referencing FK would complicate the delete transaction ordering and is unnecessary given the service-layer cascade. |
| 2 | Reply cap: 50 per thread vs paginating replies | **50 per thread (cap).** Bounds the unpaginated `ListCommentReplies` response; the 51st reply returns `400` with a localized "thread is full" message. Paginating replies adds machinery for a rare case. |
| 3 | `has_creator_reply` computation: subquery vs denormalized flag | **Subquery in the top-level list query.** List pages are ≤ 50 rows and the indexed `(parent_comment_id, created_at)` lookup is cheap; a denormalized flag adds a write on every reply for negligible read savings. |
| 4 | Notification type naming: `comment_reply` vs `reply` | **`comment_reply`.** Consistent with the existing `comment` type and self-documenting. The `notifications.type` CHECK is widened (guarded, idempotent). Notification-preferences (a backlog feature) will add a `comment_reply_enabled` column when it ships — see §7 coordination. |

### 1.2 Non-Goals

- Multi-level nesting beyond one (replies-to-replies flatten onto the top-level thread).
- @mentions inside reply text (a separate `mention-notifications` feature).
- Reply reactions/emoji; reply sorting options (oldest-first only).
- "Continue this thread" pagination for 50+ reply threads (the cap handles it).
- Editing the reply target after posting; collapsing individual replies.
- Email digests for reply notifications; pinned replies.
- Reply notifications to users other than the parent author.
- Admin thread-view UI (the admin list + parent column is sufficient in v1).
- **Monetization of any kind** — payments, pricing, purchases, earnings, sales, or paid-ranking surfaces are out of scope and excluded from this architecture.

## 2. System Context

```mermaid
flowchart LR
    subgraph Client
        FE[React Frontend<br/>CommentsSection / CommentItem / ReplyComposer / RepliesBucket]
        ADM[Admin Console<br/>AdminCommentsPage + parent column]
    end
    subgraph Gateway
        GW[REST Gateway :8080<br/>/api/v1/templates/{id}/comments<br/>/api/v1/comments/{id}/replies<br/>/api/v1/comments/{id}<br/>/api/v1/admin/comments...]
    end
    subgraph Backend
        PS[PromptService<br/>ListComments / CreateComment / ListCommentReplies]
        AS[AdminService<br/>ListAdminComments / DeleteAdminComment]
        CR[CommentRepository<br/>thread-aware list/create/delete]
        NR[NotificationRepository<br/>comment_reply]
        TR[TemplateRepository]
        RDS[(Redis<br/>rate-limit counters)]
        PG[(PostgreSQL 15)]
    end
    FE -->|GET/POST /api/v1/templates/{id}/comments<br/>GET /api/v1/comments/{id}/replies| GW --> PS
    ADM -->|GET /api/v1/admin/comments<br/>DELETE /api/v1/admin/comments/{id}| GW --> AS
    GW --> CR
    PS --> TR
    PS -->|fixed-window check/increment| RDS
    PS -->|after commit: comment_reply (best-effort)| NR
    AS --> CR
    CR --> PG
    NR --> PG
    TR --> PG
```

No new infrastructure. One new column on an existing table, one new index,
one new notification event type, and one new REST route.

## 3. Components Involved

| Component | Change |
|---|---|
| `backend/api/proto/v1/prompt.proto` | `Comment` gains `reply_count` (10) and `has_creator_reply` (11); new `ListCommentReplies` RPC + `ListCommentRepliesRequest`/`ListCommentRepliesResponse`; `CreateCommentRequest` gains `parent_comment_id` (3). |
| `backend/api/proto/v1/notification.proto` | `Notification` gains `snippet` (10). |
| `backend/internal/models/comment.go` | `Comment` gains `ParentCommentID`, `ReplyCount`, `HasCreatorReply`. |
| `backend/internal/models/notification.go` | `Notification` gains `Snippet`. |
| `backend/internal/repository/comment_repository.go` | `List` returns top-level only + per-item `reply_count`/`has_creator_reply`; new `ListReplies`; `Create` accepts `parent_comment_id` and maintains `reply_count`; `Delete` cascades replies and maintains both counters. |
| `backend/internal/repository/notification_repository.go` | `Create`/`List` handle the `snippet` column. |
| `backend/internal/service/comment.go` | `CreateComment` validates `parent_comment_id` (top-level parent, same template, thread cap); `ListComments` returns top-level only with thread metadata; new `ListCommentReplies`; `DeleteComment` cascades; `comment_reply` notification hook. |
| `backend/internal/service/notification.go` | `notificationToProto` maps `snippet`. |
| `backend/cmd/server/main.go` | New `/api/v1/comments/{id}/replies` GET branch; `CreateComment` body gains `parent_comment_id`. |
| `frontend/src/components/comments/CommentsSection.js` | Thread-aware rendering; one-composer-at-a-time coordination. |
| `frontend/src/components/comments/CommentItem.js` | Reply action, replies bucket toggle, Answered chip. |
| `frontend/src/components/comments/ReplyComposer.js` | **New** — compact composer variant. |
| `frontend/src/components/comments/RepliesBucket.js` | **New** — collapsed count button, lazy fetch + cache, inline error + Retry. |
| `frontend/src/pages/admin/CommentsPage.js` | **New** — parent column added to the moderation table. |
| `frontend/src/services/api.js` | `listCommentReplies`, `createTemplateComment` gains `parent_comment_id`. |
| `frontend/src/locales/{en,zh}/translation.json` | `comments.*` reply keys, `admin.comments.*` parent keys, `notifications.*` `comment_reply` keys. |
| `deployment/init.sql` (+ chart copy) | `template_comments.parent_comment_id` + `reply_count` columns, index, notifications CHECK widening. |

## 4. API Design

### 4.1 User Comment APIs

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/v1/templates/{id}/comments` | Optional | Top-level comments only, paginated, newest-first; each item carries `reply_count` and `has_creator_reply`. |
| `POST` | `/api/v1/templates/{id}/comments` | Required | Body `{content, parent_comment_id?}`. Top-level when `parent_comment_id` absent; reply when present. |
| `GET` | `/api/v1/comments/{id}/replies` | Optional | All replies to a top-level comment, oldest-first, unpaginated (≤ 50). |
| `PUT` | `/api/v1/comments/{id}` | Required | Edit (unchanged; 15-min window). |
| `DELETE` | `/api/v1/comments/{id}` | Required | Delete (author or template owner); cascades replies when the target is top-level. |

Rules:

- **Not authenticated** on POST/PUT/DELETE → `401 UNAUTHENTICATED`.
- **Banned user** posting → `403 PERMISSION_DENIED`.
- **`parent_comment_id`** must reference an existing top-level comment on the
  same template; pointing at a reply → `400 INVALID_ARGUMENT` ("parent must be
  a top-level comment"). Unknown parent → `400 INVALID_ARGUMENT`.
- **Thread cap**: the 51st reply to a thread → `400 INVALID_ARGUMENT`
  ("thread is full").
- **Rate limit** (5/min, 50/day) applies to replies identically → `429
  RESOURCE_EXHAUSTED` with `Retry-After`.
- **Visibility** follows the template exactly: public templates readable
  anonymously; private templates owner-only (`404` otherwise).
- **`comment_count`** increments only for top-level comments; replies
  increment the parent's `reply_count` instead.

### 4.2 Admin Comment APIs

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/v1/admin/comments` | Admin | Existing list + `parent` column (null for top-level, "in reply to {author}" for replies). |
| `DELETE` | `/api/v1/admin/comments/{id}` | Admin | Delete with mandatory reason + audit; follows cascade semantics. |

## 5. Data Model

### 5.1 `template_comments` (modified)

| Column | Type | Notes |
|---|---|---|
| `id` | UUID PK | unchanged |
| `template_id` | UUID FK → templates | unchanged |
| `user_id` | TEXT FK → users | unchanged |
| `content` | TEXT | unchanged |
| `created_at` / `updated_at` | TIMESTAMPTZ | unchanged |
| `parent_comment_id` | UUID, nullable | **new** — null for top-level; references a top-level comment (no FK, service-enforced) |
| `reply_count` | INT NOT NULL DEFAULT 0 | **new** — number of direct replies, maintained transactionally |

New index: `idx_template_comments_parent_created ON template_comments
(parent_comment_id, created_at ASC)`.

### 5.2 `notifications` (modified)

| Column | Type | Notes |
|---|---|---|
| `snippet` | TEXT, nullable | **new** — ≤100-char reply excerpt for `comment_reply` events; null for other types |

The `notifications.type` CHECK is widened to admit `comment_reply`.

## 6. Cross-Cutting Concerns

- **Authz**: reply visibility follows template visibility; mutations require
  auth; banned users cannot reply; delete requires author-or-owner (user
  surface) or admin (admin surface). Admin routes stay under `/api/v1/admin/*`
  with `requireAdmin`.
- **i18n**: all new strings in both `en` and `zh` under `comments.*`,
  `admin.comments.*`, and `notifications.*` with identical key sets.
- **Performance (NFR-1)**: top-level list cost unchanged (two new columns via
  subquery, no N+1); replies load per-thread on demand (≤ 50 rows, single
  indexed query).
- **Consistency (NFR-2)**: `reply_count` uses the same transactional counter
  pattern as `comment_count`/`like_count`; no drift under concurrent replies.
- **Privacy (NFR-3)**: reply visibility matches comment visibility; the
  notification snippet never exceeds the reply's first 100 chars.
- **Accessibility (NFR-5)**: threads are nested semantic lists; bucket toggle
  is a real button with `aria-expanded`; reply composer is labeled; all
  actions keyboard-operable.
- **Resilience (NFR-6)**: reply-list fetch failure shows an inline error +
  Retry inside the bucket only; reply-post failure preserves the draft.

## 7. Deployment Impact

- Append the §2 DDL block to both `deployment/init.sql` and
  `deployment/chart/open-prompts/init.sql` (byte-identical, idempotent,
  additive — existing flat comments have null `parent_comment_id` and render
  unchanged).
- `protoc` regeneration required for `prompt.proto` and `notification.proto`.
- No new services, ports, or images. No new env vars (reuses the existing
  `COMMENT_RATE_PER_MINUTE` / `COMMENT_RATE_PER_DAY`).
- **Coordination with notification-preferences** (backlog): when that feature
  ships, its `notification_preferences` table must include a
  `comment_reply_enabled BOOLEAN NOT NULL DEFAULT true` column and a
  `notifPrefs.type.comment_reply` UI row. Until then, `comment_reply` events
  are always emitted (no suppression).
- Rollback: dropping the two new columns and the index, and reverting the
  notifications CHECK widening, is sufficient; the new REST route would 404
  but nothing else degrades.

## 8. Test Strategy (summary)

- **FVT (`tests/fvt.py`)**: extend `test_template_comments()` — reply attach
  rules (top-level only, same template), thread cap (51st → 400), cascade
  delete (top-level removes replies, `comment_count` −1; reply removes itself,
  `reply_count` −1), counters, `comment_reply` notification (parent author
  notified, self-reply suppressed, snippet ≤100 chars), `has_creator_reply`
  chip, admin parent column, rate limits on replies.
- **Backend unit tests**: `parent_comment_id` validation, thread-cap boundary,
  cascade transaction, counter maintenance, `has_creator_reply` subquery,
  notification snippet truncation, permission matrix.
- **E2E (Nightwatch, test-agent)**: reply flow, bucket expand/collapse + lazy
  fetch + cache, Answered chip, admin parent column, EN/ZH strings, keyboard
  access.

Detailed contracts, DDL, layer responsibilities, i18n keys, and test hooks:
see the [detailed design](./comment-replies-threads-detailed-design.md).