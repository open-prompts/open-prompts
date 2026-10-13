# 相关模板与发现 — 架构设计

| | |
|---|---|
| **Feature ID** | related-templates |
| **Version** | 1.0 |
| **Status** | Approved for implementation（已批准实施） |
| **Author** | Architect Agent |
| **Upstream** | [需求文档](../design/related-templates-requirements-zh-CN.md), [UI/UX 设计](../design/related-templates-ui-ux-design-zh-CN.md) |
| **Downstream** | [详细设计](related-templates-detailed-design-zh-CN.md) |

## 1. 概述

模板详情页目前是个"死胡同"。本功能在每个公共模板详情页加入两条发现
rail：**"相关提示词"** rail（最多 8 张卡片，由可解释的元数据打分 —— 同
分类、共享标签、同语言、同作者）和 **"来自 {author} 的更多"** 行（最多
4 张卡片，按点赞数降序）。相关度是模板自身元数据的纯函数：无个性化、
无 ML、无追踪、无新表。两个新的只读子资源端点服务这两条 rail；前端在
主内容渲染后懒加载。

### 1.1 已决开放问题（需求 §10）

| # | 问题 | 决策 |
|---|---|---|
| 1 | 查询形态 | **单条 SQL**，CASE 打分表达式，`ORDER BY score DESC, like_count DESC, updated_at DESC, LIMIT 8`。候选池小、表达式简单，一次往返优于应用层打分。 |
| 2 | 缓存层 | **Redis，5 分钟 TTL**（技术栈已运行 Redis；`PromptService` 已持有 `RedisStore`）。Key：`related:v1:{template_id}:{limit}`。进程内缓存无法跨副本共享；HTTP/CDN 缓存对懒加载 XHR 没有必要。 |
| 3 | 端点粒度 | **两个端点**（related + more-from-author）。并行懒加载、独立缓存、独立失败 —— 合并进 `?include=` 会把它们耦合到详情响应的生命周期。 |
| 4 | 同作者权重 | **保留 0.5 权重**（FR-3）。作者自己的作品确实相关；低权重加上专属作者行，不会淹没 rail。 |
| 5 | 基准标签 | **响应上单一 `basis` 字段**（category/tag/generic），由榜首条目的主导信号推导。逐卡片原因对 v1 价值有限、负载更重。 |
| 6 | 上限 | **确认 8 相关 / 4 作者。** 所有视口同一负载；移动端用 CSS（scroll-snap）少显示几张，不发不同请求。 |

### 1.2 非目标

- 个性化推荐、浏览历史信号、共现互动矩阵（personalized-recommendations
  设计负责该领域）。
- 主页推荐 rail。
- ML/嵌入相似度或语义搜索。
- 相关合集/链路/团队。
- rail 卡片上的"不感兴趣"控制。
- **任何形式的商业化** —— 付费 rail、推广位、付费曝光永久排除在本产品
  之外。

## 2. 系统上下文

```mermaid
flowchart LR
    subgraph Client
        TD[TemplateDetails 页面<br/>+ RelatedRail + MoreFromAuthorRow]
    end
    subgraph Gateway
        GW[REST 网关 :8080<br/>/api/v1/templates/ 处理器]
    end
    subgraph Backend
        PS[PromptService<br/>GetRelatedTemplates / ListMoreFromAuthor<br/>可见性门 + Redis 缓存]
        TR[TemplateRepository<br/>ListRelated / ListMoreFromAuthor]
    end
    subgraph Stores
        PG[(PostgreSQL)]
        RD[(Redis<br/>5 分钟 rail 缓存)]
    end
    TD -->|GET /api/v1/templates/{id}/related| GW --> PS
    TD -->|GET /api/v1/templates/{id}/more-from-author| GW --> PS
    PS --> RD
    PS --> TR --> PG
```

## 3. 涉及组件

| 组件 | 变更 |
|---|---|
| `backend/api/proto/v1/prompt.proto` | `PromptService` 加两个 RPC：`GetRelatedTemplates`、`ListMoreFromAuthor`；请求/响应消息。`Template` 消息不动（字段 18/19 已被 follow-creators 预留，20–22 被 template-ratings 预留）。 |
| `backend/internal/repository/template_repository.go` | 两个查询方法：`ListRelated`（单条打分查询）与 `ListMoreFromAuthor`（简单过滤列表）。两者复用公共可见性 + 封禁用户排除片段。 |
| `backend/internal/service/prompt.go`（或新文件 `related.go`） | 可见性门（与 `ListComments` 同模式）、Redis 读/写透缓存、basis 推导、proto 映射。 |
| `backend/cmd/server/main.go` | 既有 `/api/v1/templates/` 处理器加两个 GET 分支：`parts[1] == "related"` 与 `parts[1] == "more-from-author"`。可选鉴权（无效 token → 401，与详情 GET 一致）。 |
| `frontend/src/components/related/`（新增） | `RelatedRail.js`、`MoreFromAuthorRow.js`、`RailCard`（复用 `PromptCard`）、`BasisLabel`、骨架屏。 |
| `frontend/src/pages/TemplateDetails.js` | 在提示词内容下方挂载两条 rail（comments 功能落地后位于其上方）。 |
| `frontend/src/services/api.js` | `getRelatedTemplates(id, limit)`、`getMoreFromAuthor(id, limit)`。 |
| `frontend/src/locales/{en,zh}/translation.json` | `related.*` 命名空间，中英同步。 |
| `deployment/init.sql`（两份） | **无变更** —— 不需要新表或索引（见 §5）。 |

