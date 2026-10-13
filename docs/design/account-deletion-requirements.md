# Account Deletion — Requirements Analysis

- **Feature ID:** account-deletion
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-13
- **Companion doc:** `account-deletion-ui-ux-design.md`

## 1. Competitor Research Summary

| Product | Account deletion pattern | Takeaway |
|---|---|---|
| **GitHub** | Settings → Danger Zone → "Delete your account"; requires typing username + 2FA; explains what transfers/stops; org ownership blocks deletion | Explicit friction (type-to-confirm) prevents accidents; blockers are surfaced up front |
| **OpenAI** | Settings → Data controls → Delete account; email confirmation link; states retention window | Email-confirmation loop is the standard second factor for account actions |
| **Notion** | Settings → Members → Delete account; workspace-with-content blocks with guidance | "You own things others depend on" must be resolved before deletion |
| **Reddit** | No self-serve deletion of content; account deactivation keeps posts (username disassociated) | Content-vs-account separation: communities keep the content, identity is removed |
| **Vercel/Supabase** | Delete project before delete account; final confirmation modal with itemized consequences | Itemized consequence lists ("this will delete X templates, Y collections...") set expectations |
| **PromptBase / FlowGPT** | Minimal or hidden deletion flows | Trust differentiator: a clean, honest exit builds long-term trust |

**Synthesis:** The winning v1 pattern is the **GitHub/Vercel "Danger Zone" model**: a **Danger Zone section** in the existing Profile/Settings page with a "Delete account" action that opens a **consequence-summary modal** (itemized counts of what will be deleted vs. anonymized), requires **typing the word `DELETE`** (or the username) plus **password re-verification**, then executes a **soft-delete grace period (14 days)** during which login is blocked but the user can restore by contacting support or logging in with a restore flow, followed by **hard deletion with anonymization**. Public content the community depends on (comments, ratings) is **anonymized, not deleted** (Reddit pattern) — the author becomes "Deleted user"; private content (templates, prompts, collections, presets, API keys) is **deleted**. Team ownership blocks deletion until transferred (GitHub pattern). No email infrastructure is required: confirmation is in-app (password + type-to-confirm), and the grace period is the safety net.

## 2. Problem Statement

1. **No exit exists.** The backend has no `DeleteAccount` RPC; users who want to leave have no self-serve path. The only recourse is contacting an admin — which does not scale and violates user-data-control expectations (GDPR-style right to erasure).
2. **Trust asymmetry.** Users can create accounts in seconds but cannot remove them. For a platform handling personal API keys and content, a clean exit is a trust feature.
3. **Cascade complexity is unaddressed.** A user's deletion touches: owned templates (with forks, likes, comments from others), comments they wrote on others' templates, ratings, follows, blocks, collections, presets, API keys, notifications, team memberships/ownerships, and audit references. No design currently defines these semantics.
4. **Community content dilemma.** Deleting a user's comments outright destroys other people's conversations (threads become incoherent); deleting their public templates breaks forks' lineage and others' favorites. The anonymize-vs-delete distinction must be explicit.

## 3. Target Users

- **Departing users:** anyone who wants to leave and erase their personal data.
- **Privacy-conscious users:** evaluate the product's data practices partly by whether leaving is possible.
- **Admins (secondary):** get a clean lifecycle (soft-deleted users are visible in the admin console with restore/force-delete tools).
- **The community (implicit):** conversations and fork lineage survive a departure.

## 4. User Stories

- **US-1:** As a user, I can find "Delete account" in my Profile settings' Danger Zone, so that the exit is discoverable without support.
- **US-2:** As a user, before confirming I see an itemized summary of what will be deleted vs. anonymized (my templates count, comments count, etc.), so that I make an informed decision.
- **US-3:** As a user, I must re-enter my password and type `DELETE` to confirm, so that accidental or hijacked deletion is prevented.
- **US-4:** As a user, after confirming I get a 14-day grace period during which I can cancel by logging in, so that a moment of frustration or a mistake is recoverable.
- **US-5:** As a user, after the grace period my account and private data are permanently deleted and my public comments/ratings are anonymized ("Deleted user"), so that my identity is erased while conversations stay coherent.
- **US-6:** As a team owner, I cannot delete my account until I transfer ownership or dissolve my teams, so that teammates are never orphaned.
- **US-7:** As an OAuth-only user (no password), I can confirm deletion via a re-authentication prompt from my provider, so that the flow works for every account type.
- **US-8:** As an admin, I can see soft-deleted (pending-deletion) users in the admin console, restore them within the grace window, or force-complete deletion, so that support requests are actionable.
- **US-9:** As a user, my API keys are revoked immediately at soft-delete time, so that a departing account cannot keep calling the API during the grace period.
- **US-10:** As a user, I can export my templates before deleting (link to the existing import-export feature), so that leaving does not mean losing my work.

