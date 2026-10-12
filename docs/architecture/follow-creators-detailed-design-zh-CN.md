# 关注创作者 — 详细设计

| | |
|---|---|
| **Feature ID** | follow-creators |
| **Version** | 1.0 |
| **Status** | Approved for implementation |
| **Author** | Architect Agent |
| **Upstream** | [架构设计](follow-creators-architecture-zh-CN.md) |

## 1. Proto 契约

新增至 `backend/api/proto/v1/prompt.proto`。字段号延续既有序列；`Template`
的 17–19 号字段与 template-comments 设计协同（17 = `comment_count`，由该设计
预留）。

```protobuf
// UserService — 追加到既有 service 块（第 10 行）
service UserService {
    // ... 既有 RPC ...
    rpc FollowUser(FollowUserRequest) returns (FollowUserResponse);
    rpc UnfollowUser(FollowUserRequest) returns (FollowUserResponse);
    rpc GetCreatorProfile(GetCreatorProfileRequest) returns (GetCreatorProfileResponse);
    rpc ListFollowers(ListUserRelationsRequest) returns (ListUserRelationsResponse);
    rpc ListFollowing(ListUserRelationsRequest) returns (ListUserRelationsResponse);
    rpc GetFollowingFeed(GetFollowingFeedRequest) returns (GetFollowingFeedResponse);
}

message FollowUserRequest {
    string user_id = 1;     // 被关注者
    string follower_id = 2; // 由网关从鉴权上下文注入
}

message FollowUserResponse {
    bool following = 1;
    int32 follower_count = 2;
}

message GetCreatorProfileRequest {
    string user_id = 1;
    string viewer_id = 2; // 匿名时为空
}

message GetCreatorProfileResponse {
    string id = 1;
    string display_name = 2;
    string avatar = 3;
    string bio = 4;
    int32 follower_count = 5;
    int32 following_count = 6;
    google.protobuf.Timestamp created_at = 7;
    bool is_following = 8;
    bool follows_you = 9;
}

message ListUserRelationsRequest {
    string user_id = 1;
    int32 page = 2;      // 1 起，默认 1
    int32 page_size = 3; // 默认 20，最大 50
}

message UserCard {
    string id = 1;
    string display_name = 2;
    string avatar = 3;
    int32 follower_count = 4;
    bool is_following = 5;
}

message ListUserRelationsResponse {
    repeated UserCard users = 1;
    int32 total_count = 2;
    int32 page = 3;
    int32 page_size = 4;
}

message GetFollowingFeedRequest {
    string user_id = 1;    // 关注者（me）
    int32 page_size = 2;   // 默认 20，最大 50
    string page_token = 3; // 不透明的 created_at 游标
}

message GetFollowingFeedResponse {
    repeated Template templates = 1;
    int32 total_count = 2;
    string next_page_token = 3;
}

message OwnerInfo {
    string id = 1;
    string display_name = 2;
    string avatar = 3;
    int32 follower_count = 4;
    bool is_following = 5;
}
```

既有消息的新增：

```protobuf
message UpdateProfileRequest {
    // ... 既有字段 1-4 ...
    string bio = 5; // 可选；按提交值应用（空串清除）
}

message Template {
    // ... 既有字段 1-16；comment_count = 17（comments 特性）...
    string owner_display_name = 18;
    string owner_avatar = 19;
}

message GetTemplateResponse {
    Template template = 1;
    TemplateVersion latest_version = 2;
    OwnerInfo owner = 3;
}
```

`admin.proto` — `UserStats`（第 122 行）新增：

```protobuf
message UserStats {
    // ... 既有字段 1-4 ...
    int32 follower_count = 5;
    int32 following_count = 6;
}
```

用项目既有命令重新生成：

```bash
cd backend && protoc --go_out=. --go-grpc_out=. api/proto/v1/prompt.proto api/proto/v1/admin.proto
```

## 2. 数据库 DDL

