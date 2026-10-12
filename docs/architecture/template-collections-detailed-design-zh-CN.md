# 模板合集 — 详细设计

| | |
|---|---|
| **功能 ID** | template-collections |
| **版本** | 1.0 |
| **状态** | 已批准 |
| **作者** | Architect Agent |
| **上游** | [架构设计](template-collections-architecture-zh-CN.md) · [需求文档](../design/template-collections-requirements-zh-CN.md) · [UI/UX 设计](../design/template-collections-ui-ux-design-zh-CN.md) |
| **下游** | Developer Agent |

## 1. Proto 契约

新文件 `backend/api/proto/v1/collection.proto`。package、go_package 与风格遵循 `notification.proto`。所有计数均为 `int32`（protojson 将 `int64` 序列化为 JSON 字符串——前端与 FVT 将计数作为数字比较）。时间戳为 RFC3339 字符串，与现有消息一致。

```protobuf
syntax = "proto3";

package v1;

option go_package = "backend/api/proto/v1;v1";

// CollectionService 管理用户策划的有序模板集合。
service CollectionService {
  // CreateCollection 为调用者创建合集（默认私有）。
  rpc CreateCollection(CreateCollectionRequest) returns (Collection);
  // UpdateCollection 编辑名称/描述/可见性（仅所有者）。
  rpc UpdateCollection(UpdateCollectionRequest) returns (Collection);
  // DeleteCollection 软删除合集（仅所有者）。
  rpc DeleteCollection(DeleteCollectionRequest) returns (DeleteCollectionResponse);
  // GetCollection 返回合集及其成员。公开合集可匿名读取；
  // 私有/已删除合集对非所有者返回 NotFound。
  rpc GetCollection(GetCollectionRequest) returns (Collection);
  // ListMyCollections 返回调用者的活跃合集。
  rpc ListMyCollections(ListMyCollectionsRequest) returns (ListMyCollectionsResponse);
  // ListPublicCollections 返回公开探索流，分页且可排序。
  rpc ListPublicCollections(ListPublicCollectionsRequest) returns (ListPublicCollectionsResponse);
  // AddTemplateToCollection 添加模板（仅所有者，幂等）。
  rpc AddTemplateToCollection(AddTemplateToCollectionRequest) returns (AddTemplateToCollectionResponse);
  // RemoveTemplateFromCollection 移除模板（仅所有者）。
  rpc RemoveTemplateFromCollection(RemoveTemplateFromCollectionRequest) returns (google.protobuf.Empty);
  // ReorderCollectionTemplates 应用完整有序模板 ID 列表（仅所有者）。
  rpc ReorderCollectionTemplates(ReorderCollectionTemplatesRequest) returns (google.protobuf.Empty);
  // ListCollectionsContainingTemplate 返回调用者的、包含指定模板的合集
  // （用于选择器勾选态）。
  rpc ListCollectionsContainingTemplate(ListCollectionsContainingTemplateRequest) returns (ListCollectionsContainingTemplateResponse);
}

// 合集摘要。用于列表；GetCollection 额外填充 members。
message Collection {
  string id = 1;
  string owner_id = 2;
  string name = 3;
  string description = 4;
  string visibility = 5; // "private" | "public"
  int32 template_count = 6;      // 读取时计算，绝不反规范化
  string created_at = 7;         // RFC3339
  string updated_at = 8;         // RFC3339
  CollectionOwner owner = 9;     // id + display_name，用于卡片/详情
  repeated CollectionMember members = 10; // 仅 GetCollection 填充
}

message CollectionOwner {
  string id = 1;
  string display_name = 2;
}

// 成员条目：复用的 Template 消息加位置与可见性标记。
message CollectionMember {
  Template template = 1;
  int32 position = 2;
  bool is_public = 3; // 所有者视图：标记私有/已不再公开的成员
}

message CreateCollectionRequest {
  string name = 1;        // 1-80 字符，必填
  string description = 2; // 0-500 字符
  string visibility = 3;  // "private"（默认）| "public"
}

message UpdateCollectionRequest {
  string id = 1;
  string name = 2;         // 可选；非空时校验
  string description = 3;  // 可选
  string visibility = 4;   // 可选
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
  int32 page_size = 1;   // 默认 20，最大 50
  string page_token = 2; // offset 字符串化（现有约定）
  string sort = 3;       // "newest"（默认）| "most_templates"
}

message ListPublicCollectionsResponse {
  repeated Collection collections = 1;
  string next_page_token = 2; // 无更多页时为空
}

message AddTemplateToCollectionRequest {
  string collection_id = 1;
  string template_id = 2;
}

message AddTemplateToCollectionResponse {
  string collection_id = 1;
  string template_id = 2;
  int32 position = 3;
  int32 template_count = 4; // 添加后的数量
}

message RemoveTemplateFromCollectionRequest {
  string collection_id = 1;
  string template_id = 2;
}

message ReorderCollectionTemplatesRequest {
  string collection_id = 1;
  repeated string template_ids = 2; // 完整有序列表，无重复
}

message ListCollectionsContainingTemplateRequest {
  string template_id = 1;
}

message ListCollectionsContainingTemplateResponse {
  repeated Collection collections = 1; // 调用者的包含该模板的合集
}
```

