# Variable Presets — Requirements Analysis

- **Feature ID:** variable-presets
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-13

## 1. Competitor Research Summary

| Platform | Saved-input approach | Key takeaways |
|---|---|---|
| **Browser autofill** | Per-site saved values; explicit save prompt; per-field dropdown | Users expect saved values to be per-context, not global; explicit save beats silent capture |
| **Postman** | Environments (named variable sets) switchable per request; global vs. environment scope | Named sets with a scope selector is the power-user pattern; one-click switching is the core UX |
| **Stripe Dashboard** | Saved test cards / customers as reusable fixtures in API playgrounds | Reusable fixtures dramatically cut repeated typing in test/iterate loops |
| **ChatGPT (custom instructions)** | Persistent "about me" profile injected into every conversation | A small always-on profile covers the most repetitive fields (tone, language, persona) |
| **Zapier** | Saved field mappings per connected account; "reuse previous values" | Per-account context beats re-entering; "recent" list covers one-off reuse |
| **VS Code snippets** | Named snippets with placeholders; snippet picker | Naming and quick-pick retrieval make reuse fast without search UI |

**Synthesis:** The proven pattern for repeated form input is **named, per-context value sets with a quick switcher**: Postman environments prove the named-set model; browser autofill proves explicit save + per-field recall; ChatGPT custom instructions prove that a small always-on profile handles the most repetitive fields. For Open Prompts, the generator form (variable inputs on the template detail page) is re-typed on every use; a **per-template named preset** system with an optional **global default profile** ("My defaults" for tone/language/name fields) removes the bulk of that typing while staying fully user-controlled.

## 2. Problem Statement

The template generator on the detail page renders one input per `$$variable$$`. Every use re-types the same values:

1. **Repetitive typing on every use.** A user who runs "Cold Email" daily re-enters `tone=professional`, `language=English`, `company=Acme` every time. Nothing is remembered.
2. **No cross-template continuity.** Many templates share variables (`tone`, `language`, `audience`); values entered for one template never carry to another.
3. **No saved contexts.** Users who use the same template for different clients/projects (Acme vs. Beta) cannot keep two value sets; they re-type or keep notes outside the product.
4. **Copy-paste workaround.** The only "preset" today is pasting from a personal notes file — error-prone and invisible to the product.

## 3. Target Users