追加到 **两份** `deployment/init.sql` 与
`deployment/chart/open-prompts/init.sql`（保持同步）：

```sql
-- ===== follow-creators =====
CREATE TABLE IF NOT EXISTS user_follows (
    follower_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    followee_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (follower_id, followee_id),
    CONSTRAINT chk_no_self_follow CHECK (follower_id <> followee_id)
);

CREATE INDEX IF NOT EXISTS idx_user_follows_followee
    ON user_follows(followee_id, created_at DESC);

ALTER TABLE users ADD COLUMN IF NOT EXISTS bio TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS follower_count INT NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS following_count INT NOT NULL DEFAULT 0;

-- 'follow' 通知不携带模板：将 template_id 改为可空
-- （守卫化、幂等、与 notifications 特性顺序无关）。
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'notifications')
       AND EXISTS (
           SELECT 1 FROM pg_attribute
           WHERE attrelid = 'notifications'::regclass
             AND attname = 'template_id' AND attnotnull
       ) THEN
        ALTER TABLE notifications ALTER COLUMN template_id DROP NOT NULL;
    END IF;
END $$;

-- 将通知 type CHECK 扩宽以包含 'follow'（以及 template-comments 设计的
-- 'comment'）。守卫下删除/重建，与既有 CHECK 扩宽模式相同；任意落地顺序安全。
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'notifications'::regclass
          AND contype = 'c'
          AND pg_get_constraintdef(oid) LIKE '%like%'
    ) THEN
        ALTER TABLE notifications DROP CONSTRAINT chk_notifications_type;
        ALTER TABLE notifications ADD CONSTRAINT chk_notifications_type
            CHECK (type IN ('like','favorite','fork','comment','follow'));
    END IF;
END $$;
```

> **给开发者的说明**：上述块中的约束名必须与 notifications 特性实际创建的
> 名称一致（写迁移前先检查 `pg_constraint`；若不同，请同步调整两个守卫块中
> 的名称）。意图是：两个特性都落地后，`type` 接受
> `like|favorite|fork|comment|follow`，且 `template_id` 可空。

## 3. 后端分层

### 3.1 `models.User`（`backend/internal/models/user.go`）

新增字段：

```go
Bio            string    // 可空列；NULL 时为空字符串
FollowerCount  int32
FollowingCount int32
```

### 3.2 `models.Follow`（新增，`backend/internal/models/follow.go`）

```go
type Follow struct {
    FollowerID string
    FolloweeID string
    CreatedAt  time.Time
}
```

### 3.3 `FollowRepository`（新增，`backend/internal/repository/follow_repository.go`）

```go
type FollowRepository interface {
    Follow(ctx context.Context, followerID, followeeID string) (followerCount int32, err error)
    Unfollow(ctx context.Context, followerID, followeeID string) (followerCount int32, err error)
    IsFollowing(ctx context.Context, followerID, followeeID string) (bool, error)
    GetFollowCounts(ctx context.Context, userID string) (followers, following int32, err error)
    ListFollowers(ctx context.Context, userID string, page, pageSize int) ([]*models.UserCard, int, error)
    ListFollowing(ctx context.Context, userID string, page, pageSize int) ([]*models.UserCard, int, error)
    GetFollowingFeed(ctx context.Context, userID string, pageSize int, beforeCreatedAt time.Time) ([]*models.Template, int, error)
    TopCreatorsByFollowers(ctx context.Context, limit int) ([]*models.UserCard, error)
}
```

关键 SQL（每个动作单事务；仅当边确实变化时才移动计数器）：

```sql
-- Follow（事务）：
-- 1) 插入边（幂等）
INSERT INTO user_follows (follower_id, followee_id)
VALUES ($1, $2)
ON CONFLICT (follower_id, followee_id) DO NOTHING;
-- 2) 仅当插入了行时才递增计数器（检查 RowsAffected）
UPDATE users SET follower_count = follower_count + 1 WHERE id = $2;
UPDATE users SET following_count = following_count + 1 WHERE id = $1;
-- 3) 回读被关注者的 follower_count 用于响应
SELECT follower_count FROM users WHERE id = $2;
```

