# 管理员用户管理系统 — 详细设计

- **功能 ID：** `admin-user-management`
- **版本：** 1.0
- **状态：** 已批准进入实现
- **作者：** 架构 Agent
- **上游：**[架构设计](./admin-user-management-architecture-zh-CN.md) · [需求分析](../design/admin-user-management-requirements-zh-CN.md) · [UI/UX 设计](../design/admin-user-management-ui-ux-design-zh-CN.md)

---

## 1. Proto 契约（`backend/api/proto/v1/admin.proto`）

与 `prompt.proto` 并列的新文件。相同 `package v1`、相同 `go_package`。**不要修改 `prompt.proto`。**

```protobuf
syntax = "proto3";

package v1;

import "google/protobuf/timestamp.proto";

option go_package = "open-prompts/backend/api/proto/v1;v1";

// AdminService 定义管理类 RPC。除 AdminLogin 外的每个 RPC
// 都要求 admin 作用域 JWT（role=admin, scope=admin）。
service AdminService {
  // AdminLogin 认证管理员并返回短期 admin 令牌。
  rpc AdminLogin(AdminLoginRequest) returns (AdminLoginResponse);

  // RefreshAdminSession 续期 admin 令牌（滑动过期）。
  rpc RefreshAdminSession(RefreshAdminSessionRequest) returns (RefreshAdminSessionResponse);

  // GetAdminStats 返回仪表盘 KPI 与最近动态。
  rpc GetAdminStats(GetAdminStatsRequest) returns (GetAdminStatsResponse);

  // ListUsers 返回分页、可搜索、可过滤的用户列表。
  rpc ListUsers(ListUsersRequest) returns (ListUsersResponse);

  // GetUser 返回单个用户的完整管理详情。
  rpc GetUser(GetUserRequest) returns (GetUserResponse);

  // BanUser 封禁用户（必填理由）。
  rpc BanUser(BanUserRequest) returns (UserActionResponse);

  // UnbanUser 解除封禁（必填理由）。
  rpc UnbanUser(UnbanUserRequest) returns (UserActionResponse);

  // ChangeUserRole 提升/降级（必填理由；禁止自我降级）。
  rpc ChangeUserRole(ChangeUserRoleRequest) returns (UserActionResponse);

  // ListTemplates 返回所有模板的分页审核列表。
  rpc ListTemplates(ListTemplatesRequest) returns (ListTemplatesResponse);

  // GetTemplate 返回任意模板及其全部版本（只读）。
  rpc GetTemplate(GetTemplateRequest) returns (GetTemplateResponse);

  // ListAuditLogs 返回只追加的审计日志。
  rpc ListAuditLogs(ListAuditLogsRequest) returns (ListAuditLogsResponse);
}

// ---- 认证 ----

message AdminLoginRequest {
  string email = 1;
  string password = 2;
}

message AdminLoginResponse {
  string token = 1;        // admin 作用域 JWT, exp = now + ADMIN_SESSION_TTL_MINUTES
  AdminUserSummary user = 2;
}

message RefreshAdminSessionRequest {}

message RefreshAdminSessionResponse {
  string token = 1;
}

// ---- 统计 ----

message GetAdminStatsRequest {}

message GetAdminStatsResponse {
  int64 total_users = 1;
  int64 active_users = 2;
  int64 banned_users = 3;
  int64 total_templates = 4;
  int64 public_templates = 5;
  int64 prompts_created = 6;
  repeated AdminUserSummary recent_signups = 7;      // 最新 10 条
  repeated AdminTemplateSummary recent_templates = 8; // 最新 10 条公开模板，按 updated_at
}

// ---- 用户 ----

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
  string search = 1;    // 匹配 id、email、display_name（不区分大小写）
  string status = 2;    // "" | "active" | "banned"
  string role = 3;      // "" | "user" | "admin"
  string sort = 4;      // "newest"（默认）| "oldest" | "recently_updated"
  int32 page = 5;       // 从 1 开始
  int32 per_page = 6;   // 默认 20，最大 100
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
  repeated AdminTemplateSummary templates = 5;      // 最新 5 个
  repeated AdminAuditLogEntry action_history = 6;   // 针对该用户的记录，最新 20 条
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
  google.protobuf.Timestamp last_used_at = 6; // 从未使用则为 null
}

message BanUserRequest {
  string user_id = 1;
  string reason = 2;    // 3-500 字符，必填
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
  AdminUserSummary user = 1;  // 操作后的状态
}

// ---- 模板 ----

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
  string search = 1;      // 匹配标题或所有者 id/display_name
  string visibility = 2;  // "" | "public" | "private"
  string owner_id = 3;    // 从用户详情页深链过滤
  string sort = 4;        // "recently_updated"（默认）| "newest" | "oldest" | "most_likes"
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
  repeated AdminTemplateVersion versions = 3;  // 全部版本，最新在前
}

message AdminTemplateVersion {
  int32 id = 1;
  int32 version = 2;
  string content = 3;
  google.protobuf.Timestamp created_at = 4;
  string alias_name = 5;   // 无别名指向该版本时为 ""
}

// ---- 审计 ----

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
  string actor = 2;     // 匹配 actor_id 或 actor_email
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

**代码生成。** 本环境未安装 `protoc`；用 `sudo apt install protobuf-compiler` 安装（可免密 sudo），插件 `protoc-gen-go`/`protoc-gen-go-grpc` 已在 `~/go/bin`。使用与 `prompt.pb.go` 相同的参数生成（在 `backend/` 目录运行）：

```bash
protoc -I api/proto/v1 \
  --go_out=. --go_opt=module=open-prompts/backend \
  --go-grpc_out=. --go-grpc_opt=module=open-prompts/backend \
  api/proto/v1/admin.proto