`google.protobuf.Empty` 返回值遵循现有约定（`DeleteTemplate`、`UnlikeTemplate`）。

### JSON 线上格式（网关，`UseProtoNames: true`，`EmitUnpopulated: true`）

```jsonc
// GET /api/v1/collections/{id}（公开合集，匿名）
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

## 2. 数据库 DDL

相同的幂等块追加到 `deployment/init.sql` 与 `deployment/chart/open-prompts/init.sql` **两份文件**（完整 SQL 见[架构设计 §5](template-collections-architecture-zh-CN.md#5-数据模型)）。摘要：

- `collections` — UUID 主键 `uuid_generate_v4()`、`owner_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE`、`name`、`description`、`visibility` CHECK `('private','public')` 默认 `'private'`、`status` CHECK `('active','deleted')` 默认 `'active'`、时间戳。
- `collection_templates` — 复合主键 `(collection_id, template_id)`、`position INT NOT NULL`、`added_at`、两侧 FK 均 `ON DELETE CASCADE`。
- 索引：`idx_collection_templates_position (collection_id, position)`；部分索引 `idx_collections_owner (owner_id) WHERE status='active'`；部分索引 `idx_collections_public (visibility, created_at DESC) WHERE status='active' AND visibility='public'`。

## 3. 后端分层

### 3.1 模型（`backend/internal/model/collection.go` — 新增）

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
    TemplateCount int32 // 读取时计算
}

type CollectionMember struct {
    Template  model.Template
    Position  int32
    IsPublic  bool
}
```

### 3.2 仓储（`backend/internal/repository/collection_repository.go` — 新增）

接口模式遵循 `CommentRepository`（ctx 优先，构造函数 `NewCollectionRepository(db)`）：

```go
type CollectionRepository interface {
    Create(ctx context.Context, c *model.Collection) error
    Update(ctx context.Context, c *model.Collection) error // name/desc/visibility/updated_at
    SoftDelete(ctx context.Context, id string) error       // status='deleted', updated_at=NOW()
    GetByID(ctx context.Context, id string) (*model.Collection, error) // 任意状态；服务层过滤
    CountActiveByOwner(ctx context.Context, ownerID string) (int32, error)
    ListByOwner(ctx context.Context, ownerID string) ([]*model.Collection, error) // 活跃，最新优先
    ListPublic(ctx context.Context, offset, limit int32, sort string) ([]*model.Collection, error)
    CountPublic(ctx context.Context) (int32, error)
    AddMember(ctx context.Context, collectionID, templateID string, position int32) error
    RemoveMember(ctx context.Context, collectionID, templateID string) error
    ReorderMembers(ctx context.Context, collectionID string, templateIDs []string) error // 事务：删除+插入
    CountMembers(ctx context.Context, collectionID string) (int32, error)
    MaxMemberPosition(ctx context.Context, collectionID string) (int32, error)
    ListMembers(ctx context.Context, collectionID string, ownerView bool) ([]*model.CollectionMember, error)
    ListContainingTemplate(ctx context.Context, ownerID, templateID string) ([]*model.Collection, error)
}
```

