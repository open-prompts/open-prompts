# 模板导入/导出 — 详细设计

| | |
|---|---|
| **功能 ID** | FE-010 |
| **版本** | 1.0 |
| **状态** | 已批准 |
| **作者** | architect-agent |
| **日期** | 2026-10-12 |
| **架构文档** | `docs/architecture/import-export-architecture-zh-CN.md` |

## 1. 后端改动

唯一的后端改动是增强管理端模板详情读取路径。不涉及面向用户的端点、DDL 或服务层变更。

### 1.1 Proto 差异（`backend/api/proto/v1/admin.proto`）

```protobuf
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
  string category = 13;    // 新增 — 管理员导出对齐 FR-2 所需
}

message AdminTemplateVersion {
  int32 id = 1;
  int32 version = 2;
  string content = 3;
  google.protobuf.Timestamp created_at = 4;
  string alias_name = 5;   // 无别名指向该版本时为 ""
  string change_note = 6;  // 新增 — 版本无变更说明时为 ""
}
```

字段号 13 / 6 是核实后的下一个空闲槽位。两者均为 `string` 标量——不涉及 protojson int64 问题。

### 1.2 模型差异（`backend/internal/models/admin.go`）

```go
type AdminTemplate struct {
    // ... 既有字段不变 ...
    Category string `json:"category"` // 新增
}

type AdminTemplateVersion struct {
    // ... 既有字段不变 ...
    ChangeNote sql.NullString `json:"change_note"` // 新增
}
```

### 1.3 仓储差异（`backend/internal/repository/admin_repository.go`）

```go
// adminTemplateColumns：追加 t.category
const adminTemplateColumns = `t.id, t.owner_id, COALESCE(u.display_name, ''), t.title, t.description, t.visibility, t.type, t.language, t.tags, t.like_count, t.favorite_count, t.category, t.created_at, t.updated_at`
```

`GetTemplateWithVersions` 的版本查询：SELECT 增加 `tv.change_note` 并扫描到 `v.ChangeNote`。`templates.category` 与 `template_versions.change_note` 列均已存在（change_note 由 template-versioning 迁移幂等添加，见 `init.sql` ALTER）。

### 1.4 服务差异（`backend/internal/service/admin.go`）

- `templateToSummary`：增加 `Category: t.Category`。
- `GetTemplate` 版本映射：`ChangeNote: version.ChangeNote.String`（NULL → `""`，与 `alias_name` 处理一致）。

### 1.5 网关

无改动。`/api/v1/admin/templates/{id}`（GET，`requireAdmin`）已用 protojson 序列化 `GetAdminTemplateResponse`；新字段自动透出。

## 2. 前端工具 — `frontend/src/utils/templateExport.js`

纯函数，不引入 React；可独立做单元测试（参照 `utils/diff.js` + `diff.test.js` 模式）。

### 2.1 常量

```js
export const EXPORT_FORMAT = 'open-prompts-template';
export const EXPORT_FORMAT_VERSION = 1;
export const MAX_IMPORT_FILE_SIZE = 1024 * 1024; // 1 MB
export const MAX_IMPORT_VERSIONS = 50;
export const MAX_CHANGE_NOTE_LENGTH = 200;
export const MAX_TITLE_LENGTH = 200;
export const MAX_TAGS = 20;
export const MAX_TAG_LENGTH = 50;
```

### 2.2 `parseTemplateExport(file: File): Promise<ParsedExport>`

返回 `{ data, warnings }`：`data` 为校验通过的导出对象（版本时间正序，可能已截断到 50），`warnings` 为 `{ code, params }` 数组（当前仅有 `truncated_versions`）。任何 FR-11 违规抛出 `ImportValidationError`（`Error` 子类，携带 `code` 与可选 `params`）。

校验顺序（每步短路）：

