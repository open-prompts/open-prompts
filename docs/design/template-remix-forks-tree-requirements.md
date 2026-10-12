# Template Remix & Forks Tree — Requirements Analysis

- **Feature ID:** template-remix-forks-tree
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. Competitor Research Summary

| Platform | Fork/remix approach | Key takeaways |
|---|---|---|
| **GitHub** | Fork count on repo header; fork list page; "forked from parent" label on fork pages; fork network graph | Attribution label + fork count are the minimum viable lineage UX; the full network graph is power-user territory and rarely visited |
| **Scratch** | Famous "remix tree": every project shows "remixes" and its remix ancestry; remix source is always credited on the project page | Remix trees are the single strongest community loop in any user-generated-content platform: they make copying a *creative, credited* act rather than plagiarism |
| **Hugging Face** | Model pages show "copies"/derivations (e.g., DreamBooth community forks); "Model tree" listing fine-tuned derivatives | Derivative lists turn popular bases into hubs: browsing forks is a discovery surface, not just bookkeeping |
| **FlowGPT** | Users copy prompts freely with no attribution or fork tracking | The negative example: no lineage means no remix credit, no discovery through derivatives, and no engagement signal for creators |
| **CodePen** | Fork button on every pen; forked pens show "A fork of {pen} by {user}"; fork counts on cards | The one-line "forked from" credit directly under the title is the pattern users understand instantly |
| **Glitch** | "Remix" creates a copy; remixed apps show "Remixed from {app}" | "Remix" framing makes copying feel collaborative; the credit line is always visible |
| **PromptBase** | No fork concept; prompts are paid, copying is prohibited | Confirms forks are a community-platform feature, not a marketplace one — consistent with this project's no-monetization scope |

**Synthesis:** The winning pattern across GitHub/Scratch/CodePen is three surfaces: (1) a **fork count** as social proof on cards and detail pages, (2) a **"forked from" attribution line** on forked templates, and (3) a **forks list** on the source template for discovery. A full interactive network graph (GitHub-style) is overkill for v1; an **indented lineage tree** (ancestors chain + limited-depth descendants) delivers the value at a fraction of the complexity. Open Prompts already has the fork *action* (`ForkTemplate` copies title/description/tags/category/language/content into a new private template and notifies the owner) but records **no lineage whatsoever**: the copy is indistinguishable from an original, fork counts do not exist anywhere in the product, and creators cannot see how their work spreads.

## 2. Problem Statement

1. **No lineage record.** `ForkTemplate` creates a new template with no reference to its source. Once forked, the connection is lost forever — the platform cannot answer "where did this come from?" or "what did this spawn?".
2. **No fork social proof.** Templates carry `like_count` and `favorite_count`, but fork count — the strongest signal of a template being *useful enough to build on* — is not tracked or displayed anywhere.
3. **No remix credit.** A forked template presents someone else's title, description, and content as if authored from scratch. The original creator gets a notification but no lasting public credit, and the forker's audience cannot find the original.
4. **No derivative discovery.** Users who find a good template cannot browse its forks to discover improved or specialized variants — a proven discovery surface on Scratch and Hugging Face.
5. **No post-fork orientation.** After forking, the user gets a success toast but no link to their new template; they must navigate to their library to find it.

## 3. Target Users

- **Viewers/prospective users:** use fork counts and fork lists to judge a template's usefulness and discover variants.
- **Forkers:** want credit to flow to the original, want a quick path to their new fork, and want to see their fork's place in a lineage.
- **Creators (source owners):** want to see how far their template spreads (fork count, forks list, tree) — the deepest engagement signal available.
- **Admins:** need read-only lineage context when moderating (e.g., spam templates mass-forked from a popular original).

## 4. User Stories

### 4.1 Social proof & attribution (reading)

- **US-FK1:** As a viewer, I want to see a fork count on template cards and detail pages, so that I can gauge how many people built on a template.
- **US-FK2:** As a viewer, I want a "forked from {title} by {author}" attribution line on forked templates, so that I can visit the original and credit flows to the creator.
- **US-FK3:** As a viewer, I want to know which version of the source a fork was taken from, so that I can judge how current the fork is.
- **US-FK4:** As a viewer, I want the attribution line to gracefully handle a deleted source, so that the page never shows a broken link.

