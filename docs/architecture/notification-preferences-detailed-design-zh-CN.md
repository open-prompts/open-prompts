# 通知偏好 — 详细设计

- **功能 ID：** `notification-preferences`
- **版本：** 1.0
- **状态：** 已批准实施
- **作者：** Architect Agent
- **上游文档：** [需求](../design/notification-preferences-requirements-zh-CN.md) · [UI/UX 设计](../design/notification-preferences-ui-ux-design-zh-CN.md)
- **架构文档：** [架构设计（英文）](./notification-preferences-architecture.md) · [架构设计（中文）](./notification-preferences-architecture-zh-CN.md)

---

## 1. Proto 契约

两个偏好 RPC 添加到 `backend/api/proto/v1/prompt.proto`（包 `openprompts.v1`）中现有的 `UserService`。它们追加在 `UserService` 服务块的 `ListTopCreators`（`PromptService` 块之前的最后一个用户 RPC）之后。无需新 proto 文件。

```protobuf
service UserService {
  // ... 现有 RPC ...

  // GetNotificationPreferences 返回调用者的有效通知偏好（无行时物化默认值）。
  rpc GetNotificationPreferences(GetNotificationPreferencesRequest) returns (NotificationPreferences);
  // UpdateNotificationPreferences 应用偏好字段的子集并返回完整有效偏好。
  rpc UpdateNotificationPreferences(UpdateNotificationPreferencesRequest) returns (NotificationPreferences);
}

message GetNotificationPreferencesRequest {}

message NotificationPreferences {
  bool like_enabled = 1;
  bool favorite_enabled = 2;
  bool fork_enabled = 3;
  bool comment_enabled = 4;
  bool follow_enabled = 5;
  bool paused_all = 6;
}

message UpdateNotificationPreferencesRequest {
  // 可选布尔字段：仅显式存在的字段被更新。
  // 使用 proto3 optional，使缺失字段与 false 可区分。
  optional bool like_enabled = 1;
  optional bool favorite_enabled = 2;
  optional bool fork_enabled = 3;
  optional bool comment_enabled = 4;
  optional bool follow_enabled = 5;
  optional bool paused_all = 6;
}
```

**字段编号：** 六个偏好字段为 1–6，两个消息一致。无冲突（这些是新消息）。

**代码生成。** 与现有 proto 文件相同（从 `backend/` 运行）：

```bash
protoc -I api/proto/v1 \
  --go_out=. --go_opt=module=open-prompts/backend \
  --go-grpc_out=. --go-grpc_opt=module=open-prompts/backend \
  api/proto/v1/prompt.proto
```

将重新生成的 `prompt.pb.go` / `prompt_grpc.pb.go` 与 proto 一起提交（仓储约定）。

## 2. 数据库 DDL

追加到**两份** `deployment/init.sql` 和 `deployment/chart/open-prompts/init.sql`（逐字节一致块）：

```sql
-- -----------------------------------------------------------------------------
-- Feature: notification-preferences
-- Description: Per-type opt-out + master pause for in-app notifications.
--              One row per user, created lazily on first preference change
--              (absence = all-enabled / not-paused). Read at event-emission
--              time to suppress disabled/paused types.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS notification_preferences (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    like_enabled BOOLEAN NOT NULL DEFAULT true,
    favorite_enabled BOOLEAN NOT NULL DEFAULT true,
    fork_enabled BOOLEAN NOT NULL DEFAULT true,
    comment_enabled BOOLEAN NOT NULL DEFAULT true,
    follow_enabled BOOLEAN NOT NULL DEFAULT true,
    paused_all BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE notification_preferences IS
    'Per-user notification preferences; absent row = all defaults';
COMMENT ON COLUMN notification_preferences.user_id IS
    'User owning these preferences (PK, cascade-deleted with the user)';
COMMENT ON COLUMN notification_preferences.like_enabled IS
    'Receive like notifications (default true)';
COMMENT ON COLUMN notification_preferences.favorite_enabled IS
    'Receive favorite notifications (default true)';
COMMENT ON COLUMN notification_preferences.fork_enabled IS
    'Receive fork notifications (default true)';
COMMENT ON COLUMN notification_preferences.comment_enabled IS
    'Receive comment notifications (default true)';
COMMENT ON COLUMN notification_preferences.follow_enabled IS
    'Receive follow notifications (default true)';
COMMENT ON COLUMN notification_preferences.paused_all IS
    'Master pause: when true, no notification of any type is created';
COMMENT ON COLUMN notification_preferences.updated_at IS
    'Last preference change timestamp';
```

