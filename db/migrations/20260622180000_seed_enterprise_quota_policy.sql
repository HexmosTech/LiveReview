-- migrate:up

-- The enterprise plan was added to plan_catalog after the initial quota_policy_catalog
-- seed ran (20260411170000). This migration backfills all provider policies for enterprise
-- using the same token/cost parameters as all other plans.
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
    'enterprise',
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
FROM (
    VALUES
        ('default',    5.0::double precision,  15.0::double precision),
        ('openai',     5.0::double precision,  15.0::double precision),
        ('gemini',     0.3::double precision,   2.5::double precision),
        ('googleai',   0.3::double precision,   2.5::double precision),
        ('claude',    15.0::double precision,  75.0::double precision),
        ('anthropic', 15.0::double precision,  75.0::double precision),
        ('deepseek',   1.0::double precision,   2.0::double precision),
        ('openrouter', 1.0::double precision,   2.0::double precision),
        ('local',      0.0::double precision,   0.0::double precision),
        ('ollama',     0.0::double precision,   0.0::double precision),
        ('atlas',      1.0::double precision,   2.0::double precision)
) AS rates(provider_key, input_per_million, output_per_million)
ON CONFLICT (plan_code, provider_key) DO NOTHING;

-- migrate:down

DELETE FROM quota_policy_catalog WHERE plan_code = 'enterprise';
