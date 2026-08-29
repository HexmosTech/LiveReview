#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/common.sh"
cd "$ROOT_DIR"
echo ">>> Waiting for API on :8888 ..."
wait_port 8888
echo ">>> API is up. Building worker ..."
go build -o ./tmp/lrworker .
echo ">>> Starting worker ..."
exec ./tmp/lrworker worker --env-file .env
