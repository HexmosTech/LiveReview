-- migrate:up

-- Per-organization SSO (OIDC/SAML) configuration. One org may have at most
-- one OIDC config and one SAML config active at a time. Secrets
-- (oidc_client_secret_enc) are stored AES-GCM encrypted at rest (see
-- internal/sso/secretbox.go) - never plaintext, never logged.
CREATE TABLE IF NOT EXISTS org_sso_configs (
    id BIGSERIAL PRIMARY KEY,
    org_id BIGINT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    provider_type VARCHAR(16) NOT NULL CHECK (provider_type IN ('oidc', 'saml')),
    display_name VARCHAR(255) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,

    -- OIDC fields
    oidc_issuer VARCHAR(512),
    oidc_client_id VARCHAR(512),
    oidc_client_secret_enc TEXT,
    oidc_scopes VARCHAR(255),

    -- SAML fields
    saml_idp_metadata_url VARCHAR(512),
    saml_idp_metadata_xml TEXT,
    saml_idp_entity_id VARCHAR(512),

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,

    UNIQUE (org_id, provider_type)
);

CREATE INDEX IF NOT EXISTS idx_org_sso_configs_org_enabled
    ON org_sso_configs(org_id) WHERE enabled = TRUE;

CREATE OR REPLACE FUNCTION org_sso_configs_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_org_sso_configs_updated_at ON org_sso_configs;
CREATE TRIGGER trg_org_sso_configs_updated_at
BEFORE UPDATE ON org_sso_configs
FOR EACH ROW
EXECUTE PROCEDURE org_sso_configs_set_updated_at();

-- migrate:down
DROP TRIGGER IF EXISTS trg_org_sso_configs_updated_at ON org_sso_configs;
DROP FUNCTION IF EXISTS org_sso_configs_set_updated_at();
DROP TABLE IF EXISTS org_sso_configs;
