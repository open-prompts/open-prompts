# Admin Announcements — Requirements Analysis

- **Feature ID:** admin-announcements
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-13
- **Companion doc:** `admin-announcements-ui-ux-design.md`

## 1. Competitor Research

| Product | Announcement capability | Strength | Weakness |
|---|---|---|---|
| GitHub | global banner for critical notices; org/repo-level discussions | Unmissable for critical items | One-at-a-time; no scheduling |
| Discourse | admin "banner topic" pinned atop all pages until dismissed | High visibility, dismissible | Single active banner; no audience targeting |
| Notion | in-app "What's new" modal + changelog page | Rich media, versioned history | Easy to miss; no urgency levels |
| Linear | changelog feed + occasional full-screen modal for major releases | Clean, product-led | No admin targeting by audience |
| Vercel | status banner + changelog | Operational clarity | Split surfaces |
| PromptBase/FlowGPT | none (no announcement system) | — | Prompt platforms have nothing to copy; SaaS admin patterns apply |

**Takeaway:** The winning pattern is a **single active banner** (GitHub/Discourse model) for time-boxed, high-visibility notices, plus an **announcement history list** (Notion/Linear model) so users can catch up later. Audience targeting by role (admin-only vs all-users) covers internal ops without a separate channel. Prompt platforms offer nothing here — SaaS admin-console patterns apply directly.

## 2. Problem Statement

The platform has no way to tell users anything: maintenance windows, new features, policy changes, or incident notices. Today the only channels are out-of-band (email/README). Admins need an in-product, time-boxed, dismissible way to reach all users; users need a place to see current and past announcements without being nagged forever.

## 3. Target Users

- **Admins** — publish, schedule, edit, and end announcements; see read/dismiss metrics.
- **All users** — see the active announcement, dismiss it, and browse history.
- **Admins as recipients** — receive admin-only operational announcements (e.g. "deploy window") in the same system.

## 4. User Stories

- **US-1**: As an admin, I want to publish a banner announcement with a start and end time so that it appears to all users during a defined window.
- **US-2**: As an admin, I want to choose an audience (all users / admins only) so that operational notices don't noise regular users.
- **US-3**: As an admin, I want to mark an announcement as critical (non-dismissible) or standard (dismissible) so that urgency is respected.
- **US-4**: As an admin, I want to edit or end an active announcement so that mistakes and changing plans are correctable.
- **US-5**: As an admin, I want to see read and dismiss counts so that I know reach.
- **US-6**: As a user, I want the active announcement shown as a banner I can dismiss (when dismissible) so that it doesn't block my work.
- **US-7**: As a user, I want dismissed announcements to stay dismissed across sessions so that I'm not re-nagged.
- **US-8**: As a user, I want an announcement history page so that I can catch up on what I missed.
- **US-9**: As a user, I want a link in the banner (optional) so that I can act on the notice (e.g. changelog, status page).

## 5. Functional Requirements

- **FR-1**: Admins can create an announcement with: title (required, ≤ 120 chars), body (required, ≤ 2000 chars, plain text with line breaks), optional link URL + link label, audience (`all_users` | `admins_only`), severity (`standard` | `critical`), and `starts_at`/`ends_at` (ISO-8601; starts_at defaults to now; ends_at required).
- **FR-2**: At most **one** announcement may be active per audience at a time; publishing a new one to an audience with an active announcement requires explicitly ending the active one (the UI offers "end & replace").
- **FR-3**: An announcement is **active** when `now ∈ [starts_at, ends_at)`; the banner renders only for active announcements matching the viewer's audience (admins see both `all_users` and `admins_only`).
- **FR-4**: `critical` announcements render with a distinct style and are **not dismissible**; `standard` announcements are dismissible per user.
- **FR-5**: Dismissal is persisted server-side per user per announcement (`announcement_dismissals` table); a dismissed standard announcement never re-renders for that user, across sessions and devices.
- **FR-6**: The banner appears at the top of every authenticated page (above the header), is dismissible via an × button (standard only), and shows title + body excerpt + optional link.
- **FR-7**: A "View all announcements" link from the banner (and from the user menu) opens `/announcements` — a reverse-chronological list of announcements for the viewer's audience with title, body, severity chip, and active window; paginated (20/page).
- **FR-8**: Admin console: `/admin/announcements` lists all announcements (any state) with status (scheduled/active/ended), audience, severity, window, and read/dismiss counts; `/admin/announcements/new` and `/admin/announcements/:id/edit` provide the form.
- **FR-9**: Admin APIs: `POST /api/v1/admin/announcements` (create), `GET /api/v1/admin/announcements` (list, filterable by state), `GET /api/v1/admin/announcements/{id}`, `PATCH /api/v1/admin/announcements/{id}` (edit while scheduled or active), `POST /api/v1/admin/announcements/{id}/end` (set ends_at = now), `DELETE /api/v1/admin/announcements/{id}` (scheduled only).
- **FR-10**: User APIs: `GET /api/v1/announcements/active` → the viewer's active announcement (or empty), `GET /api/v1/announcements?page_size=20&page_token=...` → history for the viewer's audience, `POST /api/v1/announcements/{id}/dismiss` (standard only; idempotent).
- **FR-11**: A view is counted once per user per announcement (first render of the banner or history item) via a fire-and-forget beacon; read counts are approximate and advisory only.
- **FR-12**: Editing an active announcement does not reset dismissals or read counts; editing is limited to title, body, link, and ends_at (audience and severity are immutable after publish).
- **FR-13**: All admin announcement actions (create/edit/end/delete) are written to the audit log with actor, target id, and changed fields.
- **FR-14**: Announcements are plain text only — no HTML, Markdown rendering, or embedded media in v1; URLs in the link field must be http(s).
- **FR-15**: The banner must not appear on the admin login page or regular login/register pages (unauthenticated surfaces).
- **FR-16**: When an announcement ends (ends_at passes), the banner disappears for everyone without a page refresh within 60 seconds (client polls active announcement at most every 60 s while any banner is visible; otherwise on page load).

