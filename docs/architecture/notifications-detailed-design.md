# Notifications System — Detailed Design

- **Feature ID:** `notifications`
- **Version:** 1.0
- **Status:** Approved for implementation
- **Author:** Architect Agent
- **Upstream docs:** [Requirements](../design/notifications-requirements.md) · [UI/UX Design](../design/notifications-ui-ux-design.md)
- **Architecture doc:** [Architecture Design (EN)](./notifications-architecture.md) · [架构设计（中文）](./notifications-architecture-zh-CN.md)

---

## 1. Proto Contract

New file `backend/api/proto/v1/notification.proto` (package `openprompts.v1`, same as `prompt.proto` and `admin.proto`). Event creation is **not** an RPC — it is an internal repository call from the like/favorite/fork service paths.

```protobuf
syntax = "proto3";

package openprompts.v1;

option go_package = "open-prompts/backend/api/proto/v1";

// NotificationService serves per-user engagement notifications.
// All RPCs require authentication; the recipient is always derived
// from the authenticated user, never from request fields.
service NotificationService {
  // List notifications for the authenticated user.
  rpc ListNotifications(ListNotificationsRequest) returns (ListNotificationsResponse);
  // Unread badge count for the authenticated user.
  rpc GetUnreadNotificationCount(GetUnreadNotificationCountRequest) returns (GetUnreadNotificationCountResponse);
  // Mark a single notification read (idempotent).
  rpc MarkNotificationRead(MarkNotificationReadRequest) returns (MarkNotificationReadResponse);
  // Mark all notifications read.
  rpc MarkAllNotificationsRead(MarkAllNotificationsReadRequest) returns (MarkAllNotificationsReadResponse);
  // Delete a single notification.
  rpc DeleteNotification(DeleteNotificationRequest) returns (DeleteNotificationResponse);
}

message ListNotificationsRequest {
  string filter = 1;      // "all" (default) | "unread"
  int32 page_size = 2;    // 1-100, default 20
  string page_token = 3;  // offset-as-string (existing convention)
}

message Notification {
  string id = 1;
  string type = 2;              // "like" | "favorite" | "fork"
  string actor_id = 3;
  string actor_display_name = 4; // snapshot at event time
  string actor_avatar = 5;       // live join, may be empty
  string template_id = 6;
  string template_title = 7;     // live join
  string read_at = 8;            // RFC3339, empty when unread
  string created_at = 9;         // RFC3339
}

message ListNotificationsResponse {
  repeated Notification notifications = 1;
  string next_page_token = 2;
  int32 unread_count = 3;        // badge reconciliation in one call
}

message GetUnreadNotificationCountRequest {}
message GetUnreadNotificationCountResponse {
  int32 count = 1;
}

message MarkNotificationReadRequest {
  string id = 1;
}
message MarkNotificationReadResponse {
  bool success = 1;
}

message MarkAllNotificationsReadRequest {}
message MarkAllNotificationsReadResponse {
  bool success = 1;
  int32 updated_count = 2;
}

message DeleteNotificationRequest {
  string id = 1;
}
message DeleteNotificationResponse {
  bool success = 1;
}
```

**Code generation.** Same as `admin.proto` (run from `backend/`):

```bash
protoc -I api/proto/v1 \
  --go_out=. --go_opt=module=open-prompts/backend \
  --go-grpc_out=. --go-grpc_opt=module=open-prompts/backend \
  api/proto/v1/notification.proto
```

Commit the generated `notification.pb.go` / `notification_grpc.pb.go` alongside the proto (repo convention).

## 2. Database DDL

Append to **both** `deployment/init.sql` and `deployment/chart/open-prompts/init.sql` (byte-identical block):

```sql
-- ============================================
-- Notifications (engagement events)
-- ============================================
CREATE TABLE IF NOT EXISTS notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recipient_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    actor_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('like', 'favorite', 'fork')),
    actor_display_name TEXT NOT NULL,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT no_self_notify CHECK (recipient_id <> actor_id)
);

CREATE INDEX IF NOT EXISTS idx_notifications_recipient_unread
    ON notifications (recipient_id) WHERE read_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_notifications_recipient_created
    ON notifications (recipient_id, created_at DESC);
```

Notes:

- `gen_random_uuid()` is already used by `templates`/`template_versions` (pgcrypto/pg13+ built-in) — no extension change.
- No ALTER statements, so the `DO $$ ... EXCEPTION` idempotent-ALTER pattern is unnecessary; plain `IF NOT EXISTS` suffices.
- Existing deployments: run the block once via psql against the live DB (same as any init.sql addition), or recreate the volume.

## 3. Backend Layers

### 3.1 Model (`backend/internal/models/notification.go`)

```go
type Notification struct {
    ID               string
    RecipientID      string
    ActorID          string
    TemplateID       string
    Type             string // "like" | "favorite" | "fork"
    ActorDisplayName string
    ReadAt           *time.Time
    CreatedAt        time.Time
    // Joined at read time (not stored):
    ActorAvatar   *string
    TemplateTitle string
}
```

### 3.2 Repository (`backend/internal/repository/notification_repository.go`)

Interface (mockable, matching `service/*_test.go` style):

```go
type NotificationRepository interface {
    Create(ctx context.Context, n *models.Notification) error
    List(ctx context.Context, recipientID string, filter string, limit, offset int) ([]*models.Notification, int, error) // rows, unreadCount
    CountUnread(ctx context.Context, recipientID string) (int, error)
    MarkRead(ctx context.Context, recipientID, id string) (bool, error) // found?
    MarkAllRead(ctx context.Context, recipientID string) (int64, error)
    Delete(ctx context.Context, recipientID, id string) (bool, error)  // found?
    PurgeExpired(ctx context.Context, readRetentionDays, retentionDays int) error
}
```

**Create (dedupe, FR-3).** Single statement, no extra round-trip:

```sql
INSERT INTO notifications (recipient_id, actor_id, template_id, type, actor_display_name)
SELECT $1, $2, $3, $4, $5
WHERE NOT EXISTS (
    SELECT 1 FROM notifications
    WHERE recipient_id = $1 AND actor_id = $2 AND template_id = $3
      AND type = $4 AND read_at IS NULL
)
RETURNING id;
```

If zero rows are returned, the insert was deduped — **not an error**. The `no_self_notify` CHECK is a backstop; the service layer suppresses self-actions before calling Create.

**List.** One query with LEFT JOINs + a second cheap COUNT on the partial index:

```sql
SELECT n.id, n.type, n.actor_id, n.actor_display_name, u.avatar AS actor_avatar,
       n.template_id, t.title AS template_title, n.read_at, n.created_at
FROM notifications n
LEFT JOIN users u ON u.id = n.actor_id
LEFT JOIN templates t ON t.id = n.template_id
WHERE n.recipient_id = $1
  AND ($2 = 'unread' AND n.read_at IS NULL OR $2 = 'all')
ORDER BY n.created_at DESC
LIMIT $3 OFFSET $4;

SELECT COUNT(*) FROM notifications WHERE recipient_id = $1 AND read_at IS NULL;
```

**MarkAllRead:**

```sql
UPDATE notifications SET read_at = now()
WHERE recipient_id = $1 AND read_at IS NULL;
```

**PurgeExpired (lazy retention):**

```sql
DELETE FROM notifications
WHERE read_at < now() - ($1 || ' days')::interval
   OR created_at < now() - ($2 || ' days')::interval;
```

Throttled in the service layer: an in-memory `sync.Mutex`-guarded `lastPurgeAt time.Time`; run at most once per 10 minutes per instance, and only on list calls. Errors are logged and swallowed.

### 3.3 Service (`backend/internal/service/notification.go`)

```go
type NotificationService struct {
    pb.UnimplementedNotificationServiceServer
    repo NotificationRepository
    readRetentionDays int
    retentionDays     int
    lastPurgeAt       time.Time
    purgeMu           sync.Mutex
}
```

- `ListNotifications`: validate `filter ∈ {all, unread}` (default `all`), `page_size` 1–100 (default 20), `page_token` parse as integer offset (default 0) — same conventions as `ListTemplates`/`ListAPIKeys`. Run lazy purge (throttled). Map rows to pb (timestamps → `time.RFC3339` strings, matching the existing gateway marshaling style).
- `GetUnreadNotificationCount`, `MarkNotificationRead`, `MarkAllNotificationsRead`, `DeleteNotification`: recipient from `auth.UserIDFromContext(ctx)`; unknown/foreign id → `codes.NotFound`.
- `MarkNotificationRead` is idempotent: re-marking a read row returns success.

