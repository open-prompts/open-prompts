# 管理员模板审核 — 详细设计

| | |
|---|---|
| **功能 ID** | admin-template-moderation |
| **版本** | 1.0 |
| **状态** | 已批准实施 |
| **作者** | Architect Agent |
| **上游** | [架构](admin-template-moderation-architecture-zh-CN.md) |

## 1. Proto 契约

### 1.1 `backend/api/proto/v1/admin.proto`

在 `AdminService` 服务块末尾追加（`ResolveReportGroup` 之后，第 108 行）：

```protobuf
service AdminService {
    // ... 现有 RPC ...
    rpc UnpublishTemplate(ModerationActionRequest) returns (ModerationActionResponse);
    rpc LockTemplate(ModerationActionRequest) returns (ModerationActionResponse);
    rpc SoftDeleteTemplate(ModerationActionRequest) returns (ModerationActionResponse);
    rpc RestoreTemplate(ModerationActionRequest) returns (ModerationActionResponse);
}

message ModerationActionRequest {
    string template_id = 1;
    string reason = 2; // 3-500 字符，必填
}

message ModerationActionResponse {
    string template_id = 1;
    string moderation_status = 2; // 新状态
    google.protobuf.Timestamp moderated_at = 3;
}
```

向 `AdminTemplateSummary` 添加字段（`rating_count = 17` 之后）：

```protobuf
message AdminTemplateSummary {
    // ... 现有字段 1-17 ...
    // 审核状态："normal" | "locked" | "unpublished_by_admin" |
    // "soft_deleted" | "in_review" | "rejected"。
    string moderation_status = 18;
    // 最近一次审核操作的时间戳（首次操作前为 NULL）。
    google.protobuf.Timestamp moderated_at = 19;
    // 最近一次审核操作的管理员 id（首次操作前为 NULL）。
    string moderated_by = 20;
    // 最近一次审核操作的原因（仅管理员，对所有者可见）。
    string moderation_reason = 21;
}
```

向 `ListAdminTemplatesRequest` 添加筛选（`warning = 7` 之后）：

```protobuf
message ListAdminTemplatesRequest {
    // ... 现有字段 1-7 ...
    string moderation_status = 8; // "" | normal | locked | unpublished_by_admin | soft_deleted
}
```

重新生成：

```bash
cd backend && protoc --go_out=. --go-grpc_out=. api/proto/v1/admin.proto
```

## 2. 数据库 DDL

追加到 **两个** `deployment/init.sql` 和 `deployment/chart/open-prompts/init.sql`：

```sql
-- ===== admin-template-moderation =====
-- 扩展 moderation_status CHECK（守卫式、幂等）。该列及其原始 CHECK 由
-- pre-publication-review 引入。
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'templates'::regclass AND conname = 'templates_moderation_status_check';
    IF def IS NOT NULL AND def NOT LIKE '%unpublished_by_admin%' THEN
        ALTER TABLE templates DROP CONSTRAINT templates_moderation_status_check;
        ALTER TABLE templates ADD CONSTRAINT templates_moderation_status_check
            CHECK (moderation_status IN ('normal', 'in_review', 'rejected',
                                         'locked', 'unpublished_by_admin', 'soft_deleted'));
    END IF;
END $$;

-- 仅管理员的审核元数据（首次操作前为 NULL）。
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='templates' AND column_name='moderated_at') THEN
        ALTER TABLE templates ADD COLUMN moderated_at TIMESTAMPTZ;
        ALTER TABLE templates ADD COLUMN moderated_by TEXT;
        ALTER TABLE templates ADD COLUMN moderation_reason TEXT;
        COMMENT ON COLUMN templates.moderated_at IS '最近一次审核操作的时间戳';
        COMMENT ON COLUMN templates.moderated_by IS '最近一次审核操作的管理员 id';
        COMMENT ON COLUMN templates.moderation_reason IS '最近一次审核操作的原因（仅对所有者和管理员可见）';
    END IF;
END $$;

-- 软删除恢复窗口起点（软删除前为 NULL）。30 天硬删除 ticker 基于此时间戳扫描。
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='templates' AND column_name='deleted_at') THEN
        ALTER TABLE templates ADD COLUMN deleted_at TIMESTAMPTZ;
        COMMENT ON COLUMN templates.deleted_at IS '软删除时间；启动 30 天恢复窗口（否则为 NULL）';
    END IF;
END $$;

-- 扩展 admin_audit_logs.action CHECK 以包含审核操作（守卫式）。
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'admin_audit_logs'::regclass AND conname = 'admin_audit_logs_action_check';
    IF def IS NOT NULL AND def NOT LIKE '%template.unpublish%' THEN
        ALTER TABLE admin_audit_logs DROP CONSTRAINT admin_audit_logs_action_check;
        ALTER TABLE admin_audit_logs ADD CONSTRAINT admin_audit_logs_action_check
            CHECK (action IN ('ban', 'unban', 'role_change', 'delete_comment',
                              'review_approve', 'review_reject', 'settings_change',
                              'template.warning_add', 'template.warning_remove',
                              'appeal.create', 'appeal.review', 'appeal.resolve',
                              'report.dismiss', 'report.actioned',
                              'template.unpublish', 'template.lock',
                              'template.soft_delete', 'template.restore',
                              'template.republished_after_moderation'));
    END IF;
END $$;

-- 扩展 notifications.type CHECK 以包含 moderation 事件类型（守卫式）。
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'notifications'::regclass AND conname = 'notifications_type_check';
    IF def IS NOT NULL AND def NOT LIKE '%moderation%' THEN
        ALTER TABLE notifications DROP CONSTRAINT notifications_type_check;
        ALTER TABLE notifications ADD CONSTRAINT notifications_type_check
            CHECK (type IN ('like', 'favorite', 'fork', 'comment',
                            'template_approved', 'template_rejected',
                            'template_warning_changed', 'follow',
                            'appeal_resolved', 'comment_reply',
                            'moderation'));
    END IF;
END $$;
```

