# 内容警告 — 详细设计

| | |
|---|---|
| **Feature ID** | content-warnings |
| **Version** | 1.0 |
| **Status** | Approved for implementation |
| **Author** | Architect Agent |
| **Upstream** | [架构设计](content-warnings-architecture-zh-CN.md) |

## 1. Proto 契约

### 1.1 `backend/api/proto/v1/prompt.proto`

`Template` 字段号是跨设计协同的区块：17 = `comment_count`（已上线），
18/19 由 follow-creators 预留，20–22 由 template-ratings 预留，23/24 由
prompt-analytics 预留，**25/26 已被 pre-publication-review 占用
（`moderation_status`/`published_at`）**。**本特性使用 27/28 —— 不得复用
18–26。**

```protobuf
message Template {
    // ... 既有字段 1-17 ...
    // 审核状态："normal" | "in_review" | "rejected"（pre-publication-review）。
    string moderation_status = 25;
    // 首次审核批准的时间（pre-publication-review）。
    google.protobuf.Timestamp published_at = 26;
    // 生效的内容警告（自我申报 + 审核员施加），键来自配置驱动分类法
    // （mature | violence | profanity | sensitive）。
    repeated string content_warnings = 27;
    // content_warnings 中由审核员施加的子集；对创作者不可变。
    // 仅对所有者与管理员有意义。
    repeated string locked_warnings = 28;
}
```

`CreateTemplateRequest` 新增 `content_warnings`（10），
`UpdateTemplateRequest` 新增 `content_warnings`（11）：

```protobuf
message CreateTemplateRequest {
    // ... 既有字段 1-9 ...
    repeated string content_warnings = 10; // 分类法键；服务端校验
}

message UpdateTemplateRequest {
    // ... 既有字段 1-10 ...
    repeated string content_warnings = 11; // 完整期望集合；锁定警告被强制
}
```

`ListTemplatesRequest` 新增 `exclude_warned`（12）：

```protobuf
message ListTemplatesRequest {
    // ... 既有字段 1-11 ...
    bool exclude_warned = 12; // 为真时，过滤掉带任何内容警告的模板
}
```

`templateModelToProto` 映射两个新字段
（`ContentWarnings: m.ContentWarnings`，`LockedWarnings: m.LockedWarnings`）。

### 1.2 `backend/api/proto/v1/admin.proto`

`AdminTemplateSummary` 新增 `content_warnings`（15）与 `locked_warnings`
（16）—— 字段 13/14 被 ratings/import-export 争议，因此从 15 开始：

```protobuf
message AdminTemplateSummary {
    // ... 既有字段 1-12 ...
    repeated string content_warnings = 15;
    repeated string locked_warnings = 16;
}
```

`ListAdminTemplatesRequest` 新增 `warning`（7）：

```protobuf
message ListAdminTemplatesRequest {
    // ... 既有字段 1-6 ...
    string warning = 7; // "" = 任意 | "none" | 分类法键（mature|violence|profanity|sensitive）
}
```

`AdminService` 追加 2 个新 RPC（既有 20 个不动）：

```protobuf
service AdminService {
    // ... 既有 RPC ...
    rpc UpdateTemplateWarnings(UpdateTemplateWarningsRequest) returns (UpdateTemplateWarningsResponse);
    rpc ListWarningHistory(ListWarningHistoryRequest) returns (ListWarningHistoryResponse);
}

message UpdateTemplateWarningsRequest {
    string template_id = 1;
    repeated string add = 2;    // 要添加的警告（分类法键）
    repeated string remove = 3; // 要移除的警告（分类法键）
    string reason = 4;          // 必填，3-500 字符，审计
}

message UpdateTemplateWarningsResponse {
    string template_id = 1;
    repeated string content_warnings = 2; // 变更后的完整集合
    repeated string locked_warnings = 3;  // 变更后的锁定集合
}

message ListWarningHistoryRequest {
    string template_id = 1;
    int32 page = 2;    // 1 起，默认 1
    int32 per_page = 3; // 默认 20，最大 100
}

message WarningHistoryEntry {
    string id = 1;
    string actor_id = 2;
    string actor_email = 3;
    string action = 4; // "template.warning_add" | "template.warning_remove"
    repeated string warnings = 5; // 本次变更中添加/移除的警告
    string reason = 6;
    google.protobuf.Timestamp created_at = 7;
}

message ListWarningHistoryResponse {
    repeated WarningHistoryEntry entries = 1;
    int64 total = 2;
    int32 page = 3;
    int32 per_page = 4;
}
```