关键 SQL：

```sql
-- ListPublic，sort=newest（默认）
SELECT c.id, c.owner_id, c.name, c.description, c.visibility, c.status,
       c.created_at, c.updated_at,
       (SELECT count(*) FROM collection_templates ct
         WHERE ct.collection_id = c.id) AS template_count
  FROM collections c
 WHERE c.status = 'active' AND c.visibility = 'public'
 ORDER BY c.created_at DESC
 LIMIT $1 OFFSET $2;

-- ListPublic，sort=most_templates
 ... ORDER BY template_count DESC, c.created_at DESC ...

-- ListMembers（所有者视图：全部成员，带标记）
SELECT t.id, t.owner_id, t.title, t.description, t.visibility, t.type, t.tags,
       t.category, t.like_count, t.favorite_count, t.comment_count,
       t.created_at, t.updated_at,
       ct.position,
       (t.visibility = 'public') AS is_public
  FROM collection_templates ct
  JOIN templates t ON t.id = ct.template_id
 WHERE ct.collection_id = $1
 ORDER BY ct.position, ct.added_at;

-- ListMembers（公开视图：仅公开模板——绝不泄露私有内容）
SELECT ...同上列...
  FROM collection_templates ct
  JOIN templates t ON t.id = ct.template_id
 WHERE ct.collection_id = $1
   AND t.visibility = 'public'
 ORDER BY ct.position, ct.added_at;

-- ListContainingTemplate（限定调用者）
SELECT c.*, (SELECT count(*) ...) AS template_count
  FROM collections c
  JOIN collection_templates ct ON ct.collection_id = c.id
 WHERE c.owner_id = $1 AND c.status = 'active' AND ct.template_id = $2
 ORDER BY c.created_at DESC;

-- ReorderMembers（单事务）
BEGIN;
DELETE FROM collection_templates WHERE collection_id = $1;
INSERT INTO collection_templates (collection_id, template_id, position, added_at)
VALUES ($1, $2, 0, NOW()), ($1, $3, 1, NOW()), ...; -- 通过 SELECT 保留原 added_at
COMMIT;
-- 实现说明：重插时使用删除前行中的原 added_at（INSERT ... SELECT 自临时持有，
-- 或就地 UPDATE 位置）。最简单正确的形式：在单个事务内按 unnest 映射 UPDATE
-- collection_templates SET position = ...；只要原子且保留 added_at，两种方式均可。
```

### 3.3 服务（`backend/internal/service/collection.go` — 新增）

```go
type CollectionService struct {
    v1.UnimplementedCollectionServiceServer
    collectionRepo repository.CollectionRepository
    templateRepo   repository.TemplateRepository
    userRepo       repository.UserRepository
}
```

伪代码（括号内为错误码）：

