# Notifications — Requirements Analysis

- **Feature ID:** notifications
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-10

## 1. Competitor Research Summary

| Platform | Notifications approach | Key takeaways |
|---|---|---|
| **GitHub** | Bell icon + unread badge in header; dropdown with recent items; dedicated inbox page with filters (Unread/All); mark-read per item and bulk | The bell + badge + dropdown + inbox pattern is the canonical shape; per-item and bulk mark-read are both expected |
| **YouTube** | Bell + badge; dropdown of recent events; aggregated text ("A and 3 others liked your video"); separate history page | Aggregation reduces noise for high-traffic items; polling keeps badge fresh |
| **FlowGPT** | Profile activity feed (likes/comments on my prompts); no header badge in earlier versions | Engagement feedback drives creators to return; visibility of the surface matters (badge > buried feed) |
| **Snack Prompt** | Like/follow events surfaced on profile; lightweight toasts for own actions | Distinguish "my action succeeded" toasts from "someone acted on my content" notifications |
| **PromptBase** | No in-app notification center; email-centric | Email-only creates latency and re-engagement gaps; in-app first is the better v1 |
| **AIPRM** | Minimal/no in-app notifications | A gap, not a pattern to copy |
| **Reddit** | Inbox page with All/Unread tabs, per-item and bulk mark-read, delete | Inbox with two filters + item management is the minimal complete management surface |

**Synthesis:** The winning v1 pattern is GitHub/Reddit-style: a header bell with an unread badge (polling-based, no new infra), a lightweight dropdown for recent items, and a full history page with All/Unread filters, per-item mark-read, bulk mark-read, and delete. Aggregation ("A and 3 others") and real-time push are valuable but not required for v1. Open Prompts currently has zero feedback for creators: a template can accumulate likes, favorites, and forks and the owner never learns about it except by re-visiting the detail page.

## 2. Problem Statement

1. **No feedback loop for creators.** When someone likes, favorites, or forks a user's template, the owner has no way to find out. The only signal is the counter on the template detail page, which the owner must actively revisit.
2. **No re-engagement surface.** The product has engagement actions (like/favorite/fork) but no channel that tells the actor's target "people appreciate your work" — the single strongest organic re-engagement driver in content platforms.
3. **No notification infrastructure.** There is no notifications table, API, or UI. The existing `NotificationContext` is a transient toast system for the *current user's own action results* — a different concern that must not be conflated (naming must disambiguate).
4. **No inbox.** Users cannot review past engagement ("who favorited my template last week?") because no history exists.

## 3. Target Users

- **Creators (primary):** users who publish templates and want to know when their work receives likes, favorites, or forks.
- **Regular users:** any logged-in user with at least one owned template (every user can create templates, so every user is a potential recipient); users who never own templates simply see an empty inbox.
- **Admins:** receive notifications for their own templates exactly like regular users; no admin console surface is needed in v1 (admin moderation of notifications is unnecessary because content is system-generated from existing actions).

## 4. User Stories

### 4.1 Receiving notifications

- **US-N1:** As a creator, I want to be notified when someone likes my template, so that I know my work is appreciated.
- **US-N2:** As a creator, I want to be notified when someone favorites my template, so that I can see which templates people consider worth saving.
- **US-N3:** As a creator, I want to be notified when someone forks my template, so that I know my template is being reused and adapted.
- **US-N4:** As a user, I do NOT want notifications for my own actions on my own templates, so that the inbox is not noise.
- **US-N5:** As a user, I do NOT want duplicate notifications when someone repeatedly toggles like on/off, so that the inbox stays clean.

### 4.2 Consuming notifications

- **US-N6:** As a logged-in user, I want a bell icon in the header with an unread-count badge, so that I can see at a glance whether there is anything new.
- **US-N7:** As a user, I want the badge to refresh automatically while I use the app, so that I learn about new engagement without reloading.
- **US-N8:** As a user, I want a dropdown from the bell showing my most recent notifications, so that I can triage without leaving the current page.
- **US-N9:** As a user, I want clicking a notification to take me to the template it refers to, so that I can see the context immediately.

