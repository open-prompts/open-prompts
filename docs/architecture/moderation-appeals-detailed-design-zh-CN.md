# 审核申诉 — 详细设计

| | |
|---|---|
| **功能 ID** | moderation-appeals |
| **版本** | 1.0 |
| **状态** | 已批准实施 |
| **作者** | Architect Agent |
| **上游** | [架构](moderation-appeals-architecture-zh-CN.md) |

## 1. Proto 契约

### 1.1 `backend/api/proto/v1/prompt.proto`

在 `PromptService` 服务块末尾追加（`DeleteRating` 之后，第 130 行）：

```protobuf
service PromptService {
    // ... 现有 RPC ...
    rpc CreateAppeal(CreateAppealRequest) returns (Appeal);
    rpc ListAppeals(ListAppealsRequest) returns (ListAppealsResponse);
    rpc GetAppeal(GetAppealRequest) returns (Appeal);
}

message CreateAppealRequest {
    string moderation_action_id = 1;
    string statement = 2; // 50-2000 字符
}

message ListAppealsRequest {
    int32 page = 1;      // 从 1 开始
    int32 per_page = 2;  // 默认 20
}

message ListAppealsResponse {
    repeated Appeal appeals = 1;
    int64 total = 2;
    int32 page = 3;
    int32 per_page = 4;
}

message GetAppealRequest {
    string appeal_id = 1;
}

message Appeal {
    string id = 1;
    string moderation_action_id = 2;
    string template_id = 3;
    string template_title = 4;      // 读取时联表
    string action_type = 5;         // "unpublish" | "lock" | "soft_delete"
    string action_reason = 6;       // 原审核原因
    string actor_display_name = 7;  // 原操作的操作者
    google.protobuf.Timestamp action_created_at = 8;
    string statement = 9;
    string state = 10;              // open | under_review | resolved_upheld | resolved_reversed
    string resolution_note = 11;    // 裁决前为空
    google.protobuf.Timestamp created_at = 12;
    google.protobuf.Timestamp resolved_at = 13;
    bool superseded = 14;           // 底层操作已被直接恢复撤销
}
```

### 1.2 `backend/api/proto/v1/admin.proto`

在 `AdminService` 服务块末尾追加（`ListWarningHistory` 之后，第 81 行）：

```protobuf
service AdminService {
    // ... 现有 RPC ...
    rpc ListAppealQueue(ListAppealQueueRequest) returns (ListAppealQueueResponse);
    rpc GetAppeal(GetAdminAppealRequest) returns (AdminAppealDetail);
    rpc ReviewAppeal(ReviewAppealRequest) returns (AdminAppealDetail);
    rpc ResolveAppeal(ResolveAppealRequest) returns (AdminAppealDetail);
}

message ListAppealQueueRequest {
    string state = 1;   // "" | "open" | "under_review" | "resolved"
    int32 page = 2;
    int32 per_page = 3;
}

message ListAppealQueueResponse {
    repeated AdminAppealEntry entries = 1;
    int32 open_count = 2;
    int32 under_review_count = 3;
    int32 resolved_count = 4;
    int64 total = 5;
    int32 page = 6;
    int32 per_page = 7;
}

message AdminAppealEntry {
    string id = 1;
    string template_id = 2;
    string template_title = 3;
    string action_type = 4;
    string statement_excerpt = 5;   // 前 ~80 字符
    string state = 6;
    google.protobuf.Timestamp created_at = 7;
    string appellant_display_name = 8;
}

message GetAdminAppealRequest {
    string appeal_id = 1;
}

message ReviewAppealRequest {
    string appeal_id = 1;
}

message ResolveAppealRequest {
    string appeal_id = 1;
    string outcome = 2;  // "upheld" | "reversed"
    string note = 3;     // 20-1000 字符，必填
}

message AdminAppealDetail {
    string id = 1;
    string moderation_action_id = 2;
    string template_id = 3;
    string template_title = 4;
    string action_type = 5;
    string action_reason = 6;
    string action_actor_id = 7;
    string action_actor_email = 8;
    google.protobuf.Timestamp action_created_at = 9;
    string statement = 10;
    string state = 11;
    string appellant_id = 12;
    string appellant_display_name = 13;
    string resolution_note = 14;
    string resolver_id = 15;
    google.protobuf.Timestamp created_at = 16;
    google.protobuf.Timestamp resolved_at = 17;
    bool superseded = 18;
    string audit_entry_id = 19;   // 原操作的审计条目 id（如有）
}
```

