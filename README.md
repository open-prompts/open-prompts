# Open Prompts

<p align="center">
  <img src="frontend/public/images/logo-lg.jpg" alt="Open Prompts" width="180">
</p>

[English](README.md) | [简体中文](README_zh-CN.md)

[Live Demo](https://awsomeprompt.top) · [Issues](https://github.com/open-prompts/open-prompts/issues) · [Python SDK](python-sdk/README.md)

Open Prompts is a self-hosted web application for publishing and using prompt templates. A template stores versioned prompt content. Users render a version by filling `${variable}` placeholders; the server saves the rendered prompt together with the variable values. The application also includes public discovery, comments, collections, ratings, follows, notifications, reporting, appeals, moderation, and an admin console.

Public templates are visible to other users. Private templates are visible to their owner and to admins through admin endpoints. Public publishing may be reviewed before it becomes visible, depending on the creator's review status and the platform settings.

## Implemented features

### Templates and versions

- Create, update, and delete templates with title, description, content, category, language, tags, and visibility.
- Public and private visibility. Private templates are visible to their owner and to admins through admin endpoints.
- Templates under review (`in_review`) or rejected are hidden from other users.
- A new version is created when the submitted content differs from the latest version. Submitting identical content does not create a new version.
- Versions start at 1 and carry an optional change note of up to 200 bytes (the server checks the UTF-8 byte length).
- The `latest` alias is created with the first version and updated whenever content changes. Custom aliases point to a specific version and can be created, updated, and deleted.
- Forking creates a new private template owned by the forking user and copies the source's latest content into version 1. It does not copy content warnings or fork lineage.
- Locked templates remain visible, but edits and new comments, likes, favorites, ratings, and forks are rejected. Existing comments remain.
- Template language is a metadata field used for filtering. It is independent of the UI language: changing the UI language does not translate template content.
- Single-template JSON export and import in the web UI (`open-prompts-template`, format version 1, 1 MB file limit, up to 50 versions per file).

### Prompt rendering

- Prompt content uses `${name}` placeholders. The web editor extracts these placeholders for value entry.
- Rendering a prompt substitutes supplied values and stores the rendered content and variable map. The REST prompt endpoint also records a `web` use event.
- Saved prompts can be listed and read through the API. Prompt generation does not call an LLM or any model provider.
- The Python SDK exposes the same rendering and prompt retrieval flow.

### Discovery and search

- Public template listing with pagination.
- Keyword search across title, description, category, individual tags, and latest-version content.
- Filters for category, tag, language, and visibility, plus "my likes" and "my favorites".
- Sorting by newest, most liked, most favorited, trending, top rated, and recently updated.
- `exclude_warned` can remove templates with content warnings from public listing results. The filter matches against the built-in warning list (`mature`, `violence`, `profanity`, `sensitive`), not the current platform taxonomy.
- Related templates are scored by shared category (3), each shared tag (2), same language (1), and same author (0.5), then by like count. Results are cached in Redis for five minutes; fewer than three matches yields an empty list.
- "More from author" returns up to four other public templates from the same author, most-liked first.

### Personal dashboard

- `/dashboard` shows counts for the user's templates, saved prompts, collections, and API keys, plus recent activity.
- `/dashboard/templates` lists and manages the user's templates; `/dashboard/prompts` lists saved prompts and lets the user view or delete them.

### Community features

- Like and favorite templates; both counters are shown on template cards and detail pages.
- Rate a template from 1 to 5. A user has at most one rating per template and cannot rate their own template. Private templates cannot be rated by others.
- Comment on templates with 1 to 2,000 characters after trimming. Comments are top-level or one-level replies; replies to replies are rejected and must target a top-level comment. Reply threads are capped at 50.
- Authors can edit their own comments for 15 minutes after posting and delete their own comments. Template owners can delete comments on their templates, and admins can delete comments through the admin console.
- Template details expose `comment_count` for top-level comments. Top-level comments expose `reply_count` for direct replies and a creator-reply flag.
- Follow and unfollow creators, list followers and following, and view a following feed of public templates from followed creators.
- Follows, comments, likes, favorites, and forks create engagement notifications; comment replies notify the parent comment author.

### Collections and creator profiles

- Create, update, and delete collections with a name (1-80 characters), description (up to 500 characters), and public or private visibility; collections default to private. A user can have up to 50 active collections, and a collection holds up to 100 templates.
- Add and remove templates, reorder collection members, and list collections that contain a template.
- Private collections are visible only to their owner. Public collections are listed for discovery.
- Creator profiles show public templates, follower and following counts, and follower/following lists. Public user cards do not expose email, mobile number, or role.

### Notifications

- Notification types include like, favorite, fork, comment, comment reply, follow, review approval or rejection, moderation actions, warning changes, and appeal resolution.
- Unread count, notification list, mark one as read, mark all as read, and delete individual notifications.
- Notification preferences for like, favorite, fork, comment (which also covers comment-reply notifications), and follow, plus a pause-all switch. Review approval or rejection, moderation actions, warning changes, and appeal resolution are not covered by these engagement preferences.
- By default, read notifications are retained for 90 days and all notifications for 180 days; both values are configurable.

### Analytics

- Template view and use counters. The frontend records views (`view`). The server records use events on prompt generation through the REST prompt endpoint (`web`) and on alias reads (`api`).
- View events are de-duplicated per template and session for 30 minutes when a session identifier is provided.
- Anonymous view-event recording is allowed. View-event recording is rate-limited per client IP (default 60 per minute); use events are recorded server-side without that client rate limit.
- Creator analytics supports 7, 30, and 90 day ranges and includes the previous-period comparison, web/API source split, and top templates.
- Per-template stats are owner-only. The event endpoint records `view` events only (`event_type` must be `view`).

### Trust, safety, and moderation

- Content warning taxonomy defaults to `mature`, `violence`, `profanity`, and `sensitive`. The taxonomy can be changed through platform settings.
- Template owners can declare warnings from the current taxonomy. Warnings applied and locked by an admin cannot be removed by the owner.
- Viewers see a warning or blur for warned templates and can choose to show warned content in their profile. Public listing can exclude warned templates.
- Reporting supports templates and comments with reason codes `spam`, `offensive`, `misleading`, `privacy`, and `other`. Users cannot report their own content, and only public content can be reported. A user has one report row per item; reporting the same item again replaces the reason and note and reopens the report. Users can withdraw their own reports, and admins resolve reports by item.
- Appeals are filed against moderation actions. An open-appeal counter with a 24-hour window limits filings per user (`APPEAL_MAX_OPEN`, default 5). Users can list and read their own appeals; admins can list, review, and resolve appeals. Appeal resolution creates a notification.
- Admins review and resolve reports, appeals, and review submissions; moderation actions and admin actions are written to the audit log.

### Pre-publication review

- Creators below the approval threshold (default 3 approved public templates) and creators with remaining re-gate count have public create or publish actions routed to `in_review`. The threshold, re-gate count, and review aging window are platform settings, seeded by `deployment/init.sql` with defaults 3, 3, and 48 hours.
- Reviewers can approve or reject a submission. Rejection requires a reason code: `spam`, `plagiarism`, `harmful`, `lowQuality`, or `other`, with an optional note up to 500 bytes. Rejected templates are hidden from other users.
- A rejected template can be edited and submitted again; the review history records attempts.

### Admin console and administration

- Separate admin login at `/admin/login`; admin API under `/api/v1/admin`.
- Admin sessions use an admin-scoped token with `ADMIN_SESSION_TTL_MINUTES` (default 30 minutes).
- Admin pages: dashboard, users, user detail, templates, template detail, review queue and review detail, audit log, appeals and appeal detail, comments, and reports.
- User administration: search, ban, unban, and change role. Banned users cannot sign in or use API keys.
- Template administration: search, detail, moderation actions (unpublish, lock, soft delete, restore), a configurable retention window (default 30 days) before a daily background job hard-deletes soft-deleted templates, content-warning add/remove with warning history, and moderation state/history.
- Review administration: list pending submissions, inspect details, approve or reject, and view review metrics.
- Comment administration: list and delete comments.
- Report and appeal administration: inspect, group, review, and resolve reports and appeals.
- Audit log for admin actions, including moderation and account actions.
- Platform settings for the review approval threshold, re-gate count, aging window, and the content-warning taxonomy are available through the admin API.

### API, SDK, and localization

- REST/JSON API under `/api/v1` and gRPC on port `50051` expose the same backend services.
- API keys for programmatic access. Keys use the `sk-` prefix, are hashed at rest, show only a short prefix after creation, support optional expiry, and track last use and active state.
- Python SDK `openprompts` supports prompt retrieval, template/version lookup, template listing, comment creation, and comment reply listing.
- UI translations are provided for English and Chinese, with English fallback and browser language detection.
- Light and dark themes are available, with a toggle in the main header and the admin layout.
- Automated accessibility tests run with Jest and jest-axe in the frontend test suite.

## How it works

- A **template** is the container for prompt content, metadata, ownership, and visibility.
- A **version** is an immutable snapshot of the template's prompt content plus a change note.
- An **alias** is a name that maps to a version ID. `latest` is managed by the server; other aliases are user-defined.
- A **prompt** is a rendered and saved instance of a version. The server stores the rendered content and the variable values.
- Public templates are discoverable by other users. Private templates are not listed or accessible to other users; admins can access them through admin endpoints.
- Public publishing may be routed to review for creators below the approval threshold. Review state is stored in the template's moderation status (`in_review`, `rejected`), which is separate from visibility.

The backend exposes both the REST/JSON facade and the gRPC services from the same process. The backend listens on `:8080` for HTTP and `:50051` for gRPC. In the Docker images, Nginx serves the React build and proxies `/api/` to the backend on port 8080.

## Tech stack

| Area | Implementation |
| --- | --- |
| Frontend | React 18.2, Create React App (`react-scripts` 5.0.1), Redux Toolkit 1.9, React Router 6.30, Axios 1.13, i18next 22, Sass |
| Backend | Go 1.25, gRPC, `net/http` REST/JSON facade, protobuf/protojson, JWT (`golang-jwt/jwt/v5`), bcrypt, zap |
| Data | PostgreSQL 15 via `lib/pq`, Redis 7 via `go-redis/v9` |
| Delivery | Docker, Docker Compose, Helm, Nginx, GitHub Actions |
| Tests | Go tests, Jest with jest-axe accessibility tests, Python functional verification, Nightwatch.js end-to-end tests |

The backend uses SQL repositories rather than an ORM. The REST facade and gRPC services call the same service layer.

## Repository layout

```text
backend/
  api/proto/v1/          proto definitions and generated Go code
  cmd/server/main.go     HTTP and gRPC entry point and route registration
  internal/data/         PostgreSQL and Redis clients
  internal/models/       data models
  internal/repository/   SQL repositories
  internal/service/      business logic and gRPC services
  Dockerfile
  Makefile
frontend/
  public/                static assets
  src/a11y/              accessibility tests
  src/components/        shared UI and feature components
  src/pages/             route pages, including admin pages
  src/services/          API clients
  src/store/             Redux store and slices
  src/locales/           en and zh translations
  Dockerfile
  nginx.conf
deployment/
  docker-compose.yml     local multi-service stack
  init.sql               database schema and seed settings
  chart/open-prompts/    Helm chart
test/e2e/                Nightwatch.js end-to-end tests
tests/fvt.py             Python functional verification test runner
docs/
  architecture/          English and Chinese architecture/detailed design docs
  design/                English and Chinese requirements and UI/UX design docs
  testing/               keyboard smoke test
python-sdk/              Python package `openprompts`
```

## Getting started

### Prerequisites

- Docker with Docker Compose v2 for the full stack.
- Go 1.25 to run the backend as a local process.
- Node.js 24.12.0 to run the frontend as a local process (the value used by the Dockerfile and CI).
- Python 3.7 or later for the Python SDK; the functional verification test is run in CI with Python 3.10.
- A reachable SMTP server only if you want verification emails to be delivered.

### Run with Docker Compose

From the repository root:

```bash
cd deployment
docker compose up -d --build
```

If your environment only provides the legacy `docker-compose` command, the same arguments apply.

URLs after the stack starts:

| Service | URL or address |
| --- | --- |
| Web UI | `http://localhost:3000` |
| HTTP API, direct to backend | `http://localhost:8080/api/v1` |
| HTTP API, through the frontend Nginx proxy | `http://localhost:3000/api/v1` |
| gRPC | `localhost:50051` |
| PostgreSQL | `localhost:5432` (local development only) |
| Redis | `localhost:6379` (local development only) |

First start and database initialization:

- `deployment/docker-compose.yml` mounts `deployment/init.sql` into the PostgreSQL container as `01_init.sql`.
- The PostgreSQL entrypoint runs that file only when the database volume is empty. It creates the schema and seed platform settings.
- Existing data is not migrated by starting the compose stack again. If an older volume is reused, apply the schema changes from `deployment/init.sql` yourself or start with a fresh volume.
- To discard local data and re-run the first-start initialization, run `docker compose down -v` and then `docker compose up -d --build` again. `down -v` deletes the database volume and is destructive.

### Create the first admin

The backend reads `ADMIN_BOOTSTRAP_EMAIL` and `ADMIN_BOOTSTRAP_PASSWORD` at startup. To create an admin before the first start, create `deployment/.env` with:

```bash
ADMIN_BOOTSTRAP_EMAIL=admin@example.com
ADMIN_BOOTSTRAP_PASSWORD=replace-with-a-password-of-at-least-8-characters
```

`deployment/.env` is gitignored. Do not commit it.

Then start the stack. When `ADMIN_BOOTSTRAP_EMAIL` is non-empty, `ADMIN_BOOTSTRAP_PASSWORD` must be at least 8 bytes even if the email already exists; otherwise bootstrap fails and the account is not promoted or created. If the email does not exist, bootstrap creates an admin with that password. If the email already exists, the account is promoted to active admin but its password is not changed. If `ADMIN_BOOTSTRAP_EMAIL` is empty, no admin is created. When bootstrap creates a new account, it uses the fixed user ID `admin_bootstrap`; creation fails if that ID already belongs to a different account.

If you add the variables after the stack is already running, restart the backend from the `deployment` directory so the bootstrap runs again:

```bash
docker compose up -d backend
```

Open `http://localhost:3000/admin/login` and sign in with the bootstrap credentials.

### SMTP and verification codes

Registration requires a six-digit email verification code. The code is stored in Redis for five minutes.

If `SMTP_HOST` or `SMTP_PORT` is empty, the backend does not send email and logs the code instead. To read it during local development:

```bash
cd deployment
docker compose logs backend
```

Configure all `SMTP_*` variables in the environment or `deployment/.env` to send codes by email. Port `465` uses implicit TLS; other ports use the standard SMTP client. If SMTP is configured but sending fails, the verification request fails.

The backend also treats addresses ending in `@example.com`, or containing `test` or `fvt` (matching is case-sensitive), as test addresses and uses a fixed code without sending email. The code is logged with the `Test verification code for` prefix. This is intended for the automated tests and should not be used for production accounts.

### Local processes

Start PostgreSQL and Redis only:

```bash
docker compose -f deployment/docker-compose.yml up -d postgres redis
```

Run the backend in another shell:

```bash
cd backend
export DATABASE_URL='postgres://user:password@localhost:5432/open_prompts?sslmode=disable'
export REDIS_ADDR='localhost:6379'
export JWT_SECRET='replace-with-a-long-random-secret'
go run ./cmd/server
```

Run the frontend in another shell:

```bash
cd frontend
npm install
npm start
```

The frontend development server proxies `/api` to `http://localhost:8080` (see `frontend/package.json`). The default ports are:

- Backend HTTP: `8080`
- Backend gRPC: `50051`
- Frontend development server: `3000`

### Kubernetes with Helm

The chart at `deployment/chart/open-prompts` deploys the backend, frontend, and optionally its own PostgreSQL and Redis:

```bash
helm install open-prompts deployment/chart/open-prompts \
  --set-string backend.env.adminBootstrapEmail=admin@example.com \
  --set-string backend.env.adminBootstrapPassword='replace-with-a-password-of-at-least-8-characters' \
  --set-string backend.env.smtpHost=smtp.example.com \
  --set-string backend.env.smtpPort=587 \
  --set-string backend.env.smtpUser=replace-me \
  --set-string backend.env.smtpPassword='replace-me' \
  --set-string backend.env.smtpFrom=open-prompts@example.com
```

Use `helm upgrade` after changing values. The chart's ingress is disabled by default; enable it and set a host if the service should be reachable outside the cluster. The chart uses default image repositories and tags, a default database password, and disabled database persistence. Override the images and credentials, and enable persistence if you use the bundled database.

## Configuration

The backend reads configuration from environment variables. The most important values are:

| Variable | Default | Purpose |
| --- | --- | --- |
| `DATABASE_URL` | local PostgreSQL URL | PostgreSQL connection string used by the backend |
| `REDIS_ADDR` | `localhost:6379` | Redis address used for verification codes, rate limits, caches, and session markers |
| `JWT_SECRET` | built-in development secret | HMAC secret for user and admin JWTs. Set a long random value outside local development. |
| `SMTP_HOST` | empty | SMTP host. If empty, email is skipped and the verification code is logged. |
| `SMTP_PORT` | empty | SMTP port. Port `465` uses implicit TLS. |
| `SMTP_USER` | empty | SMTP user |
| `SMTP_PASSWORD` | empty | SMTP password |
| `SMTP_FROM` | `noreply@open-prompts.com` | From address for verification emails |
| `ADMIN_BOOTSTRAP_EMAIL` | empty | Email to create or promote as an active admin. Empty disables bootstrap. |
| `ADMIN_BOOTSTRAP_PASSWORD` | empty | Bootstrap password. Must be at least 8 bytes when `ADMIN_BOOTSTRAP_EMAIL` is non-empty. Used only when the bootstrap account is created. |
| `ADMIN_SESSION_TTL_MINUTES` | `30` | Admin session lifetime in minutes |
| `NOTIFICATION_READ_RETENTION_DAYS` | `90` | Days to keep read notifications |
| `NOTIFICATION_RETENTION_DAYS` | `180` | Days to keep all notifications |
| `FOLLOW_RATE_PER_MINUTE` | `30` | Follow and unfollow actions per account per minute |
| `NOTIF_PREFS_RATE_PER_MINUTE` | `30` | Notification preference updates per account per minute |
| `TEMPLATE_EVENT_RATE_PER_MINUTE` | `60` | View-event recordings per IP per minute |
| `RATING_RATE_PER_MINUTE` | `20` | Rating changes per account per minute |
| `TOP_RATED_MIN_RATINGS` | `5` | Minimum rating count for a template to appear in top-rated sorting |
| `APPEAL_MAX_OPEN` | `5` | Open-appeal counter limit per user in a 24-hour window |
| `REPORT_RATE_PER_HOUR` | `20` | Reports per account per hour |
| `MOD_ACTION_RATE_PER_MINUTE` | `10` | Moderation writes per admin per minute |
| `MOD_SOFT_DELETE_WINDOW_DAYS` | `30` | Days before a soft-deleted template becomes eligible for hard deletion |
| `COMMENT_RATE_PER_MINUTE` | `5` | Comment writes per account per minute |
| `COMMENT_RATE_PER_DAY` | `50` | Comment writes per account per day |

Notes:

- `JWT_SECRET` and the comment rate limits are read by the backend but are not passed by the provided Docker Compose file or exposed in the Helm values. Set them in the container environment or add them to the deployment configuration before relying on them.
- The values above are the defaults used by the backend when a variable is missing or invalid. The same defaults are reflected in `deployment/docker-compose.yml` and the Helm values for the variables those files forward.
- The compose file and the Helm chart are development-oriented defaults. Review database credentials, persistence, image tags, exposed ports, and secrets before using them outside local development.

## API

### Base paths and authentication

- REST base: `http://localhost:8080/api/v1`
- gRPC: `localhost:50051`
- Admin REST base: `http://localhost:8080/api/v1/admin`
- For REST, JWTs and API keys are sent in the `Authorization` header using the `Bearer` scheme. Set `AUTH` to the full header value before sending an authenticated request:

```bash
SCHEME="Bearer"
AUTH="Authorization: $SCHEME <JWT or API key>"
```

- For gRPC, JWTs use the `Bearer` scheme. API keys use the `ApiKey` scheme in the `authorization` metadata or the `x-api-key` metadata key. The gRPC auth interceptor does not treat a Bearer value as an API key.
- Public reads such as listing templates and reading comments are anonymous. Most write operations require authentication; registration, login, verification-code requests, and view-event recording are unauthenticated. Private templates require the owner for normal reads; admins can read them through admin endpoints. Admin endpoints require an admin-scoped token from `POST /api/v1/admin/login`.
- REST responses use protobuf JSON field names in `snake_case` and include default values because the server is configured with `UseProtoNames` and `EmitUnpopulated`.

### Representative requests

Send a verification code:

```bash
curl -s -X POST http://localhost:8080/api/v1/verification-code \
  -H 'Content-Type: application/json' \
  -d '{"email":"user@example.com","language":"en"}'
```

Register with the code:

```bash
curl -s -X POST http://localhost:8080/api/v1/register \
  -H 'Content-Type: application/json' \
  -d '{"id":"demo_user","email":"user@example.com","password":"replace-me","display_name":"Demo User","verification_code":"123456"}'
```

Register requires a user ID (letters, digits, and underscores), email, password, and verification code. Login accepts an email address or a user ID in the `email` field:

```bash
curl -s -X POST http://localhost:8080/api/v1/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"demo_user","password":"replace-me"}'
```

List public templates:

```bash
curl -s 'http://localhost:8080/api/v1/templates?page_size=10&sort=newest'
```

For the template list, `sort` accepts `newest`, `most_liked`, `most_favorited`, `trending`, `top_rated`, and `updated`. `visibility` accepts `VISIBILITY_PUBLIC` or `VISIBILITY_PRIVATE` when the request is authenticated; anonymous requests list public templates only. `tags` can be repeated as `tags=a&tags=b` or `tags[]=a&tags[]=b`.

Read a latest alias and render and save a prompt:

```bash
curl -s 'http://localhost:8080/api/v1/templates/TEMPLATE_ID/aliases/latest'

curl -s -X POST http://localhost:8080/api/v1/prompts \
  -H "$AUTH" \
  -H 'Content-Type: application/json' \
  -d '{"template_id":"TEMPLATE_ID","version_id":1,"variables":{"article":"example"}}'
```

Use the `id` returned by the alias lookup (or by the create-template response) as `version_id`. It is the numeric version record ID, not the human-readable version number.

Create a template (authenticated user):

```bash
curl -s -X POST http://localhost:8080/api/v1/templates \
  -H "$AUTH" \
  -H 'Content-Type: application/json' \
  -d '{"title":"Summarize an article","description":"A short summary template","content":"Summarize ${article} in three bullets.","category":"writing","tags":["summary"],"language":"en","visibility":"VISIBILITY_PRIVATE"}'
```

The response contains the template and its first version. Content changes through the update endpoint create a new version; the `latest` alias is updated at the same time.

## Python SDK

Install from PyPI:

```bash
pip install openprompts
```

Initialize the client with the server base URL and an API key. The base URL must not include `/api/v1`; the SDK appends the API path itself.

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

`get_prompt` returns the rendered content as a string. It first resolves the alias, then asks the server to render and save a prompt. The POST request requires authentication, so `get_prompt` needs a valid API key. If the prompt-save request fails with an `APIError`, the SDK falls back to local `${key}` substitution using the version content; `AuthError` is not caught and is raised to the caller. The alias lookup itself is a public read, so this method does not resolve private templates.

Available methods:

| Method | Returns | Notes |
| --- | --- | --- |
| `get_prompt(template_id, prompt_tag="latest", variables=None)` | `str` | Resolves the alias, renders, and saves the prompt; requires an API key |
| `get_template_version(template_id, prompt_tag="latest")` | `dict` | Raw version metadata and unrendered content; public alias read |
| `get_template(template_id)` | `dict` | Template detail |
| `list_templates(page_size=10, page_token="", **kwargs)` | `dict` | Additional query filters can be passed as keyword arguments |
| `create_comment(template_id, content, parent_comment_id=None)` | `dict` | Requires authentication; `parent_comment_id` must be a top-level comment |
| `list_comment_replies(comment_id)` | `dict` | Replies to a top-level comment, oldest first, at most 50 |

Exceptions:

- `OpenPromptsError` is the base class for SDK errors.
- `AuthError` is raised for HTTP 401 and 403 responses.
- `APIError` is raised for other non-success HTTP responses and exposes `status_code` and `response`.

## Tests and validation

Run the checks that apply to the part of the project you changed.

Backend:

```bash
cd backend
go test ./...
golangci-lint run
```

Frontend:

```bash
cd frontend
npm install
npm run lint
npm run test:a11y
npm run build
```

Functional verification:

```bash
python3 -m pip install requests
python3 tests/fvt.py
```

The functional verification test starts the Docker Compose stack from `deployment/docker-compose.yml` with `docker compose up -d --build` and expects the backend at `http://localhost:8080`. Docker and Docker Compose v2 are required. The script does not stop the stack when it finishes.

End-to-end tests:

```bash
cd test/e2e
npm install
npm run e2e
```

The Nightwatch end-to-end tests require the application to be reachable at `http://localhost:3000` and a Firefox or Chromium browser driver. See [test/e2e/README.md](test/e2e/README.md) for browser and driver setup, the Firefox default, the Chromium fallback, and the optional compose override for host port conflicts.

## Contributing

1. Fork the repository and create a topic branch.
2. Make the change and add or update tests where the behavior is covered by tests.
3. Run the relevant lint and test commands from the previous section.
4. Commit with a Conventional Commits message, for example `feat(comments): add reply pagination` or `fix(auth): reject banned users`.
5. Open a pull request that describes the behavior change and the validation performed.

Commit messages are checked with commitlint (`@commitlint/config-conventional`) and must follow the `<type>(<scope>): <subject>` format. The allowed types are `build`, `chore`, `ci`, `docs`, `feat`, `fix`, `perf`, `refactor`, `revert`, `style`, and `test`.

## Documentation

- [Architecture and detailed design](docs/architecture)
- [Requirements and UI/UX design](docs/design)
- [Keyboard smoke test](docs/testing/keyboard-smoke-test.md)
- [Python SDK](python-sdk/README.md)
- [End-to-end tests](test/e2e/README.md)

Some documents under `docs/design` and `docs/architecture` describe proposed or in-progress work. They are not a complete list of shipped behavior; the code in `backend/` and `frontend/src/` is the reference for what the application does.

## Scope and current limitations

- The application renders prompt templates and stores the rendered prompt. It does not execute prompts or call an LLM provider.
- Users can change their password from the profile page while signed in. There is no email-based forgot-password or reset flow, and no email-address change or account-deletion flow.
- OAuth login is declared in the API but not implemented; only email and password sign-in is available.
- There is no real-time collaborative editing.

## License

Licensed under the Apache License 2.0. See [LICENSE](LICENSE).
