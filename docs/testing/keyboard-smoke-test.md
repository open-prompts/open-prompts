# Keyboard Smoke Test (WCAG 2.1 AA — FR-26 / AC-1 / AC-12)

Manual walkthrough executed per release to verify the core keyboard flow.
Pass criteria: every step below works with **keyboard only** (no mouse), and
focus is always visible.

## Setup

- Open the app in a desktop browser (Chrome/Firefox).
- Ensure you are logged out (or log out first) so the flow starts at Home.

## Steps

| # | Action | Expected focus / behavior |
|---|---|---|
| 1 | Load `/` | Page title is set; focus is on the page `h1` (visually hidden on Home). |
| 2 | Press `Tab` | First stop is the **Skip link** ("Skip to content"), visible on focus. |
| 3 | Press `Enter` on Skip link | Focus moves into `main#main-content` (not just scroll). |
| 4 | Press `Tab` repeatedly | Focus moves through header controls (theme, language, login/register) then sidebar items in order. |
| 5 | In the sidebar, `Tab` to a category and press `Enter` | The category filter is applied; `aria-current` moves to the active item. |
| 6 | `Tab` to a prompt card | The card is a single tab stop (overlay button). Press `Enter` → navigates to Template Details. |
| 7 | On Template Details, `Tab` to the Like button | `aria-pressed` reflects state; press `Enter` toggles it. |
| 8 | `Tab` to the Copy button | Press `Enter` copies the generated content; a toast announces success. |
| 9 | Press `Tab` to the Back / browser back | Returns to Home. |
| 10 | Open a modal (e.g. Create Template) | Focus moves into the dialog; `Tab`/`Shift+Tab` wrap inside it; `Escape` closes and restores focus to the trigger. |
| 11 | Log in, open the user menu | `Tab` to the avatar button, press `Enter`; `ArrowDown`/`ArrowUp` move between items; `Escape` closes and restores focus. |

## Pass criteria

- Focus is always visible (2px accent outline).
- No keyboard trap: `Tab` never gets stuck.
- All interactive elements are reachable and operable by keyboard.
- Screen-reader announcements fire for toasts (live region) and page changes.