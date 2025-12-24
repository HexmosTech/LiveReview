-- migrate:up
ALTER TABLE users ADD COLUMN IF NOT EXISTS onboarding_api_key TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_cli_used_at TIMESTAMP WITH TIME ZONE;

CREATE INDEX IF NOT EXISTS idx_users_onboarding_api_key ON users(onboarding_api_key) WHERE onboarding_api_key IS NOT NULL;

-- migrate:down
DROP INDEX IF EXISTS idx_users_onboarding_api_key;
ALTER TABLE users DROP COLUMN IF EXISTS last_cli_used_at;
ALTER TABLE users DROP COLUMN IF EXISTS onboarding_api_key;