## 4. 接口设计

两个端点都是既有 `/api/v1/templates/{id}/...` 处理器的子资源，沿用
`versions`/`comments` 先例。

### 4.1 获取相关模板

`GET /api/v1/templates/{id}/related?limit=8`

| 参数 | 类型 | 默认 | 约束 |
|---|---|---|---|
| `limit` | int | 8 | 收敛到 [3, 8]。低于 3 收敛到 3（rail 本来就少于 3 隐藏）；高于 8 收敛到 8。 |

**鉴权**：可选（与详情 GET 同流程：无效 token → 401；匿名 → 公共上下文）。

**响应：**

```json
{
  "items": [ /* Template 消息，最多 8 个 */ ],
  "basis": "category"   // "category" | "tag" | "generic"
}
```

- `items` 是完整 `Template` proto（与列表端点返回的形状相同），
  `PromptCard` 无需适配即可渲染。
- `basis` 推导：若榜首条目与模板同分类 → `"category"`；否则若共享 ≥1
  个标签 → `"tag"`（标签用榜首条目的第一个共享标签）；否则
  `"generic"`。
- 少于 3 个幸存者 → `items: []` 且 `basis: "generic"`；前端隐藏该区块
  （FR-5）。端点对空结果绝不报错。

### 4.2 来自作者的更多

`GET /api/v1/templates/{id}/more-from-author?limit=4`

| 参数 | 类型 | 默认 | 约束 |
|---|---|---|---|
| `limit` | int | 4 | 收敛到 [1, 4]。 |

**鉴权**：可选，同上。

**响应：**

```json
{ "items": [ /* Template 消息，最多 4 个 */ ] }
```

作者没有其他公共模板时 `items` 为空；前端隐藏该行（FR-6）。

### 4.3 可见性规则（两个端点）

与模板详情端点和 `ListComments` 完全一致：

1. 模板不存在 → `404 NOT_FOUND`。
2. 模板私有且调用者非所有者 → `404 NOT_FOUND`（与不存在不可区分 ——
   与 comments 同约定）。
3. 封禁所有者的公共模板 → `404 NOT_FOUND`（`templateRepository.Get`
   的封禁检查已强制）。
4. rail 只在公共模板上渲染。**所有者**查看自己的私有模板时，两个端点
   也返回 `404` —— rail 是公共发现界面，不是创作辅助（FR-11："不在私有
   模板（所有者视图）上渲染"）。前端在 `visibility != "public"` 时不调用
   端点。

### 4.4 错误码

| HTTP | gRPC | 条件 |
|---|---|---|
| 404 | `NOT_FOUND` | 不存在的 id；私有模板（非所有者或所有者 —— 见 §4.3）；封禁所有者的公共模板。 |
| 401 | `UNAUTHENTICATED` | Bearer token 存在但无效（网关在服务前拒绝）。 |
| 429 | `RESOURCE_EXHAUSTED` | 既有模板读取限流（不变）。 |
| 500 | `INTERNAL` | 仓储层失败。 |

无新错误条件；`writeError` 已能映射以上全部。

### 4.5 缓存（FR-10、NFR-1、NFR-3）

- Redis key `related:v1:{template_id}:{limit}`（及
  `author:v1:{template_id}:{limit}`），TTL 5 分钟，值为响应的序列化
  JSON。
- 读透：未命中则计算、存储、返回。Redis 故障时记日志并落到数据库
  （rail 是增强 —— Redis 永远不是硬依赖）。
- 缓存失效：v1 不做（5 分钟陈旧度对发现 rail 可接受；点赞数可能轻微
  滞后）。模板删除或可见性切换在 TTL 内传播。
- 确定性（NFR-3）：SQL 完全确定（score、like_count、updated_at、最终
  id 决胜），TTL 内重复加载返回逐字节一致的响应。

## 5. 数据模型

**无 schema 变更。** 相关度只读既有列：`templates.category`、`tags`、
`language`、`owner_id`、`like_count`、`visibility`、`updated_at`
（D-1）。打分查询不需要新索引：过滤 `visibility = 'public'`（既有索引）、
排除当前模板与封禁所有者、对有界候选集排序。v1 规模（≤10 万模板）下，
公共模板顺序扫描 + 内存排序远在 200ms p95 预算内；既有
`idx_templates_category` 与 GIN tags 索引可加速常见的分类/标签驱动场景。
若 p95 恶化，已记录的升级路径是打分前先做 `LIMIT` 有界预过滤
（`category = $current` OR `tags && $current_tags`）。

