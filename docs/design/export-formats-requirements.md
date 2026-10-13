# Export Formats — Requirements Analysis

- **Feature ID:** export-formats
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-13
- **Companion doc:** `export-formats-ui-ux-design.md`

## 1. Competitor Research

| Product | Export capability | Strength | Weakness |
|---|---|---|---|
| PromptBase | "Copy prompt" button on marketplace listings | One-click consumption | No structured formats; no metadata |
| FlowGPT | Copy-to-clipboard on prompt detail | Frictionless | Plain text only; variables not delimited |
| AIPRM | JSON export/import for team portability | Structured, metadata-rich | Machine format only; not human-consumable |
| LangFlow | JSON flow export; Python/API snippets | Developer-oriented variety | Overwhelming for non-technical users |
| GitHub Gists | Raw file in any format | Universal | No prompt-specific structure |
| ChatGPT (custom prompts) | Plain-text copy | Simple | No structure, no variables |

**Takeaway:** Competitors split into two camps: frictionless-but-unstructured copy (PromptBase/FlowGPT) or structured-but-machine-only export (AIPRM/LangFlow). Nobody offers both: human-readable formats (Markdown for docs, YAML for config-driven tools, plain text for chat) alongside the existing JSON portability format. open-prompts already owns the JSON round-trip (import-export); adding consumption formats completes the picture.

## 2. Problem Statement

Users share prompts in three real-world contexts: pasting into a chat (needs clean text with delimited variables), embedding in docs/READMEs (needs Markdown with metadata), and wiring into config-driven tools like n8n/LangChain (needs YAML). Today they must hand-transform the JSON export or manually clean the rendered page. The template detail page's existing copy action copies raw content only, without variable delimiters, metadata, or format structure.

## 3. Target Users

- **Chat users** — paste a ready-to-fill prompt into any AI chat, variables clearly delimited.
- **Documentarians** — drop a prompt into a README, wiki, or team doc with its metadata.
- **Automation builders** — feed prompts into n8n, LangChain, or CI configs via YAML.
- **Template creators** — share their work in the format the consumer wants.

## 4. User Stories

- **US-1**: As a chat user, I want to copy a template as plain text with `{{variables}}` delimited so I can paste and fill it in any chat.
- **US-2**: As a documentarian, I want to download a template as a Markdown file with title, description, tags, and content so I can drop it into a README.
- **US-3**: As an automation builder, I want to download a template as YAML so I can mount it into a config-driven tool.
- **US-4**: As a user, I want a format picker on the template detail page so I can choose the right format in place.
- **US-5**: As a user, I want a live preview of the formatted output before copying/downloading so I know exactly what I'll get.
- **US-6**: As a user, I want to choose which version to export (latest or a specific historical version) so I can share a stable snapshot.
- **US-7**: As a creator, I want the export to respect my template's visibility so private templates aren't exported by non-owners.
- **US-8**: As a keyboard user, I want the format picker reachable via existing shortcuts so I don't break my flow.
- **US-9**: As a user on mobile, I want copy-to-clipboard formats available so downloads aren't my only option.

## 5. Functional Requirements

- **FR-1**: The template detail page MUST offer an "Export" menu with four formats: **Markdown (.md)**, **YAML (.yaml)**, **Plain text (.txt)**, and **JSON (.json)** (the existing import-export format, unchanged).
- **FR-2**: All formats MUST be generated client-side from existing `getTemplate` (and `getTemplateVersion` when a specific version is chosen) responses; no new backend endpoints.
- **FR-3**: **Plain text** MUST contain the template content with variables in `{{variable}}` form, followed by a blank line and a `Variables: a, b, c` line when variables exist; no metadata headers.
- **FR-4**: **Markdown** MUST contain: an H1 title, description as italic paragraph, tags as a bullet list, `**Type:**`/`**Category:**`/`**Language:**` metadata lines, an `---` separator, then the content in a fenced code block with variables in `{{variable}}` form, and a trailing `> Exported from open-prompts · <date>` line.
- **FR-5**: **YAML** MUST contain top-level keys `title`, `description`, `type`, `category`, `language`, `tags` (list), `variables` (list of names), `version` (chosen version number), `exported_at` (ISO-8601), and `content` as a block scalar (`|`) preserving line breaks.
- **FR-6**: **JSON** MUST be byte-identical in schema to the import-export FR-2 format (format identifier `open-prompts-template`, format version `1`), including full version history when exporting the latest version.
- **FR-7**: Each format MUST open a preview modal showing the exact output in a read-only code view with line numbers, a **Copy** button, and a **Download** button.
- **FR-8**: Download filenames MUST follow the import-export FR-4 slug rule: `<slug>-<id8>.<ext>` (e.g. `cold-email-template-a1b2c3d4.md`).
- **FR-9**: The preview modal MUST include a version selector defaulting to the latest version; choosing an older version regenerates the preview for all formats.
- **FR-10**: Copy MUST use the Clipboard API with a fallback to `document.execCommand('copy')` for older browsers, and MUST show a "Copied" confirmation.
- **FR-11**: Export MUST be available to any authenticated user for public templates, and to the owner only for private templates (same access rule as viewing).
- **FR-12**: The export MUST NOT include owner identity (user id, email, display name) in any format, consistent with import-export FR-6.
- **FR-13**: The Markdown/YAML/plain-text formats MUST include only the chosen version's content (not version history); only JSON carries history.
- **FR-14**: The Export menu MUST be reachable via keyboard (Tab order, Enter/Space to open, arrow keys to pick a format) and integrate with the keyboard-shortcuts help panel as `E` when the template detail page is focused.
- **FR-15**: Generation failures (e.g. clipboard permission denied) MUST show an inline error in the modal with a retry; download failures fall back to opening the data in a new tab.
- **FR-16**: The four formats MUST be offered on the admin template detail page (`/admin/templates/:id`) identically, for support workflows.

