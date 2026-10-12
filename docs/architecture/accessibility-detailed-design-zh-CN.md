# 无障碍（WCAG 2.1 AA）— 详细设计

| | |
|---|---|
| **功能 ID** | accessibility |
| **版本** | 1.0 |
| **状态** | 已批准 |
| **作者** | 架构 Agent |
| **上游** | [架构设计](accessibility-architecture-zh-CN.md) |
| **下游** | 开发者 Agent |

## 1. 范围与文件

仅前端。涉及文件：

| 文件 | 变更 |
|---|---|
| `frontend/src/index.scss` | focus-visible 规则、`--text-3` token、forced-colors、skip-link、点击区域 |
| `frontend/src/components/ui/Modal.jsx` | 焦点圈定、焦点恢复、`aria-labelledby` |
| `frontend/src/context/NotificationContext.js` | live region、错误 `role="alert"` |
| `frontend/src/components/PromptCard.js` / `.scss` | 覆盖按钮、`aria-pressed`、点击区域 |
| `frontend/src/components/Sidebar.js` / `.scss` | 按钮语义、`aria-current` |
| `frontend/src/components/Header.js` | 菜单模式、标签 |
| `frontend/src/components/Layout.js` | SkipLink、`main#main-content` |
| `frontend/src/components/SkipLink.jsx` | **新增** |
| `frontend/src/utils/a11y.js` | **新增** —— `usePageAnnouncement` |
| `frontend/src/i18n.js` | `documentElement.lang` 同步 |
| `frontend/src/pages/TemplateDetails.js` | h1、`aria-pressed`、标签 |
| `frontend/src/pages/{Home,Login,Register,Profile}.js` | h1、autocomplete、错误聚焦 |
| `frontend/src/locales/{en,zh}/translation.json` | 新增 key（§5） |
| `frontend/src/a11y/*.test.js` | **新增** —— axe 门禁 |
| `docs/testing/keyboard-smoke-test.md` | **新增** |
| `.github/workflows/ci.yml` | `frontend-a11y` job |
| `frontend/package.json` | `axe-core` devDependency、`test:a11y` script |

## 2. 全局样式（SCSS）

### 2.1 焦点可见性（替换输入框的 `outline: none`）

```scss
// 通用焦点指示器 —— 2px accent 轮廓、2px 偏移（FR-17/18）
:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
}

// 输入框此前完全移除了轮廓（.input:focus { outline: none }）；
// 现在保留柔和光环并增加可见轮廓。
.input:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
}

// 不得在无替代的情况下移除指示器
.input:focus { /* 保留现有 box-shadow 光环；不再抑制 outline */ }
```

### 2.2 对比度 token（规范值 —— 见架构设计 §5）

```scss
:root {                      // 暗色主题（默认）
  --text-3: #8b92a8;         // 原 #6e768c（3.79:1）→ --surface 上 5.55:1，所有表面 ≥4.55:1
}
html.light-theme {
  --text-3: #626b7a;         // 原 #8b92a8（3.10:1）→ --surface 上 5.38:1，所有表面 ≥4.64:1
}
```

> 注：UI/UX 设计 §5.8 为亮色主题提议 `#767e93`；实测白底对比度 4.06:1 —— **不满足 AA**。采用值为 `#626b7a`（记录于架构设计 §1 已解决开放问题 #3）。

### 2.3 跳转链接

```scss
.skip-link {
  position: absolute;
  left: -9999px;
  top: 0;
  z-index: 1000;
  background: var(--accent);
  color: #fff;
  padding: 8px 16px;
  border-radius: 0 0 8px 0;
  font-weight: 600;
}
.skip-link:focus {
  left: 0;                    // 仅在聚焦时可见
}
```

### 2.4 强制颜色模式（Windows 高对比度）

```scss
@media (forced-colors: active) {
  .prompt-card { border: 1px solid CanvasText; }
  .btn { border: 1px solid ButtonText; }
  .stat-item { forced-color-adjust: none; }
  .skip-link { border: 1px solid ButtonText; }
}
```

### 2.5 点击区域

```scss
// 点赞/收藏/关闭/菜单图标最小 24×24 CSS px（触屏建议 44px）
.stat-item {
  min-width: 24px;
  min-height: 24px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
```

## 3. 组件变更

### 3.1 `Modal.jsx` —— 焦点圈定 + 恢复 + 标注

