# 模板评论 — 详细设计

- **功能 ID：** `template-comments`
- **版本：** 1.0
- **状态：** 已批准进入实现
- **作者：** 架构 Agent
- **上游文档：**[需求分析](../design/template-comments-requirements-zh-CN.md) · [UI/UX 设计](../design/template-comments-ui-ux-design-zh-CN.md)
- **架构文档：**[架构设计（中文）](./template-comments-architecture-zh-CN.md) · [Architecture Design (EN)](./template-comments-architecture.md)

---

## 1. Proto 契约

### 1.1 `backend/api/proto/v1/prompt.proto` — 新增

`service PromptService` 追加 4 个 RPC（位于 `ListTemplateVersions` 之后）：

```protobuf
// Comment RPCs

// ListComments 列出模板评论（可见性跟随模板）。
rpc ListComments(ListCommentsRequest) returns (ListCommentsResponse);

// CreateComment 在模板上发表评论（需认证）。
rpc CreateComment(CreateCommentRequest) returns (Comment);

// UpdateComment 在编辑窗口内修改作者本人的评论。
rpc UpdateComment(UpdateCommentRequest) returns (Comment);

// DeleteComment 删除评论（作者或模板所有者）。
rpc DeleteComment(DeleteCommentRequest) returns (DeleteCommentResponse);
```

新增消息（追加到文件末尾）：

```protobuf
// CommentAuthor 是评论作者的公开身份。
message CommentAuthor {
  string id = 1;
  string display_name = 2;
  string avatar = 3;
}

// Comment 表示模板上的一条评论。
message Comment {
  string id = 1;
  string template_id = 2;
  string content = 3;
  google.protobuf.Timestamp created_at = 4;
  google.protobuf.Timestamp updated_at = 5;
  // updated_at > created_at 时为 true（已编辑标记）。
  bool is_edited = 6;
  CommentAuthor author = 7;
  // 评论作者即模板所有者时为 true。
  bool is_owner = 8;
  // 调用者为作者时为 true（驱动 编辑/删除 操作的显隐）。
  bool is_own = 9;
}

message ListCommentsRequest {
  string template_id = 1;
  int32 page = 2;       // 1 起，默认 1
  int32 page_size = 3;  // 默认 20，上限 50
}

message ListCommentsResponse {
  repeated Comment comments = 1;
  int64 total_count = 2;
  int32 page = 3;
  int32 page_size = 4;
}

message CreateCommentRequest {
  string template_id = 1;
  string content = 2;   // trim 后 1-2000 字符
}

message UpdateCommentRequest {
  string comment_id = 1;
  string content = 2;   // trim 后 1-2000 字符
}

message DeleteCommentRequest {
  string comment_id = 1;
}

message DeleteCommentResponse {
  bool success = 1;
}
```

`Template` 消息新增一个字段（下一个可用 tag）：

```protobuf
  // 评论数。
  int32 comment_count = 17;
```

### 1.2 `backend/api/proto/v1/admin.proto` — 新增

`service AdminService` 追加 2 个 RPC（位于 `ListAuditLogs` 之后）：

```protobuf
// ListAdminComments 列出全平台评论（审核）。
rpc ListAdminComments(ListAdminCommentsRequest) returns (ListAdminCommentsResponse);

// DeleteAdminComment 以强制理由删除任意评论（写审计）。
rpc DeleteAdminComment(DeleteAdminCommentRequest) returns (DeleteAdminCommentResponse);
```

新增消息：

```protobuf
// AdminCommentEntry 是带审核上下文的评论。
message AdminCommentEntry {
  string id = 1;
  string template_id = 2;
  string template_title = 3;
  string template_owner_id = 4;
  string template_owner_display_name = 5;
  string user_id = 6;            // 作者 id
  string user_display_name = 7;  // 作者显示名
  string user_email = 8;         // 作者邮箱（仅审核）
  string content = 9;
  google.protobuf.Timestamp created_at = 10;
  google.protobuf.Timestamp updated_at = 11;
}

message ListAdminCommentsRequest {
  string q = 1;          // 自由文本：匹配内容
  string template_id = 2;
  string user_id = 3;    // 作者筛选
  int32 page = 4;        // 1 起，默认 1
  int32 per_page = 5;    // 默认 20，上限 100
}

message ListAdminCommentsResponse {
  repeated AdminCommentEntry entries = 1;
  int64 total = 2;
  int32 page = 3;
  int32 per_page = 4;
}

message DeleteAdminCommentRequest {
  string comment_id = 1;
  string reason = 2;     // 3-500 字符，必填
}

message DeleteAdminCommentResponse {
  bool success = 1;
}
```

### 1.3 重新生成

