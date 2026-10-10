# Template Versioning UX — Detailed Design

- **Feature ID:** `template-versioning`
- **Version:** 1.0
- **Status:** Approved for implementation
- **Author:** Architect Agent
- **Upstream docs:** [Requirements](../design/template-versioning-requirements.md) · [UI/UX Design](../design/template-versioning-ui-ux-design.md)
- **Architecture doc:** [Architecture Design (EN)](./template-versioning-architecture.md) · [架构设计（中文）](./template-versioning-architecture-zh-CN.md)

---

## 1. Proto Contract (additive changes to `prompt.proto`)

```protobuf
// TemplateVersion represents a specific version of a template's content.
message TemplateVersion {
  int32 id = 1;
  string template_id = 2;
  int32 version = 3;
  string content = 4;
  google.protobuf.Timestamp created_at = 5;
  // Optional user-provided change note (empty string when NULL).
  string change_note = 6;
  // Number of prompts pinned to this version (populated by
  // ListTemplateVersions; 0 elsewhere and to be ignored).
  int32 prompt_count = 7;
}

message UpdateTemplateRequest {
  string template_id = 1;
  string owner_id = 2;
  string title = 3;
  string description = 4;
  Visibility visibility = 5;
  repeated string tags = 6;
  string category = 7;
  string content = 8;
  string language = 9;
  // Optional change note for the new version (max 200 chars).
  // Ignored when content is unchanged (no version is created).
  string change_note = 10;
}
```

