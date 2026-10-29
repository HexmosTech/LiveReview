-- migrate:up

-- Tracks which IdP subject (OIDC `sub` / SAML NameID) maps to which
-- LiveReview user, per org and provider. Needed so an incoming OIDC
-- back-channel logout token or SAML LogoutRequest - which only carries the
-- IdP's own subject identifier, not a LiveReview user id - can be resolved
-- to a user whose sessions should be revoked. Also holds the encrypted OIDC
-- refresh token used by the optional reconciliation poll (see
-- org_sso_configs.reconciliation_enabled) to silently re-check "is this
-- account still valid at the IdP" between logins.
CREATE TABLE IF NOT EXISTS sso_identities (
    id BIGSERIAL PRIMARY KEY,
    org_id BIGINT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider_type VARCHAR(16) NOT NULL CHECK (provider_type IN ('oidc', 'saml')),
    subject VARCHAR(512) NOT NULL,
    oidc_refresh_token_enc TEXT,
    last_checked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, provider_type, subject)
);

CREATE INDEX IF NOT EXISTS idx_sso_identities_user_id ON sso_identities(user_id);

CREATE OR REPLACE FUNCTION set_sso_identities_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_sso_identities_updated_at
    BEFORE UPDATE ON sso_identities
    FOR EACH ROW
    EXECUTE FUNCTION set_sso_identities_updated_at();

-- migrate:down
DROP TRIGGER IF EXISTS trg_sso_identities_updated_at ON sso_identities;
DROP FUNCTION IF EXISTS set_sso_identities_updated_at();
DROP TABLE IF EXISTS sso_identities;
