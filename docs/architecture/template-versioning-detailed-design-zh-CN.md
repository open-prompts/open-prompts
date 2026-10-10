# 模板版本管理 UX — 详细设计

- **功能 ID：** `template-versioning`
- **版本：** 1.0
- **状态：** 已批准进入实现
- **作者：** 架构 Agent
- **上游文档：**[需求分析](../design/template-versioning-requirements-zh-CN.md) · [UI/UX 设计](../design/template-versioning-ui-ux-design-zh-CN.md)
- **架构文档：**[架构设计（中文）](./template-versioning-architecture-zh-CN.md) · [Architecture Design (EN)](./template-versioning-architecture.md)

---

## 1. Proto 契约（对 `prompt.proto` 的增量变更）

```protobuf
// TemplateVersion 表示模板内容的某个具体版本。
message TemplateVersion {
  int32 id = 1;
  string template_id = 2;
  int32 version = 3;
  string content = 4;
  google.protobuf.Timestamp created_at = 5;
  // 可选的用户变更说明（NULL 时为空字符串）。
  string change_note = 6;
  // 固定到该版本的提示词数量（由 ListTemplateVersions 填充；
  // 其他位置为 0，应忽略）。
  int32 prompt_count = 7;
}

message UpdateTemplateRequest {
  string template_id = 1;
  string owner_id = 2;
  string title = 3;
  string description = 4;
  Visibility visibility = 5;
  repeated string tags = 6;
  string category = 7;
  string content = 8;
  string language = 9;
  // 新版本的可选变更说明（最长 200 字符）。
  // 内容未变化时忽略（不创建版本）。
  string change_note = 10;
}
```

`CreateTemplateRequest` **保持不变**（开放问题 #3：v1 不带说明）。`ListTemplateVersionsResponse` 不变 —— repeated `TemplateVersion` 消息本身新增字段。用仓库标准的 protoc 命令重新生成 `prompt.pb.go`/`prompt_grpc.pb.go` 并提交（仓库惯例）。

**网关：零改动。** PUT handler 已将整个 JSON body 反序列化为 `UpdateTemplateRequest`（proto 字段存在后 `change_note` 自动流入），versions GET handler 已透传 `page_size`/`page_token`。`protojson` 配合 `UseProtoNames` 会自动以 snake_case 输出 `change_note`/`prompt_count`。

## 2. 数据库 DDL

追加到 **两份** `deployment/init.sql` 与 `deployment/chart/open-prompts/init.sql`（字节一致的块），遵循仓库既有的幂等模式：

```sql
-- -----------------------------------------------------------------------------
-- Template versioning UX: change notes + prompt usage counts
-- -----------------------------------------------------------------------------
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='template_versions' AND column_name='change_note') THEN
        ALTER TABLE template_versions ADD COLUMN change_note TEXT;
        COMMENT ON COLUMN template_versions.change_note IS 'Optional user-provided change note (max 200 chars, enforced in service layer)';
    END IF;
END $$;

-- 支持 ListTemplateVersions 中 prompt_count 的 LEFT JOIN
CREATE INDEX IF NOT EXISTS idx_prompts_version_id ON prompts(version_id);
```

说明：

- 可空、无回填：存量行保持 NULL，渲染为"无说明"。
- 长度上限（≤ 200）在服务层强制，而非 DB CHECK —— 与仓库先例一致（visibility/type、role/status 均为服务层校验）。
- DDL 需先于或随后端一起部署（新的 SELECT 会引用该列）。

## 3. 后端分层

### 3.1 模型（`backend/internal/models/template.go`）

```go
type TemplateVersion struct {
    ID         int32          `json:"id"`
    TemplateID string         `json:"template_id"`
    Version    int32          `json:"version"`
    Content    string         `json:"content"`
    ChangeNote sql.NullString `json:"change_note"` // 可空；proto 中为空字符串
    CreatedAt  time.Time      `json:"created_at"`
    PromptCount int           `json:"prompt_count"` // 读取时计算，不落库
}
```

### 3.2 仓储（`backend/internal/repository/template_version_repository.go`）

所有查询新增 `change_note`；`List` 改为计数 JOIN（开放问题 #1：内联 LEFT JOIN）：

```sql
-- Create（INSERT 增加该列）
INSERT INTO template_versions (template_id, version, content, change_note, created_at)
VALUES ($1, $2, $3, $4, $5) RETURNING id;

-- Get / GetLatest（SELECT 增加该列）
SELECT id, template_id, version, content, change_note, created_at
FROM template_versions WHERE ...;

-- List（新形态：LEFT JOIN + COUNT，按版本主键 GROUP BY）
SELECT tv.id, tv.template_id, tv.version, tv.content, tv.change_note, tv.created_at,
       COUNT(p.id) AS prompt_count
FROM template_versions tv
LEFT JOIN prompts p ON p.version_id = tv.id
WHERE tv.template_id = $1
GROUP BY tv.id
ORDER BY tv.version DESC
LIMIT $2 OFFSET $3;
```

