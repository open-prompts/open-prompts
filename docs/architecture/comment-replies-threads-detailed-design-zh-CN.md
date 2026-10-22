# 评论回复与话题 — 详细设计

| | |
|---|---|
| **功能 ID** | comment-replies-threads |
| **版本** | 1.0 |
| **状态** | 已批准实施 |
| **作者** | Architect Agent |
| **上游** | [架构设计](comment-replies-threads-architecture-zh-CN.md) |

## 1. Proto 契约

### 1.1 `backend/api/proto/v1/prompt.proto`

**`Comment` 消息** —— 追加两个字段（下一个空闲标签 10、11）：

```protobuf
message Comment {
  string id = 1;
  string template_id = 2;
  string content = 3;
  google.protobuf.Timestamp created_at = 4;
  google.protobuf.Timestamp updated_at = 5;
  bool is_edited = 6;
  CommentAuthor author = 7;
  bool is_owner = 8;
  bool is_own = 9;
  // 直接回复数（仅顶层评论；回复为 0）。
  int32 reply_count = 10;
  // 模板所有者是否已在此话题中回复（仅顶层）。
  bool has_creator_reply = 11;
}
```

**`CreateCommentRequest`** —— 追加一个字段：

```protobuf
message CreateCommentRequest {
  string template_id = 1;
  string content = 2;   // 修剪后 1-2000 字符
  // 可选。设置时必须引用同一模板上存在的顶层评论；回复一条回复会平铺到顶层话题。
  string parent_comment_id = 3;
}
```

**`PromptService` 上的新 RPC**（在 `DeleteComment` 之后）：

```protobuf
// ListCommentReplies 列出某顶层评论的所有回复，最旧优先，不分页
// （受 50 条回复话题上限约束）。
rpc ListCommentReplies(ListCommentRepliesRequest) returns (ListCommentRepliesResponse);
```

新消息：

```protobuf
message ListCommentRepliesRequest {
  string comment_id = 1;  // 必须是顶层评论
}

message ListCommentRepliesResponse {
  repeated Comment replies = 1;  // 最旧优先
}
```

### 1.2 `backend/api/proto/v1/notification.proto`

**`Notification` 消息** —— 追加一个字段（下一个空闲标签 10）：

```protobuf
message Notification {
  string id = 1;
  string type = 2;               // "like" | "favorite" | "fork" | "comment" | ...
  string actor_id = 3;
  string actor_display_name = 4;
  string actor_avatar = 5;
  string template_id = 6;
  string template_title = 7;
  string read_at = 8;
  string created_at = 9;
  // ≤100 字符摘录；仅 comment_reply 事件填充。
  string snippet = 10;
}
```

### 1.3 重新生成

```bash
cd backend && make proto
# 等价于：
protoc -I api/proto/v1 --go_out=. --go-grpc_out=. \
  api/proto/v1/prompt.proto api/proto/v1/notification.proto
```

## 2. 数据库 DDL

追加到 **两个** `deployment/init.sql` 和 `deployment/chart/open-prompts/init.sql`（字节一致块）：

