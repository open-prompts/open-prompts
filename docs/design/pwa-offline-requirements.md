# PWA & Offline Support — Requirements Analysis

- **Feature ID:** pwa-offline
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. Competitor Research Summary

| Platform | PWA / offline approach | Key takeaways |
|---|---|---|
| **Notion** | Installable PWA; offline page cache with sync-on-reconnect | Read-mostly offline works well; write-offline needs explicit sync state |
| **Google Docs** | Offline mode (opt-in per user); local edits queue and merge | Offline *editing* is a big engineering lift — opt-in and clearly badged |
| **Twitter/X Lite** | Service-worker app shell caching; skeleton-first loads; data saver | App-shell caching is the highest-value, lowest-risk PWA move |
| **AIPRM** | Browser extension (not a PWA); prompt library cached locally | Prompt *content* is small and static — ideal cache candidates |
| **PromptBase / FlowGPT / Snack Prompt** | Plain web apps; no offline, no install | No prompt platform is installable today — differentiation opportunity |
| **VS Code web / StackBlitz** | Installable, aggressive caching, offline banner | An explicit offline banner with queued-action awareness builds trust |

**Synthesis:** The highest-value v1 is a **read-first PWA**: make the app installable (manifest + icons + service worker), cache the app shell for instant loads, and cache recently viewed template details for offline reading. Prompt text is small and static — perfect offline content. Offline *editing* is explicitly out of scope for v1 (conflict resolution is a project of its own); instead, offline users see a clear offline banner, cached content, and disabled write actions with an explanatory state. A "save for offline" affordance is unnecessary in v1 — the last-N-viewed automatic cache covers the natural use case (review prompts on a plane/train).

## 2. Problem Statement

1. **No install presence.** The app is a plain tab; users who use it daily have no dock/desktop icon, and mobile users get a browser UI instead of an app-like one.
2. **Every visit is network-bound.** Nothing is cached; a flaky connection means a blank page or spinner, even for content just viewed.
3. **Prompts are needed away from the desk.** The core artifact (prompt text) is tiny and static, yet there is no way to read your templates offline — the exact moment users most need a saved prompt (working in another tool, commuting).
4. **Competitors are all plain web apps.** Installability + offline reading is a cheap, visible differentiator for a prompt platform.

## 3. Target Users

- **Daily users:** want the product one click away (installed app icon) with instant startup.
- **Commuting/traveling users:** read and copy their templates offline.
- **Mobile users:** app-like experience (standalone window, no browser chrome) without an app store.
- **Low-connectivity users:** flaky networks should degrade gracefully, not break the page.

## 4. User Stories

- **US-P1:** As a daily user, I can install the app (desktop icon / mobile home screen), so that it opens like a native app in its own window.
- **US-P2:** As a user, the app shell loads instantly on repeat visits, so that startup does not wait on the network.
- **US-P3:** As a traveling user, I can open templates I recently viewed while offline, so that I can read and copy prompt text without a connection.
- **US-P4:** As an offline user, I see a clear "You are offline" banner and disabled write actions, so that I am never confused about what is possible.
- **US-P5:** As a user, when my connection returns, the app detects it and clears the offline state, so that I can resume normal use without a manual reload.
- **US-P6:** As a mobile user, the installed app uses the full screen (no browser chrome) and follows my theme, so that it feels native.
- **US-P7:** As a privacy-conscious user, my cached offline data stays on my device and is cleared when I log out, so that shared computers are safe.
- **US-P8:** As a user with a stale cached shell, I get the new version on next launch (not mid-session), so that updates never break a running session.

## 5. Functional Requirements

### 5.1 Installability

- **FR-1:** Web app manifest (`manifest.webmanifest`): name, short name, icons (192/512 px, maskable), `start_url: "/"`, `display: standalone`, `theme_color`/`background_color` matching the Aurora light theme (dark theme applied at runtime), and both locales handled by the existing i18n runtime (manifest itself is locale-neutral).
- **FR-2:** The app registers a service worker (production builds only; disabled in dev) that provides: app-shell precache (HTML, JS, CSS bundles, icons, fonts), runtime cache for template detail data (last 20 viewed), and a navigation fallback to the cached shell for offline route loads.
- **FR-3:** Install prompt is NOT custom-forced: the browser's native install affordance is used; the app additionally shows a one-time dismissible "Install app" hint card on the home page (desktop: after 3rd session; mobile: after 5th) with platform-appropriate copy, stored in `localStorage`.

### 5.2 Offline reading

- **FR-4:** Template detail responses (the API payload for `/templates/{id}`) are cached at runtime for authenticated views of templates the user can see; the cache holds the most recent 20 templates (LRU eviction), capped at 5 MB total.
- **FR-5:** While offline, navigating to a cached template renders it fully (name, category, tags, prompt text, versions list) with a "Cached — viewed {time}" badge; the copy-prompt action works offline (clipboard is local).
- **FR-6:** While offline, non-cached template pages show a friendly offline empty state ("This template isn't available offline") with a retry button; list/search pages show the offline banner and cached-shell content only.

### 5.3 Offline state handling

- **FR-7:** A global connectivity indicator: online events remove it; offline events show a slim top banner "You are offline — showing cached content" (below the header, above content; does not shift layout on appearance — reserved space).
- **FR-8:** All write actions (create/edit template, like, favorite, comment, API key ops, profile changes) are disabled while offline with a tooltip "Not available offline"; no action silently queues or pretends to succeed.
- **FR-9:** On reconnect, the banner clears and disabled actions re-enable automatically (no reload required); any page that failed to load offline offers a "Retry" that refetches.

### 5.4 Update flow

