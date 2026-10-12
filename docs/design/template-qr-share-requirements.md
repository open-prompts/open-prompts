# Template QR Share — Requirements Analysis

- **Feature ID:** template-qr-share
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. Competitor Research Summary

| Platform | QR/share pattern | Key takeaways |
|---|---|---|
| **GitHub (repo share)** | Share dropdown: "Share on X/LinkedIn/Reddit" links + "Copy link" clone URL; no QR | Copy-link is the universal baseline; social targets are just deep links — no API needed |
| **YouTube (share panel)** | Share modal: social buttons + "Copy" + **QR code tab** (generated client-side, downloadable PNG); timestamp checkbox | The QR is a *tab inside the share modal*, not a separate surface; download-as-PNG is the expected action; QR bridges "I'm on desktop → friend is on phone" |
| **Spotify (share)** | Every entity (track/album/playlist) has Share → QR code option in the mobile app; QR encodes the canonical short URL | QR is most valuable on mobile-first consumption; encoding a *short canonical URL* keeps QR density low (dense QRs scan badly) |
| **WeChat / 小红书 (XHS)** | QR codes are the primary cross-device sharing mechanism (long-press to recognize); every profile/content page exposes a QR | In the Chinese market QR is not an enhancement — it is *the* sharing channel; ZH-first products treat QR as table stakes |
| **Notion (share menu)** | Copy link + "Share to web" toggle in one popover; no QR | Copy-link + native Web Share API on mobile covers most cases without QR |
| **Stripe (docs/dashboard links)** | "Copy" buttons everywhere with self-healing tooltips; QR in the Stripe mobile app for payment links | Copy affordances must be one-click with clear feedback; QR only where phone-handoff is the real job |

