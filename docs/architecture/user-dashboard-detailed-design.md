# User Dashboard — Detailed Design

| | |
|---|---|
| **Feature ID** | user-dashboard |
| **Version** | 1.0 |
| **Status** | Approved |
| **Author** | Architect Agent |
| **Upstream** | [Architecture](user-dashboard-architecture.md) · [Requirements](../design/user-dashboard-requirements.md) · [UI/UX Design](../design/user-dashboard-ui-ux-design.md) |
| **Downstream** | Developer Agent |

## 1. Proto Contract

### 1.1 New file: `backend/api/proto/v1/dashboard.proto`

```protobuf
syntax = "proto3";

package v1;

option go_package = "open-prompts/backend/api/proto/v1;v1";

// DashboardService serves the authenticated user's personal workspace:
// aggregate counts and recent activity across their owned resources.
// All RPCs require a user token; identity is derived from the context.
service DashboardService {
  // GetDashboardOverview returns the caller's resource counts and the
  // 10 most recent activity items across templates, prompts, likes,
  // and favorites. Collections degrade to 0 when the feature is absent.
  rpc GetDashboardOverview(GetDashboardOverviewRequest) returns (GetDashboardOverviewResponse);
}

// GetDashboardOverviewRequest is empty; the caller is taken from auth context.
message GetDashboardOverviewRequest {}

// DashboardActivityType enumerates recent-activity item kinds.
enum DashboardActivityType {
  DASHBOARD_ACTIVITY_TYPE_UNSPECIFIED = 0;
  // Template created or updated (frontend distinguishes via created_at == updated_at).
  DASHBOARD_ACTIVITY_TYPE_TEMPLATE = 1;
  // Prompt instantiated from a template.
  DASHBOARD_ACTIVITY_TYPE_PROMPT = 2;
  // Like received on the user's template.
  DASHBOARD_ACTIVITY_TYPE_LIKE = 3;
  // Favorite received on the user's template.
  DASHBOARD_ACTIVITY_TYPE_FAVORITE = 4;
}

// DashboardActivityItem is one entry in the recent-activity feed.
message DashboardActivityItem {
  DashboardActivityType type = 1;
  // ID of the primary entity (template id, prompt id, or liked/favorited template id).
  string entity_id = 2;
  // Display title: template title, or source template title for prompts.
  string entity_title = 3;
  // When the activity happened (template updated_at, else created_at).
  google.protobuf.Timestamp timestamp = 4;
  // For PROMPT items: the template the prompt was instantiated from
  // (links to /templates/{template_id}). Empty otherwise.
  string template_id = 5;
}

// GetDashboardOverviewResponse aggregates the caller's workspace.
message GetDashboardOverviewResponse {
  int32 template_count = 1;   // all owned templates (any visibility/type)
  int32 prompt_count = 2;     // all instantiated prompts
  int32 collection_count = 3; // active collections (0 when feature absent)
  int32 api_key_count = 4;    // active, unexpired API keys
  repeated DashboardActivityItem recent_activity = 5; // newest first, max 10
}
```

Import `google/protobuf/timestamp.proto` at the top of the file (after `option go_package`).

**JSON wire example** (`GET /api/v1/dashboard/overview`):

```json
{
  "templateCount": 12,
  "promptCount": 34,
  "collectionCount": 2,
  "apiKeyCount": 1,
  "recentActivity": [
    {
      "type": "DASHBOARD_ACTIVITY_TYPE_TEMPLATE",
      "entityId": "0b6ba7b2-...",
      "entityTitle": "Code Review Assistant",
      "timestamp": "2025-01-15T09:30:00Z"
    },
    {
      "type": "DASHBOARD_ACTIVITY_TYPE_PROMPT",
      "entityId": "3f2c9d40-...",
      "entityTitle": "Code Review Assistant",
      "templateId": "0b6ba7b2-...",
      "timestamp": "2025-01-15T09:12:00Z"
    },
    {
      "type": "DASHBOARD_ACTIVITY_TYPE_LIKE",
      "entityId": "0b6ba7b2-...",
      "entityTitle": "Code Review Assistant",
      "timestamp": "2025-01-14T18:45:00Z"
    }
  ]
}
```

All counts are **int32** (protojson serializes int64 as a JSON string; the frontend and FVT compare counts as numbers).

### 1.2 `backend/api/proto/v1/prompt.proto` — additive fields

