# Prompt Chaining — Requirements Analysis

- **Feature ID:** prompt-chaining
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. Competitor Research Summary

| Platform | Chaining / composition approach | Key takeaways |
|---|---|---|
| **FlowGPT "Flows"** | Multi-step prompt chains: output of one prompt feeds the next; visual step list; run sequentially in the UI | Sequential piping is the core mental model; a visual step list (not a graph canvas) is enough for v1 |
| **AIPRM "Prompt Chains"** | Predefined chains for workflows (e.g. research → outline → draft); each step is a template with variables; run produces a combined transcript | Packaging a chain as a shareable artifact (like a template) creates a second layer of reusable content |
| **Snack Prompt** | No chaining; single prompts with variables | Single-prompt platforms demonstrate the gap chains fill for multi-step work |
| **PromptBase** | No chaining; marketplace of single prompts | Confirms chaining is a workflow feature, not a marketplace feature |
| **Zapier / n8n** | Node graphs with mappings; complex but powerful | Graph canvases are powerful but heavy — linear chains with optional branching cover most real workflows and are far more approachable |
| **LangChain (library)** | Chains as code: prompt → parser → prompt; output variables passed by name | Name-based output→input mapping is the developer-familiar contract; we mirror it visually |
| **ChatGPT "GPTs" (custom instructions)** | Single system prompt + files; no chaining | Even the largest platform keeps composition out of the product — an opportunity, not a pattern to copy |

