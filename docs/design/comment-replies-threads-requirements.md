# Comment Replies & Threads — Requirements Analysis

- **Feature ID:** comment-replies-threads
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. Competitor Research Summary

| Platform | Threading approach | Key takeaways |
|---|---|---|
| **Reddit** | Nested threads (default deep); "continue this thread" collapse; reply is the primary verb | Deep nesting is unreadable on mobile; collapsing is essential; reply is the core engagement verb |
| **GitHub (issues/discussions)** | Flat list with one level of reply indentation; reply references the parent; timeline view | **One level of indentation is the sweet spot** for Q&A: visually grouped, no deep nesting, trivially paginated |
| **Stack Overflow** | Comments + replies with @-style addressing; "show N more comments" | Reply-to-a-reply is rare and usually should target the top-level parent; lazy expansion keeps pages fast |
| **YouTube** | One-level replies under a top-level comment; "View replies (N)" collapsible bucket | The "replies bucket" pattern (collapsed by default, count visible) keeps long comment sections scannable |
| **FlowGPT** | Flat comments, creator replies inline, no threading | Confirms flat-only is workable but makes multi-party Q&A hard to follow |
| **Disqus** | Nested up to a few levels; replies notify the parent author | Reply notifications are the retention hook: they bring the parent author back |

**Synthesis:** The industry answer for comment sections (as opposed to forums) is **one level of nesting**: top-level comments plus indented replies, with replies-to-replies flattened onto the same top-level thread (GitHub/YouTube pattern). This keeps the schema simple (`parent_comment_id` on the existing `template_comments` table), keeps pagination trivial (top-level comments paginate; replies load per-thread), and matches how prompt Q&A actually flows ("does this work with GPT-4?" → creator reply → follow-up). Open Prompts shipped flat comments (commit 793a48f) with the comments design explicitly deferring threading to a future feature — this is that feature. It builds directly on the shipped `CommentsSection`/`CommentItem`/`CommentComposer` components, the 15-minute edit window, owner/admin moderation, and rate limits.

## 2. Problem Statement

1. **Multi-party conversations are impossible to follow.** With flat comments, a creator's answer to Alice's question appears as an unrelated row between other comments. Readers cannot tell which comment a reply answers.
2. **Reply intent is lost.** A user writing "@creator the JSON still breaks" has no structural way to attach the message to the question it answers; the only tool is manual @-typing, which the shipped feature does not support.
3. **The parent author is never notified.** When a creator replies to a user's comment, that user has no way to know — the shipped notifications only inform the template owner of new comments. The conversation dies silently.
4. **Q&A value is buried.** The most valuable content in a comment section is creator answers; without threading they are scattered randomly through the list, and without a filter readers cannot find answered vs. unanswered questions.

## 3. Target Users

- **Commenters (question askers):** want their follow-up attached to their original comment and want to learn when someone replies to them.
- **Creators (template owners):** want to answer specific questions in context and want answered Q&A to be findable by future readers.
- **Readers/prospective users:** want to scan a comment section and read question→answer pairs together.
- **Admins:** moderate threaded comments with the same tools as today (the thread structure must not complicate deletion).

## 4. User Stories

### 4.1 Replying

- **US-R1:** As an authenticated user, I want to reply to any comment on a public template, so that my response is attached to the comment it answers.
- **US-R2:** As a commenter, when someone replies to my comment I want a notification, so that I can continue the conversation.
- **US-R3:** As a user, when I reply to a reply, I want my reply attached to the same top-level thread (not nested deeper), so that threads stay readable.
- **US-R4:** As an anonymous visitor, I am prompted to log in when trying to reply, so that the flow is understandable.
- **US-R5:** As a banned user, I cannot reply, consistent with my restricted account state.

### 4.2 Reading threads

- **US-R6:** As a reader, I want replies indented directly under their parent comment, so that I can follow question→answer pairs.
- **US-R7:** As a reader, I want reply buckets collapsed by default with a visible count ("View 3 replies"), so that long sections stay scannable.
- **US-R8:** As a reader, I want to load a thread's replies on demand, so that a template with 200 comments stays fast.
- **US-R9:** As a reader, I want the creator's replies visually distinguished (existing CREATOR badge), so that I can find official answers within threads.

### 4.3 Moderation & lifecycle

- **US-R10:** As a commenter, I can edit/delete my own replies within the same rules as top-level comments (15-min edit window), so that behavior is consistent.
- **US-R11:** As a template owner, I can delete any reply on my template, so that owner moderation works uniformly.
- **US-R12:** As an admin, I can delete any reply with a reason from the admin console, so that moderation is unchanged by threading.
- **US-R13:** As the system, deleting a top-level comment deletes its replies (cascade), so that no orphaned replies remain.
- **US-R14:** As the system, deleting a reply leaves its parent and siblings intact.

### 4.4 System

- **US-R15:** As the system, I count only top-level comments in `templates.comment_count`, so that the counter keeps its meaning ("comments") and does not double-count conversations.
- **US-R16:** As the system, I apply the same rate limits to replies as to comments, so that threading is not a spam bypass.