```
CreateCollection(req):
    userID = GetUserIDFromContext(ctx)            # (Unauthenticated 缺失时)
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
    # 字段校验同 CreateCollection（针对出现的字段）
    # （proto3 无法区分未设置与空字符串；空 name 视为"未提供"——不支持清空名称，
    #  与 UpdateTemplate 一致）
    应用提供的字段; c.UpdatedAt = NOW()
    collectionRepo.Update(c)                      # (Internal)
    return toProtoCollection(c, owner, members=collectionRepo.ListMembers(id, ownerView=true))

DeleteCollection(req):
    ...同上所有者检查...
    collectionRepo.SoftDelete(req.ID)             # (Internal)
    return {id: req.ID}

GetCollection(req):
    c = collectionRepo.GetByID(req.ID)            # (NotFound)
    if c.Status == "deleted": return NotFound("collection not found")
    userID, authenticated = GetUserIDFromContext(ctx)  # 公开 RPC：可能缺失
    isOwner = authenticated && c.OwnerID == userID
    if c.Visibility == "private" and not isOwner: return NotFound("collection not found")
    owner = userRepo.GetByID(c.OwnerID)           # display_name；失败为 Internal
    members = collectionRepo.ListMembers(req.ID, ownerView=isOwner)
    c.TemplateCount = len(members)                # 所有者视图：真实总数；公开视图：仅公开计数
    return toProtoCollection(c, owner, members)

ListMyCollections(req):
    userID = GetUserIDFromContext(ctx)            # (Unauthenticated)
    cols = collectionRepo.ListByOwner(userID)     # 仅活跃，最新优先 (Internal)
    for each: 从 userRepo 填充 owner（单次查询，按调用缓存）
    return {collections}

ListPublicCollections(req):
    limit = req.PageSize or 20; if limit < 1 or > 50: limit = clamp  # 非法 → InvalidArgument
    sort = req.Sort or "newest"
    if sort not in {"newest","most_templates"}: return InvalidArgument("invalid sort")
    offset = ParseInt(req.PageToken) or 0         # 非法 token → InvalidArgument
    cols = collectionRepo.ListPublic(offset, limit, sort)
    next = "" ; if len(cols) == limit: next = Itoa(offset + limit)
    return {collections, next_page_token: next}

AddTemplateToCollection(req):
    ...合集所有者检查（同上 NotFound/PermissionDenied）...
    t = templateRepo.Get(req.TemplateID)          # (NotFound)
    if t.Visibility == "private" and t.OwnerID != userID:
        return NotFound("template not found")     # 无法区分，不泄露存在性
    if collectionRepo.CountMembers(req.CollectionID) >= 100:
        return ResourceExhausted("collection member limit reached (100)")
    # 幂等：若 (collectionID, templateID) 已存在，返回当前位置与计数，不报错（FR-8）
    pos = collectionRepo.MaxMemberPosition(id) + 1
    collectionRepo.AddMember(collectionID, templateID, pos)   # (Internal)
    return {collection_id, template_id, position: pos, template_count: count+1}

RemoveTemplateFromCollection(req):
    ...所有者检查...
    collectionRepo.RemoveMember(collectionID, templateID)     # (Internal；行缺失为 no-op)
    return Empty

ReorderCollectionTemplates(req):
    ...所有者检查...
    if len(req.TemplateIds) > 100: return InvalidArgument("too many templates")
    if hasDuplicates(req.TemplateIds): return InvalidArgument("duplicate template ids")
    # 每个 id 必须是当前成员（否则 NotFound）——客户端发送完整列表，
    # 不匹配意味着状态过期
    current = set(collectionRepo.ListMembers(id, ownerView=true))
    if set(req.TemplateIds) != current: return NotFound("collection members changed")
    collectionRepo.ReorderMembers(collectionID, req.TemplateIds)  # (Internal，单事务)
    return Empty

ListCollectionsContainingTemplate(req):
    userID = GetUserIDFromContext(ctx)            # (Unauthenticated)
    t = templateRepo.Get(req.TemplateID)          # (NotFound)
    if t.Visibility == "private" and t.OwnerID != userID: return NotFound("template not found")
    cols = collectionRepo.ListContainingTemplate(userID, req.TemplateID)
    return {collections}
```

`toProtoCollection` 将模型映射为 `v1.Collection`；`Template` 字段复用现有模板映射器（`toProtoTemplate`），成员载荷与 `GET /api/v1/templates` 完全一致。

### 3.4 AuthInterceptor（`backend/internal/service/auth.go`）

加入 `publicRpcMethods`：

```go
"/v1.CollectionService/GetCollection",
"/v1.CollectionService/ListPublicCollections",
```

其余 `CollectionService` RPC 均需令牌；`GetUserIDFromContext` 返回 `(string, bool)`，`GetCollection` 可据此分支匿名读取。

### 3.5 网关（`backend/cmd/server/main.go`）

在 `notificationSvc` 之后接线（约 162 行）：

```go
collectionRepo := repository.NewCollectionRepository(db)
collectionSvc := service.NewCollectionService(collectionRepo, templateRepo, userRepo)
...
pb.RegisterCollectionServiceServer(s, collectionSvc)
```

