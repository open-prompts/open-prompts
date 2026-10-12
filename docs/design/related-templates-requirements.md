# Related Templates & Discovery — Requirements Analysis

- **Feature ID:** related-templates
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. Competitor Research Summary

| Platform | Related-content approach | Key takeaways |
|---|---|---|
| **YouTube** | "Up next" recommendations driven by co-watch signals; strong personalization | Related content is the #1 session-length driver; even simple category/creator fallbacks work |
| **PromptHero** | Related prompts by tag/model on detail pages; category browsing rails | Tag-based relatedness is the standard for prompt platforms — cheap and explainable |
| **FlowGPT** | "Similar prompts" section on prompt pages; category rails on home | Same-tag + same-category rails; no personalization needed for v1 |
| **Amazon** | "More like this" (content similarity); "Customers also viewed" (co-viewing) | Co-interaction signals (favorites/likes) beat metadata once volume exists |
| **Spotify** | "Because you liked X" home rails; related artists on artist pages | Explaining *why* something is related ("Same category") builds trust |
| **PromptBase** | Minimal related content; marketplace categories only | Even the market leader skips this — easy differentiator |

**Synthesis:** The template detail page is a dead end today: a user finishes reading a prompt and the only paths are back or away. The v1 pattern (PromptHero/FlowGPT): a **"Related prompts" rail on every template detail page**, computed from explainable metadata signals — same category first, then shared tags, then same author ("More from {author}") — ranked by a simple score (shared-tag count, then likes), capped at 8 items, excluding the current template and non-public content. No personalization, no ML, no tracking in v1: relatedness is a pure function of the template's own metadata, which makes it fast, cacheable, and honest ("Related · Customer Support"). A second, smaller surface: **"More from this author"** on the detail page (up to 4, same author, most-liked first) — this also serves the follow-creators feature's goal of converting readers into followers. Home-page personalized rails are explicitly out of scope (no "for you" in v1).

## 2. Problem Statement

1. **The detail page is a dead end.** After reading a template there is no next step except back/away; session depth suffers and great templates in the same category are never seen.
2. **Discovery is search-only.** Users must already know what to look for; there is no browse-adjacent flow that surfaces "more like this."
3. **Authors get no halo.** A user who likes one of alice's templates has no path to her other work — the follow-creators feature needs this bridge.
4. **Competitors mostly skip this.** PromptBase has no related content; adding it is a cheap, visible product win.

## 3. Target Users

- **Browsing users:** finish a template and want more like it without crafting a search.
- **Authors:** gain cross-visibility for their catalog ("More from {author}").
- **Follow-creators users:** discover an author's breadth before following.
- **The platform:** longer sessions, more template views, better content circulation.

## 4. User Stories

- **US-D1:** As a browsing user, I see a "Related prompts" rail on every public template detail page, so that I can continue exploring without a new search.
- **US-D2:** As a user, related prompts share my current template's category or tags, so that the suggestions are actually relevant.
- **US-D3:** As a user, I understand why a prompt is shown ("Related · Customer Support"), so that the rail is trustworthy.
- **US-D4:** As a user, I see "More from {author}" with the author's other public templates, so that I can explore their catalog.
- **US-D5:** As a user, I never see private, unpublished, or banned-author content in related rails, so that the rails are always safe.
- **US-D6:** As an author, my other templates appear on my templates' detail pages, so that readers discover more of my work.
- **US-D7:** As a user on a template with no related matches, the section hides cleanly, so that there is no empty clutter.

## 5. Functional Requirements

### 5.1 Related prompts rail (template detail)

