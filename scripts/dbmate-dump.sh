#!/bin/bash
# Regenerates db/schema.sql via `dbmate dump`.
#
# dbmate shells out to whatever `pg_dump` it finds on PATH, and refuses to
# run if that binary's major version is older than the Postgres server's
# ("pg_dump: error: aborting because of server version mismatch"). This
# script checks that up front and fails with install instructions instead of
# letting dbmate's own confusing error surface, or silently working around it
# via Docker - a mismatch here means the host's postgresql-client package is
# genuinely stale and should be fixed once, not routed around on every dump.
set -euo pipefail
cd "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [ -f .env ]; then
    export $(grep -v '^#' .env | xargs)
fi

if [ -z "${DATABASE_URL:-}" ]; then
    echo "ERROR: DATABASE_URL not set (expected in .env)." >&2
    exit 1
fi

if ! command -v pg_dump >/dev/null 2>&1; then
    echo "ERROR: pg_dump not found. Install postgresql-client - see scripts/install-pgdg-client.sh." >&2
    exit 1
fi

if ! command -v psql >/dev/null 2>&1; then
    echo "ERROR: psql not found. Install postgresql-client - see scripts/install-pgdg-client.sh." >&2
    exit 1
fi

local_major="$(pg_dump --version | grep -oE '[0-9]+' | head -1)"
server_version_num="$(psql "$DATABASE_URL" -tAc "SHOW server_version_num" 2>/dev/null | tr -d '[:space:]')"

if [ -z "$server_version_num" ]; then
    echo "ERROR: could not connect to the dev database at \$DATABASE_URL to check its version." >&2
    exit 1
fi

# server_version_num is e.g. 180006 for 18.6 - the major version is the
# leading 2 digits for >=10, matching how pg_dump's --version already prints.
server_major="${server_version_num:0:2}"

if [ "$local_major" -lt "$server_major" ]; then
    cat >&2 <<EOF
ERROR: local pg_dump is version $local_major, but the dev Postgres server is version $server_major.
pg_dump refuses to dump a server newer than itself.

Fix: install a matching postgresql-client from the PGDG apt repo:
  PG_VERSION=$server_major ./scripts/install-pgdg-client.sh

Then make sure it's first on PATH (the installer prints how) and re-run this script.
EOF
    exit 1
fi

dbmate dump
