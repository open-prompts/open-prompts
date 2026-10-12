# Webhooks API Integration — Requirements Analysis

- **Feature ID:** webhooks-api-integration
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12
- **Related docs:** [UI/UX design](./webhooks-api-integration-ui-ux-design.md) · [中文需求](./webhooks-api-integration-requirements-zh-CN.md)

## 1. Competitor Research Summary

| Platform | Webhook approach | Key takeaways |
|---|---|---|
| **GitHub** | HMAC-SHA256 signature (`X-Hub-Signature-256`); rich event catalog per resource; delivery log with response status/body and a **Redeliver** button; a `ping` event fires on creation | The delivery log with redeliver is the debugging surface integrators actually use; ping-on-create gives instant end-to-end verification |
| **Stripe** | `Stripe-Signature: t=<unix>,v1=<hmac>` — timestamp bound into the signature defeats replay; retries with exponential backoff over up to 3 days; endpoint **auto-disabled** after sustained failure with email alert; dashboard "send test webhook" with realistic sample payloads | Timestamp-in-signature is the modern scheme; auto-disable protects both sides from a dead endpoint burning retries forever |
| **Shopify** | Webhook subscriptions managed via API (REST/GraphQL) with dotted topics (`products/update`); HMAC shared secret per endpoint | Dotted `resource.action` event naming scales cleanly and reads well in UI checkboxes |
| **Slack / Discord incoming webhooks** | Dead-simple consumer side: POST JSON to a URL, done | The dominant real-world consumer is a chat channel — creators want "post to my Discord when someone forks my prompt" with zero code |
| **Zapier / Make** | Webhook triggers are the universal no-code automation entry point | The persona is a creator, not an infra engineer: the management UI must be as simple as pasting a URL and ticking boxes |
| **PromptBase / FlowGPT / Snack Prompt** | No webhooks, no public APIs | Pattern must come from developer platforms — same conclusion as api-usage-history and rate-limit-dashboard |

**Synthesis:** Open Prompts is pull-only today. Creators have API keys (and, per design, a playground and usage history), but every signal — a like, favorite, fork, or comment on their templates, or a template lifecycle change — surfaces only in the in-app notification bell. The api-key-scopes design explicitly reserved "webhook signing secrets (webhooks-api-integration owns that surface)". This feature adds owner-scoped outbound webhooks: a catalog of 7 events (4 engagement, 3 lifecycle) hooked into the exact code paths that already emit in-app notifications (`createEngagementNotification` call sites), Stripe-style signed payloads (`t=…,sha256=…` over timestamp + body), best-effort async delivery with bounded exponential retries, a full per-webhook delivery log with redeliver and test ping, SSRF-hardened endpoint validation, and a `/webhooks` management page in the existing Aurora design language.

## 2. Problem Statement

1. **Engagement is invisible outside the app.** A creator whose template gets forked 40 times learns about it only by opening the site. There is no push channel to a chat channel, dashboard, or automation flow.
2. **Integrations must poll.** Anything built on the API (usage tracking, mirrors, CI checks on prompt changes) has to poll list endpoints — wasteful for the client and the API (and newly rate-limited).
3. **Lifecycle changes have no observers.** Template created/updated/deleted (including new versions with change notes) cannot drive any downstream automation today.
4. **The API surface has no event contract.** api-key-scopes deliberately deferred signing secrets here; without a signed, versioned payload contract, any consumer would have to scrape or poll.
5. **No debugging surface.** Even if deliveries existed, integrators need GitHub-style delivery logs (status, response code, attempts, redeliver) to answer "did my endpoint receive it, and what did it return?"

## 3. Target Users

- **Creators who automate:** pipe likes/forks/comments into Slack/Discord/Zapier to track traction without opening the site.
- **API integrators:** build mirrors, analytics pipelines, or CI checks driven by template lifecycle events instead of polling.
- **Team collaborators:** keep shared channels informed when a team member publishes or updates a prompt template.
- **Admins:** no dedicated admin surface in v1 (see §8); webhook abuse is contained by per-user caps and SSRF rules.

