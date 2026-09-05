-- migrate:up
-- Bump the system-managed "default_lite" tier from Gemini 2.5 Flash-Lite to 3.5 Flash-Lite.
UPDATE system_default_ai_configs
SET model_name = 'gemini-3.5-flash-lite', updated_at = CURRENT_TIMESTAMP
WHERE tier_name = 'default_lite' AND provider_name = 'gemini' AND model_name = 'gemini-2.5-flash-lite';

-- migrate:down
UPDATE system_default_ai_configs
SET model_name = 'gemini-2.5-flash-lite', updated_at = CURRENT_TIMESTAMP
WHERE tier_name = 'default_lite' AND provider_name = 'gemini' AND model_name = 'gemini-3.5-flash-lite';
