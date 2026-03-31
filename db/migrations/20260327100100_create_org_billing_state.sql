-- migrate:up

-- Per-org LOC billing state and plan transition metadata.
CREATE TABLE IF NOT EXISTS org_billing_state (
    id BIGSERIAL PRIMARY KEY,
    org_id BIGINT NOT NULL UNIQUE REFERENCES orgs(id) ON DELETE CASCADE,
    current_plan_code VARCHAR(64) NOT NULL REFERENCES plan_catalog(plan_code),
    billing_period_start TIMESTAMP WITH TIME ZONE NOT NULL,
    billing_period_end TIMESTAMP WITH TIME ZONE NOT NULL,
    loc_used_month BIGINT NOT NULL DEFAULT 0,
    loc_blocked BOOLEAN NOT NULL DEFAULT FALSE,
    trial_started_at TIMESTAMP WITH TIME ZONE,
    trial_ends_at TIMESTAMP WITH TIME ZONE,
    trial_readonly BOOLEAN NOT NULL DEFAULT FALSE,
    scheduled_plan_code VARCHAR(64) REFERENCES plan_catalog(plan_code),
    scheduled_plan_effective_at TIMESTAMP WITH TIME ZONE,
    last_reset_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_org_billing_period_valid CHECK (billing_period_end > billing_period_start),
    CONSTRAINT chk_org_billing_loc_used_non_negative CHECK (loc_used_month >= 0),
    CONSTRAINT chk_org_billing_trial_window_valid CHECK (
        trial_ends_at IS NULL OR trial_started_at IS NULL OR trial_ends_at > trial_started_at
    ),
    CONSTRAINT chk_org_billing_schedule_pair CHECK (
        (scheduled_plan_code IS NULL AND scheduled_plan_effective_at IS NULL) OR
        (scheduled_plan_code IS NOT NULL AND scheduled_plan_effective_at IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_org_billing_current_plan
    ON org_billing_state(current_plan_code);

CREATE INDEX IF NOT EXISTS idx_org_billing_scheduled_effective
    ON org_billing_state(scheduled_plan_effective_at)
    WHERE scheduled_plan_effective_at IS NOT NULL;

CREATE OR REPLACE FUNCTION org_billing_state_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_org_billing_state_updated_at ON org_billing_state;
CREATE TRIGGER trg_org_billing_state_updated_at
BEFORE UPDATE ON org_billing_state
FOR EACH ROW
EXECUTE PROCEDURE org_billing_state_set_updated_at();

-- migrate:down

DROP TRIGGER IF EXISTS trg_org_billing_state_updated_at ON org_billing_state;
DROP FUNCTION IF EXISTS org_billing_state_set_updated_at();
DROP INDEX IF EXISTS idx_org_billing_scheduled_effective;
DROP INDEX IF EXISTS idx_org_billing_current_plan;
DROP TABLE IF EXISTS org_billing_state;
