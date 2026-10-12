# 模板评分 — 架构设计

| | |
|---|---|
| **Feature ID** | template-ratings |
| **Version** | 1.0 |
| **Status** | Approved for implementation |
| **Author** | Architect Agent |
| **Upstream** | [需求文档](../design/template-ratings-requirements-zh-CN.md), [UI/UX 设计](../design/template-ratings-ui-ux-design-zh-CN.md) |
| **Downstream** | [详细设计](template-ratings-detailed-design-zh-CN.md) |

## 1. 概述

模板评分为平台补上缺失的*质量*信号：市场风格的 1–5 星评分，每用户每模板一条
（upsert），事务性维护聚合值（`rating_sum` + `rating_count`；平均值始终计算
得出、绝不落库），详情页展示分布直方图，卡片展示紧凑星级行，并为
search-filtering 的排序框架新增 `top_rated` 取值。评分与点赞（欣赏）、收藏
（书签）刻意分离；三者并存。

本设计复用既有模式：`ToggleLike` 的事务性计数维护形状、Redis 固定窗口的
服务层限流、以及网关 `/api/v1/templates/{id}/{subresource}` 分支风格。

### 1.1 已决开放问题

| # | 问题（来自需求 §10） | 决策 |
|---|---|---|
| 1 | 聚合值存储 | **`rating_sum`（BIGINT）+ `rating_count`（INT），平均值计算得出。** 整数运算、无浮点漂移；平均值读取时推导（`rating_sum / rating_count`），绝不持久化。 |
| 2 | `top_rated` 索引 | **方案 (a)：接受对过滤集合的排序，外加复合索引 `templates(rating_count, rating_sum)`。** v1 不用生成列、不用物化榜单；若列表延迟恶化再评估。 |
| 3 | 分布查询 | **详情页视图对 `template_ratings` 做 `GROUP BY rating`。** 详情页流量远低于列表；五个反规范化桶不值得写入路径的成本。 |
| 4 | 服务归属 | **扩展 `PromptService`** —— 评分与点赞/收藏一样是模板子资源；仓储方法并入 `TemplateRepository`。不新建服务。 |

### 1.2 非目标

- 文字评论（comments 特性负责文字反馈）。
- 按版本评分、声誉加权、验证使用门槛。
- 评分通知（匿名信号；v1 刻意静默）。
- 匿名/游客评分；管理员评分管理或删除端点。
- 评分导出/导入。
- **任何形式的商业化** —— 支付、定价、购买、收益、销售、付费排名等均不在范围内，已从本架构中剔除。

## 2. 系统上下文

```mermaid
flowchart LR
    subgraph Client
        FE[React 前端<br/>RatingWidget / StarRating / Histogram]
    end
    subgraph Gateway
        GW[REST 网关 :8080<br/>/api/v1/templates/... /rating]
    end
    subgraph Backend
        PS[PromptService<br/>RateTemplate / DeleteRating]
        TR[TemplateRepository<br/>upsert + 聚合事务]
        AS[AdminService<br/>只读聚合值]
    end
    subgraph Stores
        PG[(PostgreSQL)]
        RD[(Redis<br/>限流计数器)]
    end
    FE -->|POST/DELETE /templates/{id}/rating| GW --> PS
    FE -->|GET /templates + sort=top_rated| GW --> PS
    FE -->|GET /api/v1/admin/templates/{id}| GW --> AS
    PS --> TR --> PG
    PS -->|rating_rate_min:{user}| RD
```

## 3. 涉及组件

| 组件 | 变更 |
|---|---|
| `backend/api/proto/v1/prompt.proto` | `PromptService` 新增 `RateTemplate`/`DeleteRating` RPC；`Template` 增加 `rating_avg`/`rating_count`/`my_rating`；`GetTemplateResponse` 增加 `rating_distribution`。 |
| `backend/internal/repository/template_repository.go` | `RateTemplate`（upsert + 聚合事务）、`DeleteRating`、`GetRatingDistribution`；`Get`/`List` 中 `my_rating` LEFT JOIN；`top_rated` 排序分支。 |
| `backend/internal/service/prompt.go` | RPC 实现：校验、作者/封禁/可见性检查、Redis 限流。 |
| `backend/internal/service/admin.go` + `admin.proto` | `AdminTemplateSummary` 增加只读 `rating_avg`/`rating_count`。 |
| `backend/cmd/server/main.go` | `/api/v1/templates/` 处理器中的 `/rating` 分支（POST + DELETE）。 |
| `frontend/src/components/{StarRating,RatingWidget,RatingHistogram}.js` | **新增。** |
| `frontend/src/components/PromptCard.js`、`pages/TemplateDetails.js`、管理端模板详情 | 星级行、评分区块、管理端只读行。 |
| `deployment/init.sql`（+ chart 副本） | `template_ratings` 表、`templates.rating_sum`/`rating_count` ALTER、复合索引。 |

