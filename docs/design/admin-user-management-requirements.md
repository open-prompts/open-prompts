# Admin User Management System — Requirements Analysis

- **Feature ID:** `admin-user-management`
- **Version:** 1.0
- **Status:** Approved for architecture design
- **Author:** UI/UX Agent (Product Analysis)
- **Audience:** Architect, Developer, Test agents

---

## 1. Competitor Research Summary

Open Prompts currently has **zero administrative capability**: no admin role, no admin console, no moderation tools, and no oversight of user activity. To ground this feature in industry practice, we studied how comparable prompt-marketplace and UGC platforms solve admin & moderation:

| Platform | Admin approach | Key capabilities | Takeaway for Open Prompts |
|---|---|---|---|
| **PromptBase** | Curated marketplace with a seller/publisher review pipeline | Staff review every submitted prompt before it goes public; seller accounts can be suspended; content quality bar enforced pre-publication | A moderation queue is the core admin surface; pre-publication review is their differentiator but heavy for v1 |
| **FlowGPT** | Community UGC platform with report/flag flows | Users report prompts; moderators review reported content, ban spam accounts, feature/highlight quality prompts | Report-driven moderation scales better than pre-review; "featured" curation drives discovery |
| **Snack Prompt** | Curated prompt feed | Editorial curation (daily picks), user blocking, spam filtering | Curation + spam control are table stakes once volume grows |
| **AIPRM** | Extension-based prompt library | Centralized library management, team/role separation (owner/admin/member), usage telemetry | Role separation (admin vs member) is expected in professional tools |
| **PromptHero** | Discovery/leaderboard platform | Content takedowns, account suspension, leaderboard curation | Takedown + suspension are the minimum viable moderation actions |
| **Generic SaaS admin consoles** (Stripe, Vercel, Supabase) | Dedicated `/admin` route space, separate auth realm or elevated permission check, audit log | Dashboard with KPIs, paginated searchable user list, drill-down detail drawer, audit trail of admin actions | The "dashboard + list + detail + audit log" pattern is the industry-standard admin IA |

**Synthesis.** The consistent industry pattern is:

1. A **dedicated admin route space** (`/admin/*`) visually and navigationally separate from the end-user product.
2. **Dashboard-first IA**: admins land on an overview with KPIs (users, templates, growth) before drilling into lists.
3. **User list with search + filter + bulk actions** as the workhorse screen.
4. **Ban / unban / role management** as the core account actions, always paired with a **reason + audit record**.
5. **Template moderation overview** (flagged/reported content) as the second pillar, since content oversight is why admins exist.
6. **Separate admin authentication** — either a separate login realm or a strictly enforced permission check, never a hidden client-side-only gate.

For v1 we adopt the dashboard + user management + template moderation overview pattern, and deliberately defer pre-publication review queues and monetization/featured curation to later features.

---

## 2. Problem Statement

Open Prompts is a growing UGC platform where any registered user can publish public prompt templates. Today there is **no way** for an operator to:

- See how many users/templates exist or how the platform is growing (no metrics at all).
- Find a specific user or segment of users (no admin user list).
- Stop a malicious or spamming user (no ban capability).
- Grant or revoke administrative access (no role model).
- Review public templates for policy violations (no moderation view).

As public content grows, the platform accumulates unbounded moderation and safety risk with no tooling to manage it. This feature closes the biggest capability gap: **a secure, separate admin console for user and template oversight**.

## 3. Target Users

| Persona | Description | Primary needs |
|---|---|---|
| **Platform Operator (Admin)** | The person or team running the Open Prompts instance. Technical, uses the product daily. | At-a-glance platform health; find users fast; ban/unban with reasons; manage roles; review public templates |
| **Secondary: Regular user** | Indirect beneficiary. | A cleaner, safer public template square (they never see the admin console) |

## 4. User Stories

### 4.1 Admin personas

