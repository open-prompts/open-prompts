# 管理员模板审核 — 架构设计

| | |
|---|---|
| **功能 ID** | admin-template-moderation |
| **版本** | 1.0 |
| **状态** | 已批准实施 |
| **作者** | Architect Agent |
| **上游** | [需求](../design/admin-template-moderation-requirements-zh-CN.md), [UI/UX 设计](../design/admin-template-moderation-ui-ux-design-zh-CN.md) |
| **下游** | [详细设计](admin-template-moderation-detailed-design-zh-CN.md) |

## 1. 概述

管理员模板审核为平台提供了**分级、可审计的执法**能力。目前，管理员发现违反政策的模板时只能查看（管理控制台的模板页面按设计是只读的）或封禁所有者（核弹级手段）。本功能新增三种**状态变更而非删除**——**取消发布**（公开 → 私有，可逆，必填原因，通知创作者）、**锁定**（冻结编辑及新的评论/派生/点赞/评分/关注，内容保持公开）、**软删除**（处处隐藏，30 天恢复窗口，之后级联硬删除）。三者都写入现有的 `admin_audit_logs`，包含操作者、目标、原因和时间戳，并通过新的 `moderation` 通知类型通知创作者。

本功能复用了既有模式：`moderation_status` 列（由 pre-publication-review 引入，CHECK 范围较窄，本功能将其扩展）、`writeAudit` + `requireAdminContext` 管理服务形态、`createEngagementNotification` 尽力而为的通知钩子、`buildTemplateConditions` 封禁所有者隐藏模式（扩展至审核状态并带所有者绕过）、`moderation_actions` 表（由 moderation-appeals 定义，**在此填充**）、原因弹窗 + 输入标题确认模式（来自 admin-user-management / account-deletion），以及 scheduler-sharing/webhooks 的数据库支撑到期扫描 ticker 模式用于 30 天硬删除。

本功能还**闭环了 content-reporting**：举报队列的"处理 → 取消发布模板"流程（content-reporting FR-15）目前只将举报组解析为 `actioned` 而未真正取消发布——本功能提供该流程调用的真实取消发布端点，并在同一事务中触发自动解析钩子（content-reporting FR-12）。

### 1.1 已解决开放问题

| # | 问题（来自需求 §10） | 决策 |
|---|---|---|
| 1 | 访问检查组合：单一守卫 vs 逐端点 | **单一仓储层守卫。** 扩展现有的 `buildTemplateConditions`（所有公开读取路径流经的唯一位置），对非所有者隐藏 `unpublished_by_admin` 和 `soft_deleted` 模板，镜像封禁所有者模式。所有者绕过（`t.owner_id = $1`）已存在。这在一处覆盖首页、搜索、分类、相关推荐、嵌入、二维码分享、关注流以及 SDK 的 `get_template`/`get_prompt_by_alias`——无逐端点泄漏风险。 |
| 2 | 当前状态用列 vs 独立表 | **`templates.moderation_status` 列**（热路径读取，WHERE 子句中多一列）。历史来自 `admin_audit_logs`（不可变）加上 `moderation_actions` 表（用于申诉的可变生命周期）。无需独立当前状态表。 |
| 3 | 锁定与所有者编辑的交互 | **同时阻止编辑和重新发布**（真正冻结，FR-3）。所有者仍可查看模板及其内容；锁定横幅解释原因。这是最简单正确的语义，符合 UI/UX 的"审核期间编辑与新互动被禁用"。 |
| 4 | 硬删除复用 | **提取共享的模板级联服务。** 30 天到期调用与 account-deletion 定义的相同级联例程（版本、别名、其上的评论删除；他人的派生以"已删除来源"谱系存活）。该例程提取为可复用的 `TemplateCascadeService`，使 account-deletion 与本功能共享。 |
| 5 | 无需管理员审核的重新发布（FR-2） | **接受（选项 a）并带审计轨迹标记。** 所有者可在取消发布后立即重新发布；恶意所有者可能原样重新发布。缓解措施是 v1 接受（管理员可再次取消发布，重复违规升级为封禁），外加在重新发布被审核的模板时写入新审计事件 `template.republished_after_moderation`，使该模式在审计轨迹中可见。 |

### 1.2 非目标

