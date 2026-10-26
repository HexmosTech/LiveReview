#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/common.sh"
cd "$ROOT_DIR"
echo ">>> Waiting for API on :8888 and UI on :8081 ..."
wait_port 8888
wait_port 8081
echo ">>> Provisioning the isolated lrc/E2E sandbox ..."
if grep -Eq '^E2E_(OTP|PASSWORD)=.+' .env.e2e.local 2>/dev/null; then
  python3 scripts/e2e/control.py up --optional --interactive --headless || true
else
  python3 scripts/e2e/control.py up --optional || true
fi
echo ""
echo ">>> E2E window idle. Run 'make e2e-status' or 'make e2e' from another terminal."
exec sleep infinity