## 5. Functional Requirements

### 5.1 Data model

- **FR-1:** `template_comments` shall gain `parent_comment_id` (UUID, nullable — null for top-level comments) and `reply_count` (INT, NOT NULL, default 0 — number of direct replies, maintained transactionally). An index on `(parent_comment_id, created_at ASC)` supports per-thread reply loading. No FK constraint on `parent_comment_id` is required (deletion cascades are handled in FR-11); architect may add `REFERENCES template_comments(id) ON DELETE CASCADE` if preferred.
- **FR-2:** `CreateComment` shall accept an optional `parent_comment_id`. When set: the parent must exist, belong to the same template, and be a **top-level comment** (its own `parent_comment_id` is null) — replying to a reply attaches to the same top-level parent (US-R3). Violations return 400 with a specific message.
- **FR-3:** A reply shall inherit all existing comment rules unchanged: 1–2000 chars, banned users 403, rate limits (5/min, 50/day), edit window (15 min), author-or-owner-or-admin delete.
- **FR-4:** Posting a reply shall increment the parent's `reply_count` transactionally. `templates.comment_count` increments **only for top-level comments**; replies do not touch it (US-R15).

### 5.2 Reading

- **FR-5:** `ListComments` shall return **top-level comments only** (paginated, newest-first, unchanged shape) each extended with `reply_count` and `has_creator_reply` (bool — whether the template owner has replied in that thread; drives the "Answered" chip).
- **FR-6:** A new `ListCommentReplies` RPC (`GET /api/v1/comments/{id}/replies`) shall return all replies to a top-level comment, oldest-first (conversational order), unpaginated (bounded by FR-8's reply cap). Each reply uses the existing `Comment` shape plus `is_own`.
- **FR-7:** Both endpoints follow the shipped visibility rules: public templates readable anonymously; private templates owner-only (404 otherwise).
- **FR-8:** A thread shall hold at most **50 replies**; the 51st returns 400 with a friendly message ("This thread is full — reply as a new comment"). This bounds the unpaginated replies response.

### 5.3 Notifications

- **FR-9:** Posting a reply shall emit a `comment_reply` notification to the parent comment's author (best-effort, self-suppressed), in addition to the existing `comment` notification to the template owner. If the parent author is also the template owner, only the `comment` notification is sent (no duplicates).
- **FR-10:** The notification payload includes template id/title, the replier's display name, and a snippet of the reply (first 100 chars), matching the existing notification payload pattern.

### 5.4 Moderation & lifecycle

- **FR-11:** Deleting a top-level comment shall delete its replies in the same transaction (cascade). `templates.comment_count` decrements by 1 (the top-level comment only); the parent's `reply_count` is irrelevant (parent is being deleted).
- **FR-12:** Deleting a reply shall decrement its parent's `reply_count` transactionally; `templates.comment_count` is unchanged.
- **FR-13:** The admin comments list (`/admin/comments`) shall show a `parent` column (null for top-level, "in reply to {author}" for replies) and deleting any comment from the admin console follows FR-11/FR-12 semantics with the existing mandatory reason + audit log.
- **FR-14:** Editing a reply follows the existing edit rules (15-min window, `updated_at` only, no re-sorting); replies keep conversational (oldest-first) order regardless of edits.

### 5.5 Frontend (regular-user surface)

- **FR-15:** Each top-level `CommentItem` shall render a `Reply` action (auth only; login prompt for anonymous). Clicking it opens an inline reply composer directly under the comment (same component as the main composer, compact variant).
- **FR-16:** When `reply_count > 0`, the item renders a collapsed replies bucket: "View {N} replies ▾" (or "1 reply"); clicking expands and lazily fetches replies via FR-6; clicking again collapses. Expanded state is per-item component state (not persisted).
- **FR-17:** Replies render indented under the parent with the existing `CommentItem` visuals (avatar, name, CREATOR badge, relative time, edited marker, hover actions for edit/delete per existing permission rules).
- **FR-18:** A thread with `has_creator_reply: true` shows an "Answered" chip next to the top-level comment's timestamp, so readers can find answered questions (US-R9 companion).

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Top-level listing cost is unchanged (same query + two new columns). Replies load per-thread on demand (≤ 50 rows, single indexed query). No N+1: `reply_count`/`has_creator_reply` are computed in the top-level list query (join or subquery), not per-item.
- **NFR-2 (Consistency):** `reply_count` uses the same transactional counter pattern as `comment_count`/`like_count`; no drift under concurrent replies.
- **NFR-3 (Privacy):** Reply visibility follows the shipped comment visibility exactly; no new data exposure. Notification snippets never include more than the reply's first 100 chars.
- **NFR-4 (i18n):** All new UI strings exist in both `en` and `zh` with identical key sets; reply composer, bucket labels, "Answered" chip, and thread-full error are localized.
- **NFR-5 (Accessibility):** Threads are nested semantic lists; expand/collapse is a real button with `aria-expanded`; reply composer is labeled; all actions keyboard-operable.
- **NFR-6 (Resilience):** Reply-list fetch failure shows an inline error + Retry inside the bucket only; the rest of the section is unaffected. Reply-post failure preserves the draft (existing pattern).
- **NFR-7 (Compatibility):** Existing flat comments (all rows have null `parent_comment_id` after migration) render exactly as today. API consumers ignoring `reply_count`/`has_creator_reply` are unaffected. The `comment` notification type and its preferences entry remain valid.

## 7. Acceptance Criteria

- **AC-1:** An authenticated user can reply to a top-level comment; the reply appears indented under it (after expanding the bucket) in oldest-first order; the parent's `reply_count` increments; `templates.comment_count` does not change.
- **AC-2:** Replying to a reply attaches the new reply to the same top-level thread (one level of nesting only); the API rejects `parent_comment_id` pointing to a reply with 400.
- **AC-3:** The parent comment's author receives a `comment_reply` notification with replier name and a ≤100-char snippet; replying to your own comment sends no notification; a creator replying on their own template sends only the existing `comment` notification to themselves — none.
- **AC-4:** A thread with replies shows "View N replies"; expanding lazily loads replies; collapsing and re-expanding does not refetch (cached in component state).
- **AC-5:** Threads with a creator reply show the "Answered" chip on the top-level comment.
- **AC-6:** Editing/deleting a reply follows the same 15-min window and permission rules as comments; deleting a reply decrements `reply_count` and leaves siblings intact.
- **AC-7:** Deleting a top-level comment removes its replies in the same transaction; `comment_count` decrements by exactly 1.
- **AC-8:** The 51st reply to a thread returns 400 with the localized "thread is full" message; the UI shows it inline and preserves the draft.
- **AC-9:** Rate limits apply to replies identically (5/min, 50/day); exceeding them returns 429 with `Retry-After` and a friendly message.
- **AC-10:** `/admin/comments` shows the parent column; deleting a reply or a top-level comment (with reason) follows the cascade rules and writes the audit log row.
- **AC-11:** Anonymous visitors can read threads on public templates but see a login prompt on Reply; private templates' threads are owner-only.
- **AC-12:** Existing flat comments render unchanged (no bucket, no chip, no regression in the shipped comments FVT).
- **AC-13:** All new strings exist in both `en` and `zh`; threads render correctly in both languages and both themes.

## 8. Out of Scope (v1)

- Multi-level nesting beyond one (replies-to-replies flatten onto the top-level thread).
- @mentions inside reply text (plain text; a separate mention-notifications feature).
- Reply reactions/emoji.
- Sorting options for replies (oldest-first only).
- "Continue this thread" pagination for 50+ reply threads (the cap handles it).
- Editing the reply target after posting.
- Collapsing individual replies.
- Email digests for reply notifications.
- Creator pinning a reply.
- Reply notifications to users other than the parent author (e.g., other participants in the thread).
- Admin thread-view UI (admin list + parent column is sufficient in v1).
- Any monetization-adjacent features (per project scope rule).

## 9. Dependencies & Assumptions

- **D-1:** Template comments shipped (commit 793a48f): `template_comments` table, Comment CRUD RPCs, 15-min edit window, author-or-owner delete, Redis rate limits (5/min, 50/day), `comment_count` maintenance, admin moderation with reason + audit log, `CommentsSection`/`CommentItem`/`CommentComposer`/`CommentEditForm` frontend components. This feature extends all of them; no rework.
- **D-2:** The notifications system supports typed events (`like`/`favorite`/`fork`/`comment`); `comment_reply` is a new type requiring: enum widening in `notifications.type` CHECK, notification-preferences entry (the preferences feature's type list is data-driven), and a localized notification string. The preferences UI renders the new type automatically if it follows its data-driven design.
- **D-3:** The `Comment` proto message gains `reply_count`, `has_creator_reply`, and (for replies) nothing else — `is_own` already exists. `ListCommentsResponse` is unchanged apart from the per-item additions.
- **D-4:** The REST gateway already routes `/api/v1/comments/{id}` (update/delete); `/api/v1/comments/{id}/replies` follows the same pattern.
- **D-5:** The shipped comments FVT (`test_template_comments`) covers flat CRUD/permissions/moderation; this feature adds FVT cases for reply attach rules, cascade delete, counters, notifications, and the thread cap.

## 10. Open Questions for Architect

1. **`parent_comment_id` constraint:** plain UUID column (recommended — cascade handled in service layer, matching the shipped delete flow) vs. FK `REFERENCES template_comments(id) ON DELETE CASCADE` (DB-enforced). The shipped table uses FKs for template/user; which style for self-reference?
2. **Reply cap:** 50 per thread (my proposal, bounds the unpaginated response). Alternative: paginate replies too (more machinery for a rare case). Confirm 50 or adjust.
3. **`has_creator_reply` computation:** subquery in the top-level list query vs. denormalized flag maintained on reply insert (extra write, cheaper reads). Recommendation: subquery (list pages are ≤ 50 rows; the indexed parent lookup is cheap).
4. **Notification type naming:** `comment_reply` (recommended, consistent with `comment`) vs. `reply`. Also confirm whether notification-preferences needs a migration or renders data-driven.