重新生成：

```bash
cd backend && protoc --go_out=. --go-grpc_out=. api/proto/v1/prompt.proto api/proto/v1/admin.proto
```

## 2. 数据库 DDL

追加到**两份** `deployment/init.sql` 与
`deployment/chart/open-prompts/init.sql`（逐字节一致）：

```sql
-- -----------------------------------------------------------------------------
-- Feature: content-warnings
-- Description: config-driven warning taxonomy, per-template warning arrays,
--              moderator lock, audit + notification widenings.
-- -----------------------------------------------------------------------------

-- 模板上的警告数组（OQ-1：两个并行 TEXT[] 列；锁定为交集）。
-- 默认为空 —— 既有行无需回填。
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='templates' AND column_name='content_warnings') THEN
        ALTER TABLE templates ADD COLUMN content_warnings TEXT[] NOT NULL DEFAULT '{}';
        COMMENT ON COLUMN templates.content_warnings IS
            'Active content warnings (self-declared + moderator-applied), keys from the config-driven taxonomy';
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='templates' AND column_name='locked_warnings') THEN
        ALTER TABLE templates ADD COLUMN locked_warnings TEXT[] NOT NULL DEFAULT '{}';
        COMMENT ON COLUMN templates.locked_warnings IS
            'Subset of content_warnings applied by moderation; creator-immutable (FR-8)';
    END IF;
END $$;

-- GIN 索引服务公开列表的 exclude_warned 与管理端警告状态筛选（NFR-6）。
CREATE INDEX IF NOT EXISTS idx_templates_content_warnings
    ON templates USING GIN(content_warnings);

-- 分类法种子（NFR-2：配置驱动，无需发版即可由管理员编辑）。
INSERT INTO platform_settings (key, value) VALUES
    ('content_warning_taxonomy', '["mature","violence","profanity","sensitive"]')
ON CONFLICT (key) DO NOTHING;

-- 扩宽 admin_audit_logs.action CHECK 以支持警告变更（受保护、幂等）。
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'admin_audit_logs'::regclass AND conname = 'admin_audit_logs_action_check';
    IF def IS NOT NULL AND def NOT LIKE '%template.warning_add%' THEN
        ALTER TABLE admin_audit_logs DROP CONSTRAINT admin_audit_logs_action_check;
        ALTER TABLE admin_audit_logs ADD CONSTRAINT admin_audit_logs_action_check
            CHECK (action IN ('ban', 'unban', 'role_change', 'delete_comment',
                              'review_approve', 'review_reject', 'settings_change',
                              'template.warning_add', 'template.warning_remove'));
    END IF;
END $$;

-- 扩宽 notifications.type CHECK 以支持警告变更通知（受保护）。
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'notifications'::regclass AND conname = 'notifications_type_check';
    IF def IS NOT NULL AND def NOT LIKE '%template_warning_changed%' THEN
        ALTER TABLE notifications DROP CONSTRAINT notifications_type_check;
        ALTER TABLE notifications ADD CONSTRAINT notifications_type_check
            CHECK (type IN ('like', 'favorite', 'fork', 'comment',
                            'template_approved', 'template_rejected',
                            'template_warning_changed'));
    END IF;
END $$;
```

## 3. 后端分层

### 3.1 模型

```go
// models/template.go — 新增：
ContentWarnings pq.StringArray `json:"content_warnings"` // 生效警告（分类法键）
LockedWarnings  pq.StringArray `json:"locked_warnings"`  // 审核员施加子集（创作者不可变）

// models/warning.go — 新文件：
type WarningHistoryEntry struct {
    ID        string    `json:"id"`
    ActorID   string    `json:"actor_id"`
    ActorEmail string   `json:"actor_email"`
    Action    string    `json:"action"` // "template.warning_add" | "template.warning_remove"
    Warnings  pq.StringArray `json:"warnings"`
    Reason    string    `json:"reason"`
    CreatedAt time.Time `json:"created_at"`
}
```

