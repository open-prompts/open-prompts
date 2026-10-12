# API Usage History — Requirements Analysis

- **Feature ID:** api-usage-history
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. Competitor Research Summary

| Platform | Usage tracking approach | Key takeaways |
|---|---|---|
| **OpenAI Platform (Usage)** | Per-day usage charts (requests, tokens) filterable by API key and project; 30-day default window; CSV export | Day-granularity charts + key filter are the two most-used controls; users want to know "which key is doing what" |
| **Anthropic Console (Usage)** | Activity log of recent API requests (timestamp, model, tokens, latency, status) plus daily aggregate charts | A raw recent-requests table and an aggregate chart serve different questions ("what happened" vs "how much") — ship both |
| **Stripe Dashboard** | Evented request logs (searchable by status, method, path) + aggregate charts; logs retained 30 days | Searchable request logs with status filter are the debugging entry point; bounded retention keeps the table small |
| **GitHub (API usage insights)** | Rate-limit usage breakdown per integration, rolling windows | Framing usage per credential (not just per account) matches how users actually integrate |
| **PromptBase / FlowGPT / Snack Prompt** | No API usage surfaces (web-only products) | Prompt platforms have no usage tracking to copy — the pattern must come from developer platforms |
| **Vercel Dashboard** | Usage meters with soft/hard limits, per-project breakdown, daily charts | A simple "requests this month" meter gives users an instant sense of scale without charts |

**Synthesis:** The winning v1 pattern is the developer-platform usage page, scoped to what Open Prompts actually has: an API keyed by personal API keys, with template resolution as the core call. We introduce a lightweight `api_usage_logs` table (one row per authenticated API-key request: user, key id, endpoint, method, status, latency, timestamp), written asynchronously so the request path is unaffected. The user-facing surface is a "Usage" page: (1) a summary strip (total requests, success rate, avg latency, active keys used, last 24h/7d/30d toggles); (2) a daily-requests bar chart for the window; (3) a filterable, paginated recent-requests table (time, key name, endpoint, status, latency) with key and status filters. Retention is bounded (90 days) with pre-aggregation optional in v2. This is a *regular-user* feature (each user sees only their own usage); admins get an aggregate view in the admin console (top consumers, error rates) to spot abuse.

## 2. Problem Statement

1. **API activity is invisible.** Users create API keys and integrate them, but the product shows nothing about the calls those keys make. `api_keys.last_used_at` is the only signal — a single timestamp, invisible in the UI.
2. **Integration debugging is blind.** When a user's script fails intermittently (429s? 404s? their key expiring?), they have no request log to inspect. They must add their own logging to find out what the API returned.
3. **Key hygiene is impossible.** Users cannot see which of their keys are actively used, how often, or whether an old key is still firing requests — the API Keys page shows only name/prefix/dates.
4. **No abuse signal for admins.** The admin console shows user counts and template moderation, but nothing about API consumption. A runaway script or abusive integration is invisible until it hurts infrastructure.

## 3. Target Users

- **Integrators (primary):** users calling the API with keys — they need per-request visibility for debugging and per-key breakdowns for hygiene.
- **API-curious users:** want to see "my API is being used" as a sense of scale before investing further.
- **Admins:** need aggregate consumption and error-rate signals to detect abuse and capacity issues (admin console surface).
- **Regular web users:** unaffected (web-session traffic is not API-key traffic and is not logged in v1).

## 4. User Stories

- **US-U1:** As an integrator, I want to see my recent API requests (time, key, endpoint, status, latency), so that I can debug my integration without adding my own logging.
- **US-U2:** As an integrator, I want to filter requests by API key and by status (success/error), so that I can isolate a misbehaving script.
- **US-U3:** As an integrator, I want a daily requests chart for the last 7/30 days, so that I can see trends and spot anomalies.
- **US-U4:** As an integrator, I want a summary of total requests, success rate, and average latency over a chosen window, so that I can assess my integration's health at a glance.
- **US-U5:** As an integrator, I want to see, per key, how many requests it made in the window, so that I can decide which keys to keep or revoke.
- **US-U6:** As an integrator, I want usage data scoped strictly to my account, so that I never see another user's traffic.
- **US-U7:** As an admin, I want an aggregate view of API consumption (top consumers, error rate, requests per day), so that I can detect abuse and plan capacity.
- **US-U8:** As a user, I want old usage records to expire automatically (90 days), so that the feature doesn't become a privacy liability.