```jsx
// 打开时：保存触发器、聚焦对话框、设置 aria-labelledby
useEffect(() => {
  if (!open) return;
  restoreRef.current = document.activeElement;          // (a) 保存触发器
  const dlg = dialogRef.current;
  dlg.focus();                                            // (b) 聚焦对话框
  return () => {                                          // (c) 关闭/卸载时恢复
    const el = restoreRef.current;
    if (el && document.contains(el)) el.focus();
  };
}, [open]);

// Keydown：Escape（已有）+ Tab 圈定
const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

function handleKeyDown(e) {
  if (e.key === 'Escape') { onClose(); return; }
  if (e.key !== 'Tab') return;
  const items = dialogRef.current.querySelectorAll(FOCUSABLE);
  if (items.length === 0) { e.preventDefault(); return; }
  const first = items[0], last = items[items.length - 1];
  if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
  else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
  // 其余情况：默认 Tab 行为保持在对话框内
}
```

标记：`<div role="dialog" aria-modal="true" aria-labelledby={titleId} tabIndex={-1}>`，标题为 `<h3 id={titleId}>`（无标题时回退 `aria-label`）。保留现有 `handleKeyDownRef` 模式（避免每次渲染重新注册监听器）。

### 3.2 `NotificationContext.js` —— live region

```jsx
<div
  role="region"
  aria-label={t('a11y.notifications_region')}
  aria-live="polite"
  className="toast-container"
>
  {toasts.map(t => (
    <div key={t.id} role={t.type === 'error' ? 'alert' : 'status'} className={`toast toast-${t.type}`}>
      ...
      <button aria-label={t('common.close_notification')} ... />
    </div>
  ))}
</div>
```

toast 永不抢焦点（保留现有行为）。

### 3.3 `PromptCard.js` —— 覆盖按钮 + `aria-pressed`

```jsx
<div className="prompt-card">
  <button
    type="button"
    className="prompt-card-overlay"
    aria-label={template.title}
    onClick={() => navigate(`/templates/${template.id}`)}
  />
  <div className="prompt-card-body"> ... 标题、描述、元信息 ... </div>
  <div className="prompt-card-stats">
    <button
      type="button"
      className="stat-item stat-like"
      aria-pressed={!!template.is_liked}
      aria-label={t('template_details.like')}
      onClick={(e) => { e.stopPropagation(); handleLike(); }}
    />
    <button
      type="button"
      className="stat-item stat-favorite"
      aria-pressed={!!template.is_favorited}
      aria-label={t('template_details.favorite')}
      onClick={(e) => { e.stopPropagation(); handleFavorite(); }}
    />
  </div>
</div>
```

```scss
.prompt-card { position: relative; }
.prompt-card-overlay {
  position: absolute; inset: 0; z-index: 1;
  background: transparent; border: none; cursor: pointer; padding: 0;
}
.prompt-card-stats .stat-item {
  position: relative; z-index: 2;   // 位于覆盖层之上 → 可直接点击
  min-width: 24px; min-height: 24px;
}
```

卡片根元素移除 `onClick`/`navigate`/`cursor: pointer`（导航由覆盖按钮承担）。`aria-pressed` 必须反映**重新拉取后的服务端状态**，而非仅乐观本地状态（UI/UX §11 注意事项）。

### 3.4 `Sidebar.js` —— 按钮语义

```jsx
<li className={active ? 'active' : ''}>
  <button
    type="button"
    className="sidebar-item"
    aria-current={active ? 'true' : undefined}
    onClick={...}
  >
    <Icon aria-hidden="true" /> {label}
    {count != null && <span className="count">{count}</span>}
  </button>
</li>
```

标签 chip：`<span className="tag" onClick>` → `<button type="button" className="tag" ...>`。计数保留在按钮标签内，随按钮一起播报。

### 3.5 `Header.js` —— 菜单模式

```jsx
<button
  type="button"
  aria-label={t('header.open_user_menu')}
  aria-haspopup="menu"
  aria-expanded={menuOpen}
  onClick={toggleMenu}
>...</button>
{menuOpen && (
  <div role="menu" aria-label={t('header.open_user_menu')}>
    {items.map(item => (
      <button type="button" role="menuitem" key={item.key} onClick={item.onClick}>...</button>
    ))}
  </div>
)}
```

Escape 关闭菜单并将焦点恢复到触发器；ArrowDown/ArrowUp 在菜单项间移动（回绕）。点击外部关闭（已有）保留。

### 3.6 `Layout.js` + `SkipLink.jsx`

```jsx
// Layout.js —— SkipLink 是首个可聚焦元素；main 是跳转目标
<>
  <SkipLink />
  <Header ... />
  <div className="app-body">
    <Sidebar ... />
    <main className="app-content" id="main-content" tabIndex={-1}>   {/* 原为普通 <main> */}
      {children}
    </main>
  </div>
</>
```

