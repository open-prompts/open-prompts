# Template Ratings — Requirements Analysis

- **Feature ID:** template-ratings
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. Competitor Research Summary

| Platform | Ratings approach | Key takeaways |
|---|---|---|
| **PromptBase** | Star ratings on prompts; aggregate score + count displayed on cards and detail pages | Aggregate score + count is the standard presentation; a score without a count is meaningless |
| **FlowGPT** | Thumbs up/down votes on prompts; score = upvotes; sort by "Top rated" | A single up/down vote is the lowest-friction signal; but it collapses "works perfectly" and "meh" into one bucket |
| **Snack Prompt** | Like-based ranking; community votes drive "Trending" | Community voting drives discovery ordering; ratings and likes coexist as separate signals (quality vs appreciation) |
| **AIPRM** | Thumbs up/down + view/use counts; sort by votes | Use counts contextualize ratings ("works for 40k uses"); we already have like/favorite counts as adjacent signals |
| **PromptHero** | No ratings; hearts only | Pure appreciation signals leave a quality gap — users cannot express "this did not work for me" |
| **Amazon / e-commerce** | 1–5 stars, one rating per user per product, editable, aggregate + distribution histogram | One-rating-per-user with edit is the canonical model; the distribution histogram (5★ 70% / 1★ 5%) is the trust signal power users look for |
| **Google Chrome Web Store / VS Code Marketplace** | 1–5 stars, one per user, aggregate on cards, distribution on detail | Marketplace pattern: rating on the detail page, aggregate on cards, sort by rating in lists |

**Synthesis:** The winning v1 pattern is the marketplace model: 1–5 star ratings, one per user per template (upsert — re-rating replaces the old value), aggregate average + count on cards and detail pages, distribution histogram on the detail page, and the template owner cannot rate their own template. Ratings are a *quality* signal, deliberately separate from likes (appreciation) and favorites (bookmarking) — all three coexist. The search-filtering feature's sort options gain `top_rated`; the comments feature's detail-page section gains the rating widget as a sibling. Open Prompts currently has no quality signal at all: a template with 500 likes and a broken prompt looks identical to a working one.

## 2. Problem Statement

1. **No quality signal.** Likes and favorites measure appreciation and bookmarking, not "does this template actually produce good output". A user cannot express "this did not work for me", and a prospective user cannot see quality before investing time.
2. **Discovery cannot rank by quality.** The search-filtering feature sorts by newest/likes/favorites/trending — all popularity proxies. "Top rated" is the most requested sort on every marketplace and is impossible without ratings.
3. **No feedback gradient.** The only signals are binary (like/favorite). A 5-point scale captures "good but not great" — the most common true opinion.
4. **Creators get no quality feedback.** A creator sees like counts but cannot distinguish "works great" from "looks nice but fails in practice".

## 3. Target Users

- **Prospective users (primary consumers):** users evaluating templates who need a quality signal before using/forking; they read the average, the count, and the distribution.
- **Raters:** authenticated users who used a template and want to rate its real-world performance; banned users are excluded.
- **Creators:** receive quality feedback (average + distribution) on their templates; cannot rate their own.
- **Admins:** no admin console surface in v1 — ratings are numeric data, not content to moderate. (Admin template detail may show the aggregate as read-only context; see FR-13.)

## 4. User Stories

### 4.1 Rating

- **US-R1:** As a user who tried a template, I want to rate it 1–5 stars, so that I can share how well it actually worked.
- **US-R2:** As a user, I want to change my rating later, so that my rating reflects my current experience (e.g., after a version update).
- **US-R3:** As a user, I want to remove my rating, so that I can retract it entirely.
- **US-R4:** As a user, I want to see my current rating pre-selected when I return, so that the state is clear.
- **US-R5:** As a creator, I cannot rate my own template, so that aggregates stay honest.
- **US-R6:** As an anonymous visitor, I am prompted to log in when trying to rate, so that the flow is understandable.
- **US-R7:** As a banned user, I cannot rate, consistent with my restricted account state.

### 4.2 Consuming ratings

- **US-R8:** As a prospective user, I want to see the average rating and count on template cards, so that I can scan quality while browsing.
- **US-R9:** As a prospective user, I want the average, count, and star distribution on the detail page, so that I can judge quality in depth.
- **US-R10:** As a user, I want to sort template lists by "Top rated", so that I can find the best templates, not just the popular ones.
- **US-R11:** As a user, I want ratings to reflect the template as a whole (not per version), so that the signal stays simple and well-populated.

### 4.3 System