```sql
-- -----------------------------------------------------------------------------
-- Feature: comment-replies-threads
-- Description: template_comments 上的单层话题。reply_count 在回复插入/删除时
--              事务性维护；comment_count 仅统计顶层评论（语义不变）。
-- -----------------------------------------------------------------------------

-- parent_comment_id：顶层评论为 null；引用一条顶层评论。无外键（级联在服务层
-- 处理，与已上线删除流程一致）。reply_count = 直接回复数。
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='template_comments' AND column_name='parent_comment_id') THEN
        ALTER TABLE template_comments ADD COLUMN parent_comment_id UUID;
        COMMENT ON COLUMN template_comments.parent_comment_id IS
            '顶层评论为 null；引用一条顶层评论（无外键，服务层强制）';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='template_comments' AND column_name='reply_count') THEN
        ALTER TABLE template_comments ADD COLUMN reply_count INT NOT NULL DEFAULT 0;
        COMMENT ON COLUMN template_comments.reply_count IS
            '直接回复数（事务性维护）';
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_template_comments_parent_created
    ON template_comments (parent_comment_id, created_at ASC);

-- 为 comment_reply 事件类型扩展 notifications.type CHECK（受保护、幂等；
-- 若部分迁移的数据库缺少它，下次运行收敛）。
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'notifications'::regclass AND conname = 'notifications_type_check';
    IF def IS NOT NULL AND def NOT LIKE '%comment_reply%' THEN
        ALTER TABLE notifications DROP CONSTRAINT notifications_type_check;
        ALTER TABLE notifications ADD CONSTRAINT notifications_type_check
            CHECK (type IN ('like', 'favorite', 'fork', 'comment',
                            'template_approved', 'template_rejected',
                            'template_warning_changed', 'follow',
                            'appeal_resolved', 'comment_reply'));
    END IF;
END $$;

-- 向 notifications 添加 snippet 列（≤100 字符回复摘录）。
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='notifications' AND column_name='snippet') THEN
        ALTER TABLE notifications ADD COLUMN snippet TEXT;
        COMMENT ON COLUMN notifications.snippet IS
            '≤100 字符摘录；仅 comment_reply 事件填充';
    END IF;
END $$;
```

说明：

- `parent_comment_id`/`reply_count`/`snippet` 列添加受 `information_schema.columns` 保护，因此幂等且增量。
- notifications CHECK 扩展遵循现有 `pg_constraint` 保护模式；若部分迁移的数据库缺少 `comment_reply`，下次运行收敛。
- 现有平铺评论（所有行 `parent_comment_id` 为 null、`reply_count` = 0）渲染与今天完全一致 —— 无需回填（NFR-7）。

## 3. 后端分层

### 3.1 模型（`backend/internal/models/comment.go`）

```go
type Comment struct {
    ID         string    `json:"id"`
    TemplateID string    `json:"template_id"`
    UserID     string    `json:"user_id"`
    Content    string    `json:"content"`
    CreatedAt  time.Time `json:"created_at"`
    UpdatedAt  time.Time `json:"updated_at"`

    // 新增（话题）：
    ParentCommentID string `json:"parent_comment_id"` // 顶层为 ""
    ReplyCount      int32  `json:"reply_count"`        // 直接回复
    HasCreatorReply bool   `json:"has_creator_reply"`  // 仅顶层

    // 读取时连接（仅公开身份）：
    AuthorDisplayName string `json:"author_display_name"`
    AuthorAvatar      string `json:"author_avatar"`

    // 服务层计算：
    IsEdited bool `json:"is_edited"`
}
```

`models.Notification` 新增 `Snippet string \`json:"snippet"\``。

### 3.2 仓库（`backend/internal/repository/comment_repository.go`）

```go
type CommentRepository interface {
    // List 返回一页顶层评论（最新优先）及模板的顶层评论总数。每条评论携带
    // ReplyCount 和 HasCreatorReply（通过子查询计算，无 N+1）。
    List(ctx context.Context, templateID string, limit, offset int) ([]*models.Comment, int64, error)
    // ListReplies 返回某顶层评论的所有回复，最旧优先。
    ListReplies(ctx context.Context, parentID string) ([]*models.Comment, error)
    // Get 按 id 加载单条评论。
    Get(ctx context.Context, id string) (*models.Comment, error)
    // Create 插入一条评论（顶层或回复）并在一个事务中维护相应计数器：
    // 顶层递增 comment_count，回复递增父评论的 reply_count。
    Create(ctx context.Context, c *models.Comment) error
    // Update 设置 content 和 updated_at（绝不设置 created_at）。
    Update(ctx context.Context, id, content string) (*models.Comment, error)
    // Delete 删除一条评论。若是顶层评论，其回复在同一事务中删除并重算
    // comment_count。若是回复，递减其父评论的 reply_count。返回
    // (deleted, error)；行不存在时 deleted=false。
    Delete(ctx context.Context, id string) (bool, error)
}
```