```bash
cd backend && make proto
# 等价于：
protoc -I api/proto/v1 --go_out=. --go-grpc_out=. api/proto/v1/prompt.proto api/proto/v1/admin.proto
```

## 2. 数据库 DDL

追加到 **`deployment/init.sql` 与 `deployment/chart/open-prompts/init.sql` 两份**（字节一致的块）：

```sql
-- -----------------------------------------------------------------------------
-- Feature: template-comments
-- Description: 模板上的扁平评论。comment_count 在插入/删除时事务内维护
--              （与 like_count 相同）。
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS template_comments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content TEXT NOT NULL CHECK (char_length(content) BETWEEN 1 AND 2000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE template_comments IS '模板上的扁平评论（无盖楼）';
COMMENT ON COLUMN template_comments.content IS '纯文本，1-2000 字符，客户端仅以转义渲染';

CREATE INDEX IF NOT EXISTS idx_template_comments_template_created
    ON template_comments (template_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_template_comments_user_id ON template_comments (user_id);

-- 反规范化计数（与 like_count / favorite_count 同模式）。
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='templates' AND column_name='comment_count') THEN
        ALTER TABLE templates ADD COLUMN comment_count INT NOT NULL DEFAULT 0;
        COMMENT ON COLUMN templates.comment_count IS '评论数（事务内维护）';
    END IF;
END $$;

-- 放宽 admin_audit_logs 的 CHECK 以支持 delete_comment / comment 目标。
-- CHECK 约束无法 ALTER；以 pg_constraint 守护保证幂等：
-- 仅当现存定义缺少新值时才删除并重建。
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'admin_audit_logs'::regclass AND conname = 'admin_audit_logs_action_check';
    IF def IS NOT NULL AND def NOT LIKE '%delete_comment%' THEN
        ALTER TABLE admin_audit_logs DROP CONSTRAINT admin_audit_logs_action_check;
        ALTER TABLE admin_audit_logs ADD CONSTRAINT admin_audit_logs_action_check
            CHECK (action IN ('ban', 'unban', 'role_change', 'delete_comment'));
    END IF;
END $$;

DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'admin_audit_logs'::regclass AND conname = 'admin_audit_logs_target_type_check';
    IF def IS NOT NULL AND def NOT LIKE '%comment%' THEN
        ALTER TABLE admin_audit_logs DROP CONSTRAINT admin_audit_logs_target_type_check;
        ALTER TABLE admin_audit_logs ADD CONSTRAINT admin_audit_logs_target_type_check
            CHECK (target_type IN ('user', 'comment'));
    END IF;
END $$;
```

说明：

- 约束名是这些内联 CHECK 的 PostgreSQL 默认名（`<表>_<列>_check`）；守护块读取现存定义，因此部分迁移的数据库会在下一次执行时收敛。
- 若通知功能先落地，其 `notifications.type` CHECK 也必须容纳 `'comment'`——该放宽属于通知 DDL 块（见 §3.5 协调说明）。
- **用户删除重算（FR-18）：** 当前无用户删除 API。任何未来删除用户的功能必须在同一事务内，对被删用户评论过的每个模板重算 `comment_count`（在级联*之前*执行 `UPDATE templates t SET comment_count = (SELECT COUNT(*) FROM template_comments c WHERE c.template_id = t.id) WHERE t.id IN (SELECT DISTINCT template_id FROM template_comments WHERE user_id = $1)`，或删除后逐模板重算）。此处记录为长期义务。

## 3. 后端分层

### 3.1 模型（`backend/internal/models/comment.go`）

```go
// Comment 表示 "template_comments" 表中的一行。
type Comment struct {
    ID         string    `json:"id"`
    TemplateID string    `json:"template_id"`
    UserID     string    `json:"user_id"`
    Content    string    `json:"content"`
    CreatedAt  time.Time `json:"created_at"`
    UpdatedAt  time.Time `json:"updated_at"`

    // 读取时联表（仅公开身份）。
    AuthorDisplayName string `json:"author_display_name"`
    AuthorAvatar      string `json:"author_avatar"`

    // 服务层计算。
    IsEdited bool `json:"is_edited"`
}

// AdminComment 是带审核上下文的评论（管理端列表）。
type AdminComment struct {
    ID             string    `json:"id"`
    TemplateID     string    `json:"template_id"`
    TemplateTitle  string    `json:"template_title"`
    TemplateOwnerID string   `json:"template_owner_id"`
    OwnerDisplayName string  `json:"owner_display_name"`
    UserID         string    `json:"user_id"`
    UserDisplayName string   `json:"user_display_name"`
    UserEmail      string    `json:"user_email"`
    Content        string    `json:"content"`
    CreatedAt      time.Time `json:"created_at"`
    UpdatedAt      time.Time `json:"updated_at"`
}
```

