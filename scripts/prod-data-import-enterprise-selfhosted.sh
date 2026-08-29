#!/usr/bin/env bash
# ============================================================================
# Script: prod-data-import-enterprise-selfhosted.sh
# ============================================================================
#
# DESCRIPTION:
#   Restores a prod DB dump (from `make prod-data-export`) into the local
#   enterprise-selfhosted Postgres database. Same behavior as the original
#   prod-data-import.sh but targets the enterprise-selfhosted database
#   defined in .env.selfhosted.local.
#
# WHAT IT DOES:
#   1. Reads target DATABASE_URL from .env.selfhosted.local
#   2. Parses user/password/dbname from the URL
#   3. Finds the most recent dump in db/prod-exports/
#   4. Drops the existing enterprise-selfhosted database (with confirmation)
#   5. Creates fresh database and role (with confirmation)
#   6. Restores the dump via pg_restore
#   7. Grants privileges and fixes table ownership
#
# PREREQUISITES:
#   - .env.selfhosted.local must exist with DATABASE_URL pointing to
#     the enterprise-selfhosted database
#   - db/prod-exports/*.dump must exist (run `make prod-data-export` first)
#   - pg_restore must be available (install postgresql-client)
#
# USAGE:
#   bash scripts/prod-data-import-enterprise-selfhosted.sh
#   # or
#   make prod-data-import-enterprise-selfhosted
#
# SAFETY:
#   - This only touches the LOCAL enterprise-selfhosted database
#   - Never connects to or modifies the prod database
#   - Asks for confirmation before dropping existing database
#   - Asks for confirmation before creating new role
#
# ============================================================================
set -euo pipefail
cd "$(dirname "$0")/.."

ENV_FILE=".env.selfhosted.local"

if [ ! -f "$ENV_FILE" ]; then
  echo "ERROR: $ENV_FILE not found"
  exit 1
fi

set -a
. "./$ENV_FILE"
set +a

if [ -z "${DATABASE_URL:-}" ]; then
  echo "ERROR: DATABASE_URL not set in $ENV_FILE"
  exit 1
fi

# Parse user/password/dbname out of DATABASE_URL
# Strip the scheme (postgres:// or postgresql://) and parse the remaining components
_clean_url="${DATABASE_URL#postgres://}"
_clean_url="${_clean_url#postgresql://}"

# Extract userinfo (everything before @) and host+path (everything after @)
_userinfo="${_clean_url%%@*}"
_rest="${_clean_url#*@}"

DB_USER="${_userinfo%%:*}"
DB_PASS="${_userinfo#*:}"
DB_NAME="${_rest#*/}"
DB_NAME="${DB_NAME%%\?*}"

if [ -z "$DB_USER" ] || [ -z "$DB_NAME" ]; then
  echo "ERROR: could not parse a username and database name out of DATABASE_URL in $ENV_FILE"
  exit 1
fi

DUMP_FILE="${DUMP_FILE:-$(ls -t db/prod-exports/*.dump 2>/dev/null | head -1)}"
if [ -z "$DUMP_FILE" ] || [ ! -f "$DUMP_FILE" ]; then
  echo "ERROR: no dump file found in db/prod-exports/ - run 'make prod-data-export' first,"
  echo "       or set DUMP_FILE=path/to/file.dump"
  exit 1
fi

# Resolve the absolute path for the dump file so Docker volume mounts work
DUMP_FILE="$(cd "$(dirname "$DUMP_FILE")" && pwd)/$(basename "$DUMP_FILE")"

# --- Detect Postgres backend: Docker container or native install ---
DOCKER_PG_CONTAINER="${DOCKER_PG_CONTAINER:-livereview_pg}"
SUPERUSER=""
USE_DOCKER=false

if docker ps --format '{{.Names}}' 2>/dev/null | grep -qx "$DOCKER_PG_CONTAINER"; then
  USE_DOCKER=true
  SUPERUSER="$(docker inspect "$DOCKER_PG_CONTAINER" --format '{{range .Config.Env}}{{println .}}{{end}}' | sed -n 's/^POSTGRES_USER=//p' | head -1)"
  if [ -z "$SUPERUSER" ]; then
    SUPERUSER="postgres"
  fi
  echo "Detected Docker container '$DOCKER_PG_CONTAINER' (superuser: $SUPERUSER)"
else
  if ! id postgres &>/dev/null; then
    echo "ERROR: no Docker container '$DOCKER_PG_CONTAINER' running and no local 'postgres' user found."
    exit 1
  fi
  SUPERUSER="postgres"
  echo "Detected native Postgres install (superuser: $SUPERUSER)"
