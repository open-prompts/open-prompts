# Taxonomy Management (Categories & Tags) — Requirements Analysis

- **Feature ID:** taxonomy-management
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. Competitor Research Summary

| Platform | Taxonomy approach | Key takeaways |
|---|---|---|
| **AIPRM** | Curated category tree (60+ categories) with admin-defined subcategories; free-text tags with admin cleanup | A curated category list keeps browsing coherent; unmanaged tags drift into synonyms |
| **FlowGPT** | Fixed category list + free tags; tags shown as clickable chips | Categories are the primary browse axis; tag chips drive secondary discovery |
| **Snack Prompt** | Curated categories + tag suggestions from existing tags while typing | Type-ahead from the existing tag pool prevents new near-duplicate tags |
| **PromptHero** | Fixed categories + free tags; no admin cleanup visible | Public tag clouds accumulate noise ("ai", "AI", "a.i.") without curation |
| **WordPress (admin)** | Terms table with merge/rename; usage counts; bulk actions | Merge is the single most valuable taxonomy operation — it preserves content |
| **Stack Overflow** | Tag wiki pages, synonym mapping, burninate (retag + remove) workflow | Synonym mapping redirects old tags to canonical ones at write time |

**Synthesis:** Open Prompts today has *no* taxonomy management at all: categories and tags are derived dynamically from the `templates` table (`SELECT category, COUNT(*) …` / `SELECT unnest(tags) …`), and any user typing a new category or tag in the Create Template modal instantly creates it in the shared public list. There is no canonical list, no deduplication, no way to fix a typo'd category ("Writting"), no way to merge "ai" into "ai-assistant", and no way to retire a category. The winning v1 pattern combines: (1) a **canonical `categories` table** (name, slug, description, sort order, status) managed by admins in the admin console; (2) **tags remain free-form** but get a **canonical `tags` table** (name, status) with admin **merge/rename** operations that rewrite `templates.tags` arrays in place; (3) the Create Template modal's category select is fed by the canonical list (custom category entry removed for regular users), and the tag input gains **type-ahead suggestions from the existing tag pool** to prevent new near-duplicates; (4) all admin taxonomy mutations are written to the existing `admin_audit_logs`. This is an **admin-console feature** (new `/admin/taxonomy` pages) with a small regular-user surface change (category select + tag suggestions).

## 2. Problem Statement

