# Prompt Analytics — Detailed Design (EN)

| | |
|---|---|
| **Feature ID** | prompt-analytics |
| **Doc Type** | Detailed Design |
| **Version** | 1.0 |
| **Status** | Ready for implementation |
| **Author** | Architect Agent |
| **Upstream** | `docs/design/prompt-analytics-requirements.md`, `docs/design/prompt-analytics-ui-ux-design.md` |
| **Downstream** | Developer Agent (implementation), Test Agent (FVT/E2E) |
| **Companion** | `prompt-analytics-architecture.md` (EN), `-zh-CN.md` pair |

---

## 1. Proto Contract

All additions go into `backend/api/proto/v1/prompt.proto`, service `PromptService`.

### 1.1 RPCs (appended to `service PromptService`)

```proto
  // RecordTemplateEvent records a view event for a template (anonymous
  // allowed; rate-limited per IP). Use events are recorded server-side only.
  rpc RecordTemplateEvent(RecordTemplateEventRequest) returns (RecordTemplateEventResponse);

  // GetTemplateStats returns analytics for a single template (owner only).
  rpc GetTemplateStats(GetTemplateStatsRequest) returns (GetTemplateStatsResponse);

  // GetCreatorStats returns aggregate analytics for the caller's templates.
  rpc GetCreatorStats(GetCreatorStatsRequest) returns (GetCreatorStatsResponse);
```

### 1.2 `Template` message additions

Field numbers **23 and 24** — 18/19 are reserved by follow-creators
(`owner_display_name`/`owner_avatar`), 20–22 by template-ratings
(`rating_avg`/`rating_count`/`my_rating`). Do not reuse them.

```proto
  // Total deduplicated views (per session per 30 minutes).
  int32 view_count = 23;
  // Total prompts created from this template (web + API).
  int32 use_count = 24;
```

Both are `int32` (protojson serializes `int64` as a JSON string; the
frontend and FVT compare counts as JSON numbers).

### 1.3 New messages

```proto
// RecordTemplateEventRequest records a client-side view event.
message RecordTemplateEventRequest {
  // Template the event belongs to (required).
  string template_id = 1;
  // Event type; only "view" is accepted in v1 (required).
  string event_type = 2;
  // Client session id used for dedup (optional; empty = no dedup).
  string session_id = 3;
}

message RecordTemplateEventResponse {}

// TemplateStatsSummary is a range total for the four tracked metrics.
message TemplateStatsSummary {
  int32 views = 1;
  int32 uses = 2;
  int32 likes = 3;
  int32 favorites = 4;
}

// DailyMetricPoint is one UTC day of the daily series.
message DailyMetricPoint {
  // Day as YYYY-MM-DD (UTC).
  string date = 1;
  int32 views = 2;
  int32 uses = 3;
  int32 likes = 4;
  int32 favorites = 5;
}

// SourceSplit breaks down use events by origin.
message SourceSplit {
  int32 web = 1;
  int32 api = 2;
}

message GetTemplateStatsRequest {
  string template_id = 1;
  // 7, 30, or 90 (required; anything else is InvalidArgument).
  int32 days = 2;
}

message GetTemplateStatsResponse {
  // Totals for the requested range.
  TemplateStatsSummary current = 1;
  // Totals for the equivalent preceding range (delta baseline).
  TemplateStatsSummary previous = 2;
  // Zero-filled daily series, exactly `days` points, oldest first.
  repeated DailyMetricPoint series = 3;
  // Use-event source split for the requested range.
  SourceSplit source_split = 4;
  // uses / views for the range; 0 when views == 0.
  double view_to_use_rate = 5;
}

// TopTemplateRow is one row of the top-templates table.
message TopTemplateRow {
  string id = 1;
  string title = 2;
  int32 views = 3;
  int32 uses = 4;
  int32 likes = 5;
  int32 favorites = 6;
  // View→use rate for the range (uses / views; 0 when views == 0).
  double rate = 7;
}

message GetCreatorStatsRequest {
  // 7, 30, or 90 (required; anything else is InvalidArgument).
  int32 days = 1;
}

message GetCreatorStatsResponse {
  TemplateStatsSummary current = 1;
  TemplateStatsSummary previous = 2;
  repeated DailyMetricPoint series = 3;
  SourceSplit source_split = 4;
  // Top 10 of the caller's templates by uses in range (ties → views).
  repeated TopTemplateRow top_templates = 5;
  // Total templates owned by the caller (drives the empty state).
  int32 template_count = 6;
}
```

