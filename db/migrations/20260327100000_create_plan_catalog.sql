-- migrate:up

-- Catalog of selectable plans for LOC-based pricing.
CREATE TABLE IF NOT EXISTS plan_catalog (
    id BIGSERIAL PRIMARY KEY,
    plan_code VARCHAR(64) UNIQUE NOT NULL,
    display_name VARCHAR(120) NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    rank INTEGER NOT NULL,
    monthly_price_usd INTEGER NOT NULL,
    monthly_loc_limit BIGINT NOT NULL,
    feature_flags JSONB NOT NULL DEFAULT '[]'::jsonb,
    trial_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    trial_days INTEGER NOT NULL DEFAULT 0,
    envelope_show_price BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_plan_catalog_rank_non_negative CHECK (rank >= 0),
    CONSTRAINT chk_plan_catalog_price_non_negative CHECK (monthly_price_usd >= 0),
    CONSTRAINT chk_plan_catalog_loc_non_negative CHECK (monthly_loc_limit >= 0),
    CONSTRAINT chk_plan_catalog_trial_days_non_negative CHECK (trial_days >= 0),
    CONSTRAINT chk_plan_catalog_trial_config CHECK (
        (trial_enabled = TRUE AND trial_days > 0) OR
        (trial_enabled = FALSE)
    )
);

CREATE INDEX IF NOT EXISTS idx_plan_catalog_active_rank
    ON plan_catalog(active, rank);

CREATE OR REPLACE FUNCTION plan_catalog_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_plan_catalog_updated_at ON plan_catalog;
CREATE TRIGGER trg_plan_catalog_updated_at
BEFORE UPDATE ON plan_catalog
FOR EACH ROW
EXECUTE PROCEDURE plan_catalog_set_updated_at();

-- migrate:down

DROP TRIGGER IF EXISTS trg_plan_catalog_updated_at ON plan_catalog;
DROP FUNCTION IF EXISTS plan_catalog_set_updated_at();
DROP INDEX IF EXISTS idx_plan_catalog_active_rank;
DROP TABLE IF EXISTS plan_catalog;