`Unfollow` 对称处理：`DELETE ...` + 条件递减（用 `GREATEST(x - 1, 0)` 兜底
防漂移）。

信息流查询（封禁创作者与私有模板在 SQL 层排除）：

```sql
SELECT t.*, u.display_name AS owner_display_name, COALESCE(u.avatar,'') AS owner_avatar
FROM user_follows f
JOIN templates t ON t.owner_id = f.followee_id
JOIN users u ON u.id = t.owner_id
WHERE f.follower_id = $1
  AND t.visibility = 'public'
  AND t.created_at < $2            -- keyset 游标
  AND NOT EXISTS (SELECT 1 FROM users bu
                  WHERE bu.id = t.owner_id AND bu.status = 'banned')
ORDER BY t.created_at DESC
LIMIT $3;
-- 总数用相同 WHERE 去掉 LIMIT 查询
```

### 3.4 `UserRepository` 变更

- `GetByID` / `GetByEmail` 的 SELECT 列表加入
  `bio, follower_count, following_count`。
- `Update` 的 SET 子句加入 `bio = $n`。**契约说明**：与
  `display_name`/`avatar`（仅非空才应用）不同，`bio` 按提交值写入，空串即可
  清除。服务层在调用 `Update` 前先 trim 再校验长度（0–280）。

### 3.5 `UserService`（`backend/internal/service/user.go`）

- 结构体新增 `FollowRepo FollowRepository` 与
  `NotificationRepo repository.NotificationRepository`（nil 安全；为 nil 时
  跳过关注钩子，与 `createEngagementNotification` 语义一致）。
- `FollowUser` / `UnfollowUser`：
  1. 拒绝自我关注（`400`）。
  2. 限流检查（见下）—— 仅**成功**动作消耗配额。
  3. 加载两个用户；被关注者缺失 → `404`；任一方封禁 → `403`。
  4. 调用 `FollowRepo.Follow/Unfollow`（幂等）。
  5. 新插入边时发出 `'follow'` 通知（recipient = 被关注者，actor = 关注者，
     `template_id = NULL`）。取关不发通知。
- `GetCreatorProfile`：加载用户，构建白名单响应（绝不 email/mobile/role），
  当查看者已认证时计算 `is_following`/`follows_you`。
- `ListFollowers`/`ListFollowing`：offset 分页（默认 20，最大 50）+ 总数。
- `GetFollowingFeed`：按 `created_at` 游标的 keyset 分页；页大小默认 20，
  最大 50。
- `UpdateProfile`：接受 `bio`（trim → 校验 0–280 → 按提交值应用）。

**限流**（Redis 固定窗口，fail-open）：

```
key:    follow_rate_min:{user_id}
value:  "{count}:{window_expires_unix}"
窗口:    60s，上限 FOLLOW_RATE_PER_MINUTE（默认 30）
```

超限时：返回 `ResourceExhausted` 并附 `Retry-After: <窗口重置秒数>`。
Redis 错误记录日志后放行动作（fail-open），与 auth 节流模式一致。

### 3.6 `PromptService`（`backend/internal/service/prompt.go`）

- `GetTemplate`：加载模板后，通过 `UserRepo` 加载作者、`FollowRepo` 的
  `IsFollowing` 判断关注状态（测试中仓储缺失时 nil 安全/跳过），填充
  `GetTemplateResponse.owner`。
- `templateModelToProto`：映射 `owner_display_name`/`owner_avatar`
  （字段 18/19）。

### 3.7 `TemplateRepository.List` — 作者信息增强

在既有 SELECT（已 LEFT JOIN likes/favorites）中加入：

