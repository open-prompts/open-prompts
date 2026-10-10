# User Dashboard — Requirements Analysis

- **Feature ID:** user-dashboard
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2025-06-25

## 1. Competitor Research Summary

| Platform | Personal dashboard approach | Key takeaways |
|---|---|---|
| **PromptBase** | Seller dashboard: listings, sales, views, revenue; account settings separate | A single "my stuff" hub with clear sections; metrics inline per item |
| **FlowGPT** | Profile page doubles as dashboard: my prompts, likes, collections; edit inline | Profile-as-dashboard merges identity and content management |
| **Snack Prompt** | Personal library: saved prompts, created prompts, collections; tabs | Tabbed personal library is the standard shape |
| **AIPRM** | Personal list library organized in folders; usage stats per prompt | Organization (folders) + per-item stats |
| **GitHub** | Dashboard: overview cards, activity feed, repos list, quick links | Overview-first with quick actions; activity feed drives re-engagement |
| **Generic SaaS (Vercel/Stripe)** | Left-nav sections: Overview, [entity] list, Settings; recent-activity panel | Overview + entity lists + settings is the canonical IA |

**Synthesis:** The winning pattern is a personal hub with (1) an overview section (counts + recent activity + quick actions), (2) tabbed or routed entity lists (my templates, my prompts, my collections, my likes/favorites), and (3) settings kept separate. Open Prompts already has the entity lists scattered (My Prompts = filtered home view; prompts buried in template details; API keys on their own page); the gap is the unified hub and the overview.

## 2. Problem Statement

1. **No personal home.** After login, users land on the public square. There is no "my workspace" that answers "what have I made/used/saved recently?"
2. **My Prompts is a filter, not a surface.** "My Prompts" in the sidebar is the home grid with an owner filter — no ownership-specific actions (edit/delete inline), no template-type distinction (system vs user), no stats.
3. **Instantiated prompts are buried.** Prompts (filled-in template instances) are only visible inside each template's detail page. Users cannot see "all prompts I've created" in one place.
4. **No overview.** Counts of my templates/prompts/collections/keys and recent activity require visiting four different pages.
5. **Quick actions missing.** Creating a template requires navigating to the square's create button; there is no dashboard-level "New template" CTA.

## 3. Target Users

- **Regular users (primary):** anyone logged in who wants a personal workspace overview and quick access to their content.
- **Creators:** heavier users who manage many templates and want at-a-glance performance + management entry points.
- **Admins:** unaffected (admin console is separate; this is the regular-user surface).

## 4. User Stories

### 4.1 Overview

- **US-D1:** As a user, I want a personal dashboard at `/dashboard` with counts of my templates, prompts, collections, and API keys, so that I understand my footprint at a glance.
- **US-D2:** As a user, I want a recent-activity list (latest templates created, prompts used, templates liked/favorited), so that I can pick up where I left off.
- **US-D3:** As a user, I want quick actions (New template, New API key), so that common tasks are one click away.
- **US-D4:** As a user, I want the dashboard to be the post-login landing page, so that I start in my workspace, not the public square.

### 4.2 My content management

- **US-D5:** As a creator, I want a "My Templates" view with ownership-specific actions (edit, delete, toggle visibility), so that I can manage my templates without visiting each detail page.
- **US-D6:** As a user, I want a "My Prompts" view listing all prompts I've instantiated (across templates), so that I can revisit/reuse past work.
- **US-D7:** As a user, I want to see which template each of my prompts came from (with a link), so that I can navigate to the source.
- **US-D8:** As a user, I want to delete a prompt from the list, so that I can clean up.
- **US-D9:** As a user, I want my collections shown on the dashboard with links, so that all my content is reachable from one place.

### 4.3 System

- **US-D10:** As the system, I keep the dashboard entirely owner-scoped, so that no other user's data ever appears.
- **US-D11:** As the system, I make the dashboard fast (parallel aggregate queries), so that it feels instant.

