# Postgres backup & restore runbook

Three modes, one active:

- **Server mode (CURRENT — zero setup)**: the API takes its own nightly
  backup at 09:00 UTC: `pg_dump` against `DATABASE_URL` inside the app
  machine, uploaded PRIVATE to the Tigris storage the app already has
  credentials for (`STORAGE_*` secrets), under `backups/`. 35-day
  retention. See `internal/service/backup.go`. Nothing to set up.
- **Local mode (manual/extra copies)**: pull a dump onto any machine via
  `scripts/backup-local.sh` — kept as a manual tool and for an extra
  off-Tigris copy before risky work.
- **Cloud GHA mode (parked)**: the GitHub Actions workflow pushing to a
  dedicated bucket with its own credentials; re-enable when the app
  scales and credential separation matters.

All sit on top of Fly's ~5-day volume snapshots
(`fly volumes snapshots list`). Restores are identical in every mode:
a `-Fc` dump restored with `pg_restore`.

## Server mode (current)

- **Config** (env, all optional): `BACKUP_ENABLED` (default true; no-ops
  when storage isn't configured), `BACKUP_BUCKET` (default: the photo
  bucket), `BACKUP_PREFIX` (default `backups/`), `BACKUP_HOUR_UTC`
  (default 9), `BACKUP_RETENTION_DAYS` (default 35),
  `BACKUP_RUN_ON_BOOT` (staging sets true so every deploy smoke-tests
  the pipeline end to end).
- **Observability**: `/health` includes `last_backup` (RFC3339 of the
  last success this process lifetime, or `none_this_process`). The
  nightly run also logs `database backup complete` / `database backup
  failed`. Check after any deploy day: a machine restarted at 09:05 UTC
  simply runs again the next night — the key is date-stamped, so a
  same-day re-run overwrites harmlessly.
- **Manual run** (same pipeline):
  `fly ssh console -a routewerk -C "/app/admin backup"`
- **List backups**: with the storage credentials (same as photos):
  `AWS_ACCESS_KEY_ID=... AWS_SECRET_ACCESS_KEY=... AWS_REGION=auto aws s3 ls s3://<bucket>/backups/ --endpoint-url https://fly.storage.tigris.dev`
- **Trade-off (accepted)**: backups share the app's storage credential,
  so this does not protect against that credential being compromised —
  the GHA mode with a separate credential does. Threat model covered
  here: bad migrations, fat-fingered deletes, Fly volume loss.

## Local mode (manual)

**Setup on the backup machine (once):**

1. Install tools: `flyctl` (then `fly auth login`) and a v16 Postgres
   client (`brew install postgresql@16`, or the apt equivalent).
2. Copy `scripts/backup-local.sh` somewhere stable (or clone the repo).
3. Create the config (mode 600):
   ```
   mkdir -p ~/.routewerk && touch ~/.routewerk/backup.env && chmod 600 ~/.routewerk/backup.env
   # contents — the prod DATABASE_URL with host/port swapped to the proxy:
   # BACKUP_DATABASE_URL=postgres://routewerk:<password>@localhost:15432/routewerk?sslmode=disable
   ```
   Get the password once via `fly ssh console -a routewerk -C 'printenv DATABASE_URL'`.
4. Run it once by hand and confirm a dump lands in `~/routewerk-backups/`.

**Scheduling — macOS (launchd), survives sleep better than cron:**

```
# ~/Library/LaunchAgents/com.routewerk.backup.plist
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>com.routewerk.backup</string>
  <key>ProgramArguments</key>
  <array><string>/bin/bash</string><string>/PATH/TO/backup-local.sh</string></array>
  <key>StartCalendarInterval</key>
  <dict><key>Hour</key><integer>3</integer><key>Minute</key><integer>30</integer></dict>
  <key>StandardErrorPath</key><string>/tmp/routewerk-backup.err</string>
</dict></plist>
```
Load with `launchctl load ~/Library/LaunchAgents/com.routewerk.backup.plist`.
If the machine is asleep at the scheduled time, launchd fires the job on
wake; if it's powered off, that night is skipped — hence the freshness
check below.

