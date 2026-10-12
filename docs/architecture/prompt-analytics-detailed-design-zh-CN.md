# 提示词分析 — 详细设计

| | |
|---|---|
| **Feature ID** | prompt-analytics |
| **Doc Type** | 详细设计 |
| **Version** | 1.0 |
| **Status** | Ready for implementation |
| **Author** | Architect Agent |
| **Upstream** | `docs/design/prompt-analytics-requirements-zh-CN.md`、`docs/design/prompt-analytics-ui-ux-design-zh-CN.md` |
| **Downstream** | 开发 Agent（实现）、测试 Agent（FVT/E2E） |
| **Companion** | `prompt-analytics-architecture-zh-CN.md`（中文）、`prompt-analytics-architecture.md` / `prompt-analytics-detailed-design.md`（英文） |

---

## 1. Proto 契约

所有新增内容进入 `backend/api/proto/v1/prompt.proto` 的 `PromptService`。

### 1.1 RPC（追加到 `service PromptService`）

```proto
  // RecordTemplateEvent 记录模板的浏览事件（允许匿名；按 IP 限流）。
  // use 事件仅由服务端记录。
  rpc RecordTemplateEvent(RecordTemplateEventRequest) returns (RecordTemplateEventResponse);

  // GetTemplateStats 返回单个模板的分析数据（仅作者）。
  rpc GetTemplateStats(GetTemplateStatsRequest) returns (GetTemplateStatsResponse);

  // GetCreatorStats 返回调用者全部模板的聚合分析数据。
  rpc GetCreatorStats(GetCreatorStatsRequest) returns (GetCreatorStatsResponse);
```

### 1.2 `Template` 消息新增字段

字段号 **23 与 24** —— 18/19 由 follow-creators 预留
（`owner_display_name`/`owner_avatar`），20–22 由 template-ratings 预留
（`rating_avg`/`rating_count`/`my_rating`）。不得复用。

```proto
  // 去重后的总浏览量（每会话每 30 分钟一次）。
  int32 view_count = 23;
  // 从该模板创建的提示词总数（web + API）。
  int32 use_count = 24;
```

两者均为 `int32`（protojson 会把 `int64` 序列化为 JSON 字符串；前端与
FVT 以 JSON 数字比较计数）。

### 1.3 新消息

```proto
// RecordTemplateEventRequest 记录客户端浏览事件。
message RecordTemplateEventRequest {
  // 事件所属模板（必填）。
  string template_id = 1;
  // 事件类型；v1 仅接受 "view"（必填）。
  string event_type = 2;
  // 用于去重的客户端会话 id（可选；为空则不去重）。
  string session_id = 3;
}

message RecordTemplateEventResponse {}

// TemplateStatsSummary 是四项指标在某个范围内的总量。
message TemplateStatsSummary {
  int32 views = 1;
  int32 uses = 2;
  int32 likes = 3;
  int32 favorites = 4;
}

// DailyMetricPoint 是每日序列中的一个 UTC 日。
message DailyMetricPoint {
  // 日期，格式 YYYY-MM-DD（UTC）。
  string date = 1;
  int32 views = 2;
  int32 uses = 3;
  int32 likes = 4;
  int32 favorites = 5;
}

// SourceSplit 按来源拆分 use 事件。
message SourceSplit {
  int32 web = 1;
  int32 api = 2;
}

message GetTemplateStatsRequest {
  string template_id = 1;
  // 7、30 或 90（必填；其他值为 InvalidArgument）。
  int32 days = 2;
}

message GetTemplateStatsResponse {
  // 所请求范围的总量。
  TemplateStatsSummary current = 1;
  // 等长前一范围的总量（增量基线）。
  TemplateStatsSummary previous = 2;
  // 零填充的每日序列，恰好 `days` 个点，最旧在前。
  repeated DailyMetricPoint series = 3;
  // 所请求范围的 use 事件来源拆分。
  SourceSplit source_split = 4;
  // 范围内 uses / views；views == 0 时为 0。
  double view_to_use_rate = 5;
}

// TopTemplateRow 是 Top 模板表中的一行。
message TopTemplateRow {
  string id = 1;
  string title = 2;
  int32 views = 3;
  int32 uses = 4;
  int32 likes = 5;
  int32 favorites = 6;
  // 范围内浏览→使用率（uses / views；views == 0 时为 0）。
  double rate = 7;
}

message GetCreatorStatsRequest {
  // 7、30 或 90（必填；其他值为 InvalidArgument）。
  int32 days = 1;
}

message GetCreatorStatsResponse {
  TemplateStatsSummary current = 1;
  TemplateStatsSummary previous = 2;
  repeated DailyMetricPoint series = 3;
  SourceSplit source_split = 4;
  // 调用者模板按范围内 uses 排序的 Top 10（并列时按 views）。
  repeated TopTemplateRow top_templates = 5;
  // 调用者拥有的模板总数（驱动空状态）。
  int32 template_count = 6;
}
```

