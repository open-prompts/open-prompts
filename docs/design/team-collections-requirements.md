# Team Collections — Requirements Analysis

- **Feature ID:** team-collections
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-13
- **Companion doc:** `team-collections-ui-ux-design.md`

## 1. Competitor Research

| Product | Team content organization | Strength | Weakness |
|---|---|---|---|
| Spotify | Collaborative playlists (any member adds/reorders) | Shared curation, one artifact | No roles on playlists |
| YouTube | Playlists owned by a channel; channel members manage | Role-aligned with channel | Playlist itself is single-owner |
| Notion | Teamspaces: shared pages with per-member permissions | Full permission model | Heavy for simple lists |
| AIPRM | Team prompt libraries shared across the team | One shared library per team | No sub-grouping within the library |
| LangFlow | Projects contain flows; teams share projects | Grouping unit matches team | No collection-of-flows layer |

**Takeaway:** The Spotify collaborative-playlist model maps cleanly onto what exists: template-collections built the collection artifact (ordered list, visibility, stable URLs) but scoped to a single owner; team-collaboration built teams with roles (owner/admin/member) and team-owned templates but explicitly deferred "team-level collections" (line 119). The union is: a collection whose *owning principal* is a team, with add/remove/reorder rights flowing from team roles.

## 2. Problem Statement

Teams can now own templates, but a team with 60 templates has no way to organize them into shared groups ("Client onboarding", "Code review prompts"). Personal collections belong to individuals — when a member leaves, their collections leave with them. Teams need collections that belong to the team itself, curated by any member with edit rights, surviving membership changes.

## 3. Target Users

- **Team owners/admins** — create team collections, control who can curate.
- **Team members** — add/remove/reorder templates in team collections (when permitted).
- **Team visitors** — browse a public team's collections.

## 4. User Stories

- **US-1**: As a team owner, I want to create collections owned by my team, so that curation survives membership changes.
- **US-2**: As a team owner, I want to choose whether all members can curate or only admins, so that control matches the team's trust model.
- **US-3**: As a team member, I want to add team templates to a team collection from the template page, so that organizing is in-context.
- **US-4**: As a team member with curate rights, I want to reorder and remove templates, so that the collection stays current.
- **US-5**: As a team member, I want to see which of my team's collections a template is already in, so that I don't duplicate.
- **US-6**: As a team owner, I want to transfer or delete team collections deliberately, so that shared work isn't lost accidentally.
- **US-7**: As a visitor, I want to browse a public team's collections, so that teams can share curated sets publicly.
- **US-8**: As a team member, I want personal and team collections clearly distinguished everywhere, so that I always know whose collection I'm in.

## 5. Functional Requirements

- **FR-1**: `collections` gains a nullable `team_id` (FK to teams, ON DELETE CASCADE); `team_id IS NULL` = personal collection (existing behavior unchanged); `team_id` set = team collection. Exactly one of `user_id`/`team_id` is set (CHECK constraint); for team collections `user_id` records the *creator* (audit only).
- **FR-2**: Team owners and team admins create team collections from the team home (`/teams/{id}`) with the same fields as personal collections (name 1–80, description 0–500, visibility private default | public).
- **FR-3**: Curation policy per team: `member_curation` (all members curate — default) or `admins_only` (owner + admins only), set by the team owner on the team settings page; the policy applies to all of the team's collections.
- **FR-4**: Curate rights (per policy) allow: add template, remove template, reorder, edit name/description/visibility. Deleting a team collection is owner/admin-only regardless of policy.
- **FR-5**: Team collections may contain: templates owned by the team, public templates, and templates a member personally owns (same eligibility as personal collections, template-collections FR-7).
- **FR-6**: Adding a template to a team collection is available from the template detail page and card overflow — the "Add to collection" picker lists personal collections and each team's collections (grouped, with team avatar), filtered by the user's curate rights.
- **FR-7**: The picker marks templates already in a collection (checkmark, disabled) — idempotent no-op parity with template-collections FR-8.
- **FR-8**: Team collection pages (`/collections/{id}`) render identically to personal ones plus a team badge (avatar + name linking to `/teams/{id}`); the team home gains a "Collections" section listing the team's collections.
- **FR-9**: Limits: a team may have at most 50 collections; a team collection holds at most 100 templates (same as personal); server-enforced with friendly errors.
- **FR-10**: Deleting a team collection (owner/admin) requires typing the collection name (destructive-action pattern); soft-delete per template-collections FR-3.
- **FR-11**: When a member leaves or is removed, team collections are unaffected (they belong to the team); the member's *personal* collections are unaffected.
- **FR-12**: When a team is deleted (team-collaboration FR-10), its collections are soft-deleted with it (CASCADE semantics); public team collection URLs render a "no longer available" page.
- **FR-13**: All curation actions (create/edit/delete/add/remove/reorder) write to the team's activity surface (team-collaboration's audit/activity pattern) with actor, action, and target.
- **FR-14**: Visibility rules match personal collections: private team collections are visible to team members only (any role); public team collections are visible to everyone; search/browse integration follows template-collections' public surfaces.
- **FR-15**: The user's "My collections" page shows personal collections only; team collections are reachable via team home and the collection URL (no mixed listing).

