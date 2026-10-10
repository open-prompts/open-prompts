# Template Search & Filtering — Requirements Analysis

- **Feature ID:** template-search-filtering
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2025-06-25

## 1. Competitor Research Summary

Research covered the leading prompt-marketplace and prompt-library products, plus general content-platform search patterns that users already expect.

| Platform | Search & filtering approach | Key takeaways |
|---|---|---|
| **PromptBase** | Prominent keyword search bar on marketplace home; category chips; sort dropdown (Top, New, Price); search results page with result counts | Search is the primary discovery entry; results page shows counts and supports sorting; category browsing is secondary |
| **FlowGPT** | Large hero search box ("Search prompts…"); tag chips; category sidebar; sort options (Top rated, Most popular, Newest); infinite scroll | Hero search + tag chips is the dominant pattern; sorting is a compact dropdown, not a page |
| **Snack Prompt** | Search bar in header; filter panel (category, tags, language); sort (Trending, Newest, Most liked); community-voted ranking | Combines keyword search with structured filters; "Trending" as a default sort drives engagement |
| **AIPRM** | Search within extension library; multi-select tag filters with counts; sort (Views, Votes, Use count, Newest); saved searches | Multi-select tags with counts beat single-select; power users value saved/reusable filter sets |
| **PromptHero** | Search-first landing; model filter; sort (New, Top, Hot); keyword suggestions | "Hot/Trending" time-windowed sorts create a sense of liveliness; suggestions reduce empty results |
| **Generic SaaS (Stripe/Vercel/Supabase dashboards)** | URL-synced filters, debounced search, filter chips summarizing active filters, empty states with clear next actions | URL state makes filters shareable/bookmarkable; chips make complex filter state legible; debounce avoids request storms |

**Synthesis:** Every competitor treats keyword search as the primary discovery mechanism, layered on top of the existing category/tag browsing. Sorting is always a compact dropdown with 3–5 named options. Multi-select tags, filter chips, and URL-synced state are the patterns that separate polished products from basic ones. None of the competitors require a separate search results page — search lives on the main listing page.

## 2. Problem Statement

Open Prompts currently supports browsing templates by visibility, category, and a single tag, ordered strictly by creation time. As the template library grows:

1. **No keyword search.** Users cannot find a template by title, description, or content keywords. The only discovery paths are category browsing and tag clicks. This is the single biggest usability gap versus every competitor.
2. **No sorting.** `ORDER BY t.created_at DESC` is hardcoded. Popular, highly-liked, or recently-updated templates are buried under newer ones. Users cannot find "most liked" or "trending" content.
3. **Single-tag filtering.** The API accepts a tag array but the UI only ever sends one tag (clicking a tag replaces the selection). Users cannot combine tags.
4. **No visible filter state.** Active filters are not summarized anywhere; users must remember what they clicked in the sidebar. Filters are not reflected in the URL, so results cannot be shared or bookmarked, and browser back/forward does not restore filter state.
5. **No result counts or feedback.** The UI never tells users how many templates match, so an empty result is indistinguishable from a broken page until they read the empty state.
6. **Search backend gap.** The repository layer has no search condition support; the proto `ListTemplatesRequest` has no keyword or sort fields. Both must be extended.

## 3. Target Users

- **Regular users (browsers):** visitors and logged-in users who want to find a useful prompt quickly by describing what they need ("write email", "code review", "logo design").
- **Power users (creators):** users who maintain many templates and want to locate their own or others' work by keyword, tag combinations, and sort orders.
- **Admins:** (indirect) benefit from the same search/filter infrastructure when the admin template moderation list (admin-user-management feature) reuses these capabilities.

## 4. User Stories

### 4.1 Search & discovery

- **US-S1:** As a visitor, I want to type keywords into a search box on the home page so that I can find templates matching my need without browsing categories.
- **US-S2:** As a visitor, I want search to match title, description, and prompt content so that a template whose title doesn't contain my keyword but whose content does is still findable.
- **US-S3:** As a visitor, I want to see the number of matching results so that I know whether my filters are too narrow.
- **US-S4:** As a user, I want to combine keyword search with category and tag filters so that I can narrow within a domain (e.g., "email" keyword + "marketing" category).
- **US-S5:** As a user, I want to select multiple tags so that I can find templates at an intersection of topics.
- **US-S6:** As a user, I want to clear all active filters with one action so that I can start a fresh search quickly.
- **US-S7:** As a user, I want the current search/filters reflected in the URL so that I can bookmark, share, and restore results with browser back/forward.
- **US-S8:** As a user, I want search suggestions (popular tags/categories matching my input) so that I get guidance before submitting a full search.

