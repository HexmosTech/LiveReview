-- migrate:up
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
    'atlas',
    120,
    87,
    4,
    0.3333333333,
    0.3333333333,
    0.3333333334,
    1.0, -- input_cost_per_million_tokens_usd
    2.0, -- output_cost_per_million_tokens_usd
    6,
    TRUE
FROM plan_catalog p
ON CONFLICT (plan_code, provider_key) DO NOTHING;

-- migrate:down
DELETE FROM quota_policy_catalog WHERE provider_key = 'atlas';