### 1.4 JSON wire shapes (gateway marshaler: `EmitUnpopulated` + `UseProtoNames`)

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

- `GET /api/v1/analytics/overview?days=30` → same shape plus
  `"top_templates": [{"id": "...", "title": "...", "views": 40, "uses": 10, "likes": 2, "favorites": 1, "rate": 0.25}, "..."]`
  and `"template_count": 12`.

---

## 2. Database DDL

Applied to **both** `deployment/init.sql` and
`deployment/chart/open-prompts/init.sql` (byte-identical blocks), after the
existing `template_comments` section.

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

-- Backfill (D-2): one historical use/web event per prompts row. The
-- per-row NOT EXISTS guard makes re-runs and coexistence with live events
-- safe. Deploy DDL before the new binary so the guard imports history once.
INSERT INTO template_usage_events (template_id, event_type, source, user_id, created_at)
SELECT p.template_id, 'use', 'web', p.owner_id, p.created_at
FROM prompts p
WHERE NOT EXISTS (
    SELECT 1 FROM template_usage_events e
    WHERE e.event_type = 'use' AND e.template_id = p.template_id
      AND e.user_id IS NOT DISTINCT FROM p.owner_id
      AND e.created_at = p.created_at
);

-- Converge the denormalized counter from the event table (idempotent).
UPDATE templates t
SET use_count = sub.c
FROM (SELECT template_id, COUNT(*) AS c
      FROM template_usage_events WHERE event_type = 'use'
      GROUP BY template_id) sub
WHERE t.id = sub.template_id AND t.use_count <> sub.c;
```

`view_count` has no historical source and starts at 0 (accepted in the
architecture doc).

---

## 3. Backend Layers

### 3.1 `models.Template` (`backend/internal/models/template.go`)

```go
ViewCount int32 // denormalized; maintained by the event write path
UseCount  int32 // denormalized; maintained by the event write path + backfill
```

`templateModelToProto` (`backend/internal/service/prompt.go`) maps both into
`Template.view_count` / `Template.use_count`. Every `SELECT` that hydrates a
`models.Template` for proto mapping must include the two new columns
(`template_repository.go` `Get`/`List`/`GetByAlias` column lists and any
`Scan` that follows).

### 3.2 `AnalyticsRepository` (new `backend/internal/repository/analytics_repository.go`)

```go
type AnalyticsRepository struct{ db *sql.DB }

func NewAnalyticsRepository(db *sql.DB) *AnalyticsRepository

// InsertEvent appends one usage event.
func (r *AnalyticsRepository) InsertEvent(ctx context.Context, templateID, eventType, source, userID, sessionID string) error

// RecentViewExists reports a view event for (template, session) newer than
// window. DB fallback for the Redis dedup key.
func (r *AnalyticsRepository) RecentViewExists(ctx context.Context, templateID, sessionID string, window time.Duration) (bool, error)

// IncrementCounter bumps a denormalized counter. column is whitelisted by
// the caller ("view_count" | "use_count"); never interpolated from input.
func (r *AnalyticsRepository) IncrementCounter(ctx context.Context, templateID, column string) error

// DailyEventSeries returns per-day view/use counts. Exactly one of
// templateID / ownerID is set (NULL scopes are skipped in SQL).
func (r *AnalyticsRepository) DailyEventSeries(ctx context.Context, templateID, ownerID string, from, to time.Time) (map[string][2]int32, error)

