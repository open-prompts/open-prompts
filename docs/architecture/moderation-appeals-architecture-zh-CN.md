# 审核申诉 — 架构设计

| | |
|---|---|
| **功能 ID** | moderation-appeals |
| **版本** | 1.0 |
| **状态** | 已批准实施 |
| **作者** | Architect Agent |
| **上游** | [需求](../design/moderation-appeals-requirements-zh-CN.md), [UI/UX 设计](../design/moderation-appeals-ui-ux-design-zh-CN.md) |
| **下游** | [详细设计](moderation-appeals-detailed-design-zh-CN.md) |

## 1. 概述

审核申诉闭环了 `admin-template-moderation` 遗留的带外流程：模板被操作（下架、锁定或软删除）的创作者可以在产品内以结构化、可追踪的申诉来质疑该决定，管理员可以处理 FIFO 队列并以必填的裁决说明来反转或维持。该设计遵循 YouTube 模式 —— **每次审核操作仅一次申诉**、单一案情陈述（无线程对话）、明确的状态（`open` → `under_review` → `resolved_upheld` | `resolved_reversed`）。

该功能引入一张专门的 **`moderation_actions`** 表（OQ-1），以稳定 id 记录每次审核操作。申诉引用这些操作 id，为 FR-14「已取代」状态提供清晰的生命周期（被后续直接管理员恢复撤销的操作不能再被申诉）。由于 `admin-template-moderation` 是尚未实现的积压功能，本设计定义了 `moderation_actions` 表及其写入路径，供 `admin-template-moderation` 落地时填充；申诉功能本身只读取它。反转补救（FR-7）复用 `admin-template-moderation` FR-5 定义的恢复语义。

### 1.1 已解决的开放问题

| # | 问题（来自需求 §10） | 决定 |
|---|---|---|
| 1 | 引用审计日志条目 id 还是专门的 `moderation_actions` 表 | **专门的 `moderation_actions` 表。** 审计条目是追加式且不可变的；申诉需要可变的生命周期（`active` → `superseded`/`reversed`），审计行无法表达。该表在此定义，由 `admin-template-moderation` 落地时填充。 |
| 2 | `under_review` 是否必须在裁决前经过 | **否 —— 管理员可直接从 `open` 裁决。** 对小团队两状态是仪式；`under_review` 转换仍作为可选的分诊步骤保留（FR-6）。 |
| 3 | 申诉反转是否需要独立审计代码 vs 复用 `template.restore` | **复用 `template.restore`，并在原因中携带申诉引用。** 恢复路径相同；独立代码会重复补救。审计原因携带 `appeal=<id>` 以便追溯反转。 |
| 4 | 5 个 open 申诉限额是否可配置 | **是 —— 服务端环境变量 `APPEAL_MAX_OPEN`（默认 5）。** 镜像 `RATING_RATE_PER_MINUTE` 配置模式；v1 无需 platform_settings 行。 |
| 5 | 通知说明摘录长度 | **通知中 140 字符摘录，外加指向申诉详情的深链。** 摘录提供即时上下文；深链是事实来源。 |

### 1.2 非目标

- 多轮对话 / 线程化往返（一条陈述 + 一条说明）。
- 附件 / 证据上传；第三方仲裁；SLA 计时器或自动升级。
- 账户级操作申诉（封禁 —— 属 `admin-user-management` 范畴）。
- 评论删除申诉（仅模板操作）。
- 管理员审核操作本身（下架/锁定/软删除）—— 属 `admin-template-moderation`；本功能只读取它们。
- **任何形式的变现** —— 支付、定价、购买、收益、销售或付费排名界面均不在范围内，并排除在本架构之外。

## 2. 系统上下文

```mermaid
flowchart LR
    subgraph Client
        FE[React 前端<br/>AppealCtaSlot / AppealFormModal / UserAppealList]
        ADM[管理控制台<br/>AdminAppealQueue / AdminAppealDetail]
    end
    subgraph Gateway
        GW[REST 网关 :8080<br/>/api/v1/appeals... /api/v1/admin/appeals...]
    end
    subgraph Backend
        PS[PromptService<br/>CreateAppeal / ListAppeals / GetAppeal]
        AS[AdminService<br/>ListAppealQueue / ReviewAppeal / ResolveAppeal]
        AR[AppealRepository<br/>appeals + moderation_actions 读取]
        NR[NotificationRepository<br/>appeal_resolved]
    end
    subgraph Stores
        PG[(PostgreSQL)]
        RD[(Redis<br/>open-appeal 速率计数器)]
    end
    FE -->|POST/GET /api/v1/appeals| GW --> PS
    ADM -->|GET/POST /api/v1/admin/appeals| GW --> AS
    PS --> AR --> PG
    AS --> AR --> PG
    AS --> NR --> PG
    PS -->|appeal_open:{user}| RD
```

## 3. 涉及的组件

