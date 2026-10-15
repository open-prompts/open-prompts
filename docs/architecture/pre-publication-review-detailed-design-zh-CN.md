# 发布前审核 — 详细设计

| | |
|---|---|
| **Feature ID** | pre-publication-review |
| **Version** | 1.0 |
| **Status** | Approved for implementation |
| **Author** | Architect Agent |
| **Upstream** | [架构设计](pre-publication-review-architecture-zh-CN.md) |

## 1. Proto 契约

### 1.1 `backend/api/proto/v1/prompt.proto`

`Template` 字段号是跨设计协同的区块：17 = `comment_count`（已上线），
18/19 由 follow-creators 预留（`owner_display_name`/`owner_avatar`），
20–22 由 template-ratings 预留，23/24 由 prompt-analytics 预留。
**本特性使用 25/26 —— 不得复用 18–24。**

```protobuf
message Template {
    // ... 既有字段 1-17 ...
    // 审核状态："normal" | "in_review" | "rejected"。
    // 仅对所有者与管理员有意义；被隐藏模板对其他人 404，
    // 因此该值绝不泄露。
    string moderation_status = 25;
    // 首次审核批准的时间；在此之前为 null/缺省。
    google.protobuf.Timestamp published_at = 26;
}
```

`templateModelToProto` 映射两者（`ModerationStatus: m.ModerationStatus`，
有效时 `PublishedAt: timestamppb.New(m.PublishedAt.Time)`）。prompt.proto
无其他变更 —— 设闸复用原样的 `CreateTemplateRequest`/
`UpdateTemplateRequest`；响应 `Template` 携带新字段供前端响应。

### 1.2 `backend/api/proto/v1/admin.proto`

`AdminService` 追加 7 个新 RPC（既有 13 个不动）：

```protobuf
service AdminService {
    // ... 既有 RPC 1-13 ...
    rpc ListReviewQueue(ListReviewQueueRequest) returns (ListReviewQueueResponse);
    rpc GetReviewSubmission(GetReviewSubmissionRequest) returns (GetReviewSubmissionResponse);
    rpc ApproveReviewSubmission(ApproveReviewSubmissionRequest) returns (ReviewDecisionResponse);
    rpc RejectReviewSubmission(RejectReviewSubmissionRequest) returns (ReviewDecisionResponse);
    rpc GetReviewMetrics(GetReviewMetricsRequest) returns (ReviewMetrics);
    rpc GetPlatformSettings(GetPlatformSettingsRequest) returns (GetPlatformSettingsResponse);
    rpc UpdatePlatformSetting(UpdatePlatformSettingRequest) returns (PlatformSettingEntry);
}

message ListReviewQueueRequest {
    string category = 1;         // "" = 任意
    bool aging_only = 2;         // 仅超过超时窗口的提交
    bool resubmission_only = 3;  // 仅 attempt_number >= 2
    int32 page = 4;
    int32 per_page = 5;
}

message ReviewQueueItem {
    string id = 1;                 // 提交 id
    string template_id = 2;
    string template_title = 3;
    string creator_id = 4;
    string creator_display_name = 5;
    int32 attempt_number = 6;      // >= 2 渲染"（第 n 次）"标签
    bool is_resubmission = 7;      // attempt_number >= 2
    google.protobuf.Timestamp submitted_at = 8;
    string category = 9;
    bool aging = 10;               // submitted_at 早于超时窗口
    int32 creator_approved_count = 11;   // users.approved_template_count
    int32 creator_prior_rejections = 12; // 该创作者被拒的提交数
    bool creator_banned = 13;      // OQ-4：审核期间封禁标记
}

message ListReviewQueueResponse {
    repeated ReviewQueueItem items = 1;
    int64 total = 2;
    int32 page = 3;
    int32 per_page = 4;
}

message GetReviewSubmissionRequest {
    string id = 1; // 提交 id
}

message ReviewChainEntry {
    int32 attempt_number = 1;
    string status = 2;             // "approved" | "rejected"
    google.protobuf.Timestamp submitted_at = 3;
    google.protobuf.Timestamp decided_at = 4;
    string reject_reason_code = 5; // 批准时为空
    string reject_note = 6;
}

message ReviewSubmissionDetail {
    string id = 1;
    string template_id = 2;
    string template_title = 3;
    string template_description = 4;
    string template_content = 5;   // 最新版本内容
    repeated string tags = 6;
    string category = 7;
    string language = 8;
    string creator_id = 9;
    string creator_display_name = 10;
    google.protobuf.Timestamp creator_joined_at = 11;
    int32 creator_published_count = 12;   // 已批准模板数
    int32 creator_prior_rejections = 13;
    bool creator_banned = 14;
    int32 attempt_number = 15;
    google.protobuf.Timestamp submitted_at = 16;
    repeated ReviewChainEntry chain = 17; // 此前尝试，最旧在前
}

message GetReviewSubmissionResponse {
    ReviewSubmissionDetail submission = 1;
}

message ApproveReviewSubmissionRequest {
    string id = 1; // 提交 id
}

message RejectReviewSubmissionRequest {
    string id = 1;          // 提交 id
    string reason_code = 2; // 必填：spam | plagiarism | harmful | lowQuality | other
    string note = 3;        // 可选，<= 500 字符
}

message ReviewDecisionResponse {
    string submission_id = 1;
    string template_id = 2;
    string status = 3; // "approved" | "rejected"
}

message GetReviewMetricsRequest {}

message RejectionRateEntry {
    string reason_code = 1;
    int32 count = 2;
    double rate = 3; // 占拒绝（非全部决定）的比例
}

message ReviewMetrics {
    int64 open_count = 1;
    // 当前未决提交已等待时长的中位数（小时）。
    double median_time_in_review_hours = 2;
    int64 decisions_today = 3;
    repeated RejectionRateEntry rejection_rate_by_reason = 4;
}

message GetPlatformSettingsRequest {}

message PlatformSettingEntry {
    string key = 1;
    string value = 2;
    google.protobuf.Timestamp updated_at = 3;
}

message GetPlatformSettingsResponse {
    repeated PlatformSettingEntry settings = 1;
}

message UpdatePlatformSettingRequest {
    string key = 1;    // 必须存在于已知键白名单
    string value = 2;  // review_approval_threshold 为 >= 0 整数（0 = 关闭门控），其余为 >= 1 整数
    string reason = 3; // 3-500 字符，记入审计
}
```

