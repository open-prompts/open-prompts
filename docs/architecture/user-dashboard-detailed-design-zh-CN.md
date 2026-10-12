# 用户仪表盘 — 详细设计

| | |
|---|---|
| **功能 ID** | user-dashboard |
| **版本** | 1.0 |
| **状态** | 已批准 |
| **作者** | 架构 Agent |
| **上游** | [架构设计](user-dashboard-architecture-zh-CN.md) · [需求文档](../design/user-dashboard-requirements-zh-CN.md) · [UI/UX 设计](../design/user-dashboard-ui-ux-design-zh-CN.md) |
| **下游** | 开发 Agent |

## 1. Proto 契约

### 1.1 新文件：`backend/api/proto/v1/dashboard.proto`

```protobuf
syntax = "proto3";

package v1;

option go_package = "open-prompts/backend/api/proto/v1;v1";

// DashboardService 服务于已认证用户的个人工作区：
// 聚合其拥有资源的计数与最近动态。所有 RPC 均需用户 token，
// 身份从上下文推导。
service DashboardService {
  // GetDashboardOverview 返回调用者的资源计数与模板、提示词、
  // 赞、收藏中最近的 10 条动态。合集特性缺失时降级为 0。
  rpc GetDashboardOverview(GetDashboardOverviewRequest) returns (GetDashboardOverviewResponse);
}

// GetDashboardOverviewRequest 为空；调用者取自鉴权上下文。
message GetDashboardOverviewRequest {}

// DashboardActivityType 枚举最近动态条目类型。
enum DashboardActivityType {
  DASHBOARD_ACTIVITY_TYPE_UNSPECIFIED = 0;
  // 模板创建或更新（前端通过 created_at == updated_at 区分）。
  DASHBOARD_ACTIVITY_TYPE_TEMPLATE = 1;
  // 从模板实例化的提示词。
  DASHBOARD_ACTIVITY_TYPE_PROMPT = 2;
  // 用户模板收到的赞。
  DASHBOARD_ACTIVITY_TYPE_LIKE = 3;
  // 用户模板收到的收藏。
  DASHBOARD_ACTIVITY_TYPE_FAVORITE = 4;
}

// DashboardActivityItem 是最近动态流中的一条。
message DashboardActivityItem {
  DashboardActivityType type = 1;
  // 主实体 ID（模板 id、提示词 id，或被赞/收藏的模板 id）。
  string entity_id = 2;
  // 展示标题：模板标题，或提示词的来源模板标题。
  string entity_title = 3;
  // 动态发生时间（模板为 updated_at，其余为 created_at）。
  google.protobuf.Timestamp timestamp = 4;
  // 仅 PROMPT 条目：提示词实例化自的模板
  // （链接到 /templates/{template_id}）。其余为空。
  string template_id = 5;
}

// GetDashboardOverviewResponse 聚合调用者的工作区。
message GetDashboardOverviewResponse {
  int32 template_count = 1;   // 拥有的全部模板（任意可见性/类型）
  int32 prompt_count = 2;     // 全部已实例化提示词
  int32 collection_count = 3; // 活跃合集（特性缺失时为 0）
  int32 api_key_count = 4;    // 活跃且未过期的 API 密钥
  repeated DashboardActivityItem recent_activity = 5; // 最新优先，最多 10 条
}
```

在文件顶部（`option go_package` 之后）导入 `google/protobuf/timestamp.proto`。

**JSON 线上示例**（`GET /api/v1/dashboard/overview`）：

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

所有计数均为 **int32**（protojson 会把 int64 序列化为 JSON 字符串；前端与 FVT 以数字比较计数）。

### 1.2 `backend/api/proto/v1/prompt.proto` — 可加性字段

```protobuf
message Prompt {
  string id = 1;
  string template_id = 2;
  int32 version_id = 3;
  string owner_id = 4;
  map<string, string> variables = 5;
  google.protobuf.Timestamp created_at = 6;
  string content = 7;
  // 来源模板标题（为列表行反规范化；旧负载中为空）。
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
  // 排序："" 或 "created"（默认，created_at DESC）或 "updated"（updated_at DESC）。
  string sort = 10;
}
```

`template_title` 用于提示词页行（"from {{title}}"）与查看弹窗标题，免去客户端模板查找。`models.Prompt` 增加 `TemplateTitle string`；`PromptRepository.List` 增加 `JOIN templates t ON p.template_id = t.id` 并选择 `t.title AS template_title`；`promptModelToProto` 负责映射。