重新生成：

```bash
cd backend && protoc --go_out=. --go-grpc_out=. api/proto/v1/prompt.proto api/proto/v1/admin.proto
```

## 2. 数据库 DDL

追加到 **两个** `deployment/init.sql` 和 `deployment/chart/open-prompts/init.sql`：

```sql
-- ===== moderation-appeals =====
-- 审核操作（下架/锁定/软删除）。由 admin-template-moderation 落地时填充；申诉读取。
CREATE TABLE IF NOT EXISTS moderation_actions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    action_type TEXT NOT NULL CHECK (action_type IN ('unpublish', 'lock', 'soft_delete')),
    actor_id TEXT NOT NULL,
    actor_email TEXT NOT NULL,
    reason TEXT NOT NULL CHECK (char_length(reason) BETWEEN 3 AND 500),
    status TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'superseded', 'reversed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    superseded_at TIMESTAMPTZ
);

COMMENT ON TABLE moderation_actions IS '模板上的审核操作；申诉引用这些操作';
COMMENT ON COLUMN moderation_actions.status IS 'active（可申诉）| superseded（被直接恢复撤销）| reversed（被申诉撤销）';

CREATE INDEX IF NOT EXISTS idx_moderation_actions_template
    ON moderation_actions (template_id);
CREATE INDEX IF NOT EXISTS idx_moderation_actions_status_created
    ON moderation_actions (status, created_at);

-- 申诉：每次审核操作仅一次。
CREATE TABLE IF NOT EXISTS appeals (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    moderation_action_id UUID NOT NULL REFERENCES moderation_actions(id) ON DELETE CASCADE,
    appellant_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    statement TEXT NOT NULL CHECK (char_length(statement) BETWEEN 50 AND 2000),
    state TEXT NOT NULL DEFAULT 'open'
        CHECK (state IN ('open', 'under_review', 'resolved_upheld', 'resolved_reversed')),
    resolver_id TEXT,
    resolution_note TEXT CHECK (resolution_note IS NULL OR char_length(resolution_note) BETWEEN 20 AND 1000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    UNIQUE (moderation_action_id)
);

COMMENT ON TABLE appeals IS '创作者对审核操作的申诉；每次操作仅一次';
COMMENT ON COLUMN appeals.state IS 'open | under_review | resolved_upheld | resolved_reversed';

CREATE INDEX IF NOT EXISTS idx_appeals_appellant_created
    ON appeals (appellant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_appeals_state_created
    ON appeals (state, created_at);

-- 扩展 admin_audit_logs.action CHECK 以包含申诉操作（守卫式、幂等）。
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'admin_audit_logs'::regclass AND conname = 'admin_audit_logs_action_check';
    IF def IS NOT NULL AND def NOT LIKE '%appeal.create%' THEN
        ALTER TABLE admin_audit_logs DROP CONSTRAINT admin_audit_logs_action_check;
        ALTER TABLE admin_audit_logs ADD CONSTRAINT admin_audit_logs_action_check
            CHECK (action IN ('ban', 'unban', 'role_change', 'delete_comment',
                              'review_approve', 'review_reject', 'settings_change',
                              'template.warning_add', 'template.warning_remove',
                              'appeal.create', 'appeal.review', 'appeal.resolve'));
    END IF;
END $$;

-- 扩展 notifications.type CHECK 以包含 appeal_resolved（守卫式、幂等）。
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'notifications'::regclass AND conname = 'notifications_type_check';
    IF def IS NOT NULL AND def NOT LIKE '%appeal_resolved%' THEN
        ALTER TABLE notifications DROP CONSTRAINT notifications_type_check;
        ALTER TABLE notifications ADD CONSTRAINT notifications_type_check
            CHECK (type IN ('like', 'favorite', 'fork', 'comment',
                            'template_approved', 'template_rejected',
                            'template_warning_changed', 'follow',
                            'appeal_resolved'));
    END IF;
END $$;
```