重新生成：

```bash
cd backend && protoc --go_out=. --go-grpc_out=. api/proto/v1/prompt.proto api/proto/v1/admin.proto
```

## 2. 数据库 DDL

追加到 **`deployment/init.sql` 与
`deployment/chart/open-prompts/init.sql` 两份**（逐字节一致）：

```sql
-- -----------------------------------------------------------------------------
-- Feature: pre-publication-review
-- Description: gating state, review submissions, platform settings.
-- -----------------------------------------------------------------------------

-- Gating state on users (OQ-1: denormalized counters). The backfill runs
-- inside the column-add guard, so it executes exactly once — on first
-- migration onto this schema.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='users' AND column_name='approved_template_count') THEN
        ALTER TABLE users ADD COLUMN approved_template_count INT NOT NULL DEFAULT 0;
        COMMENT ON COLUMN users.approved_template_count IS
            'Approved templates maintained at review decision time (gating threshold N)';
        -- Backfill: existing public templates count as approved.
        UPDATE users u
        SET approved_template_count = (
            SELECT COUNT(*) FROM templates t
            WHERE t.owner_id = u.id AND t.visibility = 'public'
        );
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='users' AND column_name='review_gated_remaining') THEN
        ALTER TABLE users ADD COLUMN review_gated_remaining INT NOT NULL DEFAULT 0;
        COMMENT ON COLUMN users.review_gated_remaining IS
            'Publishes still gated after a moderation takedown/ban (re-gate count M)';
    END IF;
END $$;

-- Moderation state + first-approval time on templates. The published_at
-- backfill runs inside the column-add guard (once, on first migration).
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='templates' AND column_name='moderation_status') THEN
        ALTER TABLE templates ADD COLUMN moderation_status TEXT NOT NULL DEFAULT 'normal';
        ALTER TABLE templates ADD CONSTRAINT templates_moderation_status_check
            CHECK (moderation_status IN ('normal', 'in_review', 'rejected'));
        COMMENT ON COLUMN templates.moderation_status IS
            'normal | in_review | rejected (admin-template-moderation widens this list later)';
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='templates' AND column_name='published_at') THEN
        ALTER TABLE templates ADD COLUMN published_at TIMESTAMPTZ;
        COMMENT ON COLUMN templates.published_at IS 'First review-approval time; NULL until approved';
        -- Backfill: existing public templates are considered published at creation.
        UPDATE templates SET published_at = created_at
        WHERE visibility = 'public' AND published_at IS NULL;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_templates_moderation_status
    ON templates(moderation_status);

-- Review submissions (OQ-2: separate table; chain via UNIQUE(template_id, attempt_number)).
CREATE TABLE IF NOT EXISTS review_submissions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    creator_id TEXT NOT NULL,
    attempt_number INT NOT NULL CHECK (attempt_number >= 1),
    status TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'rejected')),
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    decided_at TIMESTAMPTZ,
    decided_by TEXT,
    reject_reason_code TEXT,
    reject_note TEXT,
    UNIQUE (template_id, attempt_number)
);

COMMENT ON TABLE review_submissions IS 'Pre-publication review queue; one row per publish attempt';
COMMENT ON COLUMN review_submissions.attempt_number IS '1 = first submission; >= 2 = resubmission';
COMMENT ON COLUMN review_submissions.status IS 'pending | approved | rejected';
COMMENT ON COLUMN review_submissions.reject_reason_code IS 'spam | plagiarism | harmful | lowQuality | other (NULL for approvals)';

-- Partial index serves the queue: pending only, aging-first + FIFO order.
CREATE INDEX IF NOT EXISTS idx_review_submissions_pending
    ON review_submissions (status, submitted_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_review_submissions_template
    ON review_submissions (template_id);
CREATE INDEX IF NOT EXISTS idx_review_submissions_creator
    ON review_submissions (creator_id, status);

-- Platform settings (NFR-5: N/M/aging configurable without deploy).
CREATE TABLE IF NOT EXISTS platform_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE platform_settings IS 'Admin-editable platform settings (key-value)';

INSERT INTO platform_settings (key, value) VALUES
    ('review_approval_threshold', '3'),
    ('review_regate_count', '3'),
    ('review_aging_hours', '48')
ON CONFLICT (key) DO NOTHING;

-- Widen admin_audit_logs CHECKs for review decisions (guarded, idempotent).
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'admin_audit_logs'::regclass AND conname = 'admin_audit_logs_action_check';
    IF def IS NOT NULL AND def NOT LIKE '%review_approve%' THEN
        ALTER TABLE admin_audit_logs DROP CONSTRAINT admin_audit_logs_action_check;
        ALTER TABLE admin_audit_logs ADD CONSTRAINT admin_audit_logs_action_check
            CHECK (action IN ('ban', 'unban', 'role_change', 'delete_comment',
                              'review_approve', 'review_reject', 'settings_change'));
    END IF;
END $$;

DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'admin_audit_logs'::regclass AND conname = 'admin_audit_logs_target_type_check';
    IF def IS NOT NULL AND def NOT LIKE '%template%' THEN
        ALTER TABLE admin_audit_logs DROP CONSTRAINT admin_audit_logs_target_type_check;
        ALTER TABLE admin_audit_logs ADD CONSTRAINT admin_audit_logs_target_type_check
            CHECK (target_type IN ('user', 'comment', 'template', 'platform'));
    END IF;
END $$;

-- Widen notifications.type CHECK for decision notifications (guarded).
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'notifications'::regclass AND conname = 'notifications_type_check';
    IF def IS NOT NULL AND def NOT LIKE '%template_approved%' THEN
        ALTER TABLE notifications DROP CONSTRAINT notifications_type_check;
        ALTER TABLE notifications ADD CONSTRAINT notifications_type_check
            CHECK (type IN ('like', 'favorite', 'fork', 'comment', 'template_approved', 'template_rejected'));
    END IF;
END $$;
```

