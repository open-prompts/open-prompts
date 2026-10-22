# 内容举报 — 详细设计

| | |
|---|---|
| **功能 ID** | content-reporting |
| **版本** | 1.0 |
| **状态** | 已批准实施 |
| **作者** | Architect Agent |
| **上游** | [架构](content-reporting-architecture-zh-CN.md) |

## 1. Proto 契约

### 1.1 `backend/api/proto/v1/prompt.proto`

在 `PromptService` 服务块末尾追加（`GetAppeal` 之后，第 144 行）：

```protobuf
service PromptService {
    // ... 现有 RPC ...
    rpc CreateReport(CreateReportRequest) returns (Report);
    rpc GetMyReport(GetMyReportRequest) returns (MyReport);
    rpc WithdrawReport(WithdrawReportRequest) returns (WithdrawReportResponse);
}

message CreateReportRequest {
    string item_type = 1; // "template" | "comment"
    string item_id = 2;
    string reason = 3;    // spam | offensive | misleading | privacy | other
    string note = 4;      // 可选，<= 500 字
}

message Report {
    string id = 1;
    string item_type = 2;
    string item_id = 3;
    string reason = 4;
    string note = 5;
    string status = 6;    // open | resolved
    google.protobuf.Timestamp created_at = 7;
}

message GetMyReportRequest {
    string item_type = 1;
    string item_id = 2;
}

message MyReport {
    Report report = 1;    // 调用者对该条目无未解决举报时为 null
}

message WithdrawReportRequest {
    string item_type = 1;
    string item_id = 2;
}

message WithdrawReportResponse {
    bool success = 1;
}
```

### 1.2 `backend/api/proto/v1/admin.proto`

在 `AdminService` 服务块末尾追加（`ResolveAppeal` 之后，第 96 行）：

```protobuf
service AdminService {
    // ... 现有 RPC ...
    rpc ListReportQueue(ListReportQueueRequest) returns (ListReportQueueResponse);
    rpc GetReportDetail(GetReportDetailRequest) returns (AdminReportDetail);
    rpc ResolveReportGroup(ResolveReportGroupRequest) returns (ResolveReportGroupResponse);
}

message ListReportQueueRequest {
    string status = 1;    // "" | "open" | "resolved"
    string item_type = 2; // "" | "template" | "comment"
    string reason = 3;    // "" | spam | offensive | misleading | privacy | other
    int32 page = 4;       // 1 起始，默认 1
    int32 per_page = 5;   // 默认 50，最大 100
}

message ListReportQueueResponse {
    repeated AdminReportGroup groups = 1;
    int64 total = 2;
    int32 page = 3;
    int32 per_page = 4;
}

message AdminReportGroup {
    string item_type = 1;
    string item_id = 2;
    string item_title = 3;        // 模板标题或评论摘要
    string author_display_name = 4;
    int32 open_count = 5;
    repeated string reasons = 6;  // 去重原因码，计数在客户端
    google.protobuf.Timestamp latest_at = 7;
    string status = 8;            // open | resolved
}

message GetReportDetailRequest {
    string item_type = 1;
    string item_id = 2;
}

message AdminReportDetail {
    string item_type = 1;
    string item_id = 2;
    string item_title = 3;
    string item_content = 4;      // 模板预览或评论文本
    string author_id = 5;
    string author_display_name = 6;
    repeated AdminReportEntry reports = 7;
}

message AdminReportEntry {
    string id = 1;
    string reporter_id = 2;
    string reporter_display_name = 3;
    string reason = 4;
    string note = 5;
    google.protobuf.Timestamp created_at = 6;
}

message ResolveReportGroupRequest {
    string item_type = 1;
    string item_id = 2;
    string resolution = 3; // "dismissed" | "actioned"
    string reason = 4;     // 3-500 字，必需
}

message ResolveReportGroupResponse {
    bool success = 1;
    int32 resolved_count = 2;
}
```

向 `GetAdminStatsResponse` 添加 `open_reports`（字段 9）：

```protobuf
message GetAdminStatsResponse {
  // ... 现有字段 1-8 ...
  int64 open_reports = 9; // 仪表盘 KPI 的未解决举报数
}
```

重新生成：

