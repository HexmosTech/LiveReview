#!/bin/bash

set -euo pipefail

# Config for the "livereview" app
PG_CONTAINER_NAME="livereview_pg"
PG_PORT=5432
PG_USER="livereview"
PG_PASSWORD="Qw7!vRu#9eLt3pZ"  # strong password
PG_DB="livereview"
PG_VERSION="15"
PG_DATA_DIR="./livereview_pgdata"

# Ensure data dir exists
mkdir -p "$PG_DATA_DIR"

usage() {
  echo "Usage: $0 {start|stop|status|logs|info|rm}"
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

# Main
cmd="${1:-}"
case "$cmd" in
  start) start_pg ;;
  stop) stop_pg ;;
  status) status_pg ;;
  logs) logs_pg ;;
  info) info_pg ;;
  rm) rm_pg ;;
  *) usage ;;
esac