**Synthesis:** The winning v1 pattern is the **linear chain**: a chain is a named, ordered list of steps, where each step references an existing template (by version/alias), declares which variables it consumes, and maps the *output* of a previous step into one of its variables. Outputs are user-declared: each step marks one variable as its "output slot" (the value the user fills for that variable becomes the step's result). Running a chain in the playground-like runner fills step 1's inputs, produces its output, which auto-fills the mapped variable in step 2, and so on; the user copies each step's result. Chains are first-class content: private by default, publishable like templates and collections, forkable, and usable through the same detail-page patterns. **LLM execution is explicitly out of scope** — a chain run resolves and assembles prompt text step by step; it never calls a model. This keeps chaining a pure composition layer over existing template infrastructure.

## 2. Problem Statement

1. **Real workflows are multi-step.** "Summarize this article → extract action items → draft a reply" is three templates today. Users must manually copy output from one generator into the next generator's variable — error-prone and tedious.
2. **Composition knowledge lives nowhere.** A user who discovers that template A's output feeds template B's input has no way to save or share that recipe. Every session starts from memory.
3. **The product's unit is too small.** Templates are single prompts; the value users get from prompt platforms increasingly comes from *sequences*. Collections group templates but do not connect them.
4. **No structured handoff.** Variable names are the natural contract between templates (`$$article_text$$` → `$$input_text$$`), but nothing in the product expresses "this output goes there".

## 3. Target Users

- **Power users / workflow builders (primary):** users with recurring multi-step text workflows who want to encode them once and reuse forever.
- **Creators:** authors of complementary templates who want to publish not just templates but *recipes* combining them (their own and others').
- **Regular users:** consumers who run published chains to get multi-step results without understanding the wiring.
- **Admins:** moderate chains as content in the admin console (same pattern as templates: visibility, takedown) — no chain-specific admin tooling in v1.

## 4. User Stories

- **US-H1:** As a power user, I want to create a chain by picking templates step by step, so that my multi-step workflow is encoded once.
- **US-H2:** As a power user, I want to mark one variable per step as its output, so that downstream steps know what to consume.
- **US-H3:** As a power user, I want to map a previous step's output into a later step's variable, so that data flows automatically when I run the chain.
- **US-H4:** As a power user, I want to run a chain and fill only the *unmapped* variables, so that each run needs just my original input.
- **US-H5:** As a power user, I want to see each step's resolved prompt and copy it, so that I can use the intermediate results in my own tools.
- **US-H6:** As a creator, I want to publish my chain with a stable URL, so that others can run my recipe.
- **US-H7:** As a regular user, I want to run a published chain without editing it, so that I benefit from the composition without understanding it.
- **US-H8:** As a user, I want to fork a published chain into my own editable copy, so that I can adapt recipes to my needs.
- **US-H9:** As an admin, I want chains to appear in template moderation with the same visibility controls, so that inappropriate chains can be unpublished.

## 5. Functional Requirements

### 5.1 Data model

- **FR-1:** New table `prompt_chains`: `id UUID PK`, `owner_id TEXT NOT NULL` (FK users), `name VARCHAR(120) NOT NULL`, `description TEXT` (0–1000 chars), `visibility VARCHAR(10) NOT NULL DEFAULT 'private' CHECK (visibility IN ('private','public'))`, `created_at`, `updated_at`. Index `(owner_id)`, `(visibility, updated_at DESC)`.
- **FR-2:** New table `prompt_chain_steps`: `id UUID PK`, `chain_id UUID NOT NULL REFERENCES prompt_chains(id) ON DELETE CASCADE`, `position INT NOT NULL`, `template_id UUID NOT NULL REFERENCES templates(id)`, `version_id INT` (nullable; null = latest at run time), `output_variable VARCHAR(50)` (nullable; the step's declared output slot), `created_at`. Unique `(chain_id, position)`. Deleting a referenced template does not delete the step (see FR-16).
- **FR-3:** New table `prompt_chain_mappings`: `id UUID PK`, `step_id UUID NOT NULL REFERENCES prompt_chain_steps(id) ON DELETE CASCADE`, `variable_name VARCHAR(50) NOT NULL`, `source_step_position INT NOT NULL` (references an earlier step in the same chain), Unique `(step_id, variable_name)`. A mapping says: this step's `variable_name` is filled with the output of the step at `source_step_position`.
- **FR-4:** Limits: a user may own at most 20 chains; a chain has 2–10 steps; each step has at most 5 mappings. Server-enforced with friendly errors.

### 5.2 Chain building (regular-user surface)

- **FR-5:** Authenticated users create chains at `/chains/new` (or a create modal): name, description, then a step editor. Steps are added sequentially; each step picks a template (searchable picker, own + public templates), optionally pins a version (default: latest), and optionally marks an output variable (dropdown of the template's variables).
- **FR-6:** For each step, the builder lists the template's variables; for each variable the user chooses: leave as *run-time input*, or *map from a previous step's output* (dropdown of earlier steps that have an output variable declared).
- **FR-7:** Validation (builder, client + server): at least 2 steps; positions unique; mappings only reference earlier steps; mapped variables must exist in the step's template; a step's output variable must exist in its template; circular references impossible by construction (earlier-only rule).
- **FR-8:** Chains are saved as drafts implicitly (private) and can be published (visibility public) from the chain detail page — same model as collections.

### 5.3 Chain detail & running

- **FR-9:** Chain detail page `/chains/{id}` shows: header (name, owner, visibility badge, stats), step list (cards in order: template title, version/alias, variables with input/mapped badges, output variable badge), description, and the **Run panel**.
- **FR-10:** The Run panel collects run-time inputs: only variables not mapped in any step (the chain's "root inputs"). Each root input renders a textarea (same generator styling). One "Run chain" button.
- **FR-11:** Running a chain resolves steps sequentially client-side: for each step, fetch the template version content (existing APIs), substitute mapped variables with previous steps' outputs and root inputs with user values, render the resolved prompt, and present it as the step's result with a copy button. The declared output variable's *user-visible value* for that step is the text the user enters/pastes into that variable slot in the result panel (see FR-12).
- **FR-12:** **Output capture:** after each step's resolved prompt is shown, the run panel displays the step's output variable name with an empty textarea: the user pastes/types the result they obtained (from their own model usage outside the product), which then feeds mapped downstream variables. A "Skip" option leaves the output empty (downstream mapped variables receive empty strings). This keeps the product LLM-free while making data flow concrete.
- **FR-13:** Run state is client-side only (no server persistence of runs in v1); a run can be reset; steps after a skipped/empty output still run with empty values (consistent with the generator's empty-string behavior).
- **FR-14:** Fork: any user can fork a public chain (or their own) into a private copy (new id, owner = forker, "Forked from {chain}" note in description). Forking is one click from the chain detail page.

### 5.4 Discovery

- **FR-15:** Public chains appear in a "Chains" section: a `/chains` browse page (grid of chain cards: name, step count, owner, updated) with search by name/description and sort by newest/most forked. Chain cards appear on the owner's profile (future creator page from follow-creators) — link-only in v1.

### 5.5 Deletions & integrity

- **FR-16:** Deleting a template referenced by a chain step does not delete the step; the step renders a "template unavailable" state in the builder and detail page, and the chain cannot run until the owner replaces or removes the step. This preserves chains against template churn.
- **FR-17:** Deleting a chain soft-deletes it (owner-only, confirmation modal); forked copies are unaffected. Admin unpublish sets visibility private (not delete).

### 5.6 Admin surface

- **FR-18:** Chains appear in the admin console template moderation list as a content type filter (`type=chain`): columns name, owner, steps, visibility, updated; actions: view (read-only detail), unpublish (visibility → private, reason recorded in audit log, same modal as template moderation). No chain editing by admins.
- **FR-19:** Admin APIs extend the existing moderation endpoints: `GET /api/v1/admin/chains?query=&visibility=&page=`, `GET /api/v1/admin/chains/{id}`, `POST /api/v1/admin/chains/{id}/unpublish` (body: reason). Audit log entries use action `chain.unpublish`.

### 5.7 API (regular-user)

- **FR-20:** `POST /api/v1/chains` (create with steps+mappings), `GET /api/v1/chains?owner=me|public&query=&sort=&page=`, `GET /api/v1/chains/{id}` (detail incl. steps with template summaries), `PUT /api/v1/chains/{id}` (update name/description/visibility/steps/mappings), `DELETE /api/v1/chains/{id}`, `POST /api/v1/chains/{id}/fork`. All owner-scoped where applicable; public reads for published chains.

### 5.8 Notifications integration

- **FR-21:** If the notifications feature has shipped, a `chain_fork` notification event is sent to the chain owner when someone forks their public chain (actor, chain name, link). If notifications has not shipped, this FR is deferred without blocking the feature.

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Chain detail + run panel loads < 1 s for 10 steps (parallel template version fetches); step resolution is client-side and instant.
- **NFR-2 (Integrity):** All FR-7 validations enforced server-side on create/update (client-side is UX only); mappings referencing missing variables are rejected with field-level errors.
- **NFR-3 (Limits):** Server-enforced limits (FR-4) return 400 with clear messages; the builder shows live counts against limits.
- **NFR-4 (i18n):** All strings EN/ZH via i18next (`chains.*` namespace); variable names and template titles are user content, never translated.
- **NFR-5 (Accessibility):** Step list and run panel fully keyboard operable; step order and data flow conveyed by text (numbered steps, "output of step 2 → variable X of step 3"), not color alone.
- **NFR-6 (Consistency):** Chain visibility, publishing, forking, and moderation reuse the exact patterns of templates and collections (same badges, modals, audit reasons).

## 7. Acceptance Criteria

- **AC-1:** A user creates a 3-step chain (each step a template with variables), marks step 1's `summary` as output, maps step 2's `input_text` ← step 1 output, and saves; the chain appears in My Chains as private.
- **AC-2:** Publishing the chain makes it browsable at `/chains/{id}` by other users; the browse page lists it.
- **AC-3:** Running the chain shows only root inputs (unmapped variables); after filling step 1's inputs, its resolved prompt renders with a copy button and an output capture box for `summary`.
- **AC-4:** Pasting text into step 1's output box auto-fills step 2's `input_text` display; step 2's resolved prompt contains the pasted text.
- **AC-5:** A chain with a mapping to a non-existent variable is rejected on save (400, field-level error).
- **AC-6:** A chain referencing a deleted template shows "template unavailable" on the step and cannot run until fixed.
- **AC-7:** Forking a public chain creates a private copy under the forker; the original is unaffected; the fork note appears in the copy's description.
- **AC-8:** Admin can filter moderation by chains, view one, and unpublish it with a reason; the action appears in the audit log as `chain.unpublish`; the chain becomes private.
- **AC-9:** Limits enforced: an 11-step chain is rejected; a 21st chain is rejected; both with friendly errors.
- **AC-10:** All strings translated EN/ZH; no hardcoded copy.
- **AC-11:** No monetization content anywhere in the feature; no LLM execution anywhere in the feature.

## 8. Out of Scope

- **LLM execution:** chains resolve and assemble prompt text only; the product never calls a model provider. Output capture is manual paste (FR-12).
- Branching/conditional logic, parallel steps, loops, graph canvases (v1 is strictly linear).
- Server-persisted runs, run history, scheduled runs, sharing run results.
- Chain versioning (chains are mutable; template version pinning covers reproducibility).
- Collaborative chain editing; chain embedding/API execution of whole chains (single-template API remains the API surface).
- Monetization of any kind (payments, pricing, purchases, earnings) — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1:** Templates, versions, and aliases (existing + template-versioning feature) provide step content; the chain references templates by id and optional version id.
- **D-2:** The collections feature's visibility/publish model and the follow-creators feature's owner identity block are the patterns to mirror (badges, publish action, profile links).
- **D-3:** The template detail page's generator (variable inputs, `$$var$$` substitution, copy) is reused as the run panel's step renderer.
- **D-4:** The admin console's moderation list + reason modal + audit log (admin-user-management feature) extend naturally to chains (FR-18/19).
- **D-5:** The notifications feature's event mechanism (if shipped) carries `chain_fork` (FR-21).
- **D-6:** The API playground feature's pattern (fetch + resolve client-side) informs the run panel's implementation but is not a dependency.

## 10. Open Questions for Architect

1. **Step content snapshotting:** Should steps pin `version_id` by default (reproducible chains, but breaks when the pinned version is deleted) or default to latest (always current, but chains silently change)? Recommendation: default latest, allow explicit pinning — confirm.
2. **Mapping semantics:** Is "earlier steps only" (FR-7) acceptable as the v1 dataflow rule (no same-step or forward references), keeping the model a DAG-free linear pipeline?
3. **Template deletion:** FR-16 keeps steps with dangling template references. Should the backend additionally block *hard* deletion of templates referenced by public chains (soft-delete instead), or is the "unavailable step" state sufficient?
4. **Fork counting:** Should `prompt_chains` carry a denormalized `fork_count` (like `like_count`) for the most-forked sort, or is a COUNT query acceptable at v1 scale?
5. **API shape:** One `PUT /chains/{id}` replacing steps+mappings wholesale (simpler, matches FR-20) vs. granular step endpoints — confirm the wholesale replace is acceptable.
6. **Notifications:** Confirm `chain_fork` as a new event type alongside like/favorite/fork/comment/follow, or defer to a v2 of the notifications event enum.
