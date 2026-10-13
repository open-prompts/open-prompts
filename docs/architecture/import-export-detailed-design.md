# Template Import/Export — Detailed Design

| | |
|---|---|
| **Feature ID** | FE-010 |
| **Version** | 1.0 |
| **Status** | Approved |
| **Author** | architect-agent |
| **Date** | 2026-10-12 |
| **Architecture** | `docs/architecture/import-export-architecture.md` |

## 1. Backend Changes

The only backend change is enriching the admin template detail read path. No user-facing endpoint, DDL, or service changes.

### 1.1 Proto diff (`backend/api/proto/v1/admin.proto`)

```protobuf
message AdminTemplateSummary {
  string id = 1;
  string title = 2;
  string visibility = 3;   // "public" | "private"
  string type = 4;         // "system" | "user"
  string language = 5;
  repeated string tags = 6;
  int32 like_count = 7;
  int32 favorite_count = 8;
  google.protobuf.Timestamp created_at = 9;
  google.protobuf.Timestamp updated_at = 10;
  string owner_id = 11;
  string owner_display_name = 12;
  string category = 13;    // NEW — required for admin export parity (FR-2)
}

message AdminTemplateVersion {
  int32 id = 1;
  int32 version = 2;
  string content = 3;
  google.protobuf.Timestamp created_at = 4;
  string alias_name = 5;   // "" if no alias points at this version
  string change_note = 6;  // NEW — "" when the version has none
}
```

Field numbers 13 / 6 are the next free slots (verified against current proto). Both are `string` scalars — no protojson int64 concerns.

### 1.2 Model diff (`backend/internal/models/admin.go`)

```go
type AdminTemplate struct {
    // ... existing fields unchanged ...
    Category string `json:"category"` // NEW
}

type AdminTemplateVersion struct {
    // ... existing fields unchanged ...
    ChangeNote sql.NullString `json:"change_note"` // NEW
}
```

### 1.3 Repository diff (`backend/internal/repository/admin_repository.go`)

```go
// adminTemplateColumns: append t.category
const adminTemplateColumns = `t.id, t.owner_id, COALESCE(u.display_name, ''), t.title, t.description, t.visibility, t.type, t.language, t.tags, t.like_count, t.favorite_count, t.category, t.created_at, t.updated_at`
```

`GetTemplateWithVersions` version query: add `tv.change_note` to the SELECT and scan into `v.ChangeNote`. The `templates.category` and `template_versions.change_note` columns already exist (change_note added idempotently by the template-versioning migration, `init.sql` ALTER).

### 1.4 Service diff (`backend/internal/service/admin.go`)

- `templateToSummary`: add `Category: t.Category`.
- `GetTemplate` version mapping: `ChangeNote: version.ChangeNote.String` (NULL → `""`, consistent with `alias_name` handling).

### 1.5 Gateway

No changes. The admin handler at `/api/v1/admin/templates/{id}` (GET, `requireAdmin`) already protojson-marshals `GetAdminTemplateResponse`; new fields flow through automatically.

## 2. Frontend Utility — `frontend/src/utils/templateExport.js`

Pure functions, no React imports; unit-testable in isolation (pattern: `utils/diff.js` + `diff.test.js`).

### 2.1 Constants

```js
export const EXPORT_FORMAT = 'open-prompts-template';
export const EXPORT_FORMAT_VERSION = 1;
export const MAX_IMPORT_FILE_SIZE = 1024 * 1024; // 1 MB
export const MAX_IMPORT_VERSIONS = 50;
export const MAX_CHANGE_NOTE_LENGTH = 200;
export const MAX_TITLE_LENGTH = 200;
export const MAX_TAGS = 20;
export const MAX_TAG_LENGTH = 50;
```

### 2.2 `parseTemplateExport(file: File): Promise<ParsedExport>`