`models.Template` 在 `FavoriteCount` 之后新增 `CommentCount int32 \`json:"comment_count"\``。

### 3.2 仓库（`backend/internal/repository/comment_repository.go`）

```go
type CommentRepository interface {
    List(ctx context.Context, templateID string, limit, offset int) ([]*models.Comment, int64, error)
    Get(ctx context.Context, id string) (*models.Comment, error)
    Create(ctx context.Context, c *models.Comment) error
    Update(ctx context.Context, id, content string) (*models.Comment, error)
    // Delete 在一个事务内删除评论并重算 templates.comment_count。
    // 返回 (deleted, newCount, error)；行不存在时 deleted=false。
    Delete(ctx context.Context, id string) (bool, int64, error)
}
```

关键语句：

```go
// List — 作者联表、最新在前、带总数的 offset 分页。
const listQuery = `
SELECT c.id, c.template_id, c.user_id, c.content, c.created_at, c.updated_at,
       u.display_name, COALESCE(u.avatar, '')
FROM template_comments c
JOIN users u ON u.id = c.user_id
WHERE c.template_id = $1
ORDER BY c.created_at DESC
LIMIT $2 OFFSET $3`

const countQuery = `SELECT COUNT(*) FROM template_comments WHERE template_id = $1`

// Create — 事务插入 + 计数维护（ToggleLike 模式）。
func (r *commentRepository) Create(ctx context.Context, c *models.Comment) error {
    tx, err := r.db.BeginTx(ctx, nil)
    if err != nil { return err }
    defer func() { _ = tx.Rollback() }()

    err = tx.QueryRowContext(ctx, `
INSERT INTO template_comments (template_id, user_id, content)
VALUES ($1, $2, $3)
RETURNING id, created_at, updated_at`,
        c.TemplateID, c.UserID, c.Content).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
    if err != nil { return err }

    if _, err = tx.ExecContext(ctx, `
UPDATE templates SET comment_count = comment_count + 1 WHERE id = $1`, c.TemplateID); err != nil {
        return err
    }
    return tx.Commit()
}

// Delete — 事务删除 + 重算（权威值，避免漂移）。
func (r *commentRepository) Delete(ctx context.Context, id string) (bool, int64, error) {
    tx, err := r.db.BeginTx(ctx, nil)
    if err != nil { return false, 0, err }
    defer func() { _ = tx.Rollback() }()

    var templateID string
    err = tx.QueryRowContext(ctx, `DELETE FROM template_comments WHERE id=$1 RETURNING template_id`, id).Scan(&templateID)
    if err == sql.ErrNoRows { return false, 0, nil }
    if err != nil { return false, 0, err }

    var count int64
    if err = tx.QueryRowContext(ctx,
        `SELECT COUNT(*) FROM template_comments WHERE template_id=$1`, templateID).Scan(&count); err != nil {
        return false, 0, err
    }
    if _, err = tx.ExecContext(ctx, `UPDATE templates SET comment_count=$1 WHERE id=$2`, count, templateID); err != nil {
        return false, 0, err
    }
    return true, count, tx.Commit()
}
```

`Update` 仅设置 `content` 与 `updated_at = NOW()`——绝不改 `created_at`，列表位置因此稳定（FR-3）。

`templateRepository.Get`/`List` 的 SELECT 列表与 Scan 目标增加 `t.comment_count`；`templateModelToProto` 将其映射到 `comment_count = 17`。

### 3.3 服务（`backend/internal/service/prompt.go`）

`PromptService` 新增 `CommentRepo repository.CommentRepository` 与 `Redis RedisStore`（接口已在 `user.go` 声明；nil 时停用限流）。`NewPromptService` 新增这两个参数——更新 `main.go` 中的唯一调用点。

