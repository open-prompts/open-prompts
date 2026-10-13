# Moderation Appeals — Requirements Analysis

- **Feature ID:** moderation-appeals
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-13
- **Companion doc:** `moderation-appeals-ui-ux-design.md`

## 1. Competitor Research

| Product | Appeal capability | Strength | Weakness |
|---|---|---|---|
| YouTube Studio | Creator can appeal each removal; one appeal per action; decision in ~days | Structured, per-action, tracked | Opaque criteria; no dialogue |
| GitHub (repo takedown) | Owner opens a support ticket; restore on reversal | Human review, documented | Out-of-band; no status surface |
| Reddit | Modmail conversation per action | Two-way dialogue | Unbounded threads; no SLA |
| Discord | Server owner can appeal via support form | Simple entry | No status tracking |
| PromptBase/FlowGPT | none | — | No moderation systems to appeal yet |

**Takeaway:** The YouTube model fits best: **one appeal per moderation action**, a structured form (reason + evidence-free text), a tracked case with explicit states (open → under review → upheld/reversed), and an admin queue to resolve. Dialogue is one-round (creator states case; admin decides with a resolution note) — not an unbounded thread (Reddit's weakness). This closes the loop admin-template-moderation left out-of-band.

## 2. Problem Statement

admin-template-moderation gives creators a reason and a "contact support" hint, but no in-product path to contest a decision. Mistakes have no structured reversal request: the creator must find an out-of-band channel, and admins have no queue to triage disputes. The audit log grounds appeals in facts (US-M9) but nothing consumes it.

## 3. Target Users

- **Creators** — contest a moderation action on their template with a structured, tracked appeal.
- **Admins** — triage an appeal queue, review the action's audit context, and reverse or uphold with a resolution note.

## 4. User Stories

- **US-1**: As a creator, I want to appeal a moderation action (unpublish/lock/delete) on my template from the notice itself, so that contesting is one click away.
- **US-2**: As a creator, I want to state my case in a structured form (why the action is wrong), so that admins get a clear, reviewable argument.
- **US-3**: As a creator, I want to see my appeal's status (open/under review/resolved) and outcome, so that I'm not left wondering.
- **US-4**: As a creator, I want exactly one appeal per moderation action, so that the process can't be spammed.
- **US-5**: As a creator, I want a notification when my appeal is resolved, with the outcome and the admin's note.
- **US-6**: As an admin, I want an appeal queue ordered by age with template and action context, so that I can triage disputes quickly.
- **US-7**: As an admin, I want the original action's audit entry (actor, reason, timestamp) inline with the appeal, so that decisions are grounded in facts.
- **US-8**: As an admin, I want to resolve an appeal as reversed or upheld with a required note, so that the creator always gets an explanation.
- **US-9**: As an admin, I want a reversed unpublish/delete to restore the template automatically, so that the remedy matches the ruling.
- **US-10**: As an admin, I want the appealant to be different from the resolver, so that self-review is impossible.

## 5. Functional Requirements

- **FR-1**: A creator can open an appeal against any moderation action (unpublish, lock, soft-delete) taken on a template they own, from: the owner-facing banner/removed-notice (admin-template-moderation FR-12), the moderation notification, or `/appeals` (new page).
- **FR-2**: The appeal form contains: the referenced action (read-only context: action type, reason, actor display name, timestamp), and a case statement (required, 50–2000 chars). No attachments in v1.
- **FR-3**: Exactly **one appeal per moderation action** — a second attempt returns 409 with "This action has already been appealed"; the UI disables the appeal entry point and shows the existing appeal's status instead.
- **FR-4**: Appeal states: `open` → `under_review` → `resolved_upheld` | `resolved_reversed`. State transitions are admin-only except the initial create.
- **FR-5**: User APIs: `POST /api/v1/appeals` (body: `{moderation_action_id, statement}`), `GET /api/v1/appeals` (own appeals, paginated 20/page, newest first), `GET /api/v1/appeals/{id}`.
- **FR-6**: Admin APIs: `GET /api/v1/admin/appeals?state=open|under_review|resolved&page...` (queue, oldest first within state), `GET /api/v1/admin/appeals/{id}`, `POST /api/v1/admin/appeals/{id}/review` (open → under_review), `POST /api/v1/admin/appeals/{id}/resolve` (body: `{outcome: upheld|reversed, note}`; note required 20–1000 chars).
- **FR-7**: Resolving as `reversed` automatically restores the template (equivalent to admin-template-moderation restore) when the action was unpublish or soft-delete; for lock, it unlocks. The restore is atomic with the resolution in one transaction.
- **FR-8**: Resolving as `upheld` changes nothing about the template; the moderation status stands.
- **FR-9**: The resolver MUST NOT be the actor of the original moderation action (self-review guard; 403 with explanation).
- **FR-10**: On resolution, the creator receives a notification (new `appeal_resolved` type, data-driven registration) with outcome and note excerpt, deep-linking to the appeal detail.
- **FR-11**: `/appeals` (user page) lists the creator's appeals with state chips, action context, and timestamps; the detail view shows the full case statement, the original action context, and the resolution note when resolved.
- **FR-12**: Admin console: `/admin/appeals` queue with state filter tabs (open/under review/resolved), age ordering, and counts; `/admin/appeals/:id` detail with the appeal, the audit entry of the original action, and Review/Resolve actions.
- **FR-13**: All appeal actions (create, review, resolve) are written to the audit log with actor, appeal id, and outcome where applicable.
- **FR-14**: Appeals are possible only while the underlying moderation action exists (not undone by a later restore — a restored template's actions show "superseded" and cannot be appealed; the appeal entry point is hidden).
- **FR-15**: A creator with an open or under-review appeal on an action sees its status inline on the owner banner (replacing the appeal CTA).
- **FR-16**: Appeal creation is rate-limited to 5 open appeals per user simultaneously; exceeding returns 429 with a clear message.

## 6. Non-Functional Requirements

- **NFR-1 (Performance)**: The admin queue loads in < 200 ms p95 for 1000 appeals (indexed on state + created_at).
- **NFR-2 (Fairness)**: Appeals are FIFO within state; no priority mechanism in v1.
- **NFR-3 (i18n)**: All UI strings EN/ZH; case statements and notes are user/admin content, shown as-is.
- **NFR-4 (Transparency)**: The creator always sees the outcome and note; there is no silent resolution.
- **NFR-5 (Safety)**: Statements and notes are plain text (no HTML/Markdown); length-bounded; rendered as text (XSS-safe by construction).

## 7. Acceptance Criteria

- **AC-1**: From the removed-notice page, "Appeal this decision" opens the form pre-filled with the action context; submitting creates an `open` appeal.
- **AC-2**: Submitting a second appeal for the same action returns 409; the UI shows the existing appeal's status instead of the form.
- **AC-3**: An admin clicking "Review" moves the appeal to `under_review`; the creator sees the state change on `/appeals`.
- **AC-4**: Resolving as `reversed` on a soft-delete appeal restores the template and flips its moderation status to `normal` in the same transaction; the creator gets an `appeal_resolved` notification.
- **AC-5**: Resolving as `upheld` leaves the template untouched; the notification states the outcome with the admin's note.
- **AC-6**: The original action's actor attempting to resolve the appeal receives 403 with "The original action's actor cannot resolve this appeal".
- **AC-7**: `/admin/appeals` shows state tabs with counts; open appeals are oldest-first.
- **AC-8**: A restored (admin-restored, not appeal-reversed) template's past actions show "superseded" and offer no appeal entry.
- **AC-9**: A user with 5 open appeals submitting a 6th receives 429 with the limit message.
- **AC-10**: A statement containing `<script>` renders as literal text everywhere it appears.

## 8. Out of Scope

- Multi-round dialogue/threaded back-and-forth (one statement + one resolution note).
- Attachments/evidence uploads; third-party arbitrators; SLA timers or auto-escalation.
- Appeals of account-level actions (bans — admin-user-management territory).
- Appeals of comment deletions (admin-template-moderation covers template actions only).
- Monetization of any kind — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1**: admin-template-moderation's moderation actions are audit-log entries with stable ids; appeals reference `moderation_action_id` (OQ-1: audit entry id vs dedicated moderation_actions table).
- **D-2**: The restore path (admin-template-moderation FR-5) is reusable as the reversal remedy (FR-7).
- **D-3**: The `appeal_resolved` notification type registers via the data-driven pattern (mention-notifications).
- **D-4**: The admin console shell and audit-log reader from admin-user-management host the queue pages.
- **D-5**: Owner banners from admin-template-moderation FR-12 gain the appeal CTA/status slot (FR-1, FR-15).

## 10. Open Questions for Architect

- **OQ-1**: Should appeals reference audit-log entry ids, or should admin-template-moderation introduce a dedicated `moderation_actions` table that appeals (and the history UI) read from? (Proposal: dedicated table — cleaner lifecycle for FR-14 superseded states.)
- **OQ-2**: Should `under_review` be required before resolve, or can an admin resolve directly from `open`? (Proposal: allow direct resolve — two states is ceremony for a small team.)
- **OQ-3**: Does the reversal restore need a distinct audit code (`template.restored_by_appeal`) vs reusing `template.restore` with an appeal reference?
- **OQ-4**: Should the 5-open-appeals limit (FR-16) be configurable server-side?
- **OQ-5**: Is the notification's note excerpt length (e.g. 140 chars) acceptable, or should it deep-link only?
