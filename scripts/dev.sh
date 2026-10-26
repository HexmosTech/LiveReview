#!/usr/bin/env bash
set -euo pipefail

SESSION="lr"
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
WIN_API="$ROOT_DIR/scripts/tmux-helpers/api.sh"
WIN_UI="$ROOT_DIR/scripts/tmux-helpers/ui.sh"
WIN_WORKER="$ROOT_DIR/scripts/tmux-helpers/worker.sh"
WIN_NICEURL="$ROOT_DIR/scripts/tmux-helpers/niceurl.sh"
WIN_E2E="$ROOT_DIR/scripts/tmux-helpers/e2e.sh"

usage() {
  cat <<'EOF'
Usage: scripts/dev.sh [COMMAND]  (or: make dev / make dev-up / make dev-down / ...)

Commands:
  up            Start all services plus the isolated E2E sandbox (default)
  down          Clean up E2E fixtures and kill the tmux session
  restart SVC   Restart a single service (api|ui|worker|niceurl)
  status        List tmux windows and their status
  attach        Attach to the running session

Environment:
  NICEURL       niceurl target (default: niceurl2)

Examples:
  scripts/dev.sh                     # start everything
  scripts/dev.sh up                  # same
  scripts/dev.sh restart api         # restart just the API (Air recompiles)
  scripts/dev.sh restart ui          # restart just the UI dev server
  NICEURL=niceurl3 scripts/dev.sh    # start with niceurl3 instead
EOF
}

ensure_helpers() {
  mkdir -p "$ROOT_DIR/scripts/tmux-helpers"

  # api.sh
  cat > "$WIN_API" <<'APIEOF'
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/common.sh"
cd "$ROOT_DIR"
echo ">>> Starting API (make run-fast) ..."
exec make run-fast
APIEOF

  # ui.sh
  cat > "$WIN_UI" <<'UIEOF'
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/common.sh"
cd "$ROOT_DIR"
echo ">>> Waiting for API on :8888 ..."
wait_port 8888
echo ">>> API is up. Starting UI (npm start) ..."
cd "$ROOT_DIR/ui"
exec npm start
UIEOF

  # worker.sh
  cat > "$WIN_WORKER" <<'WOEOF'
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
WOEOF

  # niceurl.sh
  cat > "$WIN_NICEURL" <<'NUEOF'
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/common.sh"
NICEURL="${NICEURL:-niceurl2}"
cd "$ROOT_DIR"
echo ">>> Waiting for UI on :8081 ..."
wait_port 8081
echo ">>> UI is up. Starting $NICEURL ..."
exec make "$NICEURL"
NUEOF

  # e2e.sh
  cat > "$WIN_E2E" <<'E2EEOF'
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
E2EEOF

  # common.sh (shared helpers)
  cat > "$ROOT_DIR/scripts/tmux-helpers/common.sh" <<'CMNEOF'
#!/usr/bin/env bash
# Shared helpers for tmux dev windows
ROOT_DIR="$(cd "$(dirname "$0")/../../" && pwd)"

wait_port() {
  local port="$1"
  while ! (echo >/dev/tcp/127.0.0.1/"$port") 2>/dev/null; do
    sleep 1
  done
}
CMNEOF

  chmod +x "$ROOT_DIR/scripts/tmux-helpers/"*.sh
}

cmd_up() {
  ensure_helpers

  if tmux has-session -t "$SESSION" 2>/dev/null; then
    echo "Session '$SESSION' already exists. Use 'scripts/dev.sh attach' or 'scripts/dev.sh restart <svc>'."
    exit 0
  fi

  echo "Starting tmux session '$SESSION' with 5 windows ..."

  tmux new-session -d -s "$SESSION" -n api \
    "bash '$WIN_API'"

  tmux new-window -t "$SESSION" -n ui \
    "bash '$WIN_UI'"

  tmux new-window -t "$SESSION" -n worker \
    "bash '$WIN_WORKER'"

  tmux new-window -t "$SESSION" -n niceurl \
    "NICEURL=${NICEURL:-niceurl2} bash '$WIN_NICEURL'"

  tmux new-window -t "$SESSION" -n e2e \
    "NICEURL=${NICEURL:-niceurl2} bash '$WIN_E2E'"

  # Focus on api window by default
  tmux select-window -t "$SESSION:api"

  echo ""
  echo "  tmux session '$SESSION' started."
  echo ""
  echo "  Windows:  api | ui | worker | niceurl | e2e"
  echo ""
  echo "  Attach:       scripts/dev.sh attach"
  echo "  Restart API:  scripts/dev.sh restart api"
  echo "  E2E status:   make e2e-status"
  echo "  E2E tests:    make e2e"
  echo "  Kill all:     scripts/dev.sh down"
  echo ""
  echo "  Inside tmux: prefix+n to switch windows, prefix+0-4 to jump"
  echo ""
}

