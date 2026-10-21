# Moderation Appeals — Detailed Design

| | |
|---|---|
| **Feature ID** | moderation-appeals |
| **Version** | 1.0 |
| **Status** | Approved for implementation |
| **Author** | Architect Agent |
| **Upstream** | [Architecture](moderation-appeals-architecture.md) |

## 1. Proto Contract

### 1.1 `backend/api/proto/v1/prompt.proto`

Append to the `PromptService` service block (after `DeleteRating`, line 130):

```protobuf
service PromptService {
    // ... existing RPCs ...
    rpc CreateAppeal(CreateAppealRequest) returns (Appeal);
    rpc ListAppeals(ListAppealsRequest) returns (ListAppealsResponse);
    rpc GetAppeal(GetAppealRequest) returns (Appeal);
}

message CreateAppealRequest {
    string moderation_action_id = 1;
    string statement = 2; // 50-2000 chars
}

message ListAppealsRequest {
    int32 page = 1;      // 1-based
    int32 per_page = 2;  // default 20
}

message ListAppealsResponse {
    repeated Appeal appeals = 1;
    int64 total = 2;
    int32 page = 3;
    int32 per_page = 4;
}

message GetAppealRequest {
    string appeal_id = 1;
}

message Appeal {
    string id = 1;
    string moderation_action_id = 2;
    string template_id = 3;
    string template_title = 4;      // joined at read time
    string action_type = 5;         // "unpublish" | "lock" | "soft_delete"
    string action_reason = 6;       // the original moderation reason
    string actor_display_name = 7;  // the original action's actor
    google.protobuf.Timestamp action_created_at = 8;
    string statement = 9;
    string state = 10;              // open | under_review | resolved_upheld | resolved_reversed
    string resolution_note = 11;    // empty until resolved
    google.protobuf.Timestamp created_at = 12;
    google.protobuf.Timestamp resolved_at = 13;
    bool superseded = 14;           // the underlying action was undone by a direct restore
}
```

### 1.2 `backend/api/proto/v1/admin.proto`

Append to the `AdminService` service block (after `ListWarningHistory`, line 81):

```protobuf
service AdminService {
    // ... existing RPCs ...
    rpc ListAppealQueue(ListAppealQueueRequest) returns (ListAppealQueueResponse);
    rpc GetAppeal(GetAdminAppealRequest) returns (AdminAppealDetail);
    rpc ReviewAppeal(ReviewAppealRequest) returns (AdminAppealDetail);
    rpc ResolveAppeal(ResolveAppealRequest) returns (AdminAppealDetail);
}

message ListAppealQueueRequest {
    string state = 1;   // "" | "open" | "under_review" | "resolved"
    int32 page = 2;
    int32 per_page = 3;
}

message ListAppealQueueResponse {
    repeated AdminAppealEntry entries = 1;
    int32 open_count = 2;
    int32 under_review_count = 3;
    int32 resolved_count = 4;
    int64 total = 5;
    int32 page = 6;
    int32 per_page = 7;
}

message AdminAppealEntry {
    string id = 1;
    string template_id = 2;
    string template_title = 3;
    string action_type = 4;
    string statement_excerpt = 5;   // first ~80 chars
    string state = 6;
    google.protobuf.Timestamp created_at = 7;
    string appellant_display_name = 8;
}

message GetAdminAppealRequest {
    string appeal_id = 1;
}

message ReviewAppealRequest {
    string appeal_id = 1;
}

message ResolveAppealRequest {
    string appeal_id = 1;
    string outcome = 2;  // "upheld" | "reversed"
    string note = 3;     // 20-1000 chars, required
}

message AdminAppealDetail {
    string id = 1;
    string moderation_action_id = 2;
    string template_id = 3;
    string template_title = 4;
    string action_type = 5;
    string action_reason = 6;
    string action_actor_id = 7;
    string action_actor_email = 8;
    google.protobuf.Timestamp action_created_at = 9;
    string statement = 10;
    string state = 11;
    string appellant_id = 12;
    string appellant_display_name = 13;
    string resolution_note = 14;
    string resolver_id = 15;
    google.protobuf.Timestamp created_at = 16;
    google.protobuf.Timestamp resolved_at = 17;
    bool superseded = 18;
    string audit_entry_id = 19;   // the original action's audit entry id (if any)
}
```

Regenerate:

```bash
cd backend && protoc --go_out=. --go-grpc_out=. api/proto/v1/prompt.proto api/proto/v1/admin.proto
```

