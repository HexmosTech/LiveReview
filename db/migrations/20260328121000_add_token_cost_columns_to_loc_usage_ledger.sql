-- migrate:up

ALTER TABLE loc_usage_ledger
    ADD COLUMN IF NOT EXISTS provider VARCHAR(64),
    ADD COLUMN IF NOT EXISTS model VARCHAR(128),
    ADD COLUMN IF NOT EXISTS pricing_version VARCHAR(64),
    ADD COLUMN IF NOT EXISTS input_tokens BIGINT,
    ADD COLUMN IF NOT EXISTS output_tokens BIGINT,
    ADD COLUMN IF NOT EXISTS llm_cost_usd DOUBLE PRECISION;

ALTER TABLE loc_usage_ledger
    DROP CONSTRAINT IF EXISTS chk_loc_usage_ledger_input_tokens_non_negative,
    DROP CONSTRAINT IF EXISTS chk_loc_usage_ledger_output_tokens_non_negative,
    DROP CONSTRAINT IF EXISTS chk_loc_usage_ledger_cost_non_negative;

ALTER TABLE loc_usage_ledger
    ADD CONSTRAINT chk_loc_usage_ledger_input_tokens_non_negative CHECK (input_tokens IS NULL OR input_tokens >= 0),
    ADD CONSTRAINT chk_loc_usage_ledger_output_tokens_non_negative CHECK (output_tokens IS NULL OR output_tokens >= 0),
    ADD CONSTRAINT chk_loc_usage_ledger_cost_non_negative CHECK (llm_cost_usd IS NULL OR llm_cost_usd >= 0);

CREATE INDEX IF NOT EXISTS idx_loc_usage_ledger_org_accounted_tokens
    ON loc_usage_ledger(org_id, accounted_at DESC)
    WHERE input_tokens IS NOT NULL OR output_tokens IS NOT NULL OR llm_cost_usd IS NOT NULL;

-- migrate:down

DROP INDEX IF EXISTS idx_loc_usage_ledger_org_accounted_tokens;

ALTER TABLE loc_usage_ledger
    DROP CONSTRAINT IF EXISTS chk_loc_usage_ledger_input_tokens_non_negative,
    DROP CONSTRAINT IF EXISTS chk_loc_usage_ledger_output_tokens_non_negative,
    DROP CONSTRAINT IF EXISTS chk_loc_usage_ledger_cost_non_negative;

ALTER TABLE loc_usage_ledger
    DROP COLUMN IF EXISTS llm_cost_usd,
    DROP COLUMN IF EXISTS output_tokens,
    DROP COLUMN IF EXISTS input_tokens,
    DROP COLUMN IF EXISTS pricing_version,
    DROP COLUMN IF EXISTS model,
    DROP COLUMN IF EXISTS provider;