cmd_down() {
  # Always attempt E2E cleanup before killing tmux: this revokes ephemeral API
  # keys, deactivates the temp member, and restores any journaled user config.
  if [[ -x "$ROOT_DIR/scripts/e2e/bootstrap.sh" ]]; then
    "$ROOT_DIR/scripts/e2e/bootstrap.sh" down || true
  fi
  if tmux has-session -t "$SESSION" 2>/dev/null; then
    tmux kill-session -t "$SESSION"
    echo "Session '$SESSION' killed."
  else
    echo "No session '$SESSION' running."
  fi

  # tmux does not always reap children started by air/webpack; make dev-down a
  # real stop so the next dev-up can bind the ports cleanly.
  for port in 8888 8081; do
    pids=$(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)
    if [[ -n "$pids" ]]; then
      kill -9 $pids 2>/dev/null || true
    fi
  done
  pkill -9 -f './tmp/lrworker' 2>/dev/null || true
  pkill -9 -f 'autossh -M 2000[0-4]' 2>/dev/null || true
}

cmd_restart() {
  local svc="${1:-}"
  if [[ -z "$svc" ]]; then
    echo "Usage: scripts/dev.sh restart <api|ui|worker|niceurl>"
    exit 1
  fi

  if ! tmux has-session -t "$SESSION" 2>/dev/null; then
    echo "No session '$SESSION' running. Start with 'scripts/dev.sh up' first."
    exit 1
  fi

  case "$svc" in
    api)
      echo "Restarting API (killing Air + Go processes on :8888) ..."
      tmux send-keys -t "$SESSION:api" C-c
      sleep 0.3
      # Kill anything still on the API port
      pids=$(lsof -ti TCP:8888 -sTCP:LISTEN 2>/dev/null || true)
      if [[ -n "$pids" ]]; then
        kill -9 $pids 2>/dev/null || true
      fi
      tmux send-keys -t "$SESSION:api" "make run-fast" Enter
      ;;
    ui)
      echo "Restarting UI (killing webpack dev server on :8081) ..."
      tmux send-keys -t "$SESSION:ui" C-c
      sleep 0.3
      pids=$(lsof -ti TCP:8081 -sTCP:LISTEN 2>/dev/null || true)
      if [[ -n "$pids" ]]; then
        kill -9 $pids 2>/dev/null || true
      fi
      tmux send-keys -t "$SESSION:ui" "cd '$ROOT_DIR/ui' && npm start" Enter
      ;;
    worker)
      echo "Restarting worker ..."
      tmux send-keys -t "$SESSION:worker" C-c
      sleep 0.3
      pkill -9 -f './tmp/lrworker' 2>/dev/null || true
      tmux send-keys -t "$SESSION:worker" \
        "cd '$ROOT_DIR' && go build -o ./tmp/lrworker . && ./tmp/lrworker worker --env-file .env" Enter
      ;;
    niceurl)
      echo "Restarting niceurl ..."
      tmux send-keys -t "$SESSION:niceurl" C-c
      sleep 0.3
      # Kill autossh monitors
      pkill -f 'autossh -M 2000[0-4]' 2>/dev/null || true
      tmux send-keys -t "$SESSION:niceurl" \
        "NICEURL=${NICEURL:-niceurl2} make ${NICEURL:-niceurl2}" Enter
      ;;
    *)
      echo "Unknown service: $svc (valid: api|ui|worker|niceurl)"
      exit 1
      ;;
  esac
  echo "Done. Watch 'scripts/dev.sh attach' to see it come back up."
}

cmd_status() {
  if ! tmux has-session -t "$SESSION" 2>/dev/null; then
    echo "No session '$SESSION' running."
    exit 0
  fi
  tmux list-windows -t "$SESSION"
  echo ""
  if [[ -x "$ROOT_DIR/scripts/e2e/bootstrap.sh" ]]; then
    "$ROOT_DIR/scripts/e2e/bootstrap.sh" status || true
  fi
}

cmd_attach() {
  if ! tmux has-session -t "$SESSION" 2>/dev/null; then
    echo "No session '$SESSION' running. Start with 'scripts/dev.sh up' first."
    exit 1
  fi
  tmux attach -t "$SESSION"
}

# --- main ---
case "${1:-up}" in
  up)       cmd_up ;;
  down)     cmd_down ;;
  restart)  cmd_restart "${2:-}" ;;
  status)   cmd_status ;;
  attach)   cmd_attach ;;
  help|-h|--help) usage ;;
  *)
    echo "Unknown command: $1"
    usage
    exit 1
    ;;
esac
