# Advanced Template Editor — Requirements Analysis

- **Feature ID:** advanced-editor
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. Competitor Research Summary

| Platform | Editor approach | Key takeaways |
|---|---|---|
| **PromptBase** | Simple textarea with a live preview pane; variable syntax highlighted in the preview | Live preview is the single highest-value editor feature; the input itself can stay plain |
| **FlowGPT** | Plain textarea; variables typed manually; no validation | No validation means silent typos in variable names — the #1 creator error we must prevent |
| **Snack Prompt** | Textarea + character counter + basic markdown formatting toolbar | A lightweight toolbar (bold/italic for description fields) is expected but secondary |
| **AIPRM** | Rich prompt composer with variable insertion buttons, template categories/tags pickers, and a "test" step before publishing | Insertion buttons beat typing `$$` syntax by hand; a pre-publish sanity check catches broken templates |
| **PromptHero** | Minimal text input; prompts are static text | Static-text platforms show the floor; our templates are parameterized, so the editor must treat variables as first-class |
| **Notion** | Block editor with slash commands, inline mentions, undo history | Slash-command insertion and robust undo are the patterns power users cite most; full block editing is overkill for prompt text |
| **Stack Overflow / GitHub comment boxes** | Markdown toolbar + live preview toggle + draft persistence warnings | Draft-loss prevention (beforeunload warnings, local draft) is table stakes for any long-form web editor |

**Synthesis:** The winning v1 pattern is an *augmented textarea*: keep the plain-text content model (prompt text is plain text with `$$variable$$` placeholders — no rich text), and add the four things creators actually need: (1) a **variable toolbar** that inserts `$$name$$` at the cursor and lists detected variables with counts; (2) **live preview** with variables highlighted and a sample-resolution toggle; (3) **validation** — unclosed `$$`, empty variable names, duplicate names differing only by case/whitespace, and over-length content flagged before save; and (4) **draft safety** — autosave to localStorage per template, restore prompt after reload, and a beforeunload guard while editing. This upgrades the existing edit mode on the template details page (currently a bare `<textarea>`) without introducing a heavy dependency or a new route.

## 2. Problem Statement

1. **Variables are error-prone.** Creators type `$$varname$$` by hand in a bare textarea. A missing closing `$$`, a trailing space inside the name (`$$name $$`), or inconsistent casing (`$$Name$$` vs `$$name$$`) silently produces a template whose generator renders wrong or empty values. Nothing validates any of this today.
2. **No live feedback while editing.** The current editor shows the raw text only; the creator must save a new version, exit edit mode, and scroll to the generator to see how the template resolves. Typos are discovered by end users, not the creator.
3. **Drafts are lost easily.** Edit state lives in React state only. A reload, accidental navigation, or session expiry while editing discards an arbitrary amount of work with zero warning.
4. **No editor affordances.** No character/line counts, no variable inventory, no insert-variable button, no keyboard shortcut to save. The textarea is the same control used for one-line notes.
5. **Inconsistent editing surfaces.** Template creation (Home page modal) and template editing (details page) use different, minimal controls; creators get no transferable muscle memory.

## 3. Target Users

- **Template creators (primary):** users writing or revising template content — they need correctness feedback and draft safety.
- **Power creators:** maintain many templates with many variables — they need the variable inventory and quick insertion.
- **Regular users:** unaffected in v1 (the generator on the detail page is not an editor); they benefit downstream from fewer broken templates.
- **Admins:** no admin surface in v1 — admins edit templates through the same regular-user editor as owners (existing behavior); moderation remains in the admin console.

## 4. User Stories

