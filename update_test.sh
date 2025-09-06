#!/usr/bin/env bash
# update_test.sh - Automated end-to-end smoke test for LiveReview update flow
# Purpose:
#   1. Remove any existing LiveReview installation & containers (clean slate)
#   2. Install an older version (default 0.0.6)
#   3. Run lrops.sh update (to a target newer version, default 0.0.7)
#   4. Verify containers now run the new version image
#   5. Provide manual spot-check instructions for the UI/frontend
#
# Usage:
#   ./update_test.sh                 # uses defaults: from 0.0.6 -> 0.0.7
#   ./update_test.sh 0.0.5 0.0.7      # custom from/to
#   OLD_VERSION=0.0.4 NEW_VERSION=0.0.7 ./update_test.sh
#
# Requirements:
#   - docker + compose
#   - curl, jq
#   - lrops.sh present in current directory (or will be downloaded)
#
# Safety: This script is DESTRUCTIVE for the target install dir ($HOME/livereview by default).
#         Set INSTALL_DIR to override. Set SKIP_CLEAN=1 to skip wipe (not recommended for pure test).

set -euo pipefail

OLD_VERSION="${1:-${OLD_VERSION:-0.0.7}}"
NEW_VERSION="${2:-${NEW_VERSION:-0.0.8}}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/livereview}"
LROPS="${LROPS_PATH:-./lrops.sh}"
FORCE="${FORCE:-1}"
SKIP_CLEAN="${SKIP_CLEAN:-0}"
WAIT_HEALTH_TIMEOUT="${WAIT_HEALTH_TIMEOUT:-120}"

color() { local c="$1"; shift; printf "\033[%sm%s\033[0m\n" "$c" "$*"; }
info()  { color '34' "[INFO] $*"; }
ok()    { color '32' "[ OK ] $*"; }
warn()  { color '33' "[WARN] $*"; }
err()   { color '31' "[ERR ] $*"; }
die()   { err "$*"; exit 1; }

need_cmd() { command -v "$1" >/dev/null 2>&1 || die "Missing required command: $1"; }

need_cmd docker
need_cmd curl
need_cmd jq

if [[ ! -f "$LROPS" ]]; then
	info "lrops.sh not found locally – downloading from main branch"
	curl -fsSL "https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh" -o lrops.sh || die "Failed to download lrops.sh"
	chmod +x lrops.sh
fi

info "Test Plan: $OLD_VERSION -> $NEW_VERSION (install dir: $INSTALL_DIR)"
[[ "$FORCE" == "1" ]] || warn "FORCE is not 1; clean phase may abort if existing install present"

clean_environment() {
	if [[ "$SKIP_CLEAN" == "1" ]]; then
		warn "Skipping clean phase (SKIP_CLEAN=1)"
		return 0
	fi
	info "[1/5] Cleaning existing installation & containers"

	if docker ps --format '{{.Names}}' | grep -q 'livereview'; then
		info "Stopping & removing containers"
		docker ps -a --filter name=livereview --format '{{.ID}}' | xargs -r docker rm -f || true
	fi

	if [[ -d "$INSTALL_DIR" ]]; then
		info "Removing install dir: $INSTALL_DIR"
		sudo rm -rf "$INSTALL_DIR"
	fi

	ok "Environment clean"
}

install_old_version() {
	info "[2/5] Installing old version $OLD_VERSION"
	# Use --force to ensure clean install and demo mode for simplicity
	bash "$LROPS" --force --express --version "$OLD_VERSION" setup-demo >/tmp/update_test_install.log 2>&1 || {
		err "Installation failed. Log snippet:"; tail -n 50 /tmp/update_test_install.log; return 1; }

	ok "Install command completed"
}

wait_for_health() {
	local port_backend="${LIVEREVIEW_BACKEND_PORT:-8888}"
	local start_ts=$(date +%s)
	info "Waiting for backend health on port $port_backend (timeout ${WAIT_HEALTH_TIMEOUT}s)"
	while true; do
		if curl -fs "http://localhost:${port_backend}/health" >/dev/null 2>&1; then
			ok "Backend healthy"
			return 0
		fi
		local now=$(date +%s)
		if (( now - start_ts > WAIT_HEALTH_TIMEOUT )); then
			err "Health check timeout"
			return 1
		fi
		sleep 3
	done
}