## 5. Functional Requirements

### 5.1 Data model

- **FR-1:** New table `api_usage_logs`: `id UUID PK`, `user_id TEXT NOT NULL`, `api_key_id UUID NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE`, `endpoint TEXT NOT NULL` (normalized path template, e.g. `/api/v1/templates/{id}/aliases/{alias}`), `method TEXT NOT NULL`, `status INT NOT NULL`, `latency_ms INT NOT NULL`, `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`. Indexes: `(user_id, created_at DESC)`, `(api_key_id, created_at DESC)`, `(created_at)` for retention cleanup.
- **FR-2:** Logging is asynchronous (fire-and-forget write after response, or a buffered channel in the gateway); a logging failure must never fail or delay the API response. Bounded in-memory buffer drops oldest entries under extreme load (usage logging is best-effort).
- **FR-3:** Only API-key-authenticated requests are logged (web-session traffic excluded in v1). Unauthenticated requests (401s) are not logged (no user to attribute).
- **FR-4:** Retention: rows older than 90 days are deleted by a periodic cleanup (daily). No aggregate rollup table in v1.
- **FR-5:** Endpoint paths are stored normalized (path parameters replaced by `{id}`/`{alias}` placeholders) so grouping works and no resource ids leak into analytics.

### 5.2 Regular-user surface (`/usage` page)

- **FR-6:** New route `/usage` (auth required; anonymous → login redirect), linked from the user dropdown and the API Keys page ("View usage" per key and page-level link).
- **FR-7:** **Summary strip** for the selected window (24h / 7d / 30d, default 7d): total requests, success rate (2xx share), average latency, distinct keys used.
- **FR-8:** **Daily chart:** bar chart of requests per day for the window (success vs error stacked); days with zero requests render as zero-height bars.
- **FR-9:** **Per-key breakdown:** table of keys used in the window (key name, prefix, request count, error count, last used) sorted by count desc; clicking a key filters the requests table.
- **FR-10:** **Recent requests table:** paginated (25/page), newest first; columns: time, key (name), endpoint, method, status (color-coded), latency. Filters: key (from FR-9), status (all/success/error). Each row is data-only (no per-request detail page in v1).
- **FR-11:** Empty state when no usage in window: "No API requests in this period" + link to the API Playground to make a first request.
- **FR-12:** All data is strictly owner-scoped (`user_id = me`); no cross-user visibility.

### 5.3 API

- **FR-13:** `GET /api/v1/usage/summary?window=24h|7d|30d` → `{total_requests, success_rate, avg_latency_ms, distinct_keys, daily: [{date, success, error}]}`. Auth required.
- **FR-14:** `GET /api/v1/usage/requests?key_id=&status=&page=&page_size=` → `{requests: [{id, created_at, key_name, key_prefix, endpoint, method, status, latency_ms}], total, page, page_size}`. Auth required; owner-scoped.
- **FR-15:** `GET /api/v1/usage/keys?window=` → `{keys: [{key_id, key_name, prefix, request_count, error_count, last_used_at}]}`. Auth required; owner-scoped.

### 5.4 Admin surface