// DailyLikeSeries / DailyFavoriteSeries return per-day like/favorite counts.
func (r *AnalyticsRepository) DailyLikeSeries(ctx context.Context, templateID, ownerID string, from, to time.Time) (map[string]int32, error)
func (r *AnalyticsRepository) DailyFavoriteSeries(ctx context.Context, templateID, ownerID string, from, to time.Time) (map[string]int32, error)

// UseSourceSplit returns use-event counts per source for the window.
func (r *AnalyticsRepository) UseSourceSplit(ctx context.Context, templateID, ownerID string, from, to time.Time) (web, api int32, err error)

// TopTemplatesByUses returns the owner's top templates in the window.
func (r *AnalyticsRepository) TopTemplatesByUses(ctx context.Context, ownerID string, from, to time.Time, limit int) ([]models.TopTemplateRow, error)

// CountOwnerTemplates returns how many templates the user owns.
func (r *AnalyticsRepository) CountOwnerTemplates(ctx context.Context, ownerID string) (int32, error)
```

`models.TopTemplateRow`:

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

**SQL for each method** (all parameterized; `$3::uuid`/`$4::text` NULL
parameters disable the scope):

`InsertEvent`:

```sql
INSERT INTO template_usage_events (template_id, event_type, source, user_id, session_id)
VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''));
```

`RecentViewExists`:

```sql
SELECT 1 FROM template_usage_events
WHERE template_id = $1 AND session_id = $2 AND event_type = 'view'
  AND created_at > NOW() - $3::interval
LIMIT 1;
```

`IncrementCounter` (column is a compile-time constant at the call site):

```sql
UPDATE templates SET view_count = view_count + 1 WHERE id = $1;   -- or use_count
```

`DailyEventSeries`:

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

`DailyLikeSeries` (favorites identical with `template_favorites`):

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

`UseSourceSplit`:

```sql
SELECT e.source, COUNT(*)::int
FROM template_usage_events e
JOIN templates t ON t.id = e.template_id
WHERE e.event_type = 'use' AND e.created_at >= $1 AND e.created_at < $2
  AND ($3::uuid IS NULL OR e.template_id = $3)
  AND ($4::text  IS NULL OR t.owner_id = $4)
GROUP BY e.source;
```

`TopTemplatesByUses`:

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

`CountOwnerTemplates`:

```sql
SELECT COUNT(*)::int FROM templates WHERE owner_id = $1;
```

### 3.3 `PromptService` (`backend/internal/service/prompt.go`)

New struct field (constructor parameter appended; `main.go` wires it):

```go
AnalyticsRepo *repository.AnalyticsRepository
```

#### 3.3.1 `RecordTemplateEvent` (the public view beacon)

```
func (s *PromptService) RecordTemplateEvent(ctx, req) (*pb.RecordTemplateEventResponse, error):
  1. req.TemplateId == ""                          → InvalidArgument "template id required"
  2. req.EventType != "view"                       → InvalidArgument "unsupported event type"
  3. rate limit (checkEventRate, §3.3.3, key prefix tpl_event_rate,
     env TEMPLATE_EVENT_RATE_PER_MINUTE default 60) exceeded
                                                    → ResourceExhausted "rate limited; retry after N seconds"
  4. tpl, err := s.TemplateRepo.Get(ctx, req.TemplateId)
     err                                            → NotFound "template not found"
  5. caller := GetUserFromContext(ctx)   // "" for anonymous
     tpl.Visibility == PRIVATE && caller != tpl.OwnerID
                                                    → NotFound "template not found"   // no existence leak
  6. if req.SessionId != "":
       dedup (§3.3.4). If duplicate                 → return &pb.RecordTemplateEventResponse{}, nil
  7. s.AnalyticsRepo.InsertEvent(ctx, templateID, "view", "web", caller, req.SessionId)
     err                                            → log + Internal (rare; DB down)
  8. s.AnalyticsRepo.IncrementCounter(ctx, templateID, "view_count")
     err                                            → log only, still return OK
     // event table is the source of truth; counter drift is repaired by
     // reconvergence (same UPDATE shape as the backfill) and is acceptable.
  9. return empty response, nil
