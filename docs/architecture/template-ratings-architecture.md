# Template Ratings — Architecture Design

| | |
|---|---|
| **Feature ID** | template-ratings |
| **Version** | 1.0 |
| **Status** | Approved for implementation |
| **Author** | Architect Agent |
| **Upstream** | [Requirements](../design/template-ratings-requirements.md), [UI/UX Design](../design/template-ratings-ui-ux-design.md) |
| **Downstream** | [Detailed Design](template-ratings-detailed-design.md) |

## 1. Overview

Template ratings add the missing *quality* signal: marketplace-style 1–5 star
ratings, one per user per template (upsert), with a transactionally maintained
aggregate (`rating_sum` + `rating_count`; the average is always computed,
never stored), a distribution histogram on the detail page, a compact star row
on cards, and a `top_rated` sort value for the search-filtering sort
framework. Ratings are deliberately separate from likes (appreciation) and
favorites (bookmarking); all three coexist.

The design reuses established patterns: the `ToggleLike` transactional
counter-maintenance shape, the Redis fixed-window service-level rate limit,
and the `/api/v1/templates/{id}/{subresource}` gateway branch style.

### 1.1 Resolved Open Questions

| # | Question (from requirements §10) | Decision |
|---|---|---|
| 1 | Aggregate storage | **`rating_sum` (BIGINT) + `rating_count` (INT) with computed average.** Integer math, no float drift; the average is derived at read time (`rating_sum / rating_count`) and never persisted. |
| 2 | `top_rated` indexing | **Option (a): accept a sort over the filtered set, plus a composite index `templates(rating_count, rating_sum)`.** No generated column, no materialized leaderboard in v1; revisit if list latency degrades. |
| 3 | Distribution query | **`GROUP BY rating` on `template_ratings` per detail-page view.** Detail views are low-traffic relative to lists; five denormalized buckets are not worth the write-path cost. |
| 4 | Service placement | **Extend `PromptService`** — ratings are template sub-resources exactly like likes/favorites; the repository methods join `TemplateRepository`. No new service. |

### 1.2 Non-Goals

- Written reviews (the comments feature owns text feedback).
- Per-version ratings, reputation weighting, verified-use gating.
- Rating notifications (anonymous signal; deliberately silent in v1).
- Anonymous/guest ratings; admin rating moderation or deletion endpoints.
- Rating export/import.
- **Monetization of any kind** — payments, pricing, purchases, earnings,
  sales, or paid-ranking surfaces are out of scope and excluded from this
  architecture.

## 2. System Context

```mermaid
flowchart LR
    subgraph Client
        FE[React Frontend<br/>RatingWidget / StarRating / Histogram]
    end
    subgraph Gateway
        GW[REST Gateway :8080<br/>/api/v1/templates/... /rating]
    end
    subgraph Backend
        PS[PromptService<br/>RateTemplate / DeleteRating]
        TR[TemplateRepository<br/>upsert + aggregate tx]
        AS[AdminService<br/>read-only aggregates]
    end
    subgraph Stores
        PG[(PostgreSQL)]
        RD[(Redis<br/>rate-limit counter)]
    end
    FE -->|POST/DELETE /templates/{id}/rating| GW --> PS
    FE -->|GET /templates + sort=top_rated| GW --> PS
    FE -->|GET /api/v1/admin/templates/{id}| GW --> AS
    PS --> TR --> PG
    PS -->|rating_rate_min:{user}| RD
```

## 3. Components Involved