## 4. User Stories

- **US-1:** As a creator, I want a webhook that posts to my Discord channel whenever someone likes, favorites, forks, or comments on my templates, so that I feel the traction in real time without polling the site.
- **US-2:** As a creator, I want to choose exactly which events a webhook receives (e.g. only forks and comments), so that my channel stays signal, not noise.
- **US-3:** As an integrator, I want each delivery signed with an HMAC secret and timestamp, so that I can verify payloads came from Open Prompts and were not replayed.
- **US-4:** As an integrator, I want the signing secret shown exactly once at creation (and on rotation), so that a database leak cannot forge deliveries to my endpoint.
- **US-5:** As an integrator, I want failed deliveries retried with increasing delays and then marked permanently failed, so that a transient outage doesn't lose events but a dead endpoint doesn't retry forever.
- **US-6:** As an integrator, I want a delivery log per webhook (event, status, response code, attempts, timestamps), so that I can debug what my endpoint returned.
- **US-7:** As an integrator, I want a Redeliver button on any logged delivery, so that I can replay an event my endpoint dropped during a deploy.
- **US-8:** As an integrator, I want a Send-test-event button, so that I can verify my endpoint end-to-end before waiting for real activity.
- **US-9:** As a creator, I want a webhook auto-paused (with a clear banner) after repeated failures, so that I notice my endpoint is broken instead of silently losing events.
- **US-10:** As a user, I want webhook URLs restricted to public HTTPS endpoints, so that the platform cannot be used to probe my internal network (SSRF).
- **US-11:** As a creator, I want webhooks scoped to my own templates only, so that other users' activity never reaches my endpoints.
- **US-12:** As a user, I want the webhook management UI fully in EN and ZH, so that language is never a barrier to automating.

## 5. Functional Requirements

### 5.1 Subscription management

- **FR-1:** New `webhooks` table: `id UUID PK`, `user_id TEXT FK users(id) ON DELETE CASCADE`, `name VARCHAR(100) NOT NULL`, `url TEXT NOT NULL`, `secret TEXT NOT NULL`, `events TEXT[] NOT NULL`, `is_active BOOLEAN NOT NULL DEFAULT TRUE`, `disabled_reason VARCHAR(200)`, `consecutive_failures INT NOT NULL DEFAULT 0`, `created_at/updated_at TIMESTAMPTZ`. REST CRUD (session-auth, owner-scoped): `GET /api/v1/webhooks`, `POST /api/v1/webhooks`, `PATCH /api/v1/webhooks/{id}`, `DELETE /api/v1/webhooks/{id}`. Hard cap: **5 webhooks per user** (409-style error with friendly message). URL must parse as `https://` with a hostname; name 1–100 chars.
- **FR-2:** Event catalog (dotted `resource.action` convention), all **owner-scoped** — a webhook fires only for events on templates owned by the webhook's user; self-engagement (owner liking/favoriting their own template) is suppressed, mirroring `createEngagementNotification`:
  - `template.created` — data: `template_id, title, visibility, category, tags[]`
  - `template.updated` — a new version was created — data: `template_id, title, version, change_note`
  - `template.deleted` — data: `template_id, title`
  - `template.like` / `template.favorite` — data: `template_id, title, actor_id, actor_display_name, action` (`"added"` | `"removed"`)
  - `template.fork` — data: `template_id, title, actor_id, actor_display_name, fork_template_id, fork_title`
  - `template.comment` — data: `template_id, title, actor_id, actor_display_name, comment_id, excerpt` (first 140 chars)
  - `webhook.test` — synthetic, only via the test button — data: `message, sent_at`
