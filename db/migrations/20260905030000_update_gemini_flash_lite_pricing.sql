-- migrate:up
-- Gemini 3.5 Flash-Lite replaces 2.5 Flash-Lite as the helper model
-- (20260905020000_update_default_gemini_flash_lite_model.sql). Its paid rate
-- ($0.30 / $2.50 per M input/output tokens) differs from 2.5 Flash-Lite's
-- ($0.10 / $0.40), so the dedicated gemini_flash_lite/googleai_flash_lite
-- provider_key rows (added in 20260702130000_fix_gemini_flash_lite_pricing.sql)
-- must move too.
UPDATE quota_policy_catalog
SET input_cost_per_million_tokens_usd = 0.30,
    output_cost_per_million_tokens_usd = 2.50,
    updated_at = CURRENT_TIMESTAMP
WHERE provider_key IN ('gemini_flash_lite', 'googleai_flash_lite');

-- migrate:down
UPDATE quota_policy_catalog
SET input_cost_per_million_tokens_usd = 0.10,
    output_cost_per_million_tokens_usd = 0.40,
    updated_at = CURRENT_TIMESTAMP
WHERE provider_key IN ('gemini_flash_lite', 'googleai_flash_lite');