关键语句：

```go
// List —— 仅顶层，最新优先，通过相关子查询带 reply_count + has_creator_reply
// （无 N+1；列表页 ≤ 50 行）。
const listQuery = `
SELECT c.id, c.template_id, c.user_id, c.content, c.created_at, c.updated_at,
       u.display_name, COALESCE(u.avatar, ''),
       c.reply_count,
       EXISTS (
           SELECT 1 FROM template_comments r
           WHERE r.parent_comment_id = c.id AND r.user_id = t.owner_id
       ) AS has_creator_reply
FROM template_comments c
JOIN users u ON u.id = c.user_id
JOIN templates t ON t.id = c.template_id
WHERE c.template_id = $1 AND c.parent_comment_id IS NULL
ORDER BY c.created_at DESC
LIMIT $2 OFFSET $3`

const countQuery = `
SELECT COUNT(*) FROM template_comments
WHERE template_id = $1 AND parent_comment_id IS NULL`

// ListReplies —— 最旧优先（对话式顺序），受 50 上限约束。
const listRepliesQuery = `
SELECT c.id, c.template_id, c.user_id, c.content, c.created_at, c.updated_at,
       u.display_name, COALESCE(u.avatar, ''),
       c.reply_count
FROM template_comments c
JOIN users u ON u.id = c.user_id
WHERE c.parent_comment_id = $1
ORDER BY c.created_at ASC`

// Create —— 事务性插入 + 计数器维护。
func (r *commentRepository) Create(ctx context.Context, c *models.Comment) error {
    tx, err := r.db.BeginTx(ctx, nil)
    if err != nil { return err }
    defer func() { _ = tx.Rollback() }()

    err = tx.QueryRowContext(ctx, `
INSERT INTO template_comments (template_id, user_id, content, parent_comment_id)
VALUES ($1, $2, $3, NULLIF($4, ''))
RETURNING id, created_at, updated_at`,
        c.TemplateID, c.UserID, c.Content, c.ParentCommentID).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
    if err != nil { return err }

    if c.ParentCommentID == "" {
        // 顶层：递增 templates.comment_count。
        if _, err = tx.ExecContext(ctx,
            `UPDATE templates SET comment_count = comment_count + 1 WHERE id = $1`, c.TemplateID); err != nil {
            return err
        }
    } else {
        // 回复：递增父评论的 reply_count。
        if _, err = tx.ExecContext(ctx,
            `UPDATE template_comments SET reply_count = reply_count + 1 WHERE id = $1`, c.ParentCommentID); err != nil {
            return err
        }
    }
    return tx.Commit()
}

// Delete —— 事务性级联 + 计数器维护。
func (r *commentRepository) Delete(ctx context.Context, id string) (bool, error) {
    tx, err := r.db.BeginTx(ctx, nil)
    if err != nil { return false, err }
    defer func() { _ = tx.Rollback() }()

    var templateID, parentID string
    err = tx.QueryRowContext(ctx,
        `DELETE FROM template_comments WHERE id = $1 RETURNING template_id, COALESCE(parent_comment_id, '')`,
        id).Scan(&templateID, &parentID)
    if err == sql.ErrNoRows { return false, nil }
    if err != nil { return false, err }

    if parentID == "" {
        // 顶层：删除其回复，然后重算 comment_count。
        if _, err = tx.ExecContext(ctx,
            `DELETE FROM template_comments WHERE parent_comment_id = $1`, id); err != nil {
            return false, err
        }
        var count int64
        if err = tx.QueryRowContext(ctx,
            `SELECT COUNT(*) FROM template_comments WHERE template_id = $1 AND parent_comment_id IS NULL`,
            templateID).Scan(&count); err != nil {
            return false, err
        }
        if _, err = tx.ExecContext(ctx,
            `UPDATE templates SET comment_count = $1 WHERE id = $2`, count, templateID); err != nil {
            return false, err
        }
    } else {
        // 回复：递减父评论的 reply_count（下限 0）。
        if _, err = tx.ExecContext(ctx,
            `UPDATE template_comments SET reply_count = GREATEST(reply_count - 1, 0) WHERE id = $1`,
            parentID); err != nil {
            return false, err
        }
    }
    return true, tx.Commit()
}
```

