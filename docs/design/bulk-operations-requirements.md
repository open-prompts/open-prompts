# Bulk Operations — Requirements Analysis

- **Feature ID:** bulk-operations
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. Competitor Research Summary

| Platform | Bulk pattern | Key takeaways |
|---|---|---|
| **GitHub** | Multi-select in lists (issues, projects) with a floating action bar; "select all on page" tri-state header checkbox; bulk label/close/assign | The floating bar showing "N selected + actions" is the canonical affordance; tri-state header checkbox (all/some/none) is expected |
| **Gmail** | Checkbox multi-select + toolbar actions; bulk archive/delete/label with undo toast | Undo after bulk destructive actions dramatically reduces support load; per-item failure reporting is essential |
| **YouTube Studio** | Bulk video actions: select multiple → change visibility, delete, add tags | "Change visibility" is the most-used bulk action for content owners — mirrors our private/public toggle |
| **Notion** | Multi-select pages → move/delete/export | Confirmation with an explicit item count ("Delete 12 pages?") prevents mass mistakes |
| **Stripe** | Bulk transfers/invoices with per-row result reporting and partial failure summary | Partial-failure UX (some succeed, some fail) must be first-class, not an all-or-nothing error |
| **Stack Overflow** | Moderator bulk flag handling | Admin bulk moderation with a single reason applied to many targets, each action audited individually |

**Synthesis:** The industry pattern is consistent: (1) checkbox multi-select with a tri-state "select all on this page" header, (2) a contextual action bar showing the selection count, (3) a confirmation dialog with the exact count (and item preview), (4) server-side batch endpoints that return **per-item results** so partial failures are reported precisely, and (5) destructive bulk actions confirmed with count + optional typed confirmation. Open Prompts currently has zero multi-select surfaces: `AdminTable` has no selection column, the user dashboard lists have no checkboxes, and every mutation RPC is single-target. Both the user-dashboard design (§8: "Bulk actions (multi-select delete)") and admin-user-management design (§8: "bulk ban, bulk role change") explicitly deferred this to a dedicated feature.

## 2. Problem Statement

1. **Cleanup is one-by-one.** A creator with 30 obsolete templates must open each detail page and delete individually — dozens of clicks and page loads for a single conceptual action.
2. **Visibility flips are tedious.** "Make all my drafts private before a review" or "publish these 10 templates" requires 10 separate edit flows.
3. **Admin moderation doesn't scale.** A raid of spam accounts means one ban flow per user (each with its own reason dialog); admins need to apply one reason to many targets.
4. **No shared selection infrastructure.** Every future list (comments moderation, collections management) would reinvent checkboxes; one reusable selection model + component pays off across the product.

## 3. Target Users

- **Creators:** cleaning up or re-organizing their template library (delete, visibility).
- **Admins:** moderating waves of abusive accounts (bulk ban/unban with one reason).
- **All users:** benefit from the reusable selection pattern in future list surfaces.

## 4. User Stories

### 4.1 Creators — template library management

- **US-B1:** As a creator, I want to multi-select templates in my dashboard "My Templates" list, so that I can act on many at once.
- **US-B2:** As a creator, I want to bulk delete selected templates, so that I can clean up obsolete work in one action.
- **US-B3:** As a creator, I want to bulk change visibility (make private / make public), so that I can control exposure of groups of templates.
- **US-B4:** As a creator, I want a confirmation showing exactly how many templates will be affected, so that I never destroy more than intended.
- **US-B5:** As a creator, I want to see which items failed and why after a bulk action, so that I can retry or fix them.

### 4.2 Admins — account moderation

- **US-B6:** As an admin, I want to multi-select users in the admin users list, so that I can moderate many accounts efficiently.
- **US-B7:** As an admin, I want to bulk ban selected users with one reason, so that a spam wave takes one action instead of N.
- **US-B8:** As an admin, I want to bulk unban selected users, so that I can revert a mistaken wave quickly.
- **US-B9:** As an admin, I want each bulk-banned user to get an individual audit log entry with the shared reason, so that the audit trail stays per-target.

### 4.3 System integrity

- **US-B10:** As the system, I never allow bulk actions on items the actor doesn't own (users) or lacks rights over (admins), and I report those as per-item failures rather than aborting the batch.

## 5. Functional Requirements

### 5.1 Selection model (shared)