```

#### 3.3.2 `RecordUseEvent` (internal; not an RPC, no proto)

```
func (s *PromptService) RecordUseEvent(ctx, templateID, userID, source string):
  1. if templateID == ""                            → return (log)
  2. if source != "web" && source != "api"          → return (log)
  3. err := s.AnalyticsRepo.InsertEvent(ctx, templateID, "use", source, userID, "")
     err                                            → log only
  4. err := s.AnalyticsRepo.IncrementCounter(ctx, templateID, "use_count")
     err                                            → log only
```

Never returns an error; callers run it in a goroutine with
`context.Background()` so request cancellation cannot kill the write.

#### 3.3.3 Rate limit — `checkEventRate` (new, `prompt.go` or `analytics.go`)

Clone of `checkCommentRate` (`comment.go:223`): Redis fixed window on
`tpl_event_rate:{clientIP}`; value `"{count}:{expiresUnix}"`; window 60 s;
limit from `TEMPLATE_EVENT_RATE_PER_MINUTE` (default 60). Client IP comes
from the `x-client-ip` gRPC metadata (gateway `withClientIP`). **Fail-open**
on Redis errors. Exceeding returns `ResourceExhausted` with message
`"rate limited; retry after N seconds"` — the gateway's `writeError` parses
that into an HTTP `Retry-After` header.

#### 3.3.4 View dedup

```
key := "tpl_view_dedup:" + templateID + ":" + sessionID
ok, err := redis.SetNX(ctx, key, "1", 30*time.Minute)
if err == nil:
    if !ok: duplicate → skip insert
else:  // Redis down → DB fallback
    exists, err2 := AnalyticsRepo.RecentViewExists(ctx, templateID, sessionID, 30*time.Minute)
    if err2 == nil && exists: duplicate → skip insert
    if err2 != nil: proceed (count it; brief over-counting is acceptable)
```

Empty `session_id` skips dedup entirely (always counts).

#### 3.3.5 `GetTemplateStats`

```
1. caller := GetUserFromContext(ctx); caller == ""  → Unauthenticated
2. req.Days ∈ {7, 30, 90} else                      → InvalidArgument "days must be 7, 30, or 90"
3. tpl, err := TemplateRepo.Get(ctx, req.TemplateId)
   err                                              → NotFound
4. tpl.OwnerID != caller:
     tpl.Visibility == PRIVATE                      → NotFound   // no existence leak
     else                                           → PermissionDenied "not the template owner"
5. to := time.Now().UTC(); from := to.AddDate(0, 0, -days)
   prevTo := from; prevFrom := from.AddDate(0, 0, -days)
6. evCur  := AnalyticsRepo.DailyEventSeries(ctx,  templateID, "", from, to)
   lkCur  := AnalyticsRepo.DailyLikeSeries(ctx,    templateID, "", from, to)
   fvCur  := AnalyticsRepo.DailyFavoriteSeries(ctx, templateID, "", from, to)
   evPrev/lkPrev/fvPrev := same with (prevFrom, prevTo)
   web, api := AnalyticsRepo.UseSourceSplit(ctx, templateID, "", from, to)
7. series := zeroFill(days, evCur, lkCur, fvCur)    // exactly `days` points, oldest first
   current := sums(series); previous := sums(evPrev, lkPrev, fvPrev)
   rate := current.Uses / current.Views if current.Views > 0 else 0
8. return GetTemplateStatsResponse{current, previous, series, source_split, view_to_use_rate}
```

#### 3.3.6 `GetCreatorStats`

Identical to §3.3.5 steps 1–2, 5–8 with `ownerID = caller` and
`templateID = ""` (owner-scoped SQL), plus:

```
top   := AnalyticsRepo.TopTemplatesByUses(ctx, caller, from, to, 10)
        // row.rate computed in the service: uses/views, 0 when views == 0
