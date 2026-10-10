# 通知系统 — 详细设计

- **功能 ID：** `notifications`
- **版本：** 1.0
- **状态：** 已批准进入实现
- **作者：** 架构 Agent
- **上游文档：**[需求分析](../design/notifications-requirements-zh-CN.md) · [UI/UX 设计](../design/notifications-ui-ux-design-zh-CN.md)
- **架构文档：**[架构设计（中文）](./notifications-architecture-zh-CN.md) · [Architecture Design (EN)](./notifications-architecture.md)

---

## 1. Proto 契约

新文件 `backend/api/proto/v1/notification.proto`（包名 `openprompts.v1`，与 `prompt.proto`、`admin.proto` 一致）。事件创建**不是** RPC —— 它是点赞/收藏/Fork 服务路径中的内部仓储调用。

```protobuf
syntax = "proto3";

package openprompts.v1;

option go_package = "open-prompts/backend/api/proto/v1";

// NotificationService 提供按用户的互动通知。
// 所有 RPC 均需认证；接收者永远取自认证用户，
// 绝不来自请求字段。
service NotificationService {
  // 列出当前用户的通知。
  rpc ListNotifications(ListNotificationsRequest) returns (ListNotificationsResponse);
  // 当前用户的未读徽标计数。
  rpc GetUnreadNotificationCount(GetUnreadNotificationCountRequest) returns (GetUnreadNotificationCountResponse);
  // 标记单条通知已读（幂等）。
  rpc MarkNotificationRead(MarkNotificationReadRequest) returns (MarkNotificationReadResponse);
  // 全部标记已读。
  rpc MarkAllNotificationsRead(MarkAllNotificationsReadRequest) returns (MarkAllNotificationsReadResponse);
  // 删除单条通知。
  rpc DeleteNotification(DeleteNotificationRequest) returns (DeleteNotificationResponse);
}

message ListNotificationsRequest {
  string filter = 1;      // "all"（默认）| "unread"
  int32 page_size = 2;    // 1-100，默认 20
  string page_token = 3;  // 字符串形式的 offset（现有惯例）
}

message Notification {
  string id = 1;
  string type = 2;               // "like" | "favorite" | "fork"
  string actor_id = 3;
  string actor_display_name = 4; // 事件发生时的快照
  string actor_avatar = 5;       // 实时联表，可为空
  string template_id = 6;
  string template_title = 7;     // 实时联表
  string read_at = 8;            // RFC3339，未读时为空
  string created_at = 9;         // RFC3339
}

message ListNotificationsResponse {
  repeated Notification notifications = 1;
  string next_page_token = 2;
  int32 unread_count = 3;        // 一次调用即可完成徽标对账
}

message GetUnreadNotificationCountRequest {}
message GetUnreadNotificationCountResponse {
  int32 count = 1;
}

message MarkNotificationReadRequest {
  string id = 1;
}
message MarkNotificationReadResponse {
  bool success = 1;
}

message MarkAllNotificationsReadRequest {}
message MarkAllNotificationsReadResponse {
  bool success = 1;
  int32 updated_count = 2;
}

message DeleteNotificationRequest {
  string id = 1;
}
message DeleteNotificationResponse {
  bool success = 1;
}
```

**代码生成。** 与 `admin.proto` 相同（在 `backend/` 下执行）：

```bash
protoc -I api/proto/v1 \
  --go_out=. --go_opt=module=open-prompts/backend \
  --go-grpc_out=. --go-grpc_opt=module=open-prompts/backend \
  api/proto/v1/notification.proto
```

生成的 `notification.pb.go` / `notification_grpc.pb.go` 与 proto 一并提交（仓库惯例）。

## 2. 数据库 DDL

追加到 `deployment/init.sql` 与 `deployment/chart/open-prompts/init.sql` **两处**（字节一致的块）：

```sql
-- ============================================
-- Notifications (engagement events)
-- ============================================
CREATE TABLE IF NOT EXISTS notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recipient_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    actor_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('like', 'favorite', 'fork')),
    actor_display_name TEXT NOT NULL,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT no_self_notify CHECK (recipient_id <> actor_id)
);

CREATE INDEX IF NOT EXISTS idx_notifications_recipient_unread
    ON notifications (recipient_id) WHERE read_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_notifications_recipient_created
    ON notifications (recipient_id, created_at DESC);
```

说明：