### 3.4 Event creation hooks (`backend/internal/service/prompt.go`)

`PromptService` gains a `notificationRepo NotificationRepository` dependency (constructor param in `main.go` wiring).

**ToggleLikeTemplate / ToggleFavoriteTemplate** (after the existing toggle transaction commits and only when the action was an **on**):

1. Fetch the template owner: `tpl, err := s.templateRepo.Get(ctx, templateID)` — if this fails, log and return the toggle result unchanged (the toggle already succeeded; notification is best-effort).
2. If `tpl.OwnerID == actorID` → skip (FR-2).
3. `notificationRepo.Create(ctx, &models.Notification{RecipientID: tpl.OwnerID, ActorID: actorID, TemplateID: templateID, Type: "like"|"favorite", ActorDisplayName: actorDisplayName})`. Actor display name comes from the users repository (`GetByID`) — one extra read; acceptable at engagement frequency. If it fails, log and continue.
4. The toggle response is **never** altered by notification errors (FR-7).

**ForkTemplate** (after the fork transaction commits): the source template is already loaded in-flow (`sourceTpl.OwnerID`), so no extra owner lookup — only the actor display-name read. Same suppression/dedupe/error-swallowing rules, `Type: "fork"`.

**Why after commit:** the notification must never be created for a toggle that rolled back, and a notification failure must never roll back the engagement.

### 3.5 Gateway wiring (`backend/cmd/server/main.go`)

- Construct `notificationRepo := repository.NewNotificationRepository(db)` and `notificationService := service.NewNotificationService(notificationRepo, readRetentionDays, retentionDays)`; pass `notificationRepo` into `NewPromptService(...)`.
- Register gRPC: `pb.RegisterNotificationServiceServer(grpcServer, notificationService)` in the existing goroutine.
- REST handlers follow the existing `api-keys` handler pattern (CORS + OPTIONS + `VerifyAnyToken` → `ContextWithUserID` → parse → call → `protojson` marshal with `UseProtoNames` + `EmitUnpopulated` → `writeError` mapping). One handler with path-segment routing:

```go
http.HandleFunc("/api/v1/notifications", handleNotifications)           // GET list
http.HandleFunc("/api/v1/notifications/", handleNotificationsTrailing)  // unread-count, {id}/read, read-all, {id}
```

`handleNotificationsTrailing` routes on segments: `unread-count` (GET), `read-all` (POST), `{id}/read` (POST), `{id}` (DELETE). Method mismatch → `405`. `read-all` must be matched **before** treating the first segment as an id.

## 4. Frontend Design

### 4.1 File layout

```
frontend/src/
├── components/notifications/
│   ├── NotificationBell.js        # header button + badge + polling mount
│   ├── NotificationDropdown.js    # 10 recent items, mark-all, view-all
│   └── NotificationItem.js        # shared row (dropdown + page)
├── pages/NotificationsPage.js     # /notifications history page
├── hooks/
│   ├── useNotificationPolling.js  # 60s poll, visibility, backoff
│   └── useDismissable.js          # extracted click-outside/Esc (from profile dropdown)
├── store/notificationsSlice.js
└── utils/time.js                  # formatRelativeTime(date, t)
```

### 4.2 Redux slice (`notificationsSlice.js`)

```js
{
  items: [],            // Notification[] (page-accumulated)
  unreadCount: 0,
  filter: 'all',        // 'all' | 'unread'
  nextPageToken: '',
  hasMore: false,
  loading: false,
  error: null,
  lastFetchedAt: 0,
}
```

Async thunks: `fetchNotifications({ filter, reset })`, `fetchUnreadCount()`, `markNotificationRead(id)`, `markAllNotificationsRead()`, `deleteNotification(id)`. All mutations optimistically update `items` and recompute `unreadCount` locally (no refetch needed). `fetchNotifications({ reset: true })` replaces items (dropdown refresh / tab switch); `reset: false` appends (Load more).

### 4.3 Polling hook (`useNotificationPolling.js`)