- **FR-1:** A reusable selection model shall support: toggling individual rows, a tri-state header checkbox ("select all on this page" / indeterminate when some selected / none), "Clear selection", and a visible count. Selection persists across client-side pagination within the same list session and clears on filter change, route change, or list refresh after a bulk action.
- **FR-2:** Selection shall be capped at 50 items per batch (matching the list page size). Selecting beyond 50 shows a toast explaining the cap; "select all" selects at most the current page's items.
- **FR-3:** `AdminTable` shall gain an optional selection mode (checkbox column + header tri-state + `onSelectionChange` callback) without breaking existing non-select usages (all current admin pages render unchanged).

### 5.2 Creator bulk actions (dashboard "My Templates")

- **FR-4:** When ≥ 1 template is selected in "My Templates", a floating bulk action bar shall appear (bottom of viewport) showing "{{count}} selected" with actions: **Delete**, **Make private**, **Make public**, and **Clear**.
- **FR-5:** **Bulk delete** shall open a confirmation modal listing the count and the first 5 titles ("+N more"), require typing the word `DELETE` when ≥ 5 templates are selected (plain confirm button below that), then call the batch endpoint. Deleted templates are soft-deleted per existing delete semantics; fork/like/comment counts on other entities are unaffected.
- **FR-6:** **Bulk visibility change** shall open a confirmation modal ("Make {{count}} templates private/public?") summarizing consequences (private templates disappear from public listings and embeds), then call the batch endpoint.
- **FR-7:** Both actions shall call new batch RPCs — `BatchDeleteTemplates(template_ids[], )` and `BatchUpdateTemplateVisibility(template_ids[], visibility)` — restricted to templates **owned by the caller**; foreign IDs return per-item failure `not_found` (no information leak). Each RPC executes in a single transaction per item (item-level atomicity), returns `results[] = {template_id, success, error_code?}`, and enforces the 50-item cap with `invalid_argument` beyond it.
- **FR-8:** After a bulk action, the list shall refresh, the selection shall clear, and a result toast shall show "Done: X succeeded, Y failed". If Y > 0, the toast expands (or links) to a per-item failure list (title + error message). Failed items remain in the list unselected.

### 5.3 Admin bulk actions (users list)

- **FR-9:** The admin users list (`/admin/users`) shall enable selection mode with bulk actions **Ban** and **Unban** in the floating bar.
- **FR-10:** **Bulk ban** shall open a modal requiring one reason (3–500 chars, same validation as single ban), list the count and first 5 emails, then call `BatchBanUsers(user_ids[], reason)`. **Bulk unban** mirrors this with `BatchUnbanUsers(user_ids[], reason)` (reason optional, defaults to "bulk unban").
- **FR-11:** Batch admin RPCs shall process each user independently (item-level atomicity), skip-and-report protected targets (self, other admins) as per-item failures with `forbidden`, and write **one audit log row per user** using the existing `ban`/`unban` actions and the shared reason — no schema change to `admin_audit_logs` needed.
- **FR-12:** Admin batch results shall surface identically to FR-8 (toast + expandable failures). The users list refreshes and selection clears.

### 5.4 Safety rails

- **FR-13:** The floating bar shall hide when selection is empty; destructive actions (delete, ban) always require the confirmation modal — no direct execution from the bar.
- **FR-14:** Bulk actions shall be rate-limited (e.g., 10 batch calls/min/user) to prevent accidental hammering; exceeding shows a friendly retry message.
- **FR-15:** All bulk-action UI strings shall exist in `en` and `zh` with identical keys.

## 6. Non-Functional Requirements

- **NFR-1 (Consistency):** Batch endpoints reuse existing single-item service logic per item (same validation, same side effects: notification suppression for bulk visibility, cascade semantics for delete) — no divergent behavior between bulk and single paths.
- **NFR-2 (Performance):** A 50-item batch completes < 2s p95 (sequential per-item service calls are acceptable at this scale; no parallelism required).
- **NFR-3 (Auditability):** Every admin bulk ban/unban produces per-user audit rows indistinguishable from single actions (same actor, reason, timestamps within the batch).
- **NFR-4 (Accessibility):** Checkboxes are real `input[type=checkbox]` with labels; the header checkbox announces "select all on this page"; the floating bar is a focusable `toolbar` region with `aria-live` count updates; confirm modals follow the existing modal a11y pattern.
- **NFR-5 (No regressions):** All existing list pages (admin users/templates/comments, dashboard tabs) render and behave identically when selection mode is off.
- **NFR-6 (i18n):** All new strings in `en`/`zh`; per-item error messages map from error codes to localized text.

