# Template Import/Export — Requirements Analysis

| | |
|---|---|
| **Feature ID** | FE-010 |
| **Version** | 1.0 |
| **Status** | Proposed (for architecture review) |
| **Author** | ui-ux-agent |
| **Date** | 2026-10-10 |

## 1. Competitor Research

| Product | Import/Export Approach | Strengths | Weaknesses |
|---|---|---|---|
| AIPRM | Full JSON export/import of prompt templates for team sharing; includes category, tone, topic metadata | Portable across accounts; teams migrate easily | Opaque format; no version history in export |
| PromptBase | Closed marketplace; no user-facing export — content locked to platform | Protects marketplace content | Users cannot back up their own work |
| FlowGPT | Share via public URL only; no file export | Simple sharing | No offline portability, no backup |
| ChatGPT (custom instructions) | Copy-to-clipboard only | Zero friction | No structure, no metadata, manual re-entry |
| GitHub Gists | Full file download; any text format | Universal, versioned | No prompt-specific structure |
| Cursor (rules files) | `.mdc` files shared in repos; import by placing file | Ecosystem portability | No validation, no metadata schema |

**Takeaway:** No competitor offers structured, version-aware prompt export. AIPRM's JSON approach is the closest model but lacks version history. A well-defined open JSON format with version history gives open-prompts a differentiation point: users own and can move their work.

## 2. Problem Statement

Users invest significant effort writing and iterating prompt templates, but their work is locked inside open-prompts:

1. **No backup.** A template deleted accidentally (or an account issue) means lost work. There is no way to take a copy off-platform.
2. **No transfer.** Users moving between accounts (personal → team, or migrating instances) must re-type every template and lose version history.
3. **No offline sharing.** Private templates cannot be handed to a colleague except by making them public or re-pasting raw text, which drops all metadata (category, tags, language) and history.
4. **No bulk onboarding.** A user with prompts in text files or another tool has no structured way to bring them in; each must be created manually field by field.

## 3. Target Users

- **Prompt authors** who want a backup of their templates and version history.
- **Migrating users** moving work between accounts or self-hosted instances.
- **Teams/colleagues** sharing private templates without making them public.
- **New users** arriving with existing prompt collections in files.

## 4. User Stories

- **US-1**: As a template owner, I want to export one of my templates (with its full version history) to a JSON file so that I have an off-platform backup.
- **US-2**: As a user, I want to import a previously exported template file so that I can restore or transfer my work to another account.
- **US-3**: As a user, I want to review and edit a template's fields before the import is committed so that I can fix conflicts (e.g., duplicate title) and choose visibility.
- **US-4**: As a user, I want clear validation errors when an imported file is malformed, truncated, or not an open-prompts export so that I understand what went wrong.
- **US-5**: As a colleague receiving a shared export file, I want the imported template to start as private so that nothing is exposed publicly without my consent.
- **US-6**: As a user importing a multi-version export, I want the version history preserved (as sequential versions) so that iteration context is not lost.
- **US-7**: As a user, I want the export file to be human-readable JSON so that I can inspect it, script against it, or archive it in git.
- **US-8**: As an admin, I want to export any template from the admin console (for support/debugging) so that I can inspect a user's template without impersonating them.

## 5. Functional Requirements

### 5.1 Export

- **FR-1**: Template owners can export any template they own from the template details page via an "Export" action that downloads a single `.json` file.
- **FR-2**: The export file MUST contain: a format identifier (`open-prompts-template`), a format version (`1`), export metadata (exported_at ISO-8601, source product name), template metadata (title, description, visibility, type, tags, category, language), and the full version history (ordered list of version number, content, change note if present, created_at).
- **FR-3**: Export MUST be client-side: the frontend assembles the file from existing `getTemplate` and `listTemplateVersions` responses. No new backend endpoint is required.
- **FR-4**: The downloaded filename MUST be derived from the template title (slugified, ASCII-safe, max 60 chars) plus template id prefix, e.g. `my-prompt-template-a1b2c3d4.json`.
- **FR-5**: Exporting another user's template (non-owner) MUST NOT be offered; the export action is hidden for templates the user does not own.
- **FR-6**: Export MUST NOT include owner identity (user id, email, display name) to avoid leaking personal data in shared files.

### 5.2 Import

- **FR-7**: Authenticated users can import a template file from the templates home page via an "Import" action that opens a modal accepting one `.json` file via file picker or drag-and-drop.
- **FR-8**: The import flow MUST have two steps: (1) file selection and parsing with validation feedback, (2) a review form pre-filled with the parsed fields (title, description, category, tags, language, visibility, content of the latest version) that the user can edit before confirming.
- **FR-9**: Import MUST reuse the existing `createTemplate` endpoint. No new backend endpoint is required. Multi-version imports create the template with the latest content, then apply earlier versions in order via the existing update path so history is preserved (see Open Questions).
- **FR-10**: Imported templates MUST default to `visibility: private` regardless of the exported value; the user can change it in the review step.
- **FR-11**: Validation MUST reject: non-JSON files, files > 1 MB, missing format identifier, unknown format version, missing title or content, and field values violating existing constraints (title length, tag count/length, content size). Each rejection MUST produce a specific, human-readable error identifying the offending field.
- **FR-12**: If the parsed title duplicates an existing template title of the same user, the review form MUST warn (non-blocking) and suggest a suffixed title (`title (imported)`); the user may keep or edit it.
- **FR-13**: On successful import, the user is navigated to the new template's details page with a success notification.
- **FR-14**: On validation failure, the user stays on step 1 with the error displayed and may choose a different file.

