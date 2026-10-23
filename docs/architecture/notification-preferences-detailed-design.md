# Notification Preferences — Detailed Design

- **Feature ID:** `notification-preferences`
- **Version:** 1.0
- **Status:** Approved for implementation
- **Author:** Architect Agent
- **Upstream docs:** [Requirements](../design/notification-preferences-requirements.md) · [UI/UX Design](../design/notification-preferences-ui-ux-design.md)
- **Architecture doc:** [Architecture Design (EN)](./notification-preferences-architecture.md) · [架构设计（中文）](./notification-preferences-architecture-zh-CN.md)

---

## 1. Proto Contract

The two preference RPCs are added to the existing `UserService` in
`backend/api/proto/v1/prompt.proto` (package `openprompts.v1`). They are
appended to the `UserService` service block after `ListTopCreators` (the last
user RPC before the `PromptService` block). No new proto file is needed.

```protobuf
service UserService {
  // ... existing RPCs ...

  // GetNotificationPreferences returns the caller's effective notification
  // preferences (defaults materialized when no row exists).
  rpc GetNotificationPreferences(GetNotificationPreferencesRequest) returns (NotificationPreferences);
  // UpdateNotificationPreferences applies a subset of the preference fields
  // and returns the full effective preferences.
  rpc UpdateNotificationPreferences(UpdateNotificationPreferencesRequest) returns (NotificationPreferences);
}

message GetNotificationPreferencesRequest {}

message NotificationPreferences {
  bool like_enabled = 1;
  bool favorite_enabled = 2;
  bool fork_enabled = 3;
  bool comment_enabled = 4;
  bool follow_enabled = 5;
  bool paused_all = 6;
}

message UpdateNotificationPreferencesRequest {
  // Optional bool fields: only the fields explicitly present are updated.
  // proto3 optional is used so an absent field is distinguishable from false.
  optional bool like_enabled = 1;
  optional bool favorite_enabled = 2;
  optional bool fork_enabled = 3;
  optional bool comment_enabled = 4;
  optional bool follow_enabled = 5;
  optional bool paused_all = 6;
}
```

**Field numbering:** 1–6 for the six preference fields, consistent across both
messages. No collisions (these are new messages).

**Code generation.** Same as the existing proto files (run from `backend/`):

```bash
protoc -I api/proto/v1 \
  --go_out=. --go_opt=module=open-prompts/backend \
  --go-grpc_out=. --go-grpc_opt=module=open-prompts/backend \
  api/proto/v1/prompt.proto
```

Commit the regenerated `prompt.pb.go` / `prompt_grpc.pb.go` alongside the
proto (repo convention).

## 2. Database DDL

Append to **both** `deployment/init.sql` and
`deployment/chart/open-prompts/init.sql` (byte-identical block):

```sql
-- -----------------------------------------------------------------------------
-- Feature: notification-preferences
-- Description: Per-type opt-out + master pause for in-app notifications.
--              One row per user, created lazily on first preference change
--              (absence = all-enabled / not-paused). Read at event-emission
--              time to suppress disabled/paused types.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS notification_preferences (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    like_enabled BOOLEAN NOT NULL DEFAULT true,
    favorite_enabled BOOLEAN NOT NULL DEFAULT true,
    fork_enabled BOOLEAN NOT NULL DEFAULT true,
    comment_enabled BOOLEAN NOT NULL DEFAULT true,
    follow_enabled BOOLEAN NOT NULL DEFAULT true,
    paused_all BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE notification_preferences IS
    'Per-user notification preferences; absent row = all defaults';
COMMENT ON COLUMN notification_preferences.user_id IS
    'User owning these preferences (PK, cascade-deleted with the user)';
COMMENT ON COLUMN notification_preferences.like_enabled IS
    'Receive like notifications (default true)';
COMMENT ON COLUMN notification_preferences.favorite_enabled IS
    'Receive favorite notifications (default true)';
COMMENT ON COLUMN notification_preferences.fork_enabled IS
    'Receive fork notifications (default true)';
COMMENT ON COLUMN notification_preferences.comment_enabled IS
    'Receive comment notifications (default true)';
COMMENT ON COLUMN notification_preferences.follow_enabled IS
    'Receive follow notifications (default true)';
COMMENT ON COLUMN notification_preferences.paused_all IS
    'Master pause: when true, no notification of any type is created';
COMMENT ON COLUMN notification_preferences.updated_at IS
    'Last preference change timestamp';
```