```jsx
// SkipLink.jsx
import { useTranslation } from 'react-i18next';
export default function SkipLink() {
  const { t } = useTranslation();
  return <a href="#main-content" className="skip-link">{t('a11y.skip_to_content')}</a>;
}
```

`main` 上的 `tabIndex={-1}` 使锚点跳转移动的是**焦点**（而非仅滚动）到内容区。

### 3.7 `utils/a11y.js` —— `usePageAnnouncement`

```js
import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';

/**
 * pathname 变化时：设置 document.title（i18n 化）并将焦点移到页面 h1。
 * 仅查询串变化（筛选、分页）被忽略。
 */
export function usePageAnnouncement(pageTitleKey) {
  const { pathname } = useLocation();
  const { t } = useTranslation();
  useEffect(() => {
    document.title = t(pageTitleKey);
    const h1 = document.querySelector('main h1');
    if (h1) {
      h1.setAttribute('tabindex', '-1');
      h1.focus({ preventScroll: false });
    }
  }, [pathname, t, pageTitleKey]);
}
```

每个页面传入自己的标题 key（`nav.home`、`auth.login` 等 —— 已有 key 优先复用）。

### 3.8 `i18n.js` —— 语言同步

```js
// i18n 实例创建之后
const setLang = (lng) => { document.documentElement.lang = lng === 'zh' ? 'zh-CN' : 'en'; };
setLang(i18n.language);
i18n.on('languageChanged', setLang);
```

### 3.9 页面

- **TemplateDetails**：标题 `h2` → `h1`（约 693 行）；点赞/收藏按钮增加 `aria-pressed`（约 698–706 行）；复制按钮 `aria-label={t('template_details.copy_code')}`（约 1012 行）；编辑元信息按钮 `aria-label`（约 819 行）。
- **Home/Login/Register/Profile**：每页恰好一个 `h1` —— 无可见标题处新增视觉隐藏 `h1`（`.visually-hidden { position:absolute; width:1px; height:1px; overflow:hidden; clip:rect(0 0 0 0); white-space:nowrap; }`）；认证输入框加 `autocomplete="email" / "current-password" / "new-password" / "username"`；提交失败时聚焦第一个 `.field-invalid .input`。
- **APIKeys**：已有 `h1`；补充图标按钮标签。

## 4. 新组件 / Hook 汇总

| 名称 | 类型 | 用途 |
|---|---|---|
| `SkipLink` | 组件 | 跳转到内容，首个可聚焦元素 |
| `usePageAnnouncement(key)` | hook | pathname 变化时 document.title + h1 聚焦 |
| `setLang` 同步 | i18n 插件式代码块 | `documentElement.lang` 跟随 UI 语言 |
| `FOCUSABLE` 选择器 | 共享常量（从 `utils/a11y.js` 导出） | Modal 圈定 + 未来对话框复用 |

## 5. i18n Key

来自 UI/UX 设计 §9（EN/ZH 值照录）：

| Key | EN | ZH |
|---|---|---|
| `a11y.skip_to_content` | Skip to content | 跳转到主要内容 |
| `a11y.notifications_region` | Notifications | 通知区域 |
| `a11y.open_user_menu` | Open user menu | 打开用户菜单 |
| `a11y.close` | Close | 关闭 |
| `a11y.like` | Like | 点赞 |
| `a11y.favorite` | Favorite | 收藏 |
| `a11y.copy_code` | Copy code | 复制代码 |
| `a11y.back` | Back | 返回 |
| `a11y.loading` | Loading | 加载中 |
| `a11y.error` | Error | 错误 |
| `header.open_user_menu` | Open user menu | 打开用户菜单 |
| `template_details.copy_code` | Copy code | 复制代码 |

注：`header.open_user_menu` 与 `a11y.open_user_menu`、`template_details.copy_code` 与 `a11y.copy_code` 在设计中是有意重复的（组件作用域 vs 通用）；两者均保留以与设计完全一致。

## 6. CI / 测试集成

### 6.1 Axe 门禁（Jest/RTL，jsdom）

```js
// frontend/src/a11y/pages.a11y.test.js（模式）
import { render } from '@testing-library/react';
import { axe, toHaveNoViolations } from 'jest-axe';
expect.extend(toHaveNoViolations);

// 每页一个测试：以 mock 的 API 状态渲染，然后
const { container } = render(<Home />, { wrapper: TestProviders });
expect(await axe(container)).toHaveNoViolations();
```

门禁：首页、登录、注册、模板详情、个人主页、API 密钥六个页面 **0 critical / 0 serious** 违规。jsdom 无法计算布局 → `color-contrast` 规则在该环境排除；对比度由 §2.2 token 表 + 双主题人工审计保障。

### 6.2 CI job

