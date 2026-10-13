# Template Watch — Requirements Analysis

- **Feature ID:** template-watch
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-13
- **Companion doc:** `template-watch-ui-ux-design.md`

## 1. Competitor Research Summary

| Product | Watch/subscribe pattern | Takeaway |
|---|---|---|
| **GitHub** | "Watch" on repos: All activity / Releases only / Ignore; watchers get issue/PR/release notifications; watch count shown to owner only | Watch is the artifact-level complement to following people; granularity levels keep noise low |
| **YouTube** | Subscribe to channel; per-video notifications via bell (All / Personalized / None) | The bell's per-item granularity is the answer to over-subscription |
| **Medium** | Follow writer; "Subscribe to story" via email on comments | Subscribing to a single story for follow-up discussion is a distinct, common need |
| **Reddit** | "Follow post" saves it to a personal feed with new-comment badges | Following a post ≠ bookmarking: the user wants *future changes*, not just a link |
| **npm** | "Watch package" for new releases; RSS per package | Release-watching is the strongest analogy for template version updates |
| **PromptBase / FlowGPT / Snack Prompt** | None — no artifact-level subscription exists on any prompt platform | Differentiator opportunity; also confirms the pattern is not expected baseline |

**Synthesis:** The winning v1 pattern is the **npm/GitHub hybrid**: a single **Watch** toggle on public template detail pages (no granularity levels in v1 — one level, "notify me on new versions"), a `template_watchers` table, a `template_version_published` notification event emitted when a watched template gets a new version, a **Watched** list in the user dashboard (with unread-update badges), and a **"N updates" banner** on the template detail page for returning watchers. Watching is deliberately separate from favoriting (bookmark/appreciation) and from following (person-level): a user watches a template because *that specific prompt* matters to their work and they want to know when it changes. The follow-creators design explicitly deferred "Watch template" to a future feature — this is that feature.

## 2. Problem Statement

1. **Templates change silently.** A creator publishes v5 of a prompt a consumer relies on daily; the consumer never learns the prompt they copy-pasted last week was fixed or improved. There is no artifact-level subscription.
2. **Favorite is the wrong tool.** Users favorite templates to bookmark them; favorites generate no signal when the template updates. There is no way to say "tell me when *this* changes."
3. **Follow is too broad.** Following the creator notifies about *all* their new templates (via the Following feed); a user who cares about exactly one prompt gets noise.
4. **Returning watchers see nothing.** Even a user who manually re-visits a template gets no indication that it changed since their last visit — no "updated" badge, no version delta hint.

## 3. Target Users

- **Consumers who rely on specific prompts:** daily users of a particular template who want to know when it improves or breaks.
- **Forkers:** users who forked a template and want to track upstream changes to cherry-pick improvements.
- **Creators (secondary):** see a watcher count as a quality/interest signal on their own templates (owner-only metric).

## 4. User Stories

- **US-W1:** As a consumer, I can click "Watch" on a public template so that I am notified when a new version is published.
- **US-W2:** As a consumer, I can unwatch at any time from the detail page or my Watched list, so that I control noise.
- **US-W3:** As a consumer, I see a "new version" notification in my existing notification center when a watched template publishes a version, so that I learn about updates where I already look.
- **US-W4:** As a consumer, I have a "Watched" list in my dashboard showing all watched templates with unread-update badges, so that I can triage updates in one place.
- **US-W5:** As a returning watcher, I see an "Updated since you last viewed — v4 → v6" banner on the template detail page, so that I immediately know what changed.
- **US-W6:** As a watcher, I can jump from the notification/banner to a version diff, so that I can evaluate the change in seconds.
- **US-W7:** As a creator, I can see how many users watch my template (owner-only), so that I understand which work matters most to people.
- **US-W8:** As a user, watching is suppressed for my own templates (I authored them; I know when they change), so that I never self-notify.
- **US-W9:** As a user, I can watch only public templates, so that watching cannot leak private content existence.

## 5. Functional Requirements

### 5.1 Watch action

