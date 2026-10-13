# 相关模板与发现 — 详细设计

| | |
|---|---|
| **Feature ID** | related-templates |
| **Version** | 1.0 |
| **Status** | Approved for implementation（已批准实施） |
| **Author** | Architect Agent |
| **Upstream** | [架构设计](related-templates-architecture-zh-CN.md) |

## 1. Proto 契约

`backend/api/proto/v1/prompt.proto` 新增（`PromptService` 上新 RPC；
`Template` 消息不动 —— 字段 18/19 已被 follow-creators 预留，20–22 被
template-ratings 预留）：

```protobuf
service PromptService {
    // ... 既有 RPC ...

    // GetRelatedTemplates 返回公共模板的可解释相关模板
    //（分类/标签/语言/作者元数据打分）。
    rpc GetRelatedTemplates(GetRelatedTemplatesRequest) returns (GetRelatedTemplatesResponse);

    // ListMoreFromAuthor 返回作者的其他公共模板，按点赞数降序。
    rpc ListMoreFromAuthor(ListMoreFromAuthorRequest) returns (ListMoreFromAuthorResponse);
}

message GetRelatedTemplatesRequest {
    string template_id = 1;
    int32 limit = 2; // 默认 8，收敛到 [3, 8]
}

message GetRelatedTemplatesResponse {
    repeated Template items = 1;
    string basis = 2; // "category" | "tag" | "generic"
}

message ListMoreFromAuthorRequest {
    string template_id = 1;
    int32 limit = 2; // 默认 4，收敛到 [1, 4]
}

message ListMoreFromAuthorResponse {
    repeated Template items = 1;
}
```

重新生成：

```bash
cd backend && protoc --go_out=. --go-grpc_out=. api/proto/v1/prompt.proto
```

## 2. 数据库 DDL

**无。** 无新表、新列或新索引（架构 §5）。两份 `deployment/init.sql`
本功能均不变更。

## 3. 后端分层

### 3.1 仓储层（`backend/internal/repository/template_repository.go`）

接口新增：

```go
type TemplateRepository interface {
    // ... 既有方法 ...
    ListRelated(ctx context.Context, templateID string, tpl *models.Template, limit int) ([]*models.Template, error)
    ListMoreFromAuthor(ctx context.Context, templateID, ownerID string, limit int) ([]*models.Template, error)
}
```

`ListRelated` —— 单条打分查询（全部绑定参数；`is_liked`/
`is_favorited` LEFT JOIN 与 `List` 相同，用调用者的 user id，卡片才能
渲染正确的点赞/收藏状态）：

```sql
SELECT t.id, t.owner_id, t.title, t.description, t.visibility, t.type,
       t.tags, t.category, t.language, t.like_count, t.favorite_count,
       t.comment_count, t.created_at, t.updated_at,
       CASE WHEN tl.user_id IS NOT NULL THEN true ELSE false END AS is_liked,
       CASE WHEN tf.user_id IS NOT NULL THEN true ELSE false END AS is_favorited
FROM templates t
LEFT JOIN template_likes tl ON t.id = tl.template_id AND tl.user_id = $2
LEFT JOIN template_favorites tf ON t.id = tf.template_id AND tf.user_id = $2
WHERE t.visibility = 'public'
  AND t.id != $1
  AND NOT EXISTS (SELECT 1 FROM users bu
                  WHERE bu.id = t.owner_id AND bu.status = 'banned')
ORDER BY
    (CASE WHEN t.category = $3 THEN 3 ELSE 0 END
    + CASE WHEN t.tags && $4 THEN 2 * cardinality(ARRAY(
              SELECT unnest(t.tags) INTERSECT SELECT unnest($4))) ELSE 0 END
    + CASE WHEN t.language = $5 THEN 1 ELSE 0 END
    + CASE WHEN t.owner_id = $6 THEN 0.5 ELSE 0 END) DESC,
    t.like_count DESC,
    t.updated_at DESC,
    t.id ASC
LIMIT $7
```

参数：`$1` 模板 id、`$2` 当前用户 id（匿名为空字符串 —— LEFT JOIN
自然落空）、`$3` 分类（可为 NULL —— `NULL = NULL` 为 NULL → 0 分，
正确）、`$4` 标签 `pq.Array`、`$5` 语言、`$6` 所有者 id、`$7` limit。

`ListMoreFromAuthor`：

