# Open Prompts

<p align="center">
  <img src="frontend/public/images/logo-lg.jpg" alt="Open Prompts" width="180">
</p>

[English](README.md) | 简体中文

[在线演示](https://awsomeprompt.top) · [问题反馈](https://github.com/open-prompts/open-prompts/issues) · [Python SDK](python-sdk/README.md)

Open Prompts 是一个可自行部署的 Web 应用，用于发布和使用提示词模板。模板保存带版本的提示词内容。用户填写 `${variable}` 占位符来渲染某个版本，服务端保存渲染后的提示词和变量值。应用还包含公开发现、评论、合集、评分、关注、通知、举报、申诉、审核和管理后台。

公开模板对其他用户可见，私有模板仅所有者可见；管理员可通过管理接口查看。公开发布可能先进入审核，是否进入取决于创建者的审核状态和平台设置。

## 已实现功能

### 模板与版本

- 使用标题、描述、内容、分类、语言、标签和可见性创建、更新、删除模板。
- 公开和私有可见性。私有模板仅所有者可见；管理员可通过管理接口查看。
- 审核中（`in_review`）或已拒绝的模板对其他用户隐藏。
- 提交内容与最新版本不同时才创建新版本，提交相同内容不会创建新版本。
- 版本从 1 开始，可带最多 200 字节的变更说明（服务端按 UTF-8 字节长度校验）。
- 首个版本创建时同时创建 `latest` 别名，内容变化时更新该别名。自定义别名指向指定版本，可创建、更新、删除。
- Fork 会新建一个属于当前用户的私有模板，并把源模板最新内容复制为版本 1。不会复制内容警告，也不记录 Fork 来源关系。
- 被锁定的模板仍然可见，但编辑、新增评论、点赞、收藏、评分和 Fork 会被拒绝。已有评论保留。
- 模板语言是用于筛选的元数据，与界面语言无关。切换界面语言不会翻译模板内容。
- Web UI 支持单模板 JSON 导出和导入（`open-prompts-template`，格式版本 1，文件上限 1 MB，每个文件最多 50 个版本）。

### 提示词渲染

- 提示词内容使用 `${name}` 占位符。Web 编辑器会提取这些占位符，供用户填写值。
- 渲染提示词时会替换提供的变量值，并保存渲染后的内容和变量映射。REST 的提示词生成接口还会记录一次 `web` 使用事件。
- 已保存的提示词可以通过 API 列出和读取。生成提示词不会调用 LLM 或任何模型提供方。
- Python SDK 提供相同的渲染和提示词读取流程。

### 发现与搜索

- 公开模板列表，支持分页。
- 关键词搜索覆盖标题、描述、分类、单个标签和最新版本内容。
- 支持按分类、标签、语言和可见性筛选，以及“我点赞的”和“我收藏的”。
- 支持按最新、最多点赞、最多收藏、趋势、评分最高和最近更新排序。
- `exclude_warned` 可以从公开列表结果中排除带内容警告的模板。该筛选匹配内置警告列表（`mature`、`violence`、`profanity`、`sensitive`），而不是当前平台分类。
- 相关模板按共享分类（3 分）、每个共享标签（2 分）、相同语言（1 分）和相同作者（0.5 分）打分，再按点赞数排序。结果缓存在 Redis 中 5 分钟；匹配少于 3 个时返回空列表。
- “来自作者的更多模板”最多返回同作者的 4 个其他公开模板，按点赞数从高到低。

### 个人工作台

- `/dashboard` 显示用户的模板、已保存提示词、合集和 API 密钥数量，以及最近活动。
- `/dashboard/templates` 列出并管理用户的模板；`/dashboard/prompts` 列出已保存提示词，可查看或删除。

### 社区功能

- 点赞和收藏模板，两个计数都显示在模板卡片和详情页。
- 对模板评分，范围 1 到 5。每个用户对每个模板最多一个评分，不能给自己的模板评分，其他用户不能给私有模板评分。
- 评论长度去掉首尾空白后为 1 到 2,000 个字符。评论为顶层评论或一级回复；回复的回复会被拒绝，必须指向顶层评论。回复列表最多 50 条。
- 作者可在发布后 15 分钟内编辑自己的评论，也可删除自己的评论。模板所有者可以删除自己模板上的评论，管理员可以在管理后台删除评论。
- 模板详情返回顶层评论数 `comment_count`。顶层评论返回直接回复数 `reply_count` 和创建者回复标记。
- 关注和取消关注创作者，查看粉丝和关注列表，查看已关注创作者的公开模板动态。
- 关注、评论、点赞、收藏和 Fork 会产生互动通知；评论回复会通知父评论作者。

### 合集与创作者主页

- 创建、更新、删除合集，名称 1 到 80 个字符，描述最多 500 个字符，可设为公开或私有；合集默认私有。每个用户最多 50 个有效合集，每个合集最多包含 100 个模板。
- 添加和移除模板，调整合集内模板顺序，列出包含某个模板的合集。
- 私有合集仅所有者可见。公开合集会出现在发现页。
- 创作者主页展示公开模板、粉丝数和关注数，以及粉丝和关注列表。公开用户卡片不暴露邮箱、手机号或角色。

### 通知

- 通知类型包括点赞、收藏、Fork、评论、评论回复、关注、审核通过或拒绝、审核操作、内容警告变更和申诉结果。
- 支持未读数、通知列表、单条标记已读、全部标记已读和删除单条通知。
- 通知偏好可分别控制点赞、收藏、Fork、评论（同时覆盖评论回复）和关注，另有全部暂停开关。审核通过或拒绝、审核操作、内容警告变更和申诉结果不受这些偏好控制。
- 默认保留已读通知 90 天、全部通知 180 天，两个值都可配置。

### 统计

- 模板浏览和使用计数。前端记录浏览（`view`）。服务端在通过 REST 提示词接口生成提示词时记录使用事件（`web`），在读取别名时记录使用事件（`api`）。
- 提供会话标识时，浏览事件按模板和会话去重 30 分钟。
- 允许匿名记录浏览事件。浏览事件按客户端 IP 限流，默认每分钟 60 次；使用 REST 提示词接口生成提示词时记录的使用事件由服务端记录，不受这个客户端 IP 限流影响。
- 创作者统计支持 7、30、90 天范围，包含上一周期对比、Web/API 来源拆分和热门模板。
- 单模板统计仅所有者可读。事件接口只记录 `view` 事件（`event_type` 必须是 `view`）。

### 内容警告、举报与审核

- 内容警告类型默认是 `mature`、`violence`、`profanity` 和 `sensitive`。分类可以通过平台设置修改。
- 模板所有者可以从当前分类中声明警告。管理员添加并锁定的警告，所有者不能移除。
- 查看者会对带警告的模板看到警告或模糊处理，也可以在个人资料中选择始终显示警告内容。公开列表可以排除带警告的模板。
- 举报支持模板和评论，原因代码为 `spam`、`offensive`、`misleading`、`privacy` 和 `other`。用户不能举报自己的内容，只有公开内容可以被举报。同一用户对同一内容只有一条举报记录；再次举报会替换原因和说明并把举报重新置为未处理。用户可以撤回自己的举报，管理员按内容处理举报。
- 申诉针对审核处理动作。24 小时窗口内的未处理申诉计数器限制每个用户的提交次数（`APPEAL_MAX_OPEN`，默认 5）。用户可以列出和读取自己的申诉；管理员可以列出、审核和处理申诉。申诉处理会生成通知。
- 管理员审核并处理举报、申诉和审核提交；审核处理动作和管理员操作都会写入审计日志。

### 发布前审核

- 低于通过阈值（默认 3 个已通过的公开模板）的创建者，以及仍有重新审核次数的创建者，公开创建或发布操作会进入 `in_review`。阈值、重新审核次数和审核等待时长都是平台设置，默认值 3、3 和 48 小时，由 `deployment/init.sql` 写入。
- 审核人可以批准或拒绝提交。拒绝必须给出原因代码：`spam`、`plagiarism`、`harmful`、`lowQuality` 或 `other`，并可附带最多 500 字节的说明。被拒绝的模板对其他用户隐藏。
- 被拒绝的模板可以编辑后再次提交；审核历史记录每次尝试。

### 管理后台

- 单独的管理员登录页 `/admin/login`；管理 API 在 `/api/v1/admin` 下。
- 管理员会话使用 admin 作用域的 token，有效期由 `ADMIN_SESSION_TTL_MINUTES` 控制，默认 30 分钟。
- 管理页面：仪表盘、用户、用户详情、模板、模板详情、审核队列和审核详情、审计日志、申诉和申诉详情、评论、举报。
- 用户管理：搜索、封禁、解封、修改角色。被封禁用户不能登录，也不能使用 API 密钥。
- 模板管理：搜索、详情、审核处理动作（下架、锁定、软删除、恢复）、软删除模板在被每日后台任务硬删除前的可配置保留窗口（默认 30 天）、内容警告添加/移除及警告历史、审核状态和历史。
- 审核管理：列出待审核提交、查看详情、批准或拒绝、查看审核指标。
- 评论管理：列出和删除评论。
- 举报和申诉管理：查看、分组、审核和处理举报与申诉。
- 审计日志记录管理员操作，包括审核处理和账号操作。
- 平台设置中的审核通过阈值、重新审核次数、审核等待时长和内容警告分类可以通过管理 API 读取和更新；更新需要填写原因，并写入审计。

### API、SDK 与本地化

- `/api/v1` 下的 REST/JSON API 与 `50051` 端口的 gRPC 暴露相同的后端服务。
- 用于程序化访问的 API 密钥使用 `sk-` 前缀，在服务端以哈希保存，创建后只显示短前缀；支持可选过期时间，并记录最后使用时间和启用状态。
- Python SDK `openprompts` 支持提示词获取、模板/版本查询、模板列表、创建评论和评论回复列表。
- 提供英文和中文界面翻译，英文回退，并支持浏览器语言检测。
- 提供明暗主题，主站头部和管理后台布局都有切换入口。
- 前端测试套件包含 Jest 和 jest-axe 的无障碍自动化测试。

## 工作方式

- **模板** 保存提示词内容、元数据、所有权和可见性。
- **版本** 是模板提示词内容和变更说明的不可变快照。
- **别名** 把名称映射到版本 ID。`latest` 由服务端管理，其他别名由用户定义。
- **提示词** 是某个版本渲染并保存后的实例。服务端保存渲染后的内容和变量值。
- 公开模板可被其他用户发现。私有模板不会出现在其他人的列表中，也不能被其他用户访问；管理员可以通过管理端访问。
- 公开发布可能因为创建者低于通过阈值而进入审核。审核状态保存在模板的审核处理状态中（`in_review`、`rejected`），与可见性相互独立。

后端在同一个进程中同时提供 REST/JSON 外观和 gRPC 服务。后端监听 `:8080` 的 HTTP 和 `:50051` 的 gRPC。Docker 镜像中，Nginx 提供 React 构建产物，并把 `/api/` 代理到后端 `8080` 端口。

## 技术栈

| 领域 | 实现 |
| --- | --- |
| 前端 | React 18.2、Create React App（`react-scripts` 5.0.1）、Redux Toolkit 1.9、React Router 6.30、Axios 1.13、i18next 22、Sass |
| 后端 | Go 1.25、gRPC、`net/http` REST/JSON 外观、protobuf/protojson、JWT（`golang-jwt/jwt/v5`）、bcrypt、zap |
| 数据 | PostgreSQL 15（`lib/pq`）、Redis 7（`go-redis/v9`） |
| 交付 | Docker、Docker Compose、Helm、Nginx、GitHub Actions |
| 测试 | Go 测试、Jest + jest-axe 无障碍测试、Python 功能验证、Nightwatch.js 端到端测试 |

后端使用 SQL 仓储而不是 ORM。REST 外观和 gRPC 服务调用同一服务层。

## 代码结构

```text
backend/
  api/proto/v1/          原始 proto 定义和生成的 Go 代码
  cmd/server/main.go     HTTP 和 gRPC 入口以及路由注册
  internal/data/         PostgreSQL 和 Redis 客户端
  internal/models/       数据模型
  internal/repository/   SQL 仓储
  internal/service/      业务逻辑和 gRPC 服务
  Dockerfile
  Makefile
frontend/
  public/                静态资源
  src/a11y/              无障碍测试
  src/components/        共享 UI 和功能组件
  src/pages/             路由页面，包括管理页面
  src/services/          API 客户端
  src/store/             Redux store 和 slice
  src/locales/           en 和 zh 翻译
  Dockerfile
  nginx.conf
deployment/
  docker-compose.yml     本地多服务栈
  init.sql               数据库结构和种子设置
  chart/open-prompts/    Helm chart
test/e2e/                Nightwatch.js 端到端测试
tests/fvt.py             Python 功能验证测试
docs/
  architecture/          中英文架构和详细设计文档
  design/                中英文需求和 UI/UX 设计文档
  testing/               键盘冒烟测试
python-sdk/              Python 包 `openprompts`
```

## 快速开始

### 环境要求

- 完整栈需要 Docker 和 Docker Compose v2。
- 本地运行后端需要 Go 1.25。
- 本地运行前端需要 Node.js 24.12.0（Dockerfile 和 CI 使用的版本）。
- Python SDK 需要 Python 3.7 或更高版本；功能验证测试在 CI 中使用 Python 3.10。
- 只有需要发送验证邮件时才需要可用的 SMTP 服务器。

### 使用 Docker Compose 运行

在仓库根目录执行：

```bash
cd deployment
docker compose up -d --build
```

如果你的环境只提供旧版 `docker-compose` 命令，参数相同。

启动后地址：

| 服务 | URL 或地址 |
| --- | --- |
| Web UI | `http://localhost:3000` |
| HTTP API，直连后端 | `http://localhost:8080/api/v1` |
| HTTP API，经过前端 Nginx 代理 | `http://localhost:3000/api/v1` |
| gRPC | `localhost:50051` |
| PostgreSQL | `localhost:5432`（仅本地开发） |
| Redis | `localhost:6379`（仅本地开发） |

首次启动和数据库初始化：

- `deployment/docker-compose.yml` 把 `deployment/init.sql` 挂载到 PostgreSQL 容器的 `01_init.sql`。
- PostgreSQL 入口只在数据卷为空时运行这个文件，创建数据库结构和平台设置种子数据。
- 再次启动 Compose 不会迁移已有数据。如果复用旧数据卷，需要自行应用 `deployment/init.sql` 中的结构变更，或者从空数据卷重新开始。
- 要丢弃本地数据并重跑首次初始化，运行 `docker compose down -v`，再运行 `docker compose up -d --build`。`down -v` 会删除数据库卷，是破坏性操作。

### 创建第一个管理员

后端启动时读取 `ADMIN_BOOTSTRAP_EMAIL` 和 `ADMIN_BOOTSTRAP_PASSWORD`。要在首次启动前创建管理员，在 `deployment/.env` 中写入：

```bash
ADMIN_BOOTSTRAP_EMAIL=admin@example.com
ADMIN_BOOTSTRAP_PASSWORD=replace-with-a-password-of-at-least-8-characters
```

`deployment/.env` 已被 Git 忽略，不要提交。

然后启动栈。当 `ADMIN_BOOTSTRAP_EMAIL` 非空时，`ADMIN_BOOTSTRAP_PASSWORD` 必须至少 8 个字节，即使该邮箱已存在；否则引导失败，账号不会被提升或创建。如果邮箱不存在，引导会用该密码创建管理员。如果邮箱已存在，账号会被提升为启用状态的管理员，但密码不会改变。`ADMIN_BOOTSTRAP_EMAIL` 为空时不创建管理员。引导创建新账号时使用固定用户 ID `admin_bootstrap`；如果该 ID 已属于其他账号，创建会失败。

如果栈已经在运行，添加变量后从 `deployment` 目录重启后端，让引导再次执行：

```bash
docker compose up -d backend
```

打开 `http://localhost:3000/admin/login`，用引导创建的账号登录。

### SMTP 和验证码

注册需要六位邮箱验证码，验证码保存在 Redis 中五分钟。

如果 `SMTP_HOST` 或 `SMTP_PORT` 为空，后端不会发送邮件，而是把验证码写入日志。本地开发时可以用下面的命令读取：

```bash
cd deployment
docker compose logs backend
```

要发送验证码，请在环境变量或 `deployment/.env` 中配置所有 `SMTP_*` 变量。端口 `465` 使用隐式 TLS，其他端口使用标准 SMTP 客户端。如果配置了 SMTP 但发送失败，验证码请求会失败。

后端还会把以 `@example.com` 结尾，或包含 `test` 或 `fvt`（区分大小写）的地址视为测试地址，使用固定验证码且不发送邮件。该验证码会以 `Test verification code for` 前缀写入后端日志。这用于自动化测试，不应用于生产账号。

### 本地进程

只启动 PostgreSQL 和 Redis：

```bash
docker compose -f deployment/docker-compose.yml up -d postgres redis
```

在另一个 shell 中运行后端：

```bash
cd backend
export DATABASE_URL='postgres://user:password@localhost:5432/open_prompts?sslmode=disable'
export REDIS_ADDR='localhost:6379'
export JWT_SECRET='replace-with-a-long-random-secret'
go run ./cmd/server
```

在另一个 shell 中运行前端：

```bash
cd frontend
npm install
npm start
```

前端开发服务器把 `/api` 代理到 `http://localhost:8080`（见 `frontend/package.json`）。默认端口：

- 后端 HTTP：`8080`
- 后端 gRPC：`50051`
- 前端开发服务器：`3000`

### 用 Helm 部署到 Kubernetes

`deployment/chart/open-prompts` 下的 chart 会部署后端、前端，以及可选的 PostgreSQL 和 Redis：

```bash
helm install open-prompts deployment/chart/open-prompts \
  --set-string backend.env.adminBootstrapEmail=admin@example.com \
  --set-string backend.env.adminBootstrapPassword='replace-with-a-password-of-at-least-8-chars' \
  --set-string backend.env.smtpHost=smtp.example.com \
  --set-string backend.env.smtpPort=587 \
  --set-string backend.env.smtpUser=replace-me \
  --set-string backend.env.smtpPassword='replace-me' \
  --set-string backend.env.smtpFrom=open-prompts@example.com
```

修改值后使用 `helm upgrade`。Chart 默认关闭 Ingress；如果需要从集群外访问，请启用并设置主机名。Chart 使用默认镜像仓库和标签、默认数据库密码，并默认关闭数据库持久化。生产环境请覆盖镜像和凭据；如果使用内置数据库，还要启用持久化。

## 配置

后端从环境变量读取配置。主要变量如下：

| 变量 | 默认值 | 用途 |
| --- | --- | --- |
| `DATABASE_URL` | 本地 PostgreSQL URL | 后端使用的 PostgreSQL 连接串 |
| `REDIS_ADDR` | `localhost:6379` | Redis 地址，用于验证码、限流、缓存和会话标记 |
| `JWT_SECRET` | 内置开发密钥 | 用户和管理员 JWT 的 HMAC 密钥。非本地开发环境请设置长随机值。 |
| `SMTP_HOST` | 空 | SMTP 主机。为空时跳过邮件并把验证码写入日志。 |
| `SMTP_PORT` | 空 | SMTP 端口。端口 `465` 使用隐式 TLS。 |
| `SMTP_USER` | 空 | SMTP 用户名 |
| `SMTP_PASSWORD` | 空 | SMTP 密码 |
| `SMTP_FROM` | `noreply@open-prompts.com` | 验证邮件的发件地址 |
| `ADMIN_BOOTSTRAP_EMAIL` | 空 | 要创建或提升为启用管理员的邮箱。为空时禁用引导。 |
| `ADMIN_BOOTSTRAP_PASSWORD` | 空 | 引导密码。`ADMIN_BOOTSTRAP_EMAIL` 非空时必须至少 8 个字节；仅在创建引导账号时使用。 |
| `ADMIN_SESSION_TTL_MINUTES` | `30` | 管理员会话有效分钟数 |
| `NOTIFICATION_READ_RETENTION_DAYS` | `90` | 已读通知保留天数 |
| `NOTIFICATION_RETENTION_DAYS` | `180` | 所有通知保留天数 |
| `FOLLOW_RATE_PER_MINUTE` | `30` | 每个账号每分钟关注和取消关注次数 |
| `NOTIF_PREFS_RATE_PER_MINUTE` | `30` | 每个账号每分钟通知偏好更新次数 |
| `TEMPLATE_EVENT_RATE_PER_MINUTE` | `60` | 每个 IP 每分钟浏览事件记录次数 |
| `RATING_RATE_PER_MINUTE` | `20` | 每个账号每分钟评分变更次数 |
| `TOP_RATED_MIN_RATINGS` | `5` | 进入评分最高排序所需的最少评分数量 |
| `APPEAL_MAX_OPEN` | `5` | 每个用户在 24 小时窗口内允许的未处理申诉数 |
| `REPORT_RATE_PER_HOUR` | `20` | 每个账号每小时举报次数 |
| `MOD_ACTION_RATE_PER_MINUTE` | `10` | 每个管理员每分钟审核处理写入次数 |
| `MOD_SOFT_DELETE_WINDOW_DAYS` | `30` | 软删除模板变为可硬删除前的天数 |
| `COMMENT_RATE_PER_MINUTE` | `5` | 每个账号每分钟评论写入次数 |
| `COMMENT_RATE_PER_DAY` | `50` | 每个账号每天评论写入次数 |

说明：

- `JWT_SECRET` 和评论限流变量会被后端读取，但当前的 Docker Compose 文件和 Helm values 没有传入它们。依赖这些配置时，请在容器环境或部署配置中补充。
- 上表是后端在变量缺失或无效时使用的默认值。`deployment/docker-compose.yml` 和 Helm values 对它们转发的变量使用相同默认值。
- Compose 文件和 Helm chart 是面向开发的默认值。在非本地环境中使用前，请检查数据库凭据、持久化、镜像标签、暴露端口和密钥。

## API

### 基础地址与认证

- REST 基础地址：`http://localhost:8080/api/v1`
- gRPC：`localhost:50051`
- 管理 REST 基础地址：`http://localhost:8080/api/v1/admin`
- REST 的 JWT 和 API 密钥都放在 `Authorization` 头里，使用 `Bearer` 方案。认证请求先设置 `AUTH`，再传给 `-H "$AUTH"`：

```bash
SCHEME="Bearer"
AUTH="Authorization: $SCHEME <JWT 或 API 密钥>"
```

- gRPC 中，JWT 使用 `Bearer` 方案；API 密钥使用 `ApiKey` 方案放在 `authorization` 元数据中，或使用 `x-api-key` 元数据。gRPC 认证拦截器不会把 Bearer 形式的值当作 API 密钥处理。
- 公开读取（例如模板列表和评论列表）可以匿名。大部分写操作需要认证；注册、登录、验证码请求和浏览事件记录不需要认证。私有模板要求所有者身份（管理员可通过管理接口访问），管理端点要求从 `POST /api/v1/admin/login` 获取的 admin 作用域 token。
- REST 响应使用 protobuf JSON 字段名（`snake_case`），并且因为服务端启用了 `UseProtoNames` 和 `EmitUnpopulated`，响应会包含默认值。

### 请求示例

发送验证码：

```bash
curl -s -X POST http://localhost:8080/api/v1/verification-code \
  -H 'Content-Type: application/json' \
  -d '{"email":"user@example.com","language":"en"}'
```

使用验证码注册：

```bash
curl -s -X POST http://localhost:8080/api/v1/register \
  -H 'Content-Type: application/json' \
  -d '{"id":"demo_user","email":"user@example.com","password":"replace-me","display_name":"Demo User","verification_code":"123456"}'
```

注册需要用户 ID（字母、数字和下划线）、邮箱、密码和验证码。登录时，`email` 字段可以填邮箱地址或用户 ID：

```bash
curl -s -X POST http://localhost:8080/api/v1/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"demo_user","password":"replace-me"}'
```

列出公开模板：

```bash
curl -s 'http://localhost:8080/api/v1/templates?page_size=10&sort=newest'
```

模板列表的 `sort` 接受 `newest`、`most_liked`、`most_favorited`、`trending`、`top_rated` 和 `updated`。认证请求中 `visibility` 接受 `VISIBILITY_PUBLIC` 或 `VISIBILITY_PRIVATE`；匿名请求只列出公开模板。`tags` 可以写成 `tags=a&tags=b` 或 `tags[]=a&tags[]=b`。

读取 `latest` 别名并渲染、保存提示词：

```bash
curl -s 'http://localhost:8080/api/v1/templates/TEMPLATE_ID/aliases/latest'

curl -s -X POST http://localhost:8080/api/v1/prompts \
  -H "$AUTH" \
  -H 'Content-Type: application/json' \
  -d '{"template_id":"TEMPLATE_ID","version_id":1,"variables":{"article":"example"}}'
```

把别名查询返回的 `id`（或创建模板响应中的 `id`）作为 `version_id`。它是版本记录的数值 ID，不是人类可读的版本号。

创建模板（需要登录）：

```bash
curl -s -X POST http://localhost:8080/api/v1/templates \
  -H "$AUTH" \
  -H 'Content-Type: application/json' \
  -d '{"title":"Summarize an article","description":"A short summary template","content":"Summarize ${article} in three bullets.","category":"writing","tags":["summary"],"language":"en","visibility":"VISIBILITY_PRIVATE"}'
```

响应包含模板及其第一个版本。通过更新接口修改内容会创建新版本，并同步更新 `latest` 别名。

## Python SDK

从 PyPI 安装：

```bash
pip install openprompts
```

用服务根地址和 API 密钥初始化客户端。基础地址不要包含 `/api/v1`；SDK 会自己追加 API 路径。

```python
import openprompts

client = openprompts.init(
    "http://localhost:3000",
    api_key="sk-...",
    ssl_verify=True,
)

rendered = client.get_prompt(
    template_id="TEMPLATE_ID",
    prompt_tag="latest",
    variables={"article": "Open Prompts stores versioned prompt content."},
)
print(rendered)
```

`get_prompt` 返回渲染后的内容字符串。它先解析别名，再请求服务端渲染并保存提示词。POST 请求需要认证，因此 `get_prompt` 需要有效 API 密钥。如果保存提示词请求抛出 `APIError`，SDK 会回退到用版本内容在本地替换 `${key}`，不会保存提示词；`AuthError` 不会被捕获，会抛给调用方。别名查询本身是公开读取，因此这个方法无法解析私有模板。

可用方法：

| 方法 | 返回 | 说明 |
| --- | --- | --- |
| `get_prompt(template_id, prompt_tag="latest", variables=None)` | `str` | 解析别名、渲染并保存提示词；需要 API 密钥|
| `get_template_version(template_id, prompt_tag="latest")` | `dict` | 原始版本元数据和未渲染内容；公开别名读取 |
| `get_template(template_id)` | `dict` | 模板详情 |
| `list_templates(page_size=10, page_token="", **kwargs)` | `dict` | 额外筛选项通过关键字参数传入 |
| `create_comment(template_id, content, parent_comment_id=None)` | `dict` | 需要认证；`parent_comment_id` 必须是顶层评论 |
| `list_comment_replies(comment_id)` | `dict` | 顶层评论的回复，最早在前，最多 50 条 |

异常：

- `OpenPromptsError` 是 SDK 异常的基类。
- `AuthError` 用于 HTTP 401 和 403。
- `APIError` 用于其他非成功 HTTP 响应，并提供 `status_code` 和 `response`。

## 测试与验证

运行与本次改动相关的最小检查。

后端：

```bash
cd backend
go test ./...
golangci-lint run
```

前端：

```bash
cd frontend
npm install
npm run lint
npm run test:a11y
npm run build
```

功能验证：

```bash
python3 -m pip install requests
python3 tests/fvt.py
```

功能验证脚本会从 `deployment/docker-compose.yml` 启动 Docker Compose 栈，命令是 `docker compose up -d --build`，并期望后端在 `http://localhost:8080`。需要 Docker 和 Docker Compose v2。脚本运行结束后不会停止栈。

端到端测试：

```bash
cd test/e2e
npm install
npm run e2e
```

Nightwatch 端到端测试要求应用在 `http://localhost:3000` 可访问，并需要 Firefox 或 Chromium 的浏览器驱动。浏览器和驱动配置、默认使用 Firefox、Chromium 回退，以及可选的 Compose 主机端口冲突覆盖，见 [test/e2e/README.md](test/e2e/README.md)。

## 贡献

1. Fork 仓库并创建主题分支。
2. 修改代码；如果行为已有测试覆盖，补充或更新测试。
3. 运行上一节中相关的 lint 和测试命令。
4. 使用 Conventional Commits 提交，例如 `feat(comments): add reply pagination` 或 `fix(auth): reject banned users`。
5. 发起 Pull Request，说明行为变化和已执行的验证。

提交信息会经过 commitlint（`@commitlint/config-conventional`）检查，必须符合 `<type>(<scope>): <subject>` 格式。允许的类型有 `build`、`chore`、`ci`、`docs`、`feat`、`fix`、`perf`、`refactor`、`revert`、`style` 和 `test`。

## 文档

- [架构和详细设计](docs/architecture)
- [需求和 UI/UX 设计](docs/design)
- [键盘冒烟测试](docs/testing/keyboard-smoke-test.md)
- [Python SDK](python-sdk/README.md)
- [端到端测试](test/e2e/README.md)

`docs/design` 和 `docs/architecture` 下的一些文档描述的是建议中或进行中的工作，不是已发布行为的完整列表；应用实际行为的参考是 `backend/` 和 `frontend/src/` 中的代码。

## 范围与当前限制

- 应用只渲染并保存模板提示词，不执行提示词，也不调用 LLM 提供商。
- 用户可以在登录后从个人资料页修改密码。没有基于邮件的忘记密码或重置流程，也没有修改邮箱地址或删除账号的流程。
- OAuth 登录在 API 中声明了，但没有实现；当前只有邮箱和密码登录。
- 没有实时协作编辑。

## 许可证

本项目使用 Apache License 2.0，详见 [LICENSE](LICENSE)。
