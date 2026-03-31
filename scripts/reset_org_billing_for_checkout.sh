#!/usr/bin/env bash
set -euo pipefail

# Reset one org to free_30k and remove subscription linkage so checkout testing is repeatable.
# Usage:
#   ./scripts/reset_org_billing_for_checkout.sh <org_id>
#   ./scripts/reset_org_billing_for_checkout.sh --prod <org_id>
#   ./scripts/reset_org_billing_for_checkout.sh --dry-run <org_id>
#   ./scripts/reset_org_billing_for_checkout.sh --prod --dry-run <org_id>

ENV_FILE=".env"
DRY_RUN="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --prod)
      ENV_FILE=".env.prod"
      shift
      ;;
    --dry-run)
      DRY_RUN="true"
      shift
      ;;
    -h|--help)
      echo "Usage: $0 [--prod] [--dry-run] <org_id>"
      exit 0
      ;;
    *)
      break
      ;;
  esac
done

if [[ $# -ne 1 ]]; then
  echo "Usage: $0 [--prod] [--dry-run] <org_id>"
  exit 1
fi

ORG_ID="$1"
if ! [[ "$ORG_ID" =~ ^[0-9]+$ ]]; then
  echo "Error: org_id must be a numeric value"
  exit 1
fi

if [[ ! -f "$ENV_FILE" ]]; then
  echo "Error: env file $ENV_FILE not found"
  exit 1
fi

# shellcheck disable=SC1090
set -a
source "$ENV_FILE"
set +a

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "Error: DATABASE_URL is not set in $ENV_FILE"
  exit 1
fi

PSQL=(psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -X)

echo "Using env file: $ENV_FILE"
echo "Target org_id: $ORG_ID"

echo "--- Before reset ---"
"${PSQL[@]}" -c "
SELECT
  o.id AS org_id,
  o.name,
  COALESCE(obs.current_plan_code, '(none)') AS current_plan_code,
  COALESCE(obs.scheduled_plan_code, '(none)') AS scheduled_plan_code,
  COALESCE(obs.loc_used_month, 0) AS loc_used_month,
  (SELECT COUNT(*) FROM subscriptions s WHERE s.org_id = o.id) AS subscriptions,
  (SELECT COUNT(*) FROM subscriptions s WHERE s.org_id = o.id AND lower(s.status) = 'active') AS active_subscriptions
FROM orgs o
LEFT JOIN org_billing_state obs ON obs.org_id = o.id
WHERE o.id = ${ORG_ID};
"

read -r -d '' RESET_SQL <<SQL || true
BEGIN;

-- Ensure billing row exists so reset is deterministic.
INSERT INTO org_billing_state (
  org_id,
  current_plan_code,
  billing_period_start,
  billing_period_end,
  loc_used_month,
  loc_blocked,
  trial_readonly,
  last_reset_at,
  updated_at
)
VALUES (
  ${ORG_ID},
  'free_30k',
  date_trunc('month', now() AT TIME ZONE 'UTC'),
  date_trunc('month', now() AT TIME ZONE 'UTC') + interval '1 month',
  0,
  FALSE,
  FALSE,
  NOW(),
  NOW()
)
ON CONFLICT (org_id) DO NOTHING;

-- Detach users from any team subscription for this org and force free role.
UPDATE user_roles ur
SET active_subscription_id = NULL,
    plan_type = 'free',
  license_expires_at = NULL
WHERE ur.org_id = ${ORG_ID};

-- Preserve license log rows but remove hard FK dependency before deleting subscriptions.
UPDATE license_log ll
SET subscription_id = NULL
WHERE ll.subscription_id IN (
  SELECT s.id FROM subscriptions s WHERE s.org_id = ${ORG_ID}
);

-- Remove dependent payment rows then remove subscriptions for this org.
DELETE FROM subscription_payments sp
WHERE sp.subscription_id IN (
  SELECT s.id FROM subscriptions s WHERE s.org_id = ${ORG_ID}
);

DELETE FROM subscriptions s
WHERE s.org_id = ${ORG_ID};

-- Reset org billing state to free_30k/no-scheduled plan and clear usage for repeatable test.
UPDATE org_billing_state
SET current_plan_code = 'free_30k',
    scheduled_plan_code = NULL,
    scheduled_plan_effective_at = NULL,
    trial_readonly = FALSE,
    loc_blocked = FALSE,
    loc_used_month = 0,
    billing_period_start = date_trunc('month', now() AT TIME ZONE 'UTC'),
    billing_period_end = date_trunc('month', now() AT TIME ZONE 'UTC') + interval '1 month',
    last_reset_at = NOW(),
    updated_at = NOW()
WHERE org_id = ${ORG_ID};

-- Keep legacy org-level marker aligned.
UPDATE orgs
SET subscription_plan = 'free'
WHERE id = ${ORG_ID};

COMMIT;
SQL

if [[ "$DRY_RUN" == "true" ]]; then
  echo "--- Dry run only: no changes applied ---"
  echo "$RESET_SQL"
  exit 0
fi

"${PSQL[@]}" -c "$RESET_SQL"

echo "--- After reset ---"
"${PSQL[@]}" -c "
SELECT
  o.id AS org_id,
  o.name,
  COALESCE(obs.current_plan_code, '(none)') AS current_plan_code,
  COALESCE(obs.scheduled_plan_code, '(none)') AS scheduled_plan_code,
  COALESCE(obs.loc_used_month, 0) AS loc_used_month,
  (SELECT COUNT(*) FROM subscriptions s WHERE s.org_id = o.id) AS subscriptions,
  (SELECT COUNT(*) FROM subscriptions s WHERE s.org_id = o.id AND lower(s.status) = 'active') AS active_subscriptions
FROM orgs o
LEFT JOIN org_billing_state obs ON obs.org_id = o.id
WHERE o.id = ${ORG_ID};
"

echo "Reset complete for org_id=$ORG_ID"