Returns `{ data, warnings }` where `data` is the validated export object (versions chronological, possibly truncated to 50) and `warnings` is an array of `{ code, params }` (currently only `truncated_versions`). Throws `ImportValidationError` (subclass of `Error` with `code` and optional `params`) on any FR-11 violation.

Validation order (each step short-circuits):

| Step | Check | Error code |
|---|---|---|
| 1 | `file.size > MAX_IMPORT_FILE_SIZE` | `too_large` |
| 2 | `await file.text()` then `JSON.parse` in try/catch | `not_json` |
| 3 | `data.format === EXPORT_FORMAT` and `data.template` is an object | `bad_format` |
| 4 | `data.format_version === EXPORT_FORMAT_VERSION` | `unsupported_version` (params: `{ version }`) |
| 5 | `template.title` non-empty string | `missing_field:title` |
| 6 | `Array.isArray(template.versions)` and `versions.length >= 1` | `missing_field:versions` |
| 7 | every version has non-empty string `content` | `missing_field:versions[{i}].content` |
| 8 | `template.title.length <= MAX_TITLE_LENGTH` | `invalid_field:title` |
| 9 | `template.tags` (if present) ≤ `MAX_TAGS` entries, each ≤ `MAX_TAG_LENGTH` chars | `invalid_field:tags` |
| 10 | `versions.length > MAX_IMPORT_VERSIONS` → keep last 50, push warning `truncated_versions` (params: `{ kept: 50, total }`) | — (warning, not error) |

Normalization applied to the returned `data`:

- `description` → `String(description ?? '')`
- `tags` → array of strings, deduplicated, empty array if absent
- `category` → string (may be `''`; the review form requires selecting one)
- `language` → string, default `'en'` if absent
- `visibility` → kept as-is for display; **importer ignores it** (forced private)
- `type` → kept as-is for display; **importer ignores it** (forced `user`)
- `versions` → sorted by `version` ascending; each `change_note` normalized to `String(change_note ?? '')`
- `versions[i].created_at` → informational only, never replayed

### 2.3 `buildTemplateExport(template, versions): string`

- `template`: shape from `getTemplate` (user path) or `getAdminTemplate` summary + description (admin path) — the caller flattens admin shapes into the same field set before calling.
- `versions`: chronological array of `{ version, content, change_note, created_at }`.
- Output: `JSON.stringify(obj, null, 2) + '\n'` where `obj` follows the format-version-1 schema (architecture §5). Field whitelist only — owner identity is structurally impossible to include.
- `exported_at`: `new Date().toISOString()`; `source`: `'open-prompts'`.

### 2.4 `downloadJSON(filename: string, text: string): void`

```js
const blob = new Blob([text], { type: 'application/json' });
const url = URL.createObjectURL(blob);
const a = document.createElement('a');
a.href = url;
a.download = filename;
document.body.appendChild(a);
a.click();
a.remove();
URL.revokeObjectURL(url);
```

### 2.5 `slugifyTitle(title: string, id: string): string`

1. `title.toLowerCase().normalize('NFKD')`, strip combining marks, replace non-`[a-z0-9]+` runs with `-`, trim `-`.
2. Cap at 60 chars (cut at a `-` boundary if possible).
3. `id8 = id.replace(/-/g, '').slice(0, 8)`.
4. If the slug is empty → `template`; return `` `${slug}-${id8}` `` (e.g. `my-prompt-template-a1b2c3d4`).

### 2.6 `fetchVersionsChronological(templateId): Promise<Version[]>`

Helper for the user export path: loop `listTemplateVersions(templateId, { page_size: 50, page_token })` while `next_page_token` is non-empty; concatenate `versions`; reverse. (The API returns newest-first; the file must be earliest-first.)

## 3. Frontend Components

### 3.1 `components/ExportButton.js`

```jsx
props: { template, dataSource? }   // dataSource: 'user' (default) | 'admin'
state: exporting: boolean
```