## 3. 后端分层

### 3.1 模型

```go
// models/template.go — 新增：
ModerationStatus string         `json:"moderation_status"` // "normal" | "in_review" | "rejected"
PublishedAt      sql.NullTime   `json:"published_at"`

// models/user.go — 新增：
ApprovedTemplateCount int32 `json:"approved_template_count"`
ReviewGatedRemaining  int32 `json:"review_gated_remaining"`

// models/review.go — 新文件：
type ReviewSubmission struct {
    ID               string     `json:"id"`
    TemplateID       string     `json:"template_id"`
    CreatorID        string     `json:"creator_id"`
    AttemptNumber    int32      `json:"attempt_number"`
    Status           string     `json:"status"` // "pending" | "approved" | "rejected"
    SubmittedAt      time.Time  `json:"submitted_at"`
    DecidedAt        *time.Time `json:"decided_at"`
    DecidedBy        *string    `json:"decided_by"`
    RejectReasonCode *string    `json:"reject_reason_code"`
    RejectNote       *string    `json:"reject_note"`
}

type PlatformSetting struct {
    Key       string    `json:"key"`
    Value     string    `json:"value"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

### 3.2 `TemplateRepository` 变更

`Create` INSERT 增加 `moderation_status`、`published_at` 列；`Update`
SET 子句增加两者；`Get`/`List`/`CountTemplates` SELECT 增加两者（扫描进
新模型字段）。

**`Get` 隐藏规则**（扩展现有封禁所有者块）：

```go
// in_review/rejected 模板仅对其所有者可见（管理员使用不过滤的
// admin 仓储）。对其他所有人 404 等效 —— 不泄露状态。
if t.ModerationStatus != "normal" && t.OwnerID != currentUserID {
    return nil, fmt.Errorf("template not found")
}
```

**`buildTemplateConditions`** —— 公开流的 moderation 过滤镜像封禁所有者
模式，应用于同样的条件（visibility 缺失或 public），通过已绑定的
`current_user_id` 占位符 `$1` 实现所有者旁路：

```sql
AND (
    t.moderation_status = 'normal'
    OR t.owner_id = $1   -- $1 为 current_user_id（匿名时为 ""）
)
```

因为匿名调用者的 `$1` 是 `''`，旁路永不匹配，匿名/公开流只看到
`normal` 模板；所有者的混合视图看到自己的被闸模板。`CountTemplates`
共享构建器，计数保持一致。私有流分支（`visibility = 'private'`）不动
—— 私有模板永不设闸。

### 3.3 `ReviewRepository`（新文件）

```go
type ReviewQueueFilters struct {
    Category        string
    AgingOnly       bool
    ResubmissionOnly bool
    AgingHours      int
    Limit           int
    Offset          int
}

type ReviewRepository interface {
    CreateSubmission(ctx context.Context, s *models.ReviewSubmission) error
    GetSubmission(ctx context.Context, id string) (*models.ReviewSubmission, error)
    GetLatestSubmission(ctx context.Context, templateID string) (*models.ReviewSubmission, error)
    ListQueue(ctx context.Context, f ReviewQueueFilters) ([]*models.ReviewQueueRow, int64, error)
    GetSubmissionDetail(ctx context.Context, id string) (*models.ReviewSubmissionDetail, error)
    ListChain(ctx context.Context, templateID string) ([]*models.ReviewSubmission, error)
    DecideSubmission(ctx context.Context, id string, status string, decidedBy string,
        reasonCode, note *string) (*models.ReviewSubmission, error) // tx: UPDATE ... WHERE status='pending'
    GetMetrics(ctx context.Context) (*models.ReviewMetrics, error)
    // Platform settings
    GetSettings(ctx context.Context) ([]*models.PlatformSetting, error)
    GetSetting(ctx context.Context, key string) (string, error)
    UpsertSetting(ctx context.Context, key, value string) error
    // Gating state
    GetUserGatingState(ctx context.Context, userID string) (approved int32, gatedRemaining int32, err error)
    IncrementApprovedCount(ctx context.Context, userID string) error
    DecrementGatedRemaining(ctx context.Context, userID string) error
    SetGatedRemaining(ctx context.Context, userID string, value int32) error
}
```