### 3.2 `TemplateRepository` 变更

`Create` INSERT 新增 `content_warnings`、`locked_warnings` 列；`Update` SET
子句新增两者；`Get`/`List`/`CountTemplates` SELECT 新增两者（扫描进新模型
字段）。

**`buildTemplateConditions` 中的 `exclude_warned` 筛选**（仅公开流；私有流
不动 —— 私有模板的警告仅对其所有者可见）：

```sql
-- 当 exclude_warned 为真时，追加到公开流 WHERE：
AND NOT (t.content_warnings && ARRAY['mature','violence','profanity','sensitive'])
```

分类法数组作为参数传入（来自缓存设置），绝不内联。`CountTemplates` 共享
构建器，因此计数保持一致。

**管理端警告状态筛选**（管理端列表查询 `ListAdminTemplates` 中）：

```sql
-- warning = ""（任意）：无额外子句
-- warning = "none"：  AND (t.content_warnings = '{}' OR t.content_warnings IS NULL)
-- warning = "mature"（或任一键）：AND (t.content_warnings @> ARRAY[$1])
```

### 3.3 `PromptService` 变更

`PromptService` 新增一个通过设置缓存读取分类法的辅助函数：

```go
// taxonomy 返回配置驱动的警告键（来自 platform_settings，缓存 60s）。
// 缓存未命中时回退到内置默认。
func (s *PromptService) taxonomy(ctx context.Context) []string
```

**`CreateTemplate`** —— 校验后、`TemplateRepo.Create` 前：

```go
// 对照分类法校验每个请求的警告。
for _, w := range req.ContentWarnings {
    if !slices.Contains(taxonomy, w) {
        return nil, status.Errorf(codes.InvalidArgument, "unknown content warning %q", w)
    }
}
template.ContentWarnings = req.ContentWarnings
template.LockedWarnings = nil // 新模板没有审核员施加的警告
```

**`UpdateTemplate`** —— 执行创作者不可变锁定（FR-8）：

```go
// 加载既有模板（更新时已获取）。
locked := existing.LockedWarnings
incoming := req.ContentWarnings
// 锁定警告必须保留在传入集合中。
for _, lw := range locked {
    if !slices.Contains(incoming, lw) {
        return nil, status.Errorf(codes.PermissionDenied,
            "warning %q was applied by moderation and cannot be removed", lw)
    }
}
// 对照分类法校验传入集合。
for _, w := range incoming {
    if !slices.Contains(taxonomy, w) {
        return nil, status.Errorf(codes.InvalidArgument, "unknown content warning %q", w)
    }
}
template.ContentWarnings = incoming
template.LockedWarnings = locked // 保留；创作者不能更改
```

**`ListTemplates`** —— 将请求中的 `exclude_warned` 传入仓库筛选。

### 3.4 `AdminService` 警告 RPC

`AdminService` 新增 `TemplateRepo` 引用（或复用既有管理模板仓库）与分类法
辅助函数。两个 RPC 均以 `requireAdminContext` 开头。

**`UpdateTemplateWarnings`**（尽可能单事务；通知与审计按既有模式尽力而为）：

1. 校验 `reason`（3–500 字符）→ 缺失/过短时 `InvalidArgument`。
2. 对照分类法校验 `add`/`remove` 中的每个键 → 未知时 `InvalidArgument`。
3. 加载模板；计算新集合：
   - `newContent = (current ∪ add) − remove`
   - `newLocked = (currentLocked ∪ add) − remove`（审核员可移除自己或他人
     施加的警告；移除自我申报警告只是将其从 `content_warnings` 中删除）。
4. `UPDATE templates SET content_warnings = $1, locked_warnings = $2 WHERE id = $3`
   （`WHERE id = $1` 保护；零行 → `NotFound`）。
5. 对每个添加的警告 `writeAudit(ctx, actor, "template.warning_add",
   "template", templateID, reason)`，对每个移除的警告
   `"template.warning_remove"`（或每次变更一条审计行，原因中带警告列表 ——
   历史列表读取这些行）。
6. 通知（尽力而为，自我抑制 —— 见 3.5）。