## 6. Non-Functional Requirements

- **NFR-1 (Performance)**: Format generation for a 100 KB template completes in < 50 ms client-side; preview renders without perceptible lag.
- **NFR-2 (Fidelity)**: Content is never altered — only wrapped/delimited. A round-trip (export plain text → paste into chat) preserves every character of the content body.
- **NFR-3 (Determinism)**: The same template + version + format always produces byte-identical output (stable key order in YAML/JSON, stable date format).
- **NFR-4 (Compatibility)**: YAML output is valid YAML 1.2; Markdown renders correctly in GitHub-flavored Markdown; plain text is UTF-8 with LF line endings.
- **NFR-5 (Security)**: Content is escaped correctly per format (YAML block scalars, Markdown code fences use ``` when content contains ```), preventing format injection.

## 7. Acceptance Criteria

- **AC-1**: Opening Export → Markdown on a public template shows a preview with H1 title, metadata lines, fenced content, and the trailing exported-from line; Copy places exactly that text on the clipboard.
- **AC-2**: Export → YAML produces parseable YAML with `content: |` block scalar preserving all line breaks; `variables` lists every `{{name}}` found in content.
- **AC-3**: Export → Plain text yields content + `Variables: a, b, c` line only; no title or metadata.
- **AC-4**: Export → JSON matches the import-export format exactly; importing it back into open-prompts succeeds.
- **AC-5**: Selecting version 3 of 6 in the preview regenerates all four formats to show version 3's content and `version: 3` in YAML.
- **AC-6**: A non-owner opening a private template gets no Export menu (404 page, same as today).
- **AC-7**: Downloading Markdown produces `cold-email-template-a1b2c3d4.md` (slug + id8 + extension).
- **AC-8**: Clipboard permission denied shows an inline error with retry; the preview text remains selectable for manual copy.
- **AC-9**: Pressing `E` on the template detail page opens the Export menu; arrow keys + Enter select a format without a mouse.
- **AC-10**: Content containing triple backticks exports as Markdown with a four-backtick outer fence, rendering correctly on GitHub.

## 8. Out of Scope

- Import from Markdown/YAML/plain text (import remains JSON-only, per import-export).
- Bulk export of multiple templates (deferred; import-export line 104 already defers bulk).
- PDF/DOCX/image export; custom user-defined templates for output shape.
- Server-side export endpoints; share links that render a format (template-qr-share owns link formats).
- Monetization of any kind — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1**: import-export's FR-2 JSON schema is the canonical portability format; this feature reuses it verbatim (FR-6).
- **D-2**: `getTemplate` and `getTemplateVersion` responses contain all fields needed (title, description, type, category, language, tags, content, variables extracted from content).
- **D-3**: Variable extraction follows the existing `{{variable}}` convention used across the product (variable-presets, template-health-hints).
- **D-4**: The template detail page has an existing copy action; this feature replaces/extends it with the Export menu (UI decision in companion doc).
- **D-5**: No backend changes are expected; if the architect prefers a server-side endpoint for YAML/Markdown, the format definitions here are unchanged.

## 10. Open Questions for Architect

- **OQ-1**: Should the JSON export from this menu reuse the import-export client-side assembly verbatim (shared code module), or is a shared "format registry" preferable?
- **OQ-2**: Is client-side variable extraction (regex `{{...}}`) acceptable, or should the backend expose a parsed variables list to avoid drift with variable-presets?
- **OQ-3**: For YAML, should `tags` be a flow list `[a, b]` or block list? (Proposal: block list for readability.)
- **OQ-4**: Should the admin console export include extra admin-only metadata (report counts, moderation status), or stay identical to user export?
- **OQ-5**: Does the `E` shortcut conflict with any existing binding on the template detail page?
