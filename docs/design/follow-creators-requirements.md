# Follow Creators — Requirements Analysis

- **Feature ID:** follow-creators
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. Competitor Research Summary

| Platform | Follow approach | Key takeaways |
|---|---|---|
| **FlowGPT** | Follow creators from their profile/prompt pages; "Following" feed surfaces their new prompts | Follow is the retention backbone: it converts one-off discovery into a repeatable "come back and see what's new" loop |
| **Snack Prompt** | Follow authors; followed authors' activity appears on the profile/home activity area | Follow events double as social proof on profiles ("1.2k followers"); the follow button must be reachable from content pages, not only profiles |
| **PromptBase** | No follow; discovery is search/browse-centric | Search-only discovery loses repeat visitors; follow fills that gap without any commerce mechanics |
| **AIPRM** | No creator follow (directory of prompts, not people) | Prompt-centric platforms under-serve creators; a people layer raises creator investment |
| **GitHub** | Follow users (activity feed); Watch repos | Two distinct verbs (follow person vs watch artifact) — for v1 a single "follow creator" verb is enough; watching individual templates is a separate future feature |
| **Twitter/X** | Follow/unfollow toggle, follower/following counts on profile, "Follows you" badge | The toggle + counts + reciprocal badge pattern is the universal grammar users already know |
| **YouTube** | Subscribe with bell; subscription feed | Subscription feed ordering (newest first) is the simplest viable aggregation; algorithmic feeds are v2+ |

**Synthesis:** The winning v1 pattern is Twitter/GitHub-style: a `user_follows` table (follower → followee), a Follow/Following toggle button on creator surfaces (template detail, and the creator profile page introduced by this feature), follower/following counts on profiles, a "Following" feed of followed creators' public templates (newest first), and a `follow` notification event so creators learn they have an audience. Open Prompts currently has no people layer at all: `owner_id` is rendered as a bare ID string on the template detail page ("by {owner_id}") — users cannot even see who created a template, let alone keep up with them.

## 2. Problem Statement

1. **No people layer.** The product is template-centric only. The template detail page shows the owner as a raw user ID; there is no creator profile page, no creator identity, and no way to explore a creator's other work.
2. **No retention loop for consumers.** A user who finds a great creator has no way to see that creator's next template. Every visit restarts discovery from scratch.
3. **No audience for creators.** Creators have no follower count, no sense of audience, and no signal that people want more of their work — the strongest motivator for continued contribution.
4. **No follow notification.** The notifications feature (in development) covers like/favorite/fork; gaining a follower is arguably the strongest engagement event and is missing.

## 3. Target Users

- **Consumers (primary):** users who find quality creators and want a persistent, low-effort way to keep up with their new public templates.
- **Creators:** users who want an audience, a follower count as social proof, and notification when someone follows them.
- **All users:** every user is a potential creator (anyone can publish), so every profile is followable; users with zero followers see a friendly empty state.
- **Admins:** no admin console surface in v1 — follows are personal social graph data, not content to moderate. (Admin user detail pages may show follower/following counts as read-only context; see FR-17.)

## 4. User Stories

### 4.1 Following

- **US-F1:** As a consumer, I want to follow a creator from the template detail page, so that I can keep up with their future templates.
- **US-F2:** As a consumer, I want to unfollow just as easily, so that following stays low-commitment.
- **US-F3:** As a user, I want to see whether I already follow someone (button state), so that the state is always clear.
- **US-F4:** As a user, I cannot follow myself, so that counts stay honest.
- **US-F5:** As an anonymous visitor, I am prompted to log in when trying to follow, so that the flow is understandable.
- **US-F6:** As a banned user, I cannot follow or be followed (new follows rejected), consistent with my restricted account state.

### 4.2 Creator profile

- **US-F7:** As a visitor, I want a public creator profile page showing avatar, display name, bio, follower/following counts, and public templates, so that I can evaluate and explore a creator's work.
- **US-F8:** As a creator, I want to write a short bio on my profile, so that followers know who I am.
- **US-F9:** As a creator, I want my public templates on my profile (not private ones), so that my profile respects template visibility.
- **US-F10:** As a user, I want to see "Follows you" on profiles that follow me, so that I can recognize reciprocal interest.

### 4.3 Following feed

