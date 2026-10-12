# Webhooks API 集成（Webhooks API Integration）— 需求分析

- **Feature ID:** webhooks-api-integration
- **Version:** 1.0
- **Status:** Proposed（待架构评审）
- **Author:** ui-ux-agent
- **Date:** 2026-10-12
- **相关文档：** [UI/UX 设计](./webhooks-api-integration-ui-ux-design-zh-CN.md) · [Requirements (EN)](./webhooks-api-integration-requirements.md)

## 1. 竞品调研总结

| 平台 | Webhook 方案 | 关键启示 |
|---|---|---|
| **GitHub** | HMAC-SHA256 签名（`X-Hub-Signature-256`）；按资源划分的丰富事件目录；投递日志含响应状态/响应体和**重新投递**按钮；创建时触发 `ping` 事件 | 带重新投递的投递日志才是集成者真正使用的调试界面；创建即 ping 提供了即时的端到端验证 |
| **Stripe** | `Stripe-Signature: t=<unix>,v1=<hmac>` —— 时间戳绑定进签名可防重放；指数退避重试最长 3 天；端点持续失败后**自动停用**并邮件告警；仪表盘"发送测试 webhook"带真实样例负载 | 时间戳进签名是现代方案；自动停用保护双方，避免死端点永远烧重试 |
| **Shopify** | 通过 API（REST/GraphQL）管理 webhook 订阅，点分主题（`products/update`）；每个端点一个 HMAC 共享密钥 | 点分式 `资源.动作` 事件命名扩展性好，在 UI 复选框里也易读 |
| **Slack / Discord 入站 webhook** | 消费侧极简：向 URL POST JSON 即可 | 现实中最主流的消费者是聊天频道 —— 创作者想要"有人 fork 我的提示词时发到我的 Discord"，零代码 |
| **Zapier / Make** | Webhook 触发器是无代码自动化的通用入口 | 目标用户是创作者而非基础设施工程师：管理 UI 必须简单到"粘贴 URL + 勾选框" |
| **PromptBase / FlowGPT / Snack Prompt** | 无 webhook，无公开 API | 模式必须来自开发者平台 —— 与 api-usage-history、rate-limit-dashboard 结论一致 |

**综合分析：** Open Prompts 目前只有拉取（pull）。创作者有 API key（以及设计中的 playground 和用量历史），但所有信号 —— 模板被点赞、收藏、fork、评论，或模板生命周期变化 —— 只会出现在应用内通知铃铛里。api-key-scopes 设计明确把"webhook 签名密钥（webhooks-api-integration 负责该领域）"留给本功能。本功能新增按创建者范围的出站 webhook：7 类事件（4 类互动 + 3 类生命周期）挂接到与站内通知完全相同的代码路径（`createEngagementNotification` 调用点），Stripe 风格的签名负载（`t=…,sha256=…`，覆盖时间戳 + 请求体），尽力而为的异步投递带有限次数的指数退避重试，完整的按 webhook 投递日志（支持重新投递和测试 ping），SSRF 加固的端点校验，以及一个遵循现有 Aurora 设计语言的 `/webhooks` 管理页面。

## 2. 问题陈述

1. **互动在应用外不可见。** 模板被 fork 40 次的创作者只有打开网站才能知道。没有任何推送到聊天频道、仪表盘或自动化流程的通道。
2. **集成只能轮询。** 任何基于 API 构建的东西（用量跟踪、镜像、对提示词变更的 CI 检查）都必须轮询列表端点 —— 对客户端和 API 都是浪费（而且现在已有限流）。
3. **生命周期变化没有观察者。** 模板创建/更新/删除（包括带变更说明的新版本）今天无法驱动任何下游自动化。
4. **API 面没有事件契约。** api-key-scopes 刻意把签名密钥推迟到这里；没有签名、带版本的负载契约，任何消费者都只能爬取或轮询。
5. **没有调试界面。** 即使投递存在，集成者也需要 GitHub 式的投递日志（状态、响应码、尝试次数、重新投递）来回答"我的端点收到没有？它返回了什么？"

## 3. 目标用户

