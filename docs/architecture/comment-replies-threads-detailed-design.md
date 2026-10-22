# Comment Replies & Threads — Detailed Design

| | |
|---|---|
| **Feature ID** | comment-replies-threads |
| **Version** | 1.0 |
| **Status** | Approved for implementation |
| **Author** | Architect Agent |
| **Upstream** | [Architecture](comment-replies-threads-architecture.md) |

## 1. Proto Contract

### 1.1 `backend/api/proto/v1/prompt.proto`

**`Comment` message** — append two fields (next free tags 10, 11):

```protobuf
message Comment {
  string id = 1;
  string template_id = 2;
  string content = 3;
  google.protobuf.Timestamp created_at = 4;
  google.protobuf.Timestamp updated_at = 5;
  bool is_edited = 6;
  CommentAuthor author = 7;
  bool is_owner = 8;
  bool is_own = 9;
  // Number of direct replies (top-level comments only; 0 for replies).
  int32 reply_count = 10;
  // True when the template owner has replied in this thread (top-level only).
  bool has_creator_reply = 11;
}
```

**`CreateCommentRequest`** — append one field:

```protobuf
message CreateCommentRequest {
  string template_id = 1;
  string content = 2;   // 1-2000 chars after trim
  // Optional. When set, must reference an existing top-level comment on the
  // same template; replying to a reply is flattened onto the top-level thread.
  string parent_comment_id = 3;
}
```

**New RPC** on `PromptService` (after `DeleteComment`):

```protobuf
// ListCommentReplies lists all replies to a top-level comment, oldest-first,
// unpaginated (bounded by the 50-reply thread cap).
rpc ListCommentReplies(ListCommentRepliesRequest) returns (ListCommentRepliesResponse);
```

New messages:

```protobuf
message ListCommentRepliesRequest {
  string comment_id = 1;  // must be a top-level comment
}

message ListCommentRepliesResponse {
  repeated Comment replies = 1;  // oldest-first
}
```

### 1.2 `backend/api/proto/v1/notification.proto`

**`Notification` message** — append one field (next free tag 10):

```protobuf
message Notification {
  string id = 1;
  string type = 2;               // "like" | "favorite" | "fork" | "comment" | ...
  string actor_id = 3;
  string actor_display_name = 4;
  string actor_avatar = 5;
  string template_id = 6;
  string template_title = 7;
  string read_at = 8;
  string created_at = 9;
  // ≤100-char excerpt; only populated for comment_reply events.
  string snippet = 10;
}
```

### 1.3 Regeneration

```bash
cd backend && make proto
# equivalent to:
protoc -I api/proto/v1 --go_out=. --go-grpc_out=. \
  api/proto/v1/prompt.proto api/proto/v1/notification.proto
```

## 2. Database DDL

Append to **both** `deployment/init.sql` and
`deployment/chart/open-prompts/init.sql` (byte-identical block):

```sql
-- -----------------------------------------------------------------------------
-- Feature: comment-replies-threads
-- Description: Single-level threading on template_comments. reply_count is
--              maintained transactionally on reply insert/delete; comment_count
--              counts top-level comments only (unchanged semantics).
-- -----------------------------------------------------------------------------

-- parent_comment_id: null for top-level comments; references a top-level
-- comment. No FK (cascade handled in the service layer, matching the shipped
-- delete flow). reply_count = number of direct replies.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='template_comments' AND column_name='parent_comment_id') THEN
        ALTER TABLE template_comments ADD COLUMN parent_comment_id UUID;
        COMMENT ON COLUMN template_comments.parent_comment_id IS
            'Null for top-level comments; references a top-level comment (no FK, service-enforced)';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='template_comments' AND column_name='reply_count') THEN
        ALTER TABLE template_comments ADD COLUMN reply_count INT NOT NULL DEFAULT 0;
        COMMENT ON COLUMN template_comments.reply_count IS
            'Number of direct replies (maintained transactionally)';
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_template_comments_parent_created
    ON template_comments (parent_comment_id, created_at ASC);

-- Widen notifications.type CHECK for the comment_reply event type (guarded,
-- idempotent; converges on the next run if a partially-migrated DB lacks it).
DO $$
DECLARE
    def TEXT;
BEGIN
    SELECT pg_get_constraintdef(oid) INTO def
    FROM pg_constraint
    WHERE conrelid = 'notifications'::regclass AND conname = 'notifications_type_check';
    IF def IS NOT NULL AND def NOT LIKE '%comment_reply%' THEN
        ALTER TABLE notifications DROP CONSTRAINT notifications_type_check;
        ALTER TABLE notifications ADD CONSTRAINT notifications_type_check
            CHECK (type IN ('like', 'favorite', 'fork', 'comment',
                            'template_approved', 'template_rejected',
                            'template_warning_changed', 'follow',
                            'appeal_resolved', 'comment_reply'));
    END IF;
END $$;

-- Add the snippet column to notifications (≤100-char reply excerpt).
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='notifications' AND column_name='snippet') THEN
        ALTER TABLE notifications ADD COLUMN snippet TEXT;
        COMMENT ON COLUMN notifications.snippet IS
            '≤100-char excerpt; only populated for comment_reply events';
    END IF;
END $$;
```