Notes:

- `CREATE TABLE IF NOT EXISTS` is idempotent — no `DO $$` block needed (no
  ALTERs).
- `user_id` is the PK and the FK to `users(id)` with `ON DELETE CASCADE`
  (FR-1). The `users` PK is `TEXT`, so the FK type matches.
- Existing deployments: run the block once via psql against the live DB (same
  as any init.sql addition), or recreate the volume.

## 3. Backend Layers

### 3.1 Model (`backend/internal/models/notification_preferences.go`)

```go
package models

import "time"

// NotificationPreferences is the per-user notification opt-out state.
// An absent row means all-enabled / not-paused (defaults).
type NotificationPreferences struct {
    UserID          string    `json:"user_id"`
    LikeEnabled     bool      `json:"like_enabled"`
    FavoriteEnabled bool      `json:"favorite_enabled"`
    ForkEnabled     bool      `json:"fork_enabled"`
    CommentEnabled  bool      `json:"comment_enabled"`
    FollowEnabled   bool      `json:"follow_enabled"`
    PausedAll       bool      `json:"paused_all"`
    UpdatedAt       time.Time `json:"updated_at"`
}
```

### 3.2 Repository (`backend/internal/repository/notification_preferences_repository.go`)

Interface (mockable, matching `service/*_test.go` style):

```go
type NotificationPreferencesRepository interface {
    // Get returns the effective preferences for a user. When no row exists,
    // it returns the all-defaults value (no error).
    Get(ctx context.Context, userID string) (*models.NotificationPreferences, error)
    // Upsert creates the row on first change or updates the provided fields.
    // Only the fields with non-nil pointers are written.
    Upsert(ctx context.Context, userID string, prefs *models.NotificationPreferences) error
}
```

**Get** — single PK point read; absent row → defaults:

```sql
SELECT user_id, like_enabled, favorite_enabled, fork_enabled,
       comment_enabled, follow_enabled, paused_all, updated_at
FROM notification_preferences
WHERE user_id = $1;
```

If `sql.ErrNoRows`, return `&models.NotificationPreferences{UserID: userID}`
(all booleans zero-value = false). **Important:** the service layer must
interpret zero-value booleans as the *defaults* (all-enabled, not-paused) for
an absent row. To keep this unambiguous, the repository returns a sentinel
`found bool` alongside the row, and the service applies defaults when
`found == false`:

```go
Get(ctx, userID) (*models.NotificationPreferences, bool, error) // prefs, found, err
```

**Upsert** — lazy row creation + partial update in one statement:

```sql
INSERT INTO notification_preferences
    (user_id, like_enabled, favorite_enabled, fork_enabled,
     comment_enabled, follow_enabled, paused_all, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, now())
ON CONFLICT (user_id) DO UPDATE SET
    like_enabled = COALESCE($2, notification_preferences.like_enabled),
    favorite_enabled = COALESCE($3, notification_preferences.favorite_enabled),
    fork_enabled = COALESCE($4, notification_preferences.fork_enabled),
    comment_enabled = COALESCE($5, notification_preferences.comment_enabled),
    follow_enabled = COALESCE($6, notification_preferences.follow_enabled),
    paused_all = COALESCE($7, notification_preferences.paused_all),
    updated_at = now();
```

The caller passes `*bool` pointers (nil = leave unchanged) for the six fields;
the repository converts nil → `NULL` so `COALESCE` keeps the existing value.

### 3.3 Service (`backend/internal/service/user.go`)

`UserService` gains a `NotificationPrefsRepo` field and a setter:

```go
type UserService struct {
    // ... existing fields ...
    NotificationPrefsRepo repository.NotificationPreferencesRepository
}

func (s *UserService) SetNotificationPrefsRepo(repo repository.NotificationPreferencesRepository) {
    s.NotificationPrefsRepo = repo
}
```

**GetNotificationPreferences:**

```go
func (s *UserService) GetNotificationPreferences(ctx context.Context, _ *pb.GetNotificationPreferencesRequest) (*pb.NotificationPreferences, error) {
    userID, err := GetUserIDFromContext(ctx)
    if err != nil {
        return nil, err
    }
    prefs, found, err := s.NotificationPrefsRepo.Get(ctx, userID)
    if err != nil {
        return nil, status.Errorf(codes.Internal, "failed to load notification preferences: %v", err)
    }
    return prefsToProto(prefs, found), nil
}
```

`prefsToProto` applies defaults when `found == false` (all booleans true,
`paused_all` false).

