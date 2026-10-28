#!/usr/bin/env bash
# Purge LiveReview users and their personal orgs from a database (meant for local test data).
#
# For each email: the user, and the org named after that email (the personal org sign-up creates), are deleted
# together with everything that depends on them. Rows that only optionally point at them (for example a review in
# another org created by the user) are kept with that reference set to NULL. Other orgs the user belonged to are
# kept; the user is just removed from them. Audit events for the purged orgs and users are deleted too.
#
# Usage:
#   scripts/purge_users.sh [--dry-run] [--yes] [--allow-remote] [--env-file FILE] email [email...]
#
#   --dry-run       run everything inside a transaction and roll it back (shows what would be deleted)
#   --yes           don't ask for confirmation
#   --allow-remote  allow a DATABASE_URL whose host is not localhost
#   --env-file      read DATABASE_URL from this file (default: .env next to this script's repo root)
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
env_file="$repo_root/.env"
dry_run=0 assume_yes=0 allow_remote=0
emails=()

while [ $# -gt 0 ]; do
	case "$1" in
	--dry-run) dry_run=1 ;;
	--yes) assume_yes=1 ;;
	--allow-remote) allow_remote=1 ;;
	--env-file)
		env_file="$2"
		shift
		;;
	-h | --help)
		sed -n '2,17p' "$0"
		exit 0
		;;
	-*)
		echo "unknown option: $1" >&2
		exit 2
		;;
	*) emails+=("$(echo "$1" | tr '[:upper:]' '[:lower:]')") ;;
	esac
	shift
done

if [ ${#emails[@]} -eq 0 ]; then
	echo "usage: $0 [--dry-run] [--yes] [--allow-remote] [--env-file FILE] email [email...]" >&2
	exit 2
fi
for e in "${emails[@]}"; do
	if ! [[ "$e" =~ ^[a-z0-9._%+-]+@[a-z0-9.-]+$ ]]; then
		echo "not an email address: $e" >&2
		exit 2
	fi
done

if ! command -v psql >/dev/null; then
	echo "psql not found" >&2
	exit 1
fi
db_url="$(grep -E '^DATABASE_URL=' "$env_file" | tail -1 | cut -d= -f2- | sed -E "s/^['\"]//; s/['\"]$//")"
if [ -z "$db_url" ]; then
	echo "DATABASE_URL not found in $env_file" >&2
	exit 1
fi
db_host="$(echo "$db_url" | sed -E 's#^[a-z]+://([^@/]*@)?(\[[^]]*\]|[^:/?]*).*#\2#')"
case "$db_host" in
localhost | 127.0.0.1 | "[::1]" | ::1) ;;
*)
	if [ "$allow_remote" -ne 1 ]; then
		echo "refusing: DATABASE_URL host is '$db_host', not localhost (pass --allow-remote if you really mean it)" >&2
		exit 1
	fi
	;;
esac

email_list="$(printf '%s,' "${emails[@]}")"
email_list="${email_list%,}"

echo "Database: $db_host ($(echo "$db_url" | sed -E 's#.*/([^/?]+)(\?.*)?$#\1#'))"
psql "$db_url" -X -q -v ON_ERROR_STOP=1 -v emails="$email_list" <<'SQL'
\pset footer off
\echo 'Users:'
SELECT id, email FROM users WHERE lower(email) = ANY (string_to_array(:'emails', ','));
\echo 'Orgs named after these emails (deleted with all their data):'
SELECT o.id, o.name,
       (SELECT string_agg(u.email, ', ') FROM user_roles ur JOIN users u ON u.id = ur.user_id WHERE ur.org_id = o.id) AS members
FROM orgs o WHERE lower(o.name) = ANY (string_to_array(:'emails', ','));
SQL

if [ "$dry_run" -ne 1 ] && [ "$assume_yes" -ne 1 ]; then
	read -r -p "Permanently delete the above? [y/N] " answer
	case "$answer" in
	y | Y | yes) ;;
	*)
		echo "cancelled"
		exit 1
		;;
	esac
fi

finish="COMMIT;"
if [ "$dry_run" -eq 1 ]; then
	finish="ROLLBACK;"
	echo "(dry run: nothing will be kept)"
fi