- **FR-1:** Every public template detail page renders a **"Related prompts"** rail below the prompt content: up to 8 template cards, horizontally scrollable on mobile, grid-wrapped on desktop.
- **FR-2:** Candidate pool: public templates (visibility public) by non-banned authors, excluding the current template. Signals, in priority order:
  1. Same category (strongest signal);
  2. Shared tags (each shared tag adds weight);
  3. Same language (tiebreaker);
  4. Same author (small weight — author's own work is acceptable but should not dominate).
- **FR-3:** Ranking score: `3 × same_category + 2 × shared_tags_count + 1 × same_language + 0.5 × same_author`, then by like count desc, then by updated_at desc as final tiebreaker. Deterministic for a given template (no randomization in v1).
- **FR-4:** The rail header explains the relation: "Related · {category}" when the top candidates share the category; "Related · {tag}" when tag-driven (first shared tag of the top item); fallback "Related prompts".
- **FR-5:** If fewer than 3 candidates survive (after exclusions), the section is hidden entirely (no 1–2 item stubs).

### 5.2 More from author

- **FR-6:** On public template detail pages where the author has other public templates, a **"More from {author}"** row renders below Related: up to 4 cards, most-liked first, excluding the current template. Hidden if the author has no other public templates.
- **FR-7:** The row header links to the author's public profile (follow-creators surface) when available; otherwise plain text.

### 5.3 API

- **FR-8:** `GET /api/v1/templates/{id}/related?limit=8` returns the related rail: `{ items: [TemplateCard…] , basis: "category" | "tag" | "generic" }`. Public only; 404 for a nonexistent/private template id (same visibility rules as the template detail endpoint).
- **FR-9:** `GET /api/v1/templates/{id}/more-from-author?limit=4` returns the author row: `{ items: [TemplateCard…] }`, or empty items if none. Same visibility rules.
- **FR-10:** Both endpoints are cacheable (short TTL, 5 minutes) and computed from existing template metadata (category, tags, language, owner, likes) — no new tables, no background jobs.

### 5.4 Placement & integration

- **FR-11:** Rails render on the template detail page below the prompt/version content and above comments; on mobile they appear after the action buttons. They do not render on private templates (owner-only view) or admin views.
- **FR-12:** Cards in the rails reuse the existing TemplateCard component (name, category, likes, owner) with identical click behavior (navigate to detail).
- **FR-13:** The rails are excluded from the SEO/SSR crawler rendering (they are interactive discovery, not core content) — crawler pages render content + CTA only (per the seo-landing design).

### 5.5 Admin surface

- **FR-14:** No admin surface. Rails automatically exclude unpublished templates and banned authors via the same filters as public listing endpoints (existing behavior).

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Related endpoint responds < 200 ms (p95) with a 5-minute cache; the rail renders without blocking the detail page (lazy-loaded after main content).
- **NFR-2 (Correctness):** Rails never include private/unpublished templates or banned authors' content; the current template never appears in its own rails.
- **NFR-3 (Determinism):** The same template id yields the same rail within a cache window (no flicker between loads).
- **NFR-4 (i18n):** Rail headers and empty states via i18next (`related.*` namespace) in EN/ZH; the basis label interpolates the category/tag name (proper nouns are not translated).
- **NFR-5 (Accessibility):** Rails are horizontal scroll regions with keyboard access (cards are focusable links per the accessibility design); section headers are `h2`; no autoplay/carousel timers.
- **NFR-6 (No tracking):** v1 relatedness uses only template metadata — no user behavior, no cookies, no profiling; nothing is personalized.

## 7. Acceptance Criteria

- **AC-1:** A public template detail page shows the Related rail with up to 8 cards sharing its category/tags; clicking a card navigates to that template.
- **AC-2:** The rail header shows the relation basis ("Related · Customer Support" or "Related · writing").
- **AC-3:** A template with fewer than 3 related candidates shows no rail.
- **AC-4:** The current template never appears in its own rails; private and unpublished templates never appear; banned authors' templates never appear.
- **AC-5:** "More from {author}" shows up to 4 of the author's other public templates, most-liked first; hidden when the author has none.
- **AC-6:** Both endpoints return 404 for private/nonexistent template ids when called unauthenticated.
- **AC-7:** The rails lazy-load after main content; the detail page's LCP is unaffected (rail fetch does not block render).
- **AC-8:** Rails are keyboard navigable (Tab through cards) and screen-reader labelled as complementary sections.
- **AC-9:** All rail copy is EN/ZH translated; category/tag names render as-is.
- **AC-10:** Repeated loads within the cache window return identical rails (deterministic).

## 8. Out of Scope

- Personalized recommendations ("for you", view-history signals, co-interaction matrices).
- Home-page recommendation rails or "because you liked" surfaces.
- ML/embedding similarity or semantic search.
- Related collections/chains/teams (added when those surfaces ship).
- Dislike/"not interested" controls on rail cards.
- Monetization of any kind (sponsored rails, promoted placements) — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1:** Templates carry category, tags, language, owner_id, likes, visibility, and updated_at — all existing columns; relatedness needs no schema change.
- **D-2:** The public template listing filters (visibility public, non-banned authors) are reusable query fragments from the existing list endpoints.
- **D-3:** The TemplateCard component and its accessible click behavior exist (accessibility design makes it keyboard operable).
- **D-4:** The template detail page layout (content → actions → comments) has room for two rails below content (per the template-versioning/comments designs' page structure).
- **D-5:** The follow-creators design's public profile route exists or will exist; "More from {author}" links to it when present.
- **D-6:** A short-TTL response cache mechanism exists (or is trivially added) for the two new endpoints.

## 10. Open Questions for Architect

1. **Query shape:** One SQL query with the scoring expression (CASE-based, ORDER BY score DESC, likes DESC, updated_at DESC, LIMIT 8) vs. application-side scoring after fetching candidates (same category + shared tags, capped pool of ~200)? Recommendation: single SQL query — the data is small and the expression is simple.
2. **Cache layer:** In-process TTL cache (5 min) vs redis (stack has redis) vs HTTP Cache-Control with a CDN? Recommendation: redis or in-process — no CDN dependency in v1.
3. **Endpoint granularity:** Two endpoints (related + more-from-author) vs one combined `?include=related,author` on the detail response? Recommendation: two endpoints — parallel lazy loads, independent caching.
4. **Same-author weight:** FR-3 gives same-author a small weight (0.5) so an author's own work can appear but not flood the rail — confirm, or exclude own work entirely from Related (it already has its own row)?
5. **Basis label:** FR-4 derives the label from the top item's dominant signal — confirm this simple rule vs. returning per-card relation reasons (heavier payload, richer UI).
6. **Limit defaults:** 8 related / 4 author — confirm or adjust (e.g., 6/3 on mobile via responsive CSS, same payload).
