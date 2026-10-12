# Follow Creators — Detailed Design

| | |
|---|---|
| **Feature ID** | follow-creators |
| **Version** | 1.0 |
| **Status** | Approved for implementation |
| **Author** | Architect Agent |
| **Upstream** | [Architecture](follow-creators-architecture.md) |

## 1. Proto Contract

Additions to `backend/api/proto/v1/prompt.proto`. Field numbers continue the
existing sequences; `Template` fields 17–19 are coordinated with the
template-comments design (17 = `comment_count`, reserved there).

```protobuf
// UserService — append to the existing service block (line 10)
service UserService {
    // ... existing RPCs ...
    rpc FollowUser(FollowUserRequest) returns (FollowUserResponse);
    rpc UnfollowUser(FollowUserRequest) returns (FollowUserResponse);
    rpc GetCreatorProfile(GetCreatorProfileRequest) returns (GetCreatorProfileResponse);
    rpc ListFollowers(ListUserRelationsRequest) returns (ListUserRelationsResponse);
    rpc ListFollowing(ListUserRelationsRequest) returns (ListUserRelationsResponse);
    rpc GetFollowingFeed(GetFollowingFeedRequest) returns (GetFollowingFeedResponse);
}

message FollowUserRequest {
    string user_id = 1;     // followee
    string follower_id = 2; // injected by the gateway from auth context
}

message FollowUserResponse {
    bool following = 1;
    int32 follower_count = 2;
}

message GetCreatorProfileRequest {
    string user_id = 1;
    string viewer_id = 2; // empty for anonymous
}

message GetCreatorProfileResponse {
    string id = 1;
    string display_name = 2;
    string avatar = 3;
    string bio = 4;
    int32 follower_count = 5;
    int32 following_count = 6;
    google.protobuf.Timestamp created_at = 7;
    bool is_following = 8;
    bool follows_you = 9;
}

message ListUserRelationsRequest {
    string user_id = 1;
    int32 page = 2;      // 1-based, default 1
    int32 page_size = 3; // default 20, max 50
}

message UserCard {
    string id = 1;
    string display_name = 2;
    string avatar = 3;
    int32 follower_count = 4;
    bool is_following = 5;
}

message ListUserRelationsResponse {
    repeated UserCard users = 1;
    int32 total_count = 2;
    int32 page = 3;
    int32 page_size = 4;
}

message GetFollowingFeedRequest {
    string user_id = 1;    // the follower (me)
    int32 page_size = 2;   // default 20, max 50
    string page_token = 3; // opaque created_at cursor
}

message GetFollowingFeedResponse {
    repeated Template templates = 1;
    int32 total_count = 2;
    string next_page_token = 3;
}

message OwnerInfo {
    string id = 1;
    string display_name = 2;
    string avatar = 3;
    int32 follower_count = 4;
    bool is_following = 5;
}
```

Existing-message additions:

```protobuf
message UpdateProfileRequest {
    // ... existing fields 1-4 ...
    string bio = 5; // optional; applied as-sent (empty clears)
}

message Template {
    // ... existing fields 1-16; comment_count = 17 (comments feature) ...
    string owner_display_name = 18;
    string owner_avatar = 19;
}

message GetTemplateResponse {
    Template template = 1;
    TemplateVersion latest_version = 2;
    OwnerInfo owner = 3;
}
```

`admin.proto` — `UserStats` (line 122) gains:

```protobuf
message UserStats {
    // ... existing fields 1-4 ...
    int32 follower_count = 5;
    int32 following_count = 6;
}
```

Regenerate with the project's existing command:

```bash
cd backend && protoc --go_out=. --go-grpc_out=. api/proto/v1/prompt.proto api/proto/v1/admin.proto
```

## 2. Database DDL

Append to **both** `deployment/init.sql` and
`deployment/chart/open-prompts/init.sql` (kept in sync):

