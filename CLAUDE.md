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

**Deploys are automated via GitHub Actions.** `.github/workflows/`:

- `ci.yml` — runs `go vet`, unit + integration tests, and `go build` on every push and PR to `main`/`dev`. Required status checks for merging.
- `deploy-dev.yml` — on push to `dev`, runs `flyctl deploy --remote-only --app routewerk-dev --config fly.dev.toml`.
- `deploy-prod.yml` — on push to `main`, runs `flyctl deploy --remote-only --app routewerk`.

So the canonical release flow is: merge PR into `dev` → staging auto-deploys; merge `dev → main` PR → production auto-deploys. Do not run `make deploy` unless GHA is broken or you're doing an emergency rollback — it bypasses CI and deploys from your local working directory, which is exactly how we once shipped an old `main` because the merge hadn't actually landed yet.

Config: `fly.toml` (prod, app `routewerk`), `fly.dev.toml` (staging, app `routewerk-dev`). Secrets set via `fly secrets set KEY=value -a <app>`.

Required Fly.io secrets: `DATABASE_URL`, `JWT_SECRET`, `SESSION_SECRET`, `FRONTEND_URL`, `STORAGE_*` (if using S3).

The Dockerfile builds both the API server and admin CLI. Migrations run automatically on startup.

### Watching a deploy

```
gh run list --repo shotwell-paddle/routewerk --workflow deploy-prod.yml --limit 5
gh run watch <run-id> --repo shotwell-paddle/routewerk
fly releases --app routewerk
fly logs --app routewerk
```

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

## Shell / tooling notes for Claude

- **Do not use heredocs.** My zsh wedges on `$(cat <<'EOF' ... EOF)` (stuck at `dquote cmdsubst heredoc>`). For anything multi-line (commit messages, PR bodies), write the content to a file first and pass it with a flag:
  - Good: write `/tmp/msg.txt` with the Write tool, then `git commit -F /tmp/msg.txt`
  - Good: `printf '%s\n' 'line 1' '' 'line 2' > /tmp/msg.txt && git commit -F /tmp/msg.txt`
  - Bad: `git commit -m "$(cat <<'EOF' ... EOF)"`
- Single-line `-m 'short message'` is fine.
- For PR bodies, prefer `gh pr create --body-file /tmp/body.md` or `gh pr create --web`.

## Git / GitHub workflow for this repo

The canonical remote is **`shotwell-paddle/routewerk`**. My local git config has sometimes pointed at a different remote depending on which directory I ran `gh` from, so **always pass `--repo shotwell-paddle/routewerk` to `gh` commands** unless you've just verified `gh repo view` resolves correctly.

### Branch model

- `main` — production. Only updated via PRs (never direct push).
- `dev` — staging / integration. Feature branches merge here first.
- Feature branches (`fix/...`, `feat/...`, `hotfix/...`) — branch off `dev` (or `main` for hotfixes).
- **Squash-merge feature PRs into `dev`.** "Create a merge commit" caused divergence (orphan merge commits on main, add/add conflicts on the next release) — stay on squash.
- **For `dev → main` release PRs, use "Rebase and merge" — NOT squash.** This brings dev's individual feature commits onto main (instead of one squashed "Release:" commit), so main's history stays bisectable per feature.
- **After the release PR merges, force-push dev to match main** so the two stay aligned:
  ```
  git checkout dev && git fetch origin && git reset --hard origin/main && git push --force-with-lease origin dev
  ```
  Why this is needed: GitHub's "Rebase and merge" rewrites committer dates and produces new SHAs even when a true fast-forward was possible (verified 2026-04-30 after #27). The only mechanism that would skip the resync is `git push origin origin/dev:main` from local, but main's branch protection requires PR-merging, so direct pushes are blocked. Squash-merge has the same drift; merge-commits cause add/add conflicts on the next release. Resync is the cheapest of the three.
- If a hotfix landed directly on main between releases, sync dev first (`git checkout dev && git merge --ff-only origin/main && git push`) before opening the next release PR.

### Standard flow: feature → dev → main → deploy

```
# branch off dev
git checkout dev && git pull --ff-only
git checkout -b fix/short-description

# work, commit small logical commits
git commit -m 'short imperative message'
git push -u origin fix/short-description

# open PR into dev (NOT main)
gh pr create --repo shotwell-paddle/routewerk --base dev \
  --title 'short title' --body-file /tmp/body.md

# after review + CI green, squash-merge
gh pr merge <N> --repo shotwell-paddle/routewerk --squash --delete-branch

# release to prod: open dev → main PR
gh pr create --repo shotwell-paddle/routewerk --base main --head dev \
  --title 'Release: ...' --body-file /tmp/body.md

# after CI green, rebase-merge (not squash) — brings dev's commits onto main
gh pr merge <N> --repo shotwell-paddle/routewerk --rebase

# deploy is automatic via deploy-prod.yml on push to main.

# resync dev to main (rebase-merge re-stamped SHAs, so they drift content-equal)
git checkout dev && git fetch origin && git reset --hard origin/main && git push --force-with-lease origin dev
```

### gh merge gotchas

- `gh pr merge <N> --squash --admin` bypasses the "branch must be up to date" rule, but **does NOT bypass required status checks**. If it errors with `required status checks have not succeeded`, run `gh pr checks <N>` — the merge will only go through once checks are green.
- If a PR shows `mergeStateStatus: BEHIND`, the base moved since the PR was pushed. Either update the branch (`gh pr update-branch <N>`) or use `--admin` to bypass.
- If `gh pr create` reports "No commits between main and <branch>", the branch is already merged into the base — don't keep trying, the work is there. Check `git log origin/main --oneline | grep <commit-sha>`.

### Force-push safety

- **Never use `git push --force`.** Use `--force-with-lease` so we refuse to overwrite remote work that was pushed since we last fetched.
- Don't ever force-push `main` or `dev`. If those are wrong, fix forward with a new commit/PR.
- Feature branches I own: `--force-with-lease` is fine after a rebase or amend.

### Recovering from a bad reset

Common pattern: a commit gets "lost" because a branch was reset to the wrong ref.

1. `git reflog` — every HEAD move is there, find the SHA.
2. Also check remote refs: `git log origin/<branch> --oneline -20` — the commit may still be on the remote even if the local branch lost it.
3. To recover into a new branch:
   ```
   git checkout main && git pull --ff-only
   git checkout -b hotfix/description
   git cherry-pick <sha>
   git push -u origin hotfix/description
   gh pr create --repo shotwell-paddle/routewerk --base main --title '...' --body-file /tmp/body.md
   ```

### Before running destructive commands

Before any `git reset --hard`, `git push --force-with-lease`, branch delete, or `make deploy`, **stop and state what you're about to do** and wait for me to confirm. The last time we chained reset→push→deploy without pausing, we deployed an old main because an earlier merge hadn't actually landed. Pause between the three.

### Commit messages

- Imperative mood, short title (≤ 70 chars), blank line, then body if needed.
- Use conventional-ish prefixes that already exist in history: `fix(scope):`, `feat(scope):`, `chore(scope):`, `docs:`, `refactor(scope):`.
- Multi-line bodies go through a file (see heredoc note above).
