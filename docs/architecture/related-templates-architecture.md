# Related Templates & Discovery — Architecture

| | |
|---|---|
| **Feature ID** | related-templates |
| **Version** | 1.0 |
| **Status** | Approved for implementation |
| **Author** | Architect Agent |
| **Upstream** | [Requirements](../design/related-templates-requirements.md), [UI/UX Design](../design/related-templates-ui-ux-design.md) |
| **Downstream** | [Detailed Design](related-templates-detailed-design.md) |

## 1. Overview

The template detail page is currently a dead end. This feature adds two
discovery rails to every public template detail page: a **"Related
prompts"** rail (up to 8 cards, scored from explainable metadata — same
category, shared tags, same language, same author) and a **"More from
{author}"** row (up to 4 cards, most-liked first). Relatedness is a pure
function of the template's own metadata: no personalization, no ML, no
tracking, no new tables. Two new read-only sub-resource endpoints serve
the rails; the frontend lazy-loads them after the main content renders.

### 1.1 Resolved Open Questions (Requirements §10)

| # | Question | Decision |
|---|---|---|
| 1 | Query shape | **Single SQL query** with a CASE-based scoring expression, `ORDER BY score DESC, like_count DESC, updated_at DESC, LIMIT 8`. The pool is small, the expression is simple, and one round-trip beats application-side scoring. |
| 2 | Cache layer | **Redis, 5-minute TTL** (the stack already runs Redis; `PromptService` already holds a `RedisStore`). Key: `related:v1:{template_id}:{limit}`. In-process caching would not be shared across replicas; HTTP/CDN caching is unnecessary for a lazy-loaded XHR. |
| 3 | Endpoint granularity | **Two endpoints** (related + more-from-author). They lazy-load in parallel, cache independently, and fail independently — a combined `?include=` would couple them to the detail response's lifecycle. |
| 4 | Same-author weight | **Keep the 0.5 weight** (FR-3). The author's own work is legitimately related; the low weight plus the dedicated author row keeps it from flooding the rail. |
| 5 | Basis label | **Single `basis` field on the response** (category/tag/generic), derived from the top item's dominant signal. Per-card reasons are a heavier payload for marginal v1 value. |
| 6 | Limits | **8 related / 4 author confirmed.** Same payload for all viewports; mobile shows fewer via CSS (scroll-snap), not a different request. |

### 1.2 Non-Goals

- Personalized recommendations, view-history signals, co-interaction
  matrices (the personalized-recommendations design owns that space).
- Home-page recommendation rails.
- ML/embedding similarity or semantic search.
- Related collections/chains/teams.
- "Not interested" controls on rail cards.
- **Monetization of any kind** — sponsored rails, promoted placements, or
  paid visibility are permanently excluded from this product.

## 2. System Context

```mermaid
flowchart LR
    subgraph Client
        TD[TemplateDetails page<br/>+ RelatedRail + MoreFromAuthorRow]
    end
    subgraph Gateway
        GW[REST gateway :8080<br/>/api/v1/templates/ handler]
    end
    subgraph Backend
        PS[PromptService<br/>GetRelatedTemplates / ListMoreFromAuthor<br/>visibility gate + Redis cache]
        TR[TemplateRepository<br/>ListRelated / ListMoreFromAuthor]
    end
    subgraph Stores
        PG[(PostgreSQL)]
        RD[(Redis<br/>5-min rail cache)]
    end
    TD -->|GET /api/v1/templates/{id}/related| GW --> PS
    TD -->|GET /api/v1/templates/{id}/more-from-author| GW --> PS
    PS --> RD
    PS --> TR --> PG
```

## 3. Components Involved

| Component | Change |
|---|---|
| `backend/api/proto/v1/prompt.proto` | Two RPCs on `PromptService`: `GetRelatedTemplates`, `ListMoreFromAuthor`; request/response messages. No changes to `Template` (fields 18/19 are reserved by follow-creators, 20–22 by template-ratings — untouched). |
| `backend/internal/repository/template_repository.go` | Two query methods: `ListRelated` (single scoring query) and `ListMoreFromAuthor` (simple filtered list). Both reuse the public-visibility + banned-owner exclusion fragments. |
| `backend/internal/service/prompt.go` (or new `related.go`) | Visibility gate (same pattern as `ListComments`), Redis read/write-through cache, basis derivation, proto mapping. |
| `backend/cmd/server/main.go` | Two GET branches in the existing `/api/v1/templates/` handler: `parts[1] == "related"` and `parts[1] == "more-from-author"`. Optional auth (invalid token → 401, same as detail GET). |
| `frontend/src/components/related/` (new) | `RelatedRail.js`, `MoreFromAuthorRow.js`, `RailCard` (reuses `PromptCard`), `BasisLabel`, skeletons. |
| `frontend/src/pages/TemplateDetails.js` | Mount both rails below the prompt content (above comments when the comments feature lands). |
| `frontend/src/services/api.js` | `getRelatedTemplates(id, limit)`, `getMoreFromAuthor(id, limit)`. |
| `frontend/src/locales/{en,zh}/translation.json` | `related.*` namespace, EN/ZH parity. |
| `deployment/init.sql` (both copies) | **No changes** — no new tables or indexes required (see §5). |