说明：

- `CREATE TABLE IF NOT EXISTS` 幂等——无需 `DO $$` 块（无 ALTER）。
- `user_id` 是主键，也是指向 `users(id)` 的外键，带 `ON DELETE CASCADE`（FR-1）。`users` 主键是 `TEXT`，因此外键类型匹配。
- 现有部署：通过 psql 对在线数据库运行一次该块（与任何 init.sql 新增相同），或重建卷。

## 3. 后端分层

### 3.1 模型（`backend/internal/models/notification_preferences.go`）

```go
package models

import "time"

// NotificationPreferences 是每用户通知退出状态。
// 缺失行意味着全部启用 / 未暂停（默认值）。
type NotificationPreferences struct {
    UserID          string    `json:"user_id"`
    LikeEnabled     bool      `json:"like_enabled"`
    FavoriteEnabled bool      `json:"favorite_enabled"`
    ForkEnabled     bool      `json:"fork_enabled"`
    CommentEnabled  bool      `json:"comment_enabled"`
    FollowEnabled   bool      `json:"follow_enabled"`
    PausedAll       bool      `json:"paused_all"`
    UpdatedAt       time.Time `json:"updated_at"`
}
```

### 3.2 仓储（`backend/internal/repository/notification_preferences_repository.go`）

接口（可 mock，匹配 `service/*_test.go` 风格）：

```go
type NotificationPreferencesRepository interface {
    // Get 返回用户的有效偏好。无行时返回全默认值（无错误）。
    Get(ctx context.Context, userID string) (*models.NotificationPreferences, error)
    // Upsert 首次更改时建行或更新提供的字段。
    // 仅写入非 nil 指针的字段。
    Upsert(ctx context.Context, userID string, prefs *models.NotificationPreferences) error
}
```

**Get** —— 单次主键点读；缺失行 → 默认值：

```sql
SELECT user_id, like_enabled, favorite_enabled, fork_enabled,
       comment_enabled, follow_enabled, paused_all, updated_at
FROM notification_preferences
WHERE user_id = $1;
```

若 `sql.ErrNoRows`，返回 `&models.NotificationPreferences{UserID: userID}`（所有布尔为零值 = false）。**重要：** 服务层必须将缺失行的零值布尔解释为*默认值*（全部启用、未暂停）。为消除歧义，仓储在行旁返回哨兵 `found bool`，服务在 `found == false` 时应用默认值：

```go
Get(ctx, userID) (*models.NotificationPreferences, bool, error) // prefs, found, err
```

**Upsert** —— 一条语句完成惰性建行 + 部分更新：

```sql
INSERT INTO notification_preferences
    (user_id, like_enabled, favorite_enabled, fork_enabled,
     comment_enabled, follow_enabled, paused_all, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, now())
ON CONFLICT (user_id) DO UPDATE SET
    like_enabled = COALESCE($2, notification_preferences.like_enabled),
    favorite_enabled = COALESCE($3, notification_preferences.favorite_enabled),
    fork_enabled = COALESCE($4, notification_preferences.fork_enabled),
    comment_enabled = COALESCE($5, notification_preferences.comment_enabled),
    follow_enabled = COALESCE($6, notification_preferences.follow_enabled),
    paused_all = COALESCE($7, notification_preferences.paused_all),
    updated_at = now();
```