```bash
cd backend && protoc --go_out=. --go-grpc_out=. api/proto/v1/prompt.proto api/proto/v1/admin.proto
```

## 2. 数据库 DDL

追加到 **两个** `deployment/init.sql` 和 `deployment/chart/open-prompts/init.sql`：

```sql
-- ===== content-reporting =====
-- 用户对公开模板和评论的举报。每用户每条目一次（UNIQUE）；重复举报 upsert（FR-2）。
-- 处置在条目所有未解决举报间原子完成（NFR-3）。
CREATE TABLE IF NOT EXISTS content_reports (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    item_type TEXT NOT NULL CHECK (item_type IN ('template', 'comment')),
    item_id UUID NOT NULL,
    reporter_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reason TEXT NOT NULL CHECK (reason IN ('spam', 'offensive', 'misleading', 'privacy', 'other')),
    note TEXT CHECK (note IS NULL OR char_length(note) <= 500),
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved')),
    resolution TEXT CHECK (resolution IS NULL OR resolution IN ('dismissed', 'actioned')),
    resolved_by TEXT REFERENCES users(id),
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (item_type, item_id, reporter_id)
);

COMMENT ON TABLE content_reports IS '用户对公开模板/评论的举报；每用户每条目一次';
COMMENT ON COLUMN content_reports.item_type IS 'template | comment';
COMMENT ON COLUMN content_reports.reason IS 'spam | offensive | misleading | privacy | other';
COMMENT ON COLUMN content_reports.status IS 'open | resolved';
COMMENT ON COLUMN content_reports.resolution IS 'dismissed | actioned（open 时为 NULL）';

CREATE INDEX IF NOT EXISTS idx_content_reports_status_item
    ON content_reports (status, item_type, item_id);
CREATE INDEX IF NOT EXISTS idx_content_reports_reporter
    ON content_reports (reporter_id);

-- 扩展 admin_audit_logs.action CHECK 以包含举报操作（受保护，幂等）。
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'admin_audit_logs'::regclass AND conname = 'admin_audit_logs_action_check';
    IF def IS NOT NULL AND def NOT LIKE '%report.dismiss%' THEN
        ALTER TABLE admin_audit_logs DROP CONSTRAINT admin_audit_logs_action_check;
        ALTER TABLE admin_audit_logs ADD CONSTRAINT admin_audit_logs_action_check
            CHECK (action IN ('ban', 'unban', 'role_change', 'delete_comment',
                              'review_approve', 'review_reject', 'settings_change',
                              'template.warning_add', 'template.warning_remove',
                              'appeal.create', 'appeal.review', 'appeal.resolve',
                              'report.dismiss', 'report.actioned'));
    END IF;
END $$;
```

`admin_audit_logs.target_type` 已包含 `template` 和 `comment` —— 无需扩展。

## 3. 后端分层

### 3.1 `models`（`backend/internal/models/report.go`）

```go
package models

import "time"

// ContentReport 映射到 "content_reports" 表。
type ContentReport struct {
    ID         string     `json:"id"`
    ItemType   string     `json:"item_type"`   // template | comment
    ItemID     string     `json:"item_id"`
    ReporterID string     `json:"reporter_id"`
    Reason     string     `json:"reason"`      // spam | offensive | misleading | privacy | other
    Note       string     `json:"note"`
    Status     string     `json:"status"`      // open | resolved
    Resolution string     `json:"resolution"`  // dismissed | actioned
    ResolvedBy string     `json:"resolved_by"`
    ResolvedAt *time.Time `json:"resolved_at"`
    CreatedAt  time.Time  `json:"created_at"`
}
```

### 3.2 `ReportRepository`（`backend/internal/repository/report_repository.go`）

```go
type ReportRepository interface {
    Upsert(ctx context.Context, r *models.ContentReport) error // FR-2：重复举报替换
    GetMine(ctx context.Context, reporterID, itemType, itemID string) (*models.ContentReport, error)
    DeleteMine(ctx context.Context, reporterID, itemType, itemID string) error
    ListQueue(ctx context.Context, status, itemType, reason string, limit, offset int) ([]*models.ReportGroup, int64, error)
    ListForItem(ctx context.Context, itemType, itemID string) ([]*models.ContentReport, error)
    ResolveItem(ctx context.Context, itemType, itemID, resolution, resolvedBy string) (int64, error) // 原子
    CountOpen(ctx context.Context) (int64, error) // 仪表盘 KPI
    AutoResolveItem(ctx context.Context, itemType, itemID string) error // FR-12 挂钩
}
```

