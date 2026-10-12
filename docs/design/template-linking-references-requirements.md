# Template Linking & References — Requirements Analysis

- **Feature ID:** template-linking-references
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12
- **Related docs:** [UI/UX design](./template-linking-references-ui-ux-design.md) · [中文需求](./template-linking-references-requirements-zh-CN.md)

## 1. Competitor Research Summary

| Platform | Reference/linking approach | Key takeaways |
|---|---|---|
| **GitHub (README + "Used by")** | Authors link related repos in READMEs; GitHub auto-computes "Used by" backlinks (dependency graph) | Author-declared links carry intent ("why this is related"); auto-computed backlinks create the discovery loop in the other direction |
| **Stack Overflow ("Linked" / "Related")** | "Linked" = author/community-declared duplicates and companions (left sidebar); "Related" = algorithmic (right sidebar) | The two are visually and semantically separated — declared links are curated and few; algorithmic ones are many and ranked. Both coexist without confusion |
| **Wikipedia ("See also" + "What links here")** | "See also" section curated by editors with occasional one-line justifications; "What links here" is a tools-page backlink view | A short optional note per link ("for the streaming variant") is the highest-value annotation; backlinks live behind a secondary surface, not inline |
| **PromptBase / FlowGPT / Snack Prompt** | No author-declared cross-links; only algorithmic "similar prompts" rails | Prompt platforms haven't built this — the Wikipedia/Stack Overflow pattern is the model to import |
| **npm (dependencies)** | Package authors declare dependencies; consumers traverse the graph | Declared edges become a navigation graph users actually traverse — each edge is a discovery session |

**Synthesis:** Open Prompts has two connection types today — algorithmic similarity (related-templates design: tag/model-based rails) and fork lineage (template-remix-forks-tree design: system-recorded ancestry). What's missing is the **author-declared edge**: "while you're here, also look at these templates I picked." This is the Wikipedia "See also" pattern: the author knows best which templates complement theirs (a "system prompt" linking its matching "user prompt", a v2 linking the v1 it supersedes, a chain-friendly template linking its siblings). The design: a `template_references` table (source, target, optional 200-char note, position), owner-managed via a picker modal on the detail page, rendered as a curated "References" section — visually distinct from the algorithmic related rail — plus a "Referenced by" backlink section (public sources only, privacy-safe). A lightweight `GET /templates/lookup?q=` endpoint powers the picker without depending on the unshipped search-filtering feature. A best-effort `reference` notification tells creators when their template is cited, following the existing engagement-notification pattern.

## 2. Problem Statement

1. **Authors cannot point anywhere.** A template page is a dead end: the author cannot say "this pairs with X" or "this supersedes my old Y". The only cross-navigation is the algorithmic rail, which the author does not control.
2. **Complementary templates are scattered.** Matching system/user prompt pairs, multi-part series, and alternative variants exist as unrelated rows in the listing; users must search (once search ships) or scroll category pages to assemble a working set.
3. **The algorithmic rail can't express intent.** related-templates ranks by shared tags/models; it cannot know that template B is the *predecessor* of A or the *companion* piece. Only the author can declare that, and a short note explains why.
4. **Cited creators never find out.** When someone builds on or references a template, its owner gets no signal — unlike likes/favorites/forks, which all notify.
5. **No backlink discovery.** Even if links existed one-way, the target's audience cannot discover "who points here" — the reverse edge is where serendipitous discovery happens (GitHub "Used by", Wikipedia "What links here").

## 3. Target Users

- **Template authors:** curate references to guide users to complementary work and series; want a fast picker with search.
- **Viewers/integrators:** traverse author-curated links to assemble complete prompt sets; trust declared links more than algorithmic ones.
- **Creators being referenced:** learn when their work is cited; gain a discovery channel through backlinks.
- **Admins:** no dedicated surface needed (references are user content; moderation flows apply if content-reporting extends to them — out of scope here).

## 4. User Stories

