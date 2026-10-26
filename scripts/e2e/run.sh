#!/usr/bin/env bash
# Run the local LiveReview E2E suites.  Safe by default: AI/git failures are
# reported as capability gaps, not red tests, unless --strict is passed.
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=common.sh
source "$DIR/common.sh"

STRICT=0
SKIP_UI=0
SKIP_REVIEW=0
SKIP_API=0
SKIP_URLS=0
HEADED="${E2E_HEADED:-0}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --strict) STRICT=1 ;;
    --skip-ui) SKIP_UI=1 ;;
    --skip-review) SKIP_REVIEW=1 ;;
    --skip-api) SKIP_API=1 ;;
    --skip-urls) SKIP_URLS=1 ;;
    --headed) HEADED=1 ;;
    -h|--help)
      cat <<'EOF'
Usage: scripts/e2e/run.sh [options]

  --strict        Treat broken AI/git/public URLs as failures
  --skip-ui       Skip Playwright UI tests
  --skip-review   Skip the real lrc review
  --skip-api      Skip API audit assertions
  --skip-urls     Skip the manual-talent URL sweep
  --headed        Run Playwright with a visible browser
EOF
      exit 0
      ;;
    *) echo "Unknown option: $1" >&2; exit 2 ;;
  esac
  shift
done

export E2E_HEADED="$HEADED"
mkdir -p "$E2E_DIR"
chmod 700 "$E2E_DIR" 2>/dev/null || true

if [[ ! -f "$E2E_DIR/state.json" ]]; then
  echo "No E2E state; provisioning from the cached session..."
  if ! "$PYTHON_BIN" "$DIR/control.py" up; then
    echo "Could not provision E2E. Run 'make e2e-auth' once, then retry." >&2
    exit 2
  fi
fi

load_run_env
export E2E_HEADED

if [[ "$SKIP_UI" -eq 0 ]]; then
  if ! "$PYTHON_BIN" "$DIR/control.py" sync-ui; then
    echo "Warning: could not refresh Playwright state; UI tests may be skipped or fail." >&2
  fi
fi

overall=0
declare -A steps=()

step() {
  local name="$1"; shift
  echo ""
  echo "=== $name ==="
  if "$@"; then
    steps["$name"]=0
  else
    local rc=$?
    steps["$name"]=$rc
    overall=1
  fi
}

if [[ "$SKIP_URLS" -eq 0 ]]; then
  if [[ "$STRICT" -eq 1 ]]; then
    step "public URLs" "$PYTHON_BIN" "$DIR/urls.py" --strict
  else
    step "public URLs" "$PYTHON_BIN" "$DIR/urls.py"
  fi
fi

if [[ "$SKIP_URLS" -eq 1 && "$SKIP_API" -eq 1 && "$SKIP_REVIEW" -eq 1 ]]; then
  echo ""
  echo "=== preflight ==="
  echo "  [--] skipped: UI-only run"
else
  if [[ "$STRICT" -eq 1 ]]; then
    step "preflight" "$PYTHON_BIN" "$DIR/preflight.py" --strict
  else
    step "preflight" "$PYTHON_BIN" "$DIR/preflight.py"
  fi
fi

# Run the real review before audit assertions so the AI-usage tab/ledger has a
# fresh row to verify in the same run.
if [[ "$SKIP_REVIEW" -eq 0 ]]; then
  if [[ "$STRICT" -eq 1 ]]; then
    step "lrc review" "$PYTHON_BIN" "$DIR/review_test.py" --strict
  else
    step "lrc review" "$PYTHON_BIN" "$DIR/review_test.py"
  fi
fi

if [[ "$SKIP_API" -eq 0 ]]; then
  step "audit API" "$PYTHON_BIN" "$DIR/audit_tests.py"
fi

if [[ "$SKIP_UI" -eq 0 ]]; then
  if [[ ! -f "${E2E_PLAYWRIGHT_STATE:-$E2E_DIR/playwright-state.json}" ]]; then
    echo ""
    echo "=== UI ==="
    echo "  [--] skipped: no Playwright state (run 'make e2e-auth' / 'make dev-up')"
  elif ! curl -fsS --max-time 5 "${E2E_BASE_URL:-http://localhost:8081}/" >/dev/null 2>&1; then
    echo ""
    echo "=== UI ==="
    echo "  [--] skipped: UI is not reachable at ${E2E_BASE_URL:-http://localhost:8081}"
  else
    echo ""
    echo "=== UI ==="
    if (cd "$ROOT_DIR/scripts/integration" && ./node_modules/.bin/playwright test --config=playwright.e2e.config.js); then
      steps["UI"]=0
    else
      steps["UI"]=$?
      overall=1
    fi
  fi
fi

echo ""
echo "=== E2E summary ==="
for name in "public URLs" "preflight" "audit API" "lrc review" "UI"; do
  if [[ -n "${steps[$name]+x}" ]]; then
    if [[ "${steps[$name]}" -eq 0 ]]; then
      echo "  [ok] $name"
    else
      echo "  [!!] $name"
    fi
  fi
done
if [[ -f "${E2E_ARTIFACTS:-$E2E_DIR/artifacts}/playwright-report/index.html" ]]; then
  echo "  UI report: ${E2E_ARTIFACTS:-$E2E_DIR/artifacts}/playwright-report/index.html"
fi
if [[ -f "$E2E_DIR/audit-report.json" ]]; then
  echo "  audit report: $E2E_DIR/audit-report.json"
fi

exit "$overall"
