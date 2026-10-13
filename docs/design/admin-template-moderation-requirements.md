# Admin Template Moderation — Requirements Analysis

- **Feature ID:** admin-template-moderation
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-13
- **Companion doc:** `admin-template-moderation-ui-ux-design.md`

## 1. Competitor Research Summary

| Product | Template/content moderation pattern | Takeaway |
|---|---|---|
| **Reddit (mod tools)** | Remove post (with reason), lock, spoiler; mod log records every action; removed content hidden from public but visible to mods | Reversible removal (not deletion) is the default; the mod log is non-negotiable |
| **YouTube Studio** | Video states: public / unlisted / private / removed; age-restrict; "limited ads"; appeal path for creators | A state machine beats binary delete; creators get an explanation |
| **Discord** | Delete message (logged), timeout user, slow-mode; audit log with actor + target + reason | Every destructive action pairs with actor + reason + timestamp |
| **WordPress** | Trash (30-day soft delete) → permanent delete; bulk trash; restore from trash | Soft-delete with a restore window is the universal safety net |
| **GitHub (repo takedown)** | Admin can make repo private/remove for ToS violations; transparency reports | Platform-level takedown is rare, reason-documented, and appealable |
| **FlowGPT / Snack Prompt** | Minimal visible moderation; content vanishes without explanation | The negative example: unexplained removals destroy creator trust |

**Synthesis:** The winning v1 pattern is the **Reddit/WordPress hybrid**: admin actions on templates are **state changes, not deletions** — (1) **Unpublish** (public → private, reversible, reason-required, creator notified), (2) **Lock** (freeze edits + new comments/forks, content stays public — for disputed content under review), and (3) **Soft delete** (hidden everywhere, 30-day restore window, then permanent). All three write to the existing `admin_audit_logs` with actor, target, reason. The admin template detail page (`/admin/templates/:id`, already designed read-only in admin-user-management) gains a **Moderation actions** panel; the admin templates list gains a **status filter** (public / private / locked / soft-deleted). The content-reporting feature's "act: unpublish template" flow calls this feature's unpublish endpoint — closing the gap where content-reporting assumed an "existing moderation action" that does not exist yet. Creator notification uses the existing notifications system with a new `moderation` event type (data-driven registration, per the mention-notifications pattern).

## 2. Problem Statement

1. **Admins can look but not act.** The admin console's template pages are read-only by design (admin-user-management FR-24/27 explicitly deferred "admin editing or deleting templates" to a follow-up). An admin who finds a policy-violating template must contact the owner and ask them to delete it — no enforcement path.
2. **content-reporting depends on a missing action.** The reporting queue's "act" flow assumes an "unpublish template (existing moderation action)" — but no such admin endpoint exists in the backend (`admin.proto` has only ListTemplates/GetTemplate/DeleteAdminComment). The moderation loop is broken at its final step.
3. **No proportionate response.** Today the only lever is banning the user (nuclear) or doing nothing. Moderation needs graduated responses: unpublish (reversible), lock (freeze-in-place), delete (last resort).
4. **No accountability trail for content actions.** User actions (ban/unban) are audit-logged, but there is no action vocabulary, reason requirement, or notification for template-level moderation.

## 3. Target Users

- **Admins (primary):** need graduated, auditable enforcement on templates surfaced by reports or patrol.
- **Creators (secondary):** need to know when their content is actioned, why, and how to fix or appeal it.
- **The community (implicit):** benefits from fast removal of harmful content without collateral damage (bans) for borderline cases.

## 4. User Stories

- **US-M1:** As an admin, I can unpublish any public template (public → private) with a required reason, so that policy-violating content leaves public surfaces immediately and reversibly.
- **US-M2:** As an admin, I can lock a template (freeze edits, comments, forks, new likes) while keeping it publicly visible, so that disputed content stays frozen during review without vanishing.
- **US-M3:** As an admin, I can soft-delete a template (hidden everywhere, 30-day restore window), so that the worst content is removed with a safety net against mistakes.
- **US-M4:** As an admin, I can restore an unpublished or soft-deleted template from the admin detail page, so that mistakes and successful appeals are reversible.
- **US-M5:** As an admin, I see every moderation action I could take with its consequences spelled out (what stays, what goes, who is notified), so that I choose the proportionate response.
- **US-M6:** As an admin, I can filter the templates list by moderation status (public / private / locked / soft-deleted), so that I can review actioned content and pending restores.
- **US-M7:** As a creator, I receive a notification when my template is unpublished, locked, or deleted, with the reason, so that I understand what happened and can respond (fix and re-publish, or appeal).
- **US-M8:** As a creator, I can re-publish an admin-unpublished template after fixing it, so that moderation is corrective, not punitive — unless the template is locked (re-publish blocked with an explanation).
- **US-M9:** As an admin, every moderation action I take is written to the audit log with actor, target, reason, and timestamp, so that oversight and appeals are grounded in facts.
- **US-M10:** As an admin, I can jump from a content-reporting queue item directly to the moderation action, so that the report → action loop is one flow.

## 5. Functional Requirements

### 5.1 Moderation states