- **做自动化的创作者：** 把点赞/fork/评论推送到 Slack/Discord/Zapier，不用打开网站就能实时感知热度。
- **API 集成者：** 用模板生命周期事件驱动镜像、分析管道或 CI 检查，替代轮询。
- **团队协作者：** 团队成员发布或更新提示词模板时让共享频道保持知情。
- **管理员：** v1 无专门管理界面（见 §8）；webhook 滥用通过按用户上限和 SSRF 规则约束。

## 4. 用户故事

- **US-1:** 作为创作者，我希望有人点赞、收藏、fork 或评论我的模板时 webhook 能推送到我的 Discord 频道，这样不用轮询网站就能实时感受热度。
- **US-2:** 作为创作者，我希望能精确选择一个 webhook 接收哪些事件（例如只收 fork 和评论），这样我的频道保持信号而非噪音。
- **US-3:** 作为集成者，我希望每次投递都带 HMAC 密钥和时间戳签名，这样我能验证负载确实来自 Open Prompts 且未被重放。
- **US-4:** 作为集成者，我希望签名密钥只在创建（和轮换）时展示一次，这样数据库泄露也无法伪造发往我端点的投递。
- **US-5:** 作为集成者，我希望失败的投递按递增间隔重试、之后标记为永久失败，这样瞬时故障不丢事件，死端点也不会永远重试。
- **US-6:** 作为集成者，我希望每个 webhook 有投递日志（事件、状态、响应码、尝试次数、时间戳），这样我能调试端点返回了什么。
- **US-7:** 作为集成者，我希望日志里每条投递都有"重新投递"按钮，这样端点在部署期间丢掉的事件可以重放。
- **US-8:** 作为集成者，我希望有"发送测试事件"按钮，这样在等待真实活动之前就能端到端验证端点。
- **US-9:** 作为创作者，我希望 webhook 在反复失败后自动暂停（带醒目横幅），这样我能注意到端点坏了，而不是默默丢事件。
- **US-10:** 作为用户，我希望 webhook URL 限制为公网 HTTPS 端点，这样平台不能被用来探测内网（SSRF）。
- **US-11:** 作为创作者，我希望 webhook 只作用于我自己的模板，这样其他用户的活动永远不会到达我的端点。
- **US-12:** 作为用户，我希望 webhook 管理 UI 完整支持中英文，这样语言永远不是自动化的障碍。

## 5. 功能需求

### 5.1 订阅管理

- **FR-1:** 新增 `webhooks` 表：`id UUID PK`、`user_id TEXT FK users(id) ON DELETE CASCADE`、`name VARCHAR(100) NOT NULL`、`url TEXT NOT NULL`、`secret TEXT NOT NULL`、`events TEXT[] NOT NULL`、`is_active BOOLEAN NOT NULL DEFAULT TRUE`、`disabled_reason VARCHAR(200)`、`consecutive_failures INT NOT NULL DEFAULT 0`、`created_at/updated_at TIMESTAMPTZ`。REST CRUD（会话认证、按所有者隔离）：`GET /api/v1/webhooks`、`POST /api/v1/webhooks`、`PATCH /api/v1/webhooks/{id}`、`DELETE /api/v1/webhooks/{id}`。硬上限：**每用户 5 个 webhook**（带友好提示的 409 类错误）。URL 必须解析为带主机名的 `https://`；名称 1–100 字符。
- **FR-2:** 事件目录（点分 `资源.动作` 约定），全部**按所有者范围** —— webhook 只在其所有者的模板上触发事件；自我互动（所有者点赞/收藏自己的模板）被抑制，与 `createEngagementNotification` 一致：
  - `template.created` —— 数据：`template_id, title, visibility, category, tags[]`
  - `template.updated` —— 创建了新版本 —— 数据：`template_id, title, version, change_note`
  - `template.deleted` —— 数据：`template_id, title`
  - `template.like` / `template.favorite` —— 数据：`template_id, title, actor_id, actor_display_name, action`（`"added"` | `"removed"`）
  - `template.fork` —— 数据：`template_id, title, actor_id, actor_display_name, fork_template_id, fork_title`
  - `template.comment` —— 数据：`template_id, title, actor_id, actor_display_name, comment_id, excerpt`（前 140 字符）
  - `webhook.test` —— 合成事件，仅通过测试按钮触发 —— 数据：`message, sent_at`
