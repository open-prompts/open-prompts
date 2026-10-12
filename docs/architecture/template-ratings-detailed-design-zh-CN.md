# 模板评分 — 详细设计

| | |
|---|---|
| **Feature ID** | template-ratings |
| **Version** | 1.0 |
| **Status** | Approved for implementation |
| **Author** | Architect Agent |
| **Upstream** | [架构设计](template-ratings-architecture-zh-CN.md) |

## 1. Proto 契约

新增至 `backend/api/proto/v1/prompt.proto`。`Template` 的 17–19 号字段与
comments/follow-creators 设计协同（17 = `comment_count`，18/19 =
`owner_display_name`/`owner_avatar`）；评分使用 20–22。

```protobuf
// PromptService — 追加到既有 service 块（第 33 行）
service PromptService {
    // ... 既有 RPC ...
    rpc RateTemplate(RateTemplateRequest) returns (RateTemplateResponse);
    rpc DeleteRating(DeleteRatingRequest) returns (RateTemplateResponse);
}

message RateTemplateRequest {
    string template_id = 1;
    int32 rating = 2; // 1..5
}

message DeleteRatingRequest {
    string template_id = 1;
}

message RateTemplateResponse {
    double rating_avg = 1; // rating_count = 0 时为 0
    int32 rating_count = 2;
    int32 my_rating = 3;   // 0 = 未评分
}

message Template {
    // ... 既有字段 1-16；17 comment_count，18/19 owner 字段 ...
    double rating_avg = 20;
    int32 rating_count = 21;
    int32 my_rating = 22;  // 0 = 未评分；仅对已认证调用者有意义
}

message GetTemplateResponse {
    Template template = 1;
    TemplateVersion latest_version = 2;
    // owner = 3（follow-creators 设计）
    repeated int32 rating_distribution = 4; // 索引 0 -> 1★ ... 索引 4 -> 5★
}
```

`admin.proto` — `AdminTemplateSummary`（第 165 行）新增：

```protobuf
message AdminTemplateSummary {
    // ... 既有字段 1-12 ...
    double rating_avg = 13;
    int32 rating_count = 14;
}
```

重新生成：

```bash
cd backend && protoc --go_out=. --go-grpc_out=. api/proto/v1/prompt.proto api/proto/v1/admin.proto
```

## 2. 数据库 DDL

追加到 **两份** `deployment/init.sql` 与
`deployment/chart/open-prompts/init.sql`：

```sql
-- ===== template-ratings =====
CREATE TABLE IF NOT EXISTS template_ratings (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    rating SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, template_id)
);

CREATE INDEX IF NOT EXISTS idx_template_ratings_template
    ON template_ratings(template_id);

ALTER TABLE templates ADD COLUMN IF NOT EXISTS rating_sum BIGINT NOT NULL DEFAULT 0;
ALTER TABLE templates ADD COLUMN IF NOT EXISTS rating_count INT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_templates_top_rated
    ON templates(rating_count, rating_sum);
```

## 3. 后端分层

### 3.1 `models.Template`

新增字段：

```go
RatingSum   int64   // 反规范化；不直接暴露到 proto
RatingCount int32
RatingAvg   float64 // 映射时计算：sum/count，count=0 时为 0
MyRating    int32   // 0 = 未评分；由仓储按查看者 JOIN 填充
```

### 3.2 `TemplateRepository` 新增

```go
RateTemplate(ctx context.Context, userID, templateID string, rating int32) (sum int64, count int32, err error)
DeleteRating(ctx context.Context, userID, templateID string) (sum int64, count int32, err error)
GetRatingDistribution(ctx context.Context, templateID string) ([5]int32, error)
```

`RateTemplate` 事务（`ToggleLike` 形状 —— 读取、增量、写入）：

```sql
BEGIN;
-- 1) upsert 调用者的行；捕获旧值
INSERT INTO template_ratings (user_id, template_id, rating)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, template_id)
DO UPDATE SET rating = EXCLUDED.rating, updated_at = now()
RETURNING rating;                       -- 新值
-- 旧值先读取（SELECT rating ... FOR UPDATE）或经 CTE；
-- 应用到 rating_sum 的增量为 (new - previous)。

-- 2) 事务性应用增量
UPDATE templates
SET rating_sum = GREATEST(rating_sum + $delta, 0),
    rating_count = rating_count + $count_delta  -- 插入 +1，更新 0
WHERE id = $2;

-- 3) 返回最新聚合值
SELECT rating_sum, rating_count FROM templates WHERE id = $2;
COMMIT;
```