`moderation_actions` 表**已由 moderation-appeals 定义**在 `deployment/init.sql`
中——本功能仅**填充**它（无 DDL 变更）。

## 3. 后端分层

### 3.1 `models`（`backend/internal/models/template.go`）

向 `Template` 模型添加：

```go
type Template struct {
    // ... 现有字段 ...
    ModerationStatus string       `json:"moderation_status"` // normal | in_review | rejected | locked | unpublished_by_admin | soft_deleted
    PublishedAt      sql.NullTime `json:"published_at"`
    ModeratedAt      sql.NullTime `json:"moderated_at"`
    ModeratedBy      string       `json:"moderated_by"`
    ModerationReason string       `json:"moderation_reason"`
    DeletedAt        sql.NullTime `json:"deleted_at"` // 软删除窗口起点
}
```

### 3.2 `AdminRepository`（`backend/internal/repository/admin_repository.go`）

向 `AdminRepository` 接口添加：

```go
type AdminRepository interface {
    // ... 现有方法 ...
    // SetTemplateModeration 转换模板的审核状态。
    // publish=true 设置 moderation_status='normal' + published_at=now()；
    // publish=false 设置 moderation_status='rejected'。两者都由
    // WHERE moderation_status='in_review' 守卫，保证并发决策安全。
    SetTemplateModeration(ctx context.Context, templateID string, status string, publish bool) error
    // SetModerationState 在模板上设置任意 moderation_status + 审核元数据
    // （moderated_at/by/reason）。id 未知时返回 ErrTemplateNotFound。
    SetModerationState(ctx context.Context, templateID, status, actorID, reason string) error
    // InsertModerationAction 记录一次审核操作以供申诉生命周期使用。
    // 返回新操作 id。
    InsertModerationAction(ctx context.Context, a *models.ModerationAction) (string, error)
    // MarkModerationActionSuperseded 将模板的最新 active 操作翻转为
    // 'superseded'（用于恢复）。
    MarkModerationActionSuperseded(ctx context.Context, templateID string) error
    // ListAuditByTarget 返回目标对象的审计条目（复用于审核历史）。
    ListAuditByTarget(ctx context.Context, targetID string, limit int) ([]*models.AdminAuditLog, error)
}
```

`SetModerationState` SQL：

```sql
UPDATE templates
SET moderation_status = $1,
    moderated_at = now(),
    moderated_by = $2,
    moderation_reason = $3,
    updated_at = now()
WHERE id = $4
RETURNING id;
```

`InsertModerationAction` SQL：

```sql
INSERT INTO moderation_actions (template_id, action_type, actor_id, actor_email, reason)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;
```

`MarkModerationActionSuperseded` SQL：

```sql
UPDATE moderation_actions
SET status = 'superseded', superseded_at = now()
WHERE template_id = $1 AND status = 'active';
```

