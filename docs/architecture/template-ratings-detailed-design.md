# Template Ratings — Detailed Design

| | |
|---|---|
| **Feature ID** | template-ratings |
| **Version** | 1.0 |
| **Status** | Approved for implementation |
| **Author** | Architect Agent |
| **Upstream** | [Architecture](template-ratings-architecture.md) |

## 1. Proto Contract

Additions to `backend/api/proto/v1/prompt.proto`. `Template` fields 17–19 are
coordinated with the comments/follow-creators designs (17 = `comment_count`,
18/19 = `owner_display_name`/`owner_avatar`); ratings take 20–22.

```protobuf
// PromptService — append to the existing service block (line 33)
service PromptService {
    // ... existing RPCs ...
    rpc RateTemplate(RateTemplateRequest) returns (RateTemplateResponse);
    rpc DeleteRating(DeleteRatingRequest) returns (RateTemplateResponse);
}

message RateTemplateRequest {
    string template_id = 1;
    int32 rating = 2; // 1..5
}

message DeleteRatingRequest {
    string template_id = 1;
}

message RateTemplateResponse {
    double rating_avg = 1; // 0 when rating_count = 0
    int32 rating_count = 2;
    int32 my_rating = 3;   // 0 = unrated
}

message Template {
    // ... existing fields 1-16; 17 comment_count, 18/19 owner fields ...
    double rating_avg = 20;
    int32 rating_count = 21;
    int32 my_rating = 22;  // 0 = unrated; only meaningful for authed callers
}

message GetTemplateResponse {
    Template template = 1;
    TemplateVersion latest_version = 2;
    // owner = 3 (follow-creators design)
    repeated int32 rating_distribution = 4; // index 0 -> 1★ ... index 4 -> 5★
}
```

`admin.proto` — `AdminTemplateSummary` (line 165) gains:

```protobuf
message AdminTemplateSummary {
    // ... existing fields 1-12 ...
    double rating_avg = 13;
    int32 rating_count = 14;
}
```

Regenerate:

```bash
cd backend && protoc --go_out=. --go-grpc_out=. api/proto/v1/prompt.proto api/proto/v1/admin.proto
```

## 2. Database DDL

Append to **both** `deployment/init.sql` and
`deployment/chart/open-prompts/init.sql`:

```sql
-- ===== template-ratings =====
CREATE TABLE IF NOT EXISTS template_ratings (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    rating SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, template_id)
);

CREATE INDEX IF NOT EXISTS idx_template_ratings_template
    ON template_ratings(template_id);

ALTER TABLE templates ADD COLUMN IF NOT EXISTS rating_sum BIGINT NOT NULL DEFAULT 0;
ALTER TABLE templates ADD COLUMN IF NOT EXISTS rating_count INT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_templates_top_rated
    ON templates(rating_count, rating_sum);
```

## 3. Backend Layers

### 3.1 `models.Template`

Add fields:

```go
RatingSum   int64   // denormalized; not exposed in proto directly
RatingCount int32
RatingAvg   float64 // computed at mapping time: sum/count, 0 when count=0
MyRating    int32   // 0 = unrated; filled per-viewer by repository joins
```

### 3.2 `TemplateRepository` additions

```go
RateTemplate(ctx context.Context, userID, templateID string, rating int32) (sum int64, count int32, err error)
DeleteRating(ctx context.Context, userID, templateID string) (sum int64, count int32, err error)
GetRatingDistribution(ctx context.Context, templateID string) ([5]int32, error)
```

`RateTemplate` transaction (the `ToggleLike` shape — read, delta, write):

```sql
BEGIN;
-- 1) upsert the caller's row; capture the previous value
INSERT INTO template_ratings (user_id, template_id, rating)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, template_id)
DO UPDATE SET rating = EXCLUDED.rating, updated_at = now()
RETURNING rating;                       -- new value
-- previous value read first (SELECT rating ... FOR UPDATE) or via
-- a CTE; the delta is (new - previous) applied to rating_sum.

-- 2) apply the delta transactionally
UPDATE templates
SET rating_sum = GREATEST(rating_sum + $delta, 0),
    rating_count = rating_count + $count_delta  -- +1 on insert, 0 on update
WHERE id = $2;

-- 3) return fresh aggregates
SELECT rating_sum, rating_count FROM templates WHERE id = $2;
COMMIT;
```

