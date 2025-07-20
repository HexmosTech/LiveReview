-- migrate:up
CREATE TABLE integration_tokens (
    id BIGSERIAL PRIMARY KEY,
    provider TEXT NOT NULL,  -- e.g., 'github', 'gitlab', 'bitbucket'
    provider_app_id TEXT NOT NULL,  -- e.g., GitHub App ID, GitLab client ID
    access_token TEXT NOT NULL,
    refresh_token TEXT,  -- optional; depends on provider
    token_type TEXT,  -- e.g., 'Bearer', 'JWT'
    scope TEXT,  -- optional OAuth scopes
    expires_at TIMESTAMPTZ,  -- nullable
    metadata JSONB DEFAULT '{}'::jsonb,  -- arbitrary per-provider/tenant data
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- migrate:down
DROP TABLE IF EXISTS integration_tokens;