### 5.3 Regular-user vs admin split

- **Regular users**: export own templates (FR-1–6); import files (FR-7–14).
- **Admin console** (`/admin/templates/:id`): an "Export" action identical in output format (FR-2) is available to admins for any template, supporting support/debugging workflows. Admin import is NOT provided (admins act on user-reported files through their own account's regular import).

## 6. Non-Functional Requirements

- **NFR-1 (Usability)**: Import review step reuses the existing CreateTemplateModal field layout and validation so learning cost is zero.
- **NFR-2 (Performance)**: Export of a template with ≤ 50 versions completes in < 1 s client-side; the file download starts immediately after data fetch.
- **NFR-3 (Security)**: Export files never contain credentials, tokens, or owner PII. Import parses untrusted JSON in a try/catch with size cap; malformed input cannot crash the app (error boundary safe).
- **NFR-4 (Compatibility)**: Format version `1` files remain importable in future releases; unknown future versions are rejected with a clear message rather than mis-parsed.
- **NFR-5 (Accessibility)**: The import modal is fully keyboard operable, announces validation errors via the existing Field error pattern, and the drag-drop area has a visible focus state and keyboard-browsable file button.
- **NFR-6 (i18n)**: All UI strings, validation messages, and the human-readable parts of error codes are localized (EN/ZH). The JSON file itself uses English keys and is not localized.

## 7. Acceptance Criteria

- **AC-1**: Owner opens a template details page → "Export" visible → click downloads a valid JSON file matching FR-2 structure; opening it in a text editor shows readable, indented JSON.
- **AC-2**: Non-owner (or logged-out) viewing a public template sees no "Export" action.
- **AC-3**: Exported file contains N versions in chronological order with content and change notes intact.
- **AC-4**: User imports the file from AC-1 into another account → review form shows all fields pre-filled, visibility forced to private → confirm → template created → navigated to details page → version history shows the same N versions.
- **AC-5**: Importing a random non-JSON file (e.g. `.png`) shows a specific error, not a crash.
- **AC-6**: Importing a JSON with `"format": "unknown-thing"` or `"format_version": 99` shows a "unsupported format" error.
- **AC-7**: Importing a file > 1 MB shows a size error before parsing.
- **AC-8**: A title collision produces a non-blocking warning with suggested suffixed title.
- **AC-9**: Admin exports a user's template from `/admin/templates/:id` → same file structure as regular export.
- **AC-10**: All import/export UI strings appear in both EN and ZH locale files with the same keys.
- **AC-11**: The import modal passes keyboard-only operation: tab through all controls, submit with Enter, dismiss with Escape.
- **AC-12**: Export and import work in both light and dark themes with no visual defects.

## 8. Out of Scope

- Bulk export (all templates at once / account archive) — candidate for a future iteration.
- Import from third-party formats (AIPRM JSON, ChatGPT text) — only the open-prompts format is supported.
- Cross-instance "share by link" or server-side transfer — file-based only.
- Export of prompts (usage records) or API keys — templates only.
- Any form of marketplace listing, paid distribution, or commercial distribution of export files.

## 9. Dependencies & Assumptions

- **D-1**: `getTemplate` returns full metadata and `listTemplateVersions` returns all versions with content; both are already available in `services/api.js`.
- **D-2**: `createTemplate` accepts title, description, visibility, type, tags, category, content, language (verified in proto `CreateTemplateRequest`).
- **D-3**: The template update path creates a new version when content differs (existing behavior), which multi-version import relies on.
- **D-4**: Template `type` (system/user) is part of the export; the review step exposes it as a read-only or editable field consistent with CreateTemplateModal.
- **D-5**: No backend changes are expected; if the architect prefers a server-side export endpoint for atomicity, the file format defined here is unchanged.

## 10. Open Questions for Architect

1. **Multi-version import mechanics**: creating the template with the *earliest* version then updating through to the latest produces correct history order but N+1 API calls. Alternative: create with latest content only and accept a single version. Which trade-off do you prefer? (Design assumes full history via sequential updates, capped at 50 versions.)
2. **Change notes on import**: if the versioning feature adds `change_note`, should imported versions replay their original change notes? (Design assumes yes.)
3. **File size cap**: is 1 MB a reasonable client-side cap given content column limits and typical version counts?
4. **Admin export**: confirm admins may fetch any template's versions via existing admin endpoints; if a dedicated admin read path is needed, flag it.
