# Bulk Template Moderation — Requirements Analysis

- **Feature ID:** bulk-template-moderation
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-13
- **Companion doc:** `bulk-template-moderation-ui-ux-design.md`

## 1. Competitor Research

| Product | Bulk moderation capability | Strength | Weakness |
|---|---|---|---|
| WordPress | Bulk trash/restore from list checkboxes; bulk actions dropdown | Familiar, fast for raids | No per-item audit granularity |
| Stack Overflow | Moderator bulk flag handling | One decision, many targets | Limited action types |
| YouTube Studio | Bulk actions on videos list (delete, visibility) with confirm | Scale + safety confirms | No reason reuse across items |
| Reddit | Bulk remove/approve in mod queue | Queue-native | Reason per action, repetitive |
| Shopify | Bulk product status change with result summary | Per-item result reporting | — |

**Takeaway:** The proven pattern is: list checkboxes (already built as bulk-operations' reusable selection model) + one shared reason + per-item execution with per-item results + one audit row per item. bulk-operations FR-3 built `AdminTable` selection mode and applied it to users (ban/unban); admin-template-moderation FR-15 explicitly deferred bulk moderation. This feature is the union: selection on `/admin/templates` + the three graduated moderation actions with a shared reason.

## 2. Problem Statement

A spam raid can mean dozens of templates. Today an admin must open each template's detail page and run the unpublish/lock/delete flow individually — each with its own reason dialog. admin-template-moderation FR-15 rate-limits per-admin actions partly to prevent accidental mass-action, but there is no safe, intentional path to action many templates at once with one reason and full per-item audit.

## 3. Target Users

- **Admins** — action many templates at once (unpublish/lock/soft-delete) during raids or cleanups, with one reason and per-item results.

## 4. User Stories

- **US-1**: As an admin, I want to select multiple templates on `/admin/templates` and unpublish them all with one shared reason, so that a raid is handled in one flow.
- **US-2**: As an admin, I want the same for lock and soft-delete, so that the graduated force model works in bulk.
- **US-3**: As an admin, I want per-item results (succeeded/failed with reasons), so that protected or already-actioned templates are visible failures, not silent skips.
- **US-4**: As an admin, I want one audit row per template per action, so that oversight and appeals stay per-item.
- **US-5**: As an admin, I want owners notified per template, so that moderation transparency is identical to single actions.
- **US-6**: As an admin, I want a higher-friction confirmation for bulk delete (type-to-confirm with the count), so that the most destructive bulk action can't be misclicked.
- **US-7**: As an admin, I want my own templates excluded (per-item failure), so that the self-guard from single moderation holds in bulk.
- **US-8**: As an admin, I want bulk restore from the soft-deleted filter view, so that mass mistakes are mass-correctable.

## 5. Functional Requirements

- **FR-1**: `/admin/templates` enables the bulk-operations selection mode (checkbox column, tri-state header, count, 50-item cap per batch, selection clears on filter change) with floating bulk action bar actions: **Unpublish**, **Lock**, **Delete**, **Restore**, **Clear**.
- **FR-2**: Each bulk action opens a confirmation modal reusing the single-action consequence checklist (admin-template-moderation FR-9), showing the count, the first 5 titles ("+N more"), and a **shared reason** field (3–500 chars, same validation).
- **FR-3**: Bulk **delete** additionally requires typing the count (e.g. `12`) to confirm; the typed value must match the selected count exactly.
- **FR-4**: New batch RPCs: `BatchUnpublishTemplates(template_ids[], reason)`, `BatchLockTemplates(template_ids[], reason)`, `BatchDeleteTemplates(template_ids[], reason)`, `BatchRestoreTemplates(template_ids[])` — all under `/api/v1/admin`, all enforcing the 50-item cap (`invalid_argument` beyond).
- **FR-5**: Each RPC executes per-item with item-level atomicity and returns `results[] = {template_id, success, error_code?}`; per-item failure codes include `not_found`, `invalid_state` (e.g. restore on non-soft-deleted), `self_guard` (admin owns the template), and `conflict` (already in target state).
- **FR-6**: Every item executes the full single-action semantics: state transition, moderation notification to the owner (with the shared reason), audit row (one per template, action code + reason + actor), and report auto-resolution where applicable (admin-template-moderation FR-10).
- **FR-7**: Bulk **restore** applies only to soft-deleted templates; non-soft-deleted selections return per-item `invalid_state`. Restore requires no reason (mirrors single restore).
- **FR-8**: After a bulk action: the list refreshes, selection clears, and a result toast shows "Done: X succeeded, Y failed"; failures expand to a per-item list (title + error message); failed items remain in the list unselected.
- **FR-9**: The bulk action bar's available actions are state-aware: **Restore** appears only when every selected item is soft-deleted; **Unpublish/Lock/Delete** are hidden (not just disabled) when any selected item is soft-deleted.
- **FR-10**: Bulk actions bypass the single-action per-admin rate limit only in favor of a batch-specific limit: max 5 batch calls per admin per 10 minutes (429 with retry-after); the 50-item cap bounds each call.
- **FR-11**: The status filter (admin-template-moderation FR-8) and selection compose: admins typically filter to `normal` for raids (unpublish/lock/delete) or `soft_deleted` (restore).
- **FR-12**: All batch RPCs write audit rows with a `bulk` marker in the reason field context (audit metadata `bulk: true, batch_size: N`) so oversight can distinguish bulk from single actions without a schema change.

## 6. Non-Functional Requirements

- **NFR-1 (Performance)**: A 50-item batch completes in < 5 s p95 (per-item work is bounded; notifications are queued, not inline).
- **NFR-2 (Auditability)**: One audit row per item — never one row for the batch; the batch is reconstructable via the shared reason + time window.
- **NFR-3 (Safety)**: The most destructive path (bulk delete) has two independent frictions: shared reason + typed count.
- **NFR-4 (i18n)**: All UI strings EN/ZH; the shared reason is admin content, delivered as-is to owners.
- **NFR-5 (Consistency)**: Per-item outcomes are byte-identical to running the single action — no bulk-specific state semantics.

## 7. Acceptance Criteria

- **AC-1**: Selecting 12 templates and choosing Unpublish opens the modal with the consequence checklist, "12 templates", first 5 titles, and the shared reason field; confirming unpublishes all 12 and notifies all 12 owners.
- **AC-2**: A batch including an admin-owned template completes the other items and reports that one as `self_guard` failure; the toast shows "Done: 11 succeeded, 1 failed".
- **AC-3**: Bulk delete with 8 selected requires typing `8`; typing `7` keeps the confirm button disabled.
- **AC-4**: Bulk restore on a mixed selection (2 soft-deleted + 1 normal) restores the 2 and reports the 1 as `invalid_state`.
- **AC-5**: Each of 12 unpublishes produces 12 audit rows with the shared reason and `bulk: true` metadata.
- **AC-6**: Selecting 51 templates shows the cap toast and refuses the batch (bulk-operations FR-2 behavior).
- **AC-7**: After a bulk action with failures, the expandable list shows each failed title with its error; failed items remain in the list, unselected.
- **AC-8**: A 6th batch call within 10 minutes returns 429 with retry-after.
- **AC-9**: With any soft-deleted template selected, the bar shows only Restore and Clear (Unpublish/Lock/Delete hidden).
- **AC-10**: Reports pending on bulk-unpublished templates auto-resolve as actioned (single-action FR-10 parity).

## 8. Out of Scope

- Cross-page selection ("select all N matching" beyond the current page) — bulk-operations defers it globally.
- Bulk actions on comments, users, or collections (bulk-operations covers users; comments deferred there).
- Bulk hard-delete (hard deletion remains the 30-day ticker's job, admin-template-moderation FR-16).
- Bulk appeal resolution (moderation-appeals resolves one at a time deliberately).
- Mixed-action batches (one action per batch).
- Monetization of any kind — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1**: bulk-operations FR-1–3 selection model and `AdminTable` selection mode are built and reusable as-is.
- **D-2**: admin-template-moderation's single-action semantics (state machine, notifications, audit codes, report resolution, self-guard) are the per-item building blocks (FR-6).
- **D-3**: The floating bulk action bar component from bulk-operations FR-4 is extended with the moderation actions.
- **D-4**: The result toast + expandable failures pattern (bulk-operations FR-8) is reused verbatim.
- **D-5**: Notification emission is async/queued so a 50-item batch doesn't block on 50 notification writes.

## 10. Open Questions for Architect

- **OQ-1**: Should the batch RPCs be new endpoints (`POST /api/v1/admin/templates:batchUnpublish` style) or a generic `POST /api/v1/admin/templates/batch` with an `action` field? (Proposal: separate endpoints, mirroring bulk-operations' BatchBanUsers/BatchUnbanUsers precedent.)
- **OQ-2**: Is the 5-batches-per-10-minutes limit (FR-10) the right shape, or should it be item-based (e.g. 200 items/hour)?
- **OQ-3**: Should report auto-resolution (FR-6) happen inline per item or in the same async pass as notifications?
- **OQ-4**: Does `bulk: true` audit metadata require an audit-log schema addition, or is a reason-prefix convention acceptable? (Proposal: metadata column if the audit table is JSON-typed; prefix otherwise.)
- **OQ-5**: Should bulk restore also clear the 30-day countdown rows from the ticker's scan set explicitly, or does the existing restore path already handle it?
