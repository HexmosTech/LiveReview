-- migrate:up

-- oidc_scopes is a space-separated scope list (see internal/sso/oidc.go's
-- strings.Fields(cfg.OIDCScopes)); some enterprise IdPs issue many custom
-- API scopes that can run well past 255 chars. Widen to TEXT, matching the
-- other unbounded fields on this table (saml_idp_metadata_xml).
ALTER TABLE org_sso_configs ALTER COLUMN oidc_scopes TYPE TEXT;

-- migrate:down
ALTER TABLE org_sso_configs ALTER COLUMN oidc_scopes TYPE VARCHAR(255);
