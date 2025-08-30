-- migrate:up
-- First, create a default organization for existing data
INSERT INTO orgs (name, created_at, updated_at) 
VALUES ('Default Organization', NOW(), NOW())
ON CONFLICT DO NOTHING;

-- Add org_id to business tables
ALTER TABLE ai_comments ADD COLUMN org_id BIGINT NOT NULL DEFAULT 1 REFERENCES orgs(id);
ALTER TABLE ai_connectors ADD COLUMN org_id BIGINT NOT NULL DEFAULT 1 REFERENCES orgs(id);
ALTER TABLE integration_tokens ADD COLUMN org_id BIGINT NOT NULL DEFAULT 1 REFERENCES orgs(id);
ALTER TABLE reviews ADD COLUMN org_id BIGINT NOT NULL DEFAULT 1 REFERENCES orgs(id);
ALTER TABLE webhook_registry ADD COLUMN org_id BIGINT NOT NULL DEFAULT 1 REFERENCES orgs(id);
ALTER TABLE recent_activity ADD COLUMN org_id BIGINT NOT NULL DEFAULT 1 REFERENCES orgs(id);
ALTER TABLE dashboard_cache ADD COLUMN org_id BIGINT NOT NULL DEFAULT 1 REFERENCES orgs(id);

-- Add indexes for org_id lookups
CREATE INDEX idx_ai_comments_org_id ON ai_comments(org_id);
CREATE INDEX idx_ai_connectors_org_id ON ai_connectors(org_id);
CREATE INDEX idx_integration_tokens_org_id ON integration_tokens(org_id);
CREATE INDEX idx_reviews_org_id ON reviews(org_id);
CREATE INDEX idx_webhook_registry_org_id ON webhook_registry(org_id);
CREATE INDEX idx_recent_activity_org_id ON recent_activity(org_id);
CREATE INDEX idx_dashboard_cache_org_id ON dashboard_cache(org_id);

-- migrate:down
-- Remove indexes
DROP INDEX IF EXISTS idx_dashboard_cache_org_id;
DROP INDEX IF EXISTS idx_recent_activity_org_id;
DROP INDEX IF EXISTS idx_webhook_registry_org_id;
DROP INDEX IF EXISTS idx_reviews_org_id;
DROP INDEX IF EXISTS idx_integration_tokens_org_id;
DROP INDEX IF EXISTS idx_ai_connectors_org_id;
DROP INDEX IF EXISTS idx_ai_comments_org_id;

-- Remove columns
ALTER TABLE dashboard_cache DROP COLUMN IF EXISTS org_id;
ALTER TABLE recent_activity DROP COLUMN IF EXISTS org_id;
ALTER TABLE webhook_registry DROP COLUMN IF EXISTS org_id;
ALTER TABLE reviews DROP COLUMN IF EXISTS org_id;
ALTER TABLE integration_tokens DROP COLUMN IF EXISTS org_id;
ALTER TABLE ai_connectors DROP COLUMN IF EXISTS org_id;
ALTER TABLE ai_comments DROP COLUMN IF EXISTS org_id;

