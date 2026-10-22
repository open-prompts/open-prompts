# 内容举报 — 架构设计

| | |
|---|---|
| **功能 ID** | content-reporting |
| **版本** | 1.0 |
| **状态** | 已批准实施 |
| **作者** | Architect Agent |
| **上游** | [需求](../design/content-reporting-requirements-zh-CN.md)、[UI/UX 设计](../design/content-reporting-ui-ux-design-zh-CN.md) |
| **下游** | [详细设计](content-reporting-detailed-design-zh-CN.md) |

## 1. 概述

内容举报为平台提供**入站滥用信号**：登录用户可以用固定原因和可选备注标记公开模板或公开评论，管理员则处理一个按条目分组的优先队列，审查抽屉携带完整上下文并支持一键处置（驳回或处理）。设计遵循 Reddit/FlowGPT 模式 —— 小型固定原因列表、每用户每条目一次举报（重复举报替换）、按未解决数和时效排序的队列，以及**无自动下架**（每个后果都需要明确的管理员操作，NFR-4）。

该功能引入一张新表 **`content_reports`**（OQ-1）记录每条举报。队列在读取时按 `(item_type, item_id)` 分组并计算 `COUNT(*)` 与 `MAX(created_at)`（OQ-2）—— 在预期规模下无需反规范化汇总表。处置在条目所有未解决举报间原子完成（NFR-3）。举报者对结果静默（FR-17）：无通知、无案件跟踪 —— 该功能的作用是路由注意力，而非构建案件管理系统。

### 1.1 已解决的开放问题

| # | 问题（来自需求 §10） | 决策 |
|---|---|---|
| 1 | 条目身份：评论需要稳定的公开 id | **评论 id 已暴露**在 `Comment` proto（`id = 1`）以及 `ListCommentsResponse`/`ListCommentRepliesResponse` 载荷中。模板使用其现有 UUID。无需新的 id 方案。 |
| 2 | 分组查询 vs 反规范化 `report_groups` 汇总表 | **读取时计算。** `(status, item_type, item_id)` 索引（FR-1）即可服务分组队列查询；在所述规模（1,000 条未解决举报，50/页）下，单次索引聚合远低于 NFR-1 的 1 秒预算。汇总表会增加写入路径复杂度而无实际收益。 |
| 3 | 自动处置触发器（FR-12）：事件驱动 vs 队列读取时清扫 | **在取消发布/删除时服务层挂钩。** 当模板通过任何路径被取消发布或评论被删除时，服务在同一事务中将该条目的未解决举报处置为 `actioned`。无清扫任务；无悬空队列行。 |
| 4 | 撤回语义：硬删除 vs 标记撤回 | **硬删除。** `DELETE /reports/mine` 完全删除该行；`UNIQUE (item_type, item_id, reporter_id)` 约束随后允许干净地重新举报。最简单且符合"无痕迹"意图。 |
| 5 | 速率限制存储：内存 vs redis | **Redis 计数器，1 小时 TTL**（`report_rate:{user_id}`），Redis 出错时放行。与现有评论速率限制模式（`checkCommentRate`）一致。 |
| 6 | 审计操作命名 | **新增审计操作码 `report.dismiss` 和 `report.actioned`。** `admin_audit_logs.action` CHECK 被扩展（幂等）以包含两者。`target_type` 复用现有 `template`/`comment` 值。 |

### 1.2 非目标

- 在举报阈值处自动下架或自动隐藏（v1 始终由人工决策）。
- 面向举报者的案件跟踪或处置通知（对举报者静默）。
- 举报用户/个人资料（用户滥用走现有封禁流程）。
- 举报合集、收藏或团队。
- 审核工作区、分配/认领、SLA 计时器。
- **任何形式的变现** —— 支付、定价、购买、收益、销售或付费排名界面均不在范围内，并从本架构中排除。

## 2. 系统上下文

```mermaid
flowchart LR
    subgraph Client
        FE[React 前端<br/>ReportMenu / ReportModal / ReportedMenuItem]
        ADM[管理控制台<br/>ReportsPage / ReviewDrawer / OpenReportsKPI]
    end
    subgraph Gateway
        GW[REST 网关 :8080<br/>/api/v1/reports... /api/v1/admin/reports...]
    end
    subgraph Backend
        PS[PromptService<br/>CreateReport / GetMyReport / WithdrawReport]
        AS[AdminService<br/>ListReportQueue / GetReportDetail / ResolveReportGroup]
        RR[ReportRepository<br/>content_reports CRUD + 分组队列]
        AR[AdminRepository<br/>审计 + 取消发布/删除挂钩]
    end
    subgraph Stores
        PG[(PostgreSQL)]
        RD[(Redis<br/>report_rate:{user} 计数器)]
    end
    FE -->|POST/GET/DELETE /api/v1/reports| GW --> PS
    ADM -->|GET/POST /api/v1/admin/reports| GW --> AS
    PS --> RR --> PG
    AS --> RR --> PG
    AS --> AR --> PG
    PS -->|report_rate:{user}| RD
```

## 3. 涉及组件