```protobuf
message Prompt {
  string id = 1;
  string template_id = 2;
  int32 version_id = 3;
  string owner_id = 4;
  map<string, string> variables = 5;
  google.protobuf.Timestamp created_at = 6;
  string content = 7;
  // Title of the source template (denormalized for list rows; empty on old payloads).
  string template_title = 8;
}

message ListTemplatesRequest {
  int32 page_size = 1;
  string page_token = 2;
  Visibility visibility = 3;
  string owner_id = 4;
  string category = 5;
  repeated string tags = 6;
  bool my_likes = 7;
  bool my_favorites = 8;
  string language = 9;
  // Sort order: "" or "created" (default, created_at DESC) or "updated" (updated_at DESC).
  string sort = 10;
}
```

`template_title` feeds the prompts-page row ("from {{title}}") and the view-modal title without a client-side template lookup. `models.Prompt` gains `TemplateTitle string`; `PromptRepository.List` adds `JOIN templates t ON p.template_id = t.id` and selects `t.title AS template_title`; `promptModelToProto` maps it.

## 2. Database DDL

**None.** The feature is a read-only aggregation over existing tables (`templates`, `prompts`, `template_likes`, `template_favorites`, `api_keys`, and optionally `collections`). Both `deployment/init.sql` files are untouched. Existing indexes cover every query: `idx_templates_owner_id`, `idx_prompts_owner_id`, `template_likes (user_id, template_id)` PK, `template_favorites (user_id, template_id)` PK, `idx_api_keys_user_id`.

## 3. Backend Layers

### 3.1 Models

No new models. `DashboardOverview` is a service-layer struct:

```go
type DashboardOverview struct {
    TemplateCount  int32
    PromptCount    int32
    CollectionCount int32
    APIKeyCount    int32
    Activity       []DashboardActivity
}

type DashboardActivity struct {
    Type         DashboardActivityType
    EntityID     string
    EntityTitle  string
    Timestamp    time.Time
    TemplateID   string // prompts only
}
```

### 3.2 Repository — new `backend/internal/repository/dashboard_repository.go`

```go
type DashboardRepository struct {
    db *sql.DB
}

func NewDashboardRepository(db *sql.DB) *DashboardRepository

// GetOverview returns the user's counts and the 10 most recent activity items.
func (r *DashboardRepository) GetOverview(ctx context.Context, userID string) (*service.DashboardOverview, error)
```

**Counts SQL** (single round trip, four scalar queries):

```sql
-- template_count
SELECT COUNT(*) FROM templates WHERE owner_id = $1;
-- prompt_count
SELECT COUNT(*) FROM prompts WHERE owner_id = $1;
-- collection_count (feature-detection: 42P01 -> 0)
SELECT COUNT(*) FROM collections WHERE owner_id = $1 AND status = 'active';
-- api_key_count
SELECT COUNT(*) FROM api_keys
WHERE user_id = $1 AND is_active = true
  AND (expires_at IS NULL OR expires_at > NOW());
```

On `pgconn.PgError` code `42P01` (`undefined_table`) for the collections query: log `warn "collections table absent; collection_count=0"` and continue with 0. Any other error aborts the request.

**Activity SQL** (one query, UNION ALL, newest first, LIMIT 10):

```sql
SELECT * FROM (
  SELECT 'template' AS type, t.id::text AS entity_id, t.title AS entity_title,
         t.updated_at AS timestamp, '' AS template_id
  FROM templates t
  WHERE t.owner_id = $1
  UNION ALL
  SELECT 'prompt', p.id::text, t.title, p.created_at, t.id::text
  FROM prompts p
  JOIN templates t ON t.id = p.template_id
  WHERE p.owner_id = $1
  UNION ALL
  SELECT 'like', t.id::text, t.title, l.created_at, ''
  FROM template_likes l
  JOIN templates t ON t.id = l.template_id
  WHERE t.owner_id = $1
  UNION ALL
  SELECT 'favorite', t.id::text, t.title, f.created_at, ''
  FROM template_favorites f
  JOIN templates t ON t.id = f.template_id
  WHERE t.owner_id = $1
) activity
ORDER BY timestamp DESC
LIMIT 10;
```

Rows map `'template'→TEMPLATE`, `'prompt'→PROMPT`, `'like'→LIKE`, `'favorite'→FAVORITE`.

### 3.3 Repository — `api_key_repository.go` addition

```go
// CountActiveByUserID returns the number of active, unexpired keys.
func (r *APIKeyRepository) CountActiveByUserID(ctx context.Context, userID string) (int32, error)
```

