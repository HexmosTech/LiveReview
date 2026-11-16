-- migrate:up

-- 1. Tool Credits Columns on org_billing_state
-- tool_credits_used_month: Tracks credits consumed in the current billing cycle.
-- tool_credits_limit_month: Total tool credits allowed per monthly cycle based on the org's subscription plan.
-- Defaults to 0.0 so new/free orgs start with zero tool credits until upgraded to a paid plan.
ALTER TABLE public.org_billing_state
    ADD COLUMN IF NOT EXISTS tool_credits_used_month NUMERIC(18,4) NOT NULL DEFAULT 0.0,
    ADD COLUMN IF NOT EXISTS tool_credits_limit_month NUMERIC(18,4) NOT NULL DEFAULT 0.0;

-- 2. Non-negative constraints for credit usage and credit limit
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

-- 3. Free Plan Initialization
-- Ensure free organizations ('free', 'free_30k') have 0.0 tool credit limit.
-- Free plans do not include third-party tool execution credits.
UPDATE public.org_billing_state
SET tool_credits_limit_month = 0.0
WHERE current_plan_code IN ('free', 'free_30k') AND tool_credits_limit_month <> 0.0;

-- 4. Paid Plan Backfill
-- Backfill existing paid organizations with their plan-based tool credits limit:
--   Formula: (monthly_loc_limit / 100,000) * 50,000 tool credits
--   e.g. 100k LOC plan -> 50,000 credits, 200k LOC plan -> 100,000 credits.
-- Safety notes:
--   - Maps legacy 'team' plan code to 'team_32usd' to join plan_catalog correctly.
--   - Only sets tool_credits_limit_month where currently 0.0 (prevents overwriting custom limits).
--   - IMPORTANT: Leaves tool_credits_used_month untouched so deployments/migrations never reset credit usage.
UPDATE public.org_billing_state obs
SET tool_credits_limit_month = (pc.monthly_loc_limit / 100000.0) * 50000.0
FROM public.plan_catalog pc
WHERE pc.plan_code = CASE WHEN obs.current_plan_code = 'team' THEN 'team_32usd' ELSE obs.current_plan_code END
  AND pc.monthly_loc_limit >= 100000
  AND obs.tool_credits_limit_month = 0.0;

-- 5. Global Tier 2 Rule Taxonomy Cache
-- Caches AI-generated rule classifications across tools (e.g. semgrep, trivy, gitleaks, osv).
-- rule_key is the normalized composite lookup key ("<tool_name>:<rule_id>").
-- TEXT is used for taxonomy dimensions to accommodate dynamic classifications without schema locks.
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

-- Trigger to automatically update updated_at timestamp on rule taxonomy modification
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
EXECUTE FUNCTION tool_rule_taxonomies_set_updated_at();

-- 6. Immutable Tool Credit Deduction Ledger
-- Records every tool credit deduction transaction for financial auditing and idempotency.
-- Prevents double-charging if a review worker or tool dispatcher retries an execution.
-- breakdown stores per-tool credit cost details in JSONB.
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

-- Fast lookup of credit deductions by organization and review
CREATE INDEX IF NOT EXISTS idx_tool_credit_ledger_org_review ON public.tool_credit_ledger (org_id, review_id);

-- 7. Performance Index for Review Events
-- Accelerates review worker completion queries and event stream lookups
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