**队列 SQL**（超时优先，然后 FIFO —— OQ-5）：

```sql
SELECT rs.id, rs.template_id, t.title, rs.creator_id, u.display_name,
       rs.attempt_number, rs.submitted_at, t.category,
       (rs.submitted_at < now() - ($1 * interval '1 hour')) AS aging,
       u.approved_template_count,
       (SELECT COUNT(*) FROM review_submissions x
        WHERE x.creator_id = rs.creator_id AND x.status = 'rejected') AS prior_rejections,
       (u.status = 'banned') AS creator_banned
FROM review_submissions rs
JOIN templates t ON t.id = rs.template_id
JOIN users u ON u.id = rs.creator_id
WHERE rs.status = 'pending'
  AND ($2 = '' OR t.category = $2)                       -- category 筛选
  AND (NOT $3 OR rs.submitted_at < now() - ($1 * interval '1 hour'))  -- aging_only
  AND (NOT $4 OR rs.attempt_number >= 2)                 -- resubmission_only
ORDER BY (rs.submitted_at < now() - ($1 * interval '1 hour')) DESC,
         rs.submitted_at ASC
LIMIT $5 OFFSET $6;
```

（计数查询共享 WHERE，去掉 LIMIT/OFFSET。）

**指标 SQL：**

```sql
-- open_count + 中位等待（对 pending 时长做 percentile_cont）
SELECT COUNT(*) AS open_count,
       COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY
           EXTRACT(EPOCH FROM (now() - submitted_at)) / 3600.0), 0) AS median_hours
FROM review_submissions WHERE status = 'pending';

-- 今日决定数
SELECT COUNT(*) FROM review_submissions
WHERE status <> 'pending' AND decided_at >= date_trunc('day', now());

-- 按原因的拒绝率（占拒绝的比例）
SELECT reject_reason_code, COUNT(*) AS cnt,
       COUNT(*)::float / NULLIF(SUM(COUNT(*)) OVER (), 0) AS rate
FROM review_submissions
WHERE status = 'rejected' AND reject_reason_code IS NOT NULL
GROUP BY reject_reason_code;
```

**决定事务**（`DecideSubmission`）：

```sql
UPDATE review_submissions
SET status = $2, decided_at = now(), decided_by = $3,
    reject_reason_code = $4, reject_note = $5
WHERE id = $1 AND status = 'pending'
RETURNING template_id, creator_id, attempt_number;
```

零行 → `ErrAlreadyDecided`（服务映射为 `FailedPrecondition`）。服务随后
在同一事务范围内更新 `templates` 与 `users`（见 3.5）。

### 3.4 `PromptService` 设闸钩子

`PromptService` 上的新辅助函数（新增 `ReviewRepo
repository.ReviewRepository` 字段）：

```go
// isGated 返回创作者的下一次发布是否必须进入审核。
// 满足 approved_template_count < N OR review_gated_remaining > 0 即设闸。
func (s *PromptService) isGated(ctx context.Context, userID string) bool
```

设置通过 60 秒缓存读取（`review_approval_threshold`、
`review_regate_count`、`review_aging_hours`）。

**`CreateTemplate`** —— 在 visibility 映射之后、`TemplateRepo.Create`
之前：

```go
moderationStatus := "normal"
var publishedAt sql.NullTime
if visibility == "public" && s.isGated(ctx, userID) {
    moderationStatus = "in_review"
} else if visibility == "public" {
    publishedAt = sql.NullTime{Time: time.Now(), Valid: true}
}
template.ModerationStatus = moderationStatus
template.PublishedAt = publishedAt
```

版本 1 创建成功且 `moderationStatus == "in_review"` 后：创建提交
（attempt 1）并在 > 0 时递减 `review_gated_remaining` —— 两者记录日志
但*在同一请求内*；失败则整个发布报错，被闸模板绝不能没有队列行。

**`UpdateTemplate`** —— 在 visibility 转换处（private→public，或
rejected→public 重新提交）：

```go
wasPublic := template.Visibility == "public"
// ... 既有字段更新 ...
transitioningToPublic := !wasPublic && template.Visibility == "public"
if transitioningToPublic {
    if s.isGated(ctx, userID) {
        template.ModerationStatus = "in_review"
        template.PublishedAt = sql.NullTime{}
    } else {
        template.ModerationStatus = "normal"
        if !template.PublishedAt.Valid {
            template.PublishedAt = sql.NullTime{Time: time.Now(), Valid: true}
        }
    }
}
```

