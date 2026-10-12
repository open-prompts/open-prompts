# Template Scheduler Sharing — Requirements Analysis

- **Feature ID:** template-scheduler-sharing
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12
- **Related docs:** [UI/UX design](./template-scheduler-sharing-ui-ux-design.md) · [中文需求](./template-scheduler-sharing-requirements-zh-CN.md)

## 1. Competitor Research Summary

| Platform | Scheduling approach | Key takeaways |
|---|---|---|
| **Buffer / Hootsuite** | Compose now, pick a future datetime (with quick presets like "Tomorrow 9:00"), content sits in a queue and flips live automatically | Presets beat free-form datetime entry for the common case; the queue/list with status per item is the management surface |
| **WordPress** | Future-dated posts get a `scheduled` status and a cron pass publishes them at the due time; missed runs are caught up on next pass | Catch-up semantics matter: downtime must not silently drop a schedule |
| **YouTube Studio** | "Premiere" scheduling: video is private until the chosen moment; the creator's list shows a **Scheduled** badge with the exact time | A visible badge in the content list is how creators trust that scheduling actually stuck |
| **DocSend** | Ephemeral sharing: links that expire after N days, with the expiry visible to the sender | Time-boxed sharing ("public for a week, then private again") is a distinct, valuable pattern — not just delayed publish |
| **Substack** | Schedule newsletter send for a future time; a banner shows "scheduled for …" until it fires | An explicit "pending" state in the editing surface prevents accidental double-scheduling |
| **GitHub (gists/pages)** | No scheduling at all | Pattern must come from publishing platforms |
| **PromptBase / FlowGPT / Snack Prompt** | No scheduling, no ephemeral links | Differentiator opportunity: neither competitor lets a creator stage a launch or time-box a share |

**Synthesis:** Open Prompts' only visibility control is the instant Share/Unshare toggle (`handleShare`/`handleUnshare` in `TemplateDetails.js`, a full `updateTemplate` call flipping `VISIBILITY_PUBLIC`/`VISIBILITY_PRIVATE`). Creators cannot stage a launch ("publish my prompt tomorrow morning when my audience is online") or time-box a share ("public for one week during the conference, then private again"). The template-qr-share design explicitly reserved "Scheduled/ephemeral share links (template-scheduler-sharing owns that)". This feature adds owner-scoped, one-shot **visibility schedules**: schedule a future `publish` or `unpublish` (both can be pending on the same template, forming a share window), executed by the same DB-backed due-scan ticker pattern the webhooks design established (30 s pass, catch-up after downtime), with a scheduling modal in TemplateDetails, a pending-schedule timeline with cancel, a Scheduled badge on the template page and dashboard cards, and full EN/ZH translation.

## 2. Problem Statement

1. **Publishing is all-or-nothing and instant.** A creator finishing a prompt at 1 a.m. must either share it immediately (missing their audience) or remember to come back and click Share later — and forgets.
2. **Time-boxed sharing is impossible.** Sharing a prompt "just for the duration of an event" requires a manual unshare; nothing re-privatizes content automatically, so over-shared content lingers public indefinitely.
3. **No visibility into pending automation.** Even if a creator intends to flip visibility later, neither the template page nor the dashboard shows any "this will go public at T" state — there is nothing to trust or verify.
4. **The platform has no scheduler.** No cron, ticker, or job infrastructure exists in the backend (verified: only the gRPC server goroutine in `main.go`); the webhooks design introduced the in-process DB-backed ticker as the accepted pattern, and this feature reuses it.
5. **Manual toggles and automation can conflict silently.** Today's Share button is a full `updateTemplate` payload; any future automation must be explicit about how it coexists with manual toggles, or creators will be surprised by visibility flips they cannot explain.

## 3. Target Users

- **Launch-conscious creators:** stage a prompt to go public at a specific moment (product launch, newsletter drop, event opening).
- **Time-boxed sharers:** make a template public for a bounded window (demo day, course week), confident it re-privatizes automatically.
- **Batch publishers:** creators who draft several prompts and want each to appear at a planned time without babysitting the Share button.
- **Admins:** no dedicated admin surface in v1 (schedules are owner-scoped; admin template deletion cascades them away — see §8).

## 4. User Stories

