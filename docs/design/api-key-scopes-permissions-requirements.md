# API Key Scopes & Permissions — Requirements Analysis

- **Feature ID:** api-key-scopes-permissions
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. Competitor Research Summary

| Platform | Scope pattern | Key takeaways |
|---|---|---|
| **OpenAI Platform (API keys)** | Keys are full-access by default; service accounts / project keys carry roles; users are told to "treat keys like passwords" and rotate on suspicion | Even the biggest platform ships coarse keys first, then adds scoping once users integrate broadly; the UI's job is to make the blast radius of each key visible |
| **GitHub (Personal Access Tokens)** | Fine-grained PATs: per-repository selection + a permission matrix (Contents, Issues, Metadata…) with No access / Read / Read-write states; classic tokens use coarse `scope` strings (`repo`, `read:org`) shown as chips at creation | The permission matrix with explicit per-permission levels is the gold standard; showing effective permissions as chips on the token list lets users audit at a glance; "last used" column drives cleanup |
| **Anthropic Console (API keys)** | Simple key list with name, created, last used; no per-key scopes in v1 — safety comes from workspace separation | Minimalism is viable, but users explicitly ask for scoped keys in forums; a middle ground (read-only vs full) covers most needs |
| **Hugging Face (access tokens)** | Two tiers: `read` (public content) and `write`/`fine-grained` (per-repo write, gated repos); role selector at creation, role badge on the token row | A two-tier choice (read vs write) captures ~80% of the value with ~20% of the complexity; fine-grained is opt-in |
| **Stripe (restricted API keys)** | "Standard keys" (full access) vs "Restricted keys" with per-permission Read/Write toggles; restricted keys are the recommended default for integrations | Naming the two modes explicitly ("standard" vs "restricted") and recommending restricted by default measurably reduces incident impact; permission groups are business-domain based, not endpoint based |
| **Vercel / Supabase** | Scope checkboxes at creation; scopes immutable after creation — changing means creating a new key | Immutable scopes keep the model simple; users accept "create a new key" as the path to change |

**Synthesis:** The winning v1 pattern is the **two-tier + optional fine-grained model**: every key gets a role — `read-only` (safe default for SDK/production resolution) or `full` (everything the account can do) — plus an optional per-permission matrix for users who want it (GitHub/Stripe style). Scopes are **immutable after creation** (Vercel/Supabasemodel): to change, create a new key. The API Keys page gains a role badge per key, a scope picker in the create modal, and a "last used" column that already exists. Enforcement happens in `VerifyAPIKey`, which today returns only a userID and grants the key the full power of the account (including deleting templates and creating new keys).

## 2. Problem Statement

1. **Every key is a master key.** `VerifyAPIKey` (auth.go:246-283) resolves the key to a userID and nothing else. A key minted for one purpose — resolving prompts in a production app via `GET /api/v1/templates/{id}/aliases/{alias}` — can also delete the user's templates, create new keys, and change the profile. There is no way to hand a script, a teammate, or a CI job a credential that cannot destroy data.
2. **The blast radius is invisible.** The API Keys page shows name, prefix, created, last used — nothing tells the user what a key can do. Users cannot audit "which of my keys could have done this?" after an incident.
3. **No revocation without deletion.** `api_keys.is_active` exists in the schema and is checked by `VerifyAPIKey`, but there is no RPC and no UI to toggle it. The only remediation is deleting the key entirely, destroying the audit trail (name, created_at, last_used_at) that api-usage-history relies on.
4. **No expiry choice in the UI.** `expires_days` exists in the proto (`CreateAPIKeyRequest`) and the repository, but the create modal collects only a name. Users cannot mint short-lived keys even though the backend supports it.

## 3. Target Users

- **Integrators (primary):** users who call `GET /api/v1/templates/{id}/aliases/{alias}` from their own apps with an API key — they want a read-only key that cannot modify anything if it leaks.
- **Security-conscious creators (secondary):** users with many templates who connect scripts, automations, or teammates' tools and want to limit each credential to what it actually needs.
- **Incident responders:** users (and admins triaging with them) who need to see, per key, what it could have done and when it was last used.

## 4. User Stories

- **US-A1:** As an integrator, I want to create a read-only API key, so that the key I paste into my production app can fetch prompts but can never modify or delete my templates.
- **US-A2:** As an integrator, I want to see a role badge (Read-only / Full access) on each key in the list, so that I can audit at a glance which keys are dangerous.
- **US-A3:** As a security-conscious creator, I want to pick individual permissions (e.g. "read templates" but not "write templates") when creating a key, so that each credential gets exactly the access it needs.
- **US-A4:** As a user, I want to temporarily disable a key without deleting it, so that I can investigate a suspicious usage pattern while keeping the key's history.
- **US-A5:** As a user, I want to re-enable a disabled key, so that a false alarm doesn't force me to redistribute a new secret to all my integrations.
- **US-A6:** As an integrator, I want to set an expiry (30/60/90/180 days or never) when creating a key, so that forgotten keys die on their own.
- **US-A7:** As a user, I want the create modal to warn me in plain language when I am about to mint a full-access key, so that I consciously choose the dangerous option.
- **US-A8:** As an incident responder, I want to see "last used" and the role on every key, so that I can answer "which key did this, and what could it have done?"
- **US-A9:** As a user, I want an inactive/expired key to be visually distinct (muted row, badge), so that stale keys don't look live.
- **US-A10:** As a user, I want scope changes to require creating a new key (with the old one kept until I delete it), so that scope history stays honest and simple.
- **US-A11:** As an integrator, I want a scoped key that attempts a write operation to receive a clear 403 error naming the missing scope, so that my code can surface an actionable message instead of a generic failure.
- **US-A12:** As an admin, I want users' keys to be scoped automatically by this model, so that a leaked user key can never touch admin surfaces (admin RPCs are session-token only and stay that way).