## 3. 后端分层

### 3.1 `models`（`backend/internal/models/appeal.go`）

```go
type ModerationAction struct {
    ID            string     `gorm:"primaryKey"`
    TemplateID    string
    ActionType    string     // "unpublish" | "lock" | "soft_delete"
    ActorID       string
    ActorEmail    string
    Reason        string
    Status        string     // "active" | "superseded" | "reversed"
    CreatedAt     time.Time
    SupersededAt  *time.Time
}

type Appeal struct {
    ID                 string     `gorm:"primaryKey"`
    ModerationActionID string
    AppellantID        string
    Statement          string
    State              string     // open | under_review | resolved_upheld | resolved_reversed
    ResolverID         string
    ResolutionNote     string
    CreatedAt          time.Time
    UpdatedAt          time.Time
    ResolvedAt         *time.Time
}
```

### 3.2 `AppealRepository`（`backend/internal/repository/appeal_repository.go`）

```go
type AppealRepository interface {
    CreateAppeal(ctx context.Context, a *models.Appeal) error
    GetAppeal(ctx context.Context, appealID string) (*models.Appeal, error)
    GetAppealByAction(ctx context.Context, actionID string) (*models.Appeal, error)
    ListAppeals(ctx context.Context, appellantID string, page, perPage int) ([]models.Appeal, int64, error)
    GetModerationAction(ctx context.Context, actionID string) (*models.ModerationAction, error)
    ListModerationActionsForTemplate(ctx context.Context, templateID string) ([]models.ModerationAction, error)
    ListQueue(ctx context.Context, state string, page, perPage int) ([]models.Appeal, int64, error)
    CountByState(ctx context.Context) (open, underReview, resolved int, err error)
    SetState(ctx context.Context, appealID, state string) error
    ResolveAppeal(ctx context.Context, appealID, outcome, note, resolverID string) error // 与恢复原子
    MarkActionSuperseded(ctx context.Context, actionID string) error
}
```

`CreateAppeal` SQL：

```sql
INSERT INTO appeals (moderation_action_id, appellant_id, statement)
VALUES ($1, $2, $3)
RETURNING id, created_at;
```

`UNIQUE (moderation_action_id)` 约束会暴露 `23505` 唯一冲突 → 映射为 `409 ALREADY_EXISTS`。

`ListAppeals`（最新优先）：

```sql
SELECT a.*, ma.template_id, ma.action_type, ma.reason AS action_reason,
       ma.actor_email, ma.created_at AS action_created_at, ma.status AS action_status,
       t.title AS template_title
FROM appeals a
JOIN moderation_actions ma ON ma.id = a.moderation_action_id
JOIN templates t ON t.id = ma.template_id
WHERE a.appellant_id = $1
ORDER BY a.created_at DESC
LIMIT $2 OFFSET $3;
```

`ListQueue`（状态内最早优先）：

```sql
SELECT a.*, ma.template_id, ma.action_type, t.title AS template_title,
       u.display_name AS appellant_display_name
FROM appeals a
JOIN moderation_actions ma ON ma.id = a.moderation_action_id
JOIN templates t ON t.id = ma.template_id
JOIN users u ON u.id = a.appellant_id
WHERE ($1 = '' OR a.state = $1)
ORDER BY a.created_at ASC
LIMIT $2 OFFSET $3;
```

`ResolveAppeal` —— 原子裁决+恢复事务（FR-7）：

```sql
BEGIN;
-- 1) 锁定申诉行并重新检查状态
SELECT state FROM appeals WHERE id = $1 FOR UPDATE;
--    （若已裁决 -> 以 409 中止）

-- 2) 更新申诉
UPDATE appeals
SET state = CASE WHEN $2 = 'reversed' THEN 'resolved_reversed'
                 ELSE 'resolved_upheld' END,
    resolver_id = $3,
    resolution_note = $4,
    resolved_at = now(),
    updated_at = now()
WHERE id = $1;

-- 3) 若 reversed，恢复模板（复用 admin-template-moderation 恢复语义）并将操作标记为 reversed
UPDATE moderation_actions SET status = 'reversed' WHERE id = $5;
UPDATE templates SET moderation_status = 'normal' WHERE id = $6;
COMMIT;
```