**Scheduling — Linux (cron):**

```
30 3 * * * /bin/bash /PATH/TO/backup-local.sh >> ~/routewerk-backups/cron.log 2>&1
```

**Freshness check** (the local failure mode is *silent* — machine off,
proxy broken, expired token). Run this weekly by hand, or wire a second
scheduled job; it exits non-zero if the newest dump is older than 2 days:

```
scripts/backup-local.sh --verify-latest
```

Every run also appends to `~/routewerk-backups/backup.log` — glance at
it when in doubt.

## Cloud mode: what runs when

- **Workflow**: `.github/workflows/backup-db.yml`
- **Schedule**: nightly at 09:00 UTC (~04:00 US Central), plus manual
  `workflow_dispatch`.
- **What it does**: opens `flyctl proxy 15432:5432 -a routewerk-db`, runs
  `pg_dump --no-owner --no-acl -Fc` through the tunnel to a file named
  `routewerk-YYYY-MM-DD.dump`, sanity-checks the archive with
  `pg_restore --list`, uploads it to the backup bucket via the aws CLI
  (`--endpoint-url https://fly.storage.tigris.dev`), then deletes uploads
  whose filename date is older than 35 days.
- **Failure mode**: any step failing fails the run. GitHub emails the repo
  owner on scheduled-workflow failures — a red nightly run means no backup
  landed that day; investigate promptly.
- **Connection details** (same as the Makefile targets): prod is user
  `routewerk`, database `routewerk`; staging is user `postgres`, database
  `routewerk_dev`.

Fire a manual run any time:

```
gh workflow run backup-db.yml --repo shotwell-paddle/routewerk
sleep 4 && gh run list --repo shotwell-paddle/routewerk --workflow backup-db.yml --limit 1
gh run watch <run-id> --repo shotwell-paddle/routewerk
```

## Listing available backups

Use the backup bucket credentials (the same values stored in the repo
secrets — keep a copy in your password manager):

```
AWS_ACCESS_KEY_ID=<backup key> AWS_SECRET_ACCESS_KEY=<backup secret> AWS_REGION=auto \
  aws s3 ls s3://<BACKUP_BUCKET>/ --endpoint-url https://fly.storage.tigris.dev
```

Download one:

```
AWS_ACCESS_KEY_ID=<backup key> AWS_SECRET_ACCESS_KEY=<backup secret> AWS_REGION=auto \
  aws s3 cp s3://<BACKUP_BUCKET>/routewerk-2026-07-01.dump . \
  --endpoint-url https://fly.storage.tigris.dev
```

## Verifying a backup

A custom-format dump is self-describing; listing its table of contents
proves the archive is complete and readable (a truncated upload fails
here). Requires `pg_restore` 16 (`brew install postgresql@16` locally).

```
pg_restore --list routewerk-2026-07-01.dump | head -40
```

You should see the schema objects and table data entries. For a stronger
check, do a staging restore drill (next section) — that exercises the full
recovery path, which is the only verification that really counts.

## Restore to STAGING (drill — do this quarterly)

Safe: this only touches `routewerk-dev-db`. Note it clobbers whatever is
on staging (same caveat as `make refresh-dev-db`).

1. Download the dump you want to test (see above).
2. In a separate terminal, open a proxy to the staging DB:

   ```
   fly proxy 15433:5432 -a routewerk-dev-db
   ```

3. Restore (prompts for the staging `postgres` password):

   ```
   pg_restore --clean --no-owner --no-acl -h localhost -p 15433 -U postgres -d routewerk_dev routewerk-2026-07-01.dump
   ```

   Or equivalently: `make restore-staging DUMP=routewerk-2026-07-01.dump`.

   `--clean` drops and recreates objects, so pre-existing staging state
   doesn't collide. Expect harmless "does not exist" notices on a fresh DB.

4. Verify: `curl -s https://routewerk-dev.fly.dev/health`, then log in to
   staging and spot-check recent routes/sessions match prod as of the
   backup date. (If staging is running old code, fire a staging deploy
   first — see CLAUDE.md.)