Implementation note: the cleanest form is a single CTE —

```sql
WITH prev AS (
    SELECT rating FROM template_ratings
    WHERE user_id = $1 AND template_id = $2 FOR UPDATE
), upsert AS (
    INSERT INTO template_ratings (user_id, template_id, rating)
    VALUES ($1, $2, $3)
    ON CONFLICT (user_id, template_id)
    DO UPDATE SET rating = EXCLUDED.rating, updated_at = now()
    RETURNING rating, (xmax = 0) AS inserted
)
UPDATE templates t
SET rating_sum = GREATEST(t.rating_sum + $3 - COALESCE((SELECT rating FROM prev), 0), 0),
    rating_count = t.rating_count + CASE WHEN (SELECT inserted FROM upsert) THEN 1 ELSE 0 END
WHERE t.id = $2
RETURNING t.rating_sum, t.rating_count;
```

`DeleteRating` mirrors it (delete row → decrement sum by the removed value,
decrement count, floor at 0; missing row is a no-op returning current
aggregates).

`GetRatingDistribution`:

```sql
SELECT rating, COUNT(*) FROM template_ratings
WHERE template_id = $1 GROUP BY rating;
```

**Read-path joins** — `Get` and `List` gain (alongside the existing
`is_liked`/`is_favorited` LEFT JOINs):

```sql
LEFT JOIN template_ratings mr ON mr.template_id = t.id AND mr.user_id = $viewer
-- selected: mr.rating AS my_rating (0 when NULL)
```

`rating_avg` is computed in the row scan (`sum/count`), never stored.

**`top_rated` sort branch** (in the sort whitelist, coordinated with the
search-filtering design):

```sql
ORDER BY
  (rating_count >= $threshold) DESC,          -- qualified first
  CASE WHEN rating_count >= $threshold
       THEN rating_sum::float / GREATEST(rating_count, 1) END DESC NULLS LAST,
  rating_count DESC,
  created_at DESC
```

### 3.3 `PromptService` (`backend/internal/service/prompt.go`)

`RateTemplate`:

1. Validate `rating` ∈ [1,5] → else `InvalidArgument`.
2. `GetUserIDFromContext` → `Unauthenticated` on failure.
3. Load the template (`TemplateRepo.Get`); missing or private-to-another →
   `NotFound`.