实现说明：最简洁的形式是单个 CTE ——

```sql
WITH prev AS (
    SELECT rating FROM template_ratings
    WHERE user_id = $1 AND template_id = $2 FOR UPDATE
), upsert AS (
    INSERT INTO template_ratings (user_id, template_id, rating)
    VALUES ($1, $2, $3)
    ON CONFLICT (user_id, template_id)
    DO UPDATE SET rating = EXCLUDED.rating, updated_at = now()
    RETURNING rating, (xmax = 0) AS inserted
)
UPDATE templates t
SET rating_sum = GREATEST(t.rating_sum + $3 - COALESCE((SELECT rating FROM prev), 0), 0),
    rating_count = t.rating_count + CASE WHEN (SELECT inserted FROM upsert) THEN 1 ELSE 0 END
WHERE t.id = $2
RETURNING t.rating_sum, t.rating_count;
```

`DeleteRating` 对称处理（删除行 → sum 减去被删值、count 减一、下限 0；
行不存在时为 no-op，返回当前聚合值）。

`GetRatingDistribution`：

```sql
SELECT rating, COUNT(*) FROM template_ratings
WHERE template_id = $1 GROUP BY rating;
```

**读路径 JOIN** —— `Get` 与 `List` 增加（与既有 `is_liked`/`is_favorited`
LEFT JOIN 并列）：

```sql
LEFT JOIN template_ratings mr ON mr.template_id = t.id AND mr.user_id = $viewer
-- 选择列：mr.rating AS my_rating（NULL 时为 0）
```

`rating_avg` 在行扫描时计算（`sum/count`），绝不存储。

**`top_rated` 排序分支**（位于排序白名单中，与 search-filtering 设计协同）：

```sql
ORDER BY
  (rating_count >= $threshold) DESC,          -- 达标者在前
  CASE WHEN rating_count >= $threshold
       THEN rating_sum::float / GREATEST(rating_count, 1) END DESC NULLS LAST,
  rating_count DESC,
  created_at DESC
```

### 3.3 `PromptService`（`backend/internal/service/prompt.go`）

`RateTemplate`：

1. 校验 `rating` ∈ [1,5]，否则 `InvalidArgument`。
2. `GetUserIDFromContext`，失败 → `Unauthenticated`。
3. 加载模板（`TemplateRepo.Get`）；缺失或对他人的私有模板 → `NotFound`。
4. 作者检查：`tpl.OwnerID == userID` → `PermissionDenied`（"cannot rate
   your own template"）。
5. 经 `UserRepo.GetByID` 检查封禁 → 封禁时 `PermissionDenied`。
6. 限流（见下）—— 仅成功变更消耗配额。
7. `TemplateRepo.RateTemplate` → 组装 `RateTemplateResponse{rating_avg,
   rating_count, my_rating}`。

`DeleteRating`：步骤 2–3（除鉴权与可见性外无需作者/封禁检查；调用者只能
删除自己的行），然后 `TemplateRepo.DeleteRating`（幂等）。

**无通知钩子** —— 评分在设计上是匿名的（FR-8）。

**限流**（Redis 固定窗口，fail-open）：

```
key:    rating_rate_min:{user_id}
value:  "{count}:{window_expires_unix}"
窗口:    60s，上限 RATING_RATE_PER_MINUTE（默认 20）
```

`PromptService` 新增 `RedisStore` 字段（nil 安全）。**协同**：
template-comments 设计新增同一字段；谁先落地谁引入，另一方复用 —— 不要加
两个字段。

### 3.4 `AdminService`（`backend/internal/service/admin.go`）

`GetTemplate` 与管理端模板列表映射为 `AdminTemplateSummary` 字段 13/14 增加
`rating_avg`（由 `rating_sum`/`rating_count` 计算）与 `rating_count`。
只读；不新增 RPC。

### 3.5 网关（`backend/cmd/server/main.go`）

在 `/api/v1/templates/` 处理器中新增 `/rating` 分支，沿用既有
`strings.HasSuffix(id, "/like")` 模式：