## 7. Acceptance Criteria

- **AC-1:** In "My Templates", clicking row checkboxes updates a floating bar ("N selected"); the header checkbox selects/deselects the whole page and shows indeterminate state for partial selection.
- **AC-2:** Selecting 3 templates and choosing "Make private" shows a confirmation with the count; confirming changes all 3 to private (verified by list badges), clears selection, and shows a success toast.
- **AC-3:** Bulk delete of ≥ 5 templates requires typing `DELETE`; of < 5 shows a plain confirm. After confirmation the templates disappear from the list and a "3 deleted" style toast appears.
- **AC-4:** A batch containing a foreign template ID (crafted request) returns that item as failed (`not_found`) while owned items succeed — the UI shows "X succeeded, 1 failed" with the failure reason.
- **AC-5:** Batch requests with > 50 IDs are rejected with `invalid_argument` before any mutation; the UI never sends > 50.
- **AC-6:** Admin bulk ban with reason "spam wave" on 4 users bans all 4, writes 4 audit rows (action=`ban`, same reason), and shows the result toast; selecting the admin's own account in a batch returns it as `forbidden` failure.
- **AC-7:** Admin bulk unban restores users and writes `unban` audit rows with the given reason.
- **AC-8:** Selection clears after any bulk action, on filter change, and on route change; it does not leak into other tabs.
- **AC-9:** All existing admin pages and dashboard tabs render pixel-identical with selection mode off (no layout shift from the new optional column).
- **AC-10:** Every new UI string exists in both `en` and `zh` with identical keys; error-code → message mapping is localized.
- **AC-11:** Keyboard users can toggle rows (checkbox focus + space), reach the bar actions, and complete a full bulk delete flow without a mouse.
- **AC-12:** Exceeding 10 batch calls in a minute shows a friendly rate-limit message; no partial mutations occur from the rejected call.

## 8. Out of Scope (v1)

- Bulk tag add/remove (tag taxonomy interactions deserve their own pass).
- Bulk add-to-collection / remove-from-collection.
- Bulk fork, bulk export (import-export covers single-item export; batch export may follow).
- Cross-page selection ("select all N matching" beyond the current page).
- Bulk role change for admins (kept to ban/unban; role changes stay deliberate and single).
- Bulk comment moderation (single delete exists; batch may follow the same pattern later).
- Scheduled/deferred bulk actions, undo/restore window (soft-delete restore is a versioning concern).
- Drag-to-select ranges.

## 9. Dependencies & Assumptions

- **D-1:** The user dashboard's "My Templates" tab (user-dashboard design) lists the caller's templates with `id`, `title`, `visibility` — the selection UI attaches there. If the dashboard has not shipped, the Profile page's template list is the fallback surface (same data).
- **D-2:** `DeleteTemplate` and `UpdateTemplate` service logic (ownership check, soft delete, visibility transition rules) exists and is reused per item by the batch RPCs.
- **D-3:** `BanUser`/`UnbanUser` service logic (protected-account checks, reason validation 3–500, audit row write) exists and is reused per item; `admin_audit_logs` CHECK constraints already accept `ban`/`unban` with `target_type='user'` — no migration needed.
- **D-4:** `AdminTable` is the single admin list component (used by Users/Templates/Comments pages) — the selection column is added there once.
- **D-5:** Redis-based rate limiting infrastructure (used by comments) is available for FR-14.
- **D-6:** The floating action bar pattern is new to the app; it will live in `frontend/src/components/bulk/` and be reusable by future list surfaces.

## 10. Open Questions for Architect

1. **Batch RPC shape:** one RPC per action (`BatchDeleteTemplates`, `BatchUpdateTemplateVisibility`, `BatchBanUsers`, `BatchUnbanUsers`) as proposed, vs. a generic `BatchOperation(entity, action, ids, payload)`? I recommend per-action RPCs (typed, self-documenting); confirm.
2. **Transaction scope:** item-level atomicity (commit per item, report failures) as proposed, vs. all-or-nothing transactions? I recommend item-level (matches Stripe/GitHub mental model and FR-8 UX); confirm.
3. **Visibility batch + embeds:** when a template goes private via bulk, existing embeds show "not available" (per embedding-widget design) — confirm no additional notification to the owner is needed for bulk (single-item changes don't notify either).
4. **Rate-limit key:** per-user (proposed) vs. per-IP for admin endpoints? Admins are authenticated; per-user seems sufficient.
