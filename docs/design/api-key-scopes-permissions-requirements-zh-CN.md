# API Key 权限范围（Scopes & Permissions）— 需求分析

- **Feature ID:** api-key-scopes-permissions
- **Version:** 1.0
- **Status:** Proposed（待架构评审）
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. 竞品研究摘要

| 平台 | 权限范围模式 | 关键启示 |
|---|---|---|
| **OpenAI Platform（API keys）** | 密钥默认全量访问；服务账号 / 项目密钥带角色；官方要求"把密钥当密码对待"、可疑即轮换 | 最大的平台也是先上线粗粒度密钥，待用户集成广泛后再加权限范围；UI 的职责是让每把密钥的"爆炸半径"可见 |
| **GitHub（Personal Access Tokens）** | 细粒度 PAT：按仓库选择 + 权限矩阵（Contents、Issues、Metadata…），每项为 无/只读/读写 三态；经典 token 用粗粒度 `scope` 字符串（`repo`、`read:org`），创建时以标签展示 | 带显式权限等级的权限矩阵是黄金标准；在 token 列表上以标签展示有效权限，让用户一眼完成审计；"最近使用"列驱动清理 |
| **Anthropic Console（API keys）** | 简单密钥列表（名称、创建时间、最近使用）；v1 无按密钥权限范围——安全性来自 workspace 隔离 | 极简可行，但用户在论坛明确要求范围化密钥；折中方案（只读 vs 完全）覆盖大部分需求 |
| **Hugging Face（access tokens）** | 两档：`read`（公开内容）与 `write`/`fine-grained`（按仓库写、受限仓库）；创建时角色选择器，token 行上角色徽章 | 两档选择（读 vs 写）以 20% 的复杂度拿到约 80% 的价值；细粒度为可选项 |
| **Stripe（restricted API keys）** | "标准密钥"（全量）vs "受限密钥"（每权限 读/写 开关）；受限密钥是集成场景的推荐默认 | 显式命名两种模式（"标准" vs "受限"）并默认推荐受限，可显著降低事故影响；权限分组按业务域而非端点 |
| **Vercel / Supabase** | 创建时勾选权限范围；范围创建后不可变——变更即新建密钥 | 不可变范围保持模型简单；用户接受"新建密钥"作为变更路径 |

**综合结论：** 获胜的 v1 模式是 **两档 + 可选细粒度**：每把密钥有一个角色 —— `read-only`（SDK/生产解析的安全默认）或 `full`（账号可做的一切）—— 再加一个可选的按权限矩阵（GitHub/Stripe 风格）给需要的用户。范围在**创建后不可变**（Vercel/Supabase 模式）：要变更就新建密钥。API Keys 页面为每把密钥增加角色徽章、创建弹窗中的范围选择器，以及已存在的"最近使用"列。校验发生在 `VerifyAPIKey` —— 它目前只返回 userID，等于赋予密钥账号的全部能力（包括删除模板和创建新密钥）。

## 2. 问题陈述

1. **每把密钥都是主密钥。** `VerifyAPIKey`（auth.go:246-283）把密钥解析为 userID，仅此而已。为单一用途签发的密钥 —— 在生产应用里通过 `GET /api/v1/templates/{id}/aliases/{alias}` 解析 prompt —— 也能删除用户的模板、创建新密钥、修改资料。无法给脚本、队友或 CI 一个"不能毁数据"的凭据。
2. **爆炸半径不可见。** API Keys 页面展示名称、前缀、创建时间、最近使用 —— 没有任何信息告诉用户一把密钥能做什么。事故后用户无法审计"我的哪些密钥可能做了这件事？"
3. **没有不删除的吊销手段。** `api_keys.is_active` 存在于 schema 且被 `VerifyAPIKey` 检查，但没有对应的 RPC 和 UI 开关。唯一的补救是彻底删除密钥，连带毁掉 api-usage-history 依赖的审计轨迹（名称、created_at、last_used_at）。
4. **UI 没有过期选择。** `expires_days` 在 proto（`CreateAPIKeyRequest`）和仓储层都存在，但创建弹窗只收集名称。用户无法签发短时效密钥，尽管后端已支持。

## 3. 目标用户

