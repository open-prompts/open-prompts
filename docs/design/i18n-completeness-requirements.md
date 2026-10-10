# i18n Completeness & Quality — Requirements Analysis

| | |
|---|---|
| **Feature ID** | FE-011 |
| **Version** | 1.0 |
| **Status** | Proposed (for architecture review) |
| **Author** | ui-ux-agent |
| **Date** | 2026-10-10 |

## 1. Competitor Research

| Product | i18n Approach | Strengths | Weaknesses |
|---|---|---|---|
| PromptBase | EN only | — | No international users served |
| FlowGPT | EN + machine-translated subsets | Broad coverage | Inconsistent quality, mixed-language UI |
| AIPRM | EN, DE, FR, ES, PT + community translations; locale-aware dates | Community scales translation | No visible parity CI; missing keys leak English |
| ChatGPT | Dozens of languages; per-page localized titles; plural-aware strings | Gold standard polish | — |
| Vercel/Linear (SaaS benchmark) | Locale-aware everything: titles, dates, numbers, plurals; CI checks key parity | Professional feel; no mixed-language leakage | — |

**Takeaway:** The product already has a solid EN/ZH foundation (378 keys each, perfect parity today, locale-aware dates). What it lacks is *guarantees*: nothing prevents drift when new keys are added, plurals are unhandled, page titles are static English, and 54 dead keys accumulate. Competitors that feel "native" win on the guarantees, not the raw coverage.

## 2. Problem Statement

An audit of `frontend/src/locales/{en,zh}/translation.json` and all `t()` call sites found:

1. **No drift protection.** EN and ZH are in parity *today* (378 keys each), but nothing enforces it. A developer adding a key to EN only ships a ZH UI that silently falls back to English — no error, no warning, discovered only by users.
2. **Dead keys accumulate.** 54 defined-but-unused keys exist (e.g. `admin.reasonModal.consequence_*`, `admin.common.per_page`), increasing translation cost and drift surface with every feature.
3. **Plurals are broken in English.** Three keys interpolate `{{count}}` without i18next plural forms: `"{{count}} items"` renders as "1 items". Chinese has no plural inflection, so the bug hides in the EN locale only.
4. **Static page titles.** `<title>Open Prompts</title>` never changes — no per-route titles, no localization. Browser tabs, history, and bookmarks show English regardless of language or page.
5. **Static `<html lang="en">`.** Screen readers announce the wrong language for ZH users (also flagged in the accessibility design; owned there, referenced here).
6. **Backend error strings surface raw.** API error messages (English from the gateway) are shown verbatim in toasts for ZH users, breaking the illusion of a native UI.
7. **Numbers are unformatted on user surfaces.** Like/favorite counts render as raw integers; admin has `formatNumber` but user-facing pages do not use locale grouping.

## 3. Target Users

- **Chinese-language users** who currently see English leakage in edge cases (plurals, titles, backend errors).
- **All users** who benefit from professional per-page titles and formatted numbers.
- **Developers** who need CI to catch i18n regressions before they ship.
- **Translators/future contributors** who need a clean, dead-key-free catalog.

## 4. User Stories

- **US-1**: As a ZH user, I want every UI string in Chinese so that no English leaks into my experience.
- **US-2**: As an EN user, I want correct plurals ("1 item", "2 items") so that the UI reads naturally.
- **US-3**: As a user, I want the browser tab title to reflect the current page and language so that I can find and return to the right tab.
- **US-4**: As a ZH user, I want backend error messages presented in Chinese so that failures are understandable.
- **US-5**: As a user, I want counts (likes, favorites, results) formatted per my locale so that numbers read naturally (1,234 vs 1234).
- **US-6**: As a developer, I want CI to fail when EN/ZH keys diverge or when code references a missing key so that regressions never ship.
- **US-7**: As a developer, I want unused keys reported so that the catalog stays clean and translation effort is not wasted.
- **US-8**: As a screen reader user, I want `<html lang>` to match the UI language so that pronunciation is correct (delivered by the accessibility feature; tracked here for coverage).

## 5. Functional Requirements

### 5.1 Parity & drift protection (CI)

- **FR-1**: A CI check MUST compare the flattened key sets of `en/translation.json` and `zh/translation.json` and fail if they differ in either direction, listing the offending keys.
- **FR-2**: A CI check MUST extract every `t('...')` literal from `frontend/src/**/*.{js,jsx}` and fail if a referenced key is absent from the EN catalog (missing-key check).
- **FR-3**: A CI check MUST report keys defined in the catalogs but referenced nowhere in code (unused-key check) as a **warning**, not a failure, listing them for cleanup.
- **FR-4**: A CI check MUST compare interpolation placeholders (`{{var}}`) between EN and ZH for every key and fail on mismatch (e.g. EN uses `{{name}}`, ZH omits it).
- **FR-5**: All checks run as a single script (`npm run i18n:check`) usable locally and in CI; exit code non-zero on any failure.

### 5.2 Plural correctness

- **FR-6**: All `{{count}}` keys MUST be migrated to i18next plural form keys (`key_one` / `key_other` for EN; `key_other` for ZH) so English renders "1 item" / "2 items" correctly.
- **FR-7**: The three existing count keys (`admin.common.total_count`, `admin.common.per_page`, `admin.reasonModal.char_count`) are migrated; `char_count` (a "12 / 500" counter) is exempt from pluralization and documented as such.

### 5.3 Localized page titles

- **FR-8**: Each route MUST set `document.title` to a localized, page-specific string with the pattern `"{Page} · Open Prompts"` (EN) / “{页面} · Open Prompts”（ZH）.
- **FR-9**: Routes covered: home, login, register, profile, API keys, template details (`{title} · Open Prompts`), and all `/admin/*` pages.
- **FR-10**: Title updates MUST be implemented in a single reusable hook (`usePageTitle`) that reads the active language and re-renders the title on language switch without reload.

