# API Playground — Requirements Analysis

- **Feature ID:** api-playground
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. Competitor Research Summary

| Platform | Developer console / playground approach | Key takeaways |
|---|---|---|
| **OpenAI Platform (API reference + Playground)** | Every REST endpoint has a "try it" panel: auth is injected from the logged-in session, params are editable forms, responses render as formatted JSON with status/timing | Session-injected auth removes the #1 friction (copying keys); per-endpoint forms beat raw curl for discoverability |
| **Anthropic Workbench** | In-browser console: pick a model, edit the request JSON, run, inspect response; request history per workspace | A single focused runner with history is enough — no need for a full Postman clone |
| **Stripe API Explorer** | Interactive docs: endpoint list on the left, parameter form + generated code snippet (curl/Node/Python) on the right, live "Send request" | Generated code snippets that mirror the form state dramatically shorten the path from "try" to "integrate" |
| **Swagger UI / OpenAPI explorers** | Auto-generated try-it forms from a spec; auth via a single API key field at the top | The de-facto baseline UX developers expect; a key field at the top + per-endpoint forms |
| **Postman** | Full workspace: collections, environments, scripts | Far too heavy for an in-product console; we only need "run one request against our own API" |
| **GitHub GraphQL API Explorer** | Docs + runner in one page; uses the logged-in session token, shows both response and generated curl | Session-token-by-default with an option to test a PAT is the right trust model |
| **Kong / Konga, other API gateways** | Admin consoles with request builders | Gateway consoles prove the pattern works for non-LLM APIs too |

**Synthesis:** The winning v1 pattern is the "API explorer" model: a dedicated `/playground` page in the web app where the logged-in user picks one of Open Prompts' own REST endpoints (v1: resolve prompt by alias), fills a small parameter form, and runs the request live. Authentication is injected from the current session by default, with an optional field to paste a personal API key (the same `X-API-Key` mechanism the SDK uses) so users can verify the exact credential their code will use. The response pane shows status, latency, headers, and formatted JSON. A generated code snippet (curl / Python / JavaScript) mirrors the current form state, connecting the playground to the existing SDK Integration card on the template detail page. **LLM execution is explicitly out of scope** — the playground exercises Open Prompts' own REST API (template resolution), not any model provider.

## 2. Problem Statement

1. **The API is invisible.** Open Prompts exposes a real REST API (`GET /api/v1/templates/{id}/aliases/{alias}` resolves a template version with variables, authenticated by `X-API-Key`), but there is no way to try it from the product. Users must read the SDK snippet, leave the app, and hand-write a curl command.
2. **API keys are untested.** Users create keys on the API Keys page but have no way to verify a key works before wiring it into code. A typo'd or revoked key is only discovered inside their own application's error logs.
3. **The template detail page teaches by static code only.** The SDK Integration card shows a Python snippet, but a user cannot see the actual JSON the endpoint returns — the response shape (version content, resolved variables, metadata) is undocumented in-product.
4. **Debugging integration is guesswork.** When a user's integration fails (401? 404 alias? wrong template id?), there is no in-product reference point to compare against.

## 3. Target Users

- **Integrators (primary):** logged-in users who want to call Open Prompts' API from their own code — they need to understand request/response shapes and verify their API key.
- **Template creators:** want to confirm what their template resolves to through the API path (alias → version → content) exactly as a consumer will receive it.
- **Power users evaluating the platform:** want to see "there is a real API and it works" before investing in templates.
- **Admins:** no dedicated admin surface in v1 — the playground calls the regular-user API. (Admin API exploration is out of scope; see §8.)

## 4. User Stories

- **US-P1:** As an integrator, I want to run a real request against the prompt-resolution endpoint from my browser, so that I can see the exact response JSON without writing code.
- **US-P2:** As an integrator, I want the playground to use my logged-in session by default, so that I can try the API immediately without creating a key first.
- **US-P3:** As an integrator, I want to optionally paste one of my API keys and re-run the request with it, so that I can verify the exact credential my code will use.
- **US-P4:** As an integrator, I want a generated code snippet (curl / Python / JavaScript) that mirrors my current form state, so that I can copy working code into my project.
- **US-P5:** As an integrator, I want to see status code, latency, and formatted JSON for each response, so that I can debug my integration against a known-good reference.
- **US-P6:** As a template creator, I want to select one of my templates and an alias, fill in variables, and see the resolved version content, so that I can verify what consumers receive.
- **US-P7:** As an integrator, I want my recent playground requests kept in a local history list, so that I can re-run or compare previous requests in the session.
- **US-P8:** As a user on a slow connection, I want clear loading and error states (network failure, 401, 404), so that I always know what happened.