```

按仓库惯例将生成的 `admin.pb.go` / `admin_grpc.pb.go` 与 proto 一并提交。

## 2. 数据库 DDL（追加到 `deployment/init.sql`，同步 chart 副本）

严格遵循该文件的幂等模式。追加到文件末尾：

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

说明：
- postgres ≥ 11 中 `ADD COLUMN ... NOT NULL DEFAULT` 会自动回填现有行 —— 无需单独 UPDATE。
- `users.role`/`users.status` 不加 CHECK 约束（`DO $$` 守卫只包住 ADD COLUMN；幂等地加约束需要另一层守卫 —— 校验在 service 层强制，与现有 `templates.visibility`/`type` 的处理方式一致）。
- 相同的块必须追加到 `deployment/chart/open-prompts/init.sql`。

## 3. 后端分层职责

### 3.1 模型（`backend/internal/models/`）

`user.go` —— 增加字段（保留现有 tag）：

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

`admin.go` —— 新增：

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

### 3.2 Repository 层（`backend/internal/repository/`）

**`user_repository.go`（修改）：**
- 所有 SELECT 在列清单与扫描目标中加入 `role, status`（`GetByID`、`GetByEmail` 及其他读取点）。
- `Insert` 写入 `role, status`（为空时默认 `'user'`/`'active'` —— 现有调用方不受影响）。
- 接口新增方法：

```go
UpdateRoleAndStatus(ctx context.Context, userID, role, status string) error
```

单条 `UPDATE users SET role=$2, status=$3, updated_at=NOW() WHERE id=$1`；0 行时返回 `ErrUserNotFound`。

**`admin_repository.go`（新增）。** 接口 + `*sql.DB` 实现，遵循现有 repository 风格（context、哨兵错误、`zap` 日志）：

```go
type AdminRepository interface {
    // Users
    ListUsers(ctx context.Context, f UserFilters) ([]*models.User, int64, error) // + total
    GetUserDetail(ctx context.Context, userID string) (*models.User, error)
    CountUserTemplates(ctx context.Context, userID string) (int64, error)
    GetUserStats(ctx context.Context, userID string) (*models.UserStats, error)
    ListUserIdentities(ctx context.Context, userID string) ([]*models.OAuthIdentity, error)
    // Templates（管理视图：含私有、含被封禁用户）
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
    Search  string // 对 id、email、display_name 做 ILIKE %s%
    Status  string // "" | active | banned
    Role    string // "" | user | admin
    Sort    string // newest | oldest | recently_updated
    Limit   int
    Offset  int
}
```

关键查询：
- `ListUsers`：`WHERE (id ILIKE $1 OR email ILIKE $1 OR display_name ILIKE $1) AND ($2='' OR status=$2) AND ($3='' OR role=$3) ORDER BY <sort> LIMIT/OFFSET` + 相同 WHERE 的独立 `COUNT(*)`。排序映射：`newest`→`created_at DESC`、`oldest`→`created_at ASC`、`recently_updated`→`updated_at DESC`。
- `ListTemplates`（管理）：JOIN `users` 取所有者显示名；`WHERE (title ILIKE $1 OR owner_id ILIKE $1 OR u.display_name ILIKE $1) AND ($2='' OR visibility=$2) AND ($3='' OR owner_id=$3)`；排序：`recently_updated`→`updated_at DESC`、`newest`→`created_at DESC`、`oldest`→`created_at ASC`、`most_likes`→`like_count DESC, updated_at DESC`。
- `GetStats`：单条查询 + 标量子查询（`COUNT(*) FILTER (WHERE ...)`）得到用户/活跃/封禁/模板总数/公开模板/prompts 数；`ListRecentSignups`/`ListRecentPublicTemplates` 为 `ORDER BY ... DESC LIMIT 10` 查询（最近公开模板过滤 `visibility='public'` 且所有者未被封禁，按 `updated_at DESC`）。
- `GetUserStats`：模板数、该用户模板的 `SUM(like_count)`、`SUM(favorite_count)`、prompts 数。
- `GetTemplateWithVersions`：模板行 + 全部 `template_versions`（`version DESC`）+ 别名解析（`LEFT JOIN template_aliases` on `version_id`，每个版本取任一别名 —— 无则显示 `""`）。
- `InsertAuditLog`：普通 INSERT；DB CHECK 约束是理由长度的最后防线。

**`template_repository.go`（修改）：** 公开列表路径（公开可见性的 `List`、`ListCategories`、`ListTags`）增加 `AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = t.owner_id AND u.status='banned')`（或 JOIN 变体）。`Get` 在调用者非所有者时对被封禁所有者的公开模板返回 `ErrTemplateNotFound`。

### 3.3 Service 层（`backend/internal/service/admin.go`，新增）

```go
type AdminService struct {
    pb.UnimplementedAdminServiceServer
    UserRepo    repository.UserRepository
    AdminRepo   repository.AdminRepository
    APIKeyRepo  *repository.APIKeyRepository
    Redis       RedisStore        // 复用 user.go 中的接口
    JWTSecret   []byte
    SessionTTL  time.Duration     // 来自 ADMIN_SESSION_TTL_MINUTES
}
```

逐方法契约：

| 方法 | 校验 | 行为 | 错误 |
|---|---|---|---|
| `AdminLogin` | email+password 非空 | 限流检查（Redis `admin_login_attempts:<ip>` —— 网关通过 context 或处理器设置的请求元数据 `x-client-ip` 传递客户端 IP）；按 email 查用户；bcrypt 比对；**拒绝 `status='banned'` → 403 "account banned"**；**拒绝 `role!='admin'` → 403 "this account does not have admin access"**；签发 admin JWT（`sub`、`role:"admin"`、`scope:"admin"`、`iat`、`exp=now+TTL`、`iss`） | 400 缺字段；401 凭据无效；403 封禁/非管理员；429 限流 |
| `RefreshAdminSession` | 调用者是管理员（context） | 中间件已校验当前令牌；签发新 admin JWT | 401（中间件） |
| `GetAdminStats` | — | 委托 `AdminRepo.GetStats` + 最近列表 | 500 DB 错误 |
| `ListUsers` | page≥1、per_page 1–100（默认 20）、排序白名单、status/role 白名单 | 委托 repo | 400 参数无效 |
| `GetUser` | id 非空 | 用户 + 统计 + 身份 + API Key（经 `APIKeyRepo.ListByUserID`）+ 最新 5 个模板 + 审计历史（针对该用户最新 20 条） | 404 用户不存在 |
| `BanUser` | 理由 3–500；目标存在；**目标 != 操作者**；目标未被封禁 | 1) `UpdateRoleAndStatus(id, role, 'banned')` 2) Redis SET `user_banned:<id>` + `token_revoked_before:<id>=now` 3) `InsertAuditLog{action:"ban"}` | 400 理由不合规；403 自我封禁；404 不存在；409 已封禁 |
| `UnbanUser` | 理由 3–500；目标存在；目标已被封禁 | 1) `UpdateRoleAndStatus(id, role, 'active')` 2) Redis DEL `user_banned:<id>` 3) 审计 `unban` | 400；404；409 未封禁 |
| `ChangeUserRole` | 理由 3–500；role ∈ {user, admin}；目标存在；**目标 != 操作者**（同时阻止自我提升循环与自我降级） | 1) `UpdateRoleAndStatus(id, newRole, status)` 2) Redis SET `token_revoked_before:<id>=now` 3) 审计 `role_change` | 400；403 自我操作；404；409 角色相同 |
| `ListTemplates` | page/per_page/sort/visibility 白名单 | 委托 | 400 |
| `GetTemplate` | id 非空 | 模板 + 版本 | 404 |
| `ListAuditLogs` | page/per_page、action 白名单 | 委托 | 400 |

**审计写入顺序：** DB 变更与审计插入是两条独立语句（当前代码库风格无事务；repository 层直接使用 `*sql.DB`）。仅当现有 repo 签名能轻松支持时才用 `db.BeginTx` 包住变更+审计；否则先变更、后审计，审计插入失败时记录错误日志（绝不让用户请求失败）。审计失败绝不能导致"解封"用户。

**Redis 故障回退：** ban/unban/role-change 中 `Redis.Set/Del` 出错时记录日志并继续（DB 状态是登录的事实来源；吊销键是对活跃会话的优化）。令牌校验时 `Redis.Get` 出错则回退为 DB `users.status`/`role` 检查。

### 3.4 鉴权变更（`backend/internal/service/auth.go`）

1. admin 令牌的 `generateToken` 等价函数（放在 `admin.go`）：声明 `sub`、`role`、`scope:"admin"`、`iat`、`exp`、`iss:"open-prompts"`。
2. 扩展 `VerifyToken` → 新方法 `VerifyTokenWithClaims(token) (userID, role, scope string, iat int64, err error)`；现有 `VerifyToken` 委托给它（所有现有调用方继续工作）。
3. 新增 context 助手：`ContextWithUserRole(ctx, role)`、`GetUserRoleFromContext(ctx)`、`ContextWithUserScope`、`GetUserScopeFromContext`（镜像现有 `userIDKey` 模式，使用独立 context key）。
4. `VerifyAnyToken`（REST 处理器使用）：签名校验后检查封禁/吊销：
   - `user_banned:<sub>` 存在 → `403 "account banned"`。
   - `token_revoked_before:<sub>` > `iat` → `401 "token revoked"`。
   - Redis 出错 → DB 回退（`UserRepo.GetByID` → 检查 status/role）。
   - **API Key 路径：** 解析 Key 对应用户后加载该用户，`status='banned'` 则拒绝并返回 `403 "account banned"`（FR-22）。这给每个 API-key 请求增加一次 DB 读；可接受（SDK 流量低）且正确。
5. `publicRpcMethods` 增加 `"/v1.AdminService/AdminLogin"`。
6. gRPC unary 拦截器：注入用户 ID 后，同时把 role/scope 声明注入 context。`AdminService` 方法（`AdminLogin` 除外）调用共享的 `requireAdminContext(ctx)` 助手，除非 `role=admin && scope=admin` 否则返回 `403` —— 即便 REST 网关是主入口，gRPC 路径也受保护。

### 3.5 UserService 变更（`backend/internal/service/user.go`）

- `Login`：bcrypt 成功后 `if user.Status == "banned" → 403 "account banned"`（在签发令牌之前）。
- `LoginWithOAuth`：身份解析后同样检查。
- `generateToken`：增加 `role` 声明（取自 `user.Role`）—— 普通令牌携带 role 但**没有** `scope` 声明。
- `Register`：不变（新用户经 DB 默认值得到 `role='user'`、`status='active'`；模型零值在 repo Insert 中归一化）。

### 3.6 网关接线（`backend/cmd/server/main.go`）

1. 构造 `AdminRepository`、`AdminService`（传入 `JWTSecret`、来自 `ADMIN_SESSION_TTL_MINUTES` 环境变量的 `SessionTTL`，默认 30）。
2. 在 gRPC goroutine 中注册 `pb.RegisterAdminServiceServer(s, adminSvc)`。
3. 增加 `requireAdmin(w, r) (userID string, ok bool)` 助手：提取 Bearer 令牌 → `VerifyAnyToken` → `VerifyTokenWithClaims` → 要求 `role=admin && scope=admin` → 失败时经 `writeError` 写 401/403。所有 admin 处理器先调用它。
4. 增加处理器（与现有风格一致：CORS 头、OPTIONS 短路、protojson 序列化）：

| 路由 | 处理器草图 |
|---|---|
| `POST /api/v1/admin/login` | 解析 `AdminLoginRequest`；把 `x-client-ip` 元数据（取 `r.RemoteAddr` host 部分）放入 context 供限流；调用 `AdminLogin`；200 `{token, user}` |
| `POST /api/v1/admin/session/refresh` | `requireAdmin`；调用 `RefreshAdminSession`；200 `{token}` |
| `GET /api/v1/admin/stats` | `requireAdmin`；调用 `GetAdminStats` |
| `GET /api/v1/admin/users` | `requireAdmin`；query → `ListUsersRequest`（`search`,`status`,`role`,`sort`,`page`,`per_page`）；调用 |
| `GET /api/v1/admin/users/` | `requireAdmin`；路径后缀分发：`{id}` → `GetUser`；`{id}/ban|unban|role`（POST）→ 解析 body `{reason}` / `{role, reason}` → 调用 |
| `GET /api/v1/admin/templates` | `requireAdmin`；query → `ListTemplatesRequest`（`search`,`visibility`,`owner_id`,`sort`,`page`,`per_page`） |
| `GET /api/v1/admin/templates/` | `requireAdmin`；`{id}` → `GetTemplate` |
| `GET /api/v1/admin/audit-log` | `requireAdmin`；query → `ListAuditLogsRequest`（`action`,`actor`,`page`,`per_page`） |

路由注册顺序注意：`/api/v1/admin/users/`（带尾斜杠）必须注册在 `/api/v1/admin/users` **之后** —— Go 的 ServeMux 视它们为不同 pattern；带尾斜杠的处理器需按剩余部分分发（`ban`/`unban`/`role` 后缀），镜像现有 `/api/v1/templates/` 处理器风格。

5. **启动引导**（开始服务之前）：读取 `ADMIN_BOOTSTRAP_EMAIL`/`ADMIN_BOOTSTRAP_PASSWORD`；按架构 §6.4 实现；记录结果日志；绝不记录密码。

### 3.7 新增环境变量（后端）

| 变量 | 默认值 | 说明 |
|---|---|---|
| `ADMIN_BOOTSTRAP_EMAIL` | 空（禁用） | 启动时提升/创建 |
| `ADMIN_BOOTSTRAP_PASSWORD` | 空 | 仅在创建新引导账号时使用 |
| `ADMIN_SESSION_TTL_MINUTES` | `30` | admin JWT 生命周期 |

## 4. 前端设计

### 4.1 文件布局

```
frontend/src/
├── App.js                                  （修改：/admin 路由）
├── pages/admin/
│   ├── AdminLoginPage.js/.scss
│   ├── DashboardPage.js/.scss
│   ├── UsersPage.js/.scss
│   ├── UserDetailPage.js/.scss
│   ├── TemplatesPage.js/.scss
│   ├── TemplateDetailPage.js/.scss
│   └── AuditLogPage.js/.scss
├── components/admin/
│   ├── AdminLayout.js/.scss                （外壳 + 路由守卫）
│   ├── AdminTable.js/.scss                 （通用表格：骨架屏/吸顶/行点击）
│   ├── KpiCard.js/.scss
│   ├── Badge.js/.scss                      （tone: accent|success|danger|warning|neutral）
│   ├── Pagination.js/.scss
│   ├── ReasonModal.js/.scss                （复用 ui/Modal）
│   ├── AdminSearchInput.js/.scss           （防抖 300ms + 清除）
│   ├── AdminSelect.js/.scss
│   ├── EmptyState.js/.scss
│   └── ErrorBanner.js/.scss
├── services/adminApi.js                    （新 axios 实例）
└── store/adminAuthSlice.js                 （新 redux slice）
```

### 4.2 路由（`App.js`）

```jsx
import { lazy, Suspense } from 'react';
const AdminLayout = lazy(() => import('./components/admin/AdminLayout'));
const AdminLoginPage = lazy(() => import('./pages/admin/AdminLoginPage'));
// ... 其他 admin 页面

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

