-- migrate:up

CREATE TABLE IF NOT EXISTS quota_policy_catalog (
    id BIGSERIAL PRIMARY KEY,
    plan_code VARCHAR(64) NOT NULL REFERENCES plan_catalog(plan_code) ON DELETE CASCADE,
    provider_key VARCHAR(64) NOT NULL,
    input_chars_per_loc INTEGER NOT NULL,
    output_chars_per_loc INTEGER NOT NULL,
    chars_per_token INTEGER NOT NULL,
    loc_budget_ratio DOUBLE PRECISION NOT NULL,
    context_budget_ratio DOUBLE PRECISION NOT NULL,
    ops_reserved_ratio DOUBLE PRECISION NOT NULL,
    input_cost_per_million_tokens_usd DOUBLE PRECISION NOT NULL,
    output_cost_per_million_tokens_usd DOUBLE PRECISION NOT NULL,
    rounding_scale INTEGER NOT NULL DEFAULT 6,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_quota_policy_catalog_plan_provider UNIQUE (plan_code, provider_key),
    CONSTRAINT chk_quota_policy_input_chars_positive CHECK (input_chars_per_loc > 0),
    CONSTRAINT chk_quota_policy_output_chars_positive CHECK (output_chars_per_loc > 0),
    CONSTRAINT chk_quota_policy_chars_per_token_positive CHECK (chars_per_token > 0),
    CONSTRAINT chk_quota_policy_loc_budget_ratio CHECK (loc_budget_ratio >= 0 AND loc_budget_ratio <= 1),
    CONSTRAINT chk_quota_policy_context_budget_ratio CHECK (context_budget_ratio >= 0 AND context_budget_ratio <= 1),
    CONSTRAINT chk_quota_policy_ops_reserved_ratio CHECK (ops_reserved_ratio >= 0 AND ops_reserved_ratio <= 1),
    CONSTRAINT chk_quota_policy_ratio_sum CHECK (
        abs((loc_budget_ratio + context_budget_ratio + ops_reserved_ratio) - 1.0) <= 0.000001
    ),
    CONSTRAINT chk_quota_policy_input_rate_non_negative CHECK (input_cost_per_million_tokens_usd >= 0),
    CONSTRAINT chk_quota_policy_output_rate_non_negative CHECK (output_cost_per_million_tokens_usd >= 0),
    CONSTRAINT chk_quota_policy_rounding_scale CHECK (rounding_scale >= 0 AND rounding_scale <= 12)
);

CREATE INDEX IF NOT EXISTS idx_quota_policy_catalog_lookup
    ON quota_policy_catalog(plan_code, provider_key)
    WHERE active = TRUE;

CREATE OR REPLACE FUNCTION quota_policy_catalog_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_quota_policy_catalog_updated_at ON quota_policy_catalog;
CREATE TRIGGER trg_quota_policy_catalog_updated_at
BEFORE UPDATE ON quota_policy_catalog
FOR EACH ROW
EXECUTE PROCEDURE quota_policy_catalog_set_updated_at();