新处理器（鉴权模式与现有路由一致——GET 匿名用 `context.Background()`，变更操作要求 `Authorization` → `VerifyAnyToken` → `service.ContextWithUserID`）：

```
/api/v1/collections            GET   → ListPublicCollections   (匿名；query: page_size, page_token, sort)
                               POST  → CreateCollection        (鉴权; body: name, description, visibility)
/api/v1/collections/mine       GET   → ListMyCollections       (鉴权; 先于 {id} 匹配)
/api/v1/collections/{id}       GET   → GetCollection           (匿名)
                               PATCH → UpdateCollection        (鉴权; body: name/description/visibility 任意)
                               DELETE→ DeleteCollection        (鉴权)
/api/v1/collections/{id}/templates
                               POST  → AddTemplateToCollection (鉴权; body: template_id)
                               PUT   → ReorderCollectionTemplates (鉴权; body: template_ids[])
/api/v1/collections/{id}/templates/{template_id}
                               DELETE→ RemoveTemplateFromCollection (鉴权)
/api/v1/templates/{id}/collections
                               GET   → ListCollectionsContainingTemplate (鉴权)
```

状态码来自 `writeError`（InvalidArgument→400、Unauthenticated→401、PermissionDenied→403、NotFound→404、ResourceExhausted→429、Internal→500）。未知子资源或方法错误 → 400/405，与现有处理器一致。路由使用与 `/api/v1/templates/` 处理器相同的 `strings.Split(path, "/")` + 后缀匹配。

## 4. 前端

### 4.1 新增文件

| 文件 | 内容 |
|---|---|
| `frontend/src/pages/CollectionsPage.js` | `/collections` 探索页：`CollectionCard` 网格、无限滚动（每页 20）、排序下拉（最新/最多模板）、空状态。 |
| `frontend/src/pages/MyCollectionsPage.js` | `/collections/mine`：登录守卫；"新建合集"按钮 → 创建弹窗；自己合集的网格，含编辑/删除。 |
| `frontend/src/pages/CollectionDetailPage.js` | `/collections/:id`：头部 + 成员列表（`PromptCard` 网格）；所有者操作（编辑、排序、发布/取消发布、删除）；NotFound → 404 页。 |
| `frontend/src/components/collections/CollectionCard.js` | 2×2 CSS 首字母拼贴、名称、`N templates`、策划者、更新时间；链接到详情。 |
| `frontend/src/components/collections/CollectionHeader.js` | 标题、描述、策划者行、数量、操作栏（复制链接仅公开）。 |
| `frontend/src/components/collections/CollectionPicker.js` | 弹窗：`ListMyCollections` 列表 + 来自 `ListCollectionsContainingTemplate` 的勾选态、暂存语义（勾选即添加、保存时应用移除）、内联创建、过滤输入、内联限额错误。 |
| `frontend/src/components/collections/ReorderList.js` | 带上移/下移按钮与位置输入的行；本地顺序状态；保存/放弃栏。 |
| `frontend/src/components/collections/PublishToggle.js` | 带确认弹窗的发布/取消发布。 |
| `frontend/src/components/collections/CopyLinkButton.js` | `navigator.clipboard` + `execCommand` 回退；"已复制！"反馈。 |
| `frontend/src/components/collections/CollectionEmptyState.js` | 探索空/我的空/合集空三种变体。 |

### 4.2 修改文件

| 文件 | 变更 |
|---|---|
| `frontend/src/App.js` | 三个懒加载路由 + `/collections/mine` 的 `ProtectedRoute`。 |
| `frontend/src/components/Sidebar.js` | 已登录："我的合集" → `/collections/mine`（我的收藏下方、分类上方）。匿名："探索合集" → `/collections`。 |
| `frontend/src/components/PromptCard.js` | 可选 `onAddToCollection` 属性：悬停覆盖层按钮（指针设备）/ ⋯ 菜单项（触屏）；模板为私有且不可添加时隐藏。 |
| `frontend/src/services/api.js` | 10 个函数（§4.3）。 |
| `frontend/src/locales/en/translation.json`、`.../zh/translation.json` | `collections.*` 键（§4.5）。 |

