#!/bin/bash

set -euo pipefail

# Load environment variables from .env file
if [ -f .env ]; then
    export $(grep -v '^#' .env | xargs)
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
PG_DATA_DIR="./livereview_pgdata"

# Ensure data dir exists
mkdir -p "$PG_DATA_DIR"

usage() {
  echo "Usage: $0 {start|stop|status|logs|info|rm|migrations|conn}"
  exit 1
}

start_pg() {
  if docker ps -a --format '{{.Names}}' | grep -qw "$PG_CONTAINER_NAME"; then
    echo "Container already exists. Starting..."
    docker start "$PG_CONTAINER_NAME"
  else
    echo "Creating and starting new PostgreSQL container for livereview..."
    docker run -d \
      --name "$PG_CONTAINER_NAME" \
      -e POSTGRES_USER="$PG_USER" \
      -e POSTGRES_PASSWORD="$PG_PASSWORD" \
      -e POSTGRES_DB="$PG_DB" \
      -v "$PWD/$PG_DATA_DIR":/var/lib/postgresql/data \
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
  echo "  Data Dir: $PG_DATA_DIR"
  echo ""
}

rm_pg() {
  echo "Stopping and removing container + volume (but NOT your local data dir)..."
  docker rm -f "$PG_CONTAINER_NAME" || true
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

# Main
cmd="${1:-}"
case "$cmd" in
  start) start_pg ;;
  stop) stop_pg ;;
  status) status_pg ;;
  logs) logs_pg ;;
  info) info_pg ;;
  rm) rm_pg ;;
  migrations) setup_migrations ;;
  conn) print_conn_string ;;
  *) usage ;;
esac