**`ListWarningHistory`** —— 只读；查询 `admin_audit_logs`，按
`target_type = 'template'`、`target_id = templateID`、
`action IN ('template.warning_add','template.warning_remove')` 过滤，按
`created_at DESC` 排序，分页。映射行到 `WarningHistoryEntry`。

### 3.5 警告变更通知

复用 `AdminService` 中的 `createEngagementNotification` 形状（actor = 操作
管理员，`ActorDisplayName` = 管理员显示名）：

- 类型 `template_warning_changed`。
- 通知的 `ActorDisplayName` 携带增量句子负载（增/删的警告名），使前端无需
  单独查找即可渲染 `warnings.notify.changed`。具体地，`ActorDisplayName`
  字段设为紧凑 JSON `{"added":[...],"removed":[...]}`，`NotificationItem`
  解析为本地化句子（OQ-5）。

自我操作边界情况（管理员更改自己模板的警告）：跳过并记录警告 —— 与既有
自我抑制及 `no_self_notify` CHECK 兜底一致。失败被记录并吞掉；警告变更本身
绝不受影响。

### 3.6 网关（`backend/cmd/server/main.go`）

两条路由，均位于 `requireAdmin` 之后，沿用既有 `/api/v1/admin/templates/`
TrimPrefix 模式：

```go
// POST   /api/v1/admin/templates/{id}/warnings   （JSON：add, remove, reason）
// DELETE /api/v1/admin/templates/{id}/warnings   （JSON：add, remove, reason）
// GET    /api/v1/admin/templates/{id}/warnings/history
http.HandleFunc("/api/v1/admin/templates/", func(w, r) {
    // 既有 GET 详情处理器（len(parts)==1）……
    // 扩展 switch：
    rest := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/templates/")
    parts := strings.Split(rest, "/")
    switch {
    case len(parts) == 1 && r.Method == http.MethodGet: // 既有 GetTemplate
    case len(parts) == 2 && parts[1] == "warnings" && r.Method == http.MethodPost:
        // UpdateTemplateWarnings（添加）
    case len(parts) == 2 && parts[1] == "warnings" && r.Method == http.MethodDelete:
        // UpdateTemplateWarnings（移除）
    case len(parts) == 3 && parts[1] == "warnings" && parts[2] == "history" && r.Method == http.MethodGet:
        // ListWarningHistory
    default: 404/405
    }
})
```

公开列表处理器（`/api/v1/templates`）将 `exclude_warned` 查询参数
（`== "true"`）映射进 `ListTemplatesRequest.ExcludeWarned`。管理端列表处理器
（`/api/v1/admin/templates`）将 `warning` 查询参数映射进
`ListAdminTemplatesRequest.Warning`。

## 4. 前端

### 4.1 路由

无新路由。管理端警告界面位于既有的 `/admin/templates` 与
`/admin/templates/:id` 页面内。

### 4.2 新文件

| 文件 | 用途 |
|---|---|
| `frontend/src/components/warnings/WarningCheckboxGroup.js` + `.scss` | 从分类法多选；锁定警告渲染为禁用、勾选的角标，带锁图标 + 提示。 |
| `frontend/src/components/warnings/WarningChip.js` + `.scss` | 带 ⚠ 图标 + 本地化原因名的小角标；多个时提示列出全部原因。 |
| `frontend/src/components/warnings/BlurredPreview.js` + `.scss` | 卡片预览区模糊（CSS `filter: blur(12px)`）；角标覆盖底部；标题在模糊之外。 |
| `frontend/src/components/warnings/ContentInterstitial.js` + `.scss` | 内容上方全宽面板：图标、原因列表、显示内容 / 返回；焦点陷阱，Escape = 返回。 |
| `frontend/src/components/warnings/HideWarnedFilterChip.js` + `.scss` | 筛选行中的切换角标；按用户持久；URL 同步（`?hideWarned=1`）。 |
| `frontend/src/components/warnings/ShowWarnedToggle.js` + `.scss` | 带描述的设置开关；服务端持久。 |
| `frontend/src/components/warnings/WarningActionsPanel.js` + `.scss` | 管理端面板：当前警告带来源徽章、添加下拉、移除按钮、原因输入、历史列表。 |

### 4.3 修改文件

