# Related Templates & Discovery — Detailed Design

| | |
|---|---|
| **Feature ID** | related-templates |
| **Version** | 1.0 |
| **Status** | Approved for implementation |
| **Author** | Architect Agent |
| **Upstream** | [Architecture](related-templates-architecture.md) |

## 1. Proto Contract

Additions to `backend/api/proto/v1/prompt.proto` (new RPCs on
`PromptService`; `Template` message untouched — fields 18/19 reserved by
follow-creators, 20–22 by template-ratings):

```protobuf
service PromptService {
    // ... existing RPCs ...

    // GetRelatedTemplates returns explainable related templates for a
    // public template (category/tags/language/author metadata scoring).
    rpc GetRelatedTemplates(GetRelatedTemplatesRequest) returns (GetRelatedTemplatesResponse);

    // ListMoreFromAuthor returns the author's other public templates,
    // most-liked first.
    rpc ListMoreFromAuthor(ListMoreFromAuthorRequest) returns (ListMoreFromAuthorResponse);
}

message GetRelatedTemplatesRequest {
    string template_id = 1;
    int32 limit = 2; // default 8, clamped to [3, 8]
}

message GetRelatedTemplatesResponse {
    repeated Template items = 1;
    string basis = 2; // "category" | "tag" | "generic"
}

message ListMoreFromAuthorRequest {
    string template_id = 1;
    int32 limit = 2; // default 4, clamped to [1, 4]
}

message ListMoreFromAuthorResponse {
    repeated Template items = 1;
}
```

Regenerate:

```bash
cd backend && protoc --go_out=. --go-grpc_out=. api/proto/v1/prompt.proto
```

## 2. Database DDL

**None.** No new tables, columns, or indexes (architecture §5). Both
`deployment/init.sql` copies are unchanged by this feature.

## 3. Backend Layers

### 3.1 Repository (`backend/internal/repository/template_repository.go`)

Interface additions:

```go
type TemplateRepository interface {
    // ... existing methods ...
    ListRelated(ctx context.Context, templateID string, tpl *models.Template, limit int) ([]*models.Template, error)
    ListMoreFromAuthor(ctx context.Context, templateID, ownerID string, limit int) ([]*models.Template, error)
}
```

`ListRelated` — single scoring query (all bound parameters; `is_liked`/
`is_favorited` LEFT JOINs included as in `List`, using the caller's user
id so cards render correct like/favorite state):

```sql
SELECT t.id, t.owner_id, t.title, t.description, t.visibility, t.type,
       t.tags, t.category, t.language, t.like_count, t.favorite_count,
       t.comment_count, t.created_at, t.updated_at,
       CASE WHEN tl.user_id IS NOT NULL THEN true ELSE false END AS is_liked,
       CASE WHEN tf.user_id IS NOT NULL THEN true ELSE false END AS is_favorited
FROM templates t
LEFT JOIN template_likes tl ON t.id = tl.template_id AND tl.user_id = $2
LEFT JOIN template_favorites tf ON t.id = tf.template_id AND tf.user_id = $2
WHERE t.visibility = 'public'
  AND t.id != $1
  AND NOT EXISTS (SELECT 1 FROM users bu
                  WHERE bu.id = t.owner_id AND bu.status = 'banned')
ORDER BY
    (CASE WHEN t.category = $3 THEN 3 ELSE 0 END
    + CASE WHEN t.tags && $4 THEN 2 * cardinality(ARRAY(
              SELECT unnest(t.tags) INTERSECT SELECT unnest($4))) ELSE 0 END
    + CASE WHEN t.language = $5 THEN 1 ELSE 0 END
    + CASE WHEN t.owner_id = $6 THEN 0.5 ELSE 0 END) DESC,
    t.like_count DESC,
    t.updated_at DESC,
    t.id ASC
LIMIT $7
```

Parameters: `$1` template id, `$2` current user id (empty string for
anonymous — the LEFT JOINs simply miss), `$3` category (may be NULL —
`NULL = NULL` is NULL → 0 points, correct), `$4` tags `pq.Array`, `$5`
language, `$6` owner id, `$7` limit.

`ListMoreFromAuthor`:

