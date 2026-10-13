# Personalized Recommendations — Requirements Analysis

- **Feature ID:** personalized-recommendations
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-13

## 1. Competitor Research Summary

| Platform | Personalization approach | Key takeaways |
|---|---|---|
| **YouTube** | Home feed driven by watch history; "Not interested" and "Don't recommend channel" on every card; "Why this video?" explanation link | Per-card dismissal with undo is the trust mechanism; explanation ("because you watched X") makes the feed feel fair |
| **Reddit** | Interest picker at signup seeds the feed; "Popular" fallback before enough history exists | Cold start is solved with a non-personalized default feed, not an empty page |
| **FlowGPT** | "For You" tab on home based on likes/follows; "Trending" as the default anonymous experience | A separate tab/rail keeps personalized and discovery browsing distinct |
| **AIPRM** | Library sorted by category selected at install; no per-user learning | Even static interest selection materially improves relevance vs. a generic list |
| **Spotify** | "Because you like X" rails; "Hide this song" per card; users trust the system because reasons are visible | Reason labels are cheap (one string) and are the single biggest trust driver |
| **Medium** | "Based on your reading" section; explicit followed topics editable in settings | Letting users see and edit their interest profile converts a black box into a controllable feature |

**Synthesis:** The proven v1 shape is a **content-based "For You" rail** on the home page, computed from the user's own on-platform actions (favorites, likes, uses, views), with **visible reason labels** ("Because you liked …", "More in Marketing"), **per-card "Not interested" dismissal with undo**, and an **editable interest profile** so users can see and correct what the system knows about them. Cold-start users (no signal yet) get the existing trending/popular content as a labeled fallback — never an empty rail. No ML, no embeddings, no cross-user signals in v1: the recommender is a transparent function of the user's own interaction history and template metadata.

## 2. Problem Statement

Open Prompts' home page shows every logged-in user the same newest-first list (plus search/filter/sort from template-search-filtering and metadata rails from related-templates). Three gaps remain:

1. **No "for me" surface.** A user who favorites and uses marketing templates every week still sees an undifferentiated square. The strongest engagement surface on every competitor — a personalized rail — is missing.
2. **Signals exist but are unused.** The platform already records favorites, likes, uses (`prompts`), and (per prompt-analytics) view events. None of this feeds back into what the user sees next.
3. **No user control over personalization.** Where competitors do personalize, users demand (and get) "Not interested" controls and a view into their interest profile. Designing personalization without control would be a trust regression; this feature ships both together.

This feature was explicitly deferred by two shipped designs: related-templates §8 ("Personalized recommendations, view-history signals, co-interaction matrices; dislike/'not interested' controls on rail cards") and onboarding §8 ("Category-interest personalization picker"). It now delivers those deferrals as one coherent feature.

## 3. Target Users

- **Active regular users (primary):** users with ≥3 interaction signals (favorites/likes/uses/views) who want the home page to reflect their actual interests.
- **New/cold-start users:** users with <3 signals; they see a clearly-labeled "Popular this week" fallback rail, never an empty or broken section.
- **Privacy-conscious users:** users who want to see, edit, or reset exactly what the platform inferred about them.
- **Admins:** no admin surface in this feature (see FR-24); recommendations are a regular-user experience only.

## 4. User Stories

### 4.1 Receiving recommendations

- **US-R1:** As a logged-in user with interaction history, I see a "For You" rail at the top of the home page, so that content matching my interests is immediately visible.
- **US-R2:** As a user, each recommended card shows why it was recommended ("Because you liked Cold Outreach", "More in Marketing"), so that I trust the rail.
- **US-R3:** As a user, I never see my own templates, templates I already favorited, or private templates in my recommendations.
- **US-R4:** As a user, recommendations refresh as my behavior changes (favorite a new category → it appears), so the rail stays relevant.
- **US-R5:** As a cold-start user (<3 signals), I see a "Popular this week" rail instead, so the home page never feels empty or broken.

