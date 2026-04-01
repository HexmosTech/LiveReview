-- migrate:up

ALTER TABLE org_billing_state
    ADD COLUMN IF NOT EXISTS upgrade_loc_grant_current_cycle BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS upgrade_loc_grant_expires_at TIMESTAMP WITH TIME ZONE;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'chk_org_billing_upgrade_loc_grant_non_negative'
    ) THEN
        ALTER TABLE org_billing_state
            ADD CONSTRAINT chk_org_billing_upgrade_loc_grant_non_negative
            CHECK (upgrade_loc_grant_current_cycle >= 0);
    END IF;
END $$;

-- migrate:down

ALTER TABLE org_billing_state
    DROP CONSTRAINT IF EXISTS chk_org_billing_upgrade_loc_grant_non_negative;

ALTER TABLE org_billing_state
    DROP COLUMN IF EXISTS upgrade_loc_grant_expires_at,
    DROP COLUMN IF EXISTS upgrade_loc_grant_current_cycle;
