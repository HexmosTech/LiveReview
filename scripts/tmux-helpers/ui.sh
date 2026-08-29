#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/common.sh"
cd "$ROOT_DIR"
echo ">>> Waiting for API on :8888 ..."
wait_port 8888
echo ">>> API is up. Starting UI (npm start) ..."
cd "$ROOT_DIR/ui"
exec npm start
