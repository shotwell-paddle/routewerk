#!/usr/bin/env bash
# backup-local.sh — pull a nightly logical backup of the production
# Postgres (routewerk-db) onto a local machine.
#
# MANUAL/EXTRA tool: the default backup is now server-side (the API
# dumps itself nightly to Tigris — see internal/service/backup.go and
# docs/backup-restore.md). Use this script for an additional off-Tigris
# copy, or ad-hoc dumps before risky work.
#
# Requirements on the backup machine:
#   - flyctl, authenticated (`fly auth login`) or FLY_API_TOKEN in env
#   - postgresql client v17+ (`brew install postgresql@17` / apt equivalent)
#     (client major must be >= the server major; staging runs PG 17)
#   - config file (chmod 600) at ~/.routewerk/backup.env containing:
#       BACKUP_DATABASE_URL=postgres://routewerk:<password>@localhost:15432/routewerk?sslmode=disable
#     (the prod DATABASE_URL with host/port swapped to the local proxy)
#
# Usage:
#   backup-local.sh                 run a backup + prune old dumps
#   backup-local.sh --verify-latest exit non-zero if newest dump > MAX_AGE_DAYS old
#
# Tunables (env or backup.env):
#   BACKUP_DIR        default ~/routewerk-backups
#   RETENTION_DAYS    default 35
#   MAX_AGE_DAYS      default 2   (for --verify-latest)
#   PROXY_PORT        default 15432

set -euo pipefail

CONFIG_FILE="${CONFIG_FILE:-$HOME/.routewerk/backup.env}"
if [ -f "$CONFIG_FILE" ]; then
  # shellcheck disable=SC1090
  . "$CONFIG_FILE"
fi

BACKUP_DIR="${BACKUP_DIR:-$HOME/routewerk-backups}"
RETENTION_DAYS="${RETENTION_DAYS:-35}"
MAX_AGE_DAYS="${MAX_AGE_DAYS:-2}"
PROXY_PORT="${PROXY_PORT:-15432}"
LOG_FILE="$BACKUP_DIR/backup.log"

mkdir -p "$BACKUP_DIR"

log() {
  echo "$(date -u '+%Y-%m-%dT%H:%M:%SZ') $*" | tee -a "$LOG_FILE"
}

# ── --verify-latest: freshness check for a second cron / manual ──
if [ "${1:-}" = "--verify-latest" ]; then
  latest=$(ls -t "$BACKUP_DIR"/routewerk-*.dump 2>/dev/null | head -1 || true)
  if [ -z "$latest" ]; then
    log "VERIFY FAIL: no backups found in $BACKUP_DIR"
    exit 1
  fi
  # Portable-enough mtime check (GNU + BSD stat).
  if [ -n "$(find "$latest" -mtime +"$MAX_AGE_DAYS" 2>/dev/null)" ]; then
    log "VERIFY FAIL: newest backup $latest is older than $MAX_AGE_DAYS days"
    exit 1
  fi
  log "VERIFY OK: $latest"
  exit 0
fi

if [ -z "${BACKUP_DATABASE_URL:-}" ]; then
  log "FAIL: BACKUP_DATABASE_URL not set (expected in $CONFIG_FILE)"
  exit 1
fi

command -v flyctl >/dev/null || { log "FAIL: flyctl not installed"; exit 1; }
command -v pg_dump >/dev/null || { log "FAIL: pg_dump not installed"; exit 1; }

OUT="$BACKUP_DIR/routewerk-$(date -u +%Y-%m-%d).dump"

# ── proxy to the prod DB, with cleanup on every exit path ────
flyctl proxy "$PROXY_PORT:5432" -a routewerk-db &
PROXY_PID=$!
trap 'kill "$PROXY_PID" 2>/dev/null || true' EXIT

for i in $(seq 1 30); do
  pg_isready -h localhost -p "$PROXY_PORT" -q 2>/dev/null && break
  sleep 2
done
pg_isready -h localhost -p "$PROXY_PORT" -q || { log "FAIL: proxy to routewerk-db never became ready"; exit 1; }

# ── dump + verify the archive is readable ────────────────────
pg_dump --no-owner --no-acl -Fc -d "$BACKUP_DATABASE_URL" -f "$OUT"
pg_restore --list "$OUT" > /dev/null
size=$(du -h "$OUT" | cut -f1)
log "OK: wrote $OUT ($size)"

# ── prune beyond retention ───────────────────────────────────
pruned=$(find "$BACKUP_DIR" -name 'routewerk-*.dump' -mtime +"$RETENTION_DAYS" -print -delete | wc -l | tr -d ' ')
[ "$pruned" != "0" ] && log "pruned $pruned dump(s) older than $RETENTION_DAYS days"

exit 0
