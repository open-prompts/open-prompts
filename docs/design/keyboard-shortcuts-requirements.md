# Keyboard Shortcuts & Command Palette — Requirements Analysis

- **Feature ID:** keyboard-shortcuts
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. Competitor Research Summary

| Platform | Shortcut / command approach | Key takeaways |
|---|---|---|
| **GitHub** | `s` focus search, `/` search, `g`+`d` go-to-dashboard sequences, `?` shortcut cheat sheet | Single-letter keys for the most frequent action; a discoverable `?` overlay is the reference pattern |
| **Notion** | cmdk command palette (`Cmd/Ctrl+K`): search pages, run actions, insert blocks; full shortcut map | The palette is the universal escape hatch — every action reachable from one input; scales better than memorizing keys |
| **Slack** | `Cmd/Ctrl+K` jump-to-conversation; `?` overlay; sequences for navigation | Palette for *navigation*, single keys for *actions* — clean split |
| **Gmail** | `c` compose, `j/k` list navigation, `x` select, `Enter` open | `j/k` list navigation is the power-user expectation for any list UI |
| **Linear** | cmdk palette as the primary interface; every route and action command-addressable | A palette-first product proves commands can carry a whole app; we adopt it for navigation + top actions only |
| **PromptBase / FlowGPT / Snack Prompt** | No keyboard shortcuts or palettes | Prompt platforms lag here — differentiation opportunity for power users |

**Synthesis:** Two complementary layers, both proven: (1) a **command palette** (`Cmd/Ctrl+K`) for navigation and top actions — search templates, jump to pages, create template, toggle theme/language; (2) a small set of **single-key shortcuts** on specific surfaces — `s`/`/` focus search, `j/k` move through template grids, `?` open the shortcut cheat sheet. The accessibility design (already committed) makes every control *keyboard operable* (Tab/Enter/Arrow); this feature adds *speed* on top of operability. Everything is opt-in and non-blocking: no shortcut fires while typing in an input, and all shortcuts have visible UI equivalents.

## 2. Problem Statement

1. **Every action is a mouse round-trip.** Reaching search, creating a template, or switching pages requires pointer navigation; power users feel the friction on every visit.
2. **No discoverability of speed features.** Even where keyboard operability exists (per the accessibility design), there is no cheat sheet, no hint that speed paths exist.
3. **Growing surface area needs a universal jump box.** With 17+ designed features (collections, chains, teams, playground, usage, dashboard, notifications), page-hopping via the sidebar does not scale; a palette is the standard answer.
4. **Competitors ignore keyboard users.** Prompt platforms are mouse-only today; serving keyboard-first users (developers, heavy prompt users) is a differentiator.

## 3. Target Users

- **Power users / heavy prompt users:** browse and open dozens of templates daily; benefit most from `j/k` + palette.
- **Developers using the API workflow:** jump between playground, usage, API keys, template details.
- **Keyboard-first and assistive-tech users:** already served for *operability* by the accessibility design; this feature adds efficiency.
- **Admins:** quick jumps within the admin console (palette scoped to admin routes).

## 4. User Stories

- **US-K1:** As a power user, I can press `Cmd/Ctrl+K` anywhere to open a command palette, so that I can search templates and jump to any page without the mouse.
- **US-K2:** As a user, I can press `?` to see a cheat sheet of all shortcuts, so that I can learn them without leaving the page.
- **US-K3:** As a user, I can press `s` or `/` to focus the search box, so that searching is one keystroke away.
- **US-K4:** As a browsing user, I can press `j`/`k` to move the selection through the template grid and `Enter` to open the selected card, so that I can browse without a mouse.
- **US-K5:** As a user, I can press `c` on the home page to open the create-template modal, so that creating is instant.
- **US-K6:** As a user, I can toggle the theme with `t` (when not typing), so that switching light/dark is immediate.
- **US-K7:** As a user, I can dismiss any palette/overlay with `Esc`, so that I can always back out.
- **US-K8:** As a user, I can see shortcut hints on the UI elements they trigger (tooltips), so that shortcuts are discoverable in context.
- **US-K9:** As an admin, I can open the palette inside `/admin` and get admin-scoped commands (jump to users/templates/audit log), so that the console is fast too.
- **US-K10:** As a user who dislikes shortcuts, I can ignore all of this and use the product exactly as before, so that nothing is forced on me.