```sql
SELECT t.* /* same column list + like/favorite joins */
FROM templates t
LEFT JOIN template_likes tl ON ... AND tl.user_id = $2
LEFT JOIN template_favorites tf ON ... AND tf.user_id = $2
WHERE t.owner_id = $3
  AND t.visibility = 'public'
  AND t.id != $1
ORDER BY t.like_count DESC, t.updated_at DESC, t.id ASC
LIMIT $4
```

(The banned-owner exclusion is unnecessary here: the caller reached this
code only through `Get`, which already 404s banned-owner templates.)

Both methods scan into `models.Template` exactly as `List` does.

### 3.2 Service (`backend/internal/service/related.go`, new file)

```go
const (
    relatedCacheTTL   = 5 * time.Minute
    relatedDefaultLimit = 8
    authorDefaultLimit  = 4
)

func (s *PromptService) GetRelatedTemplates(ctx context.Context, req *pb.GetRelatedTemplatesRequest) (*pb.GetRelatedTemplatesResponse, error)
func (s *PromptService) ListMoreFromAuthor(ctx context.Context, req *pb.ListMoreFromAuthorRequest) (*pb.ListMoreFromAuthorResponse, error)
```

Shared flow (both methods):

1. **Visibility gate** (identical to `ListComments`):
   ```go
   userID, _ := GetUserIDFromContext(ctx) // optional auth
   tpl, err := s.TemplateRepo.Get(ctx, req.TemplateId, userID)
   if err != nil {
       return nil, status.Error(codes.NotFound, "template not found")
   }
   if tpl.Visibility != "public" {
       // Rails are a public-discovery surface: private templates 404 for
       // everyone, including the owner (architecture §4.3).
       return nil, status.Error(codes.NotFound, "template not found")
   }
   ```
   (`TemplateRepo.Get` already 404s banned-owner public templates.)
2. **Clamp limit**: related → [3, 8] (default 8); author → [1, 4]
   (default 4).
3. **Cache read** (`GetRelatedTemplates` only — the author row is cheap
   enough to compute each time; see §3.4):
   ```go
   key := fmt.Sprintf("related:v1:%s:%d", tpl.ID, limit)
   if cached, err := s.Redis.Get(ctx, key); err == nil {
       var resp pb.GetRelatedTemplatesResponse
       if json.Unmarshal([]byte(cached), &resp) == nil { return &resp, nil }
   }
   ```
   Redis errors are logged and swallowed — fall through to the database.
4. **Query** the repository; map with the existing
   `templateModelToProto`.
5. **Basis derivation** (related only):
   - top item's `category == tpl.Category` (both non-empty) →
     `"category"`;
   - else top item shares ≥1 tag with `tpl.Tags` → `"tag"` (the frontend
     labels with the first shared tag of the top item);
   - else `"generic"`.
6. **Cache write** (related only): marshal response to JSON,
   `s.Redis.Set(ctx, key, jsonStr, relatedCacheTTL)`; log-and-swallow
   errors.
7. Fewer than 3 items → return `items: []` (never an error); the
   frontend hides the section.

`ListMoreFromAuthor` skips steps 3/6 and returns the repository result
directly (empty slice, not nil, so protojson emits `[]`).

### 3.3 Gateway (`backend/cmd/server/main.go`)

Inside the existing `/api/v1/templates/` handler, alongside the
`versions`/`comments` branches (before the default single-template
switch):

```go
// /api/v1/templates/{template_id}/related — GET (public, optional auth)
if len(parts) == 2 && parts[1] == "related" {
    if r.Method != http.MethodGet {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }
    ctx := context.Background()
    if authHeader := r.Header.Get("Authorization"); authHeader != "" {
        tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
        if userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr); err == nil {
            ctx = service.ContextWithUserID(ctx, userID)
        } else {
            http.Error(w, "Invalid token", http.StatusUnauthorized)
            return
        }
    }
    req := &pb.GetRelatedTemplatesRequest{TemplateId: parts[0]}
    if v := r.URL.Query().Get("limit"); v != "" {
        if i, err := strconv.Atoi(v); err == nil {
            req.Limit = int32(i)
        }
    }
    resp, err := svc.GetRelatedTemplates(ctx, req)
    if err != nil { writeError(w, err); return }
    w.Header().Set("Content-Type", "application/json")
    b, _ := marshaler.Marshal(resp)
    _, _ = w.Write(b)
    return
}
```