### 1.4 JSON 线上形态（网关 marshaler：`EmitUnpopulated` + `UseProtoNames`）

- `GET /api/v1/templates/{id}/stats?days=30` →

```json
{
  "current": {"views": 120, "uses": 30, "likes": 8, "favorites": 3},
  "previous": {"views": 90, "uses": 20, "likes": 5, "favorites": 1},
  "series": [{"date": "2025-01-01", "views": 12, "uses": 3, "likes": 1, "favorites": 0}, "..."],
  "source_split": {"web": 22, "api": 8},
  "view_to_use_rate": 0.25
}
```

- `GET /api/v1/analytics/overview?days=30` → 同上形态，另加
  `"top_templates": [{"id": "...", "title": "...", "views": 40, "uses": 10, "likes": 2, "favorites": 1, "rate": 0.25}, "..."]`
  与 `"template_count": 12`。

---

## 2. 数据库 DDL

应用于**两份** `deployment/init.sql` 与
`deployment/chart/open-prompts/init.sql`（字节一致的块），置于既有
`template_comments` 小节之后。

```sql
-- ===== Prompt Analytics =====
CREATE TABLE IF NOT EXISTS template_usage_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL CHECK (event_type IN ('view', 'use')),
    source TEXT NOT NULL CHECK (source IN ('web', 'api')),
    user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    session_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tue_template_created
    ON template_usage_events(template_id, created_at);
CREATE INDEX IF NOT EXISTS idx_tue_user_created
    ON template_usage_events(user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_tue_view_dedup
    ON template_usage_events(template_id, session_id, created_at)
    WHERE event_type = 'view';

COMMENT ON TABLE template_usage_events IS 'Per-template view/use events for analytics';
COMMENT ON COLUMN template_usage_events.event_type IS 'view | use';
COMMENT ON COLUMN template_usage_events.source IS 'web | api (origin of the event)';
COMMENT ON COLUMN template_usage_events.session_id IS 'Client session id (view dedup); NULL for use events';

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'templates' AND column_name = 'view_count') THEN
        ALTER TABLE templates ADD COLUMN view_count INT NOT NULL DEFAULT 0;
        COMMENT ON COLUMN templates.view_count IS 'Denormalized view count (async maintained)';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name = 'templates' AND column_name = 'use_count') THEN
        ALTER TABLE templates ADD COLUMN use_count INT NOT NULL DEFAULT 0;
        COMMENT ON COLUMN templates.use_count IS 'Denormalized use count (async maintained)';
    END IF;
END $$;

-- 回填（D-2）：每行 prompts 一条历史 use/web 事件。逐行 NOT EXISTS
-- 守卫使重跑与线上事件并存都安全。先部署 DDL 再上新二进制，
-- 守卫即可恰好导入一次历史数据。
INSERT INTO template_usage_events (template_id, event_type, source, user_id, created_at)
SELECT p.template_id, 'use', 'web', p.owner_id, p.created_at
FROM prompts p
WHERE NOT EXISTS (
    SELECT 1 FROM template_usage_events e
    WHERE e.event_type = 'use' AND e.template_id = p.template_id
      AND e.user_id IS NOT DISTINCT FROM p.owner_id
      AND e.created_at = p.created_at
);

-- 从事件表收敛反规范化计数器（幂等）。
UPDATE templates t
SET use_count = sub.c
FROM (SELECT template_id, COUNT(*) AS c
      FROM template_usage_events WHERE event_type = 'use'
      GROUP BY template_id) sub
WHERE t.id = sub.template_id AND t.use_count <> sub.c;
```

`view_count` 无历史来源，从 0 开始（架构文档中已接受）。

---

## 3. 后端分层

### 3.1 `models.Template`（`backend/internal/models/template.go`）

```go
ViewCount int32 // 反规范化；由事件写路径维护
UseCount  int32 // 反规范化；由事件写路径 + 回填维护
```

`templateModelToProto`（`backend/internal/service/prompt.go`）将两者映射到
`Template.view_count` / `Template.use_count`。所有为 proto 映射装配
`models.Template` 的 `SELECT` 必须包含这两个新列
（`template_repository.go` 中 `Get`/`List`/`GetByAlias` 的列清单及对应的
`Scan`）。

### 3.2 `AnalyticsRepository`（新增 `backend/internal/repository/analytics_repository.go`）

