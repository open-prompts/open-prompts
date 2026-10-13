# Template Health Hints — Requirements Analysis

- **Feature ID:** template-health-hints
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-13

## 1. Competitor Research Summary

| Platform | Quality/completeness approach | Key takeaways |
|---|---|---|
| **LinkedIn** | Profile completeness meter (0–100%) with named next-actions ("Add a skill") | A single score plus one-click next actions is the most-copied engagement pattern in existence; each hint must link to its fix |
| **Etsy** | Listing quality checklist: photos count, tags filled, description length; visible before publish | Pre-publish checklists raise content quality at the source; showing checks *before* publishing beats fixing after |
| **Shopify** | Product page health score with weighted factors and inline explanations | Weighted, explainable factors beat opaque scores; the explanation is the product |
| **Google Search Console** | Coverage reports: error/warning/valid states per page with drill-down | Three severity levels (error/warning/ok) map cleanly to content checks |
| **Webflow / site builders** | Pre-publish checklist modal (SEO title, description, OG image) | A compact checklist at the moment of publishing catches issues when attention is highest |
| **Prompt marketplaces (PromptBase/FlowGPT)** | No quality tooling; weak listings abound | Quality tooling is an unclaimed differentiator; better listings improve search/discovery for everyone |

**Synthesis:** The proven pattern is a **transparent, weighted checklist with severity levels and one-click fixes**: LinkedIn proves the score+next-action loop; Etsy/Shopify prove checklists raise quality at creation time; Search Console proves the error/warning/ok triad. For Open Prompts, the checks are computable from existing template metadata and content (description presence, tag count, category set, variable naming quality, content length band, update recency, change notes). The feature is creator-facing and advisory — never blocking, never scoring users publicly.

## 2. Problem Statement

Nothing in Open Prompts tells a creator their template is under-specified before users hit the problems:

1. **Weak listings ship silently.** A template with no description, no tags, and no category is published without any nudge. It is then undiscoverable (search and filtering depend on those fields) and looks abandoned in results.
2. **Variable quality issues survive editing.** advanced-editor validates syntax *while editing*, but a creator who never reopens the editor never sees that their old template has `$$Name$$` vs `$$name$$` near-duplicates or an empty `$$$$` from an earlier era.
3. **No holistic view.** The user dashboard lists templates with stats (user-dashboard) but no quality signal; a creator with 15 templates cannot see which ones need attention.
4. **Staleness is invisible.** Templates not updated in 6 months (with variable syntax drift across the ecosystem) are indistinguishable from fresh ones in the creator's own list.

## 3. Target Users

