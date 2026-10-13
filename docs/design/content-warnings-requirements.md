# Content Warnings — Requirements Analysis

- **Feature ID:** content-warnings
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-13
- **Companion doc:** `content-warnings-ui-ux-design.md`

## 1. Competitor Research

> Note: live web research was unavailable this session; the table below is
> compiled from product knowledge of each platform's behavior.

| Product | Capability | Strength | Weakness |
|---|---|---|---|
| YouTube | Age-restriction + sensitive-content screens; creator self-certification questionnaire; viewers see an interstitial before playback | Clear, consistent gating; protects viewers without deleting content | Criteria opaque; heavy appeal volume |
| X (Twitter) | Creator flags media as sensitive → blurred with a label, click to reveal; user setting to always display sensitive media | One-click reveal; viewer-controlled default | Inconsistent self-tagging; easy to bypass |
| Reddit | NSFW tag on posts; blurred thumbnails; profile setting to show NSFW; moderators can toggle the tag | Simple binary; community + self tagging | Binary only — no granularity of reason |
| Twitch | Streamer-declared content classification labels from a fixed taxonomy (gambling, drugs, mature-rated game), shown as tags | Fixed taxonomy, declared pre-emptively, no gating friction | Label-only — no visual gate at all |
| Etsy | "Mature content" listing flag; mature listings hidden from some surfaces | Marketplace precedent for policy middle ground | Blunt single flag |
| FlowGPT | NSFW filter toggle in browse; NSFW prompts exist behind the filter | Prompt-platform proof that demand exists | Coarse; relies on creator honesty |
| PromptBase | Prohibits NSFW outright (policy-only) | Zero exposure risk | Legitimate mature use cases (fiction, security) have no home |

**Takeaway:** The winning pattern is **Twitch's fixed taxonomy + X's blur/reveal + Reddit's viewer preference**: a small fixed set of warning reasons, creator self-declaration at publish time, blurred previews with a one-click reveal, a viewer preference to always show, and moderator enforcement on top. Prohibition (PromptBase) forfeits legitimate creators; label-only (Twitch) fails viewers; a graduated gate serves both.

## 2. Problem Statement

Open Prompts moderation is binary today: a template is either fully public or taken down. There is no middle layer for sensitive-but-legitimate content. Concretely:

1. **Creators face all-or-nothing.** A fiction-writing prompt with mature themes, a security-testing prompt describing attack techniques, or a medical prompt discussing self-harm risk factors is either published with zero signal to viewers or removed entirely.
2. **Viewers are surprised.** Browsing the square can surface content a viewer did not want to see, with no warning and no way to avoid it.
3. **Moderators have a blunt tool.** The only enforcement is unpublish/lock/delete — too harsh for content that merely needs a label, which pushes moderators toward either inaction or over-removal.
4. **Policy has no expression.** The platform has no mechanism to say "this content is allowed, but gated" — the gap pre-publication-review explicitly deferred to this feature.

A content-warning layer gives the platform a graduated response: content stays discoverable and rankable, but is visually gated and filterable, with creators self-declaring and moderators enforcing.

## 3. Target Users

- **Creators of sensitive-but-legitimate templates** (fiction, security research, medical/clinical topics): need a responsible way to publish without being banned.
- **General viewers**: must never be accidentally exposed; need blur + one-click reveal.
- **Frequent viewers of gated content**: need an "always show" preference so the gate never adds friction to their workflow.
- **Moderators**: need to enforce correct tagging (add/remove warnings) with audit trails, as a lighter alternative to takedowns.

## 4. User Stories

- **US-1:** As a creator, I can tag my template with one or more content warnings from a fixed list when creating or editing it, so my content reaches the right audience responsibly.
- **US-2:** As a creator, I see the applied warnings on my template's card and detail page, so I know the tagging is in effect.
- **US-3:** As a viewer, templates carrying warnings appear blurred in browse and search results with a warning label, so I am never accidentally exposed.
- **US-4:** As a viewer, I can click once to reveal a warned template's content, so the gate is a speed bump, not a wall.
- **US-5:** As a viewer, I can enable "Always show warned content" in my settings, so my workflow has zero extra clicks.
- **US-6:** As a viewer, I can hide warned templates from my browse/search results entirely, so my feed contains only unwarned content.
- **US-7:** As an anonymous visitor, warned templates are blurred and gated for me too, so protection does not require an account.
- **US-8:** As a moderator, I can add or remove warnings on any template with a required reason, so tagging is enforced and auditable.
- **US-9:** As a moderator, I can filter the admin template list by warning state, so I can audit tagging across the catalog.
- **US-10:** As a creator, warnings applied by a moderator are shown locked in my editor and I cannot remove them, so enforcement cannot be silently undone.

## 5. Functional Requirements