- **FR-3:** 签名密钥由服务端生成（`whsec_` + 32 位十六进制字符），存储在服务端（计算签名所需；`GET` 永不返回），只在 `POST`/轮换的响应体中**展示一次**。`POST /api/v1/webhooks/{id}/rotate-secret` 使旧密钥失效并一次性返回新密钥。UI 显示一次性弹窗，带复制按钮和"不再显示"警告。

### 5.2 投递引擎

- **FR-4:** 负载信封（JSON、UTF-8、≤ 32 KB）：`{ "id": "<事件 UUID>", "type": "template.fork", "created_at": "<RFC3339>", "data": { … } }`。请求头：`Content-Type: application/json`、`X-OpenPrompts-Event: <类型>`、`X-OpenPrompts-Delivery: <投递 UUID>`、`X-OpenPrompts-Signature: t=<unix>,sha256=<hex>`，其中 HMAC-SHA256 以 webhook 密钥对 `"<t>." + 原始请求体` 计算（Stripe 风格；支持重放检查：`|now − t| > 5 分钟` 即拒绝）。
- **FR-5:** 分发**尽力而为且不占热路径**：触发写入提交后，由 goroutine（与 `createEngagementNotification` 和异步 `UpdateLastUsed` 相同的模式）把事件扇出给该所有者所有订阅此类型的活跃 webhook。分发失败绝不导致或拖延用户操作。HTTP 客户端：5 秒超时、**不**跟随重定向、成功 = 任意 2xx。
- **FR-6:** 每次投递的重试计划：首次尝试，然后 +1 分钟、+5 分钟、+30 分钟、+2 小时、+6 小时（共 5 次尝试，跨度约 8.5 小时）。非 2xx、超时或连接错误 = 尝试失败 → 安排下一次。第 5 次失败后投递标记为 `failed_permanent`。调度器（ticker，每 30 秒）拾起到期的重试；v1 仅进程内（无外部队列）。
- **FR-7:** 每次尝试记录到 `webhook_deliveries`：`id UUID PK`、`webhook_id UUID FK CASCADE`、`event_id UUID`、`event_type TEXT`、`payload JSONB`、`status`（`pending|success|failed_permanent`）、`attempts INT`、`last_response_status INT`、`last_error VARCHAR(200)`、`next_retry_at TIMESTAMPTZ`、`delivered_at TIMESTAMPTZ`、`is_redelivery BOOLEAN DEFAULT FALSE`、`created_at`。保留策略：超过 30 天的行由同一调度器删除；UI 显示每个 webhook 最近 50 条。
- **FR-8:** **SSRF 防护**，在创建/更新时*以及*每次分发时重新检查（DNS 可能变化）：URL scheme 为 `https`；解析出的 IP 必须为公网（拒绝 `127.0.0.0/8`、`10.0.0.0/8`、`172.16.0.0/12`、`192.168.0.0/16`、`169.254.0.0/16`、`0.0.0.0/8`、`100.64.0.0/10`、`::1/128`、`fe80::/10`、`fc00::/7`）；不跟随重定向；最多读取 512 字节响应体（错误摘要）且未经转义绝不原样回显到 UI。

### 5.3 用户界面

- **FR-9:** 新页面 `/webhooks`（会话认证；Header 导航入口与 API Keys 并列）：用户 webhook 表格 —— 名称、URL（主机 + 截断路径）、事件徽章、启用开关、最近投递结果、创建日期；行操作：编辑、轮换密钥、删除（确认弹窗）。创建/编辑弹窗：名称、负载 URL（带 HTTPS 提示）、按"模板生命周期"/"互动"分组的事件复选框矩阵、内联校验（URL 格式、≥ 1 个事件、达到上限）。
- **FR-10:** 每个 webhook 的**投递历史**面板（可展开行或侧边抽屉）：最近 50 条投递，含事件类型、状态标签（已送达 / 重试中 / 失败）、响应码、尝试次数、时间戳；每行**重新投递**操作（`POST /api/v1/webhooks/{id}/deliveries/{delivery_id}/redeliver` —— 以相同负载创建新投递，`is_redelivery = true`，立即分发）和**发送测试事件**按钮（`POST /api/v1/webhooks/{id}/test` —— 立即分发 `webhook.test`）。
- **FR-11:** **自动暂停：** 当 `consecutive_failures` 达到 20（跨事件计数，任何成功即重置）时，webhook 被置为 inactive，`disabled_reason = 'auto: consecutive failures'`。`/webhooks` 页在自动暂停的行上显示醒目横幅（"因连续投递失败已暂停 —— 请检查端点后重新启用"）；一键重新启用并重置计数器。v1 不发邮件/站内推送（notifications 表是互动形状的；见 §8）。
- **FR-12:** 所有管理界面字符串通过 react-i18next 提供中英文（新 `webhooks` 命名空间）；API 错误体保持机器可读的英文（API 契约，非 UI）。