| 文件 | 变更 |
|---|---|
| `frontend/src/services/api.js` | `createTemplate`/`updateTemplate` 接受 `content_warnings`；`getTemplates` 接受 `exclude_warned`。 |
| `frontend/src/services/adminApi.js` | `updateTemplateWarnings(id, {add, remove, reason})`、`getWarningHistory(id, params)`。 |
| `frontend/src/components/CreateTemplateModal.js` | 表单新增 `WarningCheckboxGroup`；创建负载包含 `content_warnings`。 |
| `frontend/src/pages/TemplateDetails.js` | 当模板有警告且 `show_warned_content` 关闭时，在内容前渲染 `ContentInterstitial`；编辑表单新增带锁定显示的 `WarningCheckboxGroup`。 |
| `frontend/src/components/PromptCard.js` | 当模板有警告且观众未揭示/未选择时，渲染 `BlurredPreview` + `WarningChip`。 |
| `frontend/src/pages/Home.js` | 筛选行新增 `HideWarnedFilterChip`；将 `exclude_warned` 传入列表查询；按用户持久 + URL 同步。 |
| `frontend/src/pages/Profile.js` | 新增"内容偏好"区块，含 `ShowWarnedToggle`。 |
| `frontend/src/pages/admin/TemplatesPage.js` | 新增 `AdminWarningFilter` 下拉（任意 / 无 / 成人 / 暴力 / 语言 / 敏感）。 |
| `frontend/src/pages/admin/TemplateDetailPage.js` | 新增 `WarningActionsPanel`（当前警告、带原因增删、历史）。 |
| `frontend/src/components/notifications/NotificationItem.js` | `EVENT_ICONS` += `template_warning_changed`；解析增量负载为 `warnings.notify.changed`。 |
| `frontend/src/locales/{en,zh}/translation.json` | `warnings.*` 键逐字取自 UI/UX §8。 |

**FR-12（嵌入）：** `template-embedding-widget` 界面是积压特性（尚未实现）。
本特性使 `ContentInterstitial` 成为可复用、自包含的组件（它接收警告原因与
揭示回调作为 props，并拥有自己的 `sessionStorage` 揭示键）。当嵌入组件上线
时，它在嵌入内容前渲染 `ContentInterstitial`，作用域限定在自己的揭示键，
因此嵌入内揭示不会揭示别处的模板页（AC-10）。本特性不包含任何嵌入代码变更。

### 4.4 Redux

无新 slice：观众偏好（`show_warned_content`）随主页获取并存入既有
auth/user 状态；`exclude_warned` 为页面局部 + 通过 localStorage（登录）或
URL（匿名）持久，匹配既有筛选持久模式。揭示状态为 `sessionStorage`
（页面局部，无 Redux）。

### 4.5 i18n 键（EN/ZH 对齐）

`warnings.*` 逐字取自 UI/UX §8：

```json
{
  "warnings": {
    "group": { "title": "Content warnings (optional)", "help": "Help viewers know what to expect before opening." },
    "type": { "mature": "Mature themes", "violence": "Graphic violence", "profanity": "Strong language", "sensitive": "Sensitive topics" },
    "locked": { "appliedBy": "Applied by moderation on {{date}}: {{reason}}", "label": "Applied by moderation (locked)" },
    "chip": { "multiple": "{{first}} +{{count}} more" },
    "interstitial": { "title": "Content warning", "body": "This template covers: {{reasons}}", "subtext": "You can view it — we just want you to know first.", "show": "Show content", "back": "Go back" },
    "filter": { "hideWarned": "Hide warned content" },
    "settings": { "title": "Content preferences", "showAlways": "Always show warned content", "showAlwaysHelp": "Skip the blur and warning screen for flagged templates." },
    "admin": {
      "filter": { "label": "Warnings", "any": "Any", "none": "None" },
      "panel": { "title": "Content warnings", "current": "Current", "origin": { "creator": "creator", "moderation": "moderation" }, "add": "Add warning", "remove": "Remove", "reason": "Reason (required)", "history": "History", "historyEmpty": "No warning history" }
    },
    "notify": { "changed": "Content warnings on \"{{title}}\" were changed: {{change}}", "added": "added {{warning}}", "removed": "removed {{warning}}" }
  }
}
```

