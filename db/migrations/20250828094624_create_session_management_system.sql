-- migrate:up

-- Unified token table for sessions, API keys, and refresh tokens
CREATE TABLE auth_tokens (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL, -- SHA256 hash of the actual token
    token_type VARCHAR(20) NOT NULL CHECK (token_type IN ('session', 'refresh', 'api_key')),
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    last_used_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    user_agent TEXT,
    ip_address INET,
    -- API token specific fields (for future use)
    permissions JSONB DEFAULT '{}',
    rate_limit_requests_per_hour INTEGER DEFAULT 1000,
    last_rate_limit_reset TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    requests_this_hour INTEGER DEFAULT 0,
    -- Token management
    revoked_at TIMESTAMP WITH TIME ZONE NULL,
    is_active BOOLEAN NOT NULL DEFAULT true
);

-- Performance optimized indexes for PostgreSQL
CREATE INDEX idx_auth_tokens_user_id ON auth_tokens(user_id);
CREATE INDEX idx_auth_tokens_hash ON auth_tokens(token_hash) WHERE is_active = true;
CREATE INDEX idx_auth_tokens_expires ON auth_tokens(expires_at) WHERE is_active = true;
CREATE INDEX idx_auth_tokens_type_user ON auth_tokens(token_type, user_id) WHERE is_active = true;
CREATE INDEX idx_auth_tokens_last_used ON auth_tokens(last_used_at) WHERE is_active = true;

-- Partial index for active sessions only (major performance boost)
CREATE INDEX idx_auth_tokens_active_sessions ON auth_tokens(user_id, last_used_at) 
WHERE token_type = 'session' AND is_active = true;

-- Composite index for cleanup operations
CREATE INDEX idx_auth_tokens_cleanup ON auth_tokens(token_type, expires_at, is_active);

-- Index for refresh token lookups
CREATE INDEX idx_auth_tokens_refresh ON auth_tokens(token_hash, token_type) 
WHERE token_type = 'refresh' AND is_active = true;

-- migrate:down

-- Drop indexes first (order matters for PostgreSQL)
DROP INDEX IF EXISTS idx_auth_tokens_refresh;
DROP INDEX IF EXISTS idx_auth_tokens_cleanup;
DROP INDEX IF EXISTS idx_auth_tokens_active_sessions;
DROP INDEX IF EXISTS idx_auth_tokens_last_used;
DROP INDEX IF EXISTS idx_auth_tokens_type_user;
DROP INDEX IF EXISTS idx_auth_tokens_expires;
DROP INDEX IF EXISTS idx_auth_tokens_hash;
DROP INDEX IF EXISTS idx_auth_tokens_user_id;

-- Drop table last
DROP TABLE IF EXISTS auth_tokens;