## 6. 非功能需求

- **NFR-1（性能）：** 事件分发不增加请求路径开销 —— 扇出在响应提交后的 goroutine 中进行；HTTP 客户端强制 5 秒超时，goroutine 不会因端点挂起而堆积。调度器 tick 通过 `next_retry_at < now() AND status = 'pending'` 索引扫描，复杂度为 O(到期重试数)。
- **NFR-2（可靠性）：** 投递在重试窗口内是至少一次（重新投递或"发送后记录前崩溃"可能产生重复）；消费者按信封 `id` 去重。分发 panic 会被 recover 并记录；绝不拖垮服务器进程。
- **NFR-3（安全）：** 密钥存储在服务端（签名所需），一次性展示后永不返回，并排除在所有日志之外。SSRF 规则（FR-8）在订阅*和*分发时都执行。负载只含行动者显示名和 ID —— 绝不含邮箱或其他用户的私有数据。Webhook CRUD 按所有者隔离；ID 为不可猜测的 UUID。
- **NFR-4（一致性）：** 互动事件挂接与站内通知相同的调用点（`prompt.go` 的 fork/like/favorite、`comment.go` 的 comment），两个渠道对"事件是否发生"绝不可能不一致。生命周期事件在提交后挂接 `CreateTemplate` / `UpdateTemplate`（新版本分支）/ `DeleteTemplate`。
- **NFR-5（可观测性）：** 每次尝试记录结构化日志（webhook ID、事件类型、尝试次数、结果、响应状态、耗时），级别 info/warn。投递日志是同一数据的用户侧镜像。
- **NFR-6（国际化）：** `/webhooks` 页面、弹窗、横幅和投递面板完整翻译（中英文）；事件类型名和负载字段保持英文（面向开发者的契约）。

## 7. 验收标准

- **AC-1:** 用户创建 URL 为 `https://example.com/hook`、事件为 `[template.fork, template.comment]` 的 webhook；另一用户 fork 其模板；数秒内 URL 收到 `POST`，带 `X-OpenPrompts-Event: template.fork`、对 `t + "." + body` 有效的 `t=…,sha256=…` 签名和 fork 负载 —— 且 fork 操作的响应时间不变。
- **AC-2:** 签名密钥出现在创建响应和一次性 UI 弹窗中；此后每次 `GET /api/v1/webhooks` 返回的 webhook **不含**密钥；轮换后旧密钥验证失败、新密钥通过。
- **AC-3:** webhook 未订阅的事件（如 `template.like`）不触发投递；**其他用户**模板上的事件绝不触发投递到该 webhook。
- **AC-4:** 返回 500 的端点按约 +1 分钟、+5 分钟、+30 分钟、+2 小时、+6 小时重试；投递日志显示 `attempts: 5`、`status: failed_permanent` 及记录的响应码；第 2 次尝试返回 200 的端点将投递标记为 `success` 并重置 `consecutive_failures`。
- **AC-5:** 创建第 6 个 webhook 失败并显示友好的上限提示；创建 `http://` 或私有 IP URL 在前端和服务端都被拒绝；DNS 后来解析到私有 IP 的 webhook，其投递被安全失败并记录 SSRF 错误。
- **AC-6:** 投递面板列出最近 50 条投递（状态标签、响应码、尝试次数）；对已记录投递点重新投递会产生新的投递行（`is_redelivery`），以相同负载 `id` 到达端点。
- **AC-7:** 发送测试事件立即分发 `webhook.test`；数秒内该投递出现在日志中并带端点响应码。
- **AC-8:** 连续 20 次投递失败后 webhook 变为暂停，行上显示自动暂停横幅，重新启用前不再分发事件，一键重新启用并重置计数器。
- **AC-9:** `template.updated` 只在内容实际变化并创建新版本时触发（跳过版本创建的"内容未变"更新不触发），负载含 `version` 和 `change_note`。
- **AC-10:** 删除 webhook 级联删除其投递行；删除用户级联删除两者（FK 行为已验证）。
- **AC-11:** 超过 30 天的投递行被调度器清除；无论表多大，UI 每个 webhook 最多显示 50 条。
- **AC-12:** 整个 `/webhooks` 界面（页面、创建/编辑弹窗、密钥弹窗、投递面板、横幅、错误）以中英文完整渲染，无硬编码字符串。