## 5. Functional Requirements

### 5.1 Data model

- **FR-1:** No new database tables. The playground is a pure frontend surface over the existing REST API (`GET /api/v1/templates/{template_id}/aliases/{alias_name}`) and existing resources (templates, aliases, API keys).
- **FR-2:** Request history is stored client-side only (component state, capped at the 10 most recent entries, cleared on page unload). No server-side history in v1.

### 5.2 Regular-user surface (the `/playground` page)

- **FR-3:** A new route `/playground` is added to the web app, linked from the Header (desktop nav) and the user dropdown, labeled "API Playground" / "API 试验场".
- **FR-4:** The playground shall support exactly one endpoint in v1: `GET /api/v1/templates/{template_id}/aliases/{alias_name}` (resolve template version by alias). The endpoint is presented as a fixed, read-only "request card" (method badge `GET` + path with two path params).
- **FR-5:** The request form shall provide:
  - a **template picker** (searchable dropdown of the user's accessible templates — own templates plus public templates, reusing the existing template list API with a search box);
  - an **alias picker** (dropdown populated from `GET /api/v1/templates/{id}/aliases` for the selected template, defaulting to `latest`, with a free-text override for custom alias names);
  - an **authentication selector**: `Session (default)` or `API key`, plus a password-type input for the key value shown only when `API key` is selected. The key is sent as the `X-API-Key` header, exactly as the SDK does.
- **FR-6:** Selecting a template and alias shall prefetch the alias metadata (available variables list) and render one input field per `$$variable$$` placeholder, so the user can see and fill the variables the endpoint accepts.
- **FR-7:** The **Run** button executes the request from the browser. With `Session` auth the request is sent with the user's existing Authorization header (the web app's normal authenticated axios instance); with `API key` auth the request is sent with only `X-API-Key` set (no session header), so the test is faithful to how server-side code will call it.
- **FR-8:** The response pane shall display: HTTP status code with color coding (2xx green, 4xx amber, 5xx red), latency in ms, response headers (collapsible), and the response body as syntax-highlighted, collapsible JSON.
- **FR-9:** The code snippet pane shall generate, from the current form state, a copyable snippet in three tabs: `curl`, `Python`, `JavaScript (fetch)`. The Python tab mirrors the existing SDK Integration snippet style on the template detail page (using `OpenPromptsClient` with `base_url` and `api_key`). Snippets always use the API-key auth form (never embed the session token).
- **FR-10:** History: each executed request appends an entry (timestamp, template, alias, auth mode, status, latency) to a local list of at most 10 entries; clicking an entry restores its form state and response; entries are not persisted server-side and are lost on reload.
- **FR-11:** Error handling: network failures render a friendly error state with a Retry button; HTTP error responses (401/403/404/429) render the status, the server error message from the body, and a plain-language hint (e.g. 401 → "check that the API key is valid and active").
- **FR-12:** The playground requires login. Anonymous visitors are redirected to login with a return-to-`/playground` redirect, consistent with other authenticated pages.

### 5.3 Admin surface

- **FR-13:** None in v1. The playground calls the regular-user API only. An admin API explorer is a future candidate (see §8).

### 5.4 Rate limiting and abuse

- **FR-14:** The playground issues requests directly from the user's browser to the existing gateway; it adds no new server endpoints and therefore no new attack surface. Standard per-user API rate limits apply to the requests it makes. The Run button is disabled while a request is in flight (no double-submit).

### 5.5 Deletions and lifecycle

- **FR-15:** Deleting a template or alias used in a history entry renders that entry's re-run as a 404 with the standard error hint; history entries are never auto-purged server-side (they are client-side only).

## 6. Non-Functional Requirements