- Interval 60 s while `document.visibilityState === 'visible'` and authenticated.
- `visibilitychange` → hidden: clear interval; visible: immediate refresh + restart interval.
- `window focus` → immediate refresh (throttled to ≥ 30 s since last fetch).
- Consecutive errors → backoff 60 → 120 → 300 s (cap), reset to 60 s on success.
- Unmount/logout → stop. 401 responses are handled by the global axios interceptor (redirect to `/login`).

### 4.4 Components

**NotificationBell** (Header, between language switcher and profile, rendered only when `isAuthenticated`): button with `BellIcon` + badge (`unreadCount > 99` → "99+"); `aria-label={t('notifications.bell.ariaLabel', { count })}`; opens dropdown via `useDismissable`.

**NotificationDropdown**: fixed panel, `role="menu"`, max 10 items (`fetchNotifications({ filter: 'all', reset: true })` on open), header with title + "Mark all read", footer "View all" → `/notifications`. Empty state: bell icon + `notifications.empty.title/desc`. Items show actor avatar (fallback initial), composed sentence, relative time, unread dot; click item → mark read + navigate to template.

**NotificationItem** (shared): sentence composed client-side:

```js
// notifications.type.like = "{{name}} liked your template \"{{title}}\""
t(`notifications.type.${n.type}`, { name: n.actor_display_name, title: n.template_title })
```

**NotificationsPage** (`/notifications`, lazy route in `App.js`, inside `Layout` with `showSidebar={false}`): header with title + "Mark all read" button; tabs All/Unread with live counts; list of `NotificationItem` (20/page, Load more appends); per-item mark-read (unread dot click) and delete (trash icon → Modal confirm → toast); empty states per tab; error state with retry.

### 4.5 i18n keys (both locales, `notifications.*` + `time.*`)

Reuse the key JSON specified in the UI/UX design §9 verbatim. Shape:

```json
{
  "notifications": {
    "title": "Notifications",
    "markAllRead": "Mark all read",
    "markRead": "Mark as read",
    "delete": "Delete",
    "viewAll": "View all",
    "empty": { "title": "You're all caught up", "desc": "..." },
    "tabs": { "all": "All", "unread": "Unread" },
    "type": {
      "like": "{{name}} liked your template \"{{title}}\"",
      "favorite": "{{name}} favorited your template \"{{title}}\"",
      "fork": "{{name}} forked your template \"{{title}}\""
    },
    "bell": { "ariaLabel": "Notifications ({{count}} unread)" },
    "deleteConfirm": { "title": "Delete notification", "message": "..." },
    "loadMore": "Load more",
    "error": "Failed to load notifications"
  },
  "time": {
    "justNow": "just now",
    "minutesAgo": "{{count}}m ago",
    "hoursAgo": "{{count}}h ago",
    "daysAgo": "{{count}}d ago"
  }
}
```

The ZH locale carries equivalent sentences (e.g. `{{name}} 赞了你的模板《{{title}}》`). Both locales must ship in the same commit.

### 4.6 API client additions (`services/api.js`)

```js
getNotifications({ filter, pageSize, pageToken })   // GET /notifications
getUnreadNotificationCount()                        // GET /notifications/unread-count
markNotificationRead(id)                            // POST /notifications/{id}/read
markAllNotificationsRead()                          // POST /notifications/read-all
deleteNotification(id)                              // DELETE /notifications/{id}
```

## 5. Error Matrix

| Condition | gRPC code | HTTP | User-facing |
|---|---|---|---|
| Missing/invalid token | `Unauthenticated` | 401 | Global 401 → `/login` |
| `filter` not in {all, unread} | `InvalidArgument` | 400 | `notifications.error` toast |
| `page_size` out of 1–100 / bad `page_token` | `InvalidArgument` | 400 | `notifications.error` toast |
| Unknown or foreign notification id | `NotFound` | 404 | `notifications.error` toast |
| Method not allowed on a sub-path | — | 405 | Browser default |
| Repo/DB failure | `Internal` | 500 | `notifications.error` + retry state |
| Notification insert failure (hook) | — (logged) | — | None — engagement still succeeds (FR-7) |

## 6. Deployment

- `docker-compose.yml` backend service environment (optional, defaults shown):

```yaml
NOTIFICATION_READ_RETENTION_DAYS: "90"
NOTIFICATION_RETENTION_DAYS: "180"
```

