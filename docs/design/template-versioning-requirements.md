# Template Versioning UX — Requirements Analysis

- **Feature ID:** template-versioning
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-10

## 1. Competitor Research Summary

| Platform | Versioning approach | Key takeaways |
|---|---|---|
| **GitHub/GitLab** | Full history with commit messages; unified/side-by-side diff; revert creates a *new* forward commit; per-file history | Change notes are the #1 usability factor; diff-before-save prevents accidents; revert is non-destructive (forward-only) |
| **Notion** | Version history drawer: timestamped snapshots, restore any snapshot, no messages | Restore-without-notes works but users cannot tell versions apart beyond dates |
| **Google Docs** | Named versions + auto versions; restore; diff via "compare" | Optional naming/notes on top of auto-numbering is the sweet spot |
| **npm / Docker Hub** | Version tags with release notes; "latest" pointer | Release notes make version lists scannable; a clear "latest" indicator matters |
| **PromptBase** | No versioning — single editable content | A gap; creators edit blind with no safety net |
| **FlowGPT** | Simple edit history (timestamps only) | Timestamps-only is the minimum viable versioning — exactly where Open Prompts is today |
| **AIPRM / Snack Prompt** | No meaningful versioning | Not a pattern to copy |

**Synthesis:** The winning pattern for content platforms is: auto-numbered versions (already present) + **optional change notes** (the single highest-value addition), a **rich history list** (not just a dropdown), **diff between any two versions**, and **non-destructive restore** (restoring v3 creates v7 with v3's content — never deletes intermediate versions). Open Prompts already has the hard part (server-side version creation on content change, `UNIQUE(template_id, version)`, prompts pinned to `version_id`); what is missing is entirely the UX layer plus one nullable column.

## 2. Problem Statement

1. **Versions are indistinguishable.** The version dropdown shows only "v3 (6/20/2026)". A user with 8 versions cannot tell which one introduced a tone change, a fix, or a regression — dates alone carry no intent.
2. **No way to see what changed.** To compare v3 and v4 a user must switch versions and mentally diff two walls of text. There is no diff view.
3. **No restore.** If a save makes a template worse, the only recovery is copy-paste from an old version into the editor — error-prone and undiscoverable. The backend already supports this (UpdateTemplate creates a new version when content differs) but no UI exposes it.
4. **History is a dropdown, not a surface.** A 40px dropdown cannot show notes, usage, or actions. Version management needs a real panel.
5. **No usage visibility.** Prompts are pinned to specific `version_id`s, but nothing shows "v2 is used by 14 prompts" — information that exists in the schema and would inform restore decisions.
6. **Save flow gives no framing.** "Save New Version" asks nothing; every save is an anonymous snapshot.

## 3. Target Users

- **Creators (primary):** users who iterate on templates and need to understand, compare, and recover their version history.
- **Forkers:** users who forked a template and want to see how the source evolved (read-only history + diff).
- **Regular users/viewers:** anyone inspecting a public template who wants to see its evolution and trust its maintenance.
- **Admins:** unaffected in v1 — version history is per-template content, not a moderation concern; no admin console surface (admin template moderation, if ever built, is a separate feature).

## 4. User Stories

### 4.1 Change notes

- **US-V1:** As a creator, when saving a new version I want to optionally add a short change note ("fixed tone drift"), so that my history is self-explanatory.
- **US-V2:** As a creator, I want the note to be optional, so that quick saves are not slowed down.
- **US-V3:** As a viewer, I want to see each version's note in the history list, so that I can understand the template's evolution without diffing.

### 4.2 Version history panel

- **US-V4:** As a user, I want a version history panel on the template page listing all versions (newest first) with number, note, timestamp, content length, and a "Latest" badge, so that I can survey the history at a glance.
- **US-V5:** As a user, I want to select a version from the panel and see its content in the existing preview, so that browsing versions works like today's dropdown but richer.
- **US-V6:** As a user, I want to know how many prompts were created from each version, so that I can judge which versions actually got used.

### 4.3 Diff

- **US-V7:** As a creator, I want to diff any two versions with added/removed/changed lines highlighted, so that I can see exactly what a save changed.
- **US-V8:** As a creator, I want diff stats (+N −M lines), so that I can gauge the size of a change at a glance.
- **US-V9:** As a user, I want a quick "diff vs previous" action on each version row, so that the most common comparison is one click.

### 4.4 Restore

- **US-V10:** As a creator, I want a "Restore this version" action that creates a new version with an old version's content, so that I can recover from a bad save without losing history.
- **US-V11:** As a creator, I want the restored version to carry an auto-generated note ("Restored from v3"), so that the history stays self-explanatory.
- **US-V12:** As a creator, I want restore to never delete or renumber existing versions, so that history is append-only and safe.

### 4.5 System

- **US-V13:** As the system, I keep restore owner-only, so that only the template owner can modify content.
- **US-V14:** As the system, I cap note length, so that notes stay scannable and storage bounded.
- **US-V15:** As the system, I keep existing API clients (alias → version mapping, prompt creation pinned to version_id) fully working, so that nothing breaks.

## 5. Functional Requirements

### 5.1 Data model

- **FR-1:** `template_versions` shall gain a nullable `change_note TEXT` column (max 200 chars, enforced server-side; NULL for existing rows and quick saves).
- **FR-2:** `UpdateTemplateRequest` shall accept an optional `change_note`; when content changes and a new version is created, the note is stored with it. When content is unchanged, the note is ignored (no version is created — existing behavior preserved).
- **FR-3:** The `TemplateVersion` proto message shall expose `change_note` (empty string when NULL) in `GetTemplate.latest_version`, `ListTemplateVersions`, and `UpdateTemplateResponse.new_version`.
- **FR-4:** `ListTemplateVersionsResponse` versions shall include `prompt_count` (number of prompts whose `version_id` references that version), computed via a LEFT JOIN + COUNT on `prompts`.

### 5.2 Version history panel (template details page)

- **FR-5:** The template details page shall offer a "History" toggle next to the version selector that opens a version history panel (in-place section, not a route) listing versions newest-first: version number, "Latest" badge (newest), change note (or "No note"), relative timestamp (absolute in `title`), content length (chars/lines), prompt count, and per-row actions.
- **FR-6:** Clicking a row selects that version (same behavior as the dropdown today: preview switches, editor loads its content when entering edit mode).
- **FR-7:** The panel shall paginate via the existing `page_token` pattern (20/page, "Load more"), replacing the current one-shot `page_size: 100` fetch.
- **FR-8:** The existing `VersionSelect` dropdown shall remain (quick switching) and shall display the change note snippet under the date when present.
- **FR-9:** The panel is visible to anyone who can view the template (public template → anonymous included); actions are permission-gated (restore is owner-only per FR-13).

### 5.3 Diff view

- **FR-10:** A "Compare" action shall open a diff modal with two version selectors (A = older, B = newer; defaults: selected version vs latest) and render a unified line diff: unchanged context lines, removed lines (−, red), added lines (+, green), with line numbers and stats (+N −M).
- **FR-11:** Diff shall be computed client-side from the two versions' content (both already fetched via ListTemplateVersions); no new backend endpoint. A "Swap A/B" control and "Compare" re-run on change.
- **FR-12:** Diff is available to anyone who can view the template (read-only; no permission needed).

### 5.4 Restore

- **FR-13:** Each version row (except the latest) shall offer "Restore" to the template owner: it calls the existing `UpdateTemplate` with the old version's content and an auto-generated note `Restored from v{n}` (localized client-side is NOT allowed — the note is stored data; store the canonical English string "Restored from v{n}" and render it as-is).
- **FR-14:** Restore shall require confirmation (modal showing source version and a warning that a new version will be created).
- **FR-15:** Restore shall never delete, renumber, or modify existing versions; it always appends (backend behavior of UpdateTemplate already guarantees this).
- **FR-16:** After a successful restore the UI shall select the new version, refresh the history list, and show a success toast.

### 5.5 Save flow

- **FR-17:** Edit mode shall show an optional "Change note (optional)" input (maxlength 200) above the save button; the note is sent with `UpdateTemplate`.
- **FR-18:** Saving with unchanged content keeps today's behavior (no new version, metadata-only update path unaffected); the note input is ignored.

### 5.6 API summary

- **FR-19:** No new REST endpoints. Changes: `PUT /api/v1/templates/{id}` body gains optional `change_note`; `GET /api/v1/templates/{id}/versions` responses gain `change_note` and `prompt_count` per version; `GET /api/v1/templates/{id}` `latest_version` gains `change_note`.
- **FR-20:** Backwards compatibility: all new fields are optional/nullable; older clients ignoring them behave exactly as today. Alias mapping (`GetPromptByAlias` → version) and prompt creation (`version_id` pinning) are untouched.

### 5.7 Regular-user vs admin split

- **FR-21:** Versioning UX is a regular-user surface on the template details page. Admin users experience it identically as viewers/owners.
- **FR-22:** No admin console pages (`/admin/*`) and no admin APIs (`/api/v1/admin/*`) are part of this feature.

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** History panel first paint < 300ms p95 (existing indexed query + one JOIN); diff computation < 100ms for typical template content (≤ 32KB) client-side; restore is a single existing UpdateTemplate call.
- **NFR-2 (Compatibility):** Additive-only schema/proto changes; no breaking API changes; existing pages and the alias/API-key prompt flow unchanged (FR-20).
- **NFR-3 (Privacy/Security):** History/diff visibility follows template visibility (public → anyone, private → owner); restore is owner-only, enforced server-side by the existing UpdateTemplate authorization; change notes are plain text (React-escaped), max 200 chars.
- **NFR-4 (Accessibility):** Panel, diff modal, and restore flow fully keyboard-operable; diff lines carry semantic markup (add/remove status exposed to screen readers); see UI/UX doc §10.
- **NFR-5 (i18n):** All new UI strings EN/ZH in sync; stored change notes and auto-generated restore notes are user/system data rendered as-is (never translated at render time); relative timestamps via the shared util.
- **NFR-6 (Resilience):** If `prompt_count` cannot be computed (e.g., older backend), the UI hides the usage column instead of erroring; if the diff library is unavailable, the Compare action falls back to a side-by-side raw view.
- **NFR-7 (Data integrity):** `change_note` is NULL-able and length-capped server-side; restore notes use one canonical format so history stays machine-parseable ("Restored from v{n}").

## 7. Acceptance Criteria

- **AC-1:** Saving a version with a change note stores and displays it in the history panel and the version dropdown; saving without a note stores NULL and shows "No note".
- **AC-2:** The history panel lists versions newest-first with number, Latest badge, note, relative time, content length, and prompt count, paginated 20/page with Load more.
- **AC-3:** Clicking a history row (or dropdown option) switches the preview to that version, identical to current dropdown behavior.
- **AC-4:** Compare opens a diff modal for any two versions with correct +/- line highlighting, line numbers, and +N −M stats; Swap A/B works.
- **AC-5:** Restore (owner-only, non-latest rows) creates a new version with the old content and note "Restored from v{n}", leaves all existing versions untouched, selects the new version, and refreshes the list.
- **AC-6:** Non-owners see the panel and diff but no Restore action; anonymous users on public templates see the panel read-only.
- **AC-7:** Notes longer than 200 chars are rejected server-side (400) and prevented client-side (maxlength + counter).
- **AC-8:** Existing behavior unchanged: metadata-only saves create no version; alias → version mapping returns the same results; prompts created earlier still reference their original version_id.
- **AC-9:** All new UI strings exist in both `en` and `zh` translation files with identical key sets.
- **AC-10:** Template details page (view/edit/fork/like/favorite/prompt generation) behaves unchanged apart from the new panel, note input, and actions.
- **AC-11:** With `prompt_count` absent from the API response, the usage column hides without errors (NFR-6).

## 8. Out of Scope (v1)

- Side-by-side (two-column) diff — unified diff only in v1.
- Version deletion (history is append-only; deleting would orphan prompts pinned to that version).
- Version branching/labels (e.g., "stable" vs "beta" beyond the existing alias mechanism).
- Per-version author attribution (versions are owner-only edits today; co-editing does not exist).
- Diff of metadata (title/tags/category) — content diff only.
- Server-side diff endpoint (client-side only; revisit if templates grow very large).
- Admin console version management (`/admin/*`).
- Email/notification on version changes (notifications feature covers engagement events only).
- Mobile-app-specific layouts (responsive web only).

## 9. Dependencies & Assumptions

- **D-1:** `UpdateTemplate` (backend/internal/service/prompt.go) already creates a new version iff content differs and returns it as `new_version` — restore needs no new endpoint, only the `change_note` passthrough (FR-2).
- **D-2:** `template_versions` has `UNIQUE (template_id, version)` and an index on `template_id` (deployment/init.sql) — the history query and the prompt-count JOIN are index-supported.
- **D-3:** `prompts.version_id` references `template_versions(id)` (deployment/init.sql) — `prompt_count` is a LEFT JOIN COUNT; versions with zero prompts show 0.
- **D-4:** `ListTemplateVersions` RPC + REST route already exist and paginate; the frontend currently fetches `page_size: 100` in one shot (TemplateDetails.js) — FR-7 replaces this with real pagination.
- **D-5:** Client-side diff requires a diff implementation. Recommendation: the `diff` npm package (Myers, ~10KB) — adding a dependency is a developer/architect decision; a minimal hand-rolled LCS is the fallback (open question #2).
- **D-6:** The relative-time util is shared with the user-dashboard and notifications features (their D-4/D-6); if neither has shipped yet, this feature introduces it.
- **D-7:** The auto-generated restore note is stored in canonical English ("Restored from v{n}") because it is data, not UI chrome (NFR-5); the confirmation modal around it is fully localized.
- **D-8:** Content length display (chars/lines) is computed client-side from the already-fetched content; no API change.

## 10. Open Questions for Architect

1. **`prompt_count` computation:** inline LEFT JOIN + COUNT in `ListTemplateVersions` (simple, fine at current scale) vs. a separate batched count query. Recommendation: inline JOIN.
2. **Diff library:** add `diff` (npm, Myers) as a frontend dependency vs. hand-rolled LCS util (no dependency, ~60 lines, adequate for ≤ 32KB content). Recommendation: `diff` package for correctness on edge cases; hand-rolled acceptable if dependencies are being minimized.
3. **`change_note` on CreateTemplate:** v1 assumes the initial version (v1) needs no note ("Initial version" is implied). Confirm, or add the field to CreateTemplateRequest too for symmetry.