- `gen_random_uuid()` 已被 `templates`/`template_versions` 使用（pgcrypto/pg13+ 内置）—— 无需扩展变更。
- 没有 ALTER 语句，因此不需要 `DO $$ ... EXCEPTION` 的幂等 ALTER 模式；普通 `IF NOT EXISTS` 即可。
- 存量部署：通过 psql 对线上库执行一次该块（与任何 init.sql 新增相同），或重建数据卷。

## 3. 后端分层

### 3.1 模型（`backend/internal/models/notification.go`）

```go
type Notification struct {
    ID               string
    RecipientID      string
    ActorID          string
    TemplateID       string
    Type             string // "like" | "favorite" | "fork"
    ActorDisplayName string
    ReadAt           *time.Time
    CreatedAt        time.Time
    // 读取时联表（不落库）：
    ActorAvatar   *string
    TemplateTitle string
}
```

### 3.2 仓储（`backend/internal/repository/notification_repository.go`）

接口（可 mock，匹配 `service/*_test.go` 风格）：

```go
type NotificationRepository interface {
    Create(ctx context.Context, n *models.Notification) error
    List(ctx context.Context, recipientID string, filter string, limit, offset int) ([]*models.Notification, int, error) // 行, 未读数
    CountUnread(ctx context.Context, recipientID string) (int, error)
    MarkRead(ctx context.Context, recipientID, id string) (bool, error) // 是否存在
    MarkAllRead(ctx context.Context, recipientID string) (int64, error)
    Delete(ctx context.Context, recipientID, id string) (bool, error)  // 是否存在
    PurgeExpired(ctx context.Context, readRetentionDays, retentionDays int) error
}
```

**Create（去重，FR-3）。** 单条语句，无额外往返：

```sql
INSERT INTO notifications (recipient_id, actor_id, template_id, type, actor_display_name)
SELECT $1, $2, $3, $4, $5
WHERE NOT EXISTS (
    SELECT 1 FROM notifications
    WHERE recipient_id = $1 AND actor_id = $2 AND template_id = $3
      AND type = $4 AND read_at IS NULL
)
RETURNING id;
```

若返回零行，说明插入被去重 —— **不是错误**。`no_self_notify` CHECK 是兜底；服务层在调用 Create 之前就会抑制自我操作。

**List。** 一条带 LEFT JOIN 的查询 + 一条基于部分索引的廉价 COUNT：

```sql
SELECT n.id, n.type, n.actor_id, n.actor_display_name, u.avatar AS actor_avatar,
       n.template_id, t.title AS template_title, n.read_at, n.created_at
FROM notifications n
LEFT JOIN users u ON u.id = n.actor_id
LEFT JOIN templates t ON t.id = n.template_id
WHERE n.recipient_id = $1
  AND ($2 = 'unread' AND n.read_at IS NULL OR $2 = 'all')
ORDER BY n.created_at DESC
LIMIT $3 OFFSET $4;

SELECT COUNT(*) FROM notifications WHERE recipient_id = $1 AND read_at IS NULL;
```

**MarkAllRead：**

```sql
UPDATE notifications SET read_at = now()
WHERE recipient_id = $1 AND read_at IS NULL;
```

**PurgeExpired（惰性保留）：**

```sql
DELETE FROM notifications
WHERE read_at < now() - ($1 || ' days')::interval
   OR created_at < now() - ($2 || ' days')::interval;
```

在服务层限频：内存中由 `sync.Mutex` 保护的 `lastPurgeAt time.Time`；每实例每 10 分钟至多执行一次，且仅在列表调用时触发。错误记日志并吞掉。

### 3.3 服务（`backend/internal/service/notification.go`）

```go
type NotificationService struct {
    pb.UnimplementedNotificationServiceServer
    repo NotificationRepository
    readRetentionDays int
    retentionDays     int
    lastPurgeAt       time.Time
    purgeMu           sync.Mutex
}
```

- `ListNotifications`：校验 `filter ∈ {all, unread}`（默认 `all`）、`page_size` 1–100（默认 20）、`page_token` 解析为整数 offset（默认 0）—— 与 `ListTemplates`/`ListAPIKeys` 惯例一致。执行限频的惰性清理。将行映射为 pb（时间戳 → `time.RFC3339` 字符串，与现有网关序列化风格一致）。
- `GetUnreadNotificationCount`、`MarkNotificationRead`、`MarkAllNotificationsRead`、`DeleteNotification`：接收者取自 `auth.UserIDFromContext(ctx)`；未知/他人 id → `codes.NotFound`。
- `MarkNotificationRead` 幂等：对已读行重复标记仍返回成功。

### 3.4 事件创建埋点（`backend/internal/service/prompt.go`）