`Upsert` SQL（重复举报替换原因/备注并提升时效；重置为 open）：

```sql
INSERT INTO content_reports (item_type, item_id, reporter_id, reason, note)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (item_type, item_id, reporter_id)
DO UPDATE SET reason = EXCLUDED.reason,
              note = EXCLUDED.note,
              status = 'open',
              resolution = NULL,
              resolved_by = NULL,
              resolved_at = NULL,
              created_at = now()
RETURNING id, created_at;
```

`ListQueue`（按条目分组，未解决优先按数量再按时效）：

```sql
SELECT cr.item_type, cr.item_id,
       COALESCE(t.title, c.content) AS item_title,
       COALESCE(tu.display_name, cu.display_name) AS author_display_name,
       COUNT(*) FILTER (WHERE cr.status = 'open') AS open_count,
       array_agg(DISTINCT cr.reason) AS reasons,
       MAX(cr.created_at) AS latest_at,
       MAX(cr.status) AS status
FROM content_reports cr
LEFT JOIN templates t ON cr.item_type = 'template' AND t.id = cr.item_id
LEFT JOIN users tu ON t.owner_id = tu.id
LEFT JOIN template_comments c ON cr.item_type = 'comment' AND c.id = cr.item_id
LEFT JOIN users cu ON c.user_id = cu.id
WHERE ($1 = '' OR cr.status = $1)
  AND ($2 = '' OR cr.item_type = $2)
  AND ($3 = '' OR cr.reason = $3)
GROUP BY cr.item_type, cr.item_id, t.title, c.content, tu.display_name, cu.display_name
ORDER BY open_count DESC, latest_at DESC
LIMIT $4 OFFSET $5;
```

`ResolveItem` —— 在条目所有未解决举报间原子完成：

```sql
UPDATE content_reports
SET status = 'resolved',
    resolution = $3,
    resolved_by = $4,
    resolved_at = now()
WHERE item_type = $1 AND item_id = $2 AND status = 'open';
```

`AutoResolveItem`（FR-12 挂钩，在取消发布/删除时调用）：

```sql
UPDATE content_reports
SET status = 'resolved',
    resolution = 'actioned',
    resolved_by = $3,
    resolved_at = now()
WHERE item_type = $1 AND item_id = $2 AND status = 'open';
```

### 3.3 `PromptService`（`backend/internal/service/prompt.go`）

`CreateReport`：

1. `GetUserIDFromContext` → 失败时 `Unauthenticated`。
2. 加载用户；若 `user.Status == "banned"` → `PermissionDenied`（FR-3）。
3. 校验 `item_type` ∈ {template, comment} 且 `reason` ∈ 5 码分类 → 否则 `InvalidArgument`。
4. 校验 `note` ≤ 500 字 → 否则 `InvalidArgument`。
5. 加载条目（模板或评论）；若未找到或非公开 → `NotFound`。若条目所有者/作者是调用者 → `InvalidArgument`（FR-4 兜底）。
6. 速率限制（见下）→ 超限时 `ResourceExhausted`。
7. `ReportRepo.Upsert` → `Report` proto。

`GetMyReport`：认证，然后 `ReportRepo.GetMine` → `MyReport`（无则 null）。

`WithdrawReport`：认证，然后 `ReportRepo.DeleteMine` → `WithdrawReportResponse`。

**速率限制**（Redis 计数器，出错放行）：

```
key:    report_rate:{user_id}
value:  "{count}:{window_expires_unix}"
window: 滑动，限制 REPORT_RATE_PER_HOUR（默认 20）
```

每次 `CreateReport` 递增；达到限制时返回 `429 RESOURCE_EXHAUSTED` 并带 `Retry-After`。

### 3.4 `AdminService`（`backend/internal/service/admin.go`）

`ListReportQueue`：`requireAdminContext`，然后 `ReportRepo.ListQueue` → `ListReportQueueResponse`。

