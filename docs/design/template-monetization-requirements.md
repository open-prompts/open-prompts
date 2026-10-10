# Template Monetization — Requirements Analysis

- **Feature ID:** template-monetization
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2025-06-25

## 1. Competitor Research Summary

| Platform | Monetization approach | Key takeaways |
|---|---|---|
| **PromptBase** | Full marketplace: sellers list prompts with fixed prices ($0.99–$9.99 typical), buyers purchase → get the prompt content; PromptBase takes a commission (~30% historically); seller dashboard with earnings; featured placement | Fixed-price listings are the proven core; content is gated until purchase; seller earnings dashboard is expected; curation/featuring drives sales |
| **FlowGPT** | Free + creator monetization experiments (tips, paid creator tiers); most content free; monetization secondary to community growth | Free-first platforms struggle to charge per-prompt; tipping rewards creators without gating content |
| **Snack Prompt** | Free community; monetization nascent (creator support, badges) | Engagement-based rewards (badges/tips) suit platforms without payment rails |
| **AIPRM** | Subscription SaaS: free tier + paid plans unlock advanced prompts/features; prompts themselves are lead-gen for the subscription | Subscription works when the platform itself is the product; per-prompt sales work when creators are the product |
| **PromptHero** | Free browsing; monetization via courses/ads, not per-prompt sales | Per-prompt sales need trust signals (reviews, previews) and a payout mechanism |
| **Generic (Gumroad / Etsy / stock-photo marketplaces)** | Fixed-price digital listings: preview → purchase → instant delivery; platform commission; seller dashboards | The universal pattern: gated preview, one-click buy, instant access, seller earnings view |

**Synthesis:** Two viable models exist: (a) per-listing fixed-price marketplace (PromptBase/Gumroad) and (b) platform subscription (AIPRM). For Open Prompts — a community prompt square with individual creators — the per-listing marketplace model fits the existing product shape (templates are discrete, ownable objects with versions) and requires no subscription billing infrastructure. v1 scope: creators set a fixed price on their public templates; buyers purchase with account balance (wallet); content is gated until purchase. Real-money payouts and external payment providers (Stripe etc.) are explicitly out of scope — v1 uses a simulated wallet so the full product surface (listing, gating, purchase, library, earnings) ships and is testable end-to-end, with a clean seam to attach a real payment provider later.

## 2. Problem Statement

Open Prompts has no monetization surface at all: no prices, no purchases, no earnings. Creators who publish high-quality templates have no way to be compensated, and the platform cannot reward or surface premium content:

1. **No creator compensation.** Likes and favorites are the only feedback; there is no economic incentive to publish polished, production-grade templates.
2. **No premium tier of content.** Every public template is free; buyers cannot pay for higher-quality, maintained, versioned prompts even if they want to.
3. **No purchase flow.** There is no checkout, no owned-library view ("prompts I bought"), and no license/entitlement model to gate content.
4. **No earnings visibility.** Creators cannot see sales, revenue, or which templates earn.
5. **No marketplace discovery dimension.** Browsing cannot filter "paid vs free" or surface premium content.

## 3. Target Users

- **Sellers (primary):** creators who publish public templates and want to charge a fixed price for access.
- **Buyers:** users who find a premium template worth paying for from their wallet balance and want instant, permanent access.
- **Browsers:** users who need clear price signals (free vs paid, price) while browsing, and a gated preview before deciding.
- **Admins:** oversee marketplace health (listings, pricing abuse, refunds) via the admin console — minimal v1 surface (see FR split).

## 4. User Stories

### 4.1 Selling (creators)

- **US-M1:** As a creator, I want to set a price on my public template, so that buyers must pay to access its content.
- **US-M2:** As a creator, I want to change or remove the price later, so that I can run promotions or make content free.
- **US-M3:** As a creator, I want to see my earnings per template and in total, so that I know what sells.
- **US-M4:** As a creator, I want to see a purchase feed (who bought, when), so that I understand my audience.
- **US-M5:** As a creator, I want buyers who purchased before a price change to keep access, so that purchases are permanent.

### 4.2 Buying

- **US-M6:** As a buyer, I want to top up my wallet balance, so that I can purchase templates.
- **US-M7:** As a buyer, I want to see the price and a gated preview before purchasing, so that I know what I'm buying.
- **US-M8:** As a buyer, I want one-click purchase, so that access is instant.
- **US-M9:** As a buyer, I want my purchased templates in one library view, so that I can return to them anytime.
- **US-M10:** As a buyer, I want to use a purchased template like any owned template (fill variables, use, fork), so that purchase unlocks full utility.