`AdminLayout` 渲染 `<Outlet/>`；守卫逻辑（按 UI/UX §4）：无 `op_admin_token` → `<Navigate to="/admin/login?redirect=..." replace/>`；有令牌但用户角色 ≠ admin（本地状态过期）→ 渲染 403 页面（API 仍是安全边界）。

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

**滑动会话续期：** 小型 `ensureFreshAdminToken()` 助手（在 admin 请求成功后调用）：从 JWT payload 解码 `iat`/`exp`（base64url，无需库）；若 `now - iat > 10 分钟`，调用 `adminRefreshSession()`，存新令牌并更新 axios 默认值。失败 → 登出流程。

### 4.4 Redux（`store/adminAuthSlice.js`）

```js
const STORAGE_KEYS = { token: 'op_admin_token', user: 'op_admin_user' };
// reducers: adminLoginSuccess({user, token}), adminLogout(), initializeAdminAuth()
// 与 authSlice.js 结构相同；localStorage 键独立
```

在 `store.js` 中注册为 `adminAuth: adminAuthReducer`。

### 4.5 页面行为契约

| 页面 | 数据调用 | 关键交互 |
|---|---|---|
| `AdminLoginPage` | `adminLogin` | 内联错误（403 非管理员 → `admin.login.no_admin_access`；401 → `admin.login.invalid`；403 封禁 → `admin.login.banned`）；成功后存 token+user，跳转 `?redirect` 或 `/admin` |
| `DashboardPage` | `getAdminStats` | 6 张 KPI 卡（链接按 UI/UX §5.2）、最近注册列表（→ 用户详情）、最近模板列表（→ 模板详情）；骨架屏/空/错误状态 |
| `UsersPage` | `getAdminUsers` | 经 `useSearchParams` URL 同步 `q,status,role,sort,page,per_page`；防抖 300ms；行点击 → 详情；`⋯` 菜单 → 查看 / 封禁…/解禁… / 设为管理员…/降级…；打开 `ReasonModal`；成功后 toast + 重新拉取 |
| `UserDetailPage` | `getAdminUser` | 头部卡片 + 4 张统计卡 + 身份 + API Key + 最新 5 个模板（"查看全部" → `/admin/templates?owner_id=`）+ 操作历史；操作按钮按上下文显示（封禁/解禁、设为管理员/降级；对自己隐藏降级）；404 状态 |
| `TemplatesPage` | `getAdminTemplates` | URL 同步 `q,visibility,owner_id,sort,page,per_page`；默认排序 `recently_updated`；行 → 详情；所有者单元格 → 用户详情 |
| `TemplateDetailPage` | `getAdminTemplate` | 元数据卡 + 版本手风琴（`<pre>` 内容）；所有者链接；只读 |
| `AuditLogPage` | `getAdminAuditLogs` | URL 同步 `action,actor,page,per_page`；目标链接到用户详情；理由截断 + `title` 提示 |

