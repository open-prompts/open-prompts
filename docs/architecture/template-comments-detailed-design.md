# Template Comments — Detailed Design

- **Feature ID:** `template-comments`
- **Version:** 1.0
- **Status:** Approved for implementation
- **Author:** Architect Agent
- **Upstream docs:** [Requirements](../design/template-comments-requirements.md) · [UI/UX Design](../design/template-comments-ui-ux-design.md)
- **Architecture doc:** [Architecture Design (EN)](./template-comments-architecture.md) · [架构设计（中文）](./template-comments-architecture-zh-CN.md)

---

## 1. Proto Contract

### 1.1 `backend/api/proto/v1/prompt.proto` — additions

Four RPCs appended to `service PromptService` (after `ListTemplateVersions`):

```protobuf
// Comment RPCs

// ListComments lists comments for a template (visibility follows the template).
rpc ListComments(ListCommentsRequest) returns (ListCommentsResponse);

// CreateComment posts a comment on a template (auth required).
rpc CreateComment(CreateCommentRequest) returns (Comment);

// UpdateComment edits the author's own comment within the edit window.
rpc UpdateComment(UpdateCommentRequest) returns (Comment);

// DeleteComment deletes a comment (author or template owner).
rpc DeleteComment(DeleteCommentRequest) returns (DeleteCommentResponse);
```

New messages (appended at the end of the file):

```protobuf
// CommentAuthor is the public identity of a comment's author.
message CommentAuthor {
  string id = 1;
  string display_name = 2;
  string avatar = 3;
}

// Comment represents a single comment on a template.
message Comment {
  string id = 1;
  string template_id = 2;
  string content = 3;
  google.protobuf.Timestamp created_at = 4;
  google.protobuf.Timestamp updated_at = 5;
  // True when updated_at > created_at (edited marker).
  bool is_edited = 6;
  CommentAuthor author = 7;
  // True when the comment was authored by the template owner.
  bool is_owner = 8;
  // True when the caller is the author (drives Edit/Delete affordances).
  bool is_own = 9;
}

message ListCommentsRequest {
  string template_id = 1;
  int32 page = 2;       // 1-based, default 1
  int32 page_size = 3;  // default 20, max 50
}

message ListCommentsResponse {
  repeated Comment comments = 1;
  int64 total_count = 2;
  int32 page = 3;
  int32 page_size = 4;
}

message CreateCommentRequest {
  string template_id = 1;
  string content = 2;   // 1-2000 chars after trim
}

message UpdateCommentRequest {
  string comment_id = 1;
  string content = 2;   // 1-2000 chars after trim
}

message DeleteCommentRequest {
  string comment_id = 1;
}

message DeleteCommentResponse {
  bool success = 1;
}
```

`Template` message gains one field (next free tag):

```protobuf
  // Number of comments.
  int32 comment_count = 17;
```

### 1.2 `backend/api/proto/v1/admin.proto` — additions

Two RPCs appended to `service AdminService` (after `ListAuditLogs`):

```protobuf
// ListAdminComments lists all comments platform-wide (moderation).
rpc ListAdminComments(ListAdminCommentsRequest) returns (ListAdminCommentsResponse);

// DeleteAdminComment deletes any comment with a mandatory reason (audited).
rpc DeleteAdminComment(DeleteAdminCommentRequest) returns (DeleteAdminCommentResponse);
```

New messages:

```protobuf
// AdminCommentEntry is a comment with moderation context.
message AdminCommentEntry {
  string id = 1;
  string template_id = 2;
  string template_title = 3;
  string template_owner_id = 4;
  string template_owner_display_name = 5;
  string user_id = 6;            // author id
  string user_display_name = 7;  // author display name
  string user_email = 8;         // author email (moderation only)
  string content = 9;
  google.protobuf.Timestamp created_at = 10;
  google.protobuf.Timestamp updated_at = 11;
}

message ListAdminCommentsRequest {
  string q = 1;          // free-text: matches content
  string template_id = 2;
  string user_id = 3;    // author filter
  int32 page = 4;        // 1-based, default 1
  int32 per_page = 5;    // default 20, max 100
}

message ListAdminCommentsResponse {
  repeated AdminCommentEntry entries = 1;
  int64 total = 2;
  int32 page = 3;
  int32 per_page = 4;
}

message DeleteAdminCommentRequest {
  string comment_id = 1;
  string reason = 2;     // 3-500 chars, required
}

message DeleteAdminCommentResponse {
  bool success = 1;
}
```