- **US-1:** As a creator, I want to schedule my template to become public at a future time, so that it launches when my audience is actually online.
- **US-2:** As a creator, I want to schedule my template to become private again at a future time, so that a time-boxed share ends without me remembering to unshare.
- **US-3:** As a creator, I want to combine both (publish at T1, unpublish at T2), so that I can open a bounded share window in one sitting.
- **US-4:** As a creator, I want to see my pending schedules on the template page with their exact times, so that I can trust the automation is armed.
- **US-5:** As a creator, I want to cancel a pending schedule at any time, so that a change of plans never requires waiting for or undoing an automatic flip.
- **US-6:** As a creator, I want a Scheduled badge on my dashboard cards, so that I can see at a glance which templates have automation pending.
- **US-7:** As a creator, I want quick presets (tomorrow morning, +1 day, +7 days) in the picker, so that I don't have to type datetimes for common cases.
- **US-8:** As a creator, I want times shown in my own timezone with an explicit label, so that I never misread a schedule by a UTC offset.
- **US-9:** As a creator, I want to see recently executed and cancelled schedules, so that I can confirm what the automation actually did.
- **US-10:** As a creator, I want manual Share/Unshare to keep working independently of pending schedules, with the pending state clearly visible, so that I am never surprised by a flip I forgot about.
- **US-11:** As a creator, I want schedules to survive server restarts and catch up if their time passed during downtime, so that a deploy never silently drops my launch.
- **US-12:** As a user, I want the scheduling UI fully in EN and ZH, so that language is never a barrier to staging a launch.

## 5. Functional Requirements

### 5.1 Data model & API

- **FR-1:** New `template_schedules` table: `id UUID PK`, `template_id UUID FK templates(id) ON DELETE CASCADE`, `user_id TEXT FK users(id) ON DELETE CASCADE`, `action TEXT NOT NULL CHECK (action IN ('publish','unpublish'))`, `run_at TIMESTAMPTZ NOT NULL`, `status TEXT NOT NULL CHECK (status IN ('pending','executed','cancelled'))`, `executed_at TIMESTAMPTZ`, `created_at/updated_at TIMESTAMPTZ`. Constraints: at most **one pending schedule per (template_id, action)** (a second publish while one is pending is rejected with a friendly error); at most **20 pending schedules per user**; `run_at` must be between `now + 1 min` and `now + 90 days` at creation.
- **FR-2:** REST API (session-auth, owner-scoped; foreign or non-owned template IDs return `not_found`, no information leak):
  - `GET /api/v1/templates/{id}/schedules` — pending first (ascending `run_at`), then the latest 10 executed/cancelled; response items carry `id, action, run_at, status, executed_at, created_at`.
  - `POST /api/v1/templates/{id}/schedules` — body `{action, run_at}` (RFC3339 with offset); validates FR-1 constraints; returns `201` with the created schedule.
  - `DELETE /api/v1/schedules/{id}` — cancels a **pending** schedule of the caller (`status → cancelled`); cancelling an already-executed schedule returns `409`.
  - Creation is rate-limited to **30 schedules per hour per user** (Redis counter, same pattern as the existing API throttles) to prevent churn.

### 5.2 Execution engine

- **FR-3:** A scheduler pass (in-process ticker, every 30 s — same pattern and operational simplicity as the webhooks retry scheduler) selects due rows: `status = 'pending' AND run_at <= now()`, oldest first, batch of 50 per tick. **Catch-up:** a schedule whose `run_at` passed while the server was down executes on the first tick after restart (never silently dropped); execution is idempotent per row (claim-then-execute with `status` as the guard).
- **FR-4:** Execution semantics: flip the template's `visibility` to `public` (publish) or `private` (unpublish) using the same repository path as the manual toggle, **without creating a new template version and without touching content** (the version branch already skips when content is unchanged); set `updated_at`; mark the schedule `executed` with `executed_at = now()`. If the template is already in the target state, the schedule still marks `executed` (idempotent no-op). If the template no longer exists, the FK cascade has already removed the row. Execution never sends an engagement notification (the notifications table is engagement-shaped; see §8).
- **FR-5:** **Coexistence with manual toggles:** `handleShare`/`handleUnshare` and pending schedules are independent — a manual flip does not cancel a pending schedule, and the scheduling UI shows pending schedules prominently (badge + timeline) so a manual flip never surprises the creator. The TemplateDetails share area displays a persistent hint whenever a pending schedule exists ("Scheduled to become public Oct 15, 09:00").

### 5.3 User-facing surfaces