`TemplateRepo.Update` 成功且模板进入 `in_review` 后：
`attempt = latestSubmission.AttemptNumber + 1`（无则 1），插入提交，
> 0 时递减 `review_gated_remaining`。不跨越发布边界的编辑不动
`moderation_status`（FR-3/FR-6：in_review/rejected 保持可编辑；FR-14：
豁免创作者已发布模板的编辑保持在线）。

**`CreatePrompt` 可见性守卫**（既有缺口，此处修复）：获取版本后，经
`TemplateRepo.Get(ctx, version.TemplateID, userID)` 加载其模板，隐藏时
以 `NotFound` 失败 —— 被闸模板的内容不得经 prompt 路径可渲染。

### 3.5 `AdminService` 审核 RPC

`AdminService` 新增 `ReviewRepo repository.ReviewRepository` 字段。七个
RPC 全部以 `requireAdminContext` 开头。

**`ApproveReviewSubmission`**（尽可能单事务；通知与审计按既有模式尽力
而为）：

1. `DecideSubmission(ctx, id, "approved", admin.ID, nil, nil)` →
   未知时 `NotFound`，已决定时 `FailedPrecondition`。
2. 更新模板：`moderation_status = 'normal'`、`published_at = now()`
   （仅当当前为 `in_review` —— 用
   `WHERE id = $1 AND moderation_status = 'in_review'` 守卫）。
3. `IncrementApprovedCount(creatorID)`（FR-7：计入 N）。
4. `writeAudit(ctx, actor, "review_approve", "template", templateID,
   "Approved in pre-publication review")`（≥3 字符，满足 CHECK）。
5. 通知（尽力而为，自我抑制 —— 见 3.6）。

**`RejectReviewSubmission`：**

1. 对照白名单（`spam|plagiarism|harmful|lowQuality|other`）校验
   `reason_code`，缺失/未知 → `InvalidArgument`；`note` ≤ 500 字符 →
   `InvalidArgument`。
2. `DecideSubmission(ctx, id, "rejected", admin.ID, &reasonCode, &note)`。
3. 更新模板：`moderation_status = 'rejected'`（同样的 `in_review`
   守卫）。
4. `writeAudit(ctx, actor, "review_reject", "template", templateID,
   reasonCode + ": " + note)`（FR-10：行为者、时间戳、原因）。
5. 携带原因的通知（尽力而为）。

**`ListReviewQueue` / `GetReviewSubmission` / `GetReviewMetrics`：**
只读；将仓储行映射为 proto 消息；`GetReviewSubmission` 经 `ListChain`
组装链与作者摘要（joined_at、已发布数 = `approved_template_count`、
既往拒绝数）。

**`GetPlatformSettings` / `UpdatePlatformSetting`：** 列表读取全部行；
更新对照已知白名单
（`review_approval_threshold|review_regate_count|review_aging_hours`）
校验 `key`；`value` 对 `review_approval_threshold` 为非负整数（0 =
关闭门控 —— 所有发布直接上线），对其余键为正整数；随后调用
`UpsertSetting`，使设置缓存失效，并
`writeAudit(ctx, actor, "settings_change", "platform", key,
reason)` —— `settings_change` 动作与 `platform` target_type 已包含在
§2 的 CHECK 扩宽中。

**`BanUser` 钩子（FR-2/US-8）：** 既有封禁成功后，设置
`review_gated_remaining = M`（`SetGatedRemaining`），使用户复封后的发布
被重新设闸。尽力而为（记日志、非致命），保持封禁路径在 review 存储故障
时不变。

### 3.6 决定通知

在 `AdminService` 中复用 `createEngagementNotification` 形状（行为者 =
决定的管理员，`ActorDisplayName` = 管理员显示名）：

- 批准 → 类型 `template_approved`。
- 拒绝 → 类型 `template_rejected`。

自我决定边缘情形（管理员审核自己的提交）：跳过并记录警告 —— 与既有
自我抑制及 `no_self_notify` CHECK 兜底一致。失败记日志并吞掉；决定本身
不受影响（NFR-4 由状态界面覆盖，通知存储是投递机制）。

### 3.7 网关（`backend/cmd/server/main.go`）

七条路由，全部位于 `requireAdmin` 之后，遵循既有
`/api/v1/admin/comments/` TrimPrefix 模式：

```go
// GET  /api/v1/admin/review            (查询: category, aging, resubmission_only, page, per_page)
http.HandleFunc("/api/v1/admin/review", func(w, r) { /* 仅精确路径的列表 */ })

// GET  /api/v1/admin/review/metrics
// GET  /api/v1/admin/review/{id}
// POST /api/v1/admin/review/{id}/approve
// POST /api/v1/admin/review/{id}/reject   (JSON 体: reason_code, note)
http.HandleFunc("/api/v1/admin/review/", func(w, r) {
    rest := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/review/")
    parts := strings.Split(rest, "/")
    switch {
    case rest == "metrics" && r.Method == http.MethodGet: // GetReviewMetrics
    case len(parts) == 1 && r.Method == http.MethodGet:   // GetReviewSubmission{Id: parts[0]}
    case len(parts) == 2 && parts[1] == "approve" && r.Method == http.MethodPost:
    case len(parts) == 2 && parts[1] == "reject" && r.Method == http.MethodPost:
    default: 404/405
    }
})

// GET /api/v1/admin/settings   → GetPlatformSettings
// PUT /api/v1/admin/settings   → UpdatePlatformSetting (JSON: key, value, reason)
http.HandleFunc("/api/v1/admin/settings", func(w, r) { ... })
```