| 组件 | 变更 |
|---|---|
| `backend/api/proto/v1/prompt.proto` | `PromptService` 上新增 `CreateAppeal`/`ListAppeals`/`GetAppeal` RPC；`Appeal` 消息。 |
| `backend/api/proto/v1/admin.proto` | `AdminService` 上新增 `ListAppealQueue`/`GetAppeal`/`ReviewAppeal`/`ResolveAppeal` RPC；`AdminAppealEntry`/`AdminAppealDetail` 消息。 |
| `backend/internal/models/appeal.go` | **新增** —— `ModerationAction` 和 `Appeal` GORM 模型。 |
| `backend/internal/repository/appeal_repository.go` | **新增** —— 申诉 CRUD、队列查询、审核操作读取、原子裁决+恢复。 |
| `backend/internal/service/prompt.go` | `CreateAppeal`/`ListAppeals`/`GetAppeal` 实现（所有者 + 可见性检查、速率限制）。 |
| `backend/internal/service/admin.go` | `ListAppealQueue`/`GetAppeal`/`ReviewAppeal`/`ResolveAppeal` 实现（自我守卫、审计、通知）。 |
| `backend/cmd/server/main.go` | `/api/v1/appeals` 和 `/api/v1/appeals/{id}` 分支；`/api/v1/admin/appeals` 和 `/api/v1/admin/appeals/{id}` 分支。 |
| `frontend/src/components/AppealCtaSlot.js`、`AppealFormModal.js`、`AppealStateChip.js` | **新增。** |
| `frontend/src/pages/AppealsPage.js`、`frontend/src/pages/AppealDetailPage.js` | **新增** 用户路由。 |
| `frontend/src/pages/admin/AppealsPage.js`、`frontend/src/pages/admin/AppealDetailPage.js` | **新增** 管理路由。 |
| `frontend/src/services/api.js` | 申诉 API 客户端方法。 |
| `frontend/src/locales/{en,zh}/translation.json` | `appeal.*` 命名空间。 |
| `deployment/init.sql`（+ chart 副本） | `moderation_actions` + `appeals` 表、审计/通知 CHECK 扩展。 |

## 4. API 设计

### 4.1 用户申诉 API

| 方法 | 路径 | 认证 | 描述 |
|---|---|---|---|
| `POST` | `/api/v1/appeals` | 必需 | 请求体 `{moderation_action_id, statement}`。创建 `open` 申诉。返回创建的申诉。 |
| `GET` | `/api/v1/appeals` | 必需 | 调用者自己的申诉，每页 20 条，最新优先。 |
| `GET` | `/api/v1/appeals/{id}` | 必需 | 单条申诉（仅所有者）。 |

规则：

- **未认证** → `401 UNAUTHENTICATED`。
- **操作不存在 / 非调用者模板** → `404 NOT_FOUND`（不向非所有者泄露操作存在性）。
- **操作已被申诉** → `409 ALREADY_EXISTS`（「该操作已被申诉」）。
- **操作已取代**（被后续直接管理员恢复撤销）→ `409 ALREADY_EXISTS` 并带「已取代」消息；UI 隐藏入口。
- **陈述超出 50–2000 字符** → `400 INVALID_ARGUMENT`。
- **达到 open 申诉限额**（默认 5）→ `429 RESOURCE_EXHAUSTED` 并带 `Retry-After` 和限额消息。
- **申诉非调用者所有** → `404 NOT_FOUND`（无存在性泄露）。

### 4.2 管理员申诉 API

| 方法 | 路径 | 认证 | 描述 |
|---|---|---|---|
| `GET` | `/api/v1/admin/appeals?state=open\|under_review\|resolved&page=...` | 管理员 | 队列，状态内最早优先，带各状态计数。 |
| `GET` | `/api/v1/admin/appeals/{id}` | 管理员 | 申诉详情 + 原操作的审计上下文。 |
| `POST` | `/api/v1/admin/appeals/{id}/review` | 管理员 | `open` → `under_review`。 |
| `POST` | `/api/v1/admin/appeals/{id}/resolve` | 管理员 | 请求体 `{outcome: upheld\|reversed, note}`。裁决申诉；reversed 原子恢复模板。 |

规则：

- **非管理员** → `403 PERMISSION_DENIED`。
- **裁决者是原操作的操作者** → `403 PERMISSION_DENIED`（「原操作的操作者不能裁决此申诉」）。
- **说明超出 20–1000 字符** → `400 INVALID_ARGUMENT`。
- **申诉已裁决** → `409 ALREADY_EXISTS`。
- **对已取代操作裁决** → `409 ALREADY_EXISTS`（操作已被撤销；无可反转）。

### 4.3 错误码

| HTTP | gRPC | 条件 |
|---|---|---|
| 400 | `INVALID_ARGUMENT` | 陈述/说明长度违规；无效结果。 |
| 401 | `UNAUTHENTICATED` | 缺失/无效令牌。 |
| 403 | `PERMISSION_DENIED` | 非管理员；裁决者是原操作者。 |
| 404 | `NOT_FOUND` | 操作/申诉不存在或非所有。 |
| 409 | `ALREADY_EXISTS` | 操作已被申诉；申诉已裁决；操作已取代。 |
| 429 | `RESOURCE_EXHAUSTED` | 达到 open 申诉限额（`Retry-After`）。 |

