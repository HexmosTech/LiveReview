-- migrate:up
-- Bump the system-managed "default" tier from Gemini 2.5 Flash to 3.6 Flash.
UPDATE system_default_ai_configs
SET model_name = 'gemini-3.6-flash', updated_at = CURRENT_TIMESTAMP
WHERE tier_name = 'default' AND provider_name = 'gemini' AND model_name = 'gemini-2.5-flash';

-- migrate:down
UPDATE system_default_ai_configs
SET model_name = 'gemini-2.5-flash', updated_at = CURRENT_TIMESTAMP
WHERE tier_name = 'default' AND provider_name = 'gemini' AND model_name = 'gemini-3.6-flash';