查询参数映射与 `ListAdminTemplates` 完全一致（`page`、`per_page` 整数，
默认 1/20，上限 100）。JSON 体解码进请求结构体；`writeError` 按既有
表映射 gRPC 码。

## 4. 前端

### 4.1 路由

```js
// App.js —— /admin 懒加载块内（AdminLayout）：
<Route path="review" element={<ReviewPage />} />          // /admin/review
<Route path="review/:id" element={<ReviewDetailPage />} /> // /admin/review/:id
```

`AdminLayout` navItems 新增
`{ to: '/admin/review', label: t('admin.nav.review') }`（置于 Templates
之后）。

### 4.2 新文件

| 文件 | 用途 |
|---|---|
| `frontend/src/pages/admin/ReviewPage.js` + `.scss` | MetricsBar + QueueFilters + QueueTable；`j`/`k` 行导航。 |
| `frontend/src/pages/admin/ReviewDetailPage.js` + `.scss` | SubmissionDetail + 批准/拒绝操作 + 键盘（`A`、`R`、`→`）；决定后自动前进到下一队列项。 |
| `frontend/src/components/review/ReviewStatusCard.js` + `.scss` | 创作者卡片：状态图标、标题、提交时间、预期窗口、查看/编辑；rejected 变体显示原因 + 重新提交 CTA。 |
| `frontend/src/components/review/PublishReviewModal.js` + `.scss` | 被闸创作者的发布后确认。 |
| `frontend/src/components/review/QueueTable.js` + `.scss` | 行：超时徽章（琥珀 ⏳ + 时长）、标题、作者、上下文统计、"（第 n 次）"重新提交标签、Review 按钮。 |
| `frontend/src/components/review/QueueFilters.js` + `.scss` | 标签页 All / Aging / Resubmissions + 类别选择。 |
| `frontend/src/components/review/MetricsBar.js` + `.scss` | 未决数、中位等待、今日决定数。 |
| `frontend/src/components/review/SubmissionDetail.js` + `.scss` | 内容预览、作者摘要行、重新提交链、操作。 |
| `frontend/src/components/review/RejectDialog.js` + `.scss` | 固定原因单选（必填 —— 选择前 Reject 禁用）、可选备注、确认。 |

### 4.3 修改文件

| 文件 | 变更 |
|---|---|
| `frontend/src/services/adminApi.js` | `getReviewQueue(params)`、`getReviewMetrics()`、`getReviewSubmission(id)`、`approveReviewSubmission(id)`、`rejectReviewSubmission(id, reasonCode, note)`、`getPlatformSettings()`、`updatePlatformSetting(key, value, reason)`。 |
| `frontend/src/pages/Home.js` | 在模板网格上方为自有 `in_review`/`rejected` 模板渲染 `ReviewStatusCard` 列表（来自混合视图私有流 + 状态筛选）。 |
| `frontend/src/pages/TemplateDetails.js` | `handleShare`/`handleUnshare`：当更新响应的 `template.moderation_status === 'in_review'` 时，展示 `PublishReviewModal` 而非上线 toast。rejected 模板的所有者横幅（原因 + "修改并重新提交"）。 |
| `frontend/src/components/CreateTemplateModal.js` | `handleSubmit`：当创建响应携带 `moderation_status === 'in_review'` 时，在 `onSuccess()` 前展示 `PublishReviewModal`。 |
| `frontend/src/components/PromptCard.js` | 自有 `in_review`/`rejected` 模板的 moderation 徽章，紧邻 visibility 徽章。 |
| `frontend/src/components/notifications/NotificationItem.js` | `EVENT_ICONS` += `template_approved`/`template_rejected` 条目；句式特例为 `review.notify.approved`/`review.notify.rejected`（无行为者）。 |
| `frontend/src/components/admin/AdminLayout.js` | Review 导航项。 |
| `frontend/src/App.js` | Review 路由。 |
| `frontend/src/locales/{en,zh}/translation.json` | `review.*` 键 + `admin.nav.review` + `admin.review.*` 控制台键。 |

### 4.4 Redux

无新 slice：队列状态为页面局部（经 `adminApi` 的 `useState`/`useEffect`），
与既有管理控制台页面一致；创作者状态随模板载荷到达（Template 消息上的
`moderation_status`）。

### 4.5 i18n 键（EN/ZH 对齐）

`review.*` 逐字取自 UI/UX §8：

