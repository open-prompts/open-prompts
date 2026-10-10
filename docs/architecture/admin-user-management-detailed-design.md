# Admin User Management System — Detailed Design

- **Feature ID:** `admin-user-management`
- **Version:** 1.0
- **Status:** Approved for implementation
- **Author:** Architect Agent
- **Upstream:** [Architecture](./admin-user-management-architecture.md) · [Requirements](../design/admin-user-management-requirements.md) · [UI/UX Design](../design/admin-user-management-ui-ux-design.md)

---

## 1. Proto Contract (`backend/api/proto/v1/admin.proto`)

New file alongside `prompt.proto`. Same package `v1`, same `go_package`. **Do not modify `prompt.proto`.**

```protobuf
syntax = "proto3";

package v1;

import "google/protobuf/timestamp.proto";

option go_package = "open-prompts/backend/api/proto/v1;v1";

// AdminService defines administrative RPCs. Every RPC except AdminLogin
// requires an admin-scoped JWT (role=admin, scope=admin).
service AdminService {
  // AdminLogin authenticates an admin and returns a short-lived admin token.
  rpc AdminLogin(AdminLoginRequest) returns (AdminLoginResponse);

  // RefreshAdminSession renews the admin token (sliding expiration).
  rpc RefreshAdminSession(RefreshAdminSessionRequest) returns (RefreshAdminSessionResponse);

  // GetAdminStats returns dashboard KPIs and recent activity.
  rpc GetAdminStats(GetAdminStatsRequest) returns (GetAdminStatsResponse);

  // ListUsers returns a paginated, searchable, filterable user list.
  rpc ListUsers(ListUsersRequest) returns (ListUsersResponse);

  // GetUser returns full admin detail for one user.
  rpc GetUser(GetUserRequest) returns (GetUserResponse);

  // BanUser bans a user (reason required).
  rpc BanUser(BanUserRequest) returns (UserActionResponse);

  // UnbanUser lifts a ban (reason required).
  rpc UnbanUser(UnbanUserRequest) returns (UserActionResponse);

  // ChangeUserRole promotes/demotes (reason required; self-demotion rejected).
  rpc ChangeUserRole(ChangeUserRoleRequest) returns (UserActionResponse);

  // ListTemplates returns a paginated moderation list of ALL templates.
  rpc ListTemplates(ListTemplatesRequest) returns (ListTemplatesResponse);

  // GetTemplate returns any template with all versions (read-only).
  rpc GetTemplate(GetTemplateRequest) returns (GetTemplateResponse);

  // ListAuditLogs returns the append-only audit trail.
  rpc ListAuditLogs(ListAuditLogsRequest) returns (ListAuditLogsResponse);
}

// ---- Auth ----

message AdminLoginRequest {
  string email = 1;
  string password = 2;
}

message AdminLoginResponse {
  string token = 1;        // admin-scoped JWT, exp = now + ADMIN_SESSION_TTL_MINUTES
  AdminUserSummary user = 2;
}

message RefreshAdminSessionRequest {}

message RefreshAdminSessionResponse {
  string token = 1;
}

// ---- Stats ----

message GetAdminStatsRequest {}

message GetAdminStatsResponse {
  int64 total_users = 1;
  int64 active_users = 2;
  int64 banned_users = 3;
  int64 total_templates = 4;
  int64 public_templates = 5;
  int64 prompts_created = 6;
  repeated AdminUserSummary recent_signups = 7;      // latest 10
  repeated AdminTemplateSummary recent_templates = 8; // latest 10 public, by updated_at
}

// ---- Users ----

message AdminUserSummary {
  string id = 1;
  string email = 2;
  string display_name = 3;
  string avatar = 4;
  string role = 5;      // "user" | "admin"
  string status = 6;    // "active" | "banned"
  int32 template_count = 7;
  google.protobuf.Timestamp created_at = 8;
  google.protobuf.Timestamp updated_at = 9;
}

message ListUsersRequest {
  string search = 1;    // matches id, email, display_name (case-insensitive)
  string status = 2;    // "" | "active" | "banned"
  string role = 3;      // "" | "user" | "admin"
  string sort = 4;      // "newest" (default) | "oldest" | "recently_updated"
  int32 page = 5;       // 1-based
  int32 per_page = 6;   // default 20, max 100
}

message ListUsersResponse {
  repeated AdminUserSummary users = 1;
  int64 total = 2;
  int32 page = 3;
  int32 per_page = 4;
}

message GetUserRequest {
  string id = 1;
}

message GetUserResponse {
  AdminUserSummary user = 1;
  UserStats stats = 2;
  repeated OAuthIdentity identities = 3;
  repeated AdminAPIKeySummary api_keys = 4;
  repeated AdminTemplateSummary templates = 5;      // latest 5
  repeated AdminAuditLogEntry action_history = 6;   // entries targeting this user, latest 20
}

message UserStats {
  int64 templates = 1;
  int64 likes_received = 2;
  int64 favorites_received = 3;
  int64 prompts_created = 4;
}

message OAuthIdentity {
  string provider = 1;
  google.protobuf.Timestamp created_at = 2;
}

message AdminAPIKeySummary {
  string id = 1;
  string name = 2;
  string prefix = 3;
  bool is_active = 4;
  google.protobuf.Timestamp created_at = 5;
  google.protobuf.Timestamp last_used_at = 6; // null if never used
}

message BanUserRequest {
  string user_id = 1;
  string reason = 2;    // 3-500 chars, required
}

message UnbanUserRequest {
  string user_id = 1;
  string reason = 2;
}

message ChangeUserRoleRequest {
  string user_id = 1;
  string role = 2;      // "admin" | "user"
  string reason = 3;
}

message UserActionResponse {
  AdminUserSummary user = 1;  // post-action state
}

// ---- Templates ----

message AdminTemplateSummary {
  string id = 1;
  string title = 2;
  string visibility = 3;   // "public" | "private"
  string type = 4;         // "system" | "user"
  string language = 5;
  repeated string tags = 6;
  int32 like_count = 7;
  int32 favorite_count = 8;
  google.protobuf.Timestamp created_at = 9;
  google.protobuf.Timestamp updated_at = 10;
  string owner_id = 11;
  string owner_display_name = 12;
}

message ListTemplatesRequest {
  string search = 1;      // matches title or owner id/display_name
  string visibility = 2;  // "" | "public" | "private"
  string owner_id = 3;    // deep-link filter from user detail
  string sort = 4;        // "recently_updated" (default) | "newest" | "oldest" | "most_likes"
  int32 page = 5;
  int32 per_page = 6;
}

message ListTemplatesResponse {
  repeated AdminTemplateSummary templates = 1;
  int64 total = 2;
  int32 page = 3;
  int32 per_page = 4;
}

message GetTemplateRequest {
  string id = 1;
}

message GetTemplateResponse {
  AdminTemplateSummary template = 1;
  string description = 2;
  repeated AdminTemplateVersion versions = 3;  // all versions, newest first
}

message AdminTemplateVersion {
  int32 id = 1;
  int32 version = 2;
  string content = 3;
  google.protobuf.Timestamp created_at = 4;
  string alias_name = 5;   // "" if no alias points at this version
}

// ---- Audit ----

message AdminAuditLogEntry {
  string id = 1;
  string actor_id = 2;
  string actor_email = 3;
  string action = 4;        // "ban" | "unban" | "role_change"
  string target_type = 5;   // "user"
  string target_id = 6;
  string reason = 7;
  google.protobuf.Timestamp created_at = 8;
}

message ListAuditLogsRequest {
  string action = 1;    // "" | "ban" | "unban" | "role_change"
  string actor = 2;     // matches actor_id or actor_email
  int32 page = 3;
  int32 per_page = 4;
}

message ListAuditLogsResponse {
  repeated AdminAuditLogEntry entries = 1;
  int64 total = 2;
  int32 page = 3;
  int32 per_page = 4;
}
```