- **FR-6:** **Schedule sharing modal** in TemplateDetails (owner only; a clock-icon "Schedule" button beside Share): action selector (Publish / Unpublish, segmented control), datetime field (native `datetime-local`, minute granularity) with quick presets (Tomorrow 09:00, +1 day same time, +7 days same time), an explicit timezone label derived from the browser, and a live preview sentence ("This template will become public on Oct 15, 2026 at 09:00 (UTC+08:00)"). Inline validation: past time, beyond 90 days, duplicate pending action, cap reached.
- **FR-7:** **Pending timeline** inside the modal: pending schedules for this template as an ordered list (action icon, absolute local time + relative hint, Cancel button with confirm); below it, a collapsed "History" section with the latest 10 executed/cancelled schedules (status chip, time executed or cancelled).
- **FR-8:** **Scheduled badge:** a clock badge with the next pending action and time on the TemplateDetails owner bar (visible only to the owner) and on the user's dashboard template cards (clock icon + short relative time, tooltip with the exact time). Clicking the badge opens the scheduling modal.
- **FR-9:** **Timezone handling:** `run_at` is stored as UTC; the client sends RFC3339 with the browser's offset; all displayed times render in the browser's local timezone with an explicit `(UTC±HH:MM)` label; the preview sentence and timeline always show both absolute time and relative hint ("in 2 days").
- **FR-10:** **Dashboard integration:** the user-dashboard "My Templates" cards show the Scheduled badge when a pending schedule exists (data via the existing list query extended with a `pending_schedule` summary field); no separate dashboard tab in v1.
- **FR-11:** **Toast feedback:** creating a schedule shows a success toast ("Scheduled — this template will become public Oct 15, 09:00"); cancelling shows a confirmation dialog then a success toast; the badge and timeline update optimistically with rollback on error.
- **FR-12:** All scheduling-UI strings ship in EN and ZH via react-i18next (new `scheduling` namespace); API error bodies stay machine-readable English (API contract, not UI).

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** The scheduler tick is O(due rows) via a b-tree index on `(status, run_at)`; each tick processes at most 50 rows. Visibility flips reuse the existing single-row template update — no table scans, no locks beyond the row. The modal loads schedules in one `GET` alongside the template fetch.
- **NFR-2 (Reliability):** Schedules are DB-backed (durable across restarts); catch-up executes overdue rows on the first tick after downtime. Claim-then-execute (conditional `UPDATE … WHERE status='pending'`) makes double-execution impossible even with overlapping ticks. A panic in the execution loop is recovered and logged; one bad row never stalls the batch.
- **NFR-3 (Security):** All endpoints are session-auth and owner-scoped; schedule IDs are unguessable UUIDs; foreign IDs return `not_found`. `run_at` is validated server-side (window, RFC3339 parse) — client validation is UX only. No new data exposure: schedules contain only visibility intent, never content.
- **NFR-4 (Consistency):** Scheduled flips use the same repository visibility update as the manual Share/Unshare toggle, so the two paths can never diverge in behavior (banned-owner hiding, public listing filters, and embed checks all read the same `visibility` column).
- **NFR-5 (Observability):** Every execution logs a structured line (schedule ID, template ID, action, planned vs. actual time, outcome) at info; skipped no-ops and catch-up executions are distinguishable in logs. The history list is the user-facing mirror.
- **NFR-6 (i18n):** The modal, badge, timeline, history, toasts, and validation errors are fully translated (EN/ZH); action names in API payloads remain English (`publish`/`unpublish`, machine contract).

## 7. Acceptance Criteria

