# Onboarding Experience — Requirements Analysis

- **Feature ID:** onboarding
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-10

## 1. Competitor Research Summary

| Platform | Onboarding approach | Key takeaways |
|---|---|---|
| **GitHub** | Post-signup checklist ("Verify email, Add profile photo, Create a repository") with progress bar; task links deep-link to each action; dismissible | The checklist pattern drives activation better than tours; each item must link directly to where the action happens |
| **Notion** | Welcome modal with template gallery entry; progressive "getting started" page; generous empty states with CTAs | Empty states are onboarding surfaces — every "nothing here yet" screen should teach the next action |
| **ChatGPT** | Minimal friction: signup → straight to the product; hints appear contextually on first use | Don't gate the product behind a wizard; let users start immediately and guide inline |
| **PromptBase** | Store-first: browse marketplace immediately; seller onboarding only when you click "Sell" | For a prompt platform, showing value (browse templates) before asking for setup work is the right order |
| **FlowGPT** | Straight to feed; create flow discoverable via prominent button; no guided onboarding | No onboarding leaves activation to chance — the gap we currently have |
| **AIPRM** | Extension install flow doubles as onboarding; template categories chosen at install | Asking one lightweight preference question early can personalize the first screen |
| **Reddit** | Interest picker after signup → personalized feed | A single-step personalization (pick categories) materially improves first-session engagement |

**Synthesis:** Open Prompts currently drops new users onto the home grid after registration with zero guidance. The winning v1 pattern combines: (1) a one-time welcome modal right after registration (value props + "get started"), (2) a dismissible progress checklist on the home sidebar area with 4 activation tasks that deep-link to their actions and auto-check via existing APIs, and (3) contextual empty states with CTAs everywhere a new user sees "nothing yet". No forced wizard, no blocking tour — the product stays immediately usable, guidance is ambient. All task detection uses existing endpoints (profile, templates, API keys, prompts), so v1 needs zero backend changes.

## 2. Problem Statement