调用方为六个字段传入 `*bool` 指针（nil = 保持不变）；仓储将 nil 转为 `NULL`，使 `COALESCE` 保留现有值。

### 3.3 服务（`backend/internal/service/user.go`）

`UserService` 增加 `NotificationPrefsRepo` 字段和 setter：

```go
type UserService struct {
    // ... 现有字段 ...
    NotificationPrefsRepo repository.NotificationPreferencesRepository
}

func (s *UserService) SetNotificationPrefsRepo(repo repository.NotificationPreferencesRepository) {
    s.NotificationPrefsRepo = repo
}
```

**GetNotificationPreferences：**

```go
func (s *UserService) GetNotificationPreferences(ctx context.Context, _ *pb.GetNotificationPreferencesRequest) (*pb.NotificationPreferences, error) {
    userID, err := GetUserIDFromContext(ctx)
    if err != nil {
        return nil, err
    }
    prefs, found, err := s.NotificationPrefsRepo.Get(ctx, userID)
    if err != nil {
        return nil, status.Errorf(codes.Internal, "failed to load notification preferences: %v", err)
    }
    return prefsToProto(prefs, found), nil
}
```

`prefsToProto` 在 `found == false` 时应用默认值（所有布尔为 true，`paused_all` 为 false）。

**UpdateNotificationPreferences：**

```go
func (s *UserService) UpdateNotificationPreferences(ctx context.Context, req *pb.UpdateNotificationPreferencesRequest) (*pb.NotificationPreferences, error) {
    userID, err := GetUserIDFromContext(ctx)
    if err != nil {
        return nil, err
    }
    // 速率限制（防篡改）：NOTIF_PREFS_RATE_PER_MINUTE（默认 30）。
    if retryAfter, ok := s.checkPrefsRate(ctx, userID); !ok {
        return nil, status.Errorf(codes.ResourceExhausted,
            "notification preference changes rate limited; retry after %ds", retryAfter)
    }
    // 从可选字段构建部分更新。
    prefs := &models.NotificationPreferences{UserID: userID}
    if req.LikeEnabled != nil { prefs.LikeEnabled = *req.LikeEnabled }
    if req.FavoriteEnabled != nil { prefs.FavoriteEnabled = *req.FavoriteEnabled }
    if req.ForkEnabled != nil { prefs.ForkEnabled = *req.ForkEnabled }
    if req.CommentEnabled != nil { prefs.CommentEnabled = *req.CommentEnabled }
    if req.FollowEnabled != nil { prefs.FollowEnabled = *req.FollowEnabled }
    if req.PausedAll != nil { prefs.PausedAll = *req.PausedAll }
    if err := s.NotificationPrefsRepo.Upsert(ctx, userID, prefs); err != nil {
        return nil, status.Errorf(codes.Internal, "failed to update notification preferences: %v", err)
    }
    // 返回完整有效偏好。
    updated, found, err := s.NotificationPrefsRepo.Get(ctx, userID)
    if err != nil {
        return nil, status.Errorf(codes.Internal, "failed to reload notification preferences: %v", err)
    }
    return prefsToProto(updated, found), nil
}
```

**速率限制**（`checkPrefsRate`）镜像 `prompt.go` 中的 `checkRatingRate`（Redis 固定窗口计数器，键 `notif_prefs_rate_min:<userID>`，Redis 为 nil/不可用时放行）：

```go
const notifPrefsRatePerMinuteDef = 30

func (s *UserService) checkPrefsRate(ctx context.Context, userID string) (int, bool) {
    if s.Redis == nil {
        return 0, true
    }
    key := "notif_prefs_rate_min:" + userID
    max, _ := strconv.Atoi(os.Getenv("NOTIF_PREFS_RATE_PER_MINUTE"))
    if max <= 0 {
        max = notifPrefsRatePerMinuteDef
    }
    // ... 与 checkRatingRate 相同的固定窗口逻辑 ...
}
```

### 3.4 抑制钩子（事件发射咽喉点）

