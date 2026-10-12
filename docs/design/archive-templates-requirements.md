# Archive Templates — Requirements Analysis

- **Feature ID:** archive-templates
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. Competitor Research Summary

| Platform | Archive pattern | Key takeaways |
|---|---|---|
| **GitHub** | Repository archive: read-only, unsearchable in listings, unambiguous banner, reversible by owner | Archive = "keep but retire": content stays reachable by direct link, disappears from active surfaces, one click to restore |
| **Gmail** | Archive removes from inbox without deleting; "All Mail" still contains it; one-click undo | The mental model users already have: archive is the reversible middle between active and deleted |
| **Notion** | Page archive → moved to trash with restore window; archived pages excluded from search | Exclusion from search is the key signal of "retired"; restore must be self-serve |
| **YouTube Studio** | Video unlisted/private/deleted tiers; "unlisted" keeps the link working | A retired-but-reachable state avoids breaking external links (embeds, forks attribution) |
| **Trello** | Card archive: vanishes from board, visible in "Archive" filter, restore or delete permanently | The "Archive" filter as a first-class place to review and bulk-clean is the pattern to copy |
| **PromptBase** | No archive (only active listings) | Confirms archive is a library-management feature, not a marketplace one |

**Synthesis:** Archive is the industry-standard reversible retirement state: content disappears from public listings, search, and the owner's default dashboard view, but remains reachable by direct link (read-only view with a banner), restorable in one click, and eventually bulk-deletable from an "Archived" review surface. Open Prompts today has only two states — active (public/private) and **hard delete** (`DELETE FROM templates`, no soft-delete, no undo). A creator retiring old work must choose between clutter (private) and permanent loss (delete). Archive fills the gap with a single `archived_at` column and no destructive semantics.

## 2. Problem Statement

1. **No reversible retirement.** Creators accumulate obsolete templates (old prompt styles, superseded versions). Their only options: keep them private (cluttering the dashboard and counts) or hard-delete (permanent, irreversible, loses likes/comments/fork lineage).
2. **Dashboard clutter.** The user dashboard's "My Templates" tab lists every non-deleted template; a creator with 60 templates of which 40 are retired has no way to hide the retired ones from their working set.
3. **Public profile noise.** Public templates a creator no longer endorses still appear on their profile and in listings/search, diluting their current best work.
4. **Delete is scary.** Because delete is permanent, creators hoard private templates instead — the safest current behavior produces the worst clutter.

## 3. Target Users

- **Creators:** retiring old work without destroying it; cleaning up their library.
- **Visitors:** seeing only the creator's current, endorsed templates on profiles and listings.
- **Fork authors / embedders:** existing forks and embeds of an archived template keep working (attribution and links must not break).
- **Admins:** visibility into archived templates for moderation (archived content is still content).

## 4. User Stories

### 4.1 Archiving

- **US-A1:** As a creator, I want to archive a template from its detail page, so that it disappears from public surfaces without being deleted.
- **US-A2:** As a creator, I want to archive from my dashboard list (single + bulk), so that cleanup is fast.
- **US-A3:** As a creator, I want a confirmation that explains what archiving does, so that I understand the consequences before acting.
- **US-A4:** As a creator, I want an undo option right after archiving, so that mistakes cost nothing.

### 4.2 The archived state

- **US-A5:** As a visitor, when I open an archived template by direct link, I want a clear "archived" banner and read-only content, so that I understand its status.
- **US-A6:** As a visitor, I want archived templates excluded from home listings, search results, category/tag browsing, and creator profiles, so that discovery surfaces show only active work.
- **US-A7:** As a fork author, I want my fork's attribution line to keep working when the source template is archived, so that lineage never breaks.

### 4.3 Managing archived templates

- **US-A8:** As a creator, I want an "Archived" filter in my dashboard, so that I can review retired templates.
- **US-A9:** As a creator, I want to restore an archived template in one click, so that returning work to active is trivial.
- **US-A10:** As a creator, I want to permanently delete from the archived view (with strong confirmation), so that I can eventually truly clean up.

### 4.4 System integrity

- **US-A11:** As the system, archived templates are read-only: no likes, comments, favorites, or forks can be added; existing engagement data is preserved.
- **US-A12:** As an admin, I want archived templates visible in the admin console (with their archived status), so that moderation covers all content.

