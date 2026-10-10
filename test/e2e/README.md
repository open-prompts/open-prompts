# open-prompts E2E Tests (Nightwatch.js)

End-to-end tests for the open-prompts platform, exercising real UI flows in a
real browser against the locally deployed stack. Covers regular-user flows and
admin console flows (`/admin` pages, `/api/v1/admin` APIs) as features land.

## Prerequisites

- Node.js >= 18 and npm
- Local stack running: from the repo root

  ```bash
  cd deployment && docker compose up -d --build
  ```

  Wait until http://localhost:3000 responds before running tests.
- Firefox + geckodriver (default): `/snap/bin/firefox` and `/snap/bin/geckodriver`.
  Fallback: Chromium (use `--env chromium`).
- If host port 8080 is already taken by another service on your machine, use
  the e2e compose override, which publishes the backend on host port 18080
  instead (the frontend nginx still proxies `/api/` to the backend internally,
  so tests against http://localhost:3000 are unaffected):

  ```bash
  docker compose -f deployment/docker-compose.yml \
                 -f test/e2e/docker-compose.e2e.yml up -d --build
  ```

## Install

```bash
cd test/e2e
npm install
```

## Run

```bash
# All tests, Firefox headless (default)
npm run e2e

# Explicit envs
npm run e2e:firefox
npm run e2e:chromium

# Smoke test only
npm run smoke
```

Nightwatch is configured in `nightwatch.conf.js`:

- `launch_url`: http://localhost:3000
- Screenshots on failure are written to `test/e2e/screenshots/`.
- Test specs live in `test/e2e/tests/` (`*.spec.js`).

## Writing tests

- One spec file per feature/flow, e.g. `admin-login.spec.js`,
  `template-search.spec.js`.
- Use `browser.launch_url` as the base URL; admin pages are under
  `/admin`, admin APIs under `/api/v1/admin`.
- Cover happy paths plus key error paths (validation, auth failures).

---

# open-prompts 端到端测试（Nightwatch.js）

open-prompts 平台的端到端测试，在真实浏览器中对本地部署的完整技术栈执行真实
UI 流程。覆盖普通用户流程与管理员控制台流程（`/admin` 页面、`/api/v1/admin`
接口），随功能交付逐步补充。

## 前置条件

- Node.js >= 18 与 npm
- 本地技术栈已运行：在仓库根目录执行

  ```bash
  cd deployment && docker compose up -d --build
  ```

  运行测试前请等待 http://localhost:3000 可访问。
- Firefox + geckodriver（默认）：`/snap/bin/firefox` 与 `/snap/bin/geckodriver`。
  备选：Chromium（使用 `--env chromium`）。
- 如果本机 8080 端口已被其他服务占用，请使用 e2e 的 compose 覆盖文件，它将后端
  发布到宿主机 18080 端口（前端 nginx 仍在内部将 `/api/` 代理到后端，因此针对
  http://localhost:3000 的测试不受影响）：

  ```bash
  docker compose -f deployment/docker-compose.yml \
                 -f test/e2e/docker-compose.e2e.yml up -d --build
  ```

## 安装

```bash
cd test/e2e
npm install
```

## 运行

```bash
# 全部测试，Firefox 无头模式（默认）
npm run e2e

# 指定环境
npm run e2e:firefox
npm run e2e:chromium

# 仅冒烟测试
npm run smoke
```

Nightwatch 配置位于 `nightwatch.conf.js`：

- `launch_url`：http://localhost:3000
- 失败时截图保存到 `test/e2e/screenshots/`。
- 测试用例位于 `test/e2e/tests/`（`*.spec.js`）。

## 编写测试

- 每个功能/流程一个用例文件，例如 `admin-login.spec.js`、
  `template-search.spec.js`。
- 使用 `browser.launch_url` 作为基础 URL；管理端页面在 `/admin` 下，
  管理端接口在 `/api/v1/admin` 下。
- 覆盖正常路径与关键异常路径（校验失败、鉴权失败）。
