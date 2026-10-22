# 评论回复与话题 — 架构设计

| | |
|---|---|
| **功能 ID** | comment-replies-threads |
| **版本** | 1.0 |
| **状态** | 已批准实施 |
| **作者** | Architect Agent |
| **上游** | [需求](../design/comment-replies-threads-requirements-zh-CN.md), [UI/UX 设计](../design/comment-replies-threads-ui-ux-design-zh-CN.md) |
| **下游** | [详细设计](comment-replies-threads-detailed-design-zh-CN.md) |

## 1. 概述

本功能为已上线的平铺评论系统（`template-comments`，提交 793a48f）添加**单层话题**。它遵循 GitHub/YouTube 模式：顶层评论保持最新优先并分页；回复在其顶层父评论下缩进一层，按需以最旧优先（对话式）顺序加载，每话题上限 50 条。回复一条回复会平铺到同一顶层话题（仅一层嵌套）。父评论作者会收到新的 `comment_reply` 事件类型通知。

范围：

1. **回复** —— 任何已认证用户都可以回复公开模板上的顶层评论（被封禁用户 `403`）；回复继承所有现有评论规则（1–2000 字符、15 分钟编辑窗、作者或所有者或管理员删除、速率限制 5/分钟 + 50/天）。
2. **话题** —— 回复在其父评论下缩进渲染；折叠的「查看 N 条回复」桶支持懒加载 + 缓存；`has_creator_reply` 驱动「已解答」徽章。
3. **计数器** —— `template_comments.reply_count`（直接回复，事务性维护）和 `templates.comment_count`（仅顶层评论，语义不变）。
4. **通知** —— `comment_reply` 事件类型通知父评论作者（与 `comment` 不同，后者通知模板所有者）；负载携带回复者显示名和 ≤100 字符摘录。
5. **审核** —— 删除顶层评论会在同一事务中级联删除其回复；删除回复会递减 `reply_count`；管理评论列表新增 `parent` 列。

### 1.1 已解决的开放问题

| # | 问题（来自需求 §10） | 决定 |
|---|---|---|
| 1 | `parent_comment_id` 约束：普通 UUID 列 vs 外键 `REFERENCES template_comments(id) ON DELETE CASCADE` | **普通 UUID 列，无外键。** 级联在服务层处理（FR-11），与已上线的事务性重算计数器的删除流程一致。自引用外键会使删除事务排序复杂化，且服务层级联已足够。 |
| 2 | 回复上限：每话题 50 条 vs 回复分页 | **每话题 50 条（上限）。** 为不分页的 `ListCommentReplies` 响应设界；第 51 条回复返回 `400` 及本地化「话题已满」消息。回复分页为罕见场景增加机制。 |
| 3 | `has_creator_reply` 计算：子查询 vs 反规范化标志 | **顶层列表查询中的子查询。** 列表页 ≤ 50 行，索引化的 `(parent_comment_id, created_at)` 查询很便宜；反规范化标志在每次回复时增加一次写，而读节省可忽略。 |
| 4 | 通知类型命名：`comment_reply` vs `reply` | **`comment_reply`。** 与现有 `comment` 类型一致且自解释。`notifications.type` CHECK 被扩展（受保护、幂等）。notification-preferences（积压功能）落地时需添加 `comment_reply_enabled` 列 —— 见 §7 协调。 |

### 1.2 非目标

- 超过一层的多层嵌套（回复的回复平铺到顶层话题）。
- 回复文本中的 @提及（属单独的 `mention-notifications` 功能）。
- 回复表情/emoji；回复排序选项（仅最旧优先）。
- 50+ 回复话题的「continue this thread」分页（上限已处理）。
- 发布后编辑回复目标；折叠单条回复。
- 回复通知的邮件摘要；置顶回复。
- 向父评论作者以外的人（如话题其他参与者）发送回复通知。
- 管理员话题视图 UI（v1 的管理列表 + parent 列已足够）。
- **任何形式的变现** —— 支付、定价、购买、收益、销售或付费排名界面均不在范围内，并排除在本架构之外。

## 2. 系统上下文

```mermaid
flowchart LR
    subgraph Client
        FE[React 前端<br/>CommentsSection / CommentItem / ReplyComposer / RepliesBucket]
        ADM[管理控制台<br/>AdminCommentsPage + parent 列]
    end
    subgraph Gateway
        GW[REST 网关 :8080<br/>/api/v1/templates/{id}/comments<br/>/api/v1/comments/{id}/replies<br/>/api/v1/comments/{id}<br/>/api/v1/admin/comments...]
    end
    subgraph Backend
        PS[PromptService<br/>ListComments / CreateComment / ListCommentReplies]
        AS[AdminService<br/>ListAdminComments / DeleteAdminComment]
        CR[CommentRepository<br/>话题感知的 list/create/delete]
        NR[NotificationRepository<br/>comment_reply]
        TR[TemplateRepository]
        RDS[(Redis<br/>速率限制计数器)]
        PG[(PostgreSQL 15)]
    end
    FE -->|GET/POST /api/v1/templates/{id}/comments<br/>GET /api/v1/comments/{id}/replies| GW --> PS
    ADM -->|GET /api/v1/admin/comments<br/>DELETE /api/v1/admin/comments/{id}| GW --> AS
    GW --> CR
    PS --> TR
    PS -->|固定窗口检查/递增| RDS
    PS -->|提交后：comment_reply（尽力而为）| NR
    AS --> CR
    CR --> PG
    NR --> PG
    TR --> PG
```