`adminRepository.ListComments`/`DeleteComment` 增加父评论上下文：列表查询连接父评论作者的显示名（用于 `parent` 列），`DeleteComment` 遵循与 `CommentRepository.Delete` 相同的级联语义。

### 3.3 服务（`backend/internal/service/comment.go`）

**`CreateComment`** —— 在模板可见性检查之后、速率限制检查之前添加 `parent_comment_id` 处理：

```go
const commentThreadCap = 50

// 解析父评论（如有）。必须是同一模板上的顶层评论。
parentID := strings.TrimSpace(req.ParentCommentId)
if parentID != "" {
    parent, err := s.CommentRepo.Get(ctx, parentID)
    if err == sql.ErrNoRows {
        return nil, status.Error(codes.InvalidArgument, "parent comment not found")
    }
    if err != nil {
        return nil, status.Error(codes.Internal, "failed to load parent comment")
    }
    if parent.TemplateID != tpl.ID {
        return nil, status.Error(codes.InvalidArgument, "parent comment is on a different template")
    }
    if parent.ParentCommentID != "" {
        return nil, status.Error(codes.InvalidArgument, "parent must be a top-level comment")
    }
    if parent.ReplyCount >= commentThreadCap {
        return nil, status.Error(codes.InvalidArgument, "thread is full — post a new comment instead")
    }
}
```

然后在 `Create` 之前设置 `c.ParentCommentID = parentID`。`comment_reply` 通知在写入提交后发出，仅当 `parentID != ""` 且父评论作者与回复者不同时：

```go
if parentID != "" {
    // 通知父评论作者（而非模板所有者）。
    s.notifyCommentReply(ctx, tpl, parentID, userID, content)
}
```

**`ListComments`** —— 形状不变，但仓库现在仅返回带 `reply_count`/`has_creator_reply` 的顶层；`commentModelToProto` 映射两个新字段。

**新 `ListCommentReplies`**：

```go
func (s *PromptService) ListCommentReplies(ctx context.Context, req *pb.ListCommentRepliesRequest) (*pb.ListCommentRepliesResponse, error) {
    userID, _ := GetUserIDFromContext(ctx) // 可选认证 —— 可见性遵循模板

    parent, err := s.CommentRepo.Get(ctx, req.CommentId)
    if err == sql.ErrNoRows {
        return nil, status.Error(codes.NotFound, "comment not found")
    }
    if err != nil {
        return nil, status.Error(codes.Internal, "failed to load comment")
    }
    if parent.ParentCommentID != "" {
        return nil, status.Error(codes.InvalidArgument, "replies are only available for top-level comments")
    }

    tpl, err := s.TemplateRepo.Get(ctx, parent.TemplateID, userID)
    if err != nil {
        return nil, status.Error(codes.NotFound, "template not found")
    }
    if tpl.Visibility == "private" && tpl.OwnerID != userID {
        return nil, status.Error(codes.NotFound, "template not found") // 不可区分
    }

    replies, err := s.CommentRepo.ListReplies(ctx, parent.ID)
    if err != nil {
        return nil, status.Error(codes.Internal, "failed to list replies")
    }
    resp := &pb.ListCommentRepliesResponse{Replies: make([]*pb.Comment, 0, len(replies))}
    for _, r := range replies {
        resp.Replies = append(resp.Replies, s.commentModelToProto(r, tpl, userID))
    }
    return resp, nil
}
```

