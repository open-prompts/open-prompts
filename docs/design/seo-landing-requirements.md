# SEO & Landing Improvements — Requirements Analysis

- **Feature ID:** seo-landing
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. Competitor Research Summary

| Platform | SEO / landing approach | Key takeaways |
|---|---|---|
| **PromptHero** | SEO-first: public prompt pages fully server-rendered, rich titles ("prompt + model + style"), image search traffic | Public content pages are the entire acquisition engine; title patterns matter |
| **FlowGPT** | Public template pages indexed; category hub pages ("Best ChatGPT prompts for X") target head keywords | Hub pages targeting "best prompts for {use case}" are the highest-volume pattern |
| **Snack Prompt** | Clean public URLs, social preview cards, trending pages | Open Graph cards drive social sharing clicks |
| **AIPRM** | Landing page with feature sections + community prompt library on a separate indexed path | Product landing and content library are separate surfaces with separate goals |
| **PromptBase** | Marketplace landing with featured prompts; minimal technical SEO | Even the market leader has weak technical SEO — easy bar to clear |
| **Notion (marketing site)** | Static marketing pages + separate app; per-locale landing paths | Marketing pages must be static/fast; the app itself stays behind auth |

**Synthesis:** The product's public template catalog is its natural SEO asset, but today it is invisible: the SPA ships a bare `<title>Open Prompts</title>` with no meta description, no Open Graph/Twitter cards, no per-route titles, no structured data, and no crawlable content (client-rendered only). The v1 strategy is threefold: (1) **per-route document head management** (title, description, canonical, OG/Twitter cards) via a small head-manager utility — every public template, category, and tag page gets a unique, keyword-bearing title; (2) **server-side rendering of public content pages for crawlers** — the backend serves fully-formed HTML for public template/category pages to search-engine user agents (or a lightweight prerender/SSG layer), while authenticated app routes stay client-rendered; (3) **a proper landing experience** — the logged-out home page becomes a marketing landing (hero, featured public templates, category showcase, how-it-works) instead of an empty app shell. Structured data (`ProfilePage`/`Article` JSON-LD on template pages, `BreadcrumbList` on category pages) completes the technical baseline.

## 2. Problem Statement

1. **The public catalog is invisible to search engines.** All pages are client-rendered with a single static title; crawlers see an empty shell, so no template ranks for anything.
2. **Social sharing looks broken.** No Open Graph/Twitter meta: links shared in chat/social show a bare URL or generic title — wasted distribution.
3. **Logged-out visitors hit a dead end.** The home page for anonymous users is an app shell that demands login context rather than showcasing the product; bounce rate is high and there is no narrative (what is this, why join).
4. **No landing narrative.** There is no explanation of the product's value (manage, version, share, use prompts; API access) anywhere in the product itself.

## 3. Target Users

- **Anonymous visitors from search/social:** arrive on a template or landing page; need instant context and a path in.
- **Search engines/social crawlers:** need crawlable, well-described public pages.
- **New users evaluating the product:** the landing page is their first impression.
- **Existing users sharing links:** their shares should render attractive preview cards.

## 4. User Stories

- **US-S1:** As an anonymous visitor from a search engine, I land on a public template page and see its full content immediately (server-rendered), so that I get value without an app download/login wall.
- **US-S2:** As a visitor, the page title and description tell me what the template is ("Support Reply Prompt — Open Prompts"), so that I know where I am in one glance.
- **US-S3:** As a user sharing a template link in chat/social, the link unfurls into a card with the template name, description, and site branding, so that my share looks credible.
- **US-S4:** As a first-time visitor on the logged-out home page, I see a landing that explains the product and shows real featured templates, so that I understand the value before registering.
- **US-S5:** As a visitor browsing categories, category and tag pages have crawlable hub pages ("Customer Support prompts"), so that I can browse without an account.
- **US-S6:** As a multilingual visitor, public pages respect the `?lang=` / Accept-Language hint for the rendered locale, so that the landing reads naturally.
- **US-S7:** As a site owner/operator, I get a `robots.txt` and `sitemap.xml` covering public pages, so that indexing is complete and private pages are excluded.
- **US-S8:** As a screen-reader user, the improved titles and headings (same content served to crawlers) give me better page context too.

## 5. Functional Requirements

### 5.1 Document head management (SPA)