extract_running_version() {
	# Return image tag of livereview-app container (e.g., ghcr.io/hexmostech/livereview:0.0.6)
	docker ps --filter name=livereview-app --format '{{.Image}}' | awk -F: 'NF>1{print $NF}' | head -1
}

verify_version() {
	local expected="$1"
	local running
	running=$(extract_running_version || true)
	if [[ "$running" == "$expected" ]]; then
		ok "Detected expected version: $running"
		return 0
	else
		err "Version mismatch. Expected $expected, got ${running:-<none>}"
		return 1
	fi
}

perform_update() {
	info "[3/5] Triggering update to $NEW_VERSION"
	bash "$LROPS" update "$NEW_VERSION" >/tmp/update_test_update.log 2>&1 || {
		err "Update failed. Log snippet:"; tail -n 80 /tmp/update_test_update.log; return 1; }
	ok "Update command completed"
}

# Pause to allow manual DB / filesystem changes before proceeding with update.
# Controlled by ENV: AUTO_CONTINUE=1 to skip; or PASS --no-pause via arg (future expansion).
pause_before_update() {
	if [[ "${AUTO_CONTINUE:-0}" == "1" ]]; then
		info "AUTO_CONTINUE=1 set – skipping manual pause before update"
		return 0
	fi
	echo
	info "=== MANUAL PAUSE BEFORE UPDATE ==="
	cat <<'EOT'
You can now make manual changes (schema/data) against the OLD version before the update.

Common helper commands:
  # Open psql shell
  docker exec -it livereview-db psql -U livereview -d livereview

  # List tables / check migrations
  docker exec -it livereview-db psql -U livereview -d livereview -c "\\dt"
  docker exec -it livereview-db psql -U livereview -d livereview -c "SELECT version FROM schema_migrations ORDER BY version;"

  # Example: add dummy row / create temp table (for experimentation)
  docker exec -it livereview-db psql -U livereview -d livereview -c "CREATE TABLE IF NOT EXISTS manual_test(id serial primary key, note text, created_at timestamptz default now());"

When finished with your manual edits, type 'y' (or press Enter) to continue with the update,
or 'q' to abort the script.
EOT
	local ans=""
	while true; do
		read -r -p "Proceed with update? [Y/n/q]: " ans || ans="q"
		case "${ans,,}" in
			""|y|yes) info "Continuing to update step..."; break ;;
			n|no) warn "You chose 'no' – waiting. Type 'y' when ready." ;;
			q|quit) die "Aborted by user before update." ;;
			*) warn "Unrecognized input: $ans" ;;
		esac
	done
}

show_manual_spot_check() {
	cat <<EOF

[5/5] Manual Spot Check Instructions
------------------------------------
1. Open frontend: http://localhost:8081/ (or your configured frontend port)
2. Hit API health directly:   curl -fs http://localhost:8888/health | jq .
3. Initial setup: If no users exist, POST to /api/auth/setup-admin with JSON body:
			{"email":"admin@example.com","password":"ChangeMe123!","org_name":"TestOrg"}
4. Login via /api/auth/login; ensure response contains tokens.
5. Visit dashboard; confirm no legacy job_queue errors in app logs:
			docker logs livereview-app | grep -i job_queue   # should yield nothing
6. Confirm image tag now at $NEW_VERSION:
			docker ps --filter name=livereview-app --format '{{.Image}}'

If all checks pass, the update mechanism is functioning.
EOF
}

main() {
	clean_environment
	install_old_version
	wait_for_health || die "Old version did not become healthy"
	verify_version "$OLD_VERSION" || warn "Could not confirm old version (continuing)"
	pause_before_update
	perform_update
	wait_for_health || die "New version did not become healthy"
	info "[4/5] Verifying new version after update"
	verify_version "$NEW_VERSION" || die "Update version verification failed"
	ok "Update flow successful: $OLD_VERSION -> $NEW_VERSION"
	show_manual_spot_check
}

main "$@"