| 组件 | 变更 |
|---|---|
| `backend/api/proto/v1/prompt.proto` | `PromptService` 上的 `CreateReport`/`GetMyReport`/`WithdrawReport` RPC；`Report`/`MyReport` 消息。 |
| `backend/api/proto/v1/admin.proto` | `AdminService` 上的 `ListReportQueue`/`GetReportDetail`/`ResolveReportGroup` RPC；`AdminReportGroup`/`AdminReportDetail` 消息；`GetAdminStatsResponse` 上的 `open_reports` 字段。 |
| `backend/internal/models/report.go` | **新增** — `ContentReport` GORM 模型。 |
| `backend/internal/repository/report_repository.go` | **新增** — 举报 CRUD、分组队列、详情、原子处置、自动处置挂钩。 |
| `backend/internal/service/prompt.go` | `CreateReport`/`GetMyReport`/`WithdrawReport` 实现（认证、封禁检查、速率限制）。 |
| `backend/internal/service/admin.go` | `ListReportQueue`/`GetReportDetail`/`ResolveReportGroup` 实现（审计、取消发布/删除挂钩、KPI 计数）。 |
| `backend/cmd/server/main.go` | `/api/v1/reports` 与 `/api/v1/reports/mine` 分支；`/api/v1/admin/reports` 与 `/api/v1/admin/reports/{item_type}/{item_id}` 分支。 |
| `frontend/src/components/ReportMenu.js`、`ReportModal.js`、`ReportedMenuItem.js` | **新增。** |
| `frontend/src/pages/admin/ReportsPage.js`、`frontend/src/components/admin/ReviewDrawer.js`、`ReasonPopover.js` | **新增**管理端界面。 |
| `frontend/src/services/api.js`、`frontend/src/services/adminApi.js` | 举报 API 客户端方法。 |
| `frontend/src/locales/{en,zh}/translation.json` | `reports.*` 与 `admin.reports.*` 命名空间。 |
| `deployment/init.sql`（+ chart 副本） | `content_reports` 表、审计 CHECK 扩展。 |

## 4. API 设计

### 4.1 用户举报 API

| 方法 | 路径 | 认证 | 描述 |
|---|---|---|---|
| `POST` | `/api/v1/reports` | 必需 | Body `{item_type, item_id, reason, note}`。创建或更新调用者对该条目的举报。返回举报。 |
| `GET` | `/api/v1/reports/mine?item_type=&item_id=` | 必需 | 调用者对该条目的未解决举报（驱动对勾状态）。 |
| `DELETE` | `/api/v1/reports/mine` | 必需 | Body `{item_type, item_id}`。撤回（硬删除）调用者的举报。 |

规则：

- **未认证** → `401 UNAUTHENTICATED`。
- **被封禁用户** → `403 PERMISSION_DENIED`。
- **无效 `item_type`**（非 `template`/`comment`）→ `400 INVALID_ARGUMENT`。
- **条目未找到 / 非公开** → `404 NOT_FOUND`（举报私有或不存在的条目不可区分）。
- **举报自己的内容** → `400 INVALID_ARGUMENT`（FR-4 隐藏操作；服务端作为兜底强制）。
- **无效原因**（不在 5 码分类中）→ `400 INVALID_ARGUMENT`。
- **备注 > 500 字** → `400 INVALID_ARGUMENT`。
- **超出速率限制**（20/小时）→ `429 RESOURCE_EXHAUSTED` 并带 `Retry-After`。

### 4.2 管理端举报 API

| 方法 | 路径 | 认证 | 描述 |
|---|---|---|---|
| `GET` | `/api/v1/admin/reports?status=&item_type=&reason=&page=` | 管理员 | 分组队列，未解决优先按数量再按时效，50/页。 |
| `GET` | `/api/v1/admin/reports/{item_type}/{item_id}` | 管理员 | 详情：条目上下文 + 每条举报（举报者、原因、备注、时间）。 |
| `POST` | `/api/v1/admin/reports/{item_type}/{item_id}/resolve` | 管理员 | Body `{resolution: dismissed\|actioned, reason}`。原子处置该条目所有未解决举报。 |

规则：

- **非管理员** → `403 PERMISSION_DENIED`。
- **原因不在 3–500 字** → `400 INVALID_ARGUMENT`。
- **无效处置** → `400 INVALID_ARGUMENT`。
- **条目未找到** → `404 NOT_FOUND`。
- **无未解决举报** → `409 ALREADY_EXISTS`（无可处置）。

"处理"路径（取消发布模板 / 删除评论）复用现有管理端点；举报队列的处置端点记录处置结果，底层操作由现有流程执行（FR-10，NFR-7）。

### 4.3 错误码

| HTTP | gRPC | 条件 |
|---|---|---|
| 400 | `INVALID_ARGUMENT` | 无效 item_type/原因；备注 > 500；举报自己的内容；原因长度。 |
| 401 | `UNAUTHENTICATED` | 缺失/无效令牌。 |
| 403 | `PERMISSION_DENIED` | 被封禁用户；非管理员访问管理路由。 |
| 404 | `NOT_FOUND` | 条目未找到/非公开；举报未找到。 |
| 409 | `ALREADY_EXISTS` | 无可处置的未解决举报。 |
| 429 | `RESOURCE_EXHAUSTED` | 举报速率限制（`Retry-After`）。 |