**UpdateNotificationPreferences:**

```go
func (s *UserService) UpdateNotificationPreferences(ctx context.Context, req *pb.UpdateNotificationPreferencesRequest) (*pb.NotificationPreferences, error) {
    userID, err := GetUserIDFromContext(ctx)
    if err != nil {
        return nil, err
    }
    // Rate limit (anti-tamper): NOTIF_PREFS_RATE_PER_MINUTE (default 30).
    if retryAfter, ok := s.checkPrefsRate(ctx, userID); !ok {
        return nil, status.Errorf(codes.ResourceExhausted,
            "notification preference changes rate limited; retry after %ds", retryAfter)
    }
    // Build the partial update from the optional fields.
    prefs := &models.NotificationPreferences{UserID: userID}
    if req.LikeEnabled != nil { prefs.LikeEnabled = *req.LikeEnabled }
    if req.FavoriteEnabled != nil { prefs.FavoriteEnabled = *req.FavoriteEnabled }
    if req.ForkEnabled != nil { prefs.ForkEnabled = *req.ForkEnabled }
    if req.CommentEnabled != nil { prefs.CommentEnabled = *req.CommentEnabled }
    if req.FollowEnabled != nil { prefs.FollowEnabled = *req.FollowEnabled }
    if req.PausedAll != nil { prefs.PausedAll = *req.PausedAll }
    if err := s.NotificationPrefsRepo.Upsert(ctx, userID, prefs); err != nil {
        return nil, status.Errorf(codes.Internal, "failed to update notification preferences: %v", err)
    }
    // Return the full effective preferences.
    updated, found, err := s.NotificationPrefsRepo.Get(ctx, userID)
    if err != nil {
        return nil, status.Errorf(codes.Internal, "failed to reload notification preferences: %v", err)
    }
    return prefsToProto(updated, found), nil
}
```

**Rate limit** (`checkPrefsRate`) mirrors `checkRatingRate` in `prompt.go`
(Redis fixed-window counter, key `notif_prefs_rate_min:<userID>`, fail-open
when Redis is nil/unavailable):

```go
const notifPrefsRatePerMinuteDef = 30

func (s *UserService) checkPrefsRate(ctx context.Context, userID string) (int, bool) {
    if s.Redis == nil {
        return 0, true
    }
    key := "notif_prefs_rate_min:" + userID
    max, _ := strconv.Atoi(os.Getenv("NOTIF_PREFS_RATE_PER_MINUTE"))
    if max <= 0 {
        max = notifPrefsRatePerMinuteDef
    }
    // ... identical fixed-window logic to checkRatingRate ...
}
```

### 3.4 Suppression hooks (event-emission choke points)

The preference check is inserted at the **three** existing emission choke
points, **after** the existing self-action suppression and **before** the
`NotificationRepo.Create` call. All hooks remain strictly best-effort: a
preference-check failure is logged and swallowed (the notification is skipped
rather than failing the engagement).

**`prompt.go` — `createEngagementNotification`** (like/favorite/fork):

```go
func (s *PromptService) createEngagementNotification(ctx context.Context, notificationType, templateID, ownerID, actorID string) {
    if s.NotificationRepo == nil || s.UserRepo == nil {
        return
    }
    if ownerID == actorID {
        return // self-action suppression (unchanged)
    }
    if !s.prefsAllow(ctx, ownerID, notificationType) {
        return // suppressed by preference
    }
    // ... existing actor lookup + Create ...
}
```

**`comment.go` — `notifyCommentReply`** (comment):

```go
func (s *PromptService) notifyCommentReply(ctx context.Context, tpl *models.Template, parentID, actorID, content string) {
    if s.NotificationRepo == nil || s.UserRepo == nil {
        return
    }
    parent, err := s.CommentRepo.Get(ctx, parentID)
    if err != nil || parent == nil {
        zap.S().Warnf("comment_reply hook: failed to load parent %s: %v", parentID, err)
        return
    }
    if parent.UserID == actorID {
        return // self-reply suppression (unchanged)
    }
    if !s.prefsAllow(ctx, parent.UserID, "comment") {
        return // suppressed by preference
    }
    // ... existing actor lookup + Create ...
}
```

**`user.go` — `emitFollowNotification`** (follow):

```go
func (s *UserService) emitFollowNotification(ctx context.Context, followeeID, followerID string) {
    if s.NotificationRepo == nil || s.Repo == nil {
        return
    }
    if !s.prefsAllow(ctx, followeeID, "follow") {
        return // suppressed by preference
    }
    // ... existing actor lookup + Create ...
}
```