```sql
JOIN users ou ON ou.id = t.owner_id
-- 加入选择列：
ou.display_name AS owner_display_name, COALESCE(ou.avatar,'') AS owner_avatar
```

`models.Template` 新增 `OwnerDisplayName string` 与 `OwnerAvatar string`。
该 JOIN 安全，因为 `templates.owner_id` 对 `users` 有外键。

### 3.8 `AdminService`（`backend/internal/service/admin.go`）

`GetUserStats`（经 `adminRepository`）新增两个子查询：

```sql
(SELECT follower_count FROM users WHERE id = $1) AS follower_count,
(SELECT following_count FROM users WHERE id = $1) AS following_count
```

映射到 `UserStats` 字段 5/6。不新增管理端 RPC。

### 3.9 网关（`backend/cmd/server/main.go`）

新路由组（均在既有 CORS/鉴权中间件下）：

```
POST   /api/v1/users/{userId}/follow        → VerifyAnyToken
DELETE /api/v1/users/{userId}/follow        → VerifyAnyToken
GET    /api/v1/users/{userId}/profile       → 公开
GET    /api/v1/users/{userId}/templates     → 公开（映射到 ListTemplates：owner_id + visibility=public）
GET    /api/v1/users/{userId}/followers     → 公开
GET    /api/v1/users/{userId}/following     → 公开
GET    /api/v1/users/me/following-feed      → VerifyAnyToken
```

路由注册顺序重要：`/api/v1/users/me/following-feed` 必须先于
`/api/v1/users/{userId}/...` 模式注册，避免 `me` 被当作 userId 捕获。
`follower_id`/`viewer_id` 从鉴权上下文注入（`ContextWithUserID`），绝不取自
请求体。

## 4. 前端

### 4.1 新增文件

| 文件 | 用途 |
|---|---|
| `frontend/src/pages/CreatorProfilePage.js` + `.scss` | 公开创作者主页：头部（96px 头像、名称、bio、Follows-you 徽章、计数、加入日期），标签页 Templates/Followers/Following。自己的主页 → "Edit profile" 链接到 `/profile`。 |
| `frontend/src/components/FollowButton.js` + `.scss` | 乐观切换；`aria-pressed`；悬停/聚焦时 Following → Unfollow（危险色）；出错回滚；无确认弹窗。 |
| `frontend/src/components/UserCard.js` + `.scss` | 头像 + 显示名 + 粉丝数 + FollowButton；链接到 `/creators/:id`。 |
| `frontend/src/components/OwnerChip.js` + `.scss` | TemplateDetails 上的头像 + 名称 + 粉丝数；链至创作者主页；FollowButton 为独立 stop 目标。 |
| `frontend/src/utils/formatCount.js` | 本地化数字缩写（`1.2k` / `1.2万`）。 |

### 4.2 修改文件

| 文件 | 变更 |
|---|---|
| `frontend/src/App.js` | 新增 `/creators/:id` 路由（懒加载 + Suspense，同 `/notifications`）。 |
| `frontend/src/pages/Home.js` | "Following" 标签页：信息流网格 + Load more（复用无限滚动模式）；匿名时登录引导卡片；空状态展示前 5 位推荐创作者（`TopCreatorsByFollowers`，经 `GET /api/v1/users?sort=followers` 暴露 —— 见 §4.4）。 |
| `frontend/src/pages/TemplateDetails.js` | 将 `t('template_details.by', { owner: template.owner_id })`（约 693 行）替换为 `OwnerChip` + `FollowButton`。 |
| `frontend/src/components/PromptCard.js` | "by {display_name}" 行，链至 `/creators/:id`（`owner_display_name` 为空时隐藏）。 |
| `frontend/src/pages/Profile.js` | 带字符计数（0–280）的 bio 文本域。 |
| `frontend/src/pages/admin/UserDetailPage.js` | 统计行（约 108 行）新增只读粉丝/关注条目。 |
| `frontend/src/services/api.js` | 新客户端函数（§4.4）。 |