`GetReportDetail`：`requireAdminContext`，然后 `ReportRepo.ListForItem` + 条目上下文（模板标题/内容或评论文本）→ `AdminReportDetail`。

`ResolveReportGroup`：

1. `requireAdminContext`。
2. 校验 `resolution` ∈ {dismissed, actioned} 且 `reason` 长度 ∈ [3, 500] → 否则 `InvalidArgument`。
3. `ReportRepo.ResolveItem` → 若处置 0 行 → `AlreadyExists`（无未解决举报）。
4. 写入审计 `report.dismiss` 或 `report.actioned`（target_type `template`/`comment`，target_id `item_id`，reason 携带管理员原因）。
5. 返回 `ResolveReportGroupResponse`。

**自动处置挂钩（FR-12）：** 当模板通过任何管理路径被取消发布或评论被删除时，在同一事务中调用 `ReportRepo.AutoResolveItem`。对模板，挂钩到取消发布/审核操作路径；对评论，挂钩到 `DeleteAdminComment` 和面向用户的评论删除。

**仪表盘 KPI（FR-13）：** `GetAdminStats` 额外调用 `ReportRepo.CountOpen` 并在响应中设置 `open_reports`。

### 3.5 网关（`backend/cmd/server/main.go`）

**用户路由** —— 在 `/api/v1/appeals/` 处理器之后添加：

```go
// /api/v1/reports — POST 创建/更新（认证）。
http.HandleFunc("/api/v1/reports", func(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodPost { // auth -> CreateReportRequest{ItemType, ItemId, Reason, Note}
    }
})

// /api/v1/reports/mine — GET（认证）、DELETE（认证）。
http.HandleFunc("/api/v1/reports/mine", func(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:   // auth -> GetMyReportRequest{ItemType, ItemId}
    case http.MethodDelete: // auth -> WithdrawReportRequest{ItemType, ItemId}
    }
})
```

**管理路由** —— 在 `/api/v1/admin/appeals/` 处理器之后添加：

```go
// /api/v1/admin/reports — GET 队列（管理员）。
http.HandleFunc("/api/v1/admin/reports", func(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodGet { // admin -> ListReportQueueRequest{Status, ItemType, Reason, Page, PerPage}
    }
})

// /api/v1/admin/reports/{item_type}/{item_id} — GET 详情、POST 处置（管理员）。
http.HandleFunc("/api/v1/admin/reports/", func(w http.ResponseWriter, r *http.Request) {
    rest := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/reports/")
    parts := strings.Split(rest, "/")
    if len(parts) != 2 { http.Error(w, "not found", http.StatusNotFound); return }
    itemType, itemID := parts[0], parts[1]
    switch r.Method {
    case http.MethodGet:  // admin -> GetReportDetailRequest{ItemType, ItemId}
    case http.MethodPost: // admin -> ResolveReportGroupRequest{ItemType, ItemId, Resolution, Reason}
    }
})
```

所有路由都需要 Bearer 令牌认证流程（视情况使用用户或管理 JWT）。429 响应设置 `Retry-After`。

## 4. 前端

### 4.1 新文件

| 文件 | 用途 |
|---|---|
| `frontend/src/components/ReportMenu.js` + `.scss` | ⋯ 菜单项：举报 / 您已举报此内容（更新/撤回）。 |
| `frontend/src/components/ReportModal.js` + `.scss` | 五个单选原因 + 可选备注；焦点陷阱对话框。 |
| `frontend/src/components/ReportedMenuItem.js` + `.scss` | 对勾状态，带更新/撤回子菜单。 |
| `frontend/src/pages/admin/ReportsPage.js` + `.scss` | `/admin/reports` —— 带过滤器的分组队列。 |
| `frontend/src/components/admin/ReviewDrawer.js` + `.scss` | 右侧抽屉：条目上下文 + 举报 + 处置操作。 |
| `frontend/src/components/admin/ReasonPopover.js` + `.scss` | 驳回/处理前要求原因的确认弹窗。 |

### 4.2 修改文件