## 2. Database DDL

Append to **both** `deployment/init.sql` and
`deployment/chart/open-prompts/init.sql`:

```sql
-- ===== moderation-appeals =====
-- Moderation actions (unpublish/lock/soft-delete). Populated by
-- admin-template-moderation when it lands; read by appeals.
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

COMMENT ON TABLE moderation_actions IS 'Moderation actions on templates; appeals reference these';
COMMENT ON COLUMN moderation_actions.status IS 'active (appealable) | superseded (undone by direct restore) | reversed (undone by appeal)';

CREATE INDEX IF NOT EXISTS idx_moderation_actions_template
    ON moderation_actions (template_id);
CREATE INDEX IF NOT EXISTS idx_moderation_actions_status_created
    ON moderation_actions (status, created_at);

-- Appeals: one per moderation action.
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

COMMENT ON TABLE appeals IS 'Creator appeals against moderation actions; one per action';
COMMENT ON COLUMN appeals.state IS 'open | under_review | resolved_upheld | resolved_reversed';

CREATE INDEX IF NOT EXISTS idx_appeals_appellant_created
    ON appeals (appellant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_appeals_state_created
    ON appeals (state, created_at);

-- Widen admin_audit_logs.action CHECK for appeal actions (guarded, idempotent).
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'admin_audit_logs'::regclass AND conname = 'admin_audit_logs_action_check';
    IF def IS NOT NULL AND def NOT LIKE '%appeal.create%' THEN
        ALTER TABLE admin_audit_logs DROP CONSTRAINT admin_audit_logs_action_check;
        ALTER TABLE admin_audit_logs ADD CONSTRAINT admin_audit_logs_action_check
            CHECK (action IN ('ban', 'unban', 'role_change', 'delete_comment',
                              'review_approve', 'review_reject', 'settings_change',
                              'template.warning_add', 'template.warning_remove',
                              'appeal.create', 'appeal.review', 'appeal.resolve'));
    END IF;
END $$;

-- Widen notifications.type CHECK for appeal_resolved (guarded, idempotent).
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'notifications'::regclass AND conname = 'notifications_type_check';
    IF def IS NOT NULL AND def NOT LIKE '%appeal_resolved%' THEN
        ALTER TABLE notifications DROP CONSTRAINT notifications_type_check;
        ALTER TABLE notifications ADD CONSTRAINT notifications_type_check
            CHECK (type IN ('like', 'favorite', 'fork', 'comment',
                            'template_approved', 'template_rejected',
                            'template_warning_changed', 'follow',
                            'appeal_resolved'));
    END IF;
END $$;
```

## 3. Backend Layers

### 3.1 `models` (`backend/internal/models/appeal.go`)

```go
type ModerationAction struct {
    ID            string     `gorm:"primaryKey"`
    TemplateID    string
    ActionType    string     // "unpublish" | "lock" | "soft_delete"
    ActorID       string
    ActorEmail    string
    Reason        string
    Status        string     // "active" | "superseded" | "reversed"
    CreatedAt     time.Time
    SupersededAt  *time.Time
}

type Appeal struct {
    ID                 string     `gorm:"primaryKey"`
    ModerationActionID string
    AppellantID        string
    Statement          string
    State              string     // open | under_review | resolved_upheld | resolved_reversed
    ResolverID         string
    ResolutionNote     string
    CreatedAt          time.Time
    UpdatedAt          time.Time
    ResolvedAt         *time.Time
}
```

### 3.2 `AppealRepository` (`backend/internal/repository/appeal_repository.go`)

```go
type AppealRepository interface {
    CreateAppeal(ctx context.Context, a *models.Appeal) error
    GetAppeal(ctx context.Context, appealID string) (*models.Appeal, error)
    GetAppealByAction(ctx context.Context, actionID string) (*models.Appeal, error)
    ListAppeals(ctx context.Context, appellantID string, page, perPage int) ([]models.Appeal, int64, error)
    GetModerationAction(ctx context.Context, actionID string) (*models.ModerationAction, error)
    ListModerationActionsForTemplate(ctx context.Context, templateID string) ([]models.ModerationAction, error)
    ListQueue(ctx context.Context, state string, page, perPage int) ([]models.Appeal, int64, error)
    CountByState(ctx context.Context) (open, underReview, resolved int, err error)
    SetState(ctx context.Context, appealID, state string) error
    ResolveAppeal(ctx context.Context, appealID, outcome, note, resolverID string) error // atomic with restore
    MarkActionSuperseded(ctx context.Context, actionID string) error
}
```