ZH 逐键镜像 UI/UX §8 的指定文案（内容警告（可选）/ 帮助观众在打开前了解
内容。/ 成人主题 / 血腥暴力 / 强烈语言 / 敏感话题 / 由审核于 {{date}}
施加：{{reason}} / 由审核施加（已锁定）/ {{first}} 等 {{count}} 项 / 内容
警告 / 本模板涉及：{{reasons}} / 你可以查看 —— 我们只是想让你先知情。/ 显示
内容 / 返回 / 隐藏警告内容 / 内容偏好 / 始终显示警告内容 / 跳过已标记模板的
模糊与警告页。/ 警告 / 任意 / 无 / 内容警告 / 当前 / 创作者 / 审核 / 添加
警告 / 移除 / 原因（必填）/ 历史 / 暂无警告历史 / 你的模板"{{title}}"的内容
警告已变更：{{change}} / 已添加 {{warning}} / 已移除 {{warning}}）。

## 5. 错误矩阵

| 场景 | HTTP | gRPC | 前端处理 |
|---|---|---|---|
| 创建/更新时未知警告类型 | 400 | `INVALID_ARGUMENT` | 内联错误；复选框组回退 |
| 创作者移除锁定警告 | 403 | `PERMISSION_DENIED` | 内联错误；锁定角标保持勾选 |
| 管理端增删无原因或原因 <3 字符 | 400 | `INVALID_ARGUMENT` | Apply 按钮预禁用；竞态时内联错误 |
| 非管理员访问警告管理路由 | 401/403 | `UNAUTHENTICATED`/`PERMISSION_DENIED` | 管理员登录重定向 |
| 模板 id 未知（管理端警告变更/历史） | 404 | `NOT_FOUND` | 未找到页；返回列表 |
| 通知插入失败 | — | — | 服务端记录；警告变更不受影响 |

## 6. 部署

- DDL 增量/幂等；两份 init.sql 在同一提交中逐字节一致；CHECK 扩宽由
  `pg_get_constraintdef` 保护；列新增由列存在性检查保护；分类法种子使用
  `ON CONFLICT DO NOTHING`。
- 无新服务、端口或基础设施。无环境变量 —— 分类法存于 `platform_settings`。
- Helm chart：无模板变更（init.sql 经既有 ConfigMap 到达 chart）；
  `values.yaml` 不动。
- 绝不提交 `backend/server`（受跟踪构建产物）。

## 7. SDK

无 `python-sdk/openprompts` 变更：API 在 `get_template`/`list_templates` 上
以普通数组返回 `content_warnings`，客户端自行决定是否门控（不在范围内 ——
SDK 侧门控被明确排除）。管理端警告界面是 Web 控制台功能；SDK 仅封装公开
模板 API。

## 8. 测试策略

### 8.1 FVT（`tests/fvt.py`）—— `test_content_warnings`

用户：`creator`（自我申报）、`viewer`（观察者）、`admin` 经
`ADMIN_BOOTSTRAP_*`。

1. **AC-1：** 创作者创建带 `content_warnings: ["mature"]` 的模板 → 响应
   `Template.content_warnings == ["mature"]`，`locked_warnings` 为空；GET
   详情返回相同。
2. **AC-6：** 创作者更新移除 `mature`（自我申报）→ 200，`content_warnings`
   为空。
3. **FR-8/AC-6：** 管理员带原因添加 `violence` → 审计日志含
   `template.warning_add`（操作者/原因）；创作者通知含
   `template_warning_changed`；创作者 GET 显示 `locked_warnings ==
   ["violence"]`；创作者更新移除 `violence` → 403。
4. **FR-6/AC-4：** `GET /templates?exclude_warned=true` 排除带警告模板；
   无参数时存在。
5. **FR-9/AC-7：** 管理端列表 `?warning=none` 排除带警告；`?warning=mature`
   只显示带 mature 的；`?warning=violence` 只显示带 violence 的。
6. **FR-2：** 创建时未知警告类型 → 400。
7. **FR-11/AC-9：** 带警告模板在 `sort=newest` 与 `sort=most_liked` 中的
   位置与无警告时完全一致（创建时间戳/点赞相同的带警告与无警告模板；断言
   相对顺序不变）。
8. **NFR-2：** PUT `/admin/settings` `content_warning_taxonomy` 为自定义
   列表 → 新类型被接受；PUT 回默认。