### 3.3 `TemplateRepository`（`backend/internal/repository/template_repository.go`）

扩展 `buildTemplateConditions` 以对非所有者隐藏 `unpublished_by_admin` 和
`soft_deleted` 模板（OQ-1）。现有审核过滤已隐藏 `in_review`/`rejected`；扩展之：

```go
// 审核过滤镜像封禁所有者模式：被门控的
// （in_review/rejected/unpublished_by_admin/soft_deleted）模板从每个公开
// 流中隐藏，除非是其所有者。$1 是 current_user_id（匿名时为 ""），因此
// 绕过对匿名调用者永不匹配。
query += " AND (t.moderation_status = 'normal' OR t.owner_id = $1)"
```

这一单一子句现在覆盖全部四个非公开审核状态。`locked` 状态**有意不**隐藏——
锁定模板保持公开可见（FR-3）。

### 3.4 `AdminService`（`backend/internal/service/admin.go`）

添加共享辅助函数和四个 RPC：

```go
// moderationRateLimit 强制每管理员写入层（FR-15）。
func (s *AdminService) moderationRateLimit(ctx context.Context, adminID string) error {
    // Redis 计数器 mod_action_rate:{adminID}，滑动窗口，
    // 限制 MOD_ACTION_RATE_PER_MINUTE（默认 10），失败开放。
}

// applyModerationAction 执行共享状态变更：自守卫、转换守卫、状态更新、
// moderation_actions 插入、审计、举报自动解析、通知。
func (s *AdminService) applyModerationAction(ctx context.Context, actor *models.User,
    req *pb.ModerationActionRequest, actionType, newStatus string) (*pb.ModerationActionResponse, error) {
    // 1) 加载模板；缺失 -> NotFound。
    // 2) 自守卫：若 template.OwnerID == actor.ID -> PermissionDenied。
    // 3) 转换守卫：拒绝无效转换（见下）。
    // 4) moderationRateLimit -> 超限 ResourceExhausted。
    // 5) AdminRepo.SetModerationState(templateID, newStatus, actor.ID, reason)。
    // 6) AdminRepo.InsertModerationAction(...) -> moderation_actions 行。
    // 7) writeAudit(actor, "template."+actionType, "template", templateID, reason)。
    // 8) ReportRepo.AutoResolveItem("template", templateID, actor.ID)（FR-10）。
    // 9) notifyModeration(actor, ownerID, templateID, actionType, reason)。
    // 10) 返回 ModerationActionResponse。
}
```

`UnpublishTemplate`：`requireAdminContext`，校验原因 ∈ [3,500]，然后
`applyModerationAction(..., "unpublish", "unpublished_by_admin")`。

`LockTemplate`：`requireAdminContext`，校验原因，然后
`applyModerationAction(..., "lock", "locked")`。

`SoftDeleteTemplate`：`requireAdminContext`，校验原因，然后
`applyModerationAction(..., "soft_delete", "soft_deleted")`——额外设置
`deleted_at = now()`（恢复窗口起点）。

`RestoreTemplate`：`requireAdminContext`，校验原因，然后：

1. 加载模板；缺失 → `NotFound`。
2. 若 `template.ModerationStatus == "normal"` → `AlreadyExists`（无可恢复）。
3. `AdminRepo.SetModerationState(templateID, "normal", actor.ID, reason)`——
   保留可见性（所有者的 `visibility` 列不受影响）。
4. `AdminRepo.MarkModerationActionSuperseded(templateID)`——先前操作变为不可申诉。
   **恢复不插入新的 `moderation_actions` 行**——`action_type` CHECK 仅允许
   `('unpublish', 'lock', 'soft_delete')`，恢复记录在审计日志（`template.restore`）中。
5. `writeAudit(actor, "template.restore", "template", templateID, reason)`。
6. `notifyModeration(actor, ownerID, templateID, "restore", reason)`。

**转换守卫**（无效转换 → `409 ALREADY_EXISTS`）：

| 从 \ 到 | unpublish | lock | soft_delete | restore |
|---|---|---|---|---|
| normal | ✅ | ✅ | ✅ | ❌ |
| locked | ✅ | ❌ | ✅ | ✅ |
| unpublished_by_admin | ✅ | ✅ | ✅ | ✅ |
| soft_deleted | ❌ | ❌ | ❌ | ✅（窗口内） |
| in_review / rejected | ❌ | ❌ | ❌ | ❌ |