### 5.1 打分查询形态

```sql
SELECT t.* /* + 与 List 相同的 is_liked/is_favorited JOIN */
FROM templates t
WHERE t.visibility = 'public'
  AND t.id != $1                          -- 当前模板
  AND NOT EXISTS (SELECT 1 FROM users bu
                  WHERE bu.id = t.owner_id AND bu.status = 'banned')
ORDER BY
    (CASE WHEN t.category = $2 THEN 3 ELSE 0 END
   + CASE WHEN t.tags && $3 THEN 2 * cardinality(ARRAY(
             SELECT unnest(t.tags) INTERSECT SELECT unnest($3))) ELSE 0 END
   + CASE WHEN t.language = $4 THEN 1 ELSE 0 END
   + CASE WHEN t.owner_id = $5 THEN 0.5 ELSE 0 END) DESC,
    t.like_count DESC,
    t.updated_at DESC,
    t.id ASC
LIMIT $6
```

`$3` 是当前模板标签的 `pq.Array` 参数。`more-from-author` 查询同形态
但无打分：`owner_id = $owner AND visibility = 'public' AND id != $1`，
`ORDER BY like_count DESC, updated_at DESC, id ASC`，`LIMIT $6`。

## 6. 鉴权与授权

- 两个端点均为只读 + 可选鉴权 —— 无新授权决策。可见性门（§4.3）复用
  `ListComments` 的确切模式：经 `TemplateRepo.Get` 获取，私有非所有者
  404。
- 身份只来自 context（`GetUserIDFromContext`）；客户端绝不传 owner 或
  user id。
- 无管理界面（FR-14）：admin 模板视图不渲染 rail。

## 7. 前端

- **`RelatedRail`**：`<section aria-label>` + `h2` 标题（"相关 ·
  {category/tag}"，经 `BasisLabel`）、水平卡片列表（桌面：换行网格；
  移动：scroll-snap + 下一张卡片露出）、加载中骨架屏、`items.length < 3`
  或出错时隐藏（静默 —— rail 绝不暴露失败）。
- **`MoreFromAuthorRow`**：同结构；作者名链接到 follow-creators 公共
  主页（`/creators/:id`），该界面上线前为纯文本（FR-7/D-5）。
- **卡片**原样复用 `PromptCard`（FR-12）。
- **懒加载**（FR-11/NFR-1）：rail 在详情页主内容渲染后取数 —— 绘制后
  触发的 `useEffect`（详情页本就先加载模板；rail 是第二个非阻塞请求）。
  LCP 不受影响。
- **位置**：提示词/版本内容下方、comments 上方（若存在）。私有模板与
  admin 视图不渲染。
- **i18n**：`related.*` 命名空间；分类/标签名原样渲染（专有名词，
  NFR-4）。

## 8. 配置

无新增环境变量。缓存 TTL 是 Go 常量（`relatedCacheTTL = 5 *
time.Minute`）；上限（8/4）是 Go 常量。Redis 地址来自既有 `REDIS_ADDR`。

## 9. 安全考量

- **注入**：所有值都是绑定参数（`$1..$n`）；basis 字符串是服务端枚举，
  绝非客户端输入。
- **隐私**：无新 PII。rail 只暴露非封禁作者的公共模板 —— 与公共列表
  端点已提供的数据相同。不使用、不存储任何观看者数据（NFR-6）。
- **访问控制**：私有模板与不存在不可区分（404），与 comments 约定一致；
  封禁所有者的公共模板由既有仓储检查隐藏。
- **滥用**：只读端点在既有限流器之下；Redis 缓存吸收重复流量。无写
  面可滥用。

## 10. 部署说明

- 无 DDL、无新服务、端口或环境变量。
- Redis 已是部署的硬依赖；rail 缓存每个被浏览模板增加约 2 个 key、
  5 分钟 TTL —— 内存可忽略（每个几 KB）。
- `backend/server` 是被跟踪的构建产物 —— 绝不提交对它的修改。

## 11. 测试策略

- **FVT**（`tests/fvt.py`）：`test_related_templates` —— 预置分类簇、
  标签簇、语言簇和作者目录；验证排序（分类 > 标签 > 语言 > 仅同作者）、
  平分决胜（like_count、updated_at）、当前模板排除、私有/封禁排除、
  私有/不存在 id 404（匿名与非所有者）、<3 候选 → 空 items、
  more-from-author 排序与空场景、limit 收敛、缓存确定性（TTL 内两次
  调用响应一致）。
- **单元**：basis 推导；可见性门（私有非所有者 404、所有者看私有 404
  见 §4.3）；缓存读写透与 Redis 故障回退；仓储打分 SQL 参数化；
  `MockTemplateRepository` 两个新方法的桩。
- **E2E**（test-agent）：rail 在内容后懒加载；骨架屏 → 加载完成；隐藏
  状态（<3、无作者模板）；卡片点击跳转；作者链接；卡片键盘导航；
  `aria-label` 区块；EN/ZH 文案；私有模板视图无 rail。