The `more-from-author` branch is identical in shape (`parts[1] ==
"more-from-author"`, `ListMoreFromAuthorRequest`). Note the invalid-token
behavior matches the detail GET (401) rather than the silently-anonymous
`versions` branch — rails are served to browsers alongside the detail
page, which uses the same optional-auth dance.

### 3.4 Why the author row is not cached

The related query scores the whole public pool; the author query is a
single indexed `owner_id` lookup with `LIMIT 4`. Caching it would add
invalidation surface (author deletes/creates templates) for negligible
latency win. The 5-minute related cache is acceptable for discovery
metadata; like-count staleness within TTL is fine (NFR-3 requires
determinism, not freshness).

## 4. Frontend

### 4.1 New Files

| File | Purpose |
|---|---|
| `frontend/src/components/related/RelatedRail.js` + `.scss` | `<section aria-label>` + `h2` + BasisLabel + card list; skeleton while loading; hidden when <3 items or on error. |
| `frontend/src/components/related/MoreFromAuthorRow.js` + `.scss` | Same structure; author name (link to `/creators/:id` when available); hidden when empty or on error. |
| `frontend/src/components/related/BasisLabel.js` | Renders `related.title.category`/`title.tag`/`title.generic` with the interpolated value. |

Cards are the existing `PromptCard` — no new card component (FR-12).

### 4.2 Modified Files

| File | Change |
|---|---|
| `frontend/src/pages/TemplateDetails.js` | Import and mount `RelatedRail` + `MoreFromAuthorRow` below the prompt/version content (above `CommentsSection` when present); pass `templateId`, `template.category`, `template.tags`, `template.ownerId`, `template.visibility`; skip mounting when `visibility !== 'public'`. |
| `frontend/src/services/api.js` | Add `getRelatedTemplates(id, limit)` → `api.get(\`/templates/${id}/related?limit=${limit}\`)` and `getMoreFromAuthor(id, limit)` similarly. |
| `frontend/src/locales/{en,zh}/translation.json` | `related.*` namespace (§4.4). |

### 4.3 Component Behavior

- **Lazy load**: each rail fetches in its own `useEffect` after mount —
  the detail page's main content request is already in flight/complete;
  rails never block the first paint (AC-7).
- **Silent failure**: fetch errors set a hidden state — no error banner,
  no retry UI (rails are enhancement, never a failure surface).
- **Desktop layout**: wrapped grid (2 rows × 4 columns max). **Mobile**:
  horizontal scroll-snap; cards 72% viewport width so the next card
  peeks; the scroll region is keyboard-scrollable when focused (native
  overflow), cards are focusable links in DOM order (AC-8).
- **Card click** navigates to `/templates/:id` (standard in-app
  navigation).
- **Author link**: `/creators/:ownerId` when the follow-creators route
  exists; plain text until then (feature-detect via route registration,
  or simply render the link — a missing route shows the app's 404, which
  is acceptable during the gap; prefer plain text until follow-creators
  ships).

### 4.4 i18n Keys (EN/ZH parity)

```json
{
  "related": {
    "title": {
      "category": "Related · {{category}}",
      "tag": "Related · {{tag}}",
      "generic": "Related prompts"
    },
    "moreFromAuthor": "More from {{author}}",
    "aria": {
      "rail": "Related prompts section",
      "authorRow": "More from this author"
    }
  }
}
```

ZH: `related.title.category` = "相关 · {{category}}"、`related.title.tag`
= "相关 · {{tag}}"、`related.title.generic` = "相关提示词"、
`related.moreFromAuthor` = "来自 {{author}} 的更多"、`related.aria.rail`
= "相关提示词区块"、`related.aria.authorRow` = "来自此作者的更多"。
Category/tag names are proper nouns — never translated (NFR-4).

## 5. Error Matrix