- **US-F11:** As a consumer, I want a "Following" feed of public templates from creators I follow, newest first, so that I never miss new work.
- **US-F12:** As a consumer, I want the feed to exclude private templates automatically, so that I only ever see what creators chose to publish.
- **US-F13:** As a consumer, I want an empty state that suggests creators to follow, so that a fresh account is not a dead end.

### 4.4 Managing my graph

- **US-F14:** As a user, I want a "Following" list on my own profile, so that I can review and unfollow creators.
- **US-F15:** As a user, I want a "Followers" list on my own profile, so that I can see my audience.

### 4.5 Notifications

- **US-F16:** As a creator, I want to be notified when someone follows me, so that I know my work is building an audience. *(depends on the notifications feature; see §9)*

### 4.6 System

- **US-F17:** As the system, I enforce that follow relationships are exactly one row per (follower, followee) pair, so that counts are accurate.
- **US-F18:** As the system, I never expose follower/followee emails or private data, so that the social graph stays privacy-safe.
- **US-F19:** As an admin, I can see a user's follower/following counts on the admin user detail page, so that I have moderation context.

## 5. Functional Requirements

### 5.1 Data model

- **FR-1:** A `user_follows` table shall store: `follower_id` (FK → users, `ON DELETE CASCADE`), `followee_id` (FK → users, `ON DELETE CASCADE`), `created_at` (TIMESTAMPTZ), `PRIMARY KEY (follower_id, followee_id)`, with `CHECK (follower_id <> followee_id)`. Index on `followee_id` for follower-list queries (the PK covers following-lists).
- **FR-2:** `users.follower_count` and `users.following_count` (INT, default 0) shall be maintained transactionally on follow/unfollow (same denormalized-counter pattern as `templates.like_count`), so profiles and lists render counts without COUNT(*) per row.
- **FR-3:** `users.bio` (TEXT, nullable, max 280 chars) shall be added for the profile page; set/updated via the existing profile update flow.

### 5.2 Follow/unfollow (regular-user surface)

