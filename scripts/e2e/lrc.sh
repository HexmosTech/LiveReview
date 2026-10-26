#!/usr/bin/env bash
# Run lrc against the local E2E sandbox without touching ~/.lrc.toml.
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=common.sh
source "$DIR/common.sh"

load_run_env

if [[ -z "${E2E_LRC_HOME:-}" ]]; then
  echo "No E2E lrc sandbox. Start with: make dev-up NICEURL=niceurl3 (or make e2e-up)" >&2
  exit 2
fi

export HOME="$E2E_LRC_HOME"
export GIT_CONFIG_GLOBAL="$E2E_LRC_HOME/.gitconfig"
export LRC_API_URL="${E2E_API_URL:-http://localhost:8888}"
if [[ -n "${E2E_LRC_KEY:-}" ]]; then
  export LRC_API_KEY="$E2E_LRC_KEY"
elif [[ -n "${E2E_STANDARD_KEY:-}" ]]; then
  export LRC_API_KEY="$E2E_STANDARD_KEY"
fi

# lrc may call git and needs a writable HOME; the sandbox supplies both.  The
# real user's config is never read or written.
exec lrc "$@"