所有变更操作：不做乐观更新；成功 toast（`NotificationContext`）；错误时内联展示 + toast。

### 4.6 i18n 键（`admin.*` 命名空间，两个语言文件）

最小键集合（开发者可增不可删；**EN 与 ZH 必须保持对齐**）：

```json
{
  "admin": {
    "nav": { "dashboard": "仪表盘", "users": "用户", "templates": "模板", "audit_log": "审计日志", "back_to_app": "返回应用", "brand": "Open Prompts 管理控制台" },
    "login": { "title": "Open Prompts 管理控制台", "subtitle": "登录管理控制台", "email": "邮箱", "password": "密码", "submit": "登录", "signing_in": "登录中…", "no_admin_access": "该账号没有管理员权限。", "invalid": "邮箱或密码错误。", "banned": "该账号已被封禁。", "back": "返回 Open Prompts", "session_expired": "会话已过期，请重新登录。" },
    "dashboard": { "title": "仪表盘", "total_users": "用户总数", "active_users": "活跃用户", "banned_users": "被封禁用户", "total_templates": "模板总数", "public_templates": "公开模板", "prompts_created": "已创建 Prompts", "recent_signups": "最近注册", "recent_templates": "最近公开模板", "no_signups": "暂无注册。", "no_templates": "暂无公开模板。", "view": "查看" },
    "users": { "title": "用户", "search_placeholder": "搜索 ID、邮箱或名称…", "status": "状态", "role": "角色", "sort": "排序", "all": "全部", "active": "活跃", "banned": "已封禁", "admin": "管理员", "user": "普通用户", "newest": "最新注册", "oldest": "最早注册", "recently_updated": "最近更新", "templates": "模板", "created": "创建时间", "actions": "操作", "view": "查看", "ban": "封禁", "unban": "解禁", "make_admin": "设为管理员", "demote": "降级为普通用户", "no_match": "没有符合过滤条件的用户", "clear_filters": "清除过滤", "refresh": "刷新", "not_found": "用户不存在", "back": "返回用户列表", "registered": "注册时间", "updated": "更新时间", "likes_received": "收到的点赞", "favorites_received": "收到的收藏", "prompts": "Prompts", "identities": "OAuth 身份", "identities_none": "无关联身份", "api_keys": "API Key", "api_keys_none": "无 API Key", "api_key_last_used": "最近使用", "api_key_never": "从未使用", "view_all_templates": "查看全部", "action_history": "管理员操作历史", "action_history_none": "该用户暂无管理员操作记录" },
    "reasonModal": { "ban_title": "封禁 {{name}}", "unban_title": "解禁 {{name}}", "promote_title": "将 {{name}} 设为管理员", "demote_title": "将 {{name}} 降级为普通用户", "reason_label": "理由", "reason_required": "必须填写理由（3–500 字符）。", "char_count": "{{count}} / 500", "confirm_ban": "封禁用户", "confirm_unban": "解禁用户", "confirm_promote": "设为管理员", "confirm_demote": "降级为普通用户", "consequence_ban_1": "该用户将无法登录。", "consequence_ban_2": "其公开模板将从公开广场隐藏。", "consequence_ban_3": "活跃会话与 API Key 立即失效。", "consequence_unban_1": "该用户可以重新登录。", "consequence_unban_2": "其公开模板重新出现在公开广场。", "consequence_role_1": "该用户的活跃会话将失效，需要重新登录。" },
    "templates": { "title": "模板", "search_placeholder": "搜索标题或所有者…", "visibility": "可见性", "public": "公开", "private": "私有", "sort_recently_updated": "最近更新", "sort_newest": "最新", "sort_oldest": "最早", "sort_most_likes": "点赞最多", "owner": "所有者", "likes": "点赞", "favorites": "收藏", "updated": "更新时间", "no_match": "没有符合过滤条件的模板", "clear_filters": "清除过滤", "not_found": "模板不存在", "back": "返回模板列表", "versions": "版本", "view_content": "查看内容", "hide_content": "收起内容", "no_versions": "暂无版本", "alias": "别名" },
    "audit": { "title": "审计日志", "action": "操作", "actor": "操作者", "target": "目标", "reason": "理由", "timestamp": "时间", "all": "全部", "ban": "封禁", "unban": "解禁", "role_change": "角色变更", "search_actor": "搜索操作者…", "no_entries": "暂无管理员操作记录。" },
    "common": { "loading": "加载中…", "error": "出错了。", "retry": "重试", "forbidden": "你没有权限查看此页面。", "not_found_title": "未找到", "page": "第 {{page}} / {{total}} 页", "total_count": "{{count}} 项", "per_page": "{{count}} 条/页", "prev": "上一页", "next": "下一页", "logout": "退出登录", "profile": "个人资料", "language": "语言", "theme": "主题" }
  }
}
```

