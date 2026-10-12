# Template Collections — Detailed Design

| | |
|---|---|
| **Feature ID** | template-collections |
| **Version** | 1.0 |
| **Status** | Approved |
| **Author** | Architect Agent |
| **Upstream** | [Architecture](template-collections-architecture.md) · [Requirements](../design/template-collections-requirements.md) · [UI/UX Design](../design/template-collections-ui-ux-design.md) |
| **Downstream** | Developer Agent |

## 1. Proto Contract

New file `backend/api/proto/v1/collection.proto`. Package, go_package, and style follow `notification.proto`. All counts are `int32` (protojson serializes `int64` as a JSON string — the frontend and FVT compare counts as numbers). Timestamps are RFC3339 strings, matching existing messages.

```protobuf
syntax = "proto3";

package v1;

option go_package = "backend/api/proto/v1;v1";

// CollectionService manages user-curated, ordered sets of templates.
service CollectionService {
  // CreateCollection creates a collection for the caller (default private).
  rpc CreateCollection(CreateCollectionRequest) returns (Collection);
  // UpdateCollection edits name/description/visibility (owner only).
  rpc UpdateCollection(UpdateCollectionRequest) returns (Collection);
  // DeleteCollection soft-deletes a collection (owner only).
  rpc DeleteCollection(DeleteCollectionRequest) returns (DeleteCollectionResponse);
  // GetCollection returns a collection with its members. Public collections are
  // readable anonymously; private/deleted collections return NotFound for non-owners.
  rpc GetCollection(GetCollectionRequest) returns (Collection);
  // ListMyCollections returns the caller's active collections.
  rpc ListMyCollections(ListMyCollectionsRequest) returns (ListMyCollectionsResponse);
  // ListPublicCollections returns the public explore feed, paginated and sorted.
  rpc ListPublicCollections(ListPublicCollectionsRequest) returns (ListPublicCollectionsResponse);
  // AddTemplateToCollection adds a template (owner only, idempotent).
  rpc AddTemplateToCollection(AddTemplateToCollectionRequest) returns (AddTemplateToCollectionResponse);
  // RemoveTemplateFromCollection removes a template (owner only).
  rpc RemoveTemplateFromCollection(RemoveTemplateFromCollectionRequest) returns (google.protobuf.Empty);
  // ReorderCollectionTemplates applies a full ordered template-id list (owner only).
  rpc ReorderCollectionTemplates(ReorderCollectionTemplatesRequest) returns (google.protobuf.Empty);
  // ListCollectionsContainingTemplate returns the caller's collections that
  // contain a given template (used for picker checkmarks).
  rpc ListCollectionsContainingTemplate(ListCollectionsContainingTemplateRequest) returns (ListCollectionsContainingTemplateResponse);
}

// A collection summary. Used in lists; GetCollection additionally fills members.
message Collection {
  string id = 1;
  string owner_id = 2;
  string name = 3;
  string description = 4;
  string visibility = 5; // "private" | "public"
  int32 template_count = 6;      // read-time count, never denormalized
  string created_at = 7;         // RFC3339
  string updated_at = 8;         // RFC3339
  CollectionOwner owner = 9;     // id + display_name for cards/detail
  repeated CollectionMember members = 10; // filled by GetCollection only
}

message CollectionOwner {
  string id = 1;
  string display_name = 2;
}

// A member entry: the reused Template message plus position and visibility flag.
message CollectionMember {
  Template template = 1;
  int32 position = 2;
  bool is_public = 3; // owner view: flags private / no-longer-public members
}

message CreateCollectionRequest {
  string name = 1;        // 1-80 chars, required
  string description = 2; // 0-500 chars
  string visibility = 3;  // "private" (default) | "public"
}

message UpdateCollectionRequest {
  string id = 1;
  string name = 2;         // optional; validated when non-empty
  string description = 3;  // optional
  string visibility = 4;   // optional
}

message DeleteCollectionRequest {
  string id = 1;
}

message DeleteCollectionResponse {
  string id = 1;
}

message GetCollectionRequest {
  string id = 1;
}

message ListMyCollectionsRequest {}

message ListMyCollectionsResponse {
  repeated Collection collections = 1;
}

message ListPublicCollectionsRequest {
  int32 page_size = 1;   // default 20, max 50
  string page_token = 2; // offset-as-string (existing convention)
  string sort = 3;       // "newest" (default) | "most_templates"
}

message ListPublicCollectionsResponse {
  repeated Collection collections = 1;
  string next_page_token = 2; // empty when no more pages
}

message AddTemplateToCollectionRequest {
  string collection_id = 1;
  string template_id = 2;
}

message AddTemplateToCollectionResponse {
  string collection_id = 1;
  string template_id = 2;
  int32 position = 3;
  int32 template_count = 4; // post-add count
}

message RemoveTemplateFromCollectionRequest {
  string collection_id = 1;
  string template_id = 2;
}

message ReorderCollectionTemplatesRequest {
  string collection_id = 1;
  repeated string template_ids = 2; // full ordered list, no duplicates
}

message ListCollectionsContainingTemplateRequest {
  string template_id = 1;
}

message ListCollectionsContainingTemplateResponse {
  repeated Collection collections = 1; // caller's collections containing the template
}
```