`COUNT(p.id)` 对无提示词的版本返回 0（LEFT JOIN 语义）。`idx_prompts_version_id` 索引支撑该 JOIN。将 `change_note` 扫描进 `sql.NullString`，`prompt_count` 扫描进计算字段。

### 3.3 服务层（`backend/internal/service/prompt.go`）

**`UpdateTemplate`** —— 在既有流程上新增两处：

1. 校验，位于任何写入之前：`if len(req.ChangeNote) > 200 { return nil, status.Errorf(codes.InvalidArgument, "change_note must be at most 200 characters") }`。（按字节长度校验；客户端 maxlength 使用 UTF-16 码元计数，同一字符串下其计数 ≥ 字节数，因此对多字节输入服务端校验是约束更紧的那一个。）
2. 创建新版本时（仅内容变化路径），设置 `ChangeNote: sql.NullString{String: req.ChangeNote, Valid: req.ChangeNote != ""}`。内容未变化路径下说明被忽略且**不**写入现有最新版本（FR-2/FR-18）—— 今天的提前返回已覆盖此点，只需不触碰它。

**`versionModelToProto`** —— 映射新字段：

```go
changeNote := ""
if m.ChangeNote.Valid {
    changeNote = m.ChangeNote.String
}
return &pb.TemplateVersion{
    ...,
    ChangeNote:  changeNote,
    PromptCount: int32(m.PromptCount),
}
```

**`ListTemplateVersions`** —— 无逻辑变更；仓储现在返回计数，`versionModelToProto` 负责映射。`GetTemplate` 的 `latest_version` 自动获得 `change_note`（其 `GetLatest` SELECT 现在包含该列）；`prompt_count` 在那里为 0，文档标注为可忽略。

**恢复无需后端工作** —— 它就是一次普通的 `UpdateTemplate` 调用：内容为旧版本内容，说明为规范英文字符串 `Restored from v{n}`（存储数据，渲染时绝不翻译；NFR-5/D-7）。

### 3.4 网关（`backend/cmd/server/main.go`）

**零改动。**（已验证：PUT handler 反序列化整个 body；versions GET handler 透传分页参数；`protojson` 自动输出新字段。）

## 4. 前端设计

### 4.1 文件布局

```
frontend/src/
├── components/
│   ├── VersionHistoryPanel.js   # 面板 + 内联 VersionRow
│   ├── DiffModal.js
│   ├── RestoreConfirmModal.js
│   ├── ChangeNoteInput.js
│   └── VersionSelect.js         # 修改：说明摘要副行
├── pages/TemplateDetails.js     # 修改：开关、分页、恢复、diff 接线
├── utils/
│   ├── diff.js                  # LCS 行 diff（新增）
│   └── time.js                  # formatRelativeTime（新增，共享）
└── locales/{en,zh}/translation.json  # versioning.* + time.* 键
```

### 4.2 Diff 工具（`utils/diff.js`）

手写按行 LCS（开放问题 #2），归一化输出：

```js
// diffLines(aText, bText) => [{ type: 'add'|'remove'|'context', text, aLine, bLine }]
```

- 按 `\n` 切分，在行上构建 LCS 表（内容 ≤ 32 KB → 行数很小；按行 O(n·m) 毫无压力）。
- 回溯表生成 remove（带 `aLine`）、add（带 `bLine`）、context（两者）。
- 导出 `diffStats(rows)` → `{ added, removed }` 供统计芯片使用。
- 调用处用 try/catch 包裹；任何抛错时弹窗渲染并排回退（NFR-6）。

### 4.3 TemplateDetails 状态扩展

```js
const [isHistoryOpen, setIsHistoryOpen] = useState(false);
const [historyPageToken, setHistoryPageToken] = useState('');
const [diffState, setDiffState] = useState(null); // 弹窗打开时为 { aId, bId }
const [editNote, setEditNote] = useState('');      // 编辑模式下的变更说明
```

- **版本拉取：** 将 `listTemplateVersions(id, { page_size: 100 })` 替换为 `page_size: 20`；"加载更多"通过 `next_page_token` 追加。下拉消费累积后的列表（UX 不变）。
- **选择：** 保持单一 `setSelectedVersionId` 处理器，作为面板行、下拉变更与恢复后选择的唯一事实来源。
- **保存流程（`handleSaveContent`）：** 在更新 payload 中包含 `change_note: editNote`；成功后清空 `editNote`。`handleSaveMetadata` **不**发送说明（它发送最新内容；不创建版本）。
- **恢复流程：** 打开带源版本的 `RestoreConfirmModal`；确认后调用 `updateTemplate(template.id, { ...currentMetadata, content: version.content, change_note: \`Restored from v${version.version}\` })`；成功后重新拉取版本第 1 页、选中 `new_version.id`、toast `versioning.toast.restored`。悲观式（转圈、按钮禁用）—— 不做乐观版本插入。
- **写入后刷新：** 保存与恢复都重新拉取第 1 页，使编号与最新徽标以服务端为准。