## 4. API 设计

### 4.1 评分动作

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| `POST` | `/api/v1/templates/{id}/rating` | 必需 | 请求体 `{rating: 1..5}`。Upsert 调用者的评分。返回 `{rating_avg, rating_count, my_rating}`。 |
| `DELETE` | `/api/v1/templates/{id}/rating` | 必需 | 删除调用者的评分。幂等 —— 删除不存在的评分返回 `200` 与当前聚合值、`my_rating: 0`。 |

规则：

- **作者自评** → `403 PERMISSION_DENIED`（"cannot rate your own template"）。
- **被封禁调用者** → `403`。
- **私有模板、非作者调用者** → `404 NOT_FOUND`（不暴露模板存在性；与
  `GetTemplate` 可见性语义一致）。作者本人被自评规则阻止，因此实际上无人能
  为私有模板评分。
- **无效评分**（0、6、非整数、3.5）→ `400 INVALID_ARGUMENT`。
- **限流**：每账号每分钟 20 次评分变更 → `429 RESOURCE_EXHAUSTED`，附
  `Retry-After`。可配置（`RATING_RATE_PER_MINUTE`）。

### 4.2 读取（无新端点）

- 模板**列表**项增加 `rating_avg`（浮点，`rating_count = 0` 时为 `0` —— 由
  count 表达"暂无评分"）、`rating_count`、`my_rating`（`0` = 未评分；评分
  为 1–5，0 无歧义）。
- 模板**详情**额外在 `GetTemplateResponse` 上增加 `rating_distribution`
  （五个计数，索引 0 → 1★ … 索引 4 → 5★）。
- 任何地方都不存在公开的按评分者数据；`my_rating` 仅返回给其所有者（匿名
  调用者得到 `0`）。

### 4.3 排序

search-filtering 排序框架新增 `top_rated`：`rating_count >= threshold`
（默认 5，可配置）的模板按平均分 DESC、再 `rating_count` DESC、再
`created_at` DESC 排前；低于阈值的模板随后，按 `rating_count` DESC、
`created_at` DESC 排序。若 search-filtering 尚未发布，该排序取值等待 ——
评分数据与展示本身即独立有价值（需求 D-2）。

### 4.4 管理端（只读）

`GET /api/v1/admin/templates/{id}`（及管理端模板列表）增加只读的
`rating_avg` 与 `rating_count`。不存在管理端评分端点；尝试访问命中网关的
404/405 处理。

### 4.5 错误码

| HTTP | gRPC | 条件 |
|---|---|---|
| 400 | `INVALID_ARGUMENT` | 评分超出 1–5、非整数。 |
| 401 | `UNAUTHENTICATED` | 缺少/无效令牌。 |
| 403 | `PERMISSION_DENIED` | 作者自评；被封禁调用者。 |
| 404 | `NOT_FOUND` | 模板不存在或对他人的私有模板。 |
| 429 | `RESOURCE_EXHAUSTED` | 超出限流（`Retry-After`）。 |

## 5. 数据模型

### 5.1 新表：`template_ratings`

```sql
CREATE TABLE IF NOT EXISTS template_ratings (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    rating SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, template_id)
);
CREATE INDEX IF NOT EXISTS idx_template_ratings_template ON template_ratings(template_id);
```

### 5.2 `templates` ALTER（幂等，两份 init.sql 副本）

```sql
ALTER TABLE templates ADD COLUMN IF NOT EXISTS rating_sum BIGINT NOT NULL DEFAULT 0;
ALTER TABLE templates ADD COLUMN IF NOT EXISTS rating_count INT NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_templates_top_rated ON templates(rating_count, rating_sum);
```