恢复将 `moderation_status` 设为 `'normal'`（与 `admin-template-moderation` FR-5 应用的补救相同）。对于 `lock` 操作，模板原本就是 `normal` 可见；恢复对状态是 no-op，但操作仍标记为 `reversed`。反转的审计条目复用 `template.restore`，原因中携带 `appeal=<id>`（OQ-3）。

### 3.3 `PromptService`（`backend/internal/service/prompt.go`）

`CreateAppeal`：

1. `GetUserIDFromContext` → 失败返回 `Unauthenticated`。
2. 校验 `statement` 长度 ∈ [50, 2000] → 否则 `InvalidArgument`。
3. `AppealRepo.GetModerationAction(actionID)` → 缺失 → `NotFound`。
4. 加载模板（`TemplateRepo.Get`）；校验 `tpl.OwnerID == userID` → 否则 `NotFound`（无存在性泄露）。
5. 若 `action.Status == "superseded"` → `AlreadyExists`（「已取代」）。
6. open 申诉速率限制（见下）→ 超限 `ResourceExhausted`。
7. `AppealRepo.CreateAppeal` → 唯一冲突 → `AlreadyExists`。
8. 递增 Redis open 申诉计数器。
9. 写审计 `appeal.create`（target_type `template`，target_id `template_id`，原因 `appeal=<id>`）。

`ListAppeals` / `GetAppeal`：认证，然后 `AppealRepo`；`GetAppeal` 校验 `appeal.AppellantID == userID` → 否则 `NotFound`。

**速率限制**（Redis 计数器，fail-open）：

```
key:    appeal_open:{user_id}
value:  "{count}:{window_expires_unix}"
window: 滑动，限额 APPEAL_MAX_OPEN（默认 5）
```

创建时递增；裁决时递减（管理员路径）。计数达到限额时，进一步创建返回 `429 RESOURCE_EXHAUSTED` 并带 `Retry-After`。

### 3.4 `AdminService`（`backend/internal/service/admin.go`）

`ListAppealQueue`：`requireAdminContext`，然后 `AppealRepo.ListQueue` + `CountByState` → `ListAppealQueueResponse`。

`GetAppeal`：`requireAdminContext`，然后 `AppealRepo.GetAppeal` + `GetModerationAction` → `AdminAppealDetail`（若操作有审计条目，则从 `AdminRepo.ListAuditByTarget` 查找 `audit_entry_id`）。

`ReviewAppeal`：`requireAdminContext`；校验状态为 `open` → 否则 `AlreadyExists`；`AppealRepo.SetState(id, "under_review")`；写审计 `appeal.review`。

`ResolveAppeal`：

1. `requireAdminContext`。
2. 校验 `outcome` ∈ {upheld, reversed} 且 `note` 长度 ∈ [20, 1000] → 否则 `InvalidArgument`。
3. 加载申诉及其操作；若 `action.ActorID == admin.ID` → `PermissionDenied`（自我守卫，FR-9）。
4. 若申诉已裁决 → `AlreadyExists`。
5. 若 `action.Status == "superseded"` → `AlreadyExists`。
6. `AppealRepo.ResolveAppeal`（原子；reversed 时恢复模板）。
7. 递减 Redis open 申诉计数器。
8. 写审计 `appeal.resolve`（原因 `outcome=<outcome> appeal=<id>`）。
9. `notifyAppealResolved`（见下）→ `appeal_resolved` 通知。

`notifyAppealResolved`（镜像 `notifyReviewDecision`）：

```go
func (s *AdminService) notifyAppealResolved(ctx context.Context, actor *models.User,
    appellantID, templateID, outcome, note string) {
    if s.NotificationRepo == nil { return }
    excerpt := note
    if len([]rune(excerpt)) > 140 { excerpt = string([]rune(excerpt)[:140]) + "…" }
    if err := s.NotificationRepo.Create(ctx, &models.Notification{
        RecipientID:      appellantID,
        ActorID:          actor.ID,
        TemplateID:       templateID,
        Type:             "appeal_resolved",
        ActorDisplayName: actor.DisplayName,
    }); err != nil {
        zap.S().Errorf("appeal resolved notification: failed: %v", err)
    }
}
```