### 5.4 Backend error localization

- **FR-11**: The API error interceptor MUST map known backend error signatures (HTTP status + error code/message pattern) to localized user-facing messages before display.
- **FR-12**: Unmappable errors fall back to a generic localized message ("Something went wrong") plus the raw backend message in a `title` tooltip / expandable detail, never replacing the localized headline.
- **FR-13**: The mapping table MUST live in the locale catalogs (`errors.*` namespace) so adding translations requires no code change.

### 5.5 Number formatting

- **FR-14**: User-facing counts (like count, favorite count, template list totals) MUST be formatted with `Intl.NumberFormat` using the active language locale.
- **FR-15**: The existing `formatNumber` helper in `utils/adminFormat.js` MUST be promoted to a shared `utils/format.js` used by both admin and user surfaces (admin behavior unchanged).

### 5.6 Catalog hygiene

- **FR-16**: The 54 currently unused keys MUST be audited: keys belonging to shipped features that are imminent (per the design pipeline) are kept and whitelisted with a comment; the rest are removed in the same change that lands this feature.
- **FR-17**: The bare `welcome` top-level string MUST be normalized into a namespace (`welcome.title`) for structural consistency, with its single usage updated.

### 5.7 Regular-user vs admin split

- **Regular users**: benefit from FR-6..15 (plurals, titles, errors, numbers) on all user surfaces.
- **Admin console**: same benefits on `/admin/*` routes (titles via FR-9, errors via FR-11); admin-specific count keys migrate to plurals in FR-7. No admin-only settings are introduced — language remains the existing header toggle.

## 6. Non-Functional Requirements

- **NFR-1 (Performance)**: `i18n:check` completes in < 5 s; title updates are synchronous and cheap (no data fetching).
- **NFR-2 (Robustness)**: The error-mapping interceptor must never mask network-level failures (timeouts, offline) — those keep their existing handling.
- **NFR-3 (Compatibility)**: No user-visible behavior change other than the improvements listed; existing key paths used by shipped code are untouched except the plural migration and `welcome` normalization (both with code updates in the same change).
- **NFR-4 (Maintainability)**: The parity script is dependency-free (Node built-ins) to avoid supply-chain weight.
- **NFR-5 (Accessibility)**: `document.title` changes are announced by screen readers as a page change; `<html lang>` sync is delivered by the accessibility feature and is a prerequisite for full effect.
- **NFR-6 (i18n)**: All new strings (titles, error mappings) ship in both EN and ZH simultaneously, enforced by FR-1.

## 7. Acceptance Criteria

- **AC-1**: Deleting a key from ZH only, then running `npm run i18n:check`, fails CI with the key name listed.
- **AC-2**: Adding `t('typo.key')` to any component, then running the check, fails with the missing key listed.
- **AC-3**: A key with `{{name}}` in EN but not ZH fails the placeholder check.
- **AC-4**: EN UI renders "1 item" and "2 items"; ZH renders “1 项” and “2 项” (or the natural ZH phrasing) from the same call site.
- **AC-5**: Navigating to template details sets the tab title to the template name; switching language updates it without reload.
- **AC-6**: A backend 409 conflict surfaces a localized message in ZH; an unknown 500 shows the generic localized headline with raw detail available.
- **AC-7**: A template with 1,234 likes shows "1,234" in EN and "1,234" in ZH (both group with commas; ZH grouping matches `zh-CN` convention).
- **AC-8**: `npm run i18n:check` passes on main after cleanup; the unused-key report is empty or explicitly whitelisted.
- **AC-9**: The `welcome` string renders identically before and after normalization.
- **AC-10**: All checks run in CI on every pull request alongside existing gates.

## 8. Out of Scope

- Adding languages beyond EN/ZH (structure is ready; new locales are a future decision).
- Server-side content localization (template content, user-generated text stays as authored).
- Localized email/verification-code messages (backend domain).
- Right-to-left (RTL) language support.
- In-app language preference page (the existing header toggle remains the only switcher).
- Translation memory / external translation tooling integration.

## 9. Dependencies & Assumptions

- **D-1**: i18next is configured with `fallbackLng: 'en'` and LanguageDetector; plural suffixes (`_one`/`_other`) work with the bundled i18next version without extra configuration.
- **D-2**: All `t()` call sites use single-quoted literal keys (audit confirmed; dynamic key construction is absent), making static extraction reliable.
- **D-3**: Backend errors arrive with a stable shape (message string and/or code) that the interceptor can pattern-match; the mapping table degrades gracefully to the generic message.
- **D-4**: The accessibility feature delivers `<html lang>` sync; this feature depends on it only for screen-reader pronunciation of titles, not for any functional behavior.
- **D-5**: CI can run Node scripts (existing gates already do).

## 10. Open Questions for Architect

1. **Error mapping granularity**: should the interceptor map on HTTP status alone (coarse, stable) or on backend error strings/codes (precise, brittle)? Design assumes status + a small set of known message patterns, with the catalog as the source of truth.
2. **Unused-key policy**: warn-only (FR-3) vs fail after a grace period? Design assumes warn-only to avoid blocking feature branches that add keys before code.
3. **Plural migration timing**: the three count keys are admin-surface; confirm no in-flight admin changes conflict with renaming them to `_one`/`_other` variants.
4. **`usePageTitle` placement**: hook in each page component vs a central route-change effect in `App.js`? Design assumes per-page hook (explicit, testable) — confirm preference.
