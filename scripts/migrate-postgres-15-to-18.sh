#!/usr/bin/env bash
set -euo pipefail

# One-time migration for existing lrops.sh installs: Postgres major versions
# don't read each other's data directories, so bumping the pinned image from
# postgres:15-alpine to postgres:18-alpine (see lrops.sh) leaves old installs
# unable to start. This dumps the 15 database logically, swaps the data
# volume, brings up 18, and restores into it.
#
# Usage: migrate-postgres-15-to-18.sh [install_dir]
# Default install_dir: ~/livereview

INSTALL_DIR="${1:-$HOME/livereview}"
COMPOSE_FILE="$INSTALL_DIR/docker-compose.yml"
ENV_FILE="$INSTALL_DIR/.env"
PG_DATA_DIR="$INSTALL_DIR/lrdata/postgres"
DB_CONTAINER="livereview-db"

log() { echo "[migrate] $*"; }
die() { echo "[migrate] ERROR: $*" >&2; exit 1; }

[[ -f "$COMPOSE_FILE" ]] || die "docker-compose.yml not found at $COMPOSE_FILE"
[[ -f "$ENV_FILE" ]] || die ".env not found at $ENV_FILE"

DOCKER_COMPOSE_CMD="docker compose"
command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1 || DOCKER_COMPOSE_CMD="docker-compose"

cd "$INSTALL_DIR"

RUNNING_IMAGE=$(docker inspect --format '{{.Config.Image}}' "$DB_CONTAINER" 2>/dev/null || true)
if [[ "$RUNNING_IMAGE" == *:18* ]]; then
    log "Database container is already on Postgres 18 (image: $RUNNING_IMAGE) — nothing to do."
    exit 0
fi
[[ -n "$RUNNING_IMAGE" ]] || die "Could not detect running $DB_CONTAINER container. Start it first with: $DOCKER_COMPOSE_CMD up -d $DB_CONTAINER"

DB_PASSWORD=$(grep -E '^DB_PASSWORD=' "$ENV_FILE" | cut -d'=' -f2- | tr -d '\r')
[[ -n "$DB_PASSWORD" ]] || die "DB_PASSWORD not found in $ENV_FILE"

DUMP_FILE="$INSTALL_DIR/backups/pg15-to-pg18-$(date +%Y%m%d_%H%M%S).sql"
mkdir -p "$(dirname "$DUMP_FILE")"

log "Stopping app (keeping old DB up for the dump)..."
$DOCKER_COMPOSE_CMD stop livereview-app || true

log "Dumping database from Postgres 15 -> $DUMP_FILE"
docker exec -e PGPASSWORD="$DB_PASSWORD" "$DB_CONTAINER" pg_dump -U livereview -d livereview > "$DUMP_FILE"
[[ -s "$DUMP_FILE" ]] || die "Dump came out empty, aborting before touching data directory"

log "Stopping Postgres 15 container..."
$DOCKER_COMPOSE_CMD stop livereview-db

OLD_DATA_BACKUP="${PG_DATA_DIR}.pg15.$(date +%Y%m%d_%H%M%S)"
log "Moving old data directory aside: $OLD_DATA_BACKUP"
mv "$PG_DATA_DIR" "$OLD_DATA_BACKUP"

log "Starting fresh Postgres 18 container..."
$DOCKER_COMPOSE_CMD up -d livereview-db

log "Waiting for Postgres 18 to become healthy..."
for _ in $(seq 1 30); do
    status=$(docker inspect --format '{{.State.Health.Status}}' "$DB_CONTAINER" 2>/dev/null || echo "starting")
    [[ "$status" == "healthy" ]] && break
    sleep 2
done
[[ "$status" == "healthy" ]] || die "Postgres 18 did not become healthy in time. Old data preserved at $OLD_DATA_BACKUP"

log "Restoring dump into Postgres 18..."
# ON_ERROR_STOP: psql otherwise keeps going past failed statements and still exits 0
docker exec -i -e PGPASSWORD="$DB_PASSWORD" "$DB_CONTAINER" psql -v ON_ERROR_STOP=1 -U livereview -d livereview < "$DUMP_FILE" \
    || die "Restore failed. Old Postgres 15 data is safe at $OLD_DATA_BACKUP; dump at $DUMP_FILE"

log "Starting app..."
$DOCKER_COMPOSE_CMD up -d --force-recreate livereview-app

log "Migration complete."
log "Old Postgres 15 data kept at: $OLD_DATA_BACKUP (delete once you've verified everything)"
log "Dump file kept at: $DUMP_FILE"