- **US-1:** As an author, I want to link related templates on my template's page with an optional one-line note, so that users can find the companion pieces I intended.
- **US-2:** As an author, I want a searchable picker (by title) listing public templates and my own private ones, so that adding a reference takes seconds.
- **US-3:** As an author, I want to reorder and remove references, so that the section stays curated as my set evolves.
- **US-4:** As an author, I want a hard cap (10) enforced with a clear message, so that the section never becomes a link dump.
- **US-5:** As a viewer, I want references shown as cards with title, owner, note, and stats, so that I can judge whether to follow each link.
- **US-6:** As a viewer, I want the author-curated section visually distinct from the algorithmic "Related" rail, so that I understand who chose the links and why.
- **US-7:** As a creator, I want a "Referenced by" section on my template, so that I can see who cites my work and gain traffic from it.
- **US-8:** As a creator, I want a notification when someone references my public template, so that I discover citations without checking backlinks manually.
- **US-9:** As a user, I want private templates never to leak through references (neither as targets to others nor as backlink sources), so that visibility guarantees hold.
- **US-10:** As a user on mobile, I want the references section and picker to work in a single column, so that curation is possible from any device.
- **US-11:** As a user, I want reference links to never 404 (deleted targets disappear cleanly), so that the section is always trustworthy.
- **US-12:** As a non-English user, I want the entire references experience in my language (EN/ZH), so that curation and browsing are first-class in both.

## 5. Functional Requirements

### 5.1 Data model

- **FR-1:** New table `template_references`: `id UUID PK DEFAULT uuid_generate_v4()`, `source_template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE`, `target_template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE`, `note VARCHAR(200)` (nullable), `position INT NOT NULL DEFAULT 0`, `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`. Constraints: `UNIQUE(source_template_id, target_template_id)`, `CHECK (source_template_id <> target_template_id)`, `CHECK (char_length(note) <= 200)`. Indexes: `(source_template_id, position)` for the outbound list, `(target_template_id)` for backlinks.

### 5.2 Management API (owner of source)