偏好检查插入**三个**现有发射咽喉点，在现有自操作抑制**之后**、`NotificationRepo.Create` 调用**之前**。所有钩子保持严格尽力而为：偏好检查失败被记录并吞掉（跳过通知而非使互动失败）。

**`prompt.go` — `createEngagementNotification`**（like/favorite/fork）：

```go
func (s *PromptService) createEngagementNotification(ctx context.Context, notificationType, templateID, ownerID, actorID string) {
    if s.NotificationRepo == nil || s.UserRepo == nil {
        return
    }
    if ownerID == actorID {
        return // 自操作抑制（不变）
    }
    if !s.prefsAllow(ctx, ownerID, notificationType) {
        return // 被偏好抑制
    }
    // ... 现有 actor 查找 + Create ...
}
```

**`comment.go` — `notifyCommentReply`**（comment）：

```go
func (s *PromptService) notifyCommentReply(ctx context.Context, tpl *models.Template, parentID, actorID, content string) {
    if s.NotificationRepo == nil || s.UserRepo == nil {
        return
    }
    parent, err := s.CommentRepo.Get(ctx, parentID)
    if err != nil || parent == nil {
        zap.S().Warnf("comment_reply hook: failed to load parent %s: %v", parentID, err)
        return
    }
    if parent.UserID == actorID {
        return // 自回复抑制（不变）
    }
    if !s.prefsAllow(ctx, parent.UserID, "comment") {
        return // 被偏好抑制
    }
    // ... 现有 actor 查找 + Create ...
}
```

**`user.go` — `emitFollowNotification`**（follow）：

```go
func (s *UserService) emitFollowNotification(ctx context.Context, followeeID, followerID string) {
    if s.NotificationRepo == nil || s.Repo == nil {
        return
    }
    if !s.prefsAllow(ctx, followeeID, "follow") {
        return // 被偏好抑制
    }
    // ... 现有 actor 查找 + Create ...
}
```

**共享辅助函数** —— 单一解析路径（NFR-3），使有效偏好读取和发射时检查使用相同逻辑。放在小型共享辅助函数上（如各服务的 `prefsAllow`，委托给 `service/prefs.go` 中的公共函数）：

```go
// prefsAllow 报告接收者是否应收到给定类型的通知。
// 缺失行 = 允许。暂停全部覆盖一切。
// 仓储错误时放行（记录 + 允许），使偏好故障永不阻塞互动通知。
func prefsAllow(ctx context.Context, repo repository.NotificationPreferencesRepository, recipientID, notificationType string) bool {
    if repo == nil {
        return true
    }
    prefs, found, err := repo.Get(ctx, recipientID)
    if err != nil {
        zap.S().Warnf("prefs check failed for %s: %v", recipientID, err)
        return true // 放行
    }
    if !found {
        return true // 默认：全部启用
    }
    if prefs.PausedAll {
        return false
    }
    switch notificationType {
    case "like":
        return prefs.LikeEnabled
    case "favorite":
        return prefs.FavoriteEnabled
    case "fork":
        return prefs.ForkEnabled
    case "comment", "comment_reply":
        return prefs.CommentEnabled
    case "follow":
        return prefs.FollowEnabled
    default:
        // 管理员/系统类型（moderation、appeal_resolved、template_approved、
        // template_rejected、template_warning_changed）v1 不抑制（FR-10）。
        return true
    }
}
```

**关于 `comment` 与 `comment_reply` 的说明：** 需求的五种类型是 `like`、`favorite`、`fork`、`comment`、`follow`。已上线的 `comment_reply` 事件（来自 comment-replies-threads）是评论族事件；`comment_enabled` 开关同时管辖 `comment` 和 `comment_reply`（`comment` 类型保留给未来的顶层评论通知）。这在 UI 描述中已记录（"有人评论了你的模板"）。