## 2. 数据库 DDL

**无。** 本功能是对既有表（`templates`、`prompts`、`template_likes`、`template_favorites`、`api_keys`，以及可选的 `collections`）的只读聚合。两个 `deployment/init.sql` 均不改动。既有索引覆盖全部查询：`idx_templates_owner_id`、`idx_prompts_owner_id`、`template_likes (user_id, template_id)` 主键、`template_favorites (user_id, template_id)` 主键、`idx_api_keys_user_id`。

## 3. 后端分层

### 3.1 模型

无新模型。`DashboardOverview` 是 service 层结构体：

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
    TemplateID   string // 仅提示词
}
```

### 3.2 仓库 — 新增 `backend/internal/repository/dashboard_repository.go`

```go
type DashboardRepository struct {
    db *sql.DB
}

func NewDashboardRepository(db *sql.DB) *DashboardRepository

// GetOverview 返回用户计数与最近 10 条动态。
func (r *DashboardRepository) GetOverview(ctx context.Context, userID string) (*service.DashboardOverview, error)
```

**计数 SQL**（单次往返，四个标量查询）：

```sql
-- template_count
SELECT COUNT(*) FROM templates WHERE owner_id = $1;
-- prompt_count
SELECT COUNT(*) FROM prompts WHERE owner_id = $1;
-- collection_count（特性探测：42P01 -> 0）
SELECT COUNT(*) FROM collections WHERE owner_id = $1 AND status = 'active';
-- api_key_count
SELECT COUNT(*) FROM api_keys
WHERE user_id = $1 AND is_active = true
  AND (expires_at IS NULL OR expires_at > NOW());
```

collections 查询遇到 `pgconn.PgError` 码 `42P01`（`undefined_table`）时：记录 `warn "collections table absent; collection_count=0"` 并以 0 继续。其他错误中止请求。

**动态 SQL**（单查询，UNION ALL，最新优先，LIMIT 10）：

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

行映射：`'template'→TEMPLATE`、`'prompt'→PROMPT`、`'like'→LIKE`、`'favorite'→FAVORITE`。

### 3.3 仓库 — `api_key_repository.go` 新增

```go
// CountActiveByUserID 返回活跃且未过期密钥的数量。
func (r *APIKeyRepository) CountActiveByUserID(ctx context.Context, userID string) (int32, error)
```

```sql
SELECT COUNT(*) FROM api_keys
WHERE user_id = $1 AND is_active = true
  AND (expires_at IS NULL OR expires_at > NOW());
```

（仪表盘仓库可直接调用此方法，也可内联上述 SQL；该方法的存在便于 API 密钥页后续复用。）

### 3.4 仓库 — `template_repository.go` 排序分支

`List` 中 `ORDER BY` 变为：

```go
orderBy := "t.created_at DESC"
if filters["sort"] == "updated" {
    orderBy = "t.updated_at DESC"
}
```

`filters["sort"]` 只会是 `""` 或 `"updated"` —— service 在贯通前校验（§3.5）。

### 3.5 服务 — 新增 `backend/internal/service/dashboard.go`

```go
type DashboardService struct {
    v1.UnimplementedDashboardServiceServer
    dashboardRepo *repository.DashboardRepository
    apiKeyRepo    *repository.APIKeyRepository
}

func NewDashboardService(dashboardRepo *repository.DashboardRepository, apiKeyRepo *repository.APIKeyRepository) *DashboardService
```

**`GetDashboardOverview` 伪代码：**

```
userID, err := GetUserIDFromContext(ctx)          // auth.go:315
if err: return nil, Unauthenticated               // 拦截器通常已阻止

overview, err := dashboardRepo.GetOverview(ctx, userID)
if err: return nil, Internal

映射到 proto：
  计数为 int32
  动态条目：type 枚举、entity_id、entity_title、timestamp、
            template_id（仅 PROMPT）
返回响应
```

**`ListPrompts` 鉴权修复**（`prompt.go:754`），插入在任何仓库调用之前：

```
if req.OwnerId != "":
    userID, err := GetUserIDFromContext(ctx)
    if err: return nil, Unauthenticated           // 匿名 + owner_id
    if req.OwnerId != userID: return nil, PermissionDenied