说明摘录（140 字符）在客户端从通知负载渲染；深链指向 `/appeals/{id}`。

### 3.5 网关（`backend/cmd/server/main.go`）

**用户路由** —— 在 `/api/v1/notifications/` 处理器之后添加：

```go
// /api/v1/appeals — POST 创建, GET 列表（认证）。
http.HandleFunc("/api/v1/appeals", func(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodPost: // auth -> CreateAppealRequest{ModerationActionId, Statement}
    case http.MethodGet:  // auth -> ListAppealsRequest{Page, PerPage}
    default: http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    }
})

// /api/v1/appeals/{id} — GET 详情（认证，仅所有者）。
http.HandleFunc("/api/v1/appeals/", func(w http.ResponseWriter, r *http.Request) {
    id := strings.TrimPrefix(r.URL.Path, "/api/v1/appeals/")
    if r.Method == http.MethodGet { // auth -> GetAppealRequest{AppealId: id}
    }
})
```

**管理员路由** —— 在 `/api/v1/admin/settings` 处理器之后添加：

```go
// /api/v1/admin/appeals — GET 队列（管理员）。
http.HandleFunc("/api/v1/admin/appeals", func(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodGet { // admin -> ListAppealQueueRequest{State, Page, PerPage}
    }
})

// /api/v1/admin/appeals/{id} — GET 详情, POST review/resolve（管理员）。
http.HandleFunc("/api/v1/admin/appeals/", func(w http.ResponseWriter, r *http.Request) {
    rest := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/appeals/")
    switch {
    case strings.HasSuffix(rest, "/review"):
        // POST -> ReviewAppealRequest{AppealId}
    case strings.HasSuffix(rest, "/resolve"):
        // POST -> ResolveAppealRequest{AppealId, Outcome, Note}
    default:
        // GET -> GetAdminAppealRequest{AppealId: rest}
    }
})
```

所有路由都需要 Bearer 令牌认证舞步（用户或管理员 JWT 视情况）。429 响应设置 `Retry-After`。

## 4. 前端

### 4.1 新文件

| 文件 | 用途 |
|---|---|
| `frontend/src/components/AppealCtaSlot.js` + `.scss` | 所有者横幅 / 移除通知槽：CTA、状态徽章或结果徽章 + 说明摘录。 |
| `frontend/src/components/AppealFormModal.js` + `.scss` | 只读操作上下文 + 陈述文本域（50–2000）带实时计数；409 → 状态卡片。 |
| `frontend/src/components/AppealStateChip.js` + `.scss` | open / under review / upheld / reversed 徽章。 |
| `frontend/src/pages/AppealsPage.js` + `.scss` | `/appeals` —— 自己的申诉列表。 |
| `frontend/src/pages/AppealDetailPage.js` + `.scss` | `/appeals/:id` —— 陈述 + 操作上下文 + 裁决。 |
| `frontend/src/pages/admin/AppealsPage.js` + `.scss` | `/admin/appeals` —— 带状态标签 + 计数的队列。 |
| `frontend/src/pages/admin/AppealDetailPage.js` + `.scss` | `/admin/appeals/:id` —— 双栏详情 + Review/Resolve。 |

### 4.2 修改的文件

| 文件 | 变更 |
|---|---|
| `frontend/src/App.js` | 添加 `/appeals`、`/appeals/:id` 用户路由；`/admin/appeals`、`/admin/appeals/:id` 管理路由。 |
| `frontend/src/services/api.js` | `createAppeal`、`listAppeals`、`getAppeal`、`listAdminAppeals`、`getAdminAppeal`、`reviewAppeal`、`resolveAppeal`。 |
| 所有者横幅 / 移除通知组件 | 挂载 `AppealCtaSlot`（来自 `admin-template-moderation` FR-12）。 |
| 通知中心条目渲染器 | 渲染 `appeal_resolved`，带结果 + 说明摘录 + 深链。 |

### 4.3 Redux

无新 slice：申诉状态随 API 响应到达；表单模态和列表页管理本地状态。`AppealCtaSlot` 在挂载时获取当前操作对应的申诉状态。

