-- migrate:up

-- Lifecycle events for LOC pricing (thresholds, resets, plan changes, trial transitions).
CREATE TABLE IF NOT EXISTS loc_lifecycle_log (
    id BIGSERIAL PRIMARY KEY,
    org_id BIGINT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    event_type VARCHAR(80) NOT NULL,
    threshold_percent INTEGER,
    usage_ledger_id BIGINT REFERENCES loc_usage_ledger(id) ON DELETE SET NULL,
    plan_code VARCHAR(64) REFERENCES plan_catalog(plan_code),
    event_key VARCHAR(255) NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    notified_email BOOLEAN NOT NULL DEFAULT FALSE,
    notified_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_loc_lifecycle_threshold_range CHECK (
        threshold_percent IS NULL OR (threshold_percent >= 0 AND threshold_percent <= 100)
    ),
    CONSTRAINT uq_loc_lifecycle_org_event_key UNIQUE (org_id, event_key)
);

CREATE INDEX IF NOT EXISTS idx_loc_lifecycle_log_org_created
    ON loc_lifecycle_log(org_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_loc_lifecycle_log_event_type
    ON loc_lifecycle_log(event_type);

CREATE INDEX IF NOT EXISTS idx_loc_lifecycle_log_email_pending
    ON loc_lifecycle_log(notified_email, created_at)
    WHERE notified_email = FALSE;

-- migrate:down

DROP INDEX IF EXISTS idx_loc_lifecycle_log_email_pending;
DROP INDEX IF EXISTS idx_loc_lifecycle_log_event_type;
DROP INDEX IF EXISTS idx_loc_lifecycle_log_org_created;
DROP TABLE IF EXISTS loc_lifecycle_log;