Notes:

- The `parent_comment_id`/`reply_count`/`snippet` column additions are guarded
  on `information_schema.columns` so they are idempotent and additive.
- The notifications CHECK widening follows the existing `pg_constraint`
  guard pattern; it converges on the next run if a partially-migrated database
  lacks `comment_reply`.
- Existing flat comments (all rows have null `parent_comment_id`, `reply_count`
  = 0) render exactly as today — no backfill needed (NFR-7).

## 3. Backend Layers

### 3.1 Model (`backend/internal/models/comment.go`)

```go
type Comment struct {
    ID         string    `json:"id"`
    TemplateID string    `json:"template_id"`
    UserID     string    `json:"user_id"`
    Content    string    `json:"content"`
    CreatedAt  time.Time `json:"created_at"`
    UpdatedAt  time.Time `json:"updated_at"`

    // New (threading):
    ParentCommentID string `json:"parent_comment_id"` // "" for top-level
    ReplyCount      int32  `json:"reply_count"`        // direct replies
    HasCreatorReply bool   `json:"has_creator_reply"`  // top-level only

    // Joined at read time (public identity only).
    AuthorDisplayName string `json:"author_display_name"`
    AuthorAvatar      string `json:"author_avatar"`

    // Computed in the service layer.
    IsEdited bool `json:"is_edited"`
}
```

`models.Notification` gains `Snippet string \`json:"snippet"\``.

### 3.2 Repository (`backend/internal/repository/comment_repository.go`)

```go
type CommentRepository interface {
    // List returns a page of TOP-LEVEL comments (newest first) plus the
    // template's total top-level comment count. Each comment carries
    // ReplyCount and HasCreatorReply (computed via subquery, no N+1).
    List(ctx context.Context, templateID string, limit, offset int) ([]*models.Comment, int64, error)
    // ListReplies returns all replies to a top-level comment, oldest-first.
    ListReplies(ctx context.Context, parentID string) ([]*models.Comment, error)
    // Get loads a single comment by id.
    Get(ctx context.Context, id string) (*models.Comment, error)
    // Create inserts a comment (top-level or reply) and maintains the
    // appropriate counter in one transaction: comment_count for top-level,
    // parent.reply_count for replies.
    Create(ctx context.Context, c *models.Comment) error
    // Update sets content and updated_at (never created_at).
    Update(ctx context.Context, id, content string) (*models.Comment, error)
    // Delete removes a comment. If it is a top-level comment, its replies are
    // deleted in the same transaction and comment_count is recomputed. If it
    // is a reply, its parent's reply_count is decremented. Returns
    // (deleted, error); deleted=false when the row does not exist.
    Delete(ctx context.Context, id string) (bool, error)
}
```

Key statements:

```go
// List — top-level only, newest first, with reply_count + has_creator_reply
// via correlated subqueries (no N+1; list pages ≤ 50 rows).
const listQuery = `
SELECT c.id, c.template_id, c.user_id, c.content, c.created_at, c.updated_at,
       u.display_name, COALESCE(u.avatar, ''),
       c.reply_count,
       EXISTS (
           SELECT 1 FROM template_comments r
           WHERE r.parent_comment_id = c.id AND r.user_id = t.owner_id
       ) AS has_creator_reply