```sql
SELECT t.* /* 同样的列清单 + 点赞/收藏 JOIN */
FROM templates t
LEFT JOIN template_likes tl ON ... AND tl.user_id = $2
LEFT JOIN template_favorites tf ON ... AND tf.user_id = $2
WHERE t.owner_id = $3
  AND t.visibility = 'public'
  AND t.id != $1
ORDER BY t.like_count DESC, t.updated_at DESC, t.id ASC
LIMIT $4
```

（这里无需封禁所有者排除：调用方只有通过 `Get` 才能到达此代码，而
`Get` 已对封禁所有者模板 404。）

两个方法都按 `List` 的方式扫描进 `models.Template`。

### 3.2 服务层（`backend/internal/service/related.go`，新文件）

```go
const (
    relatedCacheTTL   = 5 * time.Minute
    relatedDefaultLimit = 8
    authorDefaultLimit  = 4
)

func (s *PromptService) GetRelatedTemplates(ctx context.Context, req *pb.GetRelatedTemplatesRequest) (*pb.GetRelatedTemplatesResponse, error)
func (s *PromptService) ListMoreFromAuthor(ctx context.Context, req *pb.ListMoreFromAuthorRequest) (*pb.ListMoreFromAuthorResponse, error)
```

共享流程（两个方法）：

1. **可见性门**（与 `ListComments` 完全一致）：
   ```go
   userID, _ := GetUserIDFromContext(ctx) // 可选鉴权
   tpl, err := s.TemplateRepo.Get(ctx, req.TemplateId, userID)
   if err != nil {
       return nil, status.Error(codes.NotFound, "template not found")
   }
   if tpl.Visibility != "public" {
       // rail 是公共发现界面：私有模板对所有人 404，
       // 包括所有者（架构 §4.3）。
       return nil, status.Error(codes.NotFound, "template not found")
   }
   ```
   （`TemplateRepo.Get` 已对封禁所有者的公共模板 404。）
2. **收敛 limit**：related → [3, 8]（默认 8）；author → [1, 4]
   （默认 4）。
3. **读缓存**（仅 `GetRelatedTemplates` —— 作者行足够便宜，每次现算；
   见 §3.4）：
   ```go
   key := fmt.Sprintf("related:v1:%s:%d", tpl.ID, limit)
   if cached, err := s.Redis.Get(ctx, key); err == nil {
       var resp pb.GetRelatedTemplatesResponse
       if json.Unmarshal([]byte(cached), &resp) == nil { return &resp, nil }
   }
   ```
   Redis 错误记日志并吞掉 —— 落到数据库。
4. **查询**仓储；用既有 `templateModelToProto` 映射。
5. **basis 推导**（仅 related）：
   - 榜首条目 `category == tpl.Category`（两者非空）→ `"category"`；
   - 否则榜首条目与 `tpl.Tags` 共享 ≥1 个标签 → `"tag"`（前端用榜首
     条目的第一个共享标签做标签）；
   - 否则 `"generic"`。
6. **写缓存**（仅 related）：响应序列化为 JSON，
   `s.Redis.Set(ctx, key, jsonStr, relatedCacheTTL)`；错误记日志并吞掉。
7. 少于 3 个条目 → 返回 `items: []`（绝不报错）；前端隐藏该区块。

`ListMoreFromAuthor` 跳过步骤 3/6，直接返回仓储结果（空 slice 而非
nil，protojson 才会输出 `[]`）。

### 3.3 网关（`backend/cmd/server/main.go`）

在既有 `/api/v1/templates/` 处理器内、`versions`/`comments` 分支旁
（默认单模板 switch 之前）：

```go
// /api/v1/templates/{template_id}/related — GET（公共，可选鉴权）
if len(parts) == 2 && parts[1] == "related" {
    if r.Method != http.MethodGet {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }
    ctx := context.Background()
    if authHeader := r.Header.Get("Authorization"); authHeader != "" {
        tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
        if userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr); err == nil {
            ctx = service.ContextWithUserID(ctx, userID)
        } else {
            http.Error(w, "Invalid token", http.StatusUnauthorized)
            return
        }
    }
    req := &pb.GetRelatedTemplatesRequest{TemplateId: parts[0]}
    if v := r.URL.Query().Get("limit"); v != "" {
        if i, err := strconv.Atoi(v); err == nil {
            req.Limit = int32(i)
        }
    }
    resp, err := svc.GetRelatedTemplates(ctx, req)
    if err != nil { writeError(w, err); return }
    w.Header().Set("Content-Type", "application/json")
    b, _ := marshaler.Marshal(resp)
    _, _ = w.Write(b)
    return
}
```