1. **No activation guidance.** After registering, a user lands on the home grid with no indication of what the product's key actions are (create a template, generate an API key, use a prompt via SDK). Discovery is left entirely to chance.
2. **No welcome moment.** Registration ends with a silent redirect to `/`. There is no "you're in, here's what this gives you" moment, which is where competing products establish value.
3. **Empty states are dead ends.** `home.no_results` ("No templates found.") is a bare string with no guidance: a new user with no matching filters sees a wall of nothing with no CTA. "My templates" empty view likewise offers no "create your first template" nudge.
4. **Key workflows are invisible.** The API-key → SDK integration path (the product's core developer loop) is only discoverable by finding the API Keys page in the profile dropdown. New developers often never find it.
5. **No progress feedback.** Users who do explore get no acknowledgment; there is no sense of "I've set everything up".
6. **First-session churn risk.** All of the above compound: the user most likely to churn (first session) gets the least support.

## 3. Target Users

- **New registered users (primary):** just completed email+code registration; need to learn the 4 activation actions and feel oriented.
- **New developer users:** came for the SDK/API; need to find API key generation and the SDK snippet on a template page.
- **Returning users who never activated:** signed up earlier but never created a template or key; the checklist (if not dismissed) continues to guide them.
- **Admin console users:** out of scope for v1 (admin console is internal; see §8).

## 4. User Stories

- **US-O1:** As a new user, right after registering I see a welcome modal that explains what I can do here, so that I feel oriented instead of dropped into a grid.
- **US-O2:** As a new user, I can dismiss the welcome modal with one click and start browsing immediately, so that onboarding never blocks me.
- **US-O3:** As a new user, I see a checklist of 4 setup tasks (complete profile, create first template, generate API key, use a prompt) with my progress, so that I know what to do next.
- **US-O4:** As a user, each checklist item links directly to where the action happens, so that I don't have to hunt for it.
- **US-O5:** As a user, checklist items check themselves off automatically when I complete the action anywhere in the app, so that I don't have to manage the list manually.
- **US-O6:** As a user, I can dismiss the checklist, so that it never becomes clutter.
- **US-O7:** As a user, when I complete all 4 tasks I see a celebratory completion state, so that setup feels finished.
- **US-O8:** As a user, when a list or view is empty I see a helpful empty state with a clear next action (e.g., "Create your first template"), so that empty screens teach instead of dead-end.
- **US-O9:** As a user browsing with filters that match nothing, the empty state helps me recover (clear filters / browse all), so that I'm never stuck.
- **US-O10:** As a user on my first visit to the API Keys page, the empty state explains what keys are for, so that I understand the feature before creating one.
- **US-O11:** As a Chinese-language user, all onboarding surfaces are fully translated, so that my first experience is in my language.
- **US-O12:** As a keyboard/screen-reader user, the welcome modal and checklist are fully accessible (focus trap, labels, progress announced), so that onboarding works for me too.
- **US-O13:** As a returning user who already did the setup actions, the checklist appears pre-completed (or not at all if fully done), so that I'm not asked to redo anything.

## 5. Functional Requirements

### 5.1 Welcome modal (post-registration)

- **FR-1:** On successful registration, after landing on `/`, show a one-time welcome modal (localStorage flag `onboarding.welcome_dismissed`).
- **FR-2:** The modal contains: greeting with the user's display name, 3 value-prop rows (browse & use community templates / create & version your own / integrate via API key + SDK), and a primary "Get started" button plus "Explore templates" secondary action.
- **FR-3:** "Get started" closes the modal and reveals/scrolls to the checklist; "Explore templates" closes the modal and focuses the template grid.
- **FR-4:** The modal is dismissible via Escape, overlay click, and close button; any dismissal sets the flag so it never reappears (per browser).
- **FR-5:** The modal must not appear for logins of existing accounts — only the first navigation after registration.

### 5.2 Activation checklist

- **FR-6:** An "Onboarding" card renders on the home page (sidebar column on desktop, above the grid on mobile) for authenticated users while: not dismissed AND not all tasks complete.
- **FR-7:** The checklist has exactly 4 items, each with a completion state, label, and deep link:
  1. Complete your profile → `/profile`
  2. Create your first template → opens Create Template modal
  3. Generate an API key → `/api-keys`
  4. Use a prompt → first public template's detail page (or home grid if none)
- **FR-8:** Completion detection (on home mount, only while the card is visible):
  - Profile complete: `getProfile()` returns display_name AND avatar (email is always present).
  - Has template: `getTemplates({ owner_id: user.id })` returns ≥1 (any visibility).
  - Has API key: `listAPIKeys()` returns ≥1 active key.
  - Has prompt: `listPrompts({ owner_id: user.id })` returns ≥1.
- **FR-9:** Detection must not block rendering: the card shows items in an indeterminate state until each check resolves; failures degrade to "not complete" silently (no error toasts).
- **FR-10:** Completion also updates live: when the user performs an action elsewhere (creates a template, generates a key) and returns to home, re-checking on mount covers it; no background polling required.
- **FR-11:** Progress indicator: "2 of 4" text + progress bar (percentage width), announced politely to screen readers (`aria-live` on the count).
- **FR-12:** Each incomplete item is a link/button; each completed item shows a check icon and is no longer a link (per accessibility: disabled-looking but not focusable).
- **FR-13:** A "Dismiss" affordance (×) hides the card permanently (localStorage `onboarding.checklist_dismissed`).
- **FR-14:** When all 4 items complete: the card shows a completion state ("You're all set!") with a confetti-free celebratory style (Aurora accent, success icon), an "Explore templates" link, and auto-hides after the next navigation (flag `onboarding.checklist_completed` set).
- **FR-15:** If all 4 tasks are already complete on first render (returning user), the card does not appear at all (straight to completion flag).

### 5.3 Contextual empty states

- **FR-16:** Home "no results" (filters match nothing): illustration/icon + contextual message + actions: "Clear filters" (resets to default view) and, if authenticated, "Create a template".
- **FR-17:** "My templates" empty (owner filter, no templates): "You haven't created any templates yet" + primary CTA "Create your first template" (opens Create Template modal).
- **FR-18:** API Keys page empty (no keys): explains what API keys do (one sentence: call the API from your code) + "Generate New Key" CTA (existing button behavior).
- **FR-19:** Likes/Favorites empty views: "Templates you like/favorite will appear here" + "Browse templates" action.
- **FR-20:** All empty states must be i18n'd (EN/ZH) and accessible (the CTA is a real button; the state is announced as a region).

### 5.4 Regular-user vs admin split

- **Regular-user surfaces:** all of the above applies to the main app (`/`, `/profile`, `/api-keys`, `/templates/:id`).
- **Admin console (`/admin`):** no onboarding in v1 — the console is used by a small internal set of admins who are already trained; adding a checklist there is low-value (see §8). The shared `EmptyState` component improvements (if reused) benefit admin pages automatically.

## 6. Non-Functional Requirements

- **NFR-1 (Zero backend changes):** all detection uses existing endpoints (`getProfile`, `getTemplates`, `listAPIKeys`, `listPrompts`); no new APIs, no schema changes.
- **NFR-2 (Performance):** detection calls fire only while the checklist card is visible (max 4 requests, once per home mount); no polling; results are not cached across sessions (fresh check each mount is cheap and correct).
- **NFR-3 (Persistence):** dismissal/completion flags in localStorage (per-browser); documented limitation — flags do not sync across devices (open question Q2 proposes server-side alternative).
- **NFR-4 (Accessibility):** welcome modal reuses the accessible `Modal` (focus trap, Escape); checklist is a semantic list with `aria-checked` semantics; progress announced via `aria-live="polite"`; all per the accessibility feature's baseline.
- **NFR-5 (i18n):** all strings in `onboarding.*` namespace, EN+ZH added in the same change; no hardcoded text.
- **NFR-6 (Visual):** strictly Aurora tokens; the checklist card matches the sidebar section styling; no new colors or illustration system (simple existing icons).
- **NFR-7 (Non-blocking):** onboarding never gates navigation or any core flow; everything is dismissible; the product is fully usable with all onboarding dismissed.

## 7. Acceptance Criteria

- **AC-1:** A newly registered user sees the welcome modal exactly once; dismissing via any path (button, Escape, overlay) prevents reappearance on subsequent logins (same browser).
- **AC-2:** The modal is accessible: focus trapped, Escape closes, focus returns to the trigger context.
- **AC-3:** The checklist card appears for a new user with 0/4 complete, shows "0 of 4", and each item deep-links to the correct target.
- **AC-4:** Completing each action (profile save with name+avatar, template create, key generate, prompt use) results in the corresponding item showing checked after returning to home.
- **AC-5:** Detection failures (network error on any check) never show errors to the user; the item simply remains unchecked.
- **AC-6:** Dismissing the checklist hides it permanently; a fully-completed user never sees the card.
- **AC-7:** All four empty-state scenarios (§5.3 FR-16..19) render with correct copy and working CTAs in both EN and ZH.
- **AC-8:** "Clear filters" on the no-results empty state restores the default home view.
- **AC-9:** All onboarding UI is fully translated (no English leaks in ZH) and uses the `onboarding.*` namespace.
- **AC-10:** Keyboard-only user can complete the entire onboarding flow (dismiss modal, navigate checklist, activate items).
- **AC-11:** No new network calls fire when the checklist is dismissed or complete (verified in network panel).
- **AC-12:** The welcome modal never appears for a login (only first post-registration navigation).

## 8. Out of Scope (v1)

- Server-side onboarding state (cross-device sync of checklist/dismissal) — proposed as open question, not v1.
- Interactive product tours / spotlight overlays (coach marks).
- Category-interest personalization picker (Reddit-style) — the sidebar already exposes categories.
- Email onboarding sequences (verification email exists; marketing emails out of scope).
- Admin console onboarding.
- Gamification (badges, streaks, points).
- Re-engagement nudges ("finish setting up" banners after long absence).

## 9. Dependencies & Assumptions

- **D-1:** Existing endpoints suffice for detection: `getProfile`, `getTemplates` (with `owner_id`), `listAPIKeys`, `listPrompts` (with `owner_id`) — all present in `services/api.js`.
- **D-2:** The accessible `Modal` component (with focus trap once the accessibility feature lands) is reused for the welcome modal.
- **D-3:** The Create Template modal can be opened programmatically from the home page (it already lives in `Layout` with a floating create button; the checklist CTA triggers the same open path).
- **D-4:** localStorage is available (already used for auth token and visibility preference); keys are namespaced `onboarding.*`.
- **D-5:** The accessibility feature's baseline (focus management, aria patterns) applies to all new components here; if that feature lands first, its primitives are consumed directly.
- **D-6:** i18n keys are added to both `en` and `zh` translation files in the same change.

## 10. Open Questions for Architect

1. **Detection call budget:** 4 sequential-ish GETs on home mount while the card is visible — acceptable, or should we add a single lightweight `GET /api/v1/users/me/onboarding-status` aggregate endpoint (backend returns 4 booleans) to reduce chatter? v1 works without it; the aggregate is a nice v2.
2. **Server-side state:** should dismissal/completion flags live on the user record (e.g., a `users.onboarding_state JSONB` column or a user_preferences table) so they sync across devices and survive cache clears? If yes, that's a small backend addition — worth it for v2.
3. **"Use a prompt" definition:** any prompt creation counts (via UI "Use" button), or should SDK-served prompts (via API key) also count? SDK usage is tracked only as `api_keys.last_used_at` today, not per-prompt — recommend UI-prompt-creation only for v1.
4. **Welcome modal trigger:** client-side flag after registration vs. a `?welcome=1` query param appended by the register handler (more explicit, survives refresh)? Recommend the query param + flag combination.
5. **Checklist placement:** sidebar column (desktop) vs. top of content area (banner style)? I've designed sidebar-section styling for consistency; confirm.
