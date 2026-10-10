# Template Collections & Sharing — Requirements Analysis

- **Feature ID:** template-collections
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2025-06-25

## 1. Competitor Research Summary

| Platform | Collections/sharing approach | Key takeaways |
|---|---|---|
| **PromptBase** | Curated marketplace categories + seller "stores"; shareable listing URLs; no user-defined collections | A dedicated, shareable URL per curated set is the core primitive; stores aggregate a creator's work |
| **FlowGPT** | User-created prompt lists/collections on profiles; share links; collections have cover images and descriptions | Collections are a first-class object with metadata; profile aggregation drives discovery |
| **Snack Prompt** | Community "spaces"/collections; users organize prompts into shareable boards; follow collections | Following a collection creates recurring engagement; boards are collaborative in some cases |
| **AIPRM** | Saved lists/folders in personal library; shareable list links; public lists ranked | Personal organization (private) and public sharing (publish) are distinct states of the same object |
| **PromptHero** | User profiles aggregate creations; no arbitrary collections | Profile-as-collection is the minimal version; arbitrary collections add flexibility |
| **Generic (Spotify playlists / YouTube playlists)** | Create → add items → optional publish → share URL → others can view/follow | The playlist pattern is universally understood: private working set → one-click publish → shareable link |

**Synthesis:** The playlist model (Spotify/YouTube) is the dominant, universally-understood pattern: a collection is a named, ordered list of templates with a description, created by any user, private by default, publishable with one action, and shareable via a stable URL. Following/subscribing adds re-engagement but is a v2 concern. Collaborative editing is rare and out of scope.

## 2. Problem Statement

Open Prompts treats every template as an isolated unit. Users who find 5 great writing templates have no way to keep them together, and creators who publish 10 related templates have no way to present them as a set:

1. **No grouping.** Favorites and likes are flat, unorganized lists. A user with 40 favorited templates cannot organize them by project or topic.
2. **No shareable sets.** Sharing today means making a single template public. There is no URL that says "here are my 6 best email templates."
3. **No creator merchandising.** A creator's templates are scattered across the square; there is no "storefront" or curated set to link from a bio, blog, or social post.
4. **No discovery surface for sets.** Browsing is per-template only; collections as a browsing dimension (like categories) don't exist.
5. **Onboarding gap.** New users cannot benefit from other users' curation work — the strongest signal of quality at low moderation cost.

## 3. Target Users

- **Curators (primary):** any logged-in user who wants to organize templates they like/use into named sets (private working sets) and optionally publish them.
- **Creators:** users who publish their own templates and want a shareable "best of" or topic set to market their work.
- **Browsers:** visitors who discover quality templates through curated collections rather than raw search.
- **Admins:** (indirect) collections are content surfaces that the admin moderation feature can oversee later; no admin UI in v1.

## 4. User Stories

### 4.1 Creating & organizing

- **US-C1:** As a user, I want to create a collection with a name and optional description, so that I can group templates for a project or topic.
- **US-C2:** As a user, I want to add any public template (or my own private one) to my collection from the template detail page or card, so that collecting is one click wherever I browse.
- **US-C3:** As a user, I want to remove a template from my collection, so that my collections stay current.
- **US-C4:** As a user, I want to reorder templates within a collection (drag or move up/down), so that the most important ones appear first.
- **US-C5:** As a user, I want my collections to be private by default, so that I can organize without publishing.
- **US-C6:** As a user, I want to see all my collections in one place, so that I can manage them.

### 4.2 Publishing & sharing

- **US-C7:** As a curator, I want to publish a collection with one action, so that it becomes publicly browsable at a stable URL.
- **US-C8:** As a curator, I want to unpublish (make private again), so that I can retire a set without deleting it.
- **US-C9:** As a curator, I want to share a public collection via a copy-link button, so that I can post it anywhere.
- **US-C10:** As a visitor, I want to open a shared collection URL and see the collection's name, description, curator, and its templates in a grid, so that I can browse and use them without an account.

### 4.3 Discovery

- **US-C11:** As a visitor, I want to browse public collections on a dedicated page, so that I can find curated sets.
- **US-C12:** As a visitor, I want to see collection counts (templates, likes), so that I can judge popularity.

### 4.4 System