`more-from-author` 分支形态相同（`parts[1] == "more-from-author"`、
`ListMoreFromAuthorRequest`）。注意无效 token 的行为与详情 GET（401）
一致，而不是 `versions` 分支的静默匿名 —— rail 与详情页一起在浏览器
渲染，用的是同一个可选鉴权流程。

### 3.4 为什么作者行不缓存

related 查询对整个公共池打分；作者查询是单条索引 `owner_id` 查询加
`LIMIT 4`。缓存它只会增加失效面（作者删除/新建模板），延迟收益微乎其
微。5 分钟的 related 缓存对发现元数据可接受；TTL 内点赞数轻微陈旧没有
问题（NFR-3 要求确定性，不要求新鲜度）。

## 4. 前端

### 4.1 新增文件

| 文件 | 用途 |
|---|---|
| `frontend/src/components/related/RelatedRail.js` + `.scss` | `<section aria-label>` + `h2` + BasisLabel + 卡片列表；加载中骨架屏；<3 条目或出错时隐藏。 |
| `frontend/src/components/related/MoreFromAuthorRow.js` + `.scss` | 同结构；作者名（可用时链接到 `/creators/:id`）；为空或出错时隐藏。 |
| `frontend/src/components/related/BasisLabel.js` | 渲染 `related.title.category`/`title.tag`/`title.generic` 并插值。 |

卡片复用既有 `PromptCard` —— 不新建卡片组件（FR-12）。

### 4.2 修改文件

| 文件 | 变更 |
|---|---|
| `frontend/src/pages/TemplateDetails.js` | 导入并在提示词/版本内容下方挂载 `RelatedRail` + `MoreFromAuthorRow`（存在 `CommentsSection` 时位于其上方）；传入 `templateId`、`template.category`、`template.tags`、`template.ownerId`、`template.visibility`；`visibility !== 'public'` 时不挂载。 |
| `frontend/src/services/api.js` | 新增 `getRelatedTemplates(id, limit)` → `api.get(\`/templates/${id}/related?limit=${limit}\`)`，`getMoreFromAuthor(id, limit)` 同理。 |
| `frontend/src/locales/{en,zh}/translation.json` | `related.*` 命名空间（§4.4）。 |

### 4.3 组件行为

- **懒加载**：每条 rail 在挂载后各自的 `useEffect` 里取数 —— 详情页
  主内容请求已在飞行/完成；rail 绝不阻塞首屏（AC-7）。
- **静默失败**：取数错误置为隐藏状态 —— 无错误横幅、无重试 UI（rail
  是增强，绝不是失败界面）。
- **桌面布局**：换行网格（最多 2 行 × 4 列）。**移动端**：水平
  scroll-snap；卡片 72% 视口宽，下一张卡片露出；滚动区域聚焦后可用
  键盘滚动（原生 overflow），卡片按 DOM 顺序可聚焦（AC-8）。
- **卡片点击**跳转 `/templates/:id`（标准应用内导航）。
- **作者链接**：follow-creators 路由存在时用 `/creators/:ownerId`；
  在那之前渲染纯文本（通过路由注册做特性检测，或直接渲染链接 —— 路由
  缺失时显示应用 404，过渡期可接受；follow-creators 上线前优先纯文本）。

### 4.4 i18n Key（中英对齐）

```json
{
  "related": {
    "title": {
      "category": "相关 · {{category}}",
      "tag": "相关 · {{tag}}",
      "generic": "相关提示词"
    },
    "moreFromAuthor": "来自 {{author}} 的更多",
    "aria": {
      "rail": "相关提示词区块",
      "authorRow": "来自此作者的更多"
    }
  }
}
```

EN：`related.title.category` = "Related · {{category}}"、
`related.title.tag` = "Related · {{tag}}"、`related.title.generic` =
"Related prompts"、`related.moreFromAuthor` = "More from {{author}}"、
`related.aria.rail` = "Related prompts section"、
`related.aria.authorRow` = "More from this author"。
分类/标签名是专有名词 —— 绝不翻译（NFR-4）。

## 5. 错误矩阵

| 场景 | HTTP | gRPC | 前端处理 |
|---|---|---|---|
| 模板 id 不存在 | 404 | `NOT_FOUND` | 区块隐藏（静默）。 |
| 私有模板（任何调用者，含所有者） | 404 | `NOT_FOUND` | 前端不调用（可见性检查）；404 也隐藏。 |
| 封禁所有者的公共模板 | 404 | `NOT_FOUND` | 区块隐藏（静默）。 |
| 无效 Bearer token | 401 | `UNAUTHENTICATED` | 既有鉴权错误处理。 |
| <3 个相关候选 | 200，`items: []` | — | 区块隐藏。 |
| 作者无其他模板 | 200，`items: []` | — | 行隐藏。 |
| Redis 宕机 | 200（数据库回退） | — | 无（仅日志）。 |
| 仓储层失败 | 500 | `INTERNAL` | 区块隐藏（静默）。 |
| `limit` 越界 | 200（已收敛） | — | 无。 |

