#!/bin/bash
# Installs a postgresql-client package from the official PGDG apt repo whose
# major version matches the LiveReview dev Postgres server (currently 18 -
# see scripts/pgctl.sh's PG_VERSION), so the host's own `pg_dump`/`psql`
# never trails the server the way Ubuntu's own packaged version does.
#
# Needs sudo. Safe to re-run.
set -euo pipefail

PG_VERSION="${PG_VERSION:-18}"

if ! command -v curl >/dev/null 2>&1; then
    echo "ERROR: curl is required." >&2
    exit 1
fi

echo "Installing postgresql-common (provides the PGDG repo setup helper)..."
sudo apt-get update
sudo apt-get install -y postgresql-common

echo "Adding the PGDG apt repository for this OS release..."
sudo /usr/share/postgresql-common/pgdg/apt.postgresql.org.sh -y

echo "Installing postgresql-client-${PG_VERSION}..."
sudo apt-get install -y "postgresql-client-${PG_VERSION}"

echo
echo "Done. Verify with:"
echo "  /usr/lib/postgresql/${PG_VERSION}/bin/pg_dump --version"
echo
echo "If that's not first on PATH yet, either:"
echo "  - add it: export PATH=\"/usr/lib/postgresql/${PG_VERSION}/bin:\$PATH\""
echo "  - or let the Debian alternatives system manage it: sudo apt-get install -y postgresql-client (meta-package tracks the newest installed major)"