`CreateAppeal` SQL:

```sql
INSERT INTO appeals (moderation_action_id, appellant_id, statement)
VALUES ($1, $2, $3)
RETURNING id, created_at;
```

The `UNIQUE (moderation_action_id)` constraint surfaces a `23505` unique
violation → mapped to `409 ALREADY_EXISTS`.

`ListAppeals` (newest first):

```sql
SELECT a.*, ma.template_id, ma.action_type, ma.reason AS action_reason,
       ma.actor_email, ma.created_at AS action_created_at, ma.status AS action_status,
       t.title AS template_title
FROM appeals a
JOIN moderation_actions ma ON ma.id = a.moderation_action_id
JOIN templates t ON t.id = ma.template_id
WHERE a.appellant_id = $1
ORDER BY a.created_at DESC
LIMIT $2 OFFSET $3;
```

`ListQueue` (oldest first within state):

```sql
SELECT a.*, ma.template_id, ma.action_type, t.title AS template_title,
       u.display_name AS appellant_display_name
FROM appeals a
JOIN moderation_actions ma ON ma.id = a.moderation_action_id
JOIN templates t ON t.id = ma.template_id
JOIN users u ON u.id = a.appellant_id
WHERE ($1 = '' OR a.state = $1)
ORDER BY a.created_at ASC
LIMIT $2 OFFSET $3;
```

`ResolveAppeal` — the atomic resolve+restore transaction (FR-7):

```sql
BEGIN;
-- 1) lock the appeal row and re-check state
SELECT state FROM appeals WHERE id = $1 FOR UPDATE;
--    (if already resolved -> abort with 409)

-- 2) update the appeal
UPDATE appeals
SET state = CASE WHEN $2 = 'reversed' THEN 'resolved_reversed'
                 ELSE 'resolved_upheld' END,
    resolver_id = $3,
    resolution_note = $4,
    resolved_at = now(),
    updated_at = now()
WHERE id = $1;

-- 3) if reversed, restore the template (reuse admin-template-moderation
--    restore semantics) and mark the action reversed
UPDATE moderation_actions SET status = 'reversed' WHERE id = $5;
UPDATE templates SET moderation_status = 'normal' WHERE id = $6;
COMMIT;
```

The restore sets `moderation_status = 'normal'` (the same remedy
`admin-template-moderation` FR-5 applies). For a `lock` action the template
was already `normal`-visible; the restore is a no-op on status but the action
is still marked `reversed`. The audit entry for the reversal reuses
`template.restore` with `appeal=<id>` in the reason (OQ-3).

### 3.3 `PromptService` (`backend/internal/service/prompt.go`)

`CreateAppeal`:

1. `GetUserIDFromContext` → `Unauthenticated` on failure.
2. Validate `statement` length ∈ [50, 2000] → else `InvalidArgument`.
3. `AppealRepo.GetModerationAction(actionID)` → missing → `NotFound`.
4. Load the template (`TemplateRepo.Get`); verify `tpl.OwnerID == userID` →
   else `NotFound` (no existence leak).
5. If `action.Status == "superseded"` → `AlreadyExists` ("superseded").
6. Open-appeal rate limit (below) → `ResourceExhausted` on limit.
7. `AppealRepo.CreateAppeal` → unique violation → `AlreadyExists`.
8. Increment the Redis open-appeal counter.
9. Write audit `appeal.create` (target_type `template`, target_id
   `template_id`, reason `appeal=<id>`).

`ListAppeals` / `GetAppeal`: auth, then `AppealRepo`; `GetAppeal` verifies
`appeal.AppellantID == userID` → else `NotFound`.

**Rate limiting** (Redis counter, fail-open):

```
key:    appeal_open:{user_id}
value:  "{count}:{window_expires_unix}"
window: sliding, limit APPEAL_MAX_OPEN (default 5)
```

Increment on create; decrement on resolution (admin path). When the count
reaches the limit, further creates return `429 RESOURCE_EXHAUSTED` with
`Retry-After`.

### 3.4 `AdminService` (`backend/internal/service/admin.go`)

`ListAppealQueue`: `requireAdminContext`, then `AppealRepo.ListQueue` +
`CountByState` → `ListAppealQueueResponse`.

`GetAppeal`: `requireAdminContext`, then `AppealRepo.GetAppeal` +
`GetModerationAction` → `AdminAppealDetail` (with `audit_entry_id` looked up
from `AdminRepo.ListAuditByTarget` if the action has one).