### 4.3 API 客户端（`frontend/src/services/api.js`）

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

（`listMyCollections` 通过 `GET /api/v1/collections/mine` 映射到 `ListMyCollections` gRPC RPC，在 `{id}` 路由之前匹配——见 §3.5。）

### 4.4 组件树（详情页，所有者视图）

```
CollectionDetailPage
├── CollectionHeader
│   ├── props: collection, isOwner, onEdit, onPublishToggle, onDelete, onCopyLink
│   ├── state: copyFeedback (bool)
│   ├── PublishToggle (确认弹窗)      i18n: collections.publish, collections.unpublish, collections.publish_confirm, collections.unpublish_confirm
│   └── CopyLinkButton                 i18n: collections.copy_link, collections.link_copied
├── members: PromptCard[] (网格)       i18n: collections.member_badge.private, collections.member_badge.no_longer_public
├── ReorderList (所有者, 编辑模式)
│   ├── props: members, onSave(order), onDiscard
│   └── state: localOrder[], dirty (bool)  i18n: collections.reorder, collections.save, collections.discard
└── CollectionEmptyState (无成员)      i18n: collections.empty.collection
```

选择器（从 `PromptCard` 覆盖层或详情页打开）：

```
CollectionPicker
├── props: template, onClose
├── state: myCollections[], containing Set<id>, stagedAdd Set<id>, stagedRemove Set<id>,
│          filter (string), creating (bool), error (string|null)
├── load: Promise.all([listMyCollections(), listCollectionsContainingTemplate(template.id)])
├── commit (保存): stagedAdd → 逐个 addTemplateToCollection; stagedRemove → 逐个 removeTemplateFromCollection
│                  （顺序执行；首个错误内联展示，已应用的变更保留）
└── i18n: collections.picker.title, .subtitle, .filter_placeholder, .create_new, .save, .cancel,
          .limit_reached, .member_limit_reached
```

### 4.5 i18n 键（`collections.*`）

| 键 | EN | ZH |
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

## 5. 错误矩阵

| 场景 | gRPC | HTTP | 前端处理 |
|---|---|---|---|
| 名称为空 / >80；描述 >500；可见性非法；排序非法；page_token 非法；重排含重复 id | `InvalidArgument` | 400 | 内联字段错误 |
| 变更操作缺少令牌 | `Unauthenticated` | 401 | 重定向登录 |
| 非所有者变更；被封禁用户 | `PermissionDenied` | 403 | 错误 toast；封禁消息 |
| 合集不存在/已删除；非所有者读私有合集；非所有者添加私有模板 | `NotFound` | 404 | 404 页（详情）/ toast（选择器） |
| 50 合集上限；100 成员上限 | `ResourceExhausted` | 429 | 选择器内联错误（`picker.limit_reached` / `picker.member_limit_reached`） |
| 仓储失败 | `Internal` | 500 | `error.load` / `error.save` toast |
| 重排列表 ≠ 当前成员 | `NotFound` | 404 | 提示刷新后重试 |

## 6. 部署

1. 将 DDL 追加到两份 `init.sql`（字节一致）——幂等、纯增量、无需回填。
2. 重新生成 proto（`collection.pb.go`、`collection_grpc.pb.go`）并与 `collection.proto` 一并提交。
3. 部署后端（新服务注册；新路由）。
4. 部署前端（懒加载引入的新页面；旧缓存不受影响）。

## 7. SDK / 公开 API 文档

- REST 面是公开 `/api/v1` API 的一部分：在 API 参考中记录五个合集路由 + 模板交叉引用，附请求/响应示例（§1 JSON 格式）。
- `ListPublicCollections` 可匿名调用——将来可从 embed/widget 层直接使用，无需密钥。
- 不破坏任何现有端点；成员内的 `Template` 载荷与 `GET /api/v1/templates/{id}` 完全一致。

## 8. 测试策略

### FVT（`tests/fvt.py::test_template_collections`）