```go
const (
    commentMaxLen        = 2000
    commentEditWindow    = 15 * time.Minute
    commentRatePerMinDef = 5
    commentRatePerDayDef = 50
)

func (s *PromptService) ListComments(ctx context.Context, req *pb.ListCommentsRequest) (*pb.ListCommentsResponse, error) {
    userID, _ := GetUserIDFromContext(ctx) // 可选认证——可见性跟随模板
    tpl, err := s.TemplateRepo.Get(ctx, req.TemplateId, userID)
    if err != nil {
        return nil, status.Error(codes.NotFound, "template not found")
    }
    page := int(req.Page); if page < 1 { page = 1 }
    size := int(req.PageSize); if size < 1 { size = 20 }; if size > 50 { size = 50 }

    comments, total, err := s.CommentRepo.List(ctx, tpl.ID, size, (page-1)*size)
    if err != nil { return nil, status.Error(codes.Internal, "failed to list comments") }
    // 映射到 pb.Comment；IsEdited = UpdatedAt.After(CreatedAt)；
    // IsOwner = c.UserID == tpl.OwnerID；IsOwn = c.UserID == userID
    ...
}

func (s *PromptService) CreateComment(ctx context.Context, req *pb.CreateCommentRequest) (*pb.Comment, error) {
    userID, err := GetUserIDFromContext(ctx)
    if err != nil { return nil, err }

    content := strings.TrimSpace(req.Content)
    if content == "" || len([]rune(content)) > commentMaxLen {
        return nil, status.Error(codes.InvalidArgument, "content must be 1-2000 characters")
    }

    user, err := s.UserRepo.GetByID(ctx, userID)
    if err != nil || user == nil {
        return nil, status.Error(codes.Unauthenticated, "user not found")
    }
    if user.Status == "banned" {
        return nil, status.Error(codes.PermissionDenied, "banned users cannot comment")
    }

    tpl, err := s.TemplateRepo.Get(ctx, req.TemplateId, userID)
    if err != nil { return nil, status.Error(codes.NotFound, "template not found") }
    if tpl.Visibility == "private" && tpl.OwnerID != userID {
        return nil, status.Error(codes.NotFound, "template not found") // 不可区分
    }

    if retryAfter, ok := s.checkCommentRate(ctx, userID); !ok {
        return nil, status.Error(codes.ResourceExhausted,
            fmt.Sprintf("rate limit exceeded, retry after %d seconds", retryAfter))
    }

    c := &models.Comment{TemplateID: tpl.ID, UserID: userID, Content: content}
    if err := s.CommentRepo.Create(ctx, c); err != nil {
        return nil, status.Error(codes.Internal, "failed to create comment")
    }

    // FR-17：写入提交后，尽力而为地通知所有者。
    s.createEngagementNotification(ctx, "comment", tpl.ID, tpl.OwnerID, userID)

    return s.commentModelToProto(c, tpl, userID), nil
}
```

**限流**——Redis 固定窗口计数器，管理员限流模式（`Get`/`Sscanf`/`Set`；`RedisStore` 接口没有 `INCR`/`TTL`）。存储值为 `"count:expires_unix"`，以便计算 `Retry-After` 的剩余秒数：

```go
// checkCommentRate 执行 COMMENT_RATE_PER_MINUTE 与 COMMENT_RATE_PER_DAY。
// 超限时返回 (retryAfterSeconds, false)。Redis 不可用时（s.Redis == nil 或
// Get/Set 出错）fail-open，与管理员限流一致。
func (s *PromptService) checkCommentRate(ctx context.Context, userID string) (int, bool) {
    if s.Redis == nil { return 0, true }
    limits := []struct{ key, max string; window time.Duration }{
        {"comment_rate_min:" + userID, os.Getenv("COMMENT_RATE_PER_MINUTE"), time.Minute},
        {"comment_rate_day:" + userID, os.Getenv("COMMENT_RATE_PER_DAY"), 24 * time.Hour},
    }
    minRetry := 0
    for _, l := range limits {
        max, _ := strconv.Atoi(l.max)
        if max <= 0 { max = defaultFor(l.window) } // 5 / 50
        v, err := s.Redis.Get(ctx, l.key)
        if err != nil || v == "" {
            _ = s.Redis.Set(ctx, l.key, "1:"+strconv.FormatInt(time.Now().Add(l.window).Unix(), 10), l.window)
            continue
        }
        var count int; var expires int64
        if _, err := fmt.Sscanf(v, "%d:%d", &count, &expires); err != nil {
            _ = s.Redis.Set(ctx, l.key, "1:"+strconv.FormatInt(time.Now().Add(l.window).Unix(), 10), l.window)
            continue
        }
        if time.Now().Unix() >= expires { // 窗口已过：重置
            _ = s.Redis.Set(ctx, l.key, "1:"+strconv.FormatInt(time.Now().Add(l.window).Unix(), 10), l.window)
            continue
        }
        if count >= max {
            if r := int(expires - time.Now().Unix()); r > minRetry { minRetry = r }
            continue
        }
        _ = s.Redis.Set(ctx, l.key, fmt.Sprintf("%d:%d", count+1, expires), time.Duration(expires-time.Now().Unix())*time.Second)
    }
    if minRetry > 0 { return minRetry, false }
    return 0, true
}
```

网关读取 `ResourceExhausted` 消息（或 grpc-web 风格 trailer）并设置 `Retry-After: <seconds>`（§3.4）。两个计数器仅在创建真正进行时递增；被拒绝的创建绝不消耗配额。