## 5. 数据模型

### 5.1 新表：`moderation_actions`

```sql
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
CREATE INDEX IF NOT EXISTS idx_moderation_actions_template
    ON moderation_actions (template_id);
CREATE INDEX IF NOT EXISTS idx_moderation_actions_status_created
    ON moderation_actions (status, created_at);
```

`status` 生命周期：`active`（可申诉）→ `superseded`（被后续直接管理员恢复撤销，不再可申诉）或 `reversed`（被申诉裁决撤销）。`admin-template-moderation` 在下架/锁定/软删除时写入行，并在直接恢复时将 `active` → `superseded`。

### 5.2 新表：`appeals`

```sql
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
CREATE INDEX IF NOT EXISTS idx_appeals_appellant_created
    ON appeals (appellant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_appeals_state_created
    ON appeals (state, created_at);
```

`UNIQUE (moderation_action_id)` 在数据库层面强制 FR-3（每次操作仅一次申诉）。`state` 转换除初始创建外仅管理员可操作。

### 5.3 CHECK 扩展（幂等，两个 init.sql 副本）

- `admin_audit_logs.action` 增加 `appeal.create`、`appeal.review`、`appeal.resolve`。
- `notifications.type` 增加 `appeal_resolved`。

## 6. 认证与授权

- **用户写入**需要会话/API 密钥认证；用户 id 来自上下文，绝不来自请求体。申诉者必须拥有操作所针对的模板。
- **管理员写入**需要管理员 JWT（`requireAdminContext`）。裁决者不得是原操作的操作者（FR-9，自我守卫）。
- **可见性**：申诉和操作绝不向非所有者 / 非管理员泄露（外部资源返回 404）。
- **速率限制**：open 申诉创建限制为每用户 `APPEAL_MAX_OPEN`（5）个并发 open 申诉，通过 Redis 计数器（`appeal_open:{user_id}`）强制，裁决时递减。Redis 出错时 fail-open。

## 7. 前端

- **AppealCtaSlot**（所有者横幅 / 移除通知）：无申诉时显示 CTA，open/under_review 时显示状态徽章，已裁决时显示结果徽章 + 说明摘录；操作已取代时隐藏。
- **AppealFormModal**：只读操作上下文 + 陈述文本域（50–2000）带实时计数；409 → 替换为现有申诉的状态卡片。
- **AppealStateChip**：open / under review / upheld / reversed 带文本标签。
- **AppealsPage**（`/appeals`）：调用者的申诉，带状态徽章、操作上下文、时间戳；已裁决时显示裁决说明摘录。
- **AppealDetailPage**（`/appeals/:id`）：陈述 + 操作上下文 + 裁决块。
- **AdminAppealsPage**（`/admin/appeals`）：状态筛选标签带计数、最早优先行、陈述摘录。
- **AdminAppealDetailPage**（`/admin/appeals/:id`）：双栏申诉 + 审计条目；Review 按钮；Resolve 表单（结果单选 + 必填说明）带自我守卫禁用状态。
- **i18n**：`appeal.*` 命名空间；按 UI/UX 文档 §8 保持 EN/ZH 对等。

## 8. 配置

| 变量 | 默认 | 描述 |
|---|---|---|
| `APPEAL_MAX_OPEN` | `5` | 每用户最大并发 open 申诉数。 |

添加到 `deployment/docker-compose.yml` 和 Helm chart（`values.yaml` + `templates/deployment-backend.yaml`），镜像 `RATING_RATE_PER_MINUTE`。

## 9. 安全考虑

- **隐私**：申诉和操作仅所有者/管理员可见；对他人无存在性泄露（404）。
- **完整性**：`UNIQUE (moderation_action_id)` 强制每次操作仅一次申诉；裁决+恢复在同一事务中原子完成；状态转换在服务端校验。
- **滥用**：open 申诉速率限制；自我守卫防止自我审查；纯文本陈述/说明（无 HTML/Markdown）以文本渲染（构造上 XSS 安全，NFR-5）。
- **注入**：仅参数化 SQL。

## 10. 部署说明

- DDL 是增量且幂等的；两个 init.sql 副本在同一提交中更新。
- 无新服务或基础设施；一个新 Redis 计数器键。
- `backend/server` 是受跟踪的构建产物 —— 绝不提交对其的更改。

## 11. 测试策略

- **FVT**（`tests/fvt.py`）：`test_moderation_appeals` —— 创建/列表/获取、每次操作仅一次申诉 409、已取代 409、陈述校验 400、open 限额 429、管理员队列排序 + 计数、review 转换、resolve upheld/reversed、reversed 原子恢复模板、自我守卫 403、非管理员 403、裁决时通知、审计条目。
- **单元**：仓库状态转换矩阵、裁决+恢复事务、速率限制计数器、服务校验 + 自我守卫。
- **E2E**（test-agent）：表单模态流程、409 → 状态卡片、队列标签、裁决确认、自我守卫禁用状态、i18n 对等。