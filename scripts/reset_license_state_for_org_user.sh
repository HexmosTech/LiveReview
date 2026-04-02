#!/usr/bin/env bash
set -euo pipefail

# Reset license/subscription state for one org validated by an org name + user email pair.
# This is intended for local testing where you want a clean billing/license state.
#
# Usage:
#   ./scripts/reset_license_state_for_org_user.sh --org-name "Hexmos01" --email "contortedexpression@gmail.com"
#   ./scripts/reset_license_state_for_org_user.sh --org-name "Hexmos01" --email "contortedexpression@gmail.com" --dry-run
#   ./scripts/reset_license_state_for_org_user.sh --org-name "Hexmos01" --email "contortedexpression@gmail.com" --yes
#   ./scripts/reset_license_state_for_org_user.sh --prod --org-name "Hexmos01" --email "contortedexpression@gmail.com" --yes

# IMPORTANT: for clearing the personal org
#  ./scripts/reset_license_state_for_org_user.sh --org-name "contortedexpression@gmail.com" --email "contortedexpression@gmail.com" --yes

ENV_FILE=".env"
DRY_RUN="false"
AUTO_YES="false"
ORG_NAME=""
USER_EMAIL=""

usage() {
  echo "Usage: $0 [--prod] [--dry-run] [--yes] --org-name <org_name> --email <user_email>"
  exit 1
}

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
    --yes)
      AUTO_YES="true"
      shift
      ;;
    --org-name)
      ORG_NAME="${2:-}"
      shift 2
      ;;
    --email)
      USER_EMAIL="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      ;;
    *)
      echo "Unknown argument: $1"
      usage
      ;;
  esac
done

if [[ -z "$ORG_NAME" || -z "$USER_EMAIL" ]]; then
  usage
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

PSQL=(psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -X -P pager=off)