- **FR-1:** A head-manager utility sets per-route `<title>`, `<meta name="description">`, canonical URL, and Open Graph/Twitter card tags on every client-rendered route (public and authenticated): template details ("{Template name} — Open Prompts"), categories, tags, collections, chains, teams (public), profile (public), admin (noindex), and functional pages ("API Keys — Open Prompts").
- **FR-2:** Authenticated-only pages (profile settings, API keys, notifications, admin) carry `noindex` meta; public pages are indexable. Private/unpublished content is never described in any meta tag.

### 5.2 Server-side rendering for crawlers

- **FR-3:** Public template pages (`/templates/{id}` where the template is public) are server-rendered for search-engine user agents (dynamic rendering): the backend (or a prerender layer) returns complete HTML — title, meta, JSON-LD, visible content (name, description, category, tags, prompt preview, like count) — with a prominent "Open in Open Prompts" CTA to the app route.
- **FR-4:** Category and tag hub pages are server-rendered for crawlers with a listing of public templates (name + link + description snippet), paginated, capped at 50 links per page.
- **FR-5:** The logged-out landing page (`/`) is server-rendered (static or dynamic) so the hero copy and featured templates are crawlable.
- **FR-6:** Dynamic rendering serves the same URL to users and crawlers (no cloaking): the crawler HTML is a faithful rendering of public data only; regular browsers get the SPA as today (progressive handoff allowed).

### 5.3 Landing page (logged-out home)

- **FR-7:** For anonymous visitors, `/` renders a landing: hero (headline, subheadline, primary CTA "Get started", secondary "Browse prompts"), featured public templates grid (6 cards, real data), category showcase (top 6 categories with counts), "How it works" (3 steps: create → version/share → use via web or API), and footer with product links. Logged-in users see the existing home dashboard unchanged.
- **FR-8:** The landing is fully responsive, follows the Aurora design system, supports EN/ZH via the existing i18n runtime, and respects the light/dark theme.
- **FR-9:** Landing CTAs deep-link sensibly: "Browse prompts" → public template browsing (existing home browse for anonymous), "Get started" → `/register`.

### 5.4 Structured data & crawl infrastructure

- **FR-10:** JSON-LD on server-rendered template pages: `Article`-like schema (name, description, datePublished/dateModified, author) and `BreadcrumbList` (Home → Category → Template). Category pages carry `BreadcrumbList` and `ItemList`.
- **FR-11:** `robots.txt` (allow public paths; disallow `/admin`, `/api`, `/profile`, `/api-keys`, `/notifications`, and all authenticated routes) and `sitemap.xml` (public templates, categories, landing; ≤ 10,000 URLs, refreshed daily or on-demand; paginated sitemap index if needed).
- **FR-12:** `og:image` uses the site logo/brand card by default; template pages use a generated brand card (template name over brand background) — no per-template AI images in v1.

### 5.5 API

- **FR-13:** No new public API endpoints for the SPA. The server-side rendering path consumes internal services directly (or existing internal endpoints). The sitemap generator reads the database (public templates/categories) via a scheduled or on-demand job.

### 5.6 Admin surface

- **FR-14:** No admin UI in v1. (Sitemap generation and SEO health are operational concerns; the audit log records unpublish actions which remove URLs from the next sitemap refresh — existing behavior.)

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Server-rendered crawler pages respond < 500 ms (p95) and are cacheable (short TTL); the SPA landing for anonymous users loads < 2 s (LCP) with the featured grid streamed from the existing public API.
- **NFR-2 (Correctness):** Meta tags always describe the actually-visible public content; no private data ever appears in HTML served to crawlers; unpublished templates return 404/410 on the SSR path.
- **NFR-3 (Parity):** The crawler HTML and the SPA render the same public facts (name, description, category, tags, counts); discrepancies are bugs.
- **NFR-4 (i18n):** Landing and SSR pages render EN/ZH based on `?lang=` then Accept-Language; `hreflang` alternates emitted on SSR pages; the SPA head-manager sets `lang` on `<html>` per the i18n-completeness design.
- **NFR-5 (Accessibility):** The landing page is a normal accessible page (landmarks, heading hierarchy, focus order); SSR pages share the same markup semantics.
- **NFR-6 (Build):** The head-manager is framework-light (no new heavy dependency); SSR/dynamic-rendering integrates with the existing Go gateway (template rendering in Go) or a build-time prerender — architect's choice (see Open Questions).