### 1.3 Regeneration

```bash
cd backend && make proto
# equivalent to:
protoc -I api/proto/v1 --go_out=. --go-grpc_out=. api/proto/v1/prompt.proto api/proto/v1/admin.proto
```

## 2. Database DDL

Append to **both** `deployment/init.sql` and `deployment/chart/open-prompts/init.sql` (byte-identical block):

```sql
-- -----------------------------------------------------------------------------
-- Feature: template-comments
-- Description: Flat comment threads on templates. comment_count is maintained
--              transactionally on insert/delete (like like_count).
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS template_comments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content TEXT NOT NULL CHECK (char_length(content) BETWEEN 1 AND 2000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE template_comments IS 'Flat comments on templates (no threading)';
COMMENT ON COLUMN template_comments.content IS 'Plain text, 1-2000 chars, rendered client-side with escaping only';

CREATE INDEX IF NOT EXISTS idx_template_comments_template_created
    ON template_comments (template_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_template_comments_user_id ON template_comments (user_id);

-- Denormalized counter (same pattern as like_count / favorite_count).
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='templates' AND column_name='comment_count') THEN
        ALTER TABLE templates ADD COLUMN comment_count INT NOT NULL DEFAULT 0;
        COMMENT ON COLUMN templates.comment_count IS 'Number of comments (maintained transactionally)';
    END IF;
END $$;

-- Widen admin_audit_logs CHECKs for delete_comment / comment targets.
-- CHECK constraints cannot be ALTERed; guard on pg_constraint so the block is
-- idempotent: only drop+re-add when the live definition lacks the new value.
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'admin_audit_logs'::regclass AND conname = 'admin_audit_logs_action_check';
    IF def IS NOT NULL AND def NOT LIKE '%delete_comment%' THEN
        ALTER TABLE admin_audit_logs DROP CONSTRAINT admin_audit_logs_action_check;
        ALTER TABLE admin_audit_logs ADD CONSTRAINT admin_audit_logs_action_check
            CHECK (action IN ('ban', 'unban', 'role_change', 'delete_comment'));
    END IF;
END $$;

DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'admin_audit_logs'::regclass AND conname = 'admin_audit_logs_target_type_check';
    IF def IS NOT NULL AND def NOT LIKE '%comment%' THEN
        ALTER TABLE admin_audit_logs DROP CONSTRAINT admin_audit_logs_target_type_check;
        ALTER TABLE admin_audit_logs ADD CONSTRAINT admin_audit_logs_target_type_check
            CHECK (target_type IN ('user', 'comment'));
    END IF;
END $$;
```

Notes:

- The constraint names are the PostgreSQL defaults for these inline CHECKs (`<table>_<column>_check`); the guard reads the live definition so a partially-migrated database converges on the next run.
- If the notifications feature lands first, its `notifications.type` CHECK must also admit `'comment'` — that widening belongs to the notifications DDL block (see §3.5 coordination note).
- **User deletion recompute (FR-18):** no user-deletion API exists today. Any future feature that deletes users must, in the same transaction, recompute `comment_count` for every template the user commented on (`UPDATE templates t SET comment_count = (SELECT COUNT(*) FROM template_comments c WHERE c.template_id = t.id) WHERE t.id IN (SELECT DISTINCT template_id FROM template_comments WHERE user_id = $1)` executed *before* the cascade, or a post-delete recompute per affected template). Recorded here as a standing obligation.

## 3. Backend Layers

### 3.1 Model (`backend/internal/models/comment.go`)

```go
// Comment represents a row in the "template_comments" table.
type Comment struct {
    ID         string    `json:"id"`
    TemplateID string    `json:"template_id"`
    UserID     string    `json:"user_id"`
    Content    string    `json:"content"`
    CreatedAt  time.Time `json:"created_at"`
    UpdatedAt  time.Time `json:"updated_at"`

    // Joined at read time (public identity only).
    AuthorDisplayName string `json:"author_display_name"`
    AuthorAvatar      string `json:"author_avatar"`

    // Computed in the service layer.
    IsEdited bool `json:"is_edited"`
}

// AdminComment is a comment with moderation context (admin list).
type AdminComment struct {
    ID             string    `json:"id"`
    TemplateID     string    `json:"template_id"`
    TemplateTitle  string    `json:"template_title"`
    TemplateOwnerID string   `json:"template_owner_id"`
    OwnerDisplayName string  `json:"owner_display_name"`
    UserID         string    `json:"user_id"`
    UserDisplayName string   `json:"user_display_name"`
    UserEmail      string    `json:"user_email"`
    Content        string    `json:"content"`
    CreatedAt      time.Time `json:"created_at"`
    UpdatedAt      time.Time `json:"updated_at"`
}
```