| Component | Change |
|---|---|
| `backend/api/proto/v1/prompt.proto` | `RateTemplate`/`DeleteRating` RPCs on `PromptService`; `Template` gains `rating_avg`/`rating_count`/`my_rating`; `GetTemplateResponse` gains `rating_distribution`. |
| `backend/internal/repository/template_repository.go` | `RateTemplate` (upsert + aggregate tx), `DeleteRating`, `GetRatingDistribution`; `my_rating` LEFT JOIN in `Get`/`List`; `top_rated` sort branch. |
| `backend/internal/service/prompt.go` | RPC implementations: validation, owner/ban/visibility checks, Redis rate limit. |
| `backend/internal/service/admin.go` + `admin.proto` | `AdminTemplateSummary` gains read-only `rating_avg`/`rating_count`. |
| `backend/cmd/server/main.go` | `/rating` branch in the `/api/v1/templates/` handler (POST + DELETE). |
| `frontend/src/components/{StarRating,RatingWidget,RatingHistogram}.js` | **New.** |
| `frontend/src/components/PromptCard.js`, `pages/TemplateDetails.js`, admin template detail | Star row, rating block, read-only admin row. |
| `deployment/init.sql` (+ chart copy) | `template_ratings` table, `templates.rating_sum`/`rating_count` ALTERs, composite index. |

## 4. API Design