- **US-R12:** As the system, I keep exactly one rating per (user, template), so that aggregates are accurate.
- **US-R13:** As the system, I recompute aggregates transactionally on every rating change, so that counts and averages never drift.
- **US-R14:** As the system, I never let a rating action expose who rated what (beyond the rater's own view of their rating), so that rating privacy is preserved.

## 5. Functional Requirements

### 5.1 Data model

- **FR-1:** A `template_ratings` table shall store: `user_id` (FK → users, `ON DELETE CASCADE`), `template_id` (FK → templates, `ON DELETE CASCADE`), `rating` (SMALLINT, `CHECK (rating BETWEEN 1 AND 5)`), `created_at`, `updated_at` (TIMESTAMPTZ), `PRIMARY KEY (user_id, template_id)`. Index on `template_id` for aggregate recompute.
- **FR-2:** `templates.rating_sum` (INT/BIGINT, default 0) and `templates.rating_count` (INT, default 0) shall be maintained transactionally on create/update/delete of ratings (same denormalized-counter pattern as `like_count`); average is computed as `rating_sum / rating_count` (never stored, to avoid drift).
- **FR-3:** The template list and detail responses shall include `rating_avg` (float, 2 decimals, null when `rating_count = 0`), `rating_count`, and — detail only — the distribution `{1: n, 2: n, 3: n, 4: n, 5: n}`.

### 5.2 Rating actions (regular-user surface)

- **FR-4:** `POST /api/v1/templates/{id}/rating` (auth required) with body `{rating: 1..5}` shall upsert the caller's rating: insert if none exists, replace if one exists (single row per user per template). Returns `{rating_avg, rating_count, my_rating}`. Self-rating (caller = owner) returns 403 with a clear message. Banned users return 403. Private templates: only the owner can rate (and self-rating is blocked, so effectively no one — see FR-6 note).
- **FR-5:** `DELETE /api/v1/templates/{id}/rating` (auth required) shall remove the caller's rating and recompute aggregates. Idempotent: deleting a non-existent rating returns 200 with current aggregates. Returns `{rating_avg, rating_count, my_rating: null}`.
- **FR-6:** `GET /api/v1/templates/{id}/rating` shall be part of the template detail response (FR-3) rather than a separate endpoint; `my_rating` (null for anonymous) is included so the UI can pre-select the user's stars. No public list of who rated what exists.
- **FR-7:** Rate limiting: at most 20 rating changes per minute per account (HTTP 429 with `Retry-After`). Configurable server-side.
- **FR-8:** Rating a template shall NOT emit a notification in v1 (rating is anonymous; a "someone rated your template 3★" notification is noise-prone — deferred with the notifications feature's future preferences work).

### 5.3 Display & discovery (regular-user surface)

- **FR-9:** Template cards (home square, search results, collections, creator profile) shall show a compact star row: average (1 decimal) + count, e.g. "★ 4.6 (128)"; hidden (rendered as "No ratings yet" or nothing) when `rating_count = 0`.
- **FR-10:** The template detail page shall show: interactive 5-star widget (hover preview, click to rate, my-rating state), the average (2 decimals), the count, and a distribution histogram (5 rows: star, bar, percentage).
- **FR-11:** The template listing sort options (search-filtering feature) shall gain `top_rated`: `rating_count >= 5 AND rating_avg DESC` — a minimum-count threshold prevents a single 5★ rating from topping the list; templates below the threshold sort after those above it (by `rating_count DESC` as tiebreak, then `created_at DESC`). The threshold is configurable server-side.
- **FR-12:** The rating widget shall be visually and functionally independent of the like/favorite buttons (separate section, separate semantics: quality vs appreciation); no UI coupling.

### 5.4 Admin surface (read-only)

- **FR-13:** The admin template detail response (`GET /api/v1/admin/templates/{id}`) shall include `rating_avg` and `rating_count` as read-only context. No admin rating-management endpoints in v1 (ratings are anonymous numeric data; there is no content to moderate).
- **FR-14:** Banning a user shall not delete their ratings (they may return after unban); their ratings continue to count in aggregates. Deleting a user cascade-deletes their ratings and recomputes affected aggregates (FR-2).

### 5.5 Behavior on deletions

- **FR-15:** Deleting a template shall cascade-delete its ratings (FK); the aggregate columns go away with the row.

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Rating upsert/delete p95 < 200 ms; aggregate recompute is two integer column updates in the same transaction (no table scans). List queries read the denormalized columns — no N+1, no runtime AVG().
- **NFR-2 (Scalability):** Rate limiting (FR-7) protects the write path. The `top_rated` sort is an indexed-ordering concern (architect may add a composite index or computed column — open question §10.2).
- **NFR-3 (Compatibility):** Additive only — one new table, two new columns on `templates`, new endpoints, card/detail display additions, one new sort value. Existing sorts, filters, and pages behave unchanged.
- **NFR-4 (Privacy):** Ratings are anonymous by design: no public per-rater data; `my_rating` is returned only to its owner; admin sees aggregates only.
- **NFR-5 (Accessibility):** The star widget is keyboard-operable (arrow keys to select, Enter/Space to submit), has an accessible name and value ("Rating: 4 of 5 stars"), and the histogram is a semantic table/list with text values — never color-only. WCAG 2.1 AA.
- **NFR-6 (i18n):** All user-facing strings EN/ZH in sync; star terminology, "Top rated" sort label, "No ratings yet", and count formatting localized; numbers use locale-aware formatting.
- **NFR-7 (Security):** Auth on all writes; server-side validation (1–5 integers only, owner check, ban check); no user-controlled markup; rate limiting.
- **NFR-8 (Resilience):** Rating failures show a toast and revert the widget; list/detail rendering never depends on rating availability (null-safe display).
- **NFR-9 (Data integrity):** Aggregates stay consistent under concurrent ratings (transactional sum/count maintenance); upsert semantics make client retries safe.

## 7. Acceptance Criteria

- **AC-1:** An authenticated user can rate a public template 1–5 stars from the detail page; the average, count, and histogram update immediately and survive reload.
- **AC-2:** Re-rating replaces the previous value (one row per user per template); the aggregate shifts correctly (e.g., 4★ → 2★ moves the average down).
- **AC-3:** The user can delete their rating; aggregates recompute; the widget returns to the unrated state.
- **AC-4:** The template owner cannot rate their own template (403 with message); anonymous visitors see a login prompt; banned users get 403.
- **AC-5:** Cards show "★ 4.6 (128)" when rated and a neutral "No ratings yet" state when `rating_count = 0`.
- **AC-6:** The detail page histogram shows all five rows with correct percentages summing to 100% (± rounding).
- **AC-7:** Sorting by `top_rated` orders by average DESC among templates with `rating_count >= 5`, then by count, then newest; templates under the threshold appear after threshold-qualified ones.
- **AC-8:** Invalid ratings (0, 6, "abc", 3.5) are rejected with 400 field-level errors.
- **AC-9:** Exceeding 20 rating changes/minute returns 429 with `Retry-After`; the UI surfaces a friendly message.
- **AC-10:** The admin template detail page shows the average and count; no admin rating endpoints exist (404 on attempted admin rating routes).
- **AC-11:** Deleting a user removes their ratings and recomputes affected templates' aggregates; deleting a template removes its ratings.
- **AC-12:** All new UI strings exist in both `en` and `zh` translation files with identical key sets; widget, histogram, and sort label render correctly in both languages and themes.
- **AC-13:** The star widget is fully keyboard-operable and announces its value to screen readers; the histogram has text values.
- **AC-14:** With rating APIs failing, the detail page and lists render fully; the widget reverts with an error toast.
- **AC-15:** No endpoint reveals which users rated a template (beyond a user's own `my_rating`).

## 8. Out of Scope (v1)

- Written reviews (text attached to ratings) — the comments feature covers text feedback; combining stars + text is a future "reviews" feature.
- Per-version ratings (ratings attach to the template as a whole).
- Rating weightings (weighted by rater reputation/usage).
- "Verified use" gating (rate only after generating a prompt with the template) — a future trust enhancement.
- Rating notifications to owners (FR-8 rationale).
- Anonymous/guest ratings.
- Admin rating moderation or deletion endpoints.
- Rating export/import.
- Any monetization-adjacent features (per project scope rule).

## 9. Dependencies & Assumptions

- **D-1:** `templates` already carries denormalized `like_count`/`favorite_count` maintained transactionally in repositories; `rating_sum`/`rating_count` follow the identical pattern (BIGINT for sum to be safe).
- **D-2:** The search-filtering feature (designed) defines the sort framework (`newest`, `most_liked`, `most_favorited`, `trending`); `top_rated` joins as a new value with a threshold rule. If search-filtering has not shipped, this feature's sort requirement waits for it (the rating data and display are independently valuable).
- **D-3:** The template detail page already renders like/favorite/fork stats and (with the comments feature) a comments section; the rating widget sits with the social stats, the histogram below it.
- **D-4:** `PromptCard` renders template list items on the home square, search results, collections, and creator profiles; adding the star row is a display-only change that must not break existing card usage.
- **D-5:** The users table has `status` ('active'/'banned') for the ban check (same as comments FR-8).
- **D-6:** Rate limiting reuses the Redis-backed counter approach (same assumption as comments D-7 and follow-creators D-7).
- **D-7:** The REST gateway routes template sub-resources today (`/templates/{id}/likes` style); the rating endpoints follow that pattern. Auth via the existing Bearer-token interceptor.

## 10. Open Questions for Architect

1. **Aggregate storage:** `rating_sum` + `rating_count` columns with computed average (recommended — integer math, no drift) vs. storing `rating_avg` directly (float drift on updates). Confirm the computed approach.
2. **`top_rated` indexing:** with a threshold (`rating_count >= 5`), a plain index on `(rating_sum/rating_count)` is awkward. Options: (a) accept a sort over the filtered set, (b) a computed/generated column `rating_avg` with an index + filter, (c) materialized leaderboard. Recommendation: (a) for v1 with a composite index on `(rating_count, rating_sum)` — revisit if list latency degrades.
3. **Distribution query (FR-3):** a `GROUP BY rating` on `template_ratings` per detail-page view, or a denormalized 5-bucket count set on `templates`? Recommendation: GROUP BY (detail page is low-traffic relative to lists; buckets add 5 more columns to maintain).
4. **Service placement:** extend `PromptService` (ratings are template sub-resources, same as likes; recommended) vs. a new `RatingService`.