**`DeleteComment`** —— 授权不变（作者或模板所有者）；仓库现在级联回复并维护两个计数器。

**`commentModelToProto`** —— 映射两个新字段：

```go
ReplyCount:      c.ReplyCount,
HasCreatorReply: c.HasCreatorReply,
```

**`notifyCommentReply`** —— 尽力而为，镜像 `createEngagementNotification`，但目标是父评论作者并携带摘录：

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
        return // 回复自己的评论不发通知
    }
    actor, err := s.UserRepo.GetByID(ctx, actorID)
    if err != nil || actor == nil {
        zap.S().Warnf("comment_reply hook: failed to load actor %s: %v", actorID, err)
        return
    }
    displayName := actor.DisplayName
    if displayName == "" {
        displayName = actor.ID
    }
    snippet := content
    if r := []rune(snippet); len(r) > 100 {
        snippet = string(r[:100])
    }
    if err := s.NotificationRepo.Create(ctx, &models.Notification{
        RecipientID:      parent.UserID,
        ActorID:          actorID,
        TemplateID:       tpl.ID,
        Type:             "comment_reply",
        ActorDisplayName: displayName,
        Snippet:          snippet,
    }); err != nil {
        zap.S().Errorf("comment_reply hook: failed to create notification (template=%s parent=%s actor=%s): %v",
            tpl.ID, parentID, actorID, err)
    }
}
```

### 3.4 通知层

`notificationRepository.Create` 和 `List` 增加 `snippet` 列；`notificationListQuery` SELECT 增加 `n.snippet`，Scan 目标映射到 `models.Notification.Snippet`。`notificationToProto` 将 `Snippet` 映射到 `pb.Notification.Snippet`。

### 3.5 网关（`backend/cmd/server/main.go`）

- `POST /api/v1/templates/{id}/comments` body 结构体新增 `ParentCommentId string \`json:"parent_comment_id"\``，映射到 `CreateCommentRequest.ParentCommentId`。
- 在现有 `/api/v1/comments/` 处理器下新增分支：当路径为 `/api/v1/comments/{id}/replies` 且方法为 `GET` 时，调用 `ListCommentReplies`。按 `/` 拆分路径路由：
  - `[id]` → PUT/DELETE（现有）。
  - `[id, "replies"]` → GET `ListCommentReplies`。

```go
http.HandleFunc("/api/v1/comments/", func(w http.ResponseWriter, r *http.Request) {
    parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/comments/"), "/")
    if len(parts) == 2 && parts[1] == "replies" && r.Method == http.MethodGet {
        // 可选认证（可见性遵循模板）
        ctx := context.Background()
        if authHeader := r.Header.Get("Authorization"); authHeader != "" {
            tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
            if userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr); err == nil {
                ctx = service.ContextWithUserID(ctx, userID)
            }
        }
        resp, err := svc.ListCommentReplies(ctx, &pb.ListCommentRepliesRequest{CommentId: parts[0]})
        if err != nil { writeError(w, err); return }
        w.Header().Set("Content-Type", "application/json")
        b, _ := marshaler.Marshal(resp)
        _, _ = w.Write(b)
        return
    }
    // ... parts[0] 的现有 PUT/DELETE 处理 ...
})
```

- `adminRepository.ListComments` 返回父评论作者显示名；管理网关响应已携带 `AdminCommentEntry` —— 添加 `parent_author_display_name` 字段（见下方 §1.2 管理说明）。

### 3.6 管理 proto（`backend/api/proto/v1/admin.proto`）

`AdminCommentEntry` 新增一个字段（下一个空闲标签 12）：

```protobuf
message AdminCommentEntry {
  // ... 现有 1-11 ...
  // 父评论作者的显示名；顶层评论为空。
  string parent_author_display_name = 12;
}
```