### 4.4 API 客户端

```js
createAppeal({ moderation_action_id, statement }) // POST /appeals
listAppeals(params)                                // GET  /appeals
getAppeal(id)                                      // GET  /appeals/:id
listAdminAppeals(params)                           // GET  /admin/appeals
getAdminAppeal(id)                                 // GET  /admin/appeals/:id
reviewAppeal(id)                                   // POST /admin/appeals/:id/review
resolveAppeal(id, { outcome, note })               // POST /admin/appeals/:id/resolve
```

### 4.5 i18n 键（EN/ZH 对等）

```json
{
  "appeal": {
    "cta": {
      "appeal": "Appeal this decision",
      "onePerAction": "One appeal per action",
      "status": {
        "open": "Appeal open",
        "underReview": "Appeal under review",
        "reversed": "Appeal reversed — template restored",
        "upheld": "Appeal upheld"
      }
    },
    "form": {
      "title": "Appeal this decision",
      "actionContext": "Action",
      "statement": "Why should this decision be reversed?",
      "statementHint": "One appeal per action. Be specific and factual.",
      "submit": "Submit appeal",
      "conflict": "This action has already been appealed",
      "tooShort": "Statement must be at least 50 characters",
      "tooLong": "Statement must be at most 2000 characters"
    },
    "list": {
      "title": "My appeals",
      "empty": "No appeals — nothing to contest"
    },
    "detail": {
      "statement": "Your statement",
      "resolution": "Resolution",
      "pending": "Pending"
    },
    "state": {
      "open": "Open",
      "under_review": "Under review",
      "upheld": "Upheld",
      "reversed": "Reversed"
    },
    "superseded": "Superseded",
    "admin": {
      "queue": { "title": "Appeals", "empty": "No appeals in this state" },
      "detail": {
        "originalAction": "Original action",
        "review": "Review",
        "resolve": "Resolve appeal",
        "outcome": "Outcome",
        "upheld": "Upheld",
        "reversed": "Reversed",
        "note": "Note (required, shown to the creator)",
        "noteRule": "20–1000 characters",
        "selfGuard": "The original action's actor cannot resolve this appeal"
      },
      "confirm": { "reversed": "Reversing will restore the template to normal" },
      "toast": { "resolved": "Appeal resolved" }
    },
    "notif": { "resolved": "Your appeal was {{outcome}}: {{note}}" },
    "rateLimit": "You can have at most 5 open appeals"
  }
}
```

ZH 按 UI/UX 文档 §8 镜像每个键（`appeal.cta.appeal` = "申诉此决定"、`appeal.state.open` = "处理中" 等）。

## 5. 错误矩阵

| 场景 | HTTP | gRPC | 前端处理 |
|---|---|---|---|
| 陈述/说明长度违规 | 400 | `INVALID_ARGUMENT` | 字段级错误；表单不变 |
| 未认证 | 401 | `UNAUTHENTICATED` | 重定向到 `/login` |
| 管理路由非管理员 | 403 | `PERMISSION_DENIED` | 管理控制台错误 toast |
| 裁决者是原操作者 | 403 | `PERMISSION_DENIED` | Resolve 表单禁用并显示自我守卫消息 |
| 操作/申诉不存在或非所有 | 404 | `NOT_FOUND` | 404 页面 |
| 操作已被申诉 | 409 | `ALREADY_EXISTS` | 表单替换为现有申诉状态卡片 |
| 申诉已裁决 | 409 | `ALREADY_EXISTS` | Toast；详情刷新 |
| 操作已取代 | 409 | `ALREADY_EXISTS` | 入口隐藏；「已取代」标签 |
| open 申诉限额 | 429 | `RESOURCE_EXHAUSTED` | 带限额消息的 Toast |
| Redis 宕机 | — | — | 服务端 fail-open；无用户影响 |

## 6. 部署

- `APPEAL_MAX_OPEN=5` 添加到 `deployment/docker-compose.yml` 后端环境变量和 Helm chart（`values.yaml` `backend.env.appealMaxOpen` + `templates/deployment-backend.yaml` 环境条目）。
- DDL 增量/幂等；两个 init.sql 副本在同一提交中。
- 无新服务、端口或基础设施。