- **集成者（主要）：** 用 API 密钥在自己的应用里调用 `GET /api/v1/templates/{id}/aliases/{alias}` 的用户 —— 他们想要一把泄露后也无法修改任何东西的只读密钥。
- **有安全意识的创作者（次要）：** 拥有大量模板、连接脚本/自动化/队友工具的用户，希望把每个凭据限制在其实际需要的范围内。
- **事故响应者：** 需要按密钥查看"它能做什么、最近何时使用"的用户（以及协助排查的管理员）。

## 4. 用户故事

- **US-A1:** 作为集成者，我希望创建只读 API 密钥，这样贴进生产应用的密钥只能获取 prompt，永远无法修改或删除我的模板。
- **US-A2:** 作为集成者，我希望在列表中每把密钥旁看到角色徽章（只读 / 完全访问），以便一眼审计哪些密钥是危险的。
- **US-A3:** 作为有安全意识的创作者，我希望创建密钥时能挑选具体权限（例如"读模板"但不"写模板"），让每个凭据只获得所需的访问。
- **US-A4:** 作为用户，我希望临时禁用一把密钥而不删除它，以便在调查可疑使用模式时保留该密钥的历史。
- **US-A5:** 作为用户，我希望重新启用已禁用的密钥，这样虚惊一场后不必向所有集成重新分发新密钥。
- **US-A6:** 作为集成者，我希望创建密钥时设置有效期（30/60/90/180 天或永久），让被遗忘的密钥自动失效。
- **US-A7:** 作为用户，我希望创建弹窗在我即将签发完全访问密钥时用平实语言警告我，让我有意识地选择危险选项。
- **US-A8:** 作为事故响应者，我希望在每把密钥上看到"最近使用"和角色，以便回答"哪把密钥做了这件事、它当时能做什么？"
- **US-A9:** 作为用户，我希望停用/过期密钥在视觉上明显不同（弱化行、徽章），让陈旧密钥看起来不像有效的。
- **US-A10:** 作为用户，我希望范围变更必须新建密钥（旧密钥保留直到我删除），让范围历史保持诚实且简单。
- **US-A11:** 作为集成者，我希望范围受限的密钥尝试写操作时收到明确指出缺失范围的 403 错误，让我的代码能给出可操作的提示而非笼统失败。
- **US-A12:** 作为管理员，我希望用户的密钥被此模型自动约束，这样泄露的用户密钥永远无法触及管理面（管理 RPC 仅限会话令牌，保持不变）。

## 5. 功能需求