- **Creators (primary):** users who publish and maintain templates and want their work to be discoverable and trustworthy.
- **New creators:** guided by the checklist toward well-formed first templates (complements onboarding's activation goal).
- **Browsers (indirect):** benefit from better-described, better-tagged templates in search results (template-search-filtering matches on these fields).
- **Admins:** no admin surface in v1 (§8); health is advisory and creator-owned, not a moderation score.

## 4. User Stories

### 4.1 Seeing health

- **US-H1:** As a creator, I see a health indicator (score + severity color) next to each of my templates on the dashboard, so that I know which need attention.
- **US-H2:** As a creator, I can open a health checklist for any of my templates showing each check with pass/warning/error state and a plain-language explanation, so that I understand exactly what to fix.
- **US-H3:** As a creator, each failed check links directly to the place I fix it (edit mode, tag editor, category picker), so that fixing is one click.
- **US-H4:** As a creator, I see the health checklist before publishing a private template or after creating a new one, so that quality issues surface at the moment of highest attention.

### 4.2 Understanding the score

- **US-H5:** As a creator, I understand how the score is computed (each check's weight is visible), so that the score is trusted, not a black box.
- **US-H6:** As a creator, my health state is private to me — visitors never see my template's health score, so that hints advise without shaming.

### 4.3 System stories

- **US-H7:** As the system, I compute checks from existing template metadata and content only, so that no new data collection is needed.
- **US-H8:** As the system, I recompute health on template change (create/update/version/publish), so that the checklist is never stale.

## 5. Functional Requirements

### 5.1 Health checks

- **FR-1:** The system shall compute the following checks per template (severity: error / warning / info; weight):
  | Check | Condition | Severity | Weight |
  |---|---|---|---|
  | `has_description` | description non-empty, ≥ 20 chars | error if empty; warning if < 20 | 15 |
  | `has_tags` | ≥ 1 tag; ≥ 3 tags for full pass | error if 0; warning if 1–2 | 15 |
  | `has_category` | category set | error if unset | 10 |
  | `variable_quality` | all variables valid names (reuse advanced-editor FR-4 rules: no empties, no whitespace-only, no unclosed delimiters, no case near-duplicates) | error on any invalid; warning on case near-duplicates only | 20 |
  | `content_length` | content between 50 and 20,000 chars | warning if outside band | 10 |
  | `has_change_note` | latest version has a change note (template-versioning field) | info if missing | 5 |
  | `recently_updated` | updated within 180 days | info if older | 10 |
  | `has_language` | language field set | info if unset | 5 |
- **FR-2:** The health score shall be the sum of weights of passing checks (0–90, normalized to 0–100%); checks with severity `error` failing cap the score at 49% regardless of other passes.
- **FR-3:** Check results shall include a `fix_action` identifier (`edit_description`, `edit_tags`, `edit_category`, `edit_content`, `add_change_note`, `edit_language`, `none`) so the UI can deep-link to the right editor surface.
- **FR-4:** Checks are advisory only: no check blocks saving, publishing, or listing. Health never affects search ranking or visibility in v1.

### 5.2 Surfaces

- **FR-5:** The user dashboard's template list (user-dashboard feature) shall show a compact health badge per template: score % with severity color (green ≥ 80, amber 50–79, red < 50), visible to the owner only.
- **FR-6:** The template detail page shall show an owner-only "Template health" panel (collapsible, below owner stats if prompt-analytics has shipped): score, checklist rows (check name, state icon, explanation, fix link).
- **FR-7:** After creating a template or saving a new version, a lightweight toast shall surface the count of failed checks ("3 improvements available — View checklist") linking to the panel; it is dismissible and never modal.
- **FR-8:** The dashboard shall offer a "Needs attention" sort/filter (health < 80%) for the owner's template list.
- **FR-9:** Health surfaces render for owners only; other viewers (including admins viewing another user's template) see nothing.

### 5.3 Computation & freshness

- **FR-10:** Health shall be computed server-side on read (dashboard list, detail page) from current template state; no stored score column in v1 (computation is 7 cheap checks over already-fetched fields — OQ-1).
- **FR-11:** The `variable_quality` check shall reuse the shared variable parsing/validation utility (advanced-editor NFR-6) server-side; the same rules apply in both places.
- **FR-12:** Where the client already has full template data (detail page), the client may compute the checklist locally with the identical rules; server and client implementations share one specification (§5.1) to prevent drift.

### 5.4 Regular-user vs admin split

- **FR-13:** All surfaces are regular-user (owner) surfaces. No admin console pages; no `/api/v1/admin` endpoints; admin moderation (admin-user-management) does not consume health data in v1.

### 5.5 API

- **FR-14:** `GET /api/v1/templates/{id}/health` → `{score, checks: [{id, state: pass|warning|error|info, explanation_key, fix_action}]}`. Owner-only (403 otherwise).
- **FR-15:** The dashboard list endpoint may embed a `health_score` integer per owned template (additive field; existing clients ignore it) — preferred over N+1 detail calls (OQ-2).

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Health computation adds < 5ms per template server-side (pure field checks + one regex pass over content); dashboard list with 50 templates stays within existing latency budget.
- **NFR-2 (Privacy):** Health is private to the owner (FR-9); scores are never exposed on public pages, APIs for anonymous users, or admin surfaces.
- **NFR-3 (Non-judgmental tone):** All copy is advisory ("improvement available"), never shaming ("bad template"); severity colors follow existing semantic palette.
- **NFR-4 (Compatibility):** No schema changes; no existing endpoint behavior changes (additive field only, FR-15).
- **NFR-5 (Accessibility):** Health badge conveys state via text+icon (not color alone); checklist is keyboard navigable; fix links are standard links (UI/UX doc §9).
- **NFR-6 (i18n):** All check names and explanations in EN and ZH with identical key sets; explanations are i18n keys resolved client-side (server returns `explanation_key`).
- **NFR-7 (Determinism):** Same template state → same score, always; the check specification in §5.1 is the single source of truth for server and client implementations.

## 7. Acceptance Criteria

- **AC-1:** A template with no description, no tags, and no category shows a red badge (< 50%) on the owner's dashboard; the checklist shows the three failed checks with fix links.
- **AC-2:** Clicking the `edit_tags` fix link opens the template in edit mode with the tags field focused.
- **AC-3:** A template whose content contains `$$$$` or `$$Name$$` + `$$name$$` fails `variable_quality` with the specific explanation; fixing the content and saving a new version turns the check green on recompute.
- **AC-4:** A fully-specified template (description ≥ 20 chars, 3 tags, category, valid variables, content in band, change note, updated recently, language set) scores 100%.
- **AC-5:** A non-owner (logged-in or anonymous) fetching `GET /api/v1/templates/{id}/health` receives 403; the detail page shows no health panel.
- **AC-6:** The post-save toast appears after creating a template with failed checks and links to the checklist; it never blocks or requires dismissal to continue.
- **AC-7:** "Needs attention" filter shows only the owner's templates scoring < 80%.
- **AC-8:** Health computation does not change search ranking or visibility (FR-4): a red-badge public template remains listed exactly as before.
- **AC-9:** Server-computed and client-computed checklists for the same template are identical (spot-check in tests).
- **AC-10:** All new UI strings exist in both `en` and `zh` translation files with identical key sets.

## 8. Out of Scope (v1)

- Public health badges or quality scores on template cards/pages (permanently private, FR-9).
- Health as a search-ranking or trending signal (FR-4).
- Automated fixes ("Fix all" AI rewriting) — links only.
- Health history/trending over time.
- Admin moderation integration (health is not a moderation input).
- Email digests about low health.
- Monetization of any kind — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1:** All checked fields exist today: `description`, `tags`, `category`, `language`, content (latest version), `updated_at`; change notes arrive with template-versioning (its `change_note` field) — if absent, `has_change_note` always reports info-missing and its weight is excluded from the denominator (graceful degradation).
- **D-2:** The variable validation rules are the shared utility specified by advanced-editor (its NFR-6) — this feature consumes the same rules server-side; if the utility is client-only today, the architect extracts/ports it (OQ-3).
- **D-3:** The user dashboard template list (user-dashboard feature) exists to host the badge and filter; if it has not shipped, the badge lands on the detail-page panel first and the dashboard integration follows.
- **D-4:** The edit-mode deep links (`fix_action` targets) correspond to existing edit surfaces (details-page edit mode; creation modal fields).
- **D-5:** The dashboard list endpoint is the owner's template list (existing `ListTemplates` with owner scope) — embedding `health_score` is an additive response field.

## 10. Open Questions for Architect

1. **Compute-on-read vs. stored score:** §5.1 checks are cheap, but the dashboard embedding (FR-15) computes per row. Confirm compute-on-read; if list latency regresses, a `health_score` column updated on template write is the fallback.
2. **Embed vs. detail calls:** FR-15 embeds `health_score` in the owner list response; the full checklist comes from FR-14 only when the panel opens. Confirm this split.
3. **Shared validation utility:** advanced-editor's validation is specified client-side (its FR-4/NFR-6). Where does the canonical implementation live so both the editor and this feature's server-side check use it (Go port vs. JS shared via a spec test)?
4. **Weight tuning:** the §5.1 weights are a product proposal; confirm or adjust before implementation (the spec table is data-driven, so tuning is a one-line change).