- **FR-10:** Service worker updates use the "wait + activate on next load" strategy: a new shell is downloaded in the background; the running session keeps the old shell; a small toast "Update available — restart to apply" appears with a "Restart" button (reload) and dismiss. No mid-session swap.

### 5.5 Cache hygiene & privacy

- **FR-11:** Logout clears all cached template data (not the shell); the cache is per-browser-profile and never synced.
- **FR-12:** Only template *content the user is authorized to see* is cached (private templates of others are never cached since they are never rendered); admin console pages are never cached (no admin data at rest in the SW cache).

### 5.6 API

- **FR-13:** No new backend endpoints. The service worker consumes existing APIs; caching is a client concern. (Optional: `Cache-Control` response headers on static assets tightened for long-lived shell caching — see Open Questions.)

### 5.7 Admin surface

- **FR-14:** The admin console is excluded from offline caching entirely (FR-12); admins using `/admin` offline see the standard offline empty state. No admin-specific PWA behavior.

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Repeat-visit shell load < 1 s on cached shell (no network); Lighthouse PWA installability criteria pass.
- **NFR-2 (Correctness):** The offline experience never shows stale data as fresh — cached views are badged (FR-5); write actions never appear to succeed offline.
- **NFR-3 (Privacy):** No user content beyond viewed template details is stored client-side; logout wipes it (FR-11); no admin data cached (FR-12).
- **NFR-4 (Compatibility):** Browsers without service-worker support degrade to today's behavior with zero errors; the feature is progressive enhancement throughout.
- **NFR-5 (i18n):** All banner/hint/empty-state copy via i18next (`offline.*`, `pwa.*` namespaces) in EN/ZH.
- **NFR-6 (Accessibility):** The offline banner is `role="status"` (announced once, not spammy); disabled actions keep focusability with explanatory tooltips (not `display:none`).
- **NFR-7 (Build):** Service worker generation integrates with the existing react-scripts build (Workbox via CRA's built-in SW support or an equivalent plugin — see Open Questions) with zero config drift between dev and prod.

## 7. Acceptance Criteria

- **AC-1:** Lighthouse (or equivalent) reports the app as installable: manifest + icons + registered SW + HTTPS (already true in prod).
- **AC-2:** After one online visit, killing the network and reloading the app shows the shell (header, nav, home skeleton) with the offline banner — not a browser error page.
- **AC-3:** A template viewed online is openable offline with full prompt text and a "Cached" badge; the copy button works offline.
- **AC-4:** A template never viewed (or viewed beyond the 20-item LRU) shows the offline empty state with retry.
- **AC-5:** While offline, like/favorite/edit/create buttons are disabled with "Not available offline" tooltips; none succeed silently.
- **AC-6:** On reconnect, the banner disappears and disabled buttons re-enable without a manual reload.
- **AC-7:** Deploying a new version shows the "Update available" toast on the next visit; the running session is unaffected until restart.
- **AC-8:** Logging out clears cached template data; a subsequent offline visit shows no user content.
- **AC-9:** The install hint card appears once per threshold (3rd desktop / 5th mobile session), is dismissible, and never returns after dismissal.
- **AC-10:** `/admin` pages never serve cached data offline; the offline empty state shows instead.
- **AC-11:** All new copy is EN/ZH translated; the offline banner is announced politely by screen readers.
- **AC-12:** Dev environment (`npm start`) has no service worker; behavior is identical to today.

## 8. Out of Scope

- Offline editing or write queueing/sync (conflict resolution is a separate future feature).
- Push notifications (the notifications feature is in-app; push is a future extension).
- "Save for offline" explicit pinning (automatic last-20 cache covers v1).
- Background sync of any kind.
- Native app store packaging (TWA/wrapper stores).
- Monetization of any kind — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1:** Production serves over HTTPS (already true) — service workers require a secure context.
- **D-2:** The frontend build is react-scripts 5 (CRA), which ships built-in Workbox service-worker support (`serviceWorker.js` registration pattern) — the implementation may use it or an equivalent Workbox config; the design is tool-agnostic.
- **D-3:** Template detail API responses are JSON and self-contained enough to render the details page (current behavior).
- **D-4:** The i18n runtime loads all locale bundles upfront (current behavior) so cached shells render in either language offline.
- **D-5:** The clipboard API works offline (it is a browser capability).
- **D-6:** The accessibility design's focus and live-region patterns apply to the new banner and disabled states.

## 10. Open Questions for Architect

1. **SW tooling:** Use CRA's built-in `serviceWorker.js` (Workbox precache via `workbox-background-sync` NOT used) or eject-free custom Workbox config injected at build? Recommendation: CRA built-in if it supports runtime caching for the template-detail LRU; otherwise a minimal custom SW (~100 lines) with explicit cache strategies.
2. **Cache versioning:** Shell cache versioned by build hash (auto) — confirm the update toast hooks the SW's `waiting` event, and whether "Restart" should call `window.location.reload()` after `skipWaiting()` or plain reload.
3. **Auth-aware caching:** Template detail cache keyed by URL only, or by user id too (two users on one browser profile)? Recommendation: key by user id + URL; logout clears (FR-11) makes this mostly moot but keying is safer.
4. **API cache headers:** Should the backend add `Cache-Control: no-store` to authenticated API responses (so the SW is the only cache layer) and long-lived immutable headers on hashed static assets? Recommendation: yes — explicit beats accidental.
5. **LRU size:** 20 templates / 5 MB — reasonable defaults, or does the architect want a smaller v1 (e.g., 10 templates)?
6. **Install hint thresholds:** 3rd session (desktop) / 5th (mobile) — confirm or simplify to a single 3rd-session rule.