### 4.2 Forks exploration (discovery)

- **US-FK5:** As a viewer, I want a "Forks" section on a template's detail page listing its public forks, so that I can discover variants.
- **US-FK6:** As a viewer, I want a lineage tree (ancestors + descendants) for a template, so that I can understand the remix chain at a glance.
- **US-FK7:** As a viewer, I want my own private forks visible to me in the tree, so that I can find my own derivatives even while they stay private.

### 4.3 Forking (writing)

- **US-FK8:** As a forker, I want the fork confirmation to tell me the fork will be linked to its source, so that I understand attribution before confirming.
- **US-FK9:** As a forker, I want a direct "View fork" action after a successful fork, so that I can start editing immediately.
- **US-FK10:** As a forker, I want to fork a fork, so that remix chains can form naturally.

### 4.4 Creators & admins

- **US-FK11:** As a creator, I want an accurate fork count on my templates, so that I can measure real impact beyond likes.
- **US-FK12:** As a creator, I want the forks list to respect forkers' privacy (private forks stay private), so that the feature does not leak others' private work.
- **US-FK13:** As an admin, I want read-only lineage info (forked from, fork count) on the admin template detail page, so that I can moderate remix abuse with context.

### 4.5 System

- **US-FK14:** As the system, I record lineage at fork time and maintain `fork_count` transactionally, so that counts are always consistent.
- **US-FK15:** As the system, I never expose private templates of other users in fork lists or trees.
- **US-FK16:** As the system, deleting a fork decrements the source's `fork_count`; deleting a source never deletes its forks.

## 5. Functional Requirements

### 5.1 Data model

- **FR-1:** The `templates` table shall gain: `forked_from_template_id` (UUID, nullable — the source template's id, recorded at fork time), `forked_from_version` (INT, nullable — the source's version number at fork time), and `fork_count` (INT, NOT NULL, default 0 — lifetime number of forks created from this template). An index on `forked_from_template_id` shall support descendant lookups.
- **FR-2:** `forked_from_template_id` shall be a **plain UUID column without an FK constraint** (recommended): when a source template is deleted, forks keep the id and the UI renders a "deleted" tombstone node, preserving chain structure. *(The alternative — FK `ON DELETE SET NULL` — severs the chain for grandchildren; see Open Question 1.)*
- **FR-3:** `ForkTemplate` shall set `forked_from_template_id` and `forked_from_version` on the new template and increment the source's `fork_count` in the same transaction as template creation. All other fork behavior (private visibility, content copy, owner notification) is unchanged.
- **FR-4:** Deleting a template that has `forked_from_template_id` set shall decrement that source's `fork_count` transactionally. Deleting a source template shall not affect its forks (they survive with a tombstone reference).
- **FR-5:** Existing templates (forked before this feature) have NULL lineage and are treated as originals. No backfill is possible (the source link was never recorded); this is accepted and documented.

### 5.2 Reading — template detail & cards

- **FR-6:** The template detail response (`GET /api/v1/templates/{id}`) shall include `fork_count` and, when lineage exists, a `forked_from` object: `{template_id, title, version, owner: {id, display_name, avatar}, deleted: bool}`. When the source has been deleted, `forked_from` returns `{template_id, deleted: true}` with no title/owner. When the template is an original, `forked_from` is null.
- **FR-7:** The template list response (home, search, collections) shall include `fork_count` so cards can display it without extra requests.
- **FR-8:** The attribution line shall be visible on the template detail page of any forked template (public or own), linking to the source when it still exists.

### 5.3 Reading — forks list & tree