**进程内缓存（Q2）：** `prefsAllow` 辅助函数通过以 `recipientID` 为键的短 TTL（30 秒）进程内缓存读取，PATCH 时失效。这避免每次互动都读数据库，同时保持抑制近乎实时。缓存是 `service/prefs.go` 中的简单 `sync.Map` + 时间戳 map；PATCH 删除该接收者的条目。缓存未命中 → 数据库读取（放行）。

### 3.5 网关接线（`backend/cmd/server/main.go`）

- 构造 `prefsRepo := repository.NewNotificationPreferencesRepository(db)` 并通过 `userSvc.SetNotificationPrefsRepo(prefsRepo)` 接入 `UserService`。
- 在 `/api/v1/users/me/notification-preferences` 注册两个 REST 处理器（在 `/api/v1/users/me/following-feed` 路由**之前**注册，使字面路径先匹配；现有 `/users/me/...` 路由使用精确路径，因此无冲突）：

```go
http.HandleFunc("/api/v1/users/me/notification-preferences", func(w http.ResponseWriter, r *http.Request) {
    // CORS + OPTIONS + VerifyAnyToken → ContextWithUserID（与 profile 相同）
    switch r.Method {
    case http.MethodGet:
        resp, err := userSvc.GetNotificationPreferences(ctx, &pb.GetNotificationPreferencesRequest{})
        // marshal + write
    case http.MethodPatch:
        body, _ := io.ReadAll(r.Body)
        var req pb.UpdateNotificationPreferencesRequest
        if err := unmarshaler.Unmarshal(body, &req); err != nil {
            http.Error(w, err.Error(), http.StatusBadRequest)
            return
        }
        resp, err := userSvc.UpdateNotificationPreferences(ctx, &req)
        // marshal + write
    default:
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    }
})
```

- `Access-Control-Allow-Methods` 头必须包含 `PATCH`。

## 4. 前端设计

### 4.1 文件布局

```
frontend/src/
├── components/notifications/
│   └── NotificationPreferencesPanel.js   # 模态框：主开关 + 5 个类型行
├── pages/NotificationsPage.js            # 头部增加齿轮图标
├── components/notifications/NotificationBell.js  # 增加页脚链接
├── store/notificationPreferencesSlice.js
└── components/ui/Icons.jsx               # 增加 GearIcon（或复用）
```

### 4.2 Redux slice（`notificationPreferencesSlice.js`）

```js
{
  prefs: { like_enabled: true, favorite_enabled: true, fork_enabled: true,
           comment_enabled: true, follow_enabled: true, paused_all: false },
  saving: { like_enabled: false, favorite_enabled: false, fork_enabled: false,
            comment_enabled: false, follow_enabled: false, paused_all: false },
  error: null,
}
```

异步 thunk：`fetchNotificationPreferences()`（GET，面板打开时）、`updateNotificationPreference({ field, value })`（PATCH 单个字段，乐观更新 + 出错回滚）。`saving[field]` 跟踪每行的进行中状态。

### 4.3 组件

**NotificationPreferencesPanel**（模态框，由齿轮图标和铃铛页脚链接打开）：

- 头部：标题 + 关闭（× / Esc）。
- **PauseAllSwitch**（主开关）：`role="switch"`、`aria-checked`；开启 → 使类型行变灰并显示"暂停全部已开启"说明；关闭 → 恢复先前状态。
- **五个 TypeToggleRow**（like/favorite/fork/comment/follow）：每个是带标签的组（`aria-labelledby`），含标题、白话描述和开关。更改时自动保存（PATCH）；乐观动画 → "已保存 ✓"（1.5 秒，`aria-live="polite"`）或回滚 + 错误 toast（`aria-live="assertive"`）。
- 页脚说明："更改自动保存。"
- 加载：打开时骨架行；错误：横幅 + 重试。
- 首次打开：GET 偏好；缺失行渲染全开默认值（无闪烁）。

**NotificationsPage** 头部：在标题旁增加 `GearIconButton`（aria-label `notifPrefs.open`）打开面板。

**NotificationBell** 下拉页脚：增加"通知偏好"链接（"查看全部"上方或下方）打开同一面板。