```

**`ListTemplates` 排序贯通**（`prompt.go:344`）：

```
if req.Sort != "" && req.Sort != "created" && req.Sort != "updated":
    return nil, InvalidArgument("sort must be \"created\" or \"updated\"")
filters["sort"] = req.Sort
```

### 3.6 AuthInterceptor

`DashboardService.GetDashboardOverview` **不**加入 `publicRpcMethods`（`auth.go:73–81`），因此拦截器要求有效用户 token。封禁用户在 `VerifyAnyToken` 处失败返回 `PermissionDenied` → HTTP 403。

### 3.7 网关 — `backend/cmd/server/main.go`

**新路由（必须鉴权，模式来自 `main.go:334`）：**

```
GET /api/v1/dashboard/overview
  authHeader := r.Header.Get("Authorization")
  if authHeader == "": 401 "Authorization header required"
  userID, err := VerifyAnyToken(authHeader)
  if err: writeError（401 Unauthenticated / 403 PermissionDenied）
  resp, err := dashboardClient.GetDashboardOverview(ctx, &v1.GetDashboardOverviewRequest{})
  if err: writeError
  writeJSON 200（protojson 序列化）
```

**`GET /api/v1/prompts` 修复（可选鉴权模式，来自 `main.go:281`）：**

```
req := &v1.ListPromptsRequest{ PageSize, PageToken, OwnerId, TemplateId 来自查询参数 }
if req.OwnerId != "":
    authHeader := r.Header.Get("Authorization")
    if authHeader != "":
        userID, err := VerifyAnyToken(authHeader)
        if err: writeError（401/403）并返回
        ctx = ContextWithUserID(ctx, userID)
    // 无 header + owner_id -> service 返回 Unauthenticated（401）
resp, err := promptClient.ListPrompts(ctx, req)
```

**`GET /api/v1/templates`**：将查询参数 `sort` 传入 `ListTemplatesRequest.Sort`。

**装配**：构造 `repository.NewDashboardRepository(db)` 与 `service.NewDashboardService(...)`；在其他服务旁注册 `v1.RegisterDashboardServiceServer(grpcServer, dashboardSvc)`（`main.go:189`）。

## 4. 前端

### 4.1 新增文件

| 文件 | 用途 |
|---|---|
| `frontend/src/pages/dashboard/DashboardLayout.js` | 路由布局：`DashboardNav` + `<Outlet/>`；token 自守卫 → `/login?next=…` |
| `frontend/src/pages/dashboard/OverviewPage.js` | 统计卡片、动态流、快捷操作 |
| `frontend/src/pages/dashboard/TemplatesPage.js` | 自有模板管理（双流合并） |
| `frontend/src/pages/dashboard/PromptsPage.js` | 已实例化提示词管理 |
| `frontend/src/components/dashboard/StatCard.js` | 整卡链接 + 计数 + `aria-label` |
| `frontend/src/components/dashboard/ActivityFeed.js` | 语义化 `<ul>`、按类型图标、相对时间 |
| `frontend/src/components/dashboard/QuickActions.js` | 新建模板 / 新建 API 密钥按钮 |
| `frontend/src/components/dashboard/TemplateManageRow.js` | 带操作的模板行 |
| `frontend/src/components/dashboard/PromptRow.js` | 提示词行（变量摘要、来源链接） |
| `frontend/src/components/dashboard/PromptViewModal.js` | 服务端渲染 `content` 查看器 |
| `frontend/src/components/dashboard/FilterChips.js` | 无障碍 radiogroup 芯片 |
| `frontend/src/utils/collections.js` | `hasCollections()` 模块级缓存探测 |

### 4.2 修改文件

| 文件 | 变更 |
|---|---|
| `frontend/src/App.js` | lazy 路由：`/dashboard` → `DashboardLayout`，索引 → `OverviewPage`，`templates` → `TemplatesPage`，`prompts` → `PromptsPage` |
| `frontend/src/pages/Login.js` | `navigate('/')` → `navigate(next \|\| '/dashboard')`，`next` 取自 `?next=` |
| `frontend/src/components/Sidebar.js` | 已登录用户顶部“我的工作区”入口（链接 `/dashboard`），位于“提示词广场”之上；既有条目不变 |
| `frontend/src/services/api.js` | `getDashboardOverview()`；`getTemplates` 接受 `sort` 参数 |
| `frontend/src/locales/en/translation.json` | `dashboard_v2.*` 块 |
| `frontend/src/locales/zh/translation.json` | `dashboard_v2.*` 块（中文） |

### 4.3 API 客户端新增（`services/api.js`）

```js
getDashboardOverview: () => api.get('/dashboard/overview'),