无新增基础设施。现有表新增一列、一个新索引、一个新通知事件类型、一条新 REST 路由。

## 3. 涉及组件

| 组件 | 变更 |
|---|---|
| `backend/api/proto/v1/prompt.proto` | `Comment` 新增 `reply_count`（10）和 `has_creator_reply`（11）；新增 `ListCommentReplies` RPC + `ListCommentRepliesRequest`/`ListCommentRepliesResponse`；`CreateCommentRequest` 新增 `parent_comment_id`（3）。 |
| `backend/api/proto/v1/notification.proto` | `Notification` 新增 `snippet`（10）。 |
| `backend/internal/models/comment.go` | `Comment` 新增 `ParentCommentID`、`ReplyCount`、`HasCreatorReply`。 |
| `backend/internal/models/notification.go` | `Notification` 新增 `Snippet`。 |
| `backend/internal/repository/comment_repository.go` | `List` 仅返回顶层 + 每项 `reply_count`/`has_creator_reply`；新增 `ListReplies`；`Create` 接受 `parent_comment_id` 并维护 `reply_count`；`Delete` 级联回复并维护两个计数器。 |
| `backend/internal/repository/notification_repository.go` | `Create`/`List` 处理 `snippet` 列。 |
| `backend/internal/service/comment.go` | `CreateComment` 校验 `parent_comment_id`（顶层父评论、同一模板、话题上限）；`ListComments` 仅返回顶层并带话题元数据；新增 `ListCommentReplies`；`DeleteComment` 级联；`comment_reply` 通知钩子。 |
| `backend/internal/service/notification.go` | `notificationToProto` 映射 `snippet`。 |
| `backend/cmd/server/main.go` | 新增 `/api/v1/comments/{id}/replies` GET 分支；`CreateComment` body 新增 `parent_comment_id`。 |
| `frontend/src/components/comments/CommentsSection.js` | 话题感知渲染；一次仅一个编辑器协调。 |
| `frontend/src/components/comments/CommentItem.js` | 回复操作、回复桶切换、已解答徽章。 |
| `frontend/src/components/comments/ReplyComposer.js` | **新增** —— 紧凑编辑器变体。 |
| `frontend/src/components/comments/RepliesBucket.js` | **新增** —— 折叠计数按钮、懒加载 + 缓存、内联错误 + 重试。 |
| `frontend/src/pages/admin/CommentsPage.js` | **新增** —— 审核表新增 parent 列。 |
| `frontend/src/services/api.js` | `listCommentReplies`，`createTemplateComment` 新增 `parent_comment_id`。 |
| `frontend/src/locales/{en,zh}/translation.json` | `comments.*` 回复键、`admin.comments.*` parent 键、`notifications.*` `comment_reply` 键。 |
| `deployment/init.sql`（+ chart 副本） | `template_comments.parent_comment_id` + `reply_count` 列、索引、notifications CHECK 扩展。 |

## 4. API 设计

### 4.1 用户评论 API

| 方法 | 路径 | 认证 | 描述 |
|---|---|---|---|
| `GET` | `/api/v1/templates/{id}/comments` | 可选 | 仅顶层评论，分页，最新优先；每项携带 `reply_count` 和 `has_creator_reply`。 |
| `POST` | `/api/v1/templates/{id}/comments` | 必需 | Body `{content, parent_comment_id?}`。无 `parent_comment_id` 时为顶层；有则为回复。 |
| `GET` | `/api/v1/comments/{id}/replies` | 可选 | 某顶层评论的所有回复，最旧优先，不分页（≤ 50）。 |
| `PUT` | `/api/v1/comments/{id}` | 必需 | 编辑（不变；15 分钟窗口）。 |
| `DELETE` | `/api/v1/comments/{id}` | 必需 | 删除（作者或模板所有者）；目标是顶层时级联回复。 |

规则：

- POST/PUT/DELETE 未认证 → `401 UNAUTHENTICATED`。
- 被封禁用户发布 → `403 PERMISSION_DENIED`。
- **`parent_comment_id`** 必须引用同一模板上存在的顶层评论；指向回复 → `400 INVALID_ARGUMENT`（「父评论必须是顶层评论」）。未知父评论 → `400 INVALID_ARGUMENT`。
- **话题上限**：某话题第 51 条回复 → `400 INVALID_ARGUMENT`（「话题已满」）。
- **速率限制**（5/分钟、50/天）同样适用于回复 → `429 RESOURCE_EXHAUSTED` 并带 `Retry-After`。
- **可见性** 完全遵循模板：公开模板可匿名读取；私有模板仅所有者（否则 `404`）。
- **`comment_count`** 仅顶层评论递增；回复递增父评论的 `reply_count`。