## 7. Acceptance Criteria

- **AC-1:** Every SPA route sets a unique title and description; navigating updates them; admin/authenticated pages carry noindex.
- **AC-2:** Fetching a public template URL with a search-engine user agent returns complete HTML with title, description, OG/Twitter tags, JSON-LD, and visible template content.
- **AC-3:** Fetching a private/unpublished template URL as a crawler returns 404; no meta or body leaks its content.
- **AC-4:** Sharing a public template link in a social/chat client unfurls a card with name, description, and brand image.
- **AC-5:** Anonymous `/` shows the landing (hero, 6 featured templates, categories, how-it-works); logged-in `/` is unchanged.
- **AC-6:** Category hub pages render crawlable listings of public templates with pagination (≤ 50 links/page).
- **AC-7:** `/robots.txt` disallows authenticated/admin paths; `/sitemap.xml` lists public templates, categories, and the landing; an unpublished template disappears from the sitemap after the next refresh.
- **AC-8:** SSR pages emit `hreflang` alternates for EN/ZH and respect `?lang=`.
- **AC-9:** The landing renders correctly in EN and ZH, light and dark themes, desktop and mobile widths.
- **AC-10:** Lighthouse SEO category ≥ 95 on the landing and a sample template page (crawler view).
- **AC-11:** All new copy is EN/ZH translated via i18next (`landing.*`, `seo.*` namespaces).
- **AC-12:** No monetization content anywhere (no pricing sections, no plan comparisons, no upgrade CTAs).

## 8. Out of Scope

- Blog/content-marketing system.
- A/B testing of landing variants.
- Per-locale URL paths (`/zh/...`) — v1 uses `?lang=` + hreflang (path-based locales are a future i18n decision).
- AI-generated per-template preview images.
- Analytics/SEO dashboards in the admin console.
- Link building, ads, or any growth marketing operations.
- Monetization of any kind (pricing pages, plan tiers, upgrade flows) — permanently out of scope for this product.

## 9. Dependencies & Assumptions

- **D-1:** Public template browsing for anonymous users exists (or is trivially enabled) — the landing's "Browse prompts" and featured grid rely on public template reads.
- **D-2:** The Go gateway can render HTML templates (standard `html/template`) for the SSR path without new runtime dependencies.
- **D-3:** The i18n-completeness design's `<html lang>` synchronization applies; this feature adds `hreflang` on SSR pages.
- **D-4:** Template categories and tags are stable identifiers usable in hub URLs (`/categories/{slug}` style or existing routes).
- **D-5:** The existing like/favorite counts and template metadata are available to the SSR layer via internal service calls.
- **D-6:** A scheduled job mechanism (or on-demand generation with caching) exists for sitemap refresh; unpublish/audit actions already exist.

## 10. Open Questions for Architect

1. **Dynamic rendering vs prerender service vs Go SSR:** Recommendation: Go `html/template` SSR in the gateway for crawler user agents (no new service, full control, cacheable). Alternatives: a prerender.io-style service (new dependency) or build-time SSG (stale for per-template pages). Which fits the deployment best?
2. **Crawler detection:** User-agent sniffing (Googlebot/Bingbot list) vs a separate `?_escaped_fragment_=` convention vs serving SSR to everyone for public pages? Recommendation: UA sniffing with a maintained list, same-URL (no cloaking risk since content is identical).
3. **Should public template pages SSR for ALL first hits** (users too) with hydration, making the SPA a progressive enhancement? Bigger change; better Core Web Vitals; recommendation: crawler-only in v1.
4. **Sitemap generation:** scheduled job (cron in the Go server? external?) vs on-demand with cache TTL; where does it live (gateway route `/sitemap.xml` reading DB directly)?
5. **Category hub URLs:** new clean routes (`/c/{category-slug}`) vs reusing existing home filter query params (query params are weak for SEO) — recommendation: new clean routes for categories and tags.
6. **og:image generation:** static brand card vs per-template dynamic image (template name rendered over brand background at request time, cached) — recommendation: static brand card in v1, dynamic later.
7. **Landing featured templates:** curated (admin-picked, needs a tiny admin mechanism) vs automatic (top by likes, weekly window) — recommendation: automatic top-by-likes in v1, no admin UI.
