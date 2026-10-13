# 模板搜索与筛选 — 详细设计

| | |
|---|---|
| **Feature ID** | template-search-filtering |
| **Version** | 1.0 |
| **Status** | Approved for implementation（已批准实施） |
| **Author** | Architect Agent |
| **Upstream** | [架构设计](template-search-filtering-architecture-zh-CN.md) |

## 1. Proto 契约

`backend/api/proto/v1/prompt.proto` 新增内容：

```protobuf
message ListTemplatesRequest {
    // ... 既有字段 1-9（page_size、page_token、visibility、
    //     owner_id、category、tags、my_likes、my_favorites、language）...
    // 自由文本关键词；对 title、description、category、tags、
    // 最新版本 content 做不区分大小写匹配。
    string keyword = 10;
    // 排序：newest（默认）| most_liked | most_favorited | trending。
    // 非法值在服务端静默回退 newest。
    string sort = 11;
}

message ListTemplatesResponse {
    repeated Template templates = 1; // 混合视图时承载公共模板
    string next_page_token = 2; // 混合视图时承载公共分页 token
    repeated Template private_templates = 3;
    string private_next_page_token = 4;
    // 当前筛选条件下公共/单流的匹配总数。
    int32 total_count = 5;
    // 私有流匹配总数（仅混合视图填充；其余为 0）。
    int32 private_total_count = 6;
}
```

计数字段用 `int32`，遵循项目约定（protojson 会把 `int64` 序列化成字符串，
破坏 JS 客户端）。

重新生成：

```bash
cd backend && protoc --go_out=. --go-grpc_out=. api/proto/v1/prompt.proto
```

## 2. 数据库 DDL

追加到 `deployment/init.sql` 与 `deployment/chart/open-prompts/init.sql`
**两份文件**：

```sql
-- ===== template-search-filtering =====
-- 趋势排序（7 天互动窗口）的性能索引。
-- 本特性不需要任何表或列变更。
CREATE INDEX IF NOT EXISTS idx_template_likes_template_created
    ON template_likes(template_id, created_at);
CREATE INDEX IF NOT EXISTS idx_template_favorites_template_created
    ON template_favorites(template_id, created_at);
```

## 3. 后端分层

### 3.1 `TemplateRepository`（`backend/internal/repository/template_repository.go`）

**重构 — 共享条件构建器。** 把 `List` 中既有的筛选拼装块（visibility、
owner_id、category、language、tags、my_likes、my_favorites、封禁用户排除）
抽取为：

```go
func (r *templateRepository) buildTemplateConditions(
    filters map[string]interface{}, args []interface{}, argID int,
) (string, []interface{}, int)
```

`List` 与新方法 `CountTemplates` 都调用它，从结构上保证计数与结果永不
分叉。

**关键词条件**（构建器在 `filters["keyword"]` 为非空字符串时追加）：

```go
kw := filters["keyword"].(string)
pattern := "%" + escapeLike(kw) + "%"
query += fmt.Sprintf(`
    AND (
        t.title ILIKE $%d
        OR t.description ILIKE $%d
        OR t.category ILIKE $%d
        OR EXISTS (SELECT 1 FROM unnest(t.tags) AS tg WHERE tg ILIKE $%d)
        OR EXISTS (SELECT 1 FROM template_versions tv
                   WHERE tv.id = (SELECT tv2.id FROM template_versions tv2
                                  WHERE tv2.template_id = t.id
                                  ORDER BY tv2.version DESC LIMIT 1)
                     AND tv.content ILIKE $%d)
    )`, argID, argID, argID, argID, argID)
args = append(args, pattern, pattern, pattern, pattern, pattern)
argID++
```