- **FR-3:** The signing secret is generated server-side (`whsec_` + 32 hex chars), stored server-side (needed to compute signatures; never returned by `GET`), and revealed **exactly once** in the `POST`/rotate response body. `POST /api/v1/webhooks/{id}/rotate-secret` invalidates the old secret and returns the new one once. The UI shows a one-time modal with a copy button and a "never shown again" warning.

### 5.2 Delivery engine

- **FR-4:** Payload envelope (JSON, UTF-8, ≤ 32 KB): `{ "id": "<event UUID>", "type": "template.fork", "created_at": "<RFC3339>", "data": { … } }`. Headers: `Content-Type: application/json`, `X-OpenPrompts-Event: <type>`, `X-OpenPrompts-Delivery: <delivery UUID>`, `X-OpenPrompts-Signature: t=<unix>,sha256=<hex>` where the HMAC-SHA256 is computed over `"<t>." + raw_body` with the webhook's secret (Stripe-style; enables replay checks: reject if `|now − t| > 5 min`).
- **FR-5:** Dispatch is **best-effort and off the hot path**: after the triggering write commits, a goroutine (same pattern as `createEngagementNotification` and the async `UpdateLastUsed`) fans the event out to all of the owner's active webhooks subscribed to that type. Dispatch failure never fails or delays the user action. HTTP client: 5 s timeout, redirects **not** followed, success = any 2xx.
- **FR-6:** Retry schedule per delivery: initial attempt, then +1 m, +5 m, +30 m, +2 h, +6 h (5 attempts total, ~8.5 h span). Non-2xx, timeout, or connection error = failed attempt → schedule next. After the 5th failure the delivery is marked `failed_permanent`. A scheduler pass (ticker, every 30 s) picks up due retries; in-process only (no external queue in v1).
- **FR-7:** Every attempt is recorded in `webhook_deliveries`: `id UUID PK`, `webhook_id UUID FK CASCADE`, `event_id UUID`, `event_type TEXT`, `payload JSONB`, `status` (`pending|success|failed_permanent`), `attempts INT`, `last_response_status INT`, `last_error VARCHAR(200)`, `next_retry_at TIMESTAMPTZ`, `delivered_at TIMESTAMPTZ`, `is_redelivery BOOLEAN DEFAULT FALSE`, `created_at`. Retention: rows older than 30 days are deleted by the same scheduler pass; the UI shows the latest 50 per webhook.
- **FR-8:** **SSRF protection**, enforced at create/update *and* re-checked at every dispatch (DNS can change): URL scheme `https`; resolved IPs must be public (reject `127.0.0.0/8`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `169.254.0.0/16`, `0.0.0.0/8`, `100.64.0.0/10`, `::1/128`, `fe80::/10`, `fc00::/7`); no redirects; at most 512 bytes of the response body are read (error snippet) and never echoed raw into the UI without escaping.

### 5.3 User-facing surfaces