## 5. Functional Requirements

### 5.1 Dashboard overview (`/dashboard`)

- **FR-1:** The dashboard shall be available at `/dashboard` (auth required; anonymous → login redirect).
- **FR-2:** The overview shall show four stat cards: My Templates (count), My Prompts (count), My Collections (count), API Keys (active count).
- **FR-3:** The overview shall show a recent-activity feed (max 10 items, newest first) merging: templates created/updated (by me), prompts created (by me), templates liked/favorited (by me). Each item: icon, text, timestamp, link to the entity.
- **FR-4:** The overview shall show quick actions: "New template" (opens existing create modal/flow), "New API key" (links to /api-keys).
- **FR-5:** Post-login redirect shall land on `/dashboard` instead of `/` (existing sessions unaffected; the square remains reachable via sidebar).
- **FR-6:** The dashboard shall be the "My Workspace" nav entry in the sidebar (top position, above Prompt Square for logged-in users).

### 5.2 My Templates tab (`/dashboard/templates`)

- **FR-7:** Lists all templates owned by the user (public + private, all types), paginated (20/page), sorted by updated_at desc; columns/cards show: title, visibility badge, type badge (system/user), category, tags (first 3), like/favorite counts, updated date.
- **FR-8:** Row/card actions: Edit (navigates to template detail edit mode), Delete (confirmation modal; existing delete API), Toggle visibility (existing update API).
- **FR-9:** Filter chips: All / Public / Private / System / User type; client-side filter is acceptable in v1 (page size bounded).
- **FR-10:** Empty state: "You haven't created any templates yet" + CTA to create.

### 5.3 My Prompts tab (`/dashboard/prompts`)

- **FR-11:** Lists all prompts (instantiated) owned by the user across all templates, paginated (20/page), sorted by created_at desc; each row: variables summary (first 2 key=value pairs, truncated), source template title (link), created date.
- **FR-12:** Row actions: View (opens the prompt content in a modal — variables rendered into the template content), Delete (confirmation; existing delete API).
- **FR-13:** Filter by source template (dropdown of my used templates; client-side acceptable v1).
- **FR-14:** Empty state: "No prompts yet — use a template to create one" + CTA to the square.

### 5.4 My Collections & API keys entries

- **FR-15:** The dashboard nav includes "My Collections" (links to /collections/mine from the collections feature; if that feature is not yet shipped, the entry is hidden) and "API Keys" (links to existing /api-keys).
- **FR-16:** The overview stat cards link to their respective tabs/pages.

### 5.5 API

- **FR-17:** New RPC `GetDashboardOverview()` → `{template_count, prompt_count, collection_count, api_key_count, recent_activity: [{type, entity_id, entity_title, timestamp}]}`. REST: `GET /api/v1/dashboard/overview`. Auth required.
- **FR-18:** Recent activity is derived from existing tables (templates.updated_at, prompts.created_at, template_likes/template_favorites.created_at) — no new event table; a UNION query with LIMIT 10.
- **FR-19:** My Templates tab reuses `ListTemplates` with `owner_id=me` (existing capability); My Prompts tab reuses `ListPrompts` with `owner_id=me` (existing capability).
- **FR-20:** Prompt "View" modal content is assembled client-side: fetch template latest version content (existing GetTemplate) and substitute variables (existing placeholder logic) — no new API.

### 5.6 Regular-user vs admin split