`UpdateComment`：加载评论（缺失 `404`）→ 作者检查（`403`）→ `time.Since(c.CreatedAt) > commentEditWindow` → `403 "editable window expired — delete and repost instead"` → 校验内容 → `CommentRepo.Update`。被封禁的作者同样被拒（与创建相同的检查）。

`DeleteComment`：加载评论（`404`）→ 调用者为作者**或** `tpl.OwnerID == userID`（模板经 `TemplateRepo.Get` 加载；模板已不存在则 `404`）→ `CommentRepo.Delete`。不写审计（FR-10）。

### 3.4 管理服务（`backend/internal/service/admin.go`）

```go
func (s *AdminService) ListAdminComments(ctx context.Context, req *pb.ListAdminCommentsRequest) (*pb.ListAdminCommentsResponse, error) {
    if _, err := s.requireAdminContext(ctx); err != nil { return nil, err }
    page := int(req.Page); if page < 1 { page = 1 }
    perPage := int(req.PerPage); if perPage < 1 { perPage = 20 }; if perPage > 100 { perPage = 100 }

    entries, total, err := s.AdminRepo.ListComments(ctx, repository.CommentFilters{
        Search: strings.TrimSpace(req.Q), TemplateID: req.TemplateId, UserID: req.UserId,
    }, perPage, (page-1)*perPage)
    ... // 映射到 pb.AdminCommentEntry
}

func (s *AdminService) DeleteAdminComment(ctx context.Context, req *pb.DeleteAdminCommentRequest) (*pb.DeleteAdminCommentResponse, error) {
    admin, err := s.requireAdminContext(ctx)
    if err != nil { return nil, err }
    reason := strings.TrimSpace(req.Reason)
    if len(reason) < 3 || len(reason) > 500 {
        return nil, status.Error(codes.InvalidArgument, "reason must be 3-500 characters")
    }
    deleted, _, err := s.AdminRepo.DeleteComment(ctx, req.CommentId) // 事务：删除 + 重算
    if err != nil { return nil, status.Error(codes.Internal, "failed to delete comment") }
    if !deleted { return nil, status.Error(codes.NotFound, "comment not found") }

    _ = s.AdminRepo.InsertAuditLog(ctx, &models.AdminAuditLog{
        ActorID: admin.ID, ActorEmail: admin.Email,
        Action: "delete_comment", TargetType: "comment",
        TargetID: req.CommentId, Reason: reason,
    }) // 审计失败仅记录日志；删除已提交（与 ban/unban 行为一致）
    return &pb.DeleteAdminCommentResponse{Success: true}, nil
}
```

`adminRepository.ListComments` 联表 `templates` + 两次 `users`（所有者、作者）获取审核上下文；`q` 匹配 `c.content ILIKE '%…%'` 并转义 `%`/`_`（既有管理端搜索先例）；`template_id`/`user_id` 为精确匹配。`adminRepository.DeleteComment` 复用与用户仓库相同的事务删除 + 重算（共享辅助函数或复制语句——由实现者选择，测试须覆盖两者）。

### 3.5 通知钩子协调

`createEngagementNotification`（prompt.go）本就对类型字符串泛化。两个集成点：

1. **枚举/DDL：** `notification.proto` 的 `type` 注释与 `notifications` DDL CHECK 必须包含 `'comment'`。若通知先落地，评论在其 DDL 中追加放宽的 `DO $$` 块（如 §2 以 `pg_get_constraintdef` 守护）；若评论先落地，钩子调用点照写，但 `NotificationRepo == nil` 使其空操作直至通知装配该字段（FR-17 顺延）。
2. **去重：** 既有 `INSERT … WHERE NOT EXISTS（相同未读行）` 原样适用——同一用户在同一模板的第二条评论在首条通知未读时不产生重复，符合 FR-17 的"遵循去重规则"。

### 3.6 网关装配（`backend/cmd/server/main.go`）

用户路由——位于既有 `/api/v1/templates/` handler 内、`/like` 与 `/favorite` 分支之后（与 `/aliases` 相同的 `strings.Split` 子资源模式）：

```go
// parts[1] == "comments"：GET（公开读，可选认证）/ POST（需认证）
if len(parts) == 2 && parts[1] == "comments" {
    switch r.Method {
    case http.MethodGet:
        // 可选认证：无令牌时 userID = ""（同 GetTemplate）
        resp, err := svc.ListComments(ctx, &pb.ListCommentsRequest{
            TemplateId: parts[0], Page: pageParam(r), PageSize: sizeParam(r),
        })
        ...
    case http.MethodPost:
        // VerifyAnyToken → ContextWithUserID（同 /like）
        ...
    default: http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    }
    return
}
```