- **FR-2:** `POST /api/v1/templates/{id}/references` — owner-only (403 for others, 401 anonymous). Body `{target_id: UUID, note?: string}`. Validations: target exists (404 `template_not_found`); target is public **or** owned by the same user (404 for others' private — never 403, to avoid existence leaks); no self-reference (400 `self_reference`); no duplicate pair (409 `duplicate_reference`); source has < 10 references (400 `reference_limit_reached`); note ≤ 200 chars (400). Returns the created reference with target summary.
- **FR-3:** `DELETE /api/v1/templates/{id}/references/{referenceId}` — owner-only; 404 if the reference does not belong to this source. Idempotent-ish (404 on missing).
- **FR-4:** `PATCH /api/v1/templates/{id}/references/order` — owner-only. Body `{order: [referenceId, …]}` containing exactly the source's current reference IDs in the desired sequence; server reassigns `position` 0..n atomically in one transaction. 400 on mismatched ID sets.

### 5.3 Read API

- **FR-5:** `GET /api/v1/templates/{id}/references` — visible when the template is public (anyone, including anonymous) or the caller is its owner. Returns ordered references, each with `id`, `note`, `position`, `created_at`, and a target summary (`target_id`, `title`, `owner_id`, `owner_display_name`, `like_count`, `favorite_count`, `fork_count`, `visibility` — visibility included so the owner can see their own private targets flagged). Private template + non-owner → 404 (matching template-detail behavior).
- **FR-6:** `GET /api/v1/templates/{id}/references/inbound` — backlinks: references whose target is this template, **from public sources only** (a private source's outbound references are its owner's business; they never appear as backlinks to anyone, including the target's owner). Each entry: source summary (id, title, owner display name, like_count) + note. Visible under the same rules as FR-5.
- **FR-7:** `GET /api/v1/templates/lookup?q=&limit=` — picker search. Auth optional. Matches `q` (min 1 char) case-insensitively as a substring of `title`; returns up to `limit` (default 10, max 20) templates that are public **or** owned by the caller, ordered by `like_count DESC, title ASC`; excludes the template currently being edited (via `exclude_id` param) and templates whose owner blocked the caller / is blocked by the caller (block-users rules). Response items: `id`, `title`, `owner_display_name`, `visibility`, `like_count`, `fork_count`.

### 5.4 UI

- **FR-8:** Template detail page gains a **References** section (below the prompt generator, above aliases): ordered cards (title, owner, optional note in muted text, like/fork counts, link to `/templates/{target_id}`); max 10. Non-owners see the section only when ≥ 1 reference exists; the owner always sees it with an "Add reference" button, per-card remove (×) and up/down reorder arrows, and an empty state ("No references yet — link related templates").
- **FR-9:** Template detail page gains a **Referenced by** section (below References, collapsed by default with a count): backlink cards (source title, owner, note) linking to the source. Hidden entirely when empty. Not editable from this surface.
- **FR-10:** **Reference picker modal** (owner, from "Add reference"): search input (debounced 300 ms, min 1 char) hitting `/templates/lookup`; result rows (title, owner, visibility badge for the owner's own private hits, like/fork counts) with an "Add" action; adding prompts for an optional note (inline field, 200-char counter) and immediately POSTs (per-action persistence, matching the alias-manager pattern); the current list is shown at the top with remove/reorder; "Done" closes. Errors (limit reached, duplicate) surface inline in the modal.
- **FR-11:** **Reference notification:** when a reference is created whose target is a public template owned by a different user, emit a notification of new type `reference` to the target owner (best-effort; no_self; suppressed when either party blocks the other; suppressed when the recipient's notification preferences disable `reference`). The notification deep-links to the *source* template.
- **FR-12:** **Deletion semantics:** deleting a target template removes its inbound reference rows via CASCADE (the source's section simply shrinks — no dead cards); deleting a source removes its outbound rows; forking a template does **not** copy its references (forks start clean, matching current `ForkTemplate` behavior).

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** References and backlinks render with the detail page's existing load characteristics: two indexed queries (FR-1 indexes), ≤ 10 rows each, joined against templates/users for summaries. Lookup is a single indexed `ILIKE` on title with LIMIT — p99 < 50 ms at v1 scale.
- **NFR-2 (Privacy/security):** No existence leaks: others' private templates are indistinguishable from nonexistent ones (404, not 403) in FR-2; backlinks expose only public sources (FR-6); lookup returns only public + own (FR-7); private templates' reference lists are owner-only (FR-5). All management routes re-verify ownership server-side.
- **NFR-3 (Consistency):** Reference cards reuse the existing template-card visual language (PromptCard-derived compact row); the picker reuses the alias-manager interaction pattern (search → add → immediate persist); notifications reuse the engagement-notification pipeline.
- **NFR-4 (Limits):** Hard caps enforced server-side: 10 references per source, 200-char notes, lookup limit ≤ 20. The UI mirrors the caps (disabled Add at 10, live note counter) so users never hit a server error by surprise.
- **NFR-5 (Accessibility):** The references list is a semantic list of links; reorder arrows and remove buttons are keyboard-operable with discernible names; the picker is a labeled search combobox (results announced); the collapsed "Referenced by" is a proper disclosure button with `aria-expanded`.
- **NFR-6 (i18n):** All strings (section titles, empty states, picker, errors, notification text) ship EN/ZH via react-i18next; user-authored notes are stored as-is and never machine-translated.

## 7. Acceptance Criteria

- **AC-1:** Owner adds a reference to a public template with note "Companion user prompt" → 201; the References section shows the card with the note; reload preserves order.
- **AC-2:** Non-owner POST/DELETE/PATCH on a template's references → 403; anonymous → 401; no state changes.
- **AC-3:** Referencing another user's private template → 404 `template_not_found` (identical response to a nonexistent ID); the reference is not created; nothing about the private template is revealed.
- **AC-4:** Self-reference → 400 `self_reference`; duplicate pair → 409 `duplicate_reference`; both surface inline messages in the picker.
- **AC-5:** Adding an 11th reference → 400 `reference_limit_reached`; the picker's Add buttons disable at 10 with the limit message shown.
- **AC-6:** Anonymous GET references on a public template → full ordered list with target summaries; on a private template (non-owner) → 404.
- **AC-7:** Public A references B → B's "Referenced by" shows A. Private C references B → B's backlinks show nothing from C (to anyone, including B's owner and admins via this surface).
- **AC-8:** Lookup with `q=sql` returns public templates (and the caller's private ones) whose titles contain "sql" case-insensitively, excludes `exclude_id`, respects limit, and hides templates from blocked/blocking owners.
- **AC-9:** Reordering with the arrows persists (PATCH order); positions are 0..n-1 with no gaps after any removal.
- **AC-10:** Creating a reference to another user's public template emits a `reference` notification to that owner, deep-linking to the source; self-references emit nothing; blocked pairs and preference-disabled recipients emit nothing.
- **AC-11:** Deleting a referenced target removes its cards from all referencing sections (no 404 links); deleting a source removes its outbound references and its backlink contributions.
- **AC-12:** All UI strings render in EN and ZH; the picker, empty states, error messages, and notification text are fully translated; notes display verbatim.

## 8. Out of Scope (v1)

- Reference *types* (e.g. "supersedes", "requires", "variant-of") — free-text notes only.
- References to non-template entities (collections, chains, users, external URLs).
- Editing notes after creation (remove + re-add achieves the same; revisit if requested).
- Fork-time reference copying or fork-aware "references inherited" badges.
- Reference analytics (click-through counts).
- Algorithmic suggestions inside the picker ("you might also link…").
- Moderation/reporting of reference notes (content-reporting extension — separate feature).
- Bulk reference management (bulk-operations extension).

## 9. Dependencies & Assumptions

- **D-1:** `templates` table and detail page exist as-is; the section slots into the existing detail layout (below prompt generator, above the owner-only alias section).
- **D-2:** The notifications `type` CHECK is widened additively (as done for `comment` and planned for `comment_reply`/`mention`) to include `reference`; notification-preferences gains a `reference` column (its design specifies a data-driven type list for exactly this extension).
- **D-3:** block-users FR-7/FR-9 semantics (no interactions with blocked users' content, bidirectional notification suppression) apply to lookup results and reference notifications.
- **D-4:** The picker's lookup endpoint is self-contained (title ILIKE) and deliberately does **not** depend on the unshipped search-filtering feature (which targets the main listing with content matching); if search-filtering ships first, lookup can delegate to it.
- **D-5:** The alias manager (TemplateAliasManager) establishes the owner-only, per-action-persist management pattern the picker follows; PromptCard establishes the card visual language.
- **D-6:** related-templates (algorithmic rail) and template-remix-forks-tree (lineage) remain separate sections; this feature's section is labeled and styled as author-curated to avoid conflation (see §10).

## 10. Open Questions for Architect

1. **Section coexistence:** detail page will have References (curated), Referenced by (backlinks), Related (algorithmic), and Forks (lineage) — is that four sections too many? Options: tabs under one "Connections" heading, or curating the algorithmic rail to merge with author picks (curated first, algorithmic after, visually marked)?
2. **Notification type widening:** add `reference` to the CHECK in the same migration as `comment_reply`/`mention` (one widening) or separately per feature?
3. **Backlink privacy:** should a private source's reference be visible to the *target's* owner as a muted "private source" row (GitHub shows private forks to the parent owner), or fully hidden as designed (Wikipedia model)? Current design: fully hidden.
4. **Lookup ranking:** like_count DESC favors popular templates; should recency or exact-prefix match rank first instead?
5. **Note editability:** is remove-and-re-add acceptable for v1, or should PATCH on a reference (note text) be included now (cheap to add)?
6. **Position semantics:** dense 0..n-1 renumbering on every removal (as designed) vs append-only positions with gaps (fewer writes)?
7. **Should the reference notification include the note text** in its payload (it's author-authored and ≤ 200 chars) or just the fact of citation?