## 5. Functional Requirements

- **FR-1:** **Schema.** Add `role TEXT NOT NULL DEFAULT 'full'` and `scopes TEXT[] NOT NULL DEFAULT '{}'` to `api_keys`. `role` ∈ {`read-only`, `full`}; `scopes` is a list of permission codes (may be empty). Existing keys migrate to `role='full', scopes='{}'` (behavior unchanged). Add `updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`.
- **FR-2:** **Permission model.** Permission codes: `templates:read`, `templates:write`, `templates:delete`, `comments:read`, `comments:write`, `keys:read`, `keys:write`. Role resolution: `full` → all permissions the account has; `read-only` → {`templates:read`, `comments:read`, `keys:read`}; if `scopes` is non-empty it **overrides** the role preset (fine-grained mode). Admin RPCs are never grantable to API keys.
- **FR-3:** **Enforcement point.** `VerifyAPIKey` returns `(userID, *KeyContext)` where `KeyContext = {Role, Scopes, KeyID}`. The gateway and service layer check the required permission per operation before executing; missing permission → `403` with body `{"error": "insufficient_scope", "required": "<code>"}`. Unauthenticated/invalid key remains `401`.
- **FR-4:** **Read-only key behavior.** A `read-only` key may: list/get templates and versions, resolve aliases, list comments, list its sibling keys' metadata. It may not: create/update/delete templates, fork, like, favorite, comment, create/update/delete aliases, create/delete keys, update profile.
- **FR-5:** **Create API.** `CreateAPIKeyRequest` gains `role` (default `full` for backward compatibility) and `repeated string scopes` and `expires_days` is surfaced in the UI (already in proto). Response metadata includes role and effective scopes. Validation: unknown role or scope code → `400 INVALID_ARGUMENT`.
- **FR-6:** **List API.** `APIKey` message gains `role`, `scopes` (effective list), and `is_active` (already in schema, now surfaced). The API Keys table renders a role badge and an active/expired state per row.
- **FR-7:** **Activation toggle.** New RPC `SetAPIKeyActive(id, is_active)`; owner-scoped; toggling writes `updated_at`; audit trail preserved (no deletion). UI: toggle in the row's action menu with a confirm step for disabling.
- **FR-8:** **Expiry.** Create modal offers Never / 30 / 60 / 90 / 180 days (default Never). Expired keys fail auth with `401 api key expired` (existing behavior) and render as "Expired" in the list.
- **FR-9:** **Immutability.** Role and scopes cannot be changed after creation. The edit affordance is absent; the UI offers "Duplicate with new scopes" (pre-fills the create modal with the same name + ` (2)` and the old key's scope selection).
- **FR-10:** **Playground integration.** The api-playground auth selector, when `API key` is chosen, shows the key's role inline after validation and blocks Run with an inline error if the selected endpoint requires a permission the key lacks (playground v1 endpoint is read-only, so read-only keys work).
- **FR-11:** **Usage integration.** api-usage-history's per-key table gains a role column; the key filter dropdown shows the role badge next to each key name.
- **FR-12:** **Security invariants.** API keys can never call admin RPCs (enforced structurally: admin handlers require the admin session token, not `VerifyAPIKey`); a key can never grant permissions the account doesn't have; `keys:write` does not allow escalating (a fine-grained key with `keys:write` can create keys only with a subset of its own effective scopes).

## 6. Non-Functional Requirements

- **NFR-1:** **Zero behavior change for existing keys.** Keys created before this feature behave exactly as before (full access). No client update required for the SDK or existing integrations.
- **NFR-2:** **Performance.** Scope resolution adds no extra DB round-trip: role/scopes are columns on the `api_keys` row already fetched by `GetByHash`. Permission check is an in-memory set lookup. p95 auth overhead < 1 ms.
- **NFR-3:** **Clarity.** Every permission denial names the missing permission code in the response body and the UI maps codes to human-readable strings in EN/ZH.
- **NFR-4:** **Accessibility.** Role badges are text (not color-only); the scope picker is keyboard-navigable with visible focus; state changes announce via live region.
- **NFR-5:** **i18n.** All new strings in EN/ZH from day one; permission names and denial messages localized.
- **NFR-6:** **Auditability.** Key creation (with role), activation toggle, and deletion are visible events: creation/toggle/deletion update `updated_at`/`last_used_at` appropriately; per-key usage remains queryable via api-usage-history.

## 7. Acceptance Criteria

- **AC-1:** Creating a key with role `read-only` and using it to `GET /api/v1/templates/{id}/aliases/{alias}` succeeds; using the same key to `PUT /api/v1/templates/{id}` returns `403` with `{"error":"insufficient_scope","required":"templates:write"}`.
- **AC-2:** The API Keys list shows a role badge per key (Read-only / Full access / Fine-grained) and an Active / Disabled / Expired state; disabled and expired rows are visually muted.
- **AC-3:** Creating a key with an invalid scope code (`foo:bar`) returns `400` and the modal shows an inline error; the list is unchanged.
- **AC-4:** Disabling a key via the row action menu requires confirmation; after confirmation the row shows Disabled, `updated_at` changes, and requests with that key return `401 api key is inactive or invalid` (existing message).
- **AC-5:** Re-enabling a disabled key restores access without re-issuing the secret; the row returns to Active.
- **AC-6:** The create modal offers expiry Never/30/60/90/180; a key created with 30 days shows "Expires in 30 days" in the list and fails with `401 api key expired` after expiry.
- **AC-7:** No edit affordance exists for role/scopes after creation; "Duplicate with new scopes" opens the create modal pre-filled with the source key's name + " (2)" and its scope selection.
- **AC-8:** A fine-grained key with scopes `[templates:read]` can list and get templates but receives `403 insufficient_scope` (naming `comments:write`) when posting a comment.
- **AC-9:** A fine-grained key with `keys:write` can create a new key, but attempting to grant the new key a scope it does not itself have returns `400` with an escalation error.
- **AC-10:** The playground, when run with a read-only key against the alias-resolution endpoint, succeeds and displays the key's role inline; the usage page's key filter shows role badges.
- **AC-11:** All existing keys (pre-feature) continue to work unchanged: full access, no client-side migration, SDK requests succeed.
- **AC-12:** All new UI strings exist in both `en` and `zh` translation files; the scope picker, badges, and denial messages are fully localized; keyboard-only users can complete create/duplicate/disable flows.

## 8. Out of Scope (v1)

- Per-template or per-collection resource restrictions (fine-grained keys are account-wide permission sets).
- Organization/team keys (team-collaboration feature owns shared credentials).
- Key rotation automation (scheduled rotation, rotation reminders via email).
- IP allowlists / CIDR restrictions per key.
- Rate limits per key (rate-limit-dashboard owns per-key limits).
- Webhook signing secrets (webhooks-api-integration owns that surface).
- Admin-issued or admin-scoped keys; admin console key management UI.
- Scope changes after creation (immutable by design; duplicate-and-replace is the path).

## 9. Dependencies & Assumptions

- **D-1:** `api_keys` table (init.sql:185-198) exists with `is_active`, `expires_at`, `last_used_at`; `APIKeyRepository` (api_key_repository.go) provides `GetByHash`, `Create`, `ListByUserID`, `Delete`. Assumes adding columns + `UpdateActive` method is straightforward.
- **D-2:** `VerifyAPIKey` (auth.go:246-283) is the single choke point for API-key auth in both the gRPC interceptor and the REST gateway (`VerifyAnyToken` fallback path); assumes threading a `KeyContext` through context is acceptable to the architect.
- **D-3:** `CreateAPIKeyRequest.expires_days` already exists in the proto; assumes extending messages with `role`/`scopes` fields is backward-compatible (proto3 optional fields).
- **D-4:** APIKeyManager component (frontend/src/components/APIKeyManager.js) and the APIKeys page exist with a create modal and table; assumes extending the modal and table is the natural extension point.
- **D-5:** api-playground (design) defines the auth selector with `X-API-Key`; api-usage-history (design) defines the per-key usage table — both list this feature as an integration point, not a blocker.
- **D-6:** Assumes the architect confirms the permission-code taxonomy in FR-2 (7 codes) and the "scopes override role preset" resolution rule.

## 10. Open Questions for Architect

1. **Enforcement placement:** centralize permission checks in the gateway handlers (per-route required-permission map) vs. inside each service method (defense in depth)? The design assumes a per-route map in the gateway plus service-level checks on mutating RPCs.
2. **KeyContext propagation:** extend `VerifyAPIKey` to return a struct carried via `context.Context` (like `ContextWithUserScope` for JWTs) — acceptable shape? Any preference on the context key type?
3. **Postgres array vs. JSONB for `scopes`:** `TEXT[]` assumed for simplicity; JSONB if queryability by scope matters later?
4. **`SetAPIKeyActive` transport:** new REST route `PATCH /api/v1/api-keys/{id}` with `{"is_active": bool}` vs. POST subresource `/activate` + `/deactivate`? Design assumes PATCH.
5. **Escalation rule (FR-12):** is "subset-of-own-scopes" the right rule for `keys:write`, or should fine-grained keys be barred from key management entirely?
6. **Migration:** default `role='full'` for existing rows — confirm no need for a backfill beyond the column default.