- **US-A1** — As an admin, I want to log into a dedicated admin console (separate from the regular user app) so that administrative power is isolated from my normal product usage.
- **US-A2** — As an admin, I want to see a dashboard of key metrics (total users, new users, total templates, public templates, prompts created, recent signups) so that I can gauge platform health at a glance.
- **US-A3** — As an admin, I want to search users by ID, email, or display name so that I can locate any account quickly.
- **US-A4** — As an admin, I want to filter users by status (active/banned) and role (admin/user) so that I can work through segments (e.g., all banned users).
- **US-A5** — As an admin, I want to paginate through the user list so that I can browse large datasets predictably.
- **US-A6** — As an admin, I want to view a user's full profile (registration date, avatar, template count, OAuth identities, API keys count) so that I can assess an account before acting.
- **US-A7** — As an admin, I want to ban a user with a required reason so that abusive accounts are stopped and the action is auditable.
- **US-A8** — As an admin, I want to unban a user with a reason so that mistakes can be reversed, also auditable.
- **US-A9** — As an admin, I want to promote a trusted user to admin, and demote an admin back to user, so that operational access matches the team.
- **US-A10** — As an admin, I want to see a template moderation overview (all public templates with owner, likes, favorites, dates; sortable) so that I can spot policy-violating or spam content.
- **US-A11** — As an admin, I want to view any template's detail (including private ones) so that I can investigate a reported template fully.
- **US-A12** — As an admin, I want every admin action (ban/unban/role change) recorded with actor, target, action, reason, timestamp so that operations are accountable.
- **US-A13** — As an admin, I want the admin console to support EN/ZH languages and light/dark themes like the main app so that my team can work in their preferred locale.

### 4.2 Regular-user impact stories

- **US-U1** — As a regular user, I want banned users' public templates hidden from the public square so that spam content disappears from discovery.
- **US-U2** — As a regular user, I want to receive a clear error when my banned account tries to log in so that I understand why access is denied.
- **US-U3** — As a regular user, I never see any admin UI, links, or routes so that the product surface stays clean.

## 5. Functional Requirements

### 5.1 Admin authentication & access control

- **FR-1** The system shall support an `admin` role on user accounts, stored server-side (users table `role` column, default `user`).
- **FR-2** Admin web console routes shall live under `/admin` (e.g. `/admin/login`, `/admin`, `/admin/users`, `/admin/users/:id`, `/admin/templates`, `/admin/templates/:id`, `/admin/audit-log`).
- **FR-3** Admin APIs shall live under `/api/v1/admin` (e.g. `POST /api/v1/admin/login`, `GET /api/v1/admin/stats`, `GET /api/v1/admin/users`, `GET /api/v1/admin/users/{id}`, `POST /api/v1/admin/users/{id}/ban`, `POST /api/v1/admin/users/{id}/unban`, `POST /api/v1/admin/users/{id}/role`, `GET /api/v1/admin/templates`, `GET /api/v1/admin/audit-log`).
- **FR-4** All `/api/v1/admin/*` endpoints shall require a valid JWT whose subject has the `admin` role; non-admin tokens shall receive `403 Forbidden`; unauthenticated requests shall receive `401 Unauthorized`.
- **FR-5** Admin login shall be a **separate page** (`/admin/login`) using the same credential set (email + password) as regular login, but issuing a token that carries the admin role claim; a non-admin attempting admin login sees "This account does not have admin access."
- **FR-6** A regular user navigating to any `/admin/*` route shall be redirected to `/admin/login` (if not authenticated) or shown a 403 page (if authenticated but not admin).
- **FR-7** The first admin account shall be bootstrapped via a server-side environment variable (e.g. `ADMIN_BOOTSTRAP_EMAIL`) or an idempotent SQL migration that promotes a configured email; no open sign-up for admin.
- **FR-8** Admin sessions shall expire and require re-login after a configurable idle window (default 30 minutes of inactivity), shorter than regular user sessions.

### 5.2 Admin dashboard

- **FR-9** The dashboard shall display KPI cards: Total Users, Active Users, Banned Users, Total Templates, Public Templates, Prompts Created (7d/30d toggle where data supports it).
- **FR-10** The dashboard shall display a "Recent signups" list (latest 10 users with ID, display name, email, registered time) linking to user detail.
- **FR-11** The dashboard shall display a "Recently updated public templates" list (latest 10) linking to template detail.
- **FR-12** All dashboard numbers shall be computed server-side from the database (no client-side aggregation over list endpoints).