```go
type AnalyticsRepository struct{ db *sql.DB }

func NewAnalyticsRepository(db *sql.DB) *AnalyticsRepository

// InsertEvent 追加一条使用事件。
func (r *AnalyticsRepository) InsertEvent(ctx context.Context, templateID, eventType, source, userID, sessionID string) error

// RecentViewExists 报告 (模板, 会话) 在窗口内是否已有 view 事件。
// Redis 去重键的 DB 回退。
func (r *AnalyticsRepository) RecentViewExists(ctx context.Context, templateID, sessionID string, window time.Duration) (bool, error)

// IncrementCounter 自增反规范化计数器。column 由调用方白名单
//（"view_count" | "use_count"）；绝不来自输入插值。
func (r *AnalyticsRepository) IncrementCounter(ctx context.Context, templateID, column string) error

// DailyEventSeries 返回按日的浏览/使用计数。templateID / ownerID
// 恰有一个非空（SQL 中跳过 NULL 范围）。
func (r *AnalyticsRepository) DailyEventSeries(ctx context.Context, templateID, ownerID string, from, to time.Time) (map[string][2]int32, error)

// DailyLikeSeries / DailyFavoriteSeries 返回按日的点赞/收藏计数。
func (r *AnalyticsRepository) DailyLikeSeries(ctx context.Context, templateID, ownerID string, from, to time.Time) (map[string]int32, error)
func (r *AnalyticsRepository) DailyFavoriteSeries(ctx context.Context, templateID, ownerID string, from, to time.Time) (map[string]int32, error)

// UseSourceSplit 返回窗口内按来源的 use 事件计数。
func (r *AnalyticsRepository) UseSourceSplit(ctx context.Context, templateID, ownerID string, from, to time.Time) (web, api int32, err error)

// TopTemplatesByUses 返回窗口内作者的 Top 模板。
func (r *AnalyticsRepository) TopTemplatesByUses(ctx context.Context, ownerID string, from, to time.Time, limit int) ([]models.TopTemplateRow, error)

// CountOwnerTemplates 返回用户拥有的模板数。
func (r *AnalyticsRepository) CountOwnerTemplates(ctx context.Context, ownerID string) (int32, error)
```

`models.TopTemplateRow`：

```go
type TopTemplateRow struct {
    ID         string
    Title      string
    Views      int32
    Uses       int32
    Likes      int32
    Favorites  int32
}
```

**各方法的 SQL**（全部参数化；`$3::uuid`/`$4::text` 的 NULL 参数用于关闭
范围限定）：

`InsertEvent`：

```sql
INSERT INTO template_usage_events (template_id, event_type, source, user_id, session_id)
VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''));
```

`RecentViewExists`：

```sql
SELECT 1 FROM template_usage_events
WHERE template_id = $1 AND session_id = $2 AND event_type = 'view'
  AND created_at > NOW() - $3::interval
LIMIT 1;
```

`IncrementCounter`（column 是调用点的编译期常量）：

```sql
UPDATE templates SET view_count = view_count + 1 WHERE id = $1;   -- 或 use_count
```

`DailyEventSeries`：

```sql
SELECT to_char(date_trunc('day', e.created_at AT TIME ZONE 'UTC'), 'YYYY-MM-DD') AS day,
       COUNT(*) FILTER (WHERE e.event_type = 'view')::int AS views,
       COUNT(*) FILTER (WHERE e.event_type = 'use')::int AS uses
FROM template_usage_events e
JOIN templates t ON t.id = e.template_id
WHERE e.created_at >= $1 AND e.created_at < $2
  AND ($3::uuid IS NULL OR e.template_id = $3)
  AND ($4::text  IS NULL OR t.owner_id = $4)
GROUP BY 1;
```

`DailyLikeSeries`（favorites 与 `template_favorites` 同理）：

```sql
SELECT to_char(date_trunc('day', l.created_at AT TIME ZONE 'UTC'), 'YYYY-MM-DD') AS day,
       COUNT(*)::int AS likes
FROM template_likes l
JOIN templates t ON t.id = l.template_id
WHERE l.created_at >= $1 AND l.created_at < $2
  AND ($3::uuid IS NULL OR l.template_id = $3)
  AND ($4::text  IS NULL OR t.owner_id = $4)
GROUP BY 1;
```

`UseSourceSplit`：

```sql
SELECT e.source, COUNT(*)::int
FROM template_usage_events e
JOIN templates t ON t.id = e.template_id
WHERE e.event_type = 'use' AND e.created_at >= $1 AND e.created_at < $2
  AND ($3::uuid IS NULL OR e.template_id = $3)
  AND ($4::text  IS NULL OR t.owner_id = $4)
GROUP BY e.source;
```

`TopTemplatesByUses`：