`ReviewAppeal`: `requireAdminContext`; verify state is `open` → else
`AlreadyExists`; `AppealRepo.SetState(id, "under_review")`; write audit
`appeal.review`.

`ResolveAppeal`:

1. `requireAdminContext`.
2. Validate `outcome` ∈ {upheld, reversed} and `note` length ∈ [20, 1000] →
   else `InvalidArgument`.
3. Load the appeal + its action; if `action.ActorID == admin.ID` →
   `PermissionDenied` (self-guard, FR-9).
4. If appeal already resolved → `AlreadyExists`.
5. If `action.Status == "superseded"` → `AlreadyExists`.
6. `AppealRepo.ResolveAppeal` (atomic; restores template when reversed).
7. Decrement the Redis open-appeal counter.
8. Write audit `appeal.resolve` (reason `outcome=<outcome> appeal=<id>`).
9. `notifyAppealResolved` (below) → `appeal_resolved` notification.

`notifyAppealResolved` (mirrors `notifyReviewDecision`):

```go
func (s *AdminService) notifyAppealResolved(ctx context.Context, actor *models.User,
    appellantID, templateID, outcome, note string) {
    if s.NotificationRepo == nil { return }
    excerpt := note
    if len([]rune(excerpt)) > 140 { excerpt = string([]rune(excerpt)[:140]) + "…" }
    if err := s.NotificationRepo.Create(ctx, &models.Notification{
        RecipientID:      appellantID,
        ActorID:          actor.ID,
        TemplateID:       templateID,
        Type:             "appeal_resolved",
        ActorDisplayName: actor.DisplayName,
    }); err != nil {
        zap.S().Errorf("appeal resolved notification: failed: %v", err)
    }
}
```

The note excerpt (140 chars) is rendered client-side from the notification
payload; the deep link points to `/appeals/{id}`.

### 3.5 Gateway (`backend/cmd/server/main.go`)

**User routes** — add after the `/api/v1/notifications/` handler:

```go
// /api/v1/appeals — POST create, GET list (auth).
http.HandleFunc("/api/v1/appeals", func(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodPost: // auth -> CreateAppealRequest{ModerationActionId, Statement}
    case http.MethodGet:  // auth -> ListAppealsRequest{Page, PerPage}
    default: http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    }
})

// /api/v1/appeals/{id} — GET detail (auth, owner-only).
http.HandleFunc("/api/v1/appeals/", func(w http.ResponseWriter, r *http.Request) {
    id := strings.TrimPrefix(r.URL.Path, "/api/v1/appeals/")
    if r.Method == http.MethodGet { // auth -> GetAppealRequest{AppealId: id}
    }
})
```

**Admin routes** — add after the `/api/v1/admin/settings` handler:

```go
// /api/v1/admin/appeals — GET queue (admin).
http.HandleFunc("/api/v1/admin/appeals", func(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodGet { // admin -> ListAppealQueueRequest{State, Page, PerPage}
    }
})

// /api/v1/admin/appeals/{id} — GET detail, POST review/resolve (admin).
http.HandleFunc("/api/v1/admin/appeals/", func(w http.ResponseWriter, r *http.Request) {
    rest := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/appeals/")
    switch {
    case strings.HasSuffix(rest, "/review"):
        // POST -> ReviewAppealRequest{AppealId}
    case strings.HasSuffix(rest, "/resolve"):
        // POST -> ResolveAppealRequest{AppealId, Outcome, Note}
    default:
        // GET -> GetAdminAppealRequest{AppealId: rest}
    }
})
```

All routes require the Bearer-token auth dance (user or admin JWT as
appropriate). `Retry-After` is set on 429 responses.

## 4. Frontend

### 4.1 New Files

| File | Purpose |
|---|---|
| `frontend/src/components/AppealCtaSlot.js` + `.scss` | Owner banner / removed-notice slot: CTA, status chip, or outcome chip + note excerpt. |
| `frontend/src/components/AppealFormModal.js` + `.scss` | Read-only action context + statement textarea (50–2000) with live counter; 409 → status card. |
| `frontend/src/components/AppealStateChip.js` + `.scss` | open / under review / upheld / reversed chip. |
| `frontend/src/pages/AppealsPage.js` + `.scss` | `/appeals` — own appeals list. |
| `frontend/src/pages/AppealDetailPage.js` + `.scss` | `/appeals/:id` — statement + action context + resolution. |
| `frontend/src/pages/admin/AppealsPage.js` + `.scss` | `/admin/appeals` — queue with state tabs + counts. |
| `frontend/src/pages/admin/AppealDetailPage.js` + `.scss` | `/admin/appeals/:id` — two-pane detail + Review/Resolve. |