```sql
-- ===== follow-creators =====
CREATE TABLE IF NOT EXISTS user_follows (
    follower_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    followee_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (follower_id, followee_id),
    CONSTRAINT chk_no_self_follow CHECK (follower_id <> followee_id)
);

CREATE INDEX IF NOT EXISTS idx_user_follows_followee
    ON user_follows(followee_id, created_at DESC);

ALTER TABLE users ADD COLUMN IF NOT EXISTS bio TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS follower_count INT NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS following_count INT NOT NULL DEFAULT 0;

-- 'follow' notifications carry no template: make template_id nullable
-- (guarded, idempotent, order-independent vs. the notifications feature).
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'notifications')
       AND EXISTS (
           SELECT 1 FROM pg_attribute
           WHERE attrelid = 'notifications'::regclass
             AND attname = 'template_id' AND attnotnull
       ) THEN
        ALTER TABLE notifications ALTER COLUMN template_id DROP NOT NULL;
    END IF;
END $$;

-- Widen the notification type CHECK to include 'follow' (and 'comment' from
-- the template-comments design). Guarded drop/re-add, same pattern as prior
-- CHECK widenings; safe in any landing order.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'notifications'::regclass
          AND contype = 'c'
          AND pg_get_constraintdef(oid) LIKE '%like%'
    ) THEN
        ALTER TABLE notifications DROP CONSTRAINT chk_notifications_type;
        ALTER TABLE notifications ADD CONSTRAINT chk_notifications_type
            CHECK (type IN ('like','favorite','fork','comment','follow'));
    END IF;
END $$;
```

> **Note for the developer**: the constraint name in the block above must
> match whatever the notifications feature actually created (inspect
> `pg_constraint` before writing the migration; adjust the name in both
> guarded blocks if it differs). The intent is: after both features land,
> `type` accepts `like|favorite|fork|comment|follow` and `template_id` is
> nullable.

## 3. Backend Layers

### 3.1 `models.User` (`backend/internal/models/user.go`)

Add fields:

```go
Bio            string    // nullable column; empty string when NULL
FollowerCount  int32
FollowingCount int32
```

### 3.2 `models.Follow` (new, `backend/internal/models/follow.go`)

```go
type Follow struct {
    FollowerID string
    FolloweeID string
    CreatedAt  time.Time
}
```

### 3.3 `FollowRepository` (new, `backend/internal/repository/follow_repository.go`)

```go
type FollowRepository interface {
    Follow(ctx context.Context, followerID, followeeID string) (followerCount int32, err error)
    Unfollow(ctx context.Context, followerID, followeeID string) (followerCount int32, err error)
    IsFollowing(ctx context.Context, followerID, followeeID string) (bool, error)
    GetFollowCounts(ctx context.Context, userID string) (followers, following int32, err error)
    ListFollowers(ctx context.Context, userID string, page, pageSize int) ([]*models.UserCard, int, error)
    ListFollowing(ctx context.Context, userID string, page, pageSize int) ([]*models.UserCard, int, error)
    GetFollowingFeed(ctx context.Context, userID string, pageSize int, beforeCreatedAt time.Time) ([]*models.Template, int, error)
    TopCreatorsByFollowers(ctx context.Context, limit int) ([]*models.UserCard, error)
}
```

Key SQL (single transaction per action; counters move only when an edge
actually changes):

```sql
-- Follow (tx):
-- 1) insert edge (idempotent)
INSERT INTO user_follows (follower_id, followee_id)
VALUES ($1, $2)
ON CONFLICT (follower_id, followee_id) DO NOTHING;
-- 2) bump counters only if a row was inserted (check RowsAffected)
UPDATE users SET follower_count = follower_count + 1 WHERE id = $2;
UPDATE users SET following_count = following_count + 1 WHERE id = $1;
-- 3) read back the followee's follower_count for the response
SELECT follower_count FROM users WHERE id = $2;
```

`Unfollow` mirrors this with `DELETE ... ` + conditional decrements (floor at
0 via `GREATEST(x - 1, 0)` to be robust against drift).

Feed query (banned creators and private templates excluded in SQL):

```sql
SELECT t.*, u.display_name AS owner_display_name, COALESCE(u.avatar,'') AS owner_avatar
FROM user_follows f
JOIN templates t ON t.owner_id = f.followee_id
JOIN users u ON u.id = t.owner_id
WHERE f.follower_id = $1
  AND t.visibility = 'public'
  AND t.created_at < $2            -- keyset cursor
  AND NOT EXISTS (SELECT 1 FROM users bu
                  WHERE bu.id = t.owner_id AND bu.status = 'banned')
ORDER BY t.created_at DESC
LIMIT $3;
-- total via the same WHERE without LIMIT
```

### 3.4 `UserRepository` changes

- `GetByID` / `GetByEmail` SELECT lists add `bio, follower_count, following_count`.
- `Update` adds `bio = $n` to the SET clause. **Contract note**: unlike
  `display_name`/`avatar` (applied only when non-empty), `bio` is written
  as-sent so an empty string clears it. The service validates length after
  trim (0–280) before calling `Update`.

