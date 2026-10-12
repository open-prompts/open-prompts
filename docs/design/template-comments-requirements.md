# Template Comments — Requirements Analysis

- **Feature ID:** template-comments
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. Competitor Research Summary

| Platform | Comments approach | Key takeaways |
|---|---|---|
| **FlowGPT** | Comment list under each prompt; plain text; creator replies inline; no threading | Flat comments are enough for prompt feedback ("works great with GPT-4", "add an example variable"); threading is nice-to-have, not must-have |
| **Snack Prompt** | Comments + emoji reactions on prompts; creator replies are visually highlighted | Highlighting creator replies makes Q&A scannable; lightweight reactions lower the participation barrier |
| **PromptBase** | Star-rating reviews with seller replies | A rating-gated review model does not fit a free community; a separate rating feature would be needed (out of scope here) |
| **AIPRM** | Public comments/feedback on prompts in its directory | Comments double as social proof: a commented prompt looks alive and maintained |
| **PromptHero** | Comments on prompt art posts; flat list, newest first | Flat, newest-first with pagination is the standard consumption pattern |
| **GitHub (discussions/issues)** | Threaded, editable, deletable, mention support; moderation by repo owners | Owner-side moderation (delete/hide) is the minimum viable moderation model; edit-with-history is overkill for v1 |
| **Reddit** | Threaded, votes, mod removal with "removed" placeholder | A visible "deleted" placeholder keeps context when a middle comment is removed |

**Synthesis:** The winning v1 pattern is a flat, paginated comment list on the template detail page: any authenticated user can post (banned users cannot), the author can edit/delete their own comments within a time window, the template owner can delete any comment on their template (owner moderation), and admins can delete comments platform-wide from the admin console. Threading, mentions, reactions, and rich text are deferred. Open Prompts currently has zero two-way communication: users cannot ask "what model does this work best with?" and creators cannot broadcast "v2 now handles edge cases" — the strongest community loop in prompt platforms is missing.

## 2. Problem Statement

1. **No two-way communication channel.** Template detail pages expose like/favorite/fork counts, but a user who wants to ask a question, report that a prompt produces poor output, or share a tip has no way to do so. The only channel is external.
2. **No social proof beyond counters.** A number next to a heart does not tell a prospective user whether the template actually works. Comments are the primary trust signal on every competitor.
3. **No creator broadcast surface.** When a creator improves a template, there is no place on the page to announce it ("v2 fixes the JSON output issue").
4. **No moderation path for user-generated text.** The product's only user-generated free text today is template content (already moderated via admin template visibility). Comments add a new, higher-volume UGC surface that requires owner moderation plus admin moderation from day one, integrated with the existing audit log.

## 3. Target Users

- **Prospective users (primary consumers):** users evaluating a template who read comments to judge quality and fit before using/forking.
- **Commenters:** authenticated users who ask questions or share tips; banned users are excluded.
- **Creators (template owners):** answer questions, broadcast updates, and moderate their own templates' comment sections.
- **Admins:** moderate comments platform-wide (delete rule-violating content) with audit trail, via the admin console.

## 4. User Stories

### 4.1 Reading comments

- **US-C1:** As a prospective user, I want to read comments on a template detail page, so that I can judge whether the template works well before using it.
- **US-C2:** As a user, I want comments sorted newest-first with pagination, so that I can browse without performance issues on popular templates.
- **US-C3:** As a user, I want to see who wrote each comment (display name, avatar) and when, so that I can weigh the feedback.
- **US-C4:** As a user, I want the creator's replies visually distinguished, so that I can find official answers quickly.
- **US-C5:** As a user, I want to see how many comments a template has (counter), so that I can gauge community activity.

### 4.2 Writing comments

- **US-C6:** As an authenticated user, I want to post a comment on a public template, so that I can ask a question or share feedback.
- **US-C7:** As a commenter, I want to edit my own comment shortly after posting, so that I can fix typos.
- **US-C8:** As a commenter, I want to delete my own comment, so that I can retract it entirely.
- **US-C9:** As an anonymous visitor, I am prompted to log in when trying to comment, so that the flow is understandable.
- **US-C10:** As a banned user, I cannot post comments, consistent with my restricted account state.