### 5.3 聚合维护

每次评分变更在单事务中执行（`ToggleLike` 形状）：读取调用者当前行，对
`rating_sum`/`rating_count` 应用增量（`GREATEST(...)` 下限防漂移），返回
最新聚合值。模板删除随行级联删除评分。用户删除级联其评分行；任何未来的
用户删除端点必须在同一事务中重算受影响模板的聚合值（SQL 见详细设计）。
封禁**不**删除评分（FR-14）。

## 6. 鉴权与授权

- 写操作需要会话/API-key 鉴权；用户 id 取自上下文，绝不取自请求体。
- 服务端校验：整数 1–5、模板存在性、作者检查、封禁检查、私有可见性检查
  —— 全部先于写入。
- `PromptService` 内通过 Redis 固定窗口（`rating_rate_min:{user_id}`）限流，
  Redis 故障时 fail-open，仅成功变更消耗配额。
- `PromptService` 新增 `RedisStore` 参数（nil 安全 —— 为 nil 时禁用限流）。
  **协同说明**：template-comments 设计新增同一参数；谁先落地谁引入，另一方
  复用。

## 7. 前端

- **StarRating**（展示）：只读星星 + 数值 + 计数；卡片尺寸在
  `rating_count = 0` 时整行省略（绝不显示 "0.0"）。
- **RatingWidget**（交互，详情页）：悬停预览、点击提交、键盘 slider 模式
  （单 tab 停靠、←/→ 调整、Enter/Space 提交、Esc 取消）、`my_rating` 状态
  与 "Clear rating" 动作、乐观填充出错回滚；作者视图显示不可交互提示；
  匿名点击路由到 `/login`。
- **RatingHistogram**：五行（星级、条、百分比文本）；语义列表、条为装饰；
  零评分时隐藏。
- **PromptCard**：标题/元信息下方的紧凑 "★ 4.6 (128)" 行。
- **排序下拉**（search-filtering 组件）：`top_rated` 选项与阈值提示。
- **管理端模板详情**：只读 "Rating: 4.62 (128)" 行。
- **i18n**：`ratings.*` 命名空间 + `sort.top_rated`；EN/ZH 对齐；星级提示
  本地化（1 Poor … 5 Excellent / 很差…很好）；平均值始终西文数字。

## 8. 配置

| 变量 | 默认值 | 说明 |
|---|---|---|
| `RATING_RATE_PER_MINUTE` | `20` | 每账号每分钟评分变更次数。 |
| `TOP_RATED_MIN_RATINGS` | `5` | `top_rated` 排序阈值的最低 `rating_count`。 |

两者加入 `deployment/docker-compose.yml` 与 Helm chart（`values.yaml` +
`templates/deployment-backend.yaml`），与 `NOTIFICATION_*` 一致。

## 9. 安全考量

- **隐私**：任何端点都不揭示谁评了什么；仅评分者本人可见自己的
  `my_rating`。
- **完整性**：PK 强制每 (user, template) 一行；聚合值事务性维护；upsert
  语义使客户端重试安全。
- **滥用**：按账号限流；阻止作者自评；阻止封禁用户；服务端校验 1–5 域。
- **注入**：仅参数化 SQL。

## 10. 部署说明

- DDL 为增量且幂等；两份 init.sql 副本在同一提交中更新。
- 无新服务或基础设施；新增一个 Redis 计数器键。
- `backend/server` 是被跟踪的构建产物 —— 绝不提交其变更。

## 11. 测试策略

- **FVT**（`tests/fvt.py`）：`test_template_ratings` —— 评分/重评/删除与
  聚合断言；作者 403；封禁 403；无效值 400；私有模板对非作者 404；限流
  429；列表/详情中的 `my_rating`；分布正确性；管理端只读字段；幂等删除。
- **单元测试**：仓储增量数学（插入/更新/删除路径、下限）、服务校验矩阵、
  限流窗口行为。
- **E2E**（test-agent）：组件交互（悬停/点击/键盘）、直方图渲染、零评分时
  卡片行省略、排序选项、i18n 对齐。