```sql
WITH ev AS (
    SELECT template_id,
           COUNT(*) FILTER (WHERE event_type = 'view')::int AS views,
           COUNT(*) FILTER (WHERE event_type = 'use')::int AS uses
    FROM template_usage_events
    WHERE created_at >= $2 AND created_at < $3
    GROUP BY template_id
), lk AS (
    SELECT template_id, COUNT(*)::int AS likes FROM template_likes
    WHERE created_at >= $2 AND created_at < $3 GROUP BY template_id
), fv AS (
    SELECT template_id, COUNT(*)::int AS favorites FROM template_favorites
    WHERE created_at >= $2 AND created_at < $3 GROUP BY template_id
)
SELECT t.id, t.title,
       COALESCE(ev.views, 0), COALESCE(ev.uses, 0),
       COALESCE(lk.likes, 0), COALESCE(fv.favorites, 0)
FROM templates t
LEFT JOIN ev ON ev.template_id = t.id
LEFT JOIN lk ON lk.template_id = t.id
LEFT JOIN fv ON fv.template_id = t.id
WHERE t.owner_id = $1
  AND (ev.template_id IS NOT NULL OR lk.template_id IS NOT NULL OR fv.template_id IS NOT NULL)
ORDER BY COALESCE(ev.uses, 0) DESC, COALESCE(ev.views, 0) DESC
LIMIT $4;
```

`CountOwnerTemplates`：

```sql
SELECT COUNT(*)::int FROM templates WHERE owner_id = $1;
```

### 3.3 `PromptService`（`backend/internal/service/prompt.go`）

新增结构体字段（构造函数参数追加；`main.go` 负责接线）：

```go
AnalyticsRepo *repository.AnalyticsRepository
```

#### 3.3.1 `RecordTemplateEvent`（公开浏览 beacon）

```
func (s *PromptService) RecordTemplateEvent(ctx, req) (*pb.RecordTemplateEventResponse, error):
  1. req.TemplateId == ""                          → InvalidArgument "template id required"
  2. req.EventType != "view"                       → InvalidArgument "unsupported event type"
  3. 限流（checkEventRate，§3.3.3，键前缀 tpl_event_rate，
     环境变量 TEMPLATE_EVENT_RATE_PER_MINUTE 默认 60）超限
                                                    → ResourceExhausted "rate limited; retry after N seconds"
  4. tpl, err := s.TemplateRepo.Get(ctx, req.TemplateId)
     err                                            → NotFound "template not found"
  5. caller := GetUserFromContext(ctx)   // 匿名为 ""
     tpl.Visibility == PRIVATE && caller != tpl.OwnerID
                                                    → NotFound "template not found"   // 不泄露存在性
  6. if req.SessionId != "":
       去重（§3.3.4）。若为重复                       → 返回 &pb.RecordTemplateEventResponse{}, nil
  7. s.AnalyticsRepo.InsertEvent(ctx, templateID, "view", "web", caller, req.SessionId)
     err                                            → 记日志 + Internal（罕见；DB 宕机）
  8. s.AnalyticsRepo.IncrementCounter(ctx, templateID, "view_count")
     err                                            → 仅记日志，仍返回 OK
     // 事件表是事实来源；计数器漂移由再收敛修复（与回填同形的 UPDATE），
     // 可以接受。
  9. 返回空响应, nil
```

#### 3.3.2 `RecordUseEvent`（内部；非 RPC，无 proto）

```
func (s *PromptService) RecordUseEvent(ctx, templateID, userID, source string):
  1. if templateID == ""                            → return（记日志）
  2. if source != "web" && source != "api"          → return（记日志）
  3. err := s.AnalyticsRepo.InsertEvent(ctx, templateID, "use", source, userID, "")
     err                                            → 仅记日志
  4. err := s.AnalyticsRepo.IncrementCounter(ctx, templateID, "use_count")
     err                                            → 仅记日志
```

绝不返回错误；调用方以 `context.Background()` 在 goroutine 中运行它，
请求取消不会杀死写入。

#### 3.3.3 限流 —— `checkEventRate`（新增，`prompt.go` 或 `analytics.go`）

克隆 `checkCommentRate`（`comment.go:223`）：Redis 固定窗口，键
`tpl_event_rate:{clientIP}`；值 `"{count}:{expiresUnix}"`；窗口 60 秒；
限额取自 `TEMPLATE_EVENT_RATE_PER_MINUTE`（默认 60）。客户端 IP 来自
`x-client-ip` gRPC metadata（网关 `withClientIP`）。Redis 故障时
**fail-open**。超限返回 `ResourceExhausted`，消息为
`"rate limited; retry after N seconds"` —— 网关的 `writeError` 会把它解析
成 HTTP `Retry-After` 头。

#### 3.3.4 浏览去重

```
key := "tpl_view_dedup:" + templateID + ":" + sessionID
ok, err := redis.SetNX(ctx, key, "1", 30*time.Minute)
if err == nil:
    if !ok: 重复 → 跳过插入
else:  // Redis 宕机 → DB 回退
    exists, err2 := AnalyticsRepo.RecentViewExists(ctx, templateID, sessionID, 30*time.Minute)
    if err2 == nil && exists: 重复 → 跳过插入
    if err2 != nil: 继续（计数；短暂的重复计数可接受）
```

`session_id` 为空则完全跳过去重（总是计数）。

#### 3.3.5 `GetTemplateStats`