ORG_NAME_SQL=${ORG_NAME//\'/\'\'}
USER_EMAIL_SQL=${USER_EMAIL//\'/\'\'}

echo "Using env file: $ENV_FILE"
echo "Target org name: $ORG_NAME"
echo "Target user email: $USER_EMAIL"

TARGET_ROW=$("${PSQL[@]}" \
  -At -F '|' \
  -c "
SELECT o.id, u.id
FROM orgs o
JOIN user_roles ur ON ur.org_id = o.id
JOIN users u ON u.id = ur.user_id
WHERE lower(o.name) = lower('${ORG_NAME_SQL}')
  AND lower(u.email) = lower('${USER_EMAIL_SQL}')
LIMIT 1;")

if [[ -z "$TARGET_ROW" ]]; then
  echo "Error: could not find membership for org '$ORG_NAME' and email '$USER_EMAIL'"
  echo "Hint: verify the org name/email and that the user belongs to that org"
  exit 1
fi

ORG_ID="${TARGET_ROW%%|*}"
USER_ID="${TARGET_ROW##*|}"

echo "Resolved org_id=$ORG_ID, user_id=$USER_ID"

echo "--- Before reset summary ---"
"${PSQL[@]}" \
  -c "
SELECT
  o.id AS org_id,
  o.name AS org_name,
  u.id AS user_id,
  u.email,
  COALESCE(obs.current_plan_code, '(none)') AS current_plan_code,
  COALESCE(obs.scheduled_plan_code, '(none)') AS scheduled_plan_code,
  COALESCE(obs.loc_used_month, 0) AS loc_used_month,
  COALESCE(obs.upgrade_loc_grant_current_cycle, 0) AS upgrade_loc_grant_current_cycle,
  (SELECT COUNT(*) FROM subscriptions s WHERE s.org_id = o.id) AS subscriptions,
  (
    SELECT COUNT(*)
    FROM subscription_payments sp
    WHERE sp.subscription_id IN (SELECT id FROM subscriptions s2 WHERE s2.org_id = o.id)
  ) AS subscription_payments,
  (SELECT COUNT(*) FROM license_log ll WHERE ll.org_id = o.id) AS license_logs,
  (SELECT COUNT(*) FROM upgrade_requests urq WHERE urq.org_id = o.id) AS upgrade_requests,
  (SELECT COUNT(*) FROM upgrade_payment_attempts upa WHERE upa.org_id = o.id) AS upgrade_payment_attempts,
  (SELECT COUNT(*) FROM loc_usage_ledger lul WHERE lul.org_id = o.id) AS loc_usage_ledger_rows,
  (SELECT COUNT(*) FROM loc_lifecycle_log lll WHERE lll.org_id = o.id) AS loc_lifecycle_rows
FROM orgs o
JOIN users u ON u.id = ${USER_ID}::bigint
LEFT JOIN org_billing_state obs ON obs.org_id = o.id
WHERE o.id = ${ORG_ID}::bigint;
"

read -r -d '' RESET_SQL <<SQL || true
BEGIN;

CREATE TEMP TABLE _target_subscriptions ON COMMIT DROP AS
SELECT id
FROM subscriptions
WHERE org_id = ${ORG_ID}::bigint;

-- Ensure all members in org are detached from paid seats/subscriptions.
UPDATE user_roles
SET active_subscription_id = NULL,
    plan_type = 'free',
    license_expires_at = NULL,
    updated_at = NOW()
WHERE org_id = ${ORG_ID}::bigint;

-- Clear seat assignments for users in this org.
DELETE FROM license_seat_assignments
WHERE user_id IN (
  SELECT ur.user_id
  FROM user_roles ur
  WHERE ur.org_id = ${ORG_ID}::bigint
);

-- Remove in-progress and historical upgrade process records.
DELETE FROM upgrade_payment_attempts
WHERE org_id = ${ORG_ID}::bigint;

DELETE FROM upgrade_requests
WHERE org_id = ${ORG_ID}::bigint;

-- Remove subscription-linked payment and audit rows for this org.
DELETE FROM subscription_payments
WHERE subscription_id IN (SELECT id FROM _target_subscriptions);

DELETE FROM license_log
WHERE org_id = ${ORG_ID}::bigint
   OR subscription_id IN (SELECT id FROM _target_subscriptions);

-- Delete all subscriptions for this org.
DELETE FROM subscriptions
WHERE id IN (SELECT id FROM _target_subscriptions);

-- Clear LOC accounting history for a fresh cycle test.
DELETE FROM loc_usage_ledger
WHERE org_id = ${ORG_ID}::bigint;

DELETE FROM loc_lifecycle_log
WHERE org_id = ${ORG_ID}::bigint;

-- Ensure deterministic free plan baseline in org billing state.
INSERT INTO org_billing_state (
  org_id,
  current_plan_code,
  billing_period_start,
  billing_period_end,
  loc_used_month,
  loc_blocked,
  trial_readonly,
  scheduled_plan_code,
  scheduled_plan_effective_at,
  last_reset_at,
  upgrade_loc_grant_current_cycle,
  upgrade_loc_grant_expires_at,
  created_at,
  updated_at
)
VALUES (
  ${ORG_ID}::bigint,
  'free_30k',
  date_trunc('month', now() AT TIME ZONE 'UTC'),
  date_trunc('month', now() AT TIME ZONE 'UTC') + interval '1 month',
  0,
  FALSE,
  FALSE,
  NULL,
  NULL,
  NOW(),
  0,
  NULL,
  NOW(),
  NOW()
)
ON CONFLICT (org_id) DO UPDATE
SET current_plan_code = EXCLUDED.current_plan_code,
    billing_period_start = EXCLUDED.billing_period_start,
    billing_period_end = EXCLUDED.billing_period_end,
    loc_used_month = EXCLUDED.loc_used_month,
    loc_blocked = EXCLUDED.loc_blocked,
    trial_readonly = EXCLUDED.trial_readonly,
    scheduled_plan_code = EXCLUDED.scheduled_plan_code,
    scheduled_plan_effective_at = EXCLUDED.scheduled_plan_effective_at,
    last_reset_at = EXCLUDED.last_reset_at,
    upgrade_loc_grant_current_cycle = EXCLUDED.upgrade_loc_grant_current_cycle,
    upgrade_loc_grant_expires_at = EXCLUDED.upgrade_loc_grant_expires_at,
    updated_at = NOW();

UPDATE orgs
SET subscription_plan = 'free',
    updated_at = NOW()
WHERE id = ${ORG_ID}::bigint;

COMMIT;
SQL

if [[ "$DRY_RUN" == "true" ]]; then
  echo "--- Dry run only: no changes applied ---"
  printf '%s\n' "$RESET_SQL"
  exit 0
fi

if [[ "$AUTO_YES" != "true" ]]; then
  echo ""
  echo "This will DELETE subscriptions/payments/license logs/upgrade records for org_id=$ORG_ID."
  read -r -p "Continue? [y/N] " CONFIRM
  if [[ ! "$CONFIRM" =~ ^[Yy]$ ]]; then
    echo "Cancelled."
    exit 0
  fi
fi

"${PSQL[@]}" -c "$RESET_SQL"

echo "--- After reset summary ---"
"${PSQL[@]}" \
  -c "
SELECT
  o.id AS org_id,
  o.name AS org_name,
  u.id AS user_id,
  u.email,
  COALESCE(obs.current_plan_code, '(none)') AS current_plan_code,
  COALESCE(obs.scheduled_plan_code, '(none)') AS scheduled_plan_code,
  COALESCE(obs.loc_used_month, 0) AS loc_used_month,
  COALESCE(obs.upgrade_loc_grant_current_cycle, 0) AS upgrade_loc_grant_current_cycle,
  (SELECT COUNT(*) FROM subscriptions s WHERE s.org_id = o.id) AS subscriptions,
  (
    SELECT COUNT(*)
    FROM subscription_payments sp
    WHERE sp.subscription_id IN (SELECT id FROM subscriptions s2 WHERE s2.org_id = o.id)
  ) AS subscription_payments,
  (SELECT COUNT(*) FROM license_log ll WHERE ll.org_id = o.id) AS license_logs,
  (SELECT COUNT(*) FROM upgrade_requests urq WHERE urq.org_id = o.id) AS upgrade_requests,
  (SELECT COUNT(*) FROM upgrade_payment_attempts upa WHERE upa.org_id = o.id) AS upgrade_payment_attempts,
  (SELECT COUNT(*) FROM loc_usage_ledger lul WHERE lul.org_id = o.id) AS loc_usage_ledger_rows,
  (SELECT COUNT(*) FROM loc_lifecycle_log lll WHERE lll.org_id = o.id) AS loc_lifecycle_rows
FROM orgs o
JOIN users u ON u.id = ${USER_ID}::bigint
LEFT JOIN org_billing_state obs ON obs.org_id = o.id
WHERE o.id = ${ORG_ID}::bigint;
"

echo "Reset complete for org '$ORG_NAME' (org_id=$ORG_ID), validated via user '$USER_EMAIL' (user_id=$USER_ID)."