## 5. Functional Requirements

### 5.1 Data model

- **FR-1:** `templates` shall gain `archived_at TIMESTAMPTZ NULL` (NULL = active). An index on `(owner_id) WHERE archived_at IS NOT NULL` supports the dashboard archived filter. No other schema changes; all existing columns (likes, comments, fork lineage) are untouched.
- **FR-2:** Archive state is orthogonal to visibility: an archived template retains its public/private flag, but archived always wins for exposure (an archived public template is hidden from public surfaces exactly like an archived private one).

### 5.2 Archiving actions

- **FR-3:** The template detail page shall offer **Archive** (in the owner's action row, where Edit/Fork/Delete live today) opening a confirmation modal: "Archive '{{title}}'? It will disappear from public listings, search, and your profile. Existing links keep working; you can restore it anytime." Confirm → `POST /api/v1/templates/{id}/archive`.
- **FR-4:** The dashboard "My Templates" list shall offer Archive in the row actions menu, and the bulk action bar (bulk-operations feature) shall gain **Archive** and **Unarchive** actions (same confirmation pattern as bulk delete, plain confirm — no typed confirmation since it is reversible).
- **FR-5:** Archiving shall show an **undo toast** for 8 seconds ("Template archived — Undo"); Undo calls the restore endpoint immediately. After the toast expires, restoration happens from the Archived filter (FR-9).
- **FR-6:** Archiving shall be owner-only (admins archive via the admin console, FR-12). Archiving a template with active forks or embeds succeeds — those keep working (FR-8, US-A7).

### 5.3 The archived state (exposure rules)

- **FR-7:** Archived templates shall be excluded from: home/square listings, search results, category and tag browsing, creator profile template lists, related-templates, and the "forks of" listings. The exclusion is a single `archived_at IS NULL` predicate added to those queries.
- **FR-8:** Direct access to an archived template shall render the normal read-only detail page with an **archived banner** ("This template is archived — the author is no longer actively maintaining it") and the content visible. Likes, favorites, comments, and fork actions are disabled (buttons hidden with explanatory tooltip). Existing comments remain visible. The embed route (embedding-widget) renders the same banner in card form; embeds never break.
- **FR-9:** The dashboard "My Templates" tab shall gain an **Archived** filter chip (All / Active / Archived, default Active). The Archived view lists archived templates with **Restore** and **Delete permanently** row actions. Restore → `POST /api/v1/templates/{id}/unarchive` → template returns to its previous visibility in all surfaces.

### 5.4 Permanence & admin

- **FR-10:** **Delete permanently** (from the Archived view only) shall require typing the template title (stronger than bulk delete's `DELETE` — this is the only irreversible path for archived content) and shall behave exactly as today's delete (hard delete, cascades per existing semantics).
- **FR-11:** Archiving/unarchiving shall be recorded in the template's `updated_at` and appear in the dashboard activity feed (existing UNION query picks up `updated_at` changes automatically).
- **FR-12:** The admin templates list shall show an **Archived** status filter and the archived badge on rows; admins can archive/unarchive from the admin template detail page (audited as `archive`/`unarchive` — requires extending the `admin_audit_logs` action CHECK constraint). Admin archive actions use the same endpoints with admin authZ.

## 6. Non-Functional Requirements

- **NFR-1 (Reversibility):** Archive/unarchive is lossless — no counts, comments, versions, aliases, or fork lineage are modified. Restore returns the template to byte-identical pre-archive state plus timestamps.
- **NFR-2 (Performance):** The `archived_at IS NULL` predicate uses the existing indexes; listing queries must not regress (verified by the same p95 targets as the dashboard design).
- **NFR-3 (Link stability):** No URL changes; archived templates keep their IDs, aliases, and version history. External links (forks attribution, embeds, shared URLs) never 404 due to archiving.
- **NFR-4 (Clarity):** The archived banner is unmissable on the detail page and embed card; the dashboard Archived filter is discoverable but not prominent.
- **NFR-5 (i18n):** All new strings in `en`/`zh` with identical keys.
- **NFR-6 (Accessibility):** Banner, filter chips, and confirmations follow existing patterns; disabled engagement buttons are hidden (not merely grayed) to reduce noise.
- **NFR-7 (No regressions):** Templates with `archived_at IS NULL` behave identically to today in every surface (the predicate is additive).

## 7. Acceptance Criteria

- **AC-1:** Archiving a public template from its detail page shows the confirmation; confirming archives it: it disappears from home listings, search, its category, and the creator's profile within the same session, and an undo toast appears.
- **AC-2:** Clicking Undo within 8 seconds restores the template to all surfaces immediately.
- **AC-3:** Opening an archived template by its direct URL shows the archived banner and content; Like/Favorite/Comment/Fork controls are absent; existing comments render.
- **AC-4:** An embed of an archived template renders the card with the archived note (not the unavailable card — the content remains visible per FR-8).
- **AC-5:** A fork created before archiving still shows its attribution line to the archived source, and the source link opens the archived detail page.
- **AC-6:** The dashboard Archived filter lists archived templates; Restore returns one to active surfaces with its previous visibility; the undo toast is not shown for manual restores (it is immediate).
- **AC-7:** Delete permanently requires typing the exact title; on confirmation the template is hard-deleted (gone from Archived list and direct URL 404s).
- **AC-8:** Bulk Archive from the dashboard archives N selected templates with a count-confirmed modal and per-item result toast (bulk-operations pattern); bulk Unarchive mirrors it.
- **AC-9:** The admin templates list filters by archived status; admin archive/unarchive writes `archive`/`unarchive` audit rows.
- **AC-10:** Listing/search/category/profile queries return zero archived templates; a template restored from archive reappears in all of them.
- **AC-11:** All new strings exist in `en` and `zh`; the archived banner and embed note are localized.
- **AC-12:** Active (`archived_at IS NULL`) templates behave identically to pre-feature in every surface (regression check).

## 8. Out of Scope (v1)

- Auto-archive rules ("archive templates unused for 6 months") — v1 is manual only.
- Archive expiry (auto-delete after N days) — archive has no time bomb.
- Archiving other entities (prompts, collections).
- Visitor-facing "archived templates by {creator}" browsing page (archived is excluded from discovery by design).
- Notification to fork authors when a source template is archived (forks keep working; no action needed).
- Bulk export of archived templates specifically (import-export covers all templates).
- Admin bulk archive (single admin actions only in v1).

## 9. Dependencies & Assumptions

- **D-1:** The bulk-operations feature (designed) provides the floating action bar, confirm modal, and result toast; FR-4 adds Archive/Unarchive actions to it (its `BatchUpdateTemplateVisibility` pattern extends naturally to a `BatchArchiveTemplates`/`BatchUnarchiveTemplates` pair, or a generic state-batch — architect's call per its §10).
- **D-2:** The embedding-widget feature (designed) defines the embed route; FR-8's embed banner integrates there (the embed page checks `archived_at` alongside visibility).
- **D-3:** The template-remix-forks-tree feature (designed) defines the attribution line; FR-8/AC-5 rely on its tombstone/link behavior for archived (not deleted) sources — archived sources are reachable, so attribution links work without tombstones.
- **D-4:** The user dashboard (designed) defines the "My Templates" tab and filter chips; FR-9 adds the Archived chip there.
- **D-5:** `admin_audit_logs.action` CHECK currently allows `ban|unban|role_change`; FR-12 requires adding `archive|unarchive` (and the comments feature's moderation actions if not yet added) via migration.
- **D-6:** All listing/search/profile queries flow through the template repository's list methods; the `archived_at IS NULL` predicate is added once per query path (repository-level default with an explicit opt-in for archived/admin queries).

## 10. Open Questions for Architect

1. **Predicate placement:** repository-level default (`archived_at IS NULL` baked into list queries, with an `includeArchived`/`archivedOnly` option) vs. per-callsite predicates? I recommend the repository default (impossible to forget); confirm.
2. **Batch RPCs:** extend bulk-operations with `BatchArchiveTemplates`/`BatchUnarchiveTemplates` (mirroring visibility batches) vs. a shared `BatchUpdateTemplateState`? Either fits its §10 question 1; please decide together.
3. **Audit CHECK migration:** adding `archive|unarchive` to `admin_audit_logs.action` — one migration alongside the comments moderation actions if that hasn't shipped yet, or separate?
4. **Embed banner vs unavailable:** FR-8 renders archived content with a banner (my recommendation — link stability and content preservation) vs. treating archived like private (unavailable card). Confirm the banner approach.