```
1. caller := GetUserFromContext(ctx); caller == ""  → Unauthenticated
2. req.Days ∈ {7, 30, 90} 否则                      → InvalidArgument "days must be 7, 30, or 90"
3. tpl, err := TemplateRepo.Get(ctx, req.TemplateId)
   err                                              → NotFound
4. tpl.OwnerID != caller:
     tpl.Visibility == PRIVATE                      → NotFound   // 不泄露存在性
     else                                           → PermissionDenied "not the template owner"
5. to := time.Now().UTC(); from := to.AddDate(0, 0, -days)
   prevTo := from; prevFrom := from.AddDate(0, 0, -days)
6. evCur  := AnalyticsRepo.DailyEventSeries(ctx,  templateID, "", from, to)
   lkCur  := AnalyticsRepo.DailyLikeSeries(ctx,    templateID, "", from, to)
   fvCur  := AnalyticsRepo.DailyFavoriteSeries(ctx, templateID, "", from, to)
   evPrev/lkPrev/fvPrev := 同上，用 (prevFrom, prevTo)
   web, api := AnalyticsRepo.UseSourceSplit(ctx, templateID, "", from, to)
7. series := zeroFill(days, evCur, lkCur, fvCur)    // 恰好 `days` 个点，最旧在前
   current := sum(series); previous := sum(evPrev, lkPrev, fvPrev)
   rate := current.Uses / current.Views（current.Views > 0 时），否则 0
8. 返回 GetTemplateStatsResponse{current, previous, series, source_split, view_to_use_rate}
```

#### 3.3.6 `GetCreatorStats`

与 §3.3.5 的第 1–2、5–8 步相同，但 `ownerID = caller`、
`templateID = ""`（owner 范围 SQL），另加：

```
top   := AnalyticsRepo.TopTemplatesByUses(ctx, caller, from, to, 10)
        // 行内 rate 在服务层计算：uses/views，views == 0 时为 0
count := AnalyticsRepo.CountOwnerTemplates(ctx, caller)
```

无需归属权检查 —— 范围本身就是调用者（FR-26：管理员也无跨用户访问）。

#### 3.3.7 `zeroFill` 辅助函数

```
for i := days - 1; i >= 0; i-- {
    day := from.AddDate(0, 0, i).Format("2006-01-02")   // UTC
    point := DailyMetricPoint{Date: day, views/likes/favorites 取自各 map}
}
```

范围总量是零填充序列之和（无需额外查询）。

### 3.4 `AuthInterceptor`（`backend/internal/service/auth.go`）

在 `NewAuthInterceptor` 的 `publicRpcMethods` 中加入：

```go
"/v1.PromptService/RecordTemplateEvent": true,
```

行为与网关一致：匿名放行；出现的 token 会被校验（无效 →
`Unauthenticated`）并注入用户 id 用于归属。`GetTemplateStats`/
`GetCreatorStats` 保持非公开。

### 3.5 网关（`backend/cmd/server/main.go`）

#### 3.5.1 `POST /api/v1/templates/{id}/events`

在既有的 `/api/v1/templates/` 处理器内，为 `parts[1] == "events"` 增加
分支（与 `aliases`、`likes`、`favorites`、`comments` 并列）：

```
case http.MethodPost:
  1. ctx := context.Background()
     若存在 Authorization 头:
         userID, err := authInterceptor.VerifyAnyToken(r.Context(), token)
         err                                          → 401 "Invalid token"
         ctx = service.ContextWithUserID(ctx, userID)
     // 无头 → 匿名（允许）
  2. 读取请求体；解析 {type, session_id}              → JSON 非法时 400
  3. req := &pb.RecordTemplateEventRequest{TemplateId: id,
         EventType: body.Type, SessionId: body.SessionID}
  4. resp, err := svc.RecordTemplateEvent(withClientIP(ctx, clientIP(r)), req)
     err → writeError(w, err)   // 400 / 404 / 429 + Retry-After / 500
  5. 200，响应体 {}（marshaler 输出 "{}"）
case 其他方法                                        → 405
```

#### 3.5.2 `GET /api/v1/templates/{id}/stats?days=30`

同一处理器，分支 `parts[1] == "stats"`：

```
case http.MethodGet:
  1. 需要 Authorization；VerifyAnyToken                → 缺失/无效时 401
  2. ctx = service.ContextWithUserID(context.Background(), userID)
  3. days := atoi(r.URL.Query().Get("days")); 缺失 → 30
  4. resp, err := svc.GetTemplateStats(ctx, &pb.GetTemplateStatsRequest{
         TemplateId: id, Days: int32(days)})
     err → writeError   // 400 非法 days / 401 / 403 非作者 / 404
  5. 200 JSON
case 其他方法                                         → 405
```

#### 3.5.3 `GET /api/v1/analytics/overview?days=30`（新处理器）

