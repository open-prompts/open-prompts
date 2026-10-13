# Pre-Publication Review — Requirements Analysis

- **Feature ID:** pre-publication-review
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-13
- **Companion doc:** `pre-publication-review-ui-ux-design.md`

## 1. Competitor Research

| Product | Capability | Strength | Weakness |
|---|---|---|---|
| YouTube | Manual review for some channels/edits before content goes live; creator sees "in review" status | Clear status; protects viewers from policy-violating uploads | Slow at scale; opaque timelines |
| Medium | New publications from small publications may be reviewed; spam checks before distribution | Lightweight; most content unaffected | Criteria unclear to creators |
| PromptBase | Every listed prompt is reviewed by staff before it appears in the marketplace | High floor of quality; consistent presentation | Long wait; rejection reasons sometimes generic |
| FlowGPT | Post-moderation only (report-driven) after publish | Zero publishing friction | Bad content can be live before anyone sees it |
| Etsy | Listing review before first publication per shop | First-listing gate catches most abuse at onboarding | One-time only; later listings skip review |

**Takeaway:** Two viable models — gate everything (PromptBase/Etsy-style, high quality floor but heavy reviewer load and slow publishing) or gate selectively (YouTube-style, risk-based). For a growing platform, selective gating (first-N templates per creator, plus re-review triggers) gives most of the quality benefit at a fraction of the review load, and creators always see a transparent status.

## 2. Problem Statement

Today every saved template is instantly public. Moderation is reactive: content goes live, and only reports or admin patrols unpublish it. This means policy-violating templates can collect views, shares, and embeds before anyone reviews them, and takedowns generate creator friction (and appeals). New creators have no track record, so the platform cannot distinguish a good-faith first template from a drive-by policy violation. A pre-publication review queue — applied selectively — lets the platform catch problems before exposure while keeping publishing nearly instant for established creators.

## 3. Target Users

- **New creators** publishing their first templates (subject to review; need clear status and feedback).
- **Established creators** (exempt or lightly gated; must not experience added friction).
- **Admins/moderators** reviewing submissions (need a fast queue with context and consistent actions).
- **Regular browsers** (must never see in-review content; unaware this feature exists).

## 4. User Stories

- **US-1:** As a new creator, when I publish my first template, it enters review and I see an honest "In review" status with an expected timeframe, so I know what to expect.
- **US-2:** As a new creator, if my template is rejected, I receive the specific policy reason and can edit and resubmit, so I can succeed next time.
- **US-3:** As a new creator, once I have N approved templates, my subsequent publishes go live instantly, so good behavior is rewarded.
- **US-4:** As an established creator, my publishes are never delayed by review, so my workflow is unchanged.
- **US-5:** As a moderator, I see a queue of submissions ordered oldest-first with template content, author history, and prior decisions, so I can decide quickly and consistently.
- **US-6:** As a moderator, I can approve or reject with a required reason for rejection, so creators get actionable feedback.
- **US-7:** As a moderator, I can see when a submission is a resubmission of a previously rejected template, so repeat offenders are visible.
- **US-8:** As a banned-then-reinstated creator, my publishes are re-gated, so evasion via new templates is harder.

## 5. Functional Requirements

- **FR-1:** The system SHALL gate publication for review based on creator state: creators with fewer than N (default 3) approved templates have each publish enter `in_review`; creators at or above N publish instantly.
- **FR-2:** The system SHALL re-gate a creator (next M publishes, default 3) after any template of theirs is unpublished by moderation, so violations reset trust.
- **FR-3:** A gated template SHALL have `moderation_status = in_review` (reusing the admin-template-moderation state machine), be invisible to all non-admin users and excluded from search, feeds, and public profiles, but remain editable by its creator.
- **FR-4:** The creator SHALL see, on their dashboard, an "In review" badge with a plain-language expected timeframe (target: 24 hours) and a link to the template.
- **FR-5:** On rejection, the system SHALL set the template to `rejected` (unpublished), record the policy reason (selected from a fixed list plus optional free-text note), and notify the creator with the reason.
- **FR-6:** A rejected template SHALL be editable by its creator; publishing the edited version SHALL create a new review submission linked to the prior rejection (resubmission chain visible to moderators).
- **FR-7:** On approval, the system SHALL set the template to `published`, set `published_at` to approval time, notify the creator, and count toward the creator's approved total for FR-1.
- **FR-8:** The admin console SHALL provide a review queue at `/admin/review` listing `in_review` templates oldest-first, with filters (category, submission age, resubmission-only) and pagination.
- **FR-9:** The queue detail view SHALL show full template content, author summary (joined date, template count, prior rejections), and the resubmission chain if any.
- **FR-10:** Moderators SHALL approve or reject from the queue; rejection requires a reason from the fixed list; both actions are audit-logged with actor, timestamp, and reason.
- **FR-11:** The system SHALL notify the creator on every decision (approved / rejected with reason) via in-app notification.
- **FR-12:** The system SHALL surface queue metrics to admins: open count, median time-in-review, decisions today, rejection rate by reason.
- **FR-13:** The system SHALL auto-escalate submissions older than 48 hours to the top of the queue with an "aging" indicator.
- **FR-14:** The system SHALL NOT gate edits to already-published templates of exempt creators (edits stay live; reactive moderation covers them).

