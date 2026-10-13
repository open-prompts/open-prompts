# Template Search & Filtering — Detailed Design

| | |
|---|---|
| **Feature ID** | template-search-filtering |
| **Version** | 1.0 |
| **Status** | Approved for implementation |
| **Author** | Architect Agent |
| **Upstream** | [Architecture](template-search-filtering-architecture.md) |

## 1. Proto Contract

Additions to `backend/api/proto/v1/prompt.proto`:

```protobuf
message ListTemplatesRequest {
    // ... existing fields 1-9 (page_size, page_token, visibility,
    //     owner_id, category, tags, my_likes, my_favorites, language) ...
    // Free-text keyword; matched case-insensitively against title,
    // description, category, tags, and latest-version content.
    string keyword = 10;
    // Sort order: newest (default) | most_liked | most_favorited | trending.
    // Invalid values fall back to newest server-side.
    string sort = 11;
}

message ListTemplatesResponse {
    repeated Template templates = 1; // Used for public templates when mixed
    string next_page_token = 2; // Used for public pagination when mixed
    repeated Template private_templates = 3;
    string private_next_page_token = 4;
    // Total matches in the public/single stream for the current filters.
    int32 total_count = 5;
    // Total matches in the private stream (mixed view only; else 0).
    int32 private_total_count = 6;
}
```

Count fields are `int32` per project convention (protojson serializes
`int64` as a string, which breaks JS clients).

Regenerate:

```bash
cd backend && protoc --go_out=. --go-grpc_out=. api/proto/v1/prompt.proto
```

## 2. Database DDL

Append to **both** `deployment/init.sql` and
`deployment/chart/open-prompts/init.sql`:

```sql
-- ===== template-search-filtering =====
-- Performance indexes for the trending sort (7-day engagement window).
-- No table or column changes are required by this feature.
CREATE INDEX IF NOT EXISTS idx_template_likes_template_created
    ON template_likes(template_id, created_at);
CREATE INDEX IF NOT EXISTS idx_template_favorites_template_created
    ON template_favorites(template_id, created_at);
```

## 3. Backend Layers

### 3.1 `TemplateRepository` (`backend/internal/repository/template_repository.go`)

**Refactor — shared condition builder.** Extract the existing
filter-assembly block of `List` (visibility, owner_id, category, language,
tags, my_likes, my_favorites, banned-owner exclusion) into:

```go
func (r *templateRepository) buildTemplateConditions(
    filters map[string]interface{}, args []interface{}, argID int,
) (string, []interface{}, int)
```

`List` and the new `CountTemplates` both call it, guaranteeing counts and
results can never diverge.

**Keyword condition** (appended by the builder when
`filters["keyword"]` is a non-empty string):

```go
kw := filters["keyword"].(string)
pattern := "%" + escapeLike(kw) + "%"
query += fmt.Sprintf(`
    AND (
        t.title ILIKE $%d
        OR t.description ILIKE $%d
        OR t.category ILIKE $%d
        OR EXISTS (SELECT 1 FROM unnest(t.tags) AS tg WHERE tg ILIKE $%d)
        OR EXISTS (SELECT 1 FROM template_versions tv
                   WHERE tv.id = (SELECT tv2.id FROM template_versions tv2
                                  WHERE tv2.template_id = t.id
                                  ORDER BY tv2.version DESC LIMIT 1)
                     AND tv.content ILIKE $%d)
    )`, argID, argID, argID, argID, argID)
args = append(args, pattern, pattern, pattern, pattern, pattern)
argID++
```

`escapeLike` escapes `\`, `%`, `_` (backslash-escaping each) so user input
is always matched literally. `t.description` is nullable — `NULL ILIKE`
yields NULL, which the OR chain treats as no-match, so no `COALESCE` is
needed.

**Sort whitelist** (replaces the hardcoded
`ORDER BY t.created_at DESC`):

```go
func templateOrderBy(sort string) string {
    switch sort {
    case "most_liked":
        return " ORDER BY t.like_count DESC, t.created_at DESC"
    case "most_favorited":
        return " ORDER BY t.favorite_count DESC, t.created_at DESC"
    case "trending":
        return " ORDER BY (COALESCE(el.cnt, 0) + COALESCE(ef.cnt, 0)) DESC, t.created_at DESC"
    default: // "newest" and any invalid value
        return " ORDER BY t.created_at DESC"
    }
}
```

**Trending joins** — added to the FROM clause only when `sort == "trending"`
(keeps every other query byte-identical to today):

```sql
LEFT JOIN (SELECT template_id, COUNT(*) AS cnt FROM template_likes
           WHERE created_at > now() - interval '7 days'
           GROUP BY template_id) el ON el.template_id = t.id
