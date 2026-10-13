# Notification Source Preferences — Requirements Analysis

- **Feature ID:** notification-source-preferences
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-13
- **Companion doc:** `notification-source-preferences-ui-ux-design.md`

## 1. Competitor Research

| Product | Per-source control | Strength | Weakness |
|---|---|---|---|
| YouTube | Per-channel bell: all / personalized / none | Most-used control after global; three levels match real intent | Levels are opaque ("personalized"?) |
| Reddit | Per-community overrides on top of per-type toggles | Global first, override second — correct layering | Two settings surfaces confuse |
| GitHub | Watch dropdown per repo: all / participating / ignore | Precise; "participating" is well-understood | Per-repo only, no per-actor |
| Twitter/X | Per-account: off / accounts I follow / everyone | Simple trichotomy | Buried in notification item overflow |
| Instagram | Per-account: off / mute story/reels/posts separately | Fine-grained per source | Too many sub-toggles |

**Takeaway:** notification-preferences shipped per-type toggles (like/favorite/fork/comment/follow + pause-all) and explicitly deferred per-source granularity as v2 (line 102). The YouTube/GitHub pattern is the right v2: a **per-source (per-creator) level** — `all` (default, current behavior) / `mentions_only` (only events where you're directly addressed) / `none` — set from the notification item itself or the creator's profile, layered *on top of* per-type toggles (most restrictive wins). No per-template granularity (GitHub's repo-level is the closest analog, but template volume makes it noise; deferred).

## 2. Problem Statement

Per-type toggles are all-or-nothing: a user who wants likes from most creators but is flooded by one prolific creator must turn off all like notifications. The missing control is per-source: "mute this person" or "only when I'm mentioned". YouTube's data (per-channel bells are the most-used control after global) says this is the highest-value v2 preference.

## 3. Target Users

- **Notification recipients** — quiet one noisy creator without losing the firehose; keep mentions from everyone.
- **Creators (as actors)** — unaffected; preferences are receiver-side only.

## 4. User Stories

- **US-1**: As a recipient, I want to set a per-creator level (all / mentions only / none) from a notification's overflow menu, so that muting is one click where the noise is.
- **US-2**: As a recipient, I want the same control on the creator's profile page, so that I can pre-emptively manage sources.
- **US-3**: As a recipient, I want "mentions only" to keep @mentions and replies while dropping engagement events from that creator, so that conversation survives muting.
- **US-4**: As a recipient, I want to see and edit all my per-source levels in one list, so that I can revisit past decisions.
- **US-5**: As a recipient, I want per-source levels to compose with per-type toggles (most restrictive wins), so that behavior is predictable.
- **US-6**: As a recipient, I want a muted creator's existing notifications to stay in history (no retroactive deletion), so that muting isn't data loss.
- **US-7**: As a recipient, I want the level to apply to future events only, so that the semantics are clear.

## 5. Functional Requirements

- **FR-1**: New table `notification_source_preferences`: `user_id` (the recipient), `source_user_id` (the actor/creator), `level` (`all` | `mentions_only` | `none`), `updated_at`; PK `(user_id, source_user_id)`. Absence of a row = `all` (default; no backfill).
- **FR-2**: Levels apply at **event-emission time** (same model as per-type toggles): a suppressed event never creates a notification row, keeping history clean and the unread badge accurate.
- **FR-3**: Level semantics per event type: `none` suppresses all events sourced from that creator (like, favorite, fork, comment, reply, follow, mention); `mentions_only` suppresses engagement events (like/favorite/fork/follow) but keeps mention and reply events; `all` suppresses nothing.
- **FR-4**: Composition rule: an event is emitted only if BOTH the per-type toggle is on AND the per-source level allows it (most restrictive wins). Per-source never *enables* what a per-type toggle disabled.
- **FR-5**: Entry points: (a) overflow menu on any notification item whose actor is a creator → "Notifications from this user" submenu with the three levels; (b) the creator's profile page → bell dropdown (YouTube pattern) with the three levels; (c) the notifications settings panel → "Per-source preferences" list.
- **FR-6**: APIs: `GET /api/v1/notification-preferences/sources?page_size=50&page_token=...` (list non-default rows, newest-updated first), `PUT /api/v1/notification-preferences/sources/{user_id}` (body: `{level}`; setting `all` deletes the row), `GET /api/v1/notification-preferences/sources/{user_id}` (current level, for profile rendering).
- **FR-7**: The per-source list in settings shows: creator avatar + display name, current level (dropdown), updated time, and a link to the profile; paginated 50/page; empty state explains the feature.
- **FR-8**: Muting is receiver-side only: the muted creator is never informed, and their ability to interact (comment, like) is unaffected — only notification emission changes.
- **FR-9**: Existing notifications from a newly-muted creator remain in history; muting applies to future events only (FR-2 emission-time model gives this for free).
- **FR-10**: Self-preference is impossible: the API rejects `source_user_id == user_id` with 400; the UI never offers the control on one's own profile.
- **FR-11**: Block-users interplay: an existing block already suppresses notifications both directions (block-users FR-9); a per-source level on a blocked user is redundant but harmless — the block wins regardless of level.
- **FR-12**: The bell dropdown on profiles shows the current level with a checkmark; changing it is immediate (auto-save, no confirm) with a brief toast.
- **FR-13**: Limits: at most 500 per-source preference rows per user (server-enforced; friendly error) — protection against unbounded rows.
- **FR-14**: All levels are per-recipient; nothing about a creator's global reach changes.

## 6. Non-Functional Requirements

- **NFR-1 (Performance)**: Emission-time check is a single indexed lookup on `(user_id, source_user_id)`; adds < 2 ms to notification creation.
- **NFR-2 (Predictability)**: The composition rule (most restrictive wins) is stated in the UI so users can reason about behavior.
- **NFR-3 (i18n)**: All UI strings EN/ZH; level names are translatable.
- **NFR-4 (Privacy)**: Levels are private to the recipient; no surface reveals them to the source.
- **NFR-5 (Consistency)**: Same emission-time suppression model as notification-preferences — no second suppression path.

## 7. Acceptance Criteria

- **AC-1**: Setting a creator to `none` from a notification's overflow stops future like/comment/follow events from them; existing rows remain.
- **AC-2**: `mentions_only` keeps a reply from that creator arriving as a notification but drops their like on the same template.
- **AC-3**: With the per-type `like` toggle off and a source at `all`, no like notification is emitted (per-type wins).
- **AC-4**: The profile bell dropdown shows the current level with a checkmark; changing to `none` persists immediately with a toast.
- **AC-5**: The settings list shows all non-default rows with level dropdowns; changing a level to `all` removes the row (list shrinks).
- **AC-6**: `PUT` with `source_user_id == user_id` returns 400.
- **AC-7**: A blocked user's events are suppressed regardless of their per-source level.
- **AC-8**: The 501st per-source row creation fails with a friendly limit error.
- **AC-9**: The muted creator's interactions (a like, a comment) still appear on the template — only the notification is suppressed.
- **AC-10**: Pagination works at 50/page in the settings list.

## 8. Out of Scope

- Per-template granularity (GitHub repo-watch analog) — template volume makes it noise; revisit if creators ask.
- Custom levels (e.g. "only forks"), time-boxed mutes ("mute for 7 days"), or keyword filters.
- Email/push channels (in-app only, per notification-preferences).
- Revealing mute status to the source (never — privacy).
- Bulk mute (multi-select in the settings list).
- Monetization of any kind — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1**: notification-preferences' per-type toggles, settings panel, and emission-time suppression model exist as designed.
- **D-2**: The notifications event types (like, favorite, fork, comment, reply, follow, mention) are registered in the data-driven type registry.
- **D-3**: block-users FR-9 bidirectional suppression is in place (FR-11 interplay).
- **D-4**: Creator profile pages exist (follow-creators) and host the bell dropdown.
- **D-5**: The notification item overflow menu exists (mark read/delete) and gains the submenu.

## 10. Open Questions for Architect

- **OQ-1**: Should the emission-time check join the existing per-type check in one query, or stay a separate lookup? (Proposal: one query — the per-type row and per-source row can be fetched together.)
- **OQ-2**: Is `mentions_only` the right middle level, or should it be `comments_only` (comments + mentions + replies)? (Proposal: mentions_only — replies and mentions are the "addressed me" set.)
- **OQ-3**: Should the 500-row limit (FR-13) evict oldest-updated rows automatically or hard-fail? (Proposal: hard-fail with guidance to clean up.)
- **OQ-4**: Does the profile bell dropdown need an "all" → row-deletion optimization, or is an explicit `all` row acceptable? (Proposal: delete the row — keeps the table sparse.)
- **OQ-5**: Should the settings list support search/filter by creator name at 500 rows, or is pagination enough for v2?