## 4. 前端设计

### 4.1 文件布局

```
frontend/src/components/comments/
├── CommentsSection.js / .scss   // 话题感知：向下传递话题 props
├── CommentItem.js / .scss       // + 回复操作、桶切换、已解答徽章
├── CommentComposer.js / .scss   // 不变（主编辑器）
├── ReplyComposer.js / .scss     // 新增 —— 紧凑变体，parent_comment_id
├── RepliesBucket.js / .scss     // 新增 —— 折叠计数、懒加载 + 缓存
└── CommentEditForm.js / .scss   // 不变
frontend/src/pages/admin/CommentsPage.js / .scss   // 新增 —— parent 列
```

### 4.2 状态

`CommentsSection` 保持页面级本地状态（无新 Redux slice）：

```js
const [comments, setComments] = useState([]);       // 仅顶层
const [openComposerId, setOpenComposerId] = useState(null); // 一次仅一个编辑器
const [drafts, setDrafts] = useState({});           // 按评论 id 的草稿映射
const [expandedThreads, setExpandedThreads] = useState({}); // 评论 id -> 回复缓存
```

`RepliesBucket` 持有自己的 `replies`、`loading`、`error` 和 `expanded` 状态；获取的回复缓存在 `expandedThreads` 中，因此折叠 + 重新展开不会重新获取（AC-4）。

### 4.3 组件

- **CommentsSection** —— 渲染顶层 `CommentItem`；协调单个打开的编辑器（`openComposerId`）；向下传递 `onReply`、`onExpandThread` 和缓存的回复。
- **CommentItem** —— 添加 `Reply` 操作（仅认证；匿名看到 `comments.loginToReply`）；当 `comment.has_creator_reply` 时渲染 `Answered` 徽章；当 `comment.reply_count > 0` 时渲染 `RepliesBucket`。
- **ReplyComposer** —— `CommentComposer` 的紧凑变体；带标签的 textarea（`aria-label` "Reply to {name}"）、字符计数、取消/回复按钮；以 `parent_comment_id` 发布；成功后自动展开桶并追加回复；400 话题已满时显示内联本地化消息并保留草稿；429 时显示现有速率限制错误。
- **RepliesBucket** —— 折叠的「查看 {N} 条回复 ▾」按钮（单数「1 条回复」）；展开时通过 `listCommentReplies` 懒加载（或从缓存渲染）；最旧优先的 `CommentItem` 列表（回复不再嵌套）；仅在桶内显示内联错误 + 重试。
- **AdminCommentsPage** —— 镜像现有审核表；添加 `parent` 列（顶层为「—」，回复为「回复 {{author}}」）；删除流程不变（理由弹窗 + 审计）。

### 4.4 i18n 键（两个语言环境）

```json
{
  "comments": {
    "reply": "回复",
    "replyTo": "回复 {{name}}…",
    "loginToReply": "登录以回复",
    "viewReplies": "查看 {{count}} 条回复",
    "viewOneReply": "查看 1 条回复",
    "hideReplies": "收起回复",
    "answered": "已解答",
    "threadFull": "此话题已满 —— 请发表新评论。",
    "repliesError": "无法加载回复。",
    "replyPosted": "回复已发布"
  },
  "admin": {
    "comments": {
      "parent": "所属评论",
      "inReplyTo": "回复 {{author}}",
      "topLevel": "—"
    }
  },
  "notifications": {
    "event": {
      "comment_reply": "{{actor}} 回复了你在\"{{title}}\"下的评论"
    },
    "snippet": {
      "comment_reply": "\"{{snippet}}\""
    }
  }
}
```

ZH 语言环境镜像每个键。`notifPrefs.type.comment_reply` / `notifPrefs.type.comment_reply.desc` 属于 notification-preferences 功能（积压），在 §7 协调中说明。

