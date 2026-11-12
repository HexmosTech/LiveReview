-- migrate:up

-- Tool credits columns on org_billing_state (default limit 50,000 matches free tier monthly allowance)
ALTER TABLE public.org_billing_state
    ADD COLUMN IF NOT EXISTS tool_credits_used_month NUMERIC(18,4) NOT NULL DEFAULT 0.0,
    ADD COLUMN IF NOT EXISTS tool_credits_limit_month NUMERIC(18,4) NOT NULL DEFAULT 50000.0;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_org_billing_state_tool_credits_used'
    ) THEN
        ALTER TABLE public.org_billing_state
            ADD CONSTRAINT chk_org_billing_state_tool_credits_used CHECK (tool_credits_used_month >= 0);
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_org_billing_state_tool_credits_limit'
    ) THEN
        ALTER TABLE public.org_billing_state
            ADD CONSTRAINT chk_org_billing_state_tool_credits_limit CHECK (tool_credits_limit_month >= 0);
    END IF;
END $$;

-- Global Tier 2 Rule Taxonomy Cache
-- rule_key is the normalized composite lookup key ("<tool_name>:<rule_id>")
-- TEXT is used for taxonomy dimensions to accommodate dynamic AI-generated classifications without schema locks
CREATE TABLE IF NOT EXISTS public.tool_rule_taxonomies (
    rule_key    TEXT PRIMARY KEY,
    tool_name   TEXT NOT NULL,
    rule_id     TEXT NOT NULL,
    category    TEXT NOT NULL,
    subcategory TEXT NOT NULL,
    severity    TEXT NOT NULL,
    confidence  TEXT NOT NULL,
    type        TEXT NOT NULL,
    classifier_version INT NOT NULL DEFAULT 1,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT uq_tool_rule_taxonomies_tool_rule UNIQUE (tool_name, rule_id)
);

CREATE OR REPLACE FUNCTION tool_rule_taxonomies_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_tool_rule_taxonomies_updated_at ON public.tool_rule_taxonomies;
CREATE TRIGGER trg_tool_rule_taxonomies_updated_at
BEFORE UPDATE ON public.tool_rule_taxonomies
FOR EACH ROW
EXECUTE PROCEDURE tool_rule_taxonomies_set_updated_at();

-- Immutable ledger for tracking tool credit deductions
CREATE TABLE IF NOT EXISTS public.tool_credit_ledger (
    id BIGSERIAL PRIMARY KEY,
    org_id BIGINT NOT NULL REFERENCES public.orgs(id) ON DELETE RESTRICT,
    review_id BIGINT REFERENCES public.reviews(id) ON DELETE SET NULL,
    credits_deducted NUMERIC(18,4) NOT NULL CHECK (credits_deducted <> 0),
    breakdown JSONB NOT NULL DEFAULT '[]'::jsonb,
    idempotency_key VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_tool_credit_ledger_idempotency UNIQUE(org_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_tool_credit_ledger_org_review ON public.tool_credit_ledger (org_id, review_id);

-- Performance index for review worker tool completion queries
CREATE INDEX IF NOT EXISTS idx_review_events_review_event_type ON public.review_events (review_id, event_type);

-- migrate:down

DROP INDEX IF EXISTS idx_review_events_review_event_type;
DROP INDEX IF EXISTS idx_tool_credit_ledger_org_review;
DROP TABLE IF EXISTS public.tool_credit_ledger;

DROP TRIGGER IF EXISTS trg_tool_rule_taxonomies_updated_at ON public.tool_rule_taxonomies;
DROP FUNCTION IF EXISTS tool_rule_taxonomies_set_updated_at();
DROP TABLE IF EXISTS public.tool_rule_taxonomies;

ALTER TABLE public.org_billing_state
    DROP CONSTRAINT IF EXISTS chk_org_billing_state_tool_credits_used,
    DROP CONSTRAINT IF EXISTS chk_org_billing_state_tool_credits_limit,
    DROP COLUMN IF EXISTS tool_credits_used_month,
    DROP COLUMN IF EXISTS tool_credits_limit_month;