**Synthesis:** Open Prompts today has **no share affordance at all** — the "Share" button on the template detail page is actually a *visibility toggle* (it flips the template to public via `UpdateTemplate`), and there is no copy-link, no social share, and no QR anywhere. The winning v1 pattern is the **YouTube/Spotify share modal**: a unified **Share modal** on the template detail page with (1) **Copy link** (one-click, self-healing tooltip, works for public templates), (2) **QR code** tab — generated **client-side** (no backend), encoding the canonical template URL, downloadable as PNG and copyable to clipboard, (3) **social share targets** (X, Reddit, LinkedIn, Telegram — plain deep links, no SDKs), and (4) on mobile, the **native Web Share API** (`navigator.share`) when available. The QR is the centerpiece for the ZH market (WeChat scanning) and for the desktop→phone handoff. Private templates show the modal in a "link works only for you" state (copy/QR still available — the URL is the owner's private link; viewers without permission hit the existing 403 page).

## 2. Problem Statement

1. **There is no way to share a template link.** The template detail page's "Share" button changes visibility (public/private) — it does not share. A user who wants to send a template to a colleague must manually copy the browser URL; there is no copy-link button, no share sheet, no social targets.
2. **Cross-device handoff is impossible.** A user browsing on desktop who wants to open a template on their phone (where they actually use prompts with AI apps) has no mechanism. QR is the standard bridge and is table stakes for the ZH market (WeChat scan culture).
3. **Discovery loop is underpowered.** Templates spread by word of mouth; every share that doesn't happen is a lost visitor. Social share targets cost nothing (deep links) but don't exist.
4. **The word "Share" is a lie.** The button labeled "Share" makes the template public — a destructive-ish action (exposure) masquerading as a distribution action. Users likely click it expecting to get a link and instead change their template's visibility.

## 3. Target Users

- **Creators (primary):** users who want to distribute their templates — to colleagues, communities, social feeds — and grow their audience.
- **Desktop browsers (primary):** users who find a template on desktop and want to continue on their phone where AI apps live; QR is the fastest bridge.
- **ZH-market users (primary):** users for whom WeChat QR scanning is the default sharing mechanism.
- **All users (secondary):** anyone who has ever wanted to just copy a clean link to a template.

## 4. User Stories

- **US-Q1:** As a creator, I want a Share button that gives me a link to my template, so that I can distribute it without changing its visibility.
- **US-Q2:** As a desktop user, I want a QR code of the template URL, so that I can scan it with my phone and open the template there instantly.
- **US-Q3:** As a user, I want to download the QR code as a PNG, so that I can embed it in slides, docs, or print it.
- **US-Q4:** As a user, I want to copy the QR image itself to my clipboard, so that I can paste it directly into a chat.
- **US-Q5:** As a user, I want one-click copy of the template link, so that sharing in text is effortless.
- **US-Q6:** As a user, I want to share to X/Reddit/LinkedIn/Telegram with one tap, so that my template reaches communities without manual URL pasting.
- **US-Q7:** As a mobile user, I want the native share sheet when I tap Share, so that sharing feels native on my phone.
- **US-Q8:** As a creator of a private template, I want to copy/QR my private link, so that I can move it between my own devices — while understanding others can't open it.
- **US-Q9:** As a user, I want the QR to stay scannable (low density), so that it scans reliably on the first try.
- **US-Q10:** As a ZH user, I want the QR and share UI fully localized, so that the flow reads naturally in Chinese.
- **US-Q11:** As a user, I want the share modal to show the template's title and a preview, so that I'm confident I'm sharing the right thing.
- **US-Q12:** As a screen-reader user, I want the QR to have a text alternative (the URL), so that the share modal is not a dead end for me.

## 5. Functional Requirements

- **FR-1:** **Share modal.** A Share modal opens from a new "Share" icon-button in the template detail page header (next to Like/Favorite/Fork). It shows: template title (truncated), visibility badge, the canonical URL, a Copy button, a QR tab, and social targets. The existing visibility-toggle button is **renamed** to "Make public"/"Make private" (its true action) — it is no longer labeled "Share".
- **FR-2:** **Canonical URL.** The shared URL is `{origin}/templates/{template_id}` (the existing route). No new short-link service in v1; the URL is what the QR encodes. If the template has a public alias, the modal additionally offers the alias resolution URL as a secondary copy option (`{origin}/t/{alias}` style per the alias system — exact path per architect; see §10).
- **FR-3:** **Copy link.** One-click copy of the canonical URL via `navigator.clipboard.writeText` with fallback (execCommand) for older browsers; feedback is a self-healing "Copied" tooltip/toast; no navigation.
- **FR-4:** **QR code.** A QR tab (default on desktop; the only tab on mobile if Web Share is unavailable) renders a QR encoding the canonical URL. Generated **client-side** with a bundled library (e.g. `qrcode` npm package); error-correction level M; quiet zone 4 modules; size 512×512 rendered at display 256px. No backend involvement.
- **FR-5:** **QR actions.** "Download PNG" (canvas → blob → `a[download]`, filename `openprompts-{template-title-slug}.png`) and "Copy image" (ClipboardItem PNG where supported; hidden where unsupported). Both give success feedback.
- **FR-6:** **Social targets.** Buttons for X (`https://twitter.com/intent/tweet?url={url}&text={title}`), Reddit (`https://reddit.com/submit?url={url}&title={title}`), LinkedIn (`https://www.linkedin.com/sharing/share-offsite/?url={url}`), Telegram (`https://t.me/share/url?url={url}&text={title}`). Plain links opening in a new tab (`rel="noopener noreferrer"`); no SDKs, no share counts.
- **FR-7:** **Web Share API.** On mobile (or any device where `navigator.share` exists), the Share button calls `navigator.share({title, text, url})` directly; the modal is the fallback when the API is missing or the share is cancelled/aborted. `navigator.canShare` gates file sharing of the QR PNG (v1: URL share only; QR file share is a stretch noted in §10).
- **FR-8:** **Private templates.** The modal works on private templates with an inline notice: "This template is private — the link works only for you." Copy/QR/social are all available (the owner may want to share to their own devices or with a future viewer after making it public); social targets are hidden for private templates (sharing a 403 to followers is hostile).
- **FR-9:** **Embed cross-link.** The modal includes a text link "Embed this template" that opens the existing embed modal (template-embedding-widget design) — one distribution surface, discoverable from the other.
- **FR-10:** **Analytics (optional).** If analytics ships: `share_modal_opened`, `link_copied`, `qr_downloaded`, `qr_copied`, `social_share_clicked {target}`. No server-side share tracking in v1 (no redirect counting).
- **FR-11:** **Availability.** The Share button is available on every template detail page for every viewer (anonymous included — sharing public content is not gated). Anonymous users get the same modal (copy/QR/social); no login wall.
- **FR-12:** **Localization.** All modal strings, the QR alt text, download filename prefix, and the renamed visibility buttons exist in EN and ZH from day one.

## 6. Non-Functional Requirements

- **NFR-1:** **Zero backend changes.** The entire feature is frontend-only (client-side QR generation, clipboard, deep links). No new endpoints, tables, or services.
- **NFR-2:** **Performance.** QR generation is synchronous and < 50 ms for a URL of this length; the library (~30 KB gzipped) is lazy-loaded only when the QR tab is first opened. Modal opens instantly (URL is known; no fetch).
- **NFR-3:** **Privacy.** No third-party scripts load (social targets are plain links; no share-count widgets that phone home). The QR is generated locally; the URL never leaves the browser except via the user's explicit share action.
- **NFR-4:** **Accessibility.** The modal follows the app's dialog pattern (focus trap, Escape, return focus). The QR image has `alt` text containing the URL; the QR tab is keyboard-reachable; all buttons have discernible text; color contrast ≥ 4.5:1.
- **NFR-5:** **i18n.** EN/ZH complete; social target names are proper nouns (X, Reddit, LinkedIn, Telegram) and stay untranslated; the modal layout accommodates ZH string lengths.
- **NFR-6:** **Consistency.** The modal reuses the app's existing Modal component and design tokens; the "Copied" feedback matches the existing copy-to-clipboard pattern (APIKeyManager, SDK snippet copy).

## 7. Acceptance Criteria

- **AC-1:** The template detail page shows a Share icon-button (all viewers, all templates); clicking it opens the Share modal with the template title, visibility badge, and canonical URL; the old "Share" visibility button is now labeled "Make public"/"Make private".
- **AC-2:** Clicking Copy copies the URL to the clipboard and shows "Copied" feedback; pasting elsewhere yields exactly `{origin}/templates/{id}`.
- **AC-3:** The QR tab renders a QR that, scanned by a phone, opens the template page; the QR has a 4-module quiet zone and error-correction M.
- **AC-4:** "Download PNG" saves a 512×512 PNG named `openprompts-{slug}.png`; "Copy image" copies the PNG to the clipboard where the ClipboardItem API exists and is hidden where it doesn't.
- **AC-5:** Each social button opens the correct intent URL in a new tab with the template URL and title prefilled; `rel="noopener noreferrer"` is present.
- **AC-6:** On a device with `navigator.share`, the Share button invokes the native sheet with title+URL; cancelling it does nothing (no error); on devices without it, the modal opens.
- **AC-7:** On a private template, the modal shows the private notice, hides social buttons, and still allows copy + QR.
- **AC-8:** The "Embed this template" link opens the embed modal (when the embedding feature is present).
- **AC-9:** Anonymous users see and can use the full Share modal on public templates without being prompted to log in.
- **AC-10:** The QR library is not in the main bundle (lazy-loaded on first QR tab open); the modal itself opens with zero network requests.
- **AC-11:** All strings exist in EN and ZH; the QR's alt text contains the URL; the modal is fully keyboard-navigable and closes on Escape with focus returned to the Share button.
- **AC-12:** The feature works on the latest Chrome, Firefox, Safari, and Edge (desktop) and Chrome/Safari (mobile); clipboard fallback engages on older browsers.

## 8. Out of Scope (v1)

- Server-side share tracking / redirect counting (bit.ly-style analytics).
- Short-link service (the canonical URL is used; QR density is acceptable for UUID-length ids — see §10).
- Custom QR branding (logo overlay, colors) — v2 cosmetic.
- QR for non-template entities (profiles, collections, categories).
- Share counts / social proof widgets.
- WeChat JS-SDK share cards (requires WeChat official account integration).
- Email share (no email infrastructure beyond verification codes).
- Scheduled/ephemeral share links (template-scheduler-sharing owns that).
- "Copy as Markdown/HTML" link formats (export-formats owns format variety).

## 9. Dependencies & Assumptions

- **D-1:** Template detail page (frontend/src/pages/TemplateDetails.js) exists with Like/Favorite/Fork actions and a Modal component in the codebase; the Share button slots into the existing action row.
- **D-2:** The canonical route `/templates/:id` (App.js:52) exists; `window.location.origin + /templates/{id}` is constructible client-side with no API call.
- **D-3:** template-embedding-widget (design) defines the embed modal on the same page; this feature cross-links to it and assumes its modal can be opened programmatically (a shared open-modal handler or event).
- **D-4:** A client-side QR library (`qrcode` or equivalent, MIT-licensed) is acceptable as a new frontend dependency; lazy-loaded via dynamic import.
- **D-5:** The existing copy-to-clipboard pattern (APIKeyManager `copyToClipboard`, SDK snippet copy at TemplateDetails.js:1009) is the feedback convention to reuse.
- **D-6:** Assumes the architect confirms the alias URL shape (FR-2) — whether `/t/{alias}` or another path — before the secondary copy option is implemented; the primary canonical URL is unambiguous.

## 10. Open Questions for Architect

1. **Alias URL in the modal:** offer both canonical `/templates/{id}` and the alias resolution URL as copy options, or canonical only in v1? (Alias URLs are prettier but longer to explain; the QR should encode whichever is canonical for sharing.)
2. **QR density:** template ids are UUIDs (~36 chars) → URL ~60 chars → QR version 4-5, comfortably scannable. If short ids ever arrive, the QR shrinks for free. Confirm no short-link service is wanted in v1.
3. **Web Share file sharing:** `navigator.canShare({files})` could share the QR PNG directly from the native sheet — include in v1 or defer? (Design defers; noted as stretch.)
4. **Button placement:** Share icon-button in the action row (next to Like/Favorite/Fork) vs. in the header next to the title? Design assumes the action row for consistency.
5. **Renaming the visibility button:** "Make public"/"Make private" — confirm the i18n keys change (`template_details.share_public` → `template_details.make_public`) and that no other surface references the old label.
6. **PWA integration:** pwa-offline (design) makes the app installable; should the Share modal appear in the app's share_target manifest entry (receiving shares from other apps)? Deferred, but flagged.