- Renders `.btn-ghost .btn-sm` with `⬇` icon + `t('import_export.export')`; `disabled` + spinner while `exporting`.
- **User path** (`TemplateDetails.js`, mounted only when `isOwner`): `Promise.all([getTemplate(template.id), fetchVersionsChronological(template.id)])` → `buildTemplateExport` → `downloadJSON(slugifyTitle(...))` → success toast `t('import_export.export_success', { title, count })`.
- **Admin path** (`admin/TemplateDetailPage.js`): `getAdminTemplate(template.id)` → flatten `{ ...res.data.template, description: res.data.description, versions: [...res.data.versions].reverse() }` → same build/download/toast.
- Failure: error toast `t('import_export.export_failed')`; button re-enables. Errors are caught; no unhandled rejections.

### 3.2 `components/ImportButton.js`

- `.btn-ghost .btn-sm` with `⬆` icon + `t('import_export.import')`; opens `ImportModal`.
- Mounted in `Home.js` `home-search-row` (after `SortDropdown`), rendered only when `isAuthenticated` (Home already selects it from auth state).

### 3.3 `components/ImportModal.js`

Two-step state machine: `step: 'pick' | 'review'`.

**Step 1 — pick**

- `DropZone`: dashed `--border-strong` border, `--radius-12`; drag-over → `--accent` border + `--surface-2` fill; hidden `<input type="file" accept=".json,application/json">`; `tabIndex={0}`, `role="button"`, Enter/Space opens picker (NFR-5/AC-11).
- `onFile(file)`: `setParsing(true)` → `parseTemplateExport(file)`:
  - throws → `setValidationError(err)` (rendered in the error slot with the mapped i18n string; raw exception never shown), stay on step 1 (FR-14).
  - resolves → `setParsed({ data, warnings })`, advance to step 2.
- Non-`.json` drop: shake animation (respect `prefers-reduced-motion`) + `bad_format` error.
- Cancel/Escape closes (step 1 has nothing to discard).

**Step 2 — review**

- Summary line: `t('import_export.review_summary', { file, count })`; if `warnings` contains `truncated_versions`, append the truncation warning string.
- Form fields (reuse `Field`/`TagInput` components and `create_template.*` labels, NFR-1):
  - Title* — pre-filled from file; **collision check** on entry: `getTemplates({ owner_id: user.id, page_size: 100 })`, case-insensitive compare against first page; on match show `t('import_export.title_collision')` + "Use suggested title" button that sets `` `${title} (imported)` `` (localized suffix via `import_export.suggested_suffix`). Non-blocking (FR-12/AC-8).
  - Category* — pre-filled if the file's category exists in `getCategories()`; otherwise empty (user must pick; same required validation as create form).
  - Visibility — radio, **default Private** regardless of file (FR-10); user may switch to Public.
  - Language — pre-filled from file.
  - Tags — pre-filled from file via `TagInput`.
  - Description — pre-filled.
  - Content* — textarea pre-filled with the **latest** version's content (editable; it seeds the final replay step's last update).
- Back → returns to step 1 keeping parsed data (no re-drop needed).
- Escape/Cancel with dirty form → confirm-discard prompt.
- Submit (`t('import_export.confirm')`, spinner while running):

```js
const { data } = parsed;
const versions = data.template.versions;           // chronological, ≤ 50
const base = {
  title, description, category, tags, language,    // reviewed values
  visibility,                                      // reviewed ('private' default)
  type: 'user',                                    // forced
};
// 1. create with EARLIEST content (architecture §7.1)
const created = await createTemplate({ ...base, content: versions[0].content });
const newId = created.data.template.id;
// 2. replay v2..vN, earliest → latest
for (let i = 1; i < versions.length; i++) {
  await updateTemplate(newId, {
    ...base,
    content: i === versions.length - 1 ? reviewedContent : versions[i].content,
    change_note: versions[i].change_note.slice(0, 197),  // defensive (service cap 200)
  });
}
// 3. navigate + toast
navigate(`/templates/${newId}`);
addNotification({ kind: 'success', subtitle: t('import_export.import_success') });
```

  - Note: the reviewed `content` field replaces the latest version's content in the final replay call, so user edits to content in the review form are honored.
  - Mid-replay failure: stop, toast `import_export.import_partial` (new key, params `{ imported, total }`), keep the modal open with the error; the created template remains (private) and the user can navigate to it manually.
  - Create failure: form-level error (same handling as `CreateTemplateModal`).