FROM template_comments c
JOIN users u ON u.id = c.user_id
JOIN templates t ON t.id = c.template_id
WHERE c.template_id = $1 AND c.parent_comment_id IS NULL
ORDER BY c.created_at DESC
LIMIT $2 OFFSET $3`

const countQuery = `
SELECT COUNT(*) FROM template_comments
WHERE template_id = $1 AND parent_comment_id IS NULL`

// ListReplies — oldest-first (conversational order), bounded by the 50 cap.
const listRepliesQuery = `
SELECT c.id, c.template_id, c.user_id, c.content, c.created_at, c.updated_at,
       u.display_name, COALESCE(u.avatar, ''),
       c.reply_count
FROM template_comments c
JOIN users u ON u.id = c.user_id
WHERE c.parent_comment_id = $1
ORDER BY c.created_at ASC`

// Create — transactional insert + counter maintenance.
func (r *commentRepository) Create(ctx context.Context, c *models.Comment) error {
    tx, err := r.db.BeginTx(ctx, nil)
    if err != nil { return err }
    defer func() { _ = tx.Rollback() }()

    err = tx.QueryRowContext(ctx, `
INSERT INTO template_comments (template_id, user_id, content, parent_comment_id)
VALUES ($1, $2, $3, NULLIF($4, ''))
RETURNING id, created_at, updated_at`,
        c.TemplateID, c.UserID, c.Content, c.ParentCommentID).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
    if err != nil { return err }

    if c.ParentCommentID == "" {
        // Top-level: bump templates.comment_count.
        if _, err = tx.ExecContext(ctx,
            `UPDATE templates SET comment_count = comment_count + 1 WHERE id = $1`, c.TemplateID); err != nil {
            return err
        }
    } else {
        // Reply: bump the parent's reply_count.
        if _, err = tx.ExecContext(ctx,
            `UPDATE template_comments SET reply_count = reply_count + 1 WHERE id = $1`, c.ParentCommentID); err != nil {
            return err
        }
    }
    return tx.Commit()
}

// Delete — transactional cascade + counter maintenance.
func (r *commentRepository) Delete(ctx context.Context, id string) (bool, error) {
    tx, err := r.db.BeginTx(ctx, nil)
    if err != nil { return false, err }
    defer func() { _ = tx.Rollback() }()

    var templateID, parentID string
    err = tx.QueryRowContext(ctx,
        `DELETE FROM template_comments WHERE id = $1 RETURNING template_id, COALESCE(parent_comment_id, '')`,
        id).Scan(&templateID, &parentID)
    if err == sql.ErrNoRows { return false, nil }
    if err != nil { return false, err }

    if parentID == "" {
        // Top-level: delete its replies, then recount comment_count.
        if _, err = tx.ExecContext(ctx,
            `DELETE FROM template_comments WHERE parent_comment_id = $1`, id); err != nil {
            return false, err
        }
        var count int64
        if err = tx.QueryRowContext(ctx,
            `SELECT COUNT(*) FROM template_comments WHERE template_id = $1 AND parent_comment_id IS NULL`,
            templateID).Scan(&count); err != nil {
            return false, err
        }
        if _, err = tx.ExecContext(ctx,
            `UPDATE templates SET comment_count = $1 WHERE id = $2`, count, templateID); err != nil {
            return false, err
        }
    } else {
        // Reply: decrement the parent's reply_count (floor at 0).
        if _, err = tx.ExecContext(ctx,
            `UPDATE template_comments SET reply_count = GREATEST(reply_count - 1, 0) WHERE id = $1`,
            parentID); err != nil {
            return false, err
        }
    }
    return true, tx.Commit()
}
```

`adminRepository.ListComments`/`DeleteComment` gain the parent context: the
list query joins the parent author's display name (for the `parent` column),
and `DeleteComment` follows the same cascade semantics as `CommentRepository.Delete`.

### 3.3 Service (`backend/internal/service/comment.go`)

**`CreateComment`** — add `parent_comment_id` handling after the template
visibility check and before the rate-limit check:

```go
const commentThreadCap = 50

// Resolve the parent (if any). Must be a top-level comment on the same template.
parentID := strings.TrimSpace(req.ParentCommentId)
if parentID != "" {
    parent, err := s.CommentRepo.Get(ctx, parentID)
    if err == sql.ErrNoRows {
        return nil, status.Error(codes.InvalidArgument, "parent comment not found")
    }
    if err != nil {
        return nil, status.Error(codes.Internal, "failed to load parent comment")
    }
    if parent.TemplateID != tpl.ID {
        return nil, status.Error(codes.InvalidArgument, "parent comment is on a different template")
    }
    if parent.ParentCommentID != "" {
        return nil, status.Error(codes.InvalidArgument, "parent must be a top-level comment")
    }
    if parent.ReplyCount >= commentThreadCap {
        return nil, status.Error(codes.InvalidArgument, "thread is full — post a new comment instead")
    }
}
```

Then set `c.ParentCommentID = parentID` on the model before `Create`. The
`comment_reply` notification is emitted after the write commits, only when
`parentID != ""` and the parent's author differs from the replier:

```go
if parentID != "" {
    // Notify the parent comment's author (not the template owner).
    s.notifyCommentReply(ctx, tpl, parentID, userID, content)
}
```

**`ListComments`** — unchanged shape, but the repository now returns top-level
only with `reply_count`/`has_creator_reply`; `commentModelToProto` maps the two
new fields.

**New `ListCommentReplies`**:

```go
func (s *PromptService) ListCommentReplies(ctx context.Context, req *pb.ListCommentRepliesRequest) (*pb.ListCommentRepliesResponse, error) {
    userID, _ := GetUserIDFromContext(ctx) // optional auth — visibility follows the template

    parent, err := s.CommentRepo.Get(ctx, req.CommentId)
    if err == sql.ErrNoRows {
        return nil, status.Error(codes.NotFound, "comment not found")
    }
    if err != nil {
        return nil, status.Error(codes.Internal, "failed to load comment")
    }
    if parent.ParentCommentID != "" {
        return nil, status.Error(codes.InvalidArgument, "replies are only available for top-level comments")
    }

    tpl, err := s.TemplateRepo.Get(ctx, parent.TemplateID, userID)
    if err != nil {
        return nil, status.Error(codes.NotFound, "template not found")
    }
    if tpl.Visibility == "private" && tpl.OwnerID != userID {
        return nil, status.Error(codes.NotFound, "template not found") // indistinguishable
    }

    replies, err := s.CommentRepo.ListReplies(ctx, parent.ID)
    if err != nil {
        return nil, status.Error(codes.Internal, "failed to list replies")
    }
    resp := &pb.ListCommentRepliesResponse{Replies: make([]*pb.Comment, 0, len(replies))}
    for _, r := range replies {
        resp.Replies = append(resp.Replies, s.commentModelToProto(r, tpl, userID))
    }
    return resp, nil
}
```

**`DeleteComment`** — unchanged authorization (author or template owner); the
repository now cascades replies and maintains both counters.

**`commentModelToProto`** — map the two new fields:

```go
ReplyCount:      c.ReplyCount,
HasCreatorReply: c.HasCreatorReply,
```

**`notifyCommentReply`** — best-effort, mirrors `createEngagementNotification`
but targets the parent comment's author and carries a snippet:

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
        return // replying to your own comment sends no notification
    }
    actor, err := s.UserRepo.GetByID(ctx, actorID)
    if err != nil || actor == nil {
        zap.S().Warnf("comment_reply hook: failed to load actor %s: %v", actorID, err)
        return
    }
    displayName := actor.DisplayName
    if displayName == "" {
        displayName = actor.ID
    }
    snippet := content
    if r := []rune(snippet); len(r) > 100 {
        snippet = string(r[:100])
    }
    if err := s.NotificationRepo.Create(ctx, &models.Notification{
        RecipientID:      parent.UserID,
        ActorID:          actorID,
        TemplateID:       tpl.ID,
        Type:             "comment_reply",
        ActorDisplayName: displayName,
        Snippet:          snippet,
    }); err != nil {
        zap.S().Errorf("comment_reply hook: failed to create notification (template=%s parent=%s actor=%s): %v",
            tpl.ID, parentID, actorID, err)
    }
}
```

### 3.4 Notification layer

`notificationRepository.Create` and `List` gain the `snippet` column; the
`notificationListQuery` SELECT adds `n.snippet` and the Scan target maps it to
`models.Notification.Snippet`. `notificationToProto` maps `Snippet` to
`pb.Notification.Snippet`.

### 3.5 Gateway (`backend/cmd/server/main.go`)

- `POST /api/v1/templates/{id}/comments` body struct gains
  `ParentCommentId string \`json:"parent_comment_id"\``, mapped to
  `CreateCommentRequest.ParentCommentId`.
