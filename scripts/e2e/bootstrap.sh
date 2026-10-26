#!/usr/bin/env bash
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=common.sh
source "$DIR/common.sh"

cmd="${1:-status}"
shift || true

case "$cmd" in
  up)
    exec "$PYTHON_BIN" "$DIR/control.py" up "$@"
    ;;
  down)
    exec "$PYTHON_BIN" "$DIR/control.py" down "$@"
    ;;
  status)
    exec "$PYTHON_BIN" "$DIR/control.py" status "$@"
    ;;
  auth)
    exec "$PYTHON_BIN" "$DIR/control.py" auth "$@"
    ;;
  doctor)
    exec "$PYTHON_BIN" "$DIR/control.py" doctor "$@"
    ;;
  env)
    exec "$PYTHON_BIN" "$DIR/control.py" env "$@"
    ;;
  sync-ui)
    exec "$PYTHON_BIN" "$DIR/control.py" sync-ui "$@"
    ;;
  clean)
    exec "$PYTHON_BIN" "$DIR/control.py" clean "$@"
    ;;
  preflight)
    exec "$PYTHON_BIN" "$DIR/preflight.py" "$@"
    ;;
  review)
    exec "$PYTHON_BIN" "$DIR/review_test.py" "$@"
    ;;
  audit)
    exec "$PYTHON_BIN" "$DIR/audit_tests.py" "$@"
    ;;
  urls)
    exec "$PYTHON_BIN" "$DIR/urls.py" "$@"
    ;;
  *)
    cat >&2 <<EOF
Usage: scripts/e2e/bootstrap.sh <command>

Commands:
  up [--interactive] [--fresh]   Provision fixtures, keys, and the lrc sandbox
  down [--forget-auth]           Clean up and restore any temporary config
  status                         Show E2E status
  auth [--headless]              Cache a browser/password session
  preflight [--strict]           Probe API, UI, worker, AI, and git connector
  audit                          Run real-account audit assertions
  review [--strict]              Run a real lrc review
  urls [--strict]                Probe all manual-talent URLs
  doctor                         Check prerequisites and file permissions
  env                            Print the generated run.env path
  sync-ui                        Rewrite Playwright state for the current base URL
  clean                          Clean up without forgetting the cached session
EOF
    exit 2
    ;;
esac