**Shared helper** — a single resolution path (NFR-3) so the effective-prefs
read and the emission-time check use the same logic. Placed on a small shared
helper (e.g. `prefsAllow` on each service, delegating to a common function in
`service/prefs.go`):

```go
// prefsAllow reports whether the recipient should receive a notification of
// the given type. Absent row = allowed. Paused-all overrides everything.
// Fail-open on repository error (log + allow) so a prefs outage never blocks
// engagement notifications.
func prefsAllow(ctx context.Context, repo repository.NotificationPreferencesRepository, recipientID, notificationType string) bool {
    if repo == nil {
        return true
    }
    prefs, found, err := repo.Get(ctx, recipientID)
    if err != nil {
        zap.S().Warnf("prefs check failed for %s: %v", recipientID, err)
        return true // fail-open
    }
    if !found {
        return true // defaults: all enabled
    }
    if prefs.PausedAll {
        return false
    }
    switch notificationType {
    case "like":
        return prefs.LikeEnabled
    case "favorite":
        return prefs.FavoriteEnabled
    case "fork":
        return prefs.ForkEnabled
    case "comment", "comment_reply":
        return prefs.CommentEnabled
    case "follow":
        return prefs.FollowEnabled
    default:
        // Admin/system types (moderation, appeal_resolved, template_approved,
        // template_rejected, template_warning_changed) are NOT suppressed in
        // v1 (FR-10).
        return true
    }
}
```

**Note on `comment` vs `comment_reply`:** the requirements' five types are
`like`, `favorite`, `fork`, `comment`, `follow`. The shipped `comment_reply`
event (from comment-replies-threads) is a comment-family event; the
`comment_enabled` toggle governs both `comment` and `comment_reply` (the
`comment` type is reserved for future top-level-comment notifications). This
is documented in the UI description ("Someone comments on your template").

**In-process cache (Q2):** the `prefsAllow` helper reads through a short-TTL
(30 s) in-process cache keyed by `recipientID`, invalidated on PATCH. This
avoids a DB read per engagement while keeping suppression near-real-time. The
cache is a simple `sync.Map` + timestamp map in `service/prefs.go`; a PATCH
deletes the recipient's entry. Fail-open on cache miss → DB read.

### 3.5 Gateway wiring (`backend/cmd/server/main.go`)

- Construct `prefsRepo := repository.NewNotificationPreferencesRepository(db)`
  and wire it into `UserService` via `userSvc.SetNotificationPrefsRepo(prefsRepo)`.
- Register two REST handlers under `/api/v1/users/me/notification-preferences`
  (registered **before** the `/api/v1/users/me/following-feed` route so the
  literal path is matched first; the existing `/users/me/...` routes use exact
  paths so there is no conflict):

```go
http.HandleFunc("/api/v1/users/me/notification-preferences", func(w http.ResponseWriter, r *http.Request) {
    // CORS + OPTIONS + VerifyAnyToken → ContextWithUserID (same as profile)
    switch r.Method {
    case http.MethodGet:
        resp, err := userSvc.GetNotificationPreferences(ctx, &pb.GetNotificationPreferencesRequest{})
        // marshal + write
    case http.MethodPatch:
        body, _ := io.ReadAll(r.Body)
        var req pb.UpdateNotificationPreferencesRequest
        if err := unmarshaler.Unmarshal(body, &req); err != nil {
            http.Error(w, err.Error(), http.StatusBadRequest)
            return
        }
        resp, err := userSvc.UpdateNotificationPreferences(ctx, &req)
        // marshal + write
    default:
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    }
})
```

- The `Access-Control-Allow-Methods` header must include `PATCH`.

## 4. Frontend Design

### 4.1 File layout

```
frontend/src/
├── components/notifications/
│   └── NotificationPreferencesPanel.js   # modal: master switch + 5 type rows
├── pages/NotificationsPage.js            # add gear icon in header
├── components/notifications/NotificationBell.js  # add footer link
├── store/notificationPreferencesSlice.js
└── components/ui/Icons.jsx               # add GearIcon (or reuse)
```

### 4.2 Redux slice (`notificationPreferencesSlice.js`)

```js
{
  prefs: { like_enabled: true, favorite_enabled: true, fork_enabled: true,
           comment_enabled: true, follow_enabled: true, paused_all: false },
  saving: { like_enabled: false, favorite_enabled: false, fork_enabled: false,
            comment_enabled: false, follow_enabled: false, paused_all: false },
  error: null,
}
```