### 4.3 Managing notifications

- **US-N10:** As a user, I want a full notifications history page, so that I can browse all past engagement.
- **US-N11:** As a user, I want to filter the history by All / Unread, so that I can process new items first.
- **US-N12:** As a user, I want to mark a single notification as read, so that I control my unread state.
- **US-N13:** As a user, I want to mark all notifications as read in one action, so that I can zero out the badge quickly.
- **US-N14:** As a user, I want to delete a notification, so that I can remove items I do not care about.

### 4.4 System

- **US-N15:** As the system, I keep notifications strictly scoped to their recipient, so that no user can ever read another user's notifications.
- **US-N16:** As the system, I generate notification content server-side from known-safe data (actor display name, template title), so that no user-controlled markup is ever rendered.
- **US-N17:** As the system, I purge stale notifications automatically, so that the table does not grow unbounded.

## 5. Functional Requirements

### 5.1 Event generation (server-side)

- **FR-1:** The system shall create a notification when a user likes (toggle-on), favorites (toggle-on), or forks a template owned by another user. Recipient = template owner; actor = acting user; type ∈ {`like`, `favorite`, `fork`}.
- **FR-2:** The system shall NOT create a notification when the actor is the template owner (self-action suppression).
- **FR-3:** The system shall NOT create a notification if an identical unread notification (same recipient, actor, template, type) already exists (toggle-spam dedupe). Re-notification after the earlier one is read or deleted is allowed.
- **FR-4:** Each notification shall store a snapshot of the actor's `display_name` at event time; the actor's current `avatar` (nullable) is joined at read time. No other actor data (never email) is exposed.
- **FR-5:** The fork notification shall be emitted inside the existing `ForkTemplate` service flow (source template and actor are known there); no schema change to `templates` is required.
- **FR-6:** Deleting a template shall cascade-delete its notifications (FK `ON DELETE CASCADE`); deleting a user shall cascade-delete notifications where they are recipient or actor.
- **FR-7:** Notification creation shall never fail the triggering action: if the insert errors, the like/favorite/fork still succeeds and the error is logged.

### 5.2 Bell & unread badge (header)

- **FR-8:** An authenticated user shall see a bell icon in the header (between the language switch and the profile menu). Anonymous users see no bell.
- **FR-9:** The bell shall show an unread-count badge when unread_count > 0, capped at "99+". Badge hidden when 0.
- **FR-10:** The unread count shall be refreshed by polling `GET /api/v1/notifications/unread-count` every 60 seconds while the tab is visible and the user is authenticated; polling pauses when the tab is hidden, resumes (with an immediate refresh) on visibility change or window focus; on errors the interval backs off exponentially (60s → 120s → 300s max) and resets on success; polling stops and state clears on logout.
- **FR-11:** The badge shall update immediately (optimistically) when the user marks notifications read anywhere in the app, then reconcile with the server response.

### 5.3 Bell dropdown

- **FR-12:** Clicking the bell shall open a dropdown listing the 10 most recent notifications (read and unread mixed, newest first), fetched on open. Unread items are visually distinct.
- **FR-13:** The dropdown header shall show the title "Notifications" and a "Mark all read" action, visible only when unread_count > 0.
- **FR-14:** Clicking a notification item shall mark it read (optimistic), close the dropdown, and navigate to the referenced template (`/templates/:id`).
- **FR-15:** The dropdown footer shall offer "View all" linking to `/notifications`.
- **FR-16:** The dropdown shall close on outside click, Escape, or route change (matching the existing profile-dropdown pattern).

### 5.4 History page (`/notifications`)

