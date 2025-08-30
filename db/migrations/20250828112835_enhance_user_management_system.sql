-- migrate:up
-- Enhance users table for direct admin-managed user creation
ALTER TABLE users ADD COLUMN IF NOT EXISTS first_name VARCHAR(100);
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_name VARCHAR(100);
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMP NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS created_by_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS deactivated_at TIMESTAMP NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS deactivated_by_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_reset_required BOOLEAN NOT NULL DEFAULT false;

-- User management audit trail for compliance and debugging
CREATE TABLE user_management_audit (
    id BIGSERIAL PRIMARY KEY,
    org_id BIGINT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    target_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    performed_by_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    action VARCHAR(50) NOT NULL, -- 'created', 'updated', 'deactivated', 'role_changed', 'password_reset'
    details JSONB DEFAULT '{}', -- Additional context about the action
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Performance indexes for user management queries
CREATE INDEX idx_users_org_active ON users(id) WHERE is_active = true;
CREATE INDEX idx_users_created_by ON users(created_by_user_id, created_at DESC);
CREATE INDEX idx_users_last_login ON users(last_login_at DESC) WHERE is_active = true;
CREATE INDEX idx_users_password_reset ON users(id) WHERE password_reset_required = true;

-- Audit trail indexes
CREATE INDEX idx_audit_target_time ON user_management_audit(target_user_id, created_at DESC);
CREATE INDEX idx_audit_org_action ON user_management_audit(org_id, action, created_at DESC);
CREATE INDEX idx_audit_performed_by ON user_management_audit(performed_by_user_id, created_at DESC);


-- migrate:down
-- Drop audit trail
DROP INDEX IF EXISTS idx_audit_performed_by;
DROP INDEX IF EXISTS idx_audit_org_action;
DROP INDEX IF EXISTS idx_audit_target_time;
DROP TABLE IF EXISTS user_management_audit;

-- Drop user management indexes
DROP INDEX IF EXISTS idx_users_password_reset;
DROP INDEX IF EXISTS idx_users_last_login;
DROP INDEX IF EXISTS idx_users_created_by;
DROP INDEX IF EXISTS idx_users_org_active;

-- Remove user management columns
ALTER TABLE users DROP COLUMN IF EXISTS password_reset_required;
ALTER TABLE users DROP COLUMN IF EXISTS deactivated_by_user_id;
ALTER TABLE users DROP COLUMN IF EXISTS deactivated_at;
ALTER TABLE users DROP COLUMN IF EXISTS created_by_user_id;
ALTER TABLE users DROP COLUMN IF EXISTS last_login_at;
ALTER TABLE users DROP COLUMN IF EXISTS is_active;
ALTER TABLE users DROP COLUMN IF EXISTS last_name;
ALTER TABLE users DROP COLUMN IF EXISTS first_name;

