#!/bin/bash

set -euo pipefail

# Determine which env file to use
ENV_FILE=".env"
if [ "${1:-}" = "--prod" ]; then
    ENV_FILE=".env.prod"
    shift  # Remove --prod from arguments
fi

# Load environment variables from env file
if [ -f "$ENV_FILE" ]; then
    echo "Using environment file: $ENV_FILE"
    export $(grep -v '^#' "$ENV_FILE" | xargs)
else
    echo "ERROR: $ENV_FILE not found"
    exit 1
fi

# Parse DATABASE_URL to extract connection details
# DATABASE_URL format: postgres://user:password@host:port/database?sslmode=disable
if [ -z "${DATABASE_URL:-}" ]; then
    echo "ERROR: DATABASE_URL not found in .env file"
    exit 1
fi

# Extract components from DATABASE_URL
# Remove the postgres:// prefix
DB_URL_WITHOUT_SCHEME="${DATABASE_URL#postgres://}"
# Extract user:password@host:port/database?params
USER_PASS_HOST_PORT_DB="${DB_URL_WITHOUT_SCHEME%\?*}"
# Extract user:password part
USER_PASS="${USER_PASS_HOST_PORT_DB%@*}"
# Extract host:port/database part
HOST_PORT_DB="${USER_PASS_HOST_PORT_DB#*@}"
# Extract user and password
PG_USER="${USER_PASS%:*}"
PG_PASSWORD="${USER_PASS#*:}"
# Extract host:port and database
HOST_PORT="${HOST_PORT_DB%/*}"
PG_DB="${HOST_PORT_DB#*/}"
# Extract port (default to 5432 if not specified)
if [[ "$HOST_PORT" == *:* ]]; then
    PG_PORT="${HOST_PORT#*:}"
else
    PG_PORT="5432"
fi

# Config for the "livereview" app
PG_CONTAINER_NAME="livereview_pg"
PG_VERSION="15"
PG_VOLUME_NAME="livereview_pgdata"
LEGACY_PG_DATA_DIR="./.livereview_pgdata"

usage() {
  echo "Usage: $0 [--prod] {start|stop|status|logs|info|rm|reset|migrations|conn|shell|migrate-legacy-data}"
  echo ""
  echo "Options:"
  echo "  --prod    Use .env.prod instead of .env"
  echo ""
  echo "Commands:"
  echo "  start       Start PostgreSQL container"
  echo "  stop        Stop PostgreSQL container"
  echo "  status      Show container status"
  echo "  logs        Tail container logs"
  echo "  info        Show connection info"
  echo "  rm          Remove container"
  echo "  reset       Reset database (destructive)"
  echo "  migrations  Setup dbmate"
  echo "  conn        Print connection string"
  echo "  shell       Open psql shell or run SQL with -c"
  echo "  migrate-legacy-data  Copy data from $LEGACY_PG_DATA_DIR into Docker volume"
  exit 1
}

ensure_pg_volume() {
  if ! docker volume inspect "$PG_VOLUME_NAME" >/dev/null 2>&1; then
    echo "Creating Docker volume: $PG_VOLUME_NAME"
    docker volume create "$PG_VOLUME_NAME" >/dev/null
  fi
}

is_pg_volume_empty() {
  docker run --rm -v "$PG_VOLUME_NAME":/volume postgres:"$PG_VERSION" sh -c '[ -z "$(ls -A /volume 2>/dev/null)" ]'
}

pg_data_mount_type() {
  docker inspect --format '{{ range .Mounts }}{{ if eq .Destination "/var/lib/postgresql/data" }}{{ .Type }}{{ end }}{{ end }}' "$PG_CONTAINER_NAME"
}

start_pg() {
  ensure_pg_volume

  if docker ps -a --format '{{.Names}}' | grep -qw "$PG_CONTAINER_NAME"; then
    local mount_type
    mount_type="$(pg_data_mount_type)"
    if [ "$mount_type" != "volume" ]; then
      echo "ERROR: Existing container $PG_CONTAINER_NAME uses legacy non-volume storage ($mount_type)."
      echo "Run './pgctl.sh migrate-legacy-data', then './pgctl.sh rm', then './pgctl.sh start'."
      exit 1
    fi

    echo "Container already exists. Starting..."
    docker update --restart unless-stopped "$PG_CONTAINER_NAME" >/dev/null
    docker start "$PG_CONTAINER_NAME"
  else
    if [ -d "$LEGACY_PG_DATA_DIR" ] && is_pg_volume_empty; then
      echo "WARNING: Legacy data directory found at $LEGACY_PG_DATA_DIR"
      echo "Run './pgctl.sh migrate-legacy-data' before start if you need existing local data."
    fi

    echo "Creating and starting new PostgreSQL container for livereview..."
    docker run -d \
      --name "$PG_CONTAINER_NAME" \
      --restart unless-stopped \
      -e POSTGRES_USER="$PG_USER" \
      -e POSTGRES_PASSWORD="$PG_PASSWORD" \
      -e POSTGRES_DB="$PG_DB" \
      -v "$PG_VOLUME_NAME":/var/lib/postgresql/data \
      -p "$PG_PORT":5432 \
      postgres:"$PG_VERSION"
  fi
}