- **FR-17:** A notifications history page shall exist at `/notifications` (auth required; anonymous → login redirect).
- **FR-18:** The page shall list notifications newest first, 20 per page, with a "Load more" button using the existing page_token pattern while more results exist.
- **FR-19:** Filter tabs "All" and "Unread" (with live counts); switching filters resets pagination.
- **FR-20:** Each row shall show: event icon (heart / star / fork), text ("{{actor}} liked your template \"{{title}}\""), relative time (absolute time in `title`), unread indicator, and actions: "Mark read" (unread items only) and "Delete".
- **FR-21:** A page-level "Mark all read" button shall mark every unread notification read (server-side bulk) and refresh the list and badge.
- **FR-22:** Empty states: All filter → "No notifications yet — when someone likes, favorites, or forks your templates, you'll see it here."; Unread filter → "You're all caught up."

### 5.5 API

- **FR-23:** `GET /api/v1/notifications?filter=all|unread&page_size=20&page_token=...` → `{notifications: [{id, type, actor: {id, display_name, avatar}, template: {id, title}, created_at, read_at}], next_page_token, unread_count}`. Auth required; always recipient-scoped from the token.
- **FR-24:** `GET /api/v1/notifications/unread-count` → `{unread_count}`. Auth required. Cheap indexed count suitable for 60s polling.
- **FR-25:** `POST /api/v1/notifications/{id}/read` → marks one notification read (idempotent; 404 if not owned by caller).
- **FR-26:** `POST /api/v1/notifications/read-all` → marks all of the caller's unread notifications read; returns `{updated_count}`.
- **FR-27:** `DELETE /api/v1/notifications/{id}` → deletes one notification (404 if not owned).
- **FR-28:** gRPC surface (recommendation, architect's choice): a new `NotificationService` in the existing proto with `ListNotifications`, `GetUnreadNotificationCount`, `MarkNotificationRead`, `MarkAllNotificationsRead`, `DeleteNotification`; event creation is internal (repository insert called from the like/favorite/fork service paths), not an RPC.

### 5.6 Regular-user vs admin split

- **FR-29:** Notifications are a per-user regular surface: every authenticated user (including admins) receives notifications for engagement on their own templates, identically.
- **FR-30:** No admin console pages (`/admin/*`) and no admin APIs (`/api/v1/admin/*`) are part of this feature. Notification content is system-generated from existing actions, so there is nothing for an admin to moderate in v1.

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** `unread-count` p95 < 100ms (partial index on recipient + unread); list p95 < 300ms with 10k notifications per user; mark-all bounded by the index.
- **NFR-2 (Scalability):** Polling design target: 10k concurrent authenticated tabs → ~170 unread-count requests/s, all cache-friendly indexed counts; interval and backoff are client-enforced and configurable in one constant.
- **NFR-3 (Compatibility):** No breaking changes; one new table, new endpoints, one header addition. Existing pages unchanged. The existing toast system (`NotificationContext`) is untouched and remains responsible for action-result toasts.
- **NFR-4 (Privacy):** Recipient-only access enforced server-side (token-derived user id, never client params); actor exposure limited to display_name + avatar; no emails or IDs beyond what the UI already shows.
- **NFR-5 (Accessibility):** Bell, dropdown, and history page fully keyboard-operable; badge count exposed to screen readers; live region announces new unread counts politely (see UI/UX doc §10).
- **NFR-6 (i18n):** All user-facing strings EN/ZH in sync; notification text composed client-side from typed data (type + actor + title) so it is fully translatable; relative times via the shared util; template titles are user content and never translated.
- **NFR-7 (Retention):** Read notifications older than 90 days and all notifications older than 180 days are purged (periodic backend cleanup; mechanism is an architect decision — see §10).
- **NFR-8 (Resilience):** Polling/network failures never break the UI: badge keeps its last known value, dropdown/page show error + Retry; event-insert failures never fail the triggering action (FR-7).
- **NFR-9 (Security):** All endpoints require auth; ownership checks on every id-addressed operation; notification content is rendered as plain text (React escaping), never as HTML.

## 7. Acceptance Criteria

- **AC-1:** Liking, favoriting, or forking another user's template creates exactly one notification for the owner (self-actions create none; rapid toggle on/off creates no duplicates).
- **AC-2:** The bell appears for authenticated users with a correct unread badge (capped "99+"); it is absent for anonymous users.
- **AC-3:** The badge refreshes within 60s of a new notification while the tab is visible, pauses when hidden, and refreshes immediately on tab focus.
- **AC-4:** The dropdown lists the 10 most recent notifications with unread items visually distinct; "Mark all read" appears only when unread > 0.
- **AC-5:** Clicking a dropdown item marks it read, closes the dropdown, and navigates to the template.
- **AC-6:** `/notifications` lists all notifications newest-first with working All/Unread filters, Load more pagination, per-item mark-read, per-item delete, and Mark all read.
- **AC-7:** Marking read (single or all) updates the badge immediately and remains consistent after a page reload.
- **AC-8:** Anonymous access to `/notifications` or any notification API redirects/returns 401; user A cannot read, mark, or delete user B's notifications (404).
- **AC-9:** Deleting a template removes its notifications; deleting a user removes their notifications (recipient and actor sides).
- **AC-10:** All new UI strings exist in both `en` and `zh` translation files with identical key sets; notification sentences render correctly in both languages.
- **AC-11:** Existing pages (square, template detail, profile, API keys) behave unchanged; the existing toast system is unaffected.
- **AC-12:** With the notifications API unreachable, the app otherwise works; badge/dropdown/page degrade to error states with Retry.

## 8. Out of Scope (v1)

- Real-time delivery (WebSocket/SSE) — polling only.
- Email or web-push digests.
- Aggregation ("Alice and 3 others liked…") — individual events only in v1.
- Comment notifications — the product has no comments feature.
- Follow/subscribe notifications — the product has no follow feature.
- Notification preferences/settings page (per-type opt-out) — v2 once volume justifies it.
- Admin console notification management (`/admin/*`) — nothing to moderate in v1.
- Any event types beyond engagement (like/favorite/fork) — v1 covers exactly these three; new event types require a new design round.
- Mobile-app-specific layouts (responsive web only).

## 9. Dependencies & Assumptions

- **D-1:** `template_likes` and `template_favorites` rows carry `created_at` (confirmed in `deployment/init.sql`); the like/favorite service paths (`ToggleLikeTemplate`, `ToggleFavoriteTemplate`) know the actor, template, and owner at toggle-on time — the notification insert hooks there.
- **D-2:** `ForkTemplate` (backend/internal/service/prompt.go) currently does not persist the source template on the fork; the fork notification is emitted inside that flow where the source is known — no `templates` schema change required.
- **D-3:** `users.display_name` is NOT NULL (snapshot always available); `users.avatar` is nullable (UI falls back to the initial-letter placeholder, matching the Header pattern).
- **D-4:** The relative-time util is shared with the user-dashboard feature (its D-6); if that feature has not shipped yet, this feature introduces it.
- **D-5:** Polling requires no new infrastructure (no Redis pub/sub, no WebSocket server); the gateway gains the notification routes alongside existing ones.
- **D-6:** The header already implements the dropdown open/outside-click-close pattern (profile dropdown) — the bell dropdown reuses it.
- **D-7:** The existing `NotificationContext` toasts keep their name; the new UI uses a distinct `notifications.*` i18n namespace and `notificationsSlice` Redux slice to avoid any collision.
- **D-8:** Badge state must be shared between the header bell, the dropdown, and the history page — a small Redux slice (`notificationsSlice`: `unreadCount`, `lastFetchedAt`, `error`) is the assumed mechanism, consistent with the existing Redux Toolkit usage.

## 10. Open Questions for Architect

1. **Service placement:** new `NotificationService` in the existing proto (recommended — clean domain boundary) vs. extending `PromptService` (fewer moving parts). Either is acceptable; the REST surface is identical.
2. **Retention mechanism:** dedicated cron/goroutine ticker in the backend vs. lazy purge (e.g., opportunistic delete on `read-all`). Recommendation: lazy purge first (no scheduler dependency), ticker later if needed.
3. **`unread_count` in list responses (FR-23):** included so the dropdown/history can refresh the badge without an extra call — confirm this is acceptable vs. keeping the count endpoint as the single source.