**重新发布标记（OQ-5）：** 当所有者通过正常 `UpdateTemplate` 流程重新发布
`unpublished_by_admin` 模板时，服务写入审计事件 `template.republished_after_moderation`
（目标 `template`，target_id 模板 id，原因 `"owner re-published after admin
unpublish"`）。这是 `UpdateTemplate`（prompt.go）中的钩子，当 `visibility`
转换为 `public` 且 `moderation_status` 为 `unpublished_by_admin` 时触发。

`notifyModeration`（镜像 `notifyReviewDecision`）：

```go
func (s *AdminService) notifyModeration(ctx context.Context, actor *models.User,
    ownerID, templateID, actionType, reason string) {
    if s.NotificationRepo == nil { return }
    excerpt := reason
    if len([]rune(excerpt)) > 140 { excerpt = string([]rune(excerpt)[:140]) + "…" }
    if err := s.NotificationRepo.Create(ctx, &models.Notification{
        RecipientID:      ownerID,
        ActorID:          actor.ID,
        TemplateID:       templateID,
        Type:             "moderation",
        ActorDisplayName: actor.DisplayName,
        Snippet:          excerpt,
    }); err != nil {
        zap.S().Errorf("moderation notification: failed: %v", err)
    }
}
```

`moderation` 通知负载携带操作类型和原因摘录；深链指向模板详情（所有者视图）。
对于取消发布，前端渲染"修复并重新发布"CTA。

### 3.5 `TemplateCascadeService`（`backend/internal/service/template_cascade.go`）

**新增**——共享硬删除级联（OQ-4），为 account-deletion 复用而提取：

```go
type TemplateCascadeService struct {
    db *sql.DB
}

// HardDeleteTemplate 永久移除模板及其拥有的子项（版本、别名、其上的评论、
// 点赞/收藏/评分、关注、预设）。他人的派生以"已删除来源"谱系存活
// （其 fork_source_id 被置空 / 标记为已删除）。
func (s *TemplateCascadeService) HardDeleteTemplate(ctx context.Context, templateID string) error {
    // BEGIN;
    // DELETE FROM template_versions WHERE template_id = $1;
    // DELETE FROM template_aliases WHERE template_id = $1;
    // DELETE FROM template_comments WHERE template_id = $1;
    // DELETE FROM template_likes WHERE template_id = $1;
    // DELETE FROM template_favorites WHERE template_id = $1;
    // DELETE FROM template_ratings WHERE template_id = $1;
    // DELETE FROM template_watches WHERE template_id = $1;
    // DELETE FROM template_presets WHERE template_id = $1;
    // UPDATE templates SET fork_source_id = NULL WHERE fork_source_id = $1;
    // DELETE FROM templates WHERE id = $1;
    // COMMIT;
}
```

### 3.6 Ticker（`backend/internal/service/ticker.go`）

**新增**——30 天硬删除到期扫描（scheduler-sharing/webhooks 模式）：

```go
// scanExpiredSoftDeletes 每 24 小时运行一次（并在启动时，可追赶）。
// 硬删除超过恢复窗口的 soft_deleted 模板。
func (s *TickerService) scanExpiredSoftDeletes(ctx context.Context) error {
    rows, err := s.db.QueryContext(ctx, `
        SELECT id FROM templates
        WHERE moderation_status = 'soft_deleted'
          AND deleted_at < now() - ($1::int * interval '1 day')
        LIMIT 1000`, modSoftDeleteWindowDays)
    // 对每个 id：TemplateCascadeService.HardDeleteTemplate(id)
    // writeAudit(actor=system, "template.soft_delete", "template", id,
    //            "expired soft-delete window; hard-deleted")
}
```

幂等：`WHERE moderation_status = 'soft_deleted'` 守卫意味着已恢复的模板永不被
拾取；批处理中途崩溃可安全重跑。

### 3.7 网关（`backend/cmd/server/main.go`）

在现有 `/api/v1/admin/templates/` 处理器中添加（警告路由之后、fallthrough
`Method not allowed` 之前）：

