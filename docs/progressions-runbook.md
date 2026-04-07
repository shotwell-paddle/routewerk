# Routewerk Progressions — Implementation Runbook

This document captures every decision made during planning and provides step-by-step instructions for the dev environment setup. All feature code is written against the `dev` branch and staging environment. Production is never touched until an explicit merge to `main`.

---

## Decisions log

These decisions are final unless revisited explicitly.

### Dev environment

- **Stay on Fly Postgres.** No Neon migration. A second Fly Postgres cluster (`routewerk-dev-db`) provides the staging database. Dump/restore from production when fresh data is needed — at our data size this takes seconds.
- **Branch-based deploys.** `main` → production (`routewerk`). `dev` → staging (`routewerk-dev`). Feature branches merge into `dev` via PR.
- **Separate Fly API tokens.** Production and staging use scoped deploy tokens. A misconfigured workflow cannot deploy to the wrong app.
- **Branch protection on `main`.** Requires PR with passing CI and at least one review. No direct pushes.

### Architecture

- **Event bus.** In-memory pub/sub with `sync.WaitGroup` and `Shutdown(ctx)` for graceful drain of async handlers. Services publish events; listeners handle side effects. The bus is injected as a constructor dependency, not a global.
- **Web-only handlers for v1.** No API (JSON) handlers except `GET /api/v1/notifications/count`. The service layer returns clean domain types so API handlers are trivial to add later — a day of work, not a refactoring project.
- **Build on existing infrastructure.** The `notifications` table (migration 000019), `NotificationService`, job queue, `settings_json` column (migration 000014), and `UserSettings` model already exist. We extend these, not duplicate them.

### Schema

- **No dedup constraints on `quest_logs`.** Climbers can log the same route on a quest multiple times in a day (e.g., two separate sessions). The only unique constraint is on `climber_quests(user_id, quest_id) WHERE status = 'active'` — one active enrollment per quest.
- **Route skill tags use a join table** (`route_skill_tags`), matching the existing `route_tags` pattern.
- **Migrations start at 000022.** Current highest is 000021 (`app_admin`).

### Testing

- **Service layer unit tests** — highest priority. Mock repos and bus, test all quest lifecycle logic.
- **Event bus unit tests** — sync/async dispatch, shutdown drain, error handling, typed payloads.
- **Listener unit tests** — verify correct repo calls from event payloads, user settings respected.
- **Repository integration tests** — real Postgres via GitHub Actions service container. Tagged with `//go:build integration` so `go test ./...` still works without a database.
- **Migration round-trip tests** — up/down/up without errors.
- **Handler tests** — `httptest` with injected deps, validate routing, auth, response codes.
- **No browser/E2E tests in v1.**

### Scope

- **Feature flag** (`progressions_enabled` on `gyms` table) gates all climber-facing UI. Admin tools are accessible regardless of the flag.
- **No native mobile app.** All templates are mobile-first (375px+, 44px tap targets). The web app is the only client.
- **Badge library is code-defined.** \~20 pre-made designs in `internal/badge_library.go`. No upload in v1.
- **Share images deferred.** Completion share pages use OG meta tags only. Server-side image generation (fogleman/gg) is post-v1.

---

## Part 1: Dev environment setup

Complete these steps in order. Do not write any feature code until the dev environment is verified.

### Prerequisites

You need:
- `flyctl` CLI installed and authenticated (`fly auth login`)
- Access to the `shotwell-paddle` GitHub org with admin permissions on the `routewerk` repo
- `pg_dump` and `pg_restore` available locally (install via `brew install libpq` and add to PATH)
- The production `DATABASE_URL` from `fly secrets list -a routewerk`

### Step 1: Create the staging Fly Postgres cluster

```bash
# Create a new Postgres cluster for dev/staging
# Use the same region as production (ord = Chicago)
fly postgres create \
  --name routewerk-dev-db \
  --region ord \
  --vm-size shared-cpu-1x \
  --initial-cluster-size 1 \
  --volume-size 1
```