**Code generation.** `protoc` is not installed in this environment; install with `sudo apt install protobuf-compiler` (passwordless sudo available), plugins `protoc-gen-go`/`protoc-gen-go-grpc` are already in `~/go/bin`. Generate with the same flags used for `prompt.pb.go` (run from `backend/`):

```bash
protoc -I api/proto/v1 \
  --go_out=. --go_opt=module=open-prompts/backend \
  --go-grpc_out=. --go-grpc_opt=module=open-prompts/backend \
  api/proto/v1/admin.proto
```

Commit the generated `admin.pb.go` / `admin_grpc.pb.go` alongside the proto (repo convention).

## 2. Database DDL (append to `deployment/init.sql`, sync chart copy)

Follows the file's idempotent pattern exactly. Append at the end:

```sql
-- -----------------------------------------------------------------------------
-- Admin: users.role / users.status
-- Description: Role-based access control and account status for administration.
-- -----------------------------------------------------------------------------
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='role') THEN
        ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'user';
        COMMENT ON COLUMN users.role IS 'Account role: user or admin';
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='status') THEN
        ALTER TABLE users ADD COLUMN status TEXT NOT NULL DEFAULT 'active';
        COMMENT ON COLUMN users.status IS 'Account status: active or banned';
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);
CREATE INDEX IF NOT EXISTS idx_users_created_at ON users(created_at);

-- -----------------------------------------------------------------------------
-- Table: admin_audit_logs
-- Description: Append-only audit trail of administrative mutations.
--              Deliberately no FK: history must survive user deletion.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS admin_audit_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    actor_id TEXT NOT NULL,
    actor_email TEXT NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('ban', 'unban', 'role_change')),
    target_type TEXT NOT NULL CHECK (target_type IN ('user')),
    target_id TEXT NOT NULL,
    reason TEXT NOT NULL CHECK (char_length(reason) BETWEEN 3 AND 500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE admin_audit_logs IS 'Append-only audit trail of admin mutations';
COMMENT ON COLUMN admin_audit_logs.actor_id IS 'Acting admin user ID (from verified JWT)';
COMMENT ON COLUMN admin_audit_logs.actor_email IS 'Admin email snapshot at action time';
COMMENT ON COLUMN admin_audit_logs.action IS 'ban | unban | role_change';
COMMENT ON COLUMN admin_audit_logs.target_type IS 'Target entity type (user)';
COMMENT ON COLUMN admin_audit_logs.target_id IS 'Affected user ID';
COMMENT ON COLUMN admin_audit_logs.reason IS 'Mandatory justification, 3-500 chars';

CREATE INDEX IF NOT EXISTS idx_admin_audit_logs_created_at ON admin_audit_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_admin_audit_logs_actor_id ON admin_audit_logs(actor_id);
CREATE INDEX IF NOT EXISTS idx_admin_audit_logs_target_id ON admin_audit_logs(target_id);
```

Notes:
- `ADD COLUMN ... NOT NULL DEFAULT` backfills existing rows automatically in postgres ≥ 11 — no separate UPDATE needed.
- No CHECK constraint on `users.role`/`users.status` (the `DO $$` guard only wraps the ADD COLUMN; adding constraints idempotently would need another guard — validation is enforced in the service layer, consistent with how `visibility`/`type` on templates are handled today).
- The identical block must be appended to `deployment/chart/open-prompts/init.sql`.

## 3. Backend Layer Responsibilities

### 3.1 Models (`backend/internal/models/`)

`user.go` — add fields (keep existing tags):

