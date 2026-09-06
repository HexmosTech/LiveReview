#!/usr/bin/env bash
# Idempotently creates (or reuses) the "Block on Critical or Security" CI/CD
# ruleset in LiveReview. Prints its id -- export that as LIVEREVIEW_RULESET_ID
# and store it as the claude-world repo secret of the same name.

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./config.sh
source "$SCRIPT_DIR/config.sh"
require_api_key

# save_ruleset_id <id>
# Persists the id into demo/cicd/.env (creating it from .env.example if
# missing) so every other script picks it up automatically next time.
save_ruleset_id() {
  local id="$1" env_file="$SCRIPT_DIR/.env"
  if [ ! -f "$env_file" ]; then
    cp "$SCRIPT_DIR/.env.example" "$env_file"
  fi
  if grep -q '^LIVEREVIEW_RULESET_ID=' "$env_file"; then
    sed -i "s/^LIVEREVIEW_RULESET_ID=.*/LIVEREVIEW_RULESET_ID=$id/" "$env_file"
  else
    echo "LIVEREVIEW_RULESET_ID=$id" >> "$env_file"
  fi
  echo "Saved LIVEREVIEW_RULESET_ID=$id to demo/cicd/.env" >&2
}

echo "Looking for an existing ruleset named '$RULESET_NAME'..." >&2
list_resp=$(lr_curl GET /ci-rulesets)
list_code=$(echo "$list_resp" | tail -1)
list_body=$(echo "$list_resp" | sed '$d')
if [ "$list_code" != "200" ]; then
  echo "Failed to list rulesets ($list_code): $list_body" >&2
  exit 1
fi

existing_id=$(echo "$list_body" | jq -r --arg name "$RULESET_NAME" '.rows[]? | select(.name == $name) | .id' | head -1)
if [ -n "$existing_id" ]; then
  echo "Reusing existing ruleset id=$existing_id" >&2
  save_ruleset_id "$existing_id"
  echo "$existing_id"
  exit 0
fi

echo "Creating ruleset '$RULESET_NAME'..." >&2
body=$(jq -n --arg name "$RULESET_NAME" --arg desc "Demo gate: blocks PR merges when the review found any critical-severity or security-category finding." --arg jq "$RULESET_JQ" \
  '{name: $name, description: $desc, jq_expr: $jq}')
create_resp=$(lr_curl POST /ci-rulesets "$body")
create_code=$(echo "$create_resp" | tail -1)
create_body=$(echo "$create_resp" | sed '$d')
if [ "$create_code" != "200" ]; then
  echo "Failed to create ruleset ($create_code): $create_body" >&2
  exit 1
fi

new_id=$(echo "$create_body" | jq -r '.ruleset.id')
echo "Created ruleset id=$new_id" >&2
save_ruleset_id "$new_id"
echo "$new_id"
