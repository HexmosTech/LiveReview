-- migrate:up
-- Add organization management fields
ALTER TABLE orgs ADD COLUMN IF NOT EXISTS settings JSONB DEFAULT '{}';
ALTER TABLE orgs ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE orgs ADD COLUMN IF NOT EXISTS created_by_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE orgs ADD COLUMN IF NOT EXISTS subscription_plan VARCHAR(50) DEFAULT 'free';
ALTER TABLE orgs ADD COLUMN IF NOT EXISTS max_users INTEGER DEFAULT 10;

-- Add organization settings index for JSON queries
CREATE INDEX idx_orgs_settings ON orgs USING GIN (settings) WHERE settings IS NOT NULL;
CREATE INDEX idx_orgs_active ON orgs(is_active, created_at);
CREATE INDEX idx_orgs_plan ON orgs(subscription_plan, is_active);

-- Add audit trail for user role changes
CREATE TABLE user_role_history (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id BIGINT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    old_role_id BIGINT NULL REFERENCES roles(id) ON DELETE SET NULL,
    new_role_id BIGINT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    changed_by_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reason TEXT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Indexes for audit trail queries
CREATE INDEX idx_user_role_history_user ON user_role_history(user_id, created_at);
CREATE INDEX idx_user_role_history_org ON user_role_history(org_id, created_at);
CREATE INDEX idx_user_role_history_changed_by ON user_role_history(changed_by_user_id, created_at);


-- migrate:down
-- Drop audit trail
DROP INDEX IF EXISTS idx_user_role_history_changed_by;
DROP INDEX IF EXISTS idx_user_role_history_org;
DROP INDEX IF EXISTS idx_user_role_history_user;
DROP TABLE IF EXISTS user_role_history;

-- Drop organization indexes
DROP INDEX IF EXISTS idx_orgs_plan;
DROP INDEX IF EXISTS idx_orgs_active;
DROP INDEX IF EXISTS idx_orgs_settings;

-- Remove organization columns
ALTER TABLE orgs DROP COLUMN IF EXISTS max_users;
ALTER TABLE orgs DROP COLUMN IF EXISTS subscription_plan;
ALTER TABLE orgs DROP COLUMN IF EXISTS created_by_user_id;
ALTER TABLE orgs DROP COLUMN IF EXISTS is_active;
ALTER TABLE orgs DROP COLUMN IF EXISTS settings;