- **FR-9:** New page `/webhooks` (session-auth; Header nav entry beside API Keys): table of the user's webhooks — name, URL (host + truncated path), event badges, active toggle, last delivery outcome, created date; row actions: edit, rotate secret, delete (confirm dialog). Create/edit modal: name, payload URL (with HTTPS hint), event checkbox matrix grouped "Template lifecycle" / "Engagement", inline validation (URL format, ≥ 1 event, cap reached).
- **FR-10:** Per-webhook **delivery history** panel (expandable row or side drawer): latest 50 deliveries with event type, status chip (Delivered / Retrying / Failed), response code, attempt count, timestamps; per-row **Redeliver** action (`POST /api/v1/webhooks/{id}/deliveries/{delivery_id}/redeliver` — creates a new delivery with the same payload, `is_redelivery = true`, immediate dispatch) and a **Send test event** button (`POST /api/v1/webhooks/{id}/test` — dispatches `webhook.test` immediately).
- **FR-11:** **Auto-pause:** when `consecutive_failures` reaches 20 (counted across events, reset on any success), the webhook is set inactive with `disabled_reason = 'auto: consecutive failures'`. The `/webhooks` page shows a prominent banner on auto-paused rows ("paused after repeated failed deliveries — check your endpoint and re-enable"); re-enabling is one click and resets the counter. No email/in-app push in v1 (the notifications table is engagement-shaped; see §8).
- **FR-12:** All management-UI strings ship in EN and ZH via react-i18next (new `webhooks` namespace); API error bodies stay machine-readable English (API contract, not UI).

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Event dispatch adds nothing to the request path — fan-out happens in a goroutine after the response is committed; the HTTP client enforces a 5 s timeout so goroutines never accumulate on a hung endpoint. The scheduler tick is O(due retries) via a `next_retry_at < now() AND status = 'pending'` index scan.
- **NFR-2 (Reliability):** Delivery is at-least-once within the retry window (duplicates are possible on redeliver or crash-between-send-and-log); consumers deduplicate on the envelope `id`. A dispatch panic is recovered and logged; it never takes down the server process.
- **NFR-3 (Security):** Secrets are stored server-side (required for signing), never returned after the one-time reveal, and excluded from all logs. SSRF rules (FR-8) are enforced at subscription *and* dispatch. Payloads contain actor display names and IDs only — never emails or other users' private data. Webhook CRUD is owner-scoped; IDs are unguessable UUIDs.
- **NFR-4 (Consistency):** Engagement events hook into the same call sites as in-app notifications (`prompt.go` fork/like/favorite, `comment.go` comment), so the two channels can never disagree about whether an event happened. Lifecycle events hook into `CreateTemplate` / `UpdateTemplate` (new-version branch) / `DeleteTemplate` after commit.
- **NFR-5 (Observability):** Every attempt logs a structured line (webhook ID, event type, attempt, outcome, response status, duration) at info/warn. The delivery log is the user-facing mirror of the same data.
- **NFR-6 (i18n):** The `/webhooks` page, modals, banners, and delivery panel are fully translated (EN/ZH); event type names and payload fields remain English (developer-facing contract).

## 7. Acceptance Criteria

- **AC-1:** A user creates a webhook with URL `https://example.com/hook` and events `[template.fork, template.comment]`; another user forks their template; within seconds a `POST` arrives at the URL with `X-OpenPrompts-Event: template.fork`, a valid `t=…,sha256=…` signature over `t + "." + body`, and the fork payload — and the fork action's response time is unchanged.
- **AC-2:** The signing secret appears in the create response and the one-time UI modal; every subsequent `GET /api/v1/webhooks` returns webhooks **without** secrets; after rotation the old secret fails verification and the new one passes.
- **AC-3:** Events the webhook is not subscribed to (e.g. `template.like`) do not trigger a delivery; events on **other users'** templates never trigger a delivery to this webhook.
- **AC-4:** An endpoint returning 500 causes retries at roughly +1 m, +5 m, +30 m, +2 h, +6 h; the delivery log shows `attempts: 5`, `status: failed_permanent`, and the recorded response codes; an endpoint returning 200 on attempt 2 marks the delivery `success` and resets `consecutive_failures`.
- **AC-5:** Creating a 6th webhook fails with the friendly cap message; creating one with `http://` or a private-IP URL is rejected inline and server-side; a webhook whose DNS later resolves to a private IP has deliveries failed closed with an SSRF error recorded.
- **AC-6:** The delivery panel lists the latest 50 deliveries with status chips, response codes, and attempt counts; Redeliver on a logged delivery produces a new delivery row (`is_redelivery`) that arrives at the endpoint with the same payload `id`.
- **AC-7:** Send test event dispatches `webhook.test` immediately; the delivery appears in the log within seconds with the endpoint's response code.
- **AC-8:** After 20 consecutive failed deliveries the webhook flips to paused, the row shows the auto-paused banner, no further events are dispatched until re-enabled, and one click re-enables it with the counter reset.
- **AC-9:** `template.updated` fires only when content actually changes and a new version is created (unchanged-content updates, which skip version creation, do not fire), with `version` and `change_note` in the payload.
- **AC-10:** Deleting a webhook cascades its delivery rows; deleting the user cascades both (FK behavior verified).
- **AC-11:** Delivery rows older than 30 days are purged by the scheduler; the UI never shows more than 50 per webhook regardless of table size.
- **AC-12:** The entire `/webhooks` surface (page, create/edit modal, secret modal, delivery panel, banners, errors) renders completely in EN and ZH with no hardcoded strings.

