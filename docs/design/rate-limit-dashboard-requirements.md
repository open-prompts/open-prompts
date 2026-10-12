# Rate Limit Dashboard — Requirements Analysis

- **Feature ID:** rate-limit-dashboard
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12
- **Related docs:** [UI/UX design](./rate-limit-dashboard-ui-ux-design.md) · [中文需求](./rate-limit-dashboard-requirements-zh-CN.md)

## 1. Competitor Research Summary

| Platform | Rate limiting approach | Key takeaways |
|---|---|---|
| **GitHub REST API** | 60 req/h unauthenticated (per IP), 5,000/h authenticated (per user); `X-RateLimit-Limit/Remaining/Reset` headers on **every** response; 429 with `Retry-After` | Headers on every response (not just errors) let well-behaved clients self-regulate before hitting the wall — the single most-copied convention |
| **OpenAI Platform** | Tiered RPM/TPM limits per endpoint class; 429 with structured error + retry guidance; usage dashboard shows consumption against limits | Tiering by endpoint class (cheap reads vs expensive writes) protects the DB without punishing browsing; showing "usage vs limit" meters reduces support tickets |
| **Stripe** | Separate limits for live vs test traffic; sensitive endpoints (e.g. token creation) throttled harder; 429 body explains the limit | Sensitive endpoints deserve their own, tighter bucket — one global limit is never right |
| **Cloudflare (rate limiting rules)** | Admin dashboard: blocked-requests chart over time, top client IPs, rule hit counts, manual block/allow | The admin surface matters as much as the enforcement: "who is being throttled right now" is the first question during an incident |
| **Vercel** | Usage meters per project with soft/hard limits and reset countdowns | A simple "X of Y used, resets in N min" meter communicates 90% of what users need |
| **PromptBase / FlowGPT / Snack Prompt** | No public APIs, no rate limiting surfaces | Pattern must come from developer platforms — same conclusion as api-usage-history |

**Synthesis:** Open Prompts currently throttles exactly two paths — comments (per-user, Redis fixed-window, `COMMENT_RATE_PER_MINUTE/DAY`) and admin login (per-IP, 5 fails/15 min) — while the other ~23 public REST routes and the email-sending `/api/v1/verification-code` endpoint have **no limiting at all**. The api-playground design already *assumes* "standard per-user API rate limits apply", and api-usage-history deliberately excluded enforcement. This feature closes that gap with the codebase's existing pattern: a gateway-level Redis fixed-window limiter keyed by identity (user ID when authenticated, client IP otherwise), tiered by endpoint class (read / write / sensitive), emitting standard `429 + Retry-After + X-RateLimit-*` headers. The user-facing surface is (1) a global 429 toast with a humanized retry countdown (generalizing what CommentComposer already does locally) and (2) a "Limits" card on the `/usage` page showing current-window meters. The admin surface is a `/admin/rate-limits` dashboard: throttled-request summary, top throttled IPs/users, and a reason-gated "clear throttle" remediation action written to the admin audit log.

## 2. Problem Statement

1. **The public API is unprotected.** All 25 REST routes in `cmd/server/main.go` (templates, categories, tags, login, register, profile, comments, API keys, …) accept unbounded request rates. A single script can hammer template listing or fork endpoints.
2. **The verification-code endpoint is an email bomb.** `SendVerificationCode` (`user.go:57-91`) sends an email on every call with no per-email or per-IP limit — a loop over the endpoint can spam arbitrary inboxes and exhaust the email provider quota.
3. **429 handling is local, not global.** `CommentComposer` and `AdminLoginPage` each hand-roll 429 parsing; every other surface (template create, like, fork, API key management) shows a generic error when throttled — which will become common once limits exist.
4. **Users have no visibility into their own limits.** Once limits exist, integrators need to know how close they are (GitHub-style headers + a meter), or they will file "the API is broken" reports.
5. **Admins are blind during incidents.** There is no surface showing who is being throttled, on which endpoint, or how often — the first question in any abuse or outage investigation.

## 3. Target Users

