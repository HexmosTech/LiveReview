#!/bin/bash
# Regenerates db/schema.sql via `dbmate dump`.
#
# dbmate shells out to whatever `pg_dump` it finds on PATH, and refuses to run
# if that binary's version is older than the Postgres server's (pg_dump
# "aborting because of server version mismatch"). The local dev Postgres runs
# in Docker (see scripts/pgctl.sh, container "livereview_pg") on whatever
# major version PG_VERSION there is pinned to - the host's own
# postgresql-client package commonly lags behind that. Rather than requiring
# a matching postgresql-client install on every machine, this shims `pg_dump`
# for the duration of the dump to forward into the running container, whose
# pg_dump always matches its own server exactly.
set -euo pipefail
cd "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

PG_CONTAINER_NAME="${PG_CONTAINER_NAME:-livereview_pg}"

if ! docker ps --format '{{.Names}}' | grep -qx "$PG_CONTAINER_NAME"; then
    echo "ERROR: Postgres container '$PG_CONTAINER_NAME' is not running (see scripts/pgctl.sh)." >&2
    exit 1
fi

SHIM_DIR="$(mktemp -d)"
trap 'rm -rf "$SHIM_DIR"' EXIT

cat > "$SHIM_DIR/pg_dump" <<EOF
#!/bin/bash
exec docker exec -i $PG_CONTAINER_NAME pg_dump "\$@"
EOF
chmod +x "$SHIM_DIR/pg_dump"

PATH="$SHIM_DIR:$PATH" dbmate dump