count := AnalyticsRepo.CountOwnerTemplates(ctx, caller)
```

No ownership check is needed — the scope *is* the caller (FR-26: admins get
no cross-user access).

#### 3.3.7 `zeroFill` helper

```
for i := days - 1; i >= 0; i-- {
    day := from.AddDate(0, 0, i).Format("2006-01-02")   // UTC
    point := DailyMetricPoint{Date: day, views/likes/favorites from maps}
}
```

Range totals are the sums of the zero-filled series (no extra query).

### 3.4 `AuthInterceptor` (`backend/internal/service/auth.go`)

Add to `publicRpcMethods` in `NewAuthInterceptor`:

```go
"/v1.PromptService/RecordTemplateEvent": true,
```

Behavior matches the gateway: anonymous passes through; a presented token is
verified (invalid → `Unauthenticated`) and injects the user id for
attribution. `GetTemplateStats`/`GetCreatorStats` stay non-public.

### 3.5 Gateway (`backend/cmd/server/main.go`)

#### 3.5.1 `POST /api/v1/templates/{id}/events`

Inside the existing `/api/v1/templates/` handler, add a branch for
`parts[1] == "events"` (alongside `aliases`, `likes`, `favorites`,
`comments`):

```
case http.MethodPost:
  1. ctx := context.Background()
     if Authorization header present:
         userID, err := authInterceptor.VerifyAnyToken(r.Context(), token)
         err                                          → 401 "Invalid token"
         ctx = service.ContextWithUserID(ctx, userID)
     // absent header → anonymous (allowed)
  2. read body; unmarshal {type, session_id}          → 400 on bad JSON
  3. req := &pb.RecordTemplateEventRequest{TemplateId: id,
         EventType: body.Type, SessionId: body.SessionID}
  4. resp, err := svc.RecordTemplateEvent(withClientIP(ctx, clientIP(r)), req)
     err → writeError(w, err)   // 400 / 404 / 429 + Retry-After / 500
  5. 200, body {} (marshaler emits "{}")
case other methods                                    → 405
```

#### 3.5.2 `GET /api/v1/templates/{id}/stats?days=30`

Same handler, branch `parts[1] == "stats"`:

```
case http.MethodGet:
  1. Authorization required; VerifyAnyToken            → 401 on missing/invalid
  2. ctx = service.ContextWithUserID(context.Background(), userID)
  3. days := atoi(r.URL.Query().Get("days")); missing → 30
  4. resp, err := svc.GetTemplateStats(ctx, &pb.GetTemplateStatsRequest{
         TemplateId: id, Days: int32(days)})
     err → writeError   // 400 invalid days / 401 / 403 non-owner / 404
  5. 200 JSON
case other methods                                     → 405
```

#### 3.5.3 `GET /api/v1/analytics/overview?days=30` (new handler)

```
http.HandleFunc("/api/v1/analytics/overview", ...):
  1. CORS preflight (same headers as other handlers)
  2. method != GET                                     → 405
  3. Authorization required; VerifyAnyToken            → 401 on missing/invalid
  4. ctx = service.ContextWithUserID(context.Background(), userID)
  5. days := atoi(query "days"); missing → 30
  6. resp, err := svc.GetCreatorStats(ctx, &pb.GetCreatorStatsRequest{Days: int32(days)})
     err → writeError
  7. 200 JSON