`models.Template` gains `CommentCount int32 \`json:"comment_count"\`` (after `FavoriteCount`).

### 3.2 Repository (`backend/internal/repository/comment_repository.go`)

```go
type CommentRepository interface {
    List(ctx context.Context, templateID string, limit, offset int) ([]*models.Comment, int64, error)
    Get(ctx context.Context, id string) (*models.Comment, error)
    Create(ctx context.Context, c *models.Comment) error
    Update(ctx context.Context, id, content string) (*models.Comment, error)
    // Delete removes the comment and recomputes templates.comment_count in one
    // transaction. Returns (deleted, newCount, error); deleted=false when the
    // row does not exist.
    Delete(ctx context.Context, id string) (bool, int64, error)
}
```

Key statements:

```go
// List — author join, newest first, offset pagination with total.
const listQuery = `
SELECT c.id, c.template_id, c.user_id, c.content, c.created_at, c.updated_at,
       u.display_name, COALESCE(u.avatar, '')
FROM template_comments c
JOIN users u ON u.id = c.user_id
WHERE c.template_id = $1
ORDER BY c.created_at DESC
LIMIT $2 OFFSET $3`

const countQuery = `SELECT COUNT(*) FROM template_comments WHERE template_id = $1`

// Create — transactional insert + counter maintenance (ToggleLike pattern).
func (r *commentRepository) Create(ctx context.Context, c *models.Comment) error {
    tx, err := r.db.BeginTx(ctx, nil)
    if err != nil { return err }
    defer func() { _ = tx.Rollback() }()

    err = tx.QueryRowContext(ctx, `
INSERT INTO template_comments (template_id, user_id, content)
VALUES ($1, $2, $3)
RETURNING id, created_at, updated_at`,
        c.TemplateID, c.UserID, c.Content).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
    if err != nil { return err }

    if _, err = tx.ExecContext(ctx, `
UPDATE templates SET comment_count = comment_count + 1 WHERE id = $1`, c.TemplateID); err != nil {
        return err
    }
    return tx.Commit()
}

// Delete — transactional delete + recount (authoritative, avoids drift).
func (r *commentRepository) Delete(ctx context.Context, id string) (bool, int64, error) {
    tx, err := r.db.BeginTx(ctx, nil)
    if err != nil { return false, 0, err }
    defer func() { _ = tx.Rollback() }()

    var templateID string
    err = tx.QueryRowContext(ctx, `DELETE FROM template_comments WHERE id=$1 RETURNING template_id`, id).Scan(&templateID)
    if err == sql.ErrNoRows { return false, 0, nil }
    if err != nil { return false, 0, err }

    var count int64
    if err = tx.QueryRowContext(ctx,
        `SELECT COUNT(*) FROM template_comments WHERE template_id=$1`, templateID).Scan(&count); err != nil {
        return false, 0, err
    }
    if _, err = tx.ExecContext(ctx, `UPDATE templates SET comment_count=$1 WHERE id=$2`, count, templateID); err != nil {
        return false, 0, err
    }
    return true, count, tx.Commit()
}
```

`Update` sets `content` and `updated_at = NOW()` only — never `created_at`, so list position is stable (FR-3).

`templateRepository.Get`/`List` add `t.comment_count` to their SELECT lists and Scan targets; `templateModelToProto` maps it to `comment_count = 17`.

### 3.3 Service (`backend/internal/service/prompt.go`)

`PromptService` gains `CommentRepo repository.CommentRepository` and `Redis RedisStore` (the interface already declared in `user.go`; nil disables rate limiting). `NewPromptService` gains the two parameters — update the single call site in `main.go`.