4. Owner check: `tpl.OwnerID == userID` → `PermissionDenied` ("cannot rate
   your own template").
5. Ban check via `UserRepo.GetByID` → `PermissionDenied` when banned.
6. Rate limit (below) — only successful changes consume quota.
7. `TemplateRepo.RateTemplate` → build `RateTemplateResponse{rating_avg,
   rating_count, my_rating}`.

`DeleteRating`: steps 2–3 (no owner/ban checks needed beyond auth and
visibility; the caller can only delete their own row), then
`TemplateRepo.DeleteRating` (idempotent).

**No notification hook** — ratings are anonymous by design (FR-8).

**Rate limiting** (Redis fixed window, fail-open):

```
key:    rating_rate_min:{user_id}
value:  "{count}:{window_expires_unix}"
window: 60s, limit RATING_RATE_PER_MINUTE (default 20)
```

`PromptService` gains a `RedisStore` field (nil-safe). **Coordination**: the
template-comments design adds the same field; whichever lands first introduces
it, the other reuses it — do not add two fields.

### 3.4 `AdminService` (`backend/internal/service/admin.go`)

`GetTemplate` and the admin template list mapping add `rating_avg` (computed
from `rating_sum`/`rating_count`) and `rating_count` to
`AdminTemplateSummary` fields 13/14. Read-only; no new RPCs.

### 3.5 Gateway (`backend/cmd/server/main.go`)

Add a `/rating` branch in the `/api/v1/templates/` handler, following the
existing `strings.HasSuffix(id, "/like")` pattern:

```go
if strings.HasSuffix(id, "/rating") {
    templateID := strings.TrimSuffix(id, "/rating")
    switch r.Method {
    case http.MethodPost:   // auth → RateTemplateRequest{TemplateId, Rating}
    case http.MethodDelete: // auth → DeleteRatingRequest{TemplateId}
    default: http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    }
}
```

Both methods require the Bearer-token auth dance (same as `/like`), with the
rating parsed from the JSON body on POST. `Retry-After` is set on 429
responses (the writeError mapping already handles `ResourceExhausted`; add
the header when the error code matches).

## 4. Frontend

### 4.1 New Files

| File | Purpose |
|---|---|
| `frontend/src/components/StarRating.js` + `.scss` | Read-only stars + value + count; `size` prop (sm for cards, lg for detail/admin); row omitted at `rating_count = 0` on cards. |
| `frontend/src/components/RatingWidget.js` + `.scss` | Interactive 5-star widget: hover preview, click commit, keyboard slider (single tab stop, ←/→, Enter/Space, Esc), `my_rating` state, Clear-rating action with lightweight confirm popover, optimistic fill + revert. |
| `frontend/src/components/RatingHistogram.js` + `.scss` | 5 rows (star label, bar, percentage text); semantic `<ul>`; bars `aria-hidden`; hidden at zero ratings. |

### 4.2 Modified Files

| File | Change |
|---|---|
| `frontend/src/components/PromptCard.js` | Compact `StarRating size="sm"` row below title/meta (above like/favorite row); omitted at zero ratings. |
| `frontend/src/pages/TemplateDetails.js` | Rating block with social stats (above comments): `RatingWidget` + average (2 decimals) + count + `RatingHistogram`; owner view shows non-interactive hint; anonymous click → `/login`. |
| Sort dropdown (search-filtering component) | `top_rated` option + threshold tooltip. |
| `frontend/src/pages/admin/TemplateDetailPage.js` | Read-only "Rating: 4.62 (128)" row in the stats section. |
| `frontend/src/services/api.js` | `rateTemplate(id, rating)`, `deleteRating(id)`. |

### 4.3 Redux

No new slice: rating state arrives with template detail/list payloads;
`my_rating` updates flow through the widget's local state (optimistic) and the
action responses (source of truth).

### 4.4 API Client

```js
rateTemplate(templateId, rating)   // POST   /templates/${templateId}/rating {rating}
deleteRating(templateId)           // DELETE /templates/${templateId}/rating
```

### 4.5 i18n Keys (EN/ZH parity)

```json
{
  "ratings": {
    "rate": "Rate this template",
    "ratings_count": "{{count}} ratings",
    "no_ratings": "No ratings yet",
    "be_first": "No ratings yet — be the first",
    "clear": "Clear rating",
    "clear_confirm": "Remove your rating?",
    "you_rated": "You rated {{count}}★",
    "owner_hint": "You can't rate your own template",
    "submitted": "Rated {{count}} stars",
    "error": "Could not save your rating",
    "hint_1": "Poor", "hint_2": "Fair", "hint_3": "Good",
    "hint_4": "Very good", "hint_5": "Excellent",
    "aria_label": "Rated {{avg}} out of 5 with {{count}} ratings"
  },
  "sort": {
    "top_rated": "Top rated",
    "top_rated_hint": "Average rating, minimum {{count}} ratings"
  }
}
```

ZH mirrors every key (`ratings.rate` = "为模板评分", `sort.top_rated` =
"评分最高", hints 很差/较差/一般/好/很好, etc.).

## 5. Error Matrix

| Scenario | HTTP | gRPC | Frontend handling |
|---|---|---|---|
| Rating 0/6/non-integer | 400 | `INVALID_ARGUMENT` | Field-level error; widget unchanged |
| Not authenticated | 401 | `UNAUTHENTICATED` | Redirect to `/login` |
| Owner self-rating | 403 | `PERMISSION_DENIED` | Widget shows owner hint (pre-checked) |
| Banned caller | 403 | `PERMISSION_DENIED` | Error toast; widget reverts |
| Template missing/private | 404 | `NOT_FOUND` | 404 page |
| Rate limited | 429 | `RESOURCE_EXHAUSTED` | Toast with retry countdown; revert |
| Redis down | — | — | Fail-open server-side; no user impact |

## 6. Deployment

- `RATING_RATE_PER_MINUTE=20` and `TOP_RATED_MIN_RATINGS=5` added to
  `deployment/docker-compose.yml` backend environment and the Helm chart
  (`values.yaml` `backend.env.ratingRatePerMinute` /
  `topRatedMinRatings` + `templates/deployment-backend.yaml` env entries).
- DDL additive/idempotent; both init.sql copies in the same commit.
- No new services, ports, or infrastructure.

## 7. SDK

No python-sdk changes: ratings are a web-surface feature; the SDK wraps
template resolution. The REST endpoints are documented in the API surface.

## 8. Test Strategy

### 8.1 FVT (`tests/fvt.py`) — `test_template_ratings`

Two users (alice = owner, bob = rater) via `get_auth_token`:

1. alice creates a public template; bob rates it 4 → `{rating_avg: 4.0,
   rating_count: 1, my_rating: 4}`.
2. bob re-rates 2 → `{rating_avg: 2.0, rating_count: 1, my_rating: 2}` (row
   replaced, count unchanged).
3. carol rates 5 → `{rating_avg: 3.5, rating_count: 2}`.
4. bob deletes his rating → `{rating_avg: 5.0, rating_count: 1, my_rating:
   0}`; repeat delete → identical response (idempotent).
5. alice rates her own template → 403; anonymous POST → 401; bob rates with
   0/6/3.5 → 400.
6. Ban carol (admin API) → carol's POST → 403; her existing rating still
   counts in aggregates (FR-14).
7. Private template: bob's rate attempt → 404.
8. Detail response: `rating_distribution` matches the rating rows; list
   items carry `rating_avg`/`rating_count`/`my_rating`.
9. Rate limit: 21 rapid changes → 429 with `Retry-After`.
10. Admin template detail shows `rating_avg`/`rating_count`; admin rating
    mutation routes → 404/405.

### 8.2 Unit Tests

- Repository: CTE upsert delta math (insert/update/no-change), delete floor,
  distribution GROUP BY, `top_rated` ordering with threshold.
- Service: validation matrix, owner/ban/visibility checks, rate-limit window
  (fake Redis), response mapping (avg rounding).
- Update `MockTemplateRepository` with the three new methods.

### 8.3 E2E (test-agent, Nightwatch)

- Widget: hover preview, click commit, keyboard (←/→/Enter/Esc), clear with
  popover; optimistic revert on forced failure.
- Histogram renders 5 rows with text percentages; hidden at zero.
- Card row omitted at zero ratings; shown "★ 4.6 (128)" otherwise.
- `top_rated` sort option present once search-filtering ships.
- i18n EN/ZH parity for all `ratings.*` and `sort.top_rated` keys.

## 9. Implementation Order

1. DDL (both init.sql copies) + models + repository methods + unit tests.
2. Proto changes + regenerate + read-path joins (`my_rating`, `rating_avg`).
3. Service RPCs + rate limit + `RedisStore` wiring.
4. Admin read-only fields.
5. Gateway `/rating` branch.
6. Frontend: api client → components → pages → i18n.
7. FVT + E2E.

## 10. Constraints for Developer

1. **Never commit `backend/server`** — tracked build artifact; `git checkout
   -- backend/server` if modified.
2. Stage only your own files; never `git add -A`.
3. Both init.sql copies in sync in the same commit.
4. `PromptService.RedisStore` is shared with the template-comments design —
   introduce it once; do not duplicate the field.
5. `Template` proto field numbers 17–22 are a coordinated block across
   comments (17), follow-creators (18/19), and ratings (20–22) — do not
   renumber.
6. The average is never stored; compute `rating_sum/rating_count` at mapping
   time and guard division by zero.
7. `my_rating = 0` means "unrated" (ratings are 1–5); the frontend treats 0
   as the unrated state.
8. Rate limiting consumes quota only on successful changes; fail-open on
   Redis errors.
9. No notification is emitted for rating actions.
10. No monetization surfaces of any kind.