- New branch under the existing `/api/v1/comments/` handler: when the path is
  `/api/v1/comments/{id}/replies` and method is `GET`, call
  `ListCommentReplies`. Route by splitting the path on `/`:
  - `[id]` → PUT/DELETE (existing).
  - `[id, "replies"]` → GET `ListCommentReplies`.

```go
http.HandleFunc("/api/v1/comments/", func(w http.ResponseWriter, r *http.Request) {
    parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/comments/"), "/")
    if len(parts) == 2 && parts[1] == "replies" && r.Method == http.MethodGet {
        // optional auth (visibility follows the template)
        ctx := context.Background()
        if authHeader := r.Header.Get("Authorization"); authHeader != "" {
            tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
            if userID, err := authInterceptor.VerifyAnyToken(r.Context(), tokenStr); err == nil {
                ctx = service.ContextWithUserID(ctx, userID)
            }
        }
        resp, err := svc.ListCommentReplies(ctx, &pb.ListCommentRepliesRequest{CommentId: parts[0]})
        if err != nil { writeError(w, err); return }
        w.Header().Set("Content-Type", "application/json")
        b, _ := marshaler.Marshal(resp)
        _, _ = w.Write(b)
        return
    }
    // ... existing PUT/DELETE handling for parts[0] ...
})
```

- `adminRepository.ListComments` returns the parent author display name; the
  admin gateway response already carries the `AdminCommentEntry` — add a
  `parent_author_display_name` field (see §1.2 admin note below).

### 3.6 Admin proto (`backend/api/proto/v1/admin.proto`)

`AdminCommentEntry` gains one field (next free tag 12):

```protobuf
message AdminCommentEntry {
  // ... existing 1-11 ...
  // Display name of the parent comment's author; empty for top-level comments.
  string parent_author_display_name = 12;
}
```

## 4. Frontend Design

### 4.1 File layout

```
frontend/src/components/comments/
├── CommentsSection.js / .scss   // thread-aware: passes thread props down
├── CommentItem.js / .scss       // + Reply action, bucket toggle, Answered chip
├── CommentComposer.js / .scss   // unchanged (main composer)
├── ReplyComposer.js / .scss     // NEW — compact variant, parent_comment_id
├── RepliesBucket.js / .scss     // NEW — collapsed count, lazy fetch + cache
└── CommentEditForm.js / .scss   // unchanged
frontend/src/pages/admin/CommentsPage.js / .scss   // NEW — parent column
```

### 4.2 State

`CommentsSection` keeps page-scoped local state (no new Redux slice):

```js
const [comments, setComments] = useState([]);       // top-level only
const [openComposerId, setOpenComposerId] = useState(null); // one composer at a time
const [drafts, setDrafts] = useState({});           // per-comment-id draft map
const [expandedThreads, setExpandedThreads] = useState({}); // comment-id -> replies cache
```

`RepliesBucket` holds its own `replies`, `loading`, `error`, and `expanded`
state; the fetched replies are cached in `expandedThreads` so collapse +
re-expand does not refetch (AC-4).

### 4.3 Components

- **CommentsSection** — renders top-level `CommentItem`s; coordinates the
  single open composer (`openComposerId`); passes `onReply`, `onExpandThread`,
  and the cached replies down.
- **CommentItem** — adds a `Reply` action (auth only; anonymous see
  `comments.loginToReply`); renders the `Answered` chip when
  `comment.has_creator_reply`; renders the `RepliesBucket` when
  `comment.reply_count > 0`.
- **ReplyComposer** — compact variant of `CommentComposer`; labeled textarea
  (`aria-label` "Reply to {name}"), char counter, Cancel/Reply buttons; posts
  with `parent_comment_id`; on success auto-expands the bucket and appends the
  reply; on 400 thread-full shows the inline localized message and preserves
  the draft; on 429 shows the existing rate-limit error.
- **RepliesBucket** — collapsed "View {N} replies ▾" button (singular "1
  reply"); on expand, lazily fetches via `listCommentReplies` (or renders from
  cache); oldest-first list of `CommentItem`s (replies never nest further);
  inline error + Retry inside the bucket only.