LEFT JOIN (SELECT template_id, COUNT(*) AS cnt FROM template_favorites
           WHERE created_at > now() - interval '7 days'
           GROUP BY template_id) ef ON ef.template_id = t.id
```

The 7-day window is a Go constant (`trendingWindow = 7 * 24 * time.Hour`)
passed as a bound interval parameter, not interpolated SQL text.

**New method:**

```go
CountTemplates(ctx context.Context, filters map[string]interface{}) (int32, error)
```

`SELECT COUNT(*)` over the same FROM/JOIN/WHERE produced by the shared
builder (trending joins included when the sort is trending — they do not
change the count, but reusing the builder verbatim is simpler and safe).

**Interface addition** (and a no-op stub in `MockTemplateRepository` in
`backend/internal/service/prompt_test.go`):

```go
type TemplateRepository interface {
    // ... existing methods ...
    CountTemplates(ctx context.Context, filters map[string]interface{}) (int32, error)
}
```

### 3.2 `PromptService` (`backend/internal/service/prompt.go`)

**Normalization helpers** (package-level):

```go
// normalizeKeyword trims, collapses internal whitespace, and caps at 100
// runes (not bytes — CJK-safe). Empty result means "no keyword filter".
func normalizeKeyword(s string) string

// normalizeSort returns a whitelisted sort value; anything else -> "newest".
func normalizeSort(s string) string
```

**`ListTemplates` changes** — apply in **all four** paths (my-likes/
my-favorites, anonymous, specific-visibility, mixed):

1. `keyword := normalizeKeyword(req.Keyword)`; if non-empty, set
   `filters["keyword"] = keyword`.
2. `sort := normalizeSort(req.Sort)`; set `filters["sort"] = sort`.
3. The `fetch` closure passes `filters` through to `TemplateRepo.List`
   (unchanged signature).
4. **Counts**: after each `fetch`, call `TemplateRepo.CountTemplates` with
   the same filters map (minus `current_user_id`, which the builder does
   not use for conditions) and populate `total_count`. In the mixed path,
   count the public filters → `total_count` and the private filters →
   `private_total_count`. In my-likes/my-favorites, anonymous, and
   specific-visibility paths, `private_total_count` stays 0.
5. **Observability** (NFR-9): extend the existing entry log with
   `keyword_len=%d sort=%s` — never the keyword content.

Count queries run only when the request is the first page of a query
(`page_token == ""`); subsequent pages reuse the count the client already
has. This bounds the cost to one extra `COUNT(*)` per query (NFR-4).

### 3.3 Gateway (`backend/cmd/server/main.go`)

In the `GET` branch of the `/api/v1/templates` handler, after the existing
query-parameter mapping:

```go
req.Keyword = q.Get("keyword")
if req.Keyword == "" {
    req.Keyword = q.Get("q") // alias used by the frontend URL contract
}
req.Sort = q.Get("sort")
```

No validation here — the service normalizes. `writeError` handles the
(unchanged) error surface.

## 4. Frontend

### 4.1 New Files

| File | Purpose |
|---|---|
| `frontend/src/hooks/useTemplateFilters.js` | State owner: keyword/category/tags/sort/visibility (+ transient `my_likes`/`my_favorites`/`owner_id`), URL sync, debounce, race guard, fetch orchestration, result counts. |
| `frontend/src/components/SearchBar.js` + `.scss` | `role="search"` input, 300ms debounce, Enter submit, clear button, combobox a11y for suggestions. |
| `frontend/src/components/SortDropdown.js` + `.scss` | Custom Aurora dropdown, 4 options, listbox keyboard pattern (↑↓ Enter Esc). |
| `frontend/src/components/FilterChipRow.js` + `.scss` | Removable chips per active filter + "Clear all". |
| `frontend/src/components/SuggestionsDropdown.js` + `.scss` | Max 5 rows (categories then tags) from shared taxonomy data; combobox pattern. |
| `frontend/src/components/ResultCount.js` | `aria-live="polite"` count; "—" while unavailable. |
| `frontend/src/components/EmptyState.js` + `.scss` | Search variant: echoed keyword, guidance, "Clear all filters". |
| `frontend/src/components/SkeletonCards.js` + `.scss` | 8 shimmer placeholder cards (pure CSS keyframes). |

### 4.2 Modified Files

| File | Change |
|---|---|
| `frontend/src/pages/Home.js` | Replace local filter/fetch state with `useTemplateFilters`; render search row (SearchBar + SortDropdown), FilterChipRow, ResultCount, SkeletonCards/EmptyState/error banner; keep infinite scroll and mixed-view merge logic. |
| `frontend/src/components/Sidebar.js` | Tag rows → multi-select checkboxes (toggle in `tags` array); taxonomy data (categories/tags) lifted to Home and passed down so SuggestionsDropdown shares it without refetch; category click still resets tags. |
| `frontend/src/components/Layout.js` | Pass taxonomy data through to children (props shape unchanged otherwise). |
| `frontend/src/services/api.js` | No change needed — `getTemplates(params)` already passes arbitrary params through; callers add `keyword`/`sort`. |
| `frontend/src/locales/{en,zh}/translation.json` | Add `search.*` namespace (§4.4). |

### 4.3 `useTemplateFilters` Contract

```js
const {
  filters,          // { keyword, category, tags, sort, visibility,
                    //   myLikes, myFavorites, ownerId }
  setKeyword,       // debounced 300ms; Enter bypasses via submitKeyword()
  submitKeyword,
  toggleTag, setCategory, setSort, setVisibility,
  clearAll,         // resets keyword/category/tags/sort; visibility -> persisted default
  templates, privateTemplates,
  totalCount,       // total_count + private_total_count
  nextPageToken, hasMore, loadingMore, isFirstLoading, error,
  loadMore, retry,
} = useTemplateFilters({ pageSize: 20 });
```

URL contract (home route): `?q=`, `?category=`, `?tags=a,b`,
`?sort=`, `?visibility=`. Keyword typing → `setSearchParams(..., { replace:
true })`; discrete changes → push. Restore on load: URL → localStorage
visibility → auth-state default. Every request carries a monotonically
increasing `requestId`; responses with stale IDs are discarded. Changing
any filter resets pagination. Failed queries keep the last successful
results rendered and expose `error`/`retry`.

### 4.4 i18n Keys (EN/ZH parity)

```json
{
  "search": {
    "placeholder": "Search prompts…",
    "clear": "Clear",
    "sort": {
      "label": "Sort",
      "newest": "Newest",
      "most_liked": "Most Liked",
      "most_favorited": "Most Favorited",
      "trending": "Trending"
    },
    "chips": {
      "clear_all": "Clear all",
      "remove_filter": "Remove filter: {{label}}"
    },
    "results_count": "{{count}} template",
    "results_count_other": "{{count}} templates",
    "suggestions": {
      "category": "Category",
      "tag": "Tag",
      "none": "No suggestions"
    },
    "empty": {
      "title": "No results for \"{{keyword}}\"",
      "body": "Try different keywords or clear filters.",
      "clear_all": "Clear all filters"
    },
    "error": {
      "fetch": "Search failed. Please try again.",
      "retry": "Retry"
    }
  }
}
```

ZH mirror (`search.placeholder` = "搜索提示词…", `search.sort.label` =
"排序", `search.sort.newest` = "最新", `search.sort.most_liked` = "最多点赞",
`search.sort.most_favorited` = "最多收藏", `search.sort.trending` = "趋势",
`search.chips.clear_all` = "全部清除", `search.chips.remove_filter` =
"移除筛选：{{label}}", `search.results_count` / `_other` = "{{count}} 个模板",
`search.suggestions.category` = "分类", `search.suggestions.tag` = "标签",
`search.suggestions.none` = "暂无建议", `search.empty.title` = "没有找到
\"{{keyword}}\" 的结果", `search.empty.body` = "试试其他关键词或清除筛选。",
`search.empty.clear_all` = "清除全部筛选", `search.error.fetch` = "搜索失败，请重试。",
`search.error.retry` = "重试"). `results_count` uses i18next pluralization;
ZH uses the same string for both forms. Sort API values are ASCII slugs,
never localized labels.

## 5. Error Matrix

| Scenario | HTTP | gRPC | Frontend handling |
|---|---|---|---|
| Invalid Bearer token | 401 | `UNAUTHENTICATED` | Existing optional-auth behavior (gateway rejects before service). |
| Invalid `sort` value | 200 | — | Silent server-side fallback to `newest`; UI never sends invalid values. |
| Keyword > 100 runes | 200 | — | Truncated server-side; input `maxLength=100` prevents it client-side. |
| Whitespace-only keyword | 200 | — | Treated as no keyword filter. |
| Repository failure | 500 | `INTERNAL` | Error banner + Retry; last successful results stay rendered. |
| Empty result set | 200 | — | EmptyState with echoed keyword + "Clear all filters". |

## 6. Deployment

- Two idempotent indexes; both init.sql copies in the same commit.
- No new services, ports, environment variables, or Redis keys.
- No frontend build-config changes.

## 7. SDK

`python-sdk/openprompts/client.py` `list_templates(**kwargs)` already
passes arbitrary query parameters through — `keyword=` and `sort=` work
with **zero SDK changes**. Update the SDK README's parameter list to
document them (one line each).

## 8. Test Strategy

### 8.1 FVT (`tests/fvt.py`) — `test_template_search_filtering`

Fixtures: alice creates templates with distinct titles/descriptions/tags/
categories and multiple versions (older version containing a marker the
latest does not); bob likes/favorites with controlled timestamps where
possible (trending verified by comparing relative order of recent vs.
all-time engagement).

1. Keyword matches title (case-insensitive), description, category, tag
   value, and latest-version content; a marker present only in an *older*
   version does **not** match.
2. CJK substring match (e.g. "邮件" matches a template whose title contains
   邮件营销).
3. Multi-word phrase: "email marketing" matches only fields containing the
   exact phrase; fields containing just "email" do not match.
4. LIKE metacharacters: keyword `100%` matches only literal "100%" text,
   not "100x"; `_` likewise literal.
5. Multi-tag AND: `tags=a&tags=b` returns only templates with both.
6. Sorts: seed like/favorite counts → `most_liked`/`most_favorited` order
   verified; `trending` ranks by 7-day window (a template with high
   all-time but zero recent engagement ranks below a low-all-time but
   recently-engaged one when the data allows).
7. `total_count` equals the number of matching templates; paginating with
   `keyword`+`sort` preserves both; `private_total_count` populated only in
   mixed view.
8. Anonymous search never returns private templates; logged-in mixed view
   returns own private matches (AC-7/AC-8).
9. `sort=bogus` → 200 with newest ordering (AC-9); omitted
   `keyword`/`sort` → response shape identical to pre-feature (plus the
   new count fields).
10. Whitespace-only keyword = no filter; 101-rune keyword truncated to 100.
11. Banned owner's public templates excluded from keyword results.
12. `my_likes=true&keyword=...` combines correctly.

### 8.2 Unit Tests

- `normalizeKeyword`: trim, whitespace collapse, 100-rune cap with CJK
  (rune count, not bytes), empty results.
- `normalizeSort`: whitelist + fallback.
- Repository builder: parameter numbering with/without keyword, escaping,
  sort branch selection, trending join presence only for trending.
- `MockTemplateRepository.CountTemplates` stub wired into service tests;
  mixed-view count population.

### 8.3 E2E (test-agent, Nightwatch)

- SearchBar: debounce (≤1 request per 300ms), Enter bypass, clear button.
- Suggestions: appear at ≥2 chars, categories-then-tags order, selection
  applies filter, combobox a11y attributes.
- SortDropdown: keyboard navigation, selection updates URL `?sort=`.
- Chips: remove per-filter, Clear all, focus management.
- URL: deep-link restore, back/forward, replace-vs-push behavior.
- ResultCount updates; skeletons on new query; empty state; error+retry
  keeps last results.
- i18n EN/ZH parity for all `search.*` keys.

## 9. Implementation Order

1. Proto fields + regenerate.
2. Repository: condition builder refactor, keyword condition, sort
   whitelist, trending joins, `CountTemplates` + unit tests.
3. Service: normalization, all four paths, counts, logging.
4. Gateway: `keyword`/`q`/`sort` mapping.
5. DDL indexes (both init.sql copies).
6. Frontend: `useTemplateFilters` → components → Home/Sidebar wiring →
   i18n.
7. FVT + E2E.

## 10. Constraints for Developer

1. **Never commit `backend/server`** — tracked build artifact; `git
   checkout -- backend/server` if modified.
2. Stage only your own files; never `git add -A`.
3. Both init.sql copies in sync in the same commit; DDL idempotent
   (`IF NOT EXISTS`).
4. Sort values map to hardcoded SQL via the whitelist switch — never
   interpolate user input into `ORDER BY`.
5. Escape `%`/`_`/`\` in keywords before building the ILIKE pattern.
6. Keyword cap is 100 **runes**, not bytes (CJK-safe).
7. `total_count`/`private_total_count` are `int32` in proto.
8. Counts are computed only on first-page requests (`page_token == ""`).
9. The mixed-view token format `public:private` and all existing
   parameters must keep working unchanged (AC-10).
10. Logs record keyword length, never keyword content.
11. No new endpoints, no schema changes beyond the two indexes.
12. No monetization surfaces of any kind.