Note the connection string from the output. You'll need it for the next step.

Attach it to a staging app (created in Step 3), or connect directly:

```bash
# Get the dev database connection string
fly postgres connect -a routewerk-dev-db
```

### Step 2: Seed the staging database from production

The `.flycast` addresses only resolve inside Fly's private network. From your local machine, you need to proxy both databases.

Open three terminal tabs:

**Terminal 1** — proxy production DB:
```bash
fly proxy 15432:5432 -a routewerk-db
```

**Terminal 2** — proxy dev DB:
```bash
fly proxy 15433:5432 -a routewerk-dev-db
```

**Terminal 3** — dump and restore:
```bash
# Dump production (prompts for password — use the one from the prod connection string)
pg_dump --no-owner --no-acl -Fc -h localhost -p 15432 -U routewerk -d routewerk -f routewerk_prod.dump

# Restore into dev (prompts for password — use the one from the dev connection string)
pg_restore --no-owner --no-acl -h localhost -p 15433 -U postgres -d postgres routewerk_prod.dump
```

Verify data integrity — connect and spot-check:

```bash
fly postgres connect -a routewerk-dev-db
```

```sql
SELECT COUNT(*) FROM users;
SELECT COUNT(*) FROM routes;
SELECT COUNT(*) FROM locations;
-- Should match production counts
```

For future refreshes, a `make refresh-dev-db` target has been added to the Makefile. It uses the same proxy approach — start both proxies in separate terminals first, then run the target.

**Note:** The `--clean` flag on restore drops existing objects before recreating them. This is safe for the dev database — it gives you a fresh copy of production every time.

### Step 3: Create the staging Fly app

```bash
# Create the staging app (no database attached yet)
fly apps create routewerk-dev
```

Attach the dev Postgres cluster:

```bash
fly postgres attach routewerk-dev-db -a routewerk-dev
```

This sets the `DATABASE_URL` secret on `routewerk-dev` automatically.

Set the remaining secrets. **Use different values from production** for session and JWT secrets:

```bash
fly secrets set -a routewerk-dev \
  SESSION_SECRET="$(openssl rand -hex 32)" \
  JWT_SECRET="$(openssl rand -hex 32)" \
  ENV="staging" \
  FRONTEND_URL="https://routewerk-dev.fly.dev"
```

If you use S3/Tigris storage, set those secrets too. For staging you can either use the same bucket (reads are fine, writes go to the same place) or create a separate dev bucket.

### Step 4: Create `fly.dev.toml`

Create this file in the repo root:

```toml
# fly.dev.toml — staging environment
# NEVER rename this to fly.toml. Production uses fly.toml.
app = "routewerk-dev"
primary_region = "ord"

[build]

[env]
  PORT = "8080"
  ENV = "staging"

[http_service]
  internal_port = 8080
  force_https = true
  auto_stop_machines = "stop"
  auto_start_machines = true
  min_machines_running = 0

  [http_service.concurrency]
    type = "requests"
    hard_limit = 250
    soft_limit = 200

  [[http_service.checks]]
    interval = "30s"
    timeout = "5s"
    grace_period = "10s"
    method = "GET"
    path = "/health"

[[vm]]
  memory = "256mb"
  cpu_kind = "shared"
  cpus = 1
```

Test it manually before setting up CI:

```bash
fly deploy --remote-only --app routewerk-dev --config fly.dev.toml
```

Verify: `curl https://routewerk-dev.fly.dev/health`

### Step 5: Create scoped Fly deploy tokens

This ensures the staging workflow **cannot** deploy to production, and vice versa.

```bash
# Production deploy token (can only deploy to routewerk)
fly tokens create deploy -a routewerk

# Staging deploy token (can only deploy to routewerk-dev)
fly tokens create deploy -a routewerk-dev
```

