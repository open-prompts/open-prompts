# Accessibility (WCAG 2.1 AA) — Detailed Design

| | |
|---|---|
| **Feature ID** | accessibility |
| **Version** | 1.0 |
| **Status** | Approved |
| **Author** | Architect Agent |
| **Upstream** | [Architecture](accessibility-architecture.md) |
| **Downstream** | Developer Agent |

## 1. Scope & Files

Frontend-only. Files touched:

| File | Change |
|---|---|
| `frontend/src/index.scss` | focus-visible rules, `--text-3` tokens, forced-colors, skip-link, hit areas |
| `frontend/src/components/ui/Modal.jsx` | focus trap, focus restore, `aria-labelledby` |
| `frontend/src/context/NotificationContext.js` | live region, `role="alert"` for errors |
| `frontend/src/components/PromptCard.js` / `.scss` | overlay button, `aria-pressed`, hit areas |
| `frontend/src/components/Sidebar.js` / `.scss` | button semantics, `aria-current` |
| `frontend/src/components/Header.js` | menu pattern, labels |
| `frontend/src/components/Layout.js` | SkipLink, `main#main-content` |
| `frontend/src/components/SkipLink.jsx` | **new** |
| `frontend/src/utils/a11y.js` | **new** — `usePageAnnouncement` |
| `frontend/src/i18n.js` | `documentElement.lang` sync |
| `frontend/src/pages/TemplateDetails.js` | h1, `aria-pressed`, labels |
| `frontend/src/pages/{Home,Login,Register,Profile}.js` | h1, autocomplete, error focus |
| `frontend/src/locales/{en,zh}/translation.json` | new keys (§5) |
| `frontend/src/a11y/*.test.js` | **new** — axe gate |
| `docs/testing/keyboard-smoke-test.md` | **new** |
| `.github/workflows/ci.yml` | `frontend-a11y` job |
| `frontend/package.json` | `axe-core` devDependency, `test:a11y` script |

## 2. Global Styles (SCSS)

### 2.1 Focus visibility (replaces `outline: none` on inputs)

```scss
// Universal focus indicator — 2px accent outline, 2px offset (FR-17/18)
:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
}

// Inputs previously removed the outline entirely (.input:focus { outline: none });
// they now keep the soft ring AND the visible outline.
.input:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
}

// Never remove the indicator without replacing it
.input:focus { /* keep existing box-shadow ring; outline no longer suppressed */ }
```

### 2.2 Contrast tokens (normative values — see architecture §5)

```scss
:root {                      // dark theme (default)
  --text-3: #8b92a8;         // was #6e768c (3.79:1) → 5.55:1 on --surface, ≥4.55:1 all surfaces
}
html.light-theme {
  --text-3: #626b7a;         // was #8b92a8 (3.10:1) → 5.38:1 on --surface, ≥4.64:1 all surfaces
}
```