| 步骤 | 检查 | 错误码 |
|---|---|---|
| 1 | `file.size > MAX_IMPORT_FILE_SIZE` | `too_large` |
| 2 | `await file.text()` 后 `JSON.parse`（try/catch） | `not_json` |
| 3 | `data.format === EXPORT_FORMAT` 且 `data.template` 为对象 | `bad_format` |
| 4 | `data.format_version === EXPORT_FORMAT_VERSION` | `unsupported_version`（params: `{ version }`） |
| 5 | `template.title` 为非空字符串 | `missing_field:title` |
| 6 | `Array.isArray(template.versions)` 且 `versions.length >= 1` | `missing_field:versions` |
| 7 | 每个版本的 `content` 为非空字符串 | `missing_field:versions[{i}].content` |
| 8 | `template.title.length <= MAX_TITLE_LENGTH` | `invalid_field:title` |
| 9 | `template.tags`（若存在）≤ `MAX_TAGS` 个、每个 ≤ `MAX_TAG_LENGTH` 字符 | `invalid_field:tags` |
| 10 | `versions.length > MAX_IMPORT_VERSIONS` → 保留最后 50 个，追加警告 `truncated_versions`（params: `{ kept: 50, total }`） | —（警告而非错误） |

对返回 `data` 的归一化：

- `description` → `String(description ?? '')`
- `tags` → 字符串数组、去重、缺失时为空数组
- `category` → 字符串（可为 `''`；评审表单要求必须选择）
- `language` → 字符串，缺失时默认 `'en'`
- `visibility` → 原样保留用于展示；**导入方忽略**（强制 private）
- `type` → 原样保留用于展示；**导入方忽略**（强制 `user`）
- `versions` → 按 `version` 升序排序；每个 `change_note` 归一为 `String(change_note ?? '')`
- `versions[i].created_at` → 仅信息性，绝不回放

### 2.3 `buildTemplateExport(template, versions): string`

- `template`：来自 `getTemplate`（用户路径）或 `getAdminTemplate` 摘要 + description（管理路径）的字段集——调用方先把管理端结构拍平成同一字段集。
- `versions`：时间正序的 `{ version, content, change_note, created_at }` 数组。
- 输出：`JSON.stringify(obj, null, 2) + '\n'`，`obj` 遵循格式版本 1 schema（架构文档 §5）。仅按字段白名单构造——结构上不可能混入所有者身份。
- `exported_at`：`new Date().toISOString()`；`source`：`'open-prompts'`。

### 2.4 `downloadJSON(filename: string, text: string): void`

```js
const blob = new Blob([text], { type: 'application/json' });
const url = URL.createObjectURL(blob);
const a = document.createElement('a');
a.href = url;
a.download = filename;
document.body.appendChild(a);
a.click();
a.remove();
URL.revokeObjectURL(url);
```

### 2.5 `slugifyTitle(title: string, id: string): string`

1. `title.toLowerCase().normalize('NFKD')`，去除组合附加符号，非 `[a-z0-9]+` 连续段替换为 `-`，修剪首尾 `-`。
2. 上限 60 字符（尽量在 `-` 边界截断）。
3. `id8 = id.replace(/-/g, '').slice(0, 8)`。
4. slug 为空时回退 `template`；返回 `` `${slug}-${id8}` ``（如 `my-prompt-template-a1b2c3d4`）。

### 2.6 `fetchVersionsChronological(templateId): Promise<Version[]>`

用户导出路径的辅助函数：循环 `listTemplateVersions(templateId, { page_size: 50, page_token })` 直到 `next_page_token` 为空；拼接 `versions`；反转。（API 返回最新在前；文件必须最早在前。）

## 3. 前端组件

### 3.1 `components/ExportButton.js`

```jsx
props: { template, dataSource? }   // dataSource: 'user'（默认）| 'admin'
state: exporting: boolean
```

- 渲染 `.btn-ghost .btn-sm`，`⬇` 图标 + `t('import_export.export')`；`exporting` 时 `disabled` + 加载动画。
- **用户路径**（`TemplateDetails.js`，仅 `isOwner` 时挂载）：`Promise.all([getTemplate(template.id), fetchVersionsChronological(template.id)])` → `buildTemplateExport` → `downloadJSON(slugifyTitle(...))` → 成功 toast `t('import_export.export_success', { title, count })`。
- **管理路径**（`admin/TemplateDetailPage.js`）：`getAdminTemplate(template.id)` → 拍平 `{ ...res.data.template, description: res.data.description, versions: [...res.data.versions].reverse() }` → 同样的构造/下载/toast。
- 失败：错误 toast `t('import_export.export_failed')`；按钮恢复。所有异常均被捕获，无未处理 rejection。