- **FR-4:** `POST /api/v1/users/{userId}/follow` (auth required) shall create a follow (follower = token user). Idempotent: following an already-followed user returns 200 with the current state (no duplicate row, no double count). Returns `{following: true, follower_count}`.
- **FR-5:** `DELETE /api/v1/users/{userId}/follow` (auth required) shall remove the follow. Idempotent: unfollowing a non-followed user returns 200 with `{following: false, follower_count}`. Deleting a non-existent user returns 404.
- **FR-6:** The system shall reject self-follow with 400 and a clear message. Banned followees or banned followers shall reject new follows with 403 (existing relationships are retained; banned users' templates are excluded from following feeds per FR-12 and FR-18).
- **FR-7:** Rate limiting: at most 30 follow/unfollow actions per minute per account (HTTP 429 with `Retry-After`), to blunt follow-spam. Configurable server-side.

### 5.3 Creator profile (regular-user surface)

- **FR-8:** `GET /api/v1/users/{userId}/profile` shall be public (anonymous readable) and return: `id`, `display_name`, `avatar`, `bio`, `follower_count`, `following_count`, `created_at` (join date), and `is_following` (bool, false for anonymous) + `follows_you` (bool, false for anonymous) for the caller. Never email, mobile, or role.
- **FR-9:** `GET /api/v1/users/{userId}/templates` shall be public and return the user's public templates only (paginated, newest first), reusing the existing template list shape (so PromptCard renders unchanged). Private templates are never listed for non-owners.
- **FR-10:** `GET /api/v1/users/{userId}/followers` and `GET /api/v1/users/{userId}/following` shall be public, paginated (default 20), returning public user cards (`id`, `display_name`, `avatar`, `follower_count`, `is_following` for the caller) — never email.
- **FR-11:** The profile update flow (`PUT /api/v1/users/me` or the existing profile endpoint) shall accept `bio` (0–280 chars after trim; empty clears it) with the same validation error shape as other profile fields.

### 5.4 Following feed (regular-user surface)

- **FR-12:** `GET /api/v1/users/me/following-feed` (auth required) shall return public templates from all followed creators, newest first, paginated (default 20, max 50). Private templates are excluded server-side regardless of any stale follow relationship. Templates from banned creators are excluded. Deleted templates disappear naturally.
- **FR-13:** The feed response shall use the existing template list item shape plus the owner's `display_name`/`avatar` (so cards can show "by Alice" instead of a raw ID), and `total_count` for pagination UI.

### 5.5 Template detail integration

- **FR-14:** The template detail response (`GET /api/v1/templates/{id}`) shall include an `owner` object (`id`, `display_name`, `avatar`, `follower_count`, `is_following` for the caller) so the page can render a real creator identity and follow button without extra calls.
- **FR-15:** The template list response used by the home square shall include owner `display_name` (and optionally avatar) per item, so cards can show "by Alice"; this is a display-only enrichment, no behavior change.

### 5.6 Notifications integration

- **FR-16:** When a user follows another user, the system shall emit a `follow` notification to the followee via the notifications feature's event mechanism (recipient = followee, actor = follower, type = `follow`), subject to that feature's self-action suppression rules (self-follow is already impossible). Unfollow shall NOT emit a notification. If the notifications feature has not shipped, this requirement is deferred until it does.

### 5.7 Admin surface (read-only)

- **FR-17:** The admin user detail response (`GET /api/v1/admin/users/{id}`) shall include `follower_count` and `following_count` as read-only context fields. No admin follow-management endpoints in v1 (follows are personal graph data; there is nothing to moderate — no content, no text).
- **FR-18:** Banning a user shall not delete their follow graph (they may return after unban); new follows involving a banned user are rejected per FR-6, and their templates are excluded from following feeds per FR-12.

### 5.8 Behavior on deletions

- **FR-19:** Deleting a user shall cascade-delete their follow rows (both directions) and the counters on affected users shall be corrected by the same transactional maintenance (FR-2) or recomputed.

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Profile, followers/following lists, and feed endpoints p95 < 150 ms at 100k follow rows (PK/index-backed); follow/unfollow p95 < 200 ms. Denormalized counts avoid per-row COUNT(*).
- **NFR-2 (Scalability):** Rate limiting (FR-7) protects the write path. The following-feed query is a join of `user_follows` × `templates` on indexed keys; if it ever becomes hot, a materialized feed is a v2 optimization (open question §10.3).
- **NFR-3 (Compatibility):** Additive only — one new table, three new columns, new endpoints, one new page (`/creators/:id`), one new feed surface, and display-only enrichment of existing responses. The existing "by {owner_id}" rendering is replaced by the owner object; no other page changes behavior.
- **NFR-4 (Privacy):** Public surfaces expose only `id`, `display_name`, `avatar`, `bio`, counts, join date, and caller-relative follow flags; never email, mobile, role, or private templates. Follow lists are public (consistent with every competitor), but contain only public user cards.
- **NFR-5 (Accessibility):** Follow button states (follow/following/hover-unfollow) are text-labeled, not color-only; profile and feed pages are fully keyboard-operable; WCAG 2.1 AA consistent with the accessibility feature design.
- **NFR-6 (i18n):** All user-facing strings EN/ZH in sync; bios and display names are user content and never translated; counts use locale-aware pluralization and number formatting (1.2k style abbreviations in both locales).
- **NFR-7 (Security):** Auth on all writes; server-side validation of every action; no user-controlled markup rendered (React escaping); rate limiting on the follow/unfollow path.
- **NFR-8 (Resilience):** Follow button failures show a toast and revert the visual state; feed/profile load failures show section-level error + Retry and never break navigation.
- **NFR-9 (Data integrity):** Counts and rows stay consistent under concurrent follow/unfollow (transactional counter maintenance); idempotent endpoints make client retries safe.

## 7. Acceptance Criteria

- **AC-1:** An authenticated user can follow and unfollow a creator from the template detail page and the creator profile; the button state and follower count update immediately and survive reload.
- **AC-2:** Anonymous visitors can view creator profiles, public templates, and follower/following lists, but see a login prompt when trying to follow.
- **AC-3:** Self-follow returns 400; banned users cannot follow or gain new followers (403).
- **AC-4:** Repeated follow/unfollow calls are idempotent — no duplicate rows, no count drift.
- **AC-5:** `/creators/:id` shows avatar, display name, bio, join date, follower/following counts, Follow/Following button (with "Follows you" badge when applicable), and the creator's public templates; private templates never appear.
- **AC-6:** The "Following" feed lists public templates from followed creators newest-first, excludes private and banned creators' templates, and paginates correctly.
- **AC-7:** A user's own profile shows their Following and Followers lists with working unfollow from the list.
- **AC-8:** The template detail page shows the creator's identity (avatar + display name linking to `/creators/:id`) instead of a raw owner ID, plus a working follow button.
- **AC-9:** Following someone emits a `follow` notification to them (once notifications ships); unfollowing emits nothing.
- **AC-10:** Exceeding 30 follow actions/minute returns 429 with `Retry-After`; the UI surfaces a friendly message.
- **AC-11:** The admin user detail page shows follower/following counts; no admin follow-management endpoints exist (404 on any attempted admin follow route).
- **AC-12:** Deleting a user removes their follow rows in both directions and corrects affected counts.
- **AC-13:** All new UI strings exist in both `en` and `zh` translation files with identical key sets; profile, feed, and lists render correctly in both languages and both themes.
- **AC-14:** With follow APIs failing, the template detail page and home still render fully; follow buttons revert with an error toast; feed/profile show error + Retry.
- **AC-15:** No email, mobile, or role is ever present in any public profile, follower, following, or feed response (verified by response inspection).

## 8. Out of Scope (v1)

- Follow recommendation algorithms ("suggested for you" beyond a simple recent-creators fallback in the empty state).
- Blocking users / block lists (a separate safety feature requiring its own design).
- Muting creators (keep feed minus someone) — unfollow is the v1 answer.
- Follow requests/approval (private accounts) — all profiles are public in v1.
- "Watch template" (follow an individual template for version updates) — distinct verb, separate future feature.
- Feed ranking/relevance algorithms (strict newest-first).
- Feed aggregation or digest emails.
- Creator analytics about followers (who followed when) — the prompt-analytics feature covers template metrics.
- Admin follow management or moderation endpoints.
- Import/export of the follow graph.
- Any monetization-adjacent features (per project scope rule).

## 9. Dependencies & Assumptions

- **D-1:** `users` table exists with `display_name` NOT NULL and nullable `avatar` (confirmed in `deployment/init.sql`); adding `bio`, `follower_count`, `following_count` follows the same `ALTER TABLE ... IF NOT EXISTS` migration pattern already used for `avatar`, `role`, `status`.
- **D-2:** The denormalized counter pattern (`like_count`/`favorite_count` on templates, maintained in repositories) is the established pattern; `follower_count`/`following_count` follow it.
- **D-3:** The template detail page currently renders `t('template_details.by', { owner: template.owner_id })` — a raw ID. The `owner` object (FR-14) replaces this; the i18n key gains a display-name parameter. Home square cards get the same enrichment (FR-15).
- **D-4:** The notifications feature (designed, in development) defines the event mechanism; FR-16 hooks into it as a fourth/fifth event type (`follow` alongside like/favorite/fork and the comments feature's `comment`). Its requirements doc lists "follow notifications" as out of scope *because the follow feature did not exist* — this feature adds them.
- **D-5:** Routing: `/creators/:id` is a new public route in `App.js` alongside `/templates/:id`; the home square gains a "Following" entry point (tab or section — UI/UX doc §4.2) consistent with the existing navigation pattern.
- **D-6:** The REST gateway routes `/api/v1/users/*` today for profile operations; follow endpoints extend that prefix. The following-feed is a `me`-scoped endpoint consistent with `GET /api/v1/users/me` style.
- **D-7:** Rate limiting reuses the Redis-backed counter approach from the auth service's login throttle (same assumption as the comments feature's D-7).
- **D-8:** `PromptCard` renders template list items; adding an owner line is a display-only change that must not break existing card usage (square, collections, search results).

## 10. Open Questions for Architect

1. **Service placement:** extend `UserService` (follows are user relationships; recommended) vs. a new `FollowService` vs. `PromptService` (the feed queries templates). Recommendation: `UserService` for graph + follow actions; the feed query can live there too (it reads templates via the existing repository) or in `PromptService` — architect's call.
2. **Feed shape:** dedicated `following-feed` endpoint (recommended — clear contract, `me`-scoped) vs. a `following=true` filter on the existing template list endpoint.
3. **Feed scale:** at what follow-graph size should the newest-first join be replaced by a materialized/fan-out feed? v1 assumes the join is fine (indexed, paginated); a threshold or metric should be agreed.
4. **`follow` notification type:** confirm adding it to the notifications design's event enum (alongside `comment` from the comments feature), or defer FR-16 to a notifications v2.
5. **Owner enrichment scope (FR-15):** enrich all template list responses (home, search, collections) or only the detail page in v1? Recommendation: all lists — the join is cheap and "by Alice" on cards is a large UX win.