评论资源的顶级 handler：

```go
http.HandleFunc("/api/v1/comments/", func(w http.ResponseWriter, r *http.Request) {
    // PUT（编辑）/ DELETE（作者或所有者删除）；均需认证
    id := strings.TrimPrefix(r.URL.Path, "/api/v1/comments/")
    if id == "" { http.Error(w, "ID required", http.StatusBadRequest); return }
    // VerifyAnyToken → ctx；按方法路由到 UpdateComment / DeleteComment
})
```

管理路由（位于 `/api/v1/admin/audit-log` 之后）：

```go
http.HandleFunc("/api/v1/admin/comments", func(w http.ResponseWriter, r *http.Request) {
    // 仅 GET；requireAdmin；q/template_id/user_id/page/per_page 查询参数
})

http.HandleFunc("/api/v1/admin/comments/", func(w http.ResponseWriter, r *http.Request) {
    // 仅 DELETE；requireAdmin；请求体 {"reason": "..."}；在 ResourceExhausted
    //（此处不应出现）或 429 路径上网关设置 Retry-After
})
```

**`Retry-After`：** 在 `writeError` 中，当 gRPC 码为 `ResourceExhausted` 时，从消息尾部解析秒数（服务层格式化为 `"rate limit exceeded, retry after N seconds"`）并在写 429 前设置 `w.Header().Set("Retry-After", N)`。这是通用改进——网关中不出现评论专属分支。

## 4. 前端设计

### 4.1 文件布局

```
frontend/src/components/comments/
├── CommentsSection.js / .scss   // 区块外壳：计数、列表、加载更多、错误/重试
├── CommentComposer.js / .scss   // 带标签 textarea、字符计数、提交
├── CommentItem.js / .scss       // 行：作者、头像、时间、内容、[CREATOR]、操作
└── CommentEditForm.js / .scss   // 内联编辑：textarea + 保存/取消
frontend/src/pages/admin/CommentsPage.js / .scss   // 审核列表
```

### 4.2 状态

`CommentsSection` 内的组件局部状态（不新增 Redux slice——页面级作用域，同 `TemplateDetails`）：

```js
const [comments, setComments] = useState([]);
const [page, setPage] = useState(1);
const [totalCount, setTotalCount] = useState(0);
const [pageSize] = useState(20);
const [loading, setLoading] = useState(false);
const [error, setError] = useState(null);   // 区块级，绝不阻塞页面
const [draft, setDraft] = useState('');     // 失败时保留（NFR-8）
const [editingId, setEditingId] = useState(null);
```

`TemplateDetails` 以 props 传入 `template.id`、`template.owner_id` 与认证 `user`（用于 `is_own`）。头部计数与 `total_count` 同步，首次加载时与模板的 `comment_count` 对齐。

### 4.3 组件

- **CommentsSection** — 标题 `t('comments.title')` + 计数；渲染列表、输入框（仅认证用户，否则显示指向 `/login` 的登录提示）、`comments.length < totalCount` 时的加载更多按钮；加载失败时区块级错误横幅 + 重试。
- **CommentComposer** — `<label>` + textarea（`maxLength` 2000）、实时字符计数、提交期间禁用；429 时显示 `t('comments.error_rate_limited')` 及返回的秒数；400 时显示字段错误并保留草稿。
- **CommentItem** — 头像、显示名、`is_owner` 时的 `[CREATOR]` 徽章、相对时间（`utils/time.js`）、`is_edited` 时的"已编辑"标记、以 `white-space: pre-wrap` 文本节点渲染的内容；`is_own` 时悬停/聚焦显示 `编辑`/`删除`，查看者为模板所有者时显示 `删除`。
- **CommentEditForm** — 预填 textarea、保存/取消；403 窗口过期时显示 toast 并退出编辑模式。
- **AdminCommentsPage** — 镜像 `TemplatesPage`：`AdminSearchInput`（q）+ 模板/作者筛选 + `AdminTable` + `Pagination`；删除打开 `ReasonModal`（复用）→ `deleteAdminComment(id, reason)`；行链接到 `/admin/templates/:id`。
- **Admin TemplateDetailPage** — 新增"评论"面板：同一表格（按模板筛选）、同一删除流程。

### 4.4 i18n 键（双语言，`comments.*` + `admin.comments.*`）