```go
// 审核操作：
//   POST /api/v1/admin/templates/{id}/unpublish
//   POST /api/v1/admin/templates/{id}/lock
//   POST /api/v1/admin/templates/{id}/delete
//   POST /api/v1/admin/templates/{id}/restore
if len(parts) == 2 && r.Method == http.MethodPost {
    var body struct {
        Reason string `json:"reason"`
    }
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        http.Error(w, "Invalid request body", http.StatusBadRequest)
        return
    }
    var resp *pb.ModerationActionResponse
    var err error
    switch parts[1] {
    case "unpublish":
        resp, err = adminSvc.UnpublishTemplate(ctx, &pb.ModerationActionRequest{TemplateId: templateID, Reason: body.Reason})
    case "lock":
        resp, err = adminSvc.LockTemplate(ctx, &pb.ModerationActionRequest{TemplateId: templateID, Reason: body.Reason})
    case "delete":
        resp, err = adminSvc.SoftDeleteTemplate(ctx, &pb.ModerationActionRequest{TemplateId: templateID, Reason: body.Reason})
    case "restore":
        resp, err = adminSvc.RestoreTemplate(ctx, &pb.ModerationActionRequest{TemplateId: templateID, Reason: body.Reason})
    default:
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }
    if err != nil {
        writeError(w, err)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    b, _ := marshaler.Marshal(resp)
    _, _ = w.Write(b)
    return
}
```

同时在 `ListTemplates` 服务方法中添加 `moderation_status`：解析查询参数，
对照允许集合校验，并传入 `TemplateFilters.ModerationStatus`。

## 4. 前端

### 4.1 新文件

| 文件 | 用途 |
|---|---|
| `frontend/src/components/admin/ModerationPanel.js` + `.scss` | 管理模板详情上的状态徽章 + 操作按钮 + 历史列表。 |
| `frontend/src/components/admin/ActionReasonModal.js` + `.scss` | 后果清单 + 原因文本域（3–500）+ 可选输入标题确认（删除）。 |
| `frontend/src/components/admin/StatusChip.js` + `.scss` | 每个 moderation_status 的彩色徽章。 |
| `frontend/src/components/admin/RestoreCountdown.js` + `.scss` | 软删除行上的"剩余 N 天"。 |
| `frontend/src/components/moderation/OwnerModerationBanner.js` + `.scss` | 所有者视图横幅（已取消发布/锁定/已移除变体）。 |
| `frontend/src/components/moderation/LockBanner.js` + `.scss` | 公开通用锁定通知。 |
| `frontend/src/components/moderation/RemovedNotice.js` + `.scss` | 所有者软删除通知，含原因 + 申诉提示。 |

### 4.2 修改文件

| 文件 | 变更 |
|---|---|
| `frontend/src/pages/admin/TemplatesPage.js` | 添加 `moderation_status` 筛选（AdminSelect）+ 状态徽章列 + 软删除行上的 RestoreCountdown。 |
| `frontend/src/pages/admin/TemplateDetailPage.js` | 挂载 `ModerationPanel`（通过按目标过滤的 `getAdminAuditLogs` 获取审核历史）。 |
| `frontend/src/components/admin/ReviewDrawer.js` | "处理 → 取消发布"调用 `unpublishTemplate(id, { reason })` 然后 `resolveReportGroup(..., { resolution: 'actioned', reason })`。 |
| `frontend/src/components/notifications/NotificationItem.js` | 渲染 `moderation` 类型（图标 + 操作 + 标题 + 原因摘录 + 深链）。 |
| `frontend/src/pages/TemplateDetails.js` | 基于 `moderation_status` 挂载所有者横幅 / 锁定横幅 / 移除通知。 |
| `frontend/src/services/adminApi.js` | `unpublishTemplate`、`lockTemplate`、`softDeleteTemplate`、`restoreTemplate`。 |
| `frontend/src/App.js` | 无新路由（审核位于现有管理模板页面上）。 |

### 4.3 Redux

无新 slice：审核状态随 API 响应到达；面板和弹窗管理本地状态。

### 4.4 API 客户端

```js
unpublishTemplate(id, { reason })  // POST /admin/templates/:id/unpublish
lockTemplate(id, { reason })       // POST /admin/templates/:id/lock
softDeleteTemplate(id, { reason }) // POST /admin/templates/:id/delete
restoreTemplate(id, { reason })    // POST /admin/templates/:id/restore
```

### 4.5 i18n 键（EN/ZH 对齐）