```go
if strings.HasSuffix(id, "/rating") {
    templateID := strings.TrimSuffix(id, "/rating")
    switch r.Method {
    case http.MethodPost:   // 鉴权 → RateTemplateRequest{TemplateId, Rating}
    case http.MethodDelete: // 鉴权 → DeleteRatingRequest{TemplateId}
    default: http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    }
}
```

两个方法都需要 Bearer-token 鉴权流程（同 `/like`），POST 时从 JSON body
解析 rating。429 响应设置 `Retry-After`（writeError 映射已处理
`ResourceExhausted`；错误码匹配时补充该头）。

## 4. 前端

### 4.1 新增文件

| 文件 | 用途 |
|---|---|
| `frontend/src/components/StarRating.js` + `.scss` | 只读星星 + 数值 + 计数；`size` 属性（卡片 sm，详情/管理端 lg）；卡片在 `rating_count = 0` 时整行省略。 |
| `frontend/src/components/RatingWidget.js` + `.scss` | 交互式五星组件：悬停预览、点击提交、键盘 slider（单 tab 停靠、←/→、Enter/Space、Esc）、`my_rating` 状态、带轻量确认气泡的 Clear-rating 动作、乐观填充 + 回滚。 |
| `frontend/src/components/RatingHistogram.js` + `.scss` | 5 行（星级标签、条、百分比文本）；语义 `<ul>`；条 `aria-hidden`；零评分时隐藏。 |

### 4.2 修改文件

| 文件 | 变更 |
|---|---|
| `frontend/src/components/PromptCard.js` | 标题/元信息下方（点赞/收藏行上方）的紧凑 `StarRating size="sm"` 行；零评分时省略。 |
| `frontend/src/pages/TemplateDetails.js` | 与社交统计并列的评分区块（comments 之上）：`RatingWidget` + 平均值（2 位小数）+ 计数 + `RatingHistogram`；作者视图显示不可交互提示；匿名点击 → `/login`。 |
| 排序下拉（search-filtering 组件） | `top_rated` 选项 + 阈值提示。 |
| `frontend/src/pages/admin/TemplateDetailPage.js` | 统计区只读 "Rating: 4.62 (128)" 行。 |
| `frontend/src/services/api.js` | `rateTemplate(id, rating)`、`deleteRating(id)`。 |

### 4.3 Redux

无需新 slice：评分状态随模板详情/列表载荷到达；`my_rating` 更新经组件本地
状态（乐观）与动作响应（事实来源）流转。

### 4.4 API 客户端

```js
rateTemplate(templateId, rating)   // POST   /templates/${templateId}/rating {rating}
deleteRating(templateId)           // DELETE /templates/${templateId}/rating
```

### 4.5 i18n 键（EN/ZH 对齐）

```json
{
  "ratings": {
    "rate": "为模板评分",
    "ratings_count": "{{count}} 条评分",
    "no_ratings": "还没有评分",
    "be_first": "还没有评分 —— 来打第一个分吧",
    "clear": "清除评分",
    "clear_confirm": "移除你的评分？",
    "you_rated": "你的评分：{{count}} 星",
    "owner_hint": "无法为自己的模板评分",
    "submitted": "已评 {{count}} 星",
    "error": "评分保存失败",
    "hint_1": "很差", "hint_2": "较差", "hint_3": "一般",
    "hint_4": "好", "hint_5": "很好",
    "aria_label": "{{count}} 条评分，平均 {{avg}} 星"
  },
  "sort": {
    "top_rated": "评分最高",
    "top_rated_hint": "平均评分，至少 {{count}} 条评分"
  }
}
```

英文版本逐键对应（`ratings.rate` = "Rate this template"、
`sort.top_rated` = "Top rated"，提示 Poor/Fair/Good/Very good/Excellent 等）。

## 5. 错误矩阵

| 场景 | HTTP | gRPC | 前端处理 |
|---|---|---|---|
| 评分 0/6/非整数 | 400 | `INVALID_ARGUMENT` | 字段级错误；组件不变 |
| 未认证 | 401 | `UNAUTHENTICATED` | 重定向 `/login` |
| 作者自评 | 403 | `PERMISSION_DENIED` | 组件显示作者提示（预检） |
| 被封禁调用者 | 403 | `PERMISSION_DENIED` | 错误 toast；组件回滚 |
| 模板缺失/私有 | 404 | `NOT_FOUND` | 404 页 |
| 触发限流 | 429 | `RESOURCE_EXHAUSTED` | 带重试倒计时的 toast；回滚 |
| Redis 宕机 | — | — | 服务端 fail-open；用户无感知 |