### 3.5 `UserService` (`backend/internal/service/user.go`)

- Struct gains `FollowRepo FollowRepository` and
  `NotificationRepo repository.NotificationRepository` (nil-safe; the follow
  hook is skipped when nil, matching `createEngagementNotification` semantics).
- `FollowUser` / `UnfollowUser`:
  1. Reject self-follow (`400`).
  2. Rate-limit check (see below) — only **successful** actions consume quota.
  3. Load both users; `404` if the followee is missing; `403` if either party
     is banned.
  4. Call `FollowRepo.Follow/Unfollow` (idempotent).
  5. On a newly inserted edge, emit a `'follow'` notification
     (recipient = followee, actor = follower, `template_id = NULL`).
     Unfollow emits nothing.
- `GetCreatorProfile`: loads the user, builds the allow-listed response
  (never email/mobile/role), computes `is_following`/`follows_you` when the
  viewer is authenticated.
- `ListFollowers`/`ListFollowing`: offset pagination (default 20, max 50) +
  total count.
- `GetFollowingFeed`: keyset pagination by `created_at` cursor; page size
  default 20, max 50.
- `UpdateProfile`: accepts `bio` (trim → validate 0–280 → apply as-sent).

**Rate limiting** (Redis fixed window, fail-open):

```
key:    follow_rate_min:{user_id}
value:  "{count}:{window_expires_unix}"
window: 60s, limit FOLLOW_RATE_PER_MINUTE (default 30)
```

On limit: return `ResourceExhausted` with `Retry-After: <seconds to window
reset>`. Redis errors are logged and the action proceeds (fail-open), matching
the auth-throttle pattern.

### 3.6 `PromptService` (`backend/internal/service/prompt.go`)

- `GetTemplate`: after loading the template, load the owner via `UserRepo` and
  `IsFollowing` via `FollowRepo` (both nil-safe / skipped when repos absent in
  tests) and populate `GetTemplateResponse.owner`.
- `templateModelToProto`: map `owner_display_name`/`owner_avatar` (fields 18/19).

### 3.7 `TemplateRepository.List` — owner enrichment

Add to the existing SELECT (which already LEFT JOINs likes/favorites):

```sql
JOIN users ou ON ou.id = t.owner_id
-- add to selected columns:
ou.display_name AS owner_display_name, COALESCE(ou.avatar,'') AS owner_avatar
```

`models.Template` gains `OwnerDisplayName string` and `OwnerAvatar string`.
The JOIN is safe because `templates.owner_id` has an FK to `users`.

### 3.8 `AdminService` (`backend/internal/service/admin.go`)

`GetUserStats` (via `adminRepository`) adds two subqueries:

```sql
(SELECT follower_count FROM users WHERE id = $1) AS follower_count,
(SELECT following_count FROM users WHERE id = $1) AS following_count
```

Mapped into `UserStats` fields 5/6. No new admin RPCs.

### 3.9 Gateway (`backend/cmd/server/main.go`)

New route group (all under the existing CORS/auth middleware):

```
POST   /api/v1/users/{userId}/follow        → VerifyAnyToken
DELETE /api/v1/users/{userId}/follow        → VerifyAnyToken
GET    /api/v1/users/{userId}/profile       → public
GET    /api/v1/users/{userId}/templates     → public (maps to ListTemplates: owner_id + visibility=public)
GET    /api/v1/users/{userId}/followers     → public
GET    /api/v1/users/{userId}/following     → public
GET    /api/v1/users/me/following-feed      → VerifyAnyToken
```

Route registration order matters: `/api/v1/users/me/following-feed` must be
registered **before** `/api/v1/users/{userId}/...` patterns so `me` is not
captured as a userId. `follower_id`/`viewer_id` are injected from the auth
context (`ContextWithUserID`), never taken from the request body.

## 4. Frontend

### 4.1 New Files

