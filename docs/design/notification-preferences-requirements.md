# Notification Preferences — Requirements Analysis

- **Feature ID:** notification-preferences
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. Competitor Research Summary

| Platform | Preferences approach | Key takeaways |
|---|---|---|
| **GitHub** | Per-category notification settings in a Watch/subscription UI; granular per-repo overrides | Granular per-repo control is loved but heavy; global categories are the 80% case |
| **Twitter/X** | Notifications tab → Settings gear → per-type toggles (likes, reposts, follows, mentions) + quality filter | The "settings gear inside the notifications surface" pattern is the standard entry point |
| **YouTube** | Per-channel bell levels (all/personalized/none) + global defaults | Per-source granularity (per-creator) is the most-used control after global toggles |
| **Reddit** | Per-type toggles + per-community overrides; "chat requests" separate | Global toggles first, per-source overrides second — never the reverse |
| **FlowGPT / Snack Prompt** | No notification preferences (no notification systems) | Prompt platforms have nothing to copy; social-platform patterns apply |
| **Notion** | Single "notifications" settings page with per-type mail/in-app matrix | A matrix (type × channel) is only needed with multiple channels; we have one |

**Synthesis:** The notifications feature (in development) ships a header bell, dropdown, and history page with like/favorite/fork events, and explicitly defers preferences to "v2 once volume justifies it." The social layer is now arriving (comments add `comment` events; follow-creators adds `follow` events), which is exactly the volume trigger the deferral anticipated. The winning v1 pattern is the Twitter/Reddit hybrid: a **Notification preferences** section inside the existing notifications surface (gear icon on the notifications page → settings panel), with **per-type toggles** for the five event types (like, favorite, fork, comment, follow), **default-on for all**, changes taking effect immediately (no save button — auto-save per toggle), and a **master "pause all" switch** for overwhelm relief. No per-source (per-creator) granularity in v1 (YouTube-style per-channel bells are v2). No email/push channels (in-app only — the product has no email notification infrastructure beyond verification codes). Suppression is at event-emission time: a disabled type never creates the row, so the history page stays clean and the unread badge never counts suppressed events.

## 2. Problem Statement

1. **No user control over notification volume.** With five event types arriving (like, favorite, fork, comment, follow), a popular creator's notification feed becomes noise; the only remedy today is ignoring the bell.
2. **Preferences were explicitly deferred.** The notifications design lists "per-type opt-out" as v2; the social layer (comments, follows) is the volume trigger that justifies building it now.
3. **No overwhelm relief.** There is no "mute everything for now" control for users who want a quiet period without abandoning the product.
4. **Suppression semantics undefined.** Without a preferences contract, each new event type (comment, follow, and future ones) would improvise whether and how users can opt out — inconsistent by construction.

## 3. Target Users

- **Active creators (primary):** users whose templates attract likes/favorites/forks/comments and whose profiles attract follows — the highest-notification-volume cohort.
- **Regular users (secondary):** anyone who wants fewer interruptions or a quieter bell.
- **All users (future-proofing):** every new event type ships with a preference toggle from day one instead of retrofitting.

## 4. User Stories

- **US-P1:** As a creator, I can open notification preferences from the notifications page, so control lives where the noise lives.
- **US-P2:** As a creator, I can toggle off a specific event type (e.g., likes), so high-volume low-signal events stop reaching me.
- **US-P3:** As a user, I can pause all notifications with one switch, so I get relief without unfollowing life or deleting history.
- **US-P4:** As a user, my preference changes take effect immediately without a save step, so there is no ambiguity about whether they applied.
- **US-P5:** As a user, suppressed events never appear in my history or badge count, so my notification surface reflects only what I opted into.
- **US-P6:** As a user, I see a short plain-language description under each toggle, so I know exactly what each type covers.
- **US-P7:** As a returning user, my preferences persist across sessions and devices (server-side), so they follow my account.
- **US-P8:** As a user, I can re-enable any type at any time, so opting out is never a trap.

## 5. Functional Requirements

### 5.1 Data model

- **FR-1:** New table `notification_preferences`: `user_id` (FK → users, `ON DELETE CASCADE`, PK), `like_enabled BOOLEAN NOT NULL DEFAULT true`, `favorite_enabled BOOLEAN NOT NULL DEFAULT true`, `fork_enabled BOOLEAN NOT NULL DEFAULT true`, `comment_enabled BOOLEAN NOT NULL DEFAULT true`, `follow_enabled BOOLEAN NOT NULL DEFAULT true`, `paused_all BOOLEAN NOT NULL DEFAULT false`, `updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`. One row per user, created lazily on first preference change (absence = all defaults).
- **FR-2:** No migration backfill is needed: absent rows mean all-enabled/not-paused (defaults). The table is read on every event emission for the recipient.

### 5.2 Preference surface