`CreateTemplateRequest` is **unchanged** (open question #3: v1 carries no note). `ListTemplateVersionsResponse` is unchanged — the repeated `TemplateVersion` message itself gains the fields. Regenerate `prompt.pb.go`/`prompt_grpc.pb.go` with the repo's standard protoc invocation and commit them (repo convention).

**Gateway: zero changes.** The PUT handler already unmarshals the full JSON body into `UpdateTemplateRequest` (so `change_note` flows automatically), and the versions GET handler already passes `page_size`/`page_token` through. `protojson` with `UseProtoNames` emits `change_note`/`prompt_count` in snake_case automatically.

## 2. Database DDL

Append to **both** `deployment/init.sql` and `deployment/chart/open-prompts/init.sql` (byte-identical block), following the repo's established idempotent patterns:

```sql
-- -----------------------------------------------------------------------------
-- Template versioning UX: change notes + prompt usage counts
-- -----------------------------------------------------------------------------
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='template_versions' AND column_name='change_note') THEN
        ALTER TABLE template_versions ADD COLUMN change_note TEXT;
        COMMENT ON COLUMN template_versions.change_note IS 'Optional user-provided change note (max 200 chars, enforced in service layer)';
    END IF;
END $$;

-- Supports the prompt_count LEFT JOIN in ListTemplateVersions
CREATE INDEX IF NOT EXISTS idx_prompts_version_id ON prompts(version_id);
```

Notes:

- Nullable, no backfill: existing rows stay NULL and render as "No note".
- Length cap (≤ 200) is enforced in the service layer, not a DB CHECK — consistent with repo precedent (visibility/type, role/status are service-validated).
- Deploy DDL before or together with the backend (new SELECTs reference the column).

## 3. Backend Layers

### 3.1 Model (`backend/internal/models/template.go`)

```go
type TemplateVersion struct {
    ID         int32          `json:"id"`
    TemplateID string         `json:"template_id"`
    Version    int32          `json:"version"`
    Content    string         `json:"content"`
    ChangeNote sql.NullString `json:"change_note"` // NULL-able; empty string in proto
    CreatedAt  time.Time      `json:"created_at"`
    PromptCount int           `json:"prompt_count"` // computed at read time, not stored
}
```

### 3.2 Repository (`backend/internal/repository/template_version_repository.go`)

All queries add `change_note`; `List` becomes the count join (open question #1: inline LEFT JOIN):

```sql
-- Create (INSERT gains the column)
INSERT INTO template_versions (template_id, version, content, change_note, created_at)
VALUES ($1, $2, $3, $4, $5) RETURNING id;

-- Get / GetLatest (SELECT gains the column)
SELECT id, template_id, version, content, change_note, created_at
FROM template_versions WHERE ...;

-- List (new shape: LEFT JOIN + COUNT, GROUP BY version PK)
SELECT tv.id, tv.template_id, tv.version, tv.content, tv.change_note, tv.created_at,
       COUNT(p.id) AS prompt_count
FROM template_versions tv
LEFT JOIN prompts p ON p.version_id = tv.id
WHERE tv.template_id = $1
GROUP BY tv.id
ORDER BY tv.version DESC
LIMIT $2 OFFSET $3;
```

`COUNT(p.id)` yields 0 for versions with no prompts (LEFT JOIN semantics). The `idx_prompts_version_id` index supports the join. Scan `change_note` into `sql.NullString` and `prompt_count` into the computed field.

### 3.3 Service (`backend/internal/service/prompt.go`)

**`UpdateTemplate`** — two additions to the existing flow:

1. Validation, before any write: `if len(req.ChangeNote) > 200 { return nil, status.Errorf(codes.InvalidArgument, "change_note must be at most 200 characters") }`. (Byte-length check; the client maxlength uses UTF-16 code units, which is ≥ byte length for the same string, so the server check is the binding one for multibyte input.)
2. When creating the new version (content-changed path only), set `ChangeNote: sql.NullString{String: req.ChangeNote, Valid: req.ChangeNote != ""}`. On the unchanged-content path the note is ignored and **not** written to the existing latest version (FR-2/FR-18) — today's early-return already covers this; just don't touch it.

**`versionModelToProto`** — map the new fields:

```go
changeNote := ""
if m.ChangeNote.Valid {
    changeNote = m.ChangeNote.String
}
return &pb.TemplateVersion{
    ...,
    ChangeNote:  changeNote,
    PromptCount: int32(m.PromptCount),
}
```

**`ListTemplateVersions`** — no logic change; the repo now returns counts and `versionModelToProto` maps them. `GetTemplate`'s `latest_version` gains `change_note` automatically (its `GetLatest` SELECT now includes the column); `prompt_count` is 0 there and documented as ignorable.

**Restore needs no backend work** — it is a normal `UpdateTemplate` call whose content is an old version's content and whose note is the canonical English string `Restored from v{n}` (stored data, never translated at render; NFR-5/D-7).

### 3.4 Gateway (`backend/cmd/server/main.go`)

**Zero changes.** (Verified: the PUT handler unmarshals the whole body; the versions GET handler passes pagination through; `protojson` emits new fields automatically.)

## 4. Frontend Design

### 4.1 File layout

```
frontend/src/
├── components/
│   ├── VersionHistoryPanel.js   # panel + inline VersionRow
│   ├── DiffModal.js
│   ├── RestoreConfirmModal.js
│   ├── ChangeNoteInput.js
│   └── VersionSelect.js         # modified: note snippet sub-line
├── pages/TemplateDetails.js     # modified: toggle, pagination, restore, diff wiring
├── utils/
│   ├── diff.js                  # LCS line diff (new)
│   └── time.js                  # formatRelativeTime (new, shared)
└── locales/{en,zh}/translation.json  # versioning.* + time.* keys
```

### 4.2 Diff util (`utils/diff.js`)

Hand-rolled LCS on lines (open question #2), normalized output:

```js
// diffLines(aText, bText) => [{ type: 'add'|'remove'|'context', text, aLine, bLine }]
```

- Split on `\n`, LCS table on lines (content ≤ 32 KB → line counts are small; O(n·m) on lines is trivial).
- Walk the table to emit removes (with `aLine`), adds (with `bLine`), context (both).
- Export `diffStats(rows)` → `{ added, removed }` for the stats chip.
- Wrap the call site in try/catch; on any throw the modal renders the side-by-side fallback (NFR-6).

### 4.3 TemplateDetails state additions

```js
const [isHistoryOpen, setIsHistoryOpen] = useState(false);
const [historyPageToken, setHistoryPageToken] = useState('');
const [diffState, setDiffState] = useState(null); // { aId, bId } while modal open
const [editNote, setEditNote] = useState('');      // change note in edit mode
```

- **Version fetch:** replace `listTemplateVersions(id, { page_size: 100 })` with `page_size: 20`; "Load more" appends via `next_page_token`. The dropdown consumes the accumulated list (unchanged UX).
- **Selection:** one `setSelectedVersionId` handler remains the source of truth for panel rows, dropdown changes, and post-restore selection.
- **Save flow (`handleSaveContent`):** include `change_note: editNote` in the update payload; clear `editNote` on success. `handleSaveMetadata` does **not** send a note (it sends the latest content; no version is created).
- **Restore flow:** open `RestoreConfirmModal` with the source version; on confirm call `updateTemplate(template.id, { ...currentMetadata, content: version.content, change_note: \`Restored from v${version.version}\` })`; on success re-fetch page 1 of versions, select `new_version.id`, toast `versioning.toast.restored`. Pessimistic (spinner, disabled buttons) — no optimistic version insertion.
- **Refresh after write:** save and restore both re-fetch the first page so numbering and the Latest badge are server-truthful.

### 4.4 Components

**VersionHistoryPanel** — header ("Version history" + Close); rows newest-first: `v{n}` (mono), Latest badge (newest only), note or "No note" (italic), relative time (absolute in `title`), meta line "X chars · Y lines · Z prompts" (usage segment hidden per-response when `prompt_count` is absent), Compare + Restore actions (Restore hidden on latest row and for non-owners). Row click selects the version. `<ul>` of button-rows; actions are separate buttons with `aria-label`s ("Compare v4", "Restore v3"). States: 4 skeleton rows while loading; single-version hint; error + Retry.

**DiffModal** — A (older) / B (newer) `VersionSelect`-style selectors, Swap button, stats chip "+N −M lines", unified diff body (line number, sign marker, content; removed = danger-soft bg + `−`, added = success-soft bg + `+`, context plain; monospace 13px `pre-wrap`, max-height 60vh), Copy diff, Close. Diff recomputed on selector change or Swap; A/B normalized older→newer. Fallback: side-by-side raw panes + notice. Existing Modal `size="lg"`, focus trap, Escape.

**RestoreConfirmModal** — body per UI/UX §5.5 ("Restore v{n} as a new version (v{next})? … note \"Restored from v{n}\"."); Cancel / "Restore v{n}"; in-flight disables both.

**ChangeNoteInput** — label "Change note (optional)", input maxlength 200, live counter "n/200" (`aria-live="polite"`, warning color at 180+), placeholder "e.g., Fixed tone drift". Sits between the content textarea and the action buttons.

**VersionSelect (enhancement)** — option sub-line gains the note snippet (first ~40 chars, ellipsis) under the existing date when `change_note` is present; no other behavior change.

### 4.5 i18n keys (both locales)

`versioning.*` namespace verbatim from UI/UX doc §9 (EN and ZH JSON blocks are specified there — copy them as-is), plus the shared `time.*` keys (`justNow`, `minutesAgo`, `hoursAgo`, `daysAgo`) if not already introduced by the notifications feature. Stored notes and the auto-generated `Restored from v{n}` string are data and render verbatim in both locales.

### 4.6 API client (`services/api.js`)

**Zero changes.** `updateTemplate(id, data)` and `listTemplateVersions(id, params)` are generic pass-throughs; the new fields ride the existing payloads.

## 5. Error Matrix

| Condition | gRPC code | HTTP | User-facing |
|---|---|---|---|
| `change_note` > 200 chars | `InvalidArgument` | 400 | Error toast (`template_details.error_update`); client maxlength prevents most cases |
| Template not found (update/restore) | `NotFound` | 404 | Error toast |
| Not owner (restore) | `PermissionDenied` | 403 | Error toast; UI hides Restore for non-owners anyway |
| Unauthenticated (update/restore) | `Unauthenticated` | 401 | Global 401 → `/login` |
| Versions list failure | `Internal` | 500 | Panel error state + Retry |
| Diff computation throws (client) | — | — | Side-by-side fallback + notice (NFR-6) |
| `prompt_count` absent (old backend) | — | — | Usage column hidden (NFR-6) |

## 6. Deployment

- DDL block (§2) appended to both `init.sql` copies; idempotent; no backfill.
- No env vars, no compose/helm changes, no new services.
- Ordering: DDL before or with the backend image (new SELECTs reference `change_note`).

## 7. SDK / CLI

No changes required. The python-sdk wraps create/update/list surfaces generically; additive response fields are transparent to it. (If the SDK has typed models that enumerate fields, adding `change_note`/`prompt_count` there is optional and non-breaking — not required for this feature.)

## 8. Test Strategy

### 8.1 FVT (`tests/fvt.py`) — developer

New `test_template_versioning()` (or extend the existing versioning test):

1. Create template → update with `change_note: "first revision"` → `new_version.change_note == "first revision"`, version number incremented.
2. List versions → both versions carry `change_note` (v1 empty) and `prompt_count` (0 until prompts exist).
3. Create 2 prompts pinned to v1 → list again → v1 `prompt_count == 2`, v2 `prompt_count == 0`.
4. Update with `change_note` of 201 chars → 400.
5. Update with unchanged content + a note → no new version (count unchanged), note not stored on latest.
6. Restore: PUT with v1's content and `change_note: "Restored from v1"` → new v3 created; list shows all 3 versions intact (append-only); `latest` alias resolves to v3 (via the alias endpoint); v3 note is the canonical string.
7. Backwards compat: update without `change_note` → still works, note empty.

### 8.2 Backend unit tests

- `UpdateTemplate`: note-length validation (200 ok / 201 rejected); note persisted only on the content-changed path; unchanged-content path returns latest without writing the note.
- `versionModelToProto`: NULL → empty string; count mapping.
- Repository `List` SQL shape (mocked rows): count join returns 0 for prompt-less versions.

### 8.3 E2E (Nightwatch, test-agent)

| E2E | AC |
|---|---|
| Save with note → note appears in panel + dropdown snippet | AC-1 |
| Panel: newest-first, Latest badge, note/time/length/usage, 20/page Load more | AC-2 |
| Row click switches preview (same as dropdown) | AC-3 |
| Compare: highlighting, line numbers, stats, Swap | AC-4 |
| Restore: confirm → new version + canonical note, list refreshed, new version selected | AC-5 |
| Non-owner: panel + Compare, no Restore; anonymous on public template: read-only | AC-6 |
| Note > 200 blocked client-side (maxlength + counter) | AC-7 |
| Metadata-only save creates no version; alias flow unchanged | AC-8 |
| EN/ZH key parity, all strings translated | AC-9 |
| Template page regression: view/edit/fork/like/favorite/prompt generation unchanged | AC-10 |
| Usage column hidden when `prompt_count` absent (mocked) | AC-11 |

## 9. Implementation Order (suggested)

1. DDL (both `init.sql` copies) + apply to the running stack.
2. Proto changes + regenerate `prompt.pb.go`/`prompt_grpc.pb.go`.
3. Model + repository (column in Create/Get/GetLatest; List count join).
4. Service: `UpdateTemplate` validation + note persistence; `versionModelToProto` mapping.
5. Frontend utils: `diff.js`, `time.js`.
6. `ChangeNoteInput` + save-flow wiring; `VersionSelect` snippet.
7. `VersionHistoryPanel` + pagination refactor in `TemplateDetails`.
8. `DiffModal`, `RestoreConfirmModal` + restore flow.
9. i18n keys (both locales); SCSS for panel/diff/badge.
10. FVT + unit tests; `make lint`, `make ut`, `make fvt`, `npm run lint` green.

## 10. Constraints for the Developer (must respect)

1. **Never commit changes to `backend/server`** (tracked 26 MB binary) — `git checkout -- backend/server` before committing if it shows modified.
2. **Additive only:** no new endpoints, no new tables, no breaking proto/REST changes; `CreateTemplateRequest` untouched (open question #3).
3. Layering: proto → gateway (zero changes here) → service → repository → models; no new top-level packages.
4. `change_note` ≤ 200 chars enforced in the service layer (`InvalidArgument`); ignored (not stored) when content is unchanged.
5. Restore = existing `UpdateTemplate` with old content + canonical English note `Restored from v{n}` — never a new endpoint, never destructive; history stays append-only.
6. `prompt_count` via the inline LEFT JOIN + COUNT in the repository `List` (open question #1); 0 for prompt-less versions; absent-field tolerance on the frontend (NFR-6).
7. Diff is client-side LCS in `utils/diff.js` (open question #2) — **no new npm dependencies**; side-by-side fallback on failure.
8. i18n: every new user-visible string in **both** `en` and `zh` locales (`versioning.*`, `time.*`); stored notes render verbatim (never translated at render).
9. `init.sql` changes idempotent and mirrored into the helm chart copy; DDL deployed before/with the backend.
10. Conventional commits, English; ≥ 80 % unit-test coverage on new Go files; all CI gates green before push.