```
http.HandleFunc("/api/v1/analytics/overview", ...):
  1. CORS 预检（与其他处理器相同的头）
  2. method != GET                                     → 405
  3. 需要 Authorization；VerifyAnyToken                → 缺失/无效时 401
  4. ctx = service.ContextWithUserID(context.Background(), userID)
  5. days := atoi(query "days"); 缺失 → 30
  6. resp, err := svc.GetCreatorStats(ctx, &pb.GetCreatorStatsRequest{Days: int32(days)})
     err → writeError
  7. 200 JSON
```

#### 3.5.4 use 事件埋点（fire-and-forget）

**Web** —— 在既有 `POST /api/v1/prompts` 处理器中，`promptRepo.Create`
成功之后（该处理器绕过 `PromptService.CreatePrompt`；埋点在网关处理器，
不在服务里）：

```go
tplID, uid := prompt.TemplateID, userID // 在启动 goroutine 之前捕获
go func() {
    svc.RecordUseEvent(context.Background(), tplID, uid, "web")
}()
```

**API** —— 在既有 `GET /api/v1/templates/{id}/aliases/{alias}` 处理器中，
`GetPromptByAlias` 成功之后：

```go
authHeader := r.Header.Get("Authorization") // 在启动 goroutine 之前捕获
tplID := templateID
go func() {
    ctx := context.Background()
    uid := ""
    if authHeader != "" {
        parts := strings.SplitN(authHeader, " ", 2)
        if len(parts) == 2 {
            if id, err := authInterceptor.VerifyAnyToken(ctx, parts[1]); err == nil {
                uid = id
            }
        }
        // 校验失败被忽略：别名端点公开，事件仍以空 user_id 记录
    }
    svc.RecordUseEvent(ctx, tplID, uid, "api")
}()
```

两个埋点均不影响响应路径；`RecordUseEvent` 内部记日志（FR-7）。

---

## 4. 前端

### 4.1 新增文件

| 文件 | 用途 |
|---|---|
| `frontend/src/pages/AnalyticsPage.js` + `.scss` | `/analytics` 仪表盘 |
| `frontend/src/components/analytics/KpiCard.js` + `.scss` | 指标卡片 + 增量徽标 |
| `frontend/src/components/analytics/DailyChart.js` + `.scss` | 自研 SVG 图表、指标切换、表格视图 |
| `frontend/src/components/analytics/SourceSplit.js` + `.scss` | web/API 使用拆分 |
| `frontend/src/components/analytics/TopTemplatesTable.js` + `.scss` | Top-10 表格 |
| `frontend/src/components/analytics/RangeSelector.js` + `.scss` | 7/30/90 分段控件 |
| `frontend/src/components/analytics/OwnerStatsSection.js` + `.scss` | 模板详情页作者区块 |
| `frontend/src/components/analytics/PublicCounters.js` + `.scss` | 详情页浏览/使用计数 |
| `frontend/src/components/analytics/Sparkline.js` | 30 天迷你 SVG 折线 |
| `frontend/src/utils/formatCount.js` + `formatCount.test.js` | 紧凑 + 本地化计数 |

### 4.2 修改文件

| 文件 | 变更 |
|---|---|
| `frontend/src/App.js` | `const AnalyticsPage = lazy(...)`；带 `<Suspense>` 的 `/analytics` 路由 |
| `frontend/src/components/Sidebar.js` | "数据分析" 导航项（仅登录可见，位于 "我的收藏" 之下、分类区之上） |
| `frontend/src/pages/TemplateDetails.js` | 浏览事件 `useEffect`；统计行中的 `PublicCounters`；`isOwner` 时的 `OwnerStatsSection` |
| `frontend/src/services/api.js` | `getAnalyticsOverview`、`getTemplateStats`、`recordTemplateEvent` |
| `frontend/src/locales/en/translation.json` | `analytics.*` 键 |
| `frontend/src/locales/zh/translation.json` | 相同键集，中文文案 |

### 4.3 API 客户端（`services/api.js`）

```js
export const getAnalyticsOverview = (days = 30) =>
  api.get('/analytics/overview', { params: { days } });

export const getTemplateStats = (id, days = 30) =>
  api.get(`/templates/${id}/stats`, { params: { days } });

export const recordTemplateEvent = (id, { type = 'view', sessionId = '' } = {}) =>
  api.post(`/templates/${id}/events`, { type, session_id: sessionId });
```

### 4.4 组件树