## 6. Non-Functional Requirements

- **NFR-1 (Latency):** Queue list and detail load in under 500 ms (p95) at 1,000 open submissions.
- **NFR-2 (Fairness):** FIFO ordering with aging escalation; no submission waits longer than 72 hours without escalation visibility.
- **NFR-3 (Auditability):** Every decision (approve/reject) writes an immutable audit entry; decisions are attributable to the acting admin.
- **NFR-4 (Transparency):** Creators always know their template's current status; no silent rejections.
- **NFR-5 (Configurability):** N (approval threshold) and M (re-gate count) are admin-configurable platform settings, changeable without deploy.
- **NFR-6 (Scale):** Review queue supports 10,000 open submissions without pagination degradation.

## 7. Acceptance Criteria

- **AC-1:** Given a creator with 0 approved templates, when they publish, the template enters `in_review`, is invisible to non-admins, and the creator sees an "In review" status with timeframe.
- **AC-2:** Given a creator with 3 approved templates, when they publish, the template goes live immediately with no review step.
- **AC-3:** Given an `in_review` template, when a moderator approves it, it becomes publicly visible, `published_at` is set to approval time, and the creator is notified.
- **AC-4:** Given an `in_review` template, when a moderator rejects it with reason "Spam", the template becomes `rejected`, invisible publicly, and the creator receives a notification containing that reason.
- **AC-5:** Rejection without a selected reason is rejected by the API (400) and the UI disables the reject button until one is chosen.
- **AC-6:** Given a rejected template, when the creator edits and republishes, a new submission is created and moderators see "Resubmission (2nd)" with the prior reason.
- **AC-7:** Given a creator whose template was unpublished by moderation, their next 3 publishes are gated for review.
- **AC-8:** Given a submission older than 48 hours, it appears at the top of the queue with an "aging" badge.
- **AC-9:** A regular user searching or browsing never sees `in_review` or `rejected` templates, including via direct URL (404-equivalent: not-found page).
- **AC-10:** Queue metrics endpoint returns open count, median time-in-review, decisions today, and rejection rate by reason.

## 8. Out of Scope

- Reviewing edits to already-published templates of exempt creators.
- Third-party / automated AI pre-screening (may inform, never decides).
- Creator-initiated expedited review requests.
- Age-restriction or content-warning gating (separate future feature).
- Email notifications (in-app only; email infra is out of scope).
- Monetization of any kind — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1:** admin-template-moderation's `moderation_status` state machine and admin RPC patterns exist and are extended, not replaced.
- **D-2:** In-app notifications (notifications feature) deliver decision notifications; `template_approved` and `template_rejected` types are added.
- **D-3:** Audit logging from admin-template-moderation is reused for decisions.
- **D-4:** Platform settings storage exists (or is added by architect) for N and M configuration.
- **D-5:** Template versioning is not required for resubmission chains; the chain is a link between submission records, not versions.

## 10. Open Questions for Architect

- **OQ-1:** Should gating state be computed on the fly (count approved templates at publish time) or maintained as a denormalized `trust_level` on the user row for performance?
- **OQ-2:** Is a separate `review_submissions` table (with resubmission chain) preferable to reusing `moderation_actions` with a new action type?
- **OQ-3:** Should `in_review` templates be excluded from the creator's own public profile listing (they see it on dashboard; do others see a count discrepancy)?
- **OQ-4:** What happens to an `in_review` template if the creator is banned mid-review — auto-reject, or leave for a moderator to clean up?
- **OQ-5:** Should the aging escalation (FR-13) be a periodic ticker, a computed sort key, or both?