### 3.2 `components/ImportButton.js`

- `.btn-ghost .btn-sm`，`⬆` 图标 + `t('import_export.import')`；打开 `ImportModal`。
- 挂载在 `Home.js` 的 `home-search-row`（`SortDropdown` 之后），仅 `isAuthenticated` 时渲染（Home 已从 auth state 取该值）。

### 3.3 `components/ImportModal.js`

两步状态机：`step: 'pick' | 'review'`。

**第 1 步 — 选择文件**

- `DropZone`：`--border-strong` 虚线边框、`--radius-12`；拖入时 `--accent` 边框 + `--surface-2` 填充；隐藏 `<input type="file" accept=".json,application/json">`；`tabIndex={0}`、`role="button"`，Enter/Space 打开文件选择器（NFR-5/AC-11）。
- `onFile(file)`：`setParsing(true)` → `parseTemplateExport(file)`：
  - 抛错 → `setValidationError(err)`（错误槽渲染映射后的 i18n 文案；绝不显示原始异常），停留在第 1 步（FR-14）。
  - 成功 → `setParsed({ data, warnings })`，进入第 2 步。
- 拖入非 `.json` 文件：抖动动画（尊重 `prefers-reduced-motion`）+ `bad_format` 错误。
- 取消/Escape 关闭（第 1 步无可丢弃内容）。

**第 2 步 — 评审**

- 摘要行：`t('import_export.review_summary', { file, count })`；若 `warnings` 含 `truncated_versions`，追加截断警告文案。
- 表单字段（复用 `Field`/`TagInput` 组件与 `create_template.*` 标签，NFR-1）：
  - 标题* — 预填文件值；进入时做**冲突检查**：`getTemplates({ owner_id: user.id, page_size: 100 })`，与首页结果做大小写不敏感比较；命中则显示 `t('import_export.title_collision')` + "使用建议标题"按钮，将标题设为 `` `${title} ${t('import_export.suggested_suffix')}` ``。非阻断（FR-12/AC-8）。
  - 分类* — 文件的 category 存在于 `getCategories()` 结果中则预填；否则留空（用户必须选择；与创建表单相同的必填校验）。
  - 可见性 — 单选，**默认 Private**，无视文件值（FR-10）；用户可改为 Public。
  - 语言 — 预填文件值。
  - 标签 — 通过 `TagInput` 预填。
  - 描述 — 预填。
  - 内容* — 文本域，预填**最新**版本内容（可编辑；它作为回放最后一步的内容来源）。
- 返回 → 回到第 1 步并保留解析数据（无需重新拖放）。
- 表单有未保存修改时 Escape/取消 → 确认放弃提示。
- 提交（`t('import_export.confirm')`，运行中显示 spinner）：

```js
const { data } = parsed;
const versions = data.template.versions;           // 时间正序，≤ 50
const base = {
  title, description, category, tags, language,    // 评审后的值
  visibility,                                      // 评审后的值（默认 'private'）
  type: 'user',                                    // 强制
};
// 1. 用最早版本内容创建（架构文档 §7.1）
const created = await createTemplate({ ...base, content: versions[0].content });
const newId = created.data.template.id;
// 2. 回放 v2..vN，最早 → 最新
for (let i = 1; i < versions.length; i++) {
  await updateTemplate(newId, {
    ...base,
    content: i === versions.length - 1 ? reviewedContent : versions[i].content,
    change_note: versions[i].change_note.slice(0, 197),  // 防御性截断（服务层上限 200）
  });
}
// 3. 跳转 + toast
navigate(`/templates/${newId}`);
addNotification({ kind: 'success', subtitle: t('import_export.import_success') });
```

  - 说明：最后一次回放调用的内容使用评审表单中的 `content`，评审阶段对内容的编辑因此生效。
  - 回放中途失败：停止，toast `import_partial`（新增 key，params `{ imported, total }`），弹窗保持打开并显示错误；已创建的模板保留（私有），用户可手动前往。
  - 创建失败：表单级错误（与 `CreateTemplateModal` 相同的处理）。

