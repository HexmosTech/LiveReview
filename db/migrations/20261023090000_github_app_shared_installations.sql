-- migrate:up
-- One GitHub App installation can be connected to several LiveReview orgs (one connector per org), e.g. when two
-- people from the same GitHub organization each sign up. Each org still has at most one connector per installation.
DROP INDEX IF EXISTS uq_integration_tokens_github_app_installation;
CREATE UNIQUE INDEX IF NOT EXISTS uq_integration_tokens_github_app_installation_org
    ON integration_tokens (provider_url, provider_app_id, org_id)
    WHERE token_type = 'GITHUB_APP';

-- migrate:down
-- Fails while an installation is connected to more than one org; remove the extra connectors first.
DROP INDEX IF EXISTS uq_integration_tokens_github_app_installation_org;
CREATE UNIQUE INDEX IF NOT EXISTS uq_integration_tokens_github_app_installation
    ON integration_tokens (provider_url, provider_app_id)
    WHERE token_type = 'GITHUB_APP';