## 6. 部署

无 DDL、无新服务或环境变量。Redis 用量：每个被浏览的（模板, limit）
组合 1 个字符串 key、5 分钟 TTL、每个几 KB —— 可忽略。

## 7. SDK

无需 SDK 变更（rail 是 Web 界面）。可选：在 SDK README 的端点清单中
记录这两个 GET 端点 —— `client` 的 kwargs 透传让它们今天就能用。

## 8. 测试策略

### 8.1 FVT（`tests/fvt.py`）— `test_related_templates`

夹具：alice 建 "Customer Support" 分类簇（3+ 个模板）、"writing" 共享
标签簇、语言簇和若干其他模板；bob 在同分类建模板；carol（被 admin
封禁）有公共模板；一个私有模板。

1. related rail 返回最多 8 个公共模板，排序：同分类 > 仅共享标签 >
   仅同语言 > 无关；作者自己的模板出现但不主导（0.5 权重）。
2. 平分决胜按 like_count DESC，再 updated_at DESC。
3. 当前模板绝不在自己的 rail 里；私有模板绝不出现；封禁 carol 的模板
   绝不出现。
4. 榜首条目同分类时 `basis` 为 `"category"`；标签驱动时 `"tag"`；否则
   `"generic"`。
5. 候选少于 3 个 → `items: []`（200，不是错误）。
6. more-from-author：最多 4 个、点赞优先、排除当前模板、仅公共；作者
   没有其他模板时为空。
7. 不存在 id、私有 id（匿名、非所有者、所有者）、封禁所有者公共 id 均
   404。
8. limit 收敛：`limit=100` → 最多 8 条；related `limit=1` → 仍 ≥3 或
   空（收敛到 3）。
9. TTL 内两次相同调用返回一致响应（确定性）。
10. 无效 Bearer token → 401。

### 8.2 单元测试

- 可见性门：私有 → 所有者和非所有者都 404；公共通过。
- limit 收敛边界。
- basis 推导（category/tag/generic、空分类边界）。
- 缓存：命中返回缓存；Redis 错误落回仓储；写失败被吞掉。
- 仓储：两个查询的参数个数/顺序；空标签边界（`tags && '{}'` 为 false
  —— 0 分、不报错）。
- `MockTemplateRepository`：加 `ListRelatedFunc`/`ListMoreFromAuthorFunc`
  钩子（沿用既有 `GetFunc` 模式）。

### 8.3 E2E（test-agent，Nightwatch）

- 公共模板页 rail 出现在内容下方；私有（所有者视图）与 <3 候选时缺失。
- 骨架屏 → 加载完成切换；主内容无布局抖动。
- 卡片点击跳转到目标详情页。
- 作者行：排序、作者链接目标。
- 键盘：Tab 能到达每张卡片；区块 `aria-label` 存在。
- 所有 `related.*` key 的 EN/ZH 文案；分类/标签名不翻译。

## 9. 实施顺序

1. Proto RPC + 消息；重新生成。
2. 仓储：`ListRelated` + `ListMoreFromAuthor` + 单元测试。
3. 服务：`related.go`（门、收敛、缓存、basis）+ 单元测试。
4. 网关：两个 GET 分支。
5. 前端：api client → 组件 → TemplateDetails 挂载 → i18n。
6. FVT + E2E。

## 10. 开发者约束

1. **绝不提交 `backend/server`** —— 被跟踪的构建产物；若被修改，
   `git checkout -- backend/server`。
2. 只 stage 自己的文件；绝不 `git add -A`。
3. 无 DDL 变更 —— 两份 init.sql 保持不动。
4. 私有模板在两个端点上对**所有人** 404，包括所有者（rail 仅用于公共
   发现）。
5. 所有 SQL 值都是绑定参数；basis 字符串是服务端常量，绝非客户端输入。
6. Redis 故障记日志并吞掉 —— 绝不因缓存问题让 rail 请求失败。
7. 不要动 `Template` proto 字段 18/19（follow-creators）或 20–22
   （template-ratings）。
8. rail 出错时静默隐藏 —— 无横幅、无重试。
9. `template.visibility !== 'public'` 时前端不得调用端点。
10. 绝无任何形式的个性化信号；无追踪；无商业化界面。