## 6. 部署

- `RATING_RATE_PER_MINUTE=20` 与 `TOP_RATED_MIN_RATINGS=5` 加入
  `deployment/docker-compose.yml` 的 backend 环境与 Helm chart
  （`values.yaml` `backend.env.ratingRatePerMinute` / `topRatedMinRatings`
  + `templates/deployment-backend.yaml` env 条目）。
- DDL 增量/幂等；两份 init.sql 副本同一提交。
- 无新服务、端口或基础设施。

## 7. SDK

python-sdk 无需变更：评分是 Web 端特性；SDK 封装模板解析。REST 端点在
API 层面记录文档。

## 8. 测试策略

### 8.1 FVT（`tests/fvt.py`）— `test_template_ratings`

两个用户（alice = 作者，bob = 评分者），经 `get_auth_token`：

1. alice 创建公开模板；bob 评 4 星 → `{rating_avg: 4.0, rating_count: 1,
   my_rating: 4}`。
2. bob 改评 2 星 → `{rating_avg: 2.0, rating_count: 1, my_rating: 2}`
   （行被替换，count 不变）。
3. carol 评 5 星 → `{rating_avg: 3.5, rating_count: 2}`。
4. bob 删除评分 → `{rating_avg: 5.0, rating_count: 1, my_rating: 0}`；
   重复删除 → 响应一致（幂等）。
5. alice 给自己的模板评分 → 403；匿名 POST → 401；bob 用 0/6/3.5 评分 →
   400。
6. 封禁 carol（管理 API）→ carol 的 POST → 403；其既有评分仍计入聚合值
   （FR-14）。
7. 私有模板：bob 的评分尝试 → 404。
8. 详情响应：`rating_distribution` 与评分行一致；列表项携带
   `rating_avg`/`rating_count`/`my_rating`。
9. 限流：连续 21 次变更 → 429 附 `Retry-After`。
10. 管理端模板详情显示 `rating_avg`/`rating_count`；管理端评分变更路由 →
    404/405。

### 8.2 单元测试

- 仓储：CTE upsert 增量数学（插入/更新/无变化）、删除下限、分布
   GROUP BY、带阈值的 `top_rated` 排序。
- 服务：校验矩阵、作者/封禁/可见性检查、限流窗口（fake Redis）、响应
   映射（平均值舍入）。
- 为三个新方法更新 `MockTemplateRepository`。

### 8.3 E2E（test-agent，Nightwatch）

- 组件：悬停预览、点击提交、键盘（←/→/Enter/Esc）、气泡确认清除；强制
  失败时的乐观回滚。
- 直方图渲染 5 行带文本百分比；零评分时隐藏。
- 零评分时卡片行省略；否则显示 "★ 4.6 (128)"。
- search-filtering 发布后出现 `top_rated` 排序选项。
- 所有 `ratings.*` 与 `sort.top_rated` 键的 EN/ZH 对齐。

## 9. 实现顺序

1. DDL（两份 init.sql 副本）+ models + 仓储方法 + 单元测试。
2. Proto 变更 + 重新生成 + 读路径 JOIN（`my_rating`、`rating_avg`）。
3. 服务 RPC + 限流 + `RedisStore` 接线。
4. 管理端只读字段。
5. 网关 `/rating` 分支。
6. 前端：api 客户端 → 组件 → 页面 → i18n。
7. FVT + E2E。

## 10. 给开发者的约束

1. **绝不提交 `backend/server`** —— 被跟踪的构建产物；被修改时执行
   `git checkout -- backend/server`。
2. 仅显式暂存自己的文件；绝不 `git add -A`。
3. 两份 init.sql 副本在同一提交中保持同步。
4. `PromptService.RedisStore` 与 template-comments 设计共享 —— 只引入一次；
   不要重复该字段。
5. `Template` proto 字段号 17–22 是 comments（17）、follow-creators（18/19）、
   ratings（20–22）的协同块 —— 不要重新编号。
6. 平均值绝不存储；映射时计算 `rating_sum/rating_count` 并防除零。
7. `my_rating = 0` 表示"未评分"（评分为 1–5）；前端将 0 视为未评分状态。
8. 限流仅对成功变更消耗配额；Redis 错误时 fail-open。
9. 评分动作不发出通知。
10. 绝不出现任何商业化界面。