使用 `get_auth_token` 准备两个用户（所有者、访客）。步骤对应 AC：

1. **AC-1/2** `POST /collections`（所有者）：带名称/描述创建 → 200，UUID id，`visibility=="private"`，`template_count==0`。缺名称 → 400。名称 >80 → 400。
2. **AC-3** `PATCH /collections/{id}`：重命名 + 设 `visibility="public"` → 200。
3. **AC-4/12** 匿名 `GET /collections/{id}` → 200 含成员。第二用户读**私有**合集 → 404。
4. **AC-5** `POST /collections/{id}/templates` 添加公开模板 → 200，`position==0`，`template_count==1`。重复添加 → 200 幂等（位置不变，计数不变）。添加第二个 → `position==1`。
5. **AC-6** `DELETE /collections/{id}/templates/{tid}` → 200；`GET` 中成员消失。
6. **AC-7** `PUT /collections/{id}/templates` 重排 `[t2, t1]` → 200；`GET` 显示位置 0/1 互换。
7. **AC-8** 访客对所有人的合集 `POST` → 403。访客匿名（无令牌）→ 401。
8. **AC-9** `GET /templates/{tid}/collections`（所有者）→ 包含该合集；访客调用 → 不包含所有者的私有合集。
9. **AC-10** 匿名 `GET /collections`：列出公开合集及 `template_count`；验证 `sort=most_templates` 顺序；`page_token` 分页返回 `next_page_token`。`GET /collections/mine`（所有者）列出自己的合集（含私有）。
10. **AC-11** `DELETE /collections/{id}` → 200；随后 `GET` → 404；从 `GET /collections` 与 `GET /templates/{tid}/collections` 中消失。
11. 成员上限：在 FVT 中添加到 100 成员上限不现实——改为验证 50 合集上限：持续创建合集直到 `ResourceExhausted`（429）**或**通过单元测试验证降低阈值后的错误路径；FVT 仅在运行时允许时断言第 51 次创建的 429 契约，否则由单元测试覆盖。

### 单元测试

- 仓储：成员过滤（公开 vs 所有者视图）、按 `template_count` 排序、`ListContainingTemplate` 限定、重排原子性（坏 id 时事务回滚）。
- 服务：完整鉴权矩阵（匿名/访客/所有者/封禁 × 私有/公开/已删除）、限额错误、幂等添加、重排不匹配。

### 组件测试

- 选择器暂存语义（添加/移除集合、保存时应用两者）、ReorderList 保存/放弃、PublishToggle 确认弹窗、空状态、所有 `collections.*` 键的 EN/ZH 渲染。

## 9. 实现顺序

1. 两份 `init.sql` 中的 DDL。
2. `collection.proto` + 重新生成。
3. `model/collection.go`、`repository/collection_repository.go`（+ 单元测试）。
4. `service/collection.go`（+ 单元测试）、`auth.go` 公开 RPC。
5. 网关接线 + 路由。
6. FVT `test_template_collections`（对 docker-compose 栈运行）。
7. 前端：api.js → 组件 → 页面 → Sidebar/App 路由 → i18n。
8. 组件测试 + 手动无障碍检查。

## 10. 给开发者的约束

- **所有计数用 int32**——绝不用 int64（protojson 字符串化问题）。
- **私有读取返回 404 而非 403**——不得泄露存在性；403 仅用于你能看到但不拥有的合集上的操作。
- **被封禁用户所有变更 → 403**，绝不 401。
- **两份 `init.sql` 字节一致**；DDL 幂等（DO $$ + information_schema/pg_indexes 守卫）。
- **无反规范化计数器**——`template_count` 读取时计算。
- **公开成员视图过滤 `templates.visibility='public'`**——私有模板内容绝不能出现在公开合集响应中。
- **`ListCollectionsContainingTemplate` 限定调用者**——绝不返回他人的合集。
- 重排要求**完整成员列表**；不匹配 → 404（状态过期），而非部分应用。
- 仅软删除——服务层不做硬 `DELETE`。
- 不存在任何形式的变现钩子。