Async thunks: `fetchNotificationPreferences()` (GET, on panel open),
`updateNotificationPreference({ field, value })` (PATCH the single field,
optimistic update + rollback on error). `saving[field]` tracks the in-flight
state per row.

### 4.3 Components

**NotificationPreferencesPanel** (modal, opened by the gear icon and the bell
footer link):

- Header: title + close (× / Esc).
- **PauseAllSwitch** (master): `role="switch"`, `aria-checked`; on → greys the
  type rows with the "Paused all is on" note; off → restores prior states.
- **Five TypeToggleRows** (like/favorite/fork/comment/follow): each a labeled
  group (`aria-labelledby`) with a title, a plain-language description, and a
  switch. Auto-saves on change (PATCH); optimistic animation → "Saved ✓"
  (1.5 s, `aria-live="polite"`) or rollback + error toast (`aria-live="assertive"`).
- Footer note: "Changes save automatically."
- Loading: skeleton rows on open; error: banner + retry.
- First open: GET prefs; absent row renders all-on defaults (no flicker).

**NotificationsPage** header: add a `GearIconButton` (aria-label
`notifPrefs.open`) next to the title that opens the panel.

**NotificationBell** dropdown footer: add a "Notification preferences" link
(above or below "View all") that opens the same panel.

### 4.4 API client additions (`services/api.js`)

```js
getNotificationPreferences()                       // GET /users/me/notification-preferences
updateNotificationPreferences(patch)               // PATCH /users/me/notification-preferences
```

### 4.5 i18n keys (both locales, `notifPrefs.*`)

Reuse the key JSON specified in the UI/UX design §8 verbatim. Shape:

```json
{
  "notifPrefs": {
    "title": "Notification preferences",
    "pauseAll": "Pause all notifications",
    "pauseAllDesc": "Stop all notifications of every type.",
    "type": {
      "like": "Likes",
      "like.desc": "Someone likes your template.",
      "favorite": "Favorites",
      "favorite.desc": "Someone favorites your template.",
      "fork": "Forks",
      "fork.desc": "Someone forks your template.",
      "comment": "Comments",
      "comment.desc": "Someone comments on your template.",
      "follow": "Follows",
      "follow.desc": "Someone follows you."
    },
    "pausedNote": "Paused all is on — turn it off to adjust individual types.",
    "autoSave": "Changes save automatically.",
    "saved": "Saved",
    "error": "Couldn't save. Try again.",
    "open": "Notification preferences"
  }
}
```

The ZH locale carries equivalent strings (e.g. `通知偏好`, `暂停所有通知`,
`点赞`, `有人点赞了你的模板。`). Both locales must ship in the same commit.

## 5. Error Matrix

| Condition | gRPC code | HTTP | User-facing |
|---|---|---|---|
| Missing/invalid token | `Unauthenticated` | 401 | Global 401 → `/login` |
| Malformed body / non-boolean field | `InvalidArgument` | 400 | `notifPrefs.error` toast |
| Rate limit exceeded (PATCH) | `ResourceExhausted` | 429 | `notifPrefs.error` toast + `Retry-After` |
| Repo/DB failure | `Internal` | 500 | `notifPrefs.error` + retry state |
| Preference-check failure (hook) | — (logged) | — | None — notification skipped, engagement succeeds |

## 6. Deployment

- `docker-compose.yml` backend service environment (optional, default shown):

```yaml
NOTIF_PREFS_RATE_PER_MINUTE: "30"
```

- Helm: `values.yaml` gains `notifPrefsRatePerMinute: 30` (or a flat env map,
  matching the chart's existing convention), rendered into the backend
  deployment env.
- No new services/ports/volumes. DDL applied via the existing init.sql
  mechanism (idempotent).

## 7. SDK / CLI

No changes. `python-sdk/` wraps the public prompt/user surface; notification
preferences are a UI-first feature and the SDK surface is out of scope for v1
(requirements §8 non-goals). If SDK support is requested later, the REST
contract in §1/§4.4 is stable to wrap.

## 8. Test Strategy

### 8.1 FVT (`tests/fvt.py`) — developer

New `test_notification_preferences()` after existing tests (uses
`get_auth_token` for two distinct users):

1. Fresh user GET → all defaults (`like_enabled` etc. true, `paused_all`
   false).
2. PATCH `{ "like_enabled": false }` → GET shows only `like_enabled` false,
   others unchanged.
3. Suppression: user B likes user A's template; A has `like_enabled` false →
   A's notification list has no like notification, unread badge unchanged.
4. Pause-all: A sets `paused_all` true → B favorites/forks/comments/follows →
   A receives none of them; existing history remains browsable.
5. While paused, PATCH a type toggle is still accepted server-side (the UI
   disables it, but the API allows it — the persisted value is stored and
   takes effect once unpaused).
6. Re-enable: A sets `paused_all` false → a new like creates a notification.
7. PATCH with a non-boolean value → 400.
8. PATCH with an unknown field → ignored (200, unchanged).
9. Cross-user isolation: user C cannot read or modify A's preferences (no
   such endpoint exists — only `/me`).