### 4.1 Rating Actions

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/api/v1/templates/{id}/rating` | Required | Body `{rating: 1..5}`. Upserts the caller's rating. Returns `{rating_avg, rating_count, my_rating}`. |
| `DELETE` | `/api/v1/templates/{id}/rating` | Required | Removes the caller's rating. Idempotent — deleting a non-existent rating returns `200` with current aggregates and `my_rating: 0`. |

Rules:

- **Owner self-rating** → `403 PERMISSION_DENIED` ("cannot rate your own
  template").
- **Banned caller** → `403`.
- **Private template, non-owner caller** → `404 NOT_FOUND` (the template's
  existence is not revealed; consistent with `GetTemplate` visibility
  semantics). The owner themselves is blocked by the self-rating rule, so
  effectively nobody rates private templates.
- **Invalid rating** (0, 6, non-integer, 3.5) → `400 INVALID_ARGUMENT`.
- **Rate limit**: 20 rating changes per minute per account → `429
  RESOURCE_EXHAUSTED` with `Retry-After`. Configurable
  (`RATING_RATE_PER_MINUTE`).

### 4.2 Reads (no new endpoints)

- Template **list** items gain `rating_avg` (float, `0` when `rating_count = 0`
  — the count signals "no ratings"), `rating_count`, and `my_rating`
  (`0` = unrated; ratings are 1–5 so 0 is unambiguous).
- Template **detail** additionally gains `rating_distribution` (five counts,
  index 0 → 1★ … index 4 → 5★) on `GetTemplateResponse`.
- No public per-rater data exists anywhere; `my_rating` is returned only to
  its owner (anonymous callers get `0`).

### 4.3 Sort

The search-filtering sort framework gains `top_rated`: templates with
`rating_count >= threshold` (default 5, configurable) sort first by average
DESC, then `rating_count` DESC, then `created_at` DESC; templates below the
threshold follow, ordered by `rating_count` DESC then `created_at` DESC. If
search-filtering has not shipped, this sort value waits — the rating data and
display are independently valuable (requirements D-2).

### 4.4 Admin (read-only)

`GET /api/v1/admin/templates/{id}` (and the admin template list) gains
`rating_avg` and `rating_count` as read-only context. No admin rating
endpoints exist; attempts hit the gateway's 404/405 handling.

### 4.5 Error Codes

| HTTP | gRPC | Condition |
|---|---|---|
| 400 | `INVALID_ARGUMENT` | Rating outside 1–5, non-integer. |
| 401 | `UNAUTHENTICATED` | Missing/invalid token. |
| 403 | `PERMISSION_DENIED` | Owner self-rating; banned caller. |
| 404 | `NOT_FOUND` | Template not found or private-to-another. |
| 429 | `RESOURCE_EXHAUSTED` | Rate limit exceeded (`Retry-After`). |

## 5. Data Model

### 5.1 New Table: `template_ratings`

```sql
CREATE TABLE IF NOT EXISTS template_ratings (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    rating SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, template_id)
);
CREATE INDEX IF NOT EXISTS idx_template_ratings_template ON template_ratings(template_id);
```

### 5.2 `templates` ALTERs (idempotent, both init.sql copies)

```sql
ALTER TABLE templates ADD COLUMN IF NOT EXISTS rating_sum BIGINT NOT NULL DEFAULT 0;
ALTER TABLE templates ADD COLUMN IF NOT EXISTS rating_count INT NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_templates_top_rated ON templates(rating_count, rating_sum);
```

### 5.3 Aggregate Maintenance

Every rating change runs in one transaction (the `ToggleLike` shape):
read the caller's current row, apply the delta to `rating_sum`/`rating_count`
(`GREATEST(...)` floors guard against drift), and return the fresh
aggregates. Template deletion cascades ratings away with the row. User
deletion cascades their rating rows; any future user-deletion endpoint must
recompute affected templates' aggregates in the same transaction (SQL in the
detailed design). Banning does **not** delete ratings (FR-14).

## 6. Auth & Authorization

- Writes require session/API-key auth; user id from context, never the body.
- Server-side validation: integer 1–5, template existence, owner check,
  ban check, private-visibility check — all before the write.
- Rate limiting in `PromptService` via Redis fixed window
  (`rating_rate_min:{user_id}`), fail-open on Redis errors, only successful
  changes consume quota.
- `PromptService` gains a `RedisStore` parameter (nil-safe — rate limiting
  disabled when nil). **Coordination note**: the template-comments design
  adds the same parameter; whichever lands first introduces it, the other
  reuses it.

## 7. Frontend

- **StarRating** (display): read-only stars + value + count; card size omits
  the row entirely at `rating_count = 0` (never "0.0").
- **RatingWidget** (interactive, detail page): hover preview, click commits,
  keyboard slider pattern (single tab stop, ←/→ adjust, Enter/Space commit,
  Esc cancels), `my_rating` state with "Clear rating" action, optimistic fill
  with revert; owner view shows a non-interactive hint; anonymous click
  routes to `/login`.
- **RatingHistogram**: five rows (star, bar, percentage text); semantic list,
  bars decorative; hidden at zero ratings.
- **PromptCard**: compact "★ 4.6 (128)" row below title/meta.
- **Sort dropdown** (search-filtering component): `top_rated` option with
  threshold tooltip.
- **Admin template detail**: read-only "Rating: 4.62 (128)" row.
- **i18n**: `ratings.*` namespace + `sort.top_rated`; EN/ZH parity; star
  hints localized (1 Poor … 5 Excellent / 很差…很好); averages always western
  digits.

## 8. Configuration

| Variable | Default | Description |
|---|---|---|
| `RATING_RATE_PER_MINUTE` | `20` | Rating changes per minute per account. |
| `TOP_RATED_MIN_RATINGS` | `5` | Minimum `rating_count` for the `top_rated` sort threshold. |

Both added to `deployment/docker-compose.yml` and the Helm chart
(`values.yaml` + `templates/deployment-backend.yaml`), mirroring
`NOTIFICATION_*`.

## 9. Security Considerations

- **Privacy**: no endpoint reveals who rated what; only a rater sees their own
  `my_rating`.
- **Integrity**: one row per (user, template) enforced by the PK; aggregates
  maintained transactionally; upsert semantics make client retries safe.
- **Abuse**: per-account rate limit; owner self-rating blocked; banned users
  blocked; server-side validation of the 1–5 domain.
- **Injection**: parameterized SQL only.

## 10. Deployment Notes

- DDL is additive and idempotent; both init.sql copies updated in the same
  commit.
- No new services or infrastructure; one new Redis counter key.
- `backend/server` is a tracked build artifact — never commit changes to it.

## 11. Test Strategy

- **FVT** (`tests/fvt.py`): `test_template_ratings` — rate/re-rate/delete with
  aggregate assertions; owner 403; banned 403; invalid values 400; private
  template 404 for non-owner; rate limit 429; `my_rating` in list/detail;
  distribution correctness; admin read-only fields; idempotent delete.
- **Unit**: repository delta math (insert/update/delete paths, floors),
  service validation matrix, rate-limit window behavior.
- **E2E** (test-agent): widget interaction (hover/click/keyboard), histogram
  rendering, card row omission at zero, sort option, i18n parity.