### 4.2 Controlling recommendations

- **US-R6:** As a user, I can dismiss any recommended card with a "Not interested" action, so that it stops appearing.
- **US-R7:** As a user, after dismissing I get an undo option, so that mistakes are reversible.
- **US-R8:** As a user, I can open an "Interests" page showing the categories and tags inferred from my activity with weight bars, so that I understand what drives my rail.
- **US-R9:** As a user, I can remove a topic from my interest profile or reset the whole profile, so that I control my personalization.
- **US-R10:** As a user, I can turn recommendations off entirely, so that the home page returns to the standard browsing experience.

### 4.3 System stories

- **US-R11:** As the system, I compute recommendations only from the user's own interactions and public template metadata, so that no cross-user profiling exists.
- **US-R12:** As the system, I cache recommendation results per user (short TTL) so the home page stays fast.
- **US-R13:** As the system, I never include dismissed templates again unless the user resets dismissals.

## 5. Functional Requirements

### 5.1 Signal model

- **FR-1:** The system shall compute a per-user **interest profile** from four signal sources, weighted: favorite = 5, use = 4, like = 3, view = 1.
- **FR-2:** Signals shall be counted over a trailing 90-day window; older interactions do not contribute.
- **FR-3:** The profile shall be a list of `{type: category|tag, value, score}` entries, aggregated per topic: score = Σ(signal weight) over the user's interactions with templates carrying that topic.
- **FR-4:** Signal sources in v1: `template_favorites`, `template_likes`, `prompts` (uses, web source), and view events from the prompt-analytics event table. If prompt-analytics has not shipped, views contribute 0 and the feature degrades gracefully (favorites/likes/uses only) — see D-3.
- **FR-5:** The profile shall keep at most 20 topics (top by score); ties broken alphabetically for determinism.

### 5.2 Recommendation computation