### 4.3 Browsing & system

- **US-M11:** As a browser, I want free/paid badges and prices on cards and lists, so that I can filter my attention.
- **US-M12:** As the system, I must never expose paid content to non-purchasers (in list endpoints, detail endpoints, aliases, or API keys), so that gating is airtight.
- **US-M13:** As the system, I want purchases to be idempotent and atomic, so that double-clicks and retries never double-charge or grant partial access.

## 5. Functional Requirements

### 5.1 Pricing (creators)

- **FR-1:** Template owners shall set a price on their own templates: `price_cents` (integer, 0 = free, 99–9900 allowed range i.e. $0.99–$99.00), currency fixed to USD in v1.
- **FR-2:** Only public templates may be priced; setting a price on a private template returns an error (a private template has nothing to gate).
- **FR-3:** Owners shall change the price at any time; existing purchases keep access regardless of later price changes or the template becoming free (FR-4).
- **FR-4:** Owners shall set the price back to 0 (free) at any time; the template becomes freely accessible to everyone.
- **FR-5:** Price changes take effect immediately; no approval workflow in v1.

### 5.2 Wallet & top-up

- **FR-6:** Every user has a wallet: `balance_cents` (integer, ≥ 0), single currency (USD), created lazily on first access.
- **FR-7:** Users shall top up their wallet via a simulated top-up flow: choose a preset amount ($5 / $10 / $20) or enter a custom amount ($1–$500), confirm, and receive balance instantly. v1 records a `wallet_transactions` row of type `topup` (source `simulated`); no real payment provider is integrated.
- **FR-8:** Wallet balance shall be visible in the header (logged-in users) and on a wallet page with a transaction history (topups, purchases, earnings, refunds) paginated.
- **FR-9:** Wallet transactions are append-only; balances are derived from the transaction ledger (sum of credits minus debits) and cached on the wallet row for fast reads.

### 5.3 Purchase

- **FR-10:** A logged-in user shall purchase a priced template they don't own: the system checks balance ≥ price, debits the buyer, credits the seller (minus platform commission, FR-14), creates a `purchases` entitlement row, and records two `wallet_transactions` (buyer debit, seller credit) — atomically in one DB transaction.
- **FR-11:** Purchases are idempotent: purchasing an already-owned template is a no-op returning success (no double charge); concurrent duplicate purchases are prevented by a unique constraint on (buyer_id, template_id).
- **FR-12:** Owners automatically "own" their templates (no self-purchase); forked copies are new templates with no price inherited (the forker sets their own price if desired).
- **FR-13:** Purchase grants permanent access: view full content, use with variables, fork, add to collections, API-key instantiation — identical to owning, except the buyer cannot edit the original or see its version history beyond the latest published version (v1 simplification; see D-6).

### 5.4 Earnings & commission

- **FR-14:** The platform takes a configurable commission (default 20%, env-configurable `MARKETPLACE_COMMISSION_BPS`, e.g. 2000) on each purchase; the seller receives the remainder. Commission is recorded on the purchase row.
- **FR-15:** Sellers shall view earnings: total earned, earnings per template (units sold, gross, net), and a recent sales feed — on a dedicated "Earnings" tab in their workspace.
- **FR-16:** Earnings credit to the seller's wallet balance and are spendable on other templates (v1: no payout/withdrawal — D-3).

### 5.5 Gating (content protection)

