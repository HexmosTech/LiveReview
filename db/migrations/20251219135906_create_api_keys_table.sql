-- migrate:up

CREATE TABLE api_keys (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id BIGINT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    key_hash VARCHAR(128) NOT NULL UNIQUE,
    key_prefix VARCHAR(16) NOT NULL,
    label VARCHAR(255) NOT NULL,
    scopes JSONB DEFAULT '[]'::JSONB,
    last_used_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP,
    revoked_at TIMESTAMP,
    CONSTRAINT valid_expiry CHECK (expires_at IS NULL OR expires_at > created_at)
);

CREATE INDEX idx_api_keys_user_id ON api_keys(user_id);
CREATE INDEX idx_api_keys_org_id ON api_keys(org_id);
CREATE INDEX idx_api_keys_key_hash ON api_keys(key_hash);
CREATE INDEX idx_api_keys_key_prefix ON api_keys(key_prefix);

COMMENT ON TABLE api_keys IS 'Personal API keys for programmatic access';
COMMENT ON COLUMN api_keys.key_hash IS 'SHA-256 hash of the API key';
COMMENT ON COLUMN api_keys.key_prefix IS 'First 8 chars of the key for display purposes';
COMMENT ON COLUMN api_keys.label IS 'User-provided label for the key';
COMMENT ON COLUMN api_keys.scopes IS 'JSON array of scope strings (e.g., ["read", "write"])';
COMMENT ON COLUMN api_keys.last_used_at IS 'Timestamp of last successful authentication';

-- migrate:down

DROP TABLE IF EXISTS api_keys;