| 文件 | 变更 |
|---|---|
| `frontend/src/App.js` | 添加 `/admin/reports` 管理路由。 |
| `frontend/src/services/api.js` | `createReport`、`getMyReport`、`withdrawReport`。 |
| `frontend/src/services/adminApi.js` | `listAdminReports`、`getAdminReportDetail`、`resolveReportGroup`。 |
| `frontend/src/components/admin/AdminLayout.js` | 添加 Reports 导航项。 |
| `frontend/src/pages/admin/DashboardPage.js` | 添加 OpenReportsKPI 卡片（FR-13）。 |
| TemplateDetails / 评论 ⋯ 菜单 | 挂载 `ReportMenu`。 |

### 4.3 Redux

无新 slice：举报状态随 API 响应到达；模态框和队列页面管理本地状态。`ReportMenu` 在挂载时获取 `getMyReport` 以决定对勾状态。

### 4.4 API 客户端

```js
createReport({ item_type, item_id, reason, note }) // POST /reports
getMyReport(item_type, item_id)                    // GET  /reports/mine
withdrawReport(item_type, item_id)                 // DELETE /reports/mine
listAdminReports(params)                           // GET  /admin/reports
getAdminReportDetail(item_type, item_id)           // GET  /admin/reports/:item_type/:item_id
resolveReportGroup(item_type, item_id, data)       // POST /admin/reports/:item_type/:item_id/resolve
```

### 4.5 i18n 键（EN/ZH 对齐）

```json
{
  "reports": {
    "action": "Report",
    "modal": {
      "title": { "template": "Report this template", "comment": "Report this comment" },
      "question": "Why are you reporting this?",
      "noteLabel": "Additional details (optional)",
      "submit": "Submit report",
      "cancel": "Cancel"
    },
    "reason": {
      "spam": "Spam or scam",
      "offensive": "Offensive or hateful",
      "misleading": "Misleading or harmful",
      "privacy": "Privacy violation",
      "other": "Something else"
    },
    "toast": { "submitted": "Report submitted — thank you. Our moderators will review it.", "updated": "Your report was updated." },
    "reported": "You reported this",
    "update": "Update report…",
    "withdraw": "Withdraw report",
    "withdrawConfirm": "Withdraw your report?",
    "rateLimited": "You've submitted several reports recently — please try again later."
  },
  "admin": {
    "nav": { "reports": "Reports" },
    "reports": {
      "title": "Reports",
      "open": "Open",
      "resolved": "Resolved",
      "openCount": "Open reports",
      "review": "Review",
      "view": "View",
      "dismiss": "Dismiss reports",
      "reasonRequired": "A reason is required",
      "empty": "No open reports — all clear",
      "type": "Type",
      "item": "Item",
      "author": "Author",
      "reasons": "Reasons",
      "latest": "Latest",
      "status": "Status",
      "actions": "Actions"
    },
    "dashboard": { "open_reports": "Open reports" }
  }
}
```

ZH 按 UI/UX 文档 §8 镜像每个键（`reports.action` = "举报"、`reports.reason.spam` = "垃圾信息或诈骗"、`admin.reports.title` = "举报" 等）。

## 5. 错误矩阵

| 场景 | HTTP | gRPC | 前端处理 |
|---|---|---|---|
| 无效 item_type/原因；备注 > 500；自己的内容 | 400 | `INVALID_ARGUMENT` | 字段级错误；表单不变 |
| 未认证 | 401 | `UNAUTHENTICATED` | 重定向到 `/login` |
| 被封禁用户 | 403 | `PERMISSION_DENIED` | Toast；操作隐藏 |
| 非管理员访问管理路由 | 403 | `PERMISSION_DENIED` | 管理控制台错误 Toast |
| 条目未找到 / 非公开 | 404 | `NOT_FOUND` | 404 页面 |
| 无可处置的未解决举报 | 409 | `ALREADY_EXISTS` | Toast；队列刷新 |
| 举报速率限制 | 429 | `RESOURCE_EXHAUSTED` | 带速率限制消息的 Toast |
| Redis 宕机 | — | — | 服务端放行；无用户影响 |

## 6. 部署

- `REPORT_RATE_PER_HOUR=20` 添加到 `deployment/docker-compose.yml` 后端环境以及 Helm chart（`values.yaml` `backend.env.reportRatePerHour` + `templates/deployment-backend.yaml` 环境条目）。
- DDL 增量/幂等；两个 init.sql 副本在同一提交中。
- 无新服务、端口或基础设施。

## 7. SDK