- **NFR-1 (Usability):** A first-time integrator can go from the API Keys page → playground → successful response in under 2 minutes without reading docs.
- **NFR-2 (Performance):** The playground page itself loads with the app bundle; the Run round-trip is bounded only by the existing API latency. Response JSON rendering must handle bodies up to 1 MB without freezing the UI (virtualize/collapse large arrays).
- **NFR-3 (Security):** The API key input is `type="password"` with a show/hide toggle; keys are never written to localStorage; generated snippets contain the literal placeholder `YOUR_API_KEY`, never a real key. The playground never displays or logs session tokens.
- **NFR-4 (Compatibility):** The page works in the same browser matrix as the rest of the app (React 18, modern evergreen browsers), light and dark themes, and is fully responsive down to mobile (stacked layout).
- **NFR-5 (i18n):** All strings EN/ZH via i18next, matching the existing translation.json structure.
- **NFR-6 (Accessibility):** Keyboard-operable end to end (pickers, variable inputs, Run, snippet tabs); WCAG 2.1 AA contrast for status colors; screen-reader announcements for response arrival.
- **NFR-7 (Maintainability):** The endpoint definition (path, params, snippet generators) is data-driven in one config object so future endpoints can be added without restructuring the page.

## 7. Acceptance Criteria

- **AC-1:** Logged-in user can open `/playground` from the Header, pick a template and alias, fill variables, and receive a 200 response showing status, latency, headers, and formatted JSON.
- **AC-2:** Switching auth to `API key` and pasting a valid key from the API Keys page yields a 200; pasting a revoked/garbage key yields 401 with the plain-language hint.
- **AC-3:** The three snippet tabs (curl/Python/JavaScript) reflect the current template id, alias, and variables; copying yields runnable code with `YOUR_API_KEY` placeholder.
- **AC-4:** A request against a non-existent alias yields 404 with the server message and hint rendered in the response pane.
- **AC-5:** History shows the last ≤10 requests in-session; clicking one restores form + response; reload clears it.
- **AC-6:** Anonymous access to `/playground` redirects to login and returns to `/playground` after login.
- **AC-7:** All strings are translated EN/ZH; no hardcoded copy.
- **AC-8:** No monetization content anywhere in the feature.

## 8. Out of Scope

- **LLM execution / model provider calls.** The playground exercises Open Prompts' own REST API only. It never calls any AI model provider and has no model parameters, tokens, or cost concepts.
- Admin API (`/api/v1/admin/*`) exploration.
- Server-side request history, saved collections, or shared playground states.
- POST/PUT/DELETE endpoints (v1 is the read-only resolution endpoint).
- Monetization of any kind (payments, pricing, purchases, earnings) — permanently out of scope for this product.
- Request editing as raw JSON (the form is the single source of truth in v1).
- Mock mode / offline sandbox.

## 9. Dependencies & Assumptions

- **D-1:** The existing endpoint `GET /api/v1/templates/{template_id}/aliases/{alias_name}` (gateway → `PromptService.GetPromptByAlias`) remains the resolution API; the playground is a consumer, not a change to it.
- **D-2:** The existing `X-API-Key` auth path (`AuthInterceptor.VerifyAPIKey`) accepts the same keys created on the API Keys page.
- **D-3:** The existing template list/search API can populate the template picker (public + own templates).
- **D-4:** The existing `GET /api/v1/templates/{id}/aliases` endpoint populates the alias picker.
- **D-5:** The notifications feature (in development) is not required; the playground emits no notifications.
- **D-6:** Assumes the follow-creators feature's "owner identity" work does not conflict; no overlap in endpoints.

## 10. Open Questions for Architect

1. **CORS:** The playground sends browser requests with `X-API-Key` (and session Authorization) to the same origin — confirm the gateway's CORS config already allows the `X-API-Key` header for same-origin (it should be a non-issue, but the header is currently not in `Access-Control-Allow-Headers`).
2. **Auth-mode fidelity:** When `API key` mode is selected, should the request also omit cookies/session headers entirely (recommended, to faithfully simulate server-to-server), and does the gateway treat header-auth and session-auth requests identically for rate limiting?
3. **Endpoint catalog:** Is the single-endpoint v1 acceptable, or should the config-driven design (NFR-7) ship with a second endpoint (e.g. `GET /api/v1/templates/{id}`) to prove the pattern?
4. **Snippet accuracy:** The Python snippet references `OpenPromptsClient` (matching the template detail SDK card). Is there a published SDK to link to, or should the Python tab use plain `requests` until an SDK exists?