## 5. 数据模型

### 5.1 新表：`content_reports`

```sql
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
CREATE INDEX IF NOT EXISTS idx_content_reports_status_item
    ON content_reports (status, item_type, item_id);
CREATE INDEX IF NOT EXISTS idx_content_reports_reporter
    ON content_reports (reporter_id);
```

`UNIQUE (item_type, item_id, reporter_id)` 在数据库层面强制 FR-1（每用户每条目一次举报）；`23505` 冲突映射为 upsert（重复举报替换，FR-2）。`status`/`resolution` 生命周期：`open` → `resolved`，`resolution` 为 `dismissed` 或 `actioned`。

### 5.2 CHECK 扩展（幂等，两个 init.sql 副本）

- `admin_audit_logs.action` 增加 `report.dismiss`、`report.actioned`。
- `admin_audit_logs.target_type` 已包含 `template` 和 `comment` —— 无需扩展。

## 6. 认证与授权

- **用户写入**需要会话/API 密钥认证；用户 id 来自上下文，绝不来自 body。封禁用户被拒绝（FR-3）。用户不能举报自己的内容。
- **管理端写入**需要管理 JWT（`requireAdminContext`）。
- **可见性**：举报者仅对管理员可见（NFR-2）；面向用户的 `GET /reports/mine` 仅返回调用者自己的举报。内容作者永远看不到谁举报了他们。
- **速率限制**：举报提交限制为每用户每小时 20 次，通过 Redis 计数器（`report_rate:{user_id}`，1 小时 TTL），Redis 出错时放行。

## 7. 前端

- **ReportMenu**（模板和评论上的 ⋯ 菜单项）：无未解决举报时显示"举报"；调用者有未解决举报时显示"您已举报此内容"并带更新/撤回子菜单（FR-7）。对用户自己的内容隐藏（FR-4）。
- **ReportModal**：五个单选原因带描述 + 可选备注（≤500）、取消/提交；未选原因前提交禁用；焦点陷阱对话框（NFR-6）。标题为"举报此模板"/"举报此评论"。
- **ReportedMenuItem**：对勾状态，带更新（重新打开预填模态框）和撤回（确认弹窗）。
- **ReportsPage**（`/admin/reports`）：分组队列表 —— 类型徽章、条目标题/摘要、作者、未解决数、原因芯片、最新时间、状态、审查/查看操作；过滤器（状态/类型/原因）；未解决优先按数量再按时效排序（FR-8/FR-9）。
- **ReviewDrawer**：右侧抽屉，含条目上下文卡片、作者链接、每条举报（举报者、原因、备注、时间）以及处置操作 —— 驳回举报 / 处理（取消发布模板或删除评论），每个都需要原因（FR-10）。"封禁作者"链接到管理端用户详情。
- **OpenReportsKPI**：仪表盘卡片，显示未解决举报数并链接到 `/admin/reports`（FR-13）。
- **i18n**：`reports.*` 与 `admin.reports.*` 命名空间；按 UI/UX 文档 §8 保持 EN/ZH 对齐。

## 8. 配置

| 变量 | 默认值 | 描述 |
|---|---|---|
| `REPORT_RATE_PER_HOUR` | `20` | 每用户每小时最大举报提交次数。 |

添加到 `deployment/docker-compose.yml` 和 Helm chart（`values.yaml` + `templates/deployment-backend.yaml`），与 `RATING_RATE_PER_MINUTE` 一致。

## 9. 安全考量

- **隐私**：举报者仅管理员可见（NFR-2）；备注为机密审核数据；对非所有者无存在性泄露（404）。
- **完整性**：`UNIQUE (item_type, item_id, reporter_id)` 强制每用户每条目一次举报；处置在条目所有未解决举报间原子完成（NFR-3）；取消发布/删除时的自动处置是事务性的。
- **滥用**：20/小时速率限制；封禁用户被阻止；无自动下架（NFR-4）。
- **注入**：仅使用参数化 SQL。
- **XSS**：备注和原因均为纯文本，仅转义渲染。

## 10. 部署说明

- DDL 为增量且幂等；两个 init.sql 副本在同一提交中更新。
- 无新服务或基础设施；一个新增 Redis 计数器键。
- `backend/server` 是受跟踪的构建产物 —— 切勿提交对其的更改。

## 11. 测试策略

- **FVT**（`tests/fvt.py`）：`test_content_reporting` —— 创建/更新举报、每用户 upsert、撤回、封禁 403、匿名 401、速率限制 429、管理队列分组 + 排序 + 过滤、详情、驳回/处理处置、取消发布/删除时自动处置、审计条目、KPI 计数。
- **单元**：仓库 upsert + 唯一冲突映射、分组队列查询、原子处置、自动处置挂钩、速率限制计数器、服务校验 + 授权。
- **E2E**（测试代理）：举报模态框流程、已举报状态菜单、队列表、审查抽屉、驳回/处理、KPI 卡片、i18n 对齐。