`escapeLike` 转义 `\`、`%`、`_`（各加反斜杠前缀），用户输入永远按字面量
匹配。`t.description` 可空 —— `NULL ILIKE` 结果为 NULL，OR 链按不命中
处理，无需 `COALESCE`。

**排序白名单**（替换硬编码的 `ORDER BY t.created_at DESC`）：

```go
func templateOrderBy(sort string) string {
    switch sort {
    case "most_liked":
        return " ORDER BY t.like_count DESC, t.created_at DESC"
    case "most_favorited":
        return " ORDER BY t.favorite_count DESC, t.created_at DESC"
    case "trending":
        return " ORDER BY (COALESCE(el.cnt, 0) + COALESCE(ef.cnt, 0)) DESC, t.created_at DESC"
    default: // "newest" 及一切非法值
        return " ORDER BY t.created_at DESC"
    }
}
```

**趋势 JOIN** —— 仅当 `sort == "trending"` 时加入 FROM 子句（其余查询与
今天逐字节一致）：

```sql
LEFT JOIN (SELECT template_id, COUNT(*) AS cnt FROM template_likes
           WHERE created_at > now() - interval '7 days'
           GROUP BY template_id) el ON el.template_id = t.id
LEFT JOIN (SELECT template_id, COUNT(*) AS cnt FROM template_favorites
           WHERE created_at > now() - interval '7 days'
           GROUP BY template_id) ef ON ef.template_id = t.id
```

7 天窗口是 Go 常量（`trendingWindow = 7 * 24 * time.Hour`），以绑定参数
形式传入，不拼接 SQL 文本。

**新方法：**

```go
CountTemplates(ctx context.Context, filters map[string]interface{}) (int32, error)
```

`SELECT COUNT(*)`，FROM/JOIN/WHERE 由共享构建器产出（排序为 trending 时
同样带上趋势 JOIN —— 不影响计数，但原样复用构建器更简单且安全）。

**接口新增**（并在 `backend/internal/service/prompt_test.go` 的
`MockTemplateRepository` 加桩）：

```go
type TemplateRepository interface {
    // ... 既有方法 ...
    CountTemplates(ctx context.Context, filters map[string]interface{}) (int32, error)
}
```

### 3.2 `PromptService`（`backend/internal/service/prompt.go`）

**归一化 helper**（包级函数）：

```go
// normalizeKeyword：trim、折叠内部空白、按 100 rune 截断（不是字节 ——
// CJK 安全）。结果为空表示"无关键词筛选"。
func normalizeKeyword(s string) string