### 4.5 API 客户端新增（`frontend/src/services/api.js`）

```js
export const listCommentReplies = (commentId) =>
  api.get(`/comments/${commentId}/replies`);
// createTemplateComment 增加可选 parent_comment_id：
export const createTemplateComment = (templateId, content, parentCommentId) =>
  api.post(`/templates/${templateId}/comments`, {
    content,
    ...(parentCommentId ? { parent_comment_id: parentCommentId } : {}),
  });
```

## 5. 错误矩阵

| HTTP | gRPC 代码 | 条件 | 用户可见消息 |
|---|---|---|---|
| 400 | `InvalidArgument` | 内容为空 / 修剪后 > 2000 | `comments.error_content_length`（现有） |
| 400 | `InvalidArgument` | `parent_comment_id` 未知 / 不同模板 / 指向回复 | `comments.error_parent_invalid`（通用） |
| 400 | `InvalidArgument` | 话题已满（第 51 条回复） | `comments.threadFull` |
| 400 | `InvalidArgument` | `ListCommentReplies` 作用于回复（非顶层） | 通用 400 |
| 401 | `Unauthenticated` | POST/PUT/DELETE 缺少/无效令牌 | 现有认证 toast |
| 403 | `PermissionDenied` | 被封禁用户发布 | `comments.error_banned`（现有） |
| 403 | `PermissionDenied` | 非作者编辑 / 非作者非所有者删除 | 现有通用 |
| 403 | `PermissionDenied` | 管理路由非管理员 | 现有管理 403 |
| 404 | `NotFound` | 未知模板/评论；私有模板非所有者 | `comments.error_load` / 通用 |
| 429 | `ResourceExhausted` | 回复速率限制（5/分钟或 50/天） | `comments.error_rate_limited` + `Retry-After` |
| 500 | `Internal` | DB/Redis 失败 | 通用错误 toast |

## 6. 部署

- 将 §2 DDL 块追加到两个 `init.sql` 副本（字节一致、幂等、增量）。
- 照常重建后端（`protoc` + `go build`）和前端。无新增服务、端口、卷或迁移工具。无新增环境变量。
- **与 notification-preferences 的协调**（积压）：该功能落地时，其 `notification_preferences` 表必须包含 `comment_reply_enabled BOOLEAN NOT NULL DEFAULT true` 和 `notifPrefs.type.comment_reply` UI 行。在此之前，`comment_reply` 事件始终发出。

## 7. SDK / CLI

`python-sdk` 增加透传辅助方法（评论是模板作用域的）：

```python
def list_comment_replies(self, comment_id): ...
def create_comment(self, template_id, content, parent_comment_id=None): ...
```

`create_comment` 在设置时把 `parent_comment_id` 透传到 REST body。v1 无 CLI 界面。

## 8. 测试策略

### 8.1 FVT（`tests/fvt.py`）—— 扩展 `test_template_comments()`

1. 作者发布顶层评论；所有者回复 → 200；回复出现在 `GET /comments/{id}/replies` 最旧优先；父评论 `reply_count` = 1；`templates.comment_count` 不变。
2. 回复一条回复 → 400（父评论必须是顶层）。
3. 不同模板上的 `parent_comment_id` → 400。
4. 话题上限：发布 50 条回复，第 51 条 → 400 及话题已满消息。
5. 所有者已回复时 `has_creator_reply` 为 true；否则为 false。
6. `comment_reply` 通知：父作者收到带回复者名 + ≤100 字符摘录；回复自己的评论不发；创作者在自己模板上回复不发。
7. 删除一条回复 → `reply_count` −1，兄弟回复完好，`comment_count` 不变。
8. 删除一条顶层评论 → 其回复消失（级联），`comment_count` −1。
9. 管理列表显示 parent 列；管理删除回复/顶层评论遵循级联 + 审计。
10. 速率限制：一分钟内第 6 条回复 → 429 + `Retry-After`。
11. 匿名在公开模板上读取话题；私有模板话题仅所有者（其他人 404）。

