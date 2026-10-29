-- migrate:up

-- Fallback deprovisioning for orgs that configure OIDC login but never wire
-- up SCIM at their IdP (a separate integration step many admins skip). Only
-- meaningful for provider_type='oidc' - SAML has no standing credential to
-- silently re-check, so the settings UI only exposes this toggle for OIDC.
ALTER TABLE org_sso_configs
    ADD COLUMN IF NOT EXISTS reconciliation_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS reconciliation_interval TEXT NOT NULL DEFAULT 'daily'
        CHECK (reconciliation_interval IN ('hourly', 'daily', 'weekly')),
    ADD COLUMN IF NOT EXISTS last_reconciled_at TIMESTAMPTZ;

-- migrate:down
ALTER TABLE org_sso_configs
    DROP COLUMN IF EXISTS reconciliation_enabled,
    DROP COLUMN IF EXISTS reconciliation_interval,
    DROP COLUMN IF EXISTS last_reconciled_at;