```json
{
  "review": {
    "status": { "inReview": "In review", "rejected": "Rejected", "approved": "Live now" },
    "card": { "submitted": "Submitted {{time}}", "expected": "Expected decision within 24 hours" },
    "modal": {
      "title": "Submitted for review",
      "body": "Your template will go live after a quick review — usually within 24 hours. You'll get a notification either way."
    },
    "queue": {
      "title": "Review queue", "open": "Open: {{count}}", "median": "Median wait: {{duration}}",
      "today": "Today: {{count}} decisions", "aging": "Aging >48h", "resubmissions": "Resubmissions",
      "empty": "Queue is clear"
    },
    "row": { "resubmission": "({{n}}th)" },
    "detail": {
      "author": "Joined {{date}} · {{published}} published · {{rejections}} prior rejections",
      "chain": "Resubmission history"
    },
    "action": { "approve": "Approve", "reject": "Reject…" },
    "reject": {
      "title": "Reject template", "reasonRequired": "Pick a reason to continue",
      "note": "Note to creator (optional)",
      "reason": {
        "spam": "Spam", "plagiarism": "Plagiarism", "harmful": "Harmful content",
        "lowQuality": "Low quality", "other": "Other"
      }
    },
    "notify": {
      "approved": "Your template \"{{title}}\" is live",
      "rejected": "Your template \"{{title}}\" was rejected: {{reason}}"
    },
    "cta": { "resubmit": "Fix and resubmit" },
    "notFound": "This page doesn't exist"
  },
  "admin": { "nav": { "review": "Review" } }
}
```

ZH 按键镜像，与 UI/UX §8 规定完全一致（审核中 / 已拒绝 / 已上线 /
提交于 {{time}} / 预计 24 小时内出结果 / 已提交审核 / … / 队列已清空 /
（第 {{n}} 次）/ 注册于 … / 重新提交历史 / 通过 / 拒绝… / 拒绝模板 /
请选择原因后继续 / 给创作者的备注（可选）/ 垃圾内容 / 抄袭 / 有害内容 /
质量低下 / 其他 / 你的模板"{{title}}"已上线 /
你的模板"{{title}}"被拒绝：{{reason}} / 修改并重新提交 / 页面不存在 /
`admin.nav.review` = "审核"）。其余控制台键（设置表单的
`admin.review.settings.*`、超时徽章标签）遵循同样的 EN/ZH 对齐规则。

## 5. 错误矩阵

| 场景 | HTTP | gRPC | 前端处理 |
|---|---|---|---|
| 无 reason_code 拒绝 | 400 | `INVALID_ARGUMENT` | Reject 按钮预禁用；竞态时行内错误 |
| 未知原因码 / 备注 >500 | 400 | `INVALID_ARGUMENT` | 行内错误；对话框保持打开 |
| 非法设置键/值 | 400 | `INVALID_ARGUMENT` | 设置表单字段错误 |
| 非管理员调用任何 review/settings 路由 | 401/403 | `UNAUTHENTICATED`/`PERMISSION_DENIED` | 重定向管理员登录 |
| 提交 id 未知 | 404 | `NOT_FOUND` | 未找到页；返回队列 |
| 提交已有决定 | 409 | `FAILED_PRECONDITION` | toast"已有决定"；刷新队列 |
| 非所有者访问被闸模板（详情/fork/评论/prompt） | 404 | `NOT_FOUND` | 标准未找到页（不泄露状态） |
| 通知插入失败 | — | — | 服务端记日志；决定不受影响 |

## 6. 部署

- DDL 增量/幂等；两份 init.sql 在同一提交中逐字节一致；CHECK 扩宽由
  `pg_get_constraintdef` 守卫；回填由列存在性检查守卫。
- 无新服务、端口或基础设施。无环境变量 —— 阈值存于
  `platform_settings`。
- Helm chart：无模板变更（init.sql 经既有 ConfigMap 到达 chart）；
  `values.yaml` 不动。
- 绝不提交 `backend/server`（被跟踪的构建产物）。

## 7. SDK

`python-sdk/openprompts` 无变更：被闸模板绝不向非所有者返回
（`list_templates`/`get_template` 由仓储过滤），且
`get_prompt_by_alias` 在 `CreatePrompt`/alias 可见性守卫落地后对隐藏
模板 404。管理审核面是 web 控制台特性；SDK 仅封装公开模板 API。

## 8. 测试策略

### 8.1 FVT（`tests/fvt.py`）—— `test_pre_publication_review`

用户：`newbie`（0 已批准）、`established`（先创建 3 个公开模板 —— 即时
发布）、`second`（观察者）。管理员经 `ADMIN_BOOTSTRAP_*`（POST
`/admin/login` → admin headers）。

1. **AC-1/AC-2：** newbie 发布 → 响应 `moderation_status ==
   "in_review"`、`published_at` 为空；established 发布 →
   `moderation_status == "normal"`、`published_at` 已设，立即出现在公开
   列表。
2. **AC-9：** newbie 的模板：匿名 GET → 404；second 用户 GET → 404；
   公开列表/搜索（关键词 = 标题）→ 缺席；newbie GET → 200 且
   `in_review`。
3. **FR-8/AC-8：** 管理员 GET `/admin/review` → 含 newbie 的提交；
   最旧优先排序；创建两个顺序提交并断言相对顺序；新条目 aging 为
   false。
4. **AC-3：** 批准 → 模板 `moderation_status == "normal"`、
   `published_at` ≈ 现在，匿名可见；newbie 通知列表含
   `template_approved`；newbie 的下一次发布仍被闸（已批准数 1 < 3）。
5. **AC-5：** 无 `reason_code` 拒绝 → 400；UI 层面由 e2e 覆盖。
6. **AC-4：** 以 `spam` 拒绝 → 模板 `rejected`，second 用户 404；
   newbie 通知含带原因的 `template_rejected`；所有者 GET → 200 且
   `rejected`。