- 批量审核操作（多模板取消发布/删除）——明确不在范围内（FR-15）。
- 发布前审核队列（提交 → 批准）——独立功能（pre-publication-review）。
- 创作者申诉工作流（应用内申诉表单、案件状态）——独立功能（moderation-appeals）；v1 为通知 + 站外联系。
- 管理员编辑模板内容（仅状态变更；内容编辑仍归所有者）。
- 年龄限制 / 内容警告覆盖层——独立功能（content-warnings）。
- 影子封禁或分级可见性（仅搜索降权）。
- **任何形式的变现**——支付、定价、购买、收益、销售或付费排名界面均不在范围内，并从本架构中排除。

## 2. 系统上下文

```mermaid
flowchart LR
    subgraph Client
        FE[React 前端<br/>OwnerModerationBanner / LockBanner / RemovedNotice]
        AD[管理控制台<br/>TemplatesPage 状态筛选 / TemplateDetailPage ModerationPanel]
    end
    subgraph Gateway
        GW[REST 网关 :8080<br/>/api/v1/admin/templates/{id}/unpublish|lock|delete|restore]
    end
    subgraph Backend
        AS[AdminService<br/>UnpublishTemplate / LockTemplate / SoftDeleteTemplate / RestoreTemplate]
        TR[TemplateRepository<br/>审核过滤 + 访问守卫]
        AR[AdminRepository<br/>SetTemplateModeration / 审计 / moderation_actions 插入]
        RR[ReportRepository<br/>AutoResolveItem 钩子]
        NR[NotificationRepository<br/>moderation 通知]
        TC[TemplateCascadeService<br/>30 天硬删除]
    end
    subgraph Stores
        PG[(PostgreSQL)]
        RD[(Redis<br/>mod_action_rate:{admin} 计数器)]
    end
    FE -->|所有者视图| GW
    AD -->|POST /api/v1/admin/templates/{id}/...| GW --> AS
    AS --> TR --> PG
    AS --> AR --> PG
    AS --> RR --> PG
    AS --> NR
    TC -->|ticker 扫描| PG
    AS -->|mod_action_rate:{admin}| RD
```

## 3. 涉及组件

| 组件 | 变更 |
|---|---|
| `backend/api/proto/v1/admin.proto` | `AdminService` 新增 4 个 RPC：`UnpublishTemplate`、`LockTemplate`、`SoftDeleteTemplate`、`RestoreTemplate`；新增请求/响应消息；`AdminTemplateSummary` 新增 `moderation_status`/`moderated_at`/`moderated_by`/`moderation_reason` 字段；`ListAdminTemplatesRequest` 新增 `moderation_status` 筛选。 |
| `backend/internal/service/admin.go` | `UnpublishTemplate`/`LockTemplate`/`SoftDeleteTemplate`/`RestoreTemplate` 实现（自守卫、原因校验、审计、通知、moderation_actions 插入、举报自动解析钩子）。 |
| `backend/internal/repository/admin_repository.go` | `SetTemplateModeration` 扩展至新状态；新增 `InsertModerationAction`；复用 `ListAuditByTarget` 用于审核历史。 |
| `backend/internal/repository/template_repository.go` | `buildTemplateConditions` 扩展为对非所有者隐藏 `unpublished_by_admin`/`soft_deleted`（所有者绕过已存在）。 |
| `backend/internal/repository/report_repository.go` | 从取消发布/软删除路径调用 `AutoResolveItem` 钩子（已由 content-reporting 定义）。 |
| `backend/internal/service/template_cascade.go` | **新增**——共享硬删除级联服务（为 account-deletion 复用而提取）。 |
| `backend/internal/service/ticker.go` | **新增**——30 天硬删除到期扫描 ticker（scheduler-sharing/webhooks 模式）。 |
| `backend/cmd/server/main.go` | `/api/v1/admin/templates/{id}/...` 下 4 个管理路由（全部 `requireAdmin`）。 |
| `frontend/src/pages/admin/TemplatesPage.js` | 状态筛选（公开/私有/锁定/已取消发布/软删除）+ 状态徽章列 + 恢复倒计时。 |
| `frontend/src/pages/admin/TemplateDetailPage.js` | **ModerationPanel**（状态徽章、操作、历史）。 |
| `frontend/src/components/admin/ModerationPanel.js`、`ActionReasonModal.js`、`StatusChip.js`、`RestoreCountdown.js` | **新增。** |
| `frontend/src/components/moderation/*` | **新增**——`OwnerModerationBanner`、`LockBanner`、`RemovedNotice`。 |
| `frontend/src/components/notifications/NotificationItem.js` | 渲染 `moderation` 通知类型。 |
| `frontend/src/components/admin/ReviewDrawer.js` | "处理 → 取消发布"现在调用真实的取消发布端点（FR-10）。 |
| `frontend/src/locales/{en,zh}/translation.json` | `mod.*` 命名空间（逐字取自 UI/UX §8）。 |
| `deployment/init.sql`（+ chart 副本） | 扩展 `templates_moderation_status_check`；新增 `moderated_at`/`moderated_by`/`moderation_reason` 列；扩展审计 + 通知 CHECK。 |

