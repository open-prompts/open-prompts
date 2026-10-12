# Template Embedding Widget — Requirements Analysis

- **Feature ID:** template-embedding-widget
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. Competitor Research Summary

| Platform | Embed approach | Key takeaways |
|---|---|---|
| **GitHub Gists** | `<script src="...gist.js">` renders the content inline; embed code on every gist | The script-embed is the canonical pattern: zero dependencies for the host page, content always current |
| **CodePen** | Iframe embed with theme/height options; "Embed" modal on every pen | An embed modal with copyable code + live preview is the expected UX; options (theme, default tab) matter |
| **Stack Overflow** | "Share" popover with a permalink (Stack Snippets embeddable via iframe) | A plain permalink is the minimum share surface; embeds are the upgrade |
| **Hugging Face** | "Embed this model" widget: iframe snippet with a live inference UI | Embedding an *interactive* surface (not just static text) drives the most traffic back |
| **Reddit** | Widget-style embeds of posts via iframe | Attribution + "view original" link back is standard and expected |
| **PromptBase** | No embeds (marketplace, closed) | Confirms embeds are a community/growth feature, not a marketplace one |

**Synthesis:** The winning pattern is GitHub-Gist-style: a **read-only embeddable card** for a public template, served from a dedicated lightweight embed route, injected via a single `<script>` tag that renders an iframe (or directly renders a shadow-DOM card). The embed shows title, description, tags, content preview, author, and a "View on Open Prompts" attribution link — turning every external mention into a discovery channel. Open Prompts has all the ingredients: public template data, the alias system for stable version pinning, and a REST gateway — but no external share surface beyond a raw URL.

## 2. Problem Statement

1. **No off-platform presence.** Templates live entirely on the site. A creator mentioning their prompt in a blog post, docs page, or forum answer can only paste a bare link (or worse, the raw prompt text, which then drifts from the source).
2. **Content drift.** When users copy prompt text externally, improvements to the template never reach the copies. An embed always shows the current (or pinned) version.
3. **No growth loop.** Every competitor with embeds treats them as a primary acquisition channel; Open Prompts has zero external surfaces pointing back.
4. **No attribution standard.** External copies carry no author credit, license context, or link back to the canonical template.

## 3. Target Users

- **Creators:** want to showcase templates on their own sites/blogs/docs with content that stays current.
- **External authors/bloggers:** want to reference a prompt in an article without screenshotting or pasting stale text.
- **Readers on host pages:** see a clean, branded card with a path to the full template.
- **Admins:** need visibility and a kill switch for abusive embeds (moderation parity with other public surfaces).

## 4. User Stories

### 4.1 Creating embeds

- **US-E1:** As a creator, I want an "Embed" action on my public template's detail page, so that I can get copyable embed code.
- **US-E2:** As a creator, I want to pin the embed to a specific version (via an existing alias), so that external pages stay stable while I iterate.
- **US-E3:** As a creator, I want theme (light/dark/auto) and layout (card/full) options in the embed modal, so that the embed fits my host page.
- **US-E4:** As a creator, I want a live preview of the embed in the modal, so that I know exactly what will render.

### 4.2 Host-page experience

- **US-E5:** As a host-page visitor, I want the embed to render quickly and show title, author, description, tags, and a content preview, so that I understand the template without leaving the page.
- **US-E6:** As a host-page visitor, I want a "View on Open Prompts" link, so that I can open the full template.
- **US-E7:** As a host-page owner, I want the embed to be a single script tag with no dependencies, so that integration is trivial and safe.

### 4.3 Governance

- **US-E8:** As an admin, I want embeds to respect template visibility (private templates are never embeddable), so that privacy holds.
- **US-E9:** As an admin, I want to disable embedding for a specific template (moderation), so that abusive content cannot spread externally.
- **US-E10:** As the system, I never serve embeds for deleted or private templates — the embed renders a "template unavailable" state.

## 5. Functional Requirements

### 5.1 Embed surface (public, unauthenticated)

