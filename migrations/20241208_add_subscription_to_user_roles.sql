-- migrate:up
-- Add subscription-related columns to user_roles table
-- This enables subscription-based enforcement for cloud deployments

ALTER TABLE user_roles 
ADD COLUMN IF NOT EXISTS plan_type VARCHAR(50) DEFAULT 'free',
ADD COLUMN IF NOT EXISTS license_expires_at TIMESTAMP,
ADD COLUMN IF NOT EXISTS active_subscription_id BIGINT;

COMMENT ON COLUMN user_roles.plan_type IS 'User plan in this org: free, team, enterprise';
COMMENT ON COLUMN user_roles.license_expires_at IS 'When the license expires for this user in this org';
COMMENT ON COLUMN user_roles.active_subscription_id IS 'Reference to subscriptions table (future)';

-- CRITICAL: Create covering index for extremely fast lookups
-- This index allows PostgreSQL to serve queries from index alone (no heap access)
CREATE INDEX IF NOT EXISTS idx_user_roles_user_org_plan 
ON user_roles(user_id, org_id) 
INCLUDE (plan_type, license_expires_at);

COMMENT ON INDEX idx_user_roles_user_org_plan IS 'Covering index for subscription lookups - enables index-only scans for <2ms query time';

-- Verify index will be used (uncomment to test manually):
-- EXPLAIN (ANALYZE, BUFFERS) 
-- SELECT plan_type, license_expires_at 
-- FROM user_roles WHERE user_id = 1 AND org_id = 1;
-- Expected: "Index Only Scan using idx_user_roles_user_org_plan"
-- migrate:down
-- Rollback subscription-related changes to user_roles table

DROP INDEX IF EXISTS idx_user_roles_user_org_plan;

ALTER TABLE user_roles 
DROP COLUMN IF EXISTS active_subscription_id,
DROP COLUMN IF EXISTS license_expires_at,
DROP COLUMN IF EXISTS plan_type;