getTemplates: (params = {}) =>
  api.get('/templates', { params }), // params 现可包含 sort: 'updated'
```

### 4.4 组件树

```
DashboardLayout（自守卫：!token -> /login?next=）
├── DashboardNav                     // 每项一个 NavLink；!hasCollections() 时隐藏“我的合集”
│   ├── 概览 (/dashboard)
│   ├── 我的模板 (/dashboard/templates)
│   ├── 我的提示词 (/dashboard/prompts)
│   ├── 我的合集 (/collections/mine)   // 条件渲染
│   └── API 密钥 (/api-keys)
└── <Outlet/>
    ├── OverviewPage
    │   ├── StatCard ×4              // to=/dashboard/templates, /dashboard/prompts,
    │   │                            // /collections/mine（!hasCollections() 时隐藏）, /api-keys
    │   ├── ActivityFeed
    │   │   └── li ×≤10              // 图标 + 文本 + 相对时间（title=绝对时间）
    │   └── QuickActions
    │       ├── button btn-primary   // 新建模板 -> CreateTemplateModal
    │       └── button btn-ghost     // 新建 API 密钥 -> navigate('/api-keys')
    ├── TemplatesPage
    │   ├── FilterChips              // 全部/公开/私有/系统/用户（客户端）
    │   ├── TemplateManageRow ×N     // 标题链接、徽章、分类、标签、计数、
    │   │                            // 更新日期、编辑/删除/切换可见性
    │   └── LoadMore（双 token）     // token = `${next_page_token}:${private_next_page_token}`
    └── PromptsPage
        ├── FilterChips              // 全部 + 每个来源模板一个芯片（客户端）
        ├── PromptRow ×N             // 变量芯片、来源链接、创建日期、查看/删除
        ├── PromptViewModal          // content 等宽 pre-wrap，复制/删除/关闭
        └── LoadMore
```

**TemplatesPage 双流合并**（对应 Home.js:94–108）：

```js
const merged = [...templates, ...privateTemplates]
  .sort((a, b) => new Date(b.updatedAt) - new Date(a.updatedAt));
const nextToken = privateNextPageToken
  ? `${nextPageToken}:${privateNextPageToken}`
  : nextPageToken;
