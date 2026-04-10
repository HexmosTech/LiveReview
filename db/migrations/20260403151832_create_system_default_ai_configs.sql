-- migrate:up
CREATE TABLE system_default_ai_configs (
    id SERIAL PRIMARY KEY,
    tier_name VARCHAR(64) UNIQUE NOT NULL,
    provider_name VARCHAR(64) NOT NULL,
    model_name VARCHAR(128) NOT NULL,
    master_api_key TEXT NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Seed initial Gemini model
INSERT INTO system_default_ai_configs (tier_name, provider_name, model_name, master_api_key)
VALUES ('default', 'gemini', 'gemini-2.5-flash', '');

-- migrate:down
DROP TABLE IF EXISTS system_default_ai_configs;