```json
{
  "comments": {
    "title": "评论",
    "count_one": "{{count}} 条评论",
    "count_other": "{{count}} 条评论",
    "composer_label": "发表评论",
    "placeholder": "分享你使用这个模板的体验…",
    "post": "发布",
    "posting": "发布中…",
    "cancel": "取消",
    "save": "保存",
    "saving": "保存中…",
    "edit": "编辑",
    "delete": "删除",
    "edited": "已编辑",
    "creator_badge": "创作者",
    "char_count": "{{count}} / 2000",
    "load_more": "加载更多",
    "empty": "还没有评论。来抢沙发吧。",
    "sign_in_to_comment": "登录后评论",
    "confirm_delete": "删除这条评论？",
    "confirm_delete_owner": "删除你模板上的这条评论？",
    "error_load": "评论加载失败",
    "error_post": "评论发布失败",
    "error_edit": "评论编辑失败",
    "error_delete": "评论删除失败",
    "error_content_length": "评论须为 1-2000 个字符",
    "error_rate_limited": "评论过于频繁，请 {{seconds}} 秒后重试。",
    "error_window_expired": "编辑窗口已过期——请删除后重新发布。",
    "error_banned": "你的账号无法发表评论。"
  },
  "admin": {
    "nav": { "comments": "评论" },
    "comments": {
      "title": "评论审核",
      "search_placeholder": "搜索评论内容…",
      "filter_template": "模板 ID",
      "filter_author": "作者 ID",
      "col_content": "评论",
      "col_template": "模板",
      "col_author": "作者",
      "col_created": "发布时间",
      "empty": "未找到评论",
      "delete_reason_title": "删除评论",
      "deleted": "评论已删除"
    }
  }
}
```

英文 locale 镜像全部键（见英文详细设计 §4.4）。`admin.nav.comments` 插入既有 `admin.nav` 块。

### 4.5 API 客户端新增

`frontend/src/services/api.js`：

```js
export const listTemplateComments = (templateId, params) =>
  api.get(`/templates/${templateId}/comments`, { params });
export const createTemplateComment = (templateId, content) =>
  api.post(`/templates/${templateId}/comments`, { content });
export const updateComment = (commentId, content) =>
  api.put(`/comments/${commentId}`, { content });
export const deleteComment = (commentId) =>
  api.delete(`/comments/${commentId}`);
```

`frontend/src/services/adminApi.js`：

```js
export const getAdminComments = (params) => adminApi.get('/admin/comments', { params });
export const deleteAdminComment = (id, reason) =>
  adminApi.delete(`/admin/comments/${id}`, { data: { reason } });
```

## 5. 错误矩阵

| HTTP | gRPC 码 | 条件 | 用户可见消息 |
|---|---|---|---|
| 400 | `InvalidArgument` | 内容为空 / trim 后超 2000 | `comments.error_content_length` |
| 400 | `InvalidArgument` | 管理端理由不在 3–500 字符 | `admin.common.error_reason_length`（既有） |
| 400 | `InvalidArgument` | `page`/`page_size`/`per_page` 非数字 | 通用 400 |
| 401 | `Unauthenticated` | POST/PUT/DELETE 令牌缺失/无效 | 既有认证 toast |
| 403 | `PermissionDenied` | 封禁用户发帖 | `comments.error_banned` |
| 403 | `PermissionDenied` | 非作者编辑 | `comments.error_edit`（通用） |
| 403 | `PermissionDenied` | 编辑窗口过期 | `comments.error_window_expired` |
| 403 | `PermissionDenied` | 非作者/非所有者删除 | `comments.error_delete`（通用） |
| 403 | `PermissionDenied` | 非管理员访问管理路由 | 既有管理端 403 |
| 404 | `NotFound` | 模板/评论不存在；非所有者访问私有模板 | `comments.error_load` / 通用 |
| 429 | `ResourceExhausted` | 限流（5/分钟 或 50/天） | `comments.error_rate_limited` + `Retry-After` 头 |
| 500 | `Internal` | 数据库/Redis 故障 | 通用错误 toast |

## 6. 部署

- 将 §2 DDL 块追加到两份 `init.sql`；存量部署重启即收敛（全部语句幂等）。
- `deployment/docker-compose.yml`（backend 服务 `environment:`）与 helm `values.yaml` + deployment 模板：

  ```yaml
  - COMMENT_RATE_PER_MINUTE=5
  - COMMENT_RATE_PER_DAY=50
  ```

  均可选；默认值已编译进代码。
- 照常重建后端（`protoc` + `go build`）与前端。无新服务、端口、卷或迁移工具。

## 7. SDK / CLI

`python-sdk` 新增透传辅助方法（评论是模板级资源，镜像 `list_template_versions`）：

```python
def list_comments(self, template_id, page=1, page_size=20): ...
def create_comment(self, template_id, content): ...
def update_comment(self, comment_id, content): ...
def delete_comment(self, comment_id): ...
```

v1 无 CLI 表面。

## 8. 测试策略