CREATE TABLE IF NOT EXISTS quota_batch_settlements (
    id BIGSERIAL PRIMARY KEY,
    org_id BIGINT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    review_id BIGINT REFERENCES reviews(id) ON DELETE SET NULL,
    operation_type VARCHAR(64) NOT NULL,
    trigger_source VARCHAR(64) NOT NULL,
    operation_id VARCHAR(128) NOT NULL,
    idempotency_key VARCHAR(255) NOT NULL,
    batch_index INTEGER NOT NULL,
    plan_code VARCHAR(64) NOT NULL REFERENCES plan_catalog(plan_code),
    policy_provider_key VARCHAR(64) NOT NULL,
    pricing_version VARCHAR(64) NOT NULL,
    raw_loc_batch BIGINT NOT NULL,
    effective_loc_batch BIGINT NOT NULL,
    extra_effective_loc_batch BIGINT NOT NULL,
    diff_input_tokens_batch BIGINT NOT NULL,
    context_chars_batch BIGINT NOT NULL,
    context_tokens_batch BIGINT NOT NULL,
    allowed_context_tokens_batch BIGINT NOT NULL,
    extra_context_tokens_batch BIGINT NOT NULL,
    provider_total_input_tokens_batch BIGINT NOT NULL,
    output_tokens_batch BIGINT NOT NULL,
    input_cost_usd_batch DOUBLE PRECISION NOT NULL,
    output_cost_usd_batch DOUBLE PRECISION NOT NULL,
    total_cost_usd_batch DOUBLE PRECISION NOT NULL,
    context_tokens_per_loc_allowance DOUBLE PRECISION NOT NULL,
    accounted_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_quota_batch_settlements_dedupe UNIQUE (org_id, idempotency_key, batch_index),
    CONSTRAINT chk_quota_batch_index_positive CHECK (batch_index > 0),
    CONSTRAINT chk_quota_batch_raw_loc_non_negative CHECK (raw_loc_batch >= 0),
    CONSTRAINT chk_quota_batch_effective_loc_non_negative CHECK (effective_loc_batch >= 0),
    CONSTRAINT chk_quota_batch_extra_loc_non_negative CHECK (extra_effective_loc_batch >= 0),
    CONSTRAINT chk_quota_batch_diff_tokens_non_negative CHECK (diff_input_tokens_batch >= 0),
    CONSTRAINT chk_quota_batch_context_chars_non_negative CHECK (context_chars_batch >= 0),
    CONSTRAINT chk_quota_batch_context_tokens_non_negative CHECK (context_tokens_batch >= 0),
    CONSTRAINT chk_quota_batch_allowed_context_tokens_non_negative CHECK (allowed_context_tokens_batch >= 0),
    CONSTRAINT chk_quota_batch_extra_context_tokens_non_negative CHECK (extra_context_tokens_batch >= 0),
    CONSTRAINT chk_quota_batch_provider_input_tokens_non_negative CHECK (provider_total_input_tokens_batch >= 0),
    CONSTRAINT chk_quota_batch_output_tokens_non_negative CHECK (output_tokens_batch >= 0),
    CONSTRAINT chk_quota_batch_input_cost_non_negative CHECK (input_cost_usd_batch >= 0),
    CONSTRAINT chk_quota_batch_output_cost_non_negative CHECK (output_cost_usd_batch >= 0),
    CONSTRAINT chk_quota_batch_total_cost_non_negative CHECK (total_cost_usd_batch >= 0),
    CONSTRAINT chk_quota_batch_context_allowance_non_negative CHECK (context_tokens_per_loc_allowance >= 0)
);

CREATE INDEX IF NOT EXISTS idx_quota_batch_settlements_org_time
    ON quota_batch_settlements(org_id, accounted_at DESC);

CREATE INDEX IF NOT EXISTS idx_quota_batch_settlements_org_idempotency
    ON quota_batch_settlements(org_id, idempotency_key);

CREATE OR REPLACE FUNCTION quota_batch_settlements_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_quota_batch_settlements_updated_at ON quota_batch_settlements;
CREATE TRIGGER trg_quota_batch_settlements_updated_at
BEFORE UPDATE ON quota_batch_settlements
FOR EACH ROW
EXECUTE PROCEDURE quota_batch_settlements_set_updated_at();

CREATE TABLE IF NOT EXISTS quota_operation_aggregates (
    id BIGSERIAL PRIMARY KEY,
    org_id BIGINT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    review_id BIGINT REFERENCES reviews(id) ON DELETE SET NULL,
    operation_type VARCHAR(64) NOT NULL,
    trigger_source VARCHAR(64) NOT NULL,
    operation_id VARCHAR(128) NOT NULL,
    idempotency_key VARCHAR(255) NOT NULL,
    plan_code VARCHAR(64) NOT NULL REFERENCES plan_catalog(plan_code),
    provider VARCHAR(64),
    model VARCHAR(128),
    pricing_version VARCHAR(64) NOT NULL,
    batch_count INTEGER NOT NULL,
    raw_loc_total BIGINT NOT NULL,
    effective_loc_total BIGINT NOT NULL,
    extra_effective_loc_total BIGINT NOT NULL,
    diff_input_tokens_total BIGINT NOT NULL,
    context_chars_total BIGINT NOT NULL,
    context_tokens_total BIGINT NOT NULL,
    allowed_context_tokens_total BIGINT NOT NULL,
    extra_context_tokens_total BIGINT NOT NULL,
    provider_total_input_tokens_total BIGINT NOT NULL,
    output_tokens_total BIGINT NOT NULL,
    input_cost_usd_total DOUBLE PRECISION NOT NULL,
    output_cost_usd_total DOUBLE PRECISION NOT NULL,
    total_cost_usd_total DOUBLE PRECISION NOT NULL,
    finalized_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_quota_operation_aggregates_dedupe UNIQUE (org_id, idempotency_key),
    CONSTRAINT chk_quota_operation_batch_count_positive CHECK (batch_count > 0),
    CONSTRAINT chk_quota_operation_raw_loc_non_negative CHECK (raw_loc_total >= 0),
    CONSTRAINT chk_quota_operation_effective_loc_non_negative CHECK (effective_loc_total >= 0),
    CONSTRAINT chk_quota_operation_extra_loc_non_negative CHECK (extra_effective_loc_total >= 0),
    CONSTRAINT chk_quota_operation_diff_tokens_non_negative CHECK (diff_input_tokens_total >= 0),
    CONSTRAINT chk_quota_operation_context_chars_non_negative CHECK (context_chars_total >= 0),
    CONSTRAINT chk_quota_operation_context_tokens_non_negative CHECK (context_tokens_total >= 0),
    CONSTRAINT chk_quota_operation_allowed_context_tokens_non_negative CHECK (allowed_context_tokens_total >= 0),
    CONSTRAINT chk_quota_operation_extra_context_tokens_non_negative CHECK (extra_context_tokens_total >= 0),
    CONSTRAINT chk_quota_operation_provider_input_tokens_non_negative CHECK (provider_total_input_tokens_total >= 0),
    CONSTRAINT chk_quota_operation_output_tokens_non_negative CHECK (output_tokens_total >= 0),
    CONSTRAINT chk_quota_operation_input_cost_non_negative CHECK (input_cost_usd_total >= 0),
    CONSTRAINT chk_quota_operation_output_cost_non_negative CHECK (output_cost_usd_total >= 0),
    CONSTRAINT chk_quota_operation_total_cost_non_negative CHECK (total_cost_usd_total >= 0)
);