- **Frequent users (primary):** users who instantiate the same or similar templates regularly and re-type the same variable values.
- **Multi-context users:** users using one template for different clients, projects, or personas.
- **Creators (indirect):** benefit from lower friction in the generator when testing their own templates (dogfooding loop with advanced-editor's preview).
- **Admins:** no admin surface in v1 (§8); presets are private per-user data.

## 4. User Stories

### 4.1 Saving and applying presets

- **US-V1:** As a user, after filling the generator form I can save the current values as a named preset for this template, so that I can reuse them next time.
- **US-V2:** As a user, when I open the generator I can pick a saved preset from a dropdown and all fields fill instantly, so that reuse is one click.
- **US-V3:** As a user, I can update an existing preset with the current form values, so that presets stay current.
- **US-V4:** As a user, I can delete a preset I no longer need, so that the list stays clean.
- **US-V5:** As a user, I see the preset selector only on templates I've saved presets for plus a "Save as preset" action always available, so that the UI stays calm for one-off uses.

### 4.2 Global defaults

- **US-V6:** As a user, I can maintain a "My defaults" profile of common variable values (e.g., `tone`, `language`, `name`), so that shared variables across templates pre-fill automatically.
- **US-V7:** As a user, global defaults apply only to fields I haven't explicitly set in the chosen preset or by typing, so that specific choices always win.
- **US-V8:** As a user, I can manage my defaults on a settings page, so that I control the always-on profile.

### 4.3 System stories

- **US-V9:** As the system, I store presets privately per user, so that no user ever sees another's presets.
- **US-V10:** As the system, when a template's variables change between versions, I apply presets by variable name and silently skip missing ones, so that stale presets never break the form.

## 5. Functional Requirements

### 5.1 Per-template presets

- **FR-1:** On the generator form (template detail page, logged-in users only), a "Presets" dropdown shall list saved presets for that template (name + last-used time), plus actions: "Save current as preset…", "Manage presets".
- **FR-2:** Selecting a preset shall fill every form field whose variable name exists in the preset; fields not in the preset keep their current value (merge, not replace).
- **FR-3:** Saving a preset shall require a name (1–50 chars); duplicate names for the same template prompt to overwrite (confirm dialog).
- **FR-4:** A preset shall store `{template_id, name, values: {var_name: value}, created_at, updated_at, last_used_at}`; values are plain strings, capped at the existing variable value length limit.
- **FR-5:** Each user shall be limited to 20 presets per template; exceeding prompts to delete one (server enforces with 400).
- **FR-6:** "Update preset" shall overwrite the stored values with current form values (confirm dialog; name unchanged).
- **FR-7:** Deleting a preset shall require confirmation and be immediate (no trash/undo in v1).
- **FR-8:** Applying a preset shall update `last_used_at`; the dropdown sorts by most recently used.

### 5.2 Variable-name drift

- **FR-9:** Presets shall be applied by variable name. If the template's current version lacks a variable present in the preset, that value is silently skipped (no error); the preset remains usable.
- **FR-10:** If a preset's values cover none of the current variables (fully stale), applying shows a notice ("This preset doesn't match the current variables") and fills nothing.

### 5.3 Global defaults profile

- **FR-11:** A "My defaults" profile shall be available per user: `{var_name: value}` pairs, max 30 entries, managed at `/settings/defaults`.
- **FR-12:** When the generator form first renders (no preset applied), fields whose variable name matches a default shall pre-fill with it; the pre-fill is visually indicated once (subtle hint on first field) and fully editable.
- **FR-13:** Precedence: explicit user typing > applied preset > global defaults. Applying a preset overwrites default-filled values for the variables it contains.
- **FR-14:** A "Save to my defaults" action in the generator shall offer saving individual field values (or all fields) into the defaults profile, with overwrite confirmation for existing keys.
- **FR-15:** The defaults profile can be cleared entirely (confirm dialog) or edited key-by-key on the settings page.

### 5.4 Access & visibility

- **FR-16:** Presets and defaults are available to logged-in users only; anonymous visitors see the plain generator (no preset UI, no defaults pre-fill).
- **FR-17:** Presets are private to their owner; they are not shared, not exported in v1, and not visible to template authors.
- **FR-18:** Presets work on any template the user can view (public or own private); preset data never grants template access.

### 5.5 Regular-user vs admin split

- **FR-19:** All surfaces are regular-user surfaces (generator on `/templates/{id}`, `/settings/defaults`, `/api/v1/me/*`). No admin console pages; no `/api/v1/admin` endpoints.

### 5.6 API

- **FR-20:** `GET /api/v1/templates/{id}/presets` → `{presets: [{id, name, values, updated_at, last_used_at}]}` (owner-scoped to the requesting user).
- **FR-21:** `POST /api/v1/templates/{id}/presets` body `{name, values}` → 201; `PUT /api/v1/templates/{id}/presets/{preset_id}` body `{values}` (and optional `name`) → 200; `DELETE /api/v1/templates/{id}/presets/{preset_id}` → 204.
- **FR-22:** `POST /api/v1/templates/{id}/presets/{preset_id}/use` → updates `last_used_at` → 204 (called on apply).
- **FR-23:** `GET /api/v1/me/defaults` → `{values: {var: value}}`; `PUT /api/v1/me/defaults` body `{values}` (full replace) → 200; `DELETE /api/v1/me/defaults` → 204.

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Preset list fetch is parallel and non-blocking; applying a preset is client-side (no round trip except `use` beacon); form fill < 50ms.
- **NFR-2 (Privacy):** Preset values may contain personal context (names, companies); they are private per user, never rendered to other users or template authors, and excluded from analytics payloads.
- **NFR-3 (Compatibility):** Anonymous and preset-less users see the unchanged generator; no existing endpoint changes.
- **NFR-4 (Accessibility):** Dropdown, save/update/delete dialogs, and the defaults settings page are fully keyboard operable (UI/UX doc §9).
- **NFR-5 (i18n):** All new UI strings in EN and ZH with identical key sets.
- **NFR-6 (Data):** New tables: `template_variable_presets (id, user_id, template_id, name, values JSONB, created_at, updated_at, last_used_at)` with `UNIQUE(user_id, template_id, name)`; `user_default_variables (user_id, values JSONB, updated_at)` with `UNIQUE(user_id)`.
- **NFR-7 (Limits):** Preset name ≤ 50 chars; values ≤ existing variable value limit; 20 presets/template; 30 default entries — enforced server-side with clear 400 messages.

## 7. Acceptance Criteria

- **AC-1:** Filling the generator and saving as "Acme campaign" lists it in the Presets dropdown; reopening the template and selecting it refills all matching fields.
- **AC-2:** Applying a preset after editing some fields keeps the edited fields unless the preset contains those variables (merge semantics, FR-2).
- **AC-3:** A template whose new version dropped `$$tone$$` still applies a preset containing `tone` — silently skipping it; a fully stale preset shows the notice and fills nothing.
- **AC-4:** With `language=English` in My defaults, opening any template with a `language` variable pre-fills it; typing over it wins; applying a preset containing `language` overwrites it.
- **AC-5:** The 21st preset save for a template returns 400 and the UI prompts to delete one.
- **AC-6:** Anonymous visitors see no preset UI and no defaults pre-fill.
- **AC-7:** User A's presets never appear for user B on the same template.
- **AC-8:** Deleting a preset removes it from the dropdown immediately; the confirm dialog precedes deletion.
- **AC-9:** `/settings/defaults` lists all entries with edit/delete; clearing all requires confirmation.
- **AC-10:** All new UI strings exist in both `en` and `zh` translation files with identical key sets.

## 8. Out of Scope (v1)

- Sharing presets with teams (team-collaboration may adopt later).
- Exporting/importing presets (import-export covers templates only).
- Presets for API-key usage (alias-based consumption has no form; a future API could accept preset ids).
- Auto-capture of values without explicit save (no silent history).
- Preset versioning or diffing.
- Admin visibility into user presets (privacy stance, FR-17).
- Monetization of any kind — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1:** The generator form on the template detail page renders one input per `$$variable$$` (existing behavior); the form state is client-side React state this feature hooks into.
- **D-2:** Variable extraction uses the shared `$$var$$` regex utility (advanced-editor D-1, template-compare D-2 — same contract).
- **D-3:** Auth middleware identifies the logged-in user on `/api/v1/me/*` and template-scoped routes (existing pattern).
- **D-4:** The `/settings` route prefix exists or arrives with another in-flight feature (same assumption as personalized-recommendations D-7); this feature adds `/settings/defaults`.
- **D-5:** Variable value length limit is the existing backend limit for prompt variable values; presets reuse it unchanged.
- **D-6:** JSONB storage of `values` maps is acceptable (Postgres); no per-variable rows needed.

## 10. Open Questions for Architect

1. **`use` beacon necessity:** is `last_used_at` worth a dedicated endpoint (FR-22), or should apply-time be updated opportunistically inside the next `PUT`? Design keeps the beacon (cheap, keeps MRU sorting honest).
2. **Defaults pre-fill timing:** client fetches `/me/defaults` on generator mount (one extra request per detail view) vs. bundling into the template detail response. Design assumes a lightweight parallel fetch with client caching per session.
3. **Preset cap enforcement:** 20 per template per user (FR-5) — confirm the limit and whether a global per-user cap (e.g., 200) is also needed to bound storage.
