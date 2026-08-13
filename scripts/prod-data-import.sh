#!/usr/bin/env bash
# Restores a prod DB dump (from `make prod-data-export`) into the LOCAL
# Postgres server only. Never touches prod - it only reads db/prod-exports/
# and connects via `sudo -u postgres psql`/`pg_restore`, i.e. the local
# Unix socket. The database/user it creates or drops come from THIS repo's
# local .env DATABASE_URL, not .env.prod.
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

echo "This will act on your LOCAL Postgres server only (never prod):"
echo "  local database : $DB_NAME"
echo "  local user      : $DB_USER"
echo "  dump file       : $DUMP_FILE"
echo

# sudo will prompt for your password interactively here if needed - this
# script never handles or stores that password itself.
DB_EXISTS="$(sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname = '$DB_NAME'")"

if [ "$DB_EXISTS" = "1" ]; then
  read -r -p "Local database \"$DB_NAME\" already exists. Drop it? [y/N] " CONFIRM_DROP
  if [[ "$CONFIRM_DROP" =~ ^[Yy]$ ]]; then
    echo "Terminating other sessions connected to \"$DB_NAME\" (e.g. a running dev server) ..."
    sudo -u postgres psql -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '$DB_NAME' AND pid <> pg_backend_pid();"
    echo "Dropping local database \"$DB_NAME\" ..."
    sudo -u postgres psql -c "DROP DATABASE \"$DB_NAME\";"
  else
    echo "Not dropping the existing local database. Stopping here - nothing else was changed."
    exit 1
  fi
fi

echo "Creating local database \"$DB_NAME\" ..."
sudo -u postgres psql -c "CREATE DATABASE \"$DB_NAME\";"

ROLE_EXISTS="$(sudo -u postgres psql -tAc "SELECT 1 FROM pg_roles WHERE rolname = '$DB_USER'")"

if [ "$ROLE_EXISTS" = "1" ]; then
  echo "Local role \"$DB_USER\" already exists - leaving it as-is, no changes made to it."
else
  echo
  echo "Local role \"$DB_USER\" does not exist yet. About to create it with:"
  echo "  username: $DB_USER"
  echo "  password: $DB_PASS"
  read -r -p "Create this local role? [y/N] " CONFIRM_USER
  if [[ "$CONFIRM_USER" =~ ^[Yy]$ ]]; then
    sudo -u postgres psql -c "CREATE ROLE \"$DB_USER\" WITH LOGIN PASSWORD '$DB_PASS';"
  else
    echo "Role not created - the app can't connect without it, stopping here."
    exit 1
  fi
fi

echo "Restoring $DUMP_FILE into local database \"$DB_NAME\" ..."
sudo -u postgres pg_restore --no-owner --no-privileges --dbname="$DB_NAME" --verbose "$DUMP_FILE"

echo "Granting $DB_USER full access to local database \"$DB_NAME\" ..."
sudo -u postgres psql -d "$DB_NAME" -c "GRANT ALL PRIVILEGES ON DATABASE \"$DB_NAME\" TO \"$DB_USER\";"
sudo -u postgres psql -d "$DB_NAME" -c "GRANT ALL ON SCHEMA public TO \"$DB_USER\";"
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
sudo -u postgres psql -d "$DB_NAME" -c "$SQL_BODY"

echo "✅ Done. Local \"$DB_NAME\" is restored and owned by \"$DB_USER\"."