| Scenario | HTTP | gRPC | Frontend handling |
|---|---|---|---|
| Nonexistent template id | 404 | `NOT_FOUND` | Section hidden (silent). |
| Private template (any caller incl. owner) | 404 | `NOT_FOUND` | Frontend doesn't call (visibility check); 404 also hides. |
| Banned-owner public template | 404 | `NOT_FOUND` | Section hidden (silent). |
| Invalid Bearer token | 401 | `UNAUTHENTICATED` | Existing auth-error handling. |
| <3 related candidates | 200, `items: []` | — | Section hidden. |
| No other author templates | 200, `items: []` | — | Row hidden. |
| Redis down | 200 (DB fallback) | — | None (log-only). |
| Repository failure | 500 | `INTERNAL` | Section hidden (silent). |
| `limit` out of range | 200 (clamped) | — | None. |

## 6. Deployment

No DDL, no new services/env vars. Redis usage: one string key per
(template, limit) viewed, 5-minute TTL, a few KB each — negligible.

## 7. SDK

No SDK change required (rails are a web surface). Optionally document
the two GET endpoints in the SDK README's endpoint list — `client`
kwargs pass-through makes them usable today.

## 8. Test Strategy

### 8.1 FVT (`tests/fvt.py`) — `test_related_templates`

Fixtures: alice creates a category cluster (3+ templates in "Customer
Support"), a tag cluster (shared tag "writing"), a language cluster, and
several other templates; bob creates templates in the same category;
carol (banned by admin) has public templates; one private template.

1. Related rail returns up to 8 public templates, ranked: same-category
   ahead of tag-only ahead of language-only ahead of unrelated; author
   templates appear but do not dominate (0.5 weight).
2. Score ties break by like_count DESC, then updated_at DESC.
3. Current template never in its own rail; private templates never
   appear; banned carol's templates never appear.
4. `basis` is `"category"` when the top item shares the category;
   `"tag"` when tag-driven; `"generic"` otherwise.
5. Fewer than 3 candidates → `items: []` (200, not an error).
6. More-from-author: up to 4, most-liked first, current template
   excluded, only public; empty when the author has none.
7. 404 for nonexistent id, private id (anonymous, non-owner, and owner),
   banned-owner public id.
8. `limit` clamping: `limit=100` → 8 items max; `limit=1` on related →
   still ≥3-or-empty (clamped to 3).
9. Two identical calls within the TTL return identical responses
   (determinism).
10. Invalid Bearer token → 401.

### 8.2 Unit Tests

- Visibility gate: private → 404 for owner and non-owner; public passes.
- Limit clamping bounds.
- Basis derivation (category/tag/generic, empty-category edge).
- Cache: hit returns cached; Redis error falls through to repo; write
  failure is swallowed.
- Repository: parameter counts/order for both queries; empty-tags edge
  (`tags && '{}'` is false — no points, no error).
- `MockTemplateRepository`: add `ListRelatedFunc`/`ListMoreFromAuthorFunc`
  hooks (following the existing `GetFunc` pattern).

### 8.3 E2E (test-agent, Nightwatch)

- Rails appear below content on public template pages; absent on private
  (owner view) and on <3 candidates.
- Skeleton → loaded transition; no layout shift of main content.
- Card click navigates to the target detail page.
- Author row: ordering, author link target.
- Keyboard: Tab reaches each card; section `aria-label`s present.
- EN/ZH copy for all `related.*` keys; category/tag names untranslated.

## 9. Implementation Order

1. Proto RPCs + messages; regenerate.
2. Repository: `ListRelated` + `ListMoreFromAuthor` + unit tests.
3. Service: `related.go` (gate, clamp, cache, basis) + unit tests.
4. Gateway: two GET branches.
5. Frontend: api client → components → TemplateDetails mount → i18n.
6. FVT + E2E.

## 10. Constraints for Developer

1. **Never commit `backend/server`** — tracked build artifact;
   `git checkout -- backend/server` if modified.
2. Stage only your own files; never `git add -A`.
3. No DDL changes — both init.sql copies stay untouched.
4. Private templates 404 for **everyone** on both endpoints, including
   the owner (rails are public-discovery only).
5. All SQL values are bound parameters; the basis string is a
   server-side constant, never client input.
6. Redis failures are log-and-swallow — never fail a rail request
   because of cache issues.
7. Do not touch `Template` proto fields 18/19 (follow-creators) or
   20–22 (template-ratings).
8. Rails hide silently on any error — no banners, no retries.
9. The frontend must not call the endpoints when
   `template.visibility !== 'public'`.
10. No personalization signals of any kind; no tracking; no
    monetization surfaces.