## 4. API 设计

### 4.1 管理员审核 API

| 方法 | 路径 | 认证 | 描述 |
|---|---|---|---|
| `POST` | `/api/v1/admin/templates/{id}/unpublish` | 管理员 | 请求体 `{reason}`。设置 `moderation_status = 'unpublished_by_admin'`。 |
| `POST` | `/api/v1/admin/templates/{id}/lock` | 管理员 | 请求体 `{reason}`。设置 `moderation_status = 'locked'`。 |
| `POST` | `/api/v1/admin/templates/{id}/delete` | 管理员 | 请求体 `{reason}`。设置 `moderation_status = 'soft_deleted'`、`deleted_at = now()`。 |
| `POST` | `/api/v1/admin/templates/{id}/restore` | 管理员 | 请求体 `{reason}`。恢复为 `normal`（保留可见性）。 |

规则：

- **非管理员** → `403 PERMISSION_DENIED`。
- **原因超出 3–500 字符** → `400 INVALID_ARGUMENT`。
- **管理员审核自己的模板** → `403 PERMISSION_DENIED`（FR-14）。
- **模板不存在** → `404 NOT_FOUND`。
- **无效转换**（如取消发布软删除的模板、恢复 `normal` 模板）→ `409 ALREADY_EXISTS`。
- **超出速率限制**（写入层，FR-15）→ `429 RESOURCE_EXHAUSTED` 并带 `Retry-After`。

### 4.2 管理列表筛选

`GET /api/v1/admin/templates` 新增 `moderation_status` 查询参数（`"" | normal | locked | unpublished_by_admin | soft_deleted`）。响应项携带 `moderation_status`、`moderated_at`、`moderated_by`、`moderation_reason`（仅管理员字段）。

### 4.3 错误码

| HTTP | gRPC | 条件 |
|---|---|---|
| 400 | `INVALID_ARGUMENT` | 原因长度；无效的 moderation_status 筛选。 |
| 401 | `UNAUTHENTICATED` | 缺失/无效令牌。 |
| 403 | `PERMISSION_DENIED` | 非管理员；管理员审核自己的模板。 |
| 404 | `NOT_FOUND` | 模板不存在。 |
| 409 | `ALREADY_EXISTS` | 无效状态转换。 |
| 429 | `RESOURCE_EXHAUSTED` | 审核速率限制（`Retry-After`）。 |

## 5. 数据模型

### 5.1 `templates` 新增

```sql
-- 扩展 moderation_status CHECK（守卫式、幂等）。
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
```

### 5.2 `moderation_actions`（在此填充）

`moderation_actions` 表由 **moderation-appeals 定义**（已在 `deployment/init.sql` 中）。本功能**填充**它：每次取消发布/锁定/软删除插入一行，含 `action_type`、`actor_id`、`actor_email`、`reason`、`status = 'active'`。恢复**不插入**行（`action_type` CHECK 仅允许 `('unpublish', 'lock', 'soft_delete')`）；它将先前操作标记为 `superseded`（使其不可申诉，按 moderation-appeals FR-14），并在审计日志（`template.restore`）中记录恢复。

### 5.3 CHECK 扩展（幂等，两个 init.sql 副本）

- `admin_audit_logs.action` 新增 `template.unpublish`、`template.lock`、`template.soft_delete`、`template.restore`、`template.republished_after_moderation`。
- `notifications.type` 新增 `moderation`。

## 6. 认证与授权

- 所有审核路由位于 `requireAdmin`（网关）和 `requireAdminContext`（服务）之后——role=admin 且 scope=admin，与现有管理控制台端点一致。
- **自守卫（FR-14）：** 管理员不能审核自己的模板——服务检查 `template.OwnerID == actor.ID` → `403 PERMISSION_DENIED`。
- **可归因性：** 每次操作写入审计行（操作者 id/email、操作、目标 `template`/模板 id、原因）和 `moderation_actions` 行——满足 NFR-2。
- **透明性（NFR-3）：** 审核原因仅对模板所有者和管理员可见；公开锁定横幅显示通用消息，绝不显示原因。

## 7. 前端