## 4. API Design

Both endpoints are sub-resources of the existing
`/api/v1/templates/{id}/...` handler, following the `versions`/`comments`
precedent.

### 4.1 Get Related Templates

`GET /api/v1/templates/{id}/related?limit=8`

| Param | Type | Default | Constraint |
|---|---|---|---|
| `limit` | int | 8 | Clamped to [3, 8]. Values below 3 clamp to 3 (the rail hides below 3 anyway); above 8 clamp to 8. |

**Auth:** optional (same dance as the detail GET: invalid token → 401;
anonymous → public context).

**Response:**

```json
{
  "items": [ /* Template messages, up to 8 */ ],
  "basis": "category"   // "category" | "tag" | "generic"
}
```

- `items` are full `Template` protos (the same shape the list endpoint
  returns), so `PromptCard` renders them without adaptation.
- `basis` derivation: if the top-scored item shares the template's
  category → `"category"`; else if it shares ≥1 tag → `"tag"` (label uses
  the first shared tag of the top item); else `"generic"`.
- Fewer than 3 survivors → `items: []` with `basis: "generic"`; the
  frontend hides the section (FR-5). The endpoint never errors on
  emptiness.

### 4.2 More from Author

`GET /api/v1/templates/{id}/more-from-author?limit=4`

| Param | Type | Default | Constraint |
|---|---|---|---|
| `limit` | int | 4 | Clamped to [1, 4]. |

**Auth:** optional, same as above.

**Response:**

```json
{ "items": [ /* Template messages, up to 4 */ ] }
```

Empty `items` when the author has no other public templates; the frontend
hides the row (FR-6).

### 4.3 Visibility Rules (both endpoints)

Identical to the template detail endpoint and `ListComments`:

1. Template not found → `404 NOT_FOUND`.
2. Template private and caller is not the owner → `404 NOT_FOUND`
   (indistinguishable from nonexistent — same convention as comments).
3. Public template of a banned owner → `404 NOT_FOUND` (the
   `templateRepository.Get` banned-owner check already enforces this).
