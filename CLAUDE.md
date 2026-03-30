# Routewerk

Route setting management platform for climbing gyms. Multi-tenant SaaS: orgs own locations (gyms), each with walls, routes, and setting sessions.

## Stack

- **Backend**: Go 1.24, chi router, pgx/v5 (PostgreSQL 16), golang-migrate
- **Web frontend**: Server-rendered HTML with `html/template` + HTMX, embedded static assets
- **Mobile**: Responsive web (mobile-first HTMX). REST API exists for future native app if needed (Capacitor/TWA wrapper or standalone Flutter).
- **Hosting**: Fly.io (single region: iad), Dockerfile with multi-stage Alpine build
- **Storage**: S3-compatible (Tigris on Fly.io) for avatars and route photos

## Running locally

```
cp .env.example .env   # edit DATABASE_URL, JWT_SECRET, SESSION_SECRET
make migrate            # run DB migrations
make dev                # live-reload via air (or `make run` for plain start)
```

Tests: `make test` or `go test ./...`

## Project structure

```
cmd/api/         # Main server entrypoint
cmd/admin/       # CLI for migrations, org creation, member management
internal/
  config/        # Env-based config with production validation
  database/      # pgxpool connection + embedded migrations
  handler/       # REST API handlers (JSON, one file per domain)
  handler/web/   # HTMX web handlers (server-rendered HTML)
  middleware/     # Auth (JWT + cookie sessions), authz, CSRF, rate limiting, logging, metrics
  model/         # Domain types (no DB logic)
  repository/    # SQL queries via pgx (one file per table/domain)
  router/        # Chi route registration (API v1 + web frontend)
  service/       # Business logic (auth, card generation, storage, audit)
web/
  templates/     # Go HTML templates (setter/, climber/, shared/)
  static/        # CSS, JS (htmx.min.js, app.js), embedded into binary
```

## Key conventions

- **No ORM**. All SQL is hand-written in repository files. Use pgx directly.
- **Table-driven tests** with `t.Run()` subtests. Standard `testing` package only (no testify).
- **Grades use plus/minus format** (5.9-, 5.9, 5.9+), not letter grades (5.9a-d), for sport/top rope.
- **Role hierarchy**: climber(1) < setter(2) < head_setter(3) < gym_manager(4) < org_admin(5). Checked via `middleware.RoleRankValue()`.
- **Web auth**: Cookie-based sessions (SHA-256 hashed tokens in DB). API auth: JWT + refresh tokens.
- **CSRF**: Double-submit cookie pattern. All POST/PUT/PATCH/DELETE web requests require a token.
- **File organization**: Keep handler files under ~500 lines. When a file grows past that, split by domain (e.g., climber.go split into climber.go + gym_join.go + route_cards.go).
- **Imports**: Group stdlib, then third-party, then internal packages. No blank lines between internal imports.
- **Error handling**: Web handlers render error pages via `h.renderError()`. API handlers return JSON via `handler.Error()`.
- **Context helpers**: Use `middleware.GetWebUser(ctx)`, `middleware.GetWebRole(ctx)`, etc. Context keys are unexported; cross-package test access via `middleware.SetWebUser()` etc.

## Database migrations

Migrations live in `internal/database/migrations/` as numbered `.up.sql`/`.down.sql` pairs. They're embedded into the binary and auto-run on API startup. To add a migration:

```
# Create files: internal/database/migrations/000016_description.up.sql / .down.sql
# They run automatically on next `make run` or deploy
```

Admin CLI: `./bin/admin migrate`, `migrate-down`, `migrate-version`, `migrate-force N`

## Deployment

Fly.io via `make deploy` (runs `fly deploy`). Config in `fly.toml`. Secrets set via `fly secrets set KEY=value`.

Required Fly.io secrets: `DATABASE_URL`, `JWT_SECRET`, `SESSION_SECRET`, `FRONTEND_URL`, `STORAGE_*` (if using S3).

The Dockerfile builds both the API server and admin CLI. Migrations run automatically on startup.

## Two interfaces, one server

The server serves both the HTMX web app and the REST API from a single binary:

- **Web** (`/login`, `/dashboard`, `/routes`, `/sessions`, etc.): Cookie auth, CSRF, server-rendered HTML, rate limited at 120 req/min/IP
- **API** (`/api/v1/...`): JWT auth, JSON responses, rate limited at 20 req/min/IP for auth endpoints

Both share the same repositories, services, and database. The web handler (`handler/web/`) renders templates; the API handler (`handler/`) returns JSON.

## Testing notes

- No database available in CI/test sandbox. All tests are unit tests against pure functions, HTTP handlers with `httptest`, and middleware with context injection.
- Repository layer (21 files) has no tests because it's all SQL against pgx. Integration tests would need a real Postgres instance.
- `internal/middleware/testing_helpers.go` exports context setters for cross-package test use.
- Profanity filter uses whole-word tokenization (not substring matching) — "grass" is clean even though "ass" is blocked.