### 4.3 Creator interaction & moderation

- **US-C11:** As a creator, I want to reply to comments on my template (as the highlighted creator), so that I can answer questions and build trust.
- **US-C12:** As a creator, I want to delete any comment on my own template, so that I can keep my template's section free of spam/abuse.
- **US-C13:** As a creator, I want to be notified when someone comments on my template, so that I can respond promptly. *(depends on the notifications feature; see §9)*

### 4.4 Admin moderation

- **US-C14:** As an admin, I want to browse and search all comments platform-wide, so that I can find rule-violating content.
- **US-C15:** As an admin, I want to delete any comment with a mandatory reason, so that moderation is accountable via the audit log.
- **US-C16:** As an admin, I want to see a comment's full context (template, author, content) before acting, so that I moderate accurately.

### 4.5 System

- **US-C17:** As the system, I enforce that only the comment author, the template owner, and admins can delete a given comment.
- **US-C18:** As the system, I never render user comment text as HTML, so that XSS is impossible.
- **US-C19:** As the system, deleting a template or user removes their comments, so that no orphaned content remains.

## 5. Functional Requirements

### 5.1 Data model

- **FR-1:** A `template_comments` table shall store: `id` (UUID), `template_id` (FK → templates, `ON DELETE CASCADE`), `user_id` (FK → users, `ON DELETE CASCADE`), `content` (TEXT, 1–2000 chars after trim), `created_at`, `updated_at` (TIMESTAMPTZ). Index on `(template_id, created_at DESC)` for list queries; index on `user_id`.
- **FR-2:** `templates.comment_count` (INT, default 0) shall be maintained transactionally on insert/delete (same pattern as `like_count`/`favorite_count`), so lists and detail pages can show counts without COUNT(*) per row.
- **FR-3:** Editing a comment shall update `updated_at` only; it shall not bump the comment's position in the list (sort stays by `created_at`).

### 5.2 Reading (regular-user surface)

