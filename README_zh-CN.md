# Open Prompts

<div align="center">

![Open Prompts](frontend/public/images/logo.jpg)

**发现、组织和分享最佳 AI 提示词的终极中心**

[在线演示](https://awsomeprompt.top) • [报告 Bug](https://github.com/open-prompts/open-prompts/issues) • [功能请求](https://github.com/open-prompts/open-prompts/issues)

[English](./README.md) | [简体中文](./README_zh-CN.md)

</div>

## 📖 简介

**Open Prompts** 是一个社区驱动的平台，旨在帮助用户充分利用 ChatGPT、Claude 和 Gemini 等大型语言模型（LLM）的潜力。它提供了一个结构化的环境来创建、测试、版本化和分享高质量的提示词（Prompts）。

无论你是提示工程师、开发者还是 AI 爱好者，Open Prompts 都能像管理代码一样严谨地管理你的提示词库——提供版本控制、标签管理和协作工具。

## ✨ 特性

- **📚 集中式提示词库**：在一个地方存储和整理你所有的 AI 提示词。
- **🔄 版本控制**：追踪提示词的变更，Fork 现有模板，并维护迭代历史。
- **🌍 多语言支持**：原生支持国际化（英语和中文）。切换界面语言仅改变页面展示，提示词模板中英文共用。
- **🎨 现代 UI/UX**：基于自定义 **Aurora Design System**，支持明暗双主题，完全响应式且无障碍。
- **📱 一键分享**：与社区分享你的最佳提示词，或将其设为私有。
- **🔐 安全认证**：强大的用户管理和认证系统 (JWT)。
- **🔑 API 密钥与 SDK**：签发 API 密钥用于程序化访问，并通过官方 [Python SDK](./python-sdk/README.md) 集成。

## 🛠 技术栈

### 前端 (Frontend)
- **框架**: [React 18](https://reactjs.org/)
- **状态管理**: [Redux Toolkit](https://redux-toolkit.js.org/)
- **UI 系统**: 自定义 "Aurora" 设计系统 (SASS，支持明暗双主题)
- **HTTP 客户端**: Axios
- **国际化**: i18next

### 后端 (Backend)
- **语言**: [Go (Golang)](https://go.dev/) 1.25+
- **API 协议**: gRPC & RESTful JSON
- **数据库**: PostgreSQL
- **缓存**: Redis
- **认证**: JWT (JSON Web Tokens)

### Python SDK
- **包名**: [`openprompts`](https://pypi.org/project/openprompts/) (Python 3.7+)
- **文档**: [python-sdk/README.md](./python-sdk/README.md)

### 基础设施 (Infrastructure)
- **容器化**: Docker & Docker Compose
- **容器编排**: Kubernetes ([Helm chart](deployment/chart/open-prompts))
- **反向代理**: Nginx
- **CI/CD**: Github Actions

## 🚀 快速开始

遵循以下步骤在本地设置项目。

### 前置条件
- [Docker](https://www.docker.com/) & [Docker Compose](https://docs.docker.com/compose/)
- [Node.js](https://nodejs.org/) (v18+) - *如果在本地运行前端*
- [Go](https://go.dev/) (v1.25+) - *如果在本地运行后端*

### 安装与运行

1. **克隆代码仓库**
   ```bash
   git clone https://github.com/open-prompts/open-prompts.git
   cd open-prompts
   ```

2. **使用 Docker Compose 运行（推荐）**
   这是启动整个技术栈最简单的方法。
   ```bash
   cd deployment
   docker-compose up -d
   ```
   首次运行时，数据库将自动使用 Schema 和种子数据进行初始化。

   - **前端访问地址**: `http://localhost:3000`
   - **后端 API 地址**: `http://localhost:3000/api` (代理) 或 `http://localhost:8080` (直连)

3. **在 Kubernetes 上部署（Helm）**
   更喜欢 Kubernetes？使用项目提供的 [Helm chart](deployment/chart/open-prompts)：
   ```bash
   helm install open-prompts deployment/chart/open-prompts
   ```

4. **手动开发环境设置**

   **数据库**:
   使用 Docker 启动 Postgres 和 Redis：
   ```bash
   docker-compose up -d postgres redis
   ```

   **后端**:
   ```bash
   cd backend
   go mod download
   # 运行服务器
   go run cmd/server/main.go
   ```

   **前端**:
   ```bash
   cd frontend
   npm install
   npm start
   ```

## 🤝 贡献指南

我们欢迎来自社区的贡献！无论是修复 Bug、改进文档还是添加新功能。

1. **Fork 本项目**
2. **创建你的特性分支** (`git checkout -b feature/AmazingFeature`)
3. **提交你的更改** (`git commit -m 'feat: add some AmazingFeature'`)
4. **推送到分支** (`git push origin feature/AmazingFeature`)
5. **提交 Pull Request**

### Commit Message 规范

我们遵循 [Conventional Commits](https://www.conventionalcommits.org/) 规范。这使得查看项目历史时信息更具可读性且易于追踪。

**格式**: `<type>(<scope>): <subject>`

**示例**:
- `feat(auth): support google oauth login`
- `fix(ui): fix mobile layout overflow issue`
- `docs: update readme with manual setup guide`
- `chore: update dependencies`

**常用类型**:
- `feat`: 新增功能
- `fix`: 修复 Bug
- `docs`: 仅文档变更
- `style`: 不影响代码含义的变更（空白、格式化等）
- `refactor`: 既不修复 Bug 也不添加功能的代码变更（重构）
- `perf`: 提高性能的代码变更
- `test`: 添加缺失的测试或更正现有测试
- `chore`: 构建过程或辅助工具和库的变更（如文档生成）

## 📄 许可证

本项目基于 MIT 许可证分发。详见 `LICENSE` 文件。

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=open-prompts/open-prompts&type=date&legend=top-left)](https://www.star-history.com/#open-prompts/open-prompts&type=date&legend=top-left)

---

<div align="center">
由 Open Prompts 社区用心制作 ❤️
</div>