```
AnalyticsPage（鉴权守卫；?range=7|30|90）
├── state: range, data, loading, error, requestId
├── useEffect[range]: if (!token) navigate('/login');
│   fetch 守卫: const rid = ++requestIdRef.current;
│   仅当 rid === requestIdRef.current 时应用响应（竞态守卫）
├── <h1> t('analytics.title') · <RangeSelector value={range} onChange/>
├── loading → 4 × KpiCard 骨架 + 图表骨架 + 5 行表格骨架
├── error   → 横幅 t('analytics.error.fetch') + 重试（保留上次数据）
├── template_count === 0 → 空状态（title/body/cta → navigate('/')）
├── 4 × <KpiCard label icon value delta/>
│     delta: 服务端提供的 current/previous；徽标 =
│       prev === 0 && cur > 0 → "new"（up 徽标）
│       prev === cur          → t('analytics.delta.flat')
│       else                  → ±((cur-prev)/prev*100, 保留 1 位小数)%
├── <DailyChart series={data.series} />   // 指标切换在客户端；无障碍表格视图
├── <SourceSplit web={data.source_split.web} api={data.source_split.api}/>
└── <TopTemplatesTable rows={data.top_templates}/>  // 行点击 → /templates/:id

TemplateDetails 增补
├── <PublicCounters views={template.view_count} uses={template.use_count}/>
│     // 统计行，所有访客；formatCount；t('analytics.public.views'/'uses')
├── {isOwner && <OwnerStatsSection templateId={id}/>}
│     // 自行调用 getTemplateStats(id, 30)：计数器、浏览→使用率、Sparkline
└── useEffect[mount]: 浏览 beacon
      const fired = useRef(false);
      if (fired.current) return; fired.current = true;   // StrictMode 安全
      const sid = localStorage.getItem('op_analytics_sid') ?? crypto.randomUUID()
                  （经 localStorage.setItem 持久化）;
      recordTemplateEvent(id, { sessionId: sid }).catch(() => {});
```

`formatCount(value, locale)`：

- EN：`< 1000` → 本地化分隔符；`≥ 1000` → 紧凑 `1.2k`、`1.2M`。
- ZH：`< 10000` → 本地化分隔符；`≥ 10000` → 紧凑 `1.2万`、`1.2亿`。
- 单元测试覆盖 999、1000、9999、10000、1_000_000（两种语言）。

### 4.5 i18n 键（EN/ZH 对齐 —— 键集一致，AC-11）

命名空间 `analytics.*` 与 UI/UX 设计 §9 完全一致
（`title`、`range.{7,30,90}`、`kpi.{views,uses,likes,favorites}`、
`kpi_tooltip.*`、`delta.{up,down,flat}`、`chart.*`、`source.*`、`top.*`、
`owner_stats.*`、`public.{views,uses}`、`empty.{title,body,cta}`、
`error.{fetch,retry}`）—— 将
`docs/design/prompt-analytics-ui-ux-design.md` §9 的 EN 与 ZH JSON 块原样
复制进两份 `translation.json`。

---

## 5. 错误矩阵

| 面 | 条件 | gRPC 码 | HTTP |
|---|---|---|---|
| `POST /templates/{id}/events` | 缺 template id；`type` ≠ `view`；JSON 非法 | `InvalidArgument` | 400 |
| | 模板不存在，或私有模板非作者 | `NotFound` | 404 |
| | 出现的 token 无效 | `Unauthenticated` | 401 |
| | 按 IP 限流超限 | `ResourceExhausted` | 429 + `Retry-After` |
| | 事件插入失败 | `Internal` | 500 |
| `GET /templates/{id}/stats` | 缺失/无效 token | `Unauthenticated` | 401 |
| | 调用者被封禁 | `PermissionDenied` | 403 |
| | `days` ∉ {7, 30, 90} | `InvalidArgument` | 400 |
| | 模板不存在 / 私有模板非作者 | `NotFound` | 404 |
| | 公开模板、非作者调用 | `PermissionDenied` | 403 |
| `GET /analytics/overview` | 缺失/无效 token | `Unauthenticated` | 401 |
| | 调用者被封禁 | `PermissionDenied` | 403 |
| | `days` ∉ {7, 30, 90} | `InvalidArgument` | 400 |
| use 事件埋点 | 任何失败 | —（记日志，绝不暴露） | — |

被封禁用户必须呈现为 **403**（绝非 401）：`VerifyAnyToken`/
`checkBanAndRevocation` 内的封禁检查返回 `PermissionDenied`，`writeError`
将其映射为 403。

---

## 6. 部署

1. DDL + 回填与代码同一变更进入**两份** `deployment/init.sql` 与
   `deployment/chart/open-prompts/init.sql`。
2. **先**应用 DDL **再**部署新二进制（回填守卫随后恰好导入一次历史；
   重跑为无操作）。
3. 新环境变量 `TEMPLATE_EVENT_RATE_PER_MINUTE`（默认 `60`），按既有
   `KEY: ${KEY:-default}` 模式接入 `deployment/docker-compose.yml` 与
   chart values/清单。
4. 无回滚数据迁移；删除表/列即回滚。

---

## 7. SDK / 公开 API 文档

- `GET /api/v1/templates/{id}/aliases/{alias}` 响应**不变**；use 事件埋点
  对 API 消费者不可见。
- `RecordTemplateEvent` 是第一方 web beacon，不是面向 SDK 的公开 API；
  它出现在 proto 中仅为网关路径服务。