（EN 语言文件使用对应的英文文案，键结构完全相同。）相对时间助手（`formatRelativeTime`）与本地化数字格式放在管理员页面共用的小工具模块中。

### 4.7 无障碍要求（NFR-6）

- `ReasonModal`/`Modal`：焦点陷阱、Escape 关闭、焦点恢复（现有 `Modal` 已提供）。
- 纯图标按钮（`⋯`、刷新、主题/语言切换）：加 `aria-label`。
- 表格：`<th scope="col">`；行点击同时提供显式"查看"链接。
- 徽章颜色与文字 + 图标（圆点/锁）并用 —— 颜色绝不是唯一信号。

## 5. 错误处理矩阵

| 场景 | HTTP | gRPC 码 | 消息（客户端经 i18n/toast 展示） |
|---|---|---|---|
| admin 端点无/无效令牌 | 401 | Unauthenticated | `missing or invalid admin token` |
| 令牌有效但非 admin 作用域/角色 | 403 | PermissionDenied | `admin access required` |
| 非管理员凭据调用 admin 登录 | 403 | PermissionDenied | `this account does not have admin access` |
| 被封禁凭据调用任意登录 | 403 | PermissionDenied | `account banned` |
| 被封禁用户的 API Key | 403 | PermissionDenied | `account banned` |
| admin 登录超过限流 | 429 | ResourceExhausted | `too many attempts, try again later` |
| 理由缺失/过短/过长 | 400 | InvalidArgument | `reason must be 3-500 characters` |
| 用户/模板不存在 | 404 | NotFound | `user not found` / `template not found` |
| 自我封禁 / 自我降级 | 403 | PermissionDenied | `you cannot ban/demote yourself` |
| 封禁已被封禁的用户 | 409 | FailedPrecondition | `user is already banned` |
| 解禁活跃用户 | 409 | FailedPrecondition | `user is not banned` |
| 角色变更为相同角色 | 409 | FailedPrecondition | `user already has this role` |
| 无效的分页/过滤值 | 400 | InvalidArgument | `invalid page/per_page/sort/...` |