### 4.4 API 客户端新增（`services/api.js`）

```js
getNotificationPreferences()                       // GET /users/me/notification-preferences
updateNotificationPreferences(patch)               // PATCH /users/me/notification-preferences
```

### 4.5 i18n 键（两种语言，`notifPrefs.*`）

复用 UI/UX 设计 §8 中的键 JSON。形态：

```json
{
  "notifPrefs": {
    "title": "Notification preferences",
    "pauseAll": "Pause all notifications",
    "pauseAllDesc": "Stop all notifications of every type.",
    "type": {
      "like": "Likes",
      "like.desc": "Someone likes your template.",
      "favorite": "Favorites",
      "favorite.desc": "Someone favorites your template.",
      "fork": "Forks",
      "fork.desc": "Someone forks your template.",
      "comment": "Comments",
      "comment.desc": "Someone comments on your template.",
      "follow": "Follows",
      "follow.desc": "Someone follows you."
    },
    "pausedNote": "Paused all is on — turn it off to adjust individual types.",
    "autoSave": "Changes save automatically.",
    "saved": "Saved",
    "error": "Couldn't save. Try again.",
    "open": "Notification preferences"
  }
}
```

中文语言包携带等价字符串（如 `通知偏好`、`暂停所有通知`、`点赞`、`有人点赞了你的模板。`）。两种语言必须在同一提交中发布。

## 5. 错误矩阵

| 条件 | gRPC 码 | HTTP | 用户可见 |
|---|---|---|---|
| 缺失/无效令牌 | `Unauthenticated` | 401 | 全局 401 → `/login` |
| 畸形请求体 / 非布尔字段 | `InvalidArgument` | 400 | `notifPrefs.error` toast |
| 超过速率限制（PATCH） | `ResourceExhausted` | 429 | `notifPrefs.error` toast + `Retry-After` |
| 仓储/数据库故障 | `Internal` | 500 | `notifPrefs.error` + 重试状态 |
| 偏好检查失败（钩子） | —（记录） | — | 无——跳过通知，互动成功 |

## 6. 部署

- `docker-compose.yml` 后端服务环境（可选，默认值如下）：

```yaml
NOTIF_PREFS_RATE_PER_MINUTE: "30"
```

- Helm：`values.yaml` 增加 `notifPrefsRatePerMinute: 30`（或扁平 env map，匹配 chart 现有约定），渲染进后端 deployment env。
- 无新服务/端口/卷。DDL 通过现有 init.sql 机制应用（幂等）。

## 7. SDK / CLI

无变更。`python-sdk/` 包装公开的 prompt/user 表面；通知偏好是 UI 优先功能，SDK 表面 v1 超出范围（需求 §8 非目标）。若后续需要 SDK 支持，§1/§4.4 中的 REST 契约稳定可包装。

## 8. 测试策略

### 8.1 FVT（`tests/fvt.py`）——开发者

在现有测试之后新增 `test_notification_preferences()`（用 `get_auth_token` 创建两个不同用户）：

1. 新用户 GET → 全部默认（`like_enabled` 等为 true，`paused_all` 为 false）。
2. PATCH `{ "like_enabled": false }` → GET 显示仅 `like_enabled` 为 false，其余不变。
3. 抑制：用户 B 点赞用户 A 的模板；A 的 `like_enabled` 为 false → A 的通知列表无点赞通知，未读徽章不变。
4. 暂停全部：A 设置 `paused_all` true → B 收藏/fork/评论/关注 → A 均不收到；既有历史保持可浏览。
5. 暂停时，PATCH 类型开关服务端仍接受（UI 禁用，但 API 允许——持久化值被存储，取消暂停后生效）。
6. 重新启用：A 设置 `paused_all` false → 新点赞创建通知。
7. PATCH 非布尔值 → 400。
8. PATCH 未知字段 → 忽略（200，不变）。
9. 跨用户隔离：用户 C 无法读取或修改 A 的偏好（无此类端点——仅 `/me`）。
10. 未认证 → 两个端点均 401。
11. 速率限制：一分钟内 >30 次 PATCH → 429 带 `Retry-After`。