### 8.2 后端单元测试（新文件 ≥ 80 %）

- `parent_comment_id` 校验（未知、不同模板、指向回复、话题上限边界 50/51）。
- 级联删除事务（顶层移除回复；回复递减父评论）。
- 并发插入下的计数器维护（comment_count vs reply_count）。
- `has_creator_reply` 子查询正确性。
- 通知摘录截断（≤100 字符，rune 安全）。
- 权限矩阵（作者/所有者/其他/被封禁 × 回复/编辑/删除）。
- `MockCommentRepository` 更新新方法/字段。

### 8.3 E2E（Nightwatch，test-agent）

- `/templates/:id` 上的回复流程；桶展开/折叠 + 懒加载 + 缓存；已解答徽章；管理 parent 列；EN/ZH 字符串一致性；Reply、桶切换和编辑器的纯键盘遍历。

## 9. 实施顺序（建议）

1. DDL（两个 `init.sql` 副本）+ 模型（`Comment` 字段、`Notification.Snippet`）。
2. Proto 变更（`prompt.proto`、`notification.proto`、`admin.proto`）+ 重新生成。
3. `CommentRepository`（话题感知的 list/create/delete）+ 管理仓库 parent 列。
4. `PromptService`（`CreateComment` 父评论校验、`ListCommentReplies`、`notifyCommentReply`）+ 网关路由。
5. 通知层（仓库 + proto 映射中的 `snippet` 列）。
6. 前端：`ReplyComposer`、`RepliesBucket`、`CommentItem`/`CommentsSection` 扩展、管理 `CommentsPage` parent 列；两个语言环境的 i18n。
7. FVT + 单元测试；python-sdk 辅助方法。

## 10. 给开发者的约束（必须遵守）

1. **绝不提交对 `backend/server` 的更改**（跟踪的 26 MB 二进制）—— 若显示已修改，提交前 `git checkout -- backend/server`。
2. 分层：proto → 网关（`main.go`）→ 服务 → 仓库 → 模型；无新顶层包。
3. 用户路由在 `/api/v1/templates/{id}/comments`、`/api/v1/comments/{id}/replies` 和 `/api/v1/comments/{id}` 下；管理路由在 `/api/v1/admin/comments*` 下 —— 绝不混合界面。
4. 用户 ID 始终来自令牌（`ContextWithUserID`），绝不来自请求体；每次变更都在服务端做所有权检查。
5. `parent_comment_id` 必须引用**同一**模板上的**顶层**评论；回复一条回复会平铺到顶层话题（仅一层嵌套）。
6. `comment_count` 仅统计顶层评论；`reply_count` 统计直接回复。两者仅在创建/删除事务内维护 —— v1 无后台对账。
7. 删除顶层评论在同一事务中级联其回复；删除回复递减其父评论的 `reply_count`。
8. `comment_reply` 通知目标是**父评论作者**（而非模板所有者）；自回复不发通知；摘录 ≤100 字符（rune 安全截断）。
9. Redis 不可用时速率限制 fail-open；429 响应携带 `Retry-After`。回复与评论共享相同的 5/分钟 + 50/天计数器。
10. 管理删除需 3–500 字符理由并写入 `admin_audit_logs`（`action='delete_comment'`、`target_type='comment'`）。
11. `init.sql` 变更幂等并镜像到 helm chart 副本（字节一致块）。
12. i18n：每个新用户可见字符串在 `en` 和 `zh` 两个语言环境中都存在（`comments.*`、`admin.comments.*`、`notifications.*`）。
13. 常规提交，英文；新 Go 文件单元测试覆盖率 ≥ 80 %；推送前所有 CI 门禁通过。
14. 不要构建 notification-preferences 抑制路径 —— 那是单独的积压项；本功能中 `comment_reply` 事件始终发出。