注意：`writeError` 目前把 `ResourceExhausted`/`FailedPrecondition` 映射到 500 —— **开发者必须补充这两个映射**（`ResourceExhausted`→429、`FailedPrecondition`→409）；这是增量变更，对现有端点安全。

## 6. 部署变更

### 6.1 `deployment/docker-compose.yml`（backend 服务 environment）

```yaml
      # Admin bootstrap & session
      ADMIN_BOOTSTRAP_EMAIL: ${ADMIN_BOOTSTRAP_EMAIL:-}
      ADMIN_BOOTSTRAP_PASSWORD: ${ADMIN_BOOTSTRAP_PASSWORD:-}
      ADMIN_SESSION_TTL_MINUTES: ${ADMIN_SESSION_TTL_MINUTES:-30}
```

### 6.2 Helm chart

`values.yaml` → `backend.env`：

```yaml
    adminBootstrapEmail: ""
    adminBootstrapPassword: ""
    adminSessionTtlMinutes: "30"
```

`templates/deployment-backend.yaml` → 按现有 `SMTP_*` 模式增加三个 env 条目。

### 6.3 `deployment/init.sql` + `deployment/chart/open-prompts/init.sql`

将 §2 的 DDL 块追加到**两个**文件（字节一致的块）。

### 6.4 既有数据库升级说明（写入 README/PR 描述）