- **FR-17:** For a priced template, non-purchasers (including anonymous users) shall see: title, description, tags, category, language, like/favorite counts, and a content preview limited to the first 200 characters of the latest version content (server-truncated, with `preview_truncated: true`).
- **FR-18:** The full content of a priced template shall be returned only to: the owner, purchasers, and admins. This applies to ALL read paths: `GetTemplate`, `ListTemplates` (the embedded `latest_version.content` must be redacted for non-purchasers), `GetPromptByAlias` (API-key path), and version history (non-purchasers see only the latest version's gated preview, not historical versions).
- **FR-19:** Fork of a priced template is allowed only for owner/purchasers (forking requires seeing content); the fork is a fresh private template with price 0.
- **FR-19a (admin split):** admins may view full content of any template for moderation (admin APIs are defined in the admin-user-management feature; this feature only requires that admin role checks exist — D-7).

### 5.6 Discovery & UI surfaces

- **FR-20:** PromptCard and list rows shall show a price badge (e.g. "$4.99") for priced templates and a "Free" badge for free ones; the home grid filter bar shall gain a "Pricing" filter (All / Free / Paid).
- **FR-21:** The template detail page shall show, for non-purchasers of a priced template: the gated preview with a blurred/locked overlay, the price, a "Purchase" button (disabled with reason if balance insufficient), and a link to top up.
- **FR-22:** Purchasers (and the owner) see the normal detail page (full content, variables form, use/fork actions).
- **FR-23:** A "Purchased" library view at `/dashboard/purchased` lists templates the user bought (card grid, purchase date, price paid), linking to the detail page.

### 5.7 API

- **FR-24:** New RPCs: `SetTemplatePrice`, `GetWallet`, `TopUpWallet`, `ListWalletTransactions`, `PurchaseTemplate`, `ListPurchasedTemplates`, `GetEarnings` (seller totals + per-template + recent sales). REST endpoints: `PATCH /api/v1/templates/{id}/price`, `GET /api/v1/wallet`, `POST /api/v1/wallet/topup`, `GET /api/v1/wallet/transactions`, `POST /api/v1/templates/{id}/purchase`, `GET /api/v1/templates/purchased` (or `/api/v1/purchases`), `GET /api/v1/earnings`.
- **FR-25:** `GetTemplate`/`ListTemplates` responses gain `price_cents`, `is_purchased` (for the caller), and `preview_truncated` fields; `latest_version.content` is redacted per FR-18. Existing clients that ignore new fields are unaffected.
- **FR-26:** All money mutations (topup, purchase) are atomic DB transactions with row-level locking on wallet rows (SELECT ... FOR UPDATE) to prevent lost updates under concurrency.
- **FR-27:** Purchase refunds (admin-initiated) reverse the ledger entries and revoke entitlement — admin surface deferred to the admin feature; the schema supports it (D-4).

### 5.8 Regular-user vs admin split

- **FR-28:** Regular users: pricing, wallet, purchase, purchased library, earnings (this feature). Admins: marketplace oversight (view any listing's full content per FR-19, refund per FR-27, force-freeze a listing) — the admin UI for these is part of the admin-user-management feature's console; this feature exposes only the underlying entitlement/ledger design that makes refunds possible. No admin pages are built in this feature.

## 6. Non-Functional Requirements

- **NFR-1 (Performance):** Purchase p95 < 300ms; wallet/earnings pages p95 < 500ms; card grids with price badges add no extra round-trip (price rides on existing list responses).
- **NFR-2 (Integrity):** All balance mutations go through the append-only ledger with DB transactions; balance can always be recomputed from the ledger and reconciled (a reconciliation job is a follow-up, not v1).
- **NFR-3 (Security):** Gating is enforced server-side on every read path (FR-18); the client never receives full content for non-purchasers; no client-side-only gating.
- **NFR-4 (Compatibility):** Additive schema/API only; existing endpoints gain optional fields; existing clients unaffected. Free templates behave exactly as today (price 0 = no gating, no badges beyond "Free").
- **NFR-5 (Accessibility):** Purchase/top-up flows, price badges, and the locked overlay are keyboard-operable and screen-reader friendly (see UI/UX doc §10).
- **NFR-6 (i18n):** All new strings EN/ZH in sync; prices formatted per locale (e.g., $4.99 / 4.99 美元); UI chrome fully localized.
- **NFR-7 (Scale):** `purchases` indexed on (buyer_id, purchased_at) and (template_id); `wallet_transactions` indexed on (wallet_id, created_at); ledger grows append-only (partitioning is a future concern).
- **NFR-8 (Auditability):** Every money movement has a ledger row with type, amount, counterparty, and reference (purchase id / topup id); admin refund actions are traceable.

## 7. Acceptance Criteria

- **AC-1:** A creator sets $4.99 on a public template; the card shows a "$4.99" badge; free templates show "Free".
- **AC-2:** An anonymous user opens the priced template's detail page and sees only the 200-char preview with a locked overlay; the API response contains no full content on any path (detail, list, alias).
- **AC-3:** A logged-in buyer with $10 balance purchases the $4.99 template in one click; balance becomes $5.01; the detail page now shows full content; the template appears in `/dashboard/purchased`.
- **AC-4:** The seller's balance increases by $3.99 (20% commission); their Earnings tab shows 1 sale, gross $4.99, net $3.99.
- **AC-5:** Purchasing the same template again is a no-op (no charge, success response).
- **AC-6:** The creator changes the price to $9.99 then to free; the earlier buyer retains access; when free, everyone can view the content.
- **AC-7:** A buyer with insufficient balance sees a disabled Purchase button with the shortfall and a top-up link; topping up $5 then succeeds.
- **AC-8:** The home grid "Pricing" filter (All/Free/Paid) returns only matching templates.
- **AC-9:** A non-purchaser's fork attempt on a priced template is refused; a purchaser's fork creates a private, free copy.
- **AC-10:** Wallet page lists transactions (topup, purchase debit, sale credit) newest-first, paginated, with running balance available in the header.
- **AC-11:** Concurrent purchases of the same template by the same user result in exactly one charge (unique constraint + idempotency).
- **AC-12:** All new UI strings exist in both `en` and `zh` translation files with identical key sets; prices render correctly in both locales.
- **AC-13:** Existing free-template flows (browse, like, favorite, use, fork, alias, API key) are unchanged (no regression).

## 8. Out of Scope (v1)

- Real payment providers (Stripe/Alipay/WeChat Pay) and real-money payouts/withdrawals — the wallet is simulated (topup source `simulated`); the ledger design is the seam for a future provider.
- Subscription tiers / platform plans (AIPRM model) — revisit after per-listing sales prove out.
- Pay-what-you-want / tipping (FlowGPT-style).
- Purchase invoices/receipts (PDF) and tax handling.
- Reviews/ratings of purchased templates (trust surface for a future iteration).
- Featured/boosted listings and marketplace curation pages.
- Refund UI in admin console (schema supports; UI deferred with admin feature).
- Multi-currency support (USD only).
- Seller analytics beyond units/gross/net (funnels, conversion).

## 9. Dependencies & Assumptions

- **D-1:** New tables: `wallets` (id UUID PK, user_id UNIQUE, balance_cents INT default 0, currency 'USD', created_at, updated_at), `wallet_transactions` (id UUID PK, wallet_id, type enum-ish TEXT ['topup','purchase_debit','sale_credit','refund'], amount_cents INT signed, balance_after INT, counterparty_user_id NULL, reference_id NULL (purchase/topup id), source TEXT NULL, created_at), `purchases` (id UUID PK, buyer_id, template_id, price_cents, commission_bps, seller_net_cents, status 'active'|'refunded', created_at; UNIQUE (buyer_id, template_id) where status='active' — partial unique index), `template_prices` are NOT used: price lives on `templates` as `price_cents INT default 0` (single source of truth, one column, existing queries unaffected).
- **D-2:** Wallet creation is lazy (first GET /wallet or first purchase/topup creates it); no migration backfill needed.
- **D-3:** Earnings are spendable balance (no withdrawal in v1); the `wallet_transactions` ledger with type `refund` and reference ids makes future payouts/refunds reversible.
- **D-4:** Refunds (admin) reverse ledger entries with compensating rows and set purchase status 'refunded' — entitlement revoked; buyer keeps no access. Deferred to admin feature UI.
- **D-5:** Preview truncation (FR-17) is server-side at 200 chars on the latest version content; `preview_truncated: true` signals the client to render the lock overlay.
- **D-6:** Purchasers see only the latest version of a priced template (not full history) in v1 — simplifies gating logic; version history for purchasers is a follow-up.
- **D-7:** Admin role check exists per the admin-user-management feature (roles on users); this feature's gating checks `role == 'admin'` where needed (FR-19). If the admin feature lands later, gating treats everyone as non-admin until then (graceful).
- **D-8:** Commission default 20% via `MARKETPLACE_COMMISSION_BPS` env (2000 bps); recorded per-purchase so historical sales don't shift when the env changes.
- **D-9:** The home grid "Pricing" filter (FR-20) maps to a new `pricing` filter param on ListTemplates (`all`|`free`|`paid`) — consistent with the template-search-filtering feature's param pattern.
- **D-10:** `/dashboard/purchased` extends the user-dashboard feature's workspace; if that feature hasn't landed, the route stands alone (it only needs ListPurchasedTemplates).

## 10. Open Questions for Architect

*(None blocking. Flagged decisions: D-1 (price column on templates vs separate table), D-6 (purchasers see latest version only), D-8 (commission env-configurable), FR-24 service placement (PromptService vs new MarketplaceService), and whether `GET /api/v1/templates/purchased` vs `/api/v1/purchases` is preferred for the library endpoint.)*
