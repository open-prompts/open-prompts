# Template Locks & Drafts (Concurrent Edit Protection) — Requirements Analysis

- **Feature ID:** template-locks-drafts
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. Competitor Research Summary

| Platform | Concurrency pattern | Key takeaways |
|---|---|---|
| **GitHub** | Optimistic concurrency: saving an edited file with a stale base shows a full conflict screen (yours vs theirs) and forces explicit resolution | "Your changes were made on an out-of-date base" + diff resolution is the gold standard for text conflicts; silent overwrite is never acceptable |
| **Google Docs** | Realtime presence (cursors, "X is editing") + character-level OT/CRDT | Full realtime is overkill for prompt text; but *presence awareness* alone prevents most conflicts before they happen |
| **Notion** | Presence indicators + last-write-wins with page history (restore) | Presence + recoverable history is the pragmatic middle ground when realtime merge isn't feasible |
| **MediaWiki (Wikipedia)** | Hard edit lock with a timeout ("someone is editing this page, wait or edit anyway") + edit conflict screen on save | The lease-with-timeout is battle-tested; "edit anyway" avoids lock-hostage when someone walks away |
| **Confluence** | Collaborative editing for pages; legacy mode used "you are editing, others see a lock" notification | Lock visibility to *viewers* (read-only banner) reduces accidental duplicate work |
| **Stack Overflow** | Optimistic revision check on post edit (5-minute grace, revision number) | Simplest viable pattern: send expected revision, reject stale saves with a clear message |

**Synthesis:** Two complementary mechanisms cover the space without realtime infrastructure (the platform has no WebSocket/SSE today): (1) **presence awareness** — an advisory, Redis-backed edit lease with heartbeat so a second editor sees "X is editing" *before* starting (MediaWiki/Confluence pattern, with "edit anyway" escape hatch), and (2) **optimistic concurrency** — `UpdateTemplate` accepts the `expected_version` the editor loaded; a mismatch returns a conflict that the UI resolves explicitly (Stack Overflow/GitHub pattern). Open Prompts currently has **neither**: `UpdateTemplate` is last-write-wins, and the version history (which would let a victim recover their text) is only visible after the fact. The advanced-editor design already adds localStorage drafts (client-side safety); this feature adds the *server-side* correctness layer and the presence UX, and integrates with that draft mechanism.

## 2. Problem Statement

1. **Silent lost updates.** Two sessions editing the same template (two tabs, two team members, owner + admin) both load v7; the first save creates v8; the second save creates v9 *from the v7 text* — the first editor's changes are silently erased. Nothing warns either user at any point.
2. **No presence awareness.** A second editor has no idea someone else is mid-edit; duplicate/conflicting work starts obliviously.
3. **Draft safety is client-side only.** The advanced-editor design autosaves to localStorage per browser; a user switching devices or clearing storage loses work, and the draft is invisible to the conflict story.
4. **Conflict recovery is manual archaeology.** After a lost update, the victim must notice, find the version history, and hand-merge their text from the old version — with no help.

## 3. Target Users

- **Creators editing their templates:** need their work protected from overwrites, especially multi-tab workflows.
- **Team members (team-collaboration feature):** co-editing the same team template is the norm there; conflicts are guaranteed without protection.
- **Admins:** occasionally edit templates; must not silently clobber an owner's in-flight edit.
- **Viewers:** benefit from a subtle "being edited" signal that explains why content may be about to change.

## 4. User Stories

### 4.1 Presence awareness (advisory lease)

- **US-L1:** As a creator, when I enter edit mode, I want others to see "I'm editing" with a heartbeat, so that conflicts are prevented before they start.
- **US-L2:** As a creator, when I open edit mode on a template someone else is editing, I want to see who and for how long, so that I can decide to wait or proceed.
- **US-L3:** As a creator, I want the lease to expire automatically (e.g., after 60s without heartbeat) if I close my tab, so that a walked-away editor never blocks others.
- **US-L4:** As a viewer, I want a subtle "being edited by X" indicator on the template page, so that I understand the content may change shortly.

### 4.2 Conflict detection (optimistic concurrency)

- **US-L5:** As a creator, when I save and the template has changed since I loaded it, I want a clear conflict message — not a silent overwrite — so that my work and theirs are both preserved.
- **US-L6:** As a creator in a conflict, I want to see both versions side by side (mine vs current), so that I can merge manually and save the result.
- **US-L7:** As a creator in a conflict, I want a one-click "load current version" action, so that I can discard my stale text deliberately when I choose to.
- **US-L8:** As a creator, I want my in-progress text preserved (server-side draft) across the conflict flow, so that resolving the conflict never loses either side.

### 4.3 Drafts (server-side)

- **US-L9:** As a creator, I want my edit autosaved server-side while I work, so that a device switch or browser crash doesn't lose my draft.
- **US-L10:** As a creator, when I resume editing on another device, I want to be offered my latest draft, so that I can continue where I left off.

### 4.4 Safety