const hasMore = !!nextToken && nextToken !== ':';
```

**切换可见性**为乐观更新：先翻转本地状态，调用 `updateTemplate`，出错时回滚并 toast 错误。

**PromptViewModal** 在 `<pre className="prompt-content">`（`max-height: 60vh; overflow: auto`）中渲染 `prompt.content`（服务端已替换变量的渲染结果 —— 见架构设计已解决 #3），附复制（`navigator.clipboard`）与删除（确认）按钮；`role="region"`，`aria-label` 取 `dashboard_v2.prompts.modal_title`。

**`hasCollections()`**（`utils/collections.js`）：

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

### 4.5 i18n 键（EN / ZH）

所有新键位于 `dashboard_v2.*` 之下；既有 `dashboard.*`（Sidebar/首页）不动。相对时间复用既有 `time.justNow|minutesAgo|hoursAgo|daysAgo`。下表取值与 UI/UX 设计 §9 JSON 完全一致；`stats.aria` 是架构新增键，用于满足设计的无障碍要求（StatCard `aria-label` "My templates, 12 items"）。

| 键 | EN | ZH |
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

（权威键集为 UI/UX 设计 §9 的 JSON 加上 `stats.aria`；本表与设计之间的任何漂移都是 bug。）

## 5. 错误矩阵

| 场景 | gRPC 码 | HTTP | 前端处理 |
|---|---|---|---|
| 概览无 token | `Unauthenticated` | 401 | 页面守卫在调用前重定向到登录页 |
| 概览 token 无效 | `Unauthenticated` | 401 | 错误横幅 + 重试 |
| 封禁用户 | `PermissionDenied` | 403 | 错误横幅（展示服务端消息） |
| `ListPrompts` owner_id ≠ 调用者 | `PermissionDenied` | 403 | 仪表盘 UI 不可达；FVT 断言 403 |
| `ListPrompts` 匿名 + owner_id | `Unauthenticated` | 401 | 仪表盘 UI 不可达；FVT 断言 401 |
| `sort` 值非法 | `InvalidArgument` | 400 | 仪表盘 UI 不可达（固定 `updated`） |
| 后端/数据库故障 | `Internal` | 500 | 错误横幅 + 重试 |
| collections 表缺失 | —（降级） | 200 且 `collectionCount: 0` | 通过 `hasCollections()` 隐藏“我的合集” |

## 6. 部署

1. 重新生成 proto 桩代码（`dashboard.proto`、`prompt.proto` 字段 10）。
2. 重新构建后端；无迁移、无配置变更。
3. 前端发布新页面/路由；`hasCollections()` 自动适配 template-collections 是否已部署。

## 7. SDK / 公共 API 文档

`GET /api/v1/dashboard/overview` 为用户作用域接口，写入公共 API 参考（需鉴权；响应结构见 §1.1）。`GET /api/v1/templates` 文档补充 `sort=updated` 查询参数。`ListPrompts` 鉴权变更是**破坏性安全修复** —— 枚举他人提示词的第三方调用方将收到 403；发布说明必须明确指出。

## 8. 测试策略

**FVT**（`tests/fvt.py::test_user_dashboard`，runner 位于 `:1781`，`get_auth_token` 位于 `:74`）：

| AC | 断言 |
|---|---|
| AC-1 | 匿名 `GET /dashboard` 重定向到 `/login?next=/dashboard`（页面守卫）；API 返回 401 |
| AC-2 | 概览计数与种子数据一致（测试用户创建的模板/提示词/密钥） |
| AC-3 | `recent_activity` 最新优先、≤10 条、类型合法 |
| AC-4 | 模板页列出公开 + 私有自有模板，按 `updated_at` 降序 |
| AC-5 | 可见性切换翻转徽章；API 错误 → 回滚 + toast |
| AC-6 | 删除打开回显标题的确认弹窗；确认后移除该行 |
| AC-7 | 提示词页按最新优先列出已实例化提示词及变量摘要 |
| AC-8 | 提示词查看弹窗展示已替换变量的服务端渲染内容 |
| AC-9 | `GET /api/v1/prompts?owner_id=<其他用户>` → 403 |
| AC-10 | `GET /api/v1/prompts?owner_id=<本人>` 无 token → 401 |
| AC-11 | `GET /api/v1/templates?sort=updated` 返回 `updated_at` 降序 |
| AC-12 | EN/ZH：两种语言均存在全部 `dashboard_v2.*` 键 |

**单元测试**：仓库计数（含 `42P01` → 0）、UNION 排序/limit、`CountActiveByUserID` 过期逻辑、服务鉴权分支、`sort` 校验、网关处理器状态码。

## 9. 实现顺序

1. Proto：`dashboard.proto` + `ListTemplatesRequest.sort` + `Prompt.template_title`；重新生成。
2. 仓库：`DashboardRepository.GetOverview`、`APIKeyRepository.CountActiveByUserID`、模板仓库排序分支、提示词仓库 `template_title` JOIN。
3. 服务：`DashboardService`；`ListPrompts` 鉴权；`ListTemplates` 排序贯通；`promptModelToProto` 标题映射。
4. 网关：`/api/v1/dashboard/overview` 路由；`/prompts` 可选鉴权；`/templates` 透传 sort；装配。
5. 前端：工具（`collections.js`）、API 客户端、页面/组件、路由、Login 跳转、Sidebar 入口、i18n。
6. FVT + 单元测试。

## 10. 给开发者的约束

- **不要改动**既有 `dashboard.*` i18n 键（Sidebar/首页所有）；新键只能是 `dashboard_v2.*`。
- 所有计数 **int32**；绝不用 int64（protojson 字符串怪癖）。
- 概览请求**不携带任何用户标识** —— 身份仅来自鉴权上下文。
- `ListPrompts` 鉴权检查必须在**任何仓库调用之前**执行。
- 合集降级必须“静默但有日志”（`42P01` → 0 + warn），绝不能 500。
- TemplatesPage 必须合并双流并按 Home.js 的方式组合分页 token（`public:private`）。
- PromptViewModal 直接使用 `prompt.content` —— 不做客户端变量替换。
- 无新 DDL；不要修改 `deployment/init.sql`。
