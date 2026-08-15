#!/usr/bin/env bash
# Restores a prod DB dump (from `make prod-data-export`) into the LOCAL
# Postgres server only. Never touches prod - it only reads db/prod-exports/.
# Detects whether Postgres is running in Docker (container "livereview_pg")
# or natively, and shells into the right place as the superuser.
# The database/user it creates or drops come from THIS repo's local .env
# DATABASE_URL, not .env.prod.
set -euo pipefail
cd "$(dirname "$0")/.."

if [ ! -f .env ]; then
  echo "ERROR: .env not found - this script reads the LOCAL target from .env's DATABASE_URL"
  exit 1
fi

set -a
. ./.env
set +a

if [ -z "${DATABASE_URL:-}" ]; then
  echo "ERROR: DATABASE_URL not set in .env"
  exit 1
fi

# Parse user/password/dbname out of DATABASE_URL - this never dials out,
# it just reads the string.
IFS=$'\t' read -r DB_USER DB_PASS DB_NAME <<<"$(python3 -c "
from urllib.parse import urlparse
import sys
u = urlparse('$DATABASE_URL'.replace('postgres://', 'postgresql://', 1))
print(f'{u.username or \"\"}\t{u.password or \"\"}\t{(u.path or \"/\").lstrip(\"/\")}')
")"

if [ -z "$DB_USER" ] || [ -z "$DB_NAME" ]; then
  echo "ERROR: could not parse a username and database name out of .env's DATABASE_URL"
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
  # The Docker container's POSTGRES_USER is the superuser
  SUPERUSER="$(docker inspect "$DOCKER_PG_CONTAINER" --format '{{range .Config.Env}}{{println .}}{{end}}' | sed -n 's/^POSTGRES_USER=//p' | head -1)"
  if [ -z "$SUPERUSER" ]; then
    SUPERUSER="postgres"
  fi
  echo "Detected Docker container '$DOCKER_PG_CONTAINER' (superuser: $SUPERUSER)"
else
  # Fall back to native install — need sudo -u postgres
  if ! id postgres &>/dev/null; then
    echo "ERROR: no Docker container '$DOCKER_PG_CONTAINER' running and no local 'postgres' user found."
    echo "       Either start the Docker container or install postgresql locally."
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
    # Use the host's pg_restore (which matches the pg_dump version) connecting
    # to the container's exposed port. The container's bundled pg_restore may
    # be an older version that doesn't support the dump format.
    PGPASSWORD="$DB_PASS" pg_restore -h localhost -U "$DB_USER" --no-owner --no-privileges --dbname="$DB_NAME" --verbose "$DUMP_FILE"
  else
    sudo -u postgres pg_restore --no-owner --no-privileges --dbname="$DB_NAME" --verbose "$DUMP_FILE"
  fi
}

echo "This will act on your LOCAL Postgres server only (never prod):"
echo "  local database : $DB_NAME"
echo "  local user      : $DB_USER"
echo "  dump file       : $DUMP_FILE"
echo

DB_EXISTS="$(psql_exec -d template1 -tAc "SELECT 1 FROM pg_database WHERE datname = '$DB_NAME'")"

if [ "$DB_EXISTS" = "1" ]; then
  read -r -p "Local database \"$DB_NAME\" already exists. Drop it? [y/N] " CONFIRM_DROP
  if [[ "$CONFIRM_DROP" =~ ^[Yy]$ ]]; then
    echo "Terminating other sessions connected to \"$DB_NAME\" (e.g. a running dev server) ..."
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
  echo "Local role \"$DB_USER\" already exists - leaving it as-is, no changes made to it."
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
# Ownership, not just grants: pg_restore ran as the postgres superuser
# (--no-owner meant the dump carried no ownership statements to replay), so
# every restored table/sequence is currently owned by postgres. Reassigning
# ownership to the app role is what "grant all permission" actually needs
# to mean here - a GRANT alone would leave ownership-only operations
# (ALTER TABLE, DROP, sequence ownership - none of these are grantable
# privileges in Postgres, only the owner or a superuser can do them)
# unusable by the app user, which breaks running migrations locally.
#
# This deliberately reassigns just the tables/sequences/views pg_restore
# created in "public", not "REASSIGN OWNED BY postgres" wholesale - the
# postgres role also owns a handful of Postgres-internal pinned objects
# (e.g. the plpgsql language) that can never be reassigned in any database,
# and a blanket REASSIGN OWNED BY hits those and fails outright.
# psql's `:'var'` client-side substitution does NOT reach inside a $$...$$
# dollar-quoted DO block - psql treats that whole span as an opaque token,
# so the substitution silently never happens and the literal text ":'var'"
# gets sent to the server, which is invalid SQL. Substituting DB_USER with
# plain bash string replacement first, before any of this reaches psql,
# sidesteps that entirely. The heredoc delimiter is quoted ('EOSQL') so bash
# does no interpolation of its own while building the template (all the
# $do$/$$ dollar-quoting passes through untouched).
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

echo "✅ Done. Local \"$DB_NAME\" is restored and owned by \"$DB_USER\"."