- **FR-1:** New column `templates.moderation_status VARCHAR(12) NOT NULL DEFAULT 'normal'` with values: `normal`, `locked`, `unpublished_by_admin`, `soft_deleted`. The existing `visibility` column is unchanged (public/private); moderation status is orthogonal and takes precedence in access checks.
- **FR-2:** **Unpublish** (`unpublished_by_admin`): the template behaves as private on all public surfaces (home, search, category, related rails, embeds, QR-share targets, following feeds, API `GetPromptByAlias` returns 404 for non-owners) regardless of `visibility`. The owner still sees and edits it. The owner **can** re-publish (sets `moderation_status` back to `normal` and `visibility` to public) — moderation is corrective. Re-publishing does not notify admins (v1).
- **FR-3:** **Lock** (`locked`): the template stays publicly visible (read-only) but all mutations are blocked for everyone except admins: no edits, no new comments, no forks, no likes/favorites/ratings, no watches, no preset saves. Existing comments stay visible. The detail page shows a lock banner ("This template is locked by moderators").
- **FR-4:** **Soft delete** (`soft_deleted`): the template is hidden from everyone except admins (owner sees "This template was removed by moderators" with the reason on direct visit; 404 elsewhere). `deleted_at` timestamp starts a **30-day restore window**; after 30 days a ticker job (the webhooks/scheduler due-scan pattern) hard-deletes it using the same cascade semantics as owner deletion (account-deletion FR-8/9: versions, aliases, comments on it go; others' forks survive with "deleted source" lineage).
- **FR-5:** **Restore**: admins can restore `unpublished_by_admin` (→ `normal`, visibility stays private — the owner re-publishes), `locked` (→ `normal`), and `soft_deleted` within the window (→ `normal`, previous visibility). Restore requires a reason and writes an audit entry.

### 5.2 Admin API and UI

- **FR-6:** New admin RPCs (REST under `/api/v1/admin`): `POST /api/v1/admin/templates/{id}/unpublish` (reason required), `POST /api/v1/admin/templates/{id}/lock` (reason required), `POST /api/v1/admin/templates/{id}/delete` (reason required; soft delete), `POST /api/v1/admin/templates/{id}/restore` (reason required). All require admin session; all write `admin_audit_logs` with new action codes `template.unpublish`, `template.lock`, `template.delete`, `template.restore` (actor, target template id, reason, timestamp).
- **FR-7:** `GET /api/v1/admin/templates` gains a `moderation_status` filter and the response items include `moderation_status`, `moderated_at`, `moderated_by`, `moderation_reason` (admin-only fields).
- **FR-8:** The admin template detail page (`/admin/templates/:id`) gains a **Moderation actions** panel: current status chip, the three actions with consequence summaries, restore (when applicable), and the moderation history (from audit logs filtered by target).
- **FR-9:** All three actions use a **reason modal** (3–500 chars, same validation as ban) with a consequence checklist ("Public surfaces: removed / Comments: frozen / Creator: notified"). Delete requires additionally typing the template title (type-to-confirm, matching account-deletion's pattern).
- **FR-10:** The content-reporting queue's "Act → unpublish template" calls FR-6's unpublish endpoint (closing the loop content-reporting FR-15 assumed); after success, the report group auto-resolves as `actioned` (existing content-reporting FR-11/12 semantics).

### 5.3 Creator experience

- **FR-11:** A new notification event type `moderation` (data-driven registration into the notifications system and notification-preferences panel, default **on**, suppressible): emitted to the template owner on unpublish/lock/delete/restore, payload includes action, template title, reason, and (for unpublish) a "fix and re-publish" deep link.
- **FR-12:** Owner-facing surfaces: unpublished → the template detail page (owner view) shows an admin-notice banner ("An administrator unpublished this template: {reason}. You can edit it and share it again."); locked → lock banner with reason; soft-deleted → a removed-notice page with reason and an appeal hint (contact path is out-of-band in v1 — the banner links to the existing support/contact affordance or states "reply via your notification").
- **FR-13:** Re-publish after admin unpublish is the normal share flow (existing `UpdateTemplate` visibility change) — no special endpoint; the banner's CTA deep-links to it. Locked templates block the share toggle with an explanatory tooltip.

### 5.4 Guard rails

- **FR-14:** Admins cannot moderate their own templates (403 with message) — same self-action guard as ban. One admin's action can be reversed by another admin (restore) — no super-lock in v1.
- **FR-15:** Moderation actions are rate-limited per admin (reuse the rate-limit-dashboard's write tier) to prevent accidental mass-action; bulk moderation is explicitly out of scope.
- **FR-16:** The 30-day hard-delete ticker is idempotent and catch-up safe (the scheduler-sharing/webhooks due-scan pattern); restore during the window cancels the scheduled deletion.

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Unpublish/lock take effect immediately on read paths (status checked in the existing template access layer — one extra column in the WHERE clause / response shaping); no cache invalidation beyond existing TTLs.
- **NFR-2 (Auditability):** Every action, restore, and the scheduled hard-delete writes an audit entry; the moderation history on the detail page is complete and immutable (existing audit-log semantics).
- **NFR-3 (Privacy):** Moderation reasons are shown to the template owner (transparency) and admins only — never to other users. The lock banner shows a generic message to the public ("locked by moderators"), not the reason.
- **NFR-4 (i18n):** All admin and creator-facing strings localized EN/ZH; notification content localized per the existing rendering.
- **NFR-5 (Compatibility):** No change to owner-initiated delete, archive, or visibility semantics; moderation status composes with them (e.g., an admin-unpublished template can still be owner-archived).

## 7. Acceptance Criteria

- **AC-1:** Admin opens a public template's admin detail → Moderation actions panel visible → Unpublish with reason → template disappears from home/search/embeds/API for non-owners; owner sees the admin-notice banner with the reason; audit log has `template.unpublish`.
- **AC-2:** Owner receives a `moderation` notification with action, title, and reason; clicking it opens the template (owner view) with the banner.
- **AC-3:** Owner edits the template and re-shares it → `moderation_status` returns to `normal`, template is public again; no admin notification.
- **AC-4:** Admin locks a template → public viewers see the lock banner; comments/forks/likes/ratings/watches are blocked with explanatory errors; existing comments remain visible; owner cannot edit (save blocked with the lock reason).
- **AC-5:** Admin soft-deletes a template (types the title to confirm) → non-owners get 404; owner sees the removed-notice with reason; admin list shows it under `soft_deleted` with a restore countdown.
- **AC-6:** Admin restores a soft-deleted template on day 12 → template returns to its previous visibility; audit log has `template.restore`; the scheduled hard-delete is cancelled.
- **AC-7:** A soft-deleted template past 30 days is hard-deleted by the ticker with account-deletion cascade semantics; another user's fork of it survives with "deleted source" lineage.
- **AC-8:** Admin filters `/admin/templates` by `moderation_status=locked` → only locked templates listed, each with moderated_at/by/reason.
- **AC-9:** From a content-reporting queue group, Act → unpublish → the report group auto-resolves as `actioned`; the moderation audit entry carries the same reason.
- **AC-10:** Admin attempts to unpublish their own template → 403 with message; audit log unchanged.

## 8. Out of Scope (v1)

- Bulk moderation actions (multi-template unpublish/delete).
- Pre-publication review queue (submit → approve) — future feature (per admin-user-management).
- Creator appeal workflow (in-app appeal form, case states) — v1 is notify + out-of-band contact.
- Admin editing template content (only state changes; content editing stays with the owner).
- Age-restriction / content-warning overlays (YouTube-style) — future.
- Shadow-banning or graduated visibility (search-delisting only).
- Monetization of any kind — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1:** admin-user-management provides the admin console shell, `/admin/templates` list + detail pages (currently read-only), the reason-modal pattern, and `admin_audit_logs`.
- **D-2:** content-reporting's queue "act" flow integrates with FR-6/FR-10; its auto-resolve semantics (FR-11/12) apply.
- **D-3:** The notifications system + notification-preferences provide data-driven event-type registration (the mention-notifications pattern) for the `moderation` type.
- **D-4:** The scheduler-sharing/webhooks DB-backed due-scan ticker pattern hosts the 30-day hard-delete scan.
- **D-5:** account-deletion defines the hard-delete cascade semantics (FR-8/9) reused for expired soft-deletes; template-remix-forks-tree provides the "deleted source" lineage rendering.
- **D-6:** template-embedding-widget and template-qr-share surfaces must respect `moderation_status` (embeds of unpublished/soft-deleted templates render a "removed" state, not the content).
- **D-7:** rate-limit-dashboard provides the write-tier limiter reused in FR-15.
- **D-8:** Assumption: `templates.visibility` remains the owner-controlled field; `moderation_status` is admin-controlled and takes precedence — access checks combine both.

## 10. Open Questions for Architect

1. **Access-check composition:** where should `moderation_status` be enforced — a single repository/service layer guard (recommended: one `checkTemplateAccess(template, viewer)` used by all read paths), or per-endpoint checks (risk of leaks on embed/API paths)?
2. **Column vs. separate table:** `templates.moderation_status` column (recommended — hot path read) vs. a `template_moderation_events` table with current-state derivation (more history, more joins). History can come from audit logs either way.
3. **Lock interplay with owner edit:** FR-3 blocks owner edits on locked templates. Alternative: allow owner edits but block re-publish only (so the owner can prepare a fix during review). Recommend block-both (true freeze); confirm.
4. **Hard-delete reuse:** should the 30-day expiry call the same deletion routine designed for account-deletion (shared cascade code), and does that routine exist as a reusable service or is it account-specific? Recommend extracting a shared template-cascade service.
5. **Re-publish without admin review (FR-2):** the owner can immediately re-publish after unpublish — a malicious owner could re-publish unchanged. Mitigation options: (a) accept (v1 simplicity, admin can re-unpublish; repeated violations → ban), (b) rate-limit re-publish after moderation, (c) require admin re-approval. Recommend (a) with a note in the audit trail when a moderated template is re-published (event `template.republished_after_moderation`) so patterns are visible. Confirm.
