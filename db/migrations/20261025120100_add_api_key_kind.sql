-- migrate:up

-- API key kinds. 'standard' keys keep today's behaviour (inherit the
-- creator's live role). 'audit_reader' keys are SIEM-only: they may call
-- audit read/export endpoints and nothing else.
ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS kind VARCHAR(20) NOT NULL DEFAULT 'standard';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_api_keys_kind'
    ) THEN
        ALTER TABLE api_keys
            ADD CONSTRAINT chk_api_keys_kind
            CHECK (kind IN ('standard', 'audit_reader'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_api_keys_org_kind
    ON api_keys (org_id, kind)
    WHERE revoked_at IS NULL;

-- migrate:down

DROP INDEX IF EXISTS idx_api_keys_org_kind;

ALTER TABLE api_keys DROP CONSTRAINT IF EXISTS chk_api_keys_kind;
ALTER TABLE api_keys DROP COLUMN IF EXISTS kind;