## 7. SDK

无 python-sdk 变更：申诉是 Web 界面功能（所有者 + 管理控制台）。REST 端点在 API 界面中记录；SDK 仅封装模板解析。

## 8. 测试策略

### 8.1 FVT（`tests/fvt.py`）—— `test_moderation_appeals`

设置：alice（创作者）、bob（管理员）、carol（管理员）。通过 SQL 直接播种 `moderation_actions` 行（admin-template-moderation 尚未实现）或通过仅测试辅助函数，针对 alice 的一个模板。

1. alice 创建申诉 → `open`；`GET /api/v1/appeals` 列出它。
2. alice 对同一操作创建第二个申诉 → 409。
3. 陈述 < 50 或 > 2000 字符 → 400。
4. bob（管理员）`GET /api/v1/admin/appeals?state=open` → 队列最早优先，计数正确。
5. bob `POST /review` → `under_review`；alice 在 `/appeals` 上看到该状态。
6. bob `POST /resolve {outcome: reversed, note}` → 模板 `moderation_status` 变为 `normal`；alice 收到 `appeal_resolved` 通知；审计有 `appeal.resolve`。
7. 第二个申诉：bob `POST /resolve {outcome: upheld, note}` → 模板不变；通知说明 upheld。
8. carol（原操作的操作者）尝试裁决 → 403。
9. 非管理员尝试管理路由 → 403。
10. alice 有 5 个 open 申诉时提交第 6 个 → 429。
11. 已取代操作（状态翻转为 `superseded`）→ 创建 → 409；裁决 → 409。
12. 含 `<script>` 的陈述以字面文本渲染（无 HTML 解释）。

### 8.2 单元测试

- 仓库：`CreateAppeal` 唯一冲突映射、`ListQueue` 排序 + 计数、`ResolveAppeal` 事务（reversed 恢复、upheld no-op、已裁决中止）、状态转换守卫。
- 服务：校验矩阵、所有者/可见性检查、自我守卫、速率限制计数器（假 Redis）、审计 + 通知发出。
- 用新方法更新 `MockAppealRepository`。

### 8.3 E2E（test-agent，Nightwatch）

- 表单模态：从横幅打开、实时计数、提交、409 → 状态卡片。
- `/appeals` 列表 + 详情渲染；状态徽章。
- 管理队列标签带计数；详情双栏；Review；Resolve 确认对话框；自我守卫禁用状态。
- `appeal_resolved` 通知渲染带说明摘录 + 深链。
- 所有 `appeal.*` 键的 i18n EN/ZH 对等。

## 9. 实施顺序

1. DDL（两个 init.sql 副本）+ models + `AppealRepository` + 单元测试。
2. Proto 变更 + 重新生成。
3. `PromptService` 用户 RPC + 速率限制 + Redis 计数器接线。
4. `AdminService` 管理 RPC + 自我守卫 + 审计 + 通知。
5. 网关用户 + 管理路由。
6. 前端：api 客户端 → 组件 → 页面 → 路由 → i18n。
7. FVT + E2E。

## 10. 给开发者的约束

1. **绝不提交 `backend/server`** —— 受跟踪的构建产物；若被修改则 `git checkout -- backend/server`。
2. 只暂存你自己的文件；绝不 `git add -A`。
3. 两个 init.sql 副本在同一提交中保持同步。
4. `moderation_actions` 由 `admin-template-moderation` 落地时填充 —— 不要在此实现下架/锁定/软删除；只读取该表。FVT 中直接播种行。
5. `appeals.moderation_action_id` UNIQUE 约束是 FR-3 的后备；将 `23505` 冲突映射为 `409 ALREADY_EXISTS`。
6. `ResolveAppeal` 必须原子：申诉更新 + 模板恢复 + 操作状态翻转在同一事务中。
7. 裁决者绝不能是原操作的操作者（FR-9）；在事务前于服务中强制。
8. 陈述和说明是纯文本 —— 绝不渲染为 HTML/Markdown（构造上 XSS 安全，NFR-5）。
9. 反转审计复用 `template.restore`，原因中携带 `appeal=<id>`（OQ-3）—— 不要新增审计操作代码。
10. 无任何变现界面。