## 5. Functional Requirements

### 5.1 Discovery and confirmation

- **FR-1:** The Profile page (`/profile`) gains a **Danger Zone** section (visually distinct, danger-tinted border) containing the "Delete account" action. Visible to all authenticated users.
- **FR-2:** Clicking "Delete account" opens a modal showing an itemized consequence summary fetched from a new endpoint: counts of owned templates (public/private), prompts, collections, comments written, ratings given, teams owned, API keys, and presets. The modal distinguishes **deleted** items (templates, prompts, collections, presets, API keys, personal data) from **anonymized** items (comments, ratings — content stays, authorship becomes "Deleted user").
- **FR-3:** Confirmation requires: (a) password re-verification for password accounts — wrong password shows an error and blocks; (b) for OAuth-only accounts, a re-authentication redirect to the provider (existing OAuth login flow re-run); AND (c) typing the word `DELETE` exactly. Both must pass before the button enables.
- **FR-4:** The modal includes a link to the import-export feature ("Export your templates first") and to the team-collaboration ownership-transfer flow if the user owns teams.

### 5.2 Soft delete (grace period)

- **FR-5:** Confirming calls `POST /api/v1/users/me/delete` (auth required). The backend sets `users.status = 'pending_deletion'` and `users.deletion_scheduled_at = now() + 14 days`, and immediately: revokes all API keys (existing key-deletion semantics), invalidates all sessions, and hides the user from all public surfaces (profile page returns 404, follows severed, blocks retained for safety).
- **FR-6:** During the grace period: login with credentials returns 403 with a specific "account scheduled for deletion" error and a **Cancel deletion** path (re-authentication + confirm cancel restores `status = 'active'`); the user's public templates remain visible (community continuity) but are marked read-only (no new comments/likes/forks — existing banned-user semantics reused); their comments remain visible with anonymization deferred to hard delete.
- **FR-7:** A scheduled job (the webhooks design's DB-backed due-scan ticker pattern, 30s pass) finds users with `deletion_scheduled_at <= now()` and `status = 'pending_deletion'` and executes hard deletion.

### 5.3 Hard deletion semantics

- **FR-8:** **Deleted outright (cascade):** owned templates (with their versions, aliases, forks-of-nothing — see FR-9 for others' forks), prompts, collections, presets (variable-presets), API keys, notifications, watch rows, team memberships. Team **ownership** is a blocker (FR-11) so no team is orphaned.
- **FR-9:** **Others' forks of the deleted user's templates survive** (fork lineage preserved): `forked_from_template_id` references are kept but the source template row is deleted — the lineage display (template-remix-forks-tree) shows "deleted source" gracefully. Others' favorites/likes on the deleted templates are removed (target gone); their counts vanish with the template.
- **FR-10:** **Anonymized (content preserved, identity removed):** comments written by the user on others' templates (author becomes a tombstone "Deleted user" with null avatar — the comment-replies-threads structure stays coherent), ratings given by the user (rating rows anonymized: `user_id` set to NULL, aggregate averages unchanged), mentions of the user in others' comments (text preserved — historical record; the mention no longer links).
- **FR-11:** **Blockers:** if the user owns any team (team-collaboration owner role), deletion is refused with 409 and a list of team names; the user must transfer ownership or dissolve teams first. Admin accounts cannot self-delete (403, message directs to another admin).
- **FR-12:** The `users` row is anonymized rather than removed (referential integrity for audit logs and anonymized comments): `email`/`mobile` overwritten with `deleted+{id}@invalid`, `display_name` = "Deleted user", `avatar` = NULL, `bio` = NULL, `password_hash` = NULL, `status = 'deleted'`. The row is excluded from all listings and lookups.

### 5.4 Admin surface

- **FR-13:** The admin users list (`/admin/users`) gains a status filter including `pending_deletion`; rows show "Deletion scheduled: <date>" with **Restore** (cancels, sets `active`, audit-logged) and **Delete now** (force-completes hard deletion, requires reason, audit-logged) actions.
- **FR-14:** `POST /api/v1/admin/users/{id}/restore` and `POST /api/v1/admin/users/{id}/force-delete` (reason required, 3–500 chars) — both write to the existing `admin_audit_logs` with new action codes `restore_user` / `force_delete_user`.

### 5.5 Cancel flow

- **FR-15:** A pending-deletion user who attempts login sees a dedicated interstitial page: "Your account is scheduled for deletion on {date}" with **Cancel deletion** (re-authenticate → confirm → restored) and **Keep waiting** (logout). No other app access during grace.

## 6. Non-Functional Requirements

- **NFR-1 (Security):** Password re-verification server-side (hash compare), not client-trusted. Type-to-confirm is client-side friction only. All deletion endpoints require fresh authentication (existing session or re-auth).
- **NFR-2 (Performance):** Hard deletion of a user with thousands of templates runs as a background job (not in the request path); the API returns 202-style semantics (the ticker picks it up). Grace-period cancel is instant.
- **NFR-3 (Privacy):** After hard deletion, no API response ever contains the user's email, mobile, display name, or avatar. Anonymized comments show only "Deleted user". Audit logs retain admin action records (action, target id, reason) — no PII beyond the immutable id.
- **NFR-4 (i18n):** All UI strings, error messages, and the interstitial page localized EN/ZH.
- **NFR-5 (Data integrity):** Hard deletion is transactional per aggregate (user row + cascades in one transaction where feasible; large accounts chunked by template with idempotent resume — the ticker re-scans incomplete deletions).

## 7. Acceptance Criteria

- **AC-1:** User opens Profile → Danger Zone visible → "Delete account" → modal shows itemized counts (e.g., "3 templates, 12 comments, 1 team owned") with deleted-vs-anonymized grouping.
- **AC-2:** Confirm button stays disabled until correct password AND `DELETE` typed; wrong password shows inline error.
- **AC-3:** Confirming → sessions invalidated, API keys revoked, status `pending_deletion`; re-login shows the interstitial with the scheduled date and Cancel option; cancel restores full access.
- **AC-4:** During grace, the user's public templates are visible but read-only (comment/fork attempts blocked); their profile page returns 404.
- **AC-5:** After 14 days (or admin force-delete): user row anonymized (email tombstoned, name "Deleted user"); owned templates/prompts/collections/presets gone; their comments on others' templates still render as "Deleted user"; ratings aggregates unchanged.
- **AC-6:** A team owner attempting deletion gets 409 with team names; after transferring ownership, deletion succeeds.
- **AC-7:** Another user's fork of a deleted user's template still exists; its lineage shows "deleted source" instead of a link.
- **AC-8:** Admin filters users by `pending_deletion`, restores one (audit log entry `restore_user`), force-deletes another with a reason (audit log entry `force_delete_user`).
- **AC-9:** OAuth-only user (no password set) completes deletion via provider re-authentication instead of password entry.
- **AC-10:** After hard deletion, `GET /api/v1/users/{id}/profile` returns 404 and no endpoint returns the user's original email/name.

## 8. Out of Scope (v1)

- Email notifications of pending deletion (no email infrastructure beyond verification codes).
- Selective deletion ("delete my templates but keep my comments").
- Data export bundling into the deletion flow (link only — import-export owns export).
- GDPR formal request workflow / DSR ticketing (self-serve covers the erasure path).
- Anonymization of likes/favorites given by the deleted user (counts simply decrement as targets cascade or rows delete).
- Monetization of any kind — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1:** `users.status` exists with `active`/`banned` values (admin-user-management); `pending_deletion` and `deleted` extend the enum. Banned-user content-hiding semantics are reused for grace-period read-only behavior.
- **D-2:** team-collaboration defines team ownership and (per its design) ownership transfer; this feature blocks on owned teams.
- **D-3:** The webhooks design's DB-backed due-scan ticker (30s pass, catch-up) is the scheduler for grace-period expiry; account-deletion adds a scan target.
- **D-4:** comment-replies-threads and template-ratings provide the comment/rating structures to anonymize; template-remix-forks-tree provides lineage rendering (needs a "deleted source" state).
- **D-5:** admin-user-management provides the users list, audit log, and action-with-reason patterns; this feature adds two actions.
- **D-6:** variable-presets provides the presets table to cascade.
- **D-7:** Assumption: OAuth accounts may have no password hash — the re-auth path must exist for them (US-7).
- **D-8:** Assumption: referential integrity favors anonymizing the `users` row over deleting it (audit logs, comments reference it).

## 10. Open Questions for Architect

1. **Anonymize vs. delete the users row:** FR-12 anonymizes for referential integrity. Alternative: hard-delete the row and nullable-FK the referencing tables (comments.user_id → NULL). Which fits the existing schema constraints better? Recommend anonymize (fewer schema changes).
2. **Grace-period template visibility:** FR-6 keeps public templates visible-but-readonly during grace. Alternative: hide them immediately (cleaner privacy, worse community continuity — links break for 14 days then content vanishes anyway). Recommend visible-readonly; confirm.
3. **Deletion job mechanics:** one transaction per user vs. chunked per-template with idempotent resume (NFR-5). What's the existing pattern for large cascades? Recommend chunked with a `deletion_progress` marker on the user row.
4. **Ratings anonymization:** setting `ratings.user_id = NULL` requires the column to be nullable and the "one rating per user" unique constraint to treat NULLs as distinct (Postgres default). Confirm the ratings schema supports this, or whether rating rows should be deleted (aggregates would shift).
5. **Session invalidation:** does the existing auth stack support server-side session revocation (Redis session store or JWT denylist), or is logout client-side only? Grace-period login-blocking must be server-enforced.