7. **AC-6：** newbie 编辑并重新发布（更新 visibility 为 public）→ 新
   提交 `attempt_number == 2`；队列行 `is_resubmission`；详情显示带
   此前原因的链。
8. **AC-10：** `/admin/review/metrics` → open_count 与 pending 一致；
   decisions_today ≥ 2；rejection_rate_by_reason 含 spam。
9. **FR-2/AC-7：** 管理员封禁有公开模板的创作者 → 解封 → 下一次发布
   被闸（经 BanUser 钩子的 `review_gated_remaining` 路径）。
10. **NFR-5：** GET `/admin/settings` → 三个键；PUT
    `review_approval_threshold` = 1（带原因）→ 后续 newbie 发布
    （已批准数 1）即时上线；PUT 回 3。
11. **审计：** `/admin/audit-log?action=review_approve` 与
    `action=review_reject` 含带行为者 + 原因的决定。

### 8.2 单元测试

- `isGated` 矩阵：阈值边界（N-1 设闸、N 豁免）、重新设闸计数递减、
  两条件 OR。
- 仓储：队列排序（超时优先后 FIFO）、筛选
  （category/aging/resubmission）、`DecideSubmission` 并发（第二次
  decide → `ErrAlreadyDecided`）、指标数学（中位数、比例份额）。
- 服务：原因码白名单、备注长度、设置键/值校验、`FailedPrecondition`
  映射、通知自我抑制。
- 更新 mock：`MockTemplateRepository`（新列）、`MockReviewRepository`
  （新接口）。

### 8.3 E2E（test-agent，Nightwatch）

- 队列页：指标条渲染、标签页筛选、类别选择、行骨架/空/错误状态、
  `j`/`k` 导航。
- 详情：内容预览、作者摘要行、链条目、`A` 批准带 toast + 自动前进、
  `R` 打开对话框。
- RejectDialog：选择单选前禁用；备注可选；提交后前进。
- 创作者：Home 状态卡片（in_review + rejected 带原因变体）、被闸发布
  后的弹窗、TemplateDetails 所有者横幅、PromptCard 徽章。
- 通知：approved/rejected 行以新图标和无行为者句式渲染；深链到模板。
- 直达 URL：非管理员打开 in_review URL 看到未找到页。
- 所有 `review.*` 与 `admin.nav.review` 键的 i18n EN/ZH 对齐。

## 9. 实现顺序

1. DDL（两份 init.sql）+ 模型 + `ReviewRepository` + 单元测试。
2. Proto 变更 + 重新生成 + `templateModelToProto` 映射 + 仓储
   SELECT/INSERT/UPDATE 列。
3. 可见性过滤：`Get` 规则 + `buildTemplateConditions` moderation 过滤 +
   `CreatePrompt` 守卫。
4. `CreateTemplate`/`UpdateTemplate` 设闸钩子 + 设置缓存。
5. 管理审核 RPC + `BanUser` 钩子 + 通知 + 审计。
6. 网关路由。
7. FVT `test_pre_publication_review`。
8. 前端管理端（路由、页面、组件、adminApi）。
9. 前端创作者端（Home 卡片、弹窗、横幅、徽章、NotificationItem）+
   i18n en/zh。
10. 组件测试 + 移交 test-agent 做 e2e。

## 10. 给开发者的约束

1. **绝不提交 `backend/server`** —— 被跟踪的构建产物；若被修改执行
   `git checkout -- backend/server`。
2. 只暂存自己的文件；绝不 `git add -A`（树中持有其他 agent 的 WIP）。
3. 两份 init.sql 在同一提交中逐字节一致；每个 DDL 块幂等（列存在性
   守卫、CHECK 扩宽用 `pg_get_constraintdef`、设置种子用
   `ON CONFLICT DO NOTHING`）。
4. **`Template` proto 字段号仅 25/26。** 18–24 由 follow-creators
   （18/19）、template-ratings（20–22）、prompt-analytics（23/24）
   预留 —— 绝不复用。不动 `AdminTemplateSummary`（13/14 由
   ratings/import-export 争议）与 `GetTemplateResponse`（3/4 已被
   占用）。
5. 隐藏模板语义：非所有者在所有读路径上 **404 而非 403** —— 详情、
   列表、搜索、fork、评论、prompt、alias。任何地方不泄露状态。
6. moderation 过滤必须精确镜像封禁所有者模式：公开流条件
   `AND (t.moderation_status = 'normal' OR t.owner_id =
   $current_user_id)`；私有流不动。
7. 被闸发布必须与其提交行原子：提交插入失败则发布失败 —— 没有队列行
   的被闸模板是 bug。
8. 决定尽可能事务化；通知与审计写入按既有模式尽力而为（记日志、非
   致命）。
9. `review_gated_remaining` 仅在 > 0 且被闸发布时递减；
   `approved_template_count` 仅在批准 pending 提交时递增（幂等性由
   `WHERE status='pending'` 的 decide 守卫）。
10. 原因码是服务端白名单 —— 绝不信任客户端字符串；设置键同理。
11. 所有计数用 int32；`published_at` 为 `sql.NullTime`（首次批准前
    NULL）。
12. 任何形式的商业化界面都不做。
