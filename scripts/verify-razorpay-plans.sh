#!/usr/bin/env bash

set -euo pipefail

ENV_FILE="${1:-.env.prod}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "env file not found: $ENV_FILE" >&2
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required" >&2
  exit 1
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required" >&2
  exit 1
fi

set -a
. "$ENV_FILE"
set +a

if [[ -z "${RAZORPAY_LIVE_KEY:-}" || -z "${RAZORPAY_LIVE_SECRET:-}" ]]; then
  echo "RAZORPAY_LIVE_KEY and RAZORPAY_LIVE_SECRET must be set in $ENV_FILE" >&2
  exit 1
fi

declare -a PLAN_VARS=(
  RAZORPAY_LIVE_ACTUAL_MONTHLY_PLAN_ID_USD
  RAZORPAY_LIVE_ACTUAL_YEARLY_PLAN_ID_USD
  RAZORPAY_LIVE_ACTUAL_MONTHLY_PLAN_ID_INR
  RAZORPAY_LIVE_ACTUAL_YEARLY_PLAN_ID_INR
  RAZORPAY_LIVE_LOW_PRICING_MONTHLY_PLAN_ID_USD
  RAZORPAY_LIVE_LOW_PRICING_YEARLY_PLAN_ID_USD
  RAZORPAY_LIVE_LOW_PRICING_MONTHLY_PLAN_ID_INR
  RAZORPAY_LIVE_LOW_PRICING_YEARLY_PLAN_ID_INR
)

printf '%-44s %-20s %-8s %-12s %-10s %-8s %s\n' "ENV_VAR" "PLAN_ID" "CCY" "AMOUNT" "PERIOD" "INTERVAL" "NAME"

for var_name in "${PLAN_VARS[@]}"; do
  plan_id="${!var_name:-}"
  if [[ -z "$plan_id" ]]; then
    printf '%-44s %-20s %-8s %-12s %-10s %-8s %s\n' "$var_name" "<missing>" "-" "-" "-" "-" "-"
    continue
  fi

  response="$(curl -fsS -u "$RAZORPAY_LIVE_KEY:$RAZORPAY_LIVE_SECRET" "https://api.razorpay.com/v1/plans/$plan_id")"
  parsed="$(python3 - "$response" <<'PY'
import json
import sys

payload = json.loads(sys.argv[1])
item = payload.get("item") or {}
name = str(item.get("name") or "").replace("\n", " ").replace("\r", " ")
print("\t".join([
    str(payload.get("id") or ""),
    str(item.get("currency") or ""),
    str(item.get("amount") or ""),
    str(payload.get("period") or ""),
    str(payload.get("interval") or ""),
    name,
]))
PY
)"

  IFS=$'\t' read -r resolved_id currency amount period interval plan_name <<< "$parsed"
  printf '%-44s %-20s %-8s %-12s %-10s %-8s %s\n' "$var_name" "$resolved_id" "$currency" "$amount" "$period" "$interval" "$plan_name"
done