1. **No canonical taxonomy.** Categories and tags exist only as derived aggregates of template rows. Nothing defines the valid set, so the public sidebar and search filters reflect whatever users happened to type.
2. **Unbounded category creation by users.** The Create Template modal lets any user mint a new public category via "create new" — leading to typos, synonyms, and fragmentation ("Writing", "writting", "Write") that break category browsing.
3. **Tag drift.** Free-form tags accumulate case variants and synonyms ("ai"/"AI"/"a.i.", "chatgpt"/"chat-gpt"); there is no merge, rename, or cleanup path, and tag-cloud filters grow noisy.
4. **No retirement.** A category that outlived its usefulness (or was created abusively) cannot be hidden or removed without deleting templates.
5. **No oversight surface.** The admin console has users, templates, and audit log pages, but nothing for taxonomy — admins cannot even see which categories/tags exist with usage counts (the public endpoints only expose public templates' aggregates).

## 3. Target Users

- **Admins (primary):** need to curate the category list, clean up tag drift, and keep browse/filter surfaces coherent.
- **Template creators (secondary):** get a clean category select and tag type-ahead, reducing accidental duplicates.
- **Browsing users (beneficiary):** see a coherent sidebar, category pages, and tag filters without synonym noise.

## 4. User Stories

- **US-T1:** As an admin, I can view all categories and tags with usage counts (including private/unpublished templates), so I understand the real taxonomy state.
- **US-T2:** As an admin, I can create a category with a name, slug, and description, so the canonical list is curated before templates use it.
- **US-T3:** As an admin, I can rename a category, so typos and synonyms are fixed everywhere at once.
- **US-T4:** As an admin, I can merge one tag into another, so "ai" and "AI" collapse into one canonical tag without losing templates.
- **US-T5:** As an admin, I can hide (retire) a category or tag, so useless or abusive taxonomy entries stop appearing in browse surfaces while templates keep their data.
- **US-T6:** As an admin, every taxonomy mutation I make is recorded in the audit log with a reason, so oversight exists.
- **US-T7:** As a creator, the category select shows only the canonical curated list, so I cannot fragment the taxonomy by accident.
- **US-T8:** As a creator, typing a tag suggests existing tags, so I reuse canonical tags instead of minting near-duplicates.
- **US-T9:** As a browsing user, hidden categories/tags never appear in the sidebar, category filters, or tag cloud, so browse surfaces stay clean.

## 4a. Regular-User vs Admin Split

| Capability | Regular user | Admin |
|---|---|---|
| View canonical categories/tags (public, active) | Yes (sidebar, filters, create modal) | Yes |
| View full taxonomy incl. hidden + private-template usage | No | Yes (`/admin/taxonomy`) |
| Create category | No (select from list only) | Yes |
| Rename category / merge tag | No | Yes |
| Hide/restore category or tag | No | Yes |
| Tag type-ahead suggestions | Yes (create/edit modal) | Yes |
| Audit log of taxonomy actions | No | Yes (existing audit page) |

## 5. Functional Requirements

### 5.1 Data model

- **FR-1:** New table `categories`: `id UUID PK`, `name TEXT UNIQUE NOT NULL`, `slug TEXT UNIQUE NOT NULL`, `description TEXT`, `sort_order INT NOT NULL DEFAULT 0`, `status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','hidden'))`, `created_at/updated_at TIMESTAMPTZ`. `templates.category` continues to store the category **name** (no FK; rename keeps them in sync — FR-6).
- **FR-2:** New table `tags`: `name TEXT PK` (lowercased canonical form), `status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','hidden'))`, `created_at/updated_at TIMESTAMPTZ`. `templates.tags` continues to store the array of tag names; merge/rename rewrite the arrays (FR-8/9).
- **FR-3:** A one-time backfill migration inserts distinct existing `templates.category` values into `categories` (slug derived from name) and distinct `unnest(tags)` values into `tags` (lowercased, deduplicated). Idempotent (`ON CONFLICT DO NOTHING`).

### 5.2 Admin console surface (`/admin/taxonomy`)

- **FR-4:** A **Taxonomy** page in the admin console with two tabs: **Categories** and **Tags**. Each tab is a searchable, paginated table: name, usage count (all templates regardless of visibility), status, updated time, row actions.
- **FR-5 (create category):** Admins can create a category (name, slug auto-derived and editable, optional description). Name/slug must be unique; validation errors show inline.
- **FR-6 (rename category):** Admins can rename a category (name and slug). Renaming updates `templates.category` for all matching templates in one transaction and logs to the audit log.
- **FR-7 (hide/restore category):** Admins can toggle a category between `active` and `hidden`. Hidden categories disappear from the sidebar, category filters, create-modal select, and public category endpoints; templates keep their category value (no data loss). Restoring reverses this.
- **FR-8 (merge tag):** Admins can merge tag B into tag A: every template whose `tags` array contains B has B replaced by A (added if absent, duplicates removed, array re-sorted), tag B row is deleted, and the merge is audited. Usage counts update immediately.
- **FR-9 (rename tag):** Admins can rename a tag (canonical lowercase). Same rewrite semantics as merge (target name must not already exist; if it does, the action becomes a merge — the UI offers this).
- **FR-10 (hide/restore tag):** Admins can toggle tag status. Hidden tags disappear from the tag cloud, tag filters, and tag suggestions; templates keep their tag values.
- **FR-11 (audit):** Every taxonomy mutation (create/rename/merge/hide/restore) writes an `admin_audit_logs` row. The `action` CHECK constraint is extended with taxonomy actions (`category_create`, `category_rename`, `category_hide`, `category_restore`, `tag_merge`, `tag_rename`, `tag_hide`, `tag_restore`); `target_type` gains `category`/`tag`. Reason is mandatory (3–500 chars, existing rule).

### 5.3 Regular-user surface changes

- **FR-12:** The Create/Edit Template modal's category select lists only **active canonical categories** (sorted by `sort_order`, then name). The "create new category" free-text option is **removed** for regular users. If the template's current category is hidden, it still displays (read-only context) but cannot be re-selected after changing.
- **FR-13:** The tag input in the Create/Edit modal gains **type-ahead suggestions** (up to 8) from active canonical tags, matched case-insensitively by prefix/substring. Users may still enter a new tag (free-form entry remains; it is added to `tags` as a new active row on template save — FR-15).
- **FR-14:** The public sidebar and category/tag endpoints return only **active** categories/tags (existing aggregate queries gain a join/semi-join against the canonical tables). Counts remain template-derived.
- **FR-15:** On template create/update, any tag not present in `tags` is inserted as a new active row (upsert, lowercase-normalized). Category values not in `categories` (legacy rows) are tolerated read-only and surfaced to admins via a "missing from canonical list" indicator (FR-16).
- **FR-16:** The admin Categories tab shows a **"Legacy"** badge and filter for categories referenced by templates but absent from the canonical table (pre-backfill strays created after a race); one click adopts them into the canonical list (create with derived slug).

### 5.4 API

- **FR-17:** Admin taxonomy APIs under `/api/v1/admin/taxonomy`: `GET /categories` (list with counts, status filter, search, pagination), `POST /categories`, `PATCH /categories/{id}` (rename/slug/description/sort_order), `POST /categories/{id}/hide`, `POST /categories/{id}/restore`; `GET /tags` (same shape), `POST /tags/{name}/merge` (body: `into`), `PATCH /tags/{name}` (rename), `POST /tags/{name}/hide`, `POST /tags/{name}/restore`. All require admin role (existing admin auth middleware).
- **FR-18:** Public endpoints `GET /api/v1/categories` and `GET /api/v1/tags` return only active entries, ordered canonically (categories by `sort_order` then name; tags by usage count desc as today), with template counts as today.

### 5.5 Notifications integration

- **FR-19:** No notifications. Taxonomy mutations are admin-only and silent to users (a renamed category or merged tag does not notify template owners).

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Admin taxonomy list endpoints respond < 300 ms (p95) at 10k templates; tag merge of up to 5k affected templates completes < 10 s inside one transaction.
- **NFR-2 (Atomicity):** Rename/merge operations are single-transaction (canonical row + all affected `templates` rows); a failure leaves no partial state.
- **NFR-3 (Idempotent backfill):** The FR-3 migration is re-runnable without duplicating rows.
- **NFR-4 (i18n):** All admin taxonomy UI strings and user-facing select/suggestion strings are translated (EN/ZH); category/tag **names are user data and never translated**.
- **NFR-5 (Auditability):** Every mutation produces exactly one audit row; the existing audit log page renders taxonomy actions with the same table/pagination pattern.
- **NFR-6 (No data loss):** Hide never deletes template data; merge preserves all templates (only the tag name changes); rename preserves counts (usage recomputed from templates).
- **NFR-7 (Backward compatibility):** Templates with categories/tags absent from canonical tables (legacy) continue to render and filter exactly as today until an admin acts.

## 7. Acceptance Criteria

- **AC-1:** `/admin/taxonomy` shows Categories and Tags tabs with search, status filter, usage counts (including private templates), and pagination.
- **AC-2:** Creating a category with an existing name or slug shows an inline validation error and creates nothing.
- **AC-3:** Renaming category "Writting" → "Writing" updates every template's category, the sidebar, and category filters immediately; usage counts are preserved.
- **AC-4:** Merging tag "ai" into "ai-assistant" removes "ai" from the tag table and from every template's tags array; templates that had both keep a single "ai-assistant".
- **AC-5:** Hiding a category removes it from the sidebar, public category endpoint, create-modal select, and search filters; its templates are unchanged and still reachable via search/direct link.
- **AC-6:** Every mutation writes an audit row (action, target, reason) visible on the audit log page.
- **AC-7:** The create-modal category select shows only active canonical categories; the free-text "create new category" option is gone.
- **AC-8:** Typing "ai" in the tag input suggests existing active tags (case-insensitive); selecting one reuses it.
- **AC-9:** A template saved with a brand-new tag makes that tag appear in the canonical tags table (active) and in the public tag cloud.
- **AC-10:** Legacy categories (referenced but not canonical) show a "Legacy" badge in the admin table and can be adopted in one click.
- **AC-11:** Public category/tag endpoints return only active entries; hidden entries return 404-free absence (simply not listed).
- **AC-12:** All taxonomy UI strings render in both EN and ZH; category/tag names render as-is.

## 8. Out of Scope

- Tag synonyms/redirect mapping at write time (Stack Overflow-style synonym table) — merge covers v1 cleanup.
- Category hierarchy/subcategories (flat list only).
- Tag descriptions/wiki pages.
- Bulk multi-select taxonomy operations (merge many at once).
- Taxonomy usage analytics (trends over time).
- Regular-user tag/category management of any kind.
- Monetization of any kind (payments, pricing, purchases, earnings) — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1:** `templates.category` (TEXT) and `templates.tags` (TEXT[]) are the storage of record; canonical tables are metadata overlays, not FK-enforced references (avoids a risky migration on a hot table).
- **D-2:** The existing ALTER TABLE IF NOT EXISTS migration pattern (used for `users.role/status`) extends to the `admin_audit_logs` CHECK constraints and new tables.
- **D-3:** The admin console's list-page pattern (search + table + pagination + drawer/modal) from Users/Templates pages is reused for `/admin/taxonomy`.
- **D-4:** The public `ListCategories`/`ListTags` repository queries (aggregate from templates) can gain a semi-join against canonical tables without changing response shape.
- **D-5:** The CreateTemplateModal's category select and tag input are the only creator-facing entry points (edit modal shares them).
- **D-6:** Tag normalization is lowercase-trim (existing behavior preserved for legacy mixed-case tags; merge/rename canonicalize).

## 10. Open Questions for Architect

1. **Canonical tables vs derived-only:** I propose real `categories`/`tags` tables as metadata overlays (FR-1/2) with `templates.category`/`tags` unchanged. Alternative: keep derived-only and implement rename/merge as pure `UPDATE templates` scripts with no canonical state — cheaper but cannot support hide/restore or curated ordering. Confirm the overlay approach?
2. **Category storage:** keeping `templates.category` as the name string (rename rewrites all rows, FR-6) vs migrating to `category_id` FK (cleaner but a heavy migration on the hottest table). I recommend name-string + transactional rewrite.
3. **Audit CHECK extension:** extending the `action`/`target_type` CHECK constraints via ALTER TABLE (D-2) vs dropping the CHECK for flexibility. I recommend extending (keeps the append-only guarantee explicit).
4. **Tag merge scale:** one transaction rewriting up to N templates (NFR-1 caps at 5k) vs batched background job. I recommend single transaction with a documented cap; batch if you expect larger sets.
5. **Public endpoint ordering:** categories ordered by `sort_order` then name (FR-18) — confirm, or keep pure usage-count ordering for categories as today?
6. **New-tag auto-adoption (FR-15):** free-form tags still mint new canonical rows on save. Alternative: queue new tags for admin approval before appearing publicly. I recommend auto-adopt (frictionless) + admin cleanup via merge/hide.
