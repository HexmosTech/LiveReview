#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/common.sh"
NICEURL="${NICEURL:-niceurl2}"
cd "$ROOT_DIR"
echo ">>> Waiting for UI on :8081 ..."
wait_port 8081
echo ">>> UI is up. Starting $NICEURL ..."
exec make "$NICEURL"