### 4.2 管理评论 API

| 方法 | 路径 | 认证 | 描述 |
|---|---|---|---|
| `GET` | `/api/v1/admin/comments` | 管理员 | 现有列表 + `parent` 列（顶层为 null，「回复 {{author}}」为回复）。 |
| `DELETE` | `/api/v1/admin/comments/{id}` | 管理员 | 带必填理由 + 审计删除；遵循级联语义。 |

## 5. 数据模型

### 5.1 `template_comments`（修改）

| 列 | 类型 | 说明 |
|---|---|---|
| `id` | UUID PK | 不变 |
| `template_id` | UUID FK → templates | 不变 |
| `user_id` | TEXT FK → users | 不变 |
| `content` | TEXT | 不变 |
| `created_at` / `updated_at` | TIMESTAMPTZ | 不变 |
| `parent_comment_id` | UUID，可空 | **新增** —— 顶层为 null；引用顶层评论（无外键，服务层强制） |
| `reply_count` | INT NOT NULL DEFAULT 0 | **新增** —— 直接回复数，事务性维护 |

新索引：`idx_template_comments_parent_created ON template_comments
(parent_comment_id, created_at ASC)`。

### 5.2 `notifications`（修改）

| 列 | 类型 | 说明 |
|---|---|---|
| `snippet` | TEXT，可空 | **新增** —— `comment_reply` 事件的 ≤100 字符回复摘录；其他类型为 null |

`notifications.type` CHECK 被扩展以接纳 `comment_reply`。

## 6. 横切关注点

- **授权**：回复可见性遵循模板可见性；变更需认证；被封禁用户不能回复；删除需作者或所有者（用户界面）或管理员（管理界面）。管理路由保持在 `/api/v1/admin/*` 下并 `requireAdmin`。
- **i18n**：所有新字符串在 `en` 和 `zh` 下均存在，键集相同，位于 `comments.*`、`admin.comments.*` 和 `notifications.*`。
- **性能（NFR-1）**：顶层列表成本不变（两个新列通过子查询，无 N+1）；回复按需按话题加载（≤ 50 行，单次索引查询）。
- **一致性（NFR-2）**：`reply_count` 使用与 `comment_count`/`like_count` 相同的事务性计数器模式；并发回复下无漂移。
- **隐私（NFR-3）**：回复可见性与评论可见性一致；通知摘录不超过回复前 100 字符。
- **可访问性（NFR-5）**：话题是嵌套语义列表；桶切换是带 `aria-expanded` 的真实按钮；回复编辑器有标签；所有操作可键盘操作。
- **韧性（NFR-6）**：回复列表获取失败仅在桶内显示内联错误 + 重试；回复发布失败保留草稿。

## 7. 部署影响

- 将 §2 DDL 块追加到 `deployment/init.sql` 和 `deployment/chart/open-prompts/init.sql`（字节一致、幂等、增量 —— 现有平铺评论的 `parent_comment_id` 为 null，渲染不变）。
- 需为 `prompt.proto` 和 `notification.proto` 重新生成 `protoc`。
- 无新增服务、端口或镜像。无新增环境变量（复用现有 `COMMENT_RATE_PER_MINUTE` / `COMMENT_RATE_PER_DAY`）。
- **与 notification-preferences 的协调**（积压）：该功能落地时，其 `notification_preferences` 表必须包含 `comment_reply_enabled BOOLEAN NOT NULL DEFAULT true` 列和 `notifPrefs.type.comment_reply` UI 行。在此之前，`comment_reply` 事件始终发出（无抑制）。
- 回滚：删除两个新列和索引，并还原 notifications CHECK 扩展即可；新 REST 路由会 404，但其他功能不降级。

## 8. 测试策略（摘要）

- **FVT（`tests/fvt.py`）**：扩展 `test_template_comments()` —— 回复挂载规则（仅顶层、同一模板）、话题上限（第 51 条 → 400）、级联删除（顶层移除回复，`comment_count` −1；回复移除自身，`reply_count` −1）、计数器、`comment_reply` 通知（父作者被通知、自回复抑制、摘录 ≤100 字符）、`has_creator_reply` 徽章、管理 parent 列、回复速率限制。
- **后端单元测试**：`parent_comment_id` 校验、话题上限边界、级联事务、计数器维护、`has_creator_reply` 子查询、通知摘录截断、权限矩阵。
- **E2E（Nightwatch，test-agent）**：回复流程、桶展开/折叠 + 懒加载 + 缓存、已解答徽章、管理 parent 列、EN/ZH 字符串、键盘访问。

详细契约、DDL、层职责、i18n 键和测试钩子：见[详细设计](./comment-replies-threads-detailed-design-zh-CN.md)。