## 6. Non-Functional Requirements

- **NFR-1 (Performance)**: Team collection list and pages load within the same budget as personal collections (< 300 ms p95).
- **NFR-2 (Consistency)**: Team collections reuse the collection artifact end-to-end — no forked rendering paths, only the ownership principal and permission checks differ.
- **NFR-3 (i18n)**: All UI strings EN/ZH; collection content is user content, shown as-is.
- **NFR-4 (Durability)**: Curation survives membership churn by construction (ownership on the team, not the member).
- **NFR-5 (Safety)**: Destructive actions (delete collection) use type-to-confirm; policy changes are owner-only and audited.

## 7. Acceptance Criteria

- **AC-1**: A team owner creates "Client onboarding" on the team home; it appears in the team's Collections section with the team badge.
- **AC-2**: With `member_curation`, a member adds a team template to the collection from the template page; with `admins_only`, the member's picker omits that team's collections.
- **AC-3**: The picker shows a checkmark on collections that already contain the template; clicking them is a no-op.
- **AC-4**: A member with curate rights reorders and removes templates; the changes persist for all team members.
- **AC-5**: A private team collection is invisible to non-members (direct URL → 404) but visible to every team member.
- **AC-6**: A public team collection is browsable by any visitor from the team home.
- **AC-7**: Deleting a team collection requires typing its name; it then disappears for all members.
- **AC-8**: A member leaving the team changes nothing about team collections; their personal collections remain theirs.
- **AC-9**: Deleting the team soft-deletes its collections; a previously public collection URL shows "no longer available".
- **AC-10**: Creating a 51st team collection fails with a friendly limit error.

## 8. Out of Scope

- Per-collection curation policies (policy is per-team in v1).
- Collection-level roles or outside collaborators (curators who aren't team members).
- Team chains (prompt-chaining is personal-scope in v1).
- Collection following/subscribing (deferred by template-collections as v2).
- Analytics on team collections (prompt-analytics is template-scope).
- Monetization of any kind — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1**: template-collections' artifact (schema, ordered items, visibility, stable URLs, soft delete) is the base; this feature adds the team principal.
- **D-2**: team-collaboration's teams, roles (owner/admin/member), team home, and settings page exist as designed.
- **D-3**: The "Add to collection" picker component from template-collections is extended (grouping + rights filtering), not forked.
- **D-4**: Team activity/audit surface (team-collaboration) consumes curation events (FR-13).
- **D-5**: Team deletion semantics (FR-10 there) extend to cascade collections (FR-12 here).

## 10. Open Questions for Architect

- **OQ-1**: Should `collections.user_id` stay NOT NULL (recording creator for team collections) or become nullable with a separate `created_by` column? (Proposal: keep user_id as creator — minimal migration.)
- **OQ-2**: Is the per-team `member_curation` policy a column on `teams` or a team-settings key-value? (Proposal: column — one boolean-ish enum.)
- **OQ-3**: Should team collections appear in global collection search/browse alongside personal ones, or only via team home? (Proposal: alongside — visibility rules already gate.)
- **OQ-4**: Does the 50-collections-per-team limit share budget with anything, or stand alone?
- **OQ-5**: Should curation events also emit notifications to team members (e.g. "Alice added X to Client onboarding"), or stay audit-only in v1? (Proposal: audit-only — notification volume risk.)