10. Unauthenticated → 401 on both endpoints.
11. Rate limit: >30 PATCHes in a minute → 429 with `Retry-After`.

### 8.2 Backend unit tests (≥ 80 % on new files)

- `notification_preferences_test.go` (service): effective-prefs resolution
  (absent row → defaults), partial update (only provided fields written),
  rate-limit enforcement (mock Redis), non-boolean rejection, unknown-field
  ignore.
- `prompt_test.go` / `comment_test.go` / `user_test.go` additions: suppression
  hook fires when the type is disabled or paused; fail-open on prefs-repo
  error; self-action suppression still runs first; notification error still
  does not fail the engagement.

### 8.3 E2E (Nightwatch, test-agent)

| E2E | AC |
|---|---|
| Gear icon on notifications page opens the panel with 5 toggles (all on) + Pause all (off) | AC-1 |
| Toggling off "Likes" auto-saves (Saved ✓); a subsequent like creates no notification/badge/history | AC-2 |
| Pause all stops all five types; bell shows nothing new; history remains browsable | AC-3 |
| While paused, type toggles disabled with note; unpause restores prior states | AC-4 |
| PATCH subset updates only those fields; GET returns effective prefs | AC-5 |
| Failed PATCH rolls back the toggle + error toast | AC-6 |
| Preferences persist across sessions/devices (server-side) | AC-7 |
| User with likes disabled still generates like notifications for others | AC-8 |
| Users who never open the panel see zero behavioral difference | AC-9 |
| All strings render EN + ZH; type labels match notifications labels | AC-10 |

## 9. Implementation Order (suggested)

1. DDL (both init.sql copies).
2. `prompt.proto` additions + regenerated code.
3. Model + repository (`Get` with found sentinel, `Upsert` with COALESCE).
4. `UserService` preference methods + rate limit + gateway wiring (2 REST
   handlers).
5. Shared `prefsAllow` helper + in-process cache; insert into the three
   emission choke points (prompt/comment/user).
6. Frontend: slice + api methods → panel component → gear icon + bell footer
   link → i18n keys.
7. FVT + unit tests; `make lint`, `make ut`, `make fvt`, `npm run lint` green.
8. Compose/helm env var; fresh-volume + existing-volume verification.

## 10. Constraints for the Developer (must respect)

1. **Never commit changes to `backend/server`** (tracked 26 MB binary) —
   `git checkout -- backend/server` before committing if it shows modified.
2. Layering: proto → gateway (`main.go`) → service → repository → models; no
   new top-level packages.
3. Both preference REST routes under `/api/v1/users/me/notification-preferences`;
   **no admin surface** (FR-14); user always from the token.
4. Suppression is **receiving-only** (FR-8): never suppress the actor's own
   outgoing notifications; self-action suppression (FR-9) runs first.
5. Suppression happens at **emission time** (FR-7): a disabled/paused type
   never creates a row; suppressed events are not retrievable later.
6. Admin/system notification types (moderation, appeal_resolved,
   template_approved, template_rejected, template_warning_changed) are **not**
   suppressed in v1 (FR-10).
7. `comment_enabled` governs both `comment` and `comment_reply` events.
8. Fail-open on prefs-repo error and on Redis-unavailable rate limit; a prefs
   outage must never block engagement notifications.
9. `init.sql` changes idempotent and mirrored into the helm chart copy.
10. Env var via `os.Getenv` with safe default (30).
11. Conventional commits, English; ≥ 80 % unit-test coverage on new Go files;
    all CI gates green before push.
12. The in-process prefs cache (30 s TTL) must be invalidated on PATCH so a
    user's change takes effect immediately (FR-5).