-- migrate:up
-- Gemini 3.6 Flash replaces 2.5 Flash as the default model
-- (20260905000000_update_default_gemini_flash_model.sql). Its promotional rate
-- ($0.75 / $3.75 per M input/output tokens, valid through 2026-12-31) differs
-- from 2.5 Flash's ($0.30 / $2.50), so the shared "gemini"/"googleai"
-- provider_key rows must move too or every plan keeps billing at the old rate.
-- Flash-Lite rows (provider_key gemini_flash_lite/googleai_flash_lite, added in
-- 20260702130000_fix_gemini_flash_lite_pricing.sql) are untouched.
UPDATE quota_policy_catalog
SET input_cost_per_million_tokens_usd = 0.75,
    output_cost_per_million_tokens_usd = 3.75,
    updated_at = CURRENT_TIMESTAMP
WHERE provider_key IN ('gemini', 'googleai');

-- migrate:down
UPDATE quota_policy_catalog
SET input_cost_per_million_tokens_usd = 0.3,
    output_cost_per_million_tokens_usd = 2.5,
    updated_at = CURRENT_TIMESTAMP
WHERE provider_key IN ('gemini', 'googleai');