- **US-C13:** As the system, I keep private templates out of public collections, so that publishing a collection never leaks private content.
- **US-C14:** As the system, I limit collections per user (50) and templates per collection (100), so that abuse and storage are bounded.

## 5. Functional Requirements

### 5.1 Collection CRUD

- **FR-1:** Logged-in users shall create collections with: `name` (required, 1–80 chars), `description` (optional, 0–500 chars), `visibility` (`private` default | `public`).
- **FR-2:** Users shall edit their own collection's name, description, and visibility at any time.
- **FR-3:** Users shall delete their own collections (soft delete; confirmation required in UI).
- **FR-4:** A user may own at most 50 collections; a collection may contain at most 100 templates (server-enforced, friendly error).
- **FR-5:** Collection URLs shall be stable and unguessable: `/collections/{id}` where id is a UUID; renaming never changes the URL.

### 5.2 Managing templates in collections

- **FR-6:** Users shall add a template to one or more of their collections from: (a) the template detail page ("Add to collection" button), (b) a card hover/overflow action on the home grid.
- **FR-7:** Users may add any public template or a template they own (private included) to their collections.
- **FR-8:** Adding a template already in the collection is a no-op (idempotent; no error).
- **FR-9:** Users shall remove templates from their collection (from the collection page, owner view).
- **FR-10:** Users shall reorder templates in their collection (move up/down buttons; drag-and-drop optional).
- **FR-11:** The system shall store an explicit `position` per (collection, template) so ordering survives re-fetches.

### 5.3 Visibility & sharing rules

- **FR-12:** Private collections are visible only to their owner.
- **FR-13:** Public collections are visible to anyone at `/collections/{id}` (anonymous included).
- **FR-14:** When a collection is public, only templates that are themselves public (or owned by the collection owner) shall be included in the public view; the owner's private view shows all members with a "private" badge on non-public ones.
- **FR-15:** If a template in a public collection is later made private by its owner (not the curator), the template disappears from the public collection view but remains in the curator's private view (with a "no longer public" badge).
- **FR-16:** Deleting a template removes it from all collections automatically (cascade).
- **FR-17:** The collection page shall offer a "Copy link" action (public collections only) that copies the absolute URL to the clipboard.

### 5.4 Discovery