- **Anonymous browsers:** hit per-IP read limits only in pathological cases; need a clear, calm 429 message.
- **Registered users (web):** comment/like/fork bursts can trip write limits; need the retry countdown inline where they acted.
- **API integrators:** drive the most traffic via API keys; need `X-RateLimit-*` headers and a usage-page meter to self-regulate.
- **Admins:** need to detect abuse (scrapers, bombers), distinguish them from legitimate heavy users, and remediate (clear a wrongly-throttled actor) with an audit trail.

## 4. User Stories

- **US-1:** As an anonymous visitor, I want a clear message with a retry countdown when I'm throttled, so that I know the site is not broken and when to come back.
- **US-2:** As a commenter, I want the Post button to show a live countdown after a 429, so that I don't lose my drafted comment or re-submit blindly.
- **US-3:** As an API integrator, I want `X-RateLimit-Limit/Remaining/Reset` headers on every response, so that my client can back off before being throttled.
- **US-4:** As an API integrator, I want a Limits card on my Usage page, so that I can see how close I am to the read/write ceilings without making a probe request.
- **US-5:** As an API integrator, I want a structured 429 body (`retry_after` in seconds), so that my error handling can be automatic rather than string-parsing.
- **US-6:** As a user requesting a verification code, I want sane per-email limits, so that nobody can use the endpoint to flood my inbox.
- **US-7:** As a user, I want my API-key traffic to count against a predictable per-user budget, so that one integration can't starve my other keys (and so the rules are simple to reason about).
- **US-8:** As an admin, I want a dashboard of throttled requests over the last 24h (volume, unique actors, top endpoints), so that I can spot abuse waves quickly.
- **US-9:** As an admin, I want the top throttled IPs and users with request counts and last-seen times, so that I can tell a scraper from a legitimate power user.
- **US-10:** As an admin, I want to clear a throttle counter for a wrongly-blocked actor, so that a false positive doesn't lock someone out for the full window.
- **US-11:** As an admin, I want every clear-throttle action to require a reason and appear in the audit log, so that remediation power is accountable.
- **US-12:** As a developer using the API playground, I want to see the rate-limit headers in the response panel, so that I can learn my remaining budget while experimenting.

## 5. Functional Requirements

### 5.1 Enforcement (gateway)

- **FR-1:** A rate-limit middleware wraps every `/api/v1/*` REST handler in `cmd/server/main.go`. Identity resolution: authenticated (session JWT or API key) → user ID; anonymous → client IP (existing gateway helper, already forwarded as `x-client-ip` metadata). All requests within one identity share one set of counters.
- **FR-2:** Three limit tiers, each a Redis fixed-window counter per identity (reusing the `checkCommentRate` pattern — `count:expires` value, TTL = window): **read** (GET) default 120/min; **write** (POST/PUT/PATCH/DELETE) default 30/min; **sensitive** (`/verification-code`, `/register`, `/login`) default 10/min per IP *in addition to* their write bucket. All values env-configurable (`RATE_LIMIT_READ_PER_MIN`, `RATE_LIMIT_WRITE_PER_MIN`, `RATE_LIMIT_SENSITIVE_PER_MIN`); unset env → documented default.
- **FR-3:** Every response (not only 429) carries `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset` (unix seconds) for the tier that served the request. A 429 additionally carries `Retry-After` (seconds until the strictest tripped window resets) and a JSON body `{ "error": "rate_limit_exceeded", "retry_after": <seconds>, "tier": "read|write|sensitive" }`.
- **FR-4:** `/api/v1/verification-code` gains two extra dedicated counters: per-email (default 3/hour) and per-IP (default 10/hour), checked *before* the email is sent. Tripping either returns 429 without sending.
- **FR-5:** The middleware **fails open** when Redis is unavailable (nil client, Get/Set errors) — identical to the comment and admin-login throttles. Availability of the API outranks protection; the outage is logged once per minute, not per request.
- **FR-6:** API-key-authenticated requests count against the **key owner's** user-ID buckets (not per-key buckets). Per-key limits are out of scope (see §8).

### 5.2 User-facing surfaces

