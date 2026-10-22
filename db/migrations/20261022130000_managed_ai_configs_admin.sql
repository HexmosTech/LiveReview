-- migrate:up
-- Managed AI admin page: a tier can hold several configurations (e.g. the old Vertex one and a new OpenRouter one),
-- exactly one of them active. GetSystemManagedConfig reads only the active row.
ALTER TABLE system_default_ai_configs DROP CONSTRAINT IF EXISTS system_default_ai_configs_tier_name_key;
CREATE UNIQUE INDEX IF NOT EXISTS uq_system_default_ai_configs_active_tier
    ON system_default_ai_configs (tier_name)
    WHERE is_active;
ALTER TABLE system_default_ai_configs ADD COLUMN IF NOT EXISTS label TEXT;
ALTER TABLE system_default_ai_configs ADD COLUMN IF NOT EXISTS updated_by_user_id BIGINT;

-- Orgs created through /gh-login?src=launch-trial may run reviews on the LiveReview model while on the Free plan.
ALTER TABLE org_billing_state ADD COLUMN IF NOT EXISTS free_managed_ai_granted_at TIMESTAMPTZ;

-- migrate:down
ALTER TABLE org_billing_state DROP COLUMN IF EXISTS free_managed_ai_granted_at;
ALTER TABLE system_default_ai_configs DROP COLUMN IF EXISTS updated_by_user_id;
ALTER TABLE system_default_ai_configs DROP COLUMN IF EXISTS label;
DROP INDEX IF EXISTS uq_system_default_ai_configs_active_tier;
-- Inactive rows have to go before the per-tier uniqueness can come back.
DELETE FROM system_default_ai_configs WHERE NOT is_active;
ALTER TABLE system_default_ai_configs ADD CONSTRAINT system_default_ai_configs_tier_name_key UNIQUE (tier_name);
