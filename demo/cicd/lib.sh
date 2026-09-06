#!/usr/bin/env bash
# Shared polling logic, mirroring exactly what .github/workflows/livereview-gate.yml
# runs in shrsv/claude-world -- kept here too so it can be dry-run locally
# against a real SHA before trusting it inside an actual GitHub Actions run.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./config.sh
source "$SCRIPT_DIR/config.sh"

# wait_and_evaluate <ruleset_id> <sha> [max_attempts] [sleep_secs]
# Prints ALLOW/BLOCK/timeout to stdout, returns 0 (allow), 1 (block), 2 (timeout/error).
wait_and_evaluate() {
  require_api_key
  local ruleset_id="$1" sha="$2" max_attempts="${3:-30}" sleep_secs="${4:-10}"

  for attempt in $(seq 1 "$max_attempts"); do
    local cov_resp review_id
    cov_resp=$(lr_curl POST /review-coverage "{\"commits\":[\"$sha\"]}")
    review_id=$(echo "$cov_resp" | sed '$d' | jq -r '.reports[0].review_id // empty' 2>/dev/null || true)

    if [ -n "$review_id" ]; then
      local eval_resp eval_code eval_body
      eval_resp=$(lr_curl GET "/ci-rulesets/$ruleset_id/evaluate?review_id=$review_id")
      eval_code=$(echo "$eval_resp" | tail -1)
      eval_body=$(echo "$eval_resp" | sed '$d')

      case "$eval_code" in
        200)
          echo "ALLOW (review $review_id): $eval_body"
          return 0
          ;;
        422)
          echo "BLOCK (review $review_id): $eval_body"
          return 1
          ;;
        202)
          echo "  [$attempt/$max_attempts] review $review_id still running, retrying in ${sleep_secs}s..."
          ;;
        *)
          echo "  [$attempt/$max_attempts] unexpected evaluate response ($eval_code): $eval_body"
          ;;
      esac
    else
      echo "  [$attempt/$max_attempts] no review found yet for $sha, retrying in ${sleep_secs}s..."
    fi
    sleep "$sleep_secs"
  done

  echo "TIMEOUT: no allow/block result for $sha after $((max_attempts * sleep_secs))s"
  return 2
}

# trigger_review <pr_url>
trigger_review() {
  require_api_key
  local pr_url="$1"
  lr_curl POST /connectors/trigger-review "{\"url\":\"$pr_url\"}" | sed '$d'
}

# skip_local_lrc_attestation
# This machine has a global `lrc` pre-commit hook (core.hooksPath) that
# blocks any commit without a review attestation. The demo commits are
# synthetic (deliberately vulnerable, or trivially safe) and the gate that
# actually matters for the demo runs later via GitHub Actions against the
# pushed commit -- so local commit-time review is irrelevant here and safe
# to skip. No-ops if `lrc` isn't installed.
skip_local_lrc_attestation() {
  if command -v lrc >/dev/null 2>&1; then
    lrc review --staged --skip >/dev/null 2>&1 || true
  fi
}