```

#### 3.5.4 Use-event hooks (fire-and-forget)

**Web** — in the existing `POST /api/v1/prompts` handler, after
`promptRepo.Create` succeeds (the handler bypasses `PromptService.CreatePrompt`;
hook there, not in the service):

```go
tplID, uid := prompt.TemplateID, userID // captured BEFORE the goroutine
go func() {
    svc.RecordUseEvent(context.Background(), tplID, uid, "web")
}()
```

**API** — in the existing `GET /api/v1/templates/{id}/aliases/{alias}`
handler, after a successful `GetPromptByAlias`:

```go
authHeader := r.Header.Get("Authorization") // captured BEFORE the goroutine
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
        // verification failure is ignored: the alias endpoint is public and
        // the event is still recorded with an empty user_id
    }
    svc.RecordUseEvent(ctx, tplID, uid, "api")
}()
```

Both hooks never touch the response path; `RecordUseEvent` logs errors
internally (FR-7).

---

## 4. Frontend

### 4.1 New files

| File | Purpose |
|---|---|
| `frontend/src/pages/AnalyticsPage.js` + `.scss` | `/analytics` dashboard |
| `frontend/src/components/analytics/KpiCard.js` + `.scss` | Metric card + delta badge |
| `frontend/src/components/analytics/DailyChart.js` + `.scss` | Custom SVG chart, metric toggles, table view |
| `frontend/src/components/analytics/SourceSplit.js` + `.scss` | Web/API use split |
| `frontend/src/components/analytics/TopTemplatesTable.js` + `.scss` | Top-10 table |
| `frontend/src/components/analytics/RangeSelector.js` + `.scss` | 7/30/90 segmented control |
| `frontend/src/components/analytics/OwnerStatsSection.js` + `.scss` | Template-detail owner block |
| `frontend/src/components/analytics/PublicCounters.js` + `.scss` | Detail-page view/use counters |
| `frontend/src/components/analytics/Sparkline.js` | 30-day mini SVG line |
| `frontend/src/utils/formatCount.js` + `formatCount.test.js` | Compact + locale-aware counts |

### 4.2 Modified files

| File | Change |
|---|---|
| `frontend/src/App.js` | `const AnalyticsPage = lazy(...)`; route `/analytics` with `<Suspense>` |
| `frontend/src/components/Sidebar.js` | "Analytics" nav item (logged-in only, below "My Favorites", above Categories) |
| `frontend/src/pages/TemplateDetails.js` | view-event `useEffect`; `PublicCounters` in stats row; `OwnerStatsSection` when `isOwner` |
| `frontend/src/services/api.js` | `getAnalyticsOverview`, `getTemplateStats`, `recordTemplateEvent` |
| `frontend/src/locales/en/translation.json` | `analytics.*` keys |
| `frontend/src/locales/zh/translation.json` | identical key set, ZH strings |

### 4.3 API client (`services/api.js`)

```js
export const getAnalyticsOverview = (days = 30) =>
  api.get('/analytics/overview', { params: { days } });

export const getTemplateStats = (id, days = 30) =>
  api.get(`/templates/${id}/stats`, { params: { days } });

export const recordTemplateEvent = (id, { type = 'view', sessionId = '' } = {}) =>
  api.post(`/templates/${id}/events`, { type, session_id: sessionId });
```

### 4.4 Component tree

```
AnalyticsPage (auth-guarded; ?range=7|30|90)
├── state: range, data, loading, error, requestId
├── useEffect[range]: if (!token) navigate('/login');
│   fetch guard: const rid = ++requestIdRef.current;
│   only apply response when rid === requestIdRef.current (race guard)
├── <h1> t('analytics.title') · <RangeSelector value={range} onChange/>
├── loading → 4 × KpiCard skeleton + chart skeleton + 5 table skeleton rows
├── error   → banner t('analytics.error.fetch') + Retry (last data stays)
├── template_count === 0 → empty state (title/body/cta → navigate('/'))
├── 4 × <KpiCard label icon value delta/>
│     delta: server-provided current/previous; badge =
│       prev === 0 && cur > 0 → "new" (up badge)
│       prev === cur          → t('analytics.delta.flat')
│       else                  → ±((cur-prev)/prev*100, 1 decimal)%
├── <DailyChart series={data.series} />   // metric toggles client-side; a11y table view
├── <SourceSplit web={data.source_split.web} api={data.source_split.api}/>
└── <TopTemplatesTable rows={data.top_templates}/>  // row click → /templates/:id

TemplateDetails additions
├── <PublicCounters views={template.view_count} uses={template.use_count}/>
│     // stats row, all visitors; formatCount; t('analytics.public.views'/'uses')
├── {isOwner && <OwnerStatsSection templateId={id}/>}
│     // own fetch of getTemplateStats(id, 30): counters, view→use rate, Sparkline
└── useEffect[mount]: view beacon
      const fired = useRef(false);
      if (fired.current) return; fired.current = true;   // StrictMode-safe
      const sid = localStorage.getItem('op_analytics_sid') ?? crypto.randomUUID()
                  (persist via localStorage.setItem);
      recordTemplateEvent(id, { sessionId: sid }).catch(() => {});
