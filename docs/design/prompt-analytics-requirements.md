# Prompt Analytics — Requirements Analysis

- **Feature ID:** prompt-analytics
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2025-06-25

## 1. Competitor Research Summary

| Platform | Analytics approach | Key takeaways |
|---|---|---|
| **PromptBase** | Seller dashboard: sales, views, revenue over time; per-listing stats; conversion (view→sale) | Creators are motivated by a dedicated dashboard; time-series charts are the core; conversion funnels are the differentiator |
| **FlowGPT** | Public view/like/star counts on every prompt; creator profile aggregates; trending ranks by engagement | Public counters create social proof and drive the ranking system; per-item counts are table stakes |
| **Snack Prompt** | View counts, likes, "remixes" (uses) shown per prompt; creator pages aggregate them | "Remix/use" count is the prompt-marketplace equivalent of a conversion metric |
| **AIPRM** | Views, votes, use counts per prompt; library sorted by these metrics | Use count is the primary quality signal for prompt marketplaces |
| **PromptHero** | View counts and likes per item; hot ranking from recent engagement | Time-windowed engagement (not lifetime totals) drives discovery |
| **Generic creator platforms (YouTube/Medium/GitHub)** | Creator analytics: views/reads over time, top content, traffic sources, audience geography | Time-range selector (7/30/90 days), top-content table, and a single overview page are the standard shape |

**Synthesis:** Two surfaces consistently emerge: (1) **public per-template counters** (views, uses, likes, favorites) that create social proof and feed ranking; (2) a **private creator analytics dashboard** with time-series charts, a time-range selector, and a top-templates table. Conversion-style metrics (view→use) are the PromptBase differentiator worth copying cheaply (a ratio, not a funnel).

## 2. Problem Statement

Open Prompts records likes, favorites, and prompt instantiations (`prompts` table), but exposes none of it as analytics:

1. **No view tracking.** Nobody — creator or platform — knows how often a template is viewed. The strongest engagement signal is simply missing.
2. **No use tracking surface.** The `prompts` table records instantiations (web) and `template_aliases` + `GetPromptByAlias` serve API-key usage, but there is no aggregated "uses" metric anywhere in the product.
3. **No creator dashboard.** A creator with 20 templates has no way to see which perform well, how engagement trends, or what changed after an edit. The only signals are the like/favorite counts on each card.
4. **No time dimension.** All counters are lifetime totals; there is no way to see "last 30 days" activity, which is what creators actually act on.
5. **Public pages under-inform.** Template detail shows like/favorite counts but not views/uses, weakening social proof versus every competitor.

## 3. Target Users

- **Creators (primary):** users who publish templates and want to understand performance — which templates get views, which convert views to uses, what trends after edits.
- **Browsers (secondary):** benefit from richer social proof (view/use counts) on public template pages.
- **Admins (indirect):** platform-level analytics are out of scope for v1 (see §8) but the event table introduced here is the foundation.

## 4. User Stories

### 4.1 Creators

- **US-A1:** As a creator, I want a private analytics page showing aggregate views, uses, likes, and favorites across all my templates for a selected time range, so that I understand my overall performance.
- **US-A2:** As a creator, I want a time-series chart (daily counts) for each metric, so that I can spot trends and react to changes.
- **US-A3:** As a creator, I want a top-templates table ranked by uses (or views) in the range, so that I know which templates to invest in.
- **US-A4:** As a creator, I want per-template detail stats (views, uses, likes, favorites, view→use rate) on the template page when I am the owner, so that I can evaluate a single template.
- **US-A5:** As a creator, I want to compare a time range against the previous equivalent range (e.g., this 30 days vs. previous 30 days), so that I know if I'm growing.
- **US-A6:** As a creator, I want to see the split of uses by source (web UI vs. API key), so that I understand how my templates are consumed.

### 4.2 Browsers

- **US-A7:** As a visitor, I want to see view and use counts on the template detail page, so that I can judge a template's popularity before using it.

### 4.3 System

- **US-A8:** As the system, I record a view event when a template detail page is rendered (deduplicated per session), so that view counts are meaningful without being inflated.
- **US-A9:** As the system, I record a use event when a prompt is instantiated from a template (web `CreatePrompt` or API `GetPromptByAlias`), so that the use metric captures both surfaces.
- **US-A10:** As the system, I keep event data minimal (no prompt content, no variables, no PII beyond optional user id), so that analytics storage is privacy-safe.

## 5. Functional Requirements

### 5.1 Event capture