`PromptService` 新增 `notificationRepo NotificationRepository` 依赖（`main.go` 接线中的构造参数）。

**ToggleLikeTemplate / ToggleFavoriteTemplate**（在现有 toggle 事务提交之后，且仅当动作为 **on** 时）：

1. 获取模板 owner：`tpl, err := s.templateRepo.Get(ctx, templateID)` —— 若失败，记日志并原样返回 toggle 结果（toggle 已成功；通知尽力而为）。
2. 若 `tpl.OwnerID == actorID` → 跳过（FR-2）。
3. `notificationRepo.Create(ctx, &models.Notification{RecipientID: tpl.OwnerID, ActorID: actorID, TemplateID: templateID, Type: "like"|"favorite", ActorDisplayName: actorDisplayName})`。actor 显示名来自用户仓储（`GetByID`）—— 多一次读取；在互动频率下可接受。若失败，记日志并继续。
4. toggle 响应**绝不**因通知错误而改变（FR-7）。

**ForkTemplate**（fork 事务提交之后）：源模板在流程中已加载（`sourceTpl.OwnerID`），无需额外查询 owner —— 只需读取 actor 显示名。同样的抑制/去重/吞错规则，`Type: "fork"`。

**为何在提交之后：** 通知绝不能为已回滚的 toggle 创建；通知失败也绝不能回滚互动本身。

### 3.5 网关接线（`backend/cmd/server/main.go`）

- 构造 `notificationRepo := repository.NewNotificationRepository(db)` 与 `notificationService := service.NewNotificationService(notificationRepo, readRetentionDays, retentionDays)`；将 `notificationRepo` 传入 `NewPromptService(...)`。
- 注册 gRPC：在现有 goroutine 中 `pb.RegisterNotificationServiceServer(grpcServer, notificationService)`。
- REST handler 沿用现有 `api-keys` handler 模式（CORS + OPTIONS + `VerifyAnyToken` → `ContextWithUserID` → 解析 → 调用 → `protojson` 序列化，`UseProtoNames` + `EmitUnpopulated` → `writeError` 映射）。一个 handler 按路径段路由：

```go
http.HandleFunc("/api/v1/notifications", handleNotifications)           // GET 列表
http.HandleFunc("/api/v1/notifications/", handleNotificationsTrailing)  // unread-count、{id}/read、read-all、{id}
```

`handleNotificationsTrailing` 按段路由：`unread-count`（GET）、`read-all`（POST）、`{id}/read`（POST）、`{id}`（DELETE）。方法不匹配 → `405`。`read-all` 必须在把首段当作 id 之前**先**匹配。

## 4. 前端设计

### 4.1 文件布局

```
frontend/src/
├── components/notifications/
│   ├── NotificationBell.js        # 顶栏按钮 + 徽标 + 轮询挂载
│   ├── NotificationDropdown.js    # 最近 10 条、全部已读、查看全部
│   └── NotificationItem.js        # 共享行（下拉 + 页面）
├── pages/NotificationsPage.js     # /notifications 历史页
├── hooks/
│   ├── useNotificationPolling.js  # 60s 轮询、可见性、退避
│   └── useDismissable.js          # 抽取的点击外部/Esc（来自个人资料下拉）
├── store/notificationsSlice.js
└── utils/time.js                  # formatRelativeTime(date, t)
```

### 4.2 Redux slice（`notificationsSlice.js`）

```js
{
  items: [],            // Notification[]（按页累积）
  unreadCount: 0,
  filter: 'all',        // 'all' | 'unread'
  nextPageToken: '',
  hasMore: false,
  loading: false,
  error: null,
  lastFetchedAt: 0,
}
```

异步 thunk：`fetchNotifications({ filter, reset })`、`fetchUnreadCount()`、`markNotificationRead(id)`、`markAllNotificationsRead()`、`deleteNotification(id)`。所有变更操作乐观更新 `items` 并本地重算 `unreadCount`（无需重新拉取）。`fetchNotifications({ reset: true })` 替换 items（下拉刷新/切换标签）；`reset: false` 追加（加载更多）。

### 4.3 轮询 hook（`useNotificationPolling.js`）

- `document.visibilityState === 'visible'` 且已认证时每 60 秒一次。
- `visibilitychange` → 隐藏：清除定时器；可见：立即刷新并重启定时器。
- `window focus` → 立即刷新（距上次拉取 ≥ 30 秒才触发，防抖）。
- 连续错误 → 退避 60 → 120 → 300 秒（封顶），成功后重置为 60 秒。
- 卸载/登出 → 停止。401 响应由全局 axios 拦截器处理（跳转 `/login`）。