```sql
SELECT COUNT(*) FROM api_keys
WHERE user_id = $1 AND is_active = true
  AND (expires_at IS NULL OR expires_at > NOW());
```

(The dashboard repository may call this directly or inline the SQL above; the method exists so the API Keys page can reuse it later.)

### 3.4 Repository — `template_repository.go` sort branch

In `List`, the `ORDER BY` becomes:

```go
orderBy := "t.created_at DESC"
if filters["sort"] == "updated" {
    orderBy = "t.updated_at DESC"
}
```

`filters["sort"]` is only ever `""` or `"updated"` — the service validates before plumbing (§3.5).

### 3.5 Service — new `backend/internal/service/dashboard.go`

```go
type DashboardService struct {
    v1.UnimplementedDashboardServiceServer
    dashboardRepo *repository.DashboardRepository
    apiKeyRepo    *repository.APIKeyRepository
}

func NewDashboardService(dashboardRepo *repository.DashboardRepository, apiKeyRepo *repository.APIKeyRepository) *DashboardService
```

**`GetDashboardOverview` pseudocode:**

```
userID, err := GetUserIDFromContext(ctx)          // auth.go:315
if err: return nil, Unauthenticated               // interceptor normally prevents this

overview, err := dashboardRepo.GetOverview(ctx, userID)
if err: return nil, Internal

map to proto:
  counts as int32
  activity items: type enum, entity_id, entity_title, timestamp,
                  template_id (PROMPT only)
return response
```

**`ListPrompts` authorization fix** (`prompt.go:754`), inserted before any repository call:

```
if req.OwnerId != "":
    userID, err := GetUserIDFromContext(ctx)
    if err: return nil, Unauthenticated           // anonymous + owner_id
    if req.OwnerId != userID: return nil, PermissionDenied
```

**`ListTemplates` sort plumbing** (`prompt.go:344`):

```
if req.Sort != "" && req.Sort != "created" && req.Sort != "updated":
    return nil, InvalidArgument("sort must be \"created\" or \"updated\"")
filters["sort"] = req.Sort
```

### 3.6 AuthInterceptor

`DashboardService.GetDashboardOverview` is **not** added to `publicRpcMethods` (`auth.go:73–81`), so the interceptor requires a valid user token. Banned users fail `VerifyAnyToken` with `PermissionDenied` → HTTP 403.

### 3.7 Gateway — `backend/cmd/server/main.go`

**New route (required auth, pattern from `main.go:334`):**

```
GET /api/v1/dashboard/overview
  authHeader := r.Header.Get("Authorization")
  if authHeader == "": 401 "Authorization header required"
  userID, err := VerifyAnyToken(authHeader)
  if err: writeError (401 Unauthenticated / 403 PermissionDenied)
  resp, err := dashboardClient.GetDashboardOverview(ctx, &v1.GetDashboardOverviewRequest{})
  if err: writeError
  writeJSON 200 (protojson marshal)
```

**`GET /api/v1/prompts` fix (optional-auth pattern from `main.go:281`):**

```
req := &v1.ListPromptsRequest{ PageSize, PageToken, OwnerId, TemplateId from query }
if req.OwnerId != "":
    authHeader := r.Header.Get("Authorization")
    if authHeader != "":
        userID, err := VerifyAnyToken(authHeader)
        if err: writeError (401/403) and return
        ctx = ContextWithUserID(ctx, userID)
    // no header + owner_id -> service returns Unauthenticated (401)
resp, err := promptClient.ListPrompts(ctx, req)
```

**`GET /api/v1/templates`**: pass `sort` from query into `ListTemplatesRequest.Sort`.

**Wiring**: construct `repository.NewDashboardRepository(db)` and `service.NewDashboardService(...)`; register `v1.RegisterDashboardServiceServer(grpcServer, dashboardSvc)` alongside the other services (`main.go:189`).

## 4. Frontend

### 4.1 New files