- Helm: `values.yaml` gains `notificationReadRetentionDays: 90` / `notificationRetentionDays: 180` (or a flat env map, matching the chart's existing convention), rendered into the backend deployment env.
- No new services/ports/volumes. DDL applied via the existing init.sql mechanism (idempotent).

## 7. SDK / CLI

No changes. `python-sdk/` wraps the public prompt/user surface; notifications are a UI-first feature and the SDK surface is out of scope for v1 (requirements §8 non-goals). If SDK support is requested later, the REST contract in §1/§4.6 is stable to wrap.

## 8. Test Strategy

### 8.1 FVT (`tests/fvt.py`) — developer

New `test_notifications()` after existing tests (uses `get_auth_token` for two distinct users):

1. User B likes/favorites/forks user A's public template → A's list has 3 notifications, correct types, unread_count = 3.
2. Self-action: A likes own template → no new notification.
3. Dedupe: B un-likes → re-likes while unread → still 1 like-notification.
4. List: `filter=unread` vs `all`; `page_size=2` + `page_token` pagination; `unread_count` present in list response.
5. Mark read: POST `/{id}/read` → success; repeat → still success (idempotent); unread_count drops.
6. Read-all: POST `/read-all` → `updated_count` matches; unread_count = 0.
7. Delete: DELETE `/{id}` → success; repeat → 404.
8. Cross-user: B tries to read/delete A's notification id → 404.
9. Cascade: A deletes the template → its notifications vanish from A's list.
10. Unauthenticated: no token → 401 on all 5 endpoints.

### 8.2 Backend unit tests (≥ 80 % on new files)

- `notification_test.go` (service): filter/page validation, recipient scoping, NotFound on foreign id, idempotent mark-read, purge throttle (first call runs, immediate second call skipped), unread_count mapping.
- `prompt_test.go` additions: hook fires on toggle-on only, not on toggle-off; self-action suppressed; notification error does not fail the toggle (mock repo returning error → toggle still succeeds); fork hook uses in-flow owner.

### 8.3 E2E (Nightwatch, test-agent)

| E2E | AC |
|---|---|
| Bell hidden when logged out; visible with badge after engagement | AC-1/2 |
| Dropdown: 10 items, mark-all-read clears badge, View all navigates | AC-3 |
| History page: tabs with counts, 20/page Load more | AC-4 |
| Per-item mark-read (dot disappears, count drops), delete with confirm | AC-5 |
| Sentence rendering EN + ZH, relative time | AC-6 |
| Empty states (all caught up / no unread) | AC-7 |
| Polling: badge updates within 60 s of an external engagement | AC-8 |

## 9. Implementation Order (suggested)

1. DDL (both init.sql copies).
2. `notification.proto` + generated code.
3. Model + repository (dedupe insert, list+count, mark/read/delete, purge).
4. `NotificationService` + gateway wiring (5 REST handlers).
5. Hook sites in `prompt.go` (toggle-on + fork) with error swallowing.
6. Frontend: slice + api methods → bell/dropdown → page → polling hook → i18n keys.
7. FVT + unit tests; `make lint`, `make ut`, `make fvt`, `npm run lint` green.
8. Compose/helm env vars; fresh-volume + existing-volume verification.

## 10. Constraints for the Developer (must respect)

1. **Never commit changes to `backend/server`** (tracked 26 MB binary) — `git checkout -- backend/server` before committing if it shows modified.
2. Layering: proto → gateway (`main.go`) → service → repository → models; no new top-level packages.
3. All notification REST routes under `/api/v1/notifications*`; **no admin surface** (FR-29/30); recipient always from the token.
4. Event creation is internal-only (no create RPC); hooks run **after** the engagement transaction commits; hook errors are logged and swallowed (FR-7).
5. Dedupe via the `INSERT ... WHERE NOT EXISTS` statement (§3.2); self-suppression in service + DB CHECK.
6. i18n: every new user-visible string in **both** `en` and `zh` locales (`notifications.*`, `time.*`).
7. `init.sql` changes idempotent and mirrored into the helm chart copy.
8. Env vars via `os.Getenv` with safe defaults (90/180).
9. Conventional commits, English; ≥ 80 % unit-test coverage on new Go files; all CI gates green before push.
10. Polling must never run when unauthenticated or tab hidden; backoff capped at 300 s.