- **FR-16:** Admin console gains an "API Usage" page under `/admin/usage`: platform-wide summary (total requests, error rate, avg latency for 7d), daily chart, top-10 consumers by request count (user id, email, count, error count), and top endpoints by count. Read-only; no per-user drill-down in v1 (admins can use the user detail page's future usage tab — out of scope here).
- **FR-17:** Admin APIs: `GET /api/v1/admin/usage/summary?window=`, `GET /api/v1/admin/usage/top-users?window=`, `GET /api/v1/admin/usage/endpoints?window=`. Admin session auth; read-only.

### 5.5 Privacy & deletion

- **FR-18:** When a user deletes an API key, associated usage rows cascade-delete (FK) — usage is attributed to keys, and a deleted key's history goes with it. Account deletion (if/when introduced) removes all rows via user_id.
- **FR-19:** Usage rows contain no request bodies, no variable values, no personal data beyond attribution ids — only endpoint, method, status, latency, timestamps.

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** The async log write must add < 1 ms to the API request path (buffered, non-blocking). Usage queries are indexed; summary p95 < 500 ms for a 30-day window.
- **NFR-2 (Storage):** Bounded by retention (90 days) and rate limits; estimated < 100 MB for the v1 user base. Cleanup job runs daily.
- **NFR-3 (Privacy):** Owner-scoped everywhere; admin sees aggregates only (top users list shows email for abuse contact, consistent with the existing admin user list).
- **NFR-4 (i18n):** All strings EN/ZH via i18next (`usage.*` namespace).
- **NFR-5 (Accessibility):** Chart has an accessible data table alternative (toggle); status colors paired with text; full keyboard navigation.
- **NFR-6 (Reliability):** Usage logging is best-effort; its failure never affects API availability (circuit-breaker: disable logging on repeated write errors).

## 7. Acceptance Criteria

- **AC-1:** Making 3 API-key-authenticated requests and opening `/usage` (7d window) shows total = 3, correct success rate, and the 3 rows in the requests table.
- **AC-2:** Filtering by a key shows only that key's requests; filtering by status=error shows only 4xx/5xx rows.
- **AC-3:** The daily chart matches the requests table counts per day for the window.
- **AC-4:** The per-key breakdown lists each used key with correct counts; clicking a key applies the key filter.
- **AC-5:** A web-session (non-API-key) request does not appear in usage.
- **AC-6:** Deleting an API key removes its usage rows; the key disappears from the breakdown.
- **AC-7:** Admin `/admin/usage` shows platform totals and the top-consumers list; a regular user cannot access admin usage APIs (403).
- **AC-8:** Rows older than 90 days are purged by the cleanup job.
- **AC-9:** All strings translated EN/ZH; no hardcoded copy.
- **AC-10:** No monetization content anywhere in the feature (no quotas, no limits-by-tier, no billing concepts).

## 8. Out of Scope

- Quotas, rate-limit tiers, or any limit enforcement based on usage (observability only).
- Monetization of any kind (payments, pricing, purchases, earnings) — permanently out of scope for this product.
- Web-session traffic logging; per-request detail pages; request/response body capture.
- Aggregate rollup tables (v2 if scale demands); CSV export; usage alerts/webhooks.
- Per-user admin drill-down (admin user detail usage tab — future).

## 9. Dependencies & Assumptions

- **D-1:** The gateway (`backend/cmd/server/main.go`) is the single choke point where API-key-authenticated requests are identified (via `AuthInterceptor.VerifyAPIKey` / `VerifyHTTPRequest`) and can be logged with endpoint, method, status, latency.
- **D-2:** The existing `api_keys` table and repository provide key attribution (`api_key_id`); `UpdateLastUsed` already runs async — the same pattern extends to usage logging.
- **D-3:** The user-dashboard feature's `/dashboard` exists (or will exist) as the future home for a usage widget; v1 links from the API Keys page and user dropdown suffice.
- **D-4:** The admin console's existing list-page patterns (filters + table + pagination) are reused for `/admin/usage`.
- **D-5:** Chart rendering uses a lightweight dependency-free approach or an existing chart library if one is already present; the design does not assume a specific library.

## 10. Open Questions for Architect

1. **Write path:** Should usage logging live in the gateway (net/http middleware wrapping handlers, with the normalized path known at route level) or in the gRPC services (via interceptor)? The gateway knows the REST path and status; the services know the RPC — which is the cleaner single choke point, and does the gateway have access to `api_key_id` after auth (it currently resolves only user id)?
2. **Async mechanism:** Buffered channel + background flusher in-process, or direct async INSERT per request (like `UpdateLastUsed`)? What buffer size / drop policy do you recommend?
3. **Normalized paths:** Confirm the normalization map (e.g. `/api/v1/templates/{id}/aliases/{alias}`) should be a static route-table lookup in the gateway rather than regex post-processing.
4. **Retention job:** Reuse an existing cron/scheduler if one exists (admin stats? cleanup?), or introduce a simple time.Ticker in the server? Where should it live?
5. **Admin top-users email visibility:** The admin user list already shows email; confirm the same visibility is acceptable in the top-consumers list (recommended for abuse contact).