- **FR-21:** The dashboard is exclusively the regular-user surface. Admin users see their own personal dashboard identically (admin console remains separate under /admin).
- **FR-22:** No admin APIs or pages are part of this feature.

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Overview endpoint p95 < 400ms (four COUNT queries + one UNION LIMIT 10, all indexed).
- **NFR-2 (Compatibility):** No breaking changes; new route + one new read-only endpoint; existing pages unchanged.
- **NFR-3 (Privacy):** All dashboard data is owner-scoped; no cross-user data; no new PII.
- **NFR-4 (Accessibility):** Stat cards, tabs, tables, and modals keyboard-operable; activity feed as a semantic list (see UI/UX doc §10).
- **NFR-5 (i18n):** All new strings EN/ZH in sync; timestamps locale-formatted; relative time ("2h ago"/"2小时前") via shared util.
- **NFR-6 (Resilience):** If the collections feature is absent, the dashboard degrades gracefully (hides collection card/nav; overview endpoint returns collection_count: 0 or omits).
- **NFR-7 (Security):** Overview endpoint requires auth; all queries scoped by authenticated user id from token (never from client params).

## 7. Acceptance Criteria

- **AC-1:** After login, the user lands on `/dashboard` and sees four stat cards with correct counts.
- **AC-2:** The recent-activity feed shows up to 10 items with correct icons, links, and relative timestamps.
- **AC-3:** Quick actions work: "New template" opens the create flow; "New API key" navigates to /api-keys.
- **AC-4:** My Templates lists all owned templates with working Edit/Delete/Toggle-visibility actions and filters.
- **AC-5:** My Prompts lists instantiated prompts with source template links; View modal renders substituted content; Delete works.
- **AC-6:** Anonymous access to any /dashboard/* route redirects to login.
- **AC-7:** The dashboard shows no data belonging to other users.
- **AC-8:** With the collections feature not shipped, the dashboard hides collection UI without errors.
- **AC-9:** Stat cards navigate to their tabs/pages.
- **AC-10:** All new UI strings exist in both `en` and `zh` translation files with identical key sets.
- **AC-11:** Existing home/square, template detail, profile, and API keys pages behave unchanged (no regression).
- **AC-12:** Overview endpoint responds < 400ms p95 with 1k templates/prompts (indexed queries).

## 8. Out of Scope (v1)

- Dashboard customization/rearrangement.
- Notifications center (no notification system yet).
- Creator analytics on the dashboard (belongs to prompt-analytics feature; a link may be added when it ships).
- Bulk actions (multi-select delete).
- Search within dashboard lists.
- Activity feed pagination (top 10 only).
- Onboarding checklist ("complete your profile") — future engagement feature.
- Email digests.
- Mobile app-specific layouts (responsive web only).

## 9. Dependencies & Assumptions

- **D-1:** `ListTemplates` with `owner_id` filter and `ListPrompts` with `owner_id` filter already exist and return sufficient fields (confirmed in proto/service); no backend list changes needed for tabs.
- **D-2:** Recent activity UNION (FR-18) requires `template_likes`/`template_favorites` to have `created_at` (confirmed in schema); templates activity uses `updated_at` as a proxy for "created/updated" (v1 does not distinguish).
- **D-3:** The collections feature (template-collections) may not be shipped yet; the dashboard must feature-detect (NFR-6). Detection: presence of the collections API (a 404 on the collections endpoint hides the UI) or a build-time flag — architect/developer choice.
- **D-4:** Post-login redirect (FR-5) changes the login success handler target from `/` to `/dashboard`; the square remains the default for anonymous visitors.
- **D-5:** Prompt View modal (FR-20) reuses the existing placeholder-substitution logic from the template use flow (frontend/src/pages/TemplateDetails.js) — extract to a shared util if not already.
- **D-6:** Relative-time formatting util is new but trivial (minutes/hours/days); shared across dashboard and future features.
- **D-7:** The sidebar gains a "My Workspace" entry; existing entries (Prompt Square, My Prompts, My Likes, My Favorites) remain — "My Prompts" sidebar entry may later point to the dashboard tab (v1 keeps both; de-duplication is a follow-up).
- **D-8:** Stat card counts use exact COUNT queries; caching is unnecessary at current scale.

## 10. Open Questions for Architect

*(None blocking. D-3 collections feature-detection mechanism is the main flagged decision.)*