## 5. Functional Requirements

### 5.1 Command palette

- **FR-1:** A global palette opens with `Cmd/Ctrl+K` (and a header search-box affordance: clicking the search box's `⌘K` chip opens the palette). It renders as a centered modal with a single text input.
- **FR-2:** Palette command sources (merged, ranked):
  - **Navigation:** all top-level routes the current user can access (Home, Profile, API Keys, Notifications, My Teams, Collections, Chains, Playground, Usage, Dashboard, Admin console for admins; Login/Register when logged out).
  - **Template search:** debounced (250 ms) query against the existing template search API; results show name + category, Enter opens the template.
  - **Actions:** Create template (`c`), Toggle theme, Switch language (EN/中文), Create team, Create collection (contextual).
- **FR-3:** Palette interaction: Arrow keys move the active row, `Enter` executes, `Esc` closes, `Tab` is trapped inside, focus returns to the opener on close. Mouse click also executes. Empty query shows "quick links" (top 5 navigation items + recent templates).
- **FR-4:** Palette results are grouped (Navigation / Templates / Actions) with group headers; the active row is scrolled into view; max 12 visible rows with internal scroll.
- **FR-5:** Inside `/admin/*`, the palette is scoped: navigation commands list admin pages (Dashboard, Users, Templates, Teams, Audit Log) plus "Exit to app"; template search is replaced by admin template search; user search (by id/email) is included for admins.

### 5.2 Single-key shortcuts

- **FR-6:** `s` and `/` focus the page's primary search input (home search, admin list search). If no search input exists on the page, the key is inert.
- **FR-7:** `j`/`k` move a visual selection cursor through the current template grid (home, collection, team, chain templates, search results): `j` next, `k` previous, `Enter` opens, `o` also opens (Gmail convention). The selection is indicated with the existing focus ring style; the selected card scrolls into view. The cursor starts hidden until the first `j`/`k`.
- **FR-8:** `c` opens the create-template modal on surfaces that have one (home, team home); inert elsewhere.
- **FR-9:** `t` toggles light/dark theme; `?` (Shift+/) opens the shortcut cheat sheet modal. Both are global.
- **FR-10:** **Guard rule:** no single-key shortcut fires when focus is in a text input, textarea, or contenteditable element, or when a modal is open (except `Esc` and palette-internal keys). `Esc` closes the topmost overlay (palette, cheat sheet, create modal, any existing modal) — one consistent behavior.

### 5.3 Discoverability

- **FR-11:** The `?` cheat sheet modal lists every shortcut in a two-column table (keys — action), grouped (Global / Browsing / Palette), with a footer link to the docs page. It is reachable from the header user dropdown ("Keyboard shortcuts") and the palette ("Help: keyboard shortcuts").
- **FR-12:** Tooltips on the header search box (`s`), theme toggle (`t`), and create button (`c`) show their shortcut in the standard `key` chip style.
- **FR-13:** First-run hint: after a user's 3rd session, a one-time dismissible toast on the home page says "Press `?` for keyboard shortcuts" (stored per-user preference, `localStorage`).

### 5.4 API

- **FR-14:** No new backend endpoints. The palette reuses the existing template search API and the current user/session state. (Admin user search reuses the existing admin users API.)

### 5.5 Admin surface

- **FR-15:** All shortcuts work in the admin console with scoped behavior (FR-5, FR-6); `j`/`k` apply to admin list tables (users, templates, teams) with `Enter` opening the row's detail view. The cheat sheet reflects the current scope.

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Palette opens < 100 ms (no data fetch on open; quick links are static); template search results render < 300 ms after the debounce.
- **NFR-2 (Non-interference):** Zero behavior change for users who never press the keys; all shortcuts are additive and every shortcut has a visible UI equivalent.
- **NFR-3 (Accessibility):** Palette follows the ARIA 1.2 combobox/listbox pattern (role=dialog, aria-activedescendant); cheat sheet is a standard modal; shortcuts never override browser or screen-reader reserved keys; all new UI is fully keyboard operable per the accessibility design.
- **NFR-4 (i18n):** All palette labels, group headers, cheat-sheet copy, and tooltips via i18next (`shortcuts.*`, `palette.*` namespaces) in EN/ZH; shortcut *keys* are locale-independent.
- **NFR-5 (Testability):** Shortcut map is a single declarative config (key → handler, scoped by route) so unit tests cover every binding; a manual smoke script extends the accessibility keyboard test.
- **NFR-6 (Consistency):** Visual language (modal, focus ring, key chips) reuses the Aurora design system; no new visual idioms.

## 7. Acceptance Criteria

- **AC-1:** `Cmd/Ctrl+K` opens the palette on every page (logged in and logged out); typing "prof" shows the Profile navigation item; Enter navigates.
- **AC-2:** Typing a template name in the palette returns matching templates within 300 ms (debounced); Enter opens the template detail page.
- **AC-3:** Arrow keys move the active palette row; `Esc` closes and returns focus to the opener; Tab stays trapped inside.
- **AC-4:** In `/admin`, the palette lists admin pages and admin user search; it does not list user-facing pages except "Exit to app".
- **AC-5:** `s` focuses the home search box; typing then works normally (no shortcut fires while typing).
- **AC-6:** `j`/`k` move a visible selection through the home grid; `Enter` opens the selected template; the selection scrolls into view.
- **AC-7:** `?` opens the cheat sheet listing all shortcuts; `Esc` closes it; it is also reachable from the user dropdown and palette.
- **AC-8:** `t` toggles the theme; `c` opens the create modal on home; both are inert while an input has focus or a modal is open.
- **AC-9:** `Esc` closes the topmost overlay regardless of type (palette, cheat sheet, create modal).
- **AC-10:** The first-run toast appears once (3rd session), is dismissible, and never appears again after dismissal.
- **AC-11:** All copy is EN/ZH translated; shortcut keys are identical in both locales.
- **AC-12:** A user who never uses the keyboard experiences zero difference (no layout shift, no changed clicks).

## 8. Out of Scope

- User-customizable or remappable shortcuts.
- Shortcut sequences/chords beyond `g`-style go-to prefixes (v1 keeps single keys + palette).
- Palette command extensibility (plugins, custom commands).
- Recording/macro features.
- Mobile-specific gestures (mobile uses existing touch UI; the palette is desktop-focused but responsive).
- Monetization of any kind — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1:** The accessibility design's keyboard operability (focusable cards, focus-trapped modals, menu arrow-key patterns) is the foundation; this feature builds on it and must not duplicate or conflict with it.
- **D-2:** The existing template search API (from template-search-filtering) powers palette template results; no new search endpoint.
- **D-3:** The header search box, theme toggle, and create button exist as the visible equivalents of `s`, `t`, `c`.
- **D-4:** Route list is derived from the app's router config plus the current user's permissions (admin sees admin routes).
- **D-5:** `localStorage` is available for the first-run hint preference (same mechanism as the advanced-editor drafts).
- **D-6:** The notifications, teams, collections, chains, playground, usage, dashboard features add their routes to the palette's navigation source automatically (single registry).

## 10. Open Questions for Architect

1. **Shortcut registry shape:** One global declarative config (key, scope, handler, description-i18n-key) consumed by both the key listener and the cheat sheet — confirm this as a frontend-only module with no backend involvement.
2. **Palette search API:** Reuse the public template search endpoint as-is (with its pagination) or add a lightweight `?limit=5` variant optimized for palette queries? Recommendation: reuse with limit param if supported.
3. **Admin user search in palette:** The admin users API requires admin auth — confirm the palette simply calls it when the session is admin (no new permission surface).
4. **`j`/`k` on virtualized grids:** If any grid virtualizes rows (large search results), does the selection cursor need special handling, or is v1 limited to the first page of results?
5. **First-run toast trigger:** "3rd session" requires a session counter in `localStorage` — acceptable, or prefer showing the toast on first home-page visit after the feature ships?