- **US-E1:** As a creator, when I type `$$` the editor helps me complete the variable, or lets me click a button to insert a well-formed `$$name$$` at the cursor, so that I never produce malformed placeholders.
- **US-E2:** As a creator, I want a live list of the variables my content contains with occurrence counts, so that I can spot misspellings and duplicates at a glance.
- **US-E3:** As a creator, I want validation errors (unclosed `$$`, empty name, near-duplicate names, over-length) shown inline before I can save, so that broken templates cannot be published.
- **US-E4:** As a creator, I want a live preview of the resolved prompt with sample or custom values, so that I can verify the template works without saving a version first.
- **US-E5:** As a creator, if I reload or navigate away mid-edit, I want my draft restored (or at least a warning), so that I never lose work.
- **US-E6:** As a creator, I want character and line counts plus the max-length indicator, so that I can stay within limits while writing.
- **US-E7:** As a creator, I want `Cmd/Ctrl+S` to save and `Esc` to cancel from the editor, so that frequent edits are fast.
- **US-E8:** As a creator creating a new template, I want the same variable toolbar and validation in the creation form, so that the experience is consistent everywhere I write template content.

## 5. Functional Requirements

### 5.1 Editor surface (upgrade of existing edit mode)

- **FR-1:** The advanced editor replaces the bare `<textarea>` in the template details edit mode and the template creation modal's content field. It remains a plain-text control (no rich text); content model and `$$variable$$` syntax are unchanged.
- **FR-2:** **Variable toolbar:** an "Insert variable" button opens a small popover with a name input (validated: letters, digits, underscore, 1–30 chars) and inserts `$$name$$` at the cursor position. Existing variables in the content appear as one-click chips in the same popover for re-insertion.
- **FR-3:** **Variable inventory:** a panel below the editor lists each unique variable with its occurrence count; clicking an occurrence count highlights all occurrences in the editor (v1: focuses the editor and selects the first occurrence; full in-text highlighting is a UI nicety, not a requirement).
- **FR-4:** **Validation (client-side, pre-save):** the editor flags, in a fixed validation summary area: (a) unclosed or unbalanced `$$` delimiters; (b) empty variable names (`$$$$`); (c) names with leading/trailing whitespace; (d) near-duplicates differing only by case or whitespace; (e) content exceeding the max length (same limit as today's save path). The Save button is disabled while blocking errors (a–d) exist; over-length is always blocking.
- **FR-5:** **Live preview:** a toggleable preview pane renders the content with variables highlighted; a "sample values" mode fills each variable with a placeholder value (`<name>`), and a "custom values" mode reuses the existing generator inputs. Preview updates as the user types (debounced 300 ms).
- **FR-6:** **Draft safety:** while editing, the content is autosaved to localStorage under a per-template key (`draft:template:{id}` for edits, `draft:new-template` for creation) at most every 5 s when dirty. On entering edit mode, if a draft exists and differs from the loaded version, a restore prompt offers "Restore draft" / "Discard". A `beforeunload` guard is active while dirty. Drafts are deleted on successful save or explicit discard.
- **FR-7:** **Counts:** live character count, line count, and variable count in the editor footer; the character count turns amber within 10% of the limit and red at the limit.
- **FR-8:** **Keyboard:** `Cmd/Ctrl+S` saves (when validation passes), `Esc` cancels (with a confirm modal if dirty), `Cmd/Ctrl+Enter` inserts a new variable popover trigger as an alias of the Insert button.
- **FR-9:** The editor is used in both the details-page edit mode and the creation modal; the creation modal additionally keeps its existing title/description/category/tags fields unchanged.

### 5.2 Save path (unchanged semantics, better feedback)

- **FR-10:** Saving uses the existing update/create endpoints and version-creation semantics (a content change creates a new version, per the template-versioning feature). The editor adds no new endpoints; it only gates the existing Save button on validation.
- **FR-11:** On save success the draft is cleared, a success notification shows, and edit mode exits as today. On save failure the draft is kept and the error is shown inline with a retry.

### 5.3 Admin surface

- **FR-12:** None in v1. Admin template moderation remains read-only in the admin console (existing behavior). If an admin edits a template as its owner, they get the same advanced editor via FR-1.

### 5.4 Data model

- **FR-13:** No new database tables and no backend changes. Drafts live in localStorage only; validation is client-side. The `$$variable$$` content format, template length limits, and version semantics are unchanged.

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Typing latency < 50 ms for templates up to 100 KB; validation and inventory recompute debounced (300 ms) and must not block input.
- **NFR-2 (Compatibility):** Works in the same browser matrix as the app; degrades gracefully — if localStorage is unavailable, editing works without drafts (no errors).
- **NFR-3 (Security):** Draft content in localStorage is scoped per user account where feasible (key includes user id); drafts are cleared on logout. No HTML injection: preview renders variables as text, never as markup.
- **NFR-4 (i18n):** All editor strings EN/ZH via i18next (`editor.*` namespace); variable names themselves are not translated.
- **NFR-5 (Accessibility):** Editor is fully keyboard operable; validation errors are announced via `aria-live` and linked to the editor via `aria-describedby`; preview pane is a labelled region; toolbar buttons have focusable labels.
- **NFR-6 (Maintainability):** Variable parsing/validation logic is a single shared utility (pure functions) used by the editor, the creation modal, and reusable later by import validation.

## 7. Acceptance Criteria

- **AC-1:** Typing `$$na` and clicking Insert variable with name `na` inserts a well-formed `$$na$$` at the cursor; the inventory immediately shows `na ×1`.
- **AC-2:** Content with `$$unclosed` shows a blocking validation error; Save is disabled until fixed.
- **AC-3:** Content containing `$$Name$$` and `$$name$$` shows a near-duplicate warning listing both; Save is blocked until resolved.
- **AC-4:** Toggling preview with sample values renders `My name is <name>` for content `My name is $$name$$`.
- **AC-5:** Editing a template, reloading the page, and re-entering edit mode offers draft restore; restoring returns the exact draft content.
- **AC-6:** Character count shows `1234 / 10000` style feedback and turns red at the limit, with Save blocked.
- **AC-7:** `Cmd/Ctrl+S` saves when validation passes; `Esc` on a dirty editor opens the cancel confirm modal.
- **AC-8:** The creation modal uses the same editor; creating a template with an unclosed `$$` is impossible (Save blocked with the error shown).
- **AC-9:** All strings translated EN/ZH; no hardcoded copy.
- **AC-10:** No monetization content anywhere in the feature.

## 8. Out of Scope

- Rich text / markdown rendering of template content (content stays plain text).
- Collaborative editing (multi-user cursors, presence).
- AI-assisted writing (suggestions, autocomplete of prompt text).
- Server-side validation endpoint or server-stored drafts.
- Full in-text occurrence highlighting (v1 selects the first occurrence from the inventory).
- CodeMirror/Monaco-style syntax highlighting inside the textarea itself.
- Monetization of any kind (payments, pricing, purchases, earnings) — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1:** The `$$variable$$` placeholder syntax and the client-side extraction regex (mirroring the backend's `\$\$(.*?)\$\$`) remain the template variable contract.
- **D-2:** The template-versioning feature's save semantics (content change → new version) are already designed; this feature only gates and augments the same save path.
- **D-3:** The existing creation modal (Home page) and details-page edit mode are the two surfaces to upgrade; no new routes.
- **D-4:** Template content max length follows the existing backend limit; the editor reads it from a shared constant.
- **D-5:** The accessibility feature's focus-management and live-region patterns are followed for the validation summary.

## 10. Open Questions for Architect

1. **Max length source:** Should the content max length be exposed by the backend (e.g. in a config/limits endpoint) rather than hardcoded in the frontend constant, so editor and backend can't drift?
2. **Draft key scoping:** Is including the user id in the localStorage draft key sufficient, or do you prefer drafts keyed by template id only with a per-user prefix namespace (same thing) — any security concern with draft content lingering on shared machines after logout (we clear on logout, but not on token expiry)?
3. **Validation parity:** The backend currently extracts variables with `\$\$(.*?)\$\$` (non-greedy, no name validation). Should the backend add the same name-validation rules (FR-4) at save time as defense in depth, or is client-side gating acceptable for v1?
4. **Creation modal scope:** The creation modal also has title/description/category/tags. Should variable validation extend to the description field (variables are not substituted there today), or keep validation to the content field only (recommended)?