### 4.3 Redux

无需新 slice：关注状态存于组件本地状态（乐观切换），增强的模板/作者数据经
既有获取模式到达。若团队倾向将当前模板的 `is_following` 放入全局状态，可扩展
既有 template slice —— 但 v1 保持本地。

### 4.4 API 客户端（`frontend/src/services/api.js`）

```js
followUser(userId)            // POST   /users/${userId}/follow
unfollowUser(userId)          // DELETE /users/${userId}/follow
getCreatorProfile(userId)     // GET    /users/${userId}/profile
getCreatorTemplates(userId, { page, page_size }) // GET /users/${userId}/templates
getFollowers(userId, { page, page_size })        // GET /users/${userId}/followers
getFollowing(userId, { page, page_size })        // GET /users/${userId}/following
getFollowingFeed({ page_size, page_token })      // GET /users/me/following-feed
getTopCreators({ limit })     // GET /users?sort=followers&limit=  （空信息流推荐）
```

`getTopCreators` 映射到一个小型公开列表端点：
`GET /api/v1/users?sort=followers&limit=5`，由 `TopCreatorsByFollowers`
实现（仅公开 user-card 形状）。

### 4.5 i18n 键（EN/ZH 对齐）

```json
{
  "creators": {
    "title": "创作者",
    "followers": "{{count}} 位粉丝",
    "following_count": "关注 {{count}} 人",
    "joined": "加入于 {{date}}",
    "follows_you": "关注了你",
    "tab_templates": "模板",
    "tab_followers": "粉丝",
    "tab_following": "关注中",
    "edit_profile": "编辑资料",
    "empty_templates": "暂无公开模板",
    "empty_followers": "暂无粉丝",
    "empty_following": "尚未关注任何人"
  },
  "follow": {
    "follow": "关注",
    "following": "已关注",
    "unfollow": "取消关注",
    "error": "无法更新关注状态"
  },
  "home": {
    "following": {
      "tab": "关注中",
      "empty": "关注创作者，在这里查看他们的最新模板",
      "suggested": "推荐创作者",
      "login_prompt": "登录后即可关注创作者"
    }
  },
  "template_details": {
    "by": "作者：{{owner}}"
  },
  "admin": {
    "users": {
      "followers": "粉丝",
      "following": "关注"
    }
  },
  "notifications": {
    "type_follow": "{{actor}} 关注了你"
  }
}
```

英文版本逐键对应（如 `follow.follow` = "Follow"、
`creators.follows_you` = "Follows you"、`notifications.type_follow` =
"{{actor}} followed you"）。

## 5. 错误矩阵

| 场景 | HTTP | gRPC | 前端处理 |
|---|---|---|---|
| 自我关注 | 400 | `INVALID_ARGUMENT` | 内联错误，状态不变 |
| bio 超过 280 字符 | 400 | `INVALID_ARGUMENT` | 计数变红；阻止保存 |
| 未认证 | 401 | `UNAUTHENTICATED` | 重定向登录 |
| 任一方封禁 | 403 | `PERMISSION_DENIED` | 错误 toast；按钮回滚 |
| 用户不存在 | 404 | `NOT_FOUND` | 404 页 / 空状态 |
| 触发限流 | 429 | `RESOURCE_EXHAUSTED` | 带重试倒计时的 toast；按钮回滚 |
| Redis 宕机 | — | — | 服务端 fail-open；用户无感知 |

## 6. 部署

- `FOLLOW_RATE_PER_MINUTE=30` 加入 `deployment/docker-compose.yml` 的
  backend 环境与 Helm chart（`values.yaml`
  `backend.env.followRatePerMinute: "30"` + `templates/deployment-backend.yaml`
  env 条目），与 `NOTIFICATION_*` 一致。
- DDL 块为增量/幂等；两份 init.sql 副本在同一提交中更新。
  notifications 协同块任意落地顺序均安全。