- **FR-1:** The system shall record a `view` event for a template each time its detail page is loaded by a user (authenticated or anonymous).
- **FR-2:** View deduplication: at most one view event per (template, session) per 30 minutes; a session is identified by a client-generated id stored in localStorage (anonymous) or the user id (authenticated).
- **FR-3:** The system shall record a `use` event when a prompt is created from a template via the web UI (`CreatePrompt`) with source `web`.
- **FR-4:** The system shall record a `use` event when a template version is fetched via an alias with an API key (`GetPromptByAlias`) with source `api`.
- **FR-5:** Events shall store: `id`, `template_id`, `event_type` (`view`|`use`), `source` (`web`|`api`), `user_id` (nullable), `session_id` (nullable, views only), `created_at`. No prompt content, variables, or other PII.
- **FR-6:** Like and favorite events already exist (`template_likes`, `template_favorites` with timestamps); analytics shall aggregate them from these tables rather than duplicating events.
- **FR-7:** Event writes shall never block or fail the user-facing operation that triggers them (fire-and-forget with buffered async write; loss of analytics data is acceptable under load).

### 5.2 Public counters

- **FR-8:** The template detail page shall display view count and use count alongside the existing like/favorite counts, for all visitors.
- **FR-9:** The system shall maintain denormalized `view_count` and `use_count` columns on the `templates` table (incremented on event write) for cheap public display; the event table remains the source of truth for time-series.
- **FR-10:** PromptCard shall display a compact view count (e.g., "1.2k") in v1 only if layout permits without redesign; otherwise counts appear on the detail page only. *(Decision: detail page only in v1 — see §8.)*

### 5.3 Creator analytics page

- **FR-11:** A private analytics page shall be available at `/analytics` (requires login; redirects to login when anonymous).
- **FR-12:** The page shall provide a time-range selector: Last 7 days / 30 days / 90 days (default 30).
- **FR-13:** The page shall show four KPI cards — Views, Uses, New Likes, New Favorites — each with the range total and the percentage change vs. the previous equivalent range.
- **FR-14:** The page shall show a daily time-series chart with metric toggles (Views / Uses / Likes / Favorites; default Views+Uses) for the selected range.
- **FR-15:** The page shall show a uses-by-source breakdown (web vs. API) for the range as a two-segment bar or proportion display.
- **FR-16:** The page shall show a top-templates table (top 10 by uses in range, then views): columns — template title, views, uses, new likes, new favorites, view→use rate — with a link to each template's detail page.
- **FR-17:** The selected time range shall be reflected in the URL (`?range=7|30|90`) and restorable via deep link.

### 5.4 Per-template owner stats

- **FR-18:** When the logged-in user is the template owner, the template detail page shall show an owner-only stats section: the four counters, view→use rate, and a 30-day sparkline of daily views+uses.
- **FR-19:** Owner stats shall be hidden from non-owners (they see only the public counters of FR-8).

### 5.5 API

- **FR-20:** New RPC `GetTemplateStats(template_id, days)` → per-template daily series for the four metrics, range totals, source split, and view→use rate. REST: `GET /api/v1/templates/{id}/stats?days=30`. Owner-only (403 for non-owners).
- **FR-21:** New RPC `GetCreatorStats(days)` → aggregate KPIs with previous-range deltas, daily series, source split, and top-templates list for the authenticated user's templates. REST: `GET /api/v1/analytics/overview?days=30`.
- **FR-22:** New RPC `RecordTemplateEvent(template_id, event_type, session_id)` → records a view event (use events are recorded server-side inside CreatePrompt/GetPromptByAlias, not by clients). REST: `POST /api/v1/templates/{id}/events` with body `{type: "view", session_id: "..."}`. Anonymous allowed; rate-limited.
- **FR-23:** `days` shall accept only 7, 30, 90; other values → 400 or fallback to 30 (server decision; documented for architect).
- **FR-24:** Aggregation queries shall bucket by day in the database (UTC) and return `[{date, views, uses, likes, favorites}]` arrays sized to the range.

### 5.6 Regular-user vs admin split