Add these as **separate** GitHub Actions secrets:
- `FLY_API_TOKEN_PROD` → the production deploy token
- `FLY_API_TOKEN_DEV` → the staging deploy token

**Remove the existing `FLY_API_TOKEN` secret** after migrating CI to the scoped tokens. A single unscoped token is a footgun.

### Step 6: Update GitHub Actions

Replace the existing `.github/workflows/ci.yml` with two workflows.

#### `.github/workflows/ci.yml` — runs on all branches

```yaml
name: CI

on:
  push:
    branches: [main, dev]
  pull_request:
    branches: [main, dev]

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16-alpine
        env:
          POSTGRES_DB: routewerk_test
          POSTGRES_USER: routewerk
          POSTGRES_PASSWORD: password
        ports:
          - 5432:5432
        options: >-
          --health-cmd "pg_isready -U routewerk"
          --health-interval 2s
          --health-timeout 5s
          --health-retries 10
    env:
      TEST_DATABASE_URL: postgres://routewerk:password@localhost:5432/routewerk_test?sslmode=disable
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: "1.24"

      - name: Download dependencies
        run: go mod download

      - name: Vet
        run: go vet ./...

      - name: Unit tests
        run: go test -count=1 -race -coverprofile=coverage.out ./...

      - name: Integration tests
        run: go test -count=1 -race -tags=integration ./...

      - name: Check coverage
        run: go tool cover -func=coverage.out | tail -1

  build:
    runs-on: ubuntu-latest
    needs: test
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: "1.24"

      - name: Build API
        run: CGO_ENABLED=0 go build -o bin/api ./cmd/api

      - name: Build Admin CLI
        run: CGO_ENABLED=0 go build -o bin/admin ./cmd/admin
```

#### `.github/workflows/deploy-prod.yml` — production deploys

```yaml
name: Deploy Production

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    # Only deploy after CI passes (ci.yml runs on main too)
    concurrency:
      group: deploy-production
      cancel-in-progress: true
    steps:
      - uses: actions/checkout@v4

      - uses: superfly/flyctl-actions/setup-flyctl@master

      - name: Deploy to production
        run: flyctl deploy --remote-only --app routewerk
        env:
          FLY_API_TOKEN: ${{ secrets.FLY_API_TOKEN_PROD }}
```

#### `.github/workflows/deploy-dev.yml` — staging deploys

```yaml
name: Deploy Staging

on:
  push:
    branches: [dev]

jobs:
  deploy:
    runs-on: ubuntu-latest
    concurrency:
      group: deploy-staging
      cancel-in-progress: true
    steps:
      - uses: actions/checkout@v4

      - uses: superfly/flyctl-actions/setup-flyctl@master

      - name: Deploy to staging
        run: flyctl deploy --remote-only --app routewerk-dev --config fly.dev.toml
        env:
          FLY_API_TOKEN: ${{ secrets.FLY_API_TOKEN_DEV }}
```

### Step 7: Enable branch protection on `main`

In GitHub → Settings → Branches → Add branch protection rule:

- **Branch name pattern**: `main`
- **Require a pull request before merging**: ✅
  - Required approvals: 1
- **Require status checks to pass before merging**: ✅
  - Required checks: `test`, `build` (from `ci.yml`)
- **Do not allow bypassing the above settings**: ✅ (even for admins)
- **Restrict who can push to matching branches**: ✅ (optional but recommended)

This means: no one can push directly to `main`. Every change goes through a PR with passing CI.

### Step 8: Create the `dev` branch

```bash
git checkout main
git pull origin main
git checkout -b dev
git push -u origin dev
```

Verify the staging deploy triggers and succeeds:

```bash
# Watch the GitHub Actions run
gh run watch

# Or check the staging app
curl https://routewerk-dev.fly.dev/health
```

### Step 9: Verify local dev still works

```bash
docker compose up -d db     # start local Postgres only
cp .env.example .env        # edit DATABASE_URL to local Postgres
make dev                    # should start with hot reload
```