CREATE INDEX IF NOT EXISTS idx_quota_operation_aggregates_org_time
    ON quota_operation_aggregates(org_id, finalized_at DESC);

CREATE OR REPLACE FUNCTION quota_operation_aggregates_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_quota_operation_aggregates_updated_at ON quota_operation_aggregates;
CREATE TRIGGER trg_quota_operation_aggregates_updated_at
BEFORE UPDATE ON quota_operation_aggregates
FOR EACH ROW
EXECUTE PROCEDURE quota_operation_aggregates_set_updated_at();

INSERT INTO quota_policy_catalog (
    plan_code,
    provider_key,
    input_chars_per_loc,
    output_chars_per_loc,
    chars_per_token,
    loc_budget_ratio,
    context_budget_ratio,
    ops_reserved_ratio,
    input_cost_per_million_tokens_usd,
    output_cost_per_million_tokens_usd,
    rounding_scale,
    active
)
SELECT
    p.plan_code,
    rates.provider_key,
    120,
    87,
    4,
    0.3333333333,
    0.3333333333,
    0.3333333334,
    rates.input_per_million,
    rates.output_per_million,
    6,
    TRUE
FROM plan_catalog p
CROSS JOIN (
    VALUES
        ('default', 5.0::double precision, 15.0::double precision),
        ('openai', 5.0::double precision, 15.0::double precision),
        ('gemini', 0.3::double precision, 2.5::double precision),
        ('googleai', 0.3::double precision, 2.5::double precision),
        ('claude', 15.0::double precision, 75.0::double precision),
        ('anthropic', 15.0::double precision, 75.0::double precision),
        ('deepseek', 1.0::double precision, 2.0::double precision),
        ('openrouter', 1.0::double precision, 2.0::double precision),
        ('local', 0.0::double precision, 0.0::double precision),
        ('ollama', 0.0::double precision, 0.0::double precision)
) AS rates(provider_key, input_per_million, output_per_million)
ON CONFLICT (plan_code, provider_key) DO NOTHING;

-- migrate:down

DROP TRIGGER IF EXISTS trg_quota_operation_aggregates_updated_at ON quota_operation_aggregates;
DROP FUNCTION IF EXISTS quota_operation_aggregates_set_updated_at();
DROP INDEX IF EXISTS idx_quota_operation_aggregates_org_time;
DROP TABLE IF EXISTS quota_operation_aggregates;

DROP TRIGGER IF EXISTS trg_quota_batch_settlements_updated_at ON quota_batch_settlements;
DROP FUNCTION IF EXISTS quota_batch_settlements_set_updated_at();
DROP INDEX IF EXISTS idx_quota_batch_settlements_org_idempotency;
DROP INDEX IF EXISTS idx_quota_batch_settlements_org_time;
DROP TABLE IF EXISTS quota_batch_settlements;

DROP TRIGGER IF EXISTS trg_quota_policy_catalog_updated_at ON quota_policy_catalog;
DROP FUNCTION IF EXISTS quota_policy_catalog_set_updated_at();
DROP INDEX IF EXISTS idx_quota_policy_catalog_lookup;
DROP TABLE IF EXISTS quota_policy_catalog;