### 4.4 组件

**NotificationBell**（Header，语言切换器与个人资料之间，仅 `isAuthenticated` 时渲染）：按钮含 `BellIcon` + 徽标（`unreadCount > 99` → "99+"）；`aria-label={t('notifications.bell.ariaLabel', { count })}`；通过 `useDismissable` 打开下拉。

**NotificationDropdown**：固定面板，`role="menu"`，最多 10 条（打开时 `fetchNotifications({ filter: 'all', reset: true })`），头部含标题 + "全部已读"，底部 "查看全部" → `/notifications`。空态：铃铛图标 + `notifications.empty.title/desc`。条目展示 actor 头像（缺省用首字母）、组装的语句、相对时间、未读圆点；点击条目 → 标记已读并跳转模板。

**NotificationItem**（共享）：语句在客户端组装：

```js
// notifications.type.like = "{{name}} liked your template \"{{title}}\""
t(`notifications.type.${n.type}`, { name: n.actor_display_name, title: n.template_title })
```

**NotificationsPage**（`/notifications`，`App.js` 中的懒加载路由，位于 `Layout` 内且 `showSidebar={false}`）：头部含标题 + "全部已读" 按钮；带实时计数的全部/未读标签；`NotificationItem` 列表（每页 20 条，加载更多追加）；单条已读（点击未读圆点）与删除（垃圾桶图标 → Modal 确认 → toast）；按标签的空态；带重试的错误态。

### 4.5 i18n 键（两种语言，`notifications.*` + `time.*`）

原样复用 UI/UX 设计 §9 中给出的键 JSON。结构：

```json
{
  "notifications": {
    "title": "通知",
    "markAllRead": "全部已读",
    "markRead": "标记已读",
    "delete": "删除",
    "viewAll": "查看全部",
    "empty": { "title": "已全部读完", "desc": "..." },
    "tabs": { "all": "全部", "unread": "未读" },
    "type": {
      "like": "{{name}} 赞了你的模板《{{title}}》",
      "favorite": "{{name}} 收藏了你的模板《{{title}}》",
      "fork": "{{name}} Fork 了你的模板《{{title}}》"
    },
    "bell": { "ariaLabel": "通知（{{count}} 条未读）" },
    "deleteConfirm": { "title": "删除通知", "message": "..." },
    "loadMore": "加载更多",
    "error": "通知加载失败"
  },
  "time": {
    "justNow": "刚刚",
    "minutesAgo": "{{count}} 分钟前",
    "hoursAgo": "{{count}} 小时前",
    "daysAgo": "{{count}} 天前"
  }
}
```

英文语言包携带对应英文语句（如 `{{name}} liked your template "{{title}}"`）。两种语言必须在同一次提交中交付。

### 4.6 API 客户端新增（`services/api.js`）

```js
getNotifications({ filter, pageSize, pageToken })   // GET /notifications
getUnreadNotificationCount()                        // GET /notifications/unread-count
markNotificationRead(id)                            // POST /notifications/{id}/read
markAllNotificationsRead()                          // POST /notifications/read-all
deleteNotification(id)                              // DELETE /notifications/{id}
```

## 5. 错误矩阵

| 条件 | gRPC 码 | HTTP | 用户可见 |
|---|---|---|---|
| 缺失/无效 token | `Unauthenticated` | 401 | 全局 401 → `/login` |
| `filter` 不在 {all, unread} 内 | `InvalidArgument` | 400 | `notifications.error` toast |
| `page_size` 超出 1–100 / `page_token` 非法 | `InvalidArgument` | 400 | `notifications.error` toast |
| 未知或他人通知 id | `NotFound` | 404 | `notifications.error` toast |
| 子路径方法不允许 | — | 405 | 浏览器默认 |
| 仓储/DB 故障 | `Internal` | 500 | `notifications.error` + 重试态 |
| 埋点通知插入失败 | —（记日志） | — | 无 —— 互动仍然成功（FR-7） |

## 6. 部署

- `docker-compose.yml` backend 服务 environment（可选，示默认值）：

```yaml
NOTIFICATION_READ_RETENTION_DAYS: "90"
NOTIFICATION_RETENTION_DAYS: "180"
```

- Helm：`values.yaml` 新增 `notificationReadRetentionDays: 90` / `notificationRetentionDays: 180`（或扁平 env map，遵循 chart 现有惯例），渲染进 backend deployment 的 env。
- 不新增服务/端口/卷。DDL 通过现有 init.sql 机制应用（幂等）。