### 4.4 组件

**VersionHistoryPanel** —— 头部（"版本历史" + 关闭）；行按最新在前：`v{n}`（等宽）、最新徽标（仅最新行）、说明或"无说明"（斜体）、相对时间（`title` 中为绝对时间）、元信息行"X 字符 · Y 行 · Z 提示词"（`prompt_count` 缺失时按响应隐藏使用数段）、比较 + 恢复操作（最新行与非 owner 隐藏恢复）。行点击选中该版本。按钮行组成 `<ul>`；操作为带 `aria-label` 的独立按钮（"比较 v4"、"恢复 v3"）。状态：加载中 4 行骨架屏；单版本提示；错误 + 重试。

**DiffModal** —— A（旧）/ B（新）`VersionSelect` 风格选择器、交换按钮、统计芯片"+N −M 行"、统一 diff 主体（行号、符号标记、内容；删除 = danger-soft 背景 + `−`，新增 = success-soft 背景 + `+`，上下文无修饰；等宽 13px `pre-wrap`，最大高度 60vh）、复制 diff、关闭。选择器变更或交换时重新计算 diff；A/B 归一化为旧→新。回退：并排原始内容双栏 + 提示。复用现有 Modal `size="lg"`、焦点陷阱、Escape。

**RestoreConfirmModal** —— 文案按 UI/UX §5.5（"将 v{n} 恢复为新版本（v{next}）？……说明 \"Restored from v{n}\"。"）；取消 / "恢复 v{n}"；进行中两者均禁用。

**ChangeNoteInput** —— 标签"变更说明（可选）"，输入 maxlength 200，实时计数"n/200"（`aria-live="polite"`，180+ 警告色），占位符"例如：修正语气漂移"。位于内容 textarea 与操作按钮之间。

**VersionSelect（增强）** —— 选项副行在 `change_note` 存在时于既有日期下方显示说明摘要（前约 40 字符，省略号）；其他行为不变。

### 4.5 i18n 键（两种语言）

`versioning.*` 命名空间逐字取自 UI/UX 文档 §9（EN 与 ZH 的 JSON 块在那里已给出 —— 原样复制），外加共享的 `time.*` 键（`justNow`、`minutesAgo`、`hoursAgo`、`daysAgo`，若通知功能尚未引入）。存储的说明与自动生成的 `Restored from v{n}` 字符串是数据，在两种语言下均原样渲染。

### 4.6 API 客户端（`services/api.js`）

**零改动。** `updateTemplate(id, data)` 与 `listTemplateVersions(id, params)` 是通用透传；新字段搭载既有 payload。

## 5. 错误矩阵

| 条件 | gRPC 码 | HTTP | 用户可见表现 |
|---|---|---|---|
| `change_note` > 200 字符 | `InvalidArgument` | 400 | 错误 toast（`template_details.error_update`）；客户端 maxlength 已拦截大多数情况 |
| 模板不存在（更新/恢复） | `NotFound` | 404 | 错误 toast |
| 非 owner（恢复） | `PermissionDenied` | 403 | 错误 toast；UI 对非 owner 本就隐藏恢复 |
| 未认证（更新/恢复） | `Unauthenticated` | 401 | 全局 401 → `/login` |
| 版本列表失败 | `Internal` | 500 | 面板错误状态 + 重试 |
| Diff 计算抛错（客户端） | — | — | 并排回退 + 提示（NFR-6） |
| `prompt_count` 缺失（旧后端） | — | — | 隐藏使用数列（NFR-6） |

## 6. 部署

- DDL 块（§2）追加到两份 `init.sql` 副本；幂等；无回填。
- 无环境变量、无 compose/helm 变更、无新服务。
- 顺序：DDL 先于或随后端镜像一起部署（新的 SELECT 会引用 `change_note`）。

## 7. SDK / CLI

无需变更。python-sdk 以通用方式封装 create/update/list 面；增量响应字段对它透明。（若 SDK 中有枚举字段的类型化模型，在那里补 `change_note`/`prompt_count` 是可选且非破坏性的 —— 本功能不要求。）

## 8. 测试策略

### 8.1 FVT（`tests/fvt.py`）—— 开发者

新增 `test_template_versioning()`（或扩展现有版本测试）：

