#!/usr/bin/env bash
set -euo pipefail

# Phase 7 — Prompts API spot checks
# Usage:
#   EMAIL="user@example.com" PASSWORD="secret" SERVER_URL="http://localhost:8888" bash scripts/spot_check_prompts.sh

SERVER_URL=${SERVER_URL:-http://localhost:8888}
# Defaults can be overridden via env; hardcode provided admin creds for convenience
EMAIL=${EMAIL:-shrijith@hexmos.com}
PASSWORD=${PASSWORD:-Think@1234}
TOKEN=${TOKEN:-}
ORG_ID=${ORG_ID:-}

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required. Please install jq and re-run." >&2
  exit 2
fi

if [[ -z "$TOKEN" && ( -z "$EMAIL" || -z "$PASSWORD" ) ]]; then
  echo "Set either TOKEN (preferred) or EMAIL and PASSWORD env vars. Example:" >&2
  echo "  EMAIL=shrijith@hexmos.com PASSWORD='Werfds@1234' bash scripts/spot_check_prompts.sh" >&2
  echo "  TOKEN=eyJ... ORG_ID=1 bash scripts/spot_check_prompts.sh" >&2
  exit 2
fi

step() { echo -e "\n==> $1"; }
pass() { echo "✅ $1"; }
fail() { echo "❌ $1"; exit 1; }

step "Health check"
curl -fsS "$SERVER_URL/health" | jq . || echo "(warning) health endpoint not JSON or not reachable"

if [[ -z "$TOKEN" ]]; then
  step "Login as $EMAIL"
  # Capture HTTP status and body for good diagnostics
  LOGIN_BODY=$(mktemp)
  trap 'rm -f "$LOGIN_BODY"' EXIT
  # Use --fail-with-body where available; otherwise capture output and then check status via a second HEAD
  HTTP_CODE=$(curl -sS -o "$LOGIN_BODY" -w "%{http_code}" -X POST "$SERVER_URL/api/v1/auth/login" \
    -H 'Content-Type: application/json' \
    -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}") || true
  if [[ "$HTTP_CODE" != "200" ]]; then
    echo "Login HTTP $HTTP_CODE" >&2
    sed 's/.*/  &/' "$LOGIN_BODY" >&2 || true
    fail "Login failed. Provide a valid TOKEN or correct credentials."
  fi
  TOKEN=$(jq -r '.tokens.access_token' "$LOGIN_BODY")
  if [[ -z "$TOKEN" || "$TOKEN" == "null" ]]; then
    jq . "$LOGIN_BODY" || true
    fail "Login did not return an access token."
  fi
  # Try to capture ORG_ID from login response if available
  if [[ -z "${ORG_ID:-}" ]]; then
    ORG_ID=$(jq -r '.organizations[0].id // empty' "$LOGIN_BODY")
  fi
fi

if [[ -z "$ORG_ID" ]]; then
  ORG_JSON=$(curl -fsS "$SERVER_URL/api/v1/organizations" -H "Authorization: Bearer $TOKEN") || true
  # Primary (current shape): {"organizations": [...]}
  ORG_ID=$(echo "$ORG_JSON" | jq -r '.organizations[0].id // empty')
  # Back-compat (older shape): [ ... ]
  if [[ -z "$ORG_ID" ]]; then
    ORG_ID=$(echo "$ORG_JSON" | jq -r '.[0].id // empty')
  fi
  if [[ -z "$ORG_ID" ]]; then
    echo "$ORG_JSON" | jq . || true
    fail "No organizations found for this user (unable to determine ORG_ID)"
  fi
fi

echo "Token length: ${#TOKEN}, ORG_ID: $ORG_ID"
pass "Authenticated and org context resolved"

AUTH=(-H "Authorization: Bearer $TOKEN" -H "X-Org-Context: $ORG_ID")

step "Catalog"
CATALOG=$(curl -fsS "$SERVER_URL/api/v1/prompts/catalog" "${AUTH[@]}") || true
echo "$CATALOG" | jq .

# We will use prompt_key=code_review for checks
PROMPT_KEY=code_review

step "Pre-flight: variables before creating chunks"
curl -fsS "$SERVER_URL/api/v1/prompts/$PROMPT_KEY/variables" "${AUTH[@]}" | jq . || true

create_chunk_if_missing() {
  local var_name=$1 title=$2 body=$3
  echo "-- ensuring chunk for variable '$var_name' with title '$title'"
  VARS=$(curl -fsS "$SERVER_URL/api/v1/prompts/$PROMPT_KEY/variables" "${AUTH[@]}") || true
  HAS=$(echo "$VARS" | jq -r --arg v "$var_name" --arg t "$title" '
    (.variables // []) | map(select(.name==$v) | .chunks[]?.title) | index($t) | tostring')
  if [[ "$HAS" != "null" && "$HAS" != "" ]]; then
    echo "   exists; skipping create"
    return 0
  fi
  curl -fsS -X POST "$SERVER_URL/api/v1/prompts/$PROMPT_KEY/variables/$var_name/chunks" \
    "${AUTH[@]}" -H 'Content-Type: application/json' \
    -d "$(jq -nc --arg title "$title" --arg body "$body" '{type:"user", title:$title, body:$body}')" | jq .
}

step "Create chunks (idempotent)"
create_chunk_if_missing "style_guide" "Go Style Guide" \
  "Follow Go conventions (gofmt, go vet), prefer clear naming, keep functions small."
create_chunk_if_missing "security_guidelines" "Security Guidelines" \
  "Avoid hard-coded secrets, validate inputs, use prepared statements, check authz on every handler."

step "Variables after creation"
curl -fsS "$SERVER_URL/api/v1/prompts/$PROMPT_KEY/variables" "${AUTH[@]}" | jq '{prompt_key, provider, variables: [.variables[] | {name, titles: [.chunks[].title]}]}'

step "Render preview"
RENDER=$(curl -fsS "$SERVER_URL/api/v1/prompts/$PROMPT_KEY/render" "${AUTH[@]}")
echo "$RENDER" | jq '{provider, build_id: .build_id, prompt: (.prompt | (split("\n")[0:40] | join("\n")))}'

pass "Spot checks completed"