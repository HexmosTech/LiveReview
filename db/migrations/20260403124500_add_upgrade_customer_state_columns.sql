-- migrate:up

ALTER TABLE upgrade_requests
    ADD COLUMN IF NOT EXISTS customer_state VARCHAR(64),
    ADD COLUMN IF NOT EXISTS action_needed_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN IF NOT EXISTS last_customer_state_change_at TIMESTAMP WITH TIME ZONE;

UPDATE upgrade_requests
SET customer_state = CASE
    WHEN current_status IN ('failed', 'manual_review_required') THEN 'action_needed'
    WHEN current_status = 'resolved' AND plan_grant_applied = TRUE THEN 'resolved'
    ELSE 'processing'
END
WHERE customer_state IS NULL OR btrim(customer_state) = '';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'chk_upgrade_requests_customer_state'
    ) THEN
        ALTER TABLE upgrade_requests
            ADD CONSTRAINT chk_upgrade_requests_customer_state
            CHECK (customer_state IS NULL OR customer_state IN ('processing', 'action_needed', 'resolved', 'failed'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_upgrade_requests_customer_state
    ON upgrade_requests(org_id, customer_state, updated_at DESC)
    WHERE customer_state IS NOT NULL;

-- migrate:down

DROP INDEX IF EXISTS idx_upgrade_requests_customer_state;

ALTER TABLE upgrade_requests
    DROP CONSTRAINT IF EXISTS chk_upgrade_requests_customer_state;

ALTER TABLE upgrade_requests
    DROP COLUMN IF EXISTS last_customer_state_change_at,
    DROP COLUMN IF EXISTS action_needed_at,
    DROP COLUMN IF EXISTS customer_state;