| File | Purpose |
|---|---|
| `frontend/src/pages/CreatorProfilePage.js` + `.scss` | Public creator profile: header (96px avatar, name, bio, Follows-you badge, counts, joined date), tabs Templates/Followers/Following. Own profile → "Edit profile" link to `/profile`. |
| `frontend/src/components/FollowButton.js` + `.scss` | Optimistic toggle; `aria-pressed`; Following → Unfollow (danger) on hover/focus; revert on error; no confirm modal. |
| `frontend/src/components/UserCard.js` + `.scss` | Avatar + display name + follower count + FollowButton; links to `/creators/:id`. |
| `frontend/src/components/OwnerChip.js` + `.scss` | Avatar + name + follower count on TemplateDetails; links to creator page; FollowButton is a separate stop target. |
| `frontend/src/utils/formatCount.js` | Locale-aware count abbreviation (`1.2k` / `1.2万`). |

### 4.2 Modified Files

| File | Change |
|---|---|
| `frontend/src/App.js` | Add `/creators/:id` route (lazy + Suspense, like `/notifications`). |
| `frontend/src/pages/Home.js` | "Following" tab: feed grid + Load more (reuses infinite-scroll pattern); login-prompt card when anonymous; empty state with top-5 suggested creators (`TopCreatorsByFollowers` surfaced via `GET /api/v1/users/me/following-feed` empty response or a dedicated `GET /api/v1/users?sort=followers` — see §4.4). |
| `frontend/src/pages/TemplateDetails.js` | Replace `t('template_details.by', { owner: template.owner_id })` (line ~693) with `OwnerChip` + `FollowButton`. |
| `frontend/src/components/PromptCard.js` | "by {display_name}" line linking to `/creators/:id` (hidden when `owner_display_name` empty). |
| `frontend/src/pages/Profile.js` | Bio textarea with character counter (0–280). |
| `frontend/src/pages/admin/UserDetailPage.js` | Read-only followers/following entries in the stats row (~line 108). |
| `frontend/src/services/api.js` | New client functions (§4.4). |

### 4.3 Redux

No new slice required: follow state lives in component-local state (optimistic
toggle) and the enriched template/owner data arrives via existing fetch
patterns. If the team prefers global state for `is_following` on the current
template, extend the existing template slice — but v1 keeps it local.

### 4.4 API Client (`frontend/src/services/api.js`)

```js
followUser(userId)            // POST   /users/${userId}/follow
unfollowUser(userId)          // DELETE /users/${userId}/follow
getCreatorProfile(userId)     // GET    /users/${userId}/profile
getCreatorTemplates(userId, { page, page_size }) // GET /users/${userId}/templates
getFollowers(userId, { page, page_size })        // GET /users/${userId}/followers
getFollowing(userId, { page, page_size })        // GET /users/${userId}/following
getFollowingFeed({ page_size, page_token })      // GET /users/me/following-feed
getTopCreators({ limit })     // GET /users?sort=followers&limit=  (empty-feed suggestions)
```

`getTopCreators` maps to a small public listing endpoint: `GET /api/v1/users?sort=followers&limit=5`
implemented over `TopCreatorsByFollowers` (public user-card shape only).

### 4.5 i18n Keys (EN/ZH parity)

```json
{
  "creators": {
    "title": "Creator",
    "followers": "{{count}} followers",
    "following_count": "{{count}} following",
    "joined": "Joined {{date}}",
    "follows_you": "Follows you",
    "tab_templates": "Templates",
    "tab_followers": "Followers",
    "tab_following": "Following",
    "edit_profile": "Edit profile",
    "empty_templates": "No public templates yet",
    "empty_followers": "No followers yet",
    "empty_following": "Not following anyone yet"
  },
  "follow": {
    "follow": "Follow",
    "following": "Following",
    "unfollow": "Unfollow",
    "error": "Could not update follow status"
  },
  "home": {
    "following": {
      "tab": "Following",
      "empty": "Follow creators to see their latest templates here",
      "suggested": "Suggested creators",
      "login_prompt": "Log in to follow creators"
    }
  },
  "template_details": {
    "by": "by {{owner}}"
  },
  "admin": {
    "users": {
      "followers": "Followers",
      "following": "Following"
    }
  },
  "notifications": {
    "type_follow": "{{actor}} followed you"
  }
}
```

ZH translations mirror every key (e.g. `follow.follow` = "关注",
`creators.follows_you` = "关注了你", `notifications.type_follow` =
"{{actor}} 关注了你").

## 5. Error Matrix

