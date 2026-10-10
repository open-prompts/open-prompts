# Accessibility (WCAG 2.1 AA) — Requirements Analysis

- **Feature ID:** accessibility
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-10

## 1. Competitor Research Summary

| Platform | Accessibility approach | Key takeaways |
|---|---|---|
| **GitHub** | Full keyboard support; visible focus rings; skip-to-content; ARIA on menus/dialogs; `lang` synced; status via live regions; dedicated a11y audit process | The reference for dense web apps: skip link, focus management on dialogs, and consistent landmarks are table stakes |
| **ChatGPT** | Strong keyboard/VR support; focus management on route change; aria-live announcements for streaming responses | Route-change focus reset + live-region announcements are the two highest-leverage SPA patterns |
| **PromptBase** | Basic semantic HTML; partial keyboard support; no skip link | Prompt platforms generally lag on a11y — an opportunity to differentiate, not a pattern to copy |
| **FlowGPT** | Div-based interactive cards; weak focus visibility | Click-only cards (like our current `PromptCard`) are the most common prompt-platform a11y failure |
| **AIPRM** | Browser-extension UI; partial ARIA | Extension context differs; only takeaway is that icon-only buttons need accessible names |
| **Notion** | Excellent keyboard map; cmdk palette; reduced-motion support | Publishing a keyboard shortcut reference builds trust with power users |
| **Figma](https://www.figma.com) | Aggressive focus management, announcements, forced-colors support | Forced-colors (Windows High Contrast) support is a mark of maturity; cheap to add via media query |

**Synthesis:** Open Prompts has a good foundation (semantic `header`/`nav`/`aside`/`main` landmarks, `Field` label association, `Modal` with Escape + focus-on-open, `prefers-reduced-motion` global guard, `VersionSelect` listbox ARIA) but significant gaps: no skip link, no `lang` sync on language switch, click-only `PromptCard` and sidebar items, icon-only buttons without `aria-label` (like/favorite, copy), no focus trap or focus restore in `Modal`, no route-change focus management, toasts without `aria-live`, `title`-only button naming, and no WCAG target defined. The winning approach is a systematic WCAG 2.1 AA remediation across the existing surfaces, sequenced by user impact, with an acceptance baseline that CI can enforce (axe-core) — not a redesign.

## 2. Problem Statement

1. **Keyboard users cannot use the core browse flow.** `PromptCard` is a clickable `div` — it is not focusable and not activatable with Enter/Space. The same applies to sidebar category/tag items. A keyboard-only user cannot open any template from the home grid.
2. **Screen reader users get incomplete or wrong information.** Icon-only buttons (like, favorite, copy code) rely on `title` tooltips; `document.lang` stays `en` even in the Chinese UI; toasts appear without `aria-live` so action results are never announced; the like/favorite state (pressed or not) is not exposed.
3. **Focus is lost or trapped invisibly.** The `Modal` focuses itself on open but does not trap Tab inside the dialog and does not restore focus to the trigger on close; route changes (e.g., card → details) leave focus on a removed element, resetting to `body`.
4. **No skip navigation.** The sidebar + header precede `main` in DOM order; keyboard users must tab through ~30 links on every page load before reaching content.
5. **Contrast and target-size risks.** `--text-3` on `--surface` is ~4.0:1 in dark theme (below 4.5:1 for normal text); like/favorite hit areas are 17px icons; no `forced-colors` support exists.
6. **No a11y baseline or regression guard.** Nothing prevents new code from reintroducing violations; there is no documented target level, no axe-core check, and no a11y section in the design docs' definition of done (this feature adds one for future features).

## 3. Target Users

- **Keyboard-first users** (power users, developers using the API-key workflow, people who simply never touch the mouse) — currently blocked from the primary browse → open flow.
- **Screen reader users** (NVDA/JAWS/VoiceOver, EN and ZH) — currently receive unlabeled controls, wrong-language announcements, and silent action feedback.
- **Low-vision users** — need ≥4.5:1 text contrast, visible focus indicators, and 200% zoom without loss of function.
- **Users with motor impairments** — need ≥24px effective targets and alternatives to precise hover targeting.
- **All users** — visible focus, reduced-motion respect, and zoom tolerance benefit everyone; a11y fixes routinely improve UX metrics for the general population.
- **Admin console users** (future `/admin` surfaces) — the a11y baseline must apply to admin pages from day one rather than being retrofitted.

## 4. User Stories

- **US-A1:** As a keyboard user, I can Tab to any template card and press Enter to open it, so that I can browse without a mouse.
- **US-A2:** As a keyboard user, I can press Tab once after page load and reach a "Skip to content" link, so that I do not tab through the whole sidebar on every page.
- **US-A3:** As a keyboard user, when a modal opens my focus stays inside it, and when it closes focus returns to the button that opened it, so that I never lose my place.
- **US-A4:** As a screen reader user, when I switch the UI to Chinese, my screen reader pronounces content with Chinese speech, so that labels are intelligible.
- **US-A5:** As a screen reader user, every icon-only button (like, favorite, copy, close, theme, language) announces its name, so that I know what it does before activating it.
- **US-A6:** As a screen reader user, the like/favorite buttons announce their current state (pressed/not pressed), so that I know whether I have already liked a template.
- **US-A7:** As a screen reader user, when an action completes or fails (save, copy, like), the toast is announced, so that I receive the same feedback sighted users get.
- **US-A8:** As a screen reader user, when I navigate to a new page, the page title and heading are announced, so that I know where I am.
- **US-A9:** As a low-vision user, all body text meets 4.5:1 contrast and large text 3:1 in both themes, so that I can read without straining.
- **US-A10:** As a low-vision user, I can zoom to 200% and use the app fully, so that small text is not a barrier.
- **US-A11:** As a user with motor impairments, interactive targets are at least 24×24px (44px preferred where layout allows), so that I can click accurately.
- **US-A12:** As a user sensitive to motion, animations respect my OS reduced-motion setting, so that the app does not cause discomfort (already partially true; must be kept true).
- **US-A13:** As any user, the currently focused element is always clearly visible, so that I never lose track of my position.
- **US-A14:** As a screen reader user, form validation errors are announced when submission fails, so that I can correct them (existing `Field` shows errors; needs focus/announcement wiring).
- **US-A15:** As an admin console user, all of the above hold on `/admin` pages, so that accessibility is not a second-class concern there.

## 5. Functional Requirements

### 5.1 Keyboard operability

- **FR-1:** Every interactive element must be reachable with Tab/Shift+Tab and activatable with Enter (links/buttons) or Space (buttons) — no mouse-only interactions.
- **FR-2:** `PromptCard` must become keyboard-operable: the whole card navigates via a single tab stop (anchor or button wrapping/overlaying the card content), preserving the existing click behavior.
- **FR-3:** Sidebar category, tag, and special-filter items must be operable links or buttons (list semantics kept via `ul`/`li`).
- **FR-4:** A "Skip to main content" link must be the first focusable element on every page; it becomes visible on focus and moves focus to the `main` landmark.
- **FR-5:** Focus order must follow the visual reading order (header → sidebar → content); no `tabIndex` > 0 anywhere.
- **FR-6:** The mobile sidebar drawer must trap focus while open, close on Escape, and restore focus to the menu toggle on close.
- **FR-7:** Dropdown menus (header profile, language, future notification bell) must implement the standard menu/listbox keyboard pattern: Arrow keys move, Enter/Space selects, Escape closes and restores focus.

### 5.2 Screen reader semantics

- **FR-8:** `document.documentElement.lang` must be kept in sync with the active i18n language (`en`/`zh`) on init and on every language switch.
- **FR-9:** Every icon-only button must have an accessible name via `aria-label` (or visually-hidden text): like, favorite, copy code, copy prompt, close, theme toggle, language toggle, menu toggle, avatar menu, edit-metadata, remove/add tag (already present), notification bell (future).
- **FR-10:** Like/favorite toggle buttons must expose state via `aria-pressed`.
- **FR-11:** The toast container must be an `aria-live="polite"` region (errors: `role="alert"` semantics) so action feedback is announced.
- **FR-12:** Route changes must (a) set `document.title` per page (i18n'd) and (b) move focus to the page's `h1`/main heading (with `tabIndex=-1`) so the new context is announced.
- **FR-13:** Page structure must use one `h1` per page and heading levels that do not skip (details page currently starts at `h2` — must be corrected).
- **FR-14:** Form validation failures must move focus to (or announce) the first invalid field; `Field` already renders `aria-invalid` — wire the announcement.
- **FR-15:** Landmarks must be complete and unique: `header`, `nav` (main nav; sidebar gets `aria-label`), `aside`, `main`, and the toasts as a labeled region.
- **FR-16:** Decorative icons and images must be `aria-hidden`; meaningful images keep their existing `alt` (logo, avatar already comply).

### 5.3 Focus visibility & management

- **FR-17:** All interactive elements must show a visible focus indicator (2px `--accent` outline, 2px offset) in both themes; the current `.input:focus { outline: none }` pattern must be replaced by `:focus-visible` styling that never removes the indicator.
- **FR-18:** `Modal` must trap Tab (and Shift+Tab) inside the dialog while open and restore focus to the opening trigger on close.
- **FR-19:** Focus must never be lost on element removal (toasts, list items, cards after delete): move focus intentionally or to the container.

### 5.4 Contrast, targets, motion, zoom

- **FR-20:** Text contrast must meet WCAG 2.1 AA: 4.5:1 normal, 3:1 large (≥18.66px bold / 24px) in both light and dark themes; token values must be adjusted where they fail (notably `--text-3` in dark theme).
- **FR-21:** Interactive targets must be at least 24×24px (WCAG 2.2 target size minimum) with 44×44px preferred where layout allows; like/favorite buttons must grow their hit area (padding, not icon size).
- **FR-22:** All animations must respect `prefers-reduced-motion` (global guard exists; new components must not bypass it); no essential information may be conveyed by animation alone.
- **FR-23:** The app must remain fully usable at 200% browser zoom and at 320px viewport width (reflow) without horizontal scrolling for content (except code blocks, which may scroll in place).
- **FR-24:** `forced-colors` (Windows High Contrast) must be supported: system colors respected, focus indicator visible, no information conveyed by color alone (like/favorite active state needs an icon fill/shape cue in addition to color).

### 5.5 Process & regression prevention

- **FR-25:** An axe-core accessibility check must run in CI (or as an npm script) against key pages (home, login, template details, profile) with zero critical/serious violations as the gate.
- **FR-26:** A keyboard-only smoke test script (Tab through home → open card → interact) must be documented in the repo so QA/test-agent can execute it.
- **FR-27:** The design-doc template for future features must include an Accessibility section (this and prior UI/UX docs already do; make it a stated requirement going forward).
- **FR-28:** Admin console pages (`/admin`) must meet the same baseline from their first release; admin-specific patterns (data tables) must additionally have proper `th` scope, caption, and sortable-header button semantics.

### 5.6 Regular-user vs admin split

- **Regular-user surfaces (all current pages):** FR-1..24 apply in full.
- **Admin console (`/admin`, being built new):** FR-1..24 apply identically; FR-28 adds table-specific requirements; admin pages must be included in the FR-25 CI check once routable.
- **Shared components (`Modal`, `Field`, `VersionSelect`, toasts, future bell dropdown):** fixes land here once and benefit both surfaces; these are the priority order for implementation.

## 6. Non-Functional Requirements

- **NFR-1 (Compatibility):** No visual redesign — fixes must preserve the Aurora look; token contrast adjustments are the only intentional visual changes and must be reviewed in both themes.
- **NFR-2 (Performance):** No runtime a11y library; fixes are native ARIA/HTML + a small focus-management util. Axe-core runs only in CI, never in production bundles.
- **NFR-3 (No backend changes):** Entirely frontend; no API, schema, or proto changes.
- **NFR-4 (Testability):** Each FR maps to an axe rule or a manual keyboard test step; the CI gate must fail loudly on new violations.
- **NFR-5 (Incremental delivery):** Fixes are independent and can land in any order; shared components first (Modal, toasts, PromptCard), then pages, then admin.
- **NFR-6 (Both themes):** Every contrast/focus fix must be verified in light AND dark themes.
- **NFR-7 (Both languages):** Every accessible name must be i18n'd (EN/ZH); no hardcoded English labels in `aria-label`s.

## 7. Acceptance Criteria

- **AC-1:** Keyboard-only user can complete the core flow: load home → skip link → tab to a card → Enter → details page → like → copy prompt → back — with visible focus throughout and no mouse.
- **AC-2:** axe-core (CI) reports 0 critical and 0 serious violations on home, login, register, template details, profile, and API keys pages, both themes.
- **AC-3:** With UI language set to Chinese, `document.documentElement.lang === 'zh'`; switching languages updates it immediately.
- **AC-4:** Opening any modal (create template, delete confirmations) traps focus inside; closing returns focus to the trigger; Escape closes.
- **AC-5:** Every icon-only button exposes an accessible name (verified via axe "button-name" rule) in both languages.
- **AC-6:** Like/favorite buttons expose `aria-pressed`; toggling updates the state announcement.
- **AC-7:** Toasts are announced by screen readers (aria-live region present; error toasts use assertive semantics).
- **AC-8:** Text contrast passes 4.5:1 (normal) / 3:1 (large) across both themes, verified with a contrast audit of all token pairs used for text.
- **AC-9:** At 200% zoom and 320px width, all pages reflow without horizontal scrolling (code blocks excepted).
- **AC-10:** `prefers-reduced-motion: reduce` disables all non-essential animation in both themes (existing global guard plus no new bypasses).
- **AC-11:** Route change announces the new page (title set + focus to heading); back navigation behaves identically.
- **AC-12:** The keyboard smoke test script exists in the repo and passes manually on the six core pages.

## 8. Out of Scope (v1)

- WCAG 2.1 AAA conformance (v1 targets AA).
- Full screen-reader manual audit with NVDA/JAWS/VoiceOver certification (axe + keyboard testing only in v1; manual SR passes are a follow-up).
- Subtitle/caption features (no audio/video content exists).
- Accessibility of third-party embeds (none present).
- Redesigning any visual language beyond the minimal token contrast adjustments.
- Speech input / voice control optimization.

## 9. Dependencies & Assumptions

- **D-1:** Assumes the existing `Modal`, `Field`, `VersionSelect`, and toast (`NotificationContext`) components remain the shared primitives; fixes concentrate there.
- **D-2:** Assumes react-router (or equivalent client routing) is in use so a route-change effect can set titles/focus; page components render inside the existing `Layout`.
- **D-3:** Assumes axe-core can be added as a devDependency and run via the existing test runner (Jest/RTL) or Playwright; CI already gates pushes.
- **D-4:** Assumes the Aurora token set (`--text-1/2/3`, `--surface*`) is the single source for text colors; adjusting tokens fixes all usages at once.
- **D-5:** Assumes i18n keys for all new accessible names are added to both `en` and `zh` translation files in the same change (no partial-language releases).
- **D-6:** Assumes future features (notifications bell, versioning panel, admin console) will consume the fixed shared components — sequencing this feature early prevents multiplying violations.

## 10. Open Questions for Architect

1. **CI integration:** Should axe-core run inside the existing Jest/RTL suite (jsdom, fast but limited) or via a Playwright/nightwatch pass against the running stack (slower, more accurate)? The repo has both test setups.
2. **Contrast token adjustment:** Dark-theme `--text-3` (#6e768c on #171b26 ≈ 4.0:1) fails AA for normal text. Proposed fix: lighten to ~#8b92a8 (the light-theme value, ≈ 5.4:1 on dark surfaces). Confirm acceptable as a deliberate token change, and whether `--text-3` should be reserved for large/disabled text only.
3. **PromptCard pattern:** Wrap the card in an `<a>` (best semantics, but nested interactive elements inside need care) vs. overlay-button pattern (absolute-positioned button covering the card, existing inner buttons above it)? Recommend overlay-button for minimal DOM change.
4. **Route-change focus:** Move focus to `h1` with `tabIndex={-1}` on every navigation, or only on "main" navigations (card → details) to avoid disturbing same-page filter changes?
5. **Scope of v1 pages:** Are the six core pages (home, login, register, details, profile, API keys) the right CI gate set, or should the gate start smaller (home + details)?