Verify at `http://localhost:8080/health`.

### Step 10: Fix `.env.example` discrepancy

The plan notes that `.env.example` says `FLY_REGION=iad` but `fly.toml` says `primary_region = "ord"`. Fix this:

```bash
# In .env.example, change:
# FLY_REGION=iad
# to:
# FLY_REGION=ord
```

---

## Dev environment verification checklist

Complete every item before writing feature code.

- [ ] Fly Postgres dev cluster created (`routewerk-dev-db`)
- [ ] Production data dumped and restored to dev database
- [ ] Data integrity verified (row counts match)
- [ ] Staging Fly app created (`routewerk-dev`)
- [ ] Staging app secrets set (different SESSION\_SECRET and JWT\_SECRET from prod)
- [ ] `fly.dev.toml` committed to repo
- [ ] Manual staging deploy successful (`curl https://routewerk-dev.fly.dev/health` returns 200)
- [ ] Scoped Fly deploy tokens created (one per app)
- [ ] GitHub secrets updated: `FLY_API_TOKEN_PROD` and `FLY_API_TOKEN_DEV` (old `FLY_API_TOKEN` removed)
- [ ] `ci.yml` updated to run on `main` and `dev` branches, with Postgres service container
- [ ] `deploy-prod.yml` created (deploys on push to `main` only, uses `FLY_API_TOKEN_PROD`)
- [ ] `deploy-dev.yml` created (deploys on push to `dev` only, uses `FLY_API_TOKEN_DEV`)
- [ ] Branch protection enabled on `main` (require PR, require CI, no bypass)
- [ ] `dev` branch created and pushed
- [ ] Staging deploy triggered from `dev` push and succeeded
- [ ] Local dev confirmed working against docker-compose Postgres
- [ ] `.env.example` updated (`FLY_REGION=ord`)
- [ ] No production deploys triggered by any of the above steps

---

## Production safety rules

These rules apply for the entire duration of the progressions build.

1. **All feature work happens on branches off `dev`.** Never branch from `main` for feature work.
2. **Merge to `dev` via PR.** CI must pass. This deploys to staging automatically.
3. **Merge `dev` to `main` only when a phase is complete and verified on staging.** This deploys to production.
4. **Never run `fly` CLI commands against `routewerk` (production) during development.** Use `-a routewerk-dev` for everything.
5. **Never put the production `DATABASE_URL` in `.env`, CI variables, or anywhere a dev process can reach it.** Production credentials live only in Fly's secret store for the `routewerk` app.
6. **Test migrations on staging first.** Every migration merges to `dev`, deploys to staging, and gets verified before going anywhere near `main`.
7. **The `make deploy` target still points at production.** Do not use it during development. Use `fly deploy --config fly.dev.toml -a routewerk-dev` or push to `dev` and let CI handle it.

---

## What comes next

Once the dev environment is verified, the implementation order is:

1. **Event bus** — `internal/event/` with bus interface, memory implementation, event types, typed payloads, unit tests, `Shutdown()` with WaitGroup
2. **Database migrations** — quest tables, activity\_log, progressions\_enabled flag, route\_skill\_tags (starting at 000022)
3. **Models** — domain types in `internal/model/`
4. **Repository layer** — quest, badge, activity, route tag repos with integration tests
5. **Service layer** — quest lifecycle, suggestion algorithm, event publishing, with unit tests
6. **Listeners** — badge award (sync), activity log (async), notification (async), with unit tests
7. **Admin web handlers** — quest CRUD, domain management, badge library, tag coverage
8. **Climber web handlers** — session screen, quest browser, progress logging, completion flow
9. **Notification UI** — bell icon, dropdown, mark-read (extending existing notification system)
10. **Profile** — quest list, badges, radar chart domain map

Each step gets committed, pushed to `dev`, deployed to staging, and verified before moving to the next.