```go
const (
    commentMaxLen        = 2000
    commentEditWindow    = 15 * time.Minute
    commentRatePerMinDef = 5
    commentRatePerDayDef = 50
)

func (s *PromptService) ListComments(ctx context.Context, req *pb.ListCommentsRequest) (*pb.ListCommentsResponse, error) {
    userID, _ := GetUserIDFromContext(ctx) // optional auth — visibility follows the template
    tpl, err := s.TemplateRepo.Get(ctx, req.TemplateId, userID)
    if err != nil {
        return nil, status.Error(codes.NotFound, "template not found")
    }
    page := int(req.Page); if page < 1 { page = 1 }
    size := int(req.PageSize); if size < 1 { size = 20 }; if size > 50 { size = 50 }

    comments, total, err := s.CommentRepo.List(ctx, tpl.ID, size, (page-1)*size)
    if err != nil { return nil, status.Error(codes.Internal, "failed to list comments") }
    // map to pb.Comment; IsEdited = UpdatedAt.After(CreatedAt);
    // IsOwner = c.UserID == tpl.OwnerID; IsOwn = c.UserID == userID
    ...
}

func (s *PromptService) CreateComment(ctx context.Context, req *pb.CreateCommentRequest) (*pb.Comment, error) {
    userID, err := GetUserIDFromContext(ctx)
    if err != nil { return nil, err }

    content := strings.TrimSpace(req.Content)
    if content == "" || len([]rune(content)) > commentMaxLen {
        return nil, status.Error(codes.InvalidArgument, "content must be 1-2000 characters")
    }

    user, err := s.UserRepo.GetByID(ctx, userID)
    if err != nil || user == nil {
        return nil, status.Error(codes.Unauthenticated, "user not found")
    }
    if user.Status == "banned" {
        return nil, status.Error(codes.PermissionDenied, "banned users cannot comment")
    }

    tpl, err := s.TemplateRepo.Get(ctx, req.TemplateId, userID)
    if err != nil { return nil, status.Error(codes.NotFound, "template not found") }
    if tpl.Visibility == "private" && tpl.OwnerID != userID {
        return nil, status.Error(codes.NotFound, "template not found") // indistinguishable
    }

    if retryAfter, ok := s.checkCommentRate(ctx, userID); !ok {
        return nil, status.Error(codes.ResourceExhausted,
            fmt.Sprintf("rate limit exceeded, retry after %d seconds", retryAfter))
    }

    c := &models.Comment{TemplateID: tpl.ID, UserID: userID, Content: content}
    if err := s.CommentRepo.Create(ctx, c); err != nil {
        return nil, status.Error(codes.Internal, "failed to create comment")
    }

    // FR-17: best-effort owner notification, after the write commits.
    s.createEngagementNotification(ctx, "comment", tpl.ID, tpl.OwnerID, userID)

    return s.commentModelToProto(c, tpl, userID), nil
}
```

**Rate limiting** — fixed-window counters in Redis, admin-throttle pattern (`Get`/`Sscanf`/`Set`; the `RedisStore` interface has no `INCR`/`TTL`). The stored value is `"count:expires_unix"` so remaining seconds are computable for `Retry-After`:

```go
// checkCommentRate enforces COMMENT_RATE_PER_MINUTE and COMMENT_RATE_PER_DAY.
// Returns (retryAfterSeconds, false) when over limit. Fail-open when Redis is
// unavailable (s.Redis == nil or Get/Set errors), matching the admin throttle.
func (s *PromptService) checkCommentRate(ctx context.Context, userID string) (int, bool) {
    if s.Redis == nil { return 0, true }
    limits := []struct{ key, max string; window time.Duration }{
        {"comment_rate_min:" + userID, os.Getenv("COMMENT_RATE_PER_MINUTE"), time.Minute},
        {"comment_rate_day:" + userID, os.Getenv("COMMENT_RATE_PER_DAY"), 24 * time.Hour},
    }
    minRetry := 0
    for _, l := range limits {
        max, _ := strconv.Atoi(l.max)
        if max <= 0 { max = defaultFor(l.window) } // 5 / 50
        v, err := s.Redis.Get(ctx, l.key)
        if err != nil || v == "" {
            _ = s.Redis.Set(ctx, l.key, "1:"+strconv.FormatInt(time.Now().Add(l.window).Unix(), 10), l.window)
            continue
        }
        var count int; var expires int64
        if _, err := fmt.Sscanf(v, "%d:%d", &count, &expires); err != nil {
            _ = s.Redis.Set(ctx, l.key, "1:"+strconv.FormatInt(time.Now().Add(l.window).Unix(), 10), l.window)
            continue
        }
        if time.Now().Unix() >= expires { // window elapsed: reset
            _ = s.Redis.Set(ctx, l.key, "1:"+strconv.FormatInt(time.Now().Add(l.window).Unix(), 10), l.window)
            continue
        }
        if count >= max {
            if r := int(expires - time.Now().Unix()); r > minRetry { minRetry = r }
            continue
        }
        _ = s.Redis.Set(ctx, l.key, fmt.Sprintf("%d:%d", count+1, expires), time.Duration(expires-time.Now().Unix())*time.Second)
    }
    if minRetry > 0 { return minRetry, false }
    return 0, true
}
```