## Restore to PRODUCTION

> **WARNING: destructive.** This replaces the production database with the
> backup's contents. Everything written after the backup timestamp is
> lost. Only do this for real data-loss recovery, and prefer the most
> recent dump. If the current data is damaged but present, take a manual
> dump of the damaged state first (`make backup-now`) so nothing is
> unrecoverable.

Do the steps one at a time — no chaining.

1. **Scale the app down** so nothing writes during the restore:

   ```
   fly scale show -a routewerk        # note the current count (normally 1)
   fly scale count 0 -a routewerk
   ```

2. **Proxy to the prod DB** (separate terminal, leave it running):

   ```
   fly proxy 15432:5432 -a routewerk-db
   ```

3. **Download the chosen dump** (see "Listing available backups"), verify
   it with `pg_restore --list`, then restore (prompts for the prod
   `routewerk` password):

   ```
   pg_restore --clean --no-owner --no-acl -h localhost -p 15432 -U routewerk -d routewerk routewerk-YYYY-MM-DD.dump
   ```

4. **Scale back up** to the count noted in step 1:

   ```
   fly scale count 1 -a routewerk
   ```

5. **Verify**:

   ```
   curl -s https://routewerk.fly.dev/health
   fly logs -a routewerk --no-tail | grep -E "ERROR|WARN" | tail -20
   ```

   Migrations auto-run on startup; if the dump predates the current code's
   migrations, startup applies the missing ones — watch the logs for
   `migrations applied`. Then log in and spot-check recent data.

## Local one-off backup

```
# Terminal 1:
fly proxy 15432:5432 -a routewerk-db
# Terminal 2:
make backup-now        # writes backups/routewerk-YYYY-MM-DD.dump
```

## One-time setup (operator)

The workflow is **inert until these secrets exist** — the first scheduled
run after merge will fail on the "Check required secrets" step, which is
the reminder to do this.

1. **Create the dedicated backup bucket** (do NOT reuse the app's
   avatar/photo bucket — separate blast radius, separate credentials):

   ```
   fly storage create --name routewerk-backups
   ```

   This prints `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`,
   `BUCKET_NAME`, and the endpoint **once**. Copy them into your password
   manager before closing the terminal.

2. **Build the backup connection string.** Get the prod `DATABASE_URL`:

   ```
   fly ssh console -a routewerk -C 'printenv DATABASE_URL'
   ```

   Swap the host/port to the proxy and force sslmode off (TLS is handled
   by the flyctl tunnel). Shape:

   ```
   postgres://routewerk:<password>@localhost:15432/routewerk?sslmode=disable
   ```

3. **Set the four repo secrets** (each command prompts for the value —
   paste it, don't put secrets on the command line):

   ```
   gh secret set BACKUP_DATABASE_URL --repo shotwell-paddle/routewerk
   gh secret set BACKUP_AWS_ACCESS_KEY_ID --repo shotwell-paddle/routewerk
   gh secret set BACKUP_AWS_SECRET_ACCESS_KEY --repo shotwell-paddle/routewerk
   gh secret set BACKUP_BUCKET --repo shotwell-paddle/routewerk
   ```

   The proxy step reuses the existing `FLY_API_TOKEN_PROD` secret. If that
   token is app-scoped to `routewerk` only, the proxy to `routewerk-db`
   will fail with an auth error — in that case mint a token that can reach
   the DB app (`fly tokens create org`) and update the secret.

4. **First run + drill**: fire a manual run and watch it go green:

   ```
   gh workflow run backup-db.yml --repo shotwell-paddle/routewerk
   sleep 4 && gh run list --repo shotwell-paddle/routewerk --workflow backup-db.yml --limit 1
   gh run watch <run-id> --repo shotwell-paddle/routewerk
   ```

   Then confirm the object exists (see "Listing available backups") and do
   one restore-to-staging drill (see above) so the first time you exercise
   the restore path is not during an incident.