| File | Purpose |
|---|---|
| `frontend/src/pages/dashboard/DashboardLayout.js` | Route layout: `DashboardNav` + `<Outlet/>`; token self-guard → `/login?next=…` |
| `frontend/src/pages/dashboard/OverviewPage.js` | Stat cards, activity feed, quick actions |
| `frontend/src/pages/dashboard/TemplatesPage.js` | Owned-template management (dual-stream merge) |
| `frontend/src/pages/dashboard/PromptsPage.js` | Instantiated-prompt management |
| `frontend/src/components/dashboard/StatCard.js` | Whole-card link + count + `aria-label` |
| `frontend/src/components/dashboard/ActivityFeed.js` | Semantic `<ul>`, per-type icon, relative time |
| `frontend/src/components/dashboard/QuickActions.js` | New template / New API key buttons |
| `frontend/src/components/dashboard/TemplateManageRow.js` | Template row with actions |
| `frontend/src/components/dashboard/PromptRow.js` | Prompt row (variables summary, source link) |
| `frontend/src/components/dashboard/PromptViewModal.js` | Server-rendered `content` viewer |
| `frontend/src/components/dashboard/FilterChips.js` | Accessible radiogroup chips |
| `frontend/src/utils/collections.js` | `hasCollections()` module-cached probe |

### 4.2 Modified files

| File | Change |
|---|---|
| `frontend/src/App.js` | Lazy routes: `/dashboard` → `DashboardLayout`, index → `OverviewPage`, `templates` → `TemplatesPage`, `prompts` → `PromptsPage` |
| `frontend/src/pages/Login.js` | `navigate('/')` → `navigate(next \|\| '/dashboard')` where `next` comes from `?next=` |
| `frontend/src/components/Sidebar.js` | "My Workspace" entry at top for `isAuthenticated` (links `/dashboard`), above "Prompt Square"; existing entries unchanged |
| `frontend/src/services/api.js` | `getDashboardOverview()`; `getTemplates` accepts `sort` param |
| `frontend/src/locales/en/translation.json` | `dashboard_v2.*` block |
| `frontend/src/locales/zh/translation.json` | `dashboard_v2.*` block (ZH) |

### 4.3 API client additions (`services/api.js`)

```js
getDashboardOverview: () => api.get('/dashboard/overview'),

getTemplates: (params = {}) =>
  api.get('/templates', { params }), // params may now include sort: 'updated'
```

### 4.4 Component tree

```
DashboardLayout (self-guard: !token -> /login?next=)
├── DashboardNav                     // NavLink per entry; My Collections hidden if !hasCollections()
│   ├── Overview (/dashboard)
│   ├── My Templates (/dashboard/templates)
│   ├── My Prompts (/dashboard/prompts)
│   ├── My Collections (/collections/mine)   // conditional
│   └── API Keys (/api-keys)
└── <Outlet/>
    ├── OverviewPage
    │   ├── StatCard ×4              // to=/dashboard/templates, /dashboard/prompts,
    │   │                            // /collections/mine (hidden if !hasCollections()), /api-keys
    │   ├── ActivityFeed
    │   │   └── li ×≤10              // icon + text + relative time (title=absolute)
    │   └── QuickActions
    │       ├── button btn-primary   // New template -> CreateTemplateModal
    │       └── button btn-ghost     // New API key -> navigate('/api-keys')
    ├── TemplatesPage
    │   ├── FilterChips              // All/Public/Private/System/User (client-side)
    │   ├── TemplateManageRow ×N     // title link, badges, category, tags, counts,
    │   │                            // updated date, Edit/Delete/Toggle-visibility
    │   └── LoadMore (dual-token)    // token = `${next_page_token}:${private_next_page_token}`
    └── PromptsPage
        ├── FilterChips              // All + one chip per source template (client-side)
        ├── PromptRow ×N             // variables chips, source link, created date, View/Delete
        ├── PromptViewModal          // content pre-wrap monospace, Copy/Delete/Close
        └── LoadMore
```

**TemplatesPage dual-stream merge** (mirrors Home.js:94–108):

```js
const merged = [...templates, ...privateTemplates]
  .sort((a, b) => new Date(b.updatedAt) - new Date(a.updatedAt));
const nextToken = privateNextPageToken
  ? `${nextPageToken}:${privateNextPageToken}`
  : nextPageToken;
const hasMore = !!nextToken && nextToken !== ':';
```

**Toggle visibility** is optimistic: flip local state, call `updateTemplate`, on error roll back and toast the error.