### 3.4 Mount points (exact)

| File | Change |
|---|---|
| `pages/TemplateDetails.js` | In the owner actions row (next to Edit/Fork): `{isOwner && <ExportButton template={template} />}` |
| `pages/admin/TemplateDetailPage.js` | Header actions: `<ExportButton template={template} dataSource="admin" />` |
| `pages/Home.js` | `<div className="home-search-row">` append `{isAuthenticated && <ImportButton />}` |

## 4. i18n Keys

Namespace `import_export.*` (new), added to both `frontend/src/locales/en/translation.json` and `frontend/src/locales/zh/translation.json` (verified layout). Review-form field labels reuse existing `create_template.*` keys.

**EN**

```json
{
  "import_export": {
    "export": "Export",
    "export_success": "Exported \"{{title}}\" ({{count}} versions)",
    "export_failed": "Export failed. Please try again.",
    "import": "Import",
    "import_title": "Import template",
    "drop_hint": "Drag and drop a .json file here, or",
    "browse": "Browse files",
    "drop_note": "Only open-prompts template exports (.json, max 1 MB) are supported.",
    "error_not_json": "This file is not a valid JSON file.",
    "error_too_large": "File is too large. Maximum size is 1 MB.",
    "error_bad_format": "This file is not an open-prompts template export.",
    "error_unsupported_version": "This export uses a newer format version ({{version}}) that this app cannot read.",
    "error_missing_field": "The file is missing a required field: {{field}}.",
    "error_invalid_field": "The file has an invalid value for: {{field}}.",
    "warning_truncated_versions": "Only the most recent {{kept}} of {{total}} versions will be imported.",
    "review_summary": "{{file}} — {{count}} versions will be imported as sequential versions.",
    "title_collision": "You already have a template with this title.",
    "suggested_suffix": "(imported)",
    "use_suggested": "Use suggested title",
    "confirm": "Import template",
    "import_success": "Template imported successfully.",
    "import_partial": "Imported {{imported}} of {{total}} versions. The template was created; you can retry or edit it manually."
  }
}
```

**ZH**

```json
{
  "import_export": {
    "export": "导出",
    "export_success": "已导出“{{title}}”（{{count}} 个版本）",
    "export_failed": "导出失败，请重试。",
    "import": "导入",
    "import_title": "导入模板",
    "drop_hint": "将 .json 文件拖放到此处，或",
    "browse": "浏览文件",
    "drop_note": "仅支持 open-prompts 模板导出文件（.json，最大 1 MB）。",
    "error_not_json": "该文件不是有效的 JSON 文件。",
    "error_too_large": "文件过大，最大为 1 MB。",
    "error_bad_format": "该文件不是 open-prompts 模板导出文件。",
    "error_unsupported_version": "此导出文件使用了较新的格式版本（{{version}}），当前应用无法读取。",
    "error_missing_field": "文件缺少必填字段：{{field}}。",
    "error_invalid_field": "文件的以下字段值无效：{{field}}。",
    "warning_truncated_versions": "将仅导入最近 {{kept}} 个版本（共 {{total}} 个）。",
    "review_summary": "{{file}} — 将导入 {{count}} 个版本（按顺序生成）。",
    "title_collision": "你已存在同名模板。",
    "suggested_suffix": "（导入）",
    "use_suggested": "使用建议标题",
    "confirm": "导入模板",
    "import_success": "模板导入成功。",
    "import_partial": "已导入 {{imported}} / {{total}} 个版本。模板已创建；你可以重试或手动编辑。"
  }
}
```