{
	cat <<'SQL'
\set ON_ERROR_STOP 1
BEGIN;
SET LOCAL app.audit_purge = 'on';

CREATE TEMP TABLE purge_users ON COMMIT DROP AS
	SELECT id FROM users WHERE lower(email) = ANY (string_to_array(:'emails', ','));
CREATE TEMP TABLE purge_orgs ON COMMIT DROP AS
	SELECT id FROM orgs WHERE lower(name) = ANY (string_to_array(:'emails', ','));

-- purge(tbl, cond): delete the rows of tbl matching cond, after dealing with everything that references them:
-- nullable references are set to NULL, required ones are purged recursively.
CREATE FUNCTION pg_temp.purge(tbl regclass, cond text, depth int DEFAULT 0) RETURNS void AS $$
DECLARE
	fk record;
	n bigint;
BEGIN
	IF depth > 12 THEN
		RAISE EXCEPTION 'foreign key chain too deep at %', tbl;
	END IF;
	FOR fk IN
		SELECT c.conrelid::regclass AS child, a.attname AS ccol, pa.attname AS pcol,
		       NOT a.attnotnull AS nullable, array_length(c.conkey, 1) AS ncols
		FROM pg_constraint c
		JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = c.conkey[1]
		JOIN pg_attribute pa ON pa.attrelid = c.confrelid AND pa.attnum = c.confkey[1]
		WHERE c.contype = 'f' AND c.confrelid = tbl
		  AND c.conrelid NOT IN (SELECT inhrelid FROM pg_inherits) -- partitions are reached through their parent
	LOOP
		IF fk.ncols <> 1 THEN
			RAISE EXCEPTION 'multi-column foreign key on % is not handled', fk.child;
		END IF;
		IF fk.nullable THEN
			EXECUTE format('UPDATE %s SET %I = NULL WHERE %I IN (SELECT %I FROM %s WHERE %s)',
			               fk.child, fk.ccol, fk.ccol, fk.pcol, tbl, cond);
			GET DIAGNOSTICS n = ROW_COUNT;
			IF n > 0 THEN
				RAISE NOTICE '%null %.% (%)', repeat('  ', depth), fk.child, fk.ccol, n;
			END IF;
		ELSE
			PERFORM pg_temp.purge(fk.child,
				format('%I IN (SELECT %I FROM %s WHERE %s)', fk.ccol, fk.pcol, tbl, cond), depth + 1);
		END IF;
	END LOOP;
	EXECUTE format('DELETE FROM %s WHERE %s', tbl, cond);
	GET DIAGNOSTICS n = ROW_COUNT;
	IF n > 0 THEN
		RAISE NOTICE '%delete % (%)', repeat('  ', depth), tbl, n;
	END IF;
END $$ LANGUAGE plpgsql;

-- Webhook registry rows hang off a connector through a nullable key (and older rows carry a wrong org_id), so
-- delete them with their connectors instead of leaving them orphaned.
DELETE FROM webhook_registry
WHERE integration_token_id IN (SELECT id FROM integration_tokens WHERE org_id IN (SELECT id FROM purge_orgs));

-- Rows tagged with a purged org but not linked by a foreign key (audit events, caches, ...).
DO $$
DECLARE
	t record;
BEGIN
	FOR t IN
		SELECT c.oid::regclass AS tbl
		FROM pg_class c
		JOIN pg_namespace ns ON ns.oid = c.relnamespace AND ns.nspname = 'public'
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attname = 'org_id' AND NOT a.attisdropped
		WHERE c.relkind IN ('r', 'p') AND c.oid NOT IN (SELECT inhrelid FROM pg_inherits)
		ORDER BY 1
	LOOP
		PERFORM pg_temp.purge(t.tbl, 'org_id IN (SELECT id FROM purge_orgs)');
	END LOOP;
END $$;
DELETE FROM audit_events WHERE actor_user_id IN (SELECT id FROM purge_users);

SELECT pg_temp.purge('orgs', 'id IN (SELECT id FROM purge_orgs)');
SELECT pg_temp.purge('users', 'id IN (SELECT id FROM purge_users)');

\pset footer off
SELECT 'users left' AS check, count(*) FROM users WHERE lower(email) = ANY (string_to_array(:'emails', ','))
UNION ALL
SELECT 'orgs left', count(*) FROM orgs WHERE lower(name) = ANY (string_to_array(:'emails', ','));
SQL
	echo "$finish"
} | psql "$db_url" -X -q -v emails="$email_list" 2>&1 | grep -vE '^ *purge *$|^-+$|^ *$|^\(1 row\)$'

if [ "$dry_run" -eq 1 ]; then
	echo "Dry run finished; rolled back."
else
	echo "Done."
fi
