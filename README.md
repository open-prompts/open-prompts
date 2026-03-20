# Open Prompts

<div align="center">

![Open Prompts](frontend/public/images/logo.jpg)

**Your Ultimate Hub to Discover, Organize, and Share Best AI Prompts**

[Live Demo](https://awsomeprompt.top) • [Report Bug](https://github.com/open-prompts/open-prompts/issues) • [Request Feature](https://github.com/open-prompts/open-prompts/issues)

[English](./README.md) | [简体中文](./README_zh-CN.md)

</div>

## 📖 Introduction

**Open Prompts** is a community-driven platform designed to help users harness the full potential of Large Language Models (LLMs) like ChatGPT, Claude, and Gemini. It provides a structured environment to create, test, version, and share high-quality prompts.

Whether you are a prompt engineer, a developer, or an AI enthusiast, Open Prompts manages your prompt library with the same rigor as code—featuring version control, tagging, and collaboration tools.

## ✨ Features

- **📚 Centralized Prompt Library**: Store and organize all your AI prompts in one place.
- **🔄 Version Control**: Track changes to your prompts, fork existing templates, and maintain a history of iterations.
- **🌍 Multi-language Support**: Native support for Internationalization (English & Chinese).
- **🎨 Modern UI/UX**: Built with IBM's **Carbon Design System** for a professional, accessible, and responsive interface.
- **📱 One-Click Sharing**: Share your best prompts with the community or keep them private.
- **🔐 Secure Authentication**: Robust user management and authentication system (JWT).

## 🛠 Tech Stack

### Frontend
- **Framework**: [React 18](https://reactjs.org/)
- **State Management**: [Redux Toolkit](https://redux-toolkit.js.org/)
- **UI System**: [Carbon Design System](https://carbondesignsystem.com/) (SASS)
- **HTTP Client**: Axios
- **i18n**: i18next

### Backend
- **Language**: [Go (Golang)](https://go.dev/) 1.25+
- **API Protocol**: gRPC & RESTful JSON
- **Database**: PostgreSQL
- **Caching**: Redis
- **Auth**: JWT (JSON Web Tokens)

### Infrastructure
- **Containerization**: Docker & Docker Compose
- **Reverse Proxy**: Nginx
- **CI/CD**: Github Actions

## 🚀 Getting Started

Follow these steps to set up the project locally.

### Prerequisites
- [Docker](https://www.docker.com/) & [Docker Compose](https://docs.docker.com/compose/)
- [Node.js](https://nodejs.org/) (v16+) - *If running frontend locally*
- [Go](https://go.dev/) (v1.23+) - *If running backend locally*

### Installation

1. **Clone the repository**
   ```bash
   git clone https://github.com/open-prompts/open-prompts.git
   cd open-prompts
   ```

2. **Run with Docker Compose (Recommended)**
   The easiest way to start the entire stack.
   ```bash
   cd deployment
   docker-compose up -d
   ```
   The database will be automatically initialized with the schema and seed data on the first run.

   - **Frontend**: `http://localhost:3000`
   - **Backend API**: `http://localhost:3000/api` (Proxied) or `http://localhost:8080` (Direct)

3. **Manual Development Setup**

   **Database**:
   Start Postgres and Redis using Docker:
   ```bash
   docker-compose up -d postgres redis
   ```

   **Backend**:
   ```bash
   cd backend
   go mod download
   # Run the server
   go run cmd/server/main.go
   ```

   **Frontend**:
   ```bash
   cd frontend
   npm install
   npm start
   ```

## 🤝 Contributing

We welcome contributions from the community! Whether it's fixing bugs, improving documentation, or adding new features.

1. **Fork the Project**
2. **Create your Feature Branch** (`git checkout -b feature/AmazingFeature`)
3. **Commit your Changes** (`git commit -m 'feat: add some AmazingFeature'`)
4. **Push to the Branch** (`git push origin feature/AmazingFeature`)
5. **Open a Pull Request**

### Commit Message Convention

We follow the [Conventional Commits](https://www.conventionalcommits.org/) specification. This leads to more readable messages that are easy to follow when looking through the project history.

**Format**: `<type>(<scope>): <subject>`

**Examples**:
- `feat(auth): support google oauth login`
- `fix(ui): fix mobile layout overflow issue`
- `docs: update readme with manual setup guide`
- `chore: update dependencies`

**Common Types**:
- `feat`: A new feature
- `fix`: A bug fix
- `docs`: Documentation only changes
- `style`: Changes that do not affect the meaning of the code (white-space, formatting, etc)
- `refactor`: A code change that neither fixes a bug nor adds a feature
- `perf`: A code change that improves performance
- `test`: Adding missing tests or correcting existing tests
- `chore`: Changes to the build process or auxiliary tools and libraries such as documentation generation

## 📄 License

Distributed under the MIT License. See `LICENSE` for more information.

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=open-prompts/open-prompts&type=date&legend=top-left)](https://www.star-history.com/#open-prompts/open-prompts&type=date&legend=top-left)

---

<div align="center">
Made with ❤️ by the Open Prompts Community
</div>