// normalizeSort：返回白名单内的排序值；其余一律 -> "newest"。
func normalizeSort(s string) string
```

**`ListTemplates` 变更** —— 应用到**全部四条**路径（my-likes/my-favorites、
匿名、指定可见性、混合视图）：

1. `keyword := normalizeKeyword(req.Keyword)`；非空则设
   `filters["keyword"] = keyword`。
2. `sort := normalizeSort(req.Sort)`；设 `filters["sort"] = sort`。
3. `fetch` 闭包把 `filters` 原样传给 `TemplateRepo.List`（签名不变）。
4. **计数**：每次 `fetch` 后，用同一 filters map（去掉构建器不用于条件的
   `current_user_id`）调用 `TemplateRepo.CountTemplates` 填充
   `total_count`。混合视图路径中：公共 filters 计数 → `total_count`，私有
   filters 计数 → `private_total_count`。my-likes/my-favorites、匿名、指定
   可见性路径中 `private_total_count` 保持 0。
5. **可观测性**（NFR-9）：在既有入口日志上追加 `keyword_len=%d sort=%s` ——
   绝不记关键词内容。

计数查询仅在请求为某查询的第一页（`page_token == ""`）时执行；后续页复用
客户端已有的计数。这把成本限制为每次查询至多一次额外 `COUNT(*)`（NFR-4）。

### 3.3 网关（`backend/cmd/server/main.go`）

在 `/api/v1/templates` 处理器的 `GET` 分支、既有查询参数映射之后：

```go
req.Keyword = q.Get("keyword")
if req.Keyword == "" {
    req.Keyword = q.Get("q") // 前端 URL 契约使用的别名
}
req.Sort = q.Get("sort")
```

此处不做校验 —— 归一化在服务层完成。错误面不变，仍由 `writeError` 处理。

## 4. 前端

### 4.1 新增文件

| 文件 | 用途 |
|---|---|
| `frontend/src/hooks/useTemplateFilters.js` | 状态所有者：keyword/category/tags/sort/visibility（+ 瞬态 `my_likes`/`my_favorites`/`owner_id`）、URL 同步、防抖、竞态守卫、取数编排、结果计数。 |
| `frontend/src/components/SearchBar.js` + `.scss` | `role="search"` 输入框、300ms 防抖、Enter 提交、清除按钮、建议下拉的 combobox a11y。 |
| `frontend/src/components/SortDropdown.js` + `.scss` | Aurora 风格自定义下拉，4 个选项，listbox 键盘模式（↑↓ Enter Esc）。 |
| `frontend/src/components/FilterChipRow.js` + `.scss` | 每个激活筛选一个可移除 chip + "全部清除"。 |
| `frontend/src/components/SuggestionsDropdown.js` + `.scss` | 最多 5 行（先分类后标签），数据来自共享分类法；combobox 模式。 |
| `frontend/src/components/ResultCount.js` | `aria-live="polite"` 计数；不可用时显示 "—"。 |
| `frontend/src/components/EmptyState.js` + `.scss` | 搜索变体：回显关键词、引导文案、"清除全部筛选"。 |
| `frontend/src/components/SkeletonCards.js` + `.scss` | 8 张 shimmer 占位卡（纯 CSS 关键帧）。 |

### 4.2 修改文件

| 文件 | 变更 |
|---|---|
| `frontend/src/pages/Home.js` | 本地筛选/取数状态替换为 `useTemplateFilters`；渲染搜索行（SearchBar + SortDropdown）、FilterChipRow、ResultCount、SkeletonCards/EmptyState/错误横幅；保留无限滚动与混合视图合并逻辑。 |
| `frontend/src/components/Sidebar.js` | 标签行 → 多选复选框（在 `tags` 数组中切换）；分类法数据（分类/标签）提升到 Home 下传，SuggestionsDropdown 共享数据、不重复请求；分类点击仍重置标签。 |
| `frontend/src/components/Layout.js` | 把分类法数据传给子组件（其余 props 形状不变）。 |
| `frontend/src/services/api.js` | 无需变更 —— `getTemplates(params)` 本就透传任意参数；调用方加上 `keyword`/`sort` 即可。 |
| `frontend/src/locales/{en,zh}/translation.json` | 新增 `search.*` 命名空间（§4.4）。 |

### 4.3 `useTemplateFilters` 契约

```js
const {
  filters,          // { keyword, category, tags, sort, visibility,
                    //   myLikes, myFavorites, ownerId }
  setKeyword,       // 300ms 防抖；Enter 通过 submitKeyword() 直达
  submitKeyword,
  toggleTag, setCategory, setSort, setVisibility,
  clearAll,        // 重置 keyword/category/tags/sort；visibility -> 持久化默认值
  templates, privateTemplates,
  totalCount,       // total_count + private_total_count
  nextPageToken, hasMore, loadingMore, isFirstLoading, error,
  loadMore, retry,
} = useTemplateFilters({ pageSize: 20 });
```

URL 契约（主页路由）：`?q=`、`?category=`、`?tags=a,b`、`?sort=`、
`?visibility=`。关键词输入 → `setSearchParams(..., { replace: true })`；
离散变更 → push。加载时恢复顺序：URL → localStorage 可见性 → 登录态默认。
每个请求携带单调递增 `requestId`；过期 ID 的响应直接丢弃。任何筛选变更都
重置分页。失败的查询保留最后一次成功结果继续渲染，并暴露 `error`/`retry`。

### 4.4 i18n Key 清单（中英同步）

英文（`frontend/src/locales/en/translation.json`）：

```json
{
  "search": {
    "placeholder": "Search prompts…",
    "clear": "Clear",
    "sort": {
      "label": "Sort",
      "newest": "Newest",
      "most_liked": "Most Liked",
      "most_favorited": "Most Favorited",
      "trending": "Trending"
    },
    "chips": {
      "clear_all": "Clear all",
      "remove_filter": "Remove filter: {{label}}"
    },
    "results_count": "{{count}} template",
    "results_count_other": "{{count}} templates",
    "suggestions": {
      "category": "Category",
      "tag": "Tag",
      "none": "No suggestions"
    },
    "empty": {
      "title": "No results for \"{{keyword}}\"",
      "body": "Try different keywords or clear filters.",
      "clear_all": "Clear all filters"
    },
    "error": {
      "fetch": "Search failed. Please try again.",
      "retry": "Retry"
    }
  }
}
```

中文（`frontend/src/locales/zh/translation.json`）：`search.placeholder` =
"搜索提示词…"、`search.clear` = "清除"、`search.sort.label` = "排序"、
`search.sort.newest` = "最新"、`search.sort.most_liked` = "最多点赞"、
`search.sort.most_favorited` = "最多收藏"、`search.sort.trending` = "趋势"、
`search.chips.clear_all` = "全部清除"、`search.chips.remove_filter` =
"移除筛选：{{label}}"、`search.results_count` / `search.results_count_other`
= "{{count}} 个模板"、`search.suggestions.category` = "分类"、
`search.suggestions.tag` = "标签"、`search.suggestions.none` = "暂无建议"、
`search.empty.title` = "没有找到 \"{{keyword}}\" 的结果"、`search.empty.body`
= "试试其他关键词或清除筛选。"、`search.empty.clear_all` = "清除全部筛选"、
`search.error.fetch` = "搜索失败，请重试。"、`search.error.retry` = "重试"。
`results_count` 使用 i18next 复数机制；中文两种形式用同一字符串。排序 API
值是 ASCII slug，绝不使用本地化标签。

## 5. 错误矩阵

| 场景 | HTTP | gRPC | 前端处理 |
|---|---|---|---|
| Bearer token 无效 | 401 | `UNAUTHENTICATED` | 既有可选鉴权行为（网关在服务前拒绝）。 |
| `sort` 非法值 | 200 | — | 服务端静默回退 `newest`；UI 绝不发送非法值。 |
| 关键词 > 100 rune | 200 | — | 服务端截断；输入框 `maxLength=100` 在客户端预防。 |
| 纯空白关键词 | 200 | — | 视为无关键词筛选。 |
| 仓储层失败 | 500 | `INTERNAL` | 错误横幅 + 重试；最后一次成功结果保留渲染。 |
| 空结果集 | 200 | — | EmptyState 回显关键词 + "清除全部筛选"。 |

## 6. 部署

- 两个幂等索引；两份 init.sql 同一提交内同步。
- 无新服务、端口、环境变量或 Redis key。
- 前端构建配置无变更。

## 7. SDK

`python-sdk/openprompts/client.py` 的 `list_templates(**kwargs)` 本就透传
任意查询参数 —— `keyword=` 与 `sort=` **零 SDK 变更**即可使用。SDK README
的参数列表补两行文档即可。

## 8. 测试策略

### 8.1 FVT（`tests/fvt.py`）— `test_template_search_filtering`

夹具：alice 创建标题/描述/标签/分类各不相同的模板并带多个版本（旧版本含
最新版本没有的标记词）；bob 按可控时间戳点赞/收藏（趋势通过比较近期与历史
互动的相对顺序验证）。

1. 关键词命中标题（不区分大小写）、描述、分类、标签值、最新版本内容；
   仅存在于**旧**版本的标记词**不**命中。
2. CJK 子串匹配（如 "邮件" 命中标题含"邮件营销"的模板）。
3. 多词短语："email marketing" 只命中包含该完整短语的字段；只含 "email"
   的字段不命中。
4. LIKE 元字符：关键词 `100%` 只匹配字面 "100%" 文本，不匹配 "100x"；
   `_` 同理按字面量。
5. 多标签 AND：`tags=a&tags=b` 只返回同时含两标签的模板。
6. 排序：预置点赞/收藏数 → 验证 `most_liked`/`most_favorited` 顺序；
   `trending` 按 7 天窗口排序（在数据允许时，历史总量高但近期零互动的模板
   排在总量低但近期有互动的模板之后）。
7. `total_count` 等于匹配模板数；带 `keyword`+`sort` 分页时两者保持；
   `private_total_count` 仅混合视图填充。
8. 匿名搜索绝不返回私有模板；登录混合视图返回自己的私有命中（AC-7/AC-8）。
9. `sort=bogus` → 200 且按 newest 排序（AC-9）；不发送
   `keyword`/`sort` → 响应形状与特性前一致（仅多出计数字段）。
10. 纯空白关键词 = 无筛选；101 rune 关键词截断到 100。
11. 封禁用户的公共模板不出现在关键词结果中。
12. `my_likes=true&keyword=...` 正确组合。

### 8.2 单元测试

- `normalizeKeyword`：trim、空白折叠、100 rune 截断（CJK 按 rune 而非
  字节）、空结果。
- `normalizeSort`：白名单 + 回退。
- 仓储构建器：带/不带关键词的参数编号、转义、排序分支选择、趋势 JOIN 仅
  在 trending 时出现。
- `MockTemplateRepository.CountTemplates` 桩接入服务测试；混合视图计数
  填充。

### 8.3 E2E（test-agent，Nightwatch）

- SearchBar：防抖（每 300ms ≤1 个请求）、Enter 直达、清除按钮。
- 建议：≥2 字符出现、先分类后标签、选中即应用筛选、combobox a11y 属性。
- SortDropdown：键盘导航、选择后更新 URL `?sort=`。
- Chips：逐个移除、全部清除、焦点管理。
- URL：深链恢复、前进/后退、replace 与 push 行为。
- ResultCount 更新；新查询时骨架屏；空态；错误+重试保留旧结果。
- i18n 中英 `search.*` key 一致。

## 9. 实施顺序

1. Proto 字段 + 重新生成。
2. 仓储：条件构建器重构、关键词条件、排序白名单、趋势 JOIN、
   `CountTemplates` + 单元测试。
3. 服务：归一化、四条路径接入、计数、日志。
4. 网关：`keyword`/`q`/`sort` 映射。
5. DDL 索引（两份 init.sql）。
6. 前端：`useTemplateFilters` → 组件 → Home/Sidebar 接线 → i18n。
7. FVT + E2E。

## 10. 给开发者的约束

1. **绝不提交 `backend/server`** —— 它是被跟踪的构建产物；若被修改，
   `git checkout -- backend/server` 还原。
2. 只暂存自己的文件；绝不 `git add -A`。
3. 两份 init.sql 同一提交内同步；DDL 幂等（`IF NOT EXISTS`）。
4. 排序值通过白名单 switch 映射到硬编码 SQL —— 绝不把用户输入拼进
   `ORDER BY`。
5. 构造 ILIKE 模式前转义关键词中的 `%`/`_`/`\`。
6. 关键词上限是 100 **rune**，不是字节（CJK 安全）。
7. `total_count`/`private_total_count` 在 proto 中是 `int32`。
8. 计数仅在首页请求（`page_token == ""`）时计算。
9. 混合视图 token 格式 `public:private` 与全部既有参数必须原样保持工作
   （AC-10）。
10. 日志只记关键词长度，绝不记内容。
11. 不新增端点；除两个索引外无 schema 变更。
12. 绝不出现任何形式的商业化界面。