The gateway reads the `ResourceExhausted` message (or a `grpc-web`-style trailer) and sets `Retry-After: <seconds>` (§3.4). Both counters are incremented only when the create actually proceeds; a rejected create never consumes quota.

`UpdateComment`: load comment (`404` if missing) → author check (`403`) → `time.Since(c.CreatedAt) > commentEditWindow` → `403 "editable window expired — delete and repost instead"` → validate content → `CommentRepo.Update`. Banned authors are also rejected (same check as create).

`DeleteComment`: load comment (`404`) → caller is author **or** `tpl.OwnerID == userID` (template loaded via `TemplateRepo.Get`; `404` if the template is gone) → `CommentRepo.Delete`. Un-audited (FR-10).

### 3.4 Admin service (`backend/internal/service/admin.go`)

```go
func (s *AdminService) ListAdminComments(ctx context.Context, req *pb.ListAdminCommentsRequest) (*pb.ListAdminCommentsResponse, error) {
    if _, err := s.requireAdminContext(ctx); err != nil { return nil, err }
    page := int(req.Page); if page < 1 { page = 1 }
    perPage := int(req.PerPage); if perPage < 1 { perPage = 20 }; if perPage > 100 { perPage = 100 }

    entries, total, err := s.AdminRepo.ListComments(ctx, repository.CommentFilters{
        Search: strings.TrimSpace(req.Q), TemplateID: req.TemplateId, UserID: req.UserId,
    }, perPage, (page-1)*perPage)
    ... // map to pb.AdminCommentEntry
}

func (s *AdminService) DeleteAdminComment(ctx context.Context, req *pb.DeleteAdminCommentRequest) (*pb.DeleteAdminCommentResponse, error) {
    admin, err := s.requireAdminContext(ctx)
    if err != nil { return nil, err }
    reason := strings.TrimSpace(req.Reason)
    if len(reason) < 3 || len(reason) > 500 {
        return nil, status.Error(codes.InvalidArgument, "reason must be 3-500 characters")
    }
    deleted, _, err := s.AdminRepo.DeleteComment(ctx, req.CommentId) // tx: delete + recount
    if err != nil { return nil, status.Error(codes.Internal, "failed to delete comment") }
    if !deleted { return nil, status.Error(codes.NotFound, "comment not found") }

    _ = s.AdminRepo.InsertAuditLog(ctx, &models.AdminAuditLog{
        ActorID: admin.ID, ActorEmail: admin.Email,
        Action: "delete_comment", TargetType: "comment",
        TargetID: req.CommentId, Reason: reason,
    }) // audit failure is logged; deletion already committed (matches ban/unban behavior)
    return &pb.DeleteAdminCommentResponse{Success: true}, nil
}
```

