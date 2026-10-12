# Block Users — Requirements Analysis

- **Feature ID:** block-users
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. Competitor Research Summary

| Platform | Blocking approach | Key takeaways |
|---|---|---|
| **Twitter/X** | Block is silent to the blocker, invisible-ish to the blocked (they can visit but not follow/interact); block list in settings | Silent-to-blocker is the standard; the blocked party is not notified |
| **Instagram** | Block hides both profiles from each other; comments/likes from the blocked user are hidden | Mutual invisibility is simpler to reason about than selective hiding |
| **GitHub** | No user blocking (repo-level only); relies on moderation | A cautionary tale — creator harassment has no user-level answer |
| **Reddit** | "Block" mutes (hides their content from you); they can still see yours | The weakest interpretation; users expect more |
| **Mastodon** | Block + optional silence; blocked user cannot follow or mention you | Blocking severs the follow graph — the most important effect |
| **FlowGPT / Snack Prompt / PromptBase** | No blocking visible | Prompt platforms have not solved this; copying the social-network pattern is the answer |

**Synthesis:** Open Prompts is acquiring a social layer (comments, follows, creator profiles at `/creators/:id`, notifications). Every social layer needs a personal safety valve that does not require admin intervention. The winning v1 pattern is the Twitter/Mastodon hybrid: a `user_blocks` table (blocker → blocked); blocking **severs the follow graph in both directions** (auto-unfollow both ways), **hides the blocked user's content from the blocker** (their templates out of the blocker's lists/search/feeds, their comments collapsed), **prevents interaction** (no follow, no like/favorite, no comment on the blocker's templates), and is **silent** (no notification to the blocked user; they are never told). The blocked user can still *see* the blocker's public templates (public content stays public — v1 does not do Instagram-style mutual invisibility; documented as out of scope) but cannot interact with them. A **Blocked accounts** management list lives in the existing Profile page (settings section). No admin surface: blocking is personal graph data, and abuse of blocking (e.g., block-everyone) harms only the blocker's own experience.

## 2. Problem Statement

1. **No personal safety valve.** With comments, follows, and notifications landing, a user harassed or annoyed by another user has no self-service remedy — they must report to admins (content-reporting) and wait.
2. **Follow graph has no exit hatch.** A user who regrets a follow or wants to sever a connection can unfollow, but the other party can simply re-follow; there is no "stop this person from following me."
3. **Content exposure is unavoidable.** A blocked-by-intention user's templates still flood the blocker's search results, tag pages, and following feed; their comments still appear on shared templates.
4. **Admins are the only enforcement.** Every interpersonal conflict currently routes through admin moderation — slow, heavy, and unnecessary for personal-preference blocking.

## 3. Target Users

- **Regular users (primary):** anyone who wants a persistent, personal "not this person" control — harassment victims, spam-averse users, or simple preference.
- **Creators (secondary):** template owners who want to stop a specific user from commenting on or liking their work.
- **Admins (relief):** fewer interpersonal complaints routed to moderation; blocking handles personal preference, reporting handles policy violations.

## 4. User Stories

- **US-B1:** As a user, I can block another user from their profile page or the ⋯ menu on their comment, so I can stop unwanted attention without admin help.
- **US-B2:** As a user, blocking severs follows in both directions (they stop following me, I stop following them), so the connection is fully cut.
- **US-B3:** As a user, a blocked user's templates disappear from my search results, tag/category pages, and following feed, so my browsing is clean.
- **US-B4:** As a user, a blocked user's comments are collapsed to a single "Blocked" line on templates I view, so threads stay readable without showing me their content.
- **US-B5:** As a creator, a blocked user cannot comment on, like, favorite, or follow me or my templates, so the block actually stops interaction.
- **US-B6:** As a user, blocking is silent — the blocked user is never notified and cannot tell exactly when it happened.
- **US-B7:** As a user, I can view and manage my blocked list in my Profile settings, and unblock if I change my mind.
- **US-B8:** As a user, I cannot block admins or myself; the system rejects it with a clear message.
- **US-B9:** As a blocked user, I can still view the blocker's public templates (public is public), but every interaction endpoint returns a generic error, so I learn the action failed without learning why.

## 5. Functional Requirements

### 5.1 Data model

- **FR-1:** New table `user_blocks`: `blocker_id` (FK → users, `ON DELETE CASCADE`), `blocked_id` (FK → users, `ON DELETE CASCADE`), `created_at` TIMESTAMPTZ, `PRIMARY KEY (blocker_id, blocked_id)`, `CHECK (blocker_id <> blocked_id)`. Index on `blocked_id` for reverse lookups.
- **FR-2:** Blocking is **not** denormalized onto `users` (no block_count); the table is the source of truth. Blocking does not affect either user's follower/following counters beyond the auto-unfollow side effects (FR-4).

### 5.2 Blocking actions

- **FR-3:** `POST /api/v1/users/{userId}/block` (auth required) creates a block (blocker = token user). Idempotent: blocking an already-blocked user returns 200 with `{blocked: true}`. Self-block returns 400; blocking an admin returns 403 with a clear message; blocking a non-existent user returns 404.
- **FR-4:** On block creation, the system transactionally removes any follow rows in **both directions** (blocker→blocked and blocked→blocker) and adjusts follower/following counters accordingly (reusing the follow feature's counter maintenance).
- **FR-5:** `DELETE /api/v1/users/{userId}/block` (auth required) removes the block. Idempotent. Unblocking does **not** restore any follow relationship.
- **FR-6:** Rate limiting: at most 20 block/unblock actions per minute per account (HTTP 429 with `Retry-After`), to blunt block-spam.

### 5.3 Interaction enforcement (server-side)

- **FR-7:** While a block exists from A to B: B cannot follow A (403), B cannot like/favorite A's templates (403), B cannot comment on A's templates (403), B cannot fork A's templates (403). Errors are generic ("Action not available") — they never reveal that a block exists.
- **FR-8:** A cannot be affected in reverse: A may still see B's public templates (public content is public) and may comment on B's templates (blocking is a shield, not a sword — A blocking B does not silence A on B's content; documented in §8 as a v2 question).
- **FR-9:** Blocking suppresses notifications in both directions: B's actions generate no notifications for A (they cannot act anyway per FR-7), and A's actions generate no notifications for B (A liking B's template does not notify B).

### 5.4 Content visibility (blocker's view)

- **FR-10:** For the blocker (A), B's templates are excluded from: home square lists, search results, tag/category filter pages, the following feed, related-templates rails, and "More from author" rows. Direct navigation to a B template's URL still works (deep links are not broken; the page renders with a subtle "You blocked this creator" notice).
- **FR-11:** For the blocker (A), B's comments on any template are collapsed to a single line "Comment from a blocked account" (expandable via an explicit click to reveal, per accessibility pattern for progressive disclosure).
- **FR-12:** B's profile page (`/creators/:id`) is viewable by A but shows a banner "You have blocked this creator" with an Unblock button; B's follower/following lists remain visible (public data).
- **FR-13:** The blocked user (B) sees no difference anywhere: A's templates appear normally in B's lists/search (public content), A's profile renders normally, and B's interaction attempts fail generically per FR-7. B is never notified of the block.

### 5.5 Management surface

- **FR-14:** The Profile page gains a **"Blocked accounts"** settings section: a searchable list of blocked users (avatar, display name, blocked date), each with an Unblock button. Empty state explains what blocking does.
- **FR-15:** Block entry points: the ⋯ menu on the creator profile page (`/creators/:id`) and the ⋯ menu on any comment (comments feature). Both open a confirmation modal ("Block {name}? They will no longer be able to follow you or interact with your content. They are not notified.") with Block/Cancel.
- **FR-16:** No admin surface: blocks are personal graph data. The admin user detail page does not show block lists (privacy of the blocker). Admins cannot be blocked (FR-3).

### 5.6 API summary

- **FR-17:** `GET /api/v1/users/me/blocks` (auth) returns the caller's blocked list (paginated, public user cards + blocked_at). `POST/DELETE /api/v1/users/{userId}/block` per FR-3/5. Template list/search/feed endpoints gain server-side block filtering (FR-10) — no new parameters; filtering is automatic for authenticated callers.

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Block/unblock p95 < 200 ms; block filtering on lists/search adds < 10 ms per query (anti-join on the PK-indexed `user_blocks`).
- **NFR-2 (Silence):** No code path may notify a user that they were blocked or unblocked; notification suppression (FR-9) is enforced at the event-emission layer.
- **NFR-3 (Atomicity):** Block creation + bidirectional unfollow (FR-4) is one transaction; a failure leaves no partial state.
- **NFR-4 (Privacy):** The blocked list is private to the blocker (only `GET /users/me/blocks`); no public endpoint reveals who blocks whom. Block existence is never disclosed via error messages (FR-7).
- **NFR-5 (i18n):** All block UI strings translated (EN/ZH); the generic interaction error is a single shared key.
- **NFR-6 (Accessibility):** Block confirmation modal follows the accessible Modal pattern; collapsed blocked comments are keyboard-expandable; the blocked list is a standard accessible table/list.
- **NFR-7 (Compatibility):** Additive only — one new table, new endpoints, list-query filters, and UI affordances; no existing response shape changes (block filtering is removal-only).

## 7. Acceptance Criteria

- **AC-1:** Blocking from a profile or comment ⋯ menu opens the confirmation modal; confirming creates the block and shows a success toast.
- **AC-2:** After A blocks B, B's follows on A and A's follows on B are both gone; counters are correct.
- **AC-3:** After A blocks B, B's templates vanish from A's home/search/tag/following-feed/related surfaces; direct URL access still renders with the "blocked creator" notice.
- **AC-4:** B's comments render collapsed ("Comment from a blocked account") for A on every template; one click expands.
- **AC-5:** B's attempts to follow/like/favorite/comment/fork on A's content all fail with the generic error; B sees no block-specific message.
- **AC-6:** B receives no notification when blocked or unblocked; A's later actions on B's content generate no notifications for B.
- **AC-7:** The Profile "Blocked accounts" section lists blocked users with Unblock; unblocking removes the row and does not restore follows.
- **AC-8:** Self-block and admin-block attempts show clear errors; repeated block of the same user is a no-op returning 200.
- **AC-9:** A's view of B's profile shows the "You have blocked this creator" banner with Unblock; B's view of A's profile is unchanged.
- **AC-10:** All block UI strings render in EN and ZH.

## 8. Out of Scope

- Mutual invisibility (Instagram-style: blocked user cannot see blocker's public content) — v1 keeps public content public; revisit if harassment reports demand it.
- Blocking as a sword (A blocking B also silences A on B's content — FR-8 keeps A's speech) — v2 question.
- Block lists visible to admins; admin ability to dissolve blocks.
- Domain/email-based blocking, IP blocking, shadow-banning.
- Muting (hide content without severing follows) — a lighter future feature.
- Blocking from template cards (only profile + comment entry points in v1).
- Monetization of any kind (payments, pricing, purchases, earnings) — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1:** The follow-creators feature's `user_follows` table, counter maintenance, and follow/unfollow endpoints exist (or will); FR-4 reuses the unfollow path transactionally.
- **D-2:** The comments feature's comment rendering (with ⋯ menus from content-reporting) provides the comment-level block entry point; if comments has not shipped, the comment entry point is deferred and profile blocking ships alone.
- **D-3:** The creator profile page (`/creators/:id`) from follow-creators provides the profile block entry point and the banner surface.
- **D-4:** Template list/search queries are centralized in the repository layer (single choke point) so block filtering (FR-10) is one anti-join addition, not N per-surface patches.
- **D-5:** The notifications feature's event-emission layer can suppress events by (actor, recipient) pair — a small extension of its existing self-action suppression.
- **D-6:** The related-templates and "More from author" endpoints (related-templates feature) apply the same block filter as other lists.

## 10. Open Questions for Architect

1. **Filter scope:** FR-10 excludes blocked users' templates from lists/search/feed/related for the blocker. Should the *blocked* user (B) also see A's templates removed from their lists (symmetric filtering), or is blocker-only filtering correct for v1? I recommend blocker-only (public content stays public; symmetric filtering leaks block existence).
2. **Anti-join shape:** `NOT EXISTS (SELECT 1 FROM user_blocks WHERE blocker_id = $caller AND blocked_id = templates.owner_id)` added to every list query vs a materialized "hidden users" per-caller cache. I recommend the anti-join (PK-indexed, cheap at current scale).
3. **Comment collapse:** server-side (comments API returns `is_blocked` flag per comment for the caller) vs client-side (fetch block list, filter client-side). I recommend server-side flag (no extra round-trip, no block-list leak via API).
4. **Fork enforcement (FR-7):** fork is a copy operation — should blocked users be unable to fork the blocker's public templates? I recommend yes (interaction completeness), but it is arguable since forking is read-then-create.
5. **Notification suppression (FR-9):** suppress at emission (check block table before insert) vs at read (filter notifications list). I recommend at emission (cheaper, no stale rows).
6. **Admin immunity (FR-3):** blocking admins returns 403 — confirm, or allow blocking admins too (they are also users)? I recommend immunity to avoid support dead-ends.