### 4.2 Modified Files

| File | Change |
|---|---|
| `frontend/src/App.js` | Add `/appeals`, `/appeals/:id` user routes; `/admin/appeals`, `/admin/appeals/:id` admin routes. |
| `frontend/src/services/api.js` | `createAppeal`, `listAppeals`, `getAppeal`, `listAdminAppeals`, `getAdminAppeal`, `reviewAppeal`, `resolveAppeal`. |
| Owner banner / removed-notice component | Mount `AppealCtaSlot` (from `admin-template-moderation` FR-12). |
| Notification center item renderer | Render `appeal_resolved` with outcome + note excerpt + deep link. |

### 4.3 Redux

No new slice: appeal state arrives with API responses; the form modal and
list pages manage local state. The `AppealCtaSlot` fetches the appeal status
for the current action on mount.

### 4.4 API Client

```js
createAppeal({ moderation_action_id, statement }) // POST /appeals
listAppeals(params)                                // GET  /appeals
getAppeal(id)                                      // GET  /appeals/:id
listAdminAppeals(params)                           // GET  /admin/appeals
getAdminAppeal(id)                                 // GET  /admin/appeals/:id
reviewAppeal(id)                                   // POST /admin/appeals/:id/review
resolveAppeal(id, { outcome, note })               // POST /admin/appeals/:id/resolve
```

### 4.5 i18n Keys (EN/ZH parity)

```json
{
  "appeal": {
    "cta": {
      "appeal": "Appeal this decision",
      "onePerAction": "One appeal per action",
      "status": {
        "open": "Appeal open",
        "underReview": "Appeal under review",
        "reversed": "Appeal reversed — template restored",
        "upheld": "Appeal upheld"
      }
    },
    "form": {
      "title": "Appeal this decision",
      "actionContext": "Action",
      "statement": "Why should this decision be reversed?",
      "statementHint": "One appeal per action. Be specific and factual.",
      "submit": "Submit appeal",
      "conflict": "This action has already been appealed",
      "tooShort": "Statement must be at least 50 characters",
      "tooLong": "Statement must be at most 2000 characters"
    },
    "list": {
      "title": "My appeals",
      "empty": "No appeals — nothing to contest"
    },
    "detail": {
      "statement": "Your statement",
      "resolution": "Resolution",
      "pending": "Pending"
    },
    "state": {
      "open": "Open",
      "under_review": "Under review",
      "upheld": "Upheld",
      "reversed": "Reversed"
    },
    "superseded": "Superseded",
    "admin": {
      "queue": { "title": "Appeals", "empty": "No appeals in this state" },
      "detail": {
        "originalAction": "Original action",
        "review": "Review",
        "resolve": "Resolve appeal",
        "outcome": "Outcome",
        "upheld": "Upheld",
        "reversed": "Reversed",
        "note": "Note (required, shown to the creator)",
        "noteRule": "20–1000 characters",
        "selfGuard": "The original action's actor cannot resolve this appeal"
      },
      "confirm": { "reversed": "Reversing will restore the template to normal" },
      "toast": { "resolved": "Appeal resolved" }
    },
    "notif": { "resolved": "Your appeal was {{outcome}}: {{note}}" },
    "rateLimit": "You can have at most 5 open appeals"
  }
}
```

ZH mirrors every key per the UI/UX doc §8 (`appeal.cta.appeal` = "申诉此决定",
`appeal.state.open` = "处理中", etc.).

## 5. Error Matrix

| Scenario | HTTP | gRPC | Frontend handling |
|---|---|---|---|
| Statement/note length violation | 400 | `INVALID_ARGUMENT` | Field-level error; form unchanged |
| Not authenticated | 401 | `UNAUTHENTICATED` | Redirect to `/login` |
| Non-admin admin route | 403 | `PERMISSION_DENIED` | Admin console error toast |
| Resolver is original actor | 403 | `PERMISSION_DENIED` | Resolve form disabled with self-guard message |
| Action/appeal not found or not owned | 404 | `NOT_FOUND` | 404 page |
| Action already appealed | 409 | `ALREADY_EXISTS` | Form replaced by existing appeal status card |
| Appeal already resolved | 409 | `ALREADY_EXISTS` | Toast; detail refreshes |
| Superseded action | 409 | `ALREADY_EXISTS` | Entry point hidden; "superseded" tag |
| Open-appeal limit | 429 | `RESOURCE_EXHAUSTED` | Toast with limit message |
| Redis down | — | — | Fail-open server-side; no user impact |