4. Rails render only on public templates. When the **owner** views their
   own private template, both endpoints return `404` — rails are a
   public-discovery surface, not an authoring aid (FR-11: "do not render
   on private templates (owner-only view)"). The frontend does not call
   the endpoints when `visibility != "public"`.

### 4.4 Error Codes

| HTTP | gRPC | Condition |
|---|---|---|
| 404 | `NOT_FOUND` | Nonexistent id; private template (non-owner or owner — see §4.3); banned-owner public template. |
| 401 | `UNAUTHENTICATED` | Bearer token present but invalid (gateway rejects before service). |
| 429 | `RESOURCE_EXHAUSTED` | Existing per-user rate limit on template reads (unchanged surface). |
| 500 | `INTERNAL` | Repository failure. |

No new error conditions; `writeError` maps all of the above already.

### 4.5 Caching (FR-10, NFR-1, NFR-3)

- Redis key `related:v1:{template_id}:{limit}` (and
  `author:v1:{template_id}:{limit}`), TTL 5 minutes, value = serialized
  JSON of the response.
- Read-through: on miss, compute, store, return. On Redis failure, log
  and fall through to the database (rails are an enhancement — Redis is
  never a hard dependency).
- Cache invalidation: none in v1 (5-minute staleness is acceptable for
  discovery rails; like counts may lag slightly). A template delete or
  visibility flip propagates within the TTL.
- Determinism (NFR-3): the SQL is fully deterministic (score, then
  like_count, then updated_at, then id as final tiebreaker), so repeated
  loads within the TTL return byte-identical responses.

## 5. Data Model

**No schema changes.** Relatedness reads only existing columns:
`templates.category`, `tags`, `language`, `owner_id`, `like_count`,
`visibility`, `updated_at` (D-1). The scoring query needs no new index:
it filters `visibility = 'public'` (existing index), excludes the current
template and banned owners, and sorts a bounded candidate set. At v1
scale (≤100k templates) the sequential scan over public templates with
in-memory sort is well within the 200 ms p95 budget; the existing
`idx_templates_category` and GIN tags index help the common
category/tag-driven cases. If p95 degrades, the documented upgrade path
is a `LIMIT`-bounded pre-filter on `category = $current` OR
`tags && $current_tags` before scoring.

### 5.1 Scoring Query Shape

```sql
SELECT t.* /* + is_liked/is_favorited joins as in List */
FROM templates t
WHERE t.visibility = 'public'
  AND t.id != $1                          -- current template
  AND NOT EXISTS (SELECT 1 FROM users bu
                  WHERE bu.id = t.owner_id AND bu.status = 'banned')
ORDER BY
    (CASE WHEN t.category = $2 THEN 3 ELSE 0 END
   + CASE WHEN t.tags && $3 THEN 2 * cardinality(ARRAY(
             SELECT unnest(t.tags) INTERSECT SELECT unnest($3))) ELSE 0 END
   + CASE WHEN t.language = $4 THEN 1 ELSE 0 END
   + CASE WHEN t.owner_id = $5 THEN 0.5 ELSE 0 END) DESC,
    t.like_count DESC,
    t.updated_at DESC,
    t.id ASC
LIMIT $6
```

`$3` is the current template's tags as a `pq.Array` parameter. The
`more-from-author` query is the same shape minus scoring: `owner_id =
$owner AND visibility = 'public' AND id != $1`, `ORDER BY like_count
DESC, updated_at DESC, id ASC`, `LIMIT $6`.

## 6. Auth & Authorization

- Both endpoints are read-only with optional auth — no new authorization
  decisions. The visibility gate (§4.3) reuses the exact `ListComments`
  pattern: fetch via `TemplateRepo.Get`, 404 on private-non-owner.
- Identity comes only from context (`GetUserIDFromContext`); the client
  never supplies an owner or user id.
- No admin surface (FR-14): admin template views do not render rails.

## 7. Frontend

- **`RelatedRail`**: `<section aria-label>` with `h2` header
  ("Related · {category/tag}" via `BasisLabel`), horizontal card list
  (desktop: wrapped grid; mobile: scroll-snap with peeking cards),
  skeleton while loading, hidden when `items.length < 3` or on error
  (silent — rails never surface failures).
- **`MoreFromAuthorRow`**: same structure; author name links to the
  follow-creators public profile (`/creators/:id`) when that surface
  ships, plain text until then (FR-7/D-5).
- **Cards** reuse `PromptCard` unchanged (FR-12).
- **Lazy loading** (FR-11/NFR-1): rails fetch after the detail page's
  main content renders — a `useEffect` fired post-paint (the existing
  detail page already loads the template first; rails are a second,
  non-blocking request). LCP is unaffected.
- **Placement**: below prompt/version content, above comments (when
  present). Not rendered for private templates or admin views.
- **i18n**: `related.*` namespace; category/tag names render as-is
  (proper nouns, NFR-4).

## 8. Configuration

No new environment variables. Cache TTL is a Go constant
(`relatedCacheTTL = 5 * time.Minute`); limits (8/4) are Go constants.
Redis address comes from the existing `REDIS_ADDR`.

## 9. Security Considerations

- **Injection**: all values are bound parameters (`$1..$n`); the basis
  string is a server-side enum, never client input.
- **Privacy**: no new PII. Rails expose only public templates of
  non-banned authors — the same data the public list endpoint already
  serves. No viewer-specific data is used or stored (NFR-6).
- **Access control**: private templates are indistinguishable from
  nonexistent ones (404), matching the comments convention; banned-owner
  public templates are hidden by the existing repository check.
- **Abuse**: read-only endpoints under the existing rate limiter; the
  Redis cache absorbs repeat traffic. No write surface to abuse.

## 10. Deployment Notes

- No DDL, no new services, ports, or environment variables.
- Redis is already a hard dependency of the deployment; the rail cache
  adds ~2 keys per viewed template with a 5-minute TTL — negligible
  memory (~a few KB each).
- `backend/server` is a tracked build artifact — never commit changes to
  it.

## 11. Test Strategy

- **FVT** (`tests/fvt.py`): `test_related_templates` — seed a category
  cluster, tag cluster, language cluster, and author catalog; verify
  ranking order (category beats tags beats language beats author-only),
  score tiebreaks (like_count, updated_at), current-template exclusion,
  private/banned exclusion, 404 on private/nonexistent ids (anonymous and
  non-owner), <3 candidates → empty items, more-from-author ordering and
  empty case, limit clamping, cache determinism (two calls within TTL →
  identical responses).
- **Unit**: basis derivation; visibility gate (private-non-owner 404,
  owner-views-private 404 per §4.3); cache read/write-through and
  Redis-failure fallback; repository scoring SQL parameterization;
  `MockTemplateRepository` stubs for both new methods.
- **E2E** (test-agent): rails lazy-load after content; skeleton → loaded;
  hidden states (<3, no author templates); card click navigates; author
  link; keyboard navigation through cards; `aria-label` sections; EN/ZH
  copy; no rail on private template view.