`google.protobuf.Empty` returns follow the existing convention (`DeleteTemplate`, `UnlikeTemplate`).

### JSON wire shapes (gateway, `UseProtoNames: true`, `EmitUnpopulated: true`)

```jsonc
// GET /api/v1/collections/{id} (public collection, anonymous)
{
  "id": "0b6ba7e2-...",
  "owner_id": "user-1",
  "name": "Best coding prompts",
  "description": "Hand-picked",
  "visibility": "public",
  "template_count": 3,
  "created_at": "2025-01-01T00:00:00Z",
  "updated_at": "2025-01-02T00:00:00Z",
  "owner": { "id": "user-1", "display_name": "Ada" },
  "members": [
    { "template": { "id": "t-1", "title": "...", "visibility": "public", "like_count": 5, "...": "..." },
      "position": 0, "is_public": true }
  ]
}
```

## 2. Database DDL

Identical idempotent blocks appended to **both** `deployment/init.sql` and `deployment/chart/open-prompts/init.sql` (see [Architecture §5](template-collections-architecture.md#5-data-model) for the full SQL). Summary:

- `collections` — UUID PK `uuid_generate_v4()`, `owner_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE`, `name`, `description`, `visibility` CHECK `('private','public')` default `'private'`, `status` CHECK `('active','deleted')` default `'active'`, timestamps.
- `collection_templates` — composite PK `(collection_id, template_id)`, `position INT NOT NULL`, `added_at`, FKs `ON DELETE CASCADE` both sides.
- Indexes: `idx_collection_templates_position (collection_id, position)`; partial `idx_collections_owner (owner_id) WHERE status='active'`; partial `idx_collections_public (visibility, created_at DESC) WHERE status='active' AND visibility='public'`.

## 3. Backend Layers

### 3.1 Models (`backend/internal/model/collection.go` — new)

```go
type Collection struct {
    ID          string
    OwnerID     string
    Name        string
    Description string
    Visibility  string // "private" | "public"
    Status      string // "active" | "deleted"
    CreatedAt   time.Time
    UpdatedAt   time.Time
    TemplateCount int32 // computed at read time
}

type CollectionMember struct {
    Template  model.Template
    Position  int32
    IsPublic  bool
}
```

### 3.2 Repository (`backend/internal/repository/collection_repository.go` — new)

Interface pattern follows `CommentRepository` (ctx-first, constructor `NewCollectionRepository(db)`):

```go
type CollectionRepository interface {
    Create(ctx context.Context, c *model.Collection) error
    Update(ctx context.Context, c *model.Collection) error // name/desc/visibility/updated_at
    SoftDelete(ctx context.Context, id string) error       // status='deleted', updated_at=NOW()
    GetByID(ctx context.Context, id string) (*model.Collection, error) // any status; service filters
    CountActiveByOwner(ctx context.Context, ownerID string) (int32, error)
    ListByOwner(ctx context.Context, ownerID string) ([]*model.Collection, error) // active, newest first
    ListPublic(ctx context.Context, offset, limit int32, sort string) ([]*model.Collection, error)
    CountPublic(ctx context.Context) (int32, error)
    AddMember(ctx context.Context, collectionID, templateID string, position int32) error
    RemoveMember(ctx context.Context, collectionID, templateID string) error
    ReorderMembers(ctx context.Context, collectionID string, templateIDs []string) error // tx: delete+insert
    CountMembers(ctx context.Context, collectionID string) (int32, error)
    MaxMemberPosition(ctx context.Context, collectionID string) (int32, error)
    ListMembers(ctx context.Context, collectionID string, ownerView bool) ([]*model.CollectionMember, error)
    ListContainingTemplate(ctx context.Context, ownerID, templateID string) ([]*model.Collection, error)
}
```

Key SQL:

```sql
-- ListPublic, sort=newest (default)
SELECT c.id, c.owner_id, c.name, c.description, c.visibility, c.status,
       c.created_at, c.updated_at,
       (SELECT count(*) FROM collection_templates ct
         WHERE ct.collection_id = c.id) AS template_count
  FROM collections c
 WHERE c.status = 'active' AND c.visibility = 'public'
 ORDER BY c.created_at DESC
 LIMIT $1 OFFSET $2;

-- ListPublic, sort=most_templates
 ... ORDER BY template_count DESC, c.created_at DESC ...

-- ListMembers (owner view: all members, flagged)
SELECT t.id, t.owner_id, t.title, t.description, t.visibility, t.type, t.tags,
       t.category, t.like_count, t.favorite_count, t.comment_count,
       t.created_at, t.updated_at,
       ct.position,
       (t.visibility = 'public') AS is_public
  FROM collection_templates ct
  JOIN templates t ON t.id = ct.template_id
 WHERE ct.collection_id = $1
 ORDER BY ct.position, ct.added_at;

-- ListMembers (public view: public templates only — never leaks private content)
SELECT ...same columns...
  FROM collection_templates ct
  JOIN templates t ON t.id = ct.template_id
 WHERE ct.collection_id = $1
   AND t.visibility = 'public'
 ORDER BY ct.position, ct.added_at;

-- ListContainingTemplate (caller-scoped)
SELECT c.*, (SELECT count(*) ...) AS template_count
  FROM collections c
  JOIN collection_templates ct ON ct.collection_id = c.id
 WHERE c.owner_id = $1 AND c.status = 'active' AND ct.template_id = $2
 ORDER BY c.created_at DESC;

-- ReorderMembers (single transaction)
BEGIN;
DELETE FROM collection_templates WHERE collection_id = $1;
INSERT INTO collection_templates (collection_id, template_id, position, added_at)
VALUES ($1, $2, 0, NOW()), ($1, $3, 1, NOW()), ...; -- preserve original added_at via SELECT
COMMIT;
-- Implementation note: re-INSERT with the original added_at selected from the
-- pre-delete rows (INSERT ... SELECT from a temp hold, or UPDATE positions in
-- place). Simplest correct form: UPDATE collection_templates SET position =
-- unnest-mapping inside one tx; either approach is acceptable as long as it is
-- atomic and added_at is preserved.
```

### 3.3 Service (`backend/internal/service/collection.go` — new)

```go
type CollectionService struct {
    v1.UnimplementedCollectionServiceServer
    collectionRepo repository.CollectionRepository
    templateRepo   repository.TemplateRepository
    userRepo       repository.UserRepository
}
```

Pseudocode (error codes in parentheses):

```
CreateCollection(req):
    userID = GetUserIDFromContext(ctx)            # (Unauthenticated if missing)
    user = userRepo.GetByID(userID)               # (Unauthenticated)
    if user.Banned: return PermissionDenied("banned users cannot create collections")
    name = TrimSpace(req.Name)
    if len(name) < 1 or > 80: return InvalidArgument("name must be 1-80 characters")
    if len(req.Description) > 500: return InvalidArgument("description must be at most 500 characters")
    visibility = req.Visibility or "private"
    if visibility not in {"private","public"}: return InvalidArgument("invalid visibility")
    count = collectionRepo.CountActiveByOwner(userID)
    if count >= 50: return ResourceExhausted("collection limit reached (50)")
    c = &Collection{OwnerID: userID, Name, Description, Visibility: visibility, Status: "active"}
    collectionRepo.Create(c)                      # (Internal)
    return toProtoCollection(c, owner=user, members=nil)

UpdateCollection(req):
    userID = GetUserIDFromContext(ctx)            # (Unauthenticated)
    c = collectionRepo.GetByID(req.ID)            # (NotFound)
    if c.Status == "deleted": return NotFound("collection not found")
    if c.OwnerID != userID: return PermissionDenied("not the collection owner")
    user = userRepo.GetByID(userID); if user.Banned: return PermissionDenied(...)
    # field validation as in CreateCollection for any field present
    # (proto3 cannot distinguish unset from empty strings; treat empty name as
    #  "not provided" — clearing the name is not supported, matching UpdateTemplate)
    apply provided fields; c.UpdatedAt = NOW()
    collectionRepo.Update(c)                      # (Internal)
    return toProtoCollection(c, owner, members=collectionRepo.ListMembers(id, ownerView=true))

DeleteCollection(req):
    ...same owner checks...
    collectionRepo.SoftDelete(req.ID)             # (Internal)
    return {id: req.ID}

GetCollection(req):
    c = collectionRepo.GetByID(req.ID)            # (NotFound)
    if c.Status == "deleted": return NotFound("collection not found")
    userID, authenticated = GetUserIDFromContext(ctx)  # public RPC: may be absent
    isOwner = authenticated && c.OwnerID == userID
    if c.Visibility == "private" and not isOwner: return NotFound("collection not found")
    owner = userRepo.GetByID(c.OwnerID)           # display_name; Internal on failure
    members = collectionRepo.ListMembers(req.ID, ownerView=isOwner)
    c.TemplateCount = len(members)                # owner view: true total; public view: public-only count
    return toProtoCollection(c, owner, members)

ListMyCollections(req):
    userID = GetUserIDFromContext(ctx)            # (Unauthenticated)
    cols = collectionRepo.ListByOwner(userID)     # active only, newest first (Internal)
    for each: fill owner from userRepo (single lookup, cached per call)
    return {collections}

ListPublicCollections(req):
    limit = req.PageSize or 20; if limit < 1 or > 50: limit = clamp  # invalid → InvalidArgument
    sort = req.Sort or "newest"
    if sort not in {"newest","most_templates"}: return InvalidArgument("invalid sort")
    offset = ParseInt(req.PageToken) or 0         # invalid token → InvalidArgument
    cols = collectionRepo.ListPublic(offset, limit, sort)
    next = "" ; if len(cols) == limit: next = Itoa(offset + limit)
    return {collections, next_page_token: next}

AddTemplateToCollection(req):
    ...owner checks on collection (NotFound/PermissionDenied as above)...
    t = templateRepo.Get(req.TemplateID)          # (NotFound)
    if t.Visibility == "private" and t.OwnerID != userID:
        return NotFound("template not found")     # indistinguishable, no existence leak
    if collectionRepo.CountMembers(req.CollectionID) >= 100:
        return ResourceExhausted("collection member limit reached (100)")
    # idempotency: if (collectionID, templateID) already present, return current
    # position and count without error (FR-8)
    pos = collectionRepo.MaxMemberPosition(id) + 1
    collectionRepo.AddMember(collectionID, templateID, pos)   # (Internal)
    return {collection_id, template_id, position: pos, template_count: count+1}

RemoveTemplateFromCollection(req):
    ...owner checks...
    collectionRepo.RemoveMember(collectionID, templateID)     # (Internal; missing row is a no-op)
    return Empty

ReorderCollectionTemplates(req):
    ...owner checks...
    if len(req.TemplateIds) > 100: return InvalidArgument("too many templates")
    if hasDuplicates(req.TemplateIds): return InvalidArgument("duplicate template ids")
    # every id must be a current member (NotFound otherwise) — the client sends
    # the full list, so a mismatch means stale state
    current = set(collectionRepo.ListMembers(id, ownerView=true))
    if set(req.TemplateIds) != current: return NotFound("collection members changed")
    collectionRepo.ReorderMembers(collectionID, req.TemplateIds)  # (Internal, single tx)
    return Empty

ListCollectionsContainingTemplate(req):
    userID = GetUserIDFromContext(ctx)            # (Unauthenticated)
    t = templateRepo.Get(req.TemplateID)          # (NotFound)
    if t.Visibility == "private" and t.OwnerID != userID: return NotFound("template not found")
    cols = collectionRepo.ListContainingTemplate(userID, req.TemplateID)
    return {collections}
```

`toProtoCollection` maps model → `v1.Collection`; `Template` fields reuse the existing template mapper (`toProtoTemplate`) so member payloads match `GET /api/v1/templates` exactly.

### 3.4 AuthInterceptor (`backend/internal/service/auth.go`)

Add to `publicRpcMethods`:

```go
"/v1.CollectionService/GetCollection",
"/v1.CollectionService/ListPublicCollections",
```

All other `CollectionService` RPCs require a token; `GetUserIDFromContext` returns `(string, bool)` so `GetCollection` can branch on anonymous reads.

### 3.5 Gateway (`backend/cmd/server/main.go`)

Wire after `notificationSvc` (~line 162):

```go
collectionRepo := repository.NewCollectionRepository(db)
collectionSvc := service.NewCollectionService(collectionRepo, templateRepo, userRepo)
...
pb.RegisterCollectionServiceServer(s, collectionSvc)
```

New handlers (auth pattern identical to existing routes — GET anonymous with `context.Background()`, mutations require `Authorization` → `VerifyAnyToken` → `service.ContextWithUserID`):

```
/api/v1/collections            GET   → ListPublicCollections   (anonymous; query: page_size, page_token, sort)
                               POST  → CreateCollection        (auth; body: name, description, visibility)
/api/v1/collections/mine       GET   → ListMyCollections       (auth; matched before {id})
/api/v1/collections/{id}       GET   → GetCollection           (anonymous)
                               PATCH → UpdateCollection        (auth; body: any of name/description/visibility)
                               DELETE→ DeleteCollection        (auth)
/api/v1/collections/{id}/templates
                               POST  → AddTemplateToCollection (auth; body: template_id)
                               PUT   → ReorderCollectionTemplates (auth; body: template_ids[])
/api/v1/collections/{id}/templates/{template_id}
                               DELETE→ RemoveTemplateFromCollection (auth)
/api/v1/templates/{id}/collections
                               GET   → ListCollectionsContainingTemplate (auth)
```

Status codes come from `writeError` (InvalidArgument→400, Unauthenticated→401, PermissionDenied→403, NotFound→404, ResourceExhausted→429, Internal→500). Unknown subresource or wrong method → 400/405 as in existing handlers. Routing uses the same `strings.Split(path, "/")` + suffix matching as the `/api/v1/templates/` handler.

## 4. Frontend

### 4.1 New Files

| File | Contents |
|---|---|
| `frontend/src/pages/CollectionsPage.js` | `/collections` explore: grid of `CollectionCard`, infinite scroll (20/page), sort dropdown (newest / most templates), empty state. |
| `frontend/src/pages/MyCollectionsPage.js` | `/collections/mine`: auth-guarded; "New collection" button → create modal; grid of own collections with edit/delete. |
| `frontend/src/pages/CollectionDetailPage.js` | `/collections/:id`: header + member list (`PromptCard` grid); owner actions (Edit, Reorder, Publish/Unpublish, Delete); 404 page on NotFound. |
| `frontend/src/components/collections/CollectionCard.js` | 2×2 CSS initials collage, name, `N templates`, curator, updated; links to detail. |
| `frontend/src/components/collections/CollectionHeader.js` | Title, description, curator line, count, action bar (Copy link public-only). |
| `frontend/src/components/collections/CollectionPicker.js` | Modal: list of `ListMyCollections` with checkmarks from `ListCollectionsContainingTemplate`, staging semantics (Add commits adds, Save applies removes), inline create, filter input, inline limit errors. |
| `frontend/src/components/collections/ReorderList.js` | Rows with up/down buttons + position input; local order state; Save/Discard bar. |
| `frontend/src/components/collections/PublishToggle.js` | Publish/Unpublish with confirm modals. |
| `frontend/src/components/collections/CopyLinkButton.js` | `navigator.clipboard` + `execCommand` fallback; "Copied!" feedback. |
| `frontend/src/components/collections/CollectionEmptyState.js` | Explore-empty / mine-empty / collection-empty variants. |

### 4.2 Modified Files

| File | Change |
|---|---|
| `frontend/src/App.js` | Three lazy routes + `ProtectedRoute` for `/collections/mine`. |
| `frontend/src/components/Sidebar.js` | Authenticated: "My Collections" → `/collections/mine` (below My Favorites, above Categories). Anonymous: "Explore Collections" → `/collections`. |
| `frontend/src/components/PromptCard.js` | Optional `onAddToCollection` prop: hover overlay button (pointer) / ⋯ menu entry (touch); hidden when template is private-and-not-ownable. |
| `frontend/src/services/api.js` | 10 functions (§4.3). |
| `frontend/src/locales/en/translation.json`, `.../zh/translation.json` | `collections.*` keys (§4.5). |

### 4.3 API Client (`frontend/src/services/api.js`)

```js
listPublicCollections({ page_size, page_token, sort })   // GET  /collections
createCollection({ name, description, visibility })      // POST /collections
getCollection(id)                                        // GET  /collections/:id
updateCollection(id, { name, description, visibility })  // PATCH /collections/:id
deleteCollection(id)                                     // DELETE /collections/:id
addTemplateToCollection(collectionId, templateId)        // POST /collections/:id/templates
removeTemplateFromCollection(collectionId, templateId)   // DELETE /collections/:id/templates/:templateId
reorderCollectionTemplates(collectionId, templateIds)    // PUT  /collections/:id/templates
listCollectionsContainingTemplate(templateId)            // GET  /templates/:id/collections
listMyCollections()                                      // GET  /collections/mine → ListMyCollections
```

(`listMyCollections` maps to the `ListMyCollections` gRPC RPC via `GET /api/v1/collections/mine`, matched before the `{id}` route — see §3.5.)

### 4.4 Component Tree (detail page, owner view)

```
CollectionDetailPage
├── CollectionHeader
│   ├── props: collection, isOwner, onEdit, onPublishToggle, onDelete, onCopyLink
│   ├── state: copyFeedback (bool)
│   ├── PublishToggle (confirm modal)      i18n: collections.publish, collections.unpublish, collections.publish_confirm, collections.unpublish_confirm
│   └── CopyLinkButton                     i18n: collections.copy_link, collections.link_copied
├── members: PromptCard[] (grid)           i18n: collections.member_badge.private, collections.member_badge.no_longer_public
├── ReorderList (owner, edit mode)
│   ├── props: members, onSave(order), onDiscard
│   └── state: localOrder[], dirty (bool)  i18n: collections.reorder, collections.save, collections.discard
└── CollectionEmptyState (no members)      i18n: collections.empty.collection
```

Picker (opened from `PromptCard` overlay or detail page):

```
CollectionPicker
├── props: template, onClose
├── state: myCollections[], containing Set<id>, stagedAdd Set<id>, stagedRemove Set<id>,
│          filter (string), creating (bool), error (string|null)
├── load: Promise.all([listMyCollections(), listCollectionsContainingTemplate(template.id)])
├── commit (Save): stagedAdd → addTemplateToCollection each; stagedRemove → removeTemplateFromCollection each
│                  (sequential; first error shown inline, already-applied changes kept)
└── i18n: collections.picker.title, .subtitle, .filter_placeholder, .create_new, .save, .cancel,
          .limit_reached, .member_limit_reached
```

### 4.5 i18n Keys (`collections.*`)

| Key | EN | ZH |
|---|---|---|
| `nav_mine` | My Collections | 我的合集 |
| `nav_explore` | Explore Collections | 探索合集 |
| `explore_title` | Explore Collections | 探索合集 |
| `mine_title` | My Collections | 我的合集 |
| `new` | New collection | 新建合集 |
| `items` | {{count}} templates | {{count}} 个模板 |
| `by` | by {{name}} | 由 {{name}} 创建 |
| `updated` | Updated {{date}} | 更新于 {{date}} |
| `copy_link` | Copy link | 复制链接 |
| `link_copied` | Copied! | 已复制！ |
| `publish` | Publish | 发布 |
| `unpublish` | Unpublish | 取消发布 |
| `publish_confirm` | Make this collection public? | 将此合集设为公开？ |
| `unpublish_confirm` | Make this collection private? | 将此合集设为私有？ |
| `edit` | Edit | 编辑 |
| `reorder` | Reorder | 排序 |
| `delete` | Delete | 删除 |
| `delete_confirm` | Delete this collection? This cannot be undone. | 删除此合集？此操作无法撤销。 |
| `add_to` | Add to collection | 添加到合集 |
| `picker.title` | Add to collection | 添加到合集 |
| `picker.subtitle` | Select one or more collections | 选择一个或多个合集 |
| `picker.filter_placeholder` | Filter collections | 筛选合集 |
| `picker.create_new` | New collection… | 新建合集… |
| `picker.save` | Save | 保存 |
| `picker.cancel` | Cancel | 取消 |
| `picker.limit_reached` | You've reached the 50-collection limit. | 已达到 50 个合集的上限。 |
| `picker.member_limit_reached` | "{{name}}" is full (100 templates). | “{{name}}”已满（100 个模板）。 |
| `member_badge.private` | Private | 私有 |
| `member_badge.no_longer_public` | No longer public | 已不再公开 |
| `more_from` | More from {{name}} | 来自 {{name}} 的更多合集 |
| `empty.explore` | No public collections yet | 还没有公开合集 |
| `empty.mine` | No collections yet — create your first | 还没有合集——创建第一个吧 |
| `empty.collection` | This collection is empty | 此合集中还没有模板 |
| `error.load` | Failed to load collections | 加载合集失败 |
| `error.save` | Failed to save | 保存失败 |

## 5. Error Matrix

| Scenario | gRPC | HTTP | Frontend handling |
|---|---|---|---|
| Name empty / >80; description >500; bad visibility; bad sort; bad page_token; duplicate ids in reorder | `InvalidArgument` | 400 | Inline field errors |
| Missing token on mutation | `Unauthenticated` | 401 | Redirect to login |
| Non-owner mutation; banned user | `PermissionDenied` | 403 | Error toast; banned message |
| Collection not found / deleted; private collection non-owner read; private template non-owner add | `NotFound` | 404 | 404 page (detail) / toast (picker) |
| 50-collection cap; 100-member cap | `ResourceExhausted` | 429 | Inline picker error (`picker.limit_reached` / `picker.member_limit_reached`) |
| Repo failure | `Internal` | 500 | `error.load` / `error.save` toast |
| Reorder list ≠ current members | `NotFound` | 404 | Prompt refresh, then retry |

## 6. Deployment

1. Append DDL to both `init.sql` copies (byte-identical) — idempotent, additive, no backfill.
2. Regenerate proto (`collection.pb.go`, `collection_grpc.pb.go`) and commit alongside `collection.proto`.
3. Deploy backend (new service registered; new routes).
4. Deploy frontend (new pages behind lazy imports; old caches unaffected).

## 7. SDK / Public API Docs

- REST surface is part of the public `/api/v1` API: document the five collection routes + template cross-reference in the API reference with request/response examples (§1 JSON shapes).
- `ListPublicCollections` is anonymous — usable from the embed/widget layer later without keys.
- No breaking changes to existing endpoints; `Template` payloads inside members are identical to `GET /api/v1/templates/{id}`.

## 8. Test Strategy

### FVT (`tests/fvt.py::test_template_collections`)

Uses `get_auth_token` for two users (owner, visitor). Steps map to ACs:

1. **AC-1/2** `POST /collections` (owner): create with name/description → 200, UUID id, `visibility=="private"`, `template_count==0`. Missing name → 400. Name >80 → 400.
2. **AC-3** `PATCH /collections/{id}`: rename + set `visibility="public"` → 200.
3. **AC-4/12** Anonymous `GET /collections/{id}` → 200 with members. Second-user read of a **private** collection → 404.
4. **AC-5** `POST /collections/{id}/templates` add public template → 200, `position==0`, `template_count==1`. Re-add same → 200 idempotent (same position, count unchanged). Add second → `position==1`.
5. **AC-6** `DELETE /collections/{id}/templates/{tid}` → 200; member gone from `GET`.
6. **AC-7** `PUT /collections/{id}/templates` reorder `[t2, t1]` → 200; `GET` shows positions 0/1 swapped.
7. **AC-8** Visitor `POST` on owner's collection → 403. Visitor anonymous (no token) → 401.
8. **AC-9** `GET /templates/{tid}/collections` (owner) → contains the collection; visitor's call → not containing owner's private collection.
9. **AC-10** `GET /collections` anonymous: lists the public collection with `template_count`; `sort=most_templates` ordering verified; `page_token` pagination returns `next_page_token`. `GET /collections/mine` (owner) lists own collections including private ones.
10. **AC-11** `DELETE /collections/{id}` → 200; subsequent `GET` → 404; absent from `GET /collections` and from `GET /templates/{tid}/collections`.
11. Member cap: create collection, add up to the 100-member cap is impractical in FVT — verify the 50-collection cap instead by creating collections until `ResourceExhausted` (429) **or** verify the error path with a lowered check via unit tests; FVT asserts the 429 contract on the 51st create only if runtime permits, otherwise unit-covered.

### Unit Tests

- Repository: member filtering (public vs owner view), sort by `template_count`, `ListContainingTemplate` scoping, reorder atomicity (tx rollback on bad id).
- Service: full authz matrix (anonymous/visitor/owner/banned × private/public/deleted), limit errors, idempotent add, reorder mismatch.

### Component Tests

- Picker staging semantics (add/remove sets, Save applies both), ReorderList Save/Discard, PublishToggle confirm modals, empty states, EN/ZH rendering of all `collections.*` keys.

## 9. Implementation Order

1. DDL in both `init.sql` files.
2. `collection.proto` + regenerate.
3. `model/collection.go`, `repository/collection_repository.go` (+ unit tests).
4. `service/collection.go` (+ unit tests), `auth.go` public RPCs.
5. Gateway wiring + routes.
6. FVT `test_template_collections` (run against docker-compose stack).
7. Frontend: api.js → components → pages → Sidebar/App routes → i18n.
8. Component tests + manual a11y pass.

## 10. Constraints for Developer

- **int32 for every count** — never int64 (protojson string quirk).
- **404, not 403, for private reads** — existence must not leak; 403 is only for acting on a collection you can see but don't own.
- **Banned users → 403** on all mutations, never 401.
- **Both `init.sql` copies byte-identical**; DDL idempotent (DO $$ + information_schema/pg_indexes guards).
- **No denormalized counters** — `template_count` computed at read time.
- **Public member view filters `templates.visibility='public'`** — private template content must never appear in a public collection response.
- **`ListCollectionsContainingTemplate` is caller-scoped** — never returns other users' collections.
- Reorder requires the **full member list**; mismatch → 404 (stale state), not a partial apply.
- Soft delete only — no hard `DELETE` from the service layer.
- No monetization hooks of any kind.