fi

# Helpers that abstract Docker vs native
psql_exec() {
  if $USE_DOCKER; then
    docker exec "$DOCKER_PG_CONTAINER" psql -U "$SUPERUSER" "$@"
  else
    sudo -u postgres psql "$@"
  fi
}

pg_restore_exec() {
  if $USE_DOCKER; then
    PGPASSWORD="$DB_PASS" pg_restore -h localhost -U "$DB_USER" --no-owner --no-privileges --dbname="$DB_NAME" --verbose "$DUMP_FILE" || true
  else
    sudo -u postgres pg_restore --no-owner --no-privileges --dbname="$DB_NAME" --verbose < "$DUMP_FILE" || true
  fi
}

echo "This will act on your LOCAL Postgres server only (never prod):"
echo "  local database : $DB_NAME"
echo "  local user     : $DB_USER"
echo "  dump file      : $DUMP_FILE"
echo

DB_EXISTS="$(psql_exec -d template1 -tAc "SELECT 1 FROM pg_database WHERE datname = '$DB_NAME'")"

if [ "$DB_EXISTS" = "1" ]; then
  read -r -p "Local database \"$DB_NAME\" already exists. Drop it? [y/N] " CONFIRM_DROP
  if [[ "$CONFIRM_DROP" =~ ^[Yy]$ ]]; then
    echo "Terminating other sessions connected to \"$DB_NAME\" ..."
    psql_exec -d template1 -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '$DB_NAME' AND pid <> pg_backend_pid();"
    echo "Dropping local database \"$DB_NAME\" ..."
    psql_exec -d template1 -c "DROP DATABASE \"$DB_NAME\";"
  else
    echo "Not dropping the existing local database. Stopping here - nothing else was changed."
    exit 1
  fi
fi

echo "Creating local database \"$DB_NAME\" ..."
psql_exec -d template1 -c "CREATE DATABASE \"$DB_NAME\";"

ROLE_EXISTS="$(psql_exec -d template1 -tAc "SELECT 1 FROM pg_roles WHERE rolname = '$DB_USER'")"

if [ "$ROLE_EXISTS" = "1" ]; then
  echo "Local role \"$DB_USER\" already exists - leaving it as-is."
else
  echo
  echo "Local role \"$DB_USER\" does not exist yet. About to create it with:"
  echo "  username: $DB_USER"
  echo "  password: $DB_PASS"
  read -r -p "Create this local role? [y/N] " CONFIRM_USER
  if [[ "$CONFIRM_USER" =~ ^[Yy]$ ]]; then
    psql_exec -d template1 -c "CREATE ROLE \"$DB_USER\" WITH LOGIN PASSWORD '$DB_PASS';"
  else
    echo "Role not created - the app can't connect without it, stopping here."
    exit 1
  fi
fi

echo "Restoring $DUMP_FILE into local database \"$DB_NAME\" ..."
pg_restore_exec

echo "Granting $DB_USER full access to local database \"$DB_NAME\" ..."
psql_exec -d "$DB_NAME" -c "GRANT ALL PRIVILEGES ON DATABASE \"$DB_NAME\" TO \"$DB_USER\";"
psql_exec -d "$DB_NAME" -c "GRANT ALL ON SCHEMA public TO \"$DB_USER\";"

# Fix table ownership (same approach as original prod-data-import.sh)
DB_USER_ESCAPED=${DB_USER//\'/\'\'}
SQL_BODY=$(cat <<'EOSQL'
DO $do$
DECLARE
  r RECORD;
BEGIN
  FOR r IN SELECT tablename FROM pg_tables WHERE schemaname = 'public' LOOP
    EXECUTE format('ALTER TABLE public.%I OWNER TO %I', r.tablename, '__DB_USER__');
  END LOOP;
  FOR r IN SELECT sequencename FROM pg_sequences WHERE schemaname = 'public' LOOP
    EXECUTE format('ALTER SEQUENCE public.%I OWNER TO %I', r.sequencename, '__DB_USER__');
  END LOOP;
  FOR r IN SELECT viewname FROM pg_views WHERE schemaname = 'public' LOOP
    EXECUTE format('ALTER VIEW public.%I OWNER TO %I', r.viewname, '__DB_USER__');
  END LOOP;
END $do$;
EOSQL
)
SQL_BODY=${SQL_BODY//__DB_USER__/$DB_USER_ESCAPED}
psql_exec -d "$DB_NAME" -c "$SQL_BODY"

echo
echo "Done. Local \"$DB_NAME\" is restored and owned by \"$DB_USER\"."
echo "Now run: make prod-data-transform-selfhosted"