- **FR-1:** **Schema。** 为 `api_keys` 增加 `role TEXT NOT NULL DEFAULT 'full'` 与 `scopes TEXT[] NOT NULL DEFAULT '{}'`。`role` ∈ {`read-only`, `full`}；`scopes` 为权限码列表（可为空）。存量密钥迁移为 `role='full', scopes='{}'`（行为不变）。增加 `updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`。
- **FR-2:** **权限模型。** 权限码：`templates:read`、`templates:write`、`templates:delete`、`comments:read`、`comments:write`、`keys:read`、`keys:write`。角色解析：`full` → 账号拥有的全部权限；`read-only` → {`templates:read`, `comments:read`, `keys:read`}；若 `scopes` 非空则**覆盖**角色预设（细粒度模式）。管理 RPC 永不可授予 API 密钥。
- **FR-3:** **校验点。** `VerifyAPIKey` 返回 `(userID, *KeyContext)`，其中 `KeyContext = {Role, Scopes, KeyID}`。网关与服务层在执行前按操作检查所需权限；权限不足 → `403`，响应体 `{"error": "insufficient_scope", "required": "<code>"}`。未认证/无效密钥仍为 `401`。
- **FR-4:** **只读密钥行为。** `read-only` 密钥可以：列表/获取模板与版本、解析别名、列出评论、列出同级密钥元数据。不可以：创建/更新/删除模板、fork、点赞、收藏、评论、创建/更新/删除别名、创建/删除密钥、更新资料。
- **FR-5:** **创建 API。** `CreateAPIKeyRequest` 增加 `role`（默认 `full` 以保持向后兼容）与 `repeated string scopes`，`expires_days` 在 UI 中暴露（proto 已有）。响应元数据包含角色与有效范围。校验：未知角色或权限码 → `400 INVALID_ARGUMENT`。
- **FR-6:** **列表 API。** `APIKey` 消息增加 `role`、`scopes`（有效列表）与 `is_active`（schema 已有，现在暴露）。API Keys 表格为每行渲染角色徽章与 有效/过期 状态。
- **FR-7:** **启用开关。** 新 RPC `SetAPIKeyActive(id, is_active)`；仅限所有者；切换写入 `updated_at`；保留审计轨迹（不删除）。UI：行操作菜单中的开关，禁用需确认步骤。
- **FR-8:** **有效期。** 创建弹窗提供 永久 / 30 / 60 / 90 / 180 天（默认永久）。过期密钥认证失败返回 `401 api key expired`（现有行为），列表中渲染为"已过期"。
- **FR-9:** **不可变性。** 角色与范围创建后不可更改。不提供编辑入口；UI 提供"以新范围复制"（预填创建弹窗：同名 + ` (2)` + 原密钥的范围选择）。
- **FR-10:** **Playground 集成。** api-playground 的认证选择器在选中 `API key` 后，校验成功即内联显示密钥角色；若所选端点需要密钥缺少的权限，则阻止 Run 并内联报错（playground v1 端点为只读，只读密钥可用）。
- **FR-11:** **用量集成。** api-usage-history 的按密钥表格增加角色列；密钥筛选下拉在每个密钥名旁显示角色徽章。
- **FR-12:** **安全不变量。** API 密钥永远不能调用管理 RPC（结构性强制：管理处理器要求管理员会话令牌，而非 `VerifyAPIKey`）；密钥永远不能授予账号本身没有的权限；`keys:write` 不允许提权（带 `keys:write` 的细粒度密钥只能创建其自身有效范围子集的密钥）。

## 6. 非功能需求

- **NFR-1:** **存量密钥零行为变化。** 本功能之前创建的密钥行为与之前完全一致（完全访问）。SDK 与既有集成无需任何客户端更新。
- **NFR-2:** **性能。** 范围解析不增加额外数据库往返：role/scopes 是 `GetByHash` 已取回的 `api_keys` 行上的列。权限检查为内存集合查找。p95 认证开销 < 1 ms。
- **NFR-3:** **清晰性。** 每次权限拒绝都在响应体中指明缺失的权限码，UI 将权限码映射为 EN/ZH 的人类可读文案。
- **NFR-4:** **无障碍。** 角色徽章为文本（非仅颜色）；范围选择器可键盘导航且有可见焦点；状态变化通过 live region 播报。
- **NFR-5:** **国际化。** 所有新文案 EN/ZH 同日就绪；权限名称与拒绝消息本地化。
- **NFR-6:** **可审计。** 密钥创建（含角色）、启用切换与删除是可见事件：创建/切换/删除相应更新 `updated_at`/`last_used_at`；按密钥用量仍可通过 api-usage-history 查询。

## 7. 验收标准

- **AC-1:** 以 `read-only` 角色创建密钥并用其 `GET /api/v1/templates/{id}/aliases/{alias}` 成功；用同一密钥 `PUT /api/v1/templates/{id}` 返回 `403`，响应体 `{"error":"insufficient_scope","required":"templates:write"}`。
- **AC-2:** API Keys 列表为每把密钥显示角色徽章（只读 / 完全访问 / 细粒度）与 有效 / 已禁用 / 已过期 状态；禁用与过期行视觉弱化。
- **AC-3:** 以非法权限码（`foo:bar`）创建密钥返回 `400` 且弹窗显示内联错误；列表不变。
- **AC-4:** 通过行操作菜单禁用密钥需要确认；确认后该行显示"已禁用"、`updated_at` 变化，且用该密钥的请求返回 `401 api key is inactive or invalid`（现有消息）。
- **AC-5:** 重新启用已禁用密钥无需重新签发密文即可恢复访问；该行回到"有效"。
- **AC-6:** 创建弹窗提供 永久/30/60/90/180 有效期；以 30 天创建的密钥在列表中显示"30 天后过期"，到期后以 `401 api key expired` 失败。
- **AC-7:** 创建后不存在角色/范围的编辑入口；"以新范围复制"打开预填了源密钥名称 + " (2)" 及其范围选择的创建弹窗。
- **AC-8:** 范围为 `[templates:read]` 的细粒度密钥可以列表和获取模板，但发评论时收到 `403 insufficient_scope`（指明 `comments:write`）。
- **AC-9:** 带 `keys:write` 的细粒度密钥可以创建新密钥，但试图授予新密钥自己没有的权限时返回 `400` 提权错误。
- **AC-10:** playground 用只读密钥对别名解析端点运行成功并内联显示密钥角色；用量页的密钥筛选显示角色徽章。
- **AC-11:** 所有存量密钥（本功能前）行为不变：完全访问、无客户端迁移、SDK 请求成功。
- **AC-12:** 所有新 UI 文案在 `en` 与 `zh` 两个翻译文件中都存在；范围选择器、徽章与拒绝消息完全本地化；纯键盘用户可完成 创建/复制/禁用 流程。