```

`formatCount(value, locale)`:

- EN: `< 1000` → locale separator; `≥ 1000` → compact `1.2k`, `1.2M`.
- ZH: `< 10000` → locale separator; `≥ 10000` → compact `1.2万`, `1.2亿`.
- Unit tests at 999, 1000, 9999, 10000, 1_000_000 (both locales).

### 4.5 i18n keys (EN/ZH parity — identical key sets, AC-11)

Namespace `analytics.*` exactly as specified in the UI/UX design §9
(`title`, `range.{7,30,90}`, `kpi.{views,uses,likes,favorites}`,
`kpi_tooltip.*`, `delta.{up,down,flat}`, `chart.*`, `source.*`, `top.*`,
`owner_stats.*`, `public.{views,uses}`, `empty.{title,body,cta}`,
`error.{fetch,retry}`) — copy the EN and ZH JSON blocks verbatim from
`docs/design/prompt-analytics-ui-ux-design.md` §9 into the two
`translation.json` files.

---

## 5. Error Matrix

| Surface | Condition | gRPC code | HTTP |
|---|---|---|---|
| `POST /templates/{id}/events` | missing template id; `type` ≠ `view`; bad JSON | `InvalidArgument` | 400 |
| | template missing, or private non-owner | `NotFound` | 404 |
| | presented token invalid | `Unauthenticated` | 401 |
| | per-IP rate limit exceeded | `ResourceExhausted` | 429 + `Retry-After` |
| | event insert failure | `Internal` | 500 |
| `GET /templates/{id}/stats` | missing/invalid token | `Unauthenticated` | 401 |
| | banned caller | `PermissionDenied` | 403 |
| | `days` ∉ {7, 30, 90} | `InvalidArgument` | 400 |
| | template missing / private non-owner | `NotFound` | 404 |
| | public template, non-owner caller | `PermissionDenied` | 403 |
| `GET /analytics/overview` | missing/invalid token | `Unauthenticated` | 401 |
| | banned caller | `PermissionDenied` | 403 |
| | `days` ∉ {7, 30, 90} | `InvalidArgument` | 400 |
| Use-event hooks | any failure | — (logged, never surfaced) | — |

Banned users surface as **403** (never 401): the ban check inside
`VerifyAnyToken`/`checkBanAndRevocation` returns `PermissionDenied`, which
`writeError` maps to 403.

---

## 6. Deployment

1. DDL + backfill land in **both** `deployment/init.sql` and
   `deployment/chart/open-prompts/init.sql` in the same change as the code.
2. Apply DDL **before** deploying the new binary (the backfill guard then
   imports history exactly once; re-runs are no-ops).
3. New env var `TEMPLATE_EVENT_RATE_PER_MINUTE` (default `60`) wired in
   `deployment/docker-compose.yml` and the chart values/manifest following
   the existing `KEY: ${KEY:-default}` pattern.
4. No rollback data migration; dropping the table/columns is the rollback.

---

## 7. SDK / Public API docs

- `GET /api/v1/templates/{id}/aliases/{alias}` response is **unchanged**;
   the use-event hook is invisible to API consumers.
- `RecordTemplateEvent` is a first-party web beacon, not a documented public
   API for SDKs; it is listed in the proto for the gateway path only.
- `GET /templates/{id}/stats` and `GET /analytics/overview` are user-facing
   REST endpoints (cookie/Bearer of a logged-in user); document them in the
   user API reference alongside profile endpoints.

---

## 8. Test Strategy

### 8.1 FVT (`tests/fvt.py`) — `test_template_analytics`

Ordered scenario (follows the existing helper style: `register_test_user`,
`create_template`, `auth_headers`):

1. **View + dedup (AC-1):** anonymous `POST /templates/{id}/events`
   `{"type":"view","session_id":"s1"}` → 200 `{}`; repeat with same
   `session_id` → 200; `GET /templates/{id}` shows `view_count == 1`.
   Different `session_id` → `view_count == 2`.
2. **Web use (AC-2):** `POST /api/v1/prompts` (auth) → 200; template
   `use_count == 1`; overview `kpis.uses >= 1`, `source_split.web >= 1`.
3. **API use (AC-2):** create alias; `GET /templates/{id}/aliases/{alias}`
   with `Authorization: ApiKey sk-...` → 200; `use_count` +1;
   `source_split.api >= 1`.
4. **Public counters (AC-3):** anonymous `GET /templates/{id}` response
   contains `view_count`/`use_count` numbers.
5. **Owner stats (AC-4/AC-9):** owner `GET /templates/{id}/stats?days=30`
   → 200 with `current`/`previous`/`series` (len == 30)/`source_split`;
   non-owner → 403; anonymous → 401; missing template → 404.
6. **Overview (AC-5/AC-7):** `GET /analytics/overview?days=7` → 200;
   `series` length 7; `top_templates` ranked by `uses`; `template_count`
   matches the user's templates.
7. **Validation:** `days=14` on both endpoints → 400; `type":"click"` → 400.
8. **Rate limit:** lower `TEMPLATE_EVENT_RATE_PER_MINUTE` (or loop 60+
   posts) → 429 with `Retry-After`.