- **AdminCommentsPage** — mirrors the existing moderation table; adds a
  `parent` column ("—" for top-level, "in reply to {author}" for replies);
  delete flow unchanged (reason modal + audit).

### 4.4 i18n keys (both locales)

```json
{
  "comments": {
    "reply": "Reply",
    "replyTo": "Reply to {{name}}…",
    "loginToReply": "Log in to reply",
    "viewReplies": "View {{count}} replies",
    "viewOneReply": "View 1 reply",
    "hideReplies": "Hide replies",
    "answered": "Answered",
    "threadFull": "This thread is full — post a new comment instead.",
    "repliesError": "Couldn't load replies.",
    "replyPosted": "Reply posted"
  },
  "admin": {
    "comments": {
      "parent": "Parent",
      "inReplyTo": "in reply to {{author}}",
      "topLevel": "—"
    }
  },
  "notifications": {
    "event": {
      "comment_reply": "{{actor}} replied to your comment on \"{{title}}\""
    },
    "snippet": {
      "comment_reply": "\"{{snippet}}\""
    }
  }
}
```

The ZH locale mirrors every key. `notifPrefs.type.comment_reply` /
`notifPrefs.type.comment_reply.desc` belong to the notification-preferences
feature (backlog) and are noted in §7 coordination.

### 4.5 API client additions (`frontend/src/services/api.js`)

```js
export const listCommentReplies = (commentId) =>
  api.get(`/comments/${commentId}/replies`);
// createTemplateComment gains an optional parent_comment_id:
export const createTemplateComment = (templateId, content, parentCommentId) =>
  api.post(`/templates/${templateId}/comments`, {
    content,
    ...(parentCommentId ? { parent_comment_id: parentCommentId } : {}),
  });
```

## 5. Error Matrix

| HTTP | gRPC code | Condition | User-facing message |
|---|---|---|---|
| 400 | `InvalidArgument` | content empty / > 2000 after trim | `comments.error_content_length` (existing) |
| 400 | `InvalidArgument` | `parent_comment_id` unknown / different template / points at a reply | `comments.error_parent_invalid` (generic) |
| 400 | `InvalidArgument` | thread full (51st reply) | `comments.threadFull` |
| 400 | `InvalidArgument` | `ListCommentReplies` on a reply (not top-level) | generic 400 |
| 401 | `Unauthenticated` | missing/invalid token on POST/PUT/DELETE | existing auth toasts |
| 403 | `PermissionDenied` | banned user posting | `comments.error_banned` (existing) |
| 403 | `PermissionDenied` | non-author edit / non-author-non-owner delete | existing generic |
| 403 | `PermissionDenied` | non-admin on admin routes | existing admin 403 |
| 404 | `NotFound` | unknown template/comment; private template as non-owner | `comments.error_load` / generic |
| 429 | `ResourceExhausted` | rate limit (5/min or 50/day) on replies | `comments.error_rate_limited` + `Retry-After` |
| 500 | `Internal` | DB/Redis failure | generic error toast |

## 6. Deployment

- Append the §2 DDL block to both `init.sql` copies (byte-identical,
  idempotent, additive).
- Rebuild backend (`protoc` + `go build`) and frontend as usual. No new
  services, ports, volumes, or migrations tooling. No new env vars.
- **Coordination with notification-preferences** (backlog): when that feature
  ships, its `notification_preferences` table must include
  `comment_reply_enabled BOOLEAN NOT NULL DEFAULT true` and a
  `notifPrefs.type.comment_reply` UI row. Until then, `comment_reply` events
  are always emitted.

## 7. SDK / CLI

`python-sdk` gains pass-through helpers (comments are template-scoped):

```python
def list_comment_replies(self, comment_id): ...
def create_comment(self, template_id, content, parent_comment_id=None): ...
```

`create_comment` passes `parent_comment_id` through to the REST body when set.
No CLI surface in v1.

## 8. Test Strategy

### 8.1 FVT (`tests/fvt.py`) — extend `test_template_comments()`

1. Author posts a top-level comment; owner replies → 200; reply appears in
   `GET /comments/{id}/replies` oldest-first; parent `reply_count` = 1;
   `templates.comment_count` unchanged.