- **US-L11:** As the system, I never let a stale save create a version that silently drops another editor's changes — conflicts are always surfaced.
- **US-L12:** As the system, I keep the lease advisory (never a hard block) so that a stuck lease can always be overridden by "edit anyway".

## 5. Functional Requirements

### 5.1 Advisory edit lease (presence)

- **FR-1:** Entering edit mode shall acquire an **edit lease**: a Redis key `edit-lease:{template_id}` holding `{user_id, user_name, acquired_at, last_heartbeat}` with a 60-second TTL, renewed by a client heartbeat every 20 seconds while edit mode is active.
- **FR-2:** `GET /api/v1/templates/{id}/edit-lease` (owner/admin/team-editors only) shall return the current lease (holder name, elapsed time) or empty. The template detail page polls it every 30s while viewing (not editing) to render the "being edited" indicator (US-L4).
- **FR-3:** When a second editor opens edit mode while a live lease exists, the UI shall show a non-blocking notice: "{{name}} has been editing for {{minutes}}m — Edit anyway / Cancel". "Edit anyway" proceeds (the lease is advisory; the second editor's own lease acquisition overwrites the key, and both editors are then protected by FR-6 conflicts). "Cancel" returns to view mode.
- **FR-4:** The lease shall be released explicitly on save, cancel, or leaving edit mode (best-effort `DEL`), and implicitly by TTL expiry (60s without heartbeat). Lease operations shall never block or fail the edit flow — on Redis errors the system degrades to conflict-detection-only (FR-6 still guarantees correctness).

### 5.2 Optimistic concurrency (correctness)

- **FR-5:** `UpdateTemplateRequest` shall accept an optional `expected_version` (integer). When provided and it does not equal the template's current latest version number, the RPC shall return `Aborted` (HTTP 409) with the current latest version info — **no mutation occurs**. When omitted, behavior is unchanged (backwards compatible; the frontend always sends it after this feature ships).
- **FR-6:** On a 409 conflict, the client shall enter the **conflict resolution flow**: it fetches the current latest version, shows a side-by-side comparison (My edits vs Current — reusing the version-diff rendering from the versioning feature), and offers: **"Merge and save"** (loads the current version's text into the editor, with the user's own text preserved in the draft panel for manual copy-merge; saving then uses the new expected_version), **"Overwrite"** (saves my text as-is with the current expected_version — explicit deliberate action), and **"Cancel"** (returns to editing; draft kept).
- **FR-7:** Metadata-only edits (title/description/tags/category/visibility via the metadata editor) shall use the same `expected_version` check; a conflict there offers the same resolution flow scoped to metadata fields.

### 5.3 Server-side drafts

- **FR-8:** While editing, the client shall autosave a **server-side draft** every 15 seconds when dirty (debounced), via `PUT /api/v1/templates/{id}/draft` with `{content, metadata?, updated_at}` — stored in Redis (`draft:{template_id}:{user_id}`, TTL 7 days), one draft per user per template. The response returns the draft's timestamp for "saved" indicator display.
- **FR-9:** On entering edit mode, if a server draft exists for the user and is newer than the loaded version and differs from it, the UI shall offer: "Resume your draft from {{time}}?" — **Resume** (load draft) / **Start fresh** (discard draft). This composes with the advanced-editor's localStorage draft: the server draft wins when both exist and the server draft is newer; otherwise the localStorage prompt applies.
- **FR-10:** Drafts shall be deleted on successful save, on explicit "Start fresh", and by TTL (7 days). `DELETE /api/v1/templates/{id}/draft` is also called when the user cancels editing with unchanged-from-draft content (best-effort).
- **FR-11:** Draft endpoints shall be owner/admin/team-editor only (same authZ as edit); drafts are private to their author (a team member never sees another member's draft). Draft content is excluded from search, listings, and version history.

### 5.4 Indicators & integration

- **FR-12:** The template detail page (view mode) shall show a subtle "✏️ Edited by {name} just now / {n}m ago" indicator while a live lease exists, visible to all viewers of a public template (name only, no PII beyond display name).
- **FR-13:** The editor shall show a "Draft saved · {time}" status line (from FR-8 responses) alongside the existing localStorage autosave indicator, replacing it (server draft supersedes localStorage once this ships; localStorage remains the offline fallback).
- **FR-14:** All new endpoints shall be rate-limited (heartbeat: 30/min; draft save: 12/min; lease read: 60/min) using the existing Redis limiter infrastructure.

## 6. Non-Functional Requirements

- **NFR-1 (Correctness first):** The conflict check (FR-5) is the source of truth; the lease (FR-1) is advisory UX only. Even with Redis fully down, no silent lost update is possible once the frontend sends `expected_version`.
- **NFR-2 (Performance):** Heartbeat, lease read, and draft save are all single-key Redis operations — p99 < 50ms. The 30s view-mode poll adds negligible load (one GET per open tab per 30s).
- **NFR-3 (Backwards compatibility):** `expected_version` omitted → old behavior (no check). Existing API consumers (API keys, playground) are unaffected until they opt in.
- **NFR-4 (Privacy):** Drafts are per-user and never exposed to others; the lease exposes only the display name (already public on templates).
- **NFR-5 (Graceful degradation):** All Redis-backed pieces (lease, draft) fail open: on Redis errors, editing still works, conflicts still detected, and the UI hides the presence indicator rather than erroring.
- **NFR-6 (i18n):** All new strings in `en`/`zh` with identical keys; conflict/draft times are localized.
- **NFR-7 (Accessibility):** Conflict dialog and draft prompts follow the existing modal a11y pattern; the "being edited" indicator is `aria-live="polite"` on appearance; all conflict actions are keyboard-reachable.

## 7. Acceptance Criteria

- **AC-1:** Two tabs of the same owner edit the same template: tab A saves (v7→v8); tab B (loaded at v7) saves → gets 409 conflict, sees side-by-side (my text vs v8), and no version is created from the stale save.
- **AC-2:** In the conflict flow, "Overwrite" saves the user's text as v9 (using v8's expected_version); "Merge and save" loads v8 into the editor with the user's text available in the draft panel; "Cancel" returns to editing with the draft intact.
- **AC-3:** A second editor opening edit mode while a live lease exists sees "{{name}} has been editing for 2m" with Edit anyway / Cancel; "Edit anyway" proceeds and both editors are protected by AC-1 conflicts.
- **AC-4:** Closing the tab without saving lets the lease expire within 60s; a subsequent editor sees no presence notice.
- **AC-5:** Viewers of the template see "✏️ Edited by {name} just now" while a lease is live; it disappears within ~60s of the editor leaving.
- **AC-6:** Editing autosaves a server draft every ≤ 15s when dirty; reloading edit mode on another device offers "Resume your draft from {time}" and resumes the exact text.
- **AC-7:** Successful save deletes the server draft; "Start fresh" deletes it; drafts expire after 7 days.
- **AC-8:** A team member's draft is invisible to other members and to the owner (authZ enforced server-side; 403 for foreign draft access).
- **AC-9:** With Redis stopped: editing and saving still work; conflicts are still detected (409 on stale save); no presence indicator or draft resume is offered; no errors surface to the user.
- **AC-10:** API consumers omitting `expected_version` experience zero behavior change (no 409s).
- **AC-11:** Metadata edits conflict-check the same way (stale metadata save → 409 → resolution flow).
- **AC-12:** All new strings exist in `en` and `zh`; heartbeat/draft/lease endpoints enforce their rate limits (429 with friendly message).

## 8. Out of Scope (v1)

- Realtime collaborative editing (OT/CRDT, shared cursors) — requires WebSocket infra; revisit if the platform adds it.
- Automatic text merging (three-way merge of the user's text against the base and current versions) — v1 shows both sides; the user merges manually.
- Server-side drafts for *new* (unsaved) templates — localStorage covers creation (advanced-editor FR-6); server drafts are for existing templates only.
- Lease notifications ("X started editing your template") — presence is pull-based only.
- Locking on fork operations (forks copy; they cannot lose updates).
- Admin-forced lease breaking (the lease is advisory; "edit anyway" is always available).
- Draft diffing UI (the resume prompt compares timestamps, not content).

## 9. Dependencies & Assumptions

- **D-1:** Redis is deployed and the client wrapper (`data.NewRedisClient`) with the rate-limiter infrastructure (used by comments) is available for lease/draft/rate-limit keys.
- **D-2:** The versioning feature (shipped) provides `template_versions` with version numbers, the version list, and the unified-diff rendering that the conflict side-by-side reuses.
- **D-3:** The advanced-editor design's localStorage draft (`draft:template:{id}`) exists or will ship; FR-9 defines the composition (server draft wins when newer).
- **D-4:** `UpdateTemplate` (service) is the single mutation path for content and metadata; adding `expected_version` there covers both editors (content + metadata) on the details page.
- **D-5:** Team-collaboration (designed, not shipped) defines team-editor authZ; until it ships, lease/draft authZ is owner + admin.
- **D-6:** The frontend edit mode lives on TemplateDetails (content + metadata editors); the conflict flow is a new modal on that page.

## 10. Open Questions for Architect

1. **Conflict error shape:** `Aborted` gRPC code with a `ConflictInfo` (current_version, current_content?) in the error details vs. a dedicated `UpdateTemplateResponse.conflict` field? I recommend error details (keeps the happy path clean); confirm the gateway's error-detail pass-through.
2. **Draft storage:** Redis with 7-day TTL (proposed — simple, ephemeral, no schema change) vs. a `template_drafts` table (durable, queryable, needs migration + cleanup job)? Redis seems right for ephemeral drafts; confirm Redis memory budget (~1 KB per active draft).
3. **Lease key granularity:** per-template (proposed) vs. per-template-per-section (content vs metadata)? The details page has two editors; one lease per template is simpler and matches the mental model ("someone is editing this template").
4. **`expected_version` semantics for metadata-only saves:** same check (proposed — any mutation conflicts on any prior change) vs. skipping the check when content is unchanged? Same-check is simpler and safer; confirm.