### 8.1 FVT（`tests/fvt.py`）— 开发者

新增 `test_template_comments()`，沿用 `test_likes_and_favorites()` 的双用户模式：

1. 所有者创建公开模板；用户 B 发表评论 → 200，出现在列表，`comment_count` = 1。
2. 匿名 GET 列表 → 200（公开模板）。
3. 用户 B 窗口内编辑 → 200，`is_edited` 为 true，位置不变。
4. 用户 C（非作者）编辑/删除 → 403。
5. 所有者经 `DELETE /api/v1/comments/{id}` 删除 B 的评论 → 200，计数归 0。
6. 封禁用户（经管理 API 设置）发帖 → 403。
7. 限流：一分钟内第 6 条评论 → 429 且存在 `Retry-After` 头。
8. 管理端：`GET /api/v1/admin/comments?q=…` 筛选；`DELETE /api/v1/admin/comments/{id}` 无理由 → 400；带理由 → 200 + 审计行（`action=delete_comment`）可在 `/api/v1/admin/audit-log` 看到。
9. 删除模板 → 评论消失（经管理端列表缺失验证级联）。
10. 私有模板：非所有者 GET 评论 → 404；所有者 → 200。

### 8.2 后端单元测试（新文件 ≥ 80 %）

- 内容校验边界（0、1、2000、2001、Unicode、纯空白）。
- 编辑窗口边界（注入时钟或测试用短窗口下的 14:59 与 15:01）。
- 权限矩阵：作者/所有者/他人 × 编辑/删除。
- 以假 `RedisStore` 验证限流窗口数学（count:expires 解析、重置、出错 fail-open）。
- 创建/删除的事务重算（mock DB 或集成）。
- 管理端理由校验；审计插入以正确字段被调用。
- `MockTemplateRepository` 为 `comment_count` 更新；既有测试保持通过。

### 8.3 E2E（Nightwatch，test-agent）

- `/templates/:id` 评论区渲染与计数；发帖/编辑/删除流程；加载更多；所有者在全部行可见删除；管理端 `/admin/comments` 筛选 + 理由弹窗；EN/ZH 文案一致；仅键盘遍历输入框与操作。

## 9. 实现顺序（建议）

1. DDL（两份 `init.sql`）+ 模型（`Comment`、`Template.CommentCount`）。
2. Proto 变更 + 重新生成；`templateModelToProto` 映射 `comment_count`。
3. `CommentRepository`（+ 管理仓库方法），含事务计数维护。
4. `PromptService` 评论 RPC（校验、权限、编辑窗口、限流）+ 网关路由。
5. `AdminService` 方法 + 管理网关路由 + 审计。
6. 前端：`TemplateDetails` 上的 `CommentsSection` 家族；管理端 `CommentsPage` + 详情面板；双语言 i18n。
7. FVT + 单元测试；python-sdk 辅助方法。
8. 通知枚举/DDL 协调（§3.5）——后落地的一方负责放宽 CHECK。

## 10. 给开发者的约束（必须遵守）

1. **绝不提交 `backend/server` 的变更**（被跟踪的 26 MB 二进制）——提交前若显示修改，先 `git checkout -- backend/server`。
2. 分层：proto → 网关（`main.go`）→ 服务 → 仓库 → 模型；不新增顶层包。
3. 用户路由位于 `/api/v1/templates/{id}/comments` 与 `/api/v1/comments/{id}`；管理路由位于 `/api/v1/admin/comments*`——绝不混用表面。
4. 用户 ID 永远来自令牌（`ContextWithUserID`），绝不来自请求体；每次变更都在服务端做所有权检查。
5. `comment_count` 仅在创建/删除事务内维护（插入递增、删除重算）——v1 不做后台对账。
6. 仅纯文本：绝不将评论内容作为 HTML 渲染；该路径禁止 `dangerouslySetInnerHTML`。
7. Redis 不可用时限流 fail-open；429 响应携带 `Retry-After`。
8. 管理员删除必须携带 3–500 字符理由并写入 `admin_audit_logs`（`action='delete_comment'`、`target_type='comment'`）。
9. `init.sql` 变更幂等并镜像到 helm chart 副本（字节一致的块）。
10. i18n：每个新的用户可见字符串都必须同时进入 `en` 与 `zh` locale（`comments.*`、`admin.comments.*`、`admin.nav.comments`）。
11. 常规格式提交，英文；新 Go 文件单元测试覆盖率 ≥ 80 %；推送前全部 CI 门禁通过。
12. 若通知尚未落地，保留 `createEngagementNotification(ctx, "comment", …)` 调用——repo 为 nil 时空操作（FR-17 顺延）；不要构建独立回退。