- **FR-4:** `GET /api/v1/templates/{id}/comments` shall return a paginated list (default 20, max 50, `page`/`page_size` or cursor — architect's choice) sorted by `created_at` DESC, each item containing `id`, `content`, `created_at`, `updated_at`, `is_edited` (bool), and a public author object (`id`, `display_name`, `avatar` — never email). The response shall include `total_count` and the viewer's own comment ids (or an `is_own` flag) so the UI can show edit/delete affordances without leaking others' rights.
- **FR-5:** The endpoint shall be public (anonymous readable) for public templates, matching the visibility of the template itself; private templates' comments are readable only by the owner.
- **FR-6:** The template detail response (`GET /api/v1/templates/{id}`) shall include `comment_count`.
- **FR-7:** Deleted comments shall be hard-deleted (no placeholder rows) in v1; the list simply excludes them. *(Soft-delete with a "deleted" placeholder is an explicit non-goal for v1 — see §8.)*

### 5.3 Writing (regular-user surface)

- **FR-8:** `POST /api/v1/templates/{id}/comments` (auth required) shall create a comment with `content` of 1–2000 chars after trim; 400 with a field-level error otherwise. Banned users (`users.status = 'banned'`) receive 403. Commenting on a private template is allowed only for its owner.
- **FR-9:** `PUT /api/v1/comments/{commentId}` shall allow the author to edit content (same 1–2000 validation) within 15 minutes of `created_at`; afterwards 403 with a clear message ("editable window expired — delete and repost instead").
- **FR-10:** `DELETE /api/v1/comments/{commentId}` shall be permitted when the caller is the comment author, the template owner, or an admin. Template owners and admins act through the admin-capable endpoint (§5.4) so the action is audited; the author's self-delete is not audited.
- **FR-11:** Rate limiting: a user may post at most 5 comments per minute and 50 per day per account (HTTP 429 with `Retry-After`), to blunt spam. Exact limits configurable server-side.
- **FR-12:** Comment text shall be stored and returned as plain text; the API shall accept and round-trip Unicode, and the UI renders it with React escaping (never `dangerouslySetInnerHTML`).

### 5.4 Admin moderation (admin surface)

- **FR-13:** `GET /api/v1/admin/comments` shall list all comments platform-wide with filters: free-text `q` (matches content), `template_id`, `user_id` (author), sort by `created_at` DESC, paginated (default 20). Each row includes comment fields plus template title/owner and author display name/email for moderation context.
- **FR-14:** `DELETE /api/v1/admin/comments/{commentId}` (admin role required) shall delete any comment. The request body shall include a mandatory `reason` (3–500 chars, same rule as other admin actions); the action shall be recorded in `admin_audit_logs` (`action='delete_comment'`, `target_type='comment'`, `target_id=commentId`, plus the reason).
- **FR-15:** The admin template detail page (`/admin/templates/:id`) shall show that template's comments (read + delete with reason), reusing the same admin delete endpoint.
- **FR-16:** Admin APIs shall require the admin role exactly like existing `/api/v1/admin/*` endpoints (admin JWT); regular users receive 403.

### 5.5 Notifications integration

- **FR-17:** When a comment is created on a template owned by another user, the system shall emit a `comment` notification to the owner via the notifications feature's event mechanism (recipient = owner, actor = commenter, type = `comment`), subject to that feature's self-action suppression and dedupe rules. If the notifications feature has not shipped, this requirement is deferred until it does (no standalone fallback).

### 5.6 Behavior on related deletions

- **FR-18:** Deleting a template shall cascade-delete its comments (FK) and the `comment_count` goes away with the row; deleting a user shall cascade-delete their comments. Counters on affected templates shall be corrected by the same transactional maintenance (FR-2) or recomputed.

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Comment list endpoint p95 < 150 ms at 10k comments on a single template (indexed `(template_id, created_at DESC)`); posting p95 < 200 ms. `comment_count` on template lists adds no N+1 (denormalized column).
- **NFR-2 (Scalability):** Rate limits (FR-11) protect the write path; reads are stateless and cache-friendly. No new infrastructure (no search index — `q` filter uses SQL ILIKE on a bounded result set, consistent with existing admin search patterns).
- **NFR-3 (Compatibility):** Additive only — one new table, one new column on `templates`, new endpoints, one new section on the template detail page, one new admin page + one admin detail section. No changes to existing pages' behavior.
- **NFR-4 (Privacy):** Author exposure limited to `display_name` + `avatar` + user id (all already public on the platform); emails visible only in the admin moderation surface, matching existing admin user pages.
- **NFR-5 (Accessibility):** Comment list is a semantic list; the composer is a labeled textarea with character counter; all actions keyboard-reachable; focus management on delete follows the existing Modal pattern; WCAG 2.1 AA (consistent with the accessibility feature design).
- **NFR-6 (i18n):** All user-facing strings EN/ZH in sync; comment content is user content and never translated; timestamps via the shared relative-time util; the composer placeholder, validation messages, and moderation strings all localized.
- **NFR-7 (Security):** Auth on all writes; ownership/role checks server-side on every mutation; plain-text rendering (React escaping); admin actions audited with mandatory reason; rate limiting on the write path.
- **NFR-8 (Resilience):** Comment load failure never blocks the template detail page (section-level error + Retry); posting failures show inline errors and preserve the drafted text.

## 7. Acceptance Criteria

- **AC-1:** An authenticated user can post a comment on a public template; it appears in the list (newest-first) and `comment_count` increments, without a page reload.
- **AC-2:** Anonymous visitors can read comments on public templates but see a login prompt instead of a composer; private templates' comments are visible only to the owner.
- **AC-3:** Empty (0 chars) and > 2000 char comments are rejected with field-level errors; whitespace-only content is rejected.
- **AC-4:** The author can edit a comment within 15 minutes (shows "edited" marker afterwards); after the window, edit returns 403 with the explanatory message.
- **AC-5:** The author can delete their own comment; the row disappears and `comment_count` decrements.
- **AC-6:** The template owner can delete any comment on their template via a confirm modal; admins can delete any comment via the admin console with a mandatory reason.
- **AC-7:** A third party cannot edit or delete another user's comment (403/404); a banned user cannot post (403).
- **AC-8:** Exceeding 5 comments/minute returns 429 with `Retry-After`; the UI surfaces a friendly message and preserves the draft.
- **AC-9:** `/admin/comments` lists all comments with working q/template/author filters and pagination; deleting requires a reason and writes an `admin_audit_logs` row (`action='delete_comment'`), visible in the audit log page.
- **AC-10:** The admin template detail page shows the template's comments with the same delete-with-reason action.
- **AC-11:** Deleting a template removes its comments; deleting a user removes their comments; counters stay consistent.
- **AC-12:** All new UI strings exist in both `en` and `zh` translation files with identical key sets; the section renders correctly in both languages and both themes.
- **AC-13:** Comment text containing `<script>` or HTML renders as literal text (escaped), never executes.
- **AC-14:** With the comments API failing, the template detail page still renders fully with an inline error + Retry in the comments section.

## 8. Out of Scope (v1)

- Threading/nested replies (flat list only; a reply is just a comment).
- @mentions and mention notifications.
- Emoji reactions on comments.
- Rich text / Markdown rendering (plain text with preserved line breaks only).
- Comment voting/sorting by score (newest-first only).
- Pinned comments by the creator.
- Editing window longer than 15 minutes or edit history.
- Soft-delete with "deleted" placeholder rows.
- Anonymous/guest comments.
- Email digests of new comments.
- Automated content moderation/profanity filters (manual moderation only in v1).
- Comment notifications for commenters on their own comment threads (only the template owner is notified).
- Monetization-adjacent features of any kind (per project scope rule).

## 9. Dependencies & Assumptions

- **D-1:** `templates` already carries denormalized `like_count`/`favorite_count` maintained in repositories (confirmed in `deployment/init.sql` and `template_repository.go`); `comment_count` follows the identical pattern.
- **D-2:** `users.status` ('active'/'banned') and `users.role` ('user'/'admin') exist (confirmed); the banned check reuses the same status semantics as login/API-key flows.
- **D-3:** The admin console already has the list-page pattern (filters + pagination + table) in `UsersPage`/`TemplatesPage`, the reason-modal pattern from ban/unban, and the audit-log integration — the comments admin page reuses all three.
- **D-4:** The notifications feature (designed, in development) defines the event mechanism; FR-17 hooks into it. If it ships first, `comment` becomes a fourth event type alongside like/favorite/fork; its requirements doc lists "comment notifications" as out of scope *because comments did not exist* — this feature adds them.
- **D-5:** The template detail page already loads template + versions + prompts and has like/favorite/fork sections; the comments section is appended below the existing social stats, using the same data-fetching style (api service + local state).
- **D-6:** The REST gateway (`:8080`, `/api/v1`) routes to backend services via gRPC; new comment routes follow the existing template sub-resource pattern (`/templates/{id}/likes` style).
- **D-7:** Rate limiting infrastructure: the auth service already implements failed-login throttling with Redis; the same Redis-backed counter approach is assumed for comment rate limits (architect to confirm placement).

## 10. Open Questions for Architect

1. **Pagination style:** offset (`page`/`page_size`) vs. keyset cursor (`before_id`). Recommendation: offset for v1 (matches existing admin lists, simpler total_count), keyset later if hot templates churn fast.
2. **Service placement:** extend `PromptService` (comments are template sub-resources; recommended) vs. a new `CommentService`. Either is acceptable; the REST surface is identical.
3. **Rate-limit placement:** gateway middleware vs. service-level Redis counters. Recommendation: service-level, reusing the auth throttle pattern.
4. **`comment` notification type:** confirm adding a fourth type to the notifications design's event enum, or defer FR-17 to a notifications v2.