### 5.3 User management

- **FR-13** The user list shall support server-side pagination (default 20/page, options 10/20/50/100) with total count.
- **FR-14** The user list shall support search by keyword matching user ID, email, or display name (case-insensitive partial match).
- **FR-15** The user list shall support filters: status (all/active/banned), role (all/admin/user).
- **FR-16** The user list shall support sorting by created_at (default newest first) and updated_at.
- **FR-17** Each row shall show: avatar (or initial), display name, user ID, email, role badge, status badge, template count, created date, and a row action menu (View / Ban or Unban / Change role).
- **FR-18** Clicking a row (or "View") opens the **user detail page** showing: profile summary, statistics (templates, likes received, favorites received, prompts created), OAuth identities, API keys (name, prefix, status, last used — never the key itself), and admin action history for this user.
- **FR-19** **Ban** shall require a reason (free text, 3–500 chars). Banning sets `status='banned'`, records an audit entry, and immediately invalidates the banned user's active sessions/API-key usage.
- **FR-20** **Unban** shall require a reason. Unbanning sets `status='active'` and records an audit entry.
- **FR-21** **Role change** (promote to admin / demote to user) shall require a reason, record an audit entry, and prevent self-demotion (an admin cannot demote themselves) to avoid lockout.
- **FR-22** A banned user's login (password and OAuth) shall be rejected with a clear "account banned" error; their API keys shall be rejected with `403`.
- **FR-23** A banned user's public templates shall be excluded from public listing endpoints (ListTemplates with public visibility) and from category/tag counts.
- **FR-24** Admin shall be able to view any template including private ones (read-only oversight; editing/deleting templates is out of scope for v1).
- **FR-24a** Admin user list shall include a "Templates" count column and the user detail page shall list that user's templates (id, title, visibility, likes, created) with links to admin template detail.

### 5.4 Template moderation overview

- **FR-25** The template moderation list shall support server-side pagination, search by title/owner, filter by visibility (all/public/private) and sort by created/updated/likes.
- **FR-26** Each row shall show: title, owner (name + ID), visibility badge, type, language, tags, like count, favorite count, created/updated dates, and a "View" action.
- **FR-27** Admin template detail shall show full template metadata plus all versions (id, version number, created) read-only, and a link back to the owner's user detail.
- **FR-28** The moderation list shall surface a "Recently updated" default sort so admins see fresh content first.

### 5.5 Audit log

- **FR-29** Every admin mutation (ban, unban, role change) shall be recorded in an `admin_audit_logs` table with: id, actor_id, actor_email, action, target_type, target_id, reason, created_at.
- **FR-30** The audit log page shall list entries newest-first with pagination and filter by action type and actor.
- **FR-31** Audit entries shall be immutable (no update/delete endpoints).

### 5.6 Regular-user surface changes

- **FR-32** The regular app shall show no admin entry points. (A future feature may add an "Admin" link in the header dropdown for admins; v1 keeps surfaces fully separate.)
- **FR-33** Login errors shall distinguish "invalid credentials" from "account banned" so banned users get an actionable message.

## 6. Non-Functional Requirements

- **NFR-1 Security.** Admin endpoints must be unreachable for regular users at the API layer (server-side role check on every request, not just UI hiding). Admin login rate-limited (e.g. 5 attempts/15min/IP). Ban must invalidate sessions promptly.
- **NFR-2 Performance.** User list and template list endpoints respond < 500ms p95 at 10k users / 100k templates (DB indexes on role, status, created_at; count queries optimized).
- **NFR-3 Auditability.** No admin mutation without a reason; audit log append-only; actor always recorded from the verified JWT (never client-supplied).
- **NFR-4 i18n.** All admin UI strings in EN and ZH via i18next, following the existing `locales/{en,zh}/translation.json` pattern (admin namespace `admin.*`).
- **NFR-5 Theming.** Admin console uses the same Aurora design tokens and light/dark theme support as the main app.
- **NFR-6 Accessibility.** Keyboard navigable (focus trap in modals, Escape closes), ARIA labels on icon buttons, color contrast ≥ 4.5:1 in both themes.
- **NFR-7 Compatibility.** No breaking changes to existing user-facing APIs; `role`/`status` columns added via idempotent migration (init.sql pattern with `DO $$ ... IF NOT EXISTS ...`).
- **NFR-8 Data privacy.** Admin views expose only operational data (never password hashes, never full API keys). Emails shown to admins are acceptable for platform operation.
- **NFR-9 Responsiveness.** Admin console usable at ≥ 1024px; tables degrade gracefully (horizontal scroll) on smaller widths; mobile (< 768px) is best-effort, not a target.
- **NFR-10 Deployability.** Works with the existing docker-compose and helm chart; no new infrastructure dependencies.