9. **审计：** `/admin/audit-log?action=template.warning_add` 与
   `action=template.warning_remove` 含带操作者 + 原因的变更。

### 8.2 单元测试

- 分类法校验矩阵（有效键、未知键、空数组）。
- 锁定执行矩阵：锁定警告被移除 → 错误；锁定警告保留 → 正常；自我申报移除
  → 正常；自我申报添加 → 正常。
- `exclude_warned` 筛选 SQL（带警告排除、无警告保留、私有流不动）。
- 管理端警告状态筛选（`none`/`any`/特定键）。
- `UpdateTemplateWarnings` 集合运算（并/差、锁定保留）。
- 审计/通知尽力而为（失败记录、变更不受影响）。
- 更新 mock：`MockTemplateRepository`（新列）。

### 8.3 E2E（test-agent，Nightwatch）

- 卡片模糊 + 警告角标渲染；标题在模糊之外可读。
- 详情插页：原因列表、显示内容揭示、返回返回；同会话刷新保持揭示
  （sessionStorage），新会话重新门控。
- `show_warned_content` 开启 → 任何地方无模糊、无插页。
- `exclude_warned` 筛选：带警告模板缺席；登录用户刷新后状态保留；URL
  `?hideWarned=1` 同步。
- 管理端面板：带原因必填增删（原因 ≥3 字符前 Apply 禁用）、历史列表渲染、
  成功 toast。
- 创作者编辑器：锁定警告渲染为禁用/勾选，带锁图标 + 提示；移除被阻止。
- 通知：`template_warning_changed` 行渲染增量句子；深链到模板。
- 所有 `warnings.*` 键的 i18n EN/ZH 对齐。

## 9. 实施顺序

1. DDL（两份 init.sql）+ 模型 + 分类法种子 + 单元测试。
2. Proto 变更 + 重新生成 + `templateModelToProto` 映射 + 仓库
   SELECT/INSERT/UPDATE 列。
3. `exclude_warned` 筛选 + 管理端警告状态筛选。
4. `CreateTemplate`/`UpdateTemplate` 中的分类法校验 + 锁定执行。
5. 管理端警告 RPC（`UpdateTemplateWarnings`、`ListWarningHistory`）+ 通知 +
   审计。
6. 网关路由 + 查询参数。
7. FVT `test_content_warnings`。
8. 前端观众/创作者（复选框组、模糊、插页、筛选角标、设置开关）+ i18n
   en/zh。
9. 前端管理端（警告筛选、操作面板）+ NotificationItem。
10. 组件测试 + 移交 test-agent 做 e2e。

## 10. 给开发者的约束

1. **绝不提交 `backend/server`** —— 受跟踪构建产物；若被修改则 `git
   checkout -- backend/server`。
2. 只暂存你自己的文件；绝不 `git add -A`（工作树含其他代理的 WIP）。
3. 两份 init.sql 在同一提交中逐字节一致；每个 DDL 块幂等（列存在性保护、
   CHECK 扩宽用 `pg_get_constraintdef`、分类法种子用 `ON CONFLICT DO
   NOTHING`）。
4. **`Template` proto 字段号仅 27/28。** 18–26 被 follow-creators（18/19）、
   template-ratings（20–22）、prompt-analytics（23/24）与
   pre-publication-review（25/26）预留/占用 —— 绝不复用。不要动
   `AdminTemplateSummary` 13/14（被 ratings/import-export 争议）—— 新管理
   字段从 15 开始。
5. **不要用 `templates.moderation_status` 承载警告状态** —— 它是
   `('normal','in_review','rejected')`，属于 pre-publication-review。警告只
   存于 `content_warnings`/`locked_warnings`。
6. 锁定规则为 `locked_warnings ⊆ content_warnings`；创作者绝不能移除锁定
   警告（403）。审核员可带原因移除任何警告（锁定或自我申报）。
7. `exclude_warned` 仅作用于公开流；私有流不动。分类法数组作为参数传入，
   绝不内联。
8. 警告类型对照服务端分类法白名单校验 —— 绝不信任客户端字符串。
9. 两个警告列用 `pq.StringArray`；默认为空数组；无需回填。
10. 无任何形式的商业化界面。