| Scenario | HTTP | gRPC | Frontend handling |
|---|---|---|---|
| Self-follow | 400 | `INVALID_ARGUMENT` | Inline error, no state change |
| Bio > 280 chars | 400 | `INVALID_ARGUMENT` | Counter turns red; save blocked |
| Not authenticated | 401 | `UNAUTHENTICATED` | Redirect to login |
| Either party banned | 403 | `PERMISSION_DENIED` | Error toast; button reverts |
| User not found | 404 | `NOT_FOUND` | 404 page / empty state |
| Rate limited | 429 | `RESOURCE_EXHAUSTED` | Toast with retry countdown; button reverts |
| Redis down | — | — | Fail-open server-side; no user impact |

## 6. Deployment

- `FOLLOW_RATE_PER_MINUTE=30` added to `deployment/docker-compose.yml` backend
  environment and the Helm chart (`values.yaml`
  `backend.env.followRatePerMinute: "30"` + `templates/deployment-backend.yaml`
  env entry), mirroring `NOTIFICATION_*`.
- DDL blocks are additive/idempotent; both init.sql copies updated in the same
  commit. The notifications-coordination blocks are safe in any landing order.
- No new services, ports, or infrastructure.

## 7. SDK

No python-sdk changes required: the SDK wraps template resolution, not user
social features. The new endpoints are documented in the REST API surface only.

## 8. Test Strategy

### 8.1 FVT (`tests/fvt.py`) — `test_follow_creators`

Three users (alice, bob, carol) via the existing `get_auth_token` helper:

1. alice follows bob → `{following: true, follower_count: 1}`; bob's profile
   shows `follower_count: 1`, `is_following: true` (from alice's view),
   `follows_you: true` (from bob's view of alice).
2. Repeat follow (idempotent) → counts unchanged.
3. carol follows bob → `follower_count: 2`.
4. alice unfollows bob → `{following: false, follower_count: 1}`; repeat
   unfollow idempotent.
5. Self-follow → 400. Follow non-existent user → 404.
6. Ban carol (admin API) → carol cannot follow (403); alice cannot follow
   carol (403); existing alice→carol edge retained; carol's profile still 200.
7. Profile privacy: assert `email`, `mobile`, `role` absent from
   `/users/{id}/profile` response for both anon and authed callers.
8. Feed: bob (followed by alice) publishes public + private templates; alice's
   feed contains only public ones, newest-first; banned creator's templates
   excluded.
9. Followers/following lists: pagination + user-card shape (no email).
10. Bio: set via `PUT /api/v1/profile`, verify in profile response; clear with
    empty string; 281-char bio → 400.
11. Template detail `owner` object present with `is_following`; list responses
    carry `owner_display_name`.
12. Admin user detail `stats` includes follower/following counts.

### 8.2 Unit Tests

- `FollowRepository`: counter math on duplicate insert / missing delete;
  feed query exclusions (private, banned).
- `UserService`: self-follow, banned flows, rate-limit window (mock Redis),
  bio trim/validation, notification emission on new edge only.

### 8.3 E2E (test-agent, Nightwatch)

- Follow button toggle + optimistic revert on forced error.
- Creator profile tabs render; own profile shows Edit link.
- Following feed: login prompt (anon), empty state with suggestions, Load more.
- i18n: EN/ZH parity for all new keys; count abbreviation spot check.

## 9. Implementation Order

1. DDL (both init.sql copies) + models + `FollowRepository` + unit tests.
2. Proto changes + regenerate + `UserRepository`/`TemplateRepository` updates.
3. `UserService` RPCs (follow/unfollow/profile/lists/feed/bio) + rate limit +
   notification hook.
4. `PromptService` owner enrichment + `AdminService` stats.
5. Gateway routes.
6. Frontend: api client → components → pages → i18n.
7. FVT + E2E.

## 10. Constraints for Developer

1. **Never commit `backend/server`** — it is a tracked 26MB build artifact.
   If it shows modified, `git checkout -- backend/server`.
2. Stage only your own files; never `git add -A` (shared working tree).
3. Both init.sql copies must stay in sync in the same commit.
4. The notifications DDL coordination blocks are order-independent by design;
   verify the actual constraint name in `pg_constraint` before relying on the
   guarded drop.
5. `bio` uses apply-as-sent semantics (empty clears) — do not copy the
   non-empty-only pattern used for `display_name`/`avatar`.
6. Public endpoints must never return email/mobile/role; keep the profile
   response allow-listed in the service layer.
7. Register `/api/v1/users/me/following-feed` before `/api/v1/users/{userId}`
   route patterns.
8. Rate limiting consumes quota only on successful actions; fail-open on Redis
   errors.
9. No monetization surfaces of any kind.