## 7. Acceptance Criteria

**AC-1 Admin login separation.** Given a non-admin account, when I submit credentials at `/admin/login`, then I see "This account does not have admin access" and no admin token is issued.

**AC-2 Admin route guard.** Given an unauthenticated visitor at `/admin/users`, then I am redirected to `/admin/login?redirect=...`; given a logged-in regular user, then I see a 403 page.

**AC-3 Dashboard KPIs.** Given an admin on `/admin`, then I see Total Users / Active / Banned / Total Templates / Public Templates / Prompts Created cards with correct server-computed values, plus Recent Signups and Recent Public Templates lists.

**AC-4 User search & filter.** Given 25+ users, when I search "gmail" and filter status=banned, then only matching banned users appear, with correct pagination metadata.

**AC-5 Ban flow.** Given an active user, when I ban them with reason "spam", then: their status becomes `banned`; an audit entry (actor=me, action=ban, target=user, reason) is created; their next login attempt fails with "account banned"; their public templates vanish from the public square; their API keys return 403.

**AC-6 Unban flow.** Given a banned user, when I unban with reason "appeal accepted", then status returns to `active`, audit entry created, login works again, public templates reappear.

**AC-7 Role management.** Given an admin, when I promote a user to admin with reason, then that user can log into `/admin/login` and their list row shows an admin badge; when I attempt to demote myself, the action is rejected with a clear error.

**AC-8 User detail.** Given a user with 3 templates and 2 API keys, their detail page shows template count 3, lists the templates, and lists API key names/prefixes/status (no secrets).

**AC-9 Template moderation list.** Given mixed public/private templates, when I filter visibility=private, then only private templates appear with owner info; sorting by likes works.

**AC-10 Audit log.** Given prior ban/unban/role actions, the audit log lists them newest-first with actor, action, target, reason, timestamp; entries cannot be edited or deleted.

**AC-11 i18n & theme.** Switching language to ZH translates all admin console strings; toggling theme applies light/dark to all admin pages.

**AC-12 Regular surface unchanged.** Given a regular user, the main app shows no admin links; public square excludes banned users' templates.

## 8. Out of Scope (v1)

- Pre-publication template review queue (submit → approve flow) — future feature.
- User reporting/flagging by regular users — future feature (pairs with moderation queue).
- Admin editing or deleting templates — v1 is read-only oversight; delete/takedown is a follow-up.
- Bulk actions (bulk ban, bulk role change) — future.
- Admin notification/email to banned users — future.
- Admin management of categories/tags taxonomy — future feature.
- Admin analytics beyond the dashboard KPIs (charts, trends, retention) — future feature.
- SSO/OAuth login *into the admin console* — v1 is email+password only.
- Mobile-optimized admin layout (< 768px) — best effort only.
- Admin invitation flow / multi-tenant admin teams — future.

## 9. Dependencies & Assumptions

- **Assumption:** `users` table gains `role TEXT NOT NULL DEFAULT 'user'` and `status TEXT NOT NULL DEFAULT 'active'` columns via idempotent migration; `admin_audit_logs` table created.
- **Assumption:** JWT carries `role` claim; gateway middleware enforces admin-only on `/api/v1/admin/*`.
- **Assumption:** The existing REST gateway (cmd/server/main.go) hosts the new admin routes; no separate admin service.
- **Dependency:** Architect to define proto/REST contract details; Developer to implement; Test to cover AC-1..AC-12 in e2e.