- **FR-1:** Any authenticated user can watch any **public** template they do not own via a Watch toggle on the template detail page. Watching a private template, a foreign private template, or an archived template returns 404/403 consistent with existing visibility semantics; watching one's own template returns 400 with a clear message.
- **FR-2:** New table `template_watchers`: `id UUID PK`, `user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE`, `template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE`, `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`, `last_viewed_version INT NOT NULL DEFAULT <current version at watch time>`, `last_notified_version INT NOT NULL DEFAULT <current version at watch time>`, `UNIQUE(user_id, template_id)`. Watching again after unwatching re-inserts with fresh defaults.
- **FR-3:** `POST /api/v1/templates/{id}/watch` (watch) and `DELETE /api/v1/templates/{id}/watch` (unwatch). Both idempotent (watching an already-watched template is a no-op 200; unwatching a non-watched template is a no-op 200). Auth required; anonymous 401.
- **FR-4:** When a template is archived, hard-deleted, or made private by its owner, existing watch rows are **retained but dormant**: no notifications are emitted while the template is not publicly visible. If the template becomes public again, watching resumes. (Rows are only removed by explicit unwatch or user/template deletion.)

### 5.2 Version-update event

- **FR-5:** When `UpdateTemplate` creates a new version of a template (content change, existing behavior), the system shall emit a `template_version_published` notification to every active watcher of that template, except: the template owner (never notified for own template), banned users (no notifications per existing semantics), and watchers whose `last_notified_version >= new version` (dedupe).
- **FR-6:** The notification payload shall include: template id, template title, previous version number, new version number, and change note (if the template-versioning feature's `change_note` is present; empty string otherwise). The notification links to the template detail page with the version diff pre-opened (`/templates/{id}?diff={prev}..{new}`).
- **FR-7:** Emission is best-effort and transactional with the version insert where feasible; on failure the version is still created (a missed notification is acceptable; a lost version is not). A single batch insert per publish is preferred over N individual inserts.
- **FR-8:** The event type slots into the existing notifications system and the notification-preferences panel as a data-driven type (`template_version_published`), default **on**, suppressible at emission per the existing preference mechanism. The mention-notifications design established this data-driven pattern; no preference-schema change is needed.
- **FR-9:** Rate safety: a template that publishes many versions in a short window (e.g., an active editing session) emits at most one notification per watcher per **1 hour** per template (the newest version wins; intermediate versions are collapsed — "v3 → v7" in one notification). This prevents notification storms from rapid-save workflows.

### 5.3 Watched list (dashboard)

- **FR-10:** The user dashboard gains a **Watched** tab (`/dashboard/watched`) listing watched templates (existing template list item shape so `PromptCard` renders unchanged), sorted by **most recent update first** (templates with newer versions than `last_viewed_version` at top, marked with an unread badge), then by watch date descending.
- **FR-11:** Each row/card shows: template card content (existing), an **unread badge** ("2 new versions") when `current_version > last_viewed_version`, a **Watch toggle** (quick unwatch), and the watch date.
- **FR-12:** Opening a watched template's detail page updates `last_viewed_version` to the current version (server-side on page load via the existing `GET /api/v1/templates/{id}` response handling — a lightweight `PUT /api/v1/templates/{id}/watch/viewed` beacon, fire-and-forget), clearing the unread badge.
- **FR-13:** `GET /api/v1/users/me/watched?page=&page_size=` → `{templates: [<existing list item shape> + `new_versions: int`, `watched_at`>], total, page, page_size}`. Owner-scoped; paginated (default 20, max 50).

### 5.4 Returning-watcher banner

- **FR-14:** For a logged-in watcher viewing a template whose current version > their `last_viewed_version`, the detail page shows a dismissible banner: "Updated since you last viewed — v4 → v6 · [View changes]". Clicking View changes opens the template-versioning DiffModal for v4→v6. The banner's appearance also triggers the FR-12 viewed-update beacon.
- **FR-15:** The banner is hidden for non-watchers, anonymous users, the owner, and watchers with no new versions.

### 5.5 Owner metrics

- **FR-16:** The template detail response (`GET /api/v1/templates/{id}`) shall include `watcher_count` **only in the owner-scoped response** (the existing owner-view enrichment pattern from template-health-hints FR-15); public responses never include it. The owner's dashboard "My Templates" may show the count as a column (v1 optional, UI-level).

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Watch/unwatch is a single-row upsert/delete (< 10ms typical). The publish-time fan-out is one batch insert per publish; templates with > 10,000 watchers are capped at the first 10,000 by `created_at` (documented; prompt platforms rarely approach this).
- **NFR-2 (Privacy):** Watcher lists are never public (no "who watches this" surface in v1); only the aggregate count is shown, owner-only. Watching does not appear on profiles.
- **NFR-3 (Consistency):** Unwatch during a publish race may produce one stale notification; acceptable (documented). No notification is emitted for restores (template-versioning FR: restore creates a new version — it IS a version publish and notifies normally).
- **NFR-4 (i18n):** All UI strings localized EN/ZH; notification title/body localized per the existing notification rendering.
- **NFR-5 (Compatibility):** No change to favorite, follow, or like semantics; the Watch button is a separate control with its own icon (eye) to avoid confusion with the star (favorite).

## 7. Acceptance Criteria

- **AC-1:** Authenticated user opens a public template they don't own → Watch button visible → click → button becomes "Watching" (filled eye); notification-preferences shows the new `template_version_published` toggle (default on).
- **AC-2:** Owner publishes a new version (content change) → every eligible watcher receives exactly one `template_version_published` notification with prev→new version numbers and change note; the owner receives nothing.
- **AC-3:** Watcher opens the template detail page → "Updated since you last viewed — v4 → v6" banner shows → clicking View changes opens the v4→v6 diff → banner and unread badge clear on next dashboard visit.
- **AC-4:** Rapid publishing (5 versions within 10 minutes) → watchers receive at most one notification per hour per template, showing the collapsed range (v3 → v8).
- **AC-5:** Watched tab lists watched templates with unread badges sorted most-recent-update-first; unwatch from the list removes the row without a page reload.
- **AC-6:** Watching a private template (non-owner) → 404; watching own template → 400 with message; unwatching while not watching → 200 no-op.
- **AC-7:** Owner makes template private → watchers get no further notifications; owner re-publishes → next version notifies watchers again.
- **AC-8:** Template owner sees `watcher_count` on their template detail page; a non-owner viewer's response does not contain the field.
- **AC-9:** Disabling `template_version_published` in notification preferences → no rows created for subsequent publishes (suppression at emission, consistent with notification-preferences).
- **AC-10:** Deleting a watched template (hard delete) removes watch rows (cascade) and the Watched tab no longer lists it.

## 8. Out of Scope (v1)

- Watch granularity levels (GitHub-style "releases only / all activity / ignore") — one level only.
- Watcher lists (who watches this template) — aggregate count only, owner-only.
- Email/push channels — in-app only (consistent with the product's notification infrastructure).
- Watching comments or forks of a template — version updates only.
- "Watch collection" / "watch chain" — templates only; other entity types if demand emerges.
- Monetization of any kind — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1:** The notifications system (shipped) and notification-preferences (designed) provide the event-type registry, unread badge, history page, and emission-time suppression; this feature adds one type.
- **D-2:** template-versioning provides version numbers, the DiffModal, and `change_note`; the banner deep-links into that modal.
- **D-3:** `UpdateTemplate` creating a new version on content change is existing backend behavior; the emit hook attaches to that code path (same pattern webhooks uses for lifecycle events).
- **D-4:** The user dashboard (designed) provides the tabbed hub; Watched is a new tab alongside My Templates / My Prompts / My Collections.
- **D-5:** follow-creators §Out-of-Scope explicitly reserved "Watch template" for this feature; no overlap with the Following feed (person-level, newest-templates).
- **D-6:** Assumption: `PromptCard` and the template list item shape are stable enough to extend with two optional fields (`new_versions`, `watched_at`).

## 10. Open Questions for Architect

1. **Fan-out mechanics:** batch insert into `notifications` at publish time (one INSERT ... SELECT over `template_watchers`) vs. per-watcher inserts in a goroutine — which fits the existing `createEngagementNotification` pattern best? Recommend batch.
2. **Collapse window implementation:** the 1-hour collapse (FR-9) needs a per-(watcher, template) "last emitted at" check — reuse `last_notified_version` + a `last_notified_at` column, or query recent notifications at emit time? Recommend the column (one extra column, no emit-time query).
3. **Viewed beacon:** is a fire-and-forget `PUT .../watch/viewed` acceptable, or should `GET /api/v1/templates/{id}` perform the update server-side when the caller is a watcher (no extra request, but a write inside a read path)? Recommend the beacon (keeps GET pure).
4. **Watcher cap:** is the 10,000-watcher cap (NFR-1) worth implementing in v1, or defer until data suggests it? Recommend defer with a documented TODO.
5. **Restore semantics confirmation:** template-versioning's restore creates a new version — confirm this flows through the same publish hook (assumed yes, D-3).
