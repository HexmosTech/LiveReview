-- migrate:up

CREATE TABLE webhook_registry (
    id SERIAL PRIMARY KEY,
    provider TEXT NOT NULL, -- 'github' or 'gitlab'
    provider_project_id TEXT NOT NULL,
    project_name TEXT NOT NULL,
    project_full_name TEXT NOT NULL,
    webhook_id TEXT NOT NULL,
    webhook_url TEXT NOT NULL,
    webhook_secret TEXT,
    webhook_name TEXT,
    events TEXT, -- comma-separated list
    status TEXT DEFAULT 'active',
    last_verified_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Optional: index for quick lookups
CREATE INDEX idx_webhook_registry_provider_project
    ON webhook_registry (provider, provider_project_id);

-- migrate:down

DROP INDEX IF EXISTS idx_webhook_registry_provider_project;
DROP TABLE IF EXISTS webhook_registry;
