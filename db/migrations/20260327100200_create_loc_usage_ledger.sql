-- migrate:up

-- Immutable per-operation LOC accounting ledger.
CREATE TABLE IF NOT EXISTS loc_usage_ledger (
    id BIGSERIAL PRIMARY KEY,
    org_id BIGINT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    review_id BIGINT REFERENCES reviews(id) ON DELETE SET NULL,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    operation_type VARCHAR(64) NOT NULL,
    trigger_source VARCHAR(64) NOT NULL,
    operation_id VARCHAR(128) NOT NULL,
    idempotency_key VARCHAR(255) NOT NULL,
    billable_loc BIGINT NOT NULL,
    accounted_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    billing_period_start TIMESTAMP WITH TIME ZONE NOT NULL,
    billing_period_end TIMESTAMP WITH TIME ZONE NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'accounted',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_loc_usage_ledger_billable_positive CHECK (billable_loc > 0),
    CONSTRAINT chk_loc_usage_ledger_period_valid CHECK (billing_period_end > billing_period_start),
    CONSTRAINT chk_loc_usage_ledger_status_valid CHECK (status IN ('accounted', 'ignored')),
    CONSTRAINT uq_loc_usage_ledger_org_idempotency UNIQUE (org_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_loc_usage_ledger_org_time
    ON loc_usage_ledger(org_id, accounted_at DESC);

CREATE INDEX IF NOT EXISTS idx_loc_usage_ledger_org_review
    ON loc_usage_ledger(org_id, review_id)
    WHERE review_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_loc_usage_ledger_org_user
    ON loc_usage_ledger(org_id, user_id)
    WHERE user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_loc_usage_ledger_operation
    ON loc_usage_ledger(operation_type, trigger_source);

-- migrate:down

DROP INDEX IF EXISTS idx_loc_usage_ledger_operation;
DROP INDEX IF EXISTS idx_loc_usage_ledger_org_user;
DROP INDEX IF EXISTS idx_loc_usage_ledger_org_review;
DROP INDEX IF EXISTS idx_loc_usage_ledger_org_time;
DROP TABLE IF EXISTS loc_usage_ledger;
