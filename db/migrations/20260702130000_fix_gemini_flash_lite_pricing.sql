-- migrate:up
-- The "gemini"/"googleai" provider_key rows price every Gemini model at
-- Gemini 2.5 Flash's rate ($0.30 / $2.50 per M input/output tokens). Gemini
-- 2.5 Flash-Lite (used as the helper model) actually costs $0.10 / $0.40 per
-- M tokens, roughly 3-6x cheaper — billing it at the Flash rate overstates
-- helper-stage cost. Add dedicated provider_key rows so callers that know
-- they're pricing a Flash-Lite call can resolve the correct rate; existing
-- "gemini"/"googleai" rows are left as-is (correct for Flash) so nothing
-- that doesn't yet ask for the lite variant changes behavior.
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
    plan_code,
    lite_key,
    input_chars_per_loc,
    output_chars_per_loc,
    chars_per_token,
    loc_budget_ratio,
    context_budget_ratio,
    ops_reserved_ratio,
    0.10,
    0.40,
    rounding_scale,
    active
FROM quota_policy_catalog qpc
CROSS JOIN LATERAL (
    VALUES
        (CASE qpc.provider_key WHEN 'gemini' THEN 'gemini_flash_lite' WHEN 'googleai' THEN 'googleai_flash_lite' END)
) AS lite(lite_key)
WHERE qpc.provider_key IN ('gemini', 'googleai')
ON CONFLICT (plan_code, provider_key) DO UPDATE
SET
    input_cost_per_million_tokens_usd = EXCLUDED.input_cost_per_million_tokens_usd,
    output_cost_per_million_tokens_usd = EXCLUDED.output_cost_per_million_tokens_usd,
    updated_at = NOW();

-- migrate:down
DELETE FROM quota_policy_catalog WHERE provider_key IN ('gemini_flash_lite', 'googleai_flash_lite');
