-- migrate:up

ALTER TABLE loc_usage_ledger
    ADD COLUMN IF NOT EXISTS actor_kind VARCHAR(16),
    ADD COLUMN IF NOT EXISTS actor_email_snapshot VARCHAR(320);

UPDATE loc_usage_ledger
SET actor_kind = CASE
    WHEN user_id IS NOT NULL THEN 'member'
    WHEN COALESCE(metadata->>'actor_email', '') <> '' THEN 'system'
    ELSE 'unknown'
END
WHERE actor_kind IS NULL OR btrim(actor_kind) = '';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'chk_loc_usage_ledger_actor_kind'
    ) THEN
        ALTER TABLE loc_usage_ledger
            ADD CONSTRAINT chk_loc_usage_ledger_actor_kind
            CHECK (actor_kind IS NULL OR actor_kind IN ('member', 'system', 'unknown'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_loc_usage_ledger_org_period_user_time
    ON loc_usage_ledger(org_id, billing_period_start, user_id, accounted_at DESC)
    WHERE status = 'accounted';

-- migrate:down

DROP INDEX IF EXISTS idx_loc_usage_ledger_org_period_user_time;

ALTER TABLE loc_usage_ledger
    DROP CONSTRAINT IF EXISTS chk_loc_usage_ledger_actor_kind;

ALTER TABLE loc_usage_ledger
    DROP COLUMN IF EXISTS actor_email_snapshot,
    DROP COLUMN IF EXISTS actor_kind;