```go
type User struct {
    ID           string         `json:"id"`
    Email        string         `json:"email"`
    Mobile       sql.NullString `json:"mobile"`
    PasswordHash string         `json:"-"`
    DisplayName  string         `json:"display_name"`
    Avatar       string         `json:"avatar"`
    Role         string         `json:"role"`   // "user" | "admin"
    Status       string         `json:"status"` // "active" | "banned"
    CreatedAt    time.Time      `json:"created_at"`
    UpdatedAt    time.Time      `json:"updated_at"`
}
```

`admin.go` — new:

```go
type AdminAuditLog struct {
    ID         string    `json:"id"`
    ActorID    string    `json:"actor_id"`
    ActorEmail string    `json:"actor_email"`
    Action     string    `json:"action"` // ban | unban | role_change
    TargetType string    `json:"target_type"`
    TargetID   string    `json:"target_id"`
    Reason     string    `json:"reason"`
    CreatedAt  time.Time `json:"created_at"`
}
```

### 3.2 Repository layer (`backend/internal/repository/`)

**`user_repository.go` (modify):**
- All SELECTs add `role, status` to the column list and scan targets (both `GetByID`, `GetByEmail`, plus any other read sites).
- `Insert` writes `role, status` (default `'user'`/`'active'` when empty — keeps existing callers working).
- New method on the interface:

```go
UpdateRoleAndStatus(ctx context.Context, userID, role, status string) error
```

Single `UPDATE users SET role=$2, status=$3, updated_at=NOW() WHERE id=$1`; `ErrUserNotFound` when 0 rows.

**`admin_repository.go` (new).** Interface + `*sql.DB` implementation, following the existing repository style (context, sentinel errors, `zap` logging):

```go
type AdminRepository interface {
    // Users
    ListUsers(ctx context.Context, f UserFilters) ([]*models.User, int64, error) // + total
    GetUserDetail(ctx context.Context, userID string) (*models.User, error)
    CountUserTemplates(ctx context.Context, userID string) (int64, error)
    GetUserStats(ctx context.Context, userID string) (*models.UserStats, error)
    ListUserIdentities(ctx context.Context, userID string) ([]*models.OAuthIdentity, error)
    // Templates (admin view: includes private, includes banned owners)
    ListTemplates(ctx context.Context, f TemplateFilters) ([]*models.AdminTemplate, int64, error)
    GetTemplateWithVersions(ctx context.Context, templateID string) (*models.AdminTemplate, []*models.AdminTemplateVersion, error)
    // Stats
    GetStats(ctx context.Context) (*models.AdminStats, error)
    ListRecentSignups(ctx context.Context, limit int) ([]*models.User, error)
    ListRecentPublicTemplates(ctx context.Context, limit int) ([]*models.AdminTemplate, error)
    // Audit
    InsertAuditLog(ctx context.Context, entry *models.AdminAuditLog) error
    ListAuditLogs(ctx context.Context, f AuditFilters) ([]*models.AdminAuditLog, int64, error)
    ListAuditByTarget(ctx context.Context, targetID string, limit int) ([]*models.AdminAuditLog, error)
}

type UserFilters struct {
    Search  string // ILIKE %s% over id, email, display_name
    Status  string // "" | active | banned
    Role    string // "" | user | admin
    Sort    string // newest | oldest | recently_updated
    Limit   int
    Offset  int
}
```

Key queries:
- `ListUsers`: `WHERE (id ILIKE $1 OR email ILIKE $1 OR display_name ILIKE $1) AND ($2='' OR status=$2) AND ($3='' OR role=$3) ORDER BY <sort> LIMIT/OFFSET` + separate `COUNT(*)` with identical WHERE. Sort map: `newest`→`created_at DESC`, `oldest`→`created_at ASC`, `recently_updated`→`updated_at DESC`.
- `ListTemplates` (admin): JOIN `users` for owner display name; `WHERE (title ILIKE $1 OR owner_id ILIKE $1 OR u.display_name ILIKE $1) AND ($2='' OR visibility=$2) AND ($3='' OR owner_id=$3)`; sorts: `recently_updated`→`updated_at DESC`, `newest`→`created_at DESC`, `oldest`→`created_at ASC`, `most_likes`→`like_count DESC, updated_at DESC`.
- `GetStats`: single query with scalar subselects (`COUNT(*) FILTER (WHERE ...)`) for users/active/banned/total templates/public templates/prompts; `ListRecentSignups`/`ListRecentPublicTemplates` are `ORDER BY ... DESC LIMIT 10` queries (recent templates filtered `visibility='public'` AND owner not banned, ordered by `updated_at DESC`).
- `GetUserStats`: templates count, `SUM(like_count)`, `SUM(favorite_count)` over the user's templates, prompts count.
- `GetTemplateWithVersions`: template row + all `template_versions` ordered `version DESC`, plus alias resolution (`LEFT JOIN template_aliases` on `version_id`, pick any alias per version — display `""` if none).
- `InsertAuditLog`: plain INSERT; DB CHECK constraints are the last line of defense for reason length.

**`template_repository.go` (modify):** public listing paths (`List` with public visibility, `ListCategories`, `ListTags`) add `AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = t.owner_id AND u.status='banned')` (or JOIN variant). `Get` returns `ErrTemplateNotFound` for public templates of banned owners when the caller is not the owner.

### 3.3 Service layer (`backend/internal/service/admin.go`, new)

```go
type AdminService struct {
    pb.UnimplementedAdminServiceServer
    UserRepo    repository.UserRepository
    AdminRepo   repository.AdminRepository
    APIKeyRepo  *repository.APIKeyRepository
    Redis       RedisStore        // reuse the interface from user.go
    JWTSecret   []byte
    SessionTTL  time.Duration     // from ADMIN_SESSION_TTL_MINUTES
}
```

Method-by-method contract:

| Method | Validation | Behavior | Errors |
|---|---|---|---|
| `AdminLogin` | email+password present | rate-limit check (Redis `admin_login_attempts:<ip>` — the gateway passes the client IP via context or the request metadata `x-client-ip` set by the handler); fetch user by email; bcrypt compare; **reject `status='banned'` → 403 "account banned"**; **reject `role!='admin'` → 403 "this account does not have admin access"**; issue admin JWT (`sub`, `role:"admin"`, `scope:"admin"`, `iat`, `exp=now+TTL`, `iss`) | 400 missing fields; 401 invalid credentials; 403 banned / not admin; 429 rate limited |
| `RefreshAdminSession` | caller is admin (context) | verify current token still valid (middleware already did); issue fresh admin JWT | 401 via middleware |
| `GetAdminStats` | — | delegate to `AdminRepo.GetStats` + recent lists | 500 on DB error |
| `ListUsers` | page≥1, per_page 1–100 (default 20), sort whitelist, status/role whitelist | delegate to repo | 400 invalid params |
| `GetUser` | id present | user + stats + identities + API keys (via `APIKeyRepo.ListByUserID`) + latest 5 templates + audit history (latest 20 targeting user) | 404 unknown user |
| `BanUser` | reason 3–500; target exists; **target != actor**; target not already banned | 1) `UpdateRoleAndStatus(id, role, 'banned')` 2) Redis SET `user_banned:<id>` + `token_revoked_before:<id>=now` 3) `InsertAuditLog{action:"ban"}` | 400 bad reason; 403 self-ban; 404 unknown; 409 already banned |
| `UnbanUser` | reason 3–500; target exists; target is banned | 1) `UpdateRoleAndStatus(id, role, 'active')` 2) Redis DEL `user_banned:<id>` 3) audit `unban` | 400; 404; 409 not banned |
| `ChangeUserRole` | reason 3–500; role ∈ {user, admin}; target exists; **target != actor** (blocks both self-promote-loop and self-demotion) | 1) `UpdateRoleAndStatus(id, newRole, status)` 2) Redis SET `token_revoked_before:<id>=now` 3) audit `role_change` | 400; 403 self; 404; 409 same role |
| `ListTemplates` | page/per_page/sort/visibility whitelist | delegate | 400 |
| `GetTemplate` | id present | template + versions | 404 |
| `ListAuditLogs` | page/per_page, action whitelist | delegate | 400 |

**Audit write ordering:** the DB mutation and audit insert are separate statements (no transaction in the current codebase style; the repository layer uses `*sql.DB` directly). Wrap mutation+audit in a `db.BeginTx` **only if** trivially achievable with the existing repo signatures; otherwise mutation first, audit second, and log an error (never fail the user request) if the audit insert fails. Audit failure must not un-ban a user.

**Redis failure fallback:** if `Redis.Set/Del` errors during ban/unban/role-change, log and continue (DB status is the source of truth for login; the revocation keys are an optimization for live sessions). If `Redis.Get` errors during token verification, fall back to a DB `users.status`/`role` check.

### 3.4 Auth changes (`backend/internal/service/auth.go`)

1. `generateToken`-equivalent for admin tokens (in `admin.go`): claims `sub`, `role`, `scope:"admin"`, `iat`, `exp`, `iss:"open-prompts"`.
2. `VerifyToken` extended → new method `VerifyTokenWithClaims(token) (userID, role, scope string, iat int64, err error)`; existing `VerifyToken` delegates to it (all existing callers keep working).
3. New context helpers: `ContextWithUserRole(ctx, role)`, `GetUserRoleFromContext(ctx)`, `ContextWithUserScope`, `GetUserScopeFromContext` (mirror the existing `userIDKey` pattern with distinct context keys).
4. `VerifyAnyToken` (used by REST handlers): after signature verification, check ban/revocation:
   - `user_banned:<sub>` exists → `403 "account banned"`.
   - `token_revoked_before:<sub>` > `iat` → `401 "token revoked"`.
   - On Redis error → DB fallback (`UserRepo.GetByID` → check status/role).
   - **API key path:** after resolving the key's user, load the user and reject `status='banned'` with `403 "account banned"` (FR-22). This adds one DB read per API-key request; acceptable (SDK traffic is low) and correct.
5. `publicRpcMethods` gains `"/v1.AdminService/AdminLogin"`.
6. gRPC unary interceptor: after user-ID injection, also inject role/scope claims into context. `AdminService` methods (except `AdminLogin`) call a shared `requireAdminContext(ctx)` helper that returns `403` unless `role=admin && scope=admin` — this protects the gRPC path even though the REST gateway is the primary entry.

### 3.5 UserService changes (`backend/internal/service/user.go`)

- `Login`: after bcrypt success, `if user.Status == "banned" → 403 "account banned"` (before token issuance).
- `LoginWithOAuth`: same check after identity resolution.
- `generateToken`: add `role` claim (from `user.Role`) — regular tokens carry role but **no** `scope` claim.
- `Register`: unchanged (new users default `role='user'`, `status='active'` via DB defaults; model zero-values are normalized in the repo Insert).

### 3.6 Gateway wiring (`backend/cmd/server/main.go`)

1. Construct `AdminRepository`, `AdminService` (pass `JWTSecret`, `SessionTTL` from `ADMIN_SESSION_TTL_MINUTES` env, default 30).
2. Register `pb.RegisterAdminServiceServer(s, adminSvc)` in the gRPC goroutine.
3. Add a `requireAdmin(w, r) (userID string, ok bool)` helper: extracts Bearer token → `VerifyAnyToken` → `VerifyTokenWithClaims` → requires `role=admin && scope=admin` → writes 401/403 via `writeError` on failure. All admin handlers call it first.
4. Add handlers (same style as existing: CORS headers, OPTIONS short-circuit, protojson marshal with `EmitUnpopulated`/`UseProtoNames`, `writeError`):