- **FR-3:** A **Notification preferences** panel is reachable from the notifications history page via a gear icon in the page header (and from the bell dropdown's footer link). The panel lists the five event types, each with: type name, plain-language description, and a toggle (on by default).
- **FR-4:** A **Pause all notifications** master switch sits above the type list. When on, no notification of any type is created (events are still suppressed at emission); the bell shows no new items while paused. Existing history is untouched and remains browsable.
- **FR-5:** Toggles auto-save on change (PATCH per toggle; no save button). A subtle "Saved" indicator confirms each change; failures roll back the toggle visually and show an error toast.
- **FR-6:** The panel shows the current state honestly: while paused, type toggles are shown disabled (greyed) with a note "Paused all is on" — re-enabling requires switching pause off first.

### 5.3 Suppression semantics

- **FR-7:** Suppression happens at **event-emission time**: before inserting a notification row, the emitter checks the recipient's preferences; disabled type or paused-all ⇒ no row is created. Suppressed events never appear in history, never count toward the unread badge, and cannot be "retrieved" later.
- **FR-8:** Preferences never affect the *actor's* experience: a user with likes disabled still generates like notifications for others (the preference governs receiving, not acting).
- **FR-9:** Self-action suppression (existing notifications behavior) is unchanged; preferences layer on top (a self-action is suppressed regardless of preferences).
- **FR-10:** Admin/system notifications (if any future type emerges) are out of scope for suppression in v1 — the five user-event types only.

### 5.4 API

- **FR-11:** `GET /api/v1/users/me/notification-preferences` (auth) returns the effective preferences (materialized defaults if no row exists): `{like_enabled, favorite_enabled, fork_enabled, comment_enabled, follow_enabled, paused_all}`.
- **FR-12:** `PATCH /api/v1/users/me/notification-preferences` (auth) accepts any subset of the six boolean fields; unknown fields are ignored; validation rejects non-boolean values with the standard error shape. Returns the full effective preferences object.
- **FR-13:** The bell dropdown and history page continue to work unchanged for users who never touch preferences (zero behavioral difference by default).

### 5.5 Admin surface

- **FR-14:** None. Preferences are personal settings; no admin visibility or override in v1.

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Preference reads add < 5 ms to event emission (single-row PK lookup, cacheable); PATCH p95 < 200 ms.
- **NFR-2 (Default-zero):** Users who never open the panel experience zero change: no row is created, no UI differs, no event is suppressed.
- **NFR-3 (Consistency):** The effective-preferences read (FR-11) and the emission-time check (FR-7) use the same resolution logic (defaults for absent rows) — one code path.
- **NFR-4 (i18n):** All preference UI strings translated (EN/ZH); type names reuse the notifications feature's existing event-type labels for consistency.
- **NFR-5 (Accessibility):** Toggles are real switch roles (`role="switch"`, `aria-checked`), keyboard-operable, and grouped in a fieldset with a legend; the panel is an accessible modal or inline section per the existing pattern.
- **NFR-6 (Compatibility):** Additive only — one table, two endpoints, one panel; the notifications feature's existing surfaces (bell, dropdown, history) render unchanged.

## 7. Acceptance Criteria

- **AC-1:** The gear icon on the notifications page opens the preferences panel with five type toggles (all on) and Pause all (off).
- **AC-2:** Toggling off "Likes" persists without a save button; a subsequent like on the user's template creates no notification row, badge count, or history entry.
- **AC-3:** Turning on Pause all stops all five event types; the bell shows nothing new; existing history remains browsable.
- **AC-4:** While paused, type toggles are disabled with the "Paused all is on" note; switching pause off re-enables them with their prior states.
- **AC-5:** PATCH with a subset of fields updates only those fields; GET returns effective preferences (defaults when no row exists).
- **AC-6:** A failed PATCH rolls back the toggle visually and shows an error toast.
- **AC-7:** Preferences persist across sessions and devices (server-side storage).
- **AC-8:** A user with likes disabled still generates like notifications for other users (receiving-only semantics).
- **AC-9:** Users who never open the panel see zero behavioral difference (no row, no suppression, identical UI).
- **AC-10:** All preference UI strings render in EN and ZH; type labels match the notifications feature's labels.

## 8. Out of Scope

- Email or web-push channels (in-app only; no email notification infrastructure exists beyond verification codes).
- Per-source granularity (per-creator or per-template bells — YouTube-style per-channel levels).
- Digest/frequency options (daily/weekly summaries).
- Notification muting of individual items or threads.
- Admin visibility/override of user preferences.
- Preferences for admin/system notification types (none exist in v1).
- Monetization of any kind (payments, pricing, purchases, earnings) — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1:** The notifications feature's event-emission layer (recipient, actor, type) is the single choke point where the preference check (FR-7) is inserted; its self-action suppression runs first.
- **D-2:** The five event types (like, favorite, fork, comment, follow) are the complete v1 set: like/favorite/fork from the notifications feature; comment from the comments feature's FR; follow from follow-creators FR-16. If comments or follows have not shipped, their toggles render but suppress nothing (no events exist yet) — the panel is forward-compatible.
- **D-3:** The notifications history page exists (or will) with a page header where the gear icon lives; the bell dropdown exists with a footer link slot.
- **D-4:** The existing profile/settings section pattern (used by block-users' "Blocked accounts") provides the visual idiom if the panel is placed in Profile settings instead — the design prefers the notifications-surface placement (US-P1) but both are viable.
- **D-5:** The `users` table's PK (TEXT id) supports the FK with CASCADE; the lazy-row pattern avoids a backfill migration.

## 10. Open Questions for Architect

1. **Column-per-type vs JSONB:** six boolean columns (my recommendation — fixed type set, simple checks) vs a JSONB `disabled_types` array (flexible for new types but weaker constraints)? If you anticipate rapid type proliferation, JSONB may win.
2. **Emission-time check shape:** direct PK lookup on `notification_preferences` per emitted event (recommended, cacheable in-process with short TTL) vs joining into the emission query?
3. **Panel placement:** gear icon on the notifications history page + bell dropdown footer link (my recommendation — control lives where the noise lives) vs a Profile settings tab? Both are designed for; confirm the primary.
4. **Pause-all semantics:** suppress at emission (recommended — no rows created, badge stays clean) vs mark-as-read/silent delivery (rows created but hidden)? Emission suppression means paused users miss events permanently; confirm this is acceptable.
5. **Toggle granularity for future types:** when a new event type ships (e.g., team invites from team-collaboration), should its toggle default to on (requires a column/JSONB entry + UI row) — confirm the "every new type ships with a toggle" contract.
6. **Rate limiting:** PATCH preferences at most 30/min per account (anti-tamper) — confirm or adjust.