- **FR-25:** All v1 analytics are regular-user surface (creator = regular user who owns templates). No admin analytics pages; the admin dashboard (admin-user-management feature) shows platform KPIs only and does not consume this feature's endpoints.
- **FR-26:** Admins acting as creators see their own creator analytics like any user; admin role grants no access to other users' analytics.

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Event write path adds <5ms p99 to the triggering request (async buffer); analytics read endpoints p95 < 800ms for a creator with ≤1,000 templates and ≤1M events (daily aggregation + indexes).
- **NFR-2 (Scale):** Event table partitioning or a retention policy (e.g., raw events kept 180 days, daily rollups kept forever) may be required at scale; v1 ships unpartitioned with indexes and documents the rollup path (architect decision, see D-4).
- **NFR-3 (Privacy):** No prompt content or variables in events; user_id optional; session_id is a random client id, not a tracking cookie; analytics data is never exposed to third parties.
- **NFR-4 (Compatibility):** No breaking changes; new columns (`view_count`, `use_count`) default 0; existing clients unaffected.
- **NFR-5 (Accuracy):** Counters are eventually consistent (async writes may lag seconds); the UI does not need real-time precision.
- **NFR-6 (Accessibility):** Charts must have accessible alternatives (table toggle or `aria` summary); KPI cards and tables keyboard-navigable (see UI/UX doc §10).
- **NFR-7 (i18n):** All new strings EN/ZH in sync; numbers formatted per locale (1.2k / 1,200 conventions via i18next).
- **NFR-8 (Security):** Stats endpoints enforce ownership (FR-20/21); event endpoint is rate-limited (e.g., 60 req/min/IP) and validates template existence; no injection surface (parameterized aggregation SQL).
- **NFR-9 (Observability):** Event recorder logs event counts per minute and buffer drops; read endpoints log query duration and row counts.

## 7. Acceptance Criteria

- **AC-1:** Loading a template detail page as an anonymous visitor increments its view count (visible on reload) and records a view event; reloading within 30 minutes does not double-count.
- **AC-2:** Creating a prompt from a template in the web UI records a `use` event with source `web`; fetching a template version via alias with an API key records `use` with source `api`.
- **AC-3:** The template detail page shows view and use counts to all visitors.
- **AC-4:** The owner viewing their template detail page sees the owner stats section (counters, view→use rate, 30-day sparkline); a non-owner does not.
- **AC-5:** `/analytics` redirects anonymous users to login; logged-in users see KPI cards, chart, source split, and top-templates table for the default 30-day range.
- **AC-6:** Switching the range to 7 or 90 days updates KPIs, chart, and table, and the URL (`?range=7`).
- **AC-7:** KPI cards show the percentage change vs. the previous equivalent range (e.g., 30-day range compares to the prior 30 days).
- **AC-8:** The top-templates table ranks by uses in range, links to template detail pages, and shows view→use rate.
- **AC-9:** `GET /api/v1/templates/{id}/stats` returns 403 for a non-owner and full stats for the owner.
- **AC-10:** A user with zero templates sees a meaningful empty state on `/analytics` (explanation + link to create a template).
- **AC-11:** All new UI strings exist in both `en` and `zh` translation files with identical key sets.
- **AC-12:** Existing like/favorite counts and behaviors are unchanged (no regression).

## 8. Out of Scope (v1)

- Platform-wide/admin analytics (the admin dashboard from admin-user-management covers platform KPIs; deeper analytics later).
- Audience geography, traffic sources/referrers, device breakdown.
- Conversion funnels beyond the view→use ratio.
- Per-version analytics (v1 aggregates at template level).
- Real-time streaming analytics.
- Email/weekly digest reports.
- CSV export of analytics.
- PromptCard view-count display (detail page only in v1).
- Event retention rollup jobs (documented as future work, D-4).
- A/B testing of template content.
- Public creator profile pages with aggregate stats.

## 9. Dependencies & Assumptions

- **D-1:** `template_likes` and `template_favorites` rows carry `created_at` timestamps usable for time-series aggregation (confirmed in schema); if not, like/favorite series degrade to lifetime totals (architect to verify).
- **D-2:** The `prompts` table's `created_at` provides historical use events for backfill; v1 may backfill `use` events from it or simply aggregate it in parallel — architect to choose (backfill is recommended so day-1 charts are not empty).
- **D-3:** `GetPromptByAlias` (API path) can be instrumented server-side without changing its response contract.
- **D-4:** New table `template_usage_events` + indexes on (`template_id`, `created_at`) and (`user_id`, `created_at`); `templates` gains `view_count`/`use_count` integer columns default 0. Rollup/retention strategy is a documented future optimization (NFR-2).
- **D-5:** Frontend has no chart library; v1 charts are custom lightweight SVG components (no new dependency). Adding recharts is an alternative the developer may propose; the design accommodates either (see UI/UX doc §11).
- **D-6:** Session id for view dedup is a random UUID in localStorage (`op_analytics_sid`), created on first visit; it is not shared with any third party.
- **D-7:** `/analytics` route requires the existing auth guard pattern (like `/profile`); no new auth machinery.
- **D-8:** Timezone: all aggregation in UTC; the UI displays dates without timezone conversion (day labels as YYYY-MM-DD), acceptable for v1.
- **D-9:** Rate limiting for the public event endpoint (FR-22) assumes the gateway can apply per-IP limits (existing middleware or a simple in-memory limiter; architect to confirm mechanism).

## 10. Open Questions for Architect

*(None blocking. D-2 backfill choice, D-4 rollup strategy, and D-9 rate-limit mechanism are the flagged decisions.)*