Keys added beyond the UI/UX design doc: `error_invalid_field`, `warning_truncated_versions`, `suggested_suffix`, `import_partial` (all required by decisions in the architecture doc).

## 5. Error Matrix

| Scenario | Layer | Code / HTTP | User-visible result |
|---|---|---|---|
| File > 1 MB | client | `too_large` | Step-1 inline error, stays on step 1 |
| Non-JSON file | client | `not_json` | Step-1 inline error |
| Wrong `format` / missing `template` | client | `bad_format` | Step-1 inline error |
| `format_version` ≠ 1 | client | `unsupported_version` | Step-1 inline error with version number |
| Missing title / versions / content | client | `missing_field:{field}` | Step-1 inline error naming the field |
| Title > 200 / tags > 20 / tag > 50 | client | `invalid_field:{field}` | Step-1 inline error |
| > 50 versions | client | warning `truncated_versions` | Step-2 summary warning, import proceeds |
| Title collision | client | — | Step-2 non-blocking warning + suggested title |
| Create fails (validation) | server | 400 via `writeError` | Step-2 form error |
| Create fails (auth) | server | 401 | Global 401 handler (redirect login) |
| Replay update fails | server | 4xx/5xx via `writeError` | `import_partial` toast; partial history kept |
| Export fetch fails | server | 4xx/5xx | `export_failed` toast, button resets |
| Admin export on non-admin | server | 403 (`requireAdmin`) | Not reachable — button only in admin console |

## 6. Test Plan

### 6.1 Unit — `utils/templateExport.test.js`

- Round-trip: `buildTemplateExport` → `parseTemplateExport` (via `new File([json])`) preserves all fields.
- Each validation row of §2.2 (10 steps) with minimal fixtures.
- Truncation: 60-version file → 50 kept, warning present, kept = last 50.
- `slugifyTitle`: normal title; 100-char title caps at 60; all-CJK title → `template-{id8}`; empty title.
- `change_note` normalization: `null` → `''`; 300-char note → 197 + `…` on replay payload construction.

### 6.2 FVT — backend admin service

- `GetAdminTemplate` returns `category` on the summary and `change_note` per version; NULL change_note → `""`.
- Existing admin template tests keep passing (additive fields).

### 6.3 E2E — round trip

1. Owner creates template, edits content twice (3 versions with change notes) → Export → file downloads with 3 chronological versions.
2. Second account: Home → Import → file → review shows fields, visibility=Private → confirm → lands on details → version history shows 3 versions, same contents/notes, visibility private, type user.
3. Non-owner on a public template: no Export button.
4. Admin: `/admin/templates/:id` → Export → identical structure (diff against user export of same template, ignoring `exported_at`).
5. Corrupt file (`.png` renamed), 2 MB file, `format_version: 99` file → specific step-1 errors, no crash.

## 7. Implementation Order

1. Backend: proto fields → `make generate` (or project's proto gen) → model/repo/service mapping → FVT.
2. `utils/templateExport.js` + unit tests.
3. `ExportButton` (user + admin mounts).
4. `ImportButton` + `ImportModal` + Home mount.
5. i18n keys EN/ZH.
6. E2E round-trip; visual check light/dark (AC-12).

## 8. Developer Constraints

- **Do not** add any new REST endpoint, RPC, or DDL for this feature.
- **Do not** change `UpdateTemplate` semantics; the replay relies on current append-only behavior.
- **Do not** include owner identity in the export builder — field whitelist only.
- **Do not** trust the file's `visibility`/`type` on import — force `private`/`user`.
- Replay order is **earliest-first**; the UI/UX design doc's note 6 is superseded (see architecture §7.1).
- The ungated `/versions` endpoint hardening (architecture §9) is a **separate follow-up** — do not bundle it into this feature's PRs.
- Proto field numbers: `AdminTemplateSummary.category = 13`, `AdminTemplateVersion.change_note = 6` — do not renumber.