- **FR-6:** The system shall recommend public templates (visibility public, non-banned authors, per existing listing rules) that match the user's top interests and that the user has not already favorited, liked, or authored.
- **FR-7:** Candidate ranking score shall be: `match_score × log10(1 + like_count)`, where `match_score` = Σ(score of the candidate's topics present in the user's top-10 interests). Ties broken by `created_at DESC` for freshness.
- **FR-8:** Each recommended item shall carry a **reason**: `{type: "because_you_liked"|"because_you_used"|"because_you_favorited", template_title}` when a single source template's topic dominates, else `{type: "topic", topic_type, topic_value}` ("More in Marketing"). The reason is derived server-side and rendered as-is.
- **FR-9:** The rail shall return up to 12 items; the client requests `limit=12` and renders what it receives (rail hidden if <3, matching related-templates' convention).
- **FR-10:** Dismissed templates (FR-16) shall be excluded from results.
- **FR-11:** Results shall be cached per user for 10 minutes (server-side); new interactions are reflected within one TTL window.

### 5.3 Home page rail

- **FR-12:** A "For You" rail shall render at the top of the home page (above the template grid, below the search bar) for logged-in users with recommendations enabled and ≥1 computed item.
- **FR-13:** For logged-in users with <3 interaction signals, the rail shall instead show "Popular this week" (top by like_count + favorite_count over the last 7 days, reusing the trending definition from template-search-filtering FR-10), labeled as such.
- **FR-14:** Anonymous visitors shall see no rail (the standard browsing experience); no anonymous personalization or tracking.
- **FR-15:** The rail shall be dismissible per session via its header "Hide for now" control; it reappears on the next page load.

### 5.4 Not interested

- **FR-16:** Each recommended card shall offer a "Not interested" action (visible on hover/focus, always keyboard-reachable) that records a dismissal for (user, template).
- **FR-17:** Dismissing shall remove the card from the rail immediately (client-side) and show a toast with an **Undo** action for 8 seconds.
- **FR-18:** Dismissals persist indefinitely; a "Reset dismissals" action on the Interests page clears them all.
- **FR-19:** Dismissal of a template does not affect anything else (not likes, not favorites, not other surfaces).

### 5.5 Interests management

- **FR-20:** An **Interests** page shall be available at `/settings/interests` (requires login) showing: the interest profile as topic chips with weight bars (relative to the top topic), the recommendation on/off toggle, the reset-profile action, and the reset-dismissals action.
- **FR-21:** Removing a topic excludes it from the interest profile (and thus from future recommendations) until new interactions re-add it; removed topics are listed as "Removed topics" with a restore action.
- **FR-22:** Resetting the profile clears all scores and removed-topic records; the rail falls back to "Popular this week" until signals rebuild.
- **FR-23:** Turning recommendations off hides the rail entirely (FR-12/13 do not render); the setting persists per user.

### 5.6 Regular-user vs admin split

- **FR-24:** All surfaces in this feature are regular-user surfaces (`/`, `/settings/interests`, `/api/v1/recommendations*`, `/api/v1/me/interests*`). No admin console pages and no `/api/v1/admin` endpoints are added; admin analytics of recommendation performance are out of scope (§8).

### 5.7 API

- **FR-25:** `GET /api/v1/recommendations?limit=12` → `{items: [{template_id, reason, …card fields}], fallback: bool}`. Auth required; 401 for anonymous.
- **FR-26:** `POST /api/v1/recommendations/{template_id}/dismiss` → 204; idempotent.
- **FR-27:** `GET /api/v1/me/interests` → `{topics: [{type, value, score}], removed_topics: [{type, value}], recommendations_enabled, signal_summary: {favorites, likes, uses, views}}`.
- **FR-28:** `DELETE /api/v1/me/interests/topics/{type}/{value}` → removes topic; `POST /api/v1/me/interests/topics/{type}/{value}/restore` → restores a removed topic.
- **FR-29:** `POST /api/v1/me/interests/reset` → clears profile and removed topics; `POST /api/v1/me/interests/dismissals/reset` → clears dismissals; `PUT /api/v1/me/interests/preferences` → `{recommendations_enabled: bool}`.

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** `GET /recommendations` p95 < 300ms warm (cache hit) and < 800ms cold (profile computation over ≤90-day window); the rail loads non-blocking after the main grid query.
- **NFR-2 (Privacy):** Recommendations use only first-party interaction data already stored (favorites, likes, uses, views). No new tracking, no cross-user signals, no third-party data. The interest profile is fully visible and editable by the user (FR-20–22).
- **NFR-3 (Transparency):** Every recommended item carries a reason (FR-8); the Interests page explains the signal weights in plain language.
- **NFR-4 (Compatibility):** Users with recommendations disabled or anonymous visitors see the exact current home page; no existing endpoint changes.
- **NFR-5 (Accessibility):** Rail, dismiss action, undo toast, and Interests page are fully keyboard operable and screen-reader announced (UI/UX doc §9).
- **NFR-6 (i18n):** All new UI strings in EN and ZH with identical key sets; topic values (category/tag names) are proper nouns rendered untranslated.
- **NFR-7 (Determinism):** Same user, same profile state, same TTL window → same result order (stable tie-breakers per FR-5/FR-7).
- **NFR-8 (Data):** New tables: `recommendation_dismissals (user_id, template_id, created_at)` and `user_interest_settings (user_id, recommendations_enabled, …)`; interest profile computed on demand or cached — no profile table required in v1 (architect decision, see OQ-2).

## 7. Acceptance Criteria

- **AC-1:** A user who favorites 3 marketing templates sees a "For You" rail whose items are public marketing templates (or carry marketing topic reasons), none of which are their own or already-favorited templates.
- **AC-2:** Every rail card displays a reason label; at least one item shows "Because you {liked|used|favorited} {title}" when a dominant source template exists.
- **AC-3:** Clicking "Not interested" removes the card instantly, shows an undo toast, and the template does not reappear on reload; clicking Undo restores it.
- **AC-4:** A brand-new account sees "Popular this week" (labeled), not an empty rail or an error.
- **AC-5:** An anonymous visitor sees no rail.
- **AC-6:** The Interests page lists topics with weight bars matching the API's scores; removing a topic causes it to disappear from subsequent recommendations within one cache TTL.
- **AC-7:** "Reset profile" empties topics; the rail falls back to "Popular this week".
- **AC-8:** Turning recommendations off removes the rail for that user on every subsequent visit; turning it back on restores it.
- **AC-9:** `GET /api/v1/recommendations` returns 401 for anonymous requests; all `/me/interests` endpoints require auth.
- **AC-10:** Private templates and templates of banned authors never appear in recommendations.
- **AC-11:** All new UI strings exist in both `en` and `zh` translation files with identical key sets.

## 8. Out of Scope (v1)

- Collaborative filtering / co-interaction signals ("users like you also liked").
- ML models, embeddings, semantic similarity.
- Personalized sort of the main grid (the rail is additive; the grid stays user-controlled).
- "Not interested" reasons or per-topic feedback ("fewer like this") — v1 dismisses the single template only.
- Recommendation rails on template detail pages (related-templates owns that surface, non-personalized).
- Anonymous personalization (device fingerprinting, local-history-only rails) — permanently out of scope.
- Admin analytics or tuning UI for recommendations.
- Email digests or push notifications based on recommendations.
- Monetization of any kind (sponsored placements, promoted cards in the rail) — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1:** `template_favorites`, `template_likes`, and `prompts` tables with `user_id`/`created_at` exist and are queryable for the 90-day window; `templates` carries `category`, `tags`, `like_count`, `visibility`, `owner_id`.
- **D-2:** The public-listing visibility rules (public, non-banned authors) are reusable query fragments from the existing list endpoints (same assumption as related-templates D-2).
- **D-3:** View events depend on the prompt-analytics feature's event table (`FR-5` there). If it has not shipped, this feature computes from favorites/likes/uses only; the signal weight table and UI copy degrade gracefully (views shown as 0 on the Interests page).
- **D-4:** The trending definition (7-day like+favorite window) is shared with template-search-filtering FR-10; the fallback rail reuses it rather than redefining it.
- **D-5:** The home page (Home.js) has room above the grid for one rail; the rail follows the horizontal-card conventions established by related-templates (TemplateCard reuse, scroll-snap on mobile).
- **D-6:** Toast/undo infrastructure: notifications design provides toasts; if not yet implemented, a minimal inline undo toast is built within this feature.
- **D-7:** `/settings` route prefix exists or is introduced by another in-flight feature (e.g., notification-preferences); this feature adds `/settings/interests` under it without claiming ownership of the parent route.
- **D-8:** Auth middleware distinguishes anonymous vs authenticated for the home page data fetch; the rail request is simply not fired when unauthenticated (client-side gate + server 401).

## 10. Open Questions for Architect

1. **Profile computation timing:** compute the interest profile on every `GET /recommendations` (simple, always fresh, cost = 4 aggregate queries) vs. materialize per user with a TTL refresh job (faster, more moving parts)? Design assumes on-demand with a 10-minute result cache (FR-11).
2. **Storage of removed topics:** derived-only (recomputed minus removals needs a removal table anyway) — confirm `user_interest_topics_removed (user_id, type, value)` as a small dedicated table vs. a JSONB column on `user_interest_settings`.
3. **View-event dependency ordering:** if prompt-analytics ships later, should this feature merge first with a 3-signal model (favorites/likes/uses) and add views afterward? Design says yes — the signal table is data-driven.
4. **Dismissal scope:** per-template only in v1 (FR-19); is a future per-topic suppression worth a schema placeholder now? Design says no — add when needed.