无 python-sdk 变更：内容举报是 Web 界面功能（用户举报模态框 + 管理控制台）。REST 端点在 API 界面中记录；SDK 仅封装模板解析。

## 8. 测试策略

### 8.1 FVT（`tests/fvt.py`）—— `test_content_reporting`

设置：alice（举报者）、bob（管理员）、carol（创作者）。由 carol 播种一个公开模板及其上的公开评论。

1. alice 举报模板 → `open`；`GET /reports/mine` 返回它。
2. alice 用不同原因重复举报 → upsert（无重复）；Toast "已更新"。
3. alice 举报评论 → `open`。
4. 匿名 `POST /reports` → 401。
5. 被封禁用户 `POST /reports` → 403。
6. alice 举报自己的模板 → 400。
7. alice 一小时内提交 21 次举报 → 第 21 次 429。
8. bob `GET /admin/reports` → 分组队列，未解决优先按数量再按时效，过滤器生效。
9. bob `GET /admin/reports/template/{id}` → 含全部举报的详情。
10. bob `POST /resolve {resolution: dismissed, reason}` → 所有未解决举报已处置；审计含 `report.dismiss`；内容未动。
11. 第二个条目：bob `POST /resolve {resolution: actioned, reason}` → 已处置；审计含 `report.actioned`。
12. 取消发布模板（通过审核路径）自动将其未解决举报处置为 `actioned`（FR-12）。
13. 删除评论自动将其未解决举报处置为 `actioned`（FR-12）。
14. `GET /admin/stats` → `open_reports` KPI 计数正确。
15. 含 `<script>` 的备注按字面文本渲染（无 HTML 解释）。

### 8.2 单元测试

- 仓库：`Upsert` upsert + 唯一冲突映射、`ListQueue` 分组 + 排序 + 过滤、`ResolveItem` 原子（0 行 → 无操作）、`AutoResolveItem`、`CountOpen`。
- 服务：校验矩阵、授权（封禁、自己的内容、非管理员）、速率限制计数器（假 Redis）、审计发出、自动处置挂钩。
- 用新方法更新 `MockReportRepository`。

### 8.3 E2E（测试代理，Nightwatch）

- 举报模态框：从 ⋯ 菜单打开、单选组、提交、成功 Toast、已举报状态。
- 已举报状态菜单：更新（预填模态框）和撤回（确认）。
- `/admin/reports` 队列：分组、过滤、未解决优先排序、审查/查看。
- 审查抽屉：条目上下文、举报列表、带原因弹窗的驳回/处理。
- 仪表盘 KPI 卡片链接到队列。
- 所有 `reports.*` 和 `admin.reports.*` 键的 i18n EN/ZH 对齐。

## 9. 实施顺序

1. DDL（两个 init.sql 副本）+ 模型 + `ReportRepository` + 单元测试。
2. Proto 变更 + 重新生成。
3. `PromptService` 用户 RPC + 速率限制 + Redis 计数器接线。
4. `AdminService` 管理 RPC + 审计 + 自动处置挂钩 + KPI 计数。
5. 网关用户 + 管理路由。
6. 前端：api 客户端 → 组件 → 页面 → 路由 → i18n。
7. FVT + E2E。

## 10. 对开发者的约束

1. **切勿提交 `backend/server`** —— 受跟踪构建产物；若被修改则 `git checkout -- backend/server`。
2. 只暂存你自己的文件；切勿 `git add -A`。
3. 两个 init.sql 副本在同一提交中保持同步。
4. `content_reports` UNIQUE 约束是 FR-1 兜底；将 `23505` 冲突映射为 upsert（FR-2），而非错误。
5. `ResolveItem` 必须在条目所有未解决举报间原子完成；处置 0 行时返回 409。
6. 自动处置（FR-12）必须在同一事务中挂钩取消发布和评论删除路径 —— 无清扫任务。
7. 举报者仅管理员可见（NFR-2）；切勿向内容作者或其他用户暴露举报者身份。
8. 备注和原因均为纯文本 —— 切勿渲染为 HTML/Markdown（构造上 XSS 安全，NFR-5）。
9. 在任何举报阈值处均无自动下架（NFR-4）—— 每个后果都需要明确的管理员操作。
10. 任何形式的变现界面均不允许。