## 6. Deployment

- `APPEAL_MAX_OPEN=5` added to `deployment/docker-compose.yml` backend
  environment and the Helm chart (`values.yaml` `backend.env.appealMaxOpen` +
  `templates/deployment-backend.yaml` env entry).
- DDL additive/idempotent; both init.sql copies in the same commit.
- No new services, ports, or infrastructure.

## 7. SDK

No python-sdk changes: appeals are a web-surface feature (owner + admin
console). The REST endpoints are documented in the API surface; the SDK wraps
template resolution only.

## 8. Test Strategy

### 8.1 FVT (`tests/fvt.py`) — `test_moderation_appeals`

Setup: alice (creator), bob (admin), carol (admin). Seed a `moderation_actions`
row directly via SQL (admin-template-moderation is not yet implemented) or via
a test-only helper, targeting one of alice's templates.

1. alice creates an appeal → `open`; `GET /api/v1/appeals` lists it.
2. alice creates a second appeal for the same action → 409.
3. Statement < 50 or > 2000 chars → 400.
4. bob (admin) `GET /api/v1/admin/appeals?state=open` → queue oldest-first,
   counts correct.
5. bob `POST /review` → `under_review`; alice sees the state on `/appeals`.
6. bob `POST /resolve {outcome: reversed, note}` → template
   `moderation_status` becomes `normal`; alice gets an `appeal_resolved`
   notification; audit has `appeal.resolve`.
7. A second appeal: bob `POST /resolve {outcome: upheld, note}` → template
   untouched; notification states upheld.
8. carol (the original action's actor) attempts to resolve → 403.
9. Non-admin attempts admin routes → 403.
10. alice with 5 open appeals submits a 6th → 429.
11. A superseded action (status flipped to `superseded`) → create → 409;
    resolve → 409.
12. Statement containing `<script>` renders as literal text (no HTML
    interpretation).

### 8.2 Unit Tests

- Repository: `CreateAppeal` unique-violation mapping, `ListQueue` ordering +
  counts, `ResolveAppeal` transaction (reversed restores, upheld no-op,
  already-resolved abort), state-transition guards.
- Service: validation matrix, owner/visibility checks, self-guard, rate-limit
  counter (fake Redis), audit + notification emission.
- Update `MockAppealRepository` with the new methods.

### 8.3 E2E (test-agent, Nightwatch)

- Form modal: open from banner, live counter, submit, 409 → status card.
- `/appeals` list + detail rendering; state chips.
- Admin queue tabs with counts; detail two-pane; Review; Resolve confirm
  dialog; self-guard disabled state.
- `appeal_resolved` notification rendering with note excerpt + deep link.
- i18n EN/ZH parity for all `appeal.*` keys.

## 9. Implementation Order

1. DDL (both init.sql copies) + models + `AppealRepository` + unit tests.
2. Proto changes + regenerate.
3. `PromptService` user RPCs + rate limit + Redis counter wiring.
4. `AdminService` admin RPCs + self-guard + audit + notification.
5. Gateway user + admin routes.
6. Frontend: api client → components → pages → routes → i18n.
7. FVT + E2E.

## 10. Constraints for Developer

1. **Never commit `backend/server`** — tracked build artifact; `git checkout
   -- backend/server` if modified.
2. Stage only your own files; never `git add -A`.
3. Both init.sql copies in sync in the same commit.
4. `moderation_actions` is populated by `admin-template-moderation` when it
   lands — do not implement unpublish/lock/soft-delete here; only read the
   table. For FVT, seed rows directly.
5. The `appeals.moderation_action_id` UNIQUE constraint is the FR-3 backstop;
   map the `23505` violation to `409 ALREADY_EXISTS`.
6. `ResolveAppeal` must be atomic: appeal update + template restore + action
   status flip in one transaction.
7. The resolver must never be the original action's actor (FR-9); enforce in
   the service before the transaction.
8. Statements and notes are plain text — never render as HTML/Markdown
   (XSS-safe by construction, NFR-5).
9. The reversal audit reuses `template.restore` with `appeal=<id>` in the
   reason (OQ-3) — do not add a new audit action code.
10. No monetization surfaces of any kind.