stop_pg() {
  if docker ps -a --format '{{.Names}}' | grep -qw "$PG_CONTAINER_NAME"; then
    docker stop "$PG_CONTAINER_NAME"
  else
    echo "Container does not exist."
  fi
}

status_pg() {
  docker ps -a --filter name="^/${PG_CONTAINER_NAME}$"
}

logs_pg() {
  docker logs -f "$PG_CONTAINER_NAME"
}

info_pg() {
  echo ""
  echo "Livereview PostgreSQL DB Info:"
  echo "  Host:     localhost"
  echo "  Port:     $PG_PORT"
  echo "  User:     $PG_USER"
  echo "  Password: $PG_PASSWORD"
  echo "  Database: $PG_DB"
  echo "  Volume:   $PG_VOLUME_NAME"
  echo ""
}

rm_pg() {
  echo "Stopping and removing container (Docker volume is preserved)..."
  docker rm -f "$PG_CONTAINER_NAME" || true
}

reset_pg() {
  echo "WARNING: This will permanently delete all data and recreate the database."
  read -p "Are you sure you want to continue? [y/N] " -n 1 -r
  echo
  if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "Stopping and removing container..."
    docker rm -f "$PG_CONTAINER_NAME" || true

    echo "Removing Docker volume: $PG_VOLUME_NAME"
    docker volume rm -f "$PG_VOLUME_NAME" || true

    echo "Creating Docker volume: $PG_VOLUME_NAME"
    docker volume create "$PG_VOLUME_NAME" >/dev/null
    
    echo "Recreating database container..."
    start_pg
    
    echo "Waiting for PostgreSQL to initialize..."
    sleep 5
    
    echo "Running database migrations..."
    if command -v dbmate &> /dev/null; then
        dbmate up
    else
        echo "dbmate not found, attempting to run via 'make river-migrate'"
        make river-migrate
    fi
    
    echo "Database reset and recreation complete."
  else
    echo "Reset cancelled."
  fi
}

migrate_legacy_data() {
  if [ ! -d "$LEGACY_PG_DATA_DIR" ]; then
    echo "ERROR: Legacy data directory not found: $LEGACY_PG_DATA_DIR"
    exit 1
  fi

  if docker ps --format '{{.Names}}' | grep -qw "$PG_CONTAINER_NAME"; then
    echo "ERROR: $PG_CONTAINER_NAME is running. Stop it first with './pgctl.sh stop'."
    exit 1
  fi

  ensure_pg_volume

  if ! is_pg_volume_empty; then
    echo "ERROR: Docker volume $PG_VOLUME_NAME is not empty."
    echo "To replace existing volume data, run './pgctl.sh reset' and retry migration."
    exit 1
  fi

  echo "Migrating data from $LEGACY_PG_DATA_DIR to Docker volume $PG_VOLUME_NAME..."
  docker run --rm \
    -v "$PWD/$LEGACY_PG_DATA_DIR":/from:ro \
    -v "$PG_VOLUME_NAME":/to \
    postgres:"$PG_VERSION" sh -c 'cp -a /from/. /to/'

  docker run --rm -v "$PG_VOLUME_NAME":/to postgres:"$PG_VERSION" sh -c 'chown -R 999:999 /to && chmod 700 /to'

  echo "Migration completed. Start PostgreSQL with './pgctl.sh start'."
}

setup_migrations() {
  echo "Setting up dbmate migrations tool..."
  sudo curl -fsSL -o /usr/local/bin/dbmate https://github.com/amacneil/dbmate/releases/latest/download/dbmate-linux-amd64
  sudo chmod +x /usr/local/bin/dbmate
  /usr/local/bin/dbmate --help
}

print_conn_string() {
  # Use the DATABASE_URL from .env but replace the host with localhost for local connections
  local local_conn_string="${DATABASE_URL//livereview-db/127.0.0.1}"
  echo "DATABASE_URL=\"$local_conn_string\""
}

shell_pg() {
  if [ "${2:-}" = "-c" ] && [ -n "${3:-}" ]; then
    # Execute command with -c flag
    echo "Executing SQL command: $3"
    docker exec "$PG_CONTAINER_NAME" psql -U "$PG_USER" -d "$PG_DB" -c "$3"
  elif [ -n "${2:-}" ]; then
    # Execute command without -c flag (legacy support)
    echo "Executing SQL command..."
    docker exec "$PG_CONTAINER_NAME" psql -U "$PG_USER" -d "$PG_DB" -c "$2"
  else
    # Interactive shell
    echo "Connecting to PostgreSQL shell..."
    docker exec -it "$PG_CONTAINER_NAME" psql -U "$PG_USER" -d "$PG_DB"
  fi
}

# Main
cmd="${1:-}"
case "$cmd" in
  start) start_pg ;;
  stop) stop_pg ;;
  status) status_pg ;;
  logs) logs_pg ;;
  info) info_pg ;;
  rm) rm_pg ;;
  reset) reset_pg ;;
  migrations) setup_migrations ;;
  conn) print_conn_string ;;
  shell) shell_pg "$@" ;;
  migrate-legacy-data) migrate_legacy_data ;;
  *) usage ;;
esac
