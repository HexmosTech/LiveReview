#!/usr/bin/env bash
# Shared config for the CI/CD Gate demo scripts. Source this, don't run it.
#
# Credentials (never hardcoded/committed here) are read from demo/cicd/.env
# if it exists (gitignored -- see .env.example), otherwise from whatever's
# already exported in your shell:
#   LIVEREVIEW_API_KEY   - an owner-role API key for the org (Settings -> API Keys)
#   LIVEREVIEW_ORG_ID    - that org's id
#   LIVEREVIEW_RULESET_ID - printed by create_ruleset.sh the first time you run it
#
# make_blocked_pr.sh / make_allowed_pr.sh / reset_demo.sh also need `gh` auth
# (already logged in as shrsv).

set -euo pipefail

BASE_URL="https://manual-talent2.apps.hexmos.com"
REPO="shrsv/claude-world"
RULESET_NAME="Block on Critical or Security"
RULESET_JQ='(.counts.by_severity.critical > 0) or (.counts.by_category.security > 0)'
BRANCH_PREFIX="demo"

SCRIPT_DIR_CONFIG="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ -f "$SCRIPT_DIR_CONFIG/.env" ]; then
  set -a
  # shellcheck source=./.env
  source "$SCRIPT_DIR_CONFIG/.env"
  set +a
fi

: "${LIVEREVIEW_API_KEY:=}"
: "${LIVEREVIEW_ORG_ID:=}"
: "${LIVEREVIEW_RULESET_ID:=}"

require_api_key() {
  if [ -z "$LIVEREVIEW_API_KEY" ] || [ -z "$LIVEREVIEW_ORG_ID" ]; then
    echo "Set LIVEREVIEW_API_KEY and LIVEREVIEW_ORG_ID in your shell first (see demo/cicd/README.md)." >&2
    exit 1
  fi
}

lr_curl() {
  # lr_curl <method> <path> [json-body]
  local method="$1" path="$2" body="${3:-}"
  if [ -n "$body" ]; then
    curl -sS -w '\n%{http_code}' -X "$method" \
      -H "X-API-Key: $LIVEREVIEW_API_KEY" -H "X-Org-Context: $LIVEREVIEW_ORG_ID" \
      -H "Content-Type: application/json" -d "$body" \
      "$BASE_URL/api/v1$path"
  else
    curl -sS -w '\n%{http_code}' -X "$method" \
      -H "X-API-Key: $LIVEREVIEW_API_KEY" -H "X-Org-Context: $LIVEREVIEW_ORG_ID" \
      "$BASE_URL/api/v1$path"
  fi
}