## 8. 不做范围（v1）

- 按模板或按合集的资源级限制（细粒度密钥是账号级权限集合）。
- 组织/团队密钥（team-collaboration 特性负责共享凭据）。
- 密钥轮换自动化（定时轮换、邮件轮换提醒）。
- 按密钥的 IP 白名单 / CIDR 限制。
- 按密钥的速率限制（rate-limit-dashboard 负责按密钥限额）。
- Webhook 签名密钥（webhooks-api-integration 负责该面）。
- 管理员签发或管理员范围的密钥；管理台密钥管理 UI。
- 创建后修改范围（设计上不可变；复制替换是路径）。

## 9. 依赖与假设

- **D-1:** `api_keys` 表（init.sql:185-198）已存在 `is_active`、`expires_at`、`last_used_at`；`APIKeyRepository`（api_key_repository.go）提供 `GetByHash`、`Create`、`ListByUserID`、`Delete`。假设增加列 + `UpdateActive` 方法是直接的。
- **D-2:** `VerifyAPIKey`（auth.go:246-283）是 gRPC 拦截器与 REST 网关（`VerifyAnyToken` 回退路径）中 API 密钥认证的唯一咽喉点；假设通过 context 传递 `KeyContext` 对架构师可接受。
- **D-3:** `CreateAPIKeyRequest.expires_days` 已在 proto 中存在；假设以 `role`/`scopes` 字段扩展消息是向后兼容的（proto3 optional 字段）。
- **D-4:** APIKeyManager 组件（frontend/src/components/APIKeyManager.js）与 APIKeys 页面已存在创建弹窗与表格；假设扩展弹窗与表格是自然的扩展点。
- **D-5:** api-playground（设计）定义了带 `X-API-Key` 的认证选择器；api-usage-history（设计）定义了按密钥用量表 —— 两者都把本特性列为集成点而非阻塞项。
- **D-6:** 假设架构师确认 FR-2 的权限码分类（7 个码）与"范围覆盖角色预设"的解析规则。

## 10. 给架构师的开放问题

1. **校验位置：** 把权限检查集中在网关处理器（按路由的所需权限映射）还是各服务方法内部（纵深防御）？设计假设网关按路由映射 + 变更型 RPC 服务层检查。
2. **KeyContext 传递：** 扩展 `VerifyAPIKey` 返回一个经 `context.Context` 携带的结构体（类似 JWT 的 `ContextWithUserScope`）—— 形状可接受吗？context key 类型有偏好吗？
3. **`scopes` 用 Postgres 数组还是 JSONB：** 为简单起见假设 `TEXT[]`；若未来需要按范围查询则用 JSONB？
4. **`SetAPIKeyActive` 传输：** 新 REST 路由 `PATCH /api/v1/api-keys/{id}` 携带 `{"is_active": bool}`，还是 POST 子资源 `/activate` + `/deactivate`？设计假设 PATCH。
5. **提权规则（FR-12）：** `keys:write` 采用"自身范围子集"规则，还是干脆禁止细粒度密钥管理密钥？
6. **迁移：** 存量行默认 `role='full'` —— 确认除列默认值外无需回填。