- **FR-18:** A `/collections` page shall list public collections: card grid with name, description excerpt, curator display name, template count, cover collage (first 4 template cards' colors/initials — no image upload in v1).
- **FR-19:** The `/collections` page shall support sorting by Newest and Most templates (v1; search/sort reuse from template-search-filtering is a natural follow-up).
- **FR-20:** The Sidebar shall gain a "Collections" nav item (logged-in users: "My Collections"; all users: "Explore Collections") linking to the respective views.
- **FR-21:** A curator's public profile surface (v1: the collection page header) shall link to their other public collections ("More from {curator}").

### 5.5 API

- **FR-22:** New RPCs (PromptService or a new CollectionService — architect decision): `CreateCollection`, `UpdateCollection`, `DeleteCollection`, `GetCollection` (by id; includes members per visibility rules), `ListMyCollections`, `ListPublicCollections` (paginated, sorted), `AddTemplateToCollection`, `RemoveTemplateFromCollection`, `ReorderCollectionTemplates`, `ListCollectionsContainingTemplate` (for the "in collections" indicator).
- **FR-23:** REST endpoints under `/api/v1/collections` (list/create), `/api/v1/collections/{id}` (get/patch/delete), `/api/v1/collections/{id}/templates` (add/remove/reorder), `/api/v1/templates/{id}/collections` (which of my collections contain it). Anonymous read access for public collections; auth required for everything else.
- **FR-24:** `GetCollection` response shall include: id, name, description, visibility, owner (id + display_name), template_count, created_at, updated_at, and members (template summaries per FR-14 rules).
- **FR-25:** All mutations enforce ownership (403 for non-owners); reads of private collections return 404 (not 403) to avoid existence leaks.

### 5.6 Regular-user vs admin split

- **FR-26:** Collections are a regular-user feature; no admin surface in v1. Admin moderation of collections (hide/ban) is a future extension of the admin feature; the schema (D-1) includes a `status` column to make that forward-compatible.

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Collection pages p95 < 500ms for ≤100 members; public collection listing paginated (20/page).
- **NFR-2 (Consistency):** Template counts and member lists are read from the join table at query time (no denormalized counters in v1); counts are exact.
- **NFR-3 (Compatibility):** Purely additive schema/API; no changes to existing endpoints; existing clients unaffected.
- **NFR-4 (Privacy):** Private collection existence is not discoverable (404 for non-owners); public collections never include others' private templates (FR-14/15).
- **NFR-5 (Accessibility):** Collection creation/edit forms, add-to-collection menu, reorder controls, and copy-link must be keyboard-operable (see UI/UX doc §10).
- **NFR-6 (i18n):** All new strings EN/ZH in sync; collection names/descriptions are user content (not translated); UI chrome fully localized.
- **NFR-7 (Security):** Ownership enforced server-side on every mutation; IDs are UUIDs (unguessable); no injection surface (parameterized queries).
- **NFR-8 (Scale):** Join table indexed on (collection_id, position) and (template_id); limits (FR-4) bound worst-case sizes.

## 7. Acceptance Criteria

- **AC-1:** A logged-in user can create a private collection, add 3 public templates (one their own private template), and see all 4 in the owner view with the private one badged.
- **AC-2:** Publishing the collection makes it accessible at `/collections/{id}` to an anonymous browser, showing only the public templates (the curator's own private template included per FR-14).
- **AC-3:** Another user's private template added to a public collection disappears from the public view when its owner makes it private, but remains in the curator's private view with a badge.
- **AC-4:** "Copy link" copies the collection URL; opening it in a fresh anonymous tab shows the collection.
- **AC-5:** Reordering (move up/down) persists across reloads.
- **AC-6:** Deleting a template removes it from all collections containing it.
- **AC-7:** A non-owner cannot edit, delete, or mutate a collection (403), and cannot read a private one (404).
- **AC-8:** The `/collections` page lists public collections with counts and curator names, paginated, sorted by the selected option.
- **AC-9:** The template detail page shows an "Add to collection" action for logged-in users and marks which of my collections already contain it.
- **AC-10:** Limits are enforced: creating the 51st collection or adding the 101st template returns a friendly error.
- **AC-11:** All new UI strings exist in both `en` and `zh` translation files with identical key sets.
- **AC-12:** Existing template browsing, likes, favorites, and fork flows are unchanged (no regression).

## 8. Out of Scope (v1)

- Following/subscribing to collections (v2 engagement feature).
- Collaborative collections (multiple curators).
- Collection cover image uploads (v1 uses generated collage/initials).
- Collection likes/comments.
- Collection search and full filtering (reuses later from template-search-filtering follow-ups).
- Curator profile pages (only "More from {curator}" links on collection pages).
- Embeddable collection widgets for external sites.
- Collection templates export (JSON/PDF).
- Admin moderation UI for collections (schema is forward-compatible via `status`).
- Sharing to social networks with preview cards (og:image generation).
- Notifications when a followed collection changes (no follow feature yet).

## 9. Dependencies & Assumptions

- **D-1:** New tables: `collections` (id UUID PK, owner_id TEXT, name, description, visibility, status default 'active', created_at, updated_at) and `collection_templates` (collection_id, template_id, position INT, added_at; PK (collection_id, template_id)). Indexes: (collection_id, position), (template_id). Idempotent migration pattern per deployment/init.sql.
- **D-2:** `users.display_name` exists for curator display (confirmed in schema); if empty, fall back to a localized "Anonymous creator" label.
- **D-3:** Template summaries in collection views reuse the existing Template proto (title, description, tags, counts) — no new projection needed.
- **D-4:** The add-to-collection picker fetches `ListMyCollections` (≤50 rows) — cheap; no search within picker needed in v1.
- **D-5:** `/collections` and `/collections/:id` are new frontend routes; no conflicts with existing routes.
- **D-6:** Copy-to-clipboard uses `navigator.clipboard` with a fallback (execCommand) for older browsers; existing notification pattern confirms success.
- **D-7:** Reordering via move up/down buttons (FR-10) is the accessible baseline; drag-and-drop (HTML5 DnD) is optional polish — the API contract (position list) is identical.
- **D-8:** Public collection listing sort (FR-19) is server-side; pagination follows the existing page_token pattern.

## 10. Open Questions for Architect

*(None blocking. D-1 schema shape, FR-22 service placement (PromptService vs new CollectionService), and FR-25 404-vs-403 policy are flagged decisions.)*
