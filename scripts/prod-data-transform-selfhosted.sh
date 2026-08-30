#!/usr/bin/env bash
# ============================================================================
# Script: prod-data-transform-selfhosted.sh
# ============================================================================
#
# DESCRIPTION:
#   Applies selfhosted-compatible transformations to the enterprise-selfhosted
#   database after a prod data import. Converts cloud-specific data (billing,
#   subscriptions, trials, cloud URLs) into selfhosted-compatible state.
#
# WHAT IT DOES:
#   1. Reads DATABASE_URL and dev credentials from .env.selfhosted.local
#   2. Upserts an admin user with the dev email/password from env
#   3. Sets all orgs to 'enterprise-selfhosted' plan with unlimited LOC
#   4. Clears cloud billing data (subscriptions, payments, upgrades)
#   5. Clears cloud URLs from instance_details
#   6. Clears trial flags from org_billing_state
#   7. Clears billing audit log
#
# PREREQUISITES:
#   - .env.selfhosted.local must exist with:
#     - DATABASE_URL pointing to enterprise-selfhosted database
#     - dev_enterprise_selfhosted_login_email
#     - dev_enterprise_selfhosted_login_password
#   - The database must already have data (run prod-data-import first)
#
# USAGE:
#   bash scripts/prod-data-transform-selfhosted.sh
#   # or
#   make prod-data-transform-selfhosted
#
# SAFETY:
#   - Only modifies the local enterprise-selfhosted database
#   - Never touches the prod database
#   - All operations are reversible (data is a copy)
#
# ============================================================================
set -euo pipefail
cd "$(dirname "$0")/.."

ENV_FILE=".env.selfhosted.local"

if [ ! -f "$ENV_FILE" ]; then
  echo "ERROR: $ENV_FILE not found"
  exit 1
fi

set -a
. "./$ENV_FILE"
set +a

if [ -z "${DATABASE_URL:-}" ]; then
  echo "ERROR: DATABASE_URL not set in $ENV_FILE"
  exit 1
fi

DEV_EMAIL="${dev_enterprise_selfhosted_login_email:-}"
DEV_PASSWORD="${dev_enterprise_selfhosted_login_password:-}"

if [ -z "$DEV_EMAIL" ] || [ -z "$DEV_PASSWORD" ]; then
  echo "ERROR: dev_enterprise_selfhosted_login_email and dev_enterprise_selfhosted_login_password must be set in $ENV_FILE"
  exit 1
fi

echo "============================================"
echo "  Enterprise-Selfhosted Data Transformation"
echo "============================================"
echo ""
echo "Target: $DATABASE_URL"
echo "Dev user: $DEV_EMAIL"
echo ""

# Generate bcrypt hash for the dev password
echo "Generating password hash..."
PASSWORD_HASH=$(/usr/bin/python3 -c "import bcrypt; print(bcrypt.hashpw(b'$DEV_PASSWORD', bcrypt.gensalt(10)).decode())")

echo "Applying transformations..."

psql "$DATABASE_URL" <<SQL
-- 1. Upsert admin user with dev credentials
INSERT INTO users (email, password_hash, first_name, last_name, created_at, updated_at)
VALUES ('$DEV_EMAIL', '$PASSWORD_HASH', 'Admin', 'Enterprise', NOW(), NOW())
ON CONFLICT (email) DO UPDATE SET
  password_hash = '$PASSWORD_HASH',
  updated_at = NOW();

-- Ensure the admin user has an org and owner role
DO \$\$
DECLARE
  admin_uid BIGINT;
  default_org BIGINT;
BEGIN
  SELECT id INTO admin_uid FROM users WHERE email = '$DEV_EMAIL';
  SELECT id INTO default_org FROM orgs ORDER BY id LIMIT 1;

  IF default_org IS NULL THEN
    INSERT INTO orgs (name, created_at, updated_at)
    VALUES ('Enterprise Selfhosted Org', NOW(), NOW())
    RETURNING id INTO default_org;
  END IF;

  -- Set default_org_id
  UPDATE users SET default_org_id = default_org WHERE id = admin_uid;

  -- Ensure owner role exists in user_roles
  INSERT INTO user_roles (user_id, org_id, role_id, created_at, updated_at)
  SELECT admin_uid, default_org, id, NOW(), NOW()
  FROM roles WHERE name = 'owner'
  ON CONFLICT DO NOTHING;
END
\$\$;

-- 2. Set all orgs to enterprise-selfhosted plan with unlimited LOC
UPDATE org_billing_state
SET
  current_plan_code = 'enterprise-selfhosted',
  loc_used_month = 0,
  loc_blocked = false,
  scheduled_plan_code = NULL,
  scheduled_plan_effective_at = NULL,
  updated_at = NOW();

-- Insert billing state for orgs that don't have one yet
INSERT INTO org_billing_state (org_id, current_plan_code, loc_used_month, loc_blocked, billing_period_start, billing_period_end, created_at, updated_at)
SELECT o.id, 'enterprise-selfhosted', 0, false, NOW(), NOW() + INTERVAL '1 month', NOW(), NOW()
FROM orgs o
LEFT JOIN org_billing_state obs ON o.id = obs.org_id
WHERE obs.id IS NULL;

-- 3. Clear cloud billing data
DELETE FROM subscription_payments;
DELETE FROM upgrade_requests;
DELETE FROM upgrade_request_events;
DELETE FROM upgrade_payment_attempts;
DELETE FROM upgrade_replacement_cutovers;

-- 4. Clear cloud URLs from instance_details
UPDATE instance_details
SET livereview_prod_url = '',
    updated_at = NOW()
WHERE livereview_prod_url IS NOT NULL AND livereview_prod_url != '';

-- 5. Clear trial flags
UPDATE org_billing_state
SET
  trial_started_at = NULL,
  trial_ends_at = NULL,
  trial_readonly = false,
  updated_at = NOW();

-- 6. Clear billing audit log
TRUNCATE org_billing_state_audit RESTART IDENTITY;

-- 7. Clear subscriptions (cloud-only)
DELETE FROM subscriptions;

SQL

echo ""
echo "============================================"
echo "  Transformation Complete"
echo "============================================"
echo ""
echo "Admin login:"
echo "  Email:    $DEV_EMAIL"
echo "  Password: $DEV_PASSWORD"
echo ""
echo "Plan: enterprise-selfhosted (unlimited LOC)"
echo "============================================"