```json
{
  "mod": {
    "panel": { "title": "Moderation", "status": "Current status" },
    "status": {
      "normal": "Normal",
      "locked": "Locked",
      "unpublished": "Unpublished by admin",
      "soft_deleted": "Removed"
    },
    "action": {
      "unpublish": "Unpublish",
      "lock": "Lock",
      "delete": "Delete",
      "restore": "Restore"
    },
    "modal": {
      "unpublish": { "title": "Unpublish this template?" },
      "lock": { "title": "Lock this template?" },
      "delete": { "title": "Delete this template?" },
      "restore": { "title": "Restore this template?" },
      "consequence": {
        "public": "Removed from public surfaces (home, search, embeds, API)",
        "ownerKeeps": "The owner keeps access and can edit",
        "frozen": "Editing and new interactions are frozen",
        "notified": "The owner is notified with your reason",
        "reversible": "Reversible: restore any time",
        "window": "Hidden everywhere; 30-day restore window"
      },
      "reason": "Reason (required, visible to the owner)",
      "typeTitle": "Type the template title to confirm",
      "daysLeft": "{{count}} days left"
    },
    "history": { "title": "Moderation history", "empty": "No moderation actions yet" },
    "selfGuard": "You cannot moderate your own template",
    "banner": {
      "unpublished": "An administrator unpublished this template: {{reason}}. You can edit it and share it again.",
      "locked": "This template is locked by moderators. Editing and new interactions are disabled while under review.",
      "removed": "This template was removed by moderators: {{reason}}. If you believe this is a mistake, reply via your notification or contact support.",
      "editCta": "Edit template"
    },
    "lock": {
      "public": "This template is locked by moderators",
      "blocked": "This template is locked by moderators"
    },
    "notif": {
      "unpublish": "Your template \"{{title}}\" was unpublished: {{reason}}",
      "lock": "Your template \"{{title}}\" was locked: {{reason}}",
      "delete": "Your template \"{{title}}\" was removed: {{reason}}",
      "restore": "Your template \"{{title}}\" was restored"
    },
    "filter": { "status": "Status", "empty": "No templates match" },
    "toast": { "actioned": "Template unpublished; report resolved" }
  }
}
```

ZH 按 UI/UX 文档 §8 镜像每个键（`mod.panel.title` = "审核"、
`mod.status.normal` = "正常" 等）。

## 5. 错误矩阵

| 场景 | HTTP | gRPC | 前端处理 |
|---|---|---|---|
| 原因长度违规 | 400 | `INVALID_ARGUMENT` | 字段级错误；弹窗不变 |
| 未认证 | 401 | `UNAUTHENTICATED` | 重定向到 `/login` |
| 非管理员管理路由 | 403 | `PERMISSION_DENIED` | 管理控制台错误 toast |
| 管理员审核自己的模板 | 403 | `PERMISSION_DENIED` | 操作禁用并带自守卫提示 |
| 模板不存在 | 404 | `NOT_FOUND` | 404 页面 |
| 无效状态转换 | 409 | `ALREADY_EXISTS` | Toast；面板刷新 |
| 审核速率限制 | 429 | `RESOURCE_EXHAUSTED` | 带限制消息的 Toast |
| Redis 宕机 | — | — | 服务端失败开放；无用户影响 |

## 6. 部署

- `MOD_ACTION_RATE_PER_MINUTE=10` 和 `MOD_SOFT_DELETE_WINDOW_DAYS=30` 添加到
  `deployment/docker-compose.yml` 后端环境及 Helm chart（`values.yaml`
  `backend.env.modActionRatePerMinute` + `backend.env.modSoftDeleteWindowDays` +
  `templates/deployment-backend.yaml` 环境条目）。
- DDL 增量/幂等；两个 init.sql 副本在同一提交中。
- 无新服务、端口或基础设施。

## 7. SDK

无 python-sdk 变更：审核是管理控制台 + 网页界面功能。REST 端点在 API 界面中
记录；SDK 仅包装模板解析。SDK 的 `get_template`/`get_prompt_by_alias` 通过共享
仓储守卫自动遵循新可见性规则（无 SDK 代码变更）。

## 8. 测试策略

### 8.1 FVT（`tests/fvt.py`）— `test_admin_template_moderation`

设置：alice（创作者）、bob（管理员）、carol（管理员）。alice 拥有一个公开模板。

1. bob 带原因取消发布 alice 的模板 → `moderation_status = 'unpublished_by_admin'`；
   匿名 + carol（非所有者）`GET /templates/{id}` → 404；alice（所有者）看到带横幅；
   审计有 `template.unpublish`；alice 收到 `moderation` 通知。