### 3.4 挂载点（精确位置）

| 文件 | 改动 |
|---|---|
| `pages/TemplateDetails.js` | 所有者操作区（与 Edit/Fork 并列）：`{isOwner && <ExportButton template={template} />}` |
| `pages/admin/TemplateDetailPage.js` | 头部操作区：`<ExportButton template={template} dataSource="admin" />` |
| `pages/Home.js` | `<div className="home-search-row">` 末尾追加 `{isAuthenticated && <ImportButton />}` |

## 4. i18n Key 清单

命名空间 `import_export.*`（新增），同时加入 `frontend/src/locales/en/translation.json` 与 `frontend/src/locales/zh/translation.json`（已核实的布局）。评审表单字段标签复用既有 `create_template.*` key。

**EN**

```json
{
  "import_export": {
    "export": "Export",
    "export_success": "Exported \"{{title}}\" ({{count}} versions)",
    "export_failed": "Export failed. Please try again.",
    "import": "Import",
    "import_title": "Import template",
    "drop_hint": "Drag and drop a .json file here, or",
    "browse": "Browse files",
    "drop_note": "Only open-prompts template exports (.json, max 1 MB) are supported.",
    "error_not_json": "This file is not a valid JSON file.",
    "error_too_large": "File is too large. Maximum size is 1 MB.",
    "error_bad_format": "This file is not an open-prompts template export.",
    "error_unsupported_version": "This export uses a newer format version ({{version}}) that this app cannot read.",
    "error_missing_field": "The file is missing a required field: {{field}}.",
    "error_invalid_field": "The file has an invalid value for: {{field}}.",
    "warning_truncated_versions": "Only the most recent {{kept}} of {{total}} versions will be imported.",
    "review_summary": "{{file}} — {{count}} versions will be imported as sequential versions.",
    "title_collision": "You already have a template with this title.",
    "suggested_suffix": "(imported)",
    "use_suggested": "Use suggested title",
    "confirm": "Import template",
    "import_success": "Template imported successfully.",
    "import_partial": "Imported {{imported}} of {{total}} versions. The template was created; you can retry or edit it manually."
  }
}
```

**ZH**

```json
{
  "import_export": {
    "export": "导出",
    "export_success": "已导出“{{title}}”（{{count}} 个版本）",
    "export_failed": "导出失败，请重试。",
    "import": "导入",
    "import_title": "导入模板",
    "drop_hint": "将 .json 文件拖放到此处，或",
    "browse": "浏览文件",
    "drop_note": "仅支持 open-prompts 模板导出文件（.json，最大 1 MB）。",
    "error_not_json": "该文件不是有效的 JSON 文件。",
    "error_too_large": "文件过大，最大为 1 MB。",
    "error_bad_format": "该文件不是 open-prompts 模板导出文件。",
    "error_unsupported_version": "此导出文件使用了较新的格式版本（{{version}}），当前应用无法读取。",
    "error_missing_field": "文件缺少必填字段：{{field}}。",
    "error_invalid_field": "文件的以下字段值无效：{{field}}。",
    "warning_truncated_versions": "将仅导入最近 {{kept}} 个版本（共 {{total}} 个）。",
    "review_summary": "{{file}} — 将导入 {{count}} 个版本（按顺序生成）。",
    "title_collision": "你已存在同名模板。",
    "suggested_suffix": "（导入）",
    "use_suggested": "使用建议标题",
    "confirm": "导入模板",
    "import_success": "模板导入成功。",
    "import_partial": "已导入 {{imported}} / {{total}} 个版本。模板已创建；你可以重试或手动编辑。"
  }
}
```

相对 UI/UX 设计文档新增的 key：`error_invalid_field`、`warning_truncated_versions`、`suggested_suffix`、`import_partial`（均由架构文档中的决策引入）。

## 5. 错误矩阵