- **FR-9:** `GET /api/v1/templates/{id}/forks` shall return a paginated list (default 20, max 50) of **public** forks of the template, newest first, using the existing template list item shape (so `PromptCard` renders unchanged). Each item includes `fork_count` (a fork can itself be forked). Private forks of other users are never included; the viewer's own private forks are included (they can see their own work).
- **FR-10:** `GET /api/v1/templates/{id}/fork-tree` shall return the template's lineage, bounded: (a) **ancestors** — the full chain from the template up to its root, max 10 hops (deeper chains are cut with a `truncated: true` flag); (b) **descendants** — direct and indirect forks, breadth-first, max depth 3 and max 50 nodes (excess is flagged `truncated: true` with a count of hidden nodes). Each node: `{template_id, title, owner: {id, display_name, avatar}, fork_count, is_current, deleted}`. Visibility rules match FR-9: public templates, the viewer's own templates, and tombstones for deleted ones.
- **FR-11:** Both endpoints shall be public (anonymous readable) for public templates; for private templates, only the owner receives data (404 otherwise), matching existing template visibility semantics.
- **FR-12:** Tombstone nodes (`deleted: true`) carry no title or owner data — only the fact that a template existed at that position in the chain.

### 5.4 Forking flow (regular-user surface)

- **FR-13:** The fork confirmation modal shall state that the new template will be publicly credited as a fork of the source ("This fork will show 'forked from {title}' on its page").
- **FR-14:** On successful fork, the UI shall offer a "View fork" action (in the success toast or modal) that navigates to the newly created template, in addition to the existing success feedback.
- **FR-15:** Forking a fork shall be allowed; the new template's `forked_from_template_id` points to its immediate source (the fork), forming a chain. The fork notification goes to the immediate source's owner only (existing behavior, unchanged).

### 5.5 Admin console

- **FR-16:** The admin template detail page (`/admin/templates/:id`) shall show a read-only lineage panel: "Forked from" (title, version, owner, link to admin detail of the source when it exists) and "Fork count" with a link to the public forks list. No admin-only fork management actions in v1.

## 6. Non-Functional Requirements

- **NFR-1 (Privacy):** Private forks of other users are invisible in every surface (list, tree, counts are aggregate-only). The `fork_count` number is the sole aggregate signal; it must never be decomposable to reveal private fork identities.
- **NFR-2 (Performance):** The fork-tree endpoint is bounded by design (≤ 10 ancestor hops, ≤ 50 descendant nodes, depth ≤ 3) and shall respond within the same budget as template detail (p95 < 300 ms server-side). The forks list reuses the existing paginated list query pattern with the new index.
- **NFR-3 (Consistency):** `fork_count` uses the same transactional denormalized-counter pattern as `like_count`/`favorite_count`; no drift under concurrent forks (row-level locking or atomic `UPDATE ... SET fork_count = fork_count + 1`).
- **NFR-4 (i18n):** All new UI strings exist in both `en` and `zh` translation files with identical key sets; the attribution line, tombstone text, and tree labels are fully localized.
- **NFR-5 (Accessibility):** The lineage tree is operable and readable as nested semantic structure (headings/list, keyboard-navigable links); fork counts are text, not color- or icon-only signals.
- **NFR-6 (Resilience):** Failures of the forks list or tree never break the template detail page (section-level error + Retry, same pattern as comments). Missing/failed lineage data degrades to hiding the attribution line.
- **NFR-7 (Compatibility):** Existing fork flows, notifications, and template CRUD are unchanged except for the added lineage fields; API consumers that ignore new fields are unaffected.

## 7. Acceptance Criteria

- **AC-1:** Forking a public template creates a new private template whose detail page shows "⑂ Forked from {source title} by {source owner} · v{N}" with a working link to the source; the source's `fork_count` increments by 1.
- **AC-2:** The source template's detail page shows a "Forks (N)" section listing public forks as cards; the viewer's own private forks appear in their own view; other users' private forks never appear.
- **AC-3:** The lineage tree shows the ancestor chain to the root and descendants to depth 3 / 50 nodes; deeper or wider structures show a "showing partial tree" notice with the hidden count.
- **AC-4:** Forking a fork produces a three-node chain (A → B → C); C's tree shows both ancestors; B's tree shows C as a descendant.
- **AC-5:** Deleting the source template leaves the fork intact; the fork's attribution line and tree render a "deleted" tombstone instead of a link; no 404 or broken UI.
- **AC-6:** Deleting a fork decrements the source's `fork_count`; deleting a source does not delete or alter its forks.
- **AC-7:** Template cards show a fork count chip (⑂ N) when N > 0; cards with 0 forks show nothing (no visual noise).
- **AC-8:** The fork modal explains attribution before confirming; after a successful fork, "View fork" navigates to the new template.
- **AC-9:** Anonymous visitors can read fork counts, the forks list, and the tree of public templates; private templates' fork data is owner-only (404 for others, matching existing visibility).
- **AC-10:** `/admin/templates/:id` shows the read-only lineage panel (forked from + fork count) with correct data.
- **AC-11:** Templates forked before this feature (NULL lineage) render as originals — no attribution line, no errors; their `fork_count` starts at 0.
- **AC-12:** With the forks/tree API failing, the template detail page renders fully with inline error + Retry in the forks section; the attribution line is simply absent if its data is missing.
- **AC-13:** All new strings exist in both `en` and `zh` with identical keys; the tree, attribution, and tombstone render correctly in both languages and both themes.

