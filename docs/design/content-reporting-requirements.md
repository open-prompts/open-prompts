# Content Reporting — Requirements Analysis

- **Feature ID:** content-reporting
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. Competitor Research Summary

| Platform | Reporting approach | Key takeaways |
|---|---|---|
| **FlowGPT** | Report/flag flows on prompts; moderators review reported content; ban spam accounts | Report-driven moderation scales better than pre-publication review — the model we follow |
| **Reddit** | Report button → reason picker (spam, harassment, …) → modqueue with report counts; auto-collapse at threshold | A small fixed reason list beats free text; the modqueue groups by item, not by report |
| **GitHub** | Report abuse on repos/issues → human review; reporters never see outcomes directly | Silent-to-reporter is acceptable; but a "resolved" state keeps the queue honest |
| **YouTube** | Reporter picks a category; system auto-hides at scale thresholds pending review | Auto-hide at high report velocity is effective abuse braking — but risky for false positives |
| **PromptBase** | Staff pre-review of every submission (no user reporting needed at their scale) | Pre-review is too heavy for us; post-hoc reporting fits an open platform |
| **Wikipedia** | Flag + edit + talk pages; vandalism patrol dashboards | Giving trusted users patrol tools multiplies a small admin team |

**Synthesis:** The admin console already has a moderation list, but it has **no inbound signal**: nothing tells an admin which content needs attention. The v1 pattern (Reddit/FlowGPT): a **Report** action on every public template and public comment, with a **fixed reason list** (spam, offensive/hateful, misleading, privacy violation, other + optional note), one report per user per item (re-report replaces), and an admin **report queue** that groups reports by item, sorts by open count and recency, and offers one-click resolution (dismiss or act: unpublish template / delete comment / ban author — all existing admin actions, now reachable from the queue). No auto-takedown in v1 (human decision always); a high-velocity signal merely sorts the queue higher. Reporters get a single confirmation and no ongoing case tracking (silent-to-reporter, GitHub-style) — the feature's job is routing attention, not building a case-management system.

## 2. Problem Statement

1. **No inbound abuse signal.** The admin console can view and moderate templates, but nothing tells admins *what* to look at; moderation is purely reactive to external channels (email, chat).
2. **Users have no voice.** A visitor who spots spam, hate, or a privacy-leaking prompt has no in-product action — the only options are leaving or complaining off-platform.
3. **Comments are unmoderated.** The comments feature ships with no removal path for admins and no reporting path for users.
4. **Community platforms need this at launch.** Every UGC platform (Reddit, FlowGPT) treats reporting as table stakes; its absence blocks healthy community growth.

## 3. Target Users

- **Reporters (regular users):** spot problematic public templates or comments and want a low-friction way to flag them.
- **Platform admins:** need a prioritized queue of flagged content with enough context to act in one place.
- **Content authors:** benefit from false reports being dismissed quickly (their content stays up pending human review — no auto-takedown).

## 4. User Stories

- **US-R1:** As a user, I can report a public template from its detail page, choosing a reason and optionally adding a note, so that problematic content reaches the admins.
- **US-R2:** As a user, I can report a public comment, so that abusive discussion is flagged too.
- **US-R3:** As a user, I see immediate confirmation that my report was recorded, so that I know the action worked.
- **US-R4:** As a user, I cannot spam the report system — one report per item per user — so that the queue stays meaningful.
- **US-R5:** As an admin, I see a report queue grouped by item (template/comment), sorted by open report count and recency, so that the worst content surfaces first.
- **US-R6:** As an admin, I can open a reported item with full context (content, author, report reasons, reporter notes) without leaving the queue, so that I can decide quickly.
- **US-R7:** As an admin, I can resolve a report group by dismissing it (false report) or by acting (unpublish template / delete comment / ban author), so that each case closes with one flow.
- **US-R8:** As an admin, my resolution actions are recorded in the audit log, so that moderation stays accountable.
- **US-R9:** As an admin, I can filter the queue by status (open/resolved), item type, and reason, so that I can work through specific backlogs.

## 5. Functional Requirements

### 5.1 Data model