## 7. SDK / CLI

无变更。`python-sdk/` 封装公开的 prompt/user 面；通知是 UI 优先的功能，SDK 面不在 v1 范围内（需求 §8 非目标）。若后续需要 SDK 支持，§1/§4.6 的 REST 契约是稳定的，可直接封装。

## 8. 测试策略

### 8.1 FVT（`tests/fvt.py`）— 开发者

在既有测试之后新增 `test_notifications()`（用 `get_auth_token` 建两个不同用户）：

1. 用户 B 点赞/收藏/Fork 用户 A 的公开模板 → A 的列表有 3 条通知，类型正确，unread_count = 3。
2. 自我操作：A 点赞自己的模板 → 无新通知。
3. 去重：B 取消点赞 → 在未读期间再次点赞 → 仍只有 1 条点赞通知。
4. 列表：`filter=unread` 与 `all`；`page_size=2` + `page_token` 分页；列表响应含 `unread_count`。
5. 标记已读：POST `/{id}/read` → 成功；重复 → 仍成功（幂等）；unread_count 下降。
6. 全部已读：POST `/read-all` → `updated_count` 匹配；unread_count = 0。
7. 删除：DELETE `/{id}` → 成功；重复 → 404。
8. 跨用户：B 尝试已读/删除 A 的通知 id → 404。
9. 级联：A 删除模板 → 其通知从 A 的列表消失。
10. 未认证：无 token → 5 个端点全部 401。

### 8.2 后端单元测试（新文件覆盖率 ≥ 80 %）

- `notification_test.go`（service）：filter/分页校验、接收者范围限定、他人 id NotFound、幂等已读、清理限频（首次执行、紧接的第二次跳过）、unread_count 映射。
- `prompt_test.go` 增补：埋点仅在 toggle-on 触发、toggle-off 不触发；自我操作被抑制；通知错误不影响 toggle（mock 仓储返回错误 → toggle 仍成功）；fork 埋点使用流程内 owner。

### 8.3 E2E（Nightwatch，test-agent）

| E2E | AC |
|---|---|
| 未登录时铃铛隐藏；互动后登录态可见并带徽标 | AC-1/2 |
| 下拉：10 条、全部已读清空徽标、查看全部跳转 | AC-3 |
| 历史页：带计数的标签、每页 20 条加载更多 | AC-4 |
| 单条已读（圆点消失、计数下降）、带确认的删除 | AC-5 |
| EN + ZH 语句渲染、相对时间 | AC-6 |
| 空态（已全部读完 / 无未读） | AC-7 |
| 轮询：外部互动后 60 秒内徽标更新 | AC-8 |

## 9. 实现顺序（建议）

1. DDL（两份 init.sql 副本）。
2. `notification.proto` + 生成代码。
3. 模型 + 仓储（去重插入、列表+计数、已读/删除、清理）。
4. `NotificationService` + 网关接线（5 个 REST handler）。
5. `prompt.go` 埋点（toggle-on + fork），吞掉错误。
6. 前端：slice + api 方法 → 铃铛/下拉 → 页面 → 轮询 hook → i18n 键。
7. FVT + 单元测试；`make lint`、`make ut`、`make fvt`、`npm run lint` 全绿。
8. compose/helm 环境变量；全新卷 + 存量卷验证。

## 10. 开发者约束（必须遵守）

1. **绝不提交 `backend/server` 的变更**（被跟踪的 26 MB 二进制）—— 若显示被修改，提交前 `git checkout -- backend/server`。
2. 分层：proto → 网关（`main.go`）→ service → repository → models；不新增顶层包。
3. 所有通知 REST 路由位于 `/api/v1/notifications*`；**无管理界面**（FR-29/30）；接收者永远来自 token。
4. 事件创建仅限内部（无创建 RPC）；埋点在互动事务**提交后**执行；埋点错误记日志并吞掉（FR-7）。
5. 去重通过 `INSERT ... WHERE NOT EXISTS` 语句实现（§3.2）；自我抑制在服务层 + DB CHECK 双重保障。
6. i18n：每个新增用户可见字符串必须同时进入 `en` 与 `zh` 语言包（`notifications.*`、`time.*`）。
7. `init.sql` 变更必须幂等并镜像到 helm chart 副本。
8. 环境变量通过 `os.Getenv` 读取并带安全默认值（90/180）。
9. 常规格式英文提交；新 Go 文件单元测试覆盖率 ≥ 80 %；推送前所有 CI 门禁全绿。
10. 未认证或页面隐藏时绝不能轮询；退避封顶 300 秒。