- **管理控制台：**
  - `TemplatesPage` 新增**状态筛选**（`moderation_status`）和状态徽章列；软删除行显示 `RestoreCountdown`（"剩余 N 天"）。
  - `TemplateDetailPage` 新增 **ModerationPanel**（状态徽章、三个操作及后果摘要、适用时恢复、以及按目标过滤的审计日志审核历史）。
  - `ActionReasonModal`（原因 3–500 字符 + 后果清单；删除需输入模板标题确认）。
  - `ReviewDrawer` "处理 → 取消发布"现在调用真实的取消发布端点（FR-10）并自动解析举报组。
- **创作者界面：**
  - `OwnerModerationBanner`（已取消发布：原因 + "编辑模板" CTA）、`LockBanner`（通用，公开）、`RemovedNotice`（软删除：原因 + 申诉提示）。
  - `NotificationItem` 渲染 `moderation` 类型，含操作、标题、原因摘录，以及（取消发布时）"修复并重新发布"深链。
- **i18n：** `mod.*` 键逐字取自 UI/UX §8，EN/ZH 对齐。

## 8. 配置

| 设置 | 默认 | 描述 |
|---|---|---|
| `MOD_ACTION_RATE_PER_MINUTE` | `10` | 每管理员审核写入速率限制（FR-15），服务端环境变量。 |
| `MOD_SOFT_DELETE_WINDOW_DAYS` | `30` | 硬删除前的恢复窗口（FR-4），服务端环境变量。 |

两者都是服务端环境变量（镜像 `APPEAL_MAX_OPEN`）；v1 无需 `platform_settings` 行。

## 9. 安全与隐私考虑

- **无状态泄漏：** `unpublished_by_admin` 和 `soft_deleted` 模板对所有非所有者路径返回 404（详情、列表、搜索、个人主页、派生来源、评论、嵌入、二维码分享、SDK）——全部流经同一 `buildTemplateConditions` 可见性规则。
- **锁定模板保持公开**但所有变更在服务层被阻止（编辑、评论、派生、点赞/收藏/评分、关注、预设保存）——逐变更强制，而非仅在 UI 中。
- **可审计性：** 每次操作、恢复和计划硬删除都写入不可变审计行；详情页的审核历史完整。
- **注入：** 仅参数化 SQL；原因长度在服务端校验；筛选值永不进入 SQL 文本。
- **透明性（NFR-4）：** 每次操作都通知创作者原因；所有者始终知道其内容为何被处理。

## 10. 部署说明

- DDL 为增量且幂等；`deployment/init.sql` 和 `deployment/chart/open-prompts/init.sql` 在同一提交中逐字节一致更新。CHECK 扩展使用 `pg_get_constraintdef` 守卫。
- 无新服务、端口或基础设施；除 init.sql ConfigMap 内容外无 chart 变更。
- `backend/server` 是受跟踪的构建产物——绝不提交对其的更改。

## 11. 测试策略

- **FVT**（`tests/fvt.py`）：`test_admin_template_moderation`——取消发布 → 从公开界面隐藏 + 所有者横幅 + 审计 + 通知；锁定 → 公开可见 + 变更被阻止；软删除 → 非所有者 404 + 移除通知 + 恢复倒计时；窗口内恢复 → 先前可见性 + 审计；自守卫 403；原因校验；取消发布时举报自动解析；速率限制 429。覆盖 AC-1..10。
- **单元：** 访问守卫矩阵（所有者绕过、非所有者 404、匿名）、状态转换守卫、级联服务、ticker 幂等性、服务校验矩阵。
- **E2E**（测试代理）：审核面板渲染、操作弹窗、状态筛选、所有者横幅、锁定阻止的交互、通知渲染、i18n EN/ZH 对齐。

## 12. 性能与缓存

- **NFR-1（1k 开放时 500ms p95）：** 取消发布/锁定在读取路径上立即生效——审核状态是现有 `buildTemplateConditions` WHERE 子句中的额外一列；除现有 TTL 外无需缓存失效。
- **NFR-2（可审计性）：** 每次操作写入审计行；审核历史查询复用 `ListAuditByTarget`（按 target_id 索引）。
- **NFR-6（10k 开放）：** 管理列表筛选新增 `moderation_status` 等值子句，由现有 `idx_templates_moderation_status` 索引服务；分页不变。
- **Ticker：** 30 天硬删除扫描是单个索引查询（`WHERE moderation_status = 'soft_deleted' AND deleted_at < now() - interval '30 days'`），幂等且可追赶。