`init.sql` 只在空数据卷上自动执行。既有部署需一次性执行相同的幂等 SQL：

```bash
docker exec -i $(docker compose -f deployment/docker-compose.yml ps -q postgres) \
  psql -U user -d open_prompts < deployment/init.sql
```

## 7. Python SDK / CLI

无需 SDK 变更：admin API 仅面向运维人员（不属于公共 SDK 面）。现有 `openprompts` 客户端继续工作；被封禁用户的 API Key 将收到 403（已记录的行为变更）。仓库今天没有 CLI；"API 变更同步 CLI" 约束自然满足。

## 8. 测试策略

### 8.1 FVT（`tests/fvt.py`，开发者实现）

新增常量 + 助手：

```python
ADMIN_BASE = "http://localhost:8080/api/v1/admin"
ADMIN_EMAIL = "admin@example.com"
ADMIN_PASSWORD = "Admin_123456"
```

`main()` 变更：`run_command("ADMIN_BOOTSTRAP_EMAIL=... ADMIN_BOOTSTRAP_PASSWORD=... docker compose -f deployment/docker-compose.yml up -d --build")`（命令前缀环境变量，CI 无需改 workflow）。

新增测试函数（在现有测试之后、最终退出之前运行）：

1. `test_admin_login()` —— 引导管理员登录 200 + token；普通用户凭据 → 403；错误密码 → 401；被封禁管理员 → 403（在封禁测试之后覆盖）；限流：连续 6 次失败 → 429（限流按 IP；从 localhost 运行时该测试必须放在**最后**或使用全新 key；最简单：5 次故意输错密码后断言 429，并注明 15 分钟窗口 —— FVT 只跑一次，可接受）。
2. `test_admin_stats()` —— 用现有助手创建 N 个用户/模板；断言计数 ≥ N；最近注册列表非空。
3. `test_admin_user_list()` —— 按 email 片段搜索；status=banned 过滤；role=admin 过滤；sort=oldest；分页 `per_page=10` 返回 `total` 且分页切片正确。
4. `test_admin_ban_flow()` —— 创建用户 + 公开模板 + API Key；带理由封禁 → 200；用户旧令牌在 `/profile` 上 401/403；登录 → 403 "account banned"；API Key 调用 → 403；公开列表不再包含该模板；分类/标签计数排除它；审计日志含封禁记录；再次封禁 → 409；自我封禁 → 403。
5. `test_admin_unban_flow()` —— 带理由解禁 → 200；登录可用；模板重新出现在公开列表；审计记录存在。
6. `test_admin_role_change()` —— 提升用户 → 200；该用户可 admin 登录；降级 → 200；admin 登录变为 403；自我降级 → 403；审计记录存在；目标旧令牌失效（iat 检查）。
7. `test_admin_templates()` —— 创建公开+私有模板；无可见性过滤的列表两者都显示；`visibility=private` 只显示私有；`owner_id=` 过滤；按标题与按所有者名搜索；sort=most_likes 排序；详情返回带内容的版本。
8. `test_admin_audit_log()` —— 前面测试的记录存在、最新在前；action=ban 过滤；actor=admin@example.com 过滤；分页。