### 8.2 后端单元测试（新文件 ≥ 80%）

- `notification_preferences_test.go`（服务）：有效偏好解析（缺失行 → 默认值）、部分更新（仅写入提供的字段）、速率限制执行（mock Redis）、非布尔拒绝、未知字段忽略。
- `prompt_test.go` / `comment_test.go` / `user_test.go` 新增：类型禁用或暂停时抑制钩子触发；偏好仓储错误时放行；自操作抑制仍先运行；通知错误仍不使互动失败。

### 8.3 E2E（Nightwatch，测试代理）

| E2E | AC |
|---|---|
| 通知页齿轮图标打开面板，5 个开关（全开）+ 暂停全部（关） | AC-1 |
| 关闭"点赞"自动保存（已保存 ✓）；后续点赞不创建通知/徽章/历史 | AC-2 |
| 暂停全部停止全部五种类型；铃铛不显示新内容；历史保持可浏览 | AC-3 |
| 暂停时类型开关禁用带说明；取消暂停恢复先前状态 | AC-4 |
| PATCH 子集仅更新这些字段；GET 返回有效偏好 | AC-5 |
| 失败 PATCH 回滚开关 + 错误 toast | AC-6 |
| 偏好跨会话/设备持久（服务端） | AC-7 |
| 禁用点赞的用户仍为他人产生点赞通知 | AC-8 |
| 从未打开面板的用户零行为差异 | AC-9 |
| 所有字符串 EN + ZH 渲染；类型标签匹配通知标签 | AC-10 |

## 9. 实施顺序（建议）

1. DDL（两份 init.sql 副本）。
2. `prompt.proto` 新增 + 重新生成代码。
3. 模型 + 仓储（带 found 哨兵的 `Get`、带 COALESCE 的 `Upsert`）。
4. `UserService` 偏好方法 + 速率限制 + 网关接线（2 个 REST 处理器）。
5. 共享 `prefsAllow` 辅助函数 + 进程内缓存；插入三个发射咽喉点（prompt/comment/user）。
6. 前端：slice + api 方法 → 面板组件 → 齿轮图标 + 铃铛页脚链接 → i18n 键。
7. FVT + 单元测试；`make lint`、`make ut`、`make fvt`、`npm run lint` 全绿。
8. Compose/helm env 变量；全新卷 + 现有卷验证。

## 10. 开发者约束（必须遵守）

1. **绝不提交对 `backend/server` 的更改**（跟踪的 26 MB 二进制）——若显示已修改，提交前 `git checkout -- backend/server`。
2. 分层：proto → 网关（`main.go`）→ 服务 → 仓储 → 模型；无新顶层包。
3. 两个偏好 REST 路由在 `/api/v1/users/me/notification-preferences` 下；**无管理表面**（FR-14）；用户始终来自令牌。
4. 抑制**仅限接收**（FR-8）：绝不抑制行为者自己的对外通知；自操作抑制（FR-9）先运行。
5. 抑制发生在**发射时**（FR-7）：禁用/暂停类型绝不创建行；被抑制事件之后不可检索。
6. 管理员/系统通知类型（moderation、appeal_resolved、template_approved、template_rejected、template_warning_changed）v1 **不抑制**（FR-10）。
7. `comment_enabled` 同时管辖 `comment` 和 `comment_reply` 事件。
8. 偏好仓储错误和 Redis 不可用速率限制时放行；偏好故障绝不阻塞互动通知。
9. `init.sql` 变更幂等并镜像到 helm chart 副本。
10. 环境变量通过 `os.Getenv` 读取并带安全默认值（30）。
11. 约定式提交，英文；新 Go 文件单元测试覆盖率 ≥ 80%；推送前所有 CI 门禁全绿。
12. 进程内偏好缓存（30 秒 TTL）必须在 PATCH 时失效，使用户更改立即生效（FR-5）。