| Route | Handler sketch |
|---|---|
| `POST /api/v1/admin/login` | parse `AdminLoginRequest`; set `x-client-ip` metadata (from `r.RemoteAddr` host part) into context for rate limiting; call `AdminLogin`; 200 `{token, user}` |
| `POST /api/v1/admin/session/refresh` | `requireAdmin`; call `RefreshAdminSession`; 200 `{token}` |
| `GET /api/v1/admin/stats` | `requireAdmin`; call `GetAdminStats` |
| `GET /api/v1/admin/users` | `requireAdmin`; query → `ListUsersRequest` (`search`,`status`,`role`,`sort`,`page`,`per_page`); call |
| `GET /api/v1/admin/users/` | `requireAdmin`; path suffix dispatch: `{id}` → `GetUser`; `{id}/ban|unban|role` (POST) → parse body `{reason}` / `{role, reason}` → call |
| `GET /api/v1/admin/templates` | `requireAdmin`; query → `ListTemplatesRequest` (`search`,`visibility`,`owner_id`,`sort`,`page`,`per_page`) |
| `GET /api/v1/admin/templates/` | `requireAdmin`; `{id}` → `GetTemplate` |
| `GET /api/v1/admin/audit-log` | `requireAdmin`; query → `ListAuditLogsRequest` (`action`,`actor`,`page`,`per_page`) |

Route registration order caution: register `/api/v1/admin/users/` (trailing slash) **after** `/api/v1/admin/users` — Go's ServeMux treats them as distinct patterns; the trailing-slash handler must dispatch on the remainder (`ban`/`unban`/`role` suffixes), mirroring the existing `/api/v1/templates/` handler style.

5. **Startup bootstrap** (before serving): read `ADMIN_BOOTSTRAP_EMAIL`/`ADMIN_BOOTSTRAP_PASSWORD`; implement per architecture §6.4; log the outcome; never log the password.

### 3.7 New env vars (backend)

| Var | Default | Notes |
|---|---|---|
| `ADMIN_BOOTSTRAP_EMAIL` | empty (disabled) | Startup promotion/creation |
| `ADMIN_BOOTSTRAP_PASSWORD` | empty | Only used when creating a new bootstrap account |
| `ADMIN_SESSION_TTL_MINUTES` | `30` | Admin JWT lifetime |

## 4. Frontend Design

### 4.1 File layout

```
frontend/src/
├── App.js                                  (modified: /admin routes)
├── pages/admin/
│   ├── AdminLoginPage.js/.scss
│   ├── DashboardPage.js/.scss
│   ├── UsersPage.js/.scss
│   ├── UserDetailPage.js/.scss
│   ├── TemplatesPage.js/.scss
│   ├── TemplateDetailPage.js/.scss
│   └── AuditLogPage.js/.scss
├── components/admin/
│   ├── AdminLayout.js/.scss                (shell + route guard)
│   ├── AdminTable.js/.scss                 (generic table: skeleton/sticky/row-click)
│   ├── KpiCard.js/.scss
│   ├── Badge.js/.scss                      (tone: accent|success|danger|warning|neutral)
│   ├── Pagination.js/.scss
│   ├── ReasonModal.js/.scss                (reuses ui/Modal)
│   ├── AdminSearchInput.js/.scss           (debounce 300ms + clear)
│   ├── AdminSelect.js/.scss
│   ├── EmptyState.js/.scss
│   └── ErrorBanner.js/.scss
├── services/adminApi.js                    (new axios instance)
└── store/adminAuthSlice.js                 (new redux slice)
```

### 4.2 Routing (`App.js`)

```jsx
import { lazy, Suspense } from 'react';
const AdminLayout = lazy(() => import('./components/admin/AdminLayout'));
const AdminLoginPage = lazy(() => import('./pages/admin/AdminLoginPage'));
// ... other admin pages

<Route path="/admin/login" element={<Suspense fallback={<PageSpinner/>}><AdminLoginPage/></Suspense>} />
<Route path="/admin" element={<Suspense fallback={<PageSpinner/>}><AdminLayout/></Suspense>}>
  <Route index element={<DashboardPage/>} />
  <Route path="users" element={<UsersPage/>} />
  <Route path="users/:id" element={<UserDetailPage/>} />
  <Route path="templates" element={<TemplatesPage/>} />
  <Route path="templates/:id" element={<TemplateDetailPage/>} />
  <Route path="audit-log" element={<AuditLogPage/>} />
</Route>
```

`AdminLayout` renders `<Outlet/>`; guard logic (per UI/UX §4): no `op_admin_token` → `<Navigate to="/admin/login?redirect=..." replace/>`; token present but user role ≠ admin (stale local state) → render 403 page (the API remains the boundary).

### 4.3 `services/adminApi.js`

```js
import axios from 'axios';

const adminApi = axios.create({ baseURL: '/api/v1', timeout: 60000 });

adminApi.interceptors.request.use((config) => {
  const token = localStorage.getItem('op_admin_token');
  if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});

adminApi.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response && error.response.status === 401 &&
        window.location.pathname.startsWith('/admin')) {
      localStorage.removeItem('op_admin_token');
      localStorage.removeItem('op_admin_user');
      const redirect = encodeURIComponent(window.location.pathname + window.location.search);
      window.location.href = `/admin/login?redirect=${redirect}`;
    }
    return Promise.reject(error);
  }
);

// Admin auth
export const adminLogin = (credentials) => adminApi.post('/admin/login', credentials);
export const adminRefreshSession = () => adminApi.post('/admin/session/refresh');
// Stats
export const getAdminStats = () => adminApi.get('/admin/stats');
// Users
export const getAdminUsers = (params) => adminApi.get('/admin/users', { params });
export const getAdminUser = (id) => adminApi.get(`/admin/users/${id}`);
export const banAdminUser = (id, reason) => adminApi.post(`/admin/users/${id}/ban`, { reason });
export const unbanAdminUser = (id, reason) => adminApi.post(`/admin/users/${id}/unban`, { reason });
export const changeAdminUserRole = (id, role, reason) => adminApi.post(`/admin/users/${id}/role`, { role, reason });
// Templates
export const getAdminTemplates = (params) => adminApi.get('/admin/templates', { params });
export const getAdminTemplate = (id) => adminApi.get(`/admin/templates/${id}`);
// Audit
export const getAdminAuditLogs = (params) => adminApi.get('/admin/audit-log', { params });

export default adminApi;
```