清理：按需解禁/删除；审计记录有意保留（只追加）。

### 8.2 后端单元测试（新文件覆盖率 ≥ 80%）

- `admin_test.go`：理由校验、自我操作拒绝、已封禁/未封禁冲突、角色白名单、审计写入以正确操作者调用（按 `service/*_test.go` 现有风格 mock repo + mock Redis）。
- `auth_test.go` 增补：admin 令牌声明校验；scope 强制；revoked-before-iat 拒绝；被封禁 API Key 拒绝。

### 8.3 E2E 场景（测试 Agent，Nightwatch，`test/e2e/`）

与验收标准映射：

| E2E | AC |
|---|---|
| 管理员登录：有效管理员、非管理员（错误消息）、无效凭据 | AC-1 |
| 路由守卫：未认证重定向、普通用户 403 页面 | AC-2 |
| 仪表盘：6 张 KPI 卡渲染服务端值、最近列表 | AC-3 |
| 用户列表：搜索 + 状态过滤 + 分页元数据 | AC-4 |
| 封禁流程：弹窗理由校验（3–500）、成功 toast、徽章变化、被封禁登录错误、模板从公开广场隐藏 | AC-5 |
| 解禁流程：状态恢复、登录可用、模板重新出现 | AC-6 |
| 角色管理：提升 → admin 徽章 + admin 登录可用；自我降级被拒 | AC-7 |
| 用户详情：统计、身份、API Key（无密钥）、模板、历史 | AC-8 |
| 模板审核：可见性过滤、搜索、排序、详情版本 | AC-9 |
| 审计日志：记录最新在前、过滤、不可变（无编辑 UI） | AC-10 |
| i18n 与主题：所有 admin 文案的中文翻译、明暗切换 | AC-11 |
| 普通用户面：无 admin 入口；公开广场排除被封禁所有者 | AC-12 |

E2E 需要预置管理员账号 —— e2e 运行器（测试 Agent 的 `nightwatch.conf.js` / 全局 setup）以与 FVT 相同的 `ADMIN_BOOTSTRAP_*` 环境变量启动 compose。

## 9. 实现顺序（建议）

1. `init.sql` + chart 副本（DDL）—— 基础。
2. `admin.proto` + 生成代码。
3. 模型 + repository（用户 role/status、admin repo、模板被封禁所有者过滤）。
4. 鉴权变更（声明、context、吊销、API Key 封禁检查、`writeError` 映射）。
5. `AdminService` + 引导 + 网关接线。
6. 前端：`adminApi`、`adminAuthSlice`、`AdminLayout` + 登录页 → 仪表盘 → 用户 → 模板 → 审计。
7. i18n 键（两个语言文件）+ SCSS。
8. FVT 增补；单元测试；`make check` + `npm run lint` 通过。
9. Docker compose 验证（全新数据卷 + 既有数据卷升级路径）。

## 10. 开发者约束（必须遵守）

1. **绝不提交 `backend/server` 的变更**（被跟踪的 26 MB 二进制）—— 必要时构建到其他路径；提交前若显示 modified 则 `git checkout -- backend/server`。
2. 分层必须与现有结构一致：proto → 网关（`main.go`）→ service → repository → models；不新增顶层包。
3. 所有 admin REST 路由在 `/api/v1/admin/*` 之下；所有 admin Web 路由在 `/admin/*` 之下；普通 UI 中不出现 admin 入口（FR-32）。
4. i18n：每个新的用户可见字符串同时写入 **en** 与 **zh** 翻译文件，`admin.*` 命名空间。
5. `init.sql` 变更必须幂等并同步到 helm chart 副本。
6. 环境变量经 `os.Getenv` 读取并带安全默认值；密钥只经环境变量（`.env` 文件已 gitignore）。
7. Conventional commits、英文；本功能的 README 章节在同一 PR 更新；新增 Go 文件单元测试覆盖率 ≥ 80%；推送前 `make lint`、`make ut`、`make fvt`、`npm run lint` 全绿。
8. 理由校验（3–500）在 service 层**和** DB CHECK 双重强制。
9. 无事务要求 —— 但审计插入失败必须记录日志，绝不静默忽略。
10. `writeError` 增加 `ResourceExhausted`→429 与 `FailedPrecondition`→409 映射（增量）。