- **AC-1:** The owner schedules "publish at tomorrow 09:00 local"; the template stays private until then; within 60 s after 09:00 the template appears in public listings and its details page is reachable anonymously; the schedule shows `executed` in History with the execution time.
- **AC-2:** The owner schedules "publish at T1" and "unpublish at T2" (T2 > T1); the template is public between T1 and T2 and private afterwards, with both schedules executed in order and listed in History.
- **AC-3:** A pending schedule cancelled via the timeline never fires; it appears in History as `cancelled`; cancelling an executed schedule returns `409` and the UI prevents the attempt (Cancel only on pending rows).
- **AC-4:** Creating a second pending `publish` for the same template while one is pending fails with the friendly duplicate error inline; the 21st pending schedule for a user fails with the cap error; `run_at` in the past or beyond 90 days is rejected inline and server-side.
- **AC-5:** The server is restarted after a schedule's `run_at` passed; on startup the first tick executes it (catch-up), the log line marks it as catch-up, and the template's visibility reflects the flip.
- **AC-6:** A scheduled flip does not create a new template version (version count unchanged), does not alter content, and does not emit an engagement notification to anyone.
- **AC-7:** The owner manually unshares a template that has a pending publish; the template is private now, the pending schedule remains visible (badge + timeline), and at `run_at` the template becomes public again — the modal's hint made this behavior explicit beforehand.
- **AC-8:** Scheduling a `publish` for a template that is already public succeeds and executes as an idempotent no-op (`executed`, visibility unchanged, log marks no-op).
- **AC-9:** The TemplateDetails owner bar shows the clock badge with the next pending action and time; dashboard cards for templates with pending schedules show the badge; clicking either opens the scheduling modal focused on the timeline.
- **AC-10:** The preview sentence and timeline show absolute local time with an explicit `(UTC±HH:MM)` label plus a relative hint; a user in a different timezone sees times correctly converted (stored UTC, rendered local).
- **AC-11:** A non-owner cannot reach the Schedule button, and direct API calls to another user's template schedules return `not_found`; an anonymous API call returns `401`.
- **AC-12:** The entire scheduling surface (modal, badge, timeline, history, toasts, errors) renders fully in EN and ZH with no untranslated strings.

## 8. Out of Scope (v1)

- Recurring schedules (e.g., "public every weekday 9–17") — v1 is one-shot actions only.
- Scheduling for non-visibility actions (scheduled deletion, scheduled version pinning).
- Email or in-app notifications when a schedule executes (the notifications table is engagement-shaped; a system-notification channel is a separate feature).
- Admin console surface for schedules (platform-wide view of pending automation).
- Bulk scheduling from the dashboard (bulk-operations owns batch surfaces).
- Second-granularity or timezone-explicit scheduling (v1 uses browser-local input, minute granularity).
- Share-link expiry for *links* (qr-share owns link surfaces; this feature schedules template visibility only).

## 9. Dependencies & Assumptions

- **D-1:** template-qr-share (design) explicitly reserves "Scheduled/ephemeral share links (template-scheduler-sharing owns that)" (its §8, line 99) — no conflict; this feature schedules template visibility, not link expiry.
- **D-2:** The visibility model is the existing `templates.visibility` with `CHECK(public, private)`; the manual toggle is `handleShare`/`handleUnshare` (`TemplateDetails.js:549–610`) calling `updateTemplate` with a full payload — the scheduled path reuses the repository-level visibility update, not the client payload.
- **D-3:** The in-process DB-backed ticker (30 s pass, due-scan, no external queue) is the accepted scheduler pattern per the webhooks design's D-3; this feature adds a second ticker (or shares one — §10 question 1).
- **D-4:** `UpdateTemplate`'s version branch skips version creation when content is unchanged (`prompt.go:~284–310`), so a visibility-only flip does not bump the version — verified behavior this feature relies on.
- **D-5:** The TemplateDetails owner action bar (Share/Fork/Edit/Delete) is the insertion point for the Schedule button; the user-dashboard design's template cards are the insertion point for the badge (its card component accepts meta badges).
- **D-6:** The notifications table is engagement-shaped (`template_id`/`actor_id NOT NULL`) — the reason no execution notification ships in v1 (same conclusion as the webhooks design).

## 10. Open Questions for Architect

1. **One ticker or two:** share the webhooks 30 s ticker loop (single goroutine, both due-scans) vs. a separate goroutine per feature — does the deployment story prefer one scheduler service abstraction from the start?
2. **Table vs. columns:** `template_schedules` (auditable, extensible to recurring) vs. two nullable columns `publish_at`/`unpublish_at` on `templates` (simpler, no join) — which fits the schema conventions better?
3. **Claim mechanism:** conditional `UPDATE … WHERE status='pending'` as the claim (assumed, single-instance safe) vs. `SELECT … FOR UPDATE SKIP LOCKED` (needed only if multiple server instances ever run) — is multi-instance on the roadmap?
4. **Dashboard data path:** extend the existing list-Templates RPC with a `pending_schedule` summary (one extra query per page, assumed) vs. a separate batch endpoint the dashboard calls in parallel?
5. **Execution side effects:** should a scheduled flip update `templates.updated_at` (assumed yes, mirrors manual toggle) even though no version is created?
6. **Rate limit placement:** Redis counter at 30/hour per user (assumed, matches existing throttle pattern) vs. no limit given the 20-pending cap already bounds rows?
