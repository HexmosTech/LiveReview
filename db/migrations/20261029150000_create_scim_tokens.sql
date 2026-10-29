-- migrate:up

-- SCIM clients (Okta, Azure AD, Google Workspace) authenticate with a single
-- static bearer token per org, per the SCIM 2.0 RFC - there is no user
-- session involved. Modeled directly on api_keys' hashing scheme
-- (internal/api/api_keys.go: GenerateAPIKey/HashAPIKey/GetKeyPrefix), but
-- kept as its own table rather than reusing api_keys since a SCIM token is
-- org-bound with no owning user.
CREATE TABLE IF NOT EXISTS scim_tokens (
    id BIGSERIAL PRIMARY KEY,
    org_id BIGINT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    token_prefix VARCHAR(16) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_scim_tokens_org_id ON scim_tokens(org_id);

-- migrate:down
DROP TABLE IF EXISTS scim_tokens;