## 8. Out of Scope (v1)

- External message queue (Kafka/RabbitMQ) — in-process goroutines + DB-backed retries match the codebase's best-effort patterns; revisit at scale.
- Email or in-app push alerts on auto-pause (the `notifications` table is engagement-shaped with `template_id/actor_id NOT NULL`; extending it for system messages is a separate decision).
- Admin webhook console (per-platform delivery stats) — contained by per-user caps and SSRF rules.
- Batched/filtered subscriptions (e.g. per-template webhooks, tag filters) — v1 is all-own-templates per event type.
- Custom payload templates, mTLS, JWS signatures, or endpoint ownership verification (challenge handshake).
- Webhook metrics export (Prometheus) — the delivery log covers v1 debugging.
- Inbound webhooks (Open Prompts as a receiver) — unrelated surface.

## 9. Dependencies & Assumptions

- **D-1:** The engagement hook points exist and are stable: `createEngagementNotification` call sites at `prompt.go:219` (fork), `:604` (like), `:632` (favorite) and `comment.go:113` (comment); lifecycle hooks land in `CreateTemplate` (`prompt.go:92`), `UpdateTemplate`'s new-version branch (`prompt.go:~284–310`), and `DeleteTemplate` (`prompt.go:570`), all after commit.
- **D-2:** api-key-scopes-permissions (design) explicitly reserves webhook signing secrets for this feature (its §8, line 96) — no conflict.
- **D-3:** No external queue or scheduler service is available or required; a DB-backed due-retry scan on a 30 s ticker is acceptable at current scale (same operational simplicity as the Redis throttles).
- **D-4:** The `/webhooks` route is added to `App.js` with session-auth redirect (same pattern as `/api-keys`); the Header gains a nav entry with new i18n keys in both locales.
- **D-5:** `webhook_deliveries` growth is bounded by the 5-webhook cap, event volume, 30-day retention, and the 50-row UI window; standard b-tree indexes on `(webhook_id, created_at DESC)` and `(status, next_retry_at)` suffice.
- **D-6:** The one-time secret reveal follows the APIKeys page's generated-key modal pattern (`api_keys.generated_warning`) — users have already learned "copy it now".

## 10. Open Questions for Architect

1. **Secret storage:** plaintext column (simplest, matches the signing requirement) vs. app-level encryption with a `SECRET_ENCRYPTION_KEY` env — do we have a key-management convention yet?
2. **Scheduler placement:** one goroutine ticker inside the gRPC server process (assumed) vs. a separate worker binary — does the deployment story allow a second process later without schema changes?
3. **Retry persistence vs. Redis:** deliveries are DB-backed (durable, auditable) while throttles are Redis-backed (ephemeral) — is that split acceptable, or should retries also live in Redis for symmetry?
4. **Event envelope versioning:** ship `payload_version: 1` inside the envelope now (cheap insurance) or defer until a breaking change forces it?
5. **Auto-pause threshold:** 20 consecutive failures (~4 h of retries at worst) — reasonable default, or should it be env-configurable like the rate limits?
6. **Fan-out concurrency:** one goroutine per (webhook × event) with no cap (assumed, given the 5-webhook limit) or a small worker pool to bound goroutine counts during traffic spikes?