## 8. 不做范围（v1）

- 外部消息队列（Kafka/RabbitMQ）—— 进程内 goroutine + 数据库支撑的重试符合代码库的尽力而为模式；规模化时再评估。
- 自动暂停时的邮件或站内推送告警（`notifications` 表是互动形状的，`template_id/actor_id NOT NULL`；为系统消息扩展它是另一个决策）。
- 管理员 webhook 控制台（平台级投递统计）—— 按用户上限和 SSRF 规则已足够约束。
- 批量/过滤订阅（如按模板的 webhook、标签过滤）—— v1 为按事件类型的"自己全部模板"。
- 自定义负载模板、mTLS、JWS 签名或端点所有权验证（挑战握手）。
- Webhook 指标导出（Prometheus）—— 投递日志覆盖 v1 调试需求。
- 入站 webhook（Open Prompts 作为接收方）—— 无关领域。

## 9. 依赖与假设

- **D-1:** 互动挂接点存在且稳定：`createEngagementNotification` 调用点位于 `prompt.go:219`（fork）、`:604`（like）、`:632`（favorite）和 `comment.go:113`（comment）；生命周期挂接点在提交后落入 `CreateTemplate`（`prompt.go:92`）、`UpdateTemplate` 的新版本分支（`prompt.go:~284–310`）和 `DeleteTemplate`（`prompt.go:570`）。
- **D-2:** api-key-scopes-permissions（设计）明确把 webhook 签名密钥留给本功能（其 §8 第 96 行）—— 无冲突。
- **D-3:** 无外部队列或调度服务可用或必需；当前规模下 30 秒 ticker 上的数据库到期重试扫描可接受（与 Redis 限流器相同的运维简洁性）。
- **D-4:** `/webhooks` 路由加入 `App.js` 并带会话认证重定向（与 `/api-keys` 相同模式）；Header 新增导航入口并在两个语言包中新增 i18n key。
- **D-5:** `webhook_deliveries` 的增长受 5 个 webhook 上限、事件量、30 天保留和 50 行 UI 窗口约束；`(webhook_id, created_at DESC)` 和 `(status, next_retry_at)` 上的标准 b-tree 索引足够。
- **D-6:** 一次性密钥展示遵循 APIKeys 页的生成密钥弹窗模式（`api_keys.generated_warning`）—— 用户已经学会"现在就复制"。

## 10. 给架构师的开放问题

1. **密钥存储：** 明文列（最简单，满足签名需求）vs. 用 `SECRET_ENCRYPTION_KEY` 环境变量做应用层加密 —— 我们是否已有密钥管理约定？
2. **调度器位置：** 假设为 gRPC 服务进程内的单 goroutine ticker vs. 独立 worker 二进制 —— 部署方案是否允许以后加第二个进程而不改表结构？
3. **重试持久化 vs. Redis：** 投递走数据库（持久、可审计）而限流走 Redis（临时）—— 这个拆分可接受吗，还是重试也应放进 Redis 以保持对称？
4. **事件信封版本化：** 现在就在信封里放 `payload_version: 1`（廉价的保险）还是等到破坏性变更迫使其出现？
5. **自动暂停阈值：** 连续 20 次失败（最坏约 4 小时重试）—— 合理的默认值，还是应该像限流一样支持环境变量配置？
6. **扇出并发：** 每个（webhook × 事件）一个 goroutine 且无上限（假设如此，鉴于 5 个 webhook 限制）还是用小 worker 池在流量高峰限制 goroutine 数量？
