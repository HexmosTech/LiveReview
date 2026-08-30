-- migrate:up

-- Track tool credit budget per org per month
CREATE TABLE IF NOT EXISTS public.org_tool_billing_state (
    id BIGSERIAL PRIMARY KEY,
    org_id BIGINT NOT NULL UNIQUE REFERENCES public.orgs(id) ON DELETE CASCADE,
    credits_used_month NUMERIC(18,4) NOT NULL DEFAULT 0.0,
    credits_limit_month NUMERIC(18,4) NOT NULL DEFAULT 50000.0,
    billing_period_start TIMESTAMP WITH TIME ZONE NOT NULL,
    billing_period_end TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_tool_billing_period_valid CHECK (billing_period_end > billing_period_start),
    CONSTRAINT chk_tool_billing_used_non_negative CHECK (credits_used_month >= 0.0)
);

CREATE OR REPLACE FUNCTION org_tool_billing_state_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_org_tool_billing_state_updated_at ON public.org_tool_billing_state;
CREATE TRIGGER trg_org_tool_billing_state_updated_at
BEFORE UPDATE ON public.org_tool_billing_state
FOR EACH ROW
EXECUTE PROCEDURE org_tool_billing_state_set_updated_at();

-- Immutable ledger for tracking deductions
CREATE TABLE IF NOT EXISTS public.tool_credit_ledger (
    id BIGSERIAL PRIMARY KEY,
    org_id BIGINT NOT NULL REFERENCES public.orgs(id) ON DELETE CASCADE,
    review_id BIGINT REFERENCES public.reviews(id) ON DELETE SET NULL,
    credits_deducted NUMERIC(18,4) NOT NULL,
    idempotency_key VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_tool_credit_ledger_idempotency UNIQUE(org_id, idempotency_key)
);

-- migrate:down

DROP TABLE IF EXISTS public.tool_credit_ledger;
DROP TRIGGER IF EXISTS trg_org_tool_billing_state_updated_at ON public.org_tool_billing_state;
DROP FUNCTION IF EXISTS org_tool_billing_state_set_updated_at();
DROP TABLE IF EXISTS public.org_tool_billing_state;