## 6. Non-Functional Requirements

- **NFR-1 (Performance)**: `GET /announcements/active` responds in < 50 ms p95; the banner adds no perceptible layout shift (reserved height once loaded).
- **NFR-2 (Scale)**: dismissal and read tracking must not generate unbounded writes — one row per user per announcement, updated at most once.
- **NFR-3 (i18n)**: All UI strings EN/ZH; announcement content is admin-authored and shown as-is (no translation).
- **NFR-4 (Accessibility)**: The banner is a labelled region; critical banners use `role=alert` on first render; dismissal is keyboard-accessible.
- **NFR-5 (Safety)**: Link URLs are sanitized (http/https only) and open in a new tab with `rel="noopener noreferrer"`; body text is rendered as text, never HTML (XSS-safe by construction).

## 7. Acceptance Criteria

- **AC-1**: Creating an announcement with starts_at = now and ends_at = +7d makes it visible immediately to all users as a dismissible standard banner.
- **AC-2**: Publishing a second all-users announcement while one is active prompts "end & replace"; confirming ends the old one and activates the new one atomically.
- **AC-3**: A critical announcement renders with distinct styling and no × button; a standard one shows ×.
- **AC-4**: Dismissing a standard announcement persists after logout/login and in another browser (server-side dismissal).
- **AC-5**: An admins_only announcement is invisible to regular users (both banner and history) but visible to admins.
- **AC-6**: `/announcements` lists ended announcements newest-first with severity chips and windows; pagination works at 20/page.
- **AC-7**: Editing an active announcement's body does not reset its read/dismiss counters, and users who dismissed it still don't see it.
- **AC-8**: `POST /announcements/{id}/end` sets ends_at = now; the banner disappears for all users within 60 s without refresh.
- **AC-9**: A regular user calling any `/api/v1/admin/announcements` endpoint receives 403.
- **AC-10**: A body containing `<script>alert(1)</script>` renders as literal text, never executes.

## 8. Out of Scope

- Email/push delivery of announcements; notification-center integration (announcements are their own surface).
- Rich text/Markdown bodies, images, or embedded media.
- Audience targeting beyond all-users/admins-only (no per-user, per-language, or cohort targeting).
- Scheduling queues beyond starts_at/ends_at (no recurrence, no timezone-aware windows).
- Announcement translation workflows.
- Monetization of any kind — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1**: The audit-log infrastructure from admin-user-management is reused for FR-13.
- **D-2**: The admin console shell (`/admin` layout, nav, auth guard) from admin-user-management hosts the new pages.
- **D-3**: The authenticated app shell (header/layout component) is the single place to mount the banner.
- **D-4**: Fire-and-forget beacons follow the template-watch viewed-beacon pattern (non-blocking, lossy-tolerant).
- **D-5**: No interaction with the notifications system (notifications FR-30 keeps them separate); announcements are a standalone surface.

## 10. Open Questions for Architect

- **OQ-1**: Should `GET /announcements/active` be part of the initial page-load bootstrap payload (avoiding a flash of no-banner), or a separate call? (Proposal: separate call with reserved banner height.)
- **OQ-2**: Is a dedicated `announcement_dismissals` (user_id, announcement_id) table preferred, or a JSONB array on users? (Proposal: dedicated table — indexable, bounded.)
- **OQ-3**: Should read tracking reuse the dismissals table with a `read_at` column, or a separate `announcement_reads` table? (Proposal: same table, `read_at` nullable column.)
- **OQ-4**: Does the 60-second active-announcement poll need jitter to avoid thundering herds, or is the user base small enough to ignore?
- **OQ-5**: Should DELETE be allowed for active announcements (with audit), or scheduled-only as proposed (active must be ended first)?