- **FR-7:** A global axios response interceptor catches any 429 from `/api/v1/*`: it shows one toast ("Too many requests — try again in Ns", N from `Retry-After`, default 60) and suppresses duplicate toasts for the same window. Surfaces with bespoke handling (CommentComposer, AdminLoginPage) keep their richer local UX and opt out of the global toast.
- **FR-8:** Any submit button whose action just returned 429 disables itself and renders a live countdown ("Retry in Ns") re-enabled at zero; the user's in-progress input (draft, form state) is never cleared.
- **FR-9:** New endpoint `GET /api/v1/usage/limits` (auth required) → `{ read: {limit, remaining, reset_at}, write: {limit, remaining, reset_at} }` read from the caller's Redis counters. The `/usage` page (api-usage-history) gains a **Limits card** with two meters (read/write), remaining counts, and a reset countdown; the card degrades to "Limits unavailable" when Redis is down (fail-open parity).
- **FR-10:** The API playground response panel (api-playground design) displays the `X-RateLimit-*` and `Retry-After` headers when present, so integrators can watch their budget while testing.

### 5.3 Admin dashboard

- **FR-11:** Every 429 increments Redis hourly counters (24h retention): `rl_throttle:h:<YYYYMMDDHH>` (total), `rl_throttle:ip:<ip>:h:<…>`, `rl_throttle:user:<id>:h:<…>`, `rl_throttle:ep:<endpoint>:h:<…>`. New admin page `/admin/rate-limits` (admin session required) shows: summary cards (throttled requests 24h, unique throttled actors, most-throttled endpoint), a 24-hour throttled-requests bar chart, and two tables — top throttled IPs and top throttled users (identity, request count, last seen, endpoints hit) — each row linking to the actor's existing detail surface where applicable (user detail page).
- **FR-12:** Admin **clear-throttle** action: deletes all rate-limit counters for one actor (IP or user ID) via `DELETE /api/v1/admin/rate-limits/throttle?actor_type=ip|user&actor_id=…`. Requires a reason (3–500 chars, same rule as other audit actions), writes an `admin_audit_logs` row (action CHECK extended with `rate_limit_clear`, `target_type` extended with `ip`), and confirms in the UI. Read-only otherwise: the dashboard displays env-configured limits but cannot change them at runtime.

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** The middleware adds < 1 ms at p99 to the request path (one Redis pipeline of GET+SET per tier; counters are O(1)). No additional DB writes on the hot path — throttle analytics live in Redis only.
- **NFR-2 (Availability):** Fail-open on Redis outage (FR-5); the API never returns 429 because the *limiter* is broken. `/usage/limits` and the admin dashboard degrade gracefully to "unavailable" states.
- **NFR-3 (Security/privacy):** Redis keys hold only IPs, user IDs, endpoint paths, and counts — no payloads, no PII beyond what the audit log already stores. All counters have TTLs ≤ 24h (identity windows ≤ 1h). Clear-throttle is admin-only, reason-gated, and audited.
- **NFR-4 (Consistency):** One limiter implementation shared by all routes (the comment throttle's fixed-window pattern generalized); no per-route hand-rolled counters. Existing comment/admin-login limits remain as-is (stricter, domain-specific) and are documented on the admin page as "additional limits".
- **NFR-5 (Observability):** 429s log a structured line (identity type, tier, endpoint, retry-after) at warn level, sampled at most once per identity-minute to avoid log flooding during an attack.
- **NFR-6 (i18n):** All user-facing strings (global toast, countdown buttons, Limits card, admin dashboard) ship in EN and ZH via react-i18next; the 429 JSON body stays machine-readable English (it is an API contract, not UI).

## 7. Acceptance Criteria

- **AC-1:** With defaults, an anonymous script issuing 130 GETs to `/api/v1/templates` within one minute receives 429 on the 121st request, with `Retry-After` ≤ 60 and `X-RateLimit-Remaining: 0`; the 120 successes each carried `X-RateLimit-Remaining` counting down.
- **AC-2:** An authenticated user's GETs and POSTs draw from separate buckets: 120 reads do not block a subsequent template create; the 31st write in a minute is throttled.
- **AC-3:** Requests with a valid API key and the same user's session requests share one budget (same user ID → same counters).
- **AC-4:** Calling `/api/v1/verification-code` 4 times for one email within an hour: the first 3 send (or log, in test mode), the 4th returns 429 and sends nothing.
- **AC-5:** With Redis stopped, all routes continue serving normally (fail-open), `/usage/limits` returns 503-with-friendly-UI "Limits unavailable", and exactly one warn log per minute is emitted.
- **AC-6:** A 429 on template like/fork shows the global toast with the correct countdown and no generic "error" toast; the like button re-enables when the countdown reaches zero.
- **AC-7:** A 429 from the comment composer keeps the drafted text, shows the composer's local countdown (not the global toast), and posts successfully after the window resets.
- **AC-8:** The `/usage` page Limits card shows read/write meters matching a fresh `X-RateLimit-Remaining` from a probe request, and counts down toward the displayed reset time.
- **AC-9:** The API playground response panel lists `X-RateLimit-Limit/Remaining/Reset` when the backend sends them.
- **AC-10:** `/admin/rate-limits` shows the 24h throttled total, chart, and top-IP/user tables within 5 minutes of a throttle burst (hourly bucket granularity); an empty state renders when nothing was throttled.
- **AC-11:** Clear-throttle without a reason is rejected (3–500 chars enforced); with a valid reason the actor's counters are deleted, the row's status updates, and an `admin_audit_logs` row with action `rate_limit_clear` and the reason exists.
- **AC-12:** All limit values are overridable via env vars with documented defaults; unset vars never disable the limiter (defaults apply), and the admin dashboard displays the effective values.

## 8. Out of Scope (v1)

- Per-API-key limits (all keys share the owner's user budget — FR-6).
- Sliding-window or token-bucket algorithms (fixed window matches the existing pattern; revisit if burst-fairness complaints arise).
- Quotas, usage caps, or billing tiers (api-usage-history already excludes enforcement; no monetization in scope).
- Runtime limit configuration from the admin UI (env-only in v1).
- Automatic IP bans / abuse escalation (block-users covers user-level blocking; IP bans are a separate feature).
- gRPC-internal limits (only the public REST gateway is wrapped).
- Rate limiting of static assets or the admin console's own login (already throttled).

## 9. Dependencies & Assumptions

- **D-1:** Redis is deployed and wired in `cmd/server/main.go` (verified: `data.NewRedisClient`, line 111) and already backs the comment and admin-login throttles; the limiter reuses this client.
- **D-2:** The gateway's client-IP helper (`main.go:~1528-1539`, forwarded as `x-client-ip` metadata) is trusted — deployment sits behind a correctly-configured proxy that sets `RemoteAddr`; spoofing mitigation is a deployment concern noted in §10.
- **D-3:** api-usage-history (design) defines the `/usage` page where the Limits card lands; this feature supplies the card and the `/usage/limits` endpoint.
- **D-4:** api-playground (design) defines the response panel that will render the rate-limit headers (FR-10 is an integration note, not a blocker).
- **D-5:** The `admin_audit_logs` action/target_type CHECK constraints are extended in a migration using the same additive pattern as prior features (archive-templates extended the same CHECK).
- **D-6:** CommentComposer's and AdminLoginPage's existing local 429 handling remain the canonical bespoke UX; the global interceptor (FR-7) must not double-toast on those surfaces.

## 10. Open Questions for Architect

1. **Per-user vs per-IP for authenticated traffic:** should authenticated users *also* draw from an IP bucket (defense against one IP rotating many accounts), or is user-ID-only sufficient for v1?
2. **IPv6:** rate-limit whole /64s (recommended, prevents trivial rotation) or exact addresses?
3. **Trusted proxy:** is `RemoteAddr` guaranteed to be the real client (proxy protocol / X-Forwarded-For normalization at the ingress), or should the gateway parse a forwarded header?
4. **Window alignment:** fixed windows anchored at first request (current comment pattern) vs wall-clock-aligned minutes (simpler reset display)? The Limits card's reset countdown differs between the two.
5. **Counter write strategy:** INCR-with-TTL (two round trips, atomic) vs the existing GET-then-SET pattern (one logical read, non-atomic but consistent with the codebase) — which do we standardize on?
6. **Should `/usage/limits` also expose the sensitive tier** (verification-code budget) or only read/write, given sensitive is IP-keyed for anonymous flows?
7. **Audit CHECK extension:** extend `target_type` with `ip` as designed, or log IP clears as `target_type: user, target_id: ip:<addr>` to avoid touching the constraint?