| 场景 | 层级 | 错误码 / HTTP | 用户可见结果 |
|---|---|---|---|
| 文件 > 1 MB | 客户端 | `too_large` | 第 1 步内联错误，停留第 1 步 |
| 非 JSON 文件 | 客户端 | `not_json` | 第 1 步内联错误 |
| `format` 错误 / 缺 `template` | 客户端 | `bad_format` | 第 1 步内联错误 |
| `format_version` ≠ 1 | 客户端 | `unsupported_version` | 第 1 步内联错误，含版本号 |
| 缺标题 / 版本 / 内容 | 客户端 | `missing_field:{field}` | 第 1 步内联错误，指明字段 |
| 标题 > 200 / 标签 > 20 / 单标签 > 50 | 客户端 | `invalid_field:{field}` | 第 1 步内联错误 |
| > 50 个版本 | 客户端 | 警告 `truncated_versions` | 第 2 步摘要警告，导入继续 |
| 标题冲突 | 客户端 | — | 第 2 步非阻断警告 + 建议标题 |
| 创建失败（校验） | 服务端 | 400（`writeError`） | 第 2 步表单错误 |
| 创建失败（认证） | 服务端 | 401 | 全局 401 处理（跳转登录） |
| 回放更新失败 | 服务端 | 4xx/5xx（`writeError`） | `import_partial` toast；保留部分历史 |
| 导出拉取失败 | 服务端 | 4xx/5xx | `export_failed` toast，按钮复位 |
| 非管理员调用管理导出 | 服务端 | 403（`requireAdmin`） | 不可达——按钮仅在管理台渲染 |

## 6. 测试计划

### 6.1 单元 — `utils/templateExport.test.js`

- 往返：`buildTemplateExport` → `parseTemplateExport`（经 `new File([json])`）字段全保留。
- §2.2 的 10 个校验步骤逐一用最小夹具覆盖。
- 截断：60 版本文件 → 保留 50、警告存在、保留的是最后 50 个。
- `slugifyTitle`：普通标题；100 字符标题截到 60；纯中日韩标题 → `template-{id8}`；空标题。
- `change_note` 归一化：`null` → `''`；300 字符 note → 回放载荷构造时为 197 + `…`。

### 6.2 FVT — 后端管理服务

- `GetAdminTemplate` 摘要返回 `category`、逐版本返回 `change_note`；NULL change_note → `""`。
- 既有管理端模板测试保持通过（增量字段）。

### 6.3 E2E — 往返

1. 所有者创建模板、两次编辑内容（3 个版本带 change note）→ 导出 → 文件下载，含 3 个时间正序版本。
2. 第二账号：Home → 导入 → 文件 → 评审显示字段、visibility=Private → 确认 → 落到详情页 → 版本历史显示 3 个版本、内容/说明一致、visibility private、type user。
3. 非所有者查看公开模板：无 Export 按钮。
4. 管理员：`/admin/templates/:id` → 导出 → 结构一致（与用户导出 diff，忽略 `exported_at`）。
5. 损坏文件（改名的 `.png`）、2 MB 文件、`format_version: 99` 文件 → 第 1 步给出具体错误，不崩溃。

## 7. 实施顺序

1. 后端：proto 字段 → proto 生成 → 模型/仓储/服务映射 → FVT。
2. `utils/templateExport.js` + 单元测试。
3. `ExportButton`（用户 + 管理端挂载）。
4. `ImportButton` + `ImportModal` + Home 挂载。
5. i18n EN/ZH。
6. E2E 往返；明暗主题视觉检查（AC-12）。

## 8. 开发者约束

- **不得**为本功能新增任何 REST 端点、RPC 或 DDL。
- **不得**改动 `UpdateTemplate` 语义；回放依赖当前的仅追加行为。
- **不得**在导出构造中包含所有者身份——仅按字段白名单。
- **不得**信任文件的 `visibility`/`type`——导入强制 `private`/`user`。
- 回放顺序为**最早优先**；UI/UX 设计文档注记 6 已被取代（见架构文档 §7.1）。
- `/versions` 端点未鉴权问题的加固（架构文档 §9）是**独立跟进项**——不要并入本功能的 PR。
- Proto 字段号：`AdminTemplateSummary.category = 13`、`AdminTemplateVersion.change_note = 6`——不得重新编号。