2. Replying to a reply → 400 (parent must be top-level).
3. `parent_comment_id` on a different template → 400.
4. Thread cap: post 50 replies, the 51st → 400 with the thread-full message.
5. `has_creator_reply` true when the owner has replied; false otherwise.
6. `comment_reply` notification: parent author receives it with replier name +
   ≤100-char snippet; replying to your own comment sends none; creator replying
   on their own template sends none.
7. Delete a reply → `reply_count` −1, siblings intact, `comment_count`
   unchanged.
8. Delete a top-level comment → its replies gone (cascade), `comment_count` −1.
9. Admin list shows the parent column; admin delete of a reply / top-level
   comment follows cascade + audit.
10. Rate limit: 6th reply within a minute → 429 + `Retry-After`.
11. Anonymous reads threads on a public template; private template threads
    owner-only (404 for others).

### 8.2 Backend unit tests (≥ 80 % on new files)

- `parent_comment_id` validation (unknown, different template, points at a
  reply, thread cap boundary 50/51).
- Cascade delete transaction (top-level removes replies; reply decrements
  parent).
- Counter maintenance (comment_count vs reply_count) under concurrent inserts.
- `has_creator_reply` subquery correctness.
- Notification snippet truncation (≤100 chars, rune-safe).
- Permission matrix (author/owner/other/banned × reply/edit/delete).
- `MockCommentRepository` updated for the new methods/fields.

### 8.3 E2E (Nightwatch, test-agent)

- Reply flow on `/templates/:id`; bucket expand/collapse + lazy fetch + cache;
  Answered chip; admin parent column; EN/ZH string parity; keyboard-only
  traversal of Reply, bucket toggle, and composer.

## 9. Implementation Order (suggested)

1. DDL (both `init.sql` copies) + models (`Comment` fields, `Notification.Snippet`).
2. Proto changes (`prompt.proto`, `notification.proto`, `admin.proto`) + regeneration.
3. `CommentRepository` (thread-aware list/create/delete) + admin repo parent column.
4. `PromptService` (`CreateComment` parent validation, `ListCommentReplies`,
   `notifyCommentReply`) + gateway routes.
5. Notification layer (`snippet` column in repo + proto mapping).
6. Frontend: `ReplyComposer`, `RepliesBucket`, `CommentItem`/`CommentsSection`
   extensions, admin `CommentsPage` parent column; i18n both locales.
7. FVT + unit tests; python-sdk helpers.

## 10. Constraints for the Developer (must respect)

1. **Never commit changes to `backend/server`** (tracked 26 MB binary) —
   `git checkout -- backend/server` before committing if it shows modified.
2. Layering: proto → gateway (`main.go`) → service → repository → models; no
   new top-level packages.
3. Regular-user routes under `/api/v1/templates/{id}/comments`,
   `/api/v1/comments/{id}/replies`, and `/api/v1/comments/{id}`; admin routes
   under `/api/v1/admin/comments*` — never mix surfaces.
4. User ID always from the token (`ContextWithUserID`), never from request
   bodies; ownership checks server-side on every mutation.
5. `parent_comment_id` must reference a **top-level** comment on the **same**
   template; replying to a reply is flattened onto the top-level thread (one
   level of nesting only).
6. `comment_count` counts top-level comments only; `reply_count` counts direct
   replies. Both maintained only inside the create/delete transactions — no
   background reconciliation in v1.
7. Deleting a top-level comment cascades to its replies in the same
   transaction; deleting a reply decrements its parent's `reply_count`.
8. The `comment_reply` notification targets the **parent comment's author**
   (not the template owner); self-replies send no notification; snippet is
   ≤100 chars (rune-safe truncation).
9. Rate limiting fail-open when Redis is unavailable; 429 responses carry
   `Retry-After`. Replies share the same 5/min + 50/day counters as comments.
10. Admin deletions require a 3–500 char reason and write `admin_audit_logs`
    (`action='delete_comment'`, `target_type='comment'`).
11. `init.sql` changes idempotent and mirrored into the helm chart copy
    (byte-identical block).
12. i18n: every new user-visible string in **both** `en` and `zh` locales
    (`comments.*`, `admin.comments.*`, `notifications.*`).
13. Conventional commits, English; ≥ 80 % unit-test coverage on new Go files;
    all CI gates green before push.
14. Do not build the notification-preferences suppression path — that feature
    is a separate backlog item; `comment_reply` events are always emitted in
    this feature.