**PromptViewModal** renders `prompt.content` (server-rendered with variables substituted — see Architecture Resolved #3) in `<pre className="prompt-content">` with `max-height: 60vh; overflow: auto`, plus Copy (`navigator.clipboard`) and Delete (confirm) buttons; `role="region"`, `aria-label` from `dashboard_v2.prompts.modal_title`.

**`hasCollections()`** (`utils/collections.js`):

```js
let cached = null;
export const hasCollections = async () => {
  if (cached !== null) return cached;
  try {
    await api.get('/collections', { params: { page_size: 1 } });
    cached = true;
  } catch (e) {
    cached = e.response?.status !== 404;
  }
  return cached;
};
```

### 4.5 i18n keys (EN / ZH)

All new keys under `dashboard_v2.*`; existing `dashboard.*` (Sidebar/Home) untouched. Relative time reuses existing `time.justNow|minutesAgo|hoursAgo|daysAgo`. Values below follow the UI/UX design §9 JSON verbatim; `stats.aria` is an architecture addition required by the design's a11y spec (StatCard `aria-label` "My templates, 12 items").

| Key | EN | ZH |
|---|---|---|
| `dashboard_v2.title` | My Workspace | 我的工作区 |
| `dashboard_v2.nav.overview` | Overview | 概览 |
| `dashboard_v2.nav.templates` | My Templates | 我的模板 |
| `dashboard_v2.nav.prompts` | My Prompts | 我的提示词 |
| `dashboard_v2.nav.collections` | My Collections | 我的合集 |
| `dashboard_v2.nav.api_keys` | API Keys | API 密钥 |
| `dashboard_v2.stats.templates` | Templates | 模板 |
| `dashboard_v2.stats.prompts` | Prompts | 提示词 |
| `dashboard_v2.stats.collections` | Collections | 合集 |
| `dashboard_v2.stats.api_keys` | API Keys | API 密钥 |
| `dashboard_v2.stats.aria` | {{label}}, {{count}} items | {{label}}，{{count}} 项 |
| `dashboard_v2.activity.title` | Recent activity | 最近动态 |
| `dashboard_v2.activity.empty` | No activity yet — your actions will appear here. | 还没有动态 —— 你的操作会显示在这里。 |
| `dashboard_v2.activity.template_created` | Created "{{title}}" | 创建了“{{title}}” |
| `dashboard_v2.activity.template_updated` | Updated "{{title}}" | 更新了“{{title}}” |
| `dashboard_v2.activity.prompt_used` | Used "{{title}}" | 使用了“{{title}}” |
| `dashboard_v2.activity.liked` | Liked "{{title}}" | 赞了“{{title}}” |
| `dashboard_v2.activity.favorited` | Favorited "{{title}}" | 收藏了“{{title}}” |
| `dashboard_v2.quick.new_template` | New template | 新建模板 |
| `dashboard_v2.quick.new_api_key` | New API key | 新建 API 密钥 |
| `dashboard_v2.templates.filter.all` | All | 全部 |
| `dashboard_v2.templates.filter.public` | Public | 公开 |
| `dashboard_v2.templates.filter.private` | Private | 私有 |
| `dashboard_v2.templates.filter.system` | System | 系统 |
| `dashboard_v2.templates.filter.user` | User | 用户 |
| `dashboard_v2.templates.updated` | Updated {{date}} | 更新于 {{date}} |
| `dashboard_v2.templates.empty` | You haven't created any templates yet. | 你还没有创建任何模板。 |
| `dashboard_v2.templates.empty_cta` | Create a template | 创建模板 |
| `dashboard_v2.templates.delete_confirm` | Delete "{{title}}"? This cannot be undone. | 删除“{{title}}”？此操作不可撤销。 |
| `dashboard_v2.prompts.filter` | All templates | 全部模板 |
| `dashboard_v2.prompts.from` | from "{{title}}" | 来自“{{title}}” |
| `dashboard_v2.prompts.created` | Created {{date}} | 创建于 {{date}} |
| `dashboard_v2.prompts.empty` | No prompts yet — use a template to create one. | 还没有提示词 —— 使用模板创建一个吧。 |
| `dashboard_v2.prompts.empty_cta` | Browse the Prompt Square | 去提示词广场看看 |
| `dashboard_v2.prompts.view` | View | 查看 |
| `dashboard_v2.prompts.delete_confirm` | Delete this prompt? This cannot be undone. | 删除这条提示词？此操作不可撤销。 |
| `dashboard_v2.prompts.modal_title` | Prompt from "{{title}}" | 来自“{{title}}”的提示词 |
| `dashboard_v2.prompts.variables` | Variables | 变量 |
| `dashboard_v2.prompts.copy` | Copy | 复制 |
| `dashboard_v2.error.fetch` | Failed to load dashboard. | 加载仪表盘失败。 |
| `dashboard_v2.error.retry` | Retry | 重试 |

(The authoritative key set is the UI/UX design §9 JSON plus `stats.aria`; any drift between this table and the design is a bug.)

## 5. Error Matrix

| Scenario | gRPC code | HTTP | Frontend handling |
|---|---|---|---|
| Overview without token | `Unauthenticated` | 401 | Page guard redirects to login before the call |
| Overview with invalid token | `Unauthenticated` | 401 | Error banner + Retry |
| Banned user | `PermissionDenied` | 403 | Error banner (message from server) |
| `ListPrompts` owner_id ≠ caller | `PermissionDenied` | 403 | Not reachable from dashboard UI; FVT asserts 403 |
| `ListPrompts` anonymous + owner_id | `Unauthenticated` | 401 | Not reachable from dashboard UI; FVT asserts 401 |
| `sort` invalid value | `InvalidArgument` | 400 | Not reachable from dashboard UI (fixed `updated`) |
| Backend/DB failure | `Internal` | 500 | Error banner + Retry |
| Collections table absent | — (degraded) | 200 with `collectionCount: 0` | "My Collections" hidden via `hasCollections()` |

## 6. Deployment

1. Regenerate proto stubs (`dashboard.proto`, `prompt.proto` field 10).
2. Rebuild backend; no migration, no config change.
3. Frontend ships the new pages/routes; `hasCollections()` adapts to whether template-collections is deployed.

## 7. SDK / Public API Docs

`GET /api/v1/dashboard/overview` is user-scoped and documented in the public API reference (auth required; response shape as §1.1). The `sort=updated` query parameter is documented on `GET /api/v1/templates`. The `ListPrompts` authorization change is a **breaking security fix** — third-party callers enumerating other users' prompts now receive 403; release notes must call this out.

## 8. Test Strategy

**FVT** (`tests/fvt.py::test_user_dashboard`, runner at `:1781`, `get_auth_token` at `:74`):

| AC | Assertion |
|---|---|
| AC-1 | Anonymous `GET /dashboard` redirects to `/login?next=/dashboard` (page guard); API returns 401 |
| AC-2 | Overview counts match seeded data (templates/prompts/keys created by the test user) |
| AC-3 | `recent_activity` newest-first, ≤10 items, types valid |
| AC-4 | Templates page lists public + private owned templates sorted by `updated_at` desc |
| AC-5 | Visibility toggle flips badge; API error → rollback + toast |
| AC-6 | Delete opens confirm modal echoing title; confirm removes row |
| AC-7 | Prompts page lists instantiated prompts newest-first with variables summary |
| AC-8 | Prompt view modal shows server-rendered content with variables substituted |
| AC-9 | `GET /api/v1/prompts?owner_id=<other user>` → 403 |
| AC-10 | `GET /api/v1/prompts?owner_id=<me>` without token → 401 |
| AC-11 | `GET /api/v1/templates?sort=updated` returns `updated_at`-desc order |
| AC-12 | EN/ZH: all `dashboard_v2.*` keys present in both locales |

**Unit tests**: repository counts (incl. `42P01` → 0), UNION ordering/limit, `CountActiveByUserID` expiry logic, service authz branches, `sort` validation, gateway handler status codes.

## 9. Implementation Order

1. Proto: `dashboard.proto` + `ListTemplatesRequest.sort` + `Prompt.template_title`; regenerate.
2. Repositories: `DashboardRepository.GetOverview`, `APIKeyRepository.CountActiveByUserID`, template repo sort branch, prompt repo `template_title` JOIN.
3. Services: `DashboardService`; `ListPrompts` authz; `ListTemplates` sort plumbing; `promptModelToProto` title mapping.
4. Gateway: `/api/v1/dashboard/overview` route; `/prompts` optional-auth; `/templates` sort passthrough; wiring.
5. Frontend: utils (`collections.js`), api client, pages/components, routes, Login redirect, Sidebar entry, i18n.
6. FVT + unit tests.

## 10. Constraints for Developer

- **Do not touch** the existing `dashboard.*` i18n keys (Sidebar/Home own them); new keys are `dashboard_v2.*` only.
- All counts **int32**; never int64 (protojson string quirk).
- The overview request carries **no user identifier** — identity only from auth context.
- `ListPrompts` authz check must run **before** any repository call.
- Collections degradation must be silent-but-logged (`42P01` → 0 + warn), never a 500.
- TemplatesPage must merge both streams and combine pagination tokens exactly as Home.js does (`public:private`).
- PromptViewModal uses `prompt.content` as-is — no client-side variable substitution.
- No new DDL; do not modify `deployment/init.sql`.