**Sliding session refresh:** a small `ensureFreshAdminToken()` helper (called after successful admin responses): decode `iat`/`exp` from the JWT payload (base64url, no library needed); if `now - iat > 10 min`, call `adminRefreshSession()`, store the new token, and update the axios default. On failure → logout flow.

### 4.4 Redux (`store/adminAuthSlice.js`)

```js
const STORAGE_KEYS = { token: 'op_admin_token', user: 'op_admin_user' };
// reducers: adminLoginSuccess({user, token}), adminLogout(), initializeAdminAuth()
// identical shape to authSlice.js; separate localStorage keys
```

Register in `store.js` as `adminAuth: adminAuthReducer`.

### 4.5 Page behavior contracts

| Page | Data calls | Key interactions |
|---|---|---|
| `AdminLoginPage` | `adminLogin` | inline errors (403 non-admin → `admin.login.no_admin_access`; 401 → `admin.login.invalid`; 403 banned → `admin.login.banned`); on success store token+user, navigate to `?redirect` or `/admin` |
| `DashboardPage` | `getAdminStats` | 6 KPI cards (links per UI/UX §5.2), recent signups list (→ user detail), recent templates list (→ template detail); skeleton/empty/error states |
| `UsersPage` | `getAdminUsers` | URL-synced `q,status,role,sort,page,per_page` via `useSearchParams`; debounce 300ms; row click → detail; `⋯` menu → View / Ban…/Unban… / Make admin…/Demote…; opens `ReasonModal`; on success toast + refetch |
| `UserDetailPage` | `getAdminUser` | header card + 4 stat cards + identities + API keys + latest 5 templates ("View all" → `/admin/templates?owner_id=`) + action history; action buttons contextual (Ban/Unban, Make admin/Demote; hide Demote for self); 404 state |
| `TemplatesPage` | `getAdminTemplates` | URL-synced `q,visibility,owner_id,sort,page,per_page`; default sort `recently_updated`; row → detail; owner cell → user detail |
| `TemplateDetailPage` | `getAdminTemplate` | metadata card + versions accordion (`<pre>` content); owner link; read-only |
| `AuditLogPage` | `getAdminAuditLogs` | URL-synced `action,actor,page,per_page`; target links to user detail; reason truncation w/ `title` tooltip |

All mutations: no optimistic updates; toast on success (`NotificationContext`); on error the modal stays open and the toast shows the server message.

### 4.6 i18n keys (`admin.*` namespace, both locales)

Minimum key set (developer may add, never remove; **EN and ZH must stay in parity**):

```json
{
  "admin": {
    "nav": { "dashboard": "Dashboard", "users": "Users", "templates": "Templates", "audit_log": "Audit Log", "back_to_app": "Back to App", "brand": "Open Prompts Admin" },
    "login": { "title": "Open Prompts Admin", "subtitle": "Sign in to the admin console", "email": "Email", "password": "Password", "submit": "Sign in", "signing_in": "Signing in…", "no_admin_access": "This account does not have admin access.", "invalid": "Invalid email or password.", "banned": "This account is banned.", "back": "Back to Open Prompts", "session_expired": "Session expired. Please sign in again." },
    "dashboard": { "title": "Dashboard", "total_users": "Total Users", "active_users": "Active Users", "banned_users": "Banned Users", "total_templates": "Total Templates", "public_templates": "Public Templates", "prompts_created": "Prompts Created", "recent_signups": "Recent Signups", "recent_templates": "Recent Public Templates", "no_signups": "No signups yet.", "no_templates": "No public templates yet.", "view": "View" },
    "users": { "title": "Users", "search_placeholder": "Search ID, email, or name…", "status": "Status", "role": "Role", "sort": "Sort", "all": "All", "active": "Active", "banned": "Banned", "admin": "Admin", "user": "User", "newest": "Newest", "oldest": "Oldest", "recently_updated": "Recently updated", "templates": "Templates", "created": "Created", "actions": "Actions", "view": "View", "ban": "Ban", "unban": "Unban", "make_admin": "Make admin", "demote": "Demote to user", "no_match": "No users match your filters", "clear_filters": "Clear filters", "refresh": "Refresh", "not_found": "User not found", "back": "Back to Users", "registered": "Registered", "updated": "Updated", "likes_received": "Likes received", "favorites_received": "Favorites received", "prompts": "Prompts", "identities": "OAuth Identities", "identities_none": "No linked identities", "api_keys": "API Keys", "api_keys_none": "No API keys", "api_key_last_used": "Last used", "api_key_never": "Never", "view_all_templates": "View all", "action_history": "Admin Action History", "action_history_none": "No admin actions recorded for this user" },
    "reasonModal": { "ban_title": "Ban {{name}}", "unban_title": "Unban {{name}}", "promote_title": "Make {{name}} an admin", "demote_title": "Demote {{name}} to user", "reason_label": "Reason", "reason_required": "A reason is required (3–500 characters).", "char_count": "{{count}} / 500", "confirm_ban": "Ban user", "confirm_unban": "Unban user", "confirm_promote": "Make admin", "confirm_demote": "Demote to user", "consequence_ban_1": "The user cannot log in.", "consequence_ban_2": "Their public templates are hidden from the public square.", "consequence_ban_3": "Active sessions and API keys stop working immediately.", "consequence_unban_1": "The user can log in again.", "consequence_unban_2": "Their public templates reappear in the public square.", "consequence_role_1": "The user's active sessions are invalidated; they must log in again." },
    "templates": { "title": "Templates", "search_placeholder": "Search title or owner…", "visibility": "Visibility", "public": "Public", "private": "Private", "sort_recently_updated": "Recently updated", "sort_newest": "Newest", "sort_oldest": "Oldest", "sort_most_likes": "Most likes", "owner": "Owner", "likes": "Likes", "favorites": "Favorites", "updated": "Updated", "no_match": "No templates match your filters", "clear_filters": "Clear filters", "not_found": "Template not found", "back": "Back to Templates", "versions": "Versions", "view_content": "View content", "hide_content": "Hide content", "no_versions": "No versions", "alias": "Alias" },
    "audit": { "title": "Audit Log", "action": "Action", "actor": "Actor", "target": "Target", "reason": "Reason", "timestamp": "Timestamp", "all": "All", "ban": "Ban", "unban": "Unban", "role_change": "Role change", "search_actor": "Search actor…", "no_entries": "No admin actions recorded yet." },
    "common": { "loading": "Loading…", "error": "Something went wrong.", "retry": "Retry", "forbidden": "You do not have permission to view this page.", "not_found_title": "Not found", "page": "Page {{page}} of {{total}}", "total_count": "{{count}} items", "per_page": "{{count}} / page", "prev": "Previous", "next": "Next", "logout": "Logout", "profile": "Profile", "language": "Language", "theme": "Theme" }
  }
}
```