- **FR-1:** The system SHALL support a fixed taxonomy of four content warnings: `mature` (mature/sexual themes), `violence` (graphic violence), `profanity` (strong language), `sensitive` (sensitive/disturbing topics). The taxonomy SHALL be configuration-driven so new types can be added without deploy.
- **FR-2:** Creators SHALL set/clear content warnings in the create and edit forms (multi-select checkbox group); the selection is stored on the template and returned as `content_warnings` (array of strings) in all template API responses.
- **FR-3:** In browse, search, and any card list, templates with warnings SHALL render with their content preview blurred and a warning chip naming the reasons; the title remains readable.
- **FR-4:** The detail page of a warned template SHALL show an interstitial before content: the warning reasons and a "Show content" button. Revealing SHALL persist for the browser session (per template).
- **FR-5:** A user setting `show_warned_content` (default off) SHALL, when on, skip both the blur and the interstitial for that user.
- **FR-6:** List and search endpoints SHALL accept an `exclude_warned` boolean; the UI SHALL expose it as a "Hide warned" filter chip, default off, persisted per user (anonymous: URL parameter only).
- **FR-7:** Moderators SHALL add or remove warnings on any template from the admin template detail page; adding or removing requires a reason and writes an audit-log entry (`template.warning_add` / `template.warning_remove`) with actor, target, reason, and the changed warning set.
- **FR-8:** Warnings applied by moderators SHALL be creator-immutable: the creator's editor displays them as locked and the API rejects creator-side removal attempts (403).
- **FR-9:** The admin templates list SHALL support filtering by warning state: no warnings / any warning / a specific warning type.
- **FR-10:** The creator SHALL receive an in-app notification when a moderator adds or removes warnings on their template, including the reason.
- **FR-11:** Warnings SHALL NOT affect search ranking, trending computation, or pre-publication-review gating — visibility gating and quality/risk axes stay independent.
- **FR-12:** Embeds of warned templates (template-embedding-widget surfaces) SHALL present the same interstitial before showing content.
- **FR-13:** The system SHALL NOT allow warnings on anything other than templates in v1 (collections, comments, and profiles are out of scope).

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Blur/reveal is purely client-side; listing endpoints with `content_warnings` in the response add no measurable latency (p95 unchanged).
- **NFR-2 (Configurability):** The warning taxonomy lives in platform settings (same mechanism as pre-publication-review's N/M), changeable without deploy.
- **NFR-3 (Accessibility):** Blur is never the only signal — every gated surface also carries a text label naming the warnings; WCAG 2.1 AA contrast on both themes.
- **NFR-4 (i18n):** All warning names, interstitial copy, settings, and admin strings exist in English and Chinese with identical key sets.
- **NFR-5 (Auditability):** Every moderator warning change is attributable and immutable (existing `admin_audit_logs` semantics).
- **NFR-6 (Scale):** Filtering by warning state on the admin list and `exclude_warned` on public lists perform at catalog scale (10k+ templates) without degradation.

## 7. Acceptance Criteria

- **AC-1:** A creator tags `mature` at creation → the card shows a blurred preview with a "Mature" chip; the detail page shows the interstitial; the API returns `content_warnings: ["mature"]`.
- **AC-2:** A viewer clicks "Show content" → content is revealed; reloading in the same session keeps it revealed; a new session shows the interstitial again.
- **AC-3:** A viewer with `show_warned_content` on sees no blur and no interstitial anywhere.
- **AC-4:** With the "Hide warned" filter on, warned templates are absent from browse and search results; the filter state survives reload for logged-in users.
- **AC-5:** A moderator adds `violence` with reason → audit log contains `template.warning_add` with actor/reason; the creator receives a notification with the reason; the creator's editor shows `violence` locked.
- **AC-6:** A creator's API attempt to remove a moderator-applied warning returns 403; removing their own self-applied warning succeeds.
- **AC-7:** The admin templates list filtered by "any warning" shows only warned templates; filtered by `mature` shows only templates carrying `mature`.
- **AC-8:** An anonymous visitor sees the blur and interstitial identically to a logged-in viewer (minus the preference).
- **AC-9:** A warned template's position in trending/newest/most-liked sorts is identical to what it would be without warnings (no ranking effect).
- **AC-10:** An embed of a warned template shows the interstitial; revealing inside the embed does not reveal the template page elsewhere.

## 8. Out of Scope

- Age verification or age-gated accounts (viewers self-select via preference).
- Warnings on collections, comments, user profiles, or prompts (templates only).
- Per-warning viewer preferences (single global "always show" toggle in v1).
- Automatic/AI-assisted warning suggestions.
- Warning-gated API access for SDK consumers (API returns the array; clients decide).
- Email notifications (in-app only).
- Monetization of any kind — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1:** admin-template-moderation provides the admin console shell, `/admin/templates` list + detail pages, the reason-modal pattern, and `admin_audit_logs` — this feature adds a panel and two action codes.
- **D-2:** The notifications system delivers creator notifications; a `template.warning_changed` event type is added (data-driven registration per the mention-notifications pattern).
- **D-3:** template-embedding-widget surfaces are updated to honor the interstitial (its design assumes embed content renders; this feature gates it).
- **D-4:** Platform settings storage (also needed by pre-publication-review NFR-5) holds the taxonomy; if absent, the architect adds it.
- **D-5:** The templates table gains a `content_warnings` column (TEXT[] or JSONB — architect's choice); migration backfills empty arrays.
- **D-6:** pre-publication-review's review queue shows warning chips on submissions so reviewers see self-declared warnings in context (additive display, no gating interaction per FR-11).

## 10. Open Questions for Architect

- **OQ-1:** `content_warnings` as a Postgres array column (simple, indexable via GIN) vs. a `template_warnings` join table (more metadata: who applied, when)? Moderator-applied lock (FR-8) needs an origin marker either way — column with a parallel `locked_warnings` array, or origin on the join table?
- **OQ-2:** Should `exclude_warned` default to ON for anonymous visitors (policy: maximize protection) or OFF (policy: maximize discoverability)? Design assumes OFF with visible blur.
- **OQ-3:** Session-scoped reveal (design choice) vs. per-template persistent "revealed" list per user — which storage boundary?
- **OQ-4:** Should a moderator warning change on an `in_review` submission be visible in the pre-publication-review queue as a signal (non-blocking), or kept entirely separate?
- **OQ-5:** Does the notifications event carry the full new warning set or just the delta, and does the creator notification deep-link to the template or to a help page about warnings?