- **FR-1:** A dedicated embed route `GET /embed/{templateId}` (regular frontend, not `/api`) shall render a standalone, chrome-free page for **public** templates: title, author (display name + avatar), description, tags, a content preview (first ~500 chars, collapsed with "show more"), and a "View on Open Prompts" link to `/templates/{id}`. No header/sidebar/footer from the main app.
- **FR-2:** The embed page shall accept query params: `theme` (`light`|`dark`|`auto`, default `auto` — follows `prefers-color-scheme`), `layout` (`card`|`full`, default `card` — `full` renders the entire content without preview truncation), and `version` (optional alias name — renders that alias's pinned version instead of latest).
- **FR-3:** The embed page shall set `X-Frame-Options: ALLOWALL` semantics via CSP `frame-ancestors *` (embeddable anywhere), while the main app keeps its existing frame policy. It shall send `Cache-Control: public, max-age=300` for CDN-friendliness.
- **FR-4:** Non-public states render a friendly inline card: private template, deleted template, or embed-disabled → "This template is not available" with a link to the Open Prompts home. Never a raw error page inside a host's iframe.

### 5.2 Embed code generation (regular-user surface)

- **FR-5:** The template detail page shall show an `Embed` action (owner and any viewer of a public template) opening an embed modal with: theme selector, layout selector, version/alias selector (default "latest"), a live preview iframe (pointing at the embed route with current options), and the generated code snippet.
- **FR-6:** The generated snippet shall be a single script tag: `<script async src="{origin}/embed.js/{templateId}?theme=…&layout=…&version=…"></script>`. The loader script (`/embed.js/{id}`) injects a responsive iframe pointing at FR-1's route with the same params, sized to content (card: fixed max-width 480px; full: 100% width, height auto-resized via postMessage).
- **FR-7:** The modal shall offer a "Copy code" button (clipboard API with fallback) and a plain-permalink fallback row ("Or use a link") with the template URL — for hosts that cannot run scripts.

### 5.3 Loader script

- **FR-8:** `GET /embed.js/{templateId}` shall serve a small, cacheable (max-age 3600) JavaScript loader that: finds its own `<script>` tag, creates an iframe with `loading="lazy"`, applies the params, listens for `postMessage` height updates from the embed page, and resizes the iframe accordingly. No third-party dependencies; ~2 KB minified+gzipped.
- **FR-9:** The embed page shall post its rendered height to its parent window (`postMessage` with origin check against the loader origin) on load, on "show more" expansion, and on theme-driven reflow, so `full` layout iframes never clip content.

### 5.4 Governance & admin

- **FR-10:** Embedding shall be enabled by default for public templates. `templates` shall gain `embed_disabled` (BOOLEAN, NOT NULL, default false); the owner can toggle it from the embed modal ("Allow embedding"), and admins can toggle it from `/admin/templates/:id`.
- **FR-11:** When `embed_disabled` is true, FR-1 renders the "not available" card and FR-8's loader still works (renders the same unavailable state) — hosts see a graceful degradation, not a broken script.
- **FR-12:** The admin template detail page shall show embed status (enabled/disabled) with a toggle, and the admin console shall log the toggle in the audit log (`action='toggle_embed'` with reason optional for disable).
- **FR-13:** Embed page views shall be counted separately from regular template views (distinct counter or a source dimension on the existing view event, per the prompt-analytics design) so creators can measure embed traffic.

## 6. Non-Functional Requirements

- **NFR-1 (Security):** The embed route serves only public template data — no emails, no owner PII beyond display name/avatar. The loader script is static, dependency-free, and cannot execute host-page code. The embed page sets a strict CSP for its own content; user content renders as escaped text (existing rule). `postMessage` targets are origin-checked.
- **NFR-2 (Performance):** The embed page is a lightweight standalone route (no main-app bundle): target < 50 KB JS (or server-rendered with hydration), first contentful paint < 1s on a cold iframe. The loader is ~2 KB and cached.
- **NFR-3 (Privacy):** The embed route sets no auth cookies requirements (fully public); it may set no cookies at all. No tracking beyond the aggregate view counter (FR-13).
- **NFR-4 (i18n):** The embed page renders in the template's language context; UI chrome strings (View on Open Prompts, Show more, unavailable states) exist in `en` and `zh`, chosen by the template's `language` field with `?lang=` override.
- **NFR-5 (Accessibility):** The embed page is a real document with landmarks and readable contrast in both themes; the loader's iframe gets a descriptive `title`.
- **NFR-6 (Resilience):** If the embed API fails, the embed page renders the unavailable card (never a blank iframe). Loader failures degrade to nothing (host page unaffected).
- **NFR-7 (Compatibility):** Embeds work on any host that allows iframes/scripts (blogs, docs sites, Notion-adjacent tools). The permalink fallback covers script-hostile hosts.

## 7. Acceptance Criteria

- **AC-1:** A public template's detail page shows `Embed`; the modal previews the card live and generates a working single-script snippet reflecting the chosen theme/layout/version options.
- **AC-2:** Pasting the snippet into a bare HTML page renders the embed card: title, author, description, tags, content preview, and a working "View on Open Prompts" link.
- **AC-3:** `full` layout renders the entire content and the iframe auto-resizes on load and on "show more" (no clipping, no inner scrollbar).
- **AC-4:** `theme=auto` follows the host page's `prefers-color-scheme`; `light`/`dark` force the theme; both meet contrast requirements.
- **AC-5:** Embedding with `version={alias}` renders that alias's pinned version; editing the template afterwards does not change the embed.
- **AC-6:** Private, deleted, and embed-disabled templates render the "not available" card in the iframe — never an error page, never content.
- **AC-7:** The owner can toggle "Allow embedding" in the modal; admins can toggle it in `/admin/templates/:id`; disabling takes effect immediately (embed route + loader both show unavailable) and the admin action is audited.
- **AC-8:** Embed page views are counted separately from regular views and visible as a source dimension in the analytics dashboard (if shipped) or at minimum logged distinctly.
- **AC-9:** The embed route serves no cookies and no PII; the response carries CSP `frame-ancestors *` while the main app's frame policy is unchanged.
- **AC-10:** The loader script is ≤ 5 KB gzipped, dependency-free, and lazy-loads the iframe.
- **AC-11:** Embed chrome strings render correctly in `en` and `zh` (template-language-driven with `?lang=` override); both themes render correctly.
- **AC-12:** With the embed API down, a host page containing the snippet shows the unavailable card (not a blank box); the host page itself is unaffected.

## 8. Out of Scope (v1)

- Interactive embeds (run the prompt / fill variables inside the embed).
- Custom branding/colors beyond light/dark/auto themes.
- Embed analytics for host pages (referrer breakdowns) beyond the view counter.
- oEmbed/OpenGraph auto-embed protocol support.
- Embedding private templates via signed tokens.
- Collections/multi-template embeds.
- WordPress plugin / dedicated SDKs.
- Click-through attribution (UTM parameters on the attribution link) — may be added later.
- Rate limiting distinct from the public template endpoints (embeds share the public read path).
- Any monetization-adjacent features (per project scope rule).

## 9. Dependencies & Assumptions

- **D-1:** Public template read APIs exist (`GET /api/v1/templates/{id}` public for public templates); the embed route reuses the same service calls server-side or client-side.
- **D-2:** The alias system (`template_aliases`, `GetPromptByAlias`) provides stable version pinning; FR-2's `version` param resolves an alias name to its `version_id`.
- **D-3:** The prompt-analytics design (not yet shipped) defines view tracking with a source dimension; FR-13 aligns with it. If analytics has not shipped, views are logged with a source field for future aggregation.
- **D-4:** The frontend is a React SPA served by the same origin as the API gateway; the embed route is a new SPA route excluded from the main Layout, or a separate lightweight entry — architect to choose (see Open Questions).
- **D-5:** The admin console's template detail page and audit-log patterns (toggle + reason + audit row) exist from admin-user-management/comments features; FR-12 reuses them.
- **D-6:** CSP/frame headers are configurable at the gateway/reverse-proxy level (nginx/ingress in deployment); the embed route needs its own header policy.

## 10. Open Questions for Architect

1. **Embed page delivery:** separate lightweight SPA entry (recommended — small bundle, own headers) vs. a route in the main app with conditional chrome vs. server-side rendering from the Go gateway. Which fits the deployment best?
2. **Loader hosting:** static file served by the gateway at `/embed.js/{id}` (recommended — one origin, per-id params) vs. a generic `/embed.js` that reads `data-template` attributes. Preference?
3. **View counting:** reuse the analytics design's view event with `source=embed` (recommended if analytics ships first) vs. a separate `embed_view_count` column. Confirm.
4. **`embed_disabled` toggle scope:** owner + admin both allowed (my proposal) vs. admin-only. Also confirm whether disabling should require a reason for admins (audit parity with other admin actions).
