-- migrate:up
-- Google retired Gemini 2.5 Flash / Flash-Lite on Vertex AI on 2026-10-20. The managed tiers run on Gemini Enterprise
-- (Vertex AI), so the earlier bumps (which matched provider_name = 'gemini') never applied to them.
-- Gemini 3.x is only served from the global and us/eu multi-region locations, so the tier also records its location.
ALTER TABLE system_default_ai_configs ADD COLUMN IF NOT EXISTS gcp_location TEXT;

UPDATE system_default_ai_configs
SET model_name = 'gemini-3.8-flash', gcp_location = 'us', updated_at = CURRENT_TIMESTAMP
WHERE tier_name = 'default' AND provider_name = 'gemini-enterprise' AND model_name = 'gemini-2.5-flash';

UPDATE system_default_ai_configs
SET model_name = 'gemini-3.5-flash-lite', gcp_location = 'us', updated_at = CURRENT_TIMESTAMP
WHERE tier_name = 'default_lite' AND provider_name = 'gemini-enterprise' AND model_name = 'gemini-2.5-flash-lite';

-- migrate:down
UPDATE system_default_ai_configs
SET model_name = 'gemini-2.5-flash', updated_at = CURRENT_TIMESTAMP
WHERE tier_name = 'default' AND provider_name = 'gemini-enterprise' AND model_name = 'gemini-3.8-flash';

UPDATE system_default_ai_configs
SET model_name = 'gemini-2.5-flash-lite', updated_at = CURRENT_TIMESTAMP
WHERE tier_name = 'default_lite' AND provider_name = 'gemini-enterprise' AND model_name = 'gemini-3.5-flash-lite';

ALTER TABLE system_default_ai_configs DROP COLUMN IF EXISTS gcp_location;