2. alice 编辑并重新分享 → `moderation_status` 返回 `normal`；模板再次公开；
   审计有 `template.republished_after_moderation`。
3. bob 锁定模板 → 公开查看者看到（200）带锁定横幅；alice 的编辑被阻止
   （403/409）；评论/派生/点赞尝试被阻止；现有评论保持可见。
4. bob 软删除（输入标题确认）→ 非所有者 404；alice 看到移除通知；管理列表在
   `soft_deleted` 下显示并带恢复倒计时。
5. bob 在第 12 天恢复 → 模板返回先前可见性；审计有 `template.restore`；
   计划硬删除被取消（状态现为 `normal`）。
6. 超过 30 天的软删除模板被 ticker 以级联语义硬删除；另一用户的派生以
   "已删除来源"谱系存活。
7. bob 筛选 `/admin/templates?moderation_status=locked` → 仅列出锁定模板，
   每个带 moderated_at/by/reason。
8. 从 content-reporting 队列组，处理 → 取消发布 → 举报组自动解析为 `actioned`；
   审核审计条目携带相同原因。
9. bob 尝试取消发布自己的模板 → 403；审计日志不变。
10. 原因 < 3 或 > 500 字符 → 400。
11. 无效转换（恢复 `normal` 模板）→ 409。
12. bob 超过审核速率限制 → 429 带 `Retry-After`。

### 8.2 单元

- 仓储：`SetModerationState`（状态 + 元数据）、`InsertModerationAction`、
  `MarkModerationActionSuperseded`、`buildTemplateConditions` 可见性矩阵
  （所有者绕过、非所有者 404、匿名、锁定保持公开）。
- 服务：校验矩阵、自守卫、转换守卫、速率限制计数器（假 Redis）、审计 + 通知
  发射、举报自动解析钩子。
- 级联服务：硬删除级联 + 派生谱系保留。
- Ticker：幂等性、可追赶性、窗口边界。
- 用新方法更新 `MockAdminRepository`。

### 8.3 E2E（测试代理，Nightwatch）

- 审核面板渲染 + 操作弹窗（原因校验、删除输入标题确认）。
- `/admin/templates` 上的状态筛选；状态徽章；恢复倒计时。
- 所有者横幅（已取消发布/锁定/已移除）+ 锁定阻止的交互。
- `moderation` 通知渲染，含原因摘录 + 深链。
- 举报队列"处理 → 取消发布"流程。
- 所有 `mod.*` 键的 i18n EN/ZH 对齐。

## 9. 实施顺序

1. DDL（两个 init.sql 副本）+ models + `AdminRepository` 方法 + 单元测试。
2. Proto 变更 + 重新生成。
3. `AdminService` RPC + 自守卫 + 转换守卫 + 速率限制 + 审计 + 通知 + 举报自动解析。
4. `TemplateRepository` 可见性守卫 + `UpdateTemplate` 重新发布标记。
5. `TemplateCascadeService` + ticker。
6. 网关路由 + `ListTemplates` 筛选。
7. 前端：api 客户端 → 组件 → 页面 → i18n。
8. FVT + E2E。

## 10. 开发者约束

1. **绝不提交 `backend/server`**——受跟踪的构建产物；若修改则 `git checkout
   -- backend/server`。
2. 仅暂存你自己的文件；绝不 `git add -A`。
3. 两个 init.sql 副本在同一提交中保持同步。
4. `moderation_actions` **已由 moderation-appeals 定义**——不要重新创建它；
   仅插入行。`appeals` 表读取它。
5. `moderation_status` CHECK 扩展必须由 `pg_get_constraintdef` 守卫（幂等），
   且必须保留现有 `in_review`/`rejected` 值（pre-publication-review）。
6. `buildTemplateConditions` 中的可见性守卫必须保持 `locked` 模板公开——仅
   `unpublished_by_admin` 和 `soft_deleted`（加上现有 `in_review`/`rejected`）
   对非所有者隐藏。
7. 锁定模板在服务层阻止所有变更（编辑、评论、派生、点赞/收藏/评分、关注、
   预设保存）——逐变更强制，而非仅在 UI 中。
8. 重新发布标记（`template.republished_after_moderation`）是 `UpdateTemplate`
   （prompt.go）中的钩子，不是新端点。
9. 原因和通知摘录是纯文本——绝不渲染为 HTML/Markdown（构造上 XSS 安全，
   NFR-5）。
10. 无任何形式的变现界面。