# E2E smoke test

One Playwright spec (`smoke.spec.ts`) that walks the staff pilot flow
against a real server: password login on the server-rendered `/login`
form (cookie session + CSRF double-submit), SPA dashboard render, route
creation through the SPA form, and the new route showing up in the list.

It runs in CI as the `e2e-smoke` job (`.github/workflows/ci.yml`) against
a scratch Postgres service — **pull requests only** for now, because
`deploy-prod.yml` gates on the whole CI workflow's conclusion and a flake
on push-to-main would silently skip the prod deploy. It is **not** part
of `npm run test` — vitest only includes `src/**/*.test.ts`, and
Playwright only looks at `e2e/`.

## Running locally

Use a scratch database — the seeder writes rows and the test creates
routes. Do not point it at your dev DB. The seeder reads
`E2E_DATABASE_URL` (deliberately not `DATABASE_URL`, which dev shells
export for `make run`) and refuses to run unless the database name
contains `e2e`.

```sh
# 0. one-time: create a scratch DB owned by the app role (OWNER matters:
#    on PG15+ a postgres-owned DB denies CREATE in schema public to
#    other roles, so migrations would fail)
psql -U postgres -c 'CREATE DATABASE routewerk_e2e OWNER routewerk'

# 1. build the SPA + server (from the repo root)
cd web/spa && npm ci && npm run build && cd ../..
go build -tags=spa_embed -o bin/api ./cmd/api

# 2. boot the server against the scratch DB (migrations auto-run)
DATABASE_URL='postgres://routewerk:password@localhost:5432/routewerk_e2e?sslmode=disable' \
ENV=development PORT=8080 FRONTEND_URL=http://localhost:8080 \
BACKUP_ENABLED=false ./bin/api &

# 3. seed the fixture user/org/location/wall (idempotent)
E2E_DATABASE_URL='postgres://routewerk:password@localhost:5432/routewerk_e2e?sslmode=disable' \
go run ./web/spa/e2e/seed

# 4. run the smoke test
cd web/spa
npx playwright install chromium   # first time only
npm run test:e2e                  # E2E_BASE_URL overrides the default :8080
```

## Fixture

`seed/main.go` creates (in order — org → location → settings → wall →
user → membership):

- org "E2E Climbing" → location "E2E Gym" with explicit
  `DefaultLocationSettings` (the smoke test clicks the "Blue" hold-color
  swatch, so the fixture owns its palette)
- one boulder wall "E2E Wall"
- user `e2e-setter@routewerk.test` / `e2e-smoke-password`
- a location-scoped **setter** membership (can create routes)

It reuses the production repositories and `auth.HashPassword`, so the
seeded credentials exercise the same bcrypt path the login form checks.

Idempotent: re-runs no-op when the fixture is complete. If a prior run
died midway (user exists but membership/wall are missing) it fails loudly
— drop and recreate the scratch DB (`DROP DATABASE routewerk_e2e`) and
re-run.

## Debugging

- `npx playwright test --headed` to watch it run.
- On failure a screenshot + trace land in `test-results/`; open traces
  with `npx playwright show-trace <file>`.
- In CI the HTML report is uploaded as the `playwright-report` artifact
  on failure, and the server log is dumped to the job output.