> Note: the UI/UX design §5.8 proposed `#767e93` for light theme; measured contrast is 4.06:1 on white — **fails AA**. `#626b7a` is the adopted value (recorded in architecture §1 Resolved Open Questions #3).

### 2.3 Skip link

```scss
.skip-link {
  position: absolute;
  left: -9999px;
  top: 0;
  z-index: 1000;
  background: var(--accent);
  color: #fff;
  padding: 8px 16px;
  border-radius: 0 0 8px 0;
  font-weight: 600;
}
.skip-link:focus {
  left: 0;                    // becomes visible only when focused
}
```

### 2.4 Forced colors (Windows High Contrast)

```scss
@media (forced-colors: active) {
  .prompt-card { border: 1px solid CanvasText; }
  .btn { border: 1px solid ButtonText; }
  .stat-item { forced-color-adjust: none; }
  .skip-link { border: 1px solid ButtonText; }
}
```

### 2.5 Hit areas

```scss
// Minimum 24×24 CSS px (44 preferred on touch) for like/favorite/close/menu icons
.stat-item {
  min-width: 24px;
  min-height: 24px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
```

## 3. Component Changes

### 3.1 `Modal.jsx` — focus trap + restore + labelling

```jsx
// On open: save trigger, focus dialog, set aria-labelledby
useEffect(() => {
  if (!open) return;
  restoreRef.current = document.activeElement;          // (a) save trigger
  const dlg = dialogRef.current;
  dlg.focus();                                            // (b) focus dialog
  return () => {                                          // (c) restore on close/unmount
    const el = restoreRef.current;
    if (el && document.contains(el)) el.focus();
  };
}, [open]);

// Keydown: Escape (existing) + Tab trap
const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

function handleKeyDown(e) {
  if (e.key === 'Escape') { onClose(); return; }
  if (e.key !== 'Tab') return;
  const items = dialogRef.current.querySelectorAll(FOCUSABLE);
  if (items.length === 0) { e.preventDefault(); return; }
  const first = items[0], last = items[items.length - 1];
  if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
  else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
  // else: default Tab behavior stays inside the dialog
}
```

Markup: `<div role="dialog" aria-modal="true" aria-labelledby={titleId} tabIndex={-1}>` with `<h3 id={titleId}>` (or `aria-label` fallback when no title). The existing `handleKeyDownRef` pattern (avoids re-registering the listener on every render) is kept.

### 3.2 `NotificationContext.js` — live region

```jsx
<div
  role="region"
  aria-label={t('a11y.notifications_region')}
  aria-live="polite"
  className="toast-container"
>
  {toasts.map(t => (
    <div key={t.id} role={t.type === 'error' ? 'alert' : 'status'} className={`toast toast-${t.type}`}>
      ...
      <button aria-label={t('common.close_notification')} ... />
    </div>
  ))}
</div>
```

Toasts never take focus (existing behavior, kept).

### 3.3 `PromptCard.js` — overlay button + `aria-pressed`

```jsx
<div className="prompt-card">
  <button
    type="button"
    className="prompt-card-overlay"
    aria-label={template.title}
    onClick={() => navigate(`/templates/${template.id}`)}
  />
  <div className="prompt-card-body"> ... title, description, meta ... </div>
  <div className="prompt-card-stats">
    <button
      type="button"
      className="stat-item stat-like"
      aria-pressed={!!template.is_liked}
      aria-label={t('template_details.like')}
      onClick={(e) => { e.stopPropagation(); handleLike(); }}
    />
    <button
      type="button"
      className="stat-item stat-favorite"
      aria-pressed={!!template.is_favorited}
      aria-label={t('template_details.favorite')}
      onClick={(e) => { e.stopPropagation(); handleFavorite(); }}
    />
  </div>
</div>
```

```scss
.prompt-card { position: relative; }
.prompt-card-overlay {
  position: absolute; inset: 0; z-index: 1;
  background: transparent; border: none; cursor: pointer; padding: 0;
}
.prompt-card-stats .stat-item {
  position: relative; z-index: 2;   // stay above the overlay → directly clickable
  min-width: 24px; min-height: 24px;
}
```

The card root drops its `onClick`/`navigate`/`cursor: pointer` (the overlay button owns navigation). `aria-pressed` must reflect **server state after refetch**, not just optimistic local state (watch-out from UI/UX §11).

### 3.4 `Sidebar.js` — button semantics

```jsx
<li className={active ? 'active' : ''}>
  <button
    type="button"
    className="sidebar-item"
    aria-current={active ? 'true' : undefined}
    onClick={...}
  >
    <Icon aria-hidden="true" /> {label}
    {count != null && <span className="count">{count}</span>}
  </button>
</li>
```

Tag chips: `<span className="tag" onClick>` → `<button type="button" className="tag" ...>`. Counts stay inside the button label so they're announced with it.

### 3.5 `Header.js` — menu pattern

```jsx
<button
  type="button"
  aria-label={t('header.open_user_menu')}
  aria-haspopup="menu"
  aria-expanded={menuOpen}
  onClick={toggleMenu}
>...</button>
{menuOpen && (
  <div role="menu" aria-label={t('header.open_user_menu')}>
    {items.map(item => (
      <button type="button" role="menuitem" key={item.key} onClick={item.onClick}>...</button>
    ))}
  </div>
)}
```

Escape closes the menu and restores focus to the trigger; ArrowDown/ArrowUp move between items (wrap). Click-outside (existing) stays.

### 3.6 `Layout.js` + `SkipLink.jsx`

```jsx
// Layout.js — SkipLink is the first focusable element; main is the skip target
<>
  <SkipLink />
  <Header ... />
  <div className="app-body">
    <Sidebar ... />
    <main className="app-content" id="main-content" tabIndex={-1}>   {/* was plain <main> */}
      {children}
    </main>
  </div>
</>
```

```jsx
// SkipLink.jsx
import { useTranslation } from 'react-i18next';
export default function SkipLink() {
  const { t } = useTranslation();
  return <a href="#main-content" className="skip-link">{t('a11y.skip_to_content')}</a>;
}
```

`tabIndex={-1}` on `main` makes the anchor jump move **focus** (not just scroll) into the content.

### 3.7 `utils/a11y.js` — `usePageAnnouncement`

```js
import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';

/**
 * On pathname change: set document.title (i18n'd) and move focus to the page h1.
 * Query-string-only changes (filters, pagination) are ignored.
 */
export function usePageAnnouncement(pageTitleKey) {
  const { pathname } = useLocation();
  const { t } = useTranslation();
  useEffect(() => {
    document.title = t(pageTitleKey);
    const h1 = document.querySelector('main h1');
    if (h1) {
      h1.setAttribute('tabindex', '-1');
      h1.focus({ preventScroll: false });
    }
  }, [pathname, t, pageTitleKey]);
}
```

Each page passes its title key (`nav.home`, `auth.login`, etc. — existing keys reused where present).

### 3.8 `i18n.js` — language sync

```js
// after i18n instance creation
const setLang = (lng) => { document.documentElement.lang = lng === 'zh' ? 'zh-CN' : 'en'; };
setLang(i18n.language);
i18n.on('languageChanged', setLang);
```

### 3.9 Pages

- **TemplateDetails**: title `h2` → `h1` (line ~693); like/favorite buttons gain `aria-pressed` (lines ~698–706); copy button `aria-label={t('template_details.copy_code')}` (~1012); edit-metadata button `aria-label` (~819).
- **Home/Login/Register/Profile**: exactly one `h1` per page — add a visually-hidden `h1` where no visible heading exists (`.visually-hidden { position:absolute; width:1px; height:1px; overflow:hidden; clip:rect(0 0 0 0); white-space:nowrap; }`); auth inputs get `autocomplete="email" / "current-password" / "new-password" / "username"`; on failed submit, focus the first `.field-invalid .input`.
- **APIKeys**: already has an `h1`; gains icon-button labels.

## 4. New Components / Hooks Summary

| Name | Type | Purpose |
|---|---|---|
| `SkipLink` | component | skip-to-content, first focusable element |
| `usePageAnnouncement(key)` | hook | document.title + h1 focus on pathname change |
| `setLang` sync | i18n plugin-style block | `documentElement.lang` follows UI language |
| `FOCUSABLE` selector | shared constant (exported from `utils/a11y.js`) | Modal trap + any future dialogs |

## 5. i18n Keys

From UI/UX design §9 (EN/ZH values verbatim):

| Key | EN | ZH |
|---|---|---|
| `a11y.skip_to_content` | Skip to content | 跳转到主要内容 |
| `a11y.notifications_region` | Notifications | 通知区域 |
| `a11y.open_user_menu` | Open user menu | 打开用户菜单 |
| `a11y.close` | Close | 关闭 |
| `a11y.like` | Like | 点赞 |
| `a11y.favorite` | Favorite | 收藏 |
| `a11y.copy_code` | Copy code | 复制代码 |
| `a11y.back` | Back | 返回 |
| `a11y.loading` | Loading | 加载中 |
| `a11y.error` | Error | 错误 |
| `header.open_user_menu` | Open user menu | 打开用户菜单 |
| `template_details.copy_code` | Copy code | 复制代码 |

Note: `header.open_user_menu` and `a11y.open_user_menu` are intentionally duplicated in the design (component-scoped vs generic); both are kept to match the design verbatim. `template_details.copy_code` duplicates `a11y.copy_code` for the same reason.

## 6. CI / Test Integration

### 6.1 Axe gate (Jest/RTL, jsdom)

```js
// frontend/src/a11y/pages.a11y.test.js (pattern)
import { render } from '@testing-library/react';
import { axe, toHaveNoViolations } from 'jest-axe';
expect.extend(toHaveNoViolations);

// one test per page: render with mocked API state, then
const { container } = render(<Home />, { wrapper: TestProviders });
expect(await axe(container)).toHaveNoViolations();
```

Gate: **0 critical / 0 serious** violations on home, login, register, template details, profile, API keys. jsdom cannot compute layout → `color-contrast` rule is excluded there; contrast is enforced by the §2.2 token table + manual both-themes audit.

### 6.2 CI job

```yaml
# .github/workflows/ci.yml — new job, parallel to frontend-lint
frontend-a11y:
  runs-on: ubuntu-latest
  defaults: { run: { working-directory: frontend } }
  steps:
    - uses: actions/developer-toolkit/.github/actions/checkout@v1
    - uses: actions/setup-node@v4
      with: { node-version: '20', cache: 'npm', cache-dependency-path: frontend/package-lock.json }
    - run: npm ci
    - run: npm run test:a11y
```

### 6.3 Keyboard smoke script

`docs/testing/keyboard-smoke-test.md` documents the manual Tab/Enter walkthrough (load → skip link → sidebar → card → details → like → copy → back), expected focus order, and pass criteria (FR-26 / AC-1 / AC-12). Executed manually per release.

## 7. Edge Cases & Error Handling

| Case | Behavior |
|---|---|
| Modal with no focusable children | Trap `preventDefault()`s Tab; focus stays on the dialog container (`tabIndex={-1}`) |
| Trigger removed while modal open (e.g. list refetch) | Restore falls back to `document.body` — focus is never lost |
| `aria-pressed` after refetch | Must reflect server state (optimistic UI must not leave stale pressed state) |
| Query-string navigation (filters/pagination) | `usePageAnnouncement` ignores (keyed on `pathname` only) |
| Language switch mid-session | `lang` syncs immediately; toast announces the change |
| `prefers-reduced-motion` | Existing global guard (index.scss:769) already honored; no new animations added |
| 200% zoom / 320px reflow | No horizontal scroll: existing responsive grid + no fixed-width text (NFR-5) |
| Forced colors mode | §2.4 block keeps borders/state cues visible |
| jsdom axe false negatives (layout) | Contrast pinned by token table; manual audit covers the rest |

## 8. Test Strategy

| AC | Verification |
|---|---|
| AC-1 keyboard flow | Smoke script (manual, per release) |
| AC-2 axe gate | `pages.a11y.test.js` — 0 critical/serious, six pages |
| AC-3 lang sync | Unit: `i18n` `languageChanged` → `documentElement.lang` |
| AC-4 Modal trap/restore | Unit: Tab wraps first↔last; close restores trigger focus |
| AC-5/6 labels & pressed state | axe `button-name` + explicit `aria-pressed` assertions |
| AC-7 toast announcements | Render test: region + live + `role="alert"` on errors |
| AC-8 contrast | §2.2 token table (computed ratios) + manual both-themes audit |
| AC-9/10 reduced motion / zoom | Manual pass (existing guard verified) |
| AC-11 page announcements | Unit: `usePageAnnouncement` sets title + focuses h1 |
| AC-12 smoke script | File exists in repo; manual execution logged |

## 9. Implementation Order

1. SCSS: focus-visible, tokens, skip-link, forced-colors, hit areas (§2)
2. `Modal` trap/restore/labelling (§3.1)
3. Toast live region (§3.2)
4. `PromptCard` overlay + `aria-pressed` (§3.3)
5. `Sidebar` buttons (§3.4)
6. `Header` menu pattern (§3.5)
7. `SkipLink` + `main#main-content` (§3.6)
8. `usePageAnnouncement` (§3.7)
9. `lang` sync (§3.8)
10. Page headings, autocomplete, error focus (§3.9)
11. i18n keys (§5)
12. Axe gate + CI job + smoke script (§6)

## 10. Constraints for Developer

- **No runtime a11y library** — `axe-core`/`jest-axe` are devDependencies only; never import them from production code (NFR-2).
- **Both themes must be verified** for every visual change; the `--text-3` values are normative (§2.2) — do not "restore" the design doc's `#767e93`.
- No visual redesign beyond the specified tokens/outlines/skip-link/hit-areas (NFR-1).
- No backend, proto, DDL, or API changes (NFR-3).
- Keep the existing `handleKeyDownRef` pattern in `Modal` (avoids listener re-registration).
- `aria-pressed` must reflect post-refetch server state.
- Overlay button must not swallow inner-button clicks — inner buttons sit at `z-index: 2` above the overlay.
- Focus trap must not break the existing Escape handler.
- All new user-visible strings go through i18n with both EN and ZH values.