### 4.2 Sorting

- **US-S9:** As a user, I want to sort results by Newest, Most Liked, Most Favorited, or Trending so that I can find quality or fresh content.
- **US-S10:** As a user, I want the sort order to persist in the URL alongside filters so that a shared link reproduces the exact view.

### 4.3 System stories

- **US-S11:** As the system, I debounce keyword input (300ms) so that typing does not trigger a request per keystroke.
- **US-S12:** As the system, I keep search results consistent with visibility rules so that private templates never leak into public search results.
- **US-S12a:** As the system, I keep search results consistent with admin moderation state so that banned users' public templates are excluded once the admin feature ships (dependency noted in §9).

## 5. Functional Requirements

### 5.1 Keyword search

- **FR-1:** The system shall provide a search input on the home page (Prompt Square) that accepts free-text keywords.
- **FR-2:** The system shall match keywords against template `title`, `description`, and latest-version prompt `content` fields (case-insensitive).
- **FR-2a:** The system shall match keywords against template `tags` and `category` values (case-insensitive) so that searching a tag name finds tagged templates.
- **FR-3:** The system shall support multi-word keywords treated as a single phrase (AND semantics across fields, phrase match within each field).
- **FR-4:** The system shall return only templates visible to the requester: public templates for anonymous users; public + own private templates for logged-in users (existing visibility rules unchanged).
- **FR-5:** The system shall display the total number of matching templates for the current query/filters (result count), updated on every query change.
- **FR-6:** The system shall highlight nothing in v1 (no snippet/highlighting) — plain result cards, unchanged from today.
- **FR-7:** The system shall trim and collapse whitespace in keywords; empty/whitespace-only keyword equals no keyword filter.
- **FR-8:** The system shall cap keyword length at 100 characters; longer input is truncated for matching purposes (input field allows typing but only first 100 chars are used).

### 5.2 Sorting

- **FR-9:** The system shall support the following sort options on the template listing: `newest` (default, `created_at DESC`), `most_liked` (`like_count DESC, created_at DESC`), `most_favorited` (`favorite_count DESC, created_at DESC`), `trending` (see FR-10).
- **FR-10:** `trending` shall rank by engagement in a trailing time window (v1: like_count + favorite_count over the last 7 days, tie-break `created_at DESC`). The window is a server-side constant, not user-configurable.
- **FR-11:** The system shall apply the selected sort to all listing contexts (public, private, mixed, my-likes, my-favorites) consistently.
- **FR-12:** The system shall ignore invalid sort values, falling back to `newest` (server-side validation, client sends only valid values).

### 5.3 Tag & category filtering

