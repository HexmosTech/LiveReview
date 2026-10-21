-- migrate:up
-- One GitHub App installation can belong to exactly one LiveReview org.
CREATE UNIQUE INDEX IF NOT EXISTS uq_integration_tokens_github_app_installation
    ON integration_tokens (provider_url, provider_app_id)
    WHERE token_type = 'GITHUB_APP';

-- How the org was created; 'gh_app' for orgs created through /gh-login. Analytics + onboarding banner only.
ALTER TABLE orgs ADD COLUMN IF NOT EXISTS signup_path TEXT;

-- migrate:down
DROP INDEX IF EXISTS uq_integration_tokens_github_app_installation;
ALTER TABLE orgs DROP COLUMN IF EXISTS signup_path;