9. **Regression (AC-12):** like/favorite toggle still returns correct
   `like_count`/`favorite_count`.

### 8.2 Unit tests (Go)

- `days` validation; `event_type` validation.
- Dedup: Redis hit, Redis error → DB fallback hit/miss/error paths.
- Rate-limit window math (reuse the `checkCommentRate` test shape).
- `zeroFill` boundaries (leap-day ranges, UTC day edges).
- Delta inputs: current/previous summary mapping.

### 8.3 Frontend tests

- `formatCount` boundaries (999, 1000, 9999, 10000, 1M; EN + ZH).
- KpiCard/RangeSelector/TopTemplatesTable render + a11y roles.
- AnalyticsPage: loading → loaded, range switch refetch, empty state,
  error + retry.

---

## 9. Implementation Order

1. DDL + backfill in both `init.sql` copies.
2. Proto messages/RPCs + regenerate `prompt.pb.go`.
3. `models.Template` fields + repository column lists.
4. `AnalyticsRepository` + unit tests.
5. Service methods (`RecordTemplateEvent`, `RecordUseEvent`,
   `GetTemplateStats`, `GetCreatorStats`, `checkEventRate`, dedup) + tests.
6. `publicRpcMethods` addition.
7. Gateway: events/stats branches, `/api/v1/analytics/overview` handler,
   both use-event hooks.
8. Frontend: api client → formatCount → components → AnalyticsPage →
   TemplateDetails integration → Sidebar/route → i18n.
9. FVT `test_template_analytics`; full `tests/fvt.py` regression.

---

## 10. Constraints for Developer

1. **Proto field numbers:** `Template.view_count = 23`, `use_count = 24`.
   18–22 are reserved by follow-creators and template-ratings — never reuse.
2. **All counts are `int32`** (protojson renders `int64` as a JSON string;
   FVT compares numbers).
3. **Both `init.sql` copies** receive the identical DDL/backfill block in the
   same commit.
4. `RecordTemplateEvent` must be added to `publicRpcMethods` (raw gRPC
   parity with the gateway's anonymous-allowed route).
5. Use-event hooks capture request values (`templateID`, `userID`,
   `authHeader`) **before** spawning the goroutine and use
   `context.Background()` inside it.
6. The `POST /api/v1/prompts` handler bypasses `PromptService.CreatePrompt`
   (it calls `promptRepo.Create` directly) — hook in the **gateway handler**,
   not in the service.
7. Never surface hook errors to callers; log only (FR-7).
8. Banned users must surface as 403 (`PermissionDenied`), never 401.
9. Do not modify unrelated handlers; do not commit other agents'
   uncommitted files that may exist in the shared worktree.
10. No monetization concepts anywhere in the implementation.