1. 创建模板 → 以 `change_note: "first revision"` 更新 → `new_version.change_note == "first revision"`，版本号递增。
2. 列出版本 → 两个版本都带 `change_note`（v1 为空）与 `prompt_count`（提示词存在前为 0）。
3. 创建 2 个固定到 v1 的提示词 → 再次列出 → v1 `prompt_count == 2`，v2 `prompt_count == 0`。
4. 以 201 字符的 `change_note` 更新 → 400。
5. 以内容不变 + 说明更新 → 无新版本（数量不变），说明未存储到最新版本。
6. 恢复：以 v1 内容 PUT 且 `change_note: "Restored from v1"` → 创建新 v3；列表显示全部 3 个版本完好（只追加）；`latest` 别名解析到 v3（经别名端点）；v3 说明为规范字符串。
7. 向后兼容：不带 `change_note` 更新 → 仍正常，说明为空。

### 8.2 后端单元测试

- `UpdateTemplate`：说明长度校验（200 通过 / 201 拒绝）；说明仅在内容变化路径持久化；内容不变路径返回最新版本且不写入说明。
- `versionModelToProto`：NULL → 空字符串；计数映射。
- 仓储 `List` SQL 形态（mock 行）：计数 JOIN 对无提示词版本返回 0。

### 8.3 E2E（Nightwatch，test-agent）

| E2E | AC |
|---|---|
| 带说明保存 → 说明出现在面板 + 下拉摘要 | AC-1 |
| 面板：最新在前、最新徽标、说明/时间/长度/使用数、每页 20 加载更多 | AC-2 |
| 行点击切换预览（与下拉一致） | AC-3 |
| 比较：高亮、行号、统计、交换 | AC-4 |
| 恢复：确认 → 新版本 + 规范说明、列表刷新、新版本选中 | AC-5 |
| 非 owner：面板 + 比较、无恢复；匿名看公开模板：只读 | AC-6 |
| 说明 > 200 客户端拦截（maxlength + 计数） | AC-7 |
| 仅元数据保存不创建版本；别名流程不变 | AC-8 |
| EN/ZH 键对齐，所有字符串已翻译 | AC-9 |
| 模板页回归：查看/编辑/fork/点赞/收藏/提示词生成不变 | AC-10 |
| `prompt_count` 缺失时隐藏使用数列（mock） | AC-11 |

## 9. 实现顺序（建议）

1. DDL（两份 `init.sql` 副本）+ 应用到运行中的栈。
2. Proto 变更 + 重新生成 `prompt.pb.go`/`prompt_grpc.pb.go`。
3. 模型 + 仓储（Create/Get/GetLatest 增列；List 计数 JOIN）。
4. 服务层：`UpdateTemplate` 校验 + 说明持久化；`versionModelToProto` 映射。
5. 前端工具：`diff.js`、`time.js`。
6. `ChangeNoteInput` + 保存流程接线；`VersionSelect` 摘要。
7. `VersionHistoryPanel` + `TemplateDetails` 分页重构。
8. `DiffModal`、`RestoreConfirmModal` + 恢复流程。
9. i18n 键（两种语言）；面板/diff/徽标的 SCSS。
10. FVT + 单元测试；`make lint`、`make ut`、`make fvt`、`npm run lint` 全绿。

## 10. 开发者约束（必须遵守）

1. **绝不提交对 `backend/server` 的改动**（被跟踪的 26 MB 二进制）—— 提交前若显示 modified，先 `git checkout -- backend/server`。
2. **仅增量：** 不新增端点、不新增表、无破坏性 proto/REST 变更；`CreateTemplateRequest` 不动（开放问题 #3）。
3. 分层：proto → 网关（此处零改动）→ 服务 → 仓储 → 模型；不新增顶层包。
4. `change_note` ≤ 200 字符在服务层强制（`InvalidArgument`）；内容未变化时忽略（不存储）。
5. 恢复 = 既有 `UpdateTemplate` + 旧内容 + 规范英文说明 `Restored from v{n}` —— 绝不新增端点、绝不破坏性；历史保持只追加。
6. `prompt_count` 通过仓储 `List` 中的内联 LEFT JOIN + COUNT（开放问题 #1）；无提示词版本为 0；前端容忍字段缺失（NFR-6）。
7. Diff 是 `utils/diff.js` 中的客户端 LCS（开放问题 #2）—— **不新增 npm 依赖**；失败时走并排回退。
8. i18n：每个新增用户可见字符串进 **en** 与 **zh** 两个语言包（`versioning.*`、`time.*`）；存储的说明原样渲染（渲染时绝不翻译）。
9. `init.sql` 变更幂等并同步到 helm chart 副本；DDL 先于/随后端一起部署。
10. 规范提交、英文；新增 Go 文件单元测试覆盖率 ≥ 80%；推送前所有 CI 门禁全绿。