- **FR-1:** New table `content_reports`: `id UUID PK`, `item_type VARCHAR(10) NOT NULL CHECK (item_type IN ('template','comment'))`, `item_id UUID NOT NULL`, `reporter_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE`, `reason VARCHAR(20) NOT NULL CHECK (reason IN ('spam','offensive','misleading','privacy','other'))`, `note TEXT` (0–500 chars, optional), `status VARCHAR(10) NOT NULL DEFAULT 'open' CHECK (status IN ('open','resolved'))`, `resolution VARCHAR(10)` (NULL unless resolved: 'dismissed' | 'actioned'), `resolved_by TEXT REFERENCES users(id)`, `resolved_at TIMESTAMPTZ`, `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`, UNIQUE `(item_type, item_id, reporter_id)`. Indexes: `(status, item_type, item_id)`, `(reporter_id)`.
- **FR-2:** Re-reporting an item the user already reported updates their existing row's reason/note and `created_at` (bumping recency) instead of creating a duplicate; status resets to open if it was resolved.
- **FR-3:** Reporting requires authentication (anonymous users cannot report); banned users cannot report.

### 5.2 Regular-user surface

- **FR-4:** A **Report** action appears on every public template detail page (in the ⋯ menu next to like/favorite/fork) and on every public comment (comment ⋯ menu). It is hidden for the user's own content.
- **FR-5:** The report modal: five radio reasons with short descriptors (Spam or scam; Offensive or hateful; Misleading or harmful; Privacy violation; Something else), an optional note textarea (≤ 500 chars), Cancel/Submit. Submit is disabled until a reason is chosen.
- **FR-6:** On submit: optimistic close + success toast "Report submitted — thank you. Our moderators will review it." If the user already reported this item, the toast says "Your report was updated."
- **FR-7:** No reporter-facing case tracking: after submission there is no status page, no notification on resolution (silent-to-reporter). The Report menu item shows a checkmark state ("You reported this") if the user has an open report on the item, allowing them to update or withdraw it (withdraw = delete their row).

### 5.3 Admin surface — report queue

- **FR-8:** Admin console gains a **Reports** page at `/admin/reports`: a queue grouped by item (one row per reported item), columns: item type badge, item title/snippet, author, open report count, reasons summary (chips), latest report time, status, actions (Review).
- **FR-9:** Default sort: open items first, by open count desc, then latest report time desc. Filters: status (open/resolved/all), item type (template/comment), reason.
- **FR-10:** **Review drawer:** clicking Review opens a side drawer with full context — the item's content (template preview or comment text), author info (with link to admin user detail), each report (reporter, reason, note, time) — and resolution actions:
  - **Dismiss reports** (false alarm): resolves all open reports on the item as 'dismissed'; content untouched.
  - **Act:** unpublish template (existing moderation action) or delete comment (existing comment moderation), each requiring a reason; then reports auto-resolve as 'actioned'. A "ban author" shortcut links to the admin user detail (ban flow stays where it is).