- 无新服务、端口或基础设施。

## 7. SDK

python-sdk 无需变更：SDK 封装的是模板解析，而非用户社交功能。新端点仅在
REST API 层面记录文档。

## 8. 测试策略

### 8.1 FVT（`tests/fvt.py`）— `test_follow_creators`

三个用户（alice、bob、carol），复用既有 `get_auth_token` 助手：

1. alice 关注 bob → `{following: true, follower_count: 1}`；bob 的 profile
   显示 `follower_count: 1`、（alice 视角）`is_following: true`、
   （bob 看 alice）`follows_you: true`。
2. 重复关注（幂等）→ 计数不变。
3. carol 关注 bob → `follower_count: 2`。
4. alice 取关 bob → `{following: false, follower_count: 1}`；重复取关幂等。
5. 自我关注 → 400。关注不存在用户 → 404。
6. 封禁 carol（管理 API）→ carol 不能关注（403）；alice 不能关注 carol
   （403）；既有 alice→carol 边保留；carol 的 profile 仍返回 200。
7. 主页隐私：匿名与已认证调用者的 `/users/{id}/profile` 响应中断言无
   `email`、`mobile`、`role`。
8. 信息流：bob（被 alice 关注）发布公开 + 私有模板；alice 的信息流仅含
   公开模板，最新优先；排除封禁创作者的模板。
9. followers/following 列表：分页 + user-card 形状（无 email）。
10. bio：经 `PUT /api/v1/profile` 设置，在 profile 响应中验证；空串清除；
    281 字符 bio → 400。
11. 模板详情含 `owner` 对象及 `is_following`；列表响应携带
    `owner_display_name`。
12. 管理端用户详情 `stats` 含粉丝/关注计数。

### 8.2 单元测试

- `FollowRepository`：重复插入/删除不存在时的计数数学；信息流查询排除
  （私有、封禁）。
- `UserService`：自我关注、封禁流程、限流窗口（mock Redis）、bio
  trim/校验、仅新边发通知。

### 8.3 E2E（test-agent，Nightwatch）

- 关注按钮切换 + 强制出错时的乐观回滚。
- 创作者主页标签页渲染；自己的主页显示 Edit 链接。
- 关注信息流：登录引导（匿名）、带推荐的空状态、Load more。
- i18n：所有新键的 EN/ZH 对齐；数字缩写抽查。

## 9. 实现顺序

1. DDL（两份 init.sql 副本）+ models + `FollowRepository` + 单元测试。
2. Proto 变更 + 重新生成 + `UserRepository`/`TemplateRepository` 更新。
3. `UserService` RPC（follow/unfollow/profile/lists/feed/bio）+ 限流 +
   通知钩子。
4. `PromptService` 作者增强 + `AdminService` 统计。
5. 网关路由。
6. 前端：api 客户端 → 组件 → 页面 → i18n。
7. FVT + E2E。

## 10. 给开发者的约束

1. **绝不提交 `backend/server`** —— 它是被跟踪的 26MB 构建产物。若显示被
   修改，执行 `git checkout -- backend/server`。
2. 仅显式暂存自己的文件；绝不 `git add -A`（共享工作树）。
3. 两份 init.sql 副本必须在同一提交中保持同步。
4. notifications DDL 协同块在设计上顺序无关；依赖守卫删除前先在
   `pg_constraint` 中核实实际约束名。
5. `bio` 采用按提交值应用语义（空串清除）—— 不要照搬
   `display_name`/`avatar` 的仅非空模式。
6. 公开端点绝不返回 email/mobile/role；profile 响应形状在服务层白名单化。
7. `/api/v1/users/me/following-feed` 必须先于 `/api/v1/users/{userId}`
   路由模式注册。
8. 限流仅对成功动作消耗配额；Redis 错误时 fail-open。
9. 绝不出现任何商业化界面。
