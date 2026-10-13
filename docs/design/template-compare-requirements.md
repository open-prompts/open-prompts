# Template Compare View — Requirements Analysis

- **Feature ID:** template-compare
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-13

## 1. Competitor Research Summary

| Platform | Comparison approach | Key takeaways |
|---|---|---|
| **E-commerce (Amazon, Best Buy)** | "Add to compare" checkbox on cards; floating compare tray; dedicated compare page with attribute rows; 2–4 item cap | The card-checkbox + floating tray is the dominant entry pattern; a hard cap keeps the table readable |
| **GitHub** | Compare view across refs; unified/side-by-side diff with word-level highlighting | Word-level (not line-level) highlighting matters for prose, which prompts are |
| **IDE diff tools (VS Code)** | Side-by-side panes, inline word highlights, synchronized scrolling | Synchronized scrolling is table stakes for long content |
| **PromptBase** | No comparison; users open multiple tabs | Even the market leader lacks this — cheap, visible product win |
| **FlowGPT / Snack Prompt** | No comparison; similar prompts surfaced but not diffable | Same gap across all prompt platforms |
| **SaaS comparison sites (G2, vs-chart generators)** | Structured attribute tables with check/cross marks; "winner" badges | Attribute tables work when items share a schema — templates do (category, tags, variables, stats) |

**Synthesis:** No prompt platform offers comparison, while every adjacent domain (e-commerce, dev tools) has converged on the same shape: **select from listing cards → floating tray → dedicated compare page** with (1) a structured attribute table for metadata, (2) side-by-side content panes with word-level diff highlighting and synchronized scrolling, and (3) a 2–3 item cap. Templates share a schema (category, tags, language, variables, stats), making the attribute-table pattern directly applicable. The diff engine already exists in spirit: template-versioning ships a DiffModal for two versions of one template — this feature generalizes the surface to N distinct templates.

## 2. Problem Statement

Users evaluating similar templates (three "code review" prompts, two "cold email" variants) must today open multiple browser tabs and mentally juggle titles, tags, variables, and content:

1. **No side-by-side surface.** The only diff tool (template-versioning's DiffModal) compares two versions of the *same* template. Comparing two *different* templates is impossible in-product.
2. **Variables are the deciding factor but invisible until use.** Which template needs fewer inputs, which requires a `tone` variable — discoverable today only by reading full content or opening the generator.
3. **Metadata scatter.** Category, tags, language, version count, update recency, and engagement stats live on separate detail pages; there is no aligned view.
4. **Fork evaluation is blind.** template-remix-forks-tree shows the tree, but choosing between a fork and its source (or between sibling forks) means tab-switching.

## 3. Target Users

- **Browsers/evaluators (primary):** users choosing among similar public templates before favoriting or using one.
- **Forkers:** users deciding between a source template and its forks (template-remix-forks-tree surfaces the entry point).
- **Creators:** users comparing their template against a popular similar one to position their work.
- **Admins:** no admin surface in v1 (§8); the compare page is a regular-user surface.

## 4. User Stories

### 4.1 Selecting templates to compare

- **US-C1:** As a user, I can add a template to a compare selection from its card in any listing (grid, search results, rails), so that I can collect candidates while browsing.
- **US-C2:** As a user, I see a floating compare tray showing my selected templates with a "Compare" button, so that the selection is always visible and reachable.
- **US-C3:** As a user, I am capped at 3 templates; adding a 4th prompts me to remove one, so the comparison stays readable.
- **US-C4:** As a user, I can remove a template from the tray or clear the whole selection, so that I control the comparison set.
- **US-C5:** As a user, my compare selection persists in my browser session (localStorage), so that navigation between pages does not lose it.
- **US-C6:** As a user, I can open the compare page directly via URL (`/compare?ids=1,2,3`), so that comparisons are shareable and bookmarkable.

### 4.2 Reading the comparison

- **US-C7:** As a user, I see an aligned attribute table (title, author, category, tags, language, visibility, version count, updated date, likes, favorites, views, uses), so that metadata differences are obvious at a glance.
- **US-C8:** As a user, I see each template's variable list (names, count) extracted from `$$var$$` placeholders, so that I can judge input effort without reading full content.
- **US-C9:** As a user, I see content in side-by-side panes with word-level differences highlighted against a chosen base, so that textual divergence is immediately visible.
- **US-C10:** As a user, I can switch the diff base (any of the compared templates or "no highlighting"), so that I control the reference point.
- **US-C11:** As a user, the content panes scroll in sync, so that long templates stay aligned.
- **US-C12:** As a user, I can jump from any column to that template's detail page, so that deeper evaluation is one click away.

### 4.3 System stories

- **US-C13:** As the system, I only allow comparing templates visible to the requester, so that private templates never leak into shared compare URLs.
- **US-C14:** As the system, I compute word-level diffs client-side, so that no diff payload crosses the network.

## 5. Functional Requirements

### 5.1 Selection & tray

- **FR-1:** Every template card in listings (home grid, search results, related rails, collection views) shall offer an "Add to compare" toggle action (icon button, hover/focus visible, always in tab order).
- **FR-2:** A floating compare tray shall render at the bottom of the viewport when ≥1 template is selected, showing thumbnail/name chips, per-chip remove ✕, a "Clear" action, and a "Compare" button (enabled at ≥2).
- **FR-3:** Selection shall be capped at 3 templates. Adding a 4th shows an inline message in the tray ("Compare up to 3 — remove one first"); the add action is a no-op.
- **FR-4:** Selection shall persist in localStorage keyed per browser (not per user account); it survives navigation and reload, and is not synced server-side.
- **FR-5:** The tray shall be dismissible (collapse to a corner pill with count); it re-expands on click and never covers the last row of content (page bottom padding adjusts).

### 5.2 Compare page

- **FR-6:** A compare page shall exist at `/compare?ids=<id1>,<id2>[,<id3>]` (requires ≥2 ids; 1 or 0 → empty state with guidance).
- **FR-7:** The page shall render one column per template: header (title, author link, "Open" link), attribute table, variable list, and content pane — all vertically aligned per row.
- **FR-8:** The attribute table shall show: title, author, category, tags, language, visibility, version count, last updated, likes, favorites, views, uses. Views/uses render only when available (prompt-analytics dependency, D-4); otherwise the row is omitted.
- **FR-9:** The variable list shall be extracted client-side from the latest version's content using the existing `$$var$$` regex (advanced-editor D-1 contract), deduplicated, sorted alphabetically, with a count badge.
- **FR-10:** Content panes shall show the latest version's content with word-level diff highlighting against the selected base (default: leftmost column), computed client-side.
- **FR-11:** A base selector shall allow choosing any compared template as base or "None" (plain content, no highlights).
- **FR-12:** Content panes shall scroll in synchrony (vertical; horizontal independent), with a sync toggle.
- **FR-13:** Invalid or inaccessible ids in the URL (nonexistent, private, banned author) shall be dropped with a notice ("1 template was removed: not visible to you"); the page renders the remainder if ≥2 survive, else the empty state.
- **FR-14:** Duplicate ids in the URL shall be deduplicated.

### 5.3 Entry points beyond cards

- **FR-15:** The template detail page shall offer "Add to compare" in its action row, so users can seed a comparison from a template they already like.
- **FR-16:** The forks tree (template-remix-forks-tree) shall offer "Compare with source" on fork nodes when that feature's UI ships; until then, card selection covers the flow.

### 5.4 Regular-user vs admin split

- **FR-17:** All surfaces are regular-user surfaces (`/compare`, card actions, detail action). No admin console pages; no `/api/v1/admin` endpoints.

### 5.5 API

- **FR-18:** No new endpoints are required in v1: the page fetches each template via the existing `getTemplate` (which returns metadata + latest content) in parallel. If the architect prefers a batch endpoint (`GET /api/v1/templates/batch?ids=1,2,3`), it is an optimization, not a requirement (OQ-1).
- **FR-19:** Visibility enforcement relies on the existing `getTemplate` authorization (private templates 404/403 for non-owners); the client treats those as "dropped" per FR-13.

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Compare page interactive < 1.5s with 3 templates (3 parallel `getTemplate` calls); client-side diff of ≤64KB contents completes < 200ms.
- **NFR-2 (Compatibility):** No existing endpoint or page changes; card actions are additive. Listings without the action (e.g., third-party embeds) are unaffected.
- **NFR-3 (Accessibility):** Tray, card toggle, attribute table, and diff panes are fully keyboard operable; diff highlights have text alternatives (UI/UX doc §9).
- **NFR-4 (i18n):** All new UI strings in EN and ZH with identical key sets; diff highlight legend included.
- **NFR-5 (Privacy):** Compare URLs contain only template ids; selection stays client-side (localStorage); no server-side selection storage or history.
- **NFR-6 (Determinism):** Same URL always renders the same comparison (latest versions at fetch time); no personalization.
- **NFR-7 (Content limits):** Diff operates on the existing content length limit; templates exceeding the client diff budget (64KB) render plain content with a notice instead of highlights.

## 7. Acceptance Criteria

- **AC-1:** Adding two templates from the home grid shows the tray; clicking Compare opens `/compare?ids=…` with two aligned columns.
- **AC-2:** Adding a 4th template shows the cap message and does not change the selection.
- **AC-3:** The attribute table aligns all rows across columns; differing values are visually emphasized (e.g., tags, category).
- **AC-4:** Variable lists show deduplicated, alphabetized names with counts; a template with `$$tone$$` and `$$language$$` lists exactly those.
- **AC-5:** With base = leftmost, words present only in the right template are highlighted as additions and vice versa; switching base re-highlights; "None" removes highlights.
- **AC-6:** Scrolling one content pane scrolls the others while sync is on; toggling sync off stops it.
- **AC-7:** A compare URL containing a private template id of another user drops that column with the notice and renders the rest.
- **AC-8:** Reloading the page restores the same comparison from the URL; the tray selection survives navigation (localStorage).
- **AC-9:** All card toggle buttons are reachable by Tab and announced via screen reader ("Add to compare, template X").
- **AC-10:** All new UI strings exist in both `en` and `zh` translation files with identical key sets.

## 8. Out of Scope (v1)

- Comparing specific versions (v1 compares latest versions only; version-pair diff stays in template-versioning).
- Comparing more than 3 templates.
- Server-side diff computation or diff persistence (no saved comparisons).
- Semantic similarity scoring ("87% similar").
- Comparing non-template entities (collections, chains, users).
- Admin compare surfaces.
- Exporting/printing a comparison report.
- Monetization of any kind — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1:** `getTemplate` returns full metadata and latest-version content for any template visible to the requester (verified in `services/api.js`, same assumption as import-export D-1).
- **D-2:** The `$$variable$$` extraction regex is shared with advanced-editor (its D-1) — one client utility, two consumers.
- **D-3:** A word-level diff utility is needed client-side; template-versioning's DiffModal already implements line/word diff for two versions — this feature reuses or extracts that engine (OQ-2).
- **D-4:** Views/uses in the attribute table depend on prompt-analytics counters (`view_count`/`use_count` on templates); if not shipped, the rows are omitted (FR-8).
- **D-5:** TemplateCard supports an action slot or wrapper for the compare toggle without breaking existing click behavior (accessibility design made cards keyboard operable).
- **D-6:** React Router v6 supports the `/compare` route addition; query-param parsing via `useSearchParams` (same as template-search-filtering D-4).
- **D-7:** Likes/favorites/version counts/updated_at are existing template fields returned by `getTemplate`.

## 10. Open Questions for Architect

1. **Batch endpoint:** is 3 parallel `getTemplate` calls acceptable for v1, or should a `GET /api/v1/templates/batch?ids=` endpoint ship now (fewer round trips, single visibility check)? Design assumes parallel calls; batch is an optimization.
2. **Diff engine reuse:** extract template-versioning's DiffModal diff logic into a shared `diff.js` utility (word-level, base-parameterized) — confirm the current implementation is word-level or needs a small upgrade.
3. **localStorage key sharing:** should the compare selection be shared across tabs of the same browser (storage event sync) or per-tab? Design assumes shared (simple localStorage semantics).