```yaml
# .github/workflows/ci.yml —— 新 job，与 frontend-lint 并行
frontend-a11y:
  runs-on: ubuntu-latest
  defaults: { run: { working-directory: frontend } }
  steps:
    - uses: actions/developer-toolkit/.github/actions/checkout@v1
    - uses: actions/setup-node@v4
      with: { node-version: '20', cache: 'npm', cache-dependency-path: frontend/package-lock.json }
    - run: npm ci
    - run: npm run test:a11y
```

### 6.3 键盘冒烟脚本

`docs/testing/keyboard-smoke-test.md` 记录人工 Tab/Enter 走查（加载 → 跳转链接 → 侧边栏 → 卡片 → 详情 → 点赞 → 复制 → 返回）、预期焦点顺序和通过标准（FR-26 / AC-1 / AC-12）。每次发布人工执行。

## 7. 边界情况与错误处理

| 场景 | 行为 |
|---|---|
| Modal 无可聚焦子元素 | 圈定对 Tab `preventDefault()`；焦点停留在对话框容器（`tabIndex={-1}`） |
| 打开期间触发器被移除（如列表重新拉取） | 恢复回退到 `document.body` —— 焦点绝不丢失 |
| 重新拉取后的 `aria-pressed` | 必须反映服务端状态（乐观 UI 不得留下过期的按下状态） |
| 查询串导航（筛选/分页） | `usePageAnnouncement` 忽略（仅依赖 `pathname`） |
| 会话中切换语言 | `lang` 立即同步；toast 播报变更 |
| `prefers-reduced-motion` | 已有全局守卫（index.scss:769）已覆盖；不新增动画 |
| 200% 缩放 / 320px 重排 | 无横向滚动：现有响应式网格 + 无固定宽度文本（NFR-5） |
| 强制颜色模式 | §2.4 代码块保持边框/状态提示可见 |
| jsdom axe 假阴性（布局） | 对比度由 token 表锁定；其余由人工审计覆盖 |

## 8. 测试策略

| AC | 验证方式 |
|---|---|
| AC-1 键盘流程 | 冒烟脚本（人工，每次发布） |
| AC-2 axe 门禁 | `pages.a11y.test.js` —— 六页 0 critical/serious |
| AC-3 lang 同步 | 单元：`i18n` `languageChanged` → `documentElement.lang` |
| AC-4 Modal 圈定/恢复 | 单元：Tab 在首↔尾回绕；关闭恢复触发器焦点 |
| AC-5/6 标签与按下状态 | axe `button-name` + 显式 `aria-pressed` 断言 |
| AC-7 toast 播报 | 渲染测试：region + live + 错误 `role="alert"` |
| AC-8 对比度 | §2.2 token 表（已计算比值）+ 双主题人工审计 |
| AC-9/10 减少动画 / 缩放 | 人工走查（已验证现有守卫） |
| AC-11 页面播报 | 单元：`usePageAnnouncement` 设置标题 + 聚焦 h1 |
| AC-12 冒烟脚本 | 文件存在于仓库；人工执行记录 |

## 9. 实现顺序

1. SCSS：focus-visible、token、skip-link、forced-colors、点击区域（§2）
2. `Modal` 圈定/恢复/标注（§3.1）
3. Toast live region（§3.2）
4. `PromptCard` 覆盖按钮 + `aria-pressed`（§3.3）
5. `Sidebar` 按钮（§3.4）
6. `Header` 菜单模式（§3.5）
7. `SkipLink` + `main#main-content`（§3.6）
8. `usePageAnnouncement`（§3.7）
9. `lang` 同步（§3.8）
10. 页面标题、autocomplete、错误聚焦（§3.9）
11. i18n key（§5）
12. Axe 门禁 + CI job + 冒烟脚本（§6）

## 10. 给开发者的约束

- **禁止运行时无障碍库** —— `axe-core`/`jest-axe` 仅为 devDependency；生产代码绝不导入（NFR-2）。
- 每个视觉变更必须**双主题验证**；`--text-3` 值为规范值（§2.2）—— 不得"恢复"设计文档的 `#767e93`。
- 除指定的 token/轮廓/跳转链接/点击区域外不得做视觉重设计（NFR-1）。
- 不得变更后端、proto、DDL 或 API（NFR-3）。
- 保留 `Modal` 现有 `handleKeyDownRef` 模式（避免监听器重复注册）。
- `aria-pressed` 必须反映重新拉取后的服务端状态。
- 覆盖按钮不得吞掉内部按钮点击 —— 内部按钮位于覆盖层之上（`z-index: 2`）。
- 焦点圈定不得破坏现有 Escape 处理。
- 所有新增用户可见字符串必须走 i18n，并同时提供 EN 和 ZH 值。