- **FR-11:** Resolving records `resolved_by`, `resolved_at`, `resolution` on every open report of the item, and writes an audit log entry (`report.dismiss` or the underlying action's existing entry, e.g. `template.unpublish`) with the admin's reason.
- **FR-12:** Deleted/unpublished items: if a template is unpublished or a comment deleted by any path, its open reports auto-resolve as 'actioned' (no dangling queue rows).
- **FR-13:** The admin dashboard (existing KPI page) gains a "Open reports" KPI card linking to the queue.

### 5.4 API

- **FR-14:** User APIs: `POST /api/v1/reports` (body: item_type, item_id, reason, note), `GET /api/v1/reports/mine?item_type=&item_id=` (returns the user's open report for an item, for the checkmark state), `DELETE /api/v1/reports/mine` (body: item_type, item_id — withdraw).
- **FR-15:** Admin APIs: `GET /api/v1/admin/reports?status=&item_type=&reason=&page=` (grouped queue), `GET /api/v1/admin/reports/{item_type}/{item_id}` (detail with all reports), `POST /api/v1/admin/reports/{item_type}/{item_id}/resolve` (body: resolution 'dismissed'|'actioned', reason). Unpublish/delete actions reuse their existing endpoints.
- **FR-16:** Rate limit: a user may submit at most 20 reports per hour (server-enforced, friendly error) to prevent report spam.

### 5.5 Notifications integration

- **FR-17:** No notifications in v1 (reporters are silent-to-reporter; admins watch the queue). If the notifications feature ships an admin-notify mechanism later, an "open reports threshold" alert can be added then — out of scope now.

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Report submission < 300 ms; the admin queue with 1,000 open reports paginates (50/page) and loads < 1 s.
- **NFR-2 (Privacy):** Reporters are visible only to admins (never to content authors or other users); notes are confidential moderation data.
- **NFR-3 (Integrity):** One report per user per item is enforced by the DB constraint (FR-1); resolution is atomic across all open reports of an item.
- **NFR-4 (No auto-censorship):** No content is hidden, deprioritized, or annotated automatically based on reports in v1 — every consequence requires an explicit admin action.
- **NFR-5 (i18n):** All reason labels, modal copy, queue columns, and toasts via i18next (`reports.*`, `admin.reports.*` namespaces) in EN/ZH; reason codes are locale-independent.
- **NFR-6 (Accessibility):** The report modal is a standard focus-trapped dialog with radio group semantics; the admin drawer and queue tables follow the existing admin a11y patterns.
- **NFR-7 (Consistency):** Resolution actions reuse existing admin flows (unpublish, delete comment, ban) — no new moderation powers are invented.

## 7. Acceptance Criteria

- **AC-1:** A logged-in user can report a public template with a reason and optional note; a success toast confirms; the row appears in the admin queue.
- **AC-2:** Reporting a comment works identically from the comment ⋯ menu.
- **AC-3:** The same user reporting the same item again updates their report (no duplicate row); the toast says "updated".
- **AC-4:** A user with an open report sees "You reported this" on the item; withdrawing removes their report from the queue count.
- **AC-5:** Anonymous users see no Report action; banned users get 403 on the report API.
- **AC-6:** The admin queue groups by item, shows open counts and reason chips, and sorts open-first by count then recency.
- **AC-7:** The review drawer shows the item content, author, and every report with reporter and note.
- **AC-8:** Dismissing resolves all open reports as 'dismissed' with an audit entry; the item is untouched.
- **AC-9:** Acting (unpublish/delete) resolves reports as 'actioned' and performs the underlying action with a required reason.
- **AC-10:** Unpublishing a template through the normal moderation list (not via the queue) also auto-resolves its open reports.
- **AC-11:** The 20-reports/hour limit returns a friendly error on the 21st.
- **AC-12:** Reporters' identities are never exposed outside admin surfaces; all copy is EN/ZH translated.

## 8. Out of Scope

- Auto-takedown or auto-hide at report thresholds (human decision always in v1).
- Reporter-facing case tracking or resolution notifications.
- Reporting users/profiles (only content: templates and comments; user abuse goes through the existing ban flow).
- Reporting chains, collections, or teams (added when those surfaces ship moderation).
- Moderation workspaces, assignment/claiming, SLA timers.
- Monetization of any kind — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1:** The comments feature (designed) provides comment entities with ids and a delete path for admins; if comments has not shipped, the comment reporting portion (FR-2, FR-4 comment part, FR-10 comment actions) is deferred and template reporting ships alone.
- **D-2:** The admin console's list-page pattern (search/filter/pagination/drawer) and audit log mechanism (`admin_audit_logs`, action + reason) are reused.
- **D-3:** The existing template unpublish/moderation action (admin templates page) is the "act" path for templates.
- **D-4:** The ⋯ menu pattern on template details and comments exists or arrives with the comments feature.
- **D-5:** Rate limiting infrastructure (or a simple per-user window check) exists server-side.

## 10. Open Questions for Architect

1. **Item identity:** Templates use their existing id; comments need a stable public id — confirm the comments design's id scheme is exposed in list payloads (recommendation: yes, comment ids in responses).
2. **Grouping query:** The queue groups by (item_type, item_id) with COUNT + MAX(created_at) — confirm index `(status, item_type, item_id)` suffices at expected scale, or whether a denormalized `report_groups` summary table is preferred.
3. **Auto-resolve triggers:** FR-12 requires hooking unpublish/comment-delete paths to auto-resolve — event-driven (service layer) vs a sweep on queue read? Recommendation: service-layer hook at unpublish/delete time.
4. **Withdraw semantics:** DELETE /reports/mine removes the row entirely (no trace) vs marking withdrawn — recommendation: hard delete (simplest; the unique constraint then allows re-reporting cleanly).
5. **Rate limit storage:** in-memory window vs redis (the stack has redis) — recommendation: redis counter with 1-hour TTL.
6. **Audit action naming:** `report.dismiss` as a new audit action type alongside the existing ban/unban/role_change/template actions — confirm the audit action enum is extensible.