- `GET /templates/{id}/stats` 与 `GET /analytics/overview` 是面向用户的
  REST 端点（登录用户的 cookie/Bearer）；与个人资料端点一起写入用户 API
  参考文档。

---

## 8. 测试策略

### 8.1 FVT（`tests/fvt.py`）—— `test_template_analytics`

顺序场景（沿用既有 helper 风格：`register_test_user`、`create_template`、
`auth_headers`）：

1. **浏览 + 去重（AC-1）：** 匿名 `POST /templates/{id}/events`
   `{"type":"view","session_id":"s1"}` → 200 `{}`；同一 `session_id` 重复
   → 200；`GET /templates/{id}` 显示 `view_count == 1`。不同
   `session_id` → `view_count == 2`。
2. **Web 使用（AC-2）：** `POST /api/v1/prompts`（带鉴权）→ 200；模板
   `use_count == 1`；overview `kpis.uses >= 1`、`source_split.web >= 1`。
3. **API 使用（AC-2）：** 创建别名；带 `Authorization: ApiKey sk-...` 的
   `GET /templates/{id}/aliases/{alias}` → 200；`use_count` +1；
   `source_split.api >= 1`。
4. **公开计数（AC-3）：** 匿名 `GET /templates/{id}` 响应包含
   `view_count`/`use_count` 数字。
5. **作者统计（AC-4/AC-9）：** 作者 `GET /templates/{id}/stats?days=30`
   → 200，含 `current`/`previous`/`series`（长度 == 30）/`source_split`；
   非作者 → 403；匿名 → 401；模板缺失 → 404。
6. **Overview（AC-5/AC-7）：** `GET /analytics/overview?days=7` → 200；
   `series` 长度 7；`top_templates` 按 `uses` 排序；`template_count` 与
   用户模板数一致。
7. **校验：** 两个端点 `days=14` → 400；`type":"click"` → 400。
8. **限流：** 调低 `TEMPLATE_EVENT_RATE_PER_MINUTE`（或循环 60+ 次请求）
   → 429 且带 `Retry-After`。
9. **回归（AC-12）：** 点赞/收藏切换仍返回正确的
   `like_count`/`favorite_count`。

### 8.2 单元测试（Go）

- `days` 校验；`event_type` 校验。
- 去重：Redis 命中、Redis 错误 → DB 回退命中/未命中/错误路径。
- 限流窗口计算（复用 `checkCommentRate` 的测试形状）。
- `zeroFill` 边界（闰日范围、UTC 日界）。
- 增量输入：当前/上一周期汇总映射。

### 8.3 前端测试

- `formatCount` 边界（999、1000、9999、10000、1M；EN + ZH）。
- KpiCard/RangeSelector/TopTemplatesTable 渲染 + 无障碍角色。
- AnalyticsPage：加载 → 就绪、范围切换重新拉取、空状态、错误 + 重试。

---

## 9. 实现顺序

1. 两份 `init.sql` 的 DDL + 回填。
2. Proto 消息/RPC + 重新生成 `prompt.pb.go`。
3. `models.Template` 字段 + 仓储列清单。
4. `AnalyticsRepository` + 单元测试。
5. 服务方法（`RecordTemplateEvent`、`RecordUseEvent`、`GetTemplateStats`、
   `GetCreatorStats`、`checkEventRate`、去重）+ 测试。
6. `publicRpcMethods` 追加。
7. 网关：events/stats 分支、`/api/v1/analytics/overview` 处理器、两个
   use 事件埋点。
8. 前端：api 客户端 → formatCount → 组件 → AnalyticsPage →
   TemplateDetails 集成 → 侧边栏/路由 → i18n。
9. FVT `test_template_analytics`；完整 `tests/fvt.py` 回归。

---

## 10. 给开发者的约束

1. **Proto 字段号：** `Template.view_count = 23`、`use_count = 24`。
   18–22 由 follow-creators 与 template-ratings 预留 —— 绝不复用。
2. **所有计数均为 `int32`**（protojson 把 `int64` 渲染为 JSON 字符串；
   FVT 按数字比较）。
3. **两份 `init.sql` 副本**在同一提交中接收相同的 DDL/回填块。
4. `RecordTemplateEvent` 必须加入 `publicRpcMethods`（裸 gRPC 与网关的
   匿名允许路由行为对齐）。
5. use 事件埋点在启动 goroutine **之前**捕获请求值（`templateID`、
   `userID`、`authHeader`），goroutine 内部使用 `context.Background()`。
6. `POST /api/v1/prompts` 处理器绕过 `PromptService.CreatePrompt`
   （直接调用 `promptRepo.Create`）—— 埋点在**网关处理器**，不在服务里。
7. 绝不向调用方暴露埋点错误；仅记日志（FR-7）。
8. 被封禁用户必须呈现为 403（`PermissionDenied`），绝非 401。
9. 不修改无关处理器；不提交共享工作区中可能存在的其他 Agent 未提交文件。
10. 实现中不出现任何商业化概念。
