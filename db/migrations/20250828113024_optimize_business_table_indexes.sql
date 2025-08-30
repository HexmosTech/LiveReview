-- migrate:up
-- Optimize existing business tables for multi-tenant performance
-- These compound indexes support both org filtering and common query patterns

-- Reviews table optimization (has: org_id, created_at, status, connector_id)
CREATE INDEX IF NOT EXISTS idx_reviews_org_created ON reviews(org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_reviews_org_status ON reviews(org_id, status);
CREATE INDEX IF NOT EXISTS idx_reviews_org_connector ON reviews(org_id, connector_id);

-- AI Comments table optimization (has: org_id, review_id, created_at)
CREATE INDEX IF NOT EXISTS idx_ai_comments_org_review ON ai_comments(org_id, review_id);
CREATE INDEX IF NOT EXISTS idx_ai_comments_org_created ON ai_comments(org_id, created_at DESC);

-- AI Connectors table optimization (has: org_id, provider_name)
CREATE INDEX IF NOT EXISTS idx_ai_connectors_org_provider ON ai_connectors(org_id, provider_name);

-- Integration Tokens table optimization (has: org_id, provider, created_at)
CREATE INDEX IF NOT EXISTS idx_integration_tokens_org_provider ON integration_tokens(org_id, provider);
CREATE INDEX IF NOT EXISTS idx_integration_tokens_org_created ON integration_tokens(org_id, created_at);

-- Recent Activity table optimization (has: org_id, created_at, activity_type)
CREATE INDEX IF NOT EXISTS idx_recent_activity_org_created ON recent_activity(org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_recent_activity_org_type ON recent_activity(org_id, activity_type);

-- Dashboard Cache table optimization (has: org_id, updated_at, created_at - no cache_key or expires_at)
CREATE INDEX IF NOT EXISTS idx_dashboard_cache_org_updated ON dashboard_cache(org_id, updated_at DESC);

-- Webhook Registry table optimization (has: org_id, provider, status, created_at - no event_type)
CREATE INDEX IF NOT EXISTS idx_webhook_registry_org_provider ON webhook_registry(org_id, provider);
CREATE INDEX IF NOT EXISTS idx_webhook_registry_org_status ON webhook_registry(org_id, status);


-- migrate:down
-- Drop all the compound indexes
DROP INDEX IF EXISTS idx_webhook_registry_org_status;
DROP INDEX IF EXISTS idx_webhook_registry_org_provider;
DROP INDEX IF EXISTS idx_dashboard_cache_org_updated;
DROP INDEX IF EXISTS idx_recent_activity_org_type;
DROP INDEX IF EXISTS idx_recent_activity_org_created;
DROP INDEX IF EXISTS idx_integration_tokens_org_created;
DROP INDEX IF EXISTS idx_integration_tokens_org_provider;
DROP INDEX IF EXISTS idx_ai_connectors_org_provider;
DROP INDEX IF EXISTS idx_ai_comments_org_created;
DROP INDEX IF EXISTS idx_ai_comments_org_review;
DROP INDEX IF EXISTS idx_reviews_org_connector;
DROP INDEX IF EXISTS idx_reviews_org_status;
DROP INDEX IF EXISTS idx_reviews_org_created;