## 8. Out of Scope (v1)

- Interactive network graph visualization (D3/graph canvas) — indented tree only.
- Upstream sync / "pull changes from source" (GitHub-style merge) — forks are independent copies.
- "Source updated" notifications for forkers.
- Fork-of-fork notifications to the original root owner (only the direct source owner is notified, per existing behavior).
- Fork reasons/notes ("why I forked").
- Re-parenting or editing lineage after the fact.
- Showing private forks to anyone other than their owner (including admins in the public tree; admins use the admin console's own views).
- Fork search/filtering within the forks list (standard pagination only).
- Fork data in the JSON export format (the import-export feature's format v1 is frozen; a future format version may add it).
- API-key access to forks/tree endpoints (regular web UI only in v1).
- Any monetization-adjacent features (per project scope rule).

## 9. Dependencies & Assumptions

- **D-1:** `templates` already carries denormalized `like_count`/`favorite_count` maintained transactionally in repositories; `fork_count` follows the identical pattern (confirmed in `deployment/init.sql` and `template_repository.go`).
- **D-2:** `ForkTemplate` in `backend/internal/service/prompt.go` is the single fork entry point (copies metadata + latest version content, always private, notifies owner) — lineage recording is added there; no other code path creates forks.
- **D-3:** The REST gateway routes `/templates/{id}/fork` via suffix matching in `cmd/server/main.go`; new sub-resource routes (`/forks`, `/fork-tree`) follow the same pattern as `/templates/{id}/likes`.
- **D-4:** `PromptCard` already renders `like_count`/`favorite_count` from the list response; adding `fork_count` follows the same prop path. `TemplateDetails.js` already has the fork modal and action button.
- **D-5:** Owner display names/avatars in lineage data: the follow-creators feature introduces public profile data (`GET /api/v1/users/{userId}/profile`). If it has not shipped, the fork endpoints must embed a minimal public owner object (`id`, `display_name`, `avatar`) via join — architect to confirm the shared approach so template detail, forks list, and tree use one consistent owner shape.
- **D-6:** The template-comments feature establishes the section-below-content pattern on TemplateDetails (independent fetch, skeleton, error + Retry); the forks section reuses it.
- **D-7:** Fork notifications already exist (`type='fork'` in `notifications`); no notification changes are required.

## 10. Open Questions for Architect

1. **Lineage reference integrity:** Recommendation is a plain UUID column (no FK) + tombstones, preserving chains when sources are deleted. Alternative: FK `ON DELETE SET NULL` (simpler integrity, loses grandparent chains). Which do you prefer?
2. **`fork_count` semantics:** Recommendation: count ALL forks (including private) — it is the owner's engagement metric and matches the existing fork notification (owner is notified for every fork, private or not). The list/tree then show only public forks, with UI copy explaining "showing public forks". Alternative: count public forks only (number always equals list total, but the metric understates engagement). Confirm choice.
3. **Tree bounds:** depth 3 / 50 nodes / 10 ancestor hops are my proposals for v1. If templates rarely fork deeply, these bounds cost nothing; confirm or adjust.
4. **Owner object shape (D-5):** embed minimal owner in fork endpoints now, or wait for follow-creators' profile endpoint and link by user id only in v1?