ZH translations for every key (e.g. `nav.dashboard: "仪表盘"`, `users.ban: "封禁"`, `login.no_admin_access: "该账号没有管理员权限。"`, `audit.title: "审计日志"`). Relative-time helper (`formatRelativeTime`) and locale number formatting live in a small shared util used by admin pages.

### 4.7 Accessibility requirements (NFR-6)

- `ReasonModal`/`Modal`: focus trap, Escape close, focus restore (existing `Modal` provides this).
- Icon-only buttons (`⋯`, refresh, theme/lang toggles): `aria-label`.
- Tables: `<th scope="col">`; row click also available via explicit "View" link.
- Badges pair color with text + icon (dot/lock) — color is never the only signal.

## 5. Error Handling Matrix

| Scenario | HTTP | gRPC code | Message (client surfaces via i18n/toast) |
|---|---|---|---|
| No/invalid token on admin endpoint | 401 | Unauthenticated | `missing or invalid admin token` |
| Valid token, not admin scope/role | 403 | PermissionDenied | `admin access required` |
| Non-admin credentials at admin login | 403 | PermissionDenied | `this account does not have admin access` |
| Banned credentials at any login | 403 | PermissionDenied | `account banned` |
| Banned user's API key | 403 | PermissionDenied | `account banned` |
| Admin login rate limit exceeded | 429 | ResourceExhausted | `too many attempts, try again later` |
| Reason missing/too short/long | 400 | InvalidArgument | `reason must be 3-500 characters` |
| Unknown user/template | 404 | NotFound | `user not found` / `template not found` |
| Self-ban / self-demotion | 403 | PermissionDenied | `you cannot ban/demote yourself` |
| Ban an already-banned user | 409 | FailedPrecondition | `user is already banned` |
| Unban an active user | 409 | FailedPrecondition | `user is not banned` |
| Role change to same role | 409 | FailedPrecondition | `user already has this role` |
| Invalid pagination/filter values | 400 | InvalidArgument | `invalid page/per_page/sort/...` |

Note: `writeError` currently maps `ResourceExhausted`/`FailedPrecondition` to 500 — **the developer must add these two mappings** (`ResourceExhausted`→429, `FailedPrecondition`→409) in `writeError`; this is additive and safe for existing endpoints.

## 6. Deployment Changes

### 6.1 `deployment/docker-compose.yml` (backend service environment)

```yaml
      # Admin bootstrap & session
      ADMIN_BOOTSTRAP_EMAIL: ${ADMIN_BOOTSTRAP_EMAIL:-}
      ADMIN_BOOTSTRAP_PASSWORD: ${ADMIN_BOOTSTRAP_PASSWORD:-}
      ADMIN_SESSION_TTL_MINUTES: ${ADMIN_SESSION_TTL_MINUTES:-30}
```

### 6.2 Helm chart

`values.yaml` → `backend.env`:

```yaml
    adminBootstrapEmail: ""
    adminBootstrapPassword: ""
    adminSessionTtlMinutes: "30"
```

`templates/deployment-backend.yaml` → add three env entries following the existing `SMTP_*` pattern.

### 6.3 `deployment/init.sql` + `deployment/chart/open-prompts/init.sql`

Append the §2 DDL block to **both** files (byte-identical block).

### 6.4 Existing-database upgrade note (for README/PR description)

`init.sql` auto-runs only on an empty volume. Existing deployments apply the same idempotent SQL once:

```bash
docker exec -i $(docker compose -f deployment/docker-compose.yml ps -q postgres) \
  psql -U user -d open_prompts < deployment/init.sql
```

## 7. Python SDK / CLI

No SDK changes required: the admin API is operator-only (not part of the public SDK surface). The existing `openprompts` client continues to work; banned users' API keys now receive 403 (documented behavior change). No CLI exists in the repo today; the "API 变更同步 CLI" constraint is satisfied vacuously.

## 8. Test Strategy

### 8.1 FVT additions (`tests/fvt.py`, developer-owned)

New constants + helpers:

```python
ADMIN_BASE = "http://localhost:8080/api/v1/admin"
ADMIN_EMAIL = "admin@example.com"
ADMIN_PASSWORD = "Admin_123456"
```

`main()` change: `run_command("ADMIN_BOOTSTRAP_EMAIL=... ADMIN_BOOTSTRAP_PASSWORD=... docker compose -f deployment/docker-compose.yml up -d --build")` (env prefix on the command so CI needs no workflow change).

New test functions (run after existing tests, before final exit):

1. `test_admin_login()` — bootstrap admin login 200 + token; regular user creds → 403; wrong password → 401; banned admin → 403 (covered after ban test); rate limit: 6 rapid failures → 429 (use a distinct IP-independent path? — rate limit is per-IP; from localhost this test must run **last** or use a fresh key; simplest: assert 429 only after 5 deliberate failures with wrong passwords, then note the 15-min window — acceptable in FVT since the suite runs once).
2. `test_admin_stats()` — create N users/templates via existing helpers; assert counts ≥ N; recent signups list non-empty.
3. `test_admin_user_list()` — search by email fragment; filter status=banned; filter role=admin; sort=oldest; pagination `per_page=10` returns `total` and correct page slicing.
4. `test_admin_ban_flow()` — create user + public template + API key; ban with reason → 200; user's old token now 401/403 on `/profile`; login → 403 "account banned"; API key call → 403; public list no longer contains the template; categories/tags counts exclude it; audit log contains the ban entry; ban again → 409; self-ban → 403.
5. `test_admin_unban_flow()` — unban with reason → 200; login works; template reappears in public list; audit entry exists.
6. `test_admin_role_change()` — promote a user → 200; that user can admin-login; demote → 200; admin-login now 403; self-demotion → 403; audit entries exist; target's old tokens invalidated (iat check).
7. `test_admin_templates()` — create public+private templates; list without visibility filter shows both; `visibility=private` shows only private; `owner_id=` filter; search by title and by owner name; sort=most_likes ordering; detail returns versions with content.
8. `test_admin_audit_log()` — entries from previous tests present, newest first; filter action=ban; filter actor=admin@example.com; pagination.

Cleanup: unban/delete what's needed; audit entries are intentionally left (append-only).

### 8.2 Backend unit tests (≥ 80 % coverage on new files)

- `admin_test.go`: reason validation, self-action rejection, already-banned/not-banned conflicts, role whitelist, audit write called with correct actor (mock repo + mock Redis per existing test style in `service/*_test.go`).
- `auth_test.go` additions: admin token claims verified; scope enforcement; revoked-before-iat rejection; banned API key rejection.

### 8.3 E2E scenarios (test-agent, Nightwatch, `test/e2e/`)

Mapped to acceptance criteria:

| E2E | AC |
|---|---|
| Admin login: valid admin, non-admin (error message), invalid creds | AC-1 |
| Route guard: unauthenticated redirect, regular-user 403 page | AC-2 |
| Dashboard: 6 KPI cards render with server values, recent lists | AC-3 |
| User list: search + status filter + pagination metadata | AC-4 |
| Ban flow: modal reason validation (3–500), success toast, badge change, banned login error, template hidden from public square | AC-5 |
| Unban flow: status restored, login works, template reappears | AC-6 |
| Role management: promote → admin badge + admin login works; self-demotion rejected | AC-7 |
| User detail: stats, identities, API keys (no secrets), templates, history | AC-8 |
| Template moderation: visibility filter, search, sort, detail versions | AC-9 |
| Audit log: entries listed newest-first, filters, immutable (no edit UI) | AC-10 |
| i18n & theme: ZH translation of all admin strings, light/dark toggle | AC-11 |
| Regular surface: no admin links; public square excludes banned owner | AC-12 |

E2E needs a seeded admin account — the e2e runner (test-agent's `nightwatch.conf.js` / global setup) starts compose with the same `ADMIN_BOOTSTRAP_*` env as FVT.

## 9. Implementation Order (suggested)

1. `init.sql` + chart copy (DDL) — foundation.
2. `admin.proto` + generated code.
3. Models + repositories (user role/status, admin repo, template banned-owner filter).
4. Auth changes (claims, context, revocation, API-key ban check, `writeError` mappings).
5. `AdminService` + bootstrap + gateway wiring.
6. Frontend: `adminApi`, `adminAuthSlice`, `AdminLayout` + login page → then dashboard → users → templates → audit.
7. i18n keys (both locales) + SCSS.
8. FVT additions; unit tests; `make check` + `npm run lint` green.
9. Docker compose verification (fresh volume + existing-volume upgrade path).

## 10. Constraints for the Developer (must respect)

1. **Never commit changes to `backend/server`** (tracked 26 MB binary) — build to a different path if needed; `git checkout -- backend/server` before committing if it shows modified.
2. Layering must match the existing structure: proto → gateway (`main.go`) → service → repository → models; no new top-level packages.
3. All admin REST routes under `/api/v1/admin/*`; all admin web routes under `/admin/*`; no admin entry points in the regular UI (FR-32).
4. i18n: every new user-visible string in **both** `en` and `zh` translation files, `admin.*` namespace.
5. `init.sql` changes must be idempotent and mirrored into the helm chart copy.
6. Env vars read via `os.Getenv` with safe defaults; secrets only via env (`.env` files are gitignored).
7. Conventional commits, English; docs (this feature's README sections) updated in the same PR; ≥ 80 % unit-test coverage on new Go files; `make lint`, `make ut`, `make fvt`, `npm run lint` all green before push.
8. Reason validation (3–500) enforced in service layer **and** DB CHECK.
9. No transactions requirement — but audit insert failure must be logged, never silently ignored.
10. `writeError` gains `ResourceExhausted`→429 and `FailedPrecondition`→409 mappings (additive).