`adminRepository.ListComments` joins `templates` + `users` twice (owner, author) for moderation context; `q` matches `c.content ILIKE '%…%'` with `%`/`_` escaped (existing admin search precedent); `template_id`/`user_id` are exact matches. `adminRepository.DeleteComment` reuses the same transactional delete + recount as the user repository (shared helper or duplicated statement — implementer's choice, tests must cover both).

### 3.5 Notification hook coordination

`createEngagementNotification` (prompt.go) is already generic over the type string. Two integration points:

1. **Enum/DDL:** `notification.proto`'s `type` comment and the `notifications` DDL CHECK must include `'comment'`. If notifications lands first, comments adds the widening `DO $$` block to its own DDL (guard on `pg_get_constraintdef` like §2); if comments lands first, the hook call is written but `NotificationRepo == nil` makes it a no-op until notifications wires the field (FR-17 deferral).
2. **Dedupe:** the existing `INSERT … WHERE NOT EXISTS (identical unread row)` applies unchanged — a second comment by the same user on the same template while the first notification is unread creates no duplicate, matching FR-17's "subject to dedupe rules".

### 3.6 Gateway wiring (`backend/cmd/server/main.go`)

User routes — inside the existing `/api/v1/templates/` handler, after the `/like` and `/favorite` branches (same `strings.Split` sub-resource pattern as `/aliases`):

```go
// parts[1] == "comments": GET (public read, optional auth) / POST (auth)
if len(parts) == 2 && parts[1] == "comments" {
    switch r.Method {
    case http.MethodGet:
        // optional auth: userID = "" when no token (like GetTemplate)
        resp, err := svc.ListComments(ctx, &pb.ListCommentsRequest{
            TemplateId: parts[0], Page: pageParam(r), PageSize: sizeParam(r),
        })
        ...
    case http.MethodPost:
        // VerifyAnyToken → ContextWithUserID (like /like)
        ...
    default: http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    }
    return
}
```

New top-level handler for the comment resource:

```go
http.HandleFunc("/api/v1/comments/", func(w http.ResponseWriter, r *http.Request) {
    // PUT (edit) / DELETE (author-or-owner delete); both require auth
    id := strings.TrimPrefix(r.URL.Path, "/api/v1/comments/")
    if id == "" { http.Error(w, "ID required", http.StatusBadRequest); return }
    // VerifyAnyToken → ctx; route by method to UpdateComment / DeleteComment
})
```

Admin routes (after `/api/v1/admin/audit-log`):

```go
http.HandleFunc("/api/v1/admin/comments", func(w http.ResponseWriter, r *http.Request) {
    // GET only; requireAdmin; q/template_id/user_id/page/per_page query params
})

http.HandleFunc("/api/v1/admin/comments/", func(w http.ResponseWriter, r *http.Request) {
    // DELETE only; requireAdmin; body {"reason": "..."}; on ResourceExhausted
    // (never expected here) or 429 paths the gateway sets Retry-After
})
```

**`Retry-After`:** in `writeError`, when the gRPC code is `ResourceExhausted`, parse the trailing seconds from the message (the service formats `"rate limit exceeded, retry after N seconds"`) and set `w.Header().Set("Retry-After", N)` before writing 429. This is a generic improvement — no comments-specific branch in the gateway.

## 4. Frontend Design

### 4.1 File layout

```
frontend/src/components/comments/
├── CommentsSection.js / .scss   // section shell: counter, list, load-more, error/retry
├── CommentComposer.js / .scss   // labeled textarea, char counter, submit
├── CommentItem.js / .scss       // row: author, avatar, time, content, [CREATOR], actions
└── CommentEditForm.js / .scss   // inline edit: textarea + Save/Cancel
frontend/src/pages/admin/CommentsPage.js / .scss   // moderation list
```

### 4.2 State

Local component state in `CommentsSection` (no new Redux slice — page-scoped, like `TemplateDetails`):

```js
const [comments, setComments] = useState([]);
const [page, setPage] = useState(1);
const [totalCount, setTotalCount] = useState(0);
const [pageSize] = useState(20);
const [loading, setLoading] = useState(false);
const [error, setError] = useState(null);   // section-level, never blocks the page
const [draft, setDraft] = useState('');     // preserved on failure (NFR-8)
const [editingId, setEditingId] = useState(null);
```

`TemplateDetails` passes `template.id`, `template.owner_id`, and the auth `user` (for `is_own`) as props. The header counter syncs from `total_count` and from the template's `comment_count` on first load.

### 4.3 Components

- **CommentsSection** — heading `t('comments.title')` + count; renders list, composer (auth only, else a sign-in prompt linking to `/login`), load-more button while `comments.length < totalCount`; section-level error banner + Retry on load failure.
- **CommentComposer** — `<label>` + textarea (`maxLength` 2000), live character counter, disabled while posting; on 429 shows `t('comments.error_rate_limited')` with the returned seconds; on 400 shows the field error and keeps the draft.
- **CommentItem** — avatar, display name, `[CREATOR]` chip when `is_owner`, relative time (`utils/time.js`), `edited` marker when `is_edited`, content in a `white-space: pre-wrap` text node; hover/focus reveals `Edit`/`Delete` when `is_own`, `Delete` when the viewer is the template owner.
- **CommentEditForm** — prefilled textarea, Save/Cancel; on 403 window-expired shows the toast and exits edit mode.
- **AdminCommentsPage** — mirrors `TemplatesPage`: `AdminSearchInput` (q) + template/author filters + `AdminTable` + `Pagination`; delete opens `ReasonModal` (reuse) → `deleteAdminComment(id, reason)`; row links to `/admin/templates/:id`.
- **Admin TemplateDetailPage** — new "Comments" panel: same table (filtered by template), same delete flow.

### 4.4 i18n keys (both locales, `comments.*` + `admin.comments.*`)

```json
{
  "comments": {
    "title": "Comments",
    "count_one": "{{count}} comment",
    "count_other": "{{count}} comments",
    "composer_label": "Add a comment",
    "placeholder": "Share your experience with this template…",
    "post": "Post",
    "posting": "Posting…",
    "cancel": "Cancel",
    "save": "Save",
    "saving": "Saving…",
    "edit": "Edit",
    "delete": "Delete",
    "edited": "edited",
    "creator_badge": "Creator",
    "char_count": "{{count}} / 2000",
    "load_more": "Load more",
    "empty": "No comments yet. Be the first to comment.",
    "sign_in_to_comment": "Sign in to comment",
    "confirm_delete": "Delete this comment?",
    "confirm_delete_owner": "Delete this comment on your template?",
    "error_load": "Failed to load comments",
    "error_post": "Failed to post comment",
    "error_edit": "Failed to edit comment",
    "error_delete": "Failed to delete comment",
    "error_content_length": "Comment must be 1-2000 characters",
    "error_rate_limited": "Too many comments. Try again in {{seconds}}s.",
    "error_window_expired": "The edit window has expired — delete and repost instead.",
    "error_banned": "Your account cannot comment."
  },
  "admin": {
    "nav": { "comments": "Comments" },
    "comments": {
      "title": "Comment Moderation",
      "search_placeholder": "Search comment content…",
      "filter_template": "Template ID",
      "filter_author": "Author ID",
      "col_content": "Comment",
      "col_template": "Template",
      "col_author": "Author",
      "col_created": "Posted",
      "empty": "No comments found",
      "delete_reason_title": "Delete comment",
      "deleted": "Comment deleted"
    }
  }
}
```

The ZH locale mirrors every key (see §4.4 of the ZH detailed design). `admin.nav.comments` slots into the existing `admin.nav` block.

### 4.5 API client additions

`frontend/src/services/api.js`:

```js
export const listTemplateComments = (templateId, params) =>
  api.get(`/templates/${templateId}/comments`, { params });
export const createTemplateComment = (templateId, content) =>
  api.post(`/templates/${templateId}/comments`, { content });
export const updateComment = (commentId, content) =>
  api.put(`/comments/${commentId}`, { content });
export const deleteComment = (commentId) =>
  api.delete(`/comments/${commentId}`);
```

`frontend/src/services/adminApi.js`:

```js
export const getAdminComments = (params) => adminApi.get('/admin/comments', { params });
export const deleteAdminComment = (id, reason) =>
  adminApi.delete(`/admin/comments/${id}`, { data: { reason } });
```

## 5. Error Matrix

| HTTP | gRPC code | Condition | User-facing message |
|---|---|---|---|
| 400 | `InvalidArgument` | content empty / > 2000 after trim | `comments.error_content_length` |
| 400 | `InvalidArgument` | admin reason not 3–500 chars | `admin.common.error_reason_length` (existing) |
| 400 | `InvalidArgument` | `page`/`page_size`/`per_page` non-numeric | generic 400 |
| 401 | `Unauthenticated` | missing/invalid token on POST/PUT/DELETE | existing auth toasts |
| 403 | `PermissionDenied` | banned user posting | `comments.error_banned` |
| 403 | `PermissionDenied` | non-author edit | `comments.error_edit` (generic) |
| 403 | `PermissionDenied` | edit window expired | `comments.error_window_expired` |
| 403 | `PermissionDenied` | non-author/non-owner delete | `comments.error_delete` (generic) |
| 403 | `PermissionDenied` | non-admin on admin routes | existing admin 403 |
| 404 | `NotFound` | unknown template/comment; private template as non-owner | `comments.error_load` / generic |
| 429 | `ResourceExhausted` | rate limit (5/min or 50/day) | `comments.error_rate_limited` + `Retry-After` header |
| 500 | `Internal` | DB/Redis failure | generic error toast |

## 6. Deployment

- Append the §2 DDL block to both `init.sql` copies; existing deployments converge on restart (all statements idempotent).
- `deployment/docker-compose.yml` (backend service `environment:`) and helm `values.yaml` + deployment template:

  ```yaml
  - COMMENT_RATE_PER_MINUTE=5
  - COMMENT_RATE_PER_DAY=50
  ```

  Both optional; defaults compiled in.
- Rebuild backend (`protoc` + `go build`) and frontend as usual. No new services, ports, volumes, or migrations tooling.

## 7. SDK / CLI

`python-sdk` gains pass-through helpers (comments are template-scoped, mirroring `list_template_versions`):

```python
def list_comments(self, template_id, page=1, page_size=20): ...
def create_comment(self, template_id, content): ...
def update_comment(self, comment_id, content): ...
def delete_comment(self, comment_id): ...
```

No CLI surface in v1.

## 8. Test Strategy

### 8.1 FVT (`tests/fvt.py`) — developer

New `test_template_comments()` following the `test_likes_and_favorites()` two-user pattern:

1. Owner creates a public template; user B posts a comment → 200, appears in list, `comment_count` = 1.
2. Anonymous GET list → 200 (public template).
3. User B edits within window → 200, `is_edited` true, position unchanged.
4. User C (non-author) edit/delete → 403.
5. Owner deletes B's comment via `DELETE /api/v1/comments/{id}` → 200, count 0.
6. Banned user (set via admin API) posts → 403.
7. Rate limit: 6th comment within a minute → 429 + `Retry-After` header present.
8. Admin: `GET /api/v1/admin/comments?q=…` filters; `DELETE /api/v1/admin/comments/{id}` without reason → 400; with reason → 200 + audit row (`action=delete_comment`) visible in `/api/v1/admin/audit-log`.
9. Template deletion → comments gone (cascade verified via admin list absence).
10. Private template: non-owner GET comments → 404; owner → 200.

### 8.2 Backend unit tests (≥ 80 % on new files)

- Content validation boundaries (0, 1, 2000, 2001, Unicode, whitespace-only).
- Edit-window boundary (14:59 vs 15:01 with injected clock or short window in tests).
- Permission matrix: author/owner/other × edit/delete.
- Rate-limit window math with a fake `RedisStore` (count:expires parsing, reset, fail-open on error).
- Transactional recount on create/delete (mock DB or integration).
- Admin reason validation; audit insert called with correct fields.
- `MockTemplateRepository` updated for `comment_count`; existing tests keep passing.

### 8.3 E2E (Nightwatch, test-agent)

- Section renders on `/templates/:id` with count; post/edit/delete flows; load-more; owner sees Delete on all rows; admin `/admin/comments` filters + reason modal; EN/ZH string parity; keyboard-only traversal of composer and actions.

## 9. Implementation Order (suggested)

1. DDL (both `init.sql` copies) + models (`Comment`, `Template.CommentCount`).
2. Proto changes + regeneration; `templateModelToProto` maps `comment_count`.
3. `CommentRepository` (+ admin repo methods) with transactional counter maintenance.
4. `PromptService` comment RPCs (validation, permissions, edit window, rate limit) + gateway routes.
5. `AdminService` methods + admin gateway routes + audit.
6. Frontend: `CommentsSection` family on `TemplateDetails`; admin `CommentsPage` + detail panel; i18n both locales.
7. FVT + unit tests; python-sdk helpers.
8. Notifications enum/DDL coordination (§3.5) — whichever feature lands second widens the CHECK.

## 10. Constraints for the Developer (must respect)

1. **Never commit changes to `backend/server`** (tracked 26 MB binary) — `git checkout -- backend/server` before committing if it shows modified.
2. Layering: proto → gateway (`main.go`) → service → repository → models; no new top-level packages.
3. Regular-user routes under `/api/v1/templates/{id}/comments` and `/api/v1/comments/{id}`; admin routes under `/api/v1/admin/comments*` — never mix surfaces.
4. User ID always from the token (`ContextWithUserID`), never from request bodies; ownership checks server-side on every mutation.
5. `comment_count` maintained only inside the create/delete transactions (increment-on-insert, recount-on-delete) — no background reconciliation in v1.
6. Plain text only: never render comment content as HTML; `dangerouslySetInnerHTML` is forbidden on this path.
7. Rate limiting fail-open when Redis is unavailable; 429 responses carry `Retry-After`.
8. Admin deletions require a 3–500 char reason and write `admin_audit_logs` (`action='delete_comment'`, `target_type='comment'`).
9. `init.sql` changes idempotent and mirrored into the helm chart copy (byte-identical block).
10. i18n: every new user-visible string in **both** `en` and `zh` locales (`comments.*`, `admin.comments.*`, `admin.nav.comments`).
11. Conventional commits, English; ≥ 80 % unit-test coverage on new Go files; all CI gates green before push.
12. If notifications has not landed, leave the `createEngagementNotification(ctx, "comment", …)` call in place — it no-ops on a nil repo (FR-17 deferral); do not build a standalone fallback.
