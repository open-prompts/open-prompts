# Team Collaboration — Requirements Analysis

- **Feature ID:** team-collaboration
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. Competitor Research Summary

| Platform | Team / sharing approach | Key takeaways |
|---|---|---|
| **AIPRM** | Team/role separation (owner/admin/member) in professional tiers; shared prompt library with usage telemetry | Role separation (owner/admin/member) is the expected professional model; a shared library is the core artifact |
| **Notion** | Workspaces → pages; granular share (view/comment/edit); invite by email | "Shared with me" as a first-class surface; invite-by-email is the default mental model |
| **Google Drive** | Owner + editors + viewers on folders/files; "shared with me" list | Per-object permissioning is powerful but heavy; folder-level sharing is what teams actually use |
| **GitHub (orgs/repos)** | Org → repos → collaborator permission levels; transfer ownership | Transfer of ownership is the escape hatch every team tool needs eventually |
| **PromptBase / FlowGPT / Snack Prompt** | No team features (individual accounts only) | Prompt platforms are all individual today — a differentiation opportunity |
| **Slack (channels)** | Invite links with expiry; join without an existing account relationship | Invite links beat email invitations for small teams (no email infra dependency) |

**Synthesis:** The winning v1 pattern is the **shared workspace**: a team is a named group with a shared template space. Any user creates a team, becomes its owner, and invites members by **user id** (the product's public identifier, matching `users.id` format) — deliberately not email invitations, to avoid email-infrastructure dependencies. Templates can be owned by a team (team-owned) instead of a user; team members with the right role can edit them. Roles are exactly three: **owner** (one, transferable; everything + delete team + manage members), **admin** (manage members, edit all team templates), **member** (create/edit own templates in the team space, view all team templates). Team templates are private-to-team by default and can be published publicly by an admin/owner (publishing keeps team ownership, attribution shows the team). Personal templates are unaffected; a user's existing content stays personal unless explicitly transferred. This is the lightest model that makes "our team's prompt library" real.

## 2. Problem Statement

1. **Teams share prompts by export files.** Today the only way a group shares templates is import/export or making everything public — no shared space, no access control, no single source of truth.
2. **No attribution for team content.** A template written by a departing employee is owned by their personal account; the team loses access when the account goes.
3. **No roles.** The product has user/admin platform roles but nothing for group-level permissions; sharing a template means sharing your whole account or nothing.
4. **Individual-only competitors.** Every prompt platform is individual-only; teams (agencies, dev teams, support teams) are an unserved segment with the strongest retention profile.

## 3. Target Users

- **Team creators (primary):** users wanting a shared prompt library for their group (dev team, agency, support team).
- **Team owners/admins:** manage membership and the team's published presence.
- **Team members:** contribute templates to the shared space and reuse colleagues' work.
- **Admins (platform):** view teams in the admin console (member counts, template counts) for abuse response; no team management tooling in v1.

## 4. User Stories

- **US-T1:** As a user, I want to create a team with a name and description, so that my group has a shared space.
- **US-T2:** As a team owner, I want to invite members by their user id, so that we can start collaborating without email infrastructure.
- **US-T3:** As a team owner, I want to assign admin and member roles, so that permissions match responsibility.
- **US-T4:** As a team member, I want to create templates inside the team space, so that my work belongs to the team, not my personal account.
- **US-T5:** As a team member, I want to view and use all team templates, so that I benefit from colleagues' work.
- **US-T6:** As a team admin, I want to edit any team template, so that the team library stays accurate.
- **US-T7:** As a team owner, I want to transfer ownership to another member, so that the team survives my departure.
- **US-T8:** As a team owner, I want to remove a member, so that access ends when membership ends.
- **US-T9:** As a user, I want to see all teams I belong to in one place, so that I can switch between personal and team spaces.
- **US-T10:** As a platform admin, I want to see teams and their sizes in the admin console, so that I can respond to abuse reports.

## 5. Functional Requirements

### 5.1 Data model

- **FR-1:** New table `teams`: `id UUID PK`, `name VARCHAR(80) NOT NULL`, `description TEXT` (0–500 chars), `owner_id TEXT NOT NULL REFERENCES users(id)`, `created_at`, `updated_at`. Index `(owner_id)`.
- **FR-2:** New table `team_members`: `team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE`, `user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE`, `role VARCHAR(10) NOT NULL CHECK (role IN ('owner','admin','member'))`, `joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`, PRIMARY KEY `(team_id, user_id)`. Index `(user_id)`.
- **FR-3:** `templates` gains a nullable `team_id UUID REFERENCES teams(id) ON DELETE SET NULL` column (migration via the existing ALTER TABLE pattern). `team_id IS NULL` = personal template (existing behavior unchanged); `team_id` set = team-owned. A CHECK constraint ensures exactly one of `owner_id`/`team_id` semantics: team templates keep `owner_id` as *creator* (audit) but access control flows from `team_id`.
- **FR-4:** Limits: a user may belong to at most 10 teams; a team has at most 50 members; a team may own at most 200 templates. Server-enforced with friendly errors.

### 5.2 Team management (regular-user surface)

- **FR-5:** Authenticated users create teams at `/teams/new`: name (required, unique per platform? — see Open Questions), description. Creator becomes owner (row in team_members with role owner).
- **FR-6:** Team home `/teams/{id}` shows: team name, description, member list (avatar, display name, role badge), team templates grid (existing card patterns), and management actions per role.
- **FR-7:** **Invites:** owner/admin adds a member by exact user id (validated to exist and not banned; error if already a member). No email invitations, no invite links in v1. The invited user appears immediately with role member.
- **FR-8:** **Roles & permissions:**
  - owner: everything; transfer ownership (demotes self to admin); delete team; remove any member; change roles.
  - admin: add/remove members (not owner/admin removal of others? — admins manage members incl. removing members, but cannot remove other admins or the owner); edit/delete any team template; publish/unpublish team templates.
  - member: create templates in the team space; edit/delete only templates they created; view all team templates.
- **FR-9:** **Leaving:** members/admins may leave a team at any time (self-service). The owner cannot leave without transferring ownership or deleting the team.
- **FR-10:** **Deletion:** deleting a team (owner only, confirmation modal typing the team name) soft-deletes the team; its templates' `team_id` is set to NULL and `visibility` to private, with `owner_id` (creator) retaining personal ownership — content survives, access reverts to creators.
- **FR-11:** **My teams:** `/teams` lists teams the user belongs to (card: name, member count, template count, role badge), plus a "Create team" CTA. The Header user dropdown gains a "My Teams" entry.

### 5.3 Team templates

- **FR-12:** Creating a template "in a team" happens from the team page's create action (create modal pre-set to team context) or by transferring an existing personal template to the team (owner/admin of the team + template owner).
- **FR-13:** Team templates are visible to all team members regardless of visibility; `private` team templates are invisible outside the team; `public` team templates appear in the public square with team attribution ("by {team name}").
- **FR-14:** Editing a team template uses the same editor and versioning as personal templates; the edit permission check is role-based (FR-8) instead of `owner_id == me`.
- **FR-15:** Likes, favorites, comments, ratings, forks on team templates behave identically to personal templates (they target the template, not the owner); notifications for team template events go to the *creator* (owner_id) in v1.

### 5.4 API

- **FR-16:** `POST /api/v1/teams`, `GET /api/v1/teams?member=me&page=`, `GET /api/v1/teams/{id}`, `PUT /api/v1/teams/{id}` (name/description), `DELETE /api/v1/teams/{id}`, `POST /api/v1/teams/{id}/members` (body: user_id), `DELETE /api/v1/teams/{id}/members/{user_id}`, `PUT /api/v1/teams/{id}/members/{user_id}` (role change), `POST /api/v1/teams/{id}/transfer` (body: user_id), `POST /api/v1/teams/{id}/leave`, `POST /api/v1/teams/{id}/templates` (transfer existing template, body: template_id), and template create/update within team context via existing endpoints extended with optional `team_id`.
- **FR-17:** All team operations enforce role checks server-side; membership is required for any team-scoped read except public team template views.

### 5.5 Admin surface

- **FR-18:** Admin console gains a "Teams" page under `/admin/teams`: list (name, owner, member count, template count, created), search by name, and a read-only member drill-down. No team mutation in v1 (abuse response uses existing user ban: banning a team owner does not delete the team; the team becomes unmanaged — see Open Questions).
- **FR-19:** Admin APIs: `GET /api/v1/admin/teams?query=&page=`, `GET /api/v1/admin/teams/{id}` (with members). Read-only.

### 5.6 Notifications integration

- **FR-20:** If the notifications feature has shipped: `team_invite` (to the invited user: team name, inviter), `team_role_change` (to the affected member), `team_removed` (to a removed member). If notifications has not shipped, these are deferred without blocking the feature.

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Team page with 50 members and 200 templates loads < 1 s (paginated template grid, member list capped display).
- **NFR-2 (Security):** All permission checks server-side (FR-17); team-scoped queries filter by membership; no team data leaks through public APIs (private team templates never appear in public lists/search).
- **NFR-3 (Compatibility):** Existing personal-template behavior is 100% unchanged (team_id NULL path); the migration is additive.
- **NFR-4 (i18n):** All strings EN/ZH via i18next (`teams.*` namespace); role names translated (Owner/Admin/Member → 所有者/管理员/成员).
- **NFR-5 (Accessibility):** Role badges text+color; member lists and actions keyboard operable; modals follow existing focus-trap patterns.
- **NFR-6 (Consistency):** Team templates reuse every existing template surface (cards, detail, editor, versioning, moderation); only ownership/permission semantics differ.

## 7. Acceptance Criteria

- **AC-1:** A user creates a team, adds a member by user id, and the member sees the team in `/teams` with role badge "Member".
- **AC-2:** A member creates a template in the team space; other members can view and use it; it does not appear in the member's personal templates or the public square while private.
- **AC-3:** An admin edits a member-created team template; the member (non-creator) cannot edit another member's template (403).
- **AC-4:** Publishing a team template makes it public with "by {team name}" attribution; unpublishing reverts to team-private.
- **AC-5:** The owner transfers ownership to an admin; the former owner becomes admin and can no longer delete the team.
- **AC-6:** A member leaves the team; their team-created templates remain in the team (creator attribution preserved).
- **AC-7:** Deleting a team nulls team_id on its templates, sets them private, and leaves them with their creators.
- **AC-8:** A non-member cannot access a private team's page or its private templates (404/403); private team templates never appear in public search.
- **AC-9:** Admin `/admin/teams` lists teams with member/template counts; a regular user cannot access admin team APIs (403).
- **AC-10:** Limits enforced: the 51st member is rejected; the 11th team membership is rejected; friendly errors.
- **AC-11:** All strings translated EN/ZH; no hardcoded copy.
- **AC-12:** No monetization content anywhere in the feature (no team tiers, no seat pricing, no paid plans).

## 8. Out of Scope

- Email invitations, invite links, or joining by code.
- Team-level collections/chains (v1 is templates only; teams gain other content types later).
- Granular per-template permissions beyond the three roles.
- Team activity feeds, audit logs for team actions (platform audit log unchanged).
- Team API keys (keys remain personal).
- Monetization of any kind (payments, pricing, purchases, earnings, seat fees, team tiers) — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1:** `users.id` is a public, shareable identifier (alphanumeric + underscore, shown in profiles) — invites by user id rely on this.
- **D-2:** The templates table and all template surfaces (list, detail, editor, versioning, moderation) are extended, not replaced; `team_id` is additive.
- **D-3:** The existing ALTER TABLE IF NOT EXISTS migration pattern (used for users.role/status) applies to adding templates.team_id.
- **D-4:** The admin console's list-page pattern (search + table + pagination) is reused for `/admin/teams`.
- **D-5:** The notifications feature's event mechanism (if shipped) carries team events (FR-20).
- **D-6:** The follow-creators feature's owner identity block gains a team variant ("by {team name}") for public team templates.

## 10. Open Questions for Architect

1. **Team name uniqueness:** Should team names be globally unique (like users.id) or unique only per owner? Recommendation: globally unique, case-insensitive — simpler attribution and URLs.
2. **Banned owner:** If a team owner is banned, the team has no active owner. Options: (a) auto-promote the longest-tenured admin, (b) leave unmanaged until platform admin intervenes, (c) block banning owners with active teams. Recommendation: (b) + admin console visibility in v1.
3. **Template transfer semantics:** When a personal template transfers to a team, should likes/favorites/comments/ratings follow the template (they do — they target the template) and should the creator keep personal attribution alongside team attribution? Recommendation: show "by {team}, created by {user}".
4. **team_id ON DELETE SET NULL vs CASCADE:** FR-10 uses SET NULL + visibility private on team delete (content survives). Confirm this against the soft-delete approach (soft-deleted team keeps templates until purge).
5. **Public team attribution:** Does the public template payload need a `team: {id, name}` object (like the follow-creators owner object), and how should moderation display team-owned templates (owner column shows team)?
6. **Search scope:** Should the public square's search include public team templates by default (recommended) and offer no team filter in v1, or add a "team templates" filter?