- **FR-13:** The system shall support selecting multiple tags (AND semantics: template must have all selected tags).
- **FR-14:** The system shall display tag selection counts in the sidebar tag cloud (existing behavior) and allow multi-select via checkbox-style interaction.
- **FR-15:** The system shall support combining keyword + category + tags + visibility + sort in a single request.
- **FR-15a:** The system shall support clearing the category filter independently of tags and keyword.
- **FR-16:** The system shall provide a "Clear all" action that resets keyword, category, tags, and sort (visibility returns to the user's persisted default) and returns the user to the unfiltered listing.

### 5.4 URL state & shareability

- **FR-17:** The system shall sync keyword, category, tags, sort, and visibility to URL query parameters (`?q=`, `?category=`, `?tags=a,b`, `?sort=`, `?visibility=`) on the home route.
- **FR-18:** The system shall restore full filter state from URL parameters on page load (deep-link restore), including after language switch or refresh.
- **FR-18a:** The system shall update the URL when filters change without adding a history entry per keystroke (replace, not push, for keyword typing; push for discrete filter changes).
- **FR-19:** The system shall support browser back/forward navigation restoring previous filter states.

### 5.5 Search suggestions (v1 scope: lightweight)

- **FR-20:** While the user types (≥2 chars), the system shall show up to 5 suggestions: matching categories first, then matching tags, drawn from existing `/api/v1/categories` and `/api/v1/tags` endpoints (client-side match, no new endpoint).
- **FR-21:** Selecting a category suggestion applies the category filter; selecting a tag suggestion applies that tag filter (single-tag application from suggestion, consistent with FR-13 multi-select available in sidebar).
- **FR-22:** The system shall not block typing or search submission while suggestions load (suggestions are purely additive UI).

### 5.6 API & backend

- **FR-23:** `ListTemplatesRequest` proto shall gain `string keyword = 10;` and `string sort = 11;` fields.
- **FR-24:** The REST gateway shall accept `keyword` and `sort` query parameters on `GET /api/v1/templates` and map them to the proto fields.
- **FR-25:** The repository `List` shall implement keyword matching via SQL `ILIKE` on `title`/`description`/`tags`/`category` and latest-version content (see FR-2/FR-2a; content match via join to latest template version).
- **FR-26:** The repository `List` shall implement the four sort orders as parameterized `ORDER BY` clauses (whitelisted server-side; never interpolated from user input).
- **FR-27:** The system shall maintain existing pagination semantics (page_token as offset, next_page_token) for all new filters/sorts.
- **FR-28:** The system shall return the total match count alongside results (`total_count` in `ListTemplatesResponse`) to power FR-5. *(If total count proves expensive, v1 may return `total_count` only on the first page of each query — see NFR-4.)*
- **FR-28a:** The system shall keep the existing mixed public/private pagination token format (`public:private`) working unchanged for logged-in mixed view.

### 5.7 Frontend UI

- **FR-29:** The home page shall render a search bar above the template grid (desktop: inline with page header area; mobile: sticky below header).
- **FR-30:** The-synced filters shall be summarized as removable filter chips between the search bar and the grid: one chip per active keyword, category, tag, and sort (sort chip removable → default).
- **FR-30a:** The system shall show a loading state (skeleton cards or existing spinner) while a query is in flight, and an error state with retry when a query fails.
- **FR-31:** The sort control shall be a compact dropdown (custom, Aurora-styled) with the four options, positioned at the right of the search bar row.

### 5.8 Regular-user vs admin split

- **FR-32:** All requirements in this feature apply to the regular-user surface (home page / Prompt Square). No admin surface changes are in scope.
- **FR-33:** The admin template moderation list (admin-user-management feature) may reuse the keyword/sort repository capabilities introduced here; no admin-specific search UI is added in this feature.

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Keyword search on ≤100k templates with ILIKE + btree indexes shall return p95 < 500ms. Full-text search (tsvector) is out of scope for v1 (see §8).
- **NFR-2 (Performance):** Suggestions are computed client-side from already-fetched categories/tags; zero additional requests.
- **NFR-3 (Compatibility):** Existing clients that do not send `keyword`/`sort` shall receive identical behavior to today (default sort, no keyword filter). No breaking API change.
- **NFR-4 (Performance):** `total_count` shall be computed with the same query filters via `COUNT(*)` OVER() window function or a parallel count query; if p95 exceeds budget, degrade to first-page-only counts.
- **NFR-5 (Accessibility):** Search input, sort dropdown, filter chips, and suggestions must be keyboard-operable and screen-reader announced (see UI/UX doc §11).
- **NFR-6 (i18n):** All new UI strings in EN and ZH, kept in sync; keyword matching itself is language-agnostic (ILIKE on stored content).
- **NFR-7 (Security):** Keyword and sort values are parameterized; no SQL injection surface. Sort values whitelisted server-side.
- **NFR-8 (Data):** No schema changes required for v1 (ILIKE over existing columns; trending uses existing like/favorite counts and timestamps). *(If trending needs a rolling window, a materialized daily engagement rollup may be added later — out of scope v1.)*
- **NFR-9 (Observability):** The service shall log keyword length (not content) and sort value on ListTemplates calls for observability without PII concerns.

## 7. Acceptance Criteria

- **AC-1:** Typing "email" in the search bar and pressing Enter (or waiting for debounce) returns only templates whose title/description/content/tags/category match "email" (case-insensitive), and the result count is displayed.
- **AC-2:** Selecting tags `["marketing", "seo"]` returns only templates having both tags.
- **AC-3:** Choosing "Most Liked" re-orders results by like_count DESC; choosing "Trending" re-orders by 7-day engagement; the URL contains `?sort=most_liked` / `?sort=trending` respectively.
- **AC-4:** With filters active, the URL reflects keyword, category, tags, and sort; copying the URL into a new tab restores the identical view.
- **AC-5:** Browser back button after applying filters returns to the previous filter state.
- **AC-6:** A query with no matches shows the empty state with the active keyword echoed and a "Clear all" action.
- **AC-7:** An anonymous user searching never sees private templates in results.
- **AC-8:** A logged-in user in mixed view searching sees own private templates matching the keyword alongside public matches, with the existing mixed pagination working.
- **AC-9:** Invalid `sort` value sent to the API (e.g., `sort=bogus`) falls back to `newest` without error.
- **AC-10:** Existing tests and clients that omit `keyword`/`sort` behave exactly as before (no regression).
- **AC-11:** Typing in the search bar does not fire more than one request per 300ms window (debounce verified via network panel).
- **AC-12:** Suggestions appear after ≥2 characters, show categories then tags, and selecting one applies the corresponding filter.
- **AC-13:** All new UI strings exist in both `en` and `zh` translation files with identical key sets.

## 8. Out of Scope (v1)

- Full-text search engine (PostgreSQL tsvector/GIN or external engine like Meilisearch/Elastic) — v1 uses ILIKE; migration path documented for architect.
- Search result highlighting/snippets.
- Relevance ranking (BM25-style scoring); v1 sorts are the four explicit options only.
- Saved searches / search history per user.
- Search across users or other entities (templates only).
- Author/owner search (search by creator name).
- Date-range filters.
- Admin-specific search UI (admin list reuses backend capability only).
- Server-side search analytics (beyond NFR-9 logging).
- Phonetic/fuzzy matching and typo tolerance.
- CJK word segmentation for matching (ILIKE substring match works for CJK without segmentation).
- Materialized trending rollup table (v1 computes trending from raw counts; rollup is a future optimization).

## 9. Dependencies & Assumptions

- **D-1:** The existing `templates` table columns (`title`, `description`, `tags`, `category`, `language`, `like_count`, `favourite_count`/`favorite_count`, `created_at`) are available for ILIKE matching and sorting. *(Column name `favorite_count` per current schema.)*
- **D-2:** Latest-version content matching (FR-2) requires joining `template_versions` for the latest version per template; the architect may simplify v1 to title/description/tags/category only if the join proves expensive — the requirements treat content match as a should-have, not a blocker.
- **D-3:** Trending (FR-10) in v1 uses existing aggregate counts within a trailing window computed from `template_likes`/`template_favorites` created_at timestamps; if those tables lack timestamps, trending degrades to `like_count + favorite_count` all-time with `created_at DESC` tie-break. Architect to confirm.
- **D-4:** URL state (FR-17/18) assumes React Router v6 `useSearchParams` on the home route; no router upgrade needed.
- **D-5:** No dependency on the admin-user-management feature; the two features are independent. If both ship, admin moderation lists may reuse the search/sort repository capabilities (FR-33).
- **D-6:** The existing mixed-view pagination token format (`public:private`) is preserved (FR-28a); new filters apply to both streams.
- **D-7:** Suggestions (FR-20) rely on the existing `/api/v1/categories` and `/api/v1/tags` endpoints already fetched by the Sidebar; no new endpoints.
- **D-8:** The `total_count` response field (FR-28) is additive to `ListTemplatesResponse`; existing clients ignore unknown fields.
- **D-9:** ILIKE performance on CJK content: substring matching works, but without indexes ILIKE '%kw%' cannot use btree; pg_trgm GIN indexes are the documented upgrade path (out of scope v1, noted for architect).
- **D-10:** The home page currently persists visibility choice in localStorage; this feature preserves that behavior and layers URL state on top (URL wins on load if present, else localStorage default, else auth-state default).

## 10. Open Questions for Architect

*(None blocking — all design decisions made autonomously; D-2/D-3/D-9 flag the trade-